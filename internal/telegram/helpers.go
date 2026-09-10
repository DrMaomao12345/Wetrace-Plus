package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// pickName 给会话挑个展示用的名字
func pickName(s *model.Session) string {
	if s.NickName != "" {
		return s.NickName
	}
	return s.UserName
}

// splitNameAndRange 将 args 分成 (姓名, 时间段串)。最后一个 token 看起来像时间段就当时间段，其余为姓名
func splitNameAndRange(args []string) (name, rangeArg string, err error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("用法：/cmd <名字> [时间段]")
	}
	if len(args) == 1 {
		return args[0], "", nil
	}
	last := args[len(args)-1]
	if looksLikeRange(last) {
		return strings.Join(args[:len(args)-1], " "), last, nil
	}
	return strings.Join(args, " "), "", nil
}

// looksLikeRange 判断 token 是否疑似时间段（用于和姓名区分）
func looksLikeRange(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "today" || s == "yesterday" || s == "all" ||
		s == "this-week" || s == "last-week" ||
		s == "this-month" || s == "last-month" ||
		s == "this-year" || s == "last-year" {
		return true
	}
	if strings.HasPrefix(s, "last-") {
		return true
	}
	// 7d / 30d / 1y
	if len(s) >= 2 {
		suffix := s[len(s)-1]
		if suffix == 'd' || suffix == 'w' || suffix == 'm' || suffix == 'y' {
			// 前缀全是数字
			allDigit := true
			for _, c := range s[:len(s)-1] {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit {
				return true
			}
		}
	}
	// 包含 - 且看起来像 2024 或 2024-01-01
	if len(s) == 4 && allDigits(s) {
		return true
	}
	if strings.Contains(s, ":") || strings.Count(s, "-") >= 2 {
		return true
	}
	return false
}

// atoiSafe 把数字字符串转 int；空 / 非法 → fallback
func atoiSafe(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
		if n > 10000 {
			return fallback
		}
	}
	return n
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolveRange 解析时间段串。空 → all（1970~now）。返回带本地时区的 start/end + 显示用 label
func resolveRange(arg string) (start, end time.Time, label string) {
	if arg == "" {
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
		end = time.Now()
		return start, end, "全部时间"
	}
	s, e, ok := util.TimeRangeOf(arg)
	if !ok {
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
		end = time.Now()
		return start, end, fmt.Sprintf("（'%s' 无法解析，已退回全部时间）", arg)
	}
	return s.Local(), e.Local(), fmt.Sprintf("%s ~ %s",
		s.Local().Format("2006-01-02"), e.Local().Format("2006-01-02"))
}

// findSession 模糊匹配会话。优先精确匹配 UserName/NickName，其次部分包含
func (b *Bot) findSession(ctx context.Context, name string) (talker, displayName string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("缺少会话名")
	}
	sessions, err := b.store.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return "", "", fmt.Errorf("获取会话失败: %w", err)
	}
	lower := strings.ToLower(name)
	// 1) 精确匹配 UserName
	for _, s := range sessions {
		if strings.EqualFold(s.UserName, name) || strings.EqualFold(s.NickName, name) {
			return s.UserName, pickName(s), nil
		}
	}
	// 2) 包含匹配（找出最短匹配优先）
	var hits []*model.Session
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.NickName), lower) || strings.Contains(strings.ToLower(s.UserName), lower) {
			hits = append(hits, s)
		}
	}
	if len(hits) == 0 {
		return "", "", fmt.Errorf("没有找到「%s」。试试 /list %s 查看候选", name, name)
	}
	if len(hits) > 1 {
		// 选最近活跃的
		best := hits[0]
		for _, h := range hits {
			if h.NTime.After(best.NTime) {
				best = h
			}
		}
		// 如果歧义太多，提示用户精确
		if len(hits) > 5 {
			var sb strings.Builder
			fmt.Fprintf(&sb, "「%s」匹配了 %d 个会话，可能太宽泛。\n命中较多的几个：\n", name, len(hits))
			top := hits
			if len(top) > 5 {
				top = top[:5]
			}
			for _, h := range top {
				fmt.Fprintf(&sb, "• %s\n", pickName(h))
			}
			return "", "", fmt.Errorf("%s", sb.String())
		}
		return best.UserName, pickName(best), nil
	}
	return hits[0].UserName, pickName(hits[0]), nil
}

// fmtNum 千分位
func fmtNum(n int) string {
	if n < 0 {
		return "-" + fmtNum(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmtNum(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

// fmtDuration 秒 → "1小时23分45秒"
func fmtDuration(sec int) string {
	if sec <= 0 {
		return "0秒"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	parts := []string{}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d小时", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%d分", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d秒", s))
	}
	return strings.Join(parts, "")
}

// typeName 把消息类型 int 转中文
func typeName(t int) string {
	switch t {
	case 1:
		return "文本"
	case 3:
		return "图片"
	case 34:
		return "语音"
	case 42:
		return "名片"
	case 43:
		return "视频"
	case 47:
		return "表情"
	case 48:
		return "位置"
	case 49:
		return "链接/文件"
	case 50:
		return "通话"
	case 99:
		return "其他"
	case 10000:
		return "系统"
	default:
		return fmt.Sprintf("类型%d", t)
	}
}
