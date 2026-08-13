package repo

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/afumu/wetrace/store/bind"
	"github.com/afumu/wetrace/store/core"
)

// Repository 是数据访问层的入口，聚合了路由和连接池
type Repository struct {
	router *bind.TimelineRouter
	pool   *core.ConnectionPool

	tzMu       sync.RWMutex
	defaultTzM string // SQLite strftime 修饰符，例如 'localtime' 或 '+28800 seconds'

	tagCache    talkerTagCache  // 会话类型自动分类缓存
	scopeState  scopeState      // 当前生效的统计范围配置
	transcripts transcriptState // 语音转写文本查询表
}

// New 创建一个新的 Repository
func New(router *bind.TimelineRouter, pool *core.ConnectionPool) *Repository {
	return &Repository{
		router:     router,
		pool:       pool,
		defaultTzM: "'localtime'",
	}
}

// SetDefaultTzModifier 设置全局默认时区修饰符（被联系人侧分析查询使用）
// 例如 "'+28800 seconds'" 表示 UTC+8
func (r *Repository) SetDefaultTzModifier(mod string) {
	if mod == "" {
		mod = "'localtime'"
	}
	r.tzMu.Lock()
	r.defaultTzM = mod
	r.tzMu.Unlock()
}

// DefaultTzModifier 读当前修饰符
func (r *Repository) DefaultTzModifier() string {
	r.tzMu.RLock()
	defer r.tzMu.RUnlock()
	return r.defaultTzM
}

// GetDataVersion 返回当前所有消息 DB 文件的指纹（path + size + mtime 的 md5）。
// 数据库内容一变就会变，前端可用作判断缓存是否仍然有效。
func (r *Repository) GetDataVersion() string {
	shards := r.router.GetShards()
	parts := make([]string, 0, len(shards))
	for _, s := range shards {
		if info, err := os.Stat(s.FilePath); err == nil {
			parts = append(parts, fmt.Sprintf("%s|%d|%d", s.FilePath, info.Size(), info.ModTime().UnixNano()))
		}
	}
	sort.Strings(parts)
	joined := strings.Join(parts, "\n")
	sum := md5.Sum([]byte(joined))
	return hex.EncodeToString(sum[:])
}
