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
	Year       int             `json:"year"`
	Month      int             `json:"month"`
	TotalDays  int             `json:"total_days"`  // 该月有记录的天数（并集）
	TotalMsgs  int             `json:"total_msgs"`  // 该月消息总数
	TotalPeers int             `json:"total_peers"` // 该月互动过的会话总数（未被 limit 截断）
	Partners   []*MonthPartner `json:"partners"`
}
