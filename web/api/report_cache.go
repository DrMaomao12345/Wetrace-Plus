package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

// ReportCache 按 dataVersion + 查询参数缓存年度报告。
//
// 设计要点：
//   - 数据指纹（GetDataVersion，path + size + mtime 的 md5）作为「整体版本号」。
//     版本号变化时整张缓存被清空 —— 既保证不会返回过期数据，也避免无用的旧
//     条目永远占内存。
//   - 同一份数据下：(year, talker, exclude, defaultTz, pastStartYear, segs) 任一
//     不同就是不同 key —— 通过 AnnualReportKey / TalkerReportKey 拼出来。
//   - 缓存命中直接返回内存里那份 *model.AnnualReport（不复制 —— 调用方只读返回，
//     gin 序列化时只读字段，不会就地修改）。
//
// 调用方：handler_report.go::GetAnnualReport，进入计算之前先 Get，
// 计算完成后 Put。
type ReportCache struct {
	mu          sync.RWMutex
	dataVersion string
	entries     map[string]*cachedReport
}

type cachedReport struct {
	report   *model.AnnualReport
	storedAt time.Time
}

func NewReportCache() *ReportCache {
	return &ReportCache{entries: make(map[string]*cachedReport)}
}

// Get 命中返回缓存的报告，否则返回 nil。version 不一致直接 miss。
func (c *ReportCache) Get(version, key string) *model.AnnualReport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dataVersion != version {
		return nil
	}
	if e, ok := c.entries[key]; ok {
		return e.report
	}
	return nil
}

// Put 写入缓存。version 变了清空旧条目再写入。
func (c *ReportCache) Put(version, key string, report *model.AnnualReport) {
	if report == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dataVersion != version {
		c.dataVersion = version
		c.entries = make(map[string]*cachedReport)
	}
	c.entries[key] = &cachedReport{report: report, storedAt: time.Now()}
}

// Invalidate 显式清空缓存（同步执行完、手动重生成时调用）。
func (c *ReportCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dataVersion = ""
	c.entries = make(map[string]*cachedReport)
}

// AnnualReportKey 「全局年度报告」缓存键。
func AnnualReportKey(year, defaultTzOffset, pastStartYear int,
	exclude []string, segs []TZSegmentRequest) string {
	sorted := append([]string(nil), exclude...)
	sort.Strings(sorted)
	var sb strings.Builder
	fmt.Fprintf(&sb, "g|y=%d|tz=%d|past=%d|ex=%s",
		year, defaultTzOffset, pastStartYear, strings.Join(sorted, ","))
	for _, s := range segs {
		fmt.Fprintf(&sb, "|seg=%s~%s@%d", s.StartDate, s.EndDate, s.TZOffset)
	}
	return sb.String()
}

// TalkerReportKey 「单联系人年度报告」缓存键。
func TalkerReportKey(year int, talker string, defaultTzOffset int) string {
	return fmt.Sprintf("t|y=%d|talker=%s|tz=%d", year, talker, defaultTzOffset)
}

// reportDataVersion 是年度报告缓存用的数据指纹。
//
// 除了消息库文件本身，还要把「已转写的语音条数」算进去 —— 语音转写的字数会计入
// 报告，但转写结果存在 voice_transcripts.json 里，不影响消息库的 mtime，
// 只用 Store.GetDataVersion() 的话转写完报告还是旧数字。
func (a *API) reportDataVersion() string {
	v := a.Store.GetDataVersion()
	if a.Transcripts != nil {
		v = fmt.Sprintf("%s|tx:%d", v, a.Transcripts.Len())
	}
	return v
}
