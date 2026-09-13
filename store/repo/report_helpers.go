package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// getAnnualOverview 获取年度概览统计（跨多时区段）
func (r *Repository) getAnnualOverview(ctx context.Context, segs []reportSegment, allow func(string) bool) (model.AnnualOverview, error) {
	var overview model.AnnualOverview
	var totalMsgs, sentMsgs, recvMsgs int
	contactSet := make(map[string]bool)
	chatroomSet := make(map[string]bool)
	daySet := make(map[string]bool)
	var firstDate, lastDate string

	for _, seg := range segs {
		for _, shard := range r.router.GetShards() {
			db, err := r.pool.GetConnection(shard.FilePath)
			if err != nil {
				continue
			}
			if r.isTableExist(db, "MSG") {
				r.overviewV3(ctx, db, seg.start, seg.end, &totalMsgs, &sentMsgs, &recvMsgs, contactSet, chatroomSet, daySet, &firstDate, &lastDate, seg.tzMod)
			} else {
				r.overviewV4(ctx, db, seg.start, seg.end, &totalMsgs, &sentMsgs, &recvMsgs, contactSet, chatroomSet, daySet, &firstDate, &lastDate, seg.tzMod, allow)
			}
		}
	}

	overview.TotalMessages = totalMsgs
	overview.SentMessages = sentMsgs
	overview.ReceivedMessages = recvMsgs
	overview.FirstMessageDate = firstDate
	overview.LastMessageDate = lastDate

	// 活跃天数：从「首条消息」到今天里有消息的不同天数。
	// 之前是按 segs 范围（即一年）算，会让新会话显得只有几天活跃；
	// 改成跨整段聊天历史统计 —— 用一份不限时间的额外 overview 扫描，
	// 只取它的 daySet。其余字段（消息总数等）继续按 segs 内的来。
	// 累计活跃天数只需要「有哪些日期」，早先却跑了一整趟完整概览扫描：
	// 逐表算收发数、还为未知表做 COUNT(DISTINCT real_sender_id) 探测，
	// 结果除 daySet 外全部丢弃。改成只查去重日期，省掉大部分 IO。
	lifetimeDays := make(map[string]bool)
	lifeTz := ""
	if len(segs) > 0 {
		lifeTz = segs[0].tzMod
	}
	r.collectDistinctDays(ctx, lifetimeDays, lifeTz, allow)
	// 当年活跃天数（同比用它）与累计活跃天数（单独展示）分开给
	overview.ActiveDays = len(daySet)
	overview.ActiveDaysLifetime = len(lifetimeDays)

	activeContacts := 0
	activeChatrooms := 0
	for id := range contactSet {
		if strings.HasSuffix(id, "@chatroom") {
			activeChatrooms++
		} else {
			activeContacts++
		}
	}
	overview.ActiveContacts = activeContacts
	overview.ActiveChatrooms = activeChatrooms

	sessions, _ := r.GetSessions(ctx, types.SessionQuery{Limit: 10000})
	allowTalker := r.TalkerFilter(ctx, model.ModuleReport)
	totalContacts := 0
	totalChatrooms := 0
	for _, s := range sessions {
		if !allowTalker(s.UserName) {
			continue
		}
		if strings.HasSuffix(s.UserName, "@chatroom") {
			totalChatrooms++
		} else {
			totalContacts++
		}
	}
	overview.TotalContacts = totalContacts
	overview.TotalChatrooms = totalChatrooms

	return overview, nil
}

// collectDistinctDays 收集所有分片里出现过消息的日期（去重），用于「累计活跃天数」。
// 只做一次 SELECT DISTINCT，不碰收发方向、不做发送者探测。
func (r *Repository) collectDistinctDays(ctx context.Context, days map[string]bool, tzMod string, allow func(string) bool) {
	if tzMod == "" {
		tzMod = "'localtime'"
	}
	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			q := "SELECT DISTINCT strftime('%Y-%m-%d', CreateTime/1000, 'unixepoch', " + tzMod +
				") FROM MSG WHERE COALESCE(Type,0) != 10000"
			if rows, err := db.QueryContext(ctx, q); err == nil {
				for rows.Next() {
					var d string
					if rows.Scan(&d) == nil && d != "" {
						days[d] = true
					}
				}
				rows.Close()
			}
			continue
		}
		for _, tbl := range r.listMsgTables(ctx, db) {
			if !allow(tbl) {
				continue
			}
			q := fmt.Sprintf(
				"SELECT DISTINCT strftime('%%Y-%%m-%%d', create_time, 'unixepoch', %s) FROM %s "+
					"WHERE (local_type & 4294967295) != 10000", tzMod, tbl)
			rows, err := db.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			for rows.Next() {
				var d string
				if rows.Scan(&d) == nil && d != "" {
					days[d] = true
				}
			}
			rows.Close()
		}
	}
}

func (r *Repository) overviewV3(ctx context.Context, db *sql.DB, start, end time.Time,
	totalMsgs, sentMsgs, recvMsgs *int,
	contactSet, chatroomSet, daySet map[string]bool,
	firstDate, lastDate *string, tzMod string) {

	query := `SELECT COUNT(*),
		SUM(CASE WHEN COALESCE(IsSender, 0) = 1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN COALESCE(IsSender, 0) != 1 THEN 1 ELSE 0 END)
		FROM MSG WHERE CreateTime >= ? AND CreateTime <= ? AND COALESCE(Type,0) != 10000`
	var total, sent, recv sql.NullInt64
	if err := db.QueryRowContext(ctx, query, start.Unix()*1000, end.Unix()*1000).Scan(&total, &sent, &recv); err == nil {
		if total.Valid {
			*totalMsgs += int(total.Int64)
		}
		if sent.Valid {
			*sentMsgs += int(sent.Int64)
		}
		if recv.Valid {
			*recvMsgs += int(recv.Int64)
		}
	}

	rows, err := db.QueryContext(ctx, "SELECT DISTINCT StrTalker FROM MSG WHERE CreateTime >= ? AND CreateTime <= ?", start.Unix()*1000, end.Unix()*1000)
	if err == nil {
		for rows.Next() {
			var talker string
			rows.Scan(&talker)
			contactSet[talker] = true
		}
		rows.Close()
	}

	rows, err = db.QueryContext(ctx,
		"SELECT DISTINCT strftime('%Y-%m-%d', CreateTime/1000, 'unixepoch', "+tzMod+") as d FROM MSG WHERE CreateTime >= ? AND CreateTime <= ? ORDER BY d",
		start.Unix()*1000, end.Unix()*1000)
	if err == nil {
		for rows.Next() {
			var d string
			rows.Scan(&d)
			daySet[d] = true
			if *firstDate == "" || d < *firstDate {
				*firstDate = d
			}
			if d > *lastDate {
				*lastDate = d
			}
		}
		rows.Close()
	}
}

func (r *Repository) overviewV4(ctx context.Context, db *sql.DB, start, end time.Time,
	totalMsgs, sentMsgs, recvMsgs *int,
	contactSet, chatroomSet, daySet map[string]bool,
	firstDate, lastDate *string, tzMod string, allow func(string) bool) {

	myWxid := r.getCurrentUserWxid(ctx)
	talkerMD5Map := r.getTalkerMD5Map(ctx)
	allowTable := allow

	tables, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%'")
	if err != nil {
		return
	}
	defer tables.Close()

	for tables.Next() {
		var tableName string
		tables.Scan(&tableName)

		talker := "unknown"
		if strings.HasPrefix(tableName, "Msg_") {
			md5Hash := strings.TrimPrefix(tableName, "Msg_")
			if t, ok := talkerMD5Map[md5Hash]; ok {
				talker = t
			} else {
				var distinctSenders int
				cntQuery := fmt.Sprintf("SELECT COUNT(DISTINCT real_sender_id) FROM %s WHERE real_sender_id > 0", tableName)
				if db.QueryRowContext(ctx, cntQuery).Scan(&distinctSenders) == nil && distinctSenders >= 2 {
					talker = md5Hash + "@chatroom"
				}
			}
		}

		// 统计范围 + 排除名单：不参与统计的会话整表跳过
		if !allowTable(tableName) {
			continue
		}

		var total, sent, recv sql.NullInt64
		var query string
		if talker != "unknown" && !strings.HasSuffix(talker, "@chatroom") {
			query = fmt.Sprintf(`
				SELECT COUNT(*),
					SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) THEN 1 ELSE 0 END),
					SUM(CASE WHEN (m.status != 2 AND m.real_sender_id != 0 AND n.user_name = ?) THEN 1 ELSE 0 END)
				FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) != 10000`, tableName)
			err = db.QueryRowContext(ctx, query, talker, talker, start.Unix(), end.Unix()).Scan(&total, &sent, &recv)
		} else {
			query = fmt.Sprintf(`
				SELECT COUNT(*),
					SUM(CASE WHEN (n.user_name = ? OR m.status = 2 OR m.real_sender_id = 0) THEN 1 ELSE 0 END),
					SUM(CASE WHEN (n.user_name != ? AND m.status != 2 AND m.real_sender_id != 0) THEN 1 ELSE 0 END)
				FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) != 10000`, tableName)
			err = db.QueryRowContext(ctx, query, myWxid, myWxid, start.Unix(), end.Unix()).Scan(&total, &sent, &recv)
		}

		if err == nil && total.Valid && total.Int64 > 0 {
			*totalMsgs += int(total.Int64)
			if sent.Valid {
				*sentMsgs += int(sent.Int64)
			}
			if recv.Valid {
				*recvMsgs += int(recv.Int64)
			}
			contactSet[talker] = true
		}

		daysQuery := fmt.Sprintf(
			"SELECT DISTINCT strftime('%%Y-%%m-%%d', create_time, 'unixepoch', %s) as d FROM %s WHERE create_time >= ? AND create_time <= ? ORDER BY d",
			tzMod, tableName)
		rows, err := db.QueryContext(ctx, daysQuery, start.Unix(), end.Unix())
		if err == nil {
			for rows.Next() {
				var d string
				rows.Scan(&d)
				daySet[d] = true
				if *firstDate == "" || d < *firstDate {
					*firstDate = d
				}
				if d > *lastDate {
					*lastDate = d
				}
			}
			rows.Close()
		}
	}
}

// getAnnualTopContacts 亲密度排行（不依赖时区分段，用整年范围，包含群聊）
func (r *Repository) getAnnualTopContacts(ctx context.Context, start, end time.Time, limit int, excludeSet map[string]bool, mod model.StatsModule) ([]*model.PersonalTopContact, error) {
	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}

	type stats struct {
		sent     int
		recv     int
		lastTime int64
	}
	aggStats := make(map[string]*stats)

	allowTalker := r.TalkerFilter(ctx, mod)
	for _, session := range sessions {
		talker := session.UserName
		if excludeSet[talker] || !allowTalker(talker) {
			continue
		}

		targets := r.router.Resolve(start, end, talker)
		s := &stats{}

		for _, target := range targets {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil {
				continue
			}
			hash := md5.Sum([]byte(talker))
			tableName := "Msg_" + hex.EncodeToString(hash[:])

			if r.isTableExist(db, tableName) {
				// 群聊：real_sender_id=0 或 status=2 表示自己发的；
				// 私聊：额外用 n.user_name != talker 判断（talker 就是对方的 wxid）
				isGroup := strings.HasSuffix(talker, "@chatroom")
				var query string
				var queryArgs []interface{}
				if isGroup {
					query = fmt.Sprintf(`
						SELECT CASE WHEN (m.status = 2 OR m.real_sender_id = 0) THEN 1 ELSE 0 END as is_self,
							COUNT(*), MAX(m.create_time)
						FROM %s m
						WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) != 10000
						GROUP BY is_self`, tableName)
					queryArgs = []interface{}{start.Unix(), end.Unix()}
				} else {
					query = fmt.Sprintf(`
						SELECT CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) THEN 1 ELSE 0 END as is_self,
							COUNT(*), MAX(m.create_time)
						FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
						WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) != 10000
						GROUP BY is_self`, tableName)
					queryArgs = []interface{}{talker, start.Unix(), end.Unix()}
				}
				rows, err := db.QueryContext(ctx, query, queryArgs...)
				if err == nil {
					for rows.Next() {
						var isSelf, count int
						var maxTime int64
						if rows.Scan(&isSelf, &count, &maxTime) == nil {
							if isSelf == 1 {
								s.sent += count
							} else {
								s.recv += count
							}
							if maxTime > s.lastTime {
								s.lastTime = maxTime
							}
						}
					}
					rows.Close()
				}
			} else {
				query := "SELECT COALESCE(IsSender, 0), COUNT(*), MAX(CreateTime/1000) FROM MSG WHERE CreateTime >= ? AND CreateTime <= ?"
				var args []interface{}
				args = append(args, start.Unix()*1000, end.Unix()*1000)
				if target.TalkerID != 0 {
					query += " AND TalkerId = ?"
					args = append(args, target.TalkerID)
				} else {
					query += " AND StrTalker = ?"
					args = append(args, target.Talker)
				}
				query += " GROUP BY COALESCE(IsSender, 0)"
				rows, err := db.QueryContext(ctx, query, args...)
				if err == nil {
					for rows.Next() {
						var isSelf, count int
						var maxTime int64
						if rows.Scan(&isSelf, &count, &maxTime) == nil {
							if isSelf == 1 {
								s.sent += count
							} else {
								s.recv += count
							}
							if maxTime > s.lastTime {
								s.lastTime = maxTime
							}
						}
					}
					rows.Close()
				}
			}
		}
		if s.sent > 0 || s.recv > 0 {
			aggStats[talker] = s
		}
	}

	var talkers []string
	for t := range aggStats {
		talkers = append(talkers, t)
	}
	profiles, _ := r.getContactProfiles(ctx, talkers)

	var result []*model.PersonalTopContact
	for t, s := range aggStats {
		name := t
		var avatar string
		if p, ok := profiles[t]; ok {
			if p.Remark != "" {
				name = p.Remark
			} else if p.NickName != "" {
				name = p.NickName
			}
			avatar = p.SmallHeadURL
		}
		result = append(result, &model.PersonalTopContact{
			Talker:       t,
			Name:         name,
			Avatar:       avatar,
			IsGroup:      strings.HasSuffix(t, "@chatroom"),
			MessageCount: s.sent + s.recv,
			SentCount:    s.sent,
			RecvCount:    s.recv,
			LastTime:     s.lastTime,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].MessageCount > result[j].MessageCount
	})

	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// getAnnualMonthlyTrend 获取年度月度趋势（跨多时区段）
func (r *Repository) getAnnualMonthlyTrend(ctx context.Context, segs []reportSegment, allow func(string) bool) []*model.MonthlyStat {
	monthlyStats := make(map[int]int)

	for _, seg := range segs {
		for _, shard := range r.router.GetShards() {
			db, err := r.pool.GetConnection(shard.FilePath)
			if err != nil {
				continue
			}
			if r.isTableExist(db, "MSG") {
				query := "SELECT CAST(strftime('%m', CreateTime/1000, 'unixepoch', " + seg.tzMod + ") AS INTEGER) as month, COUNT(*) as count FROM MSG WHERE CreateTime >= ? AND CreateTime <= ? GROUP BY month"
				rows, err := db.QueryContext(ctx, query, seg.start.Unix()*1000, seg.end.Unix()*1000)
				if err == nil {
					for rows.Next() {
						var m, c int
						rows.Scan(&m, &c)
						monthlyStats[m] += c
					}
					rows.Close()
				}
			} else {
				tables, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%'")
				if err != nil {
					continue
				}
				allowTable := allow
				for tables.Next() {
					var tableName string
					tables.Scan(&tableName)
					if !allowTable(tableName) {
						continue
					}
					query := fmt.Sprintf("SELECT CAST(strftime('%%m', create_time, 'unixepoch', %s) AS INTEGER) as month, COUNT(*) as count FROM %s WHERE create_time >= ? AND create_time <= ? AND (local_type & 4294967295) != 10000 GROUP BY month", seg.tzMod, tableName)
					rows, err := db.QueryContext(ctx, query, seg.start.Unix(), seg.end.Unix())
					if err == nil {
						for rows.Next() {
							var m, c int
							rows.Scan(&m, &c)
							monthlyStats[m] += c
						}
						rows.Close()
					}
				}
				tables.Close()
			}
		}
	}

	var result []*model.MonthlyStat
	for i := 1; i <= 12; i++ {
		result = append(result, &model.MonthlyStat{Month: i, Count: monthlyStats[i]})
	}
	return result
}

func messageTypeName(t int) string {
	switch t {
	case 1:
		return "text"
	case 3:
		return "image"
	case 34:
		return "voice"
	case 43:
		return "video"
	case 49:
		return "link"
	case 47:
		return "emoji"
	case 42:
		return "card"
	case 48:
		return "location"
	case 50:
		return "voip"
	case 10000:
		return "system"
	default:
		return "other"
	}
}

// computeEarliestLatestFromSorted 是上面那套 07:00 日界规则的纯计算部分。
// 单趟扫描直接复用它，避免为了「最早/最晚」再把库扫一遍。
// 入参必须已按 unix 升序排好。
func computeEarliestLatestFromSorted(times []segTime) (earlyMinOfDay, lateMinOfDay int) {
	if len(times) == 0 {
		return -1, -1
	}

	// 把 unix 时间归到 07:00-day key（YYYY-MM-DD 字符串）。
	// 凌晨 0-7 点的消息归到「前一天」的 07:00-day。
	dayKey := func(t time.Time) string {
		if t.Hour() < 7 {
			t = t.AddDate(0, 0, -1)
		}
		return t.Format("2006-01-02")
	}

	const fourHours = int64(4 * 3600)

	type cand struct {
		minOfDay int
		dayKey   string
	}
	var morning []cand
	var night []cand
	var anyAfter7 []cand
	var allMsgs []cand

	for i, t := range times {
		ttt := time.Unix(t.unix, 0).In(t.loc)
		minOfDay := ttt.Hour()*60 + ttt.Minute()
		dk := dayKey(ttt)
		allMsgs = append(allMsgs, cand{minOfDay, dk})
		if minOfDay >= 420 {
			anyAfter7 = append(anyAfter7, cand{minOfDay, dk})
		}
		priorOK := i == 0 || (t.unix-times[i-1].unix) >= fourHours
		nextOK := i == len(times)-1 || (times[i+1].unix-t.unix) >= fourHours
		// 「起床后的第一条」：07:00 之后，而且前面隔了 4 小时没说话
		if minOfDay >= 420 && priorOK {
			morning = append(morning, cand{minOfDay, dk})
		}
		// 「睡前最后一条」：后面隔了 4 小时没说话，而且前面**没有**隔 4 小时 ——
		// 也就是一段对话的结尾，而不是一条孤零零的消息。
		// 少了 !priorOK 这个条件，清晨 06:5x 那种「醒得早、发一条又没下文」的孤立消息
		// 会拿到最大的 shifted 值（07:00 是日界），一年的数据必然得出 06:59。
		if nextOK && !priorOK {
			night = append(night, cand{minOfDay, dk})
		}
	}

	// 找最早：优先用 morning 候选，否则 fallback 用 anyAfter7
	earlyPool := morning
	if len(earlyPool) == 0 {
		earlyPool = anyAfter7
	}
	// 找最晚：优先用 night 候选，否则 fallback 用 allMsgs
	latePool := night
	if len(latePool) == 0 {
		latePool = allMsgs
	}

	if len(earlyPool) == 0 || len(latePool) == 0 {
		return -1, -1
	}

	// 在 earlyPool/latePool 笛卡尔积中找一对：dayKey 不同，且最早的 minOfDay 最小、最晚的 shifted 最大
	// 简化：先各自找最优，如果同一天则尝试找次优组合
	type best struct {
		minOfDay int
		shifted  int
		dayKey   string
		ok       bool
	}

	// 找最早 (按 minOfDay 升序排序)
	sortedEarly := make([]cand, len(earlyPool))
	copy(sortedEarly, earlyPool)
	sort.Slice(sortedEarly, func(i, j int) bool { return sortedEarly[i].minOfDay < sortedEarly[j].minOfDay })
	// 找最晚 (按 shifted 降序排序)
	sortedLate := make([]cand, len(latePool))
	copy(sortedLate, latePool)
	sort.Slice(sortedLate, func(i, j int) bool {
		si := (sortedLate[i].minOfDay - 420 + 1440) % 1440
		sj := (sortedLate[j].minOfDay - 420 + 1440) % 1440
		return si > sj
	})

	// 收集 latePool 的所有 dayKey 集合（用于判断"早晨候选所属日"是否在晚上候选范围）
	lateDays := make(map[string]bool)
	for _, c := range sortedLate {
		lateDays[c.dayKey] = true
	}
	// 早晨候选选最早的，前提是有不同日的晚候选
	var bestEarly, bestLate best
	for _, e := range sortedEarly {
		// 需要找到一个 latePool 元素，dayKey != e.dayKey
		for _, l := range sortedLate {
			if l.dayKey != e.dayKey {
				bestEarly = best{minOfDay: e.minOfDay, dayKey: e.dayKey, ok: true}
				bestLate = best{shifted: (l.minOfDay - 420 + 1440) % 1440, dayKey: l.dayKey, ok: true}
				goto Done
			}
		}
	}
	// fallback：如果整个数据集都在同一天，那就允许同一天
	{
		e := sortedEarly[0]
		l := sortedLate[0]
		bestEarly = best{minOfDay: e.minOfDay, dayKey: e.dayKey, ok: true}
		bestLate = best{shifted: (l.minOfDay - 420 + 1440) % 1440, dayKey: l.dayKey, ok: true}
	}
Done:
	_ = lateDays

	if !bestEarly.ok {
		earlyMinOfDay = -1
	} else {
		earlyMinOfDay = bestEarly.minOfDay
	}
	if !bestLate.ok {
		lateMinOfDay = -1
	} else {
		lateMinOfDay = (bestLate.shifted + 420) % 1440
	}
	return
}

func calcLongestStreak(dailyCounts map[string]int) int {
	if len(dailyCounts) == 0 {
		return 0
	}

	dates := make([]string, 0, len(dailyCounts))
	for d := range dailyCounts {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	longest := 1
	current := 1

	for i := 1; i < len(dates); i++ {
		prev, err1 := time.Parse("2006-01-02", dates[i-1])
		curr, err2 := time.Parse("2006-01-02", dates[i])
		if err1 != nil || err2 != nil {
			current = 1
			continue
		}
		if curr.Sub(prev).Hours() == 24 {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 1
		}
	}
	return longest
}
