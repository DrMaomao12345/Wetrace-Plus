package repo

import (
	"fmt"
	"testing"

	"github.com/afumu/wetrace/internal/model"
)

func mkNode(id, stage, rtype string, intimacy int) *model.GalaxyNode {
	return &model.GalaxyNode{
		ContactID: id, DisplayName: id,
		MainLifeStage: stage, RelationshipType: rtype,
		CompositeIntimacy: intimacy,
	}
}

// TestLayoutSectors 每个分组的角度必须落在自己的扇区内，分组之间不能重叠
func TestLayoutSectors(t *testing.T) {
	var nodes []*model.GalaxyNode
	for i := 0; i < 28; i++ {
		nodes = append(nodes, mkNode(fmt.Sprintf("u%d", i), "大学", "friend", 90-i))
	}
	for i := 0; i < 15; i++ {
		nodes = append(nodes, mkNode(fmt.Sprintf("h%d", i), "高中", "friend", 70-i))
	}
	for i := 0; i < 6; i++ {
		nodes = append(nodes, mkNode(fmt.Sprintf("f%d", i), "大学", "family", 80-i))
	}
	nodes = append(nodes, mkNode("j0", "初中", "friend", 53))

	LayoutGalaxy(nodes)

	counts := map[string]int{}
	for _, n := range nodes {
		counts[bucketOf(n)]++
	}
	sectors := allocateSectors(counts)

	for _, n := range nodes {
		b := bucketOf(n)
		sec := sectors[b]
		if n.TargetAngle < sec.start || n.TargetAngle > sec.end {
			t.Errorf("%s(%s) 角度 %.2f° 落在扇区 [%.2f, %.2f] 之外",
				n.ContactID, b, n.TargetAngle, sec.start, sec.end)
		}
	}

	// 扇区必须首尾相接、覆盖整圈
	total := 0.0
	for _, s := range sectors {
		total += s.end - s.start
	}
	if total < 359.9 || total > 360.1 {
		t.Errorf("扇区总张角 = %.2f°, 期望 360°", total)
	}
}

// TestLayoutEvenSpread 同一分组内应当均匀铺开，而不是挤在一起
func TestLayoutEvenSpread(t *testing.T) {
	var nodes []*model.GalaxyNode
	for i := 0; i < 10; i++ {
		nodes = append(nodes, mkNode(fmt.Sprintf("u%d", i), "大学", "friend", 100-i*5))
	}
	LayoutGalaxy(nodes)

	// 只有一个分组时应铺满整圈（去掉两端留白）
	var angles []float64
	for _, n := range nodes {
		angles = append(angles, n.TargetAngle)
	}
	minA, maxA := angles[0], angles[0]
	for _, a := range angles {
		if a < minA {
			minA = a
		}
		if a > maxA {
			maxA = a
		}
	}
	if maxA-minA < 300 {
		t.Errorf("单分组铺开跨度 = %.1f°, 期望接近 354°", maxA-minA)
	}

	// 亲密度最高的应排在扇区起点
	top := nodes[0]
	for _, n := range nodes {
		if n.CompositeIntimacy > top.CompositeIntimacy {
			top = n
		}
	}
	if top.TargetAngle > minA+1 {
		t.Errorf("亲密度最高的节点角度 %.1f°, 期望在扇区起点 %.1f°", top.TargetAngle, minA)
	}
}
