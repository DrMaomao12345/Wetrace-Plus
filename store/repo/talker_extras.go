package repo

import (
	"context"
	"sort"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// GetTalkerExtras 汇总单个联系人的日历热力图、语音统计与关系洞察指标。
// year <= 0 表示统计全部历史。
func (r *Repository) GetTalkerExtras(ctx context.Context, talker string, year, tzOffsetSec int) *model.TalkerExtras {
	loc := time.FixedZone("extras", tzOffsetSec)
	var start, end time.Time
	if year > 0 {
		start = time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		end = time.Date(year, 12, 31, 23, 59, 59, 0, loc)
	} else {
		start = time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Now()
	}

	out := &model.TalkerExtras{Year: year}

	msgs, err := r.GetMessages(ctx, types.MessageQuery{
		Talker:    talker,
		StartTime: start.UTC(),
		EndTime:   end.UTC(),
		Limit:     1000000,
	})
	if err != nil {
		return out
	}

	// 日历热力图。要和关系洞察页的全局热力图同口径 —— 那边的 SQL 排除了
	// 系统消息（local_type=10000），这里也必须排，否则同一天的数字对不上。
	daily := map[string]int{}
	for _, m := range msgs {
		if m.Type == 10000 {
			continue
		}
		daily[m.Time.In(loc).Format("2006-01-02")]++
	}
	heat := make([]*model.DayHeat, 0, len(daily))
	for d, c := range daily {
		heat = append(heat, &model.DayHeat{Date: d, Count: c})
	}
	sort.Slice(heat, func(i, j int) bool { return heat[i].Date < heat[j].Date })
	out.CalendarHeatmap = heat

	// 语音统计
	out.Voice = r.GetVoiceStats(ctx, talker, start, end)

	// 互动比 + 回复速度（剔除通话期间）
	calls := r.callWindowsForTalker(ctx, talker, start, end)
	out.CallWindows = len(calls)
	ir, rs := talkerInsight(msgs, loc, calls)
	ir.Talker, rs.Talker = talker, talker
	out.Interaction, out.ReplySpeed = ir, rs

	return out
}
