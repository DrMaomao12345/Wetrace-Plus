package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// ── 年度报告单趟扫描 ────────────────────────────────────────────
//
// 早先每个分区（概览 / 月度 / 星期 / 小时 / 类型 / 亮点 / 最早最晚 / 累计活跃天）
// 各自把所有消息表扫一遍，一份报告要跑 ~8 趟全表扫描。
//
// 这里改成：每张表只发一条 SELECT，把行拉进 Go，一次遍历同时喂给所有累加器。
// 收发方向原本靠 `LEFT JOIN Name2Id` 判断，现在把 Name2Id 预加载成内存 map，
// 连 JOIN 也省了。
//
// 时间范围故意不加下界 —— 「累计活跃天数」本来就要全历史，顺手在同一趟里算完，
// 不必再单独扫一次。

// annualAccum 是单趟扫描过程中所有分区共用的累加器
type annualAccum struct {
	segs []reportSegment

	// 概览
	total, sent, recv int
	contactSet        map[string]bool
	daySet            map[string]bool // 所选年份内有消息的日期
	lifetimeDays      map[string]bool // 全历史有消息的日期
	firstDate         string
	lastDate          string

	// 分布
	monthly  map[int]int
	weekday  map[int]int
	hourly   map[int]int
	msgTypes map[int]int

	// 亮点
	dailyCounts    map[string]int
	lateNightCount int
	times          []segTime // 供「最早/最晚」的 07:00 日界规则使用
}

type segTime struct {
	unix int64
	loc  *time.Location
}

func newAnnualAccum(segs []reportSegment) *annualAccum {
	return &annualAccum{
		segs:         segs,
		contactSet:   map[string]bool{},
		daySet:       map[string]bool{},
		lifetimeDays: map[string]bool{},
		monthly:      map[int]int{},
		weekday:      map[int]int{},
		hourly:       map[int]int{},
		msgTypes:     map[int]int{},
		dailyCounts:  map[string]int{},
	}
}

// segmentFor 找出某个时刻落在哪个时区段；不在任何段内返回 nil
func (a *annualAccum) segmentFor(unix int64) *reportSegment {
	t := time.Unix(unix, 0)
	for i := range a.segs {
		s := &a.segs[i]
		if !t.Before(s.start) && !t.After(s.end) {
			return s
		}
	}
	return nil
}

// addRow 把一行消息喂给所有累加器。isSelf 已由调用方按平台规则算好。
func (a *annualAccum) addRow(unix int64, localType int, talker string, isSelf bool) {
	if localType == 10000 {
		return // 系统消息一律不计（与各分区原有口径一致）
	}

	// 累计活跃天数用默认时区（第一段）即可，它只关心「哪些日子有消息」
	lifeLoc := time.Local
	if len(a.segs) > 0 {
		lifeLoc = a.segs[0].loc
	}
	a.lifetimeDays[time.Unix(unix, 0).In(lifeLoc).Format("2006-01-02")] = true

	seg := a.segmentFor(unix)
	if seg == nil {
		return // 不在所选年份内
	}
	t := time.Unix(unix, 0).In(seg.loc)
	day := t.Format("2006-01-02")

	a.total++
	if isSelf {
		a.sent++
	} else {
		a.recv++
	}
	a.contactSet[talker] = true

	a.daySet[day] = true
	if a.firstDate == "" || day < a.firstDate {
		a.firstDate = day
	}
	if day > a.lastDate {
		a.lastDate = day
	}

	a.monthly[int(t.Month())]++
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // 与原实现一致：周日记为 7
	}
	a.weekday[wd]++
	a.hourly[t.Hour()]++
	a.msgTypes[localType]++

	a.dailyCounts[day]++
	if h := t.Hour(); h >= 23 || h < 5 {
		a.lateNightCount++ // 与原 SQL 一致：23 点及以后，或凌晨 5 点前
	}
	a.times = append(a.times, segTime{unix: unix, loc: seg.loc})
}

// scanAnnualSinglePass 一趟扫完所有分片，产出全部分区所需的原始累加值。
func (r *Repository) scanAnnualSinglePass(ctx context.Context, segs []reportSegment,
	allow func(string) bool) *annualAccum {

	acc := newAnnualAccum(segs)
	if len(segs) == 0 {
		return acc
	}
	myWxid := r.getCurrentUserWxid(ctx)
	talkerMD5Map := r.getTalkerMD5Map(ctx)

	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			r.scanAnnualV3(ctx, db, acc)
			continue
		}
		r.scanAnnualV4(ctx, db, acc, talkerMD5Map, myWxid, allow)
	}
	return acc
}

// scanAnnualV4 扫一个 v4 分片：先把 Name2Id 读进内存，然后每张消息表一条 SELECT。
func (r *Repository) scanAnnualV4(ctx context.Context, db *sql.DB, acc *annualAccum,
	talkerMD5Map map[string]string, myWxid string, allow func(string) bool) {

	// Name2Id: rowid → user_name。原来每条消息都要 JOIN 一次，这里读一次就够。
	name2id := map[int64]string{}
	if rows, err := db.QueryContext(ctx, "SELECT rowid, user_name FROM Name2Id"); err == nil {
		for rows.Next() {
			var id int64
			var name string
			if rows.Scan(&id, &name) == nil {
				name2id[id] = name
			}
		}
		rows.Close()
	}

	for _, tbl := range r.listMsgTables(ctx, db) {
		if !allow(tbl) {
			continue
		}
		talker := talkerMD5Map[trimMsgPrefix(tbl)]
		if talker == "" {
			talker = "unknown"
		}
		isGroup := isChatroomTalker(talker)

		rows, err := db.QueryContext(ctx, fmt.Sprintf(
			"SELECT COALESCE(create_time,0), COALESCE(local_type,0) & 4294967295, "+
				"COALESCE(status,0), COALESCE(real_sender_id,0) FROM %s", tbl))
		if err != nil {
			continue
		}
		for rows.Next() {
			var ts int64
			var localType int
			var status int
			var senderID int64
			if rows.Scan(&ts, &localType, &status, &senderID) != nil {
				continue
			}
			// 方向判定与原 SQL 完全一致，只是把 JOIN 换成了内存查表
			isSelf := status == 2 || senderID == 0
			if !isSelf {
				sender := name2id[senderID]
				if isGroup || talker == "unknown" {
					isSelf = sender == myWxid && myWxid != ""
				} else {
					isSelf = sender != talker
				}
			}
			acc.addRow(ts, localType, talker, isSelf)
		}
		rows.Close()
	}
}

// scanAnnualV3 扫一个 v3 分片（单张 MSG 大表）
func (r *Repository) scanAnnualV3(ctx context.Context, db *sql.DB, acc *annualAccum) {
	rows, err := db.QueryContext(ctx,
		"SELECT CreateTime/1000, COALESCE(Type,0), COALESCE(IsSender,0), COALESCE(StrTalker,'') FROM MSG")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ts int64
		var msgType, isSender int
		var talker string
		if rows.Scan(&ts, &msgType, &isSender, &talker) != nil {
			continue
		}
		acc.addRow(ts, msgType, talker, isSender == 1)
	}
}

// buildAnnualFromAccum 把累加结果装配成报告的各个分区
func (r *Repository) buildAnnualFromAccum(ctx context.Context, acc *annualAccum,
	report *model.AnnualReport, allow func(string) bool) model.AnnualOverview {

	var ov model.AnnualOverview
	ov.TotalMessages = acc.total
	ov.SentMessages = acc.sent
	ov.ReceivedMessages = acc.recv
	ov.FirstMessageDate = acc.firstDate
	ov.LastMessageDate = acc.lastDate
	ov.ActiveDays = len(acc.daySet)
	ov.ActiveDaysLifetime = len(acc.lifetimeDays)

	for id := range acc.contactSet {
		if isChatroomTalker(id) {
			ov.ActiveChatrooms++
		} else {
			ov.ActiveContacts++
		}
	}

	// 会话总数仍来自会话表（和消息扫描无关）
	sessions, _ := r.GetSessions(ctx, types.SessionQuery{Limit: 10000})
	allowTalker := r.TalkerFilter(ctx, model.ModuleReport)
	for _, s := range sessions {
		if !allowTalker(s.UserName) {
			continue
		}
		if isChatroomTalker(s.UserName) {
			ov.TotalChatrooms++
		} else {
			ov.TotalContacts++
		}
	}

	// 月度 / 星期 / 小时 / 类型
	report.MonthlyTrend = nil
	for m := 1; m <= 12; m++ {
		report.MonthlyTrend = append(report.MonthlyTrend, &model.MonthlyStat{Month: m, Count: acc.monthly[m]})
	}
	report.WeekdayDist = nil
	for i := 1; i <= 7; i++ {
		report.WeekdayDist = append(report.WeekdayDist, &model.WeekdayStat{Weekday: i, Count: acc.weekday[i]})
	}
	report.HourlyDist = nil
	for h := 0; h < 24; h++ {
		report.HourlyDist = append(report.HourlyDist, &model.HourlyStat{Hour: h, Count: acc.hourly[h]})
	}
	report.MessageTypes = map[string]int{}
	for t, c := range acc.msgTypes {
		report.MessageTypes[messageTypeName(t)] += c
	}

	report.Highlights = buildHighlightsFromAccum(acc)
	return ov
}

// buildHighlightsFromAccum 用累加结果算亮点，规则与原实现一致
func buildHighlightsFromAccum(acc *annualAccum) model.AnnualHighlights {
	h := model.AnnualHighlights{}

	busiest := model.DayCount{}
	quietest := model.DayCount{Count: -1}
	for d, c := range acc.dailyCounts {
		if c > busiest.Count {
			busiest = model.DayCount{Date: d, Count: c}
		}
		if quietest.Count < 0 || c < quietest.Count {
			quietest = model.DayCount{Date: d, Count: c}
		}
	}
	if quietest.Count < 0 {
		quietest = model.DayCount{}
	}
	h.BusiestDay = busiest
	h.QuietestDay = quietest
	h.LateNightCount = acc.lateNightCount
	h.LongestStreak = calcLongestStreak(acc.dailyCounts)

	early, late := earliestLatestFromTimes(acc.times)
	if early >= 0 {
		h.EarliestMessageTime = fmt.Sprintf("%02d:%02d", early/60, early%60)
	}
	if late >= 0 {
		h.LatestMessageTime = fmt.Sprintf("%02d:%02d", late/60, late%60)
	}
	return h
}

// earliestLatestFromTimes 复用原来的 07:00 日界规则，只是数据来自单趟扫描而非再查一遍库。
func earliestLatestFromTimes(times []segTime) (int, int) {
	if len(times) == 0 {
		return -1, -1
	}
	sort.Slice(times, func(i, j int) bool { return times[i].unix < times[j].unix })
	return computeEarliestLatestFromSorted(times)
}

func trimMsgPrefix(tbl string) string {
	if len(tbl) > 4 && tbl[:4] == "Msg_" {
		return tbl[4:]
	}
	return tbl
}

func isChatroomTalker(s string) bool {
	const suffix = "@chatroom"
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
