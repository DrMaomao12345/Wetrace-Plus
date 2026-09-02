package model

// ── 功能9 日历热力图 ─────────────────────────────
type DayHeat struct {
	Date  string `json:"date"` // YYYY-MM-DD（按用户时区）
	Count int    `json:"count"`
}

// ── 功能3 双向互动比 ─────────────────────────────
type InteractionRatio struct {
	Talker           string `json:"talker"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	SentCount        int    `json:"sent_count"`        // 你发的条数
	RecvCount        int    `json:"recv_count"`        // 对方发的条数
	MyInitiations    int    `json:"my_initiations"`    // 你主动发起的对话次数
	TheirInitiations int    `json:"their_initiations"` // 对方主动发起的对话次数
	Total            int    `json:"total"`
	LastTime         int64  `json:"last_time"`
}

// ── 功能5 回复速度分析 ───────────────────────────
type ReplySpeed struct {
	Talker           string `json:"talker"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	MyAvgReplySec    int    `json:"my_avg_reply_sec"`    // 你回复对方的平均秒数
	MyMedianReplySec int    `json:"my_median_reply_sec"` // 你回复对方的中位数秒
	MyFastestSec     int    `json:"my_fastest_sec"`
	MySlowestSec     int    `json:"my_slowest_sec"`
	MyReplyCount     int    `json:"my_reply_count"`
	TheirAvgReplySec int    `json:"their_avg_reply_sec"` // 对方回复你的平均秒数
	TheirReplyCount  int    `json:"their_reply_count"`
	LateNightInstant int    `json:"late_night_instant"` // 深夜(0-5点)秒回(<2分钟)次数
	IgnoredByThem    int    `json:"ignored_by_them"`    // 你发起后对方 >6h 未回（近似"已读不回"）
}

// ── 功能7 年度对比 ───────────────────────────────
type ContactYearDelta struct {
	Talker  string `json:"talker"`
	Name    string `json:"name"`
	Avatar  string `json:"avatar"`
	IsGroup bool   `json:"is_group"`
	CountA  int    `json:"count_a"`
	CountB  int    `json:"count_b"`
	Delta   int    `json:"delta"`
}

type YearCompare struct {
	YearA       int                 `json:"year_a"`
	YearB       int                 `json:"year_b"`
	OverviewA   *AnnualOverview     `json:"overview_a"`
	OverviewB   *AnnualOverview     `json:"overview_b"`
	FadedOut    []*ContactYearDelta `json:"faded_out"`    // A 有、B 几乎消失
	NewlyActive []*ContactYearDelta `json:"newly_active"` // B 新出现
	Rising      []*ContactYearDelta `json:"rising"`       // 互动上升 Top
	Falling     []*ContactYearDelta `json:"falling"`      // 互动下降 Top
}

// ── 功能4 共同群聊 ───────────────────────────────
type CommonGroup struct {
	Username    string `json:"username"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	MemberCount int    `json:"member_count"`
}

// ── 日历热力图：月份下钻 ────────────────────────────────────────
// MonthPartner 某个月里和某人的互动概况。
// Days 是**有互动的天数**（去重后的日历天），不是消息条数 ——
// 「聊了多少天」比「发了多少条」更能反映这段时间的陪伴密度。
type MonthPartner struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	IsGroup  bool   `json:"is_group"`
	Days     int    `json:"days"`     // 该月有互动的天数
	Messages int    `json:"messages"` // 该月消息条数
}

// MonthPartners 某个月的下钻结果。
type MonthPartners struct {
	Year int `json:"year"`
	// Day <= 0 表示整月；>= 1 表示只统计那一天
	Month      int             `json:"month"`
	Day        int             `json:"day"`
	TotalDays  int             `json:"total_days"`  // 该月有记录的天数（并集）
	TotalMsgs  int             `json:"total_msgs"`  // 该月消息总数
	TotalPeers int             `json:"total_peers"` // 该月互动过的会话总数（未被 limit 截断）
	Partners   []*MonthPartner `json:"partners"`
}

// ── 今日报告 ───────────────────────────────────────────────────

// DailyPartner 当天与某个会话的往来概况
type DailyPartner struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	IsGroup  bool   `json:"is_group"`
	Messages int    `json:"messages"`
	Sent     int    `json:"sent"`
	Recv     int    `json:"recv"`
	LastTime int64  `json:"last_time"`
	// FirstBySelf 当天这个会话的第一条是不是我发的（谁先开的口）
	FirstBySelf bool `json:"first_by_self"`
}

// DailyTypeCount 当天的消息类型分布
type DailyTypeCount struct {
	Type  int    `json:"type"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// DailyReport 今日报告
type DailyReport struct {
	Date          string `json:"date"`
	TotalMessages int    `json:"total_messages"`
	SentMessages  int    `json:"sent_messages"`
	RecvMessages  int    `json:"recv_messages"`
	// 当天有往来的私聊数 / 群聊数
	ActivePeers  int `json:"active_peers"`
	ActiveGroups int `json:"active_groups"`
	// TotalPeers 是截断前的真实会话数，Partners 可能被 topN 截断
	TotalPeers int `json:"total_peers"`
	// 当天第一条 / 最后一条消息的 unix 秒
	FirstTime int64             `json:"first_time"`
	LastTime  int64             `json:"last_time"`
	Hourly    []int             `json:"hourly"` // 24 个桶
	Partners  []*DailyPartner   `json:"partners"`
	Types     []*DailyTypeCount `json:"types"`

	// 字数。口径与年度字数统计一致：文本算 message_content 长度（仅 local_type=1），
	// 语音算转写文本的字数。VoiceChars 已含在 Sent/RecvChars 里，不要重复相加。
	SentChars  int `json:"sent_chars"`
	RecvChars  int `json:"recv_chars"`
	VoiceChars int `json:"voice_chars"`

	// 峰值时段
	PeakHour      int `json:"peak_hour"`
	PeakHourCount int `json:"peak_hour_count"`

	// 谁先开口：当天每个会话的第一条消息由谁发出，汇总而来
	InitiatedByMe   int `json:"initiated_by_me"`
	InitiatedByThem int `json:"initiated_by_them"`

	// 对比与连贯性
	PrevDayTotal  int `json:"prev_day_total"`  // 前一天总条数
	LastWeekTotal int `json:"last_week_total"` // 上周同一天总条数
	StreakDays    int `json:"streak_days"`     // 截至当天的连续有记录天数
	// StreakCapped 为真表示连续天数顶到了回看窗口上限，实际可能更长（显示成 "N+"）
	StreakCapped bool `json:"streak_capped"`
}
