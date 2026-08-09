package model

// ── 公众号订阅画像 ────────────────────────────────────────────
// 数据来自 data/message/biz_message_*.db —— 与普通聊天消息分开存放的
// 公众号推送库。这些消息不计入任何聊天统计，只在本模块使用。

// BizAccount 是单个公众号在统计区间内的表现
type BizAccount struct {
	Talker       string  `json:"talker"` // gh_xxx
	Name         string  `json:"name"`
	Avatar       string  `json:"avatar"`
	Type         string  `json:"type"`          // subscription 订阅号 / service 服务号
	TypeLabel    string  `json:"type_label"`    // 中文名
	PushCount    int     `json:"push_count"`    // 区间内推送条数
	FirstTime    int64   `json:"first_time"`    // 区间内首条
	LastTime     int64   `json:"last_time"`     // 区间内末条
	ActiveMonths int     `json:"active_months"` // 区间内有推送的月份数
	MonthlyAvg   float64 `json:"monthly_avg"`   // 活跃月均推送
	LifetimeLast int64   `json:"lifetime_last"` // 全历史最后一次推送(用于沉默判定)
}

// BizOverview 是订阅画像的总览数字
type BizOverview struct {
	FollowedTotal   int     `json:"followed_total"`   // 关注的公众号总数
	PushingAccounts int     `json:"pushing_accounts"` // 区间内有推送的
	SilentAccounts  int     `json:"silent_accounts"`  // 区间内零推送的
	TotalPushes     int     `json:"total_pushes"`     // 区间内推送总条数
	DailyAvg        float64 `json:"daily_avg"`        // 日均收到几条
	PeakHour        int     `json:"peak_hour"`        // 推送最集中的小时
	BusiestDate     string  `json:"busiest_date"`     // 推送最多的一天
	BusiestCount    int     `json:"busiest_count"`
	SubscriptionCnt int     `json:"subscription_cnt"` // 订阅号推送条数
	ServiceCnt      int     `json:"service_cnt"`      // 服务号推送条数
}

// BizMonthStat 月度推送量
type BizMonthStat struct {
	Month string `json:"month"` // YYYY-MM
	Count int    `json:"count"`
}

// BizHourStat 小时分布
type BizHourStat struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

// BizProfile 是订阅画像的完整结果
type BizProfile struct {
	Year          int            `json:"year"`
	Overview      BizOverview    `json:"overview"`
	TopAccounts   []*BizAccount  `json:"top_accounts"`   // 推送最多的
	SilentTop     []*BizAccount  `json:"silent_top"`     // 沉默订阅(区间内零推送),按最后一次推送时间倒序
	Monthly       []*BizMonthStat `json:"monthly"`
	Hourly        []*BizHourStat `json:"hourly"`
	TitleKeywords []*BizKeyword  `json:"title_keywords"` // 推文标题高频词
	HasData       bool           `json:"has_data"`       // 是否找到了 biz 消息库
}

// BizKeyword 标题高频词
type BizKeyword struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}
