package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// SyncFunc is the function called to perform a sync operation.
type SyncFunc func() error

// SyncRecord 一次数据同步的历史记录。
type SyncRecord struct {
	ID     string `json:"id"`
	Time   string `json:"time"`   // RFC3339
	Status string `json:"status"` // "success" | "failed"
}

// maxSyncHistory 同步历史最多保留的条数。
const maxSyncHistory = 100

// Scheduler manages automatic sync scheduling.
type Scheduler struct {
	mu             sync.Mutex
	enabled        bool
	intervalMin    int
	lastSyncTime   time.Time
	lastSyncStatus string
	isSyncing      bool
	syncFunc       SyncFunc
	ticker         *time.Ticker
	stopCh         chan struct{}
	history        []SyncRecord
	historyFile    string
}

// NewScheduler creates a new sync scheduler.
func NewScheduler(syncFunc SyncFunc, historyFile string) *Scheduler {
	s := &Scheduler{
		syncFunc:       syncFunc,
		intervalMin:    30,
		lastSyncStatus: "",
		historyFile:    historyFile,
	}
	s.loadHistory()
	// 用历史里最新一条回填「上次同步时间」，重启后仍可显示
	if len(s.history) > 0 {
		if t, err := time.Parse(time.RFC3339, s.history[0].Time); err == nil {
			s.lastSyncTime = t
			s.lastSyncStatus = s.history[0].Status
		}
	}
	return s
}

// Status returns the current sync status.
type Status struct {
	Enabled        bool   `json:"enabled"`
	IntervalMin    int    `json:"interval_minutes"`
	LastSyncTime   string `json:"last_sync_time"`
	LastSyncStatus string `json:"last_sync_status"`
	IsSyncing      bool   `json:"is_syncing"`
}

// GetStatus returns the current scheduler status.
func (s *Scheduler) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	lastTime := ""
	if !s.lastSyncTime.IsZero() {
		lastTime = s.lastSyncTime.Format(time.RFC3339)
	}
	return Status{
		Enabled:        s.enabled,
		IntervalMin:    s.intervalMin,
		LastSyncTime:   lastTime,
		LastSyncStatus: s.lastSyncStatus,
		IsSyncing:      s.isSyncing,
	}
}

// GetHistory 返回同步历史（最新在前）。
func (s *Scheduler) GetHistory() []SyncRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SyncRecord, len(s.history))
	copy(out, s.history)
	return out
}

// Configure updates the scheduler settings and restarts if needed.
func (s *Scheduler) Configure(enabled bool, intervalMin int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.enabled = enabled
	if intervalMin >= 5 && intervalMin <= 1440 {
		s.intervalMin = intervalMin
	}

	s.stopTicker()
	if s.enabled {
		s.startTicker()
	}
}

func (s *Scheduler) stopTicker() {
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.stopCh)
		s.ticker = nil
		s.stopCh = nil
	}
}

func (s *Scheduler) startTicker() {
	s.ticker = time.NewTicker(time.Duration(s.intervalMin) * time.Minute)
	s.stopCh = make(chan struct{})

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.RunSync()
			case <-s.stopCh:
				return
			}
		}
	}()
	log.Info().Int("interval_minutes", s.intervalMin).Msg("auto sync scheduler started")
}

// StartSync marks the scheduler as syncing and returns true if successful.
func (s *Scheduler) StartSync() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isSyncing {
		return false
	}
	s.isSyncing = true
	return true
}

// RunSync executes a sync operation. Safe to call concurrently.
func (s *Scheduler) RunSync() {
	s.mu.Lock()
	if !s.isSyncing {
		s.isSyncing = true
	}
	s.mu.Unlock()

	log.Info().Msg("sync started")
	err := s.syncFunc()

	now := time.Now()
	record := SyncRecord{
		ID:   "sync_" + now.Format("20060102_150405"),
		Time: now.Format(time.RFC3339),
	}

	s.mu.Lock()
	s.isSyncing = false
	s.lastSyncTime = now
	if err != nil {
		s.lastSyncStatus = "failed"
		record.Status = "failed"
		log.Error().Err(err).Msg("sync failed")
	} else {
		s.lastSyncStatus = "success"
		record.Status = "success"
		log.Info().Msg("sync completed")
	}
	// 最新插到最前，超出上限裁剪
	s.history = append([]SyncRecord{record}, s.history...)
	if len(s.history) > maxSyncHistory {
		s.history = s.history[:maxSyncHistory]
	}
	s.mu.Unlock()

	s.saveHistory()
}

// Stop shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopTicker()
}

func (s *Scheduler) loadHistory() {
	if s.historyFile == "" {
		return
	}
	data, err := os.ReadFile(s.historyFile)
	if err != nil {
		return
	}
	var records []SyncRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return
	}
	s.history = records
}

func (s *Scheduler) saveHistory() {
	s.mu.Lock()
	records := make([]SyncRecord, len(s.history))
	copy(records, s.history)
	s.mu.Unlock()

	if s.historyFile == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.historyFile), 0755)

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal sync history")
		return
	}
	if err := os.WriteFile(s.historyFile, data, 0644); err != nil {
		log.Error().Err(err).Msg("failed to save sync history")
	}
}
