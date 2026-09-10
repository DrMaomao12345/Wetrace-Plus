package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util/zstd"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/wordcloud"
	"github.com/DrMaomao12345/Wetrace-Plus/store/strategy"
)

// bizTitleCap 是提取标题时最多解压的推送条数 —— 全量解压 7 万条太慢，
// 取样本足够算出稳定的高频词。
const bizTitleCap = 20000

// reTitle 从 appmsg XML 里抠标题。公众号推文的 <title> 就是文章标题。
var reTitle = regexp.MustCompile(`(?s)<title>(.*?)</title>`)

// bizAgg 是单个公众号的聚合中间态
type bizAgg struct {
	count        int
	first, last  int64
	lifetimeLast int64
	months       map[string]bool
}

// GetBizProfile 生成公众号订阅画像。
// year <= 0 表示统计全部历史。
func (r *Repository) GetBizProfile(ctx context.Context, year, tzOffsetSec int, withTitles bool) *model.BizProfile {
	out := &model.BizProfile{Year: year}

	paths, err := r.router.GetAllDBPaths(strategy.BizMessage)
	if err != nil || len(paths) == 0 {
		return out // 没有 biz 库（老版本微信或未解密）
	}
	out.HasData = true

	loc := time.FixedZone("biz", tzOffsetSec)
	var start, end time.Time
	if year > 0 {
		start = time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		end = time.Date(year, 12, 31, 23, 59, 59, 0, loc)
	} else {
		start = time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Now()
	}

	// 公众号用户名 → md5 表名后缀 的反查表（取自联系人库，而不是会话列表，
	// 因为很多只推送不聊天的号根本不在会话里）
	autoTypes := r.TalkerTypes(ctx)
	md5ToTalker := make(map[string]string, len(autoTypes))
	for talker := range autoTypes {
		h := md5.Sum([]byte(talker))
		md5ToTalker[hex.EncodeToString(h[:])] = talker
	}

	allow := r.TalkerFilter(ctx, model.ModuleBiz)

	aggs := make(map[string]*bizAgg)
	monthly := map[string]int{}
	hourly := make([]int, 24)
	daily := map[string]int{}
	var titles []string

	for _, path := range paths {
		db, err := r.pool.GetConnection(path)
		if err != nil {
			continue
		}
		r.scanBizShard(ctx, db, md5ToTalker, allow, start, end,
			aggs, monthly, hourly, daily, &titles, withTitles)
	}

	// 补名称 / 头像 / 类型
	talkers := make([]string, 0, len(aggs))
	for t := range aggs {
		talkers = append(talkers, t)
	}
	profiles, _ := r.getContactProfiles(ctx, talkers)

	accounts := make([]*model.BizAccount, 0, len(aggs))
	for talker, a := range aggs {
		acc := &model.BizAccount{
			Talker:       talker,
			Name:         talker,
			PushCount:    a.count,
			FirstTime:    a.first,
			LastTime:     a.last,
			LifetimeLast: a.lifetimeLast,
			ActiveMonths: len(a.months),
		}
		if p, ok := profiles[talker]; ok {
			if p.Remark != "" {
				acc.Name = p.Remark
			} else if p.NickName != "" {
				acc.Name = p.NickName
			}
			acc.Avatar = p.SmallHeadURL
		}
		tt := r.TalkerTypeOf(ctx, talker)
		acc.Type, acc.TypeLabel = string(tt), tt.Label()
		if acc.ActiveMonths > 0 {
			acc.MonthlyAvg = float64(acc.PushCount) / float64(acc.ActiveMonths)
		}
		accounts = append(accounts, acc)
	}

	// 区间内有推送的 / 沉默的
	pushing := make([]*model.BizAccount, 0, len(accounts))
	silent := make([]*model.BizAccount, 0, len(accounts))
	for _, a := range accounts {
		if a.PushCount > 0 {
			pushing = append(pushing, a)
		} else {
			silent = append(silent, a)
		}
	}

	sort.Slice(pushing, func(i, j int) bool { return pushing[i].PushCount > pushing[j].PushCount })
	// 沉默的按「最后一次推送」由近到远，越久没动静越排后面
	sort.Slice(silent, func(i, j int) bool { return silent[i].LifetimeLast > silent[j].LifetimeLast })

	out.TopAccounts = pushing
	if len(out.TopAccounts) > 100 {
		out.TopAccounts = out.TopAccounts[:100]
	}
	out.SilentTop = silent
	if len(out.SilentTop) > 100 {
		out.SilentTop = out.SilentTop[:100]
	}

	// 总览
	ov := &out.Overview
	ov.PushingAccounts = len(pushing)
	ov.SilentAccounts = len(silent)
	ov.FollowedTotal = len(accounts)
	for _, a := range pushing {
		ov.TotalPushes += a.PushCount
		switch a.Type {
		case string(model.TalkerService):
			ov.ServiceCnt += a.PushCount
		default:
			ov.SubscriptionCnt += a.PushCount
		}
	}
	days := end.Sub(start).Hours() / 24
	if days < 1 {
		days = 1
	}
	if year > 0 && time.Now().Year() == year {
		// 当年只算到今天，否则日均会被未来的日子摊薄
		if d := time.Since(start).Hours() / 24; d >= 1 {
			days = d
		}
	}
	ov.DailyAvg = float64(ov.TotalPushes) / days

	peak, peakCnt := 0, -1
	for h, c := range hourly {
		if c > peakCnt {
			peak, peakCnt = h, c
		}
	}
	ov.PeakHour = peak
	for d, c := range daily {
		if c > ov.BusiestCount {
			ov.BusiestDate, ov.BusiestCount = d, c
		}
	}

	// 月度
	months := make([]string, 0, len(monthly))
	for m := range monthly {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		out.Monthly = append(out.Monthly, &model.BizMonthStat{Month: m, Count: monthly[m]})
	}

	// 小时
	for h := 0; h < 24; h++ {
		out.Hourly = append(out.Hourly, &model.BizHourStat{Hour: h, Count: hourly[h]})
	}

	// 标题高频词
	if withTitles && len(titles) > 0 {
		res := wordcloud.Analyze(titles, 80)
		for _, w := range res.Words {
			out.TitleKeywords = append(out.TitleKeywords, &model.BizKeyword{Text: w.Text, Count: w.Count})
		}
	}

	return out
}

// scanBizShard 扫一个 biz 分片库里的所有公众号消息表
func (r *Repository) scanBizShard(ctx context.Context, db *sql.DB,
	md5ToTalker map[string]string, allow func(string) bool,
	start, end time.Time,
	aggs map[string]*bizAgg, monthly map[string]int, hourly []int, daily map[string]int,
	titles *[]string, withTitles bool) {

	tables := r.listMsgTables(ctx, db)
	for _, tbl := range tables {
		talker, ok := md5ToTalker[strings.TrimPrefix(tbl, "Msg_")]
		if !ok {
			continue // 联系人库里查不到的号（已取关且清理）直接跳过
		}
		if !allow(talker) {
			continue
		}

		a := aggs[talker]
		if a == nil {
			a = &bizAgg{months: map[string]bool{}}
			aggs[talker] = a
		}

		// 全历史最后一次推送 —— 沉默订阅榜要用
		var lifetimeLast sql.NullInt64
		_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT MAX(create_time) FROM %s", tbl)).Scan(&lifetimeLast)
		if lifetimeLast.Valid && lifetimeLast.Int64 > a.lifetimeLast {
			a.lifetimeLast = lifetimeLast.Int64
		}

		rows, err := db.QueryContext(ctx,
			fmt.Sprintf("SELECT create_time FROM %s WHERE create_time >= ? AND create_time <= ?", tbl),
			start.Unix(), end.Unix())
		if err != nil {
			continue
		}
		loc := start.Location()
		for rows.Next() {
			var ts int64
			if rows.Scan(&ts) != nil {
				continue
			}
			a.count++
			if a.first == 0 || ts < a.first {
				a.first = ts
			}
			if ts > a.last {
				a.last = ts
			}
			t := time.Unix(ts, 0).In(loc)
			m := t.Format("2006-01")
			a.months[m] = true
			monthly[m]++
			hourly[t.Hour()]++
			daily[t.Format("2006-01-02")]++
		}
		rows.Close()

		if withTitles && len(*titles) < bizTitleCap {
			r.collectBizTitles(ctx, db, tbl, start, end, titles)
		}
	}
}

// collectBizTitles 解压推文正文并抠出标题
func (r *Repository) collectBizTitles(ctx context.Context, db *sql.DB, tbl string,
	start, end time.Time, titles *[]string) {

	limit := bizTitleCap - len(*titles)
	if limit <= 0 {
		return
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"SELECT message_content FROM %s WHERE create_time >= ? AND create_time <= ? LIMIT %d",
		tbl, limit), start.Unix(), end.Unix())
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil || len(raw) == 0 {
			continue
		}
		content := raw
		if b, err := zstd.Decompress(raw); err == nil {
			content = b
		}
		m := reTitle.FindSubmatch(content)
		if m == nil {
			continue
		}
		title := strings.TrimSpace(string(m[1]))
		title = strings.TrimPrefix(title, "<![CDATA[")
		title = strings.TrimSuffix(title, "]]>")
		if title != "" {
			*titles = append(*titles, title)
		}
	}
}
