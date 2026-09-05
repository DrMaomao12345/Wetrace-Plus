package export

import (
	"testing"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

func ymStat(y, m, c int) *model.YearMonthStat {
	return &model.YearMonthStat{Year: y, Month: m, Count: c}
}

// 「哪几个月是真实存在的」这条规则被写错过三次（两处图表 + 一处导出），
// 所以单独钉住：左边界是第一条消息，右边界永远是当月，两头都不外扩。
func TestMonthWindowOf(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		stats     []*model.YearMonthStat
		wantFirst int
		wantLast  int
		wantOK    bool
	}{
		{
			name:      "首条之前不算数",
			stats:     []*model.YearMonthStat{ymStat(2023, 11, 30), ymStat(2023, 12, 5)},
			wantFirst: ym(2023, 11),
			wantLast:  ym(2026, 9), // 右边界是当月，不是最后一条
			wantOK:    true,
		},
		{
			name:      "尾部沉默仍在区间内 —— 不聊了，0 就是 0",
			stats:     []*model.YearMonthStat{ymStat(2026, 1, 100), ymStat(2026, 3, 50)},
			wantFirst: ym(2026, 1),
			wantLast:  ym(2026, 9),
			wantOK:    true,
		},
		{
			name:      "count=0 的行不能拿去定左边界",
			stats:     []*model.YearMonthStat{ymStat(2025, 1, 0), ymStat(2025, 6, 12)},
			wantFirst: ym(2025, 6),
			wantLast:  ym(2026, 9),
			wantOK:    true,
		},
		{
			name:   "空数据",
			stats:  nil,
			wantOK: false,
		},
		{
			name:   "全是 0",
			stats:  []*model.YearMonthStat{ymStat(2025, 1, 0)},
			wantOK: false,
		},
		{
			name:      "脏数据（越界年月、nil）一律忽略",
			stats:     []*model.YearMonthStat{nil, ymStat(1970, 1, 5), ymStat(2025, 13, 5), ymStat(2025, 4, 7)},
			wantFirst: ym(2025, 4),
			wantLast:  ym(2026, 9),
			wantOK:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, last, ok := monthWindowOf(tc.stats, now)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, 期望 %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if first != tc.wantFirst {
				t.Errorf("first = %d, 期望 %d", first, tc.wantFirst)
			}
			if last != tc.wantLast {
				t.Errorf("last = %d, 期望 %d", last, tc.wantLast)
			}
		})
	}
}

// ym 是把年月压成整数的编码，解码在 buildMonthlyStats 里，必须能对得上。
func TestYMRoundTrip(t *testing.T) {
	for _, y := range []int{2000, 2023, 2026, 2100} {
		for m := 1; m <= 12; m++ {
			k := ym(y, m)
			gotY, gotM := (k-1)/12, (k-1)%12+1
			if gotY != y || gotM != m {
				t.Errorf("ym(%d,%d)=%d 解回 (%d,%d)", y, m, k, gotY, gotM)
			}
		}
	}
}

// 相邻月份的 key 必须相差 1 —— 补空月的循环整个建立在这上面。
func TestYMContiguousAcrossYearBoundary(t *testing.T) {
	if ym(2024, 1)-ym(2023, 12) != 1 {
		t.Errorf("跨年不连续: %d → %d", ym(2023, 12), ym(2024, 1))
	}
}
