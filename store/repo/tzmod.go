package repo

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

// tzBoundary 是「从这一刻起用这个偏移」。
type tzBoundary struct {
	fromUnix int64 // 含
	offsetS  int   // 秒
}

// buildTZBoundaries 把配置摊平成一串按时间升序、互不重叠的区间。
// 分段之间的空隙用默认时区补上，首尾也用默认时区兜住。
//
// 边界按**该段自己的时区**取当天零点 —— 「从 9 月 10 号起我在英国」说的是
// 英国时间的 9 月 10 号零点，不是 UTC 零点。
func buildTZBoundaries(cfg model.TZConfig) []tzBoundary {
	defS := cfg.DefaultOffset * 60

	type seg struct {
		start, end int64
		offsetS    int
	}
	var segs []seg
	for _, s := range cfg.Segments {
		offS := s.TZOffset * 60
		loc := time.FixedZone("seg", offS)
		st, err1 := time.ParseInLocation("2006-01-02", s.StartDate, loc)
		en, err2 := time.ParseInLocation("2006-01-02", s.EndDate, loc)
		if err1 != nil || err2 != nil || en.Before(st) {
			continue
		}
		// 结束日整天都算在内
		segs = append(segs, seg{start: st.Unix(), end: en.AddDate(0, 0, 1).Unix(), offsetS: offS})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].start < segs[j].start })

	out := []tzBoundary{{fromUnix: -1 << 62, offsetS: defS}}
	cursor := int64(-1 << 62)
	for _, s := range segs {
		if s.end <= cursor {
			continue // 已被前一段完全覆盖
		}
		start := s.start
		if start < cursor {
			start = cursor // 重叠时先到先得
		}
		if start > cursor {
			// 空隙用默认时区
			out = append(out, tzBoundary{fromUnix: cursor, offsetS: defS})
		}
		out = append(out, tzBoundary{fromUnix: start, offsetS: s.offsetS})
		cursor = s.end
	}
	if cursor > -1<<62 {
		out = append(out, tzBoundary{fromUnix: cursor, offsetS: defS})
	}

	// 合并相邻且偏移相同的区间，CASE 能短不少
	merged := out[:0]
	for i, b := range out {
		if i > 0 && merged[len(merged)-1].offsetS == b.offsetS {
			continue
		}
		merged = append(merged, b)
	}
	return merged
}

// tzModifierSQL 生成可以直接插进 strftime 的修饰符参数。
//
// 没有分段时就是一个常量字符串（和以前一样）；有分段时生成 CASE 表达式，
// **一条查询按行选时区**，不必把每个 GROUP BY 拆成 N 条再合并 ——
// 已验证 CASE 的结果与手工拆段合并逐行相等。
//
// timeExpr 必须是该表里「Unix 秒」的表达式：V4 是 create_time，
// V3 是 CreateTime/1000。
func tzModifierSQL(cfg model.TZConfig, timeExpr string) string {
	bs := buildTZBoundaries(cfg)
	if len(bs) <= 1 {
		return fmt.Sprintf("'%+d seconds'", cfg.DefaultOffset*60)
	}
	var sb strings.Builder
	sb.WriteString("CASE")
	// 从后往前写 WHEN ... >= ...，最后一段落到 ELSE
	for i := len(bs) - 1; i >= 1; i-- {
		fmt.Fprintf(&sb, " WHEN (%s) >= %d THEN '%+d seconds'", timeExpr, bs[i].fromUnix, bs[i].offsetS)
	}
	fmt.Fprintf(&sb, " ELSE '%+d seconds' END", bs[0].offsetS)
	return sb.String()
}
