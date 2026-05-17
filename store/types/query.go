package types

import "time"

// ProgressCallback 报告生成进度回调
// step: 当前步骤名, current: 当前第几步(1-based), total: 总步数, data: 该步骤产出的部分数据
type ProgressCallback func(step string, current int, total int, data interface{})

// TZSegment 表示一个带时区的时间段，用于年度报告的分段时区查询
type TZSegment struct {
	Start    time.Time
	End      time.Time
	TZOffset int // 距 UTC 的偏移秒数，东正西负（例如 UTC+8 → 28800）
}

// MessageQuery 封装了查询消息的参数
type MessageQuery struct {
	StartTime time.Time
	EndTime   time.Time
	Talker    string // 多个对话者可以用逗号分隔
	Sender    string // 多个发送者可以用逗号分隔
	Keyword   string
	MsgType   int // 消息类型筛选，0 表示不限
	Limit     int
	Offset    int
	Reverse   bool // 是否按时间倒序排列
}

// ContactQuery 封装了查询联系人的参数
type ContactQuery struct {
	Keyword string
	Limit   int
	Offset  int
}

// ChatRoomQuery 封装了查询群聊的参数
type ChatRoomQuery struct {
	Keyword string
	Limit   int
	Offset  int
}

// SessionQuery 封装了查询会话的参数
type SessionQuery struct {
	Keyword string
	Limit   int
	Offset  int
}
