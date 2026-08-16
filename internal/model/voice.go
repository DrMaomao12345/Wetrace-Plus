package model

// VoiceStats 语音消息统计（时长单位统一为毫秒，前端负责格式化）
type VoiceStats struct {
	TotalCount int `json:"total_count"`
	SentCount  int `json:"sent_count"`
	RecvCount  int `json:"recv_count"`

	TotalDurationMs int64 `json:"total_duration_ms"`
	SentDurationMs  int64 `json:"sent_duration_ms"`
	RecvDurationMs  int64 `json:"recv_duration_ms"`

	AvgDurationMs      int64 `json:"avg_duration_ms"`
	SentAvgDurationMs  int64 `json:"sent_avg_duration_ms"`
	RecvAvgDurationMs  int64 `json:"recv_avg_duration_ms"`
	LongestDurationMs  int64 `json:"longest_duration_ms"`
	LongestAt          int64 `json:"longest_at"`      // unix 秒
	LongestIsSelf      bool  `json:"longest_is_self"` // 最长那条是谁发的
	// WithoutDuration 是解析不出时长的条数（老消息或格式异常），
	// 单独记着，免得平均时长被静默拉偏而用户不知道
	WithoutDuration int `json:"without_duration"`

	// ── 语音转写字数（微信自带 + 本地 Whisper 补转，按 rune 计）──
	// TranscribedCount 是有转写文本的条数，与 TotalCount 一起看才知道覆盖率：
	// 本地没存音频文件的语音永远转不出来，只报字数会让人以为漏算了。
	TranscribedCount int `json:"transcribed_count"`
	TranscribedChars int `json:"transcribed_chars"`
	SentChars        int `json:"sent_chars"`
	RecvChars        int `json:"recv_chars"`
}

// AddTranscript 累加一条语音的转写字数。text 为空表示这条没转写出来，不计数。
func (v *VoiceStats) AddTranscript(isSelf bool, text string) {
	if text == "" {
		return
	}
	n := len([]rune(text))
	v.TranscribedCount++
	v.TranscribedChars += n
	if isSelf {
		v.SentChars += n
	} else {
		v.RecvChars += n
	}
}

// Add 累加一条语音消息
func (v *VoiceStats) Add(isSelf bool, durationMs int64, at int64) {
	v.TotalCount++
	if isSelf {
		v.SentCount++
	} else {
		v.RecvCount++
	}

	if durationMs <= 0 {
		v.WithoutDuration++
		return
	}

	v.TotalDurationMs += durationMs
	if isSelf {
		v.SentDurationMs += durationMs
	} else {
		v.RecvDurationMs += durationMs
	}
	if durationMs > v.LongestDurationMs {
		v.LongestDurationMs = durationMs
		v.LongestAt = at
		v.LongestIsSelf = isSelf
	}
}

// Finish 算出各项平均值。平均值只按「有时长」的条数算。
func (v *VoiceStats) Finish() {
	withDur := v.TotalCount - v.WithoutDuration
	if withDur > 0 {
		v.AvgDurationMs = v.TotalDurationMs / int64(withDur)
	}
	if v.SentCount > 0 && v.SentDurationMs > 0 {
		v.SentAvgDurationMs = v.SentDurationMs / int64(v.SentCount)
	}
	if v.RecvCount > 0 && v.RecvDurationMs > 0 {
		v.RecvAvgDurationMs = v.RecvDurationMs / int64(v.RecvCount)
	}
}

// TalkerExtras 是单个联系人的扩展分析：日历热力图 + 语音统计 + 关系洞察指标。
// 聊天页的数据分析面板和联系人年度报告共用。
type TalkerExtras struct {
	Year            int               `json:"year"`
	CalendarHeatmap []*DayHeat        `json:"calendar_heatmap"`
	Voice           *VoiceStats       `json:"voice"`
	Interaction     *InteractionRatio `json:"interaction"`
	ReplySpeed      *ReplySpeed       `json:"reply_speed"`
	// CallExcluded 是因为落在通话区间内而没有计入回复速度的间隔数量，
	// 让用户知道这个数字被修正过
	CallWindows int `json:"call_windows"`
}
