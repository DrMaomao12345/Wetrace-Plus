package api

import (
	"github.com/DrMaomao12345/Wetrace-Plus/internal/monitor"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// GetTelegramConfig 获取 Telegram 全局配置
func (a *API) GetTelegramConfig(c *gin.Context) {
	if a.Monitor == nil {
		transport.BadRequest(c, "监控功能未初始化")
		return
	}
	cfg := a.Monitor.GetTelegramConfig()
	transport.SendSuccess(c, cfg)
}

// UpdateTelegramConfig 更新 Telegram 全局配置
func (a *API) UpdateTelegramConfig(c *gin.Context) {
	if a.Monitor == nil {
		transport.BadRequest(c, "监控功能未初始化")
		return
	}

	var req monitor.TelegramConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if err := a.Monitor.UpdateTelegramConfig(req); err != nil {
		transport.InternalServerError(c, "更新 Telegram 配置失败: "+err.Error())
		return
	}
	// 配置变化后，重启 bot worker
	a.ApplyTelegramBotConfig()
	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// TestTelegramBotGlobal 测试全局 Telegram Bot 连通性
func (a *API) TestTelegramBotGlobal(c *gin.Context) {
	if a.Monitor == nil {
		transport.BadRequest(c, "监控功能未初始化")
		return
	}

	cfg := a.Monitor.GetTelegramConfig()
	if cfg.BotToken == "" || cfg.ChatID == "" {
		transport.BadRequest(c, "Telegram Bot Token 或 Chat ID 未配置")
		return
	}

	if err := monitor.TestTelegramBot(cfg.BotToken, cfg.ChatID); err != nil {
		transport.InternalServerError(c, "Telegram 测试失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"status": "ok", "message": "Telegram 测试消息已发送"})
}
