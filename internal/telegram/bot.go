// Package telegram 实现一个 Telegram Bot 长轮询 worker，
// 用户在 Telegram 发命令即可远程查询数据。
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store"
	"github.com/rs/zerolog/log"
)

const tgAPIBase = "https://api.telegram.org/bot"

// Bot 长轮询 Telegram getUpdates，将命令分发给 handlers 处理后回复
type Bot struct {
	token          string
	store          store.Store
	allowedChatIDs map[string]bool

	mu          sync.Mutex
	lastUpdate  int64
	cancel      context.CancelFunc
	httpClient  *http.Client
	registry    *commandRegistry
}

// Config Bot 启动配置
type Config struct {
	BotToken          string
	AuthorizedChatIDs []string
	Store             store.Store
}

// New 创建一个未启动的 Bot
func New(cfg Config) *Bot {
	allowed := make(map[string]bool, len(cfg.AuthorizedChatIDs))
	for _, id := range cfg.AuthorizedChatIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			allowed[id] = true
		}
	}
	b := &Bot{
		token:          cfg.BotToken,
		store:          cfg.Store,
		allowedChatIDs: allowed,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
	}
	b.registry = newRegistry(b)
	return b
}

// Start 启动长轮询；ctx 取消会停止
func (b *Bot) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	b.cancel = cancel
	go b.loop(ctx)
	log.Info().Int("authorized", len(b.allowedChatIDs)).Msg("Telegram bot 已启动")
}

// Stop 停止
func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *Bot) loop(ctx context.Context) {
	backoff := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Telegram bot 已停止")
			return
		default:
		}
		err := b.pollOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn().Err(err).Msg("Telegram getUpdates 失败")
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		} else {
			backoff = 5 * time.Second
		}
	}
}

// Telegram API 返回结构（仅取需要字段）
type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type tgUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func (b *Bot) pollOnce(ctx context.Context) error {
	b.mu.Lock()
	offset := b.lastUpdate + 1
	b.mu.Unlock()

	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=30&allowed_updates=%%5B%%22message%%22%%5D",
		tgAPIBase, b.token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var parsed tgUpdatesResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if !parsed.OK {
		return fmt.Errorf("telegram API not ok: %s", string(body))
	}

	for _, u := range parsed.Result {
		b.mu.Lock()
		if u.UpdateID > b.lastUpdate {
			b.lastUpdate = u.UpdateID
		}
		b.mu.Unlock()

		if u.Message == nil || u.Message.Text == "" {
			continue
		}
		chatID := fmt.Sprintf("%d", u.Message.Chat.ID)
		// 白名单
		if len(b.allowedChatIDs) > 0 && !b.allowedChatIDs[chatID] {
			log.Debug().Str("chat_id", chatID).Msg("Telegram bot 忽略非白名单消息")
			continue
		}
		go b.handleMessage(ctx, chatID, u.Message.Text)
	}
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, chatID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 命令必须以 / 开头
	if !strings.HasPrefix(text, "/") {
		b.reply(chatID, "👋 发送 /help 查看可用命令")
		return
	}
	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])
	// 兼容 BotFather 风格的 /cmd@BotName
	if at := strings.Index(cmd, "@"); at >= 0 {
		cmd = cmd[:at]
	}
	args := parts[1:]

	handler, ok := b.registry.get(cmd)
	if !ok {
		b.reply(chatID, fmt.Sprintf("❓ 未知命令 /%s\n发送 /help 查看可用命令", cmd))
		return
	}
	out, err := handler(ctx, args)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}
	b.reply(chatID, out)
}

type tgSendReq struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func (b *Bot) reply(chatID, text string) {
	// Telegram 单条消息 4096 字符上限
	if len(text) > 4000 {
		text = text[:4000] + "\n...(已截断)"
	}
	payload, _ := json.Marshal(tgSendReq{ChatID: chatID, Text: text})
	url := fmt.Sprintf("%s%s/sendMessage", tgAPIBase, b.token)
	resp, err := b.httpClient.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Warn().Err(err).Msg("Telegram reply 失败")
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}
