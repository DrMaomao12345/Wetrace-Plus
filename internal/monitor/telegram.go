package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const telegramAPIBase = "https://api.telegram.org/bot"

type telegramSendMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func sendTelegram(botToken, chatID, text string) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("bot_token 和 chat_id 不能为空")
	}

	payload := telegramSendMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s%s/sendMessage", telegramAPIBase, botToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var tgResp telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&tgResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if !tgResp.OK {
		return fmt.Errorf("Telegram API 错误: %s", tgResp.Description)
	}
	return nil
}

// SendTelegramAlert 发送监控告警到 Telegram
func SendTelegramAlert(botToken, chatID string, msg WebhookMessage, ruleName string) error {
	chatType := "私聊"
	if msg.IsChatroom {
		chatType = "群聊"
	}

	text := fmt.Sprintf(
		"🔔 <b>WeTrace 监控告警</b>\n\n"+
			"<b>规则：</b>%s\n"+
			"<b>会话：</b>%s（%s）\n"+
			"<b>发送者：</b>%s\n"+
			"<b>时间：</b>%s\n\n"+
			"<b>消息内容：</b>\n%s",
		htmlEscape(ruleName),
		htmlEscape(msg.TalkerName),
		chatType,
		htmlEscape(msg.SenderName),
		htmlEscape(msg.Time),
		htmlEscape(msg.Content),
	)
	return sendTelegram(botToken, chatID, text)
}

// TestTelegramBot 测试 Telegram Bot 连通性
func TestTelegramBot(botToken, chatID string) error {
	text := "✅ <b>WeTrace Telegram Bot 连通性测试成功！</b>\n\nBot 已正确配置，可以接收监控告警。"
	return sendTelegram(botToken, chatID, text)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
