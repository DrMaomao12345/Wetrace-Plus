import { useMemo, useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { bizApi, type BizAccount } from "@/api/biz"
import { Newspaper, BellOff, Clock, TrendingUp, Hash } from "lucide-react"

const CUR = new Date().getFullYear()
const YEARS = Array.from({ length: CUR - 2021 }, (_, i) => CUR - i)

function fmtDate(ts: number): string {
  if (!ts) return "从无推送"
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function daysAgo(ts: number): string {
  if (!ts) return ""
  const d = Math.floor((Date.now() / 1000 - ts) / 86400)
  if (d < 1) return "今天"
  if (d < 30) return `${d} 天前`
  if (d < 365) return `${Math.floor(d / 30)} 个月前`
  return `${(d / 365).toFixed(1)} 年前`
}

function Card({
  title,
  icon,
  extra,
  children,
}: {
  title: string
  icon?: ReactNode
  extra?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="rounded-2xl border border-border bg-card p-5 shadow-sm">
      <div className="mb-4 flex items-center gap-2">
        {icon}
        <h3 className="text-base font-semibold">{title}</h3>
        <div className="ml-auto">{extra}</div>
      </div>
      {children}
    </div>
  )
}

function Stat({ label, value, sub }: { label: string; value: ReactNode; sub?: string }) {
  return (
    <div className="rounded-xl border border-border bg-background/50 p-4">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      {sub && <div className="mt-0.5 text-[11px] text-muted-foreground">{sub}</div>}
    </div>
  )
}

function AccountRow({ acc, max, showSilent }: { acc: BizAccount; max: number; showSilent?: boolean }) {
  const pct = max > 0 ? (acc.push_count / max) * 100 : 0
  return (
    <div className="flex items-center gap-3 py-1.5">
      <div className="w-40 shrink-0 truncate text-sm" title={acc.name}>
        {acc.name}
      </div>
      <span className="shrink-0 rounded border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground">
        {acc.type_label}
      </span>
      {showSilent ? (
        <div className="flex-1 text-xs text-muted-foreground">
          最后推送 {fmtDate(acc.lifetime_last)}
          {acc.lifetime_last > 0 && <span className="ml-2 opacity-60">({daysAgo(acc.lifetime_last)})</span>}
        </div>
      ) : (
        <>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
            <div className="h-full rounded-full bg-blue-500/80" style={{ width: `${pct}%` }} />
          </div>
          <div className="w-16 shrink-0 text-right text-sm tabular-nums">{acc.push_count}</div>
          <div className="w-20 shrink-0 text-right text-[11px] text-muted-foreground">
            {acc.active_months} 个月活跃
          </div>
        </>
      )}
    </div>
  )
}

export default function BizProfileView() {
  const [year, setYear] = useState<number>(CUR)
  const [showAllTop, setShowAllTop] = useState(false)
  const [showAllSilent, setShowAllSilent] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ["biz-profile", year],
    queryFn: () => bizApi.getProfile(year),
  })

  const top = data?.top_accounts ?? []
  const silent = data?.silent_top ?? []
  const maxPush = top[0]?.push_count ?? 0

  const hourly = data?.hourly ?? []
  const maxHour = useMemo(() => Math.max(1, ...hourly.map((h) => h.count)), [hourly])
  const monthly = data?.monthly ?? []
  const maxMonth = useMemo(() => Math.max(1, ...monthly.map((m) => m.count)), [monthly])
  const maxKw = data?.title_keywords?.[0]?.count ?? 1

  if (isLoading) {
    return <div className="p-8 text-center text-sm text-muted-foreground">正在统计公众号推送…</div>
  }

  if (!data?.has_data) {
    return (
      <div className="mx-auto max-w-2xl p-8 text-center">
        <Newspaper className="mx-auto mb-3 h-10 w-10 text-muted-foreground/40" />
        <h2 className="text-lg font-semibold">没有找到公众号数据</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          公众号推送存放在 <code className="rounded bg-muted px-1">biz_message_*.db</code>，
          需要解密后才能统计。如果你用的是旧版微信，可能没有这些库。
        </p>
      </div>
    )
  }

  const o = data.overview
  // 所有「这段时间」的表述都指向当前选中的统计区间，避免歧义
  const periodLabel = year > 0 ? `${year} 年内` : '全部历史里'
  const periodRange =
    year > 0
      ? `${year}-01-01 ~ ${year === CUR ? '今天' : `${year}-12-31`}`
      : '有记录以来的全部时间'

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-5xl space-y-5 p-6 pb-20">
        <div className="flex items-center gap-3">
          <div>
            <h2 className="text-2xl font-bold tracking-tight">公众号画像</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              这些推送不计入任何聊天统计，是一份独立的订阅数据
            </p>
          </div>
          <select
            value={year}
            onChange={(e) => setYear(Number(e.target.value))}
            className="ml-auto h-9 rounded-md border border-input bg-background px-2 text-sm"
          >
            <option value={0}>全部历史</option>
            {YEARS.map((y) => (
              <option key={y} value={y}>
                {y} 年
              </option>
            ))}
          </select>
        </div>

        {/* 总览 */}
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <Stat
            label="收到推送"
            value={o.total_pushes.toLocaleString()}
            sub={`日均 ${o.daily_avg.toFixed(1)} 条 · ${periodRange}`}
          />
          <Stat
            label="有推送的号"
            value={o.pushing_accounts}
            sub={`${periodLabel}推过 · 共关注 ${o.followed_total} 个`}
          />
          <Stat label="沉默订阅" value={o.silent_accounts} sub={`${periodLabel}一条没推`} />
          <Stat
            label="推送高峰"
            value={`${o.peak_hour}:00`}
            sub={o.busiest_date ? `最猛 ${o.busiest_date} ${o.busiest_count} 条` : undefined}
          />
        </div>

        {/* 订阅号 vs 服务号 */}
        <Card title="订阅号 vs 服务号" icon={<TrendingUp className="h-4 w-4 text-blue-500" />}>
          <div className="flex h-6 overflow-hidden rounded-lg">
            <div
              className="flex items-center justify-center bg-amber-500/80 text-[11px] text-white"
              style={{ width: `${(o.subscription_cnt / Math.max(1, o.total_pushes)) * 100}%` }}
            >
              {o.subscription_cnt > 0 && `订阅号 ${o.subscription_cnt}`}
            </div>
            <div
              className="flex items-center justify-center bg-sky-500/80 text-[11px] text-white"
              style={{ width: `${(o.service_cnt / Math.max(1, o.total_pushes)) * 100}%` }}
            >
              {o.service_cnt > 0 && `服务号 ${o.service_cnt}`}
            </div>
          </div>
        </Card>

        {/* 月度趋势 */}
        {monthly.length > 0 && (
          <Card title="推送月度趋势" icon={<TrendingUp className="h-4 w-4 text-blue-500" />}>
            <div className="flex h-36 gap-1">
              {monthly.map((m) => (
                <div key={m.month} className="group flex h-full flex-1 flex-col items-center">
                  {/* 柱子的百分比高度要有一个确定高度的父容器才生效 */}
                  <div className="flex w-full flex-1 items-end">
                    <div
                      className="w-full rounded-t bg-blue-500/70 transition-colors group-hover:bg-blue-500"
                      style={{ height: `${Math.max(2, (m.count / maxMonth) * 100)}%` }}
                      title={`${m.month}: ${m.count} 条`}
                    />
                  </div>
                  <span className="mt-1 shrink-0 text-[9px] text-muted-foreground">{m.month.slice(5)}</span>
                </div>
              ))}
            </div>
          </Card>
        )}

        {/* 小时分布 */}
        <Card title="它们什么时候推给你" icon={<Clock className="h-4 w-4 text-blue-500" />}>
          <div className="flex h-28 gap-0.5">
            {hourly.map((h) => (
              <div key={h.hour} className="flex h-full flex-1 flex-col items-center">
                <div className="flex w-full flex-1 items-end">
                  <div
                    className="w-full rounded-t bg-blue-500/60"
                    style={{ height: `${Math.max(2, (h.count / maxHour) * 100)}%` }}
                    title={`${h.hour}:00 — ${h.count} 条`}
                  />
                </div>
                <span className="mt-1 h-3 shrink-0 text-[9px] text-muted-foreground">
                  {h.hour % 3 === 0 ? h.hour : ''}
                </span>
              </div>
            ))}
          </div>
        </Card>

        {/* 推送排行 */}
        <Card
          title="谁在轰炸你"
          icon={<Newspaper className="h-4 w-4 text-blue-500" />}
          extra={
            top.length > 15 && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={() => setShowAllTop((v) => !v)}
              >
                {showAllTop ? "收起" : `展开全部 ${top.length}`}
              </button>
            )
          }
        >
          {top.length === 0 ? (
            <p className="text-sm text-muted-foreground">这段时间没有收到任何推送。</p>
          ) : (
            <div className="divide-y divide-border/50">
              {(showAllTop ? top : top.slice(0, 15)).map((a) => (
                <AccountRow key={a.talker} acc={a} max={maxPush} />
              ))}
            </div>
          )}
        </Card>

        {/* 沉默订阅 */}
        <Card
          title={`沉默的订阅（${year > 0 ? `${year} 年` : '全部历史'}）`}
          icon={<BellOff className="h-4 w-4 text-muted-foreground" />}
          extra={
            silent.length > 15 && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={() => setShowAllSilent((v) => !v)}
              >
                {showAllSilent ? "收起" : `展开全部 ${silent.length}`}
              </button>
            )
          }
        >
          <p className="mb-3 text-xs text-muted-foreground">
            关注着、但<strong className="text-foreground">在 {periodRange} 这段时间里</strong>一条都没推的号，
            按「最后一次推送」由近到远排，越往下越可以考虑取关。
          </p>
          {silent.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {year > 0
                ? `${year} 年里每个关注的号都至少推过一条。`
                : '按「全部历史」统计时，每个号都至少推过一条，所以没有沉默的订阅 —— 想找可以取关的号，请在右上角选择某一年。'}
            </p>
          ) : (
            <div className="divide-y divide-border/50">
              {(showAllSilent ? silent : silent.slice(0, 15)).map((a) => (
                <AccountRow key={a.talker} acc={a} max={maxPush} showSilent />
              ))}
            </div>
          )}
        </Card>

        {/* 标题关键词 */}
        {(data.title_keywords?.length ?? 0) > 0 && (
          <Card title="推文标题里最常出现的词" icon={<Hash className="h-4 w-4 text-blue-500" />}>
            <div className="flex flex-wrap gap-2">
              {data.title_keywords!.map((w) => {
                const scale = 0.85 + (w.count / maxKw) * 0.9
                return (
                  <span
                    key={w.text}
                    className="rounded-full border border-border px-2.5 py-1 text-muted-foreground"
                    style={{ fontSize: `${scale * 0.8}rem` }}
                    title={`${w.count} 次`}
                  >
                    {w.text}
                  </span>
                )
              })}
            </div>
          </Card>
        )}
      </div>
    </div>
  )
}
