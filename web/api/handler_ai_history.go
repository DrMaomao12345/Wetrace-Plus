package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

type SummaryHistoryItem struct {
	ID         string    `json:"id"`
	Talker     string    `json:"talker"`
	TimeRange  string    `json:"time_range"`
	PromptUsed string    `json:"prompt_used"`
	Summary    string    `json:"summary"`
	Model      string    `json:"model"` // 生成该总结所用的 AI 模型
	MsgCount   int       `json:"msg_count"`
	Status     string    `json:"status"` // "success" | "failed" | "cancelled"
	Error      string    `json:"error"`
	RetryCount int       `json:"retry_count"` // 当前是第几次重试（0=首次）
	RetryOf    string    `json:"retry_of"`    // 重试的原始失败记录 ID
	CreatedAt  time.Time `json:"created_at"`
}

var (
	summaryHistoryFilePath string
	summaryHistoryMu       sync.Mutex
)

func initSummaryHistoryFilePath(dataDir string) {
	summaryHistoryFilePath = filepath.Join(dataDir, "ai_summary_history.json")
}

func loadSummaryHistory() ([]SummaryHistoryItem, error) {
	data, err := os.ReadFile(summaryHistoryFilePath)
	if os.IsNotExist(err) {
		return []SummaryHistoryItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []SummaryHistoryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func saveSummaryHistory(items []SummaryHistoryItem) error {
	dir := filepath.Dir(summaryHistoryFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(summaryHistoryFilePath, data, 0o600)
}

func appendSummaryHistory(item SummaryHistoryItem) {
	summaryHistoryMu.Lock()
	defer summaryHistoryMu.Unlock()

	items, _ := loadSummaryHistory()
	items = append([]SummaryHistoryItem{item}, items...)
	if len(items) > 500 {
		items = items[:500]
	}
	_ = saveSummaryHistory(items)
}

// getRetryCount looks up the retry chain to determine the current retry count.
func getRetryCount(retryOf string) int {
	if retryOf == "" {
		return 0
	}
	items, err := loadSummaryHistory()
	if err != nil {
		return 0
	}
	for _, it := range items {
		if it.ID == retryOf {
			return it.RetryCount + 1
		}
	}
	return 0
}

// GetSummaryHistory 获取 AI 总结历史记录（含进行中的任务）
func (a *API) GetSummaryHistory(c *gin.Context) {
	summaryHistoryMu.Lock()
	items, err := loadSummaryHistory()
	summaryHistoryMu.Unlock()

	if err != nil {
		transport.InternalServerError(c, "读取历史记录失败: "+err.Error())
		return
	}

	// 如果有正在进行的总结任务，插到最前面
	a.mu.Lock()
	running := a.currentSummaryJob
	a.mu.Unlock()
	if running != nil {
		items = append([]SummaryHistoryItem{*running}, items...)
	}

	transport.SendSuccess(c, items)
}

// DeleteSummaryHistory 删除单条历史记录
func (a *API) DeleteSummaryHistory(c *gin.Context) {
	id := c.Param("id")

	summaryHistoryMu.Lock()
	defer summaryHistoryMu.Unlock()

	items, err := loadSummaryHistory()
	if err != nil {
		transport.InternalServerError(c, "读取历史记录失败: "+err.Error())
		return
	}
	filtered := items[:0]
	for _, it := range items {
		if it.ID != id {
			filtered = append(filtered, it)
		}
	}
	if err := saveSummaryHistory(filtered); err != nil {
		transport.InternalServerError(c, "删除失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func newHistoryID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
