package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// TZSegmentRequest 单个时区段（前端传入）
type TZSegmentRequest struct {
	StartDate string `json:"start_date"` // "2024-02-01"
	EndDate   string `json:"end_date"`   // "2024-02-19"
	TZOffset  int    `json:"tz_offset"`  // 分钟，东正西负（UTC+8 → 480）
}

// AnnualReportRequest 年度报告请求体
type AnnualReportRequest struct {
	Year           int                `json:"year"`
	TZSegments     []TZSegmentRequest `json:"tz_segments"`
	DefaultTZ      *int               `json:"default_tz_offset"`
	ExcludeTalkers []string           `json:"exclude_talkers"`
	Talker         string             `json:"talker"` // 非空时返回该联系人的年度报告
}

// GetAnnualReport 获取年度报告（支持 GET 和 POST，POST 时可传多时区段）
func (a *API) GetAnnualReport(c *gin.Context) {
	var req AnnualReportRequest

	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			transport.BadRequest(c, "请求体格式错误: "+err.Error())
			return
		}
	} else {
		// GET 兼容：?year=2024&tz_offset=480
		req.Year = time.Now().Year()
		if ys := c.Query("year"); ys != "" {
			if y, err := strconv.Atoi(ys); err == nil && y >= 2000 && y <= 2100 {
				req.Year = y
			}
		}
		if tzs := c.Query("tz_offset"); tzs != "" {
			if off, err := strconv.Atoi(tzs); err == nil {
				req.DefaultTZ = &off
			}
		}
		req.Talker = c.Query("talker")
	}

	if req.Year < 2000 || req.Year > 2100 {
		req.Year = time.Now().Year()
	}

	// 默认时区偏移（秒）
	defaultTzOffset := 0
	if req.DefaultTZ != nil {
		defaultTzOffset = *req.DefaultTZ * 60
	}

	// 指定了 talker：返回该联系人的年度报告（在内存里聚合该 talker 的消息）
	if req.Talker != "" {
		version := a.reportDataVersion()
		key := TalkerReportKey(req.Year, req.Talker, defaultTzOffset)
		if cached := a.ReportCache.Get(version, key); cached != nil {
			transport.SendSuccess(c, cached)
			return
		}
		report, err := a.Store.GetTalkerAnnualReport(c.Request.Context(), req.Year, req.Talker, defaultTzOffset)
		if err != nil {
			log.Error().Err(err).Int("year", req.Year).Str("talker", req.Talker).Msg("获取联系人年度报告失败")
			transport.InternalServerError(c, "获取联系人年度报告失败")
			return
		}
		a.ReportCache.Put(version, key, report)
		transport.SendSuccess(c, report)
		return
	}

	// 将前端 segments 转换为 store 层的类型（偏移分钟 → 秒）
	var storeSegs []types.TZSegment
	for _, s := range req.TZSegments {
		start, err1 := time.Parse("2006-01-02", s.StartDate)
		end, err2 := time.Parse("2006-01-02", s.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		storeSegs = append(storeSegs, types.TZSegment{
			Start:    start.UTC(),
			End:      end.Add(24*time.Hour - time.Nanosecond).UTC(),
			TZOffset: s.TZOffset * 60,
		})
	}

	pastStartYear := effectiveChatStartYear()
	excludeMerged := a.mergedExcludeTalkers(req.ExcludeTalkers)

	// 数据指纹 + 参数键 命中缓存就直接返回，省下整份重算
	version := a.reportDataVersion()
	cacheKey := AnnualReportKey(req.Year, defaultTzOffset, pastStartYear, excludeMerged, req.TZSegments)
	if cached := a.ReportCache.Get(version, cacheKey); cached != nil {
		transport.SendSuccess(c, cached)
		return
	}

	report, err := a.Store.GetAnnualReport(c.Request.Context(), req.Year, defaultTzOffset, pastStartYear, storeSegs, excludeMerged)
	if err != nil {
		log.Error().Err(err).Int("year", req.Year).Msg("获取年度报告失败")
		transport.InternalServerError(c, "获取年度报告失败")
		return
	}

	a.ReportCache.Put(version, cacheKey, report)
	transport.SendSuccess(c, report)
}

// excludeFromQuery 读 ?exclude=wxid_a,wxid_b 并与服务端全局排除名单合并。
//
// 参考线走的是 GET（前端要按参数缓存），排除名单只能挂在 query 上。
// 名单是 15 个 wxid 量级、300 字符出头，远在 URL 长度红线之下。
func (a *API) excludeFromQuery(c *gin.Context) []string {
	raw := c.Query("exclude")
	var list []string
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			list = append(list, t)
		}
	}
	return a.mergedExcludeTalkers(list)
}

// GetPastYearsMonthlyAvg 计算指定年份范围内每年月度趋势的平均。
// 默认 from = 「有效聊天记录起始时间」全局设置；不传 to 则取当前年-1
// GET /api/v1/report/past_monthly_avg?from=2023&to=2025&tz_offset=480&exclude=wxid_a,wxid_b
func (a *API) GetPastYearsMonthlyAvg(c *gin.Context) {
	from, _ := strconv.Atoi(c.Query("from"))
	to, _ := strconv.Atoi(c.Query("to"))
	if from < 2000 || from > 2100 {
		from = effectiveChatStartYear()
	}
	if to < 2000 || to > 2100 {
		to = time.Now().Year() - 1
	}
	tzOffsetMin, _ := strconv.Atoi(c.Query("tz_offset"))
	tzOffsetSec := tzOffsetMin * 60

	avg := a.Store.ComputeMonthlyAvgInRange(c.Request.Context(), from, to, tzOffsetSec, a.excludeFromQuery(c))
	transport.SendSuccess(c, avg)
}

// GetReportBaseline 一次返回当前生效的「往年月均」+「往年同期 overview 平均」
// 前端在「有效聊天记录起始时间」改变后，调用此端点局部刷新参考线和百分比，无需重建整份报告
// GET /api/v1/report/baseline?year=2026&tz_offset=480&exclude=wxid_a,wxid_b
func (a *API) GetReportBaseline(c *gin.Context) {
	year, err := strconv.Atoi(c.Query("year"))
	if err != nil || year < 2000 || year > 2100 {
		transport.BadRequest(c, "year 必须是有效年份")
		return
	}
	tzOffsetMin, _ := strconv.Atoi(c.Query("tz_offset"))
	tzOffsetSec := tzOffsetMin * 60
	pastStartYear := effectiveChatStartYear()
	pastEndYear := time.Now().Year() // 「往年月均」参考线固定按 [起始年, 当前年] 计算，不随所看报告年份变化

	// 和主线同一份排除名单：两条线画在同一个 Y 轴上，口径必须一致
	exclude := a.excludeFromQuery(c)
	monthlyAvg := a.Store.ComputeMonthlyAvgInRange(c.Request.Context(), pastStartYear, pastEndYear, tzOffsetSec, exclude)
	overviewAvg := a.Store.ComputePastOverviewAvg(c.Request.Context(), year, pastStartYear, tzOffsetSec, exclude)

	transport.SendSuccess(c, gin.H{
		"past_start_year":        pastStartYear,
		"past_end_year":          pastEndYear,
		"past_years_monthly_avg": monthlyAvg,
		"past_overview_avg":      overviewAvg,
	})
}

// StreamAnnualReport 流式生成年度报告
// POST /api/v1/report/annual/stream — 与 /api/v1/report/annual 同样的 body
// 返回 NDJSON 流：每行一个 JSON 事件 {type, step, current, total, data}
func (a *API) StreamAnnualReport(c *gin.Context) {
	var req AnnualReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	if req.Year < 2000 || req.Year > 2100 {
		req.Year = time.Now().Year()
	}

	defaultTzOffset := 0
	if req.DefaultTZ != nil {
		defaultTzOffset = *req.DefaultTZ * 60
	}

	var storeSegs []types.TZSegment
	for _, s := range req.TZSegments {
		start, err1 := time.Parse("2006-01-02", s.StartDate)
		end, err2 := time.Parse("2006-01-02", s.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		storeSegs = append(storeSegs, types.TZSegment{
			Start:    start.UTC(),
			End:      end.Add(24*time.Hour - time.Nanosecond).UTC(),
			TZOffset: s.TZOffset * 60,
		})
	}
	pastStartYear := effectiveChatStartYear()

	// NDJSON 流式响应头
	c.Writer.Header().Set("Content-Type", "application/x-ndjson")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	c.Writer.WriteHeader(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)

	emit := func(evt map[string]interface{}) {
		b, err := json.Marshal(evt)
		if err != nil {
			return
		}
		_, _ = c.Writer.Write(b)
		_, _ = c.Writer.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	// start 事件
	emit(map[string]interface{}{
		"type":  "start",
		"year":  req.Year,
		"total": 8,
	})

	progressFn := func(step string, current, total int, data interface{}) {
		emit(map[string]interface{}{
			"type":    "section",
			"step":    step,
			"current": current,
			"total":   total,
			"data":    data,
		})
	}

	report, err := a.Store.GetAnnualReportWithProgress(
		c.Request.Context(),
		req.Year, defaultTzOffset, pastStartYear,
		storeSegs, a.mergedExcludeTalkers(req.ExcludeTalkers),
		progressFn,
	)
	if err != nil {
		emit(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	emit(map[string]interface{}{
		"type":   "done",
		"report": report,
	})
}

// GetAnnualWordCounts 字数统计（点击概览/排行可切换显示字数）
// POST /api/v1/report/word_count，body 同 /api/v1/report/annual
func (a *API) GetAnnualWordCounts(c *gin.Context) {
	var req AnnualReportRequest
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			transport.BadRequest(c, "请求体格式错误: "+err.Error())
			return
		}
	}
	if req.Year < 2000 || req.Year > 2100 {
		req.Year = time.Now().Year()
	}

	defaultTzOffset := 0
	if req.DefaultTZ != nil {
		defaultTzOffset = *req.DefaultTZ * 60
	}

	var storeSegs []types.TZSegment
	for _, s := range req.TZSegments {
		start, err1 := time.Parse("2006-01-02", s.StartDate)
		end, err2 := time.Parse("2006-01-02", s.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		storeSegs = append(storeSegs, types.TZSegment{
			Start:    start.UTC(),
			End:      end.Add(24*time.Hour - time.Nanosecond).UTC(),
			TZOffset: s.TZOffset * 60,
		})
	}

	stat, err := a.Store.GetAnnualWordCounts(c.Request.Context(), req.Year, defaultTzOffset, storeSegs, a.mergedExcludeTalkers(req.ExcludeTalkers))
	if err != nil {
		log.Error().Err(err).Msg("获取字数统计失败")
		transport.InternalServerError(c, "获取字数统计失败")
		return
	}
	transport.SendSuccess(c, stat)
}

// mergedExcludeTalkers 把请求传入的排除名单与服务端持久化的排除配置合并去重，
// 保证服务端配置的排除名单始终生效（无论调用方是否传 exclude_talkers）。
func (a *API) mergedExcludeTalkers(reqExclude []string) []string {
	set := make(map[string]bool)
	for _, t := range reqExclude {
		set[t] = true
	}
	if a.ExcludeConfig != nil {
		for _, t := range a.ExcludeConfig.Get() {
			set[t] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	return out
}

// GetExcludeTalkers 返回服务端持久化的排除联系人名单
func (a *API) GetExcludeTalkers(c *gin.Context) {
	talkers := []string{}
	if a.ExcludeConfig != nil {
		talkers = a.ExcludeConfig.Get()
	}
	transport.SendSuccess(c, gin.H{"talkers": talkers})
}

// UpdateExcludeTalkers 覆盖保存排除联系人名单
func (a *API) UpdateExcludeTalkers(c *gin.Context) {
	var req struct {
		Talkers []string `json:"talkers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误")
		return
	}
	if a.ExcludeConfig != nil {
		a.ExcludeConfig.Set(req.Talkers)
	}
	transport.SendSuccess(c, gin.H{"status": "ok"})
}
