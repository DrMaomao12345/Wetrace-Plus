package api

import (
	"sort"
	"strings"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
)

type labeledOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// GetStatsScope 返回统计范围配置，以及前端渲染需要的类型 / 模块字典
func (a *API) GetStatsScope(c *gin.Context) {
	scope := model.DefaultStatsScope()
	if a.StatsScope != nil {
		scope = a.StatsScope.Get()
	}

	talkerTypes := make([]labeledOption, 0, len(model.AllTalkerTypes))
	for _, t := range model.AllTalkerTypes {
		talkerTypes = append(talkerTypes, labeledOption{Key: string(t), Label: t.Label()})
	}
	modules := make([]labeledOption, 0, len(model.AllStatsModules))
	for _, m := range model.AllStatsModules {
		modules = append(modules, labeledOption{Key: string(m), Label: m.Label()})
	}

	transport.SendSuccess(c, gin.H{
		"scope":        scope,
		"talker_types": talkerTypes,
		"modules":      modules,
	})
}

// UpdateStatsScope 覆盖保存统计范围配置，并让受影响的缓存失效
func (a *API) UpdateStatsScope(c *gin.Context) {
	var scope model.StatsScope
	if err := c.ShouldBindJSON(&scope); err != nil {
		transport.BadRequest(c, "无效的统计范围配置: "+err.Error())
		return
	}
	a.applyStatsScope(&scope)
	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// applyStatsScope 持久化配置 + 推给存储层 + 清掉按旧范围算出来的报告缓存
func (a *API) applyStatsScope(scope *model.StatsScope) {
	if a.StatsScope != nil {
		a.StatsScope.Set(scope)
		scope = a.StatsScope.Get()
	}
	if a.Store != nil {
		a.Store.SetStatsScope(scope)
	}
	if a.ReportCache != nil {
		a.ReportCache.Invalidate()
	}
}

type talkerTagItem struct {
	Talker    string           `json:"talker"`
	Name      string           `json:"name"`
	Auto      model.TalkerType `json:"auto"`
	Effective model.TalkerType `json:"effective"`
	Override  bool             `json:"override"`
}

// GetTalkerTags 返回会话列表及其类型标签（自动分类 + 手动覆盖），供设置页展示与搜索
func (a *API) GetTalkerTags(c *gin.Context) {
	ctx := c.Request.Context()
	keyword := strings.TrimSpace(strings.ToLower(c.Query("keyword")))
	typeFilter := strings.TrimSpace(c.Query("type"))

	sessions, err := a.Store.GetSessions(ctx, types.SessionQuery{Limit: 0})
	if err != nil {
		transport.InternalServerError(c, "获取会话列表失败")
		return
	}

	autoTypes := a.Store.TalkerTypes(ctx)
	scope := a.Store.StatsScope()

	items := make([]talkerTagItem, 0, len(sessions))
	counts := map[model.TalkerType]int{}

	for _, s := range sessions {
		auto, ok := autoTypes[s.UserName]
		if !ok {
			auto = model.ClassifyTalker(model.TalkerFacts{UserName: s.UserName})
		}
		effective := auto
		override := false
		if t, has := scope.OverrideFor(s.UserName); has {
			effective, override = t, true
		}
		counts[effective]++

		if typeFilter != "" && string(effective) != typeFilter {
			continue
		}
		name := s.NickName
		if name == "" {
			name = s.UserName
		}
		if keyword != "" &&
			!strings.Contains(strings.ToLower(name), keyword) &&
			!strings.Contains(strings.ToLower(s.UserName), keyword) {
			continue
		}

		items = append(items, talkerTagItem{
			Talker:    s.UserName,
			Name:      name,
			Auto:      auto,
			Effective: effective,
			Override:  override,
		})
	}

	// 有手动覆盖的排前面，方便用户复查自己改过什么
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Override != items[j].Override {
			return items[i].Override
		}
		return false
	})

	countList := make([]gin.H, 0, len(model.AllTalkerTypes))
	for _, t := range model.AllTalkerTypes {
		countList = append(countList, gin.H{"key": string(t), "label": t.Label(), "count": counts[t]})
	}

	transport.SendSuccess(c, gin.H{"items": items, "counts": countList})
}

// UpdateTalkerTag 设置或清除单个会话的手动类型覆盖。
// type 传空字符串表示「恢复自动分类」。
func (a *API) UpdateTalkerTag(c *gin.Context) {
	var req struct {
		Talker string `json:"talker"`
		Type   string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "无效的请求: "+err.Error())
		return
	}
	req.Talker = strings.TrimSpace(req.Talker)
	if req.Talker == "" {
		transport.BadRequest(c, "缺少会话 ID")
		return
	}

	tt := model.TalkerType(strings.TrimSpace(req.Type))
	if req.Type != "" && !tt.Valid() {
		transport.BadRequest(c, "未知的会话类型: "+req.Type)
		return
	}

	scope := a.Store.StatsScope()
	next := cloneStatsScope(scope)
	if req.Type == "" {
		delete(next.Overrides, req.Talker)
	} else {
		next.Overrides[req.Talker] = tt
	}
	a.applyStatsScope(next)

	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// cloneStatsScope 深拷贝一份配置，避免就地改动存储层正在读的那份
func cloneStatsScope(s *model.StatsScope) *model.StatsScope {
	out := model.DefaultStatsScope()
	if s == nil {
		return out
	}
	for k, v := range s.Global {
		out.Global[k] = v
	}
	out.Modules = map[model.StatsModule]map[model.TalkerType]bool{}
	for m, types := range s.Modules {
		cp := make(map[model.TalkerType]bool, len(types))
		for k, v := range types {
			cp[k] = v
		}
		out.Modules[m] = cp
	}
	out.Overrides = map[string]model.TalkerType{}
	for k, v := range s.Overrides {
		out.Overrides[k] = v
	}
	return out
}
