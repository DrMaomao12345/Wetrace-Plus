package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// GetHourlyActivity 获取每小时活跃度
func (a *API) GetHourlyActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetHourlyActivity(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取时段活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetDailyActivity 获取每日活跃度
func (a *API) GetDailyActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetDailyActivity(c.Request.Context(), sessionID)
	// year 可选：年度报告里的每日趋势应该只看当年，不传则返回全部历史
	if y := c.Query("year"); y != "" && err == nil {
		prefix := y + "-"
		filtered := stats[:0]
		for _, d := range stats {
			if strings.HasPrefix(d.Date, prefix) {
				filtered = append(filtered, d)
			}
		}
		stats = filtered
	}
	if err != nil {
		log.Error().Err(err).Msg("获取每日活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetWeekdayActivity 获取星期活跃度
func (a *API) GetWeekdayActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetWeekdayActivity(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取星期活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetMonthlyActivity 获取月份活跃度
func (a *API) GetMonthlyActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetMonthlyActivity(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取月份活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetMessageTypeDistribution 获取消息类型分布
func (a *API) GetMessageTypeDistribution(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetMessageTypeDistribution(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取消息类型分布失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetMemberActivity 获取成员活跃度
func (a *API) GetMemberActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetMemberActivity(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取成员活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetRepeatAnalysis 获取复读分析
func (a *API) GetRepeatAnalysis(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session id"})
		return
	}

	stats, err := a.Store.GetRepeatAnalysis(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取复读分析失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetPersonalTopContacts 获取个人社交排行榜
func (a *API) GetPersonalTopContacts(c *gin.Context) {
	stats, err := a.Store.GetPersonalTopContacts(c.Request.Context(), 100)
	if err != nil {
		log.Error().Err(err).Msg("获取个人社交排行榜失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetCallStats 获取通话统计
func (a *API) GetCallStats(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var cStart, cEnd time.Time
	if y := c.Query("year"); y != "" {
		if n, e := strconv.Atoi(y); e == nil && n >= 2000 && n <= 2100 {
			loc := time.FixedZone("rep", resolveTzMinutes(c)*60)
			cStart = time.Date(n, 1, 1, 0, 0, 0, 0, loc)
			cEnd = time.Date(n, 12, 31, 23, 59, 59, 0, loc)
		}
	}
	stats, err := a.Store.GetCallStats(c.Request.Context(), sessionID, cStart, cEnd)
	if err != nil {
		log.Error().Err(err).Msg("获取通话统计失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// GetYearlyMonthlyActivity 获取按年-月分组的活跃度
func (a *API) GetYearlyMonthlyActivity(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	stats, err := a.Store.GetYearlyMonthlyActivity(c.Request.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("获取年-月活跃度失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// GetTopContactsHistoricalMonthlyAvg 获取 Top N 亲密联系人过去年份的月均
func (a *API) GetTopContactsHistoricalMonthlyAvg(c *gin.Context) {
	limit := 10
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	stats, err := a.Store.GetTopContactsHistoricalMonthlyAvg(c.Request.Context(), limit)
	if err != nil {
		log.Error().Err(err).Msg("获取 Top 联系人历史月均失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
