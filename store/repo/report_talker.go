package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
)

// GetTalkerAnnualReport 生成单个联系人的年度报告。
//
// 与全局年度报告（GetAnnualReport，跨分片裸 SQL 聚合）不同，这里复用经过验证的
// GetMessages 取出该 talker 当年的全部消息，再在 Go 内存里做聚合。这样无需为
// 每张分表单独写 SQL，v3 / v4 两种库结构由 GetMessages 内部统一处理。
//
// defaultTzOffset 单位为秒（东正西负，UTC+8 = 28800）。
func (r *Repository) GetTalkerAnnualReport(ctx context.Context, year int, talker string, defaultTzOffset int) (*model.AnnualReport, error) {
	loc := time.FixedZone("report", defaultTzOffset)
	start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, loc)

	msgs, err := r.GetMessages(ctx, types.MessageQuery{
		Talker:    talker,
		StartTime: start.UTC(),
		EndTime:   end.UTC(),
		Limit:     1000000,
	})
	if err != nil {
		return nil, err
	}

	report := &model.AnnualReport{
		Year:         year,
		MessageTypes: make(map[string]int),
	}

	monthly := make(map[int]int)   // 1-12
	weekday := make(map[int]int)   // 1-7（周一到周日）
	hourly := make(map[int]int)    // 0-23
	typeStats := make(map[int]int) // 原始消息类型
	dailyCounts := make(map[string]int)

	var total, sent, recv, lateNight int
	earliestMinute, latestMinute := -1, -1
	firstDate, lastDate := "", ""

	for _, m := range msgs {
		t := m.Time.In(loc)
		dateStr := t.Format("2006-01-02")

		total++
		if m.IsSelf {
			sent++
		} else {
			recv++
		}

		monthly[int(t.Month())]++

		// time.Weekday: 0=周日..6=周六，转成 1=周一..7=周日
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		weekday[wd]++

		hour := t.Hour()
		hourly[hour]++
		if hour < 6 {
			lateNight++
		}

		dailyCounts[dateStr]++
		typeStats[int(m.Type)]++

		minuteOfDay := hour*60 + t.Minute()
		if earliestMinute < 0 || minuteOfDay < earliestMinute {
			earliestMinute = minuteOfDay
		}
		if minuteOfDay > latestMinute {
			latestMinute = minuteOfDay
		}

		if firstDate == "" || dateStr < firstDate {
			firstDate = dateStr
		}
		if dateStr > lastDate {
			lastDate = dateStr
		}
	}

	// 活跃天数改为「从首条聊天记录起算」—— 把和该联系人全周期的消息都拉
	// 一遍，按 talker 时区折算成日期再去重。其余概览数仍按当年的算。
	lifetimeMsgs, _ := r.GetMessages(ctx, types.MessageQuery{
		Talker:    talker,
		StartTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Now().Add(24 * time.Hour),
		Limit:     10_000_000,
	})
	lifetimeDays := make(map[string]bool)
	for _, m := range lifetimeMsgs {
		lifetimeDays[m.Time.In(loc).Format("2006-01-02")] = true
	}
	activeDaysLifetime := len(lifetimeDays)
	if activeDaysLifetime == 0 {
		activeDaysLifetime = len(dailyCounts) // 兜底，避免历史拉取失败时显示 0
	}

	// 日历热力图 / 语音 / 互动与回复 —— 与聊天页数据分析面板同一套口径
	report.Extras = r.GetTalkerExtras(ctx, talker, year, defaultTzOffset)

	// 概览
	activeContactCount, activeChatroomCount := 0, 0
	if total > 0 {
		if strings.HasSuffix(talker, "@chatroom") {
			activeChatroomCount = 1
		} else {
			activeContactCount = 1
		}
	}
	report.Overview = model.AnnualOverview{
		TotalMessages:    total,
		SentMessages:     sent,
		ReceivedMessages: recv,
		// 单个会话的报告：是群聊就算 1 个活跃群聊，否则算 1 个活跃联系人；
		// 当年没有消息则都为 0。早先无条件写死「联系人 1 / 群聊 0」，
		// 看群聊报告或空年份时是错的。
		ActiveContacts:     activeContactCount,
		ActiveChatrooms:    activeChatroomCount,
		ActiveDays:         len(dailyCounts),
		ActiveDaysLifetime: activeDaysLifetime,
		FirstMessageDate:   firstDate,
		LastMessageDate:    lastDate,
	}

	// 联系人年度报告也给出「相对往年同期的百分比变化」—— 用刚拉到的全周期
	// 消息在内存里分桶,逐年算 totalMessages/sent/recv/activeDays,再求往年
	// 平均,与当年对比。pastStartYear 缺省 2023(与 effectiveChatStartYear 默认一致)。
	const pastStartYear = 2023
	type bucket struct {
		total, sent, recv int
		days              map[string]bool
	}
	pastByYear := make(map[int]*bucket)
	cutMonth, cutDay := time.December, 31
	now := time.Now()
	if year == now.Year() {
		cutMonth = now.Month()
		cutDay = now.Day()
	}
	for _, m := range lifetimeMsgs {
		t := m.Time.In(loc)
		py := t.Year()
		if py >= year || py < pastStartYear {
			continue
		}
		// 截止到「当年同期」—— 例如当前 5/20,过往各年只算到 5/20
		if t.Month() > cutMonth || (t.Month() == cutMonth && t.Day() > cutDay) {
			continue
		}
		b, ok := pastByYear[py]
		if !ok {
			b = &bucket{days: make(map[string]bool)}
			pastByYear[py] = b
		}
		b.total++
		if m.IsSelf {
			b.sent++
		} else {
			b.recv++
		}
		b.days[t.Format("2006-01-02")] = true
	}
	if len(pastByYear) > 0 {
		var sumT, sumS, sumR, sumD int
		for _, b := range pastByYear {
			sumT += b.total
			sumS += b.sent
			sumR += b.recv
			sumD += len(b.days)
		}
		n := len(pastByYear)
		avgT, avgS, avgR, avgD := sumT/n, sumS/n, sumR/n, sumD/n
		calc := func(cur, past int) *float64 {
			if past == 0 {
				return nil
			}
			p := (float64(cur) - float64(past)) / float64(past) * 100
			return &p
		}
		report.OverviewDeltas = &model.OverviewDeltas{
			TotalMessages:    calc(total, avgT),
			SentMessages:     calc(sent, avgS),
			ReceivedMessages: calc(recv, avgR),
			ActiveDays:       calc(activeDaysLifetime, avgD),
			// ActiveContacts / ActiveChatrooms 对单联系人不适用
		}
	}

	// 月度趋势
	for i := 1; i <= 12; i++ {
		report.MonthlyTrend = append(report.MonthlyTrend, &model.MonthlyStat{Month: i, Count: monthly[i]})
	}

	// 星期分布
	for i := 1; i <= 7; i++ {
		report.WeekdayDist = append(report.WeekdayDist, &model.WeekdayStat{Weekday: i, Count: weekday[i]})
	}

	// 24 小时分布
	for i := 0; i <= 23; i++ {
		report.HourlyDist = append(report.HourlyDist, &model.HourlyStat{Hour: i, Count: hourly[i]})
	}

	// 消息类型分布
	for typ, count := range typeStats {
		report.MessageTypes[messageTypeName(typ)] += count
	}

	// 亮点
	highlights := model.AnnualHighlights{}
	var busiest model.DayCount
	quietest := model.DayCount{Count: int(^uint(0) >> 1)}
	for date, count := range dailyCounts {
		if count > busiest.Count {
			busiest = model.DayCount{Date: date, Count: count}
		}
		if count < quietest.Count {
			quietest = model.DayCount{Date: date, Count: count}
		}
	}
	if quietest.Count == int(^uint(0)>>1) {
		quietest = model.DayCount{}
	}
	highlights.BusiestDay = busiest
	highlights.QuietestDay = quietest
	highlights.LateNightCount = lateNight
	highlights.LongestStreak = calcLongestStreak(dailyCounts)
	if earliestMinute >= 0 {
		highlights.EarliestMessageTime = fmt.Sprintf("%02d:%02d", earliestMinute/60, earliestMinute%60)
	}
	if latestMinute >= 0 {
		highlights.LatestMessageTime = fmt.Sprintf("%02d:%02d", latestMinute/60, latestMinute%60)
	}
	report.Highlights = highlights

	report.DataVersion = r.GetDataVersion()
	return report, nil
}
