package api

import (
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
)

// GetTalkerExtras GET /api/v1/analysis/extras/:id?year=&tz_offset=
// 单个联系人的日历热力图 + 语音统计 + 互动与回复指标。
// year 传 0 表示统计全部历史。
func (a *API) GetTalkerExtras(c *gin.Context) {
	talker := c.Param("id")
	if talker == "" {
		transport.BadRequest(c, "缺少会话 ID")
		return
	}
	year := 0
	if v := c.Query("year"); v != "" {
		year = queryYear(c, "year")
	}
	tz := resolveTzMinutes(c) * 60
	transport.SendSuccess(c, a.Store.GetTalkerExtras(c.Request.Context(), talker, year, tz))
}
