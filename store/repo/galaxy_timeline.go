package repo

import (
	"math"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// BuildTimeline 从已提取的月度消息数(rawFeatures.monthCounts)生成陪伴时间轴(§10-12)。
// 第一版月度强度以消息量为主,对数压缩后跨全体归一化,保证不同联系人可比较。
func BuildTimeline(feats []*rawFeatures, nodes []*model.GalaxyNode) *model.GalaxyTimeline {
	fmap := make(map[string]*rawFeatures, len(feats))
	for _, f := range feats {
		fmap[f.contactID] = f
	}

	// 全体最大单月消息数,用于归一化
	maxCount := 1
	for _, nd := range nodes {
		if f := fmap[nd.ContactID]; f != nil {
			for _, c := range f.monthCounts {
				if c > maxCount {
					maxCount = c
				}
			}
		}
	}
	logMax := math.Log(1 + float64(maxCount))
	strengthOf := func(c int) int {
		if c <= 0 || logMax == 0 {
			return 0
		}
		return int(math.Round(100 * math.Log(1+float64(c)) / logMax))
	}

	monthSet := map[string]bool{}
	gSum := map[string]int{}
	gActive := map[string]int{}
	gStrong := map[string]int{}
	gMsg := map[string]int{}

	contacts := make([]*model.GalaxyContactTimeline, 0, len(nodes))
	for _, nd := range nodes {
		f := fmap[nd.ContactID]
		if f == nil || len(f.monthCounts) == 0 {
			continue
		}
		months := make([]string, 0, len(f.monthCounts))
		for m := range f.monthCounts {
			months = append(months, m)
		}
		sortStrings(months)

		segs := make([]model.GalaxyMonthSegment, 0, len(months))
		for _, m := range months {
			cnt := f.monthCounts[m]
			s := strengthOf(cnt)
			segs = append(segs, model.GalaxyMonthSegment{Month: m, Strength: s, MessageCount: cnt})
			monthSet[m] = true
			gSum[m] += s
			gMsg[m] += cnt
			if s >= 20 {
				gActive[m]++
			}
			if s >= 50 {
				gStrong[m]++
			}
		}
		contacts = append(contacts, &model.GalaxyContactTimeline{
			ContactID: nd.ContactID, DisplayName: nd.DisplayName, Avatar: nd.Avatar,
			Type: nd.RelationshipType, Segments: segs,
			FirstMonth: months[0], LastMonth: months[len(months)-1],
		})
	}

	// 连续月份序列(min..max)
	allMonths := make([]string, 0, len(monthSet))
	for m := range monthSet {
		allMonths = append(allMonths, m)
	}
	sortStrings(allMonths)
	var fullMonths []string
	if len(allMonths) > 0 {
		fullMonths = monthRange(allMonths[0], allMonths[len(allMonths)-1])
	}

	// 全局热力:每月强度和 → 对数压缩 → 0-100
	maxSum := 1
	for _, v := range gSum {
		if v > maxSum {
			maxSum = v
		}
	}
	logMaxSum := math.Log(1 + float64(maxSum))
	heat := make([]*model.GalaxyGlobalHeat, 0, len(fullMonths))
	for _, m := range fullMonths {
		hs := 0
		if gSum[m] > 0 && logMaxSum > 0 {
			hs = int(math.Round(100 * math.Log(1+float64(gSum[m])) / logMaxSum))
		}
		heat = append(heat, &model.GalaxyGlobalHeat{
			Month: m, HeatScore: hs,
			ActiveRelationships: gActive[m], StrongRelationships: gStrong[m],
			MessageCount: gMsg[m],
		})
	}

	return &model.GalaxyTimeline{
		Granularity: "month", Months: fullMonths,
		Contacts: contacts, GlobalHeat: heat,
	}
}

// monthRange 返回 [a, b] 闭区间内所有连续月份(形如 "2024-05")。
func monthRange(a, b string) []string {
	ta, e1 := time.Parse("2006-01", a)
	tb, e2 := time.Parse("2006-01", b)
	if e1 != nil || e2 != nil || tb.Before(ta) {
		return []string{a}
	}
	var out []string
	for t := ta; !t.After(tb); t = t.AddDate(0, 1, 0) {
		out = append(out, t.Format("2006-01"))
	}
	return out
}
