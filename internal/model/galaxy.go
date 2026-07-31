package model

// ── 用户档案:出生年月 + 人生阶段(§3/§29.3) ──────────────────
type LifeStage struct {
	Name   string `json:"name"`   // 小学/初中/高中/大学/工作…
	Start  string `json:"start"`  // YYYY-MM
	End    string `json:"end"`    // YYYY-MM,空=至今
	Manual bool   `json:"manual"` // 是否用户手改
}

type UserProfile struct {
	BirthYear      int         `json:"birth_year"`
	BirthMonth     int         `json:"birth_month"`
	LifeStages     []LifeStage `json:"life_stages"`
	CustomKeywords []string    `json:"custom_keywords,omitempty"` // 用户自定义身份词典(§17.2)
}

// ── 星图节点:一位联系人的完整关系画像 + 布局坐标(§29.4) ──────
type GalaxyNode struct {
	ContactID   string `json:"contact_id"` // wxid
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`

	RelationshipType  string `json:"relationship_type"`  // friend/family/teacher/classmate/work/online/stranger
	RelationshipLabel string `json:"relationship_label"` // 高中数学老师 等
	MainLifeStage     string `json:"main_life_stage"`
	LifeStageCount    int    `json:"life_stage_count"`

	RelationshipDepth  int `json:"relationship_depth"`  // 历史关系深度 0-100
	CurrentTemperature int `json:"current_temperature"` // 当前关系温度 0-100
	ContinuityScore    int `json:"continuity_score"`    // 陪伴持续性 0-100
	ReciprocityScore   int `json:"reciprocity_score"`   // 双向交流 0-100
	CompositeIntimacy  int `json:"composite_intimacy"`  // 综合亲密度 0-100
	Confidence         int `json:"confidence"`          // 置信度 0-100
	RecentActivity     int `json:"recent_activity"`     // 近期活跃度 0-100(控制脉动)
	Status             string `json:"status"`           // active/low_freq/faded/resumed

	// 布局(后端算好,前端只读,保证空间稳定 §33.5)
	TargetAngle  float64 `json:"target_angle"`
	TargetRadius float64 `json:"target_radius"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`

	// 统计事实(详情卡 §9 用)
	TotalMessages int    `json:"total_messages"`
	SentMessages  int    `json:"sent_messages"`
	RecvMessages  int    `json:"recv_messages"`
	FirstTime     int64  `json:"first_time"`
	LastTime      int64  `json:"last_time"`
	ActiveMonths  int    `json:"active_months"`
	Evidence      []string `json:"evidence"` // 分析依据(可解释 §33.3)

	ManualImportant bool `json:"manual_important"`
	Excluded        bool `json:"excluded"`
}

// ── 星图整体结果(缓存到 relationship_graph.json) ──────────────
type RelationshipGraph struct {
	GeneratedAt string        `json:"generated_at"`
	Version     int           `json:"version"`
	TzOffsetMin int           `json:"tz_offset_min"`
	Profile     *UserProfile  `json:"profile"`
	Nodes       []*GalaxyNode `json:"nodes"`
	TotalCount  int           `json:"total_count"` // 参与评分的联系人总数(未截断前)
}
