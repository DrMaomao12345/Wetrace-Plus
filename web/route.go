package web

import (
	"io"
	"net/http"
	"strings"

	"github.com/afumu/wetrace/web/middleware"
	"github.com/gin-gonic/gin"
)

// setupRoutes 初始化所有应用程序路由。
func (s *Service) setupRoutes() {
	// 密码保护中间件
	s.router.Use(middleware.AuthMiddleware(s.api))

	// API v1 路由组, 使用在 service 中初始化的处理器
	v1 := s.router.Group("/api/v1")
	{
		// 系统路由
		system := v1.Group("/system")
		{
			system.GET("/status", s.api.GetSystemStatus)
			system.POST("/decrypt", s.api.HandleEnvDecrypt)
			system.GET("/wxkey/db", s.api.GetWeChatDbKey)
			system.GET("/wxkey/image", s.api.GetWeChatImageKey)
			system.GET("/detect/wechat_path", s.api.DetectWeChatInstallPath)
			system.GET("/detect/db_path", s.api.DetectWeChatDataPath)
			system.POST("/select_path", s.api.SelectPath)
			system.POST("/config", s.api.UpdateConfig)

			// AI 配置路由 (需求3)
			system.GET("/ai_config", s.api.GetAIConfig)
			system.POST("/ai_config", s.api.UpdateAIConfig)
			system.POST("/ai_config/test", s.api.TestAIConfig)

			// AI 提示词配置路由
			system.GET("/ai_prompts", s.api.GetAIPrompts)
			system.POST("/ai_prompts", s.api.UpdateAIPrompts)

			// 密码保护路由 (需求6)
			system.GET("/password/status", s.api.GetPasswordStatus)
			system.POST("/password/set", s.api.SetPassword)
			system.POST("/password/verify", s.api.VerifyPassword)
			system.POST("/password/disable", s.api.DisablePassword)

			// 合规提示路由 (需求9)
			system.GET("/compliance", s.api.GetCompliance)
			system.POST("/compliance/agree", s.api.AgreeCompliance)

			// 自动同步路由 (需求5)
			system.GET("/sync_config", s.api.GetSyncConfig)
			system.POST("/sync_config", s.api.UpdateSyncConfig)
			system.POST("/sync", s.api.TriggerSync)
			system.GET("/sync_status", s.api.GetSyncStatus)

			// 自动备份路由 (需求8)
			system.GET("/backup_config", s.api.GetBackupConfig)
			system.POST("/backup_config", s.api.UpdateBackupConfig)
			system.POST("/backup/run", s.api.RunBackup)
			system.GET("/backup/history", s.api.GetBackupHistory)

			// TTS 语音转文字配置路由
			system.GET("/tts_config", s.api.GetTTSConfig)
			system.POST("/tts_config", s.api.UpdateTTSConfig)

			// 数据目录
			system.GET("/data_dir", s.api.GetDataDir)
			system.POST("/data_dir/open", s.api.OpenDataDir)

			// 数据版本指纹（前端用来判断本地缓存是否仍然有效）
			system.GET("/data_version", s.api.GetDataVersion)

			// 移动端配对（iOS App）—— 多设备记录 + 历史
			mobile := system.Group("/mobile")
			{
				mobile.GET("/pairings", s.api.ListMobilePairings)
				mobile.POST("/pairings", s.api.CreateMobilePairing)
				mobile.DELETE("/pairings/:id", s.api.DeleteMobilePairing)
				mobile.PUT("/pairings/:id", s.api.RenameMobilePairing)
				mobile.GET("/ping", s.api.MobilePing)
			}

			// 有效聊天记录起始时间（年）
			system.GET("/effective_chat_start", s.api.GetEffectiveChatStart)
			system.POST("/effective_chat_start", s.api.UpdateEffectiveChatStart)

			// 默认时区（影响联系人侧分析）
			system.GET("/default_timezone", s.api.GetDefaultTimezone)
			system.POST("/default_timezone", s.api.UpdateDefaultTimezone)
		}

		// 会话路由
		v1.GET("/sessions", s.api.GetSessions)
		v1.DELETE("/sessions/:id", s.api.DeleteSession)

		// 总览路由
		v1.GET("/dashboard", s.api.GetDashboard)

		// 消息路由
		v1.GET("/messages", s.api.GetMessages)

		// 联系人路由
		v1.GET("/contacts", s.api.GetContacts)
		v1.GET("/contacts/need-contact", s.api.GetNeedContactList)
		v1.GET("/contacts/:id", s.api.GetContactByID)
		v1.GET("/contacts/export", s.api.ExportContacts)

		// 群聊路由
		v1.GET("/chatrooms", s.api.GetChatRooms)
		v1.GET("/chatrooms/:id", s.api.GetChatRoomByID)

		// 媒体路由
		v1.GET("/media/images", s.api.GetImageList)
		v1.GET("/media/:type/:key", s.api.GetMedia)
		v1.GET("/media/emoji", s.api.GetEmoji)
		v1.POST("/media/cache/start", s.api.HandleStartCache)
		v1.GET("/media/cache/status", s.api.GetCacheStatus)
		v1.POST("/media/voice/transcribe", s.api.TranscribeVoice)
		v1.GET("/media/voice/transcript", s.api.GetVoiceTranscript)
		v1.POST("/media/voice/transcribe-session", s.api.TranscribeSession)
		v1.GET("/media/voice/transcribe-session/status", s.api.GetTranscribeSessionStatus)

		// 导出路由
		v1.GET("/export/chat", s.api.ExportChat)
		v1.GET("/export/forensic", s.api.ExportForensic)
		v1.GET("/export/voices", s.api.ExportVoices)
		v1.POST("/export/voices", s.api.ExportVoices)

		// 搜索路由
		searchGroup := v1.Group("/search")
		{
			searchGroup.GET("", s.api.Search)
			searchGroup.GET("/context", s.api.SearchContext)
		}

		// 年度报告路由
		reportGroup := v1.Group("/report")
		{
			reportGroup.GET("/annual", s.api.GetAnnualReport)
			reportGroup.POST("/annual", s.api.GetAnnualReport)
			reportGroup.POST("/annual/stream", s.api.StreamAnnualReport)
			reportGroup.POST("/word_count", s.api.GetAnnualWordCounts)
			reportGroup.GET("/past_monthly_avg", s.api.GetPastYearsMonthlyAvg)
			reportGroup.GET("/baseline", s.api.GetReportBaseline)
		}

		// AI 路由
		aiGroup := v1.Group("/ai")
		{
			aiGroup.POST("/test", s.api.AITestConnection)
			aiGroup.POST("/summarize", s.api.AISummarize)
			aiGroup.POST("/summarize/cancel", s.api.CancelAISummarize)
			aiGroup.GET("/summary_history", s.api.GetSummaryHistory)
			aiGroup.DELETE("/summary_history/:id", s.api.DeleteSummaryHistory)
			aiGroup.POST("/simulate", s.api.AISimulate)
			aiGroup.POST("/sentiment", s.api.AISentiment)
			aiGroup.POST("/summary", s.api.AISummary)
			aiGroup.POST("/todos", s.api.AIExtractTodos)
			aiGroup.POST("/extract", s.api.AIExtractInfo)
			aiGroup.POST("/voice2text", s.api.AIVoice2Text)
		}

		// 分析路由
		analysisGroup := v1.Group("/analysis")
		{
			analysisGroup.GET("/personal/top_contacts", s.api.GetPersonalTopContacts)
			analysisGroup.GET("/hourly/:id", s.api.GetHourlyActivity)
			analysisGroup.GET("/daily/:id", s.api.GetDailyActivity)
			analysisGroup.GET("/weekday/:id", s.api.GetWeekdayActivity)
			analysisGroup.GET("/monthly/:id", s.api.GetMonthlyActivity)
			analysisGroup.GET("/type_distribution/:id", s.api.GetMessageTypeDistribution)
			analysisGroup.GET("/calls/:id", s.api.GetCallStats)
			analysisGroup.GET("/yearly_monthly/:id", s.api.GetYearlyMonthlyActivity)
			analysisGroup.GET("/top_contacts_monthly_avg", s.api.GetTopContactsHistoricalMonthlyAvg)
			analysisGroup.GET("/member_activity/:id", s.api.GetMemberActivity)
			analysisGroup.GET("/repeat/:id", s.api.GetRepeatAnalysis)
			analysisGroup.GET("/wordcloud/global", s.api.GetWordCloudGlobal)
			analysisGroup.GET("/wordcloud/:id", s.api.GetWordCloud)
			analysisGroup.POST("/wordcloud/reload_stopwords", s.api.ReloadWordCloudStopwords)
			analysisGroup.GET("/wordcloud/dict", s.api.GetWordCloudDict)
			analysisGroup.POST("/wordcloud/dict", s.api.UpdateWordCloudDict)
		}

		// 回放路由 (需求16)
		v1.GET("/messages/replay", s.api.GetReplayMessages)

		// 监控配置路由 (需求14+15)
		monitorGroup := v1.Group("/monitor")
		{
			monitorGroup.GET("/configs", s.api.GetMonitorConfigs)
			monitorGroup.POST("/configs", s.api.CreateMonitorConfig)
			monitorGroup.PUT("/configs/:id", s.api.UpdateMonitorConfig)
			monitorGroup.DELETE("/configs/:id", s.api.DeleteMonitorConfig)
			monitorGroup.POST("/test", s.api.TestMonitorPush)
		}

		// 飞书配置路由 (需求15)
		feishuGroup := v1.Group("/feishu")
		{
			feishuGroup.GET("/config", s.api.GetFeishuConfig)
			feishuGroup.PUT("/config", s.api.UpdateFeishuConfig)
			feishuGroup.POST("/test", s.api.TestFeishuBot)
			feishuGroup.POST("/test_bitable", s.api.TestFeishuBitable)
		}

		// Telegram 配置路由
		telegramGroup := v1.Group("/telegram")
		{
			telegramGroup.GET("/config", s.api.GetTelegramConfig)
			telegramGroup.PUT("/config", s.api.UpdateTelegramConfig)
			telegramGroup.POST("/test", s.api.TestTelegramBotGlobal)
		}
	}

	// 健康检查
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 静态文件服务 (UI)
	if s.staticFS != nil {
		s.router.StaticFS("/assets", http.FS(s.staticFS))
		// 处理 SPA 的 fallback，除了 /api 开头的路径外，都返回 index.html
		s.router.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "API route not found"})
				return
			}

			// Try to serve file directly first (e.g. favicon.ico)
			file, err := s.staticFS.Open(strings.TrimPrefix(c.Request.URL.Path, "/"))
			if err == nil {
				defer file.Close()
				stat, err := file.Stat()
				if err == nil && !stat.IsDir() {
					http.FileServer(http.FS(s.staticFS)).ServeHTTP(c.Writer, c.Request)
					return
				}
			}

			// Serve index.html
			f, err := s.staticFS.Open("index.html")
			if err != nil {
				c.String(http.StatusNotFound, "UI not found")
				return
			}
			defer f.Close()
			c.Status(http.StatusOK)
			c.Header("Content-Type", "text/html")
			_, _ = io.Copy(c.Writer, f)
		})
	}
}
