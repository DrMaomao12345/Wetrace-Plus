// Package forecast 从日级消息数推出「当月到年底」的月度预测。
//
// 刻意做得简单。实证回测（2026-09-05，100 个联系人、约 4,400 次当月预测 +
// 5,700 次未来月预测）里，复杂模型没能稳定赢过简单组合，而且「每个联系人单独
// 挑参数」比统一规则更差 —— 小样本下个体化调参就是过拟合。所以这里只有两样
// 东西：一个向长期均值衰减的速率，和一次自助重采样。
//
// 关键性质：
//
//   - 最近一直没聊 → 重采样池全是 0 → 预测 0、区间 [0,0]。
//     不需要为稀疏联系人开特例，「是 0 就是 0」是自然结果。
//   - 速率随预测距离衰减到长期均值，不会无限外推。
//   - 区间来自重采样而不是正态假设 —— 聊天记录严重过度离散
//     （实测方差÷均值 28~94），正态区间会窄得离谱。
package forecast

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

const (
	// 趋势衰减系数。取得很小是有依据的：回测里赢的是「最近 28 天速率」这种
	// 近乎持平的基线，带趋势的 Holt 没有稳定胜出。所以这里只允许趋势
	// 轻微延续 —— 累计最多 φ/(1-φ) ≈ 0.54 个月的斜率。
	dampingPhi = 0.35
	// 趋势对速率的影响上限（相对最近水平）。挡住「最近 28 天暴涨 50 倍」
	// 这种窗口把预测推向荒谬的情况。
	maxTrendShift = 0.5
	// 自助重采样池的长度上限
	poolDays = 90
	// 近期速率窗口
	recentDays = 28
	// 模拟路径条数。2000 条足够把 10%/90% 分位稳住，且单联系人 1ms 量级
	simPaths = 2000
)

// MonthPoint 是折线图上的一个预测点。
type MonthPoint struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	// Point 是**上下界的平均**，不是中位数 —— 让点始终落在色带正中，
	// 图形自洽。计数分布右偏，所以它会略高于中位数。
	Point int `json:"point"`
	Lo    int `json:"lo"` // 10% 分位
	Hi    int `json:"hi"` // 90% 分位
	// Horizon 距当前月几个月，0 表示当前月
	Horizon int `json:"horizon"`
	// Actual 仅当前月有意义：截至今天已经发生的条数
	Actual    int  `json:"actual"`
	HasActual bool `json:"has_actual"`
}

// Result 是一次预测的全部产出。
type Result struct {
	Points []MonthPoint `json:"points"`
	// ZeroDayRatio 最近 90 天里一条消息都没有的天数占比，用来给置信度分级
	ZeroDayRatio float64 `json:"zero_day_ratio"`
	// Confidence high / medium / low
	Confidence string `json:"confidence"`
	// BasisDays 实际拿来估速率的完整自然日天数
	BasisDays int `json:"basis_days"`
}

// Input 是模型需要的全部输入。日期一律是**服务端时区**下的自然日，
// 与 GetDailyActivity 的口径一致。
type Input struct {
	// Daily 只含有消息的天（GROUP BY 的直接产物），内部会补零
	Daily map[string]int
	// FirstDay 第一条消息所在的自然日；空则由 Daily 推出
	FirstDay string
	// Today 服务端时区下的今天
	Today time.Time
	// Year 要预测到哪一年的 12 月
	Year int
	// Seed 固定随机种子，保证同样输入产出同样结果（图表不该每次刷新都跳）
	Seed int64
}

// Forecast 产出从当前月到 Year 年 12 月的预测点。
//
// 当前月 = Today 所在月，且只在 Today.Year() == Year 时才有「当月」的概念；
// 预测 Year 是往年时返回空（往事不需要预测）。
func Forecast(in Input) Result {
	if in.Year != in.Today.Year() {
		return Result{Points: []MonthPoint{}, Confidence: "low"}
	}

	// —— 1. 补齐日序列 ——
	// GROUP BY 不返回没消息的天，但「那天没聊」正是模型要学的东西
	firstDay := in.FirstDay
	if firstDay == "" {
		for d := range in.Daily {
			if firstDay == "" || d < firstDay {
				firstDay = d
			}
		}
	}
	if firstDay == "" {
		return Result{Points: []MonthPoint{}, Confidence: "low"}
	}
	first, err := time.Parse("2006-01-02", firstDay)
	if err != nil {
		return Result{Points: []MonthPoint{}, Confidence: "low"}
	}

	today := time.Date(in.Today.Year(), in.Today.Month(), in.Today.Day(), 0, 0, 0, 0, time.UTC)
	// 今天还没过完，不能当作完整一天参与速率估计；估计只用到昨天为止
	lastComplete := today.AddDate(0, 0, -1)

	var series []int // 从 firstDay 到昨天，逐日补零
	for d := first; !d.After(lastComplete); d = d.AddDate(0, 0, 1) {
		series = append(series, in.Daily[d.Format("2006-01-02")])
	}

	// —— 2. 速率与重采样池 ——
	pool := tail(series, poolDays)
	// 水平取最近 28 天；斜率取「最近 28 天 vs 再往前 28 天」的差。
	//
	// 注意这里是**衰减趋势**，不是回归长期均值。两者差别很大：
	// 某个联系人 7 月爆发过一次，90 天均值会被那一下抬高，回归均值会让预测
	// 反而一路往上爬 —— 而他实际上正在冷却。衰减趋势的做法是「保持最近水平，
	// 让升/降的势头逐月衰减到零」，不会把旧的爆发拽回来。
	rRecent := mean(tail(series, recentDays))
	rPrev := rRecent
	if len(series) >= recentDays*2 {
		rPrev = mean(series[len(series)-recentDays*2 : len(series)-recentDays])
	}
	slope := rRecent - rPrev // 每月（≈28 天）的日均变化
	rLong := mean(pool)      // 只用来把重采样池缩放到目标速率

	zeroRatio := 1.0
	if len(pool) > 0 {
		zeros := 0
		for _, v := range pool {
			if v == 0 {
				zeros++
			}
		}
		zeroRatio = float64(zeros) / float64(len(pool))
	}

	res := Result{
		ZeroDayRatio: zeroRatio,
		BasisDays:    len(pool),
		Confidence:   confidenceOf(zeroRatio, len(pool)),
		Points:       []MonthPoint{},
	}

	// 池子全是 0 —— 最近彻底没聊。直接给 0，不必模拟。
	// 这就是稀疏联系人不需要特例的原因：0 是模型的自然输出。
	poolSum := 0
	for _, v := range pool {
		poolSum += v
	}

	rng := rand.New(rand.NewSource(in.Seed))

	for m := int(today.Month()); m <= 12; m++ {
		horizon := m - int(today.Month())
		daysInMonth := daysIn(in.Year, m)

		var actual, predictDays int
		if horizon == 0 {
			// 当月：已经发生的算实际（含今天，尽管今天还没过完），预测今天之后
			for d := 1; d <= today.Day(); d++ {
				actual += in.Daily[time.Date(in.Year, time.Month(m), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")]
			}
			predictDays = daysInMonth - today.Day()
		} else {
			predictDays = daysInMonth
		}

		pt := MonthPoint{
			Year: in.Year, Month: m, Horizon: horizon,
			Actual: actual, HasActual: horizon == 0,
		}

		if poolSum == 0 || predictDays <= 0 {
			pt.Point, pt.Lo, pt.Hi = actual, actual, actual
			res.Points = append(res.Points, pt)
			continue
		}

		// 衰减趋势：势头逐月按 φ 衰减，累计不超过 φ/(1-φ)（φ=0.6 时是 1.5 个月）。
		// 所以再陡的升降也不会无限外推，最终会走平。
		damp := 0.0
		for i := 1; i <= horizon; i++ {
			damp += math.Pow(dampingPhi, float64(i))
		}
		shift := slope * damp
		// 双向都夹住：暴涨不外推成天文数字，暴跌也不会一步砍到 0
		if lim := rRecent * maxTrendShift; shift > lim {
			shift = lim
		} else if shift < -lim {
			shift = -lim
		}
		rate := rRecent + shift
		if rate < 0 {
			rate = 0 // 聊天条数不可能是负的
		}
		scale := 1.0
		if rLong > 0 {
			scale = rate / rLong
		}

		sums := make([]float64, simPaths)
		for i := 0; i < simPaths; i++ {
			s := 0.0
			for d := 0; d < predictDays; d++ {
				s += float64(pool[rng.Intn(len(pool))]) * scale
			}
			sums[i] = s + float64(actual)
		}
		sort.Float64s(sums)

		lo := quantile(sums, 0.10)
		hi := quantile(sums, 0.90)
		pt.Lo = roundNonNeg(lo)
		pt.Hi = roundNonNeg(hi)
		// 用户要的口径：点 = 上下界的平均
		pt.Point = roundNonNeg((lo + hi) / 2)
		if pt.Point < pt.Lo {
			pt.Point = pt.Lo
		}
		if pt.Point > pt.Hi {
			pt.Point = pt.Hi
		}
		res.Points = append(res.Points, pt)
	}

	return res
}

// confidenceOf 按最近 90 天的零消息天比例和可用天数分级。
// 阈值来自回测：零消息天超过 ~90% 的那批，未来 1 个月 WAPE 就已经 >100%。
func confidenceOf(zeroRatio float64, basisDays int) string {
	switch {
	case basisDays < 14:
		return "low"
	case zeroRatio > 0.90:
		return "low"
	case zeroRatio > 0.50:
		return "medium"
	default:
		return "high"
	}
}

func tail(s []int, n int) []int {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func mean(s []int) float64 {
	if len(s) == 0 {
		return 0
	}
	t := 0
	for _, v := range s {
		t += v
	}
	return float64(t) / float64(len(s))
}

// quantile 对已排序切片取分位（线性插值）。
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func roundNonNeg(v float64) int {
	r := int(math.Round(v))
	if r < 0 {
		return 0
	}
	return r
}

func daysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
