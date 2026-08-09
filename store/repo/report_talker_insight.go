package repo

import (
	"context"
	"sort"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

const (
	replyCapSec  = 6 * 3600 // 超过 6 小时的间隔不算「回复」
	newConvGap   = 3 * 3600 // 静默超过 3 小时再开口，算一次新的对话发起
	instantReply = 120      // 秒回的阈值
)

// callWindowsForTalker 取某个会话在时间范围内的通话区间
func (r *Repository) callWindowsForTalker(ctx context.Context, talker string, start, end time.Time) []callWindow {
	var out []callWindow
	tbl := v4TableName(talker)
	for _, target := range r.router.Resolve(start, end, talker) {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil || !r.isTableExist(db, tbl) {
			continue
		}
		out = append(out, r.callWindows(ctx, db, tbl, start, end)...)
	}
	return out
}

// talkerInsight 在内存里算出单个联系人的双向互动比与回复速度。
//
// 和关系洞察页那套 SQL 是同一套口径（同样的 6 小时上限、3 小时对话间隔、
// 同样剔除通话期间的间隔），区别只是这里数据量小，直接遍历更简单。
func talkerInsight(msgs []*model.Message, loc *time.Location, calls []callWindow) (*model.InteractionRatio, *model.ReplySpeed) {
	sorted := make([]*model.Message, len(msgs))
	copy(sorted, msgs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })

	ir := &model.InteractionRatio{}
	rs := &model.ReplySpeed{}

	var mySum, myCnt, theirSum, theirCnt int
	myMin := 1 << 30
	var prev *model.Message

	for _, m := range sorted {
		if m.Type == 10000 {
			continue // 系统消息不参与
		}
		if m.IsSelf {
			ir.SentCount++
		} else {
			ir.RecvCount++
		}

		if prev != nil {
			gap := int(m.Time.Sub(prev.Time).Seconds())

			// 静默够久之后的第一条，算一次「主动发起对话」
			if gap >= newConvGap {
				if m.IsSelf {
					ir.MyInitiations++
				} else {
					ir.TheirInitiations++
				}
			}

			// 方向反转 = 一次回复；通话期间的来回不算
			inCall := overlapsCall(calls, prev.Time.Unix(), m.Time.Unix())
			if gap < replyCapSec && m.IsSelf != prev.IsSelf && !inCall {
				if m.IsSelf {
					mySum += gap
					myCnt++
					if gap < myMin {
						myMin = gap
					}
					if gap > rs.MySlowestSec {
						rs.MySlowestSec = gap
					}
					if gap < instantReply {
						if h := m.Time.In(loc).Hour(); h <= 5 {
							rs.LateNightInstant++
						}
					}
				} else {
					theirSum += gap
					theirCnt++
				}
			}
		} else {
			// 第一条消息也算一次发起
			if m.IsSelf {
				ir.MyInitiations++
			} else {
				ir.TheirInitiations++
			}
		}
		prev = m
	}

	// 对方发的消息后面没有跟上我的回复 —— 近似「已读不回」。
	// 口径与关系洞察页的 SQL 保持一致（那边是 is_me=0 AND (nim=0 OR nim IS NULL)）。
	for i, m := range sorted {
		if m.IsSelf || m.Type == 10000 {
			continue
		}
		if i == len(sorted)-1 || !sorted[i+1].IsSelf {
			rs.IgnoredByThem++
		}
	}

	ir.Total = ir.SentCount + ir.RecvCount
	if n := len(sorted); n > 0 {
		ir.LastTime = sorted[n-1].Time.Unix()
	}
	rs.MyReplyCount, rs.TheirReplyCount = myCnt, theirCnt
	if myCnt > 0 {
		rs.MyAvgReplySec = mySum / myCnt
		if myMin < (1 << 30) {
			rs.MyFastestSec = myMin
		}
	}
	if theirCnt > 0 {
		rs.TheirAvgReplySec = theirSum / theirCnt
	}
	return ir, rs
}
