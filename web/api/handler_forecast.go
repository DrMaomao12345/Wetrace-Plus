package api

import (
	"strconv"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/forecast"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// GetChatForecast 返回某个会话「当月到今年 12 月」的月度预测。
//
// GET /api/v1/analysis/forecast/:id?year=2026&tz_offset=480
//
// 日期一律按**服务端时区**切天，和 GetDailyActivity / 月度趋势图同口径 ——
// 用浏览器时区算「已过几天」会在月初月末错开一天。
func (a *API) GetChatForecast(c *gin.Context) {
	talker := c.Param("id")
	if talker == "" {
		transport.BadRequest(c, "会话 ID 是必需的")
		return
	}

	tzOffsetMin, _ := strconv.Atoi(c.Query("tz_offset"))
	loc := time.FixedZone("client", tzOffsetMin*60)
	today := time.Now().In(loc)

	year, err := strconv.Atoi(c.Query("year"))
	if err != nil || year < 2000 || year > 2100 {
		year = today.Year()
	}

	daily, err := a.Store.GetDailyActivity(c.Request.Context(), talker)
	if err != nil {
		transport.InternalServerError(c, "读取日活跃度失败: "+err.Error())
		return
	}

	m := make(map[string]int, len(daily))
	firstDay := ""
	for _, d := range daily {
		if d == nil {
			continue
		}
		m[d.Date] += d.Count
		if firstDay == "" || d.Date < firstDay {
			firstDay = d.Date
		}
	}

	res := forecast.Forecast(forecast.Input{
		Daily:    m,
		FirstDay: firstDay,
		// 只取年月日：模型内部按自然日推进，带上时分秒会让边界日多算/少算一天
		Today: time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC),
		Year:  year,
		// 固定种子：同一个会话每次刷新必须得到同一条线
		Seed: seedOf(talker, year),
	})

	transport.SendSuccess(c, res)
}

// seedOf 由会话和年份派生一个稳定种子。
func seedOf(talker string, year int) int64 {
	var h int64 = 1469598103934665603 // FNV-1a offset basis
	for _, b := range []byte(talker) {
		h ^= int64(b)
		h *= 1099511628211
	}
	return h ^ int64(year)
}
