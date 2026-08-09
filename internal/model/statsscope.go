package model

// StatsModule 是可以单独覆盖统计范围的统计模块。
type StatsModule string

const (
	ModuleReport    StatsModule = "report"    // 年度报告
	ModuleInsights  StatsModule = "insights"  // 关系洞察
	ModuleGalaxy    StatsModule = "galaxy"    // 关系星图
	ModuleDashboard StatsModule = "dashboard" // 仪表盘 / 首页
	ModuleWordCloud StatsModule = "wordcloud" // 词云
	ModuleReminder  StatsModule = "reminder"  // 联系提醒
	ModuleBiz       StatsModule = "biz"       // 公众号订阅画像
)

// AllStatsModules 是全部可覆盖模块，顺序即前端展示顺序。
var AllStatsModules = []StatsModule{
	ModuleReport, ModuleInsights, ModuleGalaxy,
	ModuleDashboard, ModuleWordCloud, ModuleReminder, ModuleBiz,
}

var statsModuleLabels = map[StatsModule]string{
	ModuleReport:    "年度报告",
	ModuleInsights:  "关系洞察",
	ModuleGalaxy:    "关系星图",
	ModuleDashboard: "仪表盘",
	ModuleWordCloud: "词云",
	ModuleReminder:  "联系提醒",
	ModuleBiz:       "公众号画像",
}

// Label 返回模块中文名
func (m StatsModule) Label() string { return statsModuleLabels[m] }

// Valid 判断是否是已知模块
func (m StatsModule) Valid() bool {
	_, ok := statsModuleLabels[m]
	return ok
}

// StatsScope 描述「哪些类型的会话参与统计」。
//   - Global 是全局默认
//   - Modules 里出现的模块用自己的开关覆盖全局；没出现的模块继承全局
//   - Overrides 是对单个会话手动指定的类型标签，优先于自动分类
type StatsScope struct {
	Global    map[TalkerType]bool                `json:"global"`
	Modules   map[StatsModule]map[TalkerType]bool `json:"modules,omitempty"`
	Overrides map[string]TalkerType              `json:"overrides,omitempty"`
}

// DefaultStatsScope 默认全部类型都参与统计 —— 与本功能上线前的行为一致，
// 用户可在设置里主动收窄（或用「只统计真人 + 群聊」预设一键收窄）。
func DefaultStatsScope() *StatsScope {
	global := make(map[TalkerType]bool, len(AllTalkerTypes))
	for _, t := range AllTalkerTypes {
		global[t] = true
	}
	return &StatsScope{
		Global:    global,
		Modules:   map[StatsModule]map[TalkerType]bool{},
		Overrides: map[string]TalkerType{},
	}
}

// Normalize 补全缺失字段、丢弃未知的类型与模块，保证后续读取安全。
func (s *StatsScope) Normalize() {
	if s == nil {
		return
	}
	if s.Global == nil {
		s.Global = map[TalkerType]bool{}
	}
	for _, t := range AllTalkerTypes {
		if _, ok := s.Global[t]; !ok {
			s.Global[t] = true
		}
	}
	for t := range s.Global {
		if !t.Valid() {
			delete(s.Global, t)
		}
	}

	if s.Modules == nil {
		s.Modules = map[StatsModule]map[TalkerType]bool{}
	}
	for m, types := range s.Modules {
		if !m.Valid() || types == nil {
			delete(s.Modules, m)
			continue
		}
		for t := range types {
			if !t.Valid() {
				delete(types, t)
			}
		}
		for _, t := range AllTalkerTypes {
			if _, ok := types[t]; !ok {
				types[t] = false
			}
		}
	}

	if s.Overrides == nil {
		s.Overrides = map[string]TalkerType{}
	}
	for talker, t := range s.Overrides {
		if talker == "" || !t.Valid() {
			delete(s.Overrides, talker)
		}
	}
}

// EffectiveTypes 返回某模块实际生效的类型开关（模块覆盖优先，否则继承全局）。
func (s *StatsScope) EffectiveTypes(m StatsModule) map[TalkerType]bool {
	if s == nil {
		return DefaultStatsScope().Global
	}
	if types, ok := s.Modules[m]; ok && types != nil {
		return types
	}
	return s.Global
}

// AllowType 判断某类型在某模块下是否参与统计。
func (s *StatsScope) AllowType(m StatsModule, t TalkerType) bool {
	types := s.EffectiveTypes(m)
	if types == nil {
		return true
	}
	allowed, ok := types[t]
	if !ok {
		return true
	}
	return allowed
}

// AllTypesOn 判断某模块是否所有类型都参与统计（此时可以跳过一切过滤开销）。
func (s *StatsScope) AllTypesOn(m StatsModule) bool {
	types := s.EffectiveTypes(m)
	for _, t := range AllTalkerTypes {
		if allowed, ok := types[t]; ok && !allowed {
			return false
		}
	}
	return true
}

// OverrideFor 返回某会话的手动类型覆盖（第二个返回值表示是否存在覆盖）。
func (s *StatsScope) OverrideFor(talker string) (TalkerType, bool) {
	if s == nil || s.Overrides == nil {
		return "", false
	}
	t, ok := s.Overrides[talker]
	return t, ok
}
