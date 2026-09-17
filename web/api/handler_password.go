package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

// sessionTTL 与 cookie 的 max-age 一致。以前服务端的会话永不过期，
// token 一旦泄露（比如被抓包、留在别人电脑的浏览器里）就一直有效，直到改密码。
const sessionTTL = 24 * time.Hour

// PasswordManager 管理密码保护状态
type PasswordManager struct {
	mu       sync.Mutex
	sessions map[string]time.Time // token -> 过期时间
	failures map[string]*unlockFailures
}

// NewPasswordManager 创建密码管理器
func NewPasswordManager() *PasswordManager {
	return &PasswordManager{
		sessions: make(map[string]time.Time),
		failures: make(map[string]*unlockFailures),
	}
}

// unlockFailures 记录某个来源连续输错密码的情况。
// 密码最短只有 4 位，放到局域网上不限速的话几分钟就能穷举完。
type unlockFailures struct {
	count       int
	lockedUntil time.Time
}

const (
	freeUnlockAttempts = 5
	maxUnlockLockout   = 15 * time.Minute
)

// unlockBlocked 返回还需要等多久；0 表示可以尝试。
func (pm *PasswordManager) unlockBlocked(source string) time.Duration {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if f := pm.failures[source]; f != nil {
		if wait := time.Until(f.lockedUntil); wait > 0 {
			return wait
		}
	}
	return 0
}

// recordUnlock 记一次解锁结果：成功清零，失败超过免费次数后按 30s、60s、120s… 翻倍锁定。
func (pm *PasswordManager) recordUnlock(source string, ok bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if ok {
		delete(pm.failures, source)
		return
	}
	f := pm.failures[source]
	if f == nil {
		f = &unlockFailures{}
		pm.failures[source] = f
	}
	f.count++
	if f.count > freeUnlockAttempts {
		wait := 30 * time.Second << min(f.count-freeUnlockAttempts-1, 10)
		if wait > maxUnlockLockout {
			wait = maxUnlockLockout
		}
		f.lockedUntil = time.Now().Add(wait)
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
	now := time.Now()
	for t, exp := range pm.sessions { // 顺手清掉过期的，免得 map 只增不减
		if now.After(exp) {
			delete(pm.sessions, t)
		}
	}
	pm.sessions[token] = now.Add(sessionTTL)
}

// IsValidSession 检查会话是否有效
func (pm *PasswordManager) IsValidSession(token string) bool {
	if pm == nil || token == "" {
		return false
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	exp, ok := pm.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(pm.sessions, token)
		return false
	}
	return true
}

// ClearSessions 清除所有会话
func (pm *PasswordManager) ClearSessions() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessions = make(map[string]time.Time)
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
	if err := saveConfig(); err != nil {
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

	if a.Password == nil {
		a.Password = NewPasswordManager()
	}
	source := unlockSource(c)
	if wait := a.Password.unlockBlocked(source); wait > 0 {
		c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"error": gin.H{
				"code":    http.StatusTooManyRequests,
				"message": fmt.Sprintf("密码错误次数过多，请 %d 秒后再试", int(wait.Seconds())+1),
			},
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		a.Password.recordUnlock(source, false)
		transport.BadRequest(c, "密码错误")
		return
	}
	a.Password.recordUnlock(source, true)

	// 生成会话 token
	token, err := a.Password.generateToken()
	if err != nil {
		transport.InternalServerError(c, "生成会话失败")
		return
	}
	a.Password.AddSession(token)

	// 设置 cookie（SameSite=Strict：跨站请求一律不带上它）
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("auth_token", token, int(sessionTTL.Seconds()), "/", "", false, true)

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
	if err := saveConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}

	// 清除所有会话
	if a.Password != nil {
		a.Password.ClearSessions()
	}

	transport.SendSuccess(c, gin.H{"status": "disabled"})
}

// unlockSource 限速按真实 TCP 对端计，不看 X-Forwarded-For（可伪造）。
func unlockSource(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
