package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// ── 今日报告 ───────────────────────────────────────────────────
// GetDailyReport 统计某一天的收发概况、时段分布与聊得最多的人。
//
// 方向判定（哪条是我发的）**逐字照搬 scanAnnualV4**，所以「今天发了多少条」
// 与年度报告里的累计口径一致，不会出现两个页面数字对不上的情况。
//
// 一天的数据量很小，直接一趟扫完，不做缓存。
// exclude 是设置页的「忽略的联系人」名单，被忽略的人连总数都不计入。
func (r *Repository) GetDailyReport(ctx context.Context, date string, tzOffsetSec, topN int, exclude []string) *model.DailyReport {
	out := &model.DailyReport{
		Date:     date,
		Hourly:   make([]int, 24),
		Partners: []*model.DailyPartner{},
		Types:    []*model.DailyTypeCount{},
	}

	loc := time.FixedZone("tz", tzOffsetSec)
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return out
	}
	// 日界按用户时区划，理由同 buildSegments 对年界的处理：
	// 用 UTC 划会把凌晨几小时算丢、又混进前一天的尾巴。
	start := day
	end := day.AddDate(0, 0, 1).Add(-time.Second)

	allowTable := r.TableFilter(ctx, model.ModuleInsights)
	excluded := make(map[string]bool, len(exclude))
	for _, t := range exclude {
		excluded[t] = true
	}

	myWxid := r.getCurrentUserWxid(ctx)
	md5ToTalker := r.getTalkerMD5Map(ctx)

	type agg struct {
		sent, recv int
		lastUnix   int64
	}
	byTalker := map[string]*agg{}
	typeCount := map[int]int{}

	for _, shard := range r.router.GetShards() {
		// 分片有时间范围，与当天无交集的直接跳过，省一次开库
		if !shard.StartTime.IsZero() && shard.StartTime.After(end) {
			continue
		}
		if !shard.EndTime.IsZero() && shard.EndTime.Before(start) {
			continue
		}
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}

		// Name2Id 读一次进内存，避免逐条 JOIN
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
			if !allowTable(tbl) {
				continue
			}
			talker := md5ToTalker[strings.TrimPrefix(tbl, "Msg_")]
			if excluded[talker] {
				continue
			}
			if talker == "" {
				talker = "unknown"
			}
			isGroup := isChatroomTalker(talker)

			rows, err := db.QueryContext(ctx, fmt.Sprintf(
				"SELECT COALESCE(create_time,0), COALESCE(local_type,0) & 4294967295, "+
					"COALESCE(status,0), COALESCE(real_sender_id,0) FROM %s "+
					"WHERE create_time >= ? AND create_time <= ? AND (local_type & 4294967295) != 10000",
				tbl), start.Unix(), end.Unix())
			if err != nil {
				continue
			}
			for rows.Next() {
				var ts int64
				var localType, status int
				var senderID int64
				if rows.Scan(&ts, &localType, &status, &senderID) != nil {
					continue
				}
				// 与 scanAnnualV4 完全一致的方向判定
				isSelf := status == 2 || senderID == 0
				if !isSelf {
					sender := name2id[senderID]
					if isGroup || talker == "unknown" {
						isSelf = sender == myWxid && myWxid != ""
					} else {
						isSelf = sender != talker
					}
				}

				a := byTalker[talker]
				if a == nil {
					a = &agg{}
					byTalker[talker] = a
				}
				if isSelf {
					a.sent++
					out.SentMessages++
				} else {
					a.recv++
					out.RecvMessages++
				}
				if ts > a.lastUnix {
					a.lastUnix = ts
				}
				out.TotalMessages++
				typeCount[localType]++

				t := time.Unix(ts, 0).In(loc)
				out.Hourly[t.Hour()]++
				if out.FirstTime == 0 || ts < out.FirstTime {
					out.FirstTime = ts
				}
				if ts > out.LastTime {
					out.LastTime = ts
				}
			}
			rows.Close()
		}
	}

	// 名字解析：与 month_partners 用同一套（备注 > 昵称 > 用户名）
	usernames := make([]string, 0, len(byTalker))
	for t := range byTalker {
		usernames = append(usernames, t)
	}
	profiles, _ := r.getContactProfiles(ctx, usernames)

	for talker, a := range byTalker {
		total := a.sent + a.recv
		if total == 0 {
			continue
		}
		name, _ := displayName(talker, profiles)
		isGroup := isChatroomTalker(talker)
		if isGroup {
			out.ActiveGroups++
		} else {
			out.ActivePeers++
		}
		out.Partners = append(out.Partners, &model.DailyPartner{
			Username: talker,
			Name:     name,
			IsGroup:  isGroup,
			Messages: total,
			Sent:     a.sent,
			Recv:     a.recv,
			LastTime: a.lastUnix,
		})
	}

	sort.Slice(out.Partners, func(i, j int) bool {
		if out.Partners[i].Messages != out.Partners[j].Messages {
			return out.Partners[i].Messages > out.Partners[j].Messages
		}
		return out.Partners[i].Username < out.Partners[j].Username // 同分定序，保证可复现
	})
	out.TotalPeers = len(out.Partners)
	if topN > 0 && len(out.Partners) > topN {
		out.Partners = out.Partners[:topN]
	}

	for t, c := range typeCount {
		out.Types = append(out.Types, &model.DailyTypeCount{
			Type: t, Name: dailyTypeLabel(t), Count: c,
		})
	}
	sort.Slice(out.Types, func(i, j int) bool {
		if out.Types[i].Count != out.Types[j].Count {
			return out.Types[i].Count > out.Types[j].Count
		}
		return out.Types[i].Type < out.Types[j].Type
	})

	return out
}

// dailyTypeLabel 把微信的 local_type 映射成人话。
// 与前端年度报告的类型名保持一致，避免同一个类型两个页面两种叫法。
func dailyTypeLabel(t int) string {
	switch t {
	case 1:
		return "文本"
	case 3:
		return "图片"
	case 34:
		return "语音"
	case 43:
		return "视频"
	case 47:
		return "表情"
	case 48:
		return "位置"
	case 49:
		return "链接/文件"
	case 10000:
		return "系统消息"
	default:
		return fmt.Sprintf("类型 %d", t)
	}
}
