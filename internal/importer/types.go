package importer

import "time"

const (
	FormatChatlogKeeperJSON = "chatlog-keeper-json"
	FormatChatlogJSON       = "chatlog-json"
	FormatChatlogCSV        = "chatlog-csv"
	FormatMemoTraceCSV      = "memotrace-csv"
	FormatMemoTraceFullCSV  = "memotrace-full-csv"
	FormatWetracePlusJSON   = "wetrace-plus-json"
)

// Options contains identity hints that cannot always be recovered from older
// export formats. SelfName is especially useful for group-chat CSV exports.
type Options struct {
	SelfID   string `json:"self_id"`
	SelfName string `json:"self_name"`
}

// Upload is a temporary local copy of a file submitted through the import API.
// Name is the original user-facing filename; Path is removed by the API after
// ImportFiles returns.
type Upload struct {
	Name string
	Path string
	Size int64
}

// Message is the format-independent record emitted by every adapter.
type Message struct {
	ExternalID string
	Seq        int64
	Time       time.Time
	TalkerID   string
	TalkerName string
	IsChatRoom bool
	SenderID   string
	SenderName string
	IsSelf     bool
	Type       int64
	SubType    int64
	Content    string
	SourceFile string
	SourceRow  int
}

type Warning struct {
	File    string `json:"file"`
	Row     int    `json:"row,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SourceReport struct {
	File       string    `json:"file"`
	Format     string    `json:"format"`
	Messages   int       `json:"messages"`
	Imported   int       `json:"imported"`
	Duplicates int       `json:"duplicates"`
	Warnings   []Warning `json:"warnings,omitempty"`
}

type Result struct {
	Files         int            `json:"files"`
	Conversations int            `json:"conversations"`
	Messages      int            `json:"messages"`
	Imported      int            `json:"imported"`
	Duplicates    int            `json:"duplicates"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   time.Time      `json:"completed_at"`
	Sources       []SourceReport `json:"sources"`
	Warnings      []Warning      `json:"warnings,omitempty"`
}

type HistoryItem struct {
	ID           int64     `json:"id"`
	SourceName   string    `json:"source_name"`
	SourceHash   string    `json:"source_hash"`
	Formats      string    `json:"formats"`
	Messages     int       `json:"messages"`
	Imported     int       `json:"imported"`
	Duplicates   int       `json:"duplicates"`
	WarningCount int       `json:"warning_count"`
	ImportedAt   time.Time `json:"imported_at"`
}

type FormatInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Extensions  []string `json:"extensions"`
	Description string   `json:"description"`
	SelfHint    bool     `json:"self_hint"`
}

func SupportedFormats() []FormatInfo {
	return []FormatInfo{
		{
			ID:          FormatChatlogKeeperJSON,
			Name:        "chatlog-keeper 微信 JSON",
			Extensions:  []string{".json", ".zip"},
			Description: "兼容 chatlog-keeper 导出的 wechat_messages.json，保留会话、发送者、方向、类型与来源 ID。",
		},
		{
			ID:          FormatChatlogJSON,
			Name:        "chatlog JSON",
			Extensions:  []string{".json", ".zip"},
			Description: "兼容 chatlog /api/v1/chatlog?format=json 的消息数组。",
		},
		{
			ID:          FormatChatlogCSV,
			Name:        "chatlog CSV",
			Extensions:  []string{".csv", ".zip"},
			Description: "兼容 Time、SenderName、Sender、TalkerName、Talker、Content 表头。",
			SelfHint:    true,
		},
		{
			ID:          FormatMemoTraceCSV,
			Name:        "MemoTrace / WeChatMsg 会话 CSV",
			Extensions:  []string{".csv", ".zip"},
			Description: "兼容消息ID、类型、发送人、时间、内容等中文表头。",
			SelfHint:    true,
		},
		{
			ID:          FormatMemoTraceFullCSV,
			Name:        "MemoTrace / WeChatMsg 全量 CSV",
			Extensions:  []string{".csv", ".zip"},
			Description: "兼容 localId、TalkerId、Type、IsSender、CreateTime、StrContent 等全量表头。",
		},
		{
			ID:          FormatWetracePlusJSON,
			Name:        "Wetrace Plus 标准 JSON",
			Extensions:  []string{".json", ".zip"},
			Description: "稳定、公开的 Wetrace Plus 归档交换格式。",
		},
	}
}
