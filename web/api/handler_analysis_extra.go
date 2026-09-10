package api

import (
	"strconv"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// resolveTzMinutes 取 tz_offset 查询参数（分钟，东正西负）；缺省用全局默认时区设置。
func resolveTzMinutes(c *gin.Context) int {
	if v := c.Query("tz_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	off, _ := defaultTzOffsetMinutes()
	return off
}

// queryYear 取某个年份查询参数，非法/缺省则用当前年。
func queryYear(c *gin.Context, key string) int {
	if n, err := strconv.Atoi(c.Query(key)); err == nil && n >= 2000 && n <= 2100 {
		return n
	}
	return time.Now().Year()
}

// GetCalendarHeatmap 功能9：GET /api/v1/analysis/calendar_heatmap?year=&tz_offset=
func (a *API) GetCalendarHeatmap(c *gin.Context) {
	year := queryYear(c, "year")
	tzSec := resolveTzMinutes(c) * 60
	// 传 nil 即只取服务端持久化的「忽略的联系人」名单
	transport.SendSuccess(c, a.Store.GetCalendarHeatmap(c.Request.Context(), year, tzSec, a.mergedExcludeTalkers(nil)))
}

// GetDailyReport 今日报告：
// GET /api/v1/analysis/daily_report?date=YYYY-MM-DD&tz_offset=&top=
// date 省略则取用户时区下的今天。
func (a *API) GetDailyReport(c *gin.Context) {
	tzSec := resolveTzMinutes(c) * 60
	loc := time.FixedZone("tz", tzSec)

	date := c.Query("date")
	if date == "" {
		date = time.Now().In(loc).Format("2006-01-02") // 「今天」按用户时区算
	} else if _, err := time.ParseInLocation("2006-01-02", date, loc); err != nil {
		transport.BadRequest(c, "date 需为 YYYY-MM-DD")
		return
	}

	top := 20
	if v, err := strconv.Atoi(c.Query("top")); err == nil && v > 0 && v <= 200 {
		top = v
	}
	transport.SendSuccess(c, a.Store.GetDailyReport(c.Request.Context(), date, tzSec, top, a.mergedExcludeTalkers(nil)))
}

// GetHeatmapPartners 日历热力图下钻：
// GET /api/v1/analysis/heatmap_partners?year=&month=&day=&tz_offset=&limit=
// day 可省略（或 <=0）表示整月；给了 day 就只看那一天。
// 返回这段时间和谁聊过、各聊了多少天。
func (a *API) GetHeatmapPartners(c *gin.Context) {
	year := queryYear(c, "year")
	month, err := strconv.Atoi(c.Query("month"))
	if err != nil || month < 1 || month > 12 {
		transport.BadRequest(c, "month 需为 1-12")
		return
	}
	day := 0
	if v := c.Query("day"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 31 {
			day = n
		} else {
			transport.BadRequest(c, "day 需为 1-31")
			return
		}
	}
	tzSec := resolveTzMinutes(c) * 60
	limit := 30 // 弹窗可滚动，给足条数；再多就没有阅读价值了
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	transport.SendSuccess(c, a.Store.GetHeatmapPartners(c.Request.Context(), year, month, day, tzSec, limit, a.mergedExcludeTalkers(nil)))
}

// GetInteractionRatios 功能3：GET /api/v1/analysis/interaction_ratios?year=&tz_offset=&gap=&limit=
func (a *API) GetInteractionRatios(c *gin.Context) {
	year := queryYear(c, "year")
	tzSec := resolveTzMinutes(c) * 60
	gap, _ := strconv.Atoi(c.Query("gap"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 100
	}
	data, err := a.Store.GetInteractionRatios(c.Request.Context(), year, tzSec, gap, limit)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	transport.SendSuccess(c, data)
}

// GetReplySpeedRanking 功能5：GET /api/v1/analysis/reply_speed?year=&tz_offset=&limit=
func (a *API) GetReplySpeedRanking(c *gin.Context) {
	year := queryYear(c, "year")
	tzSec := resolveTzMinutes(c) * 60
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 100
	}
	data, err := a.Store.GetReplySpeedRanking(c.Request.Context(), year, tzSec, limit)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	transport.SendSuccess(c, data)
}

// GetYearCompare 功能7：GET /api/v1/report/year_compare?year_a=&year_b=&tz_offset=
func (a *API) GetYearCompare(c *gin.Context) {
	tzSec := resolveTzMinutes(c) * 60
	ya := queryYear(c, "year_a")
	yb := queryYear(c, "year_b")
	data, err := a.Store.GetYearCompare(c.Request.Context(), ya, yb, tzSec, a.mergedExcludeTalkers(nil))
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	transport.SendSuccess(c, data)
}

// GetCommonGroups 功能4：GET /api/v1/analysis/common_groups?talker=wxid
func (a *API) GetCommonGroups(c *gin.Context) {
	wxid := c.Query("talker")
	if wxid == "" {
		transport.BadRequest(c, "缺少 talker 参数")
		return
	}
	data, err := a.Store.GetCommonGroups(c.Request.Context(), wxid)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	transport.SendSuccess(c, data)
}
