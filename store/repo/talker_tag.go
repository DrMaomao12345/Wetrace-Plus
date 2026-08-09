package repo

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// talkerTagTTL 是自动分类结果的缓存时长。联系人库变动不频繁，
// 几分钟的滞后换来统计查询里零额外 IO。
const talkerTagTTL = 5 * time.Minute

type talkerTagCache struct {
	mu       sync.RWMutex
	types    map[string]model.TalkerType
	md5Map   map[string]string // md5(talker) -> talker，供消息表名反查
	loadedAt time.Time
}

// scopeState 保存当前生效的统计范围配置（由 API 层在启动与配置变更时写入）。
type scopeState struct {
	mu    sync.RWMutex
	scope *model.StatsScope
}

// SetStatsScope 覆盖当前生效的统计范围配置
func (r *Repository) SetStatsScope(s *model.StatsScope) {
	if s == nil {
		s = model.DefaultStatsScope()
	}
	s.Normalize()
	r.scopeState.mu.Lock()
	r.scopeState.scope = s
	r.scopeState.mu.Unlock()
}

// StatsScope 返回当前生效的统计范围配置（永不为 nil）
func (r *Repository) StatsScope() *model.StatsScope {
	r.scopeState.mu.RLock()
	s := r.scopeState.scope
	r.scopeState.mu.RUnlock()
	if s == nil {
		return model.DefaultStatsScope()
	}
	return s
}

// InvalidateTalkerTags 丢弃自动分类缓存，下次读取时重新扫描联系人库
func (r *Repository) InvalidateTalkerTags() {
	r.tagCache.mu.Lock()
	r.tagCache.types = nil
	r.tagCache.md5Map = nil
	r.tagCache.mu.Unlock()
}

// TalkerTypes 返回 talker -> 自动分类类型 的全量映射（带缓存，不含手动覆盖）
func (r *Repository) TalkerTypes(ctx context.Context) map[string]model.TalkerType {
	r.tagCache.mu.RLock()
	cached := r.tagCache.types
	fresh := cached != nil && time.Since(r.tagCache.loadedAt) < talkerTagTTL
	r.tagCache.mu.RUnlock()
	if fresh {
		return cached
	}

	types := r.loadTalkerTypes(ctx)

	r.tagCache.mu.Lock()
	r.tagCache.types = types
	r.tagCache.loadedAt = time.Now()
	r.tagCache.mu.Unlock()
	return types
}

// TalkerTypeOf 返回单个会话的类型：手动覆盖优先，其次自动分类，
// 联系人库里查不到就退化为按用户名前缀判断。
func (r *Repository) TalkerTypeOf(ctx context.Context, talker string) model.TalkerType {
	if t, ok := r.StatsScope().OverrideFor(talker); ok {
		return t
	}
	if t, ok := r.TalkerTypes(ctx)[talker]; ok {
		return t
	}
	return model.ClassifyTalker(model.TalkerFacts{UserName: talker})
}

// AllowTalker 判断某会话在某统计模块下是否参与统计
func (r *Repository) AllowTalker(ctx context.Context, m model.StatsModule, talker string) bool {
	return r.StatsScope().AllowType(m, r.TalkerTypeOf(ctx, talker))
}

// TalkerFilter 返回一个针对某模块的过滤闭包，供需要在循环里反复判断的统计使用。
// 一次性解析出配置与分类表，避免每次调用都走锁。
func (r *Repository) TalkerFilter(ctx context.Context, m model.StatsModule) func(talker string) bool {
	scope := r.StatsScope()
	types := r.TalkerTypes(ctx)
	effective := scope.EffectiveTypes(m)

	// 全部类型都开启时直接返回恒真闭包，热路径零开销
	if scope.AllTypesOn(m) {
		return func(string) bool { return true }
	}

	return func(talker string) bool {
		tt, ok := scope.OverrideFor(talker)
		if !ok {
			if tt, ok = types[talker]; !ok {
				tt = model.ClassifyTalker(model.TalkerFacts{UserName: talker})
			}
		}
		if allowed, ok := effective[tt]; ok {
			return allowed
		}
		return true
	}
}

// TableFilter 返回一个按 v4 消息表名（Msg_<md5(talker)>）过滤的闭包。
// 表名反查不到会话时一律放行 —— 宁可多统计，也不要因为分类不出来就丢数据。
func (r *Repository) TableFilter(ctx context.Context, m model.StatsModule) func(tableName string) bool {
	if r.StatsScope().AllTypesOn(m) {
		// 全类型放行时省掉 md5 反查表的构建
		return func(string) bool { return true }
	}

	allow := r.TalkerFilter(ctx, m)
	md5Map := r.cachedTalkerMD5Map(ctx)
	return func(tableName string) bool {
		talker, ok := md5Map[strings.TrimPrefix(tableName, "Msg_")]
		if !ok {
			return true
		}
		return allow(talker)
	}
}

// cachedTalkerMD5Map 是 getTalkerMD5Map 的带缓存版本 —— 一次报告会调用多次过滤器，
// 不该每次都把整个会话列表读一遍。
func (r *Repository) cachedTalkerMD5Map(ctx context.Context) map[string]string {
	r.tagCache.mu.RLock()
	cached := r.tagCache.md5Map
	fresh := cached != nil && time.Since(r.tagCache.loadedAt) < talkerTagTTL
	r.tagCache.mu.RUnlock()
	if fresh {
		return cached
	}

	m := r.getTalkerMD5Map(ctx)
	r.tagCache.mu.Lock()
	r.tagCache.md5Map = m
	r.tagCache.mu.Unlock()
	return m
}

// loadTalkerTypes 扫描联系人库，为每个已知会话算出自动分类
func (r *Repository) loadTalkerTypes(ctx context.Context) map[string]model.TalkerType {
	types := make(map[string]model.TalkerType)

	dbPath, err := r.router.GetContactDBPath()
	if err != nil {
		return types
	}
	db, err := r.pool.GetConnection(dbPath)
	if err != nil {
		return types
	}

	var exists int
	_ = db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='contact'").Scan(&exists)
	if exists == 1 {
		r.loadV4TalkerTypes(ctx, db, types)
		return types
	}
	r.loadV3TalkerTypes(ctx, db, types)
	return types
}

func (r *Repository) loadV4TalkerTypes(ctx context.Context, db *sql.DB, out map[string]model.TalkerType) {
	// biz_info.type 是区分订阅号 / 服务号的唯一可靠来源
	bizTypes := map[string]int{}
	if rows, err := db.QueryContext(ctx, `SELECT username, type FROM biz_info`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var t int
			if rows.Scan(&name, &t) == nil {
				bizTypes[name] = t
			}
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT username, COALESCE(local_type,0), COALESCE(verify_flag,0) FROM contact`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var localType, verifyFlag int
		if err := rows.Scan(&name, &localType, &verifyFlag); err != nil {
			continue
		}
		facts := model.TalkerFacts{
			UserName:   name,
			LocalType:  localType,
			VerifyFlag: verifyFlag,
		}
		if bt, ok := bizTypes[name]; ok {
			facts.HasBizInfo = true
			facts.BizType = bt
		}
		out[name] = model.ClassifyTalker(facts)
	}
}

func (r *Repository) loadV3TalkerTypes(ctx context.Context, db *sql.DB, out map[string]model.TalkerType) {
	rows, err := db.QueryContext(ctx, `SELECT UserName, COALESCE(Type,0), COALESCE(VerifyFlag,0), COALESCE(Reserved1,0) FROM Contact`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var cType, verifyFlag, reserved1 int
		if err := rows.Scan(&name, &cType, &verifyFlag, &reserved1); err != nil {
			continue
		}
		out[name] = model.ClassifyTalker(model.TalkerFacts{
			UserName:   name,
			VerifyFlag: verifyFlag,
			IsFriend:   reserved1 == 1,
			KnownV3:    true,
		})
	}
}
