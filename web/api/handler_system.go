package api

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/afumu/wetrace/internal/ai"
	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/internal/tts"
	"github.com/afumu/wetrace/pkg/util"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// --- AI Prompts JSON file storage ---

var (
	promptsFilePath string
	promptsMu       sync.RWMutex
)

// initPromptsFilePath sets the package-level prompts JSON file path.
// Called once from NewAPI during initialization.
func initPromptsFilePath(dataDir string) {
	promptsFilePath = filepath.Join(dataDir, "ai_prompts.json")
}

// loadPromptsFromFile reads custom prompts from the JSON file.
// Returns an empty map if the file does not exist.
func loadPromptsFromFile() (map[string]string, error) {
	promptsMu.RLock()
	defer promptsMu.RUnlock()

	data, err := os.ReadFile(promptsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	var prompts map[string]string
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, err
	}
	return prompts, nil
}

// savePromptsToFile writes custom prompts to the JSON file.
func savePromptsToFile(prompts map[string]string) error {
	promptsMu.Lock()
	defer promptsMu.Unlock()

	dir := filepath.Dir(promptsFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(promptsFilePath, data, 0644)
}

// SelectPath 让用户在服务端（本地）选择路径
func (a *API) SelectPath(c *gin.Context) {
	type SelectReq struct {
		Type string `json:"type"` // "file" or "folder"
	}
	var req SelectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	var path string
	var err error

	if req.Type == "file" {
		path, err = util.OpenFileDialog("选择微信可执行文件", "WeChat Executable|WeChat.exe;Weixin.exe|All Files|*.*")
	} else {
		path, err = util.OpenFolderDialog("选择微信数据目录 (xwechat_files)")
	}

	if err != nil {
		// 用户取消
		if err.Error() == "cancelled" {
			transport.SendSuccess(c, gin.H{"path": ""})
			return
		}
		// 其他错误
		transport.InternalServerError(c, "打开对话框失败: "+err.Error())
		return
	}

	transport.SendSuccess(c, gin.H{"path": path})
}

// GetSystemStatus 返回应用程序的当前状态。
// 目前，它只确认服务正在运行。
func (a *API) GetSystemStatus(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := gin.H{
		"store_initialized": true,
		"platform":          runtime.GOOS,
		"config": gin.H{
			"has_wechat_db_key": a.Conf.WechatDbKey != "",
			"has_image_key":     a.Media.ImageKey != "",
			"has_xor_key":       a.Media.XorKey != "",
			// 只给脱敏值，明文密钥不出接口
			"wechat_db_key_masked": maskSecret(a.Conf.WechatDbKey),
			"image_key_masked":     maskSecret(a.Media.ImageKey),
			"xor_key_masked":       a.Media.XorKey, // 单字节异或键，本身不算秘密
			"wechat_path":          a.Conf.WechatPath,
			"wechat_db_src_path":   a.Conf.WechatDbSrcPath,
		},
	}
	transport.SendSuccess(c, status)
}

// DetectWeChatInstallPath 检测微信安装路径
func (a *API) DetectWeChatInstallPath(c *gin.Context) {
	paths := util.FindWeChatInstallPaths()
	transport.SendSuccess(c, paths)
}

// DetectWeChatDataPath 检测微信数据路径
func (a *API) DetectWeChatDataPath(c *gin.Context) {
	paths := util.FindWeChatDataPaths()
	transport.SendSuccess(c, paths)
}

// UpdateConfig 更新系统配置
func (a *API) UpdateConfig(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	// 允许更新的键白名单
	allowedKeys := map[string]bool{
		"WXKEY_WECHAT_PATH":  true,
		"WECHAT_DB_SRC_PATH": true,
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	changed := false
	for k, v := range req {
		if allowedKeys[k] {
			viper.Set(k, v)
			// 同步更新内存中的配置对象
			if k == "WXKEY_WECHAT_PATH" {
				a.Conf.WechatPath = v
			} else if k == "WECHAT_DB_SRC_PATH" {
				a.Conf.WechatDbSrcPath = v
			}
			changed = true
		}
	}

	if changed {
		if err := viper.WriteConfig(); err != nil {
			// 如果文件不存在，尝试创建
			if err := viper.WriteConfigAs(".env"); err != nil {
				transport.InternalServerError(c, "保存配置文件失败: "+err.Error())
				return
			}
		}
	}

	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// maskAPIKey 对 API Key 做脱敏处理，仅显示前4位和后4位
// maskSecret 只保留头尾各 4 位，中间打码 —— 用于把密钥展示给用户核对，
// 但不把明文送出接口。
func maskSecret(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "••••"
	}
	return key[:4] + "••••••••" + key[len(key)-4:]
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// aiProviderViperKey 返回某服务商 Key 在 viper 中的配置键名
func aiProviderViperKey(provider string) string {
	return "AI_KEY_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(provider))
}

// GetAIConfig 获取 AI 配置
func (a *API) GetAIConfig(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	masked := ""
	if a.Conf.AIAPIKey != "" {
		masked = maskAPIKey(a.Conf.AIAPIKey)
	}

	// 返回所有已保存服务商的脱敏 Key
	knownProviders := []string{"openai", "deepseek", "google", "anthropic", "ollama", "moonshot", "qwen", "zhipu"}
	providerKeysMasked := make(map[string]string)
	for _, p := range knownProviders {
		k := viper.GetString(aiProviderViperKey(p))
		if k != "" {
			providerKeysMasked[p] = maskAPIKey(k)
		}
	}

	transport.SendSuccess(c, gin.H{
		"enabled":              a.Conf.AIEnabled,
		"provider":             a.Conf.AIProvider,
		"model":                a.Conf.AIModel,
		"base_url":             a.Conf.AIBaseURL,
		"api_key_masked":       masked,
		"provider_keys_masked": providerKeysMasked,
	})
}

// UpdateAIConfig 更新 AI 配置
func (a *API) UpdateAIConfig(c *gin.Context) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// 若前端传了新 Key，保存到该服务商专属配置项
	if req.APIKey != "" && req.Provider != "" {
		viper.Set(aiProviderViperKey(req.Provider), req.APIKey)
	}

	// 未传 Key 时从该服务商专属配置项加载，再回退到全局旧 Key
	if req.APIKey == "" {
		req.APIKey = viper.GetString(aiProviderViperKey(req.Provider))
	}
	if req.APIKey == "" {
		req.APIKey = a.Conf.AIAPIKey
	}

	if req.Enabled {
		if req.Model == "" || req.BaseURL == "" || req.APIKey == "" {
			transport.BadRequest(c, "启用 AI 时必须提供 model、base_url 和 api_key")
			return
		}
	}

	// 持久化到 viper
	viper.Set("AI_ENABLED", req.Enabled)
	viper.Set("AI_PROVIDER", req.Provider)
	viper.Set("AI_MODEL", req.Model)
	viper.Set("AI_BASE_URL", req.BaseURL)
	viper.Set("AI_API_KEY", req.APIKey)

	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 同步更新内存配置
	a.Conf.AIEnabled = req.Enabled
	a.Conf.AIProvider = req.Provider
	a.Conf.AIModel = req.Model
	a.Conf.AIBaseURL = req.BaseURL
	a.Conf.AIAPIKey = req.APIKey

	// 重建 AI 客户端
	if req.Enabled {
		a.AI = ai.NewClient(req.APIKey, req.BaseURL, req.Model)
	} else {
		a.AI = nil
	}

	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// TestAIConfig 测试 AI 连接
func (a *API) TestAIConfig(c *gin.Context) {
	a.mu.Lock()
	client := a.AI
	a.mu.Unlock()

	if client == nil {
		transport.BadRequest(c, "AI 未启用或未配置")
		return
	}

	start := time.Now()
	_, err := client.Chat([]ai.Message{
		{Role: "user", Content: "ping"},
	})
	latency := time.Since(start).Milliseconds()

	if err != nil {
		transport.InternalServerError(c, "AI 连接测试失败: "+err.Error())
		return
	}

	a.mu.Lock()
	model := a.Conf.AIModel
	a.mu.Unlock()

	transport.SendSuccess(c, gin.H{
		"status":     "connected",
		"model":      model,
		"latency_ms": latency,
	})
}

// GetCompliance 获取合规同意状态
func (a *API) GetCompliance(c *gin.Context) {
	agreed := viper.GetBool("COMPLIANCE_AGREED")
	agreedAt := viper.GetString("COMPLIANCE_AGREED_AT")
	version := viper.GetString("COMPLIANCE_VERSION")
	if version == "" {
		version = "1.0"
	}

	transport.SendSuccess(c, gin.H{
		"agreed":    agreed,
		"agreed_at": agreedAt,
		"version":   version,
	})
}

// defaultAIPrompts 返回所有 AI 功能的默认提示词
func defaultAIPrompts() map[string]string {
	return map[string]string{
		"summarize": "以下是一段微信聊天记录，请简要总结对话的核心内容和主要结论：\n\n",

		"simulate": `你现在是一个高级人工智能，你的任务是精准模拟一个名为 "{{target_name}}" 的人的微信聊天风格。

你需要通过分析以下提供的聊天记录，学习并模仿 {{target_name}} 的以下特征：
1. 语气与口吻：是热情、冷淡、幽默还是严肃？
2. 常用词汇：是否有特定的口头禅、简称或习惯性用语？
3. 表情习惯：是否经常使用表情符号（如 [微笑]、[呲牙]）或 Emoji？使用的频率如何？
4. 回复长度：习惯发长句子还是短句？
5. 标点符号：是否经常使用标点，还是习惯直接空格？

历史聊天记录（参考上下文）：
{{history}}

模仿要点：
- 你现在就是 {{target_name}}。
- 严禁以 AI 助手的身份说话。
- 回复内容必须简洁自然，符合微信聊天的即时性。
- 直接输出回复内容，不要附带任何解释或前缀。`,

		"sentiment": `以下是按月份整理的微信聊天记录，请进行情感分析。

{{monthly_texts}}

请严格按照以下 JSON 格式返回分析结果，不要包含任何其他文字：
{
  "overall_score": 0.72,
  "overall_label": "积极/消极/中立",
  "relationship_health": "良好/一般/需关注",
  "summary": "整体分析总结...",
  "emotion_timeline": [
    {
      "period": "2025-01",
      "score": 0.8,
      "label": "积极/消极/中立",
      "keywords": ["关键词1", "关键词2"]
    }
  ],
  "sentiment_distribution": {
    "positive": 0.58,
    "neutral": 0.30,
    "negative": 0.12
  },
  "relationship_indicators": {
    "initiative_ratio": 0.52,
    "response_speed": "快/中/慢",
    "intimacy_trend": "上升/稳定/下降"
  }
}

说明：
- overall_score: 0-1之间的情感评分，越高越积极
- emotion_timeline: 按月份的情绪变化，每个月一条记录
- initiative_ratio: 主动发起对话的比例（0-1）
- 所有数值保留两位小数`,

		"summary": `以下是一段微信聊天记录，请生成结构化摘要。要求：
1. 核心话题：列出讨论的主要话题（不超过5个）
2. 关键结论：列出达成的共识或结论
3. 待跟进事项：列出需要后续跟进的事项
4. 一句话总结：用一句话概括整段对话

请严格按照以下 JSON 格式返回，不要包含其他文字：
{
  "topics": ["话题1", "话题2"],
  "conclusions": ["结论1", "结论2"],
  "follow_ups": ["跟进事项1", "跟进事项2"],
  "one_line_summary": "一句话总结"
}

聊天记录：
`,

		"extract_todos": `以下是一段微信聊天记录，请从中提取所有待办事项、任务、提醒和承诺。
包括但不限于：
- 明确的任务分配（"帮我..."、"你去..."）
- 时间约定（"明天..."、"下周..."）
- 承诺和提醒（"记得..."、"别忘了..."）
- 需要回复或跟进的事项

请严格按照以下 JSON 格式返回，不要包含其他文字：
{
  "todos": [
    {
      "content": "待办事项描述",
      "deadline": "截止时间（如有，ISO 8601格式，无则为空字符串）",
      "priority": "high/medium/low",
      "source_msg": "原始消息内容",
      "source_time": "消息时间"
    }
  ]
}

如果没有找到任何待办事项，返回 {"todos": []}

聊天记录：
`,

		"extract_info": `以下是一段微信聊天记录，请从中提取以下类型的关键信息：{{types_hint}}

提取规则：
- address: 具体地址、地点、位置信息
- time: 时间约定、日期安排（非消息本身的时间戳）
- amount: 金额、价格、费用
- phone: 电话号码、手机号

请严格按照以下 JSON 格式返回，不要包含其他文字：
{
  "extractions": [
    {
      "type": "address/time/amount/phone",
      "value": "提取的具体值",
      "context": "包含该信息的原始消息",
      "time": "消息时间"
    }
  ]
}

如果没有找到任何信息，返回 {"extractions": []}

聊天记录：
`,
	}
}

// GetAIPrompts 获取所有 AI 提示词配置
func (a *API) GetAIPrompts(c *gin.Context) {
	defaults := defaultAIPrompts()

	custom, err := loadPromptsFromFile()
	if err != nil {
		transport.InternalServerError(c, "读取提示词文件失败: "+err.Error())
		return
	}

	// Merge: custom overrides defaults
	prompts := make(map[string]string, len(defaults))
	for key, defaultVal := range defaults {
		if v, ok := custom[key]; ok && v != "" {
			prompts[key] = v
		} else {
			prompts[key] = defaultVal
		}
	}

	transport.SendSuccess(c, gin.H{
		"prompts":  prompts,
		"defaults": defaults,
	})
}

// UpdateAIPrompts 更新 AI 提示词配置
func (a *API) UpdateAIPrompts(c *gin.Context) {
	var req struct {
		Prompts map[string]string `json:"prompts"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	defaults := defaultAIPrompts()

	// Load existing custom prompts
	custom, err := loadPromptsFromFile()
	if err != nil {
		transport.InternalServerError(c, "读取提示词文件失败: "+err.Error())
		return
	}

	// Update custom prompts
	for key, val := range req.Prompts {
		if _, ok := defaults[key]; !ok {
			continue // 忽略未知的 key
		}
		if val == "" || val == defaults[key] {
			// 如果为空或与默认值相同，删除自定义配置
			delete(custom, key)
		} else {
			custom[key] = val
		}
	}

	if err := savePromptsToFile(custom); err != nil {
		transport.InternalServerError(c, "保存提示词文件失败: "+err.Error())
		return
	}

	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// GetAIPrompt 获取指定 AI 功能的提示词（优先自定义，否则默认）
func GetAIPrompt(key string) string {
	custom, err := loadPromptsFromFile()
	if err == nil {
		if v, ok := custom[key]; ok && v != "" {
			return v
		}
	}
	defaults := defaultAIPrompts()
	if val, ok := defaults[key]; ok {
		return val
	}
	return ""
}

// AgreeCompliance 提交合规同意
func (a *API) AgreeCompliance(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	if req.Version == "" {
		req.Version = "1.0"
	}

	now := time.Now().Format(time.RFC3339)

	viper.Set("COMPLIANCE_AGREED", true)
	viper.Set("COMPLIANCE_AGREED_AT", now)
	viper.Set("COMPLIANCE_VERSION", req.Version)

	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	transport.SendSuccess(c, gin.H{"status": "agreed"})
}

// GetTTSConfig 获取语音转文字配置
func (a *API) GetTTSConfig(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	masked := ""
	key := viper.GetString("TTS_API_KEY")
	if key != "" {
		masked = maskAPIKey(key)
	}

	transport.SendSuccess(c, gin.H{
		"enabled":        viper.GetBool("TTS_ENABLED"),
		"provider":       viper.GetString("TTS_PROVIDER"),
		"base_url":       viper.GetString("TTS_BASE_URL"),
		"api_key_masked": masked,
		"model":          viper.GetString("TTS_MODEL"),
		"local_mode":     viper.GetBool("TTS_LOCAL_MODE"),
		"local_binary":   viper.GetString("TTS_LOCAL_BINARY"),
		"local_model":    viper.GetString("TTS_LOCAL_MODEL"),
		"auto":           viper.GetBool("TTS_AUTO"),
	})
}

// UpdateTTSConfig 更新语音转文字配置
func (a *API) UpdateTTSConfig(c *gin.Context) {
	var req struct {
		Enabled     bool   `json:"enabled"`
		Provider    string `json:"provider"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		Model       string `json:"model"`
		LocalMode   bool   `json:"local_mode"`
		LocalBinary string `json:"local_binary"`
		LocalModel  string `json:"local_model"`
		// Auto 打开后，每次数据同步完成会自动把新语音转成文字
		Auto bool `json:"auto"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	viper.Set("TTS_ENABLED", req.Enabled)
	viper.Set("TTS_PROVIDER", req.Provider)
	viper.Set("TTS_BASE_URL", req.BaseURL)
	if req.APIKey != "" {
		viper.Set("TTS_API_KEY", req.APIKey)
	}
	viper.Set("TTS_MODEL", req.Model)
	viper.Set("TTS_LOCAL_MODE", req.LocalMode)
	viper.Set("TTS_LOCAL_BINARY", req.LocalBinary)
	viper.Set("TTS_LOCAL_MODEL", req.LocalModel)
	viper.Set("TTS_AUTO", req.Auto)

	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 重建 TTS 客户端
	if req.Enabled {
		if req.LocalMode {
			if req.LocalBinary != "" && req.LocalModel != "" {
				a.TTS = tts.NewLocalClient(req.LocalBinary, req.LocalModel)
			}
		} else {
			apiKey := req.APIKey
			if apiKey == "" {
				apiKey = viper.GetString("TTS_API_KEY")
			}
			if apiKey != "" && req.BaseURL != "" {
				a.TTS = tts.NewClient(apiKey, req.BaseURL, req.Model)
			}
		}
	} else {
		a.TTS = nil
	}

	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// GetDataDir 返回数据目录的绝对路径
func (a *API) GetDataDir(c *gin.Context) {
	abs, err := filepath.Abs(a.Conf.DataDir)
	if err != nil {
		abs = a.Conf.DataDir
	}
	transport.SendSuccess(c, gin.H{"path": abs})
}

// OpenDataDir 在系统文件管理器中打开数据目录
func (a *API) OpenDataDir(c *gin.Context) {
	abs, err := filepath.Abs(a.Conf.DataDir)
	if err != nil {
		abs = a.Conf.DataDir
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		transport.InternalServerError(c, "目录不存在: "+err.Error())
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", abs)
	case "darwin":
		cmd = exec.Command("open", abs)
	default:
		cmd = exec.Command("xdg-open", abs)
	}
	_ = cmd.Start()
	transport.SendSuccess(c, gin.H{"path": abs})
}

// GetEffectiveChatStart 获取「有效聊天记录起始时间」(用于年度报告往年同期对比的下界年份)
func (a *API) GetEffectiveChatStart(c *gin.Context) {
	year := viper.GetInt("EFFECTIVE_CHAT_START_YEAR")
	if year == 0 {
		year = 2023
	}
	transport.SendSuccess(c, gin.H{"year": year})
}

// UpdateEffectiveChatStart 更新「有效聊天记录起始时间」
func (a *API) UpdateEffectiveChatStart(c *gin.Context) {
	var req struct {
		Year int `json:"year"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误")
		return
	}
	if req.Year < 2000 || req.Year > 2100 {
		transport.BadRequest(c, "年份必须在 2000-2100 之间")
		return
	}
	viper.Set("EFFECTIVE_CHAT_START_YEAR", req.Year)
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"year": req.Year})
}

// effectiveChatStartYear 内部读取（默认 2023）
func effectiveChatStartYear() int {
	y := viper.GetInt("EFFECTIVE_CHAT_START_YEAR")
	if y < 2000 || y > 2100 {
		return 2023
	}
	return y
}

// GetDefaultTimezone 获取「默认时区偏移分钟」（影响联系人侧的小时/星期/月度等查询）
func (a *API) GetDefaultTimezone(c *gin.Context) {
	off, hasKey := defaultTzOffsetMinutes()
	transport.SendSuccess(c, gin.H{"offset": off, "has_key": hasKey})
}

// UpdateDefaultTimezone 更新默认时区
func (a *API) UpdateDefaultTimezone(c *gin.Context) {
	var req struct {
		Offset int `json:"offset"` // 分钟，东正西负，UTC+8 → 480
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误")
		return
	}
	if req.Offset < -720 || req.Offset > 840 {
		transport.BadRequest(c, "offset 必须在 -720 ~ 840 之间（分钟）")
		return
	}
	viper.Set("DEFAULT_TZ_OFFSET_MINUTES", req.Offset)
	viper.Set("DEFAULT_TZ_OFFSET_SET", true)
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}
	// 同步到 Repository
	a.Store.SetDefaultTzModifier(buildTzModifier(req.Offset))
	transport.SendSuccess(c, gin.H{"offset": req.Offset})
}

// defaultTzOffsetMinutes 内部读取（未设置返回 false 让前端用浏览器时区填充）
func defaultTzOffsetMinutes() (int, bool) {
	if !viper.IsSet("DEFAULT_TZ_OFFSET_SET") || !viper.GetBool("DEFAULT_TZ_OFFSET_SET") {
		return 0, false
	}
	return viper.GetInt("DEFAULT_TZ_OFFSET_MINUTES"), true
}

// buildTzModifier 把分钟偏移转换为 SQLite strftime 修饰符字符串
func buildTzModifier(offsetMinutes int) string {
	if offsetMinutes == 0 {
		return "'utc'"
	}
	return fmt.Sprintf("'%+d seconds'", offsetMinutes*60)
}

// GetDataVersion 返回当前数据的指纹，前端用它判断本地缓存的报告是否还有效。
// 指纹里除了消息库文件，还含已转写的语音条数 —— 转写字数会计入报告，
// 但转写结果不在消息库里，只看消息库的话转写完前端还会拿旧缓存。
func (a *API) GetDataVersion(c *gin.Context) {
	transport.SendSuccess(c, gin.H{"version": a.reportDataVersion()})
}

// GetChangelog 返回内嵌的 CHANGELOG.md 内容。
func (a *API) GetChangelog(c *gin.Context) {
	transport.SendSuccess(c, gin.H{"content": a.Conf.ChangelogContent})
}

// ── 全站时区口径：默认时区 + 分段 ─────────────────────────────
//
// 以前这份配置分两处：设置页的「默认时区」存服务端、只管联系人统计；
// 年度报告的分段存浏览器 localStorage、只管报告。同一批消息在两个页面
// 会被切到不同的自然日里。现在合成一份，所有按时区分桶的查询共用。

// tzSegmentsFromViper 读出持久化的分段。存的是 JSON 字符串，
// 因为 viper 写 .env 时只能存标量。
func tzSegmentsFromViper() []model.TZSegmentConfig {
	raw := viper.GetString("TZ_SEGMENTS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var segs []model.TZSegmentConfig
	if err := json.Unmarshal([]byte(raw), &segs); err != nil {
		log.Warn().Err(err).Msg("TZ_SEGMENTS 解析失败，按无分段处理")
		return nil
	}
	return segs
}

// CurrentTZConfig 汇总出当前生效的时区口径。
func CurrentTZConfig() model.TZConfig {
	off, _ := defaultTzOffsetMinutes()
	return model.TZConfig{DefaultOffset: off, Segments: tzSegmentsFromViper()}
}

// GetTZConfig GET /api/v1/system/tz_config
func (a *API) GetTZConfig(c *gin.Context) {
	off, hasKey := defaultTzOffsetMinutes()
	segs := tzSegmentsFromViper()
	if segs == nil {
		segs = []model.TZSegmentConfig{}
	}
	transport.SendSuccess(c, gin.H{
		"default_offset": off,
		"has_key":        hasKey,
		"segments":       segs,
	})
}

// UpdateTZConfig POST /api/v1/system/tz_config
func (a *API) UpdateTZConfig(c *gin.Context) {
	var req struct {
		DefaultOffset int                     `json:"default_offset"`
		Segments      []model.TZSegmentConfig `json:"segments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误")
		return
	}
	if req.DefaultOffset < -720 || req.DefaultOffset > 840 {
		transport.BadRequest(c, "default_offset 必须在 -720 ~ 840 之间（分钟）")
		return
	}
	for _, s := range req.Segments {
		if s.TZOffset < -720 || s.TZOffset > 840 {
			transport.BadRequest(c, "分段时区必须在 -720 ~ 840 之间（分钟）")
			return
		}
		if _, err := time.Parse("2006-01-02", s.StartDate); err != nil {
			transport.BadRequest(c, "分段开始日期格式应为 YYYY-MM-DD: "+s.StartDate)
			return
		}
		if _, err := time.Parse("2006-01-02", s.EndDate); err != nil {
			transport.BadRequest(c, "分段结束日期格式应为 YYYY-MM-DD: "+s.EndDate)
			return
		}
		if s.EndDate < s.StartDate {
			transport.BadRequest(c, "分段结束日期不能早于开始日期: "+s.StartDate+" ~ "+s.EndDate)
			return
		}
	}

	blob, err := json.Marshal(req.Segments)
	if err != nil {
		transport.InternalServerError(c, "序列化分段失败: "+err.Error())
		return
	}
	viper.Set("DEFAULT_TZ_OFFSET_MINUTES", req.DefaultOffset)
	viper.Set("DEFAULT_TZ_OFFSET_SET", true)
	viper.Set("TZ_SEGMENTS", string(blob))
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 同步到 Repository：常量修饰符仍留着当兜底，分段走 CASE
	a.Store.SetDefaultTzModifier(buildTzModifier(req.DefaultOffset))
	a.Store.SetTZConfig(model.TZConfig{DefaultOffset: req.DefaultOffset, Segments: req.Segments})
	transport.SendSuccess(c, gin.H{"default_offset": req.DefaultOffset, "segments": req.Segments})
}
