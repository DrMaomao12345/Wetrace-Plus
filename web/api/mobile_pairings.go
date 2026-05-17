package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PairingRecord 一条移动端配对记录（一个设备/一次配对对应一条）
type PairingRecord struct {
	ID         string `json:"id"`
	Token      string `json:"token"`
	Label      string `json:"label"`        // 用户可改的备注名
	CreatedAt  int64  `json:"created_at"`   // 秒
	LastSeenAt int64  `json:"last_seen_at"` // 最后一次用此 token 访问的时间，0 = 从未
}

// MobilePairingStore 持久化所有配对记录到 data/mobile_pairings.json
type MobilePairingStore struct {
	mu      sync.RWMutex
	path    string
	records []PairingRecord
}

// NewMobilePairingStore 加载（或创建）配对记录文件
func NewMobilePairingStore(dataDir string) *MobilePairingStore {
	s := &MobilePairingStore{
		path: filepath.Join(dataDir, "mobile_pairings.json"),
	}
	s.load()
	return s
}

func (s *MobilePairingStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var recs []PairingRecord
	if json.Unmarshal(data, &recs) == nil {
		s.records = recs
	}
}

// save 必须在持有写锁时调用
func (s *MobilePairingStore) save() {
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0755)
	_ = os.WriteFile(s.path, data, 0644)
}

// List 返回所有配对记录（按创建时间倒序）
func (s *MobilePairingStore) List() []PairingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PairingRecord, len(s.records))
	copy(out, s.records)
	// 倒序：新的在前
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Create 新建一条配对记录
func (s *MobilePairingStore) Create(label string) (PairingRecord, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return PairingRecord{}, err
	}
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes)

	rec := PairingRecord{
		ID:        hex.EncodeToString(idBytes),
		Token:     hex.EncodeToString(b),
		Label:     label,
		CreatedAt: time.Now().Unix(),
	}
	s.mu.Lock()
	s.records = append(s.records, rec)
	s.save()
	s.mu.Unlock()
	return rec, nil
}

// Delete 按 ID 删除
func (s *MobilePairingStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// Rename 改备注名
func (s *MobilePairingStore) Rename(id, label string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].Label = label
			s.save()
			return true
		}
	}
	return false
}

// IsValidToken 校验 token 是否属于某条配对记录
func (s *MobilePairingStore) IsValidToken(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.records {
		if r.Token == token {
			return true
		}
	}
	return false
}

// Touch 更新某 token 的 last_seen（节流：同一分钟内不重复写盘）
func (s *MobilePairingStore) Touch(token string) {
	if token == "" {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].Token == token {
			if now-s.records[i].LastSeenAt >= 60 {
				s.records[i].LastSeenAt = now
				s.save()
			}
			return
		}
	}
}

// MigrateLegacyToken 把旧的单 token（viper MOBILE_API_TOKEN）迁移成一条记录
func (s *MobilePairingStore) MigrateLegacyToken(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 已存在则跳过
	for _, r := range s.records {
		if r.Token == token {
			return
		}
	}
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes)
	s.records = append(s.records, PairingRecord{
		ID:        hex.EncodeToString(idBytes),
		Token:     token,
		Label:     "（旧配对）",
		CreatedAt: time.Now().Unix(),
	})
	s.save()
}
