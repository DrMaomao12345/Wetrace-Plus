package repo

import (
	"context"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

// BuildGalaxy 完整跑一遍关系星图流水线:特征提取 → 评分 → 过滤 → 排序取 TopN → 布局。
// topN<=0 表示不截断(全部联系人)。pinned 里的联系人始终保留,不受 TopN 截断影响。
func (r *Repository) BuildGalaxy(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int, overrides map[string]*model.ContactOverride) (*model.RelationshipGraph, error) {
	feats, err := r.ExtractGalaxyFeatures(ctx, tzOffsetSec)
	if err != nil {
		return nil, err
	}
	return assembleGalaxy(feats, profile, tzOffsetSec, topN, overrides), nil
}

// assembleGalaxy 把原始特征跑完 评分 → 排除服务号(§16) → 排序取 TopN → 布局 → 时间轴，产出图。
// 全量与增量两条路径共用它,保证归一化/布局一致。
func assembleGalaxy(feats []*rawFeatures, profile *model.UserProfile, tzOffsetSec, topN int, overrides map[string]*model.ContactOverride) *model.RelationshipGraph {
	nodes := ScoreGalaxy(feats, profile)

	// 用户的手动修正必须在布局之前套用 —— 关系类型和人生阶段决定节点落在哪个扇区，
	// 之前是在建图之后才改标签，导致「改成家人」的人仍留在原来的扇区里，
	// 标签和位置对不上。
	pinSet := map[string]bool{}
	vis := make([]*model.GalaxyNode, 0, len(nodes))
	for _, nd := range nodes {
		if o := overrides[nd.ContactID]; o != nil {
			if o.Hidden {
				continue
			}
			if o.RelationshipType != "" {
				nd.RelationshipType = o.RelationshipType
			}
			if o.RelationshipLabel != "" {
				nd.RelationshipLabel = o.RelationshipLabel
			}
			if o.MainLifeStage != "" {
				nd.MainLifeStage = o.MainLifeStage
			}
			if o.ManualImportant {
				nd.ManualImportant = true
			}
			if o.Pinned {
				nd.Pinned = true
				pinSet[nd.ContactID] = true
			}
		}
		if !nd.Excluded {
			vis = append(vis, nd)
		}
	}
	sortByIntimacy(vis)
	total := len(vis)

	if topN > 0 && len(vis) > topN {
		kept := vis[:topN]
		// 被 pin 的人即使排在 TopN 之外也要补回来(§固定人物置顶)
		inKept := make(map[string]bool, len(kept))
		for _, n := range kept {
			inKept[n.ContactID] = true
		}
		extra := make([]*model.GalaxyNode, 0, len(pinSet))
		for _, n := range vis[topN:] {
			if pinSet[n.ContactID] && !inKept[n.ContactID] {
				extra = append(extra, n)
			}
		}
		vis = append(append(make([]*model.GalaxyNode, 0, len(kept)+len(extra)), kept...), extra...)
		sortByIntimacy(vis)
	}

	for _, n := range vis {
		if pinSet[n.ContactID] {
			n.Pinned = true
		}
	}

	LayoutGalaxy(vis)
	return &model.RelationshipGraph{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Version:     1,
		TzOffsetMin: tzOffsetSec / 60,
		Profile:     profile,
		Nodes:       vis,
		TotalCount:  total,
		Timeline:    BuildTimeline(feats, vis),
	}
}
