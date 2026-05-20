package api

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// PrewarmTopContacts 服务启动后台跑一次：拉当年「全局年度报告」+ Top5 亲密度
// 联系人各自的年度报告，写入 ReportCache。下次进入年度报告页可秒开。
//
// 设计要点（避免数据错乱 / 抢占首屏资源）：
//   - 延迟 30 秒启动 —— 让 App 首屏请求先到先服务，再做后台预热。
//   - 单 goroutine 串行执行 —— 不并发查同一份 sqlite 库，避免锁争抢和潜在的
//     游标错乱。
//   - 整份 prewarm 套一个 10 分钟 ctx 上限 —— 极端慢盘也不让它无限挂着。
//   - 每写一份缓存前再检查 ReportCache（可能在等待期间用户已经触发并缓存了
//     同一份），避免重复算。
//   - context 取消 / 数据指纹变了 都会让循环提前退出，让位给真实请求。
func (a *API) PrewarmTopContacts() {
	go func() {
		// 让首屏请求先跑
		select {
		case <-time.After(30 * time.Second):
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		year := time.Now().Year()
		defaultTzSec := 0
		if off, ok := defaultTzOffsetMinutes(); ok {
			defaultTzSec = off * 60
		}
		pastStartYear := effectiveChatStartYear()
		exclude := a.mergedExcludeTalkers(nil)

		// 1) 全局年度报告
		version := a.Store.GetDataVersion()
		gKey := AnnualReportKey(year, defaultTzSec, pastStartYear, exclude, nil)
		if a.ReportCache.Get(version, gKey) == nil {
			log.Info().Int("year", year).Msg("[prewarm] 计算全局年度报告")
			report, err := a.Store.GetAnnualReport(ctx, year, defaultTzSec, pastStartYear, nil, exclude)
			if err != nil {
				log.Warn().Err(err).Msg("[prewarm] 全局年度报告失败，跳过 Top5 预热")
				return
			}
			a.ReportCache.Put(version, gKey, report)
		}
		if ctx.Err() != nil {
			return
		}

		// 2) Top5 亲密度联系人各自的年度报告（串行，避免 DB 抢占）
		global := a.ReportCache.Get(a.Store.GetDataVersion(), gKey)
		if global == nil {
			return
		}
		top := global.TopContacts
		if len(top) > 5 {
			top = top[:5]
		}
		for _, tc := range top {
			if ctx.Err() != nil {
				return
			}
			ver := a.Store.GetDataVersion()
			key := TalkerReportKey(year, tc.Talker, defaultTzSec)
			if a.ReportCache.Get(ver, key) != nil {
				continue
			}
			log.Info().Int("year", year).Str("talker", tc.Talker).Msg("[prewarm] 计算联系人年度报告")
			r, err := a.Store.GetTalkerAnnualReport(ctx, year, tc.Talker, defaultTzSec)
			if err != nil {
				log.Warn().Err(err).Str("talker", tc.Talker).Msg("[prewarm] 联系人年度报告失败")
				continue
			}
			a.ReportCache.Put(ver, key, r)
		}
		log.Info().Msg("[prewarm] 完成")
	}()
}
