package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/xuri/excelize/v2"
)

// MonthlyStatRow 是月度统计导出的一行。
type MonthlyStatRow struct {
	YearMonth  string // 2023-11
	Year       int
	Month      int
	Count      int
	Cumulative int
	SharePct   float64 // 占该会话全部消息的百分比
}

// ym 把 (年, 月) 压成一个能直接比较、直接自增的整数。
func ym(year, month int) int { return year*12 + month }

// monthWindowOf 给出「真实存在」的月份闭区间 —— **和前端 lib/monthSeries.ts
// 是同一条规则**，改一边记得同步另一边。
//
//	区间 = [第一条消息所在的月, 右边界]
//
// 左边界之前数据尚不存在（联系人还没加上），这一头永远裁。
//
// 右边界由 fillToNow 决定 —— 这两种口径都说得通，所以做成开关：
//
//	true （默认）：延到**当月**。最后一条之后没聊的月份记作 0 —— 那段沉默
//	              是真事，「三月起就没说过话」正是时间序列该讲的话。
//	false        ：停在**最后一条消息所在的月**。只要有记录的那一段，
//	              不想要一条长长的 0 尾巴时用。
//
// 第三个返回值为 false 表示这个会话一条消息都没有。
func monthWindowOf(stats []*model.YearMonthStat, now time.Time, fillToNow bool) (first, last int, ok bool) {
	first = 1 << 30
	lastWithData := 0
	for _, st := range stats {
		if st == nil || st.Count <= 0 || st.Month < 1 || st.Month > 12 || st.Year < 2000 || st.Year > 2100 {
			continue
		}
		k := ym(st.Year, st.Month)
		if k < first {
			first = k
		}
		if k > lastWithData {
			lastWithData = k
		}
	}
	if first == 1<<30 {
		return 0, 0, false
	}
	if !fillToNow {
		return first, lastWithData, true
	}
	// 当月永远是右上限：再往后是还没发生的月份，补 0 就是造数据。
	// 数据比当月还新（时区/机器时钟偏差）时以数据为准，别把它裁掉。
	nowKey := ym(now.Year(), int(now.Month()))
	if lastWithData > nowKey {
		return first, lastWithData, true
	}
	return first, nowKey, true
}

// buildMonthlyStats 取某个会话按年-月聚合的消息数，补齐成一条连续的月度序列。
// fillToNow 见 monthWindowOf。
//
// 补 0 是必须的：底层是 GROUP BY，一条消息都没有的月份根本不会有行，直接导出
// 会得到一份「跳月」的表 —— 拿去画图或者做同比全是坑。范围由 monthWindowOf
// 决定，两头都不外扩：没发生过的月份不凭空造。
func (s *Service) buildMonthlyStats(ctx context.Context, talker string, fillToNow bool) ([]MonthlyStatRow, error) {
	stats, err := s.Store.GetYearlyMonthlyActivity(ctx, talker)
	if err != nil {
		return nil, err
	}

	minKey, maxKey, ok := monthWindowOf(stats, time.Now(), fillToNow)
	if !ok {
		return nil, nil
	}

	byKey := make(map[int]int, len(stats))
	total := 0
	for _, st := range stats {
		if st == nil || st.Month < 1 || st.Month > 12 || st.Year < 2000 || st.Year > 2100 {
			continue
		}
		k := ym(st.Year, st.Month)
		if k < minKey || k > maxKey {
			continue // 理论上不会有：早于首条、或晚于当月
		}
		byKey[k] += st.Count
		total += st.Count
	}

	rows := make([]MonthlyStatRow, 0, maxKey-minKey+1)
	cumulative := 0
	for k := minKey; k <= maxKey; k++ {
		count := byKey[k]
		cumulative += count
		share := 0.0
		if total > 0 {
			share = float64(count) * 100 / float64(total)
		}
		year, month := (k-1)/12, (k-1)%12+1
		rows = append(rows, MonthlyStatRow{
			YearMonth:  fmt.Sprintf("%04d-%02d", year, month),
			Year:       year,
			Month:      month,
			Count:      count,
			Cumulative: cumulative,
			SharePct:   share,
		})
	}
	return rows, nil
}

var monthlyStatsHeader = []string{"年月", "年", "月", "消息数", "累计", "占比%"}

// ExportMonthlyStatsCSV 导出「从第一条消息起，每个月多少条」为 CSV。
func (s *Service) ExportMonthlyStatsCSV(ctx context.Context, talker string, fillToNow bool) ([]byte, error) {
	rows, err := s.buildMonthlyStats(ctx, talker, fillToNow)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM，不写 Excel 打开就是乱码
	w := csv.NewWriter(&buf)
	if err := w.Write(monthlyStatsHeader); err != nil {
		return nil, fmt.Errorf("写入CSV表头失败: %w", err)
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.YearMonth,
			strconv.Itoa(r.Year),
			strconv.Itoa(r.Month),
			strconv.Itoa(r.Count),
			strconv.Itoa(r.Cumulative),
			strconv.FormatFloat(r.SharePct, 'f', 2, 64),
		}); err != nil {
			return nil, fmt.Errorf("写入CSV数据失败: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("CSV写入错误: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportMonthlyStatsXLSX 同上，但输出 Excel：表头加粗冻结、数字列带千分位。
func (s *Service) ExportMonthlyStatsXLSX(ctx context.Context, talker, talkerName string, fillToNow bool) ([]byte, error) {
	rows, err := s.buildMonthlyStats(ctx, talker, fillToNow)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := sheetNameOf(talkerName)
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("设置工作表名失败: %w", err)
	}

	for i, h := range monthlyStatsHeader {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	if style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		f.SetCellStyle(sheet, "A1", "F1", style)
	}
	// 数字列右对齐 + 千分位，几千条的月份一眼能看出量级
	numStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("#,##0")})
	pctStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("0.00")})

	for i, r := range rows {
		row := i + 2
		f.SetCellValue(sheet, cellAt(1, row), r.YearMonth)
		f.SetCellValue(sheet, cellAt(2, row), r.Year)
		f.SetCellValue(sheet, cellAt(3, row), r.Month)
		f.SetCellValue(sheet, cellAt(4, row), r.Count)
		f.SetCellValue(sheet, cellAt(5, row), r.Cumulative)
		f.SetCellValue(sheet, cellAt(6, row), r.SharePct)
	}
	if len(rows) > 0 {
		last := len(rows) + 1
		f.SetCellStyle(sheet, cellAt(4, 2), cellAt(5, last), numStyle)
		f.SetCellStyle(sheet, cellAt(6, 2), cellAt(6, last), pctStyle)
	}

	f.SetColWidth(sheet, "A", "A", 12)
	f.SetColWidth(sheet, "B", "C", 8)
	f.SetColWidth(sheet, "D", "E", 12)
	f.SetColWidth(sheet, "F", "F", 10)
	// 冻结表头，几十行往下翻还能看见列名
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("生成XLSX失败: %w", err)
	}
	return buf.Bytes(), nil
}

func cellAt(col, row int) string {
	c, _ := excelize.CoordinatesToCellName(col, row)
	return c
}

func strPtr(s string) *string { return &s }

// sheetNameOf 把昵称收拾成合法的工作表名。
// Excel 的限制是 31 个**字符**且不能含 : \ / ? * [ ]，所以要按 rune 截，
// 按字节截会把一个汉字劈成两半，写出来的文件 Excel 直接打不开。
func sheetNameOf(name string) string {
	const forbidden = `:\/?*[]`
	out := make([]rune, 0, 31)
	for _, r := range name {
		if r < 0x20 {
			continue
		}
		for _, b := range forbidden {
			if r == b {
				r = '_'
				break
			}
		}
		out = append(out, r)
		if len(out) == 31 {
			break
		}
	}
	if len(out) == 0 {
		return "月度统计"
	}
	return string(out)
}
