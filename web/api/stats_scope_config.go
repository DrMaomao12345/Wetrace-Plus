package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

// StatsScopeStore 持久化「统计范围」配置 —— 存到 data/stats_scope.json。
// 网页与移动端共用同一份。
type StatsScopeStore struct {
	mu    sync.Mutex
	path  string
	scope *model.StatsScope
}

func NewStatsScopeStore(dataDir string) *StatsScopeStore {
	s := &StatsScopeStore{
		path:  filepath.Join(dataDir, "stats_scope.json"),
		scope: model.DefaultStatsScope(),
	}
	s.load()
	return s
}

func (s *StatsScopeStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var scope model.StatsScope
	if json.Unmarshal(data, &scope) != nil {
		return
	}
	scope.Normalize()
	s.scope = &scope
}

// Get 返回当前配置（调用方只读，不要就地改）
func (s *StatsScopeStore) Get() *model.StatsScope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scope == nil {
		return model.DefaultStatsScope()
	}
	return s.scope
}

// Set 覆盖保存配置
func (s *StatsScopeStore) Set(scope *model.StatsScope) {
	if scope == nil {
		scope = model.DefaultStatsScope()
	}
	scope.Normalize()

	s.mu.Lock()
	s.scope = scope
	data, _ := json.MarshalIndent(scope, "", "  ")
	path := s.path
	s.mu.Unlock()

	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}
