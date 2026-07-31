package repo

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// ── 身份关键词词典(§17.2,可被 user_profile 扩展) ──────────────
var (
	kwFamily    = []string{"妈妈", "爸爸", "母亲", "父亲", "老妈", "老爸", "姐姐", "哥哥", "弟弟", "妹妹", "奶奶", "爷爷", "外婆", "外公", "姑", "叔", "舅", "姨", "表哥", "表弟", "表姐", "表妹", "家"}
	kwTeacher   = []string{"老师", "导师", "教授", "班主任", "教练", "助教", "teacher", "tutor", "professor", "coach"}
	kwClassmate = []string{"同学", "室友", "舍友", "同桌", "班长", "学委"}
	kwWork      = []string{"同事", "老板", "经理", "总监", "hr", "leader", "主管", "公司", "实习"}
	kwService   = []string{"客服", "中介", "销售", "外卖", "快递", "维修", "房东", "房产", "贷款", "理财", "官方", "助手", "小助手", "通知"}
)

// DefaultLifeStages 依据出生年月生成默认教育阶段(§3.2),按常见入学年龄。
func DefaultLifeStages(birthYear, birthMonth int) []model.LifeStage {
	if birthYear < 1950 || birthYear > 2025 {
		return nil
	}
	ym := func(y, m int) string { return fmt.Sprintf("%04d-%02d", y, m) }
	b := birthYear
	return []model.LifeStage{
		{Name: "小学", Start: ym(b+6, 9), End: ym(b+12, 6)},
		{Name: "初中", Start: ym(b+12, 9), End: ym(b+15, 6)},
		{Name: "高中", Start: ym(b+15, 9), End: ym(b+18, 6)},
		{Name: "大学", Start: ym(b+18, 9), End: ym(b+22, 6)},
		{Name: "工作", Start: ym(b+22, 9), End: ""},
	}
}

// ── 归一化:log 压缩 + Min-Max → 0..100(§19) ─────────────────
func normalizeLog(vals []float64) []float64 {
	logs := make([]float64, len(vals))
	mn, mx := math.Inf(1), math.Inf(-1)
	for i, v := range vals {
		l := math.Log(1 + v)
		logs[i] = l
		if l < mn {
			mn = l
		}
		if l > mx {
			mx = l
		}
	}
	out := make([]float64, len(vals))
	if mx-mn < 1e-9 {
		return out // 全相同 → 0
	}
	for i, l := range logs {
		out[i] = (l - mn) / (mx - mn) * 100
	}
	return out
}

func clamp100(v float64) int {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return int(v + 0.5)
}

// ScoreGalaxy 把原始特征评分为完整关系画像(不含布局坐标)。
func ScoreGalaxy(feats []*rawFeatures, profile *model.UserProfile) []*model.GalaxyNode {
	n := len(feats)
	if n == 0 {
		return nil
	}
	// 收集用于跨联系人归一化的原始量
	totV := make([]float64, n)
	sessV := make([]float64, n)
	amV := make([]float64, n)
	r30V := make([]float64, n)
	r90V := make([]float64, n)
	r365V := make([]float64, n)
	streakV := make([]float64, n)
	for i, f := range feats {
		totV[i] = float64(f.total)
		sessV[i] = float64(f.sessionCount)
		amV[i] = float64(f.activeMonths())
		r30V[i] = float64(f.recent30)
		r90V[i] = float64(f.recent90)
		r365V[i] = float64(f.recent365)
		streakV[i] = float64(f.longestActiveStreak())
	}
	msgN := normalizeLog(totV)
	sessN := normalizeLog(sessV)
	amN := normalizeLog(amV)
	r30N := normalizeLog(r30V)
	r90N := normalizeLog(r90V)
	r365N := normalizeLog(r365V)
	streakN := normalizeLog(streakV)

	now := time.Now().Unix()
	nodes := make([]*model.GalaxyNode, 0, n)

	for i, f := range feats {
		// 双向交流(§18.2):比例,天然 0-100
		recip := 0.0
		if f.sent+f.recv > 0 {
			recip = 2 * float64(min(f.sent, f.recv)) / float64(f.sent+f.recv) * 100
		}

		// 人生阶段
		mainStage, stageCount := assignLifeStage(f, profile)
		crossScore := crossStageScore(stageCount)

		// 关系类型 + 标签 + 是否服务号
		rtype, label, isService := classifyRelation(f, mainStage)

		// 历史关系深度(§20.1)
		depth := 0.30*msgN[i] + 0.25*sessN[i] + 0.20*amN[i] + 0.15*recip + 0.10*crossScore
		// 当前温度(§20.2)
		temp := 0.50*r30N[i] + 0.30*r90N[i] + 0.20*r365N[i]
		// 陪伴持续性(§20.3):活跃月占比 + 最长连续 +(恢复联系简化)
		amRatio := 0.0
		if sp := f.spanMonths(); sp > 0 {
			amRatio = float64(f.activeMonths()) / float64(sp) * 100
		}
		continuity := 0.60*amRatio + 0.40*streakN[i]
		// 综合亲密度(§20.5)
		composite := 0.40*depth + 0.30*temp + 0.20*continuity + 0.10*recip
		// 近期活跃度(控制脉动):用 r30/r90 归一
		recentAct := 0.6*r30N[i] + 0.4*r90N[i]

		status := relationStatus(depth, temp, f, now)
		conf := confidence(f, rtype, stageCount)

		node := &model.GalaxyNode{
			ContactID:         f.contactID,
			DisplayName:       displayNameOf(f),
			Avatar:            f.avatar,
			RelationshipType:  rtype,
			RelationshipLabel: label,
			MainLifeStage:     mainStage,
			LifeStageCount:    stageCount,
			RelationshipDepth: clamp100(depth),
			CurrentTemperature: clamp100(temp),
			ContinuityScore:    clamp100(continuity),
			ReciprocityScore:   clamp100(recip),
			CompositeIntimacy:  clamp100(composite),
			Confidence:         clamp100(float64(conf)),
			RecentActivity:     clamp100(recentAct),
			Status:             status,
			TotalMessages:      f.total,
			SentMessages:       f.sent,
			RecvMessages:       f.recv,
			FirstTime:          f.firstTime,
			LastTime:           f.lastTime,
			ActiveMonths:       f.activeMonths(),
			Excluded:           isService && f.activeDays <= 14 && f.sessionCount <= 5,
			Evidence:           buildEvidence(f, rtype, mainStage, status),
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// assignLifeStage 判定主要人生阶段 + 跨越阶段数(§21)。
func assignLifeStage(f *rawFeatures, profile *model.UserProfile) (string, int) {
	if profile == nil || len(profile.LifeStages) == 0 {
		return "未分阶段", 1
	}
	stageMsg := map[string]int{}
	stageMonths := map[string]int{}
	for mon, cnt := range f.monthCounts {
		st := stageOfMonth(profile.LifeStages, mon)
		if st == "" {
			st = "其他"
		}
		stageMsg[st] += cnt
		stageMonths[st]++
	}
	if len(stageMsg) == 0 {
		return "未分阶段", 1
	}
	total := f.total
	best, bestScore := "", -1.0
	touched := 0
	for st, msg := range stageMsg {
		if float64(msg)/float64(total) >= 0.05 {
			touched++
		}
		score := 0.55*float64(msg)/float64(total) + 0.30*float64(stageMonths[st])/float64(f.activeMonths()+1)
		if score > bestScore {
			bestScore, best = score, st
		}
	}
	if touched < 1 {
		touched = 1
	}
	return best, touched
}

func stageOfMonth(stages []model.LifeStage, mon string) string {
	for _, s := range stages {
		if mon >= s.Start && (s.End == "" || mon <= s.End) {
			return s.Name
		}
	}
	return ""
}

func crossStageScore(count int) float64 {
	switch {
	case count >= 4:
		return 100
	case count == 3:
		return 75
	case count == 2:
		return 50
	default:
		return 25
	}
}

// classifyRelation 按词典判定关系类型 + 标签,返回 (type,label,isService)。
func classifyRelation(f *rawFeatures, stage string) (string, string, bool) {
	text := strings.ToLower(f.remark + " " + f.nickName)
	has := func(kws []string) bool {
		for _, k := range kws {
			if strings.Contains(text, strings.ToLower(k)) {
				return true
			}
		}
		return false
	}
	switch {
	case has(kwService):
		return "service", "服务/商家", true
	case has(kwFamily):
		return "family", "家人", false
	case has(kwTeacher):
		lbl := "老师"
		if stage != "" && stage != "未分阶段" && stage != "其他" {
			lbl = stage + "老师"
		}
		return "teacher", lbl, false
	case has(kwClassmate):
		lbl := "同学"
		if stage != "" && stage != "未分阶段" && stage != "其他" {
			lbl = stage + "同学"
		}
		return "classmate", lbl, false
	case has(kwWork):
		return "work", "工作关系", false
	default:
		// 无身份词:按亲密度粗分朋友 / 陌生
		return "friend", "朋友", false
	}
}

func relationStatus(depth, temp float64, f *rawFeatures, now int64) string {
	noRecent180 := f.lastTime > 0 && now-f.lastTime > 180*86400
	switch {
	case f.recent90 > 0 && temp >= 35:
		return "active"
	case depth >= 50 && noRecent180:
		return "faded"
	case f.recent365 > 0:
		return "low_freq"
	default:
		return "faded"
	}
}

func confidence(f *rawFeatures, rtype string, stageCount int) int {
	c := 40.0
	if f.remark != "" {
		c += 25 // 有备注 → 身份更明确
	}
	if rtype != "friend" && rtype != "stranger" {
		c += 15 // 命中身份词典
	}
	if stageCount == 1 {
		c += 20 // 阶段集中
	} else if stageCount >= 3 {
		c -= 5
	}
	if c > 100 {
		c = 100
	}
	return int(c)
}

func displayNameOf(f *rawFeatures) string {
	if f.remark != "" {
		return f.remark
	}
	if f.nickName != "" {
		return f.nickName
	}
	if len(f.contactID) > 8 {
		return f.contactID[:8]
	}
	return f.contactID
}

func buildEvidence(f *rawFeatures, rtype, stage, status string) []string {
	ev := []string{}
	if f.remark != "" {
		ev = append(ev, "备注:"+f.remark)
	}
	if stage != "" && stage != "未分阶段" {
		ev = append(ev, "消息高峰集中于"+stage)
	}
	ev = append(ev, fmt.Sprintf("共 %d 条消息 / 活跃 %d 个月", f.total, f.activeMonths()))
	switch status {
	case "faded":
		ev = append(ev, "近半年无有效聊天,已淡出")
	case "active":
		ev = append(ev, "近期仍在活跃联系")
	case "low_freq":
		ev = append(ev, "低频维持")
	}
	return ev
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
