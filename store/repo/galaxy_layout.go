package repo

import (
	"hash/fnv"
	"math"
	"sort"

	"github.com/afumu/wetrace/internal/model"
)

// 人生阶段 → 圆周扇区(度)(§5.6)
type sector struct{ start, end float64 }

func stageSector(stage, rtype string) sector {
	if rtype == "family" {
		return sector{0, 45}
	}
	switch stage {
	case "小学":
		return sector{45, 90}
	case "初中":
		return sector{90, 150}
	case "高中":
		return sector{150, 220}
	case "大学":
		return sector{220, 300}
	case "工作":
		return sector{300, 360}
	default:
		return sector{300, 360} // 其他/未分阶段 归到工作及其他扇区
	}
}

const (
	minRadius = 90.0
	maxRadius = 540.0
	nodeGap   = 46.0 // 碰撞避让的最小间距
)

// stableOffset 由联系人 id 生成 [0,1) 的稳定偏移,保证每次打开位置一致(§33.5)。
func stableOffset(id string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return float64(h.Sum32()%10000) / 10000.0
}

// LayoutGalaxy 计算每个节点的目标半径/角度与最终坐标。
func LayoutGalaxy(nodes []*model.GalaxyNode) {
	for _, nd := range nodes {
		sec := stageSector(nd.MainLifeStage, nd.RelationshipType)
		// 扇区内稳定角度
		ang := sec.start + stableOffset(nd.ContactID)*(sec.end-sec.start)
		// 亲密度越高越靠近中心(§5.5)
		radius := minRadius + (1-float64(nd.CompositeIntimacy)/100.0)*(maxRadius-minRadius)
		nd.TargetAngle = ang
		nd.TargetRadius = radius
		rad := ang * math.Pi / 180.0
		nd.X = radius * math.Cos(rad)
		nd.Y = radius * math.Sin(rad)
	}
	resolveCollisions(nodes)
}

// resolveCollisions 轻量迭代把过近的节点推开,同时尽量保持各自半径(§6)。
func resolveCollisions(nodes []*model.GalaxyNode) {
	for iter := 0; iter < 60; iter++ {
		moved := false
		for i := 0; i < len(nodes); i++ {
			for j := i + 1; j < len(nodes); j++ {
				a, b := nodes[i], nodes[j]
				dx, dy := b.X-a.X, b.Y-a.Y
				d := math.Hypot(dx, dy)
				if d < nodeGap && d > 1e-6 {
					push := (nodeGap - d) / 2
					ux, uy := dx/d, dy/d
					a.X -= ux * push
					a.Y -= uy * push
					b.X += ux * push
					b.Y += uy * push
					moved = true
				}
			}
		}
		if !moved {
			break
		}
	}
	// 别离中心太近(避免和"我"重合)
	for _, nd := range nodes {
		if d := math.Hypot(nd.X, nd.Y); d < minRadius && d > 1e-6 {
			s := minRadius / d
			nd.X *= s
			nd.Y *= s
		}
	}
}

// sortByIntimacy 按综合亲密度降序。
func sortByIntimacy(nodes []*model.GalaxyNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CompositeIntimacy > nodes[j].CompositeIntimacy
	})
}
