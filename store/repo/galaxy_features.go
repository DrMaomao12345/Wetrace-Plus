package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
)

// rawFeatures 是单个私聊联系人在「全部历史」上的原始统计特征(未归一化)。
type rawFeatures struct {
	contactID string
	remark    string
	nickName  string
	avatar    string

	total, sent, recv             int
	firstTime, lastTime           int64
	activeDays                    int
	monthCounts                   map[string]int // YYYY-MM -> 消息数
	sessionCount                  int
	recent30, recent90, recent365 int
}

func (f *rawFeatures) activeMonths() int { return len(f.monthCounts) }

// longestActiveStreak 返回最长连续活跃月份数。
func (f *rawFeatures) longestActiveStreak() int {
	if len(f.monthCounts) == 0 {
		return 0
	}
	months := make([]string, 0, len(f.monthCounts))
	for m := range f.monthCounts {
		months = append(months, m)
	}
	// 月份字符串可直接字典序排序
	sortStrings(months)
	best, cur := 1, 1
	for i := 1; i < len(months); i++ {
		if isNextMonth(months[i-1], months[i]) {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 1
		}
	}
	return best
}

// spanMonths 返回首末有效聊天之间的总月份数(用于活跃月份占比)。
func (f *rawFeatures) spanMonths() int {
	if f.firstTime == 0 || f.lastTime == 0 {
		return 0
	}
	a := time.Unix(f.firstTime, 0)
	b := time.Unix(f.lastTime, 0)
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month()) + 1
}

// ExtractGalaxyFeatures 抽取所有一对一联系人的原始特征(§15.2:第一版只用私聊)。
func (r *Repository) ExtractGalaxyFeatures(ctx context.Context, tzOffsetSec int) ([]*rawFeatures, error) {
	loc := time.FixedZone("galaxy", tzOffsetSec)
	tzMod := tzModifier(loc)
	rangeStart := time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Now()
	now := time.Now().Unix()
	t30, t90, t365 := now-30*86400, now-90*86400, now-365*86400

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 8000})
	if err != nil {
		return nil, err
	}

	out := make([]*rawFeatures, 0, 256)
	var talkers []string

	allowTalker := r.TalkerFilter(ctx, model.ModuleGalaxy)
	for _, s := range sessions {
		talker := s.UserName
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}
		if !allowTalker(talker) {
			continue
		}
		f := &rawFeatures{contactID: talker, monthCounts: map[string]int{}}
		tbl := v4TableName(talker)

		for _, target := range r.router.Resolve(rangeStart, rangeEnd, talker) {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil || !r.isTableExist(db, tbl) {
				continue
			}
			r.galaxyMonthly(ctx, db, tbl, talker, tzMod, f)
			r.galaxySessionsRecency(ctx, db, tbl, t30, t90, t365, f)
		}

		if f.total > 0 {
			out = append(out, f)
			talkers = append(talkers, talker)
		}
	}

	// 批量补 备注/昵称/头像
	profiles, _ := r.getContactProfiles(ctx, talkers)
	for _, f := range out {
		if p, ok := profiles[f.contactID]; ok {
			f.remark, f.nickName, f.avatar = p.Remark, p.NickName, p.SmallHeadURL
		}
	}
	return out, nil
}

// galaxyMonthly 累加月度消息/活跃天/发收/首末时间。
func (r *Repository) galaxyMonthly(ctx context.Context, db *sql.DB, tbl, talker, tzMod string, f *rawFeatures) {
	q := fmt.Sprintf(`
		SELECT strftime('%%Y-%%m', m.create_time, 'unixepoch', %s) AS mon,
			COUNT(*) AS cnt,
			COUNT(DISTINCT strftime('%%Y-%%m-%%d', m.create_time, 'unixepoch', %s)) AS days,
			COALESCE(SUM(CASE WHEN (m.status=2 OR m.real_sender_id=0 OR n.user_name != ?) THEN 1 ELSE 0 END),0) AS sent,
			COALESCE(SUM(CASE WHEN NOT (m.status=2 OR m.real_sender_id=0 OR n.user_name != ?) THEN 1 ELSE 0 END),0) AS recv,
			MIN(m.create_time) AS mn, MAX(m.create_time) AS mx
		FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
		WHERE (m.local_type & 4294967295) != 10000
		GROUP BY mon`, tzMod, tzMod, tbl)
	rows, err := db.QueryContext(ctx, q, talker, talker)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var mon string
		var cnt, days, sent, recv int
		var mn, mx sql.NullInt64
		if rows.Scan(&mon, &cnt, &days, &sent, &recv, &mn, &mx) != nil {
			continue
		}
		f.monthCounts[mon] += cnt
		f.total += cnt
		f.sent += sent
		f.recv += recv
		f.activeDays += days
		if mn.Valid && (f.firstTime == 0 || mn.Int64 < f.firstTime) {
			f.firstTime = mn.Int64
		}
		if mx.Valid && mx.Int64 > f.lastTime {
			f.lastTime = mx.Int64
		}
	}
}

// galaxySessionsRecency 累加会话数(30min 间隔)与近 30/90/365 天消息数。
func (r *Repository) galaxySessionsRecency(ctx context.Context, db *sql.DB, tbl string, t30, t90, t365 int64, f *rawFeatures) {
	q := fmt.Sprintf(`
		WITH o AS (
			SELECT m.create_time AS ct,
				LAG(m.create_time) OVER (ORDER BY m.create_time, m.local_id) AS prev
			FROM %s m
			WHERE (m.local_type & 4294967295) != 10000
		)
		SELECT
			COALESCE(SUM(CASE WHEN prev IS NULL OR ct-prev > 1800 THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN ct >= ? THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN ct >= ? THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN ct >= ? THEN 1 ELSE 0 END),0)
		FROM o`, tbl)
	var sess, r30, r90, r365 sql.NullInt64
	if err := db.QueryRowContext(ctx, q, t30, t90, t365).Scan(&sess, &r30, &r90, &r365); err == nil {
		f.sessionCount += int(sess.Int64)
		f.recent30 += int(r30.Int64)
		f.recent90 += int(r90.Int64)
		f.recent365 += int(r365.Int64)
	}
}

// —— 小工具 ——
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// isNextMonth 判断 b 是否是 a 的下一个自然月(a,b 形如 "2024-05")。
func isNextMonth(a, b string) bool {
	ay, am, ok1 := parseYM(a)
	by, bm, ok2 := parseYM(b)
	if !ok1 || !ok2 {
		return false
	}
	if am == 12 {
		return by == ay+1 && bm == 1
	}
	return by == ay && bm == am+1
}

func parseYM(s string) (int, int, bool) {
	if len(s) != 7 || s[4] != '-' {
		return 0, 0, false
	}
	y := atoiSafe(s[:4])
	m := atoiSafe(s[5:])
	if y == 0 || m == 0 {
		return 0, 0, false
	}
	return y, m, true
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
