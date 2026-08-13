package transcripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store 语音转文字缓存，持久化到 JSON 文件
type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]string // voice_id → text
}

// NewStore 从 dataDir 加载或创建转文字缓存
func NewStore(dataDir string) (*Store, error) {
	s := &Store{
		path: filepath.Join(dataDir, "voice_transcripts.json"),
		data: make(map[string]string),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Get 返回已缓存的转文字结果，未找到时返回 ("", false)
func (s *Store) Get(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	text, ok := s.data[id]
	return text, ok
}

// Len 返回已缓存的转写条数。统计侧用它判断「是否值得为语音字数多跑一趟查询」。
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// Set 保存转文字结果并持久化
func (s *Store) Set(id, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = text
	return s.save()
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.data)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0644)
}
