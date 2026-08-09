package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
	"github.com/rs/zerolog/log"
)

// reportSegment 是内部使用的分段，已携带解析好的 loc 和 tzMod
type reportSegment struct {
	start time.Time
	end   time.Time
	loc   *time.Location
	tzMod string
}

// tzModifier 将偏移秒数转为 SQLite strftime 修饰符
func tzModifier(loc *time.Location) string {
	if loc == nil {
		return "'localtime'"
	}
	_, offset := time.Unix(0, 0).In(loc).Zone()
	if offset == 0 {
		return "'utc'"
	}
	return fmt.Sprintf("'%+d seconds'", offset)
}

// buildSegments 将用户指定的 TZSegment 列表补全为覆盖整年的连续分段。
// defaultTzOffset 是用户明确指定的默认时区偏移（秒），用于填充没有被 segments 覆盖的区间。
// 重叠的 segments 会被自动裁剪（后来者覆盖先来者）。
func buildSegments(year int, defaultTzOffset int, userSegs []types.TZSegment) []reportSegment {
	defaultLoc := time.FixedZone("default", defaultTzOffset)

	// 年界必须按用户时区划，不能用 UTC。
	// 早先写成 time.Date(year,1,1,...,time.UTC)，对 UTC+8 用户来说
	// 「2026 年」实际是本地时间 2026-01-01 08:00 到 2027-01-01 07:59 ——
	// 元旦头 8 小时被漏掉，还混进了下一年元旦的头 8 小时。
	yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, defaultLoc)
	yearEnd := time.Date(year, 12, 31, 23, 59, 59, 999999999, defaultLoc)

	// 排序用户 segments（按开始时间升序）
	sorted := make([]types.TZSegment, len(userSegs))
	copy(sorted, userSegs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})

	var result []reportSegment
	cursor := yearStart

	for _, seg := range sorted {
		// 裁剪到年范围
		segStart := seg.Start
		segEnd := seg.End
		if segEnd.Before(yearStart) || segStart.After(yearEnd) {
			continue
		}
		if segStart.Before(yearStart) {
			segStart = yearStart
		}
		if segEnd.After(yearEnd) {
			segEnd = yearEnd
		}

		// 已被之前 segment 覆盖的部分跳过（cursor 推进后，segStart 可能已落后于 cursor）
		if cursor.After(segEnd) {
			continue
		}
		effectiveStart := segStart
		if cursor.After(segStart) {
			effectiveStart = cursor
		}

		// 填补 cursor 到 effectiveStart 的空隙（用默认时区）
		if cursor.Before(effectiveStart) {
			result = append(result, reportSegment{
				start: cursor,
				end:   effectiveStart.Add(-time.Nanosecond),
				loc:   defaultLoc,
				tzMod: tzModifier(defaultLoc),
			})
		}

		loc := time.FixedZone("seg", seg.TZOffset)
		result = append(result, reportSegment{
			start: effectiveStart,
			end:   segEnd,
			loc:   loc,
			tzMod: tzModifier(loc),
		})
		cursor = segEnd.Add(time.Nanosecond)
	}

	// 填补末尾到年末的空隙
	if cursor.Before(yearEnd) {
		result = append(result, reportSegment{
			start: cursor,
			end:   yearEnd,
			loc:   defaultLoc,
			tzMod: tzModifier(defaultLoc),
		})
	}

	// 如果没有任何 segment，整年用默认时区
	if len(result) == 0 {
		result = append(result, reportSegment{
			start: yearStart,
			end:   yearEnd,
			loc:   defaultLoc,
			tzMod: tzModifier(defaultLoc),
		})
	}

	return result
}

// computePastYearsMonthlyAvg 计算「往年月均」参考线。
// 固定按 [pastStartYear, 当前年] 计算，与所看报告的年份无关，保证同一条参考线不随切换报告年份而改变。
// 用于年度报告内置默认参考线。
func (r *Repository) computePastYearsMonthlyAvg(ctx context.Context, year, pastStartYear, defaultTzOffset int) []*model.MonthlyStat {
	if pastStartYear < 2010 {
		pastStartYear = 2023
	}
	_ = year // 参考线固定到当前年，不再随报告年份变化
	to := time.Now().Year()
	return r.ComputeMonthlyAvgInRange(ctx, pastStartYear, to, defaultTzOffset)
}

// ComputeMonthlyAvgInRange 计算 [fromYear, toYear] 闭区间内每年月度趋势的平均值。
// 输出 12 个月的平均值（无数据的月份为 0）。
func (r *Repository) ComputeMonthlyAvgInRange(ctx context.Context, fromYear, toYear, tzOffsetSeconds int) []*model.MonthlyStat {
	monthlyTotal := make(map[int]int)
	monthlyCount := make(map[int]int)

	loc := time.FixedZone("default", tzOffsetSeconds)
	tzMod := tzModifier(loc)
	nowLoc := time.Now().In(loc)

	if fromYear > toYear {
		fromYear, toYear = toYear, fromYear
	}

	for py := fromYear; py <= toYear; py++ {
		seg := reportSegment{
			start: time.Date(py, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(py, 12, 31, 23, 59, 59, 999999999, time.UTC),
			loc:   loc,
			tzMod: tzMod,
		}
		trend := r.getAnnualMonthlyTrend(ctx, []reportSegment{seg}, r.ReportTableFilter(ctx, nil))
		var anyData bool
		for _, t := range trend {
			if t.Count > 0 {
				anyData = true
				break
			}
		}
		if !anyData {
			continue
		}
		// 当前年是不完整的：只统计已经过去的月份，避免尚未到来的月份（count=0）把该月均值拉低
		maxMonth := 12
		if py == nowLoc.Year() {
			maxMonth = int(nowLoc.Month())
		}
		for _, t := range trend {
			if t.Month > maxMonth {
				continue
			}
			monthlyTotal[t.Month] += t.Count
			monthlyCount[t.Month] += 1
		}
	}

	result := make([]*model.MonthlyStat, 0, 12)
	for m := 1; m <= 12; m++ {
		avg := 0
		if monthlyCount[m] > 0 {
			avg = monthlyTotal[m] / monthlyCount[m]
		}
		result = append(result, &model.MonthlyStat{Month: m, Count: avg})
	}
	return result
}

// GetAnnualReport 获取年度报告数据
// pastStartYear 是「有效聊天记录起始年份」，用作往年同期对比的下界
func (r *Repository) GetAnnualReport(ctx context.Context, year int, defaultTzOffset, pastStartYear int, userSegs []types.TZSegment, excludeTalkers []string) (*model.AnnualReport, error) {
	segs := buildSegments(year, defaultTzOffset, userSegs)

	excludeSet := make(map[string]bool, len(excludeTalkers))
	for _, t := range excludeTalkers {
		excludeSet[t] = true
	}

	report := &model.AnnualReport{
		Year:         year,
		MessageTypes: make(map[string]int),
	}

	// 排除名单与统计范围要贯穿报告的每一个分区，不能只作用于亲密度排行
	allow := r.ReportTableFilter(ctx, excludeSet)

	yearStart := segs[0].start
	yearEnd := segs[len(segs)-1].end

	// 各分区互不依赖，且全是只读 SQLite 查询 —— 串行跑等于把每一趟全表扫描
	// 的时间加起来。并行之后总耗时约等于最慢的那一个分区。
	// （唯一的依赖：同比 delta 需要先有 overview，放在 Wait 之后算。）
	var wg sync.WaitGroup
	var overview model.AnnualOverview

	run := func(name string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					log.Warn().Interface("panic", p).Str("section", name).Msg("年度报告分区计算异常")
				}
			}()
			fn()
		}()
	}

	run("概览", func() {
		ov, err := r.getAnnualOverview(ctx, segs, allow)
		if err != nil {
			log.Warn().Err(err).Msg("获取年度概览失败")
		}
		overview = ov
	})
	run("亲密度排行", func() {
		tc, err := r.getAnnualTopContacts(ctx, yearStart, yearEnd, 20, excludeSet, model.ModuleReport)
		if err != nil {
			log.Warn().Err(err).Msg("获取年度亲密度排行失败")
		}
		report.TopContacts = tc
	})
	run("月度趋势", func() { report.MonthlyTrend = r.getAnnualMonthlyTrend(ctx, segs, allow) })
	run("往年月均", func() {
		report.PastYearsMonthlyAvg = r.computePastYearsMonthlyAvg(ctx, year, pastStartYear, defaultTzOffset)
	})
	run("星期分布", func() { report.WeekdayDist = r.getAnnualWeekdayDist(ctx, segs, allow) })
	run("小时分布", func() { report.HourlyDist = r.getAnnualHourlyDist(ctx, segs, allow) })
	run("类型分布", func() { report.MessageTypes = r.getAnnualMessageTypes(ctx, yearStart, yearEnd, allow) })
	run("亮点", func() { report.Highlights = r.getAnnualHighlights(ctx, segs, allow) })
	wg.Wait()

	report.Overview = overview
	// 同比依赖 overview，必须等上面跑完
	report.OverviewDeltas = r.computeOverviewDeltas(ctx, year, pastStartYear, defaultTzOffset, overview)

	// 设置数据版本指纹（供前端缓存校验）
	report.DataVersion = r.GetDataVersion()

	return report, nil
}

// GetAnnualReportWithProgress 获取年度报告数据，并通过 progressFn 回调报告每步进度和部分数据。
// progressFn 接收步骤名、当前步数(1-based)、总步数(8)、该步骤产出的部分数据。
func (r *Repository) GetAnnualReportWithProgress(ctx context.Context, year int, defaultTzOffset, pastStartYear int, userSegs []types.TZSegment, excludeTalkers []string, progressFn types.ProgressCallback) (*model.AnnualReport, error) {
	segs := buildSegments(year, defaultTzOffset, userSegs)

	excludeSet := make(map[string]bool, len(excludeTalkers))
	for _, t := range excludeTalkers {
		excludeSet[t] = true
	}

	report := &model.AnnualReport{
		Year:         year,
		MessageTypes: make(map[string]int),
	}

	// 排除名单与统计范围要贯穿报告的每一个分区，不能只作用于亲密度排行
	allow := r.ReportTableFilter(ctx, excludeSet)

	const totalSteps = 8
	yearStart := segs[0].start
	yearEnd := segs[len(segs)-1].end

	// 1. 获取概览数据
	overview, err := r.getAnnualOverview(ctx, segs, allow)
	if err != nil {
		log.Warn().Err(err).Msg("获取年度概览失败")
	}
	report.Overview = overview
	if progressFn != nil {
		progressFn("overview", 1, totalSteps, overview)
	}

	// 2. 获取亲密度排行
	topContacts, err := r.getAnnualTopContacts(ctx, yearStart, yearEnd, 20, excludeSet, model.ModuleReport)
	if err != nil {
		log.Warn().Err(err).Msg("获取年度亲密度排行失败")
	}
	report.TopContacts = topContacts
	if progressFn != nil {
		progressFn("top_contacts", 2, totalSteps, topContacts)
	}

	// 3. 获取月度趋势
	report.MonthlyTrend = r.getAnnualMonthlyTrend(ctx, segs, allow)
	if progressFn != nil {
		progressFn("monthly_trend", 3, totalSteps, report.MonthlyTrend)
	}

	// 4. 计算往年月度趋势平均
	report.PastYearsMonthlyAvg = r.computePastYearsMonthlyAvg(ctx, year, pastStartYear, defaultTzOffset)
	if progressFn != nil {
		progressFn("past_years_avg", 4, totalSteps, report.PastYearsMonthlyAvg)
	}

	// 5. 获取星期分布
	report.WeekdayDist = r.getAnnualWeekdayDist(ctx, segs, allow)
	if progressFn != nil {
		progressFn("weekday_dist", 5, totalSteps, report.WeekdayDist)
	}

	// 6. 获取小时分布
	report.HourlyDist = r.getAnnualHourlyDist(ctx, segs, allow)
	if progressFn != nil {
		progressFn("hourly_dist", 6, totalSteps, report.HourlyDist)
	}

	// 7. 获取消息类型分布
	report.MessageTypes = r.getAnnualMessageTypes(ctx, yearStart, yearEnd, allow)
	if progressFn != nil {
		progressFn("message_types", 7, totalSteps, report.MessageTypes)
	}

	// 8. 获取亮点数据 + 往年 delta
	report.Highlights = r.getAnnualHighlights(ctx, segs, allow)
	report.OverviewDeltas = r.computeOverviewDeltas(ctx, year, pastStartYear, defaultTzOffset, overview)
	report.DataVersion = r.GetDataVersion()
	if progressFn != nil {
		progressFn("highlights", 8, totalSteps, map[string]interface{}{
			"highlights":      report.Highlights,
			"overview_deltas": report.OverviewDeltas,
		})
	}

	return report, nil
}

// ComputePastOverviewAvg 计算往年同期 (YTD) 概览数据的平均
// 给定 year, pastStartYear, defaultTzOffset，返回 [pastStartYear, year-1] 各年同期 overview 的平均
// 当没有任何有效往年数据时返回 nil
func (r *Repository) ComputePastOverviewAvg(ctx context.Context, year, pastStartYear, defaultTzOffset int) *model.AnnualOverview {
	now := time.Now()
	currentYear := now.Year()
	loc := time.FixedZone("default", defaultTzOffset)
	tzMod := tzModifier(loc)

	cutoffMonth := time.December
	cutoffDay := 31
	if year == currentYear {
		cutoffMonth = now.Month()
		cutoffDay = now.Day()
	}

	if pastStartYear < 2010 {
		pastStartYear = 2023
	}

	var sumTotal, sumSent, sumRecv, sumContacts, sumRooms, sumDays int
	count := 0
	for py := pastStartYear; py <= year-1; py++ {
		startUTC := time.Date(py, 1, 1, 0, 0, 0, 0, time.UTC)
		endUTC := time.Date(py, cutoffMonth, cutoffDay, 23, 59, 59, 999999999, time.UTC)
		if endUTC.Before(startUTC) {
			continue
		}
		seg := reportSegment{start: startUTC, end: endUTC, loc: loc, tzMod: tzMod}
		ov, err := r.getAnnualOverview(ctx, []reportSegment{seg}, r.ReportTableFilter(ctx, nil))
		if err != nil || ov.TotalMessages == 0 {
			continue
		}
		sumTotal += ov.TotalMessages
		sumSent += ov.SentMessages
		sumRecv += ov.ReceivedMessages
		sumContacts += ov.ActiveContacts
		sumRooms += ov.ActiveChatrooms
		sumDays += ov.ActiveDays
		count++
	}
	if count == 0 {
		return nil
	}
	return &model.AnnualOverview{
		TotalMessages:    sumTotal / count,
		SentMessages:     sumSent / count,
		ReceivedMessages: sumRecv / count,
		ActiveContacts:   sumContacts / count,
		ActiveChatrooms:  sumRooms / count,
		ActiveDays:       sumDays / count,
	}
}

// computeOverviewDeltas 用 ComputePastOverviewAvg 的结果与 current 比较得出百分比差
func (r *Repository) computeOverviewDeltas(ctx context.Context, year, pastStartYear, defaultTzOffset int, current model.AnnualOverview) *model.OverviewDeltas {
	avg := r.ComputePastOverviewAvg(ctx, year, pastStartYear, defaultTzOffset)
	if avg == nil {
		return nil
	}
	calc := func(cur, past int) *float64 {
		if past == 0 {
			return nil
		}
		p := (float64(cur) - float64(past)) / float64(past) * 100
		return &p
	}
	return &model.OverviewDeltas{
		TotalMessages:    calc(current.TotalMessages, avg.TotalMessages),
		SentMessages:     calc(current.SentMessages, avg.SentMessages),
		ReceivedMessages: calc(current.ReceivedMessages, avg.ReceivedMessages),
		ActiveContacts:   calc(current.ActiveContacts, avg.ActiveContacts),
		ActiveChatrooms:  calc(current.ActiveChatrooms, avg.ActiveChatrooms),
		ActiveDays:       calc(current.ActiveDays, avg.ActiveDays),
	}
}
