package telegram

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// commandHandler 接收解析后的参数，返回回复文本或错误
type commandHandler func(ctx context.Context, args []string) (string, error)

type commandRegistry struct {
	bot    *Bot
	cmds   map[string]commandHandler
	names  []string // 注册顺序
	titles map[string]string
}

func newRegistry(b *Bot) *commandRegistry {
	r := &commandRegistry{
		bot:    b,
		cmds:   make(map[string]commandHandler),
		titles: make(map[string]string),
	}
	r.register("help", "显示帮助", b.cmdHelp)
	r.register("status", "系统状态（数据版本、活跃会话数、最近一条消息时间）", b.cmdStatus)
	r.register("list", "/list [关键词|数字]  列会话；纯数字 N → 聊天 Top N 排行", b.cmdList)
	r.register("stat", "/stat <名字> [时间段]  概览：总数/发送/接收/活跃天", b.cmdStat)
	r.register("words", "/words <名字> [时间段]  字数统计：总字数/发送/接收", b.cmdWords)
	r.register("calls", "/calls <名字>  通话统计：次数/总时长/语音/视频", b.cmdCalls)
	r.register("types", "/types <名字>  消息类型分布（仅展示前 8 种）", b.cmdTypes)
	return r
}

func (r *commandRegistry) register(name, title string, h commandHandler) {
	r.cmds[name] = h
	r.titles[name] = title
	r.names = append(r.names, name)
}

func (r *commandRegistry) get(name string) (commandHandler, bool) {
	h, ok := r.cmds[name]
	return h, ok
}

// ---------- /help ----------

func (b *Bot) cmdHelp(_ context.Context, _ []string) (string, error) {
	var sb strings.Builder
	sb.WriteString("📖 WeTrace Bot 命令清单\n\n")
	for _, name := range b.registry.names {
		fmt.Fprintf(&sb, "/%s — %s\n", name, b.registry.titles[name])
	}
	sb.WriteString("\n⏱ 时间段写法（可省略）：\n")
	sb.WriteString("• 相对：7d / 30d / 90d / 1y\n")
	sb.WriteString("• 命名：today / yesterday / this-week / last-week\n")
	sb.WriteString("        this-month / last-month / this-year / last-year\n")
	sb.WriteString("• 具体年份：2024\n")
	sb.WriteString("• 自定义：2024-01-01:2024-12-31\n")
	sb.WriteString("\n例：/stat 张三 7d  /words 老妈 last-month")
	return sb.String(), nil
}

// ---------- /status ----------

func (b *Bot) cmdStatus(ctx context.Context, _ []string) (string, error) {
	// 数据指纹
	dv := b.store.GetDataVersion()
	dvShort := dv
	if len(dvShort) > 12 {
		dvShort = dvShort[:12]
	}

	// 活跃会话数
	sessions, _ := b.store.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	totalSessions := len(sessions)
	groups, contacts := 0, 0
	var lastMsg time.Time
	for _, s := range sessions {
		if strings.HasSuffix(s.UserName, "@chatroom") {
			groups++
		} else {
			contacts++
		}
		if s.NTime.After(lastMsg) {
			lastMsg = s.NTime
		}
	}
	lastMsgStr := "无"
	if !lastMsg.IsZero() {
		lastMsgStr = lastMsg.Local().Format("2006-01-02 15:04:05")
	}

	// 进程信息
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var sb strings.Builder
	sb.WriteString("🟢 WeTrace 系统状态\n\n")
	fmt.Fprintf(&sb, "数据指纹：%s\n", dvShort)
	fmt.Fprintf(&sb, "总会话：%d（私聊 %d / 群聊 %d）\n", totalSessions, contacts, groups)
	fmt.Fprintf(&sb, "最近一条消息：%s\n", lastMsgStr)
	fmt.Fprintf(&sb, "服务器时间：%s\n", time.Now().Format("2006-01-02 15:04:05 -0700"))
	fmt.Fprintf(&sb, "Go 版本：%s / %s\n", runtime.Version(), runtime.GOOS)
	fmt.Fprintf(&sb, "内存占用：%.1f MB\n", float64(ms.Alloc)/1024/1024)
	return sb.String(), nil
}

// ---------- /list ----------

func (b *Bot) cmdList(ctx context.Context, args []string) (string, error) {
	// 纯数字参数 → 走亲密度 Top N 排行
	if len(args) == 1 && allDigits(args[0]) {
		n := atoiSafe(args[0], 10)
		if n <= 0 {
			n = 10
		}
		if n > 50 {
			n = 50
		}
		return b.listTopContacts(ctx, n)
	}
	keyword := strings.Join(args, " ")
	sessions, err := b.store.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return "", fmt.Errorf("获取会话失败: %w", err)
	}
	type item struct {
		name, talker string
		isGroup      bool
	}
	var items []item
	kw := strings.ToLower(strings.TrimSpace(keyword))
	for _, s := range sessions {
		name := pickName(s)
		if kw != "" && !strings.Contains(strings.ToLower(name), kw) && !strings.Contains(strings.ToLower(s.UserName), kw) {
			continue
		}
		items = append(items, item{name: name, talker: s.UserName, isGroup: strings.HasSuffix(s.UserName, "@chatroom")})
	}
	if len(items) == 0 {
		return "没有匹配的会话。试试 /list 不带关键词，或换个词。", nil
	}
	if len(items) > 30 {
		items = items[:30]
	}
	var sb strings.Builder
	if kw != "" {
		fmt.Fprintf(&sb, "🔎 \"%s\" 匹配 %d 条（最多展示 30 条）\n\n", keyword, len(items))
	} else {
		fmt.Fprintf(&sb, "📋 会话列表（最多 30 条，关键词过滤更精确）\n\n")
	}
	for _, it := range items {
		tag := ""
		if it.isGroup {
			tag = " [群]"
		}
		fmt.Fprintf(&sb, "• %s%s\n", it.name, tag)
	}
	sb.WriteString("\n用 /stat <名字> 查看统计")
	return sb.String(), nil
}

// listTopContacts 实现 /list <N>：拿亲密度排行
func (b *Bot) listTopContacts(ctx context.Context, n int) (string, error) {
	top, err := b.store.GetPersonalTopContacts(ctx, n)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	if len(top) == 0 {
		return "暂无消息数据，无法统计排行。", nil
	}
	if n > len(top) {
		n = len(top)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "🏆 聊天 Top %d 排行（按总消息数）\n\n", n)
	for i, c := range top[:n] {
		tag := ""
		if strings.HasSuffix(c.Talker, "@chatroom") {
			tag = " [群]"
		}
		name := c.Name
		if name == "" {
			name = c.Talker
		}
		fmt.Fprintf(&sb, "%d. %s%s — %s 条（发 %s / 收 %s）\n",
			i+1, name, tag,
			fmtNum(c.MessageCount),
			fmtNum(c.SentCount),
			fmtNum(c.RecvCount),
		)
	}
	sb.WriteString("\n用 /stat <名字> 查看单个会话详情")
	return sb.String(), nil
}

// ---------- /stat ----------

func (b *Bot) cmdStat(ctx context.Context, args []string) (string, error) {
	name, rangeArg, err := splitNameAndRange(args)
	if err != nil {
		return "", err
	}
	talker, displayName, err := b.findSession(ctx, name)
	if err != nil {
		return "", err
	}
	start, end, label := resolveRange(rangeArg)

	// 用 daily 统计算总数，hourly 求未实现的分发；这里走一个加权汇总
	daily, err := b.store.GetDailyActivity(ctx, talker)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")
	var total int
	var activeDays int
	var firstDate, lastDate string
	for _, d := range daily {
		if d.Date < startStr || d.Date > endStr {
			continue
		}
		total += d.Count
		activeDays++
		if firstDate == "" || d.Date < firstDate {
			firstDate = d.Date
		}
		if d.Date > lastDate {
			lastDate = d.Date
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 %s — %s\n\n", displayName, label)
	if total == 0 {
		sb.WriteString("此时间段内没有消息记录。")
		return sb.String(), nil
	}
	fmt.Fprintf(&sb, "总消息数：%s\n", fmtNum(total))
	fmt.Fprintf(&sb, "活跃天数：%d\n", activeDays)
	if activeDays > 0 {
		fmt.Fprintf(&sb, "日均消息：%s\n", fmtNum(total/activeDays))
	}
	if firstDate != "" {
		fmt.Fprintf(&sb, "首条：%s\n末条：%s\n", firstDate, lastDate)
	}
	sb.WriteString("\n继续查看：/words 字数  /calls 通话  /types 消息类型")
	return sb.String(), nil
}

// ---------- /words ----------

func (b *Bot) cmdWords(ctx context.Context, args []string) (string, error) {
	name, _, err := splitNameAndRange(args)
	if err != nil {
		return "", err
	}
	talker, displayName, err := b.findSession(ctx, name)
	if err != nil {
		return "", err
	}
	// 字数当前只有"整年 + 排除"接口；这里用当前年快速查一份
	wc, err := b.store.GetAnnualWordCounts(ctx, time.Now().Year(), 0, nil, nil)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	for _, c := range wc.Contacts {
		if c.Talker == talker {
			var sb strings.Builder
			fmt.Fprintf(&sb, "✍️ %s — %d 年字数统计\n\n", displayName, time.Now().Year())
			fmt.Fprintf(&sb, "总字数：%s\n", fmtNum(c.TotalChars))
			fmt.Fprintf(&sb, "发送：%s 字\n", fmtNum(c.SentChars))
			fmt.Fprintf(&sb, "接收：%s 字\n", fmtNum(c.RecvChars))
			return sb.String(), nil
		}
	}
	return fmt.Sprintf("%s — %d 年没有文本消息记录。", displayName, time.Now().Year()), nil
}

// ---------- /calls ----------

func (b *Bot) cmdCalls(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("用法：/calls <名字>")
	}
	name := strings.Join(args, " ")
	talker, displayName, err := b.findSession(ctx, name)
	if err != nil {
		return "", err
	}
	stats, err := b.store.GetCallStats(ctx, talker, time.Time{}, time.Time{})
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	if stats.TotalCalls == 0 {
		return fmt.Sprintf("%s — 没有通话记录。", displayName), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📞 %s — 通话统计\n\n", displayName)
	fmt.Fprintf(&sb, "总通话：%d 次（接通 %d，未接 %d）\n", stats.TotalCalls, stats.CompletedCalls, stats.MissedCalls)
	fmt.Fprintf(&sb, "语音通话：%d  视频通话：%d\n", stats.VoiceCalls, stats.VideoCalls)
	fmt.Fprintf(&sb, "总时长：%s\n", fmtDuration(stats.TotalDuration))
	fmt.Fprintf(&sb, "最长一次：%s\n", fmtDuration(stats.LongestDuration))
	fmt.Fprintf(&sb, "平均时长：%s", fmtDuration(stats.AvgDuration))
	return sb.String(), nil
}

// ---------- /types ----------

func (b *Bot) cmdTypes(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("用法：/types <名字>")
	}
	name := strings.Join(args, " ")
	talker, displayName, err := b.findSession(ctx, name)
	if err != nil {
		return "", err
	}
	stats, err := b.store.GetMessageTypeDistribution(ctx, talker)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	if len(stats) == 0 {
		return fmt.Sprintf("%s — 没有消息记录。", displayName), nil
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	var sb strings.Builder
	fmt.Fprintf(&sb, "📦 %s — 消息类型分布\n\n", displayName)
	var total int
	for _, s := range stats {
		total += s.Count
	}
	for i, s := range stats {
		if i >= 8 {
			break
		}
		pct := 0.0
		if total > 0 {
			pct = float64(s.Count) * 100 / float64(total)
		}
		fmt.Fprintf(&sb, "%s：%s (%.1f%%)\n", typeName(s.Type), fmtNum(s.Count), pct)
	}
	return sb.String(), nil
}
