package repo

import (
	"context"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// BuildGalaxy 完整跑一遍关系星图流水线:特征提取 → 评分 → 过滤 → 排序取 TopN → 布局。
// topN<=0 表示不截断(全部联系人)。
func (r *Repository) BuildGalaxy(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int) (*model.RelationshipGraph, error) {
	feats, err := r.ExtractGalaxyFeatures(ctx, tzOffsetSec)
	if err != nil {
		return nil, err
	}
	return assembleGalaxy(feats, profile, tzOffsetSec, topN), nil
}

// assembleGalaxy 把原始特征跑完 评分 → 排除服务号(§16) → 排序取 TopN → 布局 → 时间轴，产出图。
// 全量与增量两条路径共用它,保证归一化/布局一致。
func assembleGalaxy(feats []*rawFeatures, profile *model.UserProfile, tzOffsetSec, topN int) *model.RelationshipGraph {
	nodes := ScoreGalaxy(feats, profile)
	vis := make([]*model.GalaxyNode, 0, len(nodes))
	for _, nd := range nodes {
		if !nd.Excluded {
			vis = append(vis, nd)
		}
	}
	sortByIntimacy(vis)
	total := len(vis)
	if topN > 0 && len(vis) > topN {
		vis = vis[:topN]
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
