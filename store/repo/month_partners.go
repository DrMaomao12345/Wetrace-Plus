package repo

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// ── 日历热力图月份下钻 ─────────────────────────────────────────
// GetMonthPartners 统计某年某月里，你和每个会话**分别聊了多少天**。
//
// 为什么按「天数」而不是「条数」排：条数容易被一两次爆发式聊天带偏，
// 而「这个月有多少天在联系」更能反映陪伴密度 —— 这也是这个下钻想回答的问题。
//
// limit <= 0 表示不截断。TotalPeers 始终是截断前的真实会话数。
func (r *Repository) GetMonthPartners(ctx context.Context, year, month, tzOffsetSec, limit int) *model.MonthPartners {
	out := &model.MonthPartners{Year: year, Month: month, Partners: []*model.MonthPartner{}}
	if month < 1 || month > 12 {
		return out
	}

	// 月界必须按用户时区划，理由同 buildSegments 里对年界的处理：
	// 用 UTC 划会把月初几小时算丢、又混进上个月的尾巴。
	loc := time.FixedZone("tz", tzOffsetSec)
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	tzMod := tzModifier(loc)

	// 表名是 Msg_<md5(talker)>，只能正向算 md5 再反查。
	// 取联系人库而不是会话列表 —— 会话列表可能不含只在群里出现过的对象。
	autoTypes := r.TalkerTypes(ctx)
	md5ToTalker := make(map[string]string, len(autoTypes))
	for talker := range autoTypes {
		h := md5.Sum([]byte(talker))
		md5ToTalker[hex.EncodeToString(h[:])] = talker
	}

	allowTable := r.TableFilter(ctx, model.ModuleInsights)

	type agg struct {
		days     map[string]struct{}
		messages int
	}
	byTalker := make(map[string]*agg)
	unionDays := make(map[string]struct{})
	totalMsgs := 0

	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		for _, tbl := range r.listMsgTables(ctx, db) {
			if !allowTable(tbl) {
				continue
			}
			talker := md5ToTalker[strings.TrimPrefix(tbl, "Msg_")]
			if talker == "" {
				// 联系人库里查不到的表（已删除的会话等），跳过而不是记成一个
				// md5 乱码的「联系人」——那会在界面上显示成一串十六进制
				continue
			}
			q := fmt.Sprintf(
				"SELECT strftime('%%Y-%%m-%%d', create_time, 'unixepoch', %s) d, COUNT(*) "+
					"FROM %s WHERE create_time >= ? AND create_time <= ? "+
					"AND (local_type & 4294967295)!=10000 GROUP BY d", tzMod, tbl)
			rows, err := db.QueryContext(ctx, q, start.Unix(), end.Unix())
			if err != nil {
				continue
			}
			a := byTalker[talker]
			if a == nil {
				a = &agg{days: map[string]struct{}{}}
				byTalker[talker] = a
			}
			for rows.Next() {
				var d string
				var c int
				if rows.Scan(&d, &c) != nil {
					continue
				}
				a.days[d] = struct{}{}
				a.messages += c
				unionDays[d] = struct{}{}
				totalMsgs += c
			}
			rows.Close()
			if len(a.days) == 0 {
				delete(byTalker, talker) // 该月没有互动，别留一条全 0 的记录
			}
		}
	}

	out.TotalDays = len(unionDays)
	out.TotalMsgs = totalMsgs
	out.TotalPeers = len(byTalker)
	if len(byTalker) == 0 {
		return out
	}

	usernames := make([]string, 0, len(byTalker))
	for t := range byTalker {
		usernames = append(usernames, t)
	}
	profiles, _ := r.getContactProfiles(ctx, usernames)

	for t, a := range byTalker {
		name, avatar := displayName(t, profiles)
		out.Partners = append(out.Partners, &model.MonthPartner{
			Username: t,
			Name:     name,
			Avatar:   avatar,
			IsGroup:  strings.HasSuffix(t, "@chatroom"),
			Days:     len(a.days),
			Messages: a.messages,
		})
	}
	sort.Slice(out.Partners, func(i, j int) bool {
		if out.Partners[i].Days != out.Partners[j].Days {
			return out.Partners[i].Days > out.Partners[j].Days
		}
		if out.Partners[i].Messages != out.Partners[j].Messages {
			return out.Partners[i].Messages > out.Partners[j].Messages
		}
		return out.Partners[i].Username < out.Partners[j].Username // 同分定序，保证可复现
	})
	if limit > 0 && len(out.Partners) > limit {
		out.Partners = out.Partners[:limit]
	}
	return out
}
