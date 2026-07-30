package api

import (
	"strconv"
	"time"

	"github.com/afumu/wetrace/web/transport"
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
	transport.SendSuccess(c, a.Store.GetCalendarHeatmap(c.Request.Context(), year, tzSec))
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
	data, err := a.Store.GetYearCompare(c.Request.Context(), ya, yb, tzSec)
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
