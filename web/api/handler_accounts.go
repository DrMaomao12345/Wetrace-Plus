package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/afumu/wetrace/decrypt"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Account 表示一个微信账号配置。
type Account struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Label    string    `json:"label"`
	LastUsed time.Time `json:"last_used"`
}

// accountsFile 是持久化到磁盘的账号列表文件结构。
type accountsFile struct {
	ActiveID string    `json:"active_id"`
	Accounts []Account `json:"accounts"`
}

// AccountStore 管理多个微信账号配置。
type AccountStore struct {
	mu       sync.RWMutex
	filePath string
	data     accountsFile
}

// activateMu 串行化账号切换的后台解密 + 重载，
// 避免连续切换时多个 goroutine 同时 RunTask / Store.Reload 造成数据竞态。
var activateMu sync.Mutex

// NewAccountStore 创建一个新的 AccountStore，从 dataDir/accounts.json 加载数据。
func NewAccountStore(dataDir string) *AccountStore {
	s := &AccountStore{
		filePath: filepath.Join(dataDir, "accounts.json"),
	}
	s.load()
	return s
}

// load 从磁盘读取账号列表。调用方应持有写锁或在初始化期间调用。
func (s *AccountStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		// 文件不存在时使用空列表，属于正常情况
		return
	}
	_ = json.Unmarshal(data, &s.data)
}

// save 将账号列表写入磁盘。调用方应已持有写锁。
func (s *AccountStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化账号列表失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	// 0600：含微信数据路径（路径里带 wxid 用户名），与 .env 一致收紧权限
	return os.WriteFile(s.filePath, data, 0600)
}

// List 返回所有账号列表和当前活跃账号 ID。
func (s *AccountStore) List() ([]Account, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accs := make([]Account, len(s.data.Accounts))
	copy(accs, s.data.Accounts)
	return accs, s.data.ActiveID
}

// Add 添加一个新账号并返回该账号。写盘失败时回滚内存并返回错误。
func (s *AccountStore) Add(path, label string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc := Account{
		ID:       fmt.Sprintf("acc_%d", time.Now().UnixNano()),
		Path:     path,
		Label:    label,
		LastUsed: time.Now(),
	}
	s.data.Accounts = append(s.data.Accounts, acc)
	if err := s.save(); err != nil {
		s.data.Accounts = s.data.Accounts[:len(s.data.Accounts)-1]
		return Account{}, err
	}
	return acc, nil
}

// Delete 删除指定 ID 的账号。
func (s *AccountStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.data.Accounts[:0]
	for _, acc := range s.data.Accounts {
		if acc.ID != id {
			filtered = append(filtered, acc)
		}
	}
	s.data.Accounts = filtered
	if s.data.ActiveID == id {
		s.data.ActiveID = ""
	}
	return s.save()
}

// SetActive 将指定 ID 的账号设为活跃账号，并更新其 LastUsed 时间。
func (s *AccountStore) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i := range s.data.Accounts {
		if s.data.Accounts[i].ID == id {
			s.data.Accounts[i].LastUsed = time.Now()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("账号 %s 不存在", id)
	}
	s.data.ActiveID = id
	return s.save()
}

// GetByID 根据 ID 查找账号。
func (s *AccountStore) GetByID(id string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, acc := range s.data.Accounts {
		if acc.ID == id {
			return acc, true
		}
	}
	return Account{}, false
}

// UpdateLabel 更新指定账号的标签。
func (s *AccountStore) UpdateLabel(id, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Accounts {
		if s.data.Accounts[i].ID == id {
			s.data.Accounts[i].Label = label
			return s.save()
		}
	}
	return fmt.Errorf("账号 %s 不存在", id)
}

// DetectAccounts 扫描 srcPath 的父目录及常见微信数据目录，
// 查找以 wxid_ 开头的子目录，返回检测到的账号列表。
func DetectAccounts(srcPath string) []Account {
	var result []Account
	seen := make(map[string]bool)

	scanDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.HasPrefix(entry.Name(), "wxid_") {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			if seen[full] {
				continue
			}
			seen[full] = true
			result = append(result, Account{
				ID:    "",
				Path:  full,
				Label: entry.Name(),
			})
		}
	}

	// 1. 扫描 srcPath 的父目录
	if srcPath != "" {
		scanDir(filepath.Dir(srcPath))
	}

	// 2. 扫描常见微信数据目录
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		scanDir(filepath.Join(userProfile, "Documents", "WeChat Files"))
		scanDir(filepath.Join(userProfile, "Documents", "xwechat_files"))
	}

	return result
}

// ListAccounts 返回所有已注册账号、当前活跃账号 ID，以及自动检测到的账号。
func (a *API) ListAccounts(c *gin.Context) {
	accounts, activeID := a.Accounts.List()

	srcPath := viper.GetString("WECHAT_DB_SRC_PATH")
	detected := DetectAccounts(srcPath)

	transport.SendSuccess(c, gin.H{
		"accounts":  accounts,
		"active_id": activeID,
		"detected":  detected,
	})
}

// AddAccount 将新账号添加到账号列表中。
func (a *API) AddAccount(c *gin.Context) {
	var body struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.BadRequest(c, "请求体解析失败: "+err.Error())
		return
	}
	if body.Path == "" {
		transport.BadRequest(c, "path 不能为空")
		return
	}
	if info, err := os.Stat(body.Path); err != nil || !info.IsDir() {
		transport.BadRequest(c, "路径不存在或不是目录: "+body.Path)
		return
	}
	acc, err := a.Accounts.Add(body.Path, body.Label)
	if err != nil {
		transport.InternalServerError(c, "保存账号失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, acc)
}

// DeleteAccount 删除指定 ID 的账号。
func (a *API) DeleteAccount(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		transport.BadRequest(c, "缺少账号 ID")
		return
	}
	if err := a.Accounts.Delete(id); err != nil {
		transport.InternalServerError(c, "删除账号失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"deleted": true})
}

// ActivateAccount 切换当前活跃账号，并在后台触发解密 + 重载。
func (a *API) ActivateAccount(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		transport.BadRequest(c, "缺少账号 ID")
		return
	}

	acc, ok := a.Accounts.GetByID(id)
	if !ok {
		transport.BadRequest(c, fmt.Sprintf("账号 %s 不存在", id))
		return
	}

	if err := a.Accounts.SetActive(id); err != nil {
		transport.InternalServerError(c, "切换活跃账号失败: "+err.Error())
		return
	}

	// 更新 viper 配置
	viper.Set("WECHAT_DB_SRC_PATH", acc.Path)
	if err := updateEnv(map[string]string{"WECHAT_DB_SRC_PATH": acc.Path}); err != nil {
		log.Error().Err(err).Msg("更新 .env 文件中的 WECHAT_DB_SRC_PATH 失败")
	}

	// 更新内存中的配置
	a.mu.Lock()
	a.Conf.WechatDbSrcPath = acc.Path
	a.mu.Unlock()

	// 后台执行解密 + 重载
	dbKey := viper.GetString("WECHAT_DB_KEY")
	go func() {
		activateMu.Lock()
		defer activateMu.Unlock()
		log.Info().Str("path", acc.Path).Msg("后台解密并重载数据...")
		if _, _, err := decrypt.RunTask(acc.Path, dbKey); err != nil {
			log.Error().Err(err).Str("path", acc.Path).Msg("切换账号后解密失败")
			return
		}
		if err := a.Store.Reload(); err != nil {
			log.Error().Err(err).Msg("切换账号后重载数据存储失败")
		}
	}()

	transport.SendSuccess(c, gin.H{
		"activated": true,
		"account":   acc,
	})
}

// UpdateAccountLabel 更新指定账号的显示标签。
func (a *API) UpdateAccountLabel(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		transport.BadRequest(c, "缺少账号 ID")
		return
	}

	var body struct {
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.BadRequest(c, "请求体解析失败: "+err.Error())
		return
	}

	if err := a.Accounts.UpdateLabel(id, body.Label); err != nil {
		transport.BadRequest(c, err.Error())
		return
	}

	transport.SendSuccess(c, gin.H{"updated": true})
}
