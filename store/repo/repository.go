package repo

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/DrMaomao12345/Wetrace-Plus/store/bind"
	"github.com/DrMaomao12345/Wetrace-Plus/store/core"
)

// Repository 是数据访问层的入口，聚合了路由和连接池
type Repository struct {
	router *bind.TimelineRouter
	pool   *core.ConnectionPool

	tzMu       sync.RWMutex
	defaultTzM string         // 无分段时的常量修饰符，例如 'localtime' 或 '+28800 seconds'
	tzCfg      model.TZConfig // 全站统一的时区口径（默认时区 + 分段）

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

// SetTZConfig 设置全站统一的时区口径。分段会被编译进 SQL 的 CASE 表达式，
// 所有按时区分桶的查询共用。
func (r *Repository) SetTZConfig(cfg model.TZConfig) {
	r.tzMu.Lock()
	r.tzCfg = cfg
	r.tzMu.Unlock()
}

// TZConfig 读当前时区配置。
func (r *Repository) TZConfig() model.TZConfig {
	r.tzMu.RLock()
	defer r.tzMu.RUnlock()
	return r.tzCfg
}

// TzModifier 返回可直接插进 strftime 的修饰符参数。
//
// timeExpr 必须是该表里「Unix 秒」的表达式 —— V4 是 create_time，
// V3 是 CreateTime/1000。没有分段时返回一个常量（和以前一样）；
// 有分段时返回 CASE 表达式，一条查询按行选时区。
func (r *Repository) TzModifier(timeExpr string) string {
	r.tzMu.RLock()
	cfg, fallback := r.tzCfg, r.defaultTzM
	r.tzMu.RUnlock()
	// 还没配过时区配置时，退回旧的常量修饰符（可能是 'localtime'）
	if cfg.DefaultOffset == 0 && len(cfg.Segments) == 0 {
		return fallback
	}
	return tzModifierSQL(cfg, timeExpr)
}

// TzModV4 / TzModV3 是两种表结构下的时间列快捷方式。
// V4 的 create_time 已经是秒，V3 的 CreateTime 是毫秒 —— 分段边界要和它比大小，
// 单位必须先对齐，否则 CASE 永远命中同一个分支。
func (r *Repository) TzModV4() string { return r.TzModifier("create_time") }
func (r *Repository) TzModV3() string { return r.TzModifier("CreateTime/1000") }

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
