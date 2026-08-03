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

	RelationshipDepth  int    `json:"relationship_depth"`  // 历史关系深度 0-100
	CurrentTemperature int    `json:"current_temperature"` // 当前关系温度 0-100
	ContinuityScore    int    `json:"continuity_score"`    // 陪伴持续性 0-100
	ReciprocityScore   int    `json:"reciprocity_score"`   // 双向交流 0-100
	CompositeIntimacy  int    `json:"composite_intimacy"`  // 综合亲密度 0-100
	Confidence         int    `json:"confidence"`          // 置信度 0-100
	RecentActivity     int    `json:"recent_activity"`     // 近期活跃度 0-100(控制脉动)
	Status             string `json:"status"`              // active/low_freq/faded/resumed

	// 布局(后端算好,前端只读,保证空间稳定 §33.5)
	TargetAngle  float64 `json:"target_angle"`
	TargetRadius float64 `json:"target_radius"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`

	// 统计事实(详情卡 §9 用)
	TotalMessages int      `json:"total_messages"`
	SentMessages  int      `json:"sent_messages"`
	RecvMessages  int      `json:"recv_messages"`
	FirstTime     int64    `json:"first_time"`
	LastTime      int64    `json:"last_time"`
	ActiveMonths  int      `json:"active_months"`
	Evidence      []string `json:"evidence"` // 分析依据(可解释 §33.3)

	ManualImportant bool `json:"manual_important"`
	Excluded        bool `json:"excluded"`
}

// ── 星图整体结果(缓存到 relationship_graph.json) ──────────────
type RelationshipGraph struct {
	GeneratedAt string          `json:"generated_at"`
	Version     int             `json:"version"`
	TzOffsetMin int             `json:"tz_offset_min"`
	Profile     *UserProfile    `json:"profile"`
	Nodes       []*GalaxyNode   `json:"nodes"`
	TotalCount  int             `json:"total_count"` // 参与评分的联系人总数(未截断前)
	Timeline    *GalaxyTimeline `json:"timeline,omitempty"`
}

// ── 功能11 陪伴时间轴(§10-12) ─────────────────────────────────
type GalaxyMonthSegment struct {
	Month        string `json:"month"` // YYYY-MM
	Strength     int    `json:"strength"`
	MessageCount int    `json:"message_count"`
}

type GalaxyContactTimeline struct {
	ContactID   string               `json:"contact_id"`
	DisplayName string               `json:"display_name"`
	Avatar      string               `json:"avatar"`
	Type        string               `json:"relationship_type"`
	Segments    []GalaxyMonthSegment `json:"segments"` // 仅含有有效互动的月份(§11.1)
	FirstMonth  string               `json:"first_month"`
	LastMonth   string               `json:"last_month"`
}

type GalaxyGlobalHeat struct {
	Month               string `json:"month"`
	HeatScore           int    `json:"heat_score"` // 0-100
	ActiveRelationships int    `json:"active_relationships"`
	StrongRelationships int    `json:"strong_relationships"`
	MessageCount        int    `json:"message_count"`
}

type GalaxyTimeline struct {
	Granularity string                   `json:"granularity"` // month
	Months      []string                 `json:"months"`      // 连续月份序列 YYYY-MM
	Contacts    []*GalaxyContactTimeline `json:"contacts"`
	GlobalHeat  []*GalaxyGlobalHeat      `json:"global_heat"`
}

// ── Phase4 用户手动修正(§28,存 overrides.json,读取图时套用,重建不丢) ──
type ContactOverride struct {
	RelationshipType  string `json:"relationship_type,omitempty"`
	RelationshipLabel string `json:"relationship_label,omitempty"`
	MainLifeStage     string `json:"main_life_stage,omitempty"`
	ManualImportant   bool   `json:"manual_important,omitempty"`
	Hidden            bool   `json:"hidden,omitempty"`
}

type GalaxyOverrides struct {
	Contacts map[string]*ContactOverride `json:"contacts"`
}

// ── §30 增量更新:缓存每人原始特征(raw_features.json)以便只重扫有新消息的人 ──
type GalaxyRawFeature struct {
	ID       string         `json:"id"`
	Remark   string         `json:"remark,omitempty"`
	Nick     string         `json:"nick,omitempty"`
	Avatar   string         `json:"avatar,omitempty"`
	Total    int            `json:"total"`
	Sent     int            `json:"sent"`
	Recv     int            `json:"recv"`
	First    int64          `json:"first"`
	Last     int64          `json:"last"`
	Days     int            `json:"days"`
	Months   map[string]int `json:"months"`
	Sessions int            `json:"sessions"`
	R30      int            `json:"r30"`
	R90      int            `json:"r90"`
	R365     int            `json:"r365"`
}
