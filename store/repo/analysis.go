package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/pkg/util"
	"github.com/afumu/wetrace/pkg/util/zstd"
	"github.com/afumu/wetrace/store/bind"
	"github.com/afumu/wetrace/store/types"
	"github.com/rs/zerolog/log"
)

func (r *Repository) getCurrentUserWxid(ctx context.Context) string {
	// 遍历所有分片尝试寻找一个“我发送”的证据
	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}

		// 寻找任何一个 Msg_ 表
		var tableName string
		_ = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%' LIMIT 1").Scan(&tableName)
		if tableName == "" {
			continue
		}

		// 寻找 status=2 (已发送) 且 real_sender_id > 0 的记录来获取我的 wxid
		var myWxid string
		query := fmt.Sprintf(`
			SELECT n.user_name 
			FROM %s m 
			JOIN Name2Id n ON m.real_sender_id = n.rowid 
			WHERE m.status = 2 AND n.user_name IS NOT NULL AND n.user_name != '' 
			LIMIT 1`, tableName)
		if err := db.QueryRowContext(ctx, query).Scan(&myWxid); err == nil && myWxid != "" {
			return myWxid
		}
	}

	// 备选：从 contact 表找 local_type = 1 (通常是个人账号)
	dbPath, err := r.router.GetContactDBPath()
	if err == nil {
		if db, err := r.pool.GetConnection(dbPath); err == nil {
			var wxid string
			_ = db.QueryRowContext(ctx, "SELECT username FROM contact WHERE local_type = 1 LIMIT 1").Scan(&wxid)
			if wxid != "" {
				return wxid
			}
		}
	}

	return ""
}

// GetHourlyActivity 获取指定会话的每小时消息活跃度
func (r *Repository) GetHourlyActivity(ctx context.Context, talker string) ([]*model.HourlyStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	if len(targets) == 0 {
		return []*model.HourlyStat{}, nil
	}

	hourlyStats := make(map[int]int)
	for _, target := range targets {
		stats, err := r.queryHourlyStatSingleShard(ctx, target, talker)
		if err != nil {
			log.Warn().Err(err).Str("db", target.FilePath).Msg("查询时段统计失败")
			continue
		}
		for _, s := range stats {
			hourlyStats[s.Hour] += s.Count
		}
	}

	var result []*model.HourlyStat
	for i := 0; i < 24; i++ {
		result = append(result, &model.HourlyStat{Hour: i, Count: hourlyStats[i]})
	}
	return result, nil
}

func (r *Repository) queryHourlyStatSingleShard(ctx context.Context, target bind.RouteResult, talker string) ([]*model.HourlyStat, error) {
	db, err := r.pool.GetConnection(target.FilePath)
	if err != nil {
		return nil, err
	}
	hash := md5.Sum([]byte(talker))
	tableName := "Msg_" + hex.EncodeToString(hash[:])
	if r.isTableExist(db, tableName) {
		return r.queryV4HourlyStat(ctx, db, tableName)
	}
	return r.queryV3HourlyStat(ctx, db, target)
}

func (r *Repository) queryV4HourlyStat(ctx context.Context, db *sql.DB, tableName string) ([]*model.HourlyStat, error) {
	tzMod := r.DefaultTzModifier()
	query := fmt.Sprintf("SELECT CAST(strftime('%%H', create_time, 'unixepoch', %s) AS INTEGER) as hour, COUNT(*) as count FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY hour", tzMod, tableName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*model.HourlyStat
	for rows.Next() {
		var s model.HourlyStat
		if err := rows.Scan(&s.Hour, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, &s)
	}
	return stats, nil
}

func (r *Repository) queryV3HourlyStat(ctx context.Context, db *sql.DB, target bind.RouteResult) ([]*model.HourlyStat, error) {
	tzMod := r.DefaultTzModifier()
	query := "SELECT CAST(strftime('%H', CreateTime / 1000, 'unixepoch', " + tzMod + ") AS INTEGER) as hour, COUNT(*) as count FROM MSG WHERE Type != 10000"
	var args []interface{}
	if target.TalkerID != 0 {
		query += " AND TalkerId = ?"
		args = append(args, target.TalkerID)
	} else {
		query += " AND StrTalker = ?"
		args = append(args, target.Talker)
	}
	query += " GROUP BY hour"
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*model.HourlyStat
	for rows.Next() {
		var s model.HourlyStat
		if err := rows.Scan(&s.Hour, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, &s)
	}
	return stats, nil
}

// GetDailyActivity 获取指定会话的每日消息活跃度
func (r *Repository) GetDailyActivity(ctx context.Context, talker string) ([]*model.DailyStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	dailyStats := make(map[string]int)
	for _, target := range targets {
		stats, err := r.queryDailyStatSingleShard(ctx, target, talker)
		if err != nil {
			continue
		}
		for _, s := range stats {
			dailyStats[s.Date] += s.Count
		}
	}
	var result []*model.DailyStat
	for date, count := range dailyStats {
		result = append(result, &model.DailyStat{Date: date, Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result, nil
}

func (r *Repository) queryDailyStatSingleShard(ctx context.Context, target bind.RouteResult, talker string) ([]*model.DailyStat, error) {
	db, err := r.pool.GetConnection(target.FilePath)
	if err != nil {
		return nil, err
	}
	hash := md5.Sum([]byte(talker))
	tableName := "Msg_" + hex.EncodeToString(hash[:])
	tzMod := r.DefaultTzModifier()
	if r.isTableExist(db, tableName) {
		query := fmt.Sprintf("SELECT strftime('%%Y-%%m-%%d', create_time, 'unixepoch', %s) as date, COUNT(*) as count FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY date", tzMod, tableName)
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var stats []*model.DailyStat
		for rows.Next() {
			var s model.DailyStat
			if err := rows.Scan(&s.Date, &s.Count); err != nil {
				return nil, err
			}
			stats = append(stats, &s)
		}
		return stats, nil
	}
	// V3 支持
	query := "SELECT strftime('%Y-%m-%d', CreateTime / 1000, 'unixepoch', " + tzMod + ") as date, COUNT(*) as count FROM MSG WHERE Type != 10000"
	var args []interface{}
	if target.TalkerID != 0 {
		query += " AND TalkerId = ?"
		args = append(args, target.TalkerID)
	} else {
		query += " AND StrTalker = ?"
		args = append(args, target.Talker)
	}
	query += " GROUP BY date"
	rows, err := db.QueryContext(ctx, query, args...)
	if err == nil {
		defer rows.Close()
		var stats []*model.DailyStat
		for rows.Next() {
			var s model.DailyStat
			if err := rows.Scan(&s.Date, &s.Count); err == nil {
				stats = append(stats, &s)
			}
		}
		return stats, nil
	}
	return []*model.DailyStat{}, nil
}

// GetWeekdayActivity 获取指定会话的星期消息活跃度
func (r *Repository) GetWeekdayActivity(ctx context.Context, talker string) ([]*model.WeekdayStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	weekdayStats := make(map[int]int)
	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		tzMod := r.DefaultTzModifier()
		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			query = fmt.Sprintf("SELECT CASE WHEN CAST(strftime('%%w', create_time, 'unixepoch', %s) AS INTEGER) = 0 THEN 7 ELSE CAST(strftime('%%w', create_time, 'unixepoch', %s) AS INTEGER) END as weekday, COUNT(*) as count FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY weekday", tzMod, tzMod, tableName)
		} else {
			query = "SELECT CASE WHEN CAST(strftime('%w', CreateTime / 1000, 'unixepoch', " + tzMod + ") AS INTEGER) = 0 THEN 7 ELSE CAST(strftime('%w', CreateTime / 1000, 'unixepoch', " + tzMod + ") AS INTEGER) END as weekday, COUNT(*) as count FROM MSG WHERE Type != 10000"
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " GROUP BY weekday"
		}

		rows, _ := db.QueryContext(ctx, query, args...)
		if rows != nil {
			for rows.Next() {
				var w, c int
				rows.Scan(&w, &c)
				weekdayStats[w] += c
			}
			rows.Close()
		}
	}
	var result []*model.WeekdayStat
	for i := 1; i <= 7; i++ {
		result = append(result, &model.WeekdayStat{Weekday: i, Count: weekdayStats[i]})
	}
	return result, nil
}

// GetMonthlyActivity 获取指定会话的月份消息活跃度
func (r *Repository) GetMonthlyActivity(ctx context.Context, talker string) ([]*model.MonthlyStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	monthlyStats := make(map[int]int)
	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		tzMod := r.DefaultTzModifier()
		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			query = fmt.Sprintf("SELECT CAST(strftime('%%m', create_time, 'unixepoch', %s) AS INTEGER) as month, COUNT(*) as count FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY month", tzMod, tableName)
		} else {
			query = "SELECT CAST(strftime('%m', CreateTime / 1000, 'unixepoch', " + tzMod + ") AS INTEGER) as month, COUNT(*) as count FROM MSG WHERE Type != 10000"
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " GROUP BY month"
		}

		rows, _ := db.QueryContext(ctx, query, args...)
		if rows != nil {
			for rows.Next() {
				var m, c int
				rows.Scan(&m, &c)
				monthlyStats[m] += c
			}
			rows.Close()
		}
	}
	var result []*model.MonthlyStat
	for i := 1; i <= 12; i++ {
		result = append(result, &model.MonthlyStat{Month: i, Count: monthlyStats[i]})
	}
	return result, nil
}

// 已知的消息类型常量。其他类型会被归入"其他"桶。
var knownMessageTypes = map[int]bool{
	1:  true, // 文本
	3:  true, // 图片
	34: true, // 语音
	42: true, // 名片
	43: true, // 视频
	47: true, // 表情
	48: true, // 位置
	49: true, // 链接/文件/小程序等
	50: true, // 通话
}

// 类型「其他」的桶 ID
const MessageTypeOther = 99

// normalizeMessageType 取 local_type 的真实类型部分（低 32 位），
// 把不常见的类型归到 99（其他）。
//
// V4 的 local_type 实际是 (sub_type << 32) | type 的打包整数，
// 直接 GROUP BY 会出现 21474836529 / 103079215153 这种奇怪值。
func normalizeMessageType(t int) int {
	t = t & 0xFFFFFFFF // 取低 32 位
	if t == 10000 {
		return -1 // 系统消息，调用方应跳过
	}
	if knownMessageTypes[t] {
		return t
	}
	return MessageTypeOther
}

// GetYearlyMonthlyActivity 获取指定会话按年-月聚合的消息数。
// 用于在月度趋势图上叠加"历史年份月均"参考线。
func (r *Repository) GetYearlyMonthlyActivity(ctx context.Context, talker string) ([]*model.YearMonthStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	stats := make(map[[2]int]int) // [year, month] -> count
	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		tzMod := r.DefaultTzModifier()
		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			query = fmt.Sprintf(`
				SELECT CAST(strftime('%%Y', create_time, 'unixepoch', %s) AS INTEGER) as y,
				       CAST(strftime('%%m', create_time, 'unixepoch', %s) AS INTEGER) as m,
				       COUNT(*) as count
				FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY y, m`, tzMod, tzMod, tableName)
		} else {
			query = "SELECT CAST(strftime('%Y', CreateTime/1000, 'unixepoch', " + tzMod + ") AS INTEGER) as y, CAST(strftime('%m', CreateTime/1000, 'unixepoch', " + tzMod + ") AS INTEGER) as m, COUNT(*) as count FROM MSG WHERE Type != 10000"
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " GROUP BY y, m"
		}
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var y, m, c int
			if rows.Scan(&y, &m, &c) == nil {
				stats[[2]int{y, m}] += c
			}
		}
		rows.Close()
	}

	var result []*model.YearMonthStat
	for k, v := range stats {
		result = append(result, &model.YearMonthStat{Year: k[0], Month: k[1], Count: v})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Year != result[j].Year {
			return result[i].Year < result[j].Year
		}
		return result[i].Month < result[j].Month
	})
	return result, nil
}

// GetTopContactsHistoricalMonthlyAvg 计算亲密度 Top N 联系人在过去年份（不含当年）的每月平均消息量。
// 返回 12 个月的平均值（单位：条），用于在月度趋势图上叠加参考线。
func (r *Repository) GetTopContactsHistoricalMonthlyAvg(ctx context.Context, limit int) ([]*model.MonthlyStat, error) {
	if limit <= 0 {
		limit = 10
	}
	topContacts, err := r.GetPersonalTopContacts(ctx, limit)
	if err != nil {
		return nil, err
	}

	currentYear := time.Now().Year()
	// 全程用 float64 累加，最后一次性四舍五入。
	// 原来是两层整除：先 pastByMonth/年数、再 累加/人数，两次都往下截断，
	// 参考线被系统性压低 —— 某人某月过去 3 年共 5 条，5/3 直接变成 1。
	monthlyAccum := make(map[int]float64) // month -> 累加(每个联系人的过去年份月均)
	contactCount := 0

	for _, tc := range topContacts {
		ymData, err := r.GetYearlyMonthlyActivity(ctx, tc.Talker)
		if err != nil {
			continue
		}
		pastYears := make(map[int]bool)
		pastByMonth := make(map[int]int)
		for _, ym := range ymData {
			if ym.Year < currentYear {
				pastYears[ym.Year] = true
				pastByMonth[ym.Month] += ym.Count
			}
		}
		if len(pastYears) == 0 {
			continue
		}
		contactCount++
		for m := 1; m <= 12; m++ {
			// 该联系人月均 = 过去年份该月总和 / 过去年份数
			monthlyAccum[m] += float64(pastByMonth[m]) / float64(len(pastYears))
		}
	}

	result := make([]*model.MonthlyStat, 0, 12)
	for m := 1; m <= 12; m++ {
		avg := 0
		if contactCount > 0 {
			avg = int(math.Round(monthlyAccum[m] / float64(contactCount)))
		}
		result = append(result, &model.MonthlyStat{Month: m, Count: avg})
	}
	return result, nil
}

// GetMessageTypeDistribution 获取指定会话的消息类型分布
func (r *Repository) GetMessageTypeDistribution(ctx context.Context, talker string) ([]*model.MessageTypeStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	typeStats := make(map[int]int)
	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			// 使用 (local_type & 0xFFFFFFFF) 排除打包后的系统消息
			query = fmt.Sprintf("SELECT local_type as type, COUNT(*) as count FROM %s WHERE (local_type & 4294967295) != 10000 GROUP BY type", tableName)
		} else {
			query = "SELECT Type as type, COUNT(*) as count FROM MSG WHERE Type != 10000"
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " GROUP BY type"
		}

		rows, _ := db.QueryContext(ctx, query, args...)
		if rows != nil {
			for rows.Next() {
				var t, c int
				rows.Scan(&t, &c)
				normalized := normalizeMessageType(t)
				if normalized == -1 {
					continue
				}
				typeStats[normalized] += c
			}
			rows.Close()
		}
	}
	var result []*model.MessageTypeStat
	for t, count := range typeStats {
		result = append(result, &model.MessageTypeStat{Type: t, Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result, nil
}

// GetMemberActivity 获取指定会话的成员活跃度排行
func (r *Repository) GetMemberActivity(ctx context.Context, talker string) ([]*model.MemberActivity, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	memberStats := make(map[string]int)
	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		if r.isTableExist(db, tableName) {
			// V4: 统计真实发送者
			query := fmt.Sprintf(`
				SELECT
					CASE
						WHEN m.status = 2 THEN 'self'
						WHEN n.user_name IS NOT NULL THEN n.user_name
						ELSE 'self'
					END as sender,
					COUNT(*) as count
				FROM %s m
				LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE (m.local_type & 4294967295) != 10000
				GROUP BY sender`, tableName)
			rows, _ := db.QueryContext(ctx, query, talker)
			if rows != nil {
				for rows.Next() {
					var s string
					var c int
					rows.Scan(&s, &c)
					memberStats[s] += c
				}
				rows.Close()
			}
		} else {
			// V3: 基础统计（仅能区分自己和对方）
			query := "SELECT CASE WHEN IsSender = 1 THEN 'self' ELSE StrTalker END as sender, COUNT(*) as count FROM MSG WHERE Type != 10000"
			var args []interface{}
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " GROUP BY sender"
			rows, _ := db.QueryContext(ctx, query, args...)
			if rows != nil {
				for rows.Next() {
					var s string
					var c int
					rows.Scan(&s, &c)
					memberStats[s] += c
				}
				rows.Close()
			}
		}
	}

	var senders []string
	for s := range memberStats {
		senders = append(senders, s)
	}
	profiles, _ := r.getContactProfiles(ctx, senders)
	var result []*model.MemberActivity
	for s, c := range memberStats {
		name := s
		var avatar string
		if p, ok := profiles[s]; ok {
			if p.Remark != "" {
				name = p.Remark
			} else if p.NickName != "" {
				name = p.NickName
			}
			avatar = p.SmallHeadURL
		} else if s == "self" {
			name = "我"
		}
		result = append(result, &model.MemberActivity{PlatformID: s, Name: name, MessageCount: c, Avatar: avatar})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].MessageCount > result[j].MessageCount })
	return result, nil
}

// GetRepeatAnalysis 复读机分析
func (r *Repository) GetRepeatAnalysis(ctx context.Context, talker string) ([]*model.RepeatStat, error) {
	targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)
	var allMsgs []struct {
		Content string
		Sender  string
	}

	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			query = fmt.Sprintf("SELECT m.message_content, n.user_name FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid WHERE m.local_type = 1 AND n.user_name IS NOT NULL ORDER BY m.create_time ASC", tableName)
		} else {
			query = "SELECT StrContent, CASE WHEN IsSender = 1 THEN 'self' ELSE StrTalker END FROM MSG WHERE Type = 1"
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
			query += " ORDER BY CreateTime ASC"
		}

		rows, _ := db.QueryContext(ctx, query, args...)
		if rows != nil {
			for rows.Next() {
				var content, sender string
				rows.Scan(&content, &sender)
				allMsgs = append(allMsgs, struct {
					Content string
					Sender  string
				}{content, sender})
			}
			rows.Close()
		}
	}

	if len(allMsgs) == 0 {
		return []*model.RepeatStat{}, nil
	}

	repeatCounts := make(map[string]*model.RepeatStat)
	lastContent := ""
	currentChainCount := 0
	lastSender := ""

	for _, msg := range allMsgs {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}

		if content == lastContent && msg.Sender != lastSender {
			currentChainCount++
			if currentChainCount >= 2 {
				if stat, ok := repeatCounts[content]; ok {
					stat.Count++
				} else {
					repeatCounts[content] = &model.RepeatStat{Content: content, Count: 1}
				}
			}
		} else {
			lastContent = content
			currentChainCount = 0
		}
		lastSender = msg.Sender
	}

	var result []*model.RepeatStat
	for _, s := range repeatCounts {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })

	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}

// GetPersonalTopContacts 统计“我”与谁聊得最多
func (r *Repository) GetPersonalTopContacts(ctx context.Context, limit int) ([]*model.PersonalTopContact, error) {
	// 获取所有会话列表作为基础，以确定需要扫描哪些 Talker
	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 1000})
	if err != nil {
		return nil, err
	}

	type stats struct {
		sent     int
		recv     int
		lastTime int64
	}
	aggStats := make(map[string]*stats)

	allowTalker := r.TalkerFilter(ctx, model.ModuleDashboard)
	for _, session := range sessions {
		talker := session.UserName
		// 排除群聊：个人社交分析只关注私聊记录
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}
		if !allowTalker(talker) {
			continue
		}

		targets := r.router.Resolve(time.Unix(0, 0), time.Now(), talker)

		s := &stats{}
		for _, target := range targets {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil {
				continue
			}
			hash := md5.Sum([]byte(talker))
			tableName := "Msg_" + hex.EncodeToString(hash[:])

			var query string
			var args []interface{}
			if r.isTableExist(db, tableName) {
				// V4: 在私聊中，发送人不是对方 (talker) 就一定是我
				// 判定条件：status=2 OR real_sender_id=0 OR n.user_name != talker
				query = fmt.Sprintf(`
					SELECT 
						CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) THEN 1 ELSE 0 END as is_self, 
						COUNT(*), 
						MAX(m.create_time) 
					FROM %s m
					LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
					WHERE (m.local_type & 4294967295) != 10000 
					GROUP BY is_self`, tableName)

				rows, err := db.QueryContext(ctx, query, talker)
				if err == nil && rows != nil {
					for rows.Next() {
						var isSelf, count int
						var maxTime int64
						if err := rows.Scan(&isSelf, &count, &maxTime); err == nil {
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
				continue
			} else {
				// V3: 使用 IsSender 代表发送
				query = "SELECT IsSender, COUNT(*), MAX(CreateTime/1000) FROM MSG WHERE Type != 10000"
				if target.TalkerID != 0 {
					query += " AND TalkerId = ?"
					args = append(args, target.TalkerID)
				} else {
					query += " AND StrTalker = ?"
					args = append(args, target.Talker)
				}
				query += " GROUP BY IsSender"
			}

			rows, err := db.QueryContext(ctx, query, args...)
			if err == nil && rows != nil {
				for rows.Next() {
					var isSelf, count int
					var maxTime int64
					if err := rows.Scan(&isSelf, &count, &maxTime); err == nil {
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
		if s.sent > 0 || s.recv > 0 {
			aggStats[talker] = s
		}
	}

	// 获取联系人基本信息以填充姓名和头像
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
			MessageCount: s.sent + s.recv,
			SentCount:    s.sent,
			RecvCount:    s.recv,
			LastTime:     s.lastTime,
		})
	}

	// 排序：按消息总数降序
	sort.Slice(result, func(i, j int) bool {
		return result[i].MessageCount > result[j].MessageCount
	})

	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// GetDashboardData 获取总览数据
func (r *Repository) GetDashboardData(ctx context.Context) (*model.DashboardData, error) {
	myWxid := r.getCurrentUserWxid(ctx)

	// 1. 获取基础统计：数据库大小和目录大小
	dbSize, _ := r.getDBSize()
	dirSize, _ := r.getDirSize()

	var totalMsgs, sentMsgs, recvMsgs int
	var earliest, latest int64
	allowTable := r.TableFilter(ctx, model.ModuleDashboard)
	allowTalker := r.TalkerFilter(ctx, model.ModuleDashboard)

	// 2. 遍历所有消息分片，直接统计全局总量和时间范围
	// 这样比按会话统计要快得多，且能覆盖所有消息
	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}

		// 检查是 V3 还是 V4
		if r.isTableExist(db, "MSG") {
			// V3 逻辑
			var count int
			var minT, maxT sql.NullInt64
			query := "SELECT COUNT(*), MIN(CreateTime/1000), MAX(CreateTime/1000) FROM MSG"
			err := db.QueryRowContext(ctx, query).Scan(&count, &minT, &maxT)
			if err == nil {
				totalMsgs += count
				if minT.Valid && (earliest == 0 || minT.Int64 < earliest) {
					earliest = minT.Int64
				}
				if maxT.Valid && maxT.Int64 > latest {
					latest = maxT.Int64
				}

				// 粗略估算发送/接收 (IsSender 在 V3 中通常存在)
				var sCount int
				_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM MSG WHERE IsSender = 1").Scan(&sCount)
				sentMsgs += sCount
				recvMsgs += (count - sCount)
			}
		} else {
			// V4 逻辑：查询所有 Msg_ 开头的表
			rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%'")
			if err == nil {
				for rows.Next() {
					var tableName string
					rows.Scan(&tableName)
					if !allowTable(tableName) {
						continue
					}

					var count int
					var minT, maxT sql.NullInt64
					query := fmt.Sprintf("SELECT COUNT(*), MIN(create_time), MAX(create_time) FROM %s", tableName)
					if err := db.QueryRowContext(ctx, query).Scan(&count, &minT, &maxT); err == nil {
						totalMsgs += count
						if minT.Valid && (earliest == 0 || minT.Int64 < earliest) {
							earliest = minT.Int64
						}
						if maxT.Valid && maxT.Int64 > latest {
							latest = maxT.Int64
						}

						// V4 增强版判定
						var sCount int
						if myWxid != "" {
							// 如果识别到了我的 wxid，那么 (是我的 ID OR status=2 OR sender_id=0) 都算我的
							q := fmt.Sprintf(`
								SELECT COUNT(*) FROM %s m 
								LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid 
								WHERE (n.user_name = ? OR m.status = 2 OR m.real_sender_id = 0) AND (m.local_type & 4294967295) != 10000`, tableName)
							_ = db.QueryRowContext(ctx, q, myWxid).Scan(&sCount)
						} else {
							_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE (status = 2 OR real_sender_id = 0)", tableName)).Scan(&sCount)
						}
						sentMsgs += sCount
						recvMsgs += (count - sCount)
					}
				}
				rows.Close()
			}
		}
	}

	// 3. 统计活跃群聊：直接从 Session 库中统计
	var groups []model.DashboardGroup
	sessions, _ := r.GetSessions(ctx, types.SessionQuery{Limit: 200})
	for _, s := range sessions {
		if !allowTalker(s.UserName) {
			continue
		}
		if strings.HasSuffix(s.UserName, "@chatroom") {
			groups = append(groups, model.DashboardGroup{
				ChatRoomName: s.UserName,
				NickName:     s.NickName,
				MessageCount: 0, // 这里的 Count 暂时不准也没关系，因为获取所有群的精确 Count 太慢
			})
		}
	}
	// 尝试为前几个群获取精确计数
	for i := 0; i < len(groups) && i < 10; i++ {
		targets := r.router.Resolve(time.Unix(0, 0), time.Now(), groups[i].ChatRoomName)
		count := 0
		for _, target := range targets {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil {
				continue
			}
			hash := md5.Sum([]byte(groups[i].ChatRoomName))
			tableName := "Msg_" + hex.EncodeToString(hash[:])
			if r.isTableExist(db, tableName) {
				_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
			} else if r.isTableExist(db, "MSG") {
				_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM MSG WHERE StrTalker = ?", groups[i].ChatRoomName).Scan(&count)
			}
		}
		groups[i].MessageCount = count
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].MessageCount > groups[j].MessageCount })

	durationDays := 0
	if latest > earliest && earliest > 0 {
		durationDays = int((latest - earliest) / 86400)
	}

	return &model.DashboardData{
		Overview: model.DashboardOverview{
			User: "微信用户",
			DBStats: model.DBStats{
				DBSizeMB:  float64(dbSize) / 1024 / 1024,
				DirSizeMB: float64(dirSize) / 1024 / 1024,
			},
			MsgStats: model.MsgStats{
				TotalMsgs:      totalMsgs,
				SentMsgs:       sentMsgs,
				ReceivedMsgs:   recvMsgs,
				UniqueMsgTypes: 0,
			},
			MsgTypes: make(map[string]int),
			Groups:   groups,
			Timeline: model.DashboardTimeline{
				EarliestMsgTime: earliest,
				LatestMsgTime:   latest,
				DurationDays:    durationDays,
			},
		},
	}, nil
}

func (r *Repository) getDBSize() (int64, error) {
	var total int64
	// 简单实现：只统计 message/ 下的 db
	files, _ := filepath.Glob(filepath.Join(r.router.GetBaseDir(), "message", "*.db"))
	for _, f := range files {
		if s, err := os.Stat(f); err == nil {
			total += s.Size()
		}
	}
	return total, nil
}

func (r *Repository) getDirSize() (int64, error) {
	var total int64
	_ = filepath.Walk(r.router.GetBaseDir(), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, nil
}

// SearchGlobalMessages 全局搜索消息（跨所有分片和表）
func (r *Repository) SearchGlobalMessages(ctx context.Context, q types.MessageQuery) ([]*model.Message, error) {
	var allMessages []*model.Message

	var talkerMD5Map map[string]string

	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}

		// 区分版本进行搜索
		if r.isTableExist(db, "MSG") {
			// V3 搜索
			msgs, _ := r.searchV3Global(ctx, db, q)
			allMessages = append(allMessages, msgs...)
		} else {
			// V4 搜索：遍历所有 Msg_xxx 表
			if talkerMD5Map == nil {
				talkerMD5Map = r.getTalkerMD5Map(ctx)
			}
			msgs, _ := r.searchV4Global(ctx, db, q, talkerMD5Map)
			allMessages = append(allMessages, msgs...)
		}

		if len(allMessages) > 1000 { // 限制全局搜索返回结果数量
			break
		}
	}

	// 丰富信息
	if len(allMessages) > 0 {
		r.enrichMessages(ctx, allMessages)
	}

	// 排序
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].Seq > allMessages[j].Seq // 全局搜索通常看最新的
	})

	return allMessages, nil
}

func (r *Repository) getTalkerMD5Map(ctx context.Context) map[string]string {
	res := make(map[string]string)
	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 0})
	if err != nil {
		return res
	}
	for _, s := range sessions {
		h := md5.Sum([]byte(s.UserName))
		res[hex.EncodeToString(h[:])] = s.UserName
	}
	return res
}

func (r *Repository) searchV3Global(ctx context.Context, db *sql.DB, q types.MessageQuery) ([]*model.Message, error) {
	conds := []string{"StrContent LIKE ?"}
	args := []interface{}{"%" + q.Keyword + "%"}
	if !q.StartTime.IsZero() {
		conds = append(conds, "CreateTime >= ?")
		args = append(args, q.StartTime.Unix())
	}
	if !q.EndTime.IsZero() {
		conds = append(conds, "CreateTime <= ?")
		args = append(args, q.EndTime.Unix())
	}
	args = append(args, q.Limit)
	query := "SELECT MsgSvrID, Sequence, CreateTime, StrTalker, IsSender, Type, SubType, StrContent, CompressContent, BytesExtra FROM MSG WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY CreateTime DESC LIMIT ?"
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*model.Message
	for rows.Next() {
		var msg model.MessageV3
		var compressContent, bytesExtra []byte
		rows.Scan(&msg.MsgSvrID, &msg.Sequence, &msg.CreateTime, &msg.StrTalker, &msg.IsSender, &msg.Type, &msg.SubType, &msg.StrContent, &compressContent, &bytesExtra)
		msg.CompressContent = compressContent
		msg.BytesExtra = bytesExtra
		msgs = append(msgs, msg.Wrap())
	}
	return msgs, nil
}

func (r *Repository) searchV4Global(ctx context.Context, db *sql.DB, q types.MessageQuery, talkerMD5Map map[string]string) ([]*model.Message, error) {
	tables, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%'")
	if err != nil {
		return nil, err
	}
	defer tables.Close()

	var msgs []*model.Message
	keywordParam := "%" + q.Keyword + "%"

	// 时间范围过滤
	timeCond := ""
	timeArgs := []interface{}{}
	if !q.StartTime.IsZero() {
		timeCond += " AND m.create_time >= ?"
		timeArgs = append(timeArgs, q.StartTime.Unix())
	}
	if !q.EndTime.IsZero() {
		timeCond += " AND m.create_time <= ?"
		timeArgs = append(timeArgs, q.EndTime.Unix())
	}

	for tables.Next() {
		var tableName string
		tables.Scan(&tableName)

		// V4 Search: Try to match content in message_content or compress_content
		// Note: This works for uncompressed text. Compressed zstd blobs won't match LIKE.
		query := fmt.Sprintf(`
			SELECT m.sort_seq, m.server_id, m.local_type, n.user_name, m.create_time, m.message_content, m.packed_info_data, m.status
			FROM %s m
			LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
			WHERE (m.message_content LIKE ? OR m.compress_content LIKE ?)%s
			ORDER BY m.create_time DESC LIMIT ?`, tableName, timeCond)

		args := append([]interface{}{keywordParam, keywordParam}, timeArgs...)
		args = append(args, q.Limit)
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			// Some tables might not have compress_content column, fallback to message_content only
			query = fmt.Sprintf(`
				SELECT m.sort_seq, m.server_id, m.local_type, n.user_name, m.create_time, m.message_content, m.packed_info_data, m.status
				FROM %s m
				LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE m.message_content LIKE ?%s
				ORDER BY m.create_time DESC LIMIT ?`, tableName, timeCond)
			args2 := append([]interface{}{keywordParam}, timeArgs...)
			args2 = append(args2, q.Limit)
			rows, err = db.QueryContext(ctx, query, args2...)
			if err != nil {
				continue
			}
		}

		// Derive talker from tableName
		talker := ""
		if strings.HasPrefix(tableName, "Msg_") {
			md5Hash := strings.TrimPrefix(tableName, "Msg_")
			if t, ok := talkerMD5Map[md5Hash]; ok {
				talker = t
			}
		}
		if talker == "" {
			rows.Close()
			continue
		}

		for rows.Next() {
			var msg model.MessageV4
			rows.Scan(&msg.SortSeq, &msg.ServerID, &msg.LocalType, &msg.UserName, &msg.CreateTime, &msg.MessageContent, &msg.PackedInfoData, &msg.Status)

			wrapped := msg.Wrap(talker)
			msgs = append(msgs, wrapped)
		}
		rows.Close()
		if len(msgs) >= q.Limit {
			break
		}
	}
	return msgs, nil
}

// GetTextMessagesGlobal 高效拉取全局文本消息内容（仅 Type=1），
// 用于词云分析，避免 LIKE 全表扫和 1000 条硬限制。
func (r *Repository) GetTextMessagesGlobal(ctx context.Context, start, end time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5000
	}
	var texts []string
	for _, shard := range r.router.GetShards() {
		if len(texts) >= limit {
			break
		}
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			msgs := r.fetchV3GlobalTexts(ctx, db, start, end, limit-len(texts))
			texts = append(texts, msgs...)
		} else {
			msgs := r.fetchV4GlobalTexts(ctx, db, start, end, limit-len(texts))
			texts = append(texts, msgs...)
		}
	}
	return texts, nil
}

func (r *Repository) fetchV3GlobalTexts(ctx context.Context, db *sql.DB, start, end time.Time, limit int) []string {
	conds := []string{"Type = 1"}
	args := []interface{}{}
	// V3 的 CreateTime 是毫秒时间戳（本文件其它 V3 查询都是 *1000）。
	// 这里早先传的是秒，条件几乎恒不成立，全局词云在 V3 上直接返回空。
	if !start.IsZero() {
		conds = append(conds, "CreateTime >= ?")
		args = append(args, start.Unix()*1000)
	}
	if !end.IsZero() {
		conds = append(conds, "CreateTime <= ?")
		args = append(args, end.Unix()*1000)
	}
	args = append(args, limit)
	query := "SELECT StrContent FROM MSG WHERE " + strings.Join(conds, " AND ") + " ORDER BY CreateTime DESC LIMIT ?"
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var s sql.NullString
		if rows.Scan(&s) == nil && s.Valid && s.String != "" {
			texts = append(texts, s.String)
		}
	}
	return texts
}

func (r *Repository) fetchV4GlobalTexts(ctx context.Context, db *sql.DB, start, end time.Time, limit int) []string {
	tables, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Msg_%%'")
	if err != nil {
		return nil
	}
	defer tables.Close()
	allowTable := r.TableFilter(ctx, model.ModuleWordCloud)
	var tableNames []string
	for tables.Next() {
		var name string
		if tables.Scan(&name) == nil && allowTable(name) {
			tableNames = append(tableNames, name)
		}
	}
	tables.Close()

	timeCond := ""
	timeArgs := []interface{}{}
	if !start.IsZero() {
		timeCond += " AND create_time >= ?"
		timeArgs = append(timeArgs, start.Unix())
	}
	if !end.IsZero() {
		timeCond += " AND create_time <= ?"
		timeArgs = append(timeArgs, end.Unix())
	}

	var texts []string
	for _, t := range tableNames {
		if len(texts) >= limit {
			break
		}
		// 直接读 BLOB 列以便处理 zstd 压缩
		q := fmt.Sprintf("SELECT message_content FROM %s WHERE local_type = 1%s ORDER BY create_time DESC LIMIT ?", t, timeCond)
		args := append([]interface{}{}, timeArgs...)
		args = append(args, limit-len(texts))
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) != nil || len(raw) == 0 {
				continue
			}
			content := decodeV4Content(raw)
			content = stripGroupSenderPrefix(content)
			if content != "" {
				texts = append(texts, content)
			}
		}
		rows.Close()
	}
	return texts
}

// decodeV4Content 处理可能被 zstd 压缩的 message_content
func decodeV4Content(raw []byte) string {
	if len(raw) >= 4 && raw[0] == 0x28 && raw[1] == 0xb5 && raw[2] == 0x2f && raw[3] == 0xfd {
		if b, err := zstd.Decompress(raw); err == nil {
			return string(b)
		}
		return ""
	}
	return string(raw)
}

// stripGroupSenderPrefix 移除群聊消息开头的 "wxid_xxx:\n" 或 "name:\n" 前缀
func stripGroupSenderPrefix(content string) string {
	if idx := strings.Index(content, ":\n"); idx > 0 && idx < 80 {
		return content[idx+2:]
	}
	return content
}

// GetCallStats 计算指定会话的通话统计。
// start/end 为零值时统计全部历史。
func (r *Repository) GetCallStats(ctx context.Context, talker string, start, end time.Time) (*model.CallStats, error) {
	stats := &model.CallStats{}
	if start.IsZero() {
		start = time.Unix(0, 0)
	}
	if end.IsZero() {
		end = time.Now()
	}
	targets := r.router.Resolve(start, end, talker)
	if len(targets) == 0 {
		return stats, nil
	}

	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil {
			continue
		}
		hash := md5.Sum([]byte(talker))
		tableName := "Msg_" + hex.EncodeToString(hash[:])

		// 取所有通话消息内容（type=50）
		var query string
		var args []interface{}
		if r.isTableExist(db, tableName) {
			query = fmt.Sprintf("SELECT message_content FROM %s WHERE (local_type & 4294967295) = 50 AND create_time >= ? AND create_time <= ?", tableName)
			args = append(args, start.Unix(), end.Unix())
		} else {
			query = "SELECT StrContent FROM MSG WHERE Type = 50 AND CreateTime >= ? AND CreateTime <= ?"
			args = append(args, start.Unix()*1000, end.Unix()*1000)
			if target.TalkerID != 0 {
				query += " AND TalkerId = ?"
				args = append(args, target.TalkerID)
			} else {
				query += " AND StrTalker = ?"
				args = append(args, target.Talker)
			}
		}

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) != nil || len(raw) == 0 {
				continue
			}
			content := decodeV4Content(raw)
			content = stripGroupSenderPrefix(content)
			info := util.ParseVoIP(content)

			stats.TotalCalls++
			if info.Duration > 0 {
				stats.CompletedCalls++
				stats.TotalDuration += info.Duration
				if info.Duration > stats.LongestDuration {
					stats.LongestDuration = info.Duration
				}
			} else {
				stats.MissedCalls++
			}
			if info.IsVideo {
				stats.VideoCalls++
			} else {
				stats.VoiceCalls++
			}
		}
		rows.Close()
	}

	if stats.CompletedCalls > 0 {
		stats.AvgDuration = stats.TotalDuration / stats.CompletedCalls
	}
	return stats, nil
}
