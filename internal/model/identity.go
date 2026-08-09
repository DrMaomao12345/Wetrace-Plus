package model

import "strings"

// 内置身份关键词词典(§17.2)。顺序即优先级：
// 服务/商家排最前，避免「XX 客服小张」被当成朋友。
func builtinIdentityRules() []IdentityRule {
	return []IdentityRule{
		{Type: "service", Label: "服务/商家", Keywords: []string{
			"客服", "中介", "销售", "外卖", "快递", "维修", "房东", "房产", "贷款", "理财", "官方", "助手", "小助手", "通知",
		}},
		{Type: "family", Label: "家人", Keywords: []string{
			"妈妈", "爸爸", "母亲", "父亲", "老妈", "老爸", "姐姐", "哥哥", "弟弟", "妹妹",
			"奶奶", "爷爷", "外婆", "外公", "姑", "叔", "舅", "姨", "表哥", "表弟", "表姐", "表妹", "家",
		}},
		{Type: "teacher", Label: "老师", Keywords: []string{
			"老师", "导师", "教授", "班主任", "教练", "助教", "teacher", "tutor", "professor", "coach",
		}},
		{Type: "classmate", Label: "同学", Keywords: []string{
			"同学", "室友", "舍友", "同桌", "班长", "学委",
		}},
		{Type: "work", Label: "工作关系", Keywords: []string{
			"同事", "老板", "经理", "总监", "hr", "leader", "主管", "公司", "实习",
		}},
	}
}

// IdentityTypeLabels 是关系类型的默认中文名
var IdentityTypeLabels = map[string]string{
	"family":    "家人",
	"teacher":   "老师",
	"classmate": "同学",
	"work":      "工作关系",
	"service":   "服务/商家",
	"friend":    "朋友",
}

// Matches 判断一段文本(备注 + 昵称)是否命中本规则
func (r IdentityRule) Matches(lowerText string) bool {
	for _, k := range r.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if strings.Contains(lowerText, k) {
			return true
		}
	}
	return false
}

// EffectiveIdentityRules 返回实际生效的规则链：用户自定义规则优先，其后是内置词典。
// 老配置里的扁平 CustomKeywords 视作一条追加在最后的 friend 规则。
func (p *UserProfile) EffectiveIdentityRules() []IdentityRule {
	var out []IdentityRule
	if p != nil {
		for _, r := range p.IdentityRules {
			if len(r.Keywords) == 0 || r.Type == "" {
				continue
			}
			if r.Label == "" {
				r.Label = IdentityTypeLabels[r.Type]
			}
			out = append(out, r)
		}
	}
	out = append(out, builtinIdentityRules()...)
	if p != nil && len(p.CustomKeywords) > 0 {
		out = append(out, IdentityRule{Type: "friend", Label: "朋友", Keywords: p.CustomKeywords})
	}
	return out
}
