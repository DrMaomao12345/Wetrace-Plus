package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ExcludeConfigStore 持久化「排除 / 忽略的联系人」名单。
// 网页与移动端共用同一份 —— 存到 data/exclude_talkers.json。
type ExcludeConfigStore struct {
	mu      sync.Mutex
	path    string
	talkers []string
}

func NewExcludeConfigStore(dataDir string) *ExcludeConfigStore {
	s := &ExcludeConfigStore{path: filepath.Join(dataDir, "exclude_talkers.json")}
	s.load()
	return s
}

func (s *ExcludeConfigStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var list []string
	if json.Unmarshal(data, &list) == nil {
		s.talkers = list
	}
}

// Get 返回当前排除名单副本
func (s *ExcludeConfigStore) Get() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.talkers))
	copy(out, s.talkers)
	return out
}

// Set 覆盖保存排除名单
func (s *ExcludeConfigStore) Set(list []string) {
	if list == nil {
		list = []string{}
	}
	s.mu.Lock()
	s.talkers = list
	data, _ := json.MarshalIndent(list, "", "  ")
	path := s.path
	s.mu.Unlock()

	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}
