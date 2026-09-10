package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// ── 密码哈希的存取 ──────────────────────────────────────────────
//
// bcrypt 哈希形如 `$2a$10$....`，而 viper 读 .env 用的 gotenv 会对未加引号的值
// 做 `$` 变量展开 —— `$2`、`$10` 会被当成变量吃掉，于是写进文件的哈希和程序
// 读出来的不是同一个值，bcrypt 比对永远失败、输什么密码都说错。
//
// 所以落盘时先做 base64（结果不含 `$`），读取时再解回来。
// 旧的裸哈希也兼容读取，只是那种值多半已经被展开破坏、只能重设密码。

const passwordHashKey = "PASSWORD_HASH"

func savePasswordHash(hash []byte) {
	viper.Set(passwordHashKey, "b64:"+base64.StdEncoding.EncodeToString(hash))
}

func loadPasswordHash() string {
	raw := viper.GetString(passwordHashKey)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "b64:") {
		if b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "b64:")); err == nil {
			return string(b)
		}
		return ""
	}
	// 旧格式：裸 bcrypt 哈希。没被 `$` 展开破坏的话仍然可用
	return raw
}

// passwordConfigured 判断是否设过密码（用原始值判断，避免把损坏的旧值当成没设）
func passwordConfigured() bool {
	return viper.GetString(passwordHashKey) != ""
}

// PasswordConfigured 供中间件判断「是否启用了密码保护」
func PasswordConfigured() bool { return passwordConfigured() }

// PasswordManager 管理密码保护状态
type PasswordManager struct {
	mu       sync.Mutex
	sessions map[string]bool // token -> valid
}

// NewPasswordManager 创建密码管理器
func NewPasswordManager() *PasswordManager {
	return &PasswordManager{
		sessions: make(map[string]bool),
	}
}

// generateToken 生成随机 token
func (pm *PasswordManager) generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AddSession 添加已验证的会话
func (pm *PasswordManager) AddSession(token string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessions[token] = true
}

// IsValidSession 检查会话是否有效
func (pm *PasswordManager) IsValidSession(token string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.sessions[token]
}

// ClearSessions 清除所有会话
func (pm *PasswordManager) ClearSessions() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessions = make(map[string]bool)
}

// GetPasswordStatus 获取密码保护状态
func (a *API) GetPasswordStatus(c *gin.Context) {
	enabled := passwordConfigured()

	isLocked := false
	if enabled && a.Password != nil {
		token := c.GetHeader("X-Auth-Token")
		if token == "" {
			token, _ = c.Cookie("auth_token")
		}
		isLocked = !a.Password.IsValidSession(token)
	}

	transport.SendSuccess(c, gin.H{
		"enabled":   enabled,
		"is_locked": isLocked,
	})
}

// SetPassword 设置/修改密码
func (a *API) SetPassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	if len(req.NewPassword) < 4 {
		transport.BadRequest(c, "密码长度不能少于4位")
		return
	}

	existingHash := loadPasswordHash()

	// 如果已有密码，需要验证旧密码
	if existingHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(req.OldPassword)); err != nil {
			transport.BadRequest(c, "旧密码错误")
			return
		}
	}

	// 生成新密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		transport.InternalServerError(c, "密码加密失败")
		return
	}

	savePasswordHash(hash)
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 清除所有现有会话，要求重新验证
	if a.Password != nil {
		a.Password.ClearSessions()
	}

	transport.SendSuccess(c, gin.H{"status": "password_set"})
}

// VerifyPassword 验证密码（解锁）
func (a *API) VerifyPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	hash := loadPasswordHash()
	if hash == "" {
		transport.BadRequest(c, "未设置密码")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		transport.BadRequest(c, "密码错误")
		return
	}

	// 生成会话 token
	if a.Password == nil {
		a.Password = NewPasswordManager()
	}
	token, err := a.Password.generateToken()
	if err != nil {
		transport.InternalServerError(c, "生成会话失败")
		return
	}
	a.Password.AddSession(token)

	// 设置 cookie
	c.SetCookie("auth_token", token, 86400, "/", "", false, true)

	transport.SendSuccess(c, gin.H{
		"status": "unlocked",
		"token":  token,
	})
}

// DisablePassword 关闭密码保护
func (a *API) DisablePassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	hash := loadPasswordHash()
	if hash == "" {
		transport.BadRequest(c, "未设置密码")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		transport.BadRequest(c, "密码错误")
		return
	}

	// 清除密码哈希
	viper.Set(passwordHashKey, "")
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 清除所有会话
	if a.Password != nil {
		a.Password.ClearSessions()
	}

	transport.SendSuccess(c, gin.H{"status": "disabled"})
}
