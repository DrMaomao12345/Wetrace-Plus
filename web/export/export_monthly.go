package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

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

// buildMonthlyStats 取某个会话按年-月聚合的消息数，并把中间的空月补成 0。
//
// 补 0 是必须的：底层是 GROUP BY，一条消息都没有的月份根本不会有行，直接导出
// 会得到一份「跳月」的表 —— 拿去画图或者做同比全是坑。
//
// 但两头不外扩：比第一条消息更早的月份，联系人还没加上；比最后一条更晚的月份，
// 要么还没发生、要么已经没有记录。那些 0 是假的，不该凭空造出来
// （和月度趋势图裁掉两头是同一个道理）。
func (s *Service) buildMonthlyStats(ctx context.Context, talker string) ([]MonthlyStatRow, error) {
	stats, err := s.Store.GetYearlyMonthlyActivity(ctx, talker)
	if err != nil {
		return nil, err
	}

	// key = year*12 + month，把年月压成一个能直接自增的整数，补空月才好写
	byKey := make(map[int]int, len(stats))
	minKey, maxKey, total := 0, 0, 0
	for _, st := range stats {
		if st == nil || st.Month < 1 || st.Month > 12 || st.Year < 2000 || st.Year > 2100 {
			continue
		}
		k := st.Year*12 + st.Month
		byKey[k] += st.Count
		total += st.Count
		if minKey == 0 || k < minKey {
			minKey = k
		}
		if k > maxKey {
			maxKey = k
		}
	}
	if minKey == 0 {
		return nil, nil
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
func (s *Service) ExportMonthlyStatsCSV(ctx context.Context, talker string) ([]byte, error) {
	rows, err := s.buildMonthlyStats(ctx, talker)
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
func (s *Service) ExportMonthlyStatsXLSX(ctx context.Context, talker, talkerName string) ([]byte, error) {
	rows, err := s.buildMonthlyStats(ctx, talker)
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
