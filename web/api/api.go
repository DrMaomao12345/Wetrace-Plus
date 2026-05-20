package api

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/afumu/wetrace/decrypt"
	"github.com/afumu/wetrace/internal/ai"
	"github.com/afumu/wetrace/internal/backup"
	"github.com/afumu/wetrace/internal/monitor"
	"github.com/afumu/wetrace/internal/telegram"
	intsync "github.com/afumu/wetrace/internal/sync"
	"github.com/afumu/wetrace/internal/transcripts"
	"github.com/afumu/wetrace/internal/tts"
	"github.com/afumu/wetrace/pkg/wordcloud"
	"github.com/afumu/wetrace/store"
	"github.com/afumu/wetrace/store/types"
	"github.com/afumu/wetrace/web/export"
	"github.com/afumu/wetrace/web/media"
	"github.com/spf13/viper"
)

// BatchTranscribeJob tracks progress of a batch voice-to-text job.
type BatchTranscribeJob struct {
	Talker  string
	Total   int
	Done    int
	Errors  int
	Running bool
}

// API 封装了 API 处理器所需的所有依赖。
type API struct {
	Store           store.Store
	Media           *media.Service
	Export          *export.Service
	Conf            *Config
	AI              *ai.Client
	Password        *PasswordManager
	SyncScheduler   *intsync.Scheduler
	BackupScheduler *backup.Scheduler
	Monitor         *monitor.Store
	MonitorChecker  *monitor.Checker
	TgBot           *telegram.Bot
	tgBotMu         sync.Mutex
	TTS             tts.Transcriber
	Transcripts     *transcripts.Store
	MobilePairings  *MobilePairingStore
	ExcludeConfig   *ExcludeConfigStore
	ReportCache     *ReportCache
	mu                 sync.Mutex
	summarizeCancel    context.CancelFunc
	currentSummaryJob  *SummaryHistoryItem
	batchJob           *BatchTranscribeJob
}

type Config struct {
	DataDir         string
	WechatDbSrcPath string
	WechatDbKey     string
	WxKeyDllPath    string
	WechatPath      string
	WechatDataPath  string
	ImageKey        string
	XorKey          string
	AIEnabled       bool
	AIProvider      string
	AIAPIKey        string
	AIBaseURL       string
	AIModel         string
}

// NewAPI 创建一个新的 API 处理器。
func NewAPI(s store.Store, m *media.Service, conf *Config, staticFS fs.FS) *API {
	var aiClient *ai.Client
	if conf.AIEnabled {
		aiClient = ai.NewClient(conf.AIAPIKey, conf.AIBaseURL, conf.AIModel)
	}

	exportSvc := &export.Service{
		Media:    m,
		Store:    s,
		StaticFS: staticFS,
	}

	a := &API{
		Store:    s,
		Media:    m,
		Export:   exportSvc,
		Conf:     conf,
		AI:       aiClient,
		Password: NewPasswordManager(),
	}

	// Initialize AI prompts JSON file path
	initPromptsFilePath(conf.DataDir)
	initSummaryHistoryFilePath(conf.DataDir)

	// 初始化分词器（支持用户词典 data/wordcloud_dict.txt）
	wordcloud.Init(conf.DataDir)

	// 初始化「排除联系人」配置 store（网页与移动端共用）
	a.ExcludeConfig = NewExcludeConfigStore(conf.DataDir)

	// 年度报告缓存（按数据指纹 + 参数键缓存）
	a.ReportCache = NewReportCache()

	// 初始化移动端配对记录 store，并迁移旧的单 token
	a.MobilePairings = NewMobilePairingStore(conf.DataDir)
	if legacy := viper.GetString(mobileTokenViperKey); legacy != "" {
		a.MobilePairings.MigrateLegacyToken(legacy)
		viper.Set(mobileTokenViperKey, "")
		_ = viper.WriteConfig()
	}

	// 启动时把全局默认时区配置同步到 Store（影响联系人侧分析查询）
	if off, ok := defaultTzOffsetMinutes(); ok {
		s.SetDefaultTzModifier(buildTzModifier(off))
	}

	// 初始化语音转文字缓存
	if ts, err := transcripts.NewStore(conf.DataDir); err == nil {
		a.Transcripts = ts
	}

	// Initialize sync scheduler
	syncFunc := func() error {
		_, _, err := decrypt.RunTask(conf.WechatDbSrcPath, conf.WechatDbKey)
		if err != nil {
			return err
		}
		return s.Reload()
	}
	a.SyncScheduler = intsync.NewScheduler(syncFunc, filepath.Join(conf.DataDir, "sync_history.json"))

	// Restore sync config from viper
	if viper.GetBool("SYNC_ENABLED") {
		interval := viper.GetInt("SYNC_INTERVAL_MINUTES")
		if interval < 5 {
			interval = 30
		}
		a.SyncScheduler.Configure(true, interval)
	}

	// Initialize backup scheduler
	backupFunc := a.createBackupFunc(exportSvc)
	historyFile := filepath.Join(viper.GetString("config_path"), "backup_history.json")
	if historyFile == "backup_history.json" {
		home, _ := os.UserHomeDir()
		historyFile = filepath.Join(home, ".wetrace", "backup_history.json")
	}
	a.BackupScheduler = backup.NewScheduler(backupFunc, historyFile)

	// Restore backup config from viper
	if viper.GetBool("BACKUP_ENABLED") {
		hours := viper.GetInt("BACKUP_INTERVAL_HOURS")
		if hours < 1 {
			hours = 24
		}
		bPath := viper.GetString("BACKUP_PATH")
		bFormat := viper.GetString("BACKUP_FORMAT")
		if bFormat == "" {
			bFormat = "html"
		}
		a.BackupScheduler.Configure(true, hours, bPath, bFormat)
	}

	// Initialize monitor store
	monitorDir := filepath.Join(viper.GetString("config_path"), "monitor")
	if monitorDir == "monitor" {
		home, _ := os.UserHomeDir()
		monitorDir = filepath.Join(home, ".wetrace")
	}
	monitorStore, err := monitor.NewStore(monitorDir)
	if err == nil {
		a.Monitor = monitorStore
		// Initialize monitor checker
		a.MonitorChecker = monitor.NewChecker(s, monitorStore, aiClient)
		a.MonitorChecker.Start()
	}

	// 启动 Telegram Bot worker（如果配置了 bot_chat_enabled）
	a.ApplyTelegramBotConfig()

	// 启动后台预热：当年 Top5 联系人 + 全局年度报告 缓存一遍，
	// 下次进入秒开。串行 + 30 秒延迟，避免和首屏请求抢资源 / 导致数据错乱。
	a.PrewarmTopContacts()

	// Initialize TTS client from viper config
	if viper.GetBool("TTS_ENABLED") {
		if viper.GetBool("TTS_LOCAL_MODE") {
			binPath := viper.GetString("TTS_LOCAL_BINARY")
			modelPath := viper.GetString("TTS_LOCAL_MODEL")
			if binPath != "" && modelPath != "" {
				a.TTS = tts.NewLocalClient(binPath, modelPath)
			}
		} else {
			ttsKey := viper.GetString("TTS_API_KEY")
			ttsURL := viper.GetString("TTS_BASE_URL")
			ttsModel := viper.GetString("TTS_MODEL")
			if ttsKey != "" && ttsURL != "" {
				a.TTS = tts.NewClient(ttsKey, ttsURL, ttsModel)
			}
		}
	}

	return a
}

// createBackupFunc creates the backup function that exports sessions.
// When sessionIDs is empty, all sessions are backed up.
func (a *API) createBackupFunc(exportSvc *export.Service) backup.BackupFunc {
	return func(backupPath, format string, sessionIDs []string) (string, int, error) {
		ctx := context.Background()

		sessions, err := a.Store.GetSessions(ctx, types.SessionQuery{Limit: 100000})
		if err != nil {
			return "", 0, fmt.Errorf("获取会话列表失败: %w", err)
		}

		// Filter sessions if sessionIDs is provided
		if len(sessionIDs) > 0 {
			idSet := make(map[string]bool, len(sessionIDs))
			for _, id := range sessionIDs {
				idSet[id] = true
			}
			filtered := sessions[:0]
			for _, sess := range sessions {
				if idSet[sess.UserName] {
					filtered = append(filtered, sess)
				}
			}
			sessions = filtered
		}

		// Create timestamped subdirectory: backupPath/backup_20250211_150405/
		timestamp := time.Now().Format("20060102_150405")
		backupDir := filepath.Join(backupPath, fmt.Sprintf("backup_%s", timestamp))
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return "", 0, fmt.Errorf("创建备份目录失败: %w", err)
		}

		// Use a wide time range to ensure all messages are included.
		// Zero time causes the shard router to return no results.
		allStart := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		allEnd := time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

		count := 0
		for _, sess := range sessions {
			talker := sess.UserName
			name := sess.NickName
			if name == "" {
				name = talker
			}

			var data []byte
			switch format {
			case "txt":
				data, err = exportSvc.ExportChatTxt(ctx, talker, name, allStart, allEnd)
			default:
				data, err = exportSvc.ExportChat(ctx, talker, name, allStart, allEnd)
			}
			if err != nil {
				continue
			}

			ext := ".zip"
			if format == "txt" {
				ext = ".txt"
			}
			fname := filepath.Join(backupDir, fmt.Sprintf("%s_%s%s", name, timestamp, ext))
			if writeErr := os.WriteFile(fname, data, 0644); writeErr != nil {
				continue
			}
			count++
		}

		// Write a summary marker file as the "output"
		summaryFile := filepath.Join(backupDir, "summary.txt")
		summary := fmt.Sprintf("Backup completed at %s, %d sessions exported", timestamp, count)
		_ = os.WriteFile(summaryFile, []byte(summary), 0644)

		return backupDir, count, nil
	}
}

// ApplyTelegramBotConfig 根据当前 TelegramConfig 启停 Bot worker
// 应在 API 初始化完成时调一次，以及每次用户更新配置后调一次
func (a *API) ApplyTelegramBotConfig() {
	a.tgBotMu.Lock()
	defer a.tgBotMu.Unlock()
	// 先停掉已有的
	if a.TgBot != nil {
		a.TgBot.Stop()
		a.TgBot = nil
	}
	if a.Monitor == nil {
		return
	}
	cfg := a.Monitor.GetTelegramConfig()
	if !cfg.BotChatEnabled || cfg.BotToken == "" {
		return
	}
	bot := telegram.New(telegram.Config{
		BotToken:          cfg.BotToken,
		AuthorizedChatIDs: cfg.AuthorizedChatIDs,
		Store:             a.Store,
	})
	bot.Start(context.Background())
	a.TgBot = bot
}
