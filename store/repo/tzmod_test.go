package repo

import (
	"strings"
	"testing"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

func unixAt(date string, offsetMin int) int64 {
	loc := time.FixedZone("x", offsetMin*60)
	t, _ := time.ParseInLocation("2006-01-02", date, loc)
	return t.Unix()
}

// 没有分段时必须退化成一个常量修饰符 —— 不能凭空给所有查询套上 CASE。
func TestNoSegmentsIsPlainModifier(t *testing.T) {
	got := tzModifierSQL(model.TZConfig{DefaultOffset: 480}, "create_time")
	if got != "'+28800 seconds'" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(got, "CASE") {
		t.Error("无分段时不该出现 CASE")
	}
}

// 边界按**该段自己的时区**取零点：「9/10 起在英国」= 英国时间 9/10 00:00。
func TestBoundaryUsesSegmentOwnTimezone(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "2025-09-10", EndDate: "2025-12-21", TZOffset: 0},
		},
	}
	bs := buildTZBoundaries(cfg)
	var found bool
	want := unixAt("2025-09-10", 0) // UTC+0 的 9/10 零点
	for _, b := range bs {
		if b.fromUnix == want && b.offsetS == 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("没找到按段内时区算的边界 %d，实得 %+v", want, bs)
	}
	// 用 UTC+8 的 9/10 零点会早 8 小时，不该出现
	for _, b := range bs {
		if b.fromUnix == unixAt("2025-09-10", 480) {
			t.Error("边界用了默认时区而不是段自己的时区")
		}
	}
}

// 分段之间的空隙必须回落到默认时区，首尾也要兜住。
func TestGapsFallBackToDefault(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "2025-03-01", EndDate: "2025-03-31", TZOffset: 0},
			{StartDate: "2025-06-01", EndDate: "2025-06-30", TZOffset: 60},
		},
	}
	bs := buildTZBoundaries(cfg)
	if bs[0].offsetS != 480*60 {
		t.Errorf("开头应是默认时区，实得 %d", bs[0].offsetS)
	}
	if bs[len(bs)-1].offsetS != 480*60 {
		t.Errorf("结尾应回落到默认时区，实得 %d", bs[len(bs)-1].offsetS)
	}
	// 3 月和 6 月之间那段必须是默认
	var midDefault bool
	for i, b := range bs {
		if b.offsetS == 480*60 && i > 0 && i < len(bs)-1 {
			midDefault = true
		}
	}
	if !midDefault {
		t.Errorf("3~6 月之间的空隙没回落到默认时区：%+v", bs)
	}
}

// 边界必须严格升序，否则 CASE 的 WHEN 顺序会选错分支。
func TestBoundariesStrictlyAscending(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "2026-04-12", EndDate: "2026-05-22", TZOffset: 60},
			{StartDate: "2025-09-10", EndDate: "2025-12-21", TZOffset: 0},
			{StartDate: "2026-03-22", EndDate: "2026-04-11", TZOffset: 480},
			{StartDate: "2025-12-22", EndDate: "2025-12-26", TZOffset: 540},
		},
	}
	bs := buildTZBoundaries(cfg)
	for i := 1; i < len(bs); i++ {
		if bs[i].fromUnix <= bs[i-1].fromUnix {
			t.Fatalf("边界没有严格升序：%d 之后是 %d", bs[i-1].fromUnix, bs[i].fromUnix)
		}
	}
}

// 相邻且偏移相同的区间要合并，否则 CASE 会白白变长。
func TestAdjacentSameOffsetMerged(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "2025-03-01", EndDate: "2025-03-31", TZOffset: 480}, // 和默认一样
		},
	}
	bs := buildTZBoundaries(cfg)
	if len(bs) != 1 {
		t.Errorf("全程同一个偏移应合并成 1 段，实得 %d 段：%+v", len(bs), bs)
	}
	if strings.Contains(tzModifierSQL(cfg, "create_time"), "CASE") {
		t.Error("合并后只剩一段，不该再生成 CASE")
	}
}

// 重叠分段：先到先得，不能产生倒退的边界。
func TestOverlappingSegments(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "2025-03-01", EndDate: "2025-06-30", TZOffset: 0},
			{StartDate: "2025-05-01", EndDate: "2025-08-31", TZOffset: 60},
		},
	}
	bs := buildTZBoundaries(cfg)
	for i := 1; i < len(bs); i++ {
		if bs[i].fromUnix <= bs[i-1].fromUnix {
			t.Fatalf("重叠导致边界倒退：%+v", bs)
		}
	}
}

// 生成的 SQL 里，时间表达式必须原样出现（V3 用的是 CreateTime/1000）。
func TestTimeExprIsUsed(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments:      []model.TZSegmentConfig{{StartDate: "2025-09-10", EndDate: "2025-12-21", TZOffset: 0}},
	}
	sql := tzModifierSQL(cfg, "CreateTime/1000")
	if !strings.Contains(sql, "(CreateTime/1000)") {
		t.Errorf("时间表达式没被用上：%s", sql)
	}
}

// 脏配置（日期解析不了、结束早于开始）直接忽略，不能把整份配置搞崩。
func TestBadSegmentsIgnored(t *testing.T) {
	cfg := model.TZConfig{
		DefaultOffset: 480,
		Segments: []model.TZSegmentConfig{
			{StartDate: "不是日期", EndDate: "2025-12-21", TZOffset: 0},
			{StartDate: "2025-12-21", EndDate: "2025-09-10", TZOffset: 0}, // 倒过来
		},
	}
	if got := tzModifierSQL(cfg, "create_time"); got != "'+28800 seconds'" {
		t.Errorf("脏配置应被忽略并退化成默认，实得 %s", got)
	}
}
