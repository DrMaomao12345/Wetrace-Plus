package api

import (
	"os"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// identityTypeOrder 决定前端下拉里关系类型的展示顺序
var identityTypeOrder = []string{"family", "teacher", "classmate", "work", "service", "friend"}

// GetGalaxyIdentityRules GET /api/v1/galaxy/identity_rules
// 返回用户自定义身份规则 + 内置词典（只读，供对照）+ 可选的关系类型字典。
func (a *API) GetGalaxyIdentityRules(c *gin.Context) {
	var p model.UserProfile
	readJSONFile(galaxyPath("user_profile.json"), &p)

	types := make([]gin.H, 0, len(identityTypeOrder))
	for _, t := range identityTypeOrder {
		types = append(types, gin.H{"key": t, "label": model.IdentityTypeLabels[t]})
	}

	rules := p.IdentityRules
	if rules == nil {
		rules = []model.IdentityRule{}
	}

	transport.SendSuccess(c, gin.H{
		"rules":   rules,
		"builtin": model.BuiltinIdentityKeywords(),
		"types":   types,
	})
}

// UpdateGalaxyIdentityRules PUT /api/v1/galaxy/identity_rules
// 覆盖保存自定义身份规则。规则变了关系判定就会变，旧图作废。
func (a *API) UpdateGalaxyIdentityRules(c *gin.Context) {
	var body struct {
		Rules []model.IdentityRule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	cleaned := make([]model.IdentityRule, 0, len(body.Rules))
	for _, r := range body.Rules {
		r.Type = strings.TrimSpace(r.Type)
		if model.IdentityTypeLabels[r.Type] == "" {
			transport.BadRequest(c, "未知的关系类型: "+r.Type)
			return
		}
		kws := make([]string, 0, len(r.Keywords))
		for _, k := range r.Keywords {
			if k = strings.TrimSpace(k); k != "" {
				kws = append(kws, k)
			}
		}
		if len(kws) == 0 {
			continue // 没关键词的空规则直接丢掉
		}
		r.Keywords = kws
		r.Label = strings.TrimSpace(r.Label)
		cleaned = append(cleaned, r)
	}

	var p model.UserProfile
	if !readJSONFile(galaxyPath("user_profile.json"), &p) || p.BirthYear == 0 {
		transport.BadRequest(c, "请先设置出生年月")
		return
	}
	p.IdentityRules = cleaned
	if err := writeJSONFile(galaxyPath("user_profile.json"), &p); err != nil {
		transport.InternalServerError(c, "保存失败: "+err.Error())
		return
	}
	_ = os.Remove(galaxyPath("relationship_graph.json"))

	transport.SendSuccess(c, gin.H{"rules": cleaned})
}
