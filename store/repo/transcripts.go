package repo

import "sync"

// TranscriptLookup 是语音转写文本的只读查询接口。
// 转写结果由 web 层的 transcripts.Store 持有并落盘，这里只需要能按语音 id 查文本，
// 因此用最小接口接进来，避免 store 层反向依赖 web 层。
type TranscriptLookup interface {
	Get(id string) (string, bool)
	Len() int
}

type transcriptState struct {
	mu sync.RWMutex
	tl TranscriptLookup
}

// SetTranscripts 注入语音转写查询表（传 nil 表示未启用）
func (r *Repository) SetTranscripts(t TranscriptLookup) {
	r.transcripts.mu.Lock()
	r.transcripts.tl = t
	r.transcripts.mu.Unlock()
}

// transcriptLookup 取当前转写表；没有任何转写结果时返回 nil，
// 调用方据此整段跳过语音字数统计，不产生额外查询。
func (r *Repository) transcriptLookup() TranscriptLookup {
	r.transcripts.mu.RLock()
	tl := r.transcripts.tl
	r.transcripts.mu.RUnlock()
	if tl == nil || tl.Len() == 0 {
		return nil
	}
	return tl
}
