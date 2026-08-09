package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
)

// listMsgTables 列出一个 v4 分片库里的所有会话消息表（Msg_*）。
func (r *Repository) listMsgTables(ctx context.Context, db *sql.DB) []string {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%'")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			out = append(out, n)
		}
	}
	return out
}

func v4TableName(talker string) string {
	hash := md5.Sum([]byte(talker))
	return "Msg_" + hex.EncodeToString(hash[:])
}

// ── 功能9 日历热力图 ───────────────────────────────────────────
// GetCalendarHeatmap 返回某年内按天聚合的消息总数（含收发，排除系统消息）。
func (r *Repository) GetCalendarHeatmap(ctx context.Context, year, tzOffsetSec int) []*model.DayHeat {
	segs := buildSegments(year, tzOffsetSec, nil)
	if len(segs) == 0 {
		return nil
	}
	start, end, tzMod := segs[0].start, segs[len(segs)-1].end, segs[0].tzMod
	daily := make(map[string]int)
	allowTable := r.TableFilter(ctx, model.ModuleInsights)

	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			q := "SELECT strftime('%Y-%m-%d', CreateTime/1000, 'unixepoch', " + tzMod + ") d, COUNT(*) FROM MSG WHERE CreateTime >= ? AND CreateTime <= ? AND COALESCE(Type,0)!=10000 GROUP BY d"
			if rows, err := db.QueryContext(ctx, q, start.Unix()*1000, end.Unix()*1000); err == nil {
				scanDaily(rows, daily)
			}
			continue
		}
		for _, tbl := range r.listMsgTables(ctx, db) {
			if !allowTable(tbl) {
				continue
			}
			q := fmt.Sprintf("SELECT strftime('%%Y-%%m-%%d', create_time, 'unixepoch', %s) d, COUNT(*) FROM %s WHERE create_time >= ? AND create_time <= ? AND (local_type & 4294967295)!=10000 GROUP BY d", tzMod, tbl)
			if rows, err := db.QueryContext(ctx, q, start.Unix(), end.Unix()); err == nil {
				scanDaily(rows, daily)
			}
		}
	}

	out := make([]*model.DayHeat, 0, len(daily))
	for d, c := range daily {
		out = append(out, &model.DayHeat{Date: d, Count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func scanDaily(rows *sql.Rows, daily map[string]int) {
	defer rows.Close()
	for rows.Next() {
		var d string
		var c int
		if rows.Scan(&d, &c) == nil {
			daily[d] += c
		}
	}
}

// ── 功能3 双向互动比 ───────────────────────────────────────────
// GetInteractionRatios 逐个私聊联系人统计你发/对方发的条数，以及双方主动发起对话的次数。
// gapSeconds 为「会话静默阈值」，超过则视为一次新的对话发起（默认 3 小时）。
func (r *Repository) GetInteractionRatios(ctx context.Context, year, tzOffsetSec, gapSeconds, limit int) ([]*model.InteractionRatio, error) {
	segs := buildSegments(year, tzOffsetSec, nil)
	start, end := segs[0].start, segs[len(segs)-1].end
	if gapSeconds <= 0 {
		gapSeconds = 3 * 3600
	}

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}

	type agg struct {
		sent, recv, myInit, theirInit int
		last                          int64
	}
	acc := make(map[string]*agg)

	allowTalker := r.TalkerFilter(ctx, model.ModuleInsights)
	for _, s := range sessions {
		talker := s.UserName
		if strings.HasSuffix(talker, "@chatroom") {
			continue // 双向比只针对一对一
		}
		if !allowTalker(talker) {
			continue
		}
		a := &agg{}
		tbl := v4TableName(talker)
		for _, target := range r.router.Resolve(start, end, talker) {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil || !r.isTableExist(db, tbl) {
				continue
			}
			q := fmt.Sprintf(`
				WITH o AS (
					SELECT m.create_time AS ct,
						CASE WHEN (m.status=2 OR m.real_sender_id=0 OR n.user_name != ?) THEN 1 ELSE 0 END AS is_me,
						LAG(m.create_time) OVER (ORDER BY m.create_time, m.local_id) AS prev
					FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
					WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295)!=10000
				)
				SELECT
					COALESCE(SUM(is_me),0),
					COALESCE(SUM(1-is_me),0),
					COALESCE(SUM(CASE WHEN (prev IS NULL OR ct-prev > ?) AND is_me=1 THEN 1 ELSE 0 END),0),
					COALESCE(SUM(CASE WHEN (prev IS NULL OR ct-prev > ?) AND is_me=0 THEN 1 ELSE 0 END),0),
					COALESCE(MAX(ct),0)
				FROM o`, tbl)
			var sent, recv, mi, ti, last sql.NullInt64
			if err := db.QueryRowContext(ctx, q, talker, start.Unix(), end.Unix(), gapSeconds, gapSeconds).
				Scan(&sent, &recv, &mi, &ti, &last); err == nil {
				a.sent += int(sent.Int64)
				a.recv += int(recv.Int64)
				a.myInit += int(mi.Int64)
				a.theirInit += int(ti.Int64)
				if last.Int64 > a.last {
					a.last = last.Int64
				}
			}
		}
		if a.sent > 0 || a.recv > 0 {
			acc[talker] = a
		}
	}

	talkers := make([]string, 0, len(acc))
	for t := range acc {
		talkers = append(talkers, t)
	}
	profiles, _ := r.getContactProfiles(ctx, talkers)

	out := make([]*model.InteractionRatio, 0, len(acc))
	for t, a := range acc {
		name, avatar := displayName(t, profiles)
		out = append(out, &model.InteractionRatio{
			Talker: t, Name: name, Avatar: avatar,
			SentCount: a.sent, RecvCount: a.recv,
			MyInitiations: a.myInit, TheirInitiations: a.theirInit,
			Total: a.sent + a.recv, LastTime: a.last,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ── 功能5 回复速度分析 ─────────────────────────────────────────
// GetReplySpeedRanking 逐个私聊联系人统计回复时延（你回 ta / ta 回你）、深夜秒回、近似"已读不回"。
func (r *Repository) GetReplySpeedRanking(ctx context.Context, year, tzOffsetSec, limit int) ([]*model.ReplySpeed, error) {
	segs := buildSegments(year, tzOffsetSec, nil)
	start, end, tzMod := segs[0].start, segs[len(segs)-1].end, segs[0].tzMod
	const cap = 6 * 3600 // 超过 6h 的间隔不算「回复」
	const ignoreGap = 6 * 3600

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}

	type agg struct {
		mySum, myCnt, myMin, myMax int
		theirSum, theirCnt         int
		lateNight, ignored         int
	}
	acc := make(map[string]*agg)

	allowTalker := r.TalkerFilter(ctx, model.ModuleInsights)
	for _, s := range sessions {
		talker := s.UserName
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}
		if !allowTalker(talker) {
			continue
		}
		a := &agg{myMin: 1 << 30}
		tbl := v4TableName(talker)
		for _, target := range r.router.Resolve(start, end, talker) {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil || !r.isTableExist(db, tbl) {
				continue
			}
			meExpr := "(m.status=2 OR m.real_sender_id=0 OR n.user_name != ?)"
			q := fmt.Sprintf(`
				WITH o AS (
					SELECT m.create_time AS ct,
						CASE WHEN %s THEN 1 ELSE 0 END AS is_me,
						LAG(m.create_time) OVER w AS pt,
						LAG(CASE WHEN %s THEN 1 ELSE 0 END) OVER w AS pim,
						LEAD(CASE WHEN %s THEN 1 ELSE 0 END) OVER w AS nim
					FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
					WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295)!=10000
					WINDOW w AS (ORDER BY m.create_time, m.local_id)
				)
				SELECT
					COALESCE(SUM(CASE WHEN is_me=1 AND pim=0 AND ct-pt < ? THEN ct-pt END),0),
					COALESCE(SUM(CASE WHEN is_me=1 AND pim=0 AND ct-pt < ? THEN 1 ELSE 0 END),0),
					COALESCE(MIN(CASE WHEN is_me=1 AND pim=0 AND ct-pt < ? THEN ct-pt END),0),
					COALESCE(MAX(CASE WHEN is_me=1 AND pim=0 AND ct-pt < ? THEN ct-pt END),0),
					COALESCE(SUM(CASE WHEN is_me=0 AND pim=1 AND ct-pt < ? THEN ct-pt END),0),
					COALESCE(SUM(CASE WHEN is_me=0 AND pim=1 AND ct-pt < ? THEN 1 ELSE 0 END),0),
					COALESCE(SUM(CASE WHEN is_me=1 AND pim=0 AND ct-pt < 120 AND CAST(strftime('%%H', ct, 'unixepoch', %s) AS INTEGER) BETWEEN 0 AND 5 THEN 1 ELSE 0 END),0),
					COALESCE(SUM(CASE WHEN is_me=0 AND (nim=0 OR nim IS NULL) THEN 1 ELSE 0 END),0)
				FROM o`, meExpr, meExpr, meExpr, tbl, tzMod)
			var mySum, myCnt, myMin, myMax, theirSum, theirCnt, late, ign sql.NullInt64
			if err := db.QueryRowContext(ctx, q, talker, talker, talker,
				start.Unix(), end.Unix(), cap, cap, cap, cap, cap, cap).
				Scan(&mySum, &myCnt, &myMin, &myMax, &theirSum, &theirCnt, &late, &ign); err == nil {
				a.mySum += int(mySum.Int64)
				a.myCnt += int(myCnt.Int64)
				if int(myMin.Int64) > 0 && int(myMin.Int64) < a.myMin {
					a.myMin = int(myMin.Int64)
				}
				if int(myMax.Int64) > a.myMax {
					a.myMax = int(myMax.Int64)
				}
				a.theirSum += int(theirSum.Int64)
				a.theirCnt += int(theirCnt.Int64)
				a.lateNight += int(late.Int64)
				a.ignored += int(ign.Int64)
			}
		}
		if a.myCnt > 0 || a.theirCnt > 0 {
			acc[talker] = a
		}
	}

	talkers := make([]string, 0, len(acc))
	for t := range acc {
		talkers = append(talkers, t)
	}
	profiles, _ := r.getContactProfiles(ctx, talkers)

	out := make([]*model.ReplySpeed, 0, len(acc))
	for t, a := range acc {
		name, avatar := displayName(t, profiles)
		rs := &model.ReplySpeed{
			Talker: t, Name: name, Avatar: avatar,
			MyReplyCount:     a.myCnt,
			TheirReplyCount:  a.theirCnt,
			LateNightInstant: a.lateNight, IgnoredByThem: a.ignored,
		}
		if a.myCnt > 0 {
			rs.MyAvgReplySec = a.mySum / a.myCnt
			if a.myMin < (1 << 30) {
				rs.MyFastestSec = a.myMin
			}
			rs.MySlowestSec = a.myMax
		}
		if a.theirCnt > 0 {
			rs.TheirAvgReplySec = a.theirSum / a.theirCnt
		}
		out = append(out, rs)
	}
	// 默认按你回复最快排序（回复次数≥5 的才排前面，避免样本太少）
	sort.Slice(out, func(i, j int) bool {
		return out[i].MyReplyCount > out[j].MyReplyCount
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ── 功能7 年度对比 ─────────────────────────────────────────────
// GetYearCompare 对比两年的整体概览 + 每位联系人互动量的变化（淡出/新进/升/降）。
func (r *Repository) GetYearCompare(ctx context.Context, yearA, yearB, tzOffsetSec int) (*model.YearCompare, error) {
	segsA := buildSegments(yearA, tzOffsetSec, nil)
	segsB := buildSegments(yearB, tzOffsetSec, nil)

	empty := map[string]bool{}
	topA, _ := r.getAnnualTopContacts(ctx, segsA[0].start, segsA[len(segsA)-1].end, 100000, empty, model.ModuleInsights)
	topB, _ := r.getAnnualTopContacts(ctx, segsB[0].start, segsB[len(segsB)-1].end, 100000, empty, model.ModuleInsights)

	// 用 topContacts 汇总出轻量概览（避免 getAnnualOverview 的整段历史扫描，快很多）
	buildOverview := func(year int, top []*model.PersonalTopContact) *model.AnnualOverview {
		ov := &model.AnnualOverview{}
		for _, c := range top {
			ov.TotalMessages += c.MessageCount
			ov.SentMessages += c.SentCount
			ov.ReceivedMessages += c.RecvCount
			if strings.HasSuffix(c.Talker, "@chatroom") {
				ov.ActiveChatrooms++
			} else {
				ov.ActiveContacts++
			}
		}
		ov.ActiveDays = len(r.GetCalendarHeatmap(ctx, year, tzOffsetSec))
		return ov
	}
	ovA := buildOverview(yearA, topA)
	ovB := buildOverview(yearB, topB)

	type ci struct {
		name, avatar string
		isGroup      bool
		count        int
	}
	ma := make(map[string]*ci)
	mb := make(map[string]*ci)
	for _, c := range topA {
		ma[c.Talker] = &ci{c.Name, c.Avatar, c.IsGroup, c.MessageCount}
	}
	for _, c := range topB {
		mb[c.Talker] = &ci{c.Name, c.Avatar, c.IsGroup, c.MessageCount}
	}

	mk := func(t string, ca, cb int) *model.ContactYearDelta {
		info := mb[t]
		if info == nil {
			info = ma[t]
		}
		return &model.ContactYearDelta{
			Talker: t, Name: info.name, Avatar: info.avatar, IsGroup: info.isGroup,
			CountA: ca, CountB: cb, Delta: cb - ca,
		}
	}

	// 淡出/新进/升降 名单只统计「人」，排除群聊(@chatroom)和公众号(gh_)
	isPerson := func(t string) bool {
		return !strings.HasSuffix(t, "@chatroom") && !strings.HasPrefix(t, "gh_")
	}
	var faded, newly, rising, falling []*model.ContactYearDelta
	seen := map[string]bool{}
	for t, a := range ma {
		if !isPerson(t) {
			continue
		}
		seen[t] = true
		bc := 0
		if b, ok := mb[t]; ok {
			bc = b.count
		}
		d := mk(t, a.count, bc)
		// 淡出：A 里较活跃(≥50 条)，B 里几乎消失(<10% 且 <20 条)
		if a.count >= 50 && bc < 20 && bc < a.count/10 {
			faded = append(faded, d)
		}
		if d.Delta > 0 {
			rising = append(rising, d)
		} else if d.Delta < 0 {
			falling = append(falling, d)
		}
	}
	for t, b := range mb {
		if seen[t] || !isPerson(t) {
			continue
		}
		d := mk(t, 0, b.count)
		if b.count >= 50 {
			newly = append(newly, d)
		}
		rising = append(rising, d)
	}

	sort.Slice(faded, func(i, j int) bool { return faded[i].CountA > faded[j].CountA })
	sort.Slice(newly, func(i, j int) bool { return newly[i].CountB > newly[j].CountB })
	sort.Slice(rising, func(i, j int) bool { return rising[i].Delta > rising[j].Delta })
	sort.Slice(falling, func(i, j int) bool { return falling[i].Delta < falling[j].Delta })

	trim := func(s []*model.ContactYearDelta, n int) []*model.ContactYearDelta {
		if len(s) > n {
			return s[:n]
		}
		return s
	}

	return &model.YearCompare{
		YearA: yearA, YearB: yearB,
		OverviewA: ovA, OverviewB: ovB,
		FadedOut: trim(faded, 20), NewlyActive: trim(newly, 20),
		Rising: trim(rising, 15), Falling: trim(falling, 15),
	}, nil
}

// ── 功能4 共同群聊 ─────────────────────────────────────────────
// GetCommonGroups 返回「你和某好友」共同所在的群。
// 因为你的库里只保存你所在的群，好友所在的群即为共同群。
func (r *Repository) GetCommonGroups(ctx context.Context, wxid string) ([]*model.CommonGroup, error) {
	dbPath, err := r.router.GetContactDBPath()
	if err != nil {
		return nil, err
	}
	db, err := r.pool.GetConnection(dbPath)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT cr.username,
			(SELECT COUNT(*) FROM chatroom_member x WHERE x.room_id = cr.id) AS member_count
		FROM chatroom_member cm
		JOIN contact c ON cm.member_id = c.id
		JOIN chat_room cr ON cm.room_id = cr.id
		WHERE c.username = ?`
	rows, err := db.QueryContext(ctx, q, wxid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []string
	counts := make(map[string]int)
	for rows.Next() {
		var uname string
		var cnt int
		if rows.Scan(&uname, &cnt) == nil && uname != "" {
			rooms = append(rooms, uname)
			counts[uname] = cnt
		}
	}

	profiles, _ := r.getContactProfiles(ctx, rooms)
	out := make([]*model.CommonGroup, 0, len(rooms))
	for _, u := range rooms {
		name, avatar := displayName(u, profiles)
		out = append(out, &model.CommonGroup{Username: u, Name: name, Avatar: avatar, MemberCount: counts[u]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MemberCount > out[j].MemberCount })
	return out, nil
}

// displayName 从 profiles 里取显示名与头像（备注 > 昵称 > wxid）。
func displayName(talker string, profiles map[string]contactProfile) (string, string) {
	name := talker
	var avatar string
	if p, ok := profiles[talker]; ok {
		if p.Remark != "" {
			name = p.Remark
		} else if p.NickName != "" {
			name = p.NickName
		}
		avatar = p.SmallHeadURL
	}
	return name, avatar
}
