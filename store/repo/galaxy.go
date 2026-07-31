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
	nodes := ScoreGalaxy(feats, profile)

	// 排除服务号/一次性联系人(§16)
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
	}, nil
}
