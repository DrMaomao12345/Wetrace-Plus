package model

// TZSegmentConfig 一段「这段日期我在这个时区」。
// 日期是该段**自身时区**下的自然日，闭区间。
type TZSegmentConfig struct {
	StartDate string `json:"start_date"` // 2025-09-10
	EndDate   string `json:"end_date"`   // 2025-12-21
	TZOffset  int    `json:"tz_offset"`  // 分钟，东正西负（UTC+8 → 480）
}

// TZConfig 是全站统一的时区口径：一个默认时区 + 若干时间分段。
//
// 以前这份配置分两处：设置页的「默认时区」存服务端、只管联系人统计；
// 年度报告的分段存浏览器 localStorage、只管报告。同一批消息在两个页面
// 会被切到不同的自然日里。现在合成一份，所有统计共用。
type TZConfig struct {
	DefaultOffset int               `json:"default_offset"` // 分钟
	Segments      []TZSegmentConfig `json:"segments"`
}
