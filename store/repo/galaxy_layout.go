package repo

import (
	"math"
	"sort"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

// ── 布局:人生阶段分扇区 + 扇区内按亲密度均匀铺开(§5.5/§5.6) ──────
//
// 早期版本给每个阶段写死一个固定张角，扇区内用 ID 哈希随机撒点。问题是真实数据
// 的阶段分布极不均匀 —— 大部分联系人集中在某一两个阶段，几十号人挤进一个 60°
// 的扇区，再加上大多数人亲密度偏低都被推到外圈，最后糊成一团、标签全叠在一起。
//
// 现在改成：
//  1. 扇区张角按该阶段的人数自适应分配（保底张角，避免小分组被压成一条缝）
//  2. 扇区内按亲密度排序后均匀铺开，而不是随机撒点
//
// 代价是「空间稳定」从「每个人的角度永远不变」放宽成「同一份数据布局不变」——
// 新增联系人会让同扇区的人小幅平移。换来的是可读性，值得。

const (
	minRadius = 90.0
	maxRadius = 540.0
	nodeGap   = 46.0 // 碰撞避让的最小间距

	minSectorDeg = 24.0 // 每个非空分组的保底张角
	sectorPadDeg = 3.0  // 扇区两端留白，让分组之间有视觉间隔
)

// stageOrder 是分组的排列顺序，沿圆周依次铺开，保持时间叙事感
var stageOrder = []string{"家人", "小学", "初中", "高中", "大学", "工作", "其他"}

// bucketOf 决定一个节点归入哪个分组。家人优先于人生阶段。
func bucketOf(nd *model.GalaxyNode) string {
	if nd.RelationshipType == "family" {
		return "家人"
	}
	switch nd.MainLifeStage {
	case "小学", "初中", "高中", "大学", "工作":
		return nd.MainLifeStage
	default:
		return "其他"
	}
}

type sector struct{ start, end float64 }

// allocateSectors 按各分组人数把 360° 分配下去。
// 先给每个非空分组保底张角，剩余角度再按人数比例分。
func allocateSectors(counts map[string]int) map[string]sector {
	var active []string
	total := 0
	for _, name := range stageOrder {
		if counts[name] > 0 {
			active = append(active, name)
			total += counts[name]
		}
	}
	out := make(map[string]sector, len(active))
	if len(active) == 0 || total == 0 {
		return out
	}

	// 分组太多时保底张角可能超预算，按比例缩减
	minDeg := minSectorDeg
	if float64(len(active))*minDeg > 360 {
		minDeg = 360 / float64(len(active))
	}
	remaining := 360 - float64(len(active))*minDeg

	cursor := 0.0
	for _, name := range active {
		width := minDeg + remaining*float64(counts[name])/float64(total)
		out[name] = sector{start: cursor, end: cursor + width}
		cursor += width
	}
	return out
}

// LayoutGalaxy 计算每个节点的目标半径/角度与最终坐标。
func LayoutGalaxy(nodes []*model.GalaxyNode) {
	// 1. 分组
	groups := map[string][]*model.GalaxyNode{}
	counts := map[string]int{}
	for _, nd := range nodes {
		b := bucketOf(nd)
		groups[b] = append(groups[b], nd)
		counts[b]++
	}

	sectors := allocateSectors(counts)

	// 2. 每个分组内按亲密度降序均匀铺开
	for name, group := range groups {
		sec, ok := sectors[name]
		if !ok {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].CompositeIntimacy != group[j].CompositeIntimacy {
				return group[i].CompositeIntimacy > group[j].CompositeIntimacy
			}
			return group[i].ContactID < group[j].ContactID // 同分时定序，保证可复现
		})

		width := sec.end - sec.start
		pad := sectorPadDeg
		if width < 4*pad {
			pad = width / 8 // 扇区很窄时按比例缩小留白
		}
		inner := width - 2*pad
		n := len(group)

		for i, nd := range group {
			var frac float64
			if n > 1 {
				frac = float64(i) / float64(n-1) // 两端都用满
			} else {
				frac = 0.5 // 只有一个人时放在扇区正中
			}
			ang := sec.start + pad + frac*inner

			radius := minRadius + (1-float64(nd.CompositeIntimacy)/100.0)*(maxRadius-minRadius)
			nd.TargetAngle = ang
			nd.TargetRadius = radius
			rad := ang * math.Pi / 180.0
			nd.X = radius * math.Cos(rad)
			nd.Y = radius * math.Sin(rad)
		}
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
