package api

import (
	"strconv"

	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
)

// GetBizProfile 返回公众号订阅画像。
// year=0 表示统计全部历史；with_titles=0 可跳过标题词频（省掉解压开销）。
func (a *API) GetBizProfile(c *gin.Context) {
	year := 0
	if n, err := strconv.Atoi(c.Query("year")); err == nil && n >= 2000 && n <= 2100 {
		year = n
	}
	tz := resolveTzMinutes(c) * 60
	withTitles := c.DefaultQuery("with_titles", "1") != "0"

	profile := a.Store.GetBizProfile(c.Request.Context(), year, tz, withTitles)
	transport.SendSuccess(c, profile)
}
