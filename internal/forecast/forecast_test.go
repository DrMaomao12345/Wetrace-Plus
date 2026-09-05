package forecast

import (
	"fmt"
	"testing"
	"time"
)

// mkDaily 从 start 起连续 n 天，每天 counts[i%len] 条。
func mkDaily(start string, n int, counts ...int) map[string]int {
	t, _ := time.Parse("2006-01-02", start)
	m := make(map[string]int, n)
	for i := 0; i < n; i++ {
		c := counts[i%len(counts)]
		if c > 0 {
			m[t.AddDate(0, 0, i).Format("2006-01-02")] = c
		}
	}
	return m
}

func in(daily map[string]int, today string) Input {
	d, _ := time.Parse("2006-01-02", today)
	return Input{Daily: daily, Today: d, Year: d.Year(), Seed: 42}
}

// 一直没聊的联系人必须给出 0 和 [0,0]，不需要任何特例分支。
func TestSilentContactPredictsZero(t *testing.T) {
	// 前 200 天有量，最近 120 天彻底沉默
	daily := mkDaily("2025-09-01", 200, 5)
	res := Forecast(in(daily, "2026-09-05"))

	if len(res.Points) != 4 { // 9,10,11,12
		t.Fatalf("应有 4 个点，实得 %d", len(res.Points))
	}
	for _, p := range res.Points {
		if p.Point != 0 || p.Lo != 0 || p.Hi != 0 {
			t.Errorf("%d月 应全 0，实得 point=%d [%d,%d]", p.Month, p.Point, p.Lo, p.Hi)
		}
	}
	if res.Confidence != "low" {
		t.Errorf("彻底沉默应判 low，实得 %s", res.Confidence)
	}
}

// 稳定日更的联系人：当月预测应当落在「实际 + 剩余天数×速率」附近。
func TestSteadyContactNowcast(t *testing.T) {
	daily := mkDaily("2026-01-01", 248, 10) // 每天 10 条，一直到 9/5
	res := Forecast(in(daily, "2026-09-05"))

	cur := res.Points[0]
	if cur.Month != 9 || cur.Horizon != 0 || !cur.HasActual {
		t.Fatalf("首个点应是当月：%+v", cur)
	}
	if cur.Actual != 50 { // 9/1~9/5 共 5 天 ×10
		t.Errorf("当月实际应为 50，实得 %d", cur.Actual)
	}
	// 30 天 ×10 = 300，实际 50 + 剩余 25 天 ×10 = 300
	if cur.Point < 270 || cur.Point > 330 {
		t.Errorf("当月预测应在 300 附近，实得 %d（区间 %d~%d）", cur.Point, cur.Lo, cur.Hi)
	}
	// 稳定序列的区间应该很窄
	if cur.Hi-cur.Lo > 60 {
		t.Errorf("稳定序列区间过宽：%d~%d", cur.Lo, cur.Hi)
	}
}

// 点必须落在区间内，且等于上下界的平均（用户指定的口径）。
func TestPointIsMidpointOfInterval(t *testing.T) {
	daily := mkDaily("2026-01-01", 248, 0, 3, 0, 12, 1) // 有零有爆发
	res := Forecast(in(daily, "2026-09-05"))

	for _, p := range res.Points {
		if p.Point < p.Lo || p.Point > p.Hi {
			t.Errorf("%d月 点落在区间外：%d ∉ [%d,%d]", p.Month, p.Point, p.Lo, p.Hi)
		}
		mid := (p.Lo + p.Hi) / 2
		if diff := p.Point - mid; diff < -1 || diff > 1 {
			t.Errorf("%d月 点(%d)与上下界均值(%d)相差超过 1（取整误差）", p.Month, p.Point, mid)
		}
	}
}

// 预测越远区间越宽 —— 这是「自证不确定」的视觉基础，不能退化。
func TestIntervalWidensWithHorizon(t *testing.T) {
	daily := mkDaily("2026-01-01", 248, 0, 3, 0, 12, 1)
	res := Forecast(in(daily, "2026-01-15")) // 从 1 月看，horizon 0~11

	// 跳过当月（当月有一半已成事实，天然更窄），比较 2 月 vs 12 月
	var feb, dec MonthPoint
	for _, p := range res.Points {
		if p.Month == 2 {
			feb = p
		}
		if p.Month == 12 {
			dec = p
		}
	}
	if dec.Hi-dec.Lo <= feb.Hi-feb.Lo {
		t.Errorf("12月区间(%d)应比2月(%d)宽", dec.Hi-dec.Lo, feb.Hi-feb.Lo)
	}
}

// 最近窗口暴涨时不能无限外推 —— 趋势有衰减、也有上限。
func TestSurgeIsNotExtrapolated(t *testing.T) {
	// 前 62 天每天 1 条，最近 28 天每天 50 条
	daily := map[string]int{}
	start, _ := time.Parse("2006-01-02", "2026-06-06")
	for i := 0; i < 90; i++ {
		c := 1
		if i >= 62 {
			c = 50
		}
		daily[start.AddDate(0, 0, i).Format("2006-01-02")] = c
	}
	res := Forecast(Input{Daily: daily, Today: mustDate("2026-09-04"), Year: 2026, Seed: 7})

	for _, p := range res.Points {
		if p.Horizon == 0 {
			continue // 当月一半已成事实
		}
		days := daysIn(2026, p.Month)
		// 最近水平 50/天，上限 +50% → 75/天。留一点重采样噪声余量
		if cap := int(float64(days) * 75 * 1.15); p.Point > cap {
			t.Errorf("%d月 预测 %d 超出上限 %d —— 趋势被无限外推了", p.Month, p.Point, cap)
		}
	}
}

// 正在冷却的联系人：不能因为更早的一次爆发把预测又拽上去。
// 这是回归长期均值和衰减趋势的关键分野。
func TestCoolingContactDoesNotReboundToOldSpike(t *testing.T) {
	// 90 天窗口：前 34 天每天 400 条（旧爆发），中间 28 天 200，最近 28 天 100
	daily := map[string]int{}
	start, _ := time.Parse("2006-01-02", "2026-06-06")
	for i := 0; i < 90; i++ {
		c := 400
		if i >= 34 && i < 62 {
			c = 200
		} else if i >= 62 {
			c = 100
		}
		daily[start.AddDate(0, 0, i).Format("2006-01-02")] = c
	}
	res := Forecast(Input{Daily: daily, Today: mustDate("2026-09-04"), Year: 2026, Seed: 9})

	for _, p := range res.Points {
		if p.Horizon == 0 {
			continue
		}
		days := daysIn(2026, p.Month)
		// 最近水平是 100/天，正在下降。任何未来月都不该超过它
		if cap := int(float64(days) * 100 * 1.15); p.Point > cap {
			t.Errorf("%d月 预测 %d 超过最近水平 %d —— 被旧爆发拽回去了", p.Month, p.Point, cap)
		}
	}
}

// 同样的输入必须给同样的结果 —— 图表不该每次刷新都跳。
func TestDeterministic(t *testing.T) {
	daily := mkDaily("2026-01-01", 248, 0, 3, 0, 12, 1)
	a := Forecast(in(daily, "2026-09-05"))
	b := Forecast(in(daily, "2026-09-05"))
	for i := range a.Points {
		if a.Points[i] != b.Points[i] {
			t.Fatalf("第 %d 个点不稳定：%+v vs %+v", i, a.Points[i], b.Points[i])
		}
	}
}

// 只预测当年；查看往年时不返回预测点。
func TestPastYearNoForecast(t *testing.T) {
	daily := mkDaily("2024-01-01", 600, 5)
	d := mustDate("2026-09-05")
	res := Forecast(Input{Daily: daily, Today: d, Year: 2025, Seed: 1})
	if len(res.Points) != 0 {
		t.Errorf("往年不该有预测点，实得 %d 个", len(res.Points))
	}
}

// 12 月查看时只剩当月一个点。
func TestDecemberOnlyCurrentMonth(t *testing.T) {
	daily := mkDaily("2026-01-01", 340, 4)
	res := Forecast(in(daily, "2026-12-10"))
	if len(res.Points) != 1 || res.Points[0].Month != 12 {
		t.Errorf("12月应只剩 1 个点，实得 %+v", res.Points)
	}
}

// 置信度分级跟着零消息天比例走。
func TestConfidenceTiers(t *testing.T) {
	// 阈值取自回测分层：高频零消息天 ~30%、中频 ~84%、低频 ~98%。
	// 故意避开 50% / 90% 两个边界值，别让用例卡在临界上。
	cases := []struct {
		name  string
		daily map[string]int
		want  string
	}{
		{"天天聊（0% 零天）", mkDaily("2026-01-01", 248, 10), "high"},
		{"三天一次（67% 零天）", mkDaily("2026-01-01", 248, 0, 0, 5), "medium"},
		{"二十天一次（95% 零天）", mkDaily("2026-01-01", 248,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3), "low"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Forecast(in(c.daily, "2026-09-05")).Confidence; got != c.want {
				t.Errorf("置信度 = %s，期望 %s", got, c.want)
			}
		})
	}
}

func mustDate(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(fmt.Sprint(err))
	}
	return d
}
