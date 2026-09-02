import { useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { insightsApi, type CommonGroup, type InteractionRatio, type ReplySpeed } from '@/api/insights'
import { CalendarDays, Users2, Clock, ArrowLeftRight, X, Sun, Send, Inbox } from 'lucide-react'

const CUR = new Date().getFullYear()
const YEARS = Array.from({ length: CUR - 2021 }, (_, i) => CUR - i) // 当前年 → 2022

function fmtDur(sec: number): string {
  if (!sec || sec <= 0) return '—'
  if (sec < 60) return `${sec}秒`
  if (sec < 3600) return `${Math.round(sec / 60)}分钟`
  return `${(sec / 3600).toFixed(1)}小时`
}

function Card({ title, icon, extra, children }: { title: string; icon?: ReactNode; extra?: ReactNode; children: ReactNode }) {
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

// ── 功能9 日历热力图 ────────────────────────────────
/** 热力图悬浮卡片：显示这段时间和谁聊过、各聊了多少天。
 *
 *  三个要点：
 *  - **fixed 定位**：热力图外层是 `overflow-x-auto`，absolute 会被裁掉。
 *    用 fixed + 目标元素的 getBoundingClientRect 才能真正浮在最上层、冲出图表范围。
 *  - **自身滚动 + overscroll-contain**：列表滚到底时不把滚动传给页面，
 *    否则背后的热力图会跟着动。
 *  - 数据按需拉取，react-query 缓存，来回移不重复打接口。
 */
function HeatmapPartnersPopup({
  year, month, day, anchor,
}: {
  year: number
  month: number
  day?: number
  anchor: { left: number; top: number }
}) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-heatmap-partners', year, month, day ?? 0],
    queryFn: () => insightsApi.heatmapPartners(year, month, day),
    staleTime: 5 * 60 * 1000,
  })

  const W = 300
  // 贴着锚点下方弹出；靠近视口右缘/下缘时翻到另一侧，避免被窗口切掉
  const left = Math.max(8, Math.min(anchor.left, window.innerWidth - W - 8))
  const top = anchor.top + 6

  return (
    <div
      className="fixed z-50 rounded-xl border border-border bg-popover shadow-xl"
      style={{ left, top, width: W, maxHeight: 340 }}
    >
      <div className="flex items-baseline justify-between border-b border-border px-3 py-2">
        <span className="text-sm font-semibold">
          {year} 年 {month} 月{day ? ` ${day} 日` : ''}
        </span>
        {data && (
          <span className="text-[11px] text-muted-foreground">
            {day
              ? `${data.total_msgs.toLocaleString()} 条 · ${data.total_peers} 个会话`
              : `${data.total_days} 天有记录 · ${data.total_peers} 个会话`}
          </span>
        )}
      </div>

      {isLoading && <div className="py-4 text-center text-xs text-muted-foreground">加载中…</div>}

      {data && data.partners.length === 0 && (
        <div className="py-4 text-center text-xs text-muted-foreground">
          {day ? '这天没有聊天记录' : '这个月没有聊天记录'}
        </div>
      )}

      {data && data.partners.length > 0 && (
        <div
          className="overflow-y-auto px-3 py-2"
          style={{ maxHeight: 268, overscrollBehavior: 'contain' }}
        >
          <div className="space-y-1">
            {data.partners.map((p) => (
              <div key={p.username} className="flex items-center gap-2 text-xs">
                <span className="flex-1 truncate" title={p.name}>
                  {p.is_group && <span className="mr-1 text-muted-foreground">[群]</span>}
                  {p.name}
                </span>
                {/* 看某一天时「1 天」没有信息量，条数才是主角 */}
                {!day && <span className="shrink-0 font-medium tabular-nums">{p.days} 天</span>}
                <span
                  className={`w-16 shrink-0 text-right tabular-nums ${day ? 'font-medium' : 'text-muted-foreground'}`}
                >
                  {p.messages.toLocaleString()} 条
                </span>
              </div>
            ))}
          </div>
          {data.total_peers > data.partners.length && (
            <div className="mt-2 border-t border-border pt-1.5 text-[11px] text-muted-foreground">
              仅显示前 {data.partners.length} 位，共 {data.total_peers} 个会话
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** 今日报告：当天的收发概况、时段分布、聊得最多的人。
 *
 *  方向判定（哪条是我发的）在后端逐字沿用年度报告那套，
 *  所以这里的「发/收」与年度报告、日历热力图三处口径一致。
 */
function DailyReportCard() {
  // 支持回看前几天 —— 0=今天，1=昨天…
  const [back, setBack] = useState(0)
  const date = useMemo(() => {
    const d = new Date()
    d.setDate(d.getDate() - back)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  }, [back])

  const { data, isLoading } = useQuery({
    queryKey: ['insights-daily-report', date],
    queryFn: () => insightsApi.dailyReport(date),
    staleTime: 60 * 1000,
  })

  const hhmm = (u: number) =>
    u ? new Date(u * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : '—'
  const label = back === 0 ? '今日报告' : back === 1 ? '昨日报告' : `${back} 天前`
  const maxHour = data ? Math.max(1, ...data.hourly) : 1

  return (
    <Card
      title={label}
      icon={<Sun className="h-4 w-4 text-primary" />}
      extra={
        <div className="flex items-center gap-1.5">
          <button
            className="rounded-md border border-border px-2 py-0.5 text-xs hover:bg-accent disabled:opacity-40"
            onClick={() => setBack((b) => b + 1)} disabled={back >= 30}>← 前一天</button>
          <button
            className="rounded-md border border-border px-2 py-0.5 text-xs hover:bg-accent disabled:opacity-40"
            onClick={() => setBack((b) => Math.max(0, b - 1))} disabled={back === 0}>后一天 →</button>
        </div>
      }
    >
      {isLoading && <div className="py-6 text-center text-sm text-muted-foreground">加载中…</div>}

      {data && data.total_messages === 0 && (
        <div className="py-6 text-center text-sm text-muted-foreground">
          {date} 没有聊天记录
        </div>
      )}

      {data && data.total_messages > 0 && (
        <div className="space-y-4">
          {/* 概览数字 */}
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat label="消息总数" value={data.total_messages.toLocaleString()} />
            <Stat label="我发出" value={data.sent_messages.toLocaleString()}
              icon={<Send className="h-3 w-3" />} />
            <Stat label="我收到" value={data.recv_messages.toLocaleString()}
              icon={<Inbox className="h-3 w-3" />} />
            <Stat label="活跃会话"
              value={`${data.total_peers}`}
              sub={`${data.active_peers} 私聊 · ${data.active_groups} 群`} />
          </div>

          <div className="text-xs text-muted-foreground">
            {hhmm(data.first_time)} 起，最后一条 {hhmm(data.last_time)}
            {data.types.length > 0 && (
              <span className="ml-2">
                · {data.types.slice(0, 4).map((t) => `${t.name} ${t.count}`).join(' · ')}
              </span>
            )}
          </div>

          {/* 24 小时分布 */}
          <div>
            <div className="mb-1 text-xs text-muted-foreground">时段分布</div>
            <div className="flex items-end gap-[2px]" style={{ height: 44 }}>
              {data.hourly.map((n, h) => (
                <div key={h} className="flex-1 rounded-sm"
                  title={`${h}:00–${h}:59 · ${n} 条`}
                  style={{
                    height: `${Math.max(n > 0 ? 3 : 1, (n / maxHour) * 44)}px`,
                    background: n > 0 ? 'hsl(var(--primary))' : 'rgba(100,116,139,0.22)',
                    opacity: n > 0 ? 0.35 + 0.65 * (n / maxHour) : 1,
                  }} />
              ))}
            </div>
            <div className="mt-1 flex justify-between text-[10px] text-muted-foreground">
              <span>0</span><span>6</span><span>12</span><span>18</span><span>23</span>
            </div>
          </div>

          {/* 聊得最多 */}
          <div>
            <div className="mb-1.5 text-xs text-muted-foreground">聊得最多</div>
            <div className="space-y-1">
              {data.partners.slice(0, 8).map((p) => {
                const pct = (p.messages / data.partners[0].messages) * 100
                return (
                  <div key={p.username} className="flex items-center gap-2 text-xs">
                    <span className="w-40 shrink-0 truncate" title={p.name}>
                      {p.is_group && <span className="mr-1 text-muted-foreground">[群]</span>}
                      {p.name}
                    </span>
                    {/* 条形：让「谁聊得多」一眼可比，而不是逐个读数字 */}
                    <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted/40">
                      <div className="h-full rounded-full bg-primary/70" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="w-24 shrink-0 text-right tabular-nums text-muted-foreground">
                      发{p.sent} / 收{p.recv}
                    </span>
                    <span className="w-14 shrink-0 text-right font-medium tabular-nums">
                      {p.messages} 条
                    </span>
                  </div>
                )
              })}
            </div>
            {data.total_peers > 8 && (
              <div className="mt-1.5 text-[11px] text-muted-foreground">
                仅显示前 8 位，共 {data.total_peers} 个会话
              </div>
            )}
          </div>
        </div>
      )}
    </Card>
  )
}

/** 概览小格子 */
function Stat({ label, value, sub, icon }: {
  label: string; value: string; sub?: string; icon?: ReactNode
}) {
  return (
    <div className="rounded-lg border border-border bg-muted/20 px-3 py-2">
      <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
        {icon}{label}
      </div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
      {sub && <div className="text-[10px] text-muted-foreground">{sub}</div>}
    </div>
  )
}

function Heatmap({ year }: { year: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-heatmap', year],
    queryFn: () => insightsApi.calendarHeatmap(year),
  })
  const { cells, max, months } = useMemo(() => {
    const m = new Map<string, number>()
    let max = 0
    for (const d of data || []) {
      m.set(d.date, d.count)
      if (d.count > max) max = d.count
    }
    const start = new Date(year, 0, 1)
    const end = new Date(year, 11, 31)
    const firstOffset = start.getDay() // 周日=0
    const cells: { date: string; count: number; week: number; wd: number }[] = []
    const months: { week: number; label: string; month: number }[] = []
    let lastMonth = -1
    for (let d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
      const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
      const dayOfYear = Math.floor((d.getTime() - start.getTime()) / 86400000)
      const week = Math.floor((dayOfYear + firstOffset) / 7)
      const wd = d.getDay()
      cells.push({ date: iso, count: m.get(iso) || 0, week, wd })
      if (d.getMonth() !== lastMonth) {
        months.push({ week, label: `${d.getMonth() + 1}月`, month: d.getMonth() + 1 })
        lastMonth = d.getMonth()
      }
    }
    return { cells, max, months }
  }, [data, year])

  const color = (c: number) => {
    if (c <= 0) return 'rgba(148,163,184,0.15)'
    const r = max > 0 ? c / max : 0
    const level = r > 0.66 ? 0.95 : r > 0.33 ? 0.7 : r > 0.1 ? 0.45 : 0.25
    return `rgba(236,72,153,${level})`
  }
  // 悬停目标统一描述：day 为空=整月。anchor 是**屏幕坐标**（fixed 定位要用），
  // 所以存的是 getBoundingClientRect 的结果，不是 SVG 内部坐标。
  const [hover, setHover] = useState<
    { month: number; day?: number; anchor: { left: number; top: number } } | null
  >(null)
  // 移出后延迟关闭，让鼠标能从标签移进弹窗而不中断
  const closeTimer = useRef<number | null>(null)
  const openHover = (h: { month: number; day?: number; anchor: { left: number; top: number } }) => {
    if (closeTimer.current) { window.clearTimeout(closeTimer.current); closeTimer.current = null }
    setHover(h)
  }
  const scheduleClose = () => {
    if (closeTimer.current) window.clearTimeout(closeTimer.current)
    closeTimer.current = window.setTimeout(() => setHover(null), 160)
  }
  const CS = 12, GAP = 3
  const weeks = Math.max(...cells.map((c) => c.week)) + 1
  const total = (data || []).reduce((s, d) => s + d.count, 0)

  if (isLoading) return <div className="text-sm text-muted-foreground">加载中…</div>
  return (
    <div className="overflow-x-auto">
      <div className="mb-1 text-xs text-muted-foreground">
        {year} 年共 {total.toLocaleString()} 条消息，{(data || []).length} 天有记录
        <span className="ml-2 opacity-70">· 悬停月份或某一天，看和谁聊了多少</span>
      </div>
      <svg width={weeks * (CS + GAP) + 30} height={7 * (CS + GAP) + 20}>
        {months.map((mo, i) => {
          const mx = 30 + mo.week * (CS + GAP)
          const on = hover?.month === mo.month && !hover?.day
          return (
            <g key={i}
              onMouseEnter={(e) => {
                const r = (e.currentTarget as SVGGElement).getBoundingClientRect()
                openHover({ month: mo.month, anchor: { left: r.left, top: r.bottom } })
              }}
              onMouseLeave={scheduleClose}
              style={{ cursor: 'pointer' }}>
              {/* 透明热区：文字本身只有几像素高，直接挂 hover 很难对准 */}
              <rect x={mx - 4} y={0} width={30} height={16} fill="transparent" />
              <text x={mx} y={10}
                className={on ? 'fill-foreground' : 'fill-muted-foreground'}
                style={{ fontSize: 10, fontWeight: on ? 600 : 400 }}>
                {mo.label}
              </text>
            </g>
          )
        })}
        {['一', '三', '五'].map((w, i) => (
          <text key={w} x={0} y={20 + (i * 2 + 1) * (CS + GAP) + 9} className="fill-muted-foreground" style={{ fontSize: 9 }}>{w}</text>
        ))}
        {cells.map((c) => {
          const [, mm, dd] = c.date.split('-').map(Number)
          const on = hover?.day === dd && hover?.month === mm
          return (
            <rect key={c.date}
              x={30 + c.week * (CS + GAP)} y={20 + c.wd * (CS + GAP)}
              width={CS} height={CS} rx={2}
              fill={color(c.count)}
              stroke={on ? 'currentColor' : 'none'} strokeWidth={on ? 1.5 : 0}
              style={{ cursor: c.count > 0 ? 'pointer' : 'default' }}
              onMouseEnter={(e) => {
                if (c.count <= 0) return // 没消息的日子不弹空卡片
                const r = (e.currentTarget as SVGRectElement).getBoundingClientRect()
                openHover({ month: mm, day: dd, anchor: { left: r.left, top: r.bottom } })
              }}
              onMouseLeave={scheduleClose}>
              {/* 保留原生 title 作兜底：弹窗加载中也能看到日期和条数 */}
              <title>{c.date}: {c.count} 条</title>
            </rect>
          )
        })}
      </svg>

      {hover && (
        <div onMouseEnter={() => openHover(hover)} onMouseLeave={scheduleClose}>
          <HeatmapPartnersPopup
            year={year}
            month={hover.month}
            day={hover.day}
            anchor={hover.anchor}
          />
        </div>
      )}
    </div>
  )
}

// ── 功能7 年度对比 ──────────────────────────────────
function YearCompare() {
  const [ya, setYa] = useState(CUR - 1)
  const [yb, setYb] = useState(CUR)
  const { data, isLoading } = useQuery({
    queryKey: ['insights-yearcompare', ya, yb],
    queryFn: () => insightsApi.yearCompare(ya, yb),
  })
  const sel = 'rounded-lg border border-border bg-background px-2 py-1 text-sm'
  const Stat = ({ label, a, b }: { label: string; a: number; b: number }) => (
    <div className="rounded-xl border border-border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 flex items-baseline gap-2">
        <span className="text-sm text-muted-foreground">{a.toLocaleString()}</span>
        <ArrowLeftRight className="h-3 w-3 text-muted-foreground" />
        <span className="text-lg font-semibold">{b.toLocaleString()}</span>
        <span className={`text-xs ${b >= a ? 'text-emerald-500' : 'text-rose-500'}`}>{b >= a ? '↑' : '↓'}{Math.abs(b - a).toLocaleString()}</span>
      </div>
    </div>
  )
  const List = ({ title, items, color }: { title: string; items: any[]; color: string }) => (
    <div>
      <div className="mb-2 text-sm font-medium">{title}</div>
      <div className="space-y-1">
        {items.slice(0, 8).map((x) => (
          <div key={x.talker} className="flex items-center gap-2 text-sm">
            <span className="truncate">{x.name}</span>
            <span className={`ml-auto text-xs ${color}`}>{x.count_a}→{x.count_b}</span>
          </div>
        ))}
        {items.length === 0 && <div className="text-xs text-muted-foreground">无</div>}
      </div>
    </div>
  )
  return (
    <Card title="年度对比" icon={<ArrowLeftRight className="h-4 w-4 text-primary" />}
      extra={
        <div className="flex items-center gap-1">
          <select className={sel} value={ya} onChange={(e) => setYa(+e.target.value)}>{YEARS.map((y) => <option key={y} value={y}>{y}</option>)}</select>
          <span className="text-xs text-muted-foreground">vs</span>
          <select className={sel} value={yb} onChange={(e) => setYb(+e.target.value)}>{YEARS.map((y) => <option key={y} value={y}>{y}</option>)}</select>
        </div>
      }>
      {isLoading || !data ? <div className="text-sm text-muted-foreground">加载中…</div> : (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat label="总消息" a={data.overview_a.total_messages} b={data.overview_b.total_messages} />
            <Stat label="活跃天数" a={data.overview_a.active_days} b={data.overview_b.active_days} />
            <Stat label="活跃联系人" a={data.overview_a.active_contacts} b={data.overview_b.active_contacts} />
            <Stat label="活跃群聊" a={data.overview_a.active_chatrooms} b={data.overview_b.active_chatrooms} />
          </div>
          <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
            <List title={`✨ ${yb} 新进入的人`} items={data.newly_active} color="text-emerald-500" />
            <List title={`🍂 从 ${ya} 淡出的人`} items={data.faded_out} color="text-rose-500" />
          </div>
        </>
      )}
    </Card>
  )
}

// ── 功能3 双向互动比 + 功能4 共同群 ────────────────
function InteractionList({ year, onPick }: { year: number; onPick: (r: InteractionRatio) => void }) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-interaction', year],
    queryFn: () => insightsApi.interactionRatios(year),
  })
  if (isLoading) return <div className="text-sm text-muted-foreground">加载中…</div>
  return (
    <div className="space-y-2">
      {(data || []).slice(0, 30).map((r) => {
        const t = r.sent_count + r.recv_count || 1
        const sentPct = (r.sent_count / t) * 100
        const oneSided = r.total > 100 && (r.my_initiations > r.their_initiations * 2.5 || r.their_initiations > r.my_initiations * 2.5)
        return (
          <button key={r.talker} onClick={() => onPick(r)}
            className="w-full rounded-xl border border-border p-3 text-left hover:bg-muted/50 transition-colors">
            <div className="flex items-center gap-2 text-sm">
              <span className="truncate font-medium">{r.name}</span>
              {oneSided && <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] text-amber-600">单向维系</span>}
              <span className="ml-auto text-xs text-muted-foreground">共 {r.total.toLocaleString()} 条</span>
            </div>
            <div className="mt-2 flex h-2 overflow-hidden rounded-full bg-muted">
              <div className="bg-pink-500" style={{ width: `${sentPct}%` }} />
              <div className="bg-indigo-400" style={{ width: `${100 - sentPct}%` }} />
            </div>
            <div className="mt-1 flex justify-between text-[11px] text-muted-foreground">
              <span>你发 {r.sent_count.toLocaleString()}（发起 {r.my_initiations}）</span>
              <span>ta 发 {r.recv_count.toLocaleString()}（发起 {r.their_initiations}）</span>
            </div>
          </button>
        )
      })}
    </div>
  )
}

// ── 功能5 回复速度 ──────────────────────────────────
type ReplySortKey = 'name' | 'my_avg_reply_sec' | 'my_fastest_sec' | 'late_night_instant' | 'their_avg_reply_sec'

const REPLY_COLUMNS: { key: ReplySortKey; label: string }[] = [
  { key: 'name', label: '联系人' },
  { key: 'my_avg_reply_sec', label: '你平均回复' },
  { key: 'my_fastest_sec', label: '最快' },
  { key: 'late_night_instant', label: '深夜秒回' },
  { key: 'their_avg_reply_sec', label: 'ta 平均回复' },
]

function ReplySpeedList({ year }: { year: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-replyspeed', year],
    queryFn: () => insightsApi.replySpeed(year),
  })
  // 时延类默认升序（越快越靠前），次数类默认降序
  const [sortKey, setSortKey] = useState<ReplySortKey>('my_avg_reply_sec')
  const [asc, setAsc] = useState(true)

  const clickHeader = (key: ReplySortKey) => {
    if (key === sortKey) {
      setAsc((v) => !v)
    } else {
      setSortKey(key)
      setAsc(key === 'my_avg_reply_sec' || key === 'my_fastest_sec' || key === 'their_avg_reply_sec')
    }
  }

  const rows = useMemo(() => {
    const list = (data || []).filter((r: ReplySpeed) => r.my_reply_count >= 5)
    const sorted = [...list].sort((a, b) => {
      if (sortKey === 'name') return a.name.localeCompare(b.name, 'zh')
      const av = a[sortKey] as number
      const bv = b[sortKey] as number
      // 0 表示没有数据，排序时一律沉底，别让它冒充「最快」
      if (av === 0 && bv === 0) return 0
      if (av === 0) return 1
      if (bv === 0) return -1
      return av - bv
    })
    if (!asc) sorted.reverse()
    return sorted.slice(0, 30)
  }, [data, sortKey, asc])

  if (isLoading) return <div className="text-sm text-muted-foreground">加载中…</div>

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-muted-foreground">
            {REPLY_COLUMNS.map((col) => (
              <th key={col.key} className="pb-2 font-normal">
                <button
                  onClick={() => clickHeader(col.key)}
                  className={`inline-flex items-center gap-0.5 transition-colors hover:text-foreground ${
                    sortKey === col.key ? 'font-medium text-foreground' : ''
                  }`}
                  title="点击按此列排序"
                >
                  {col.label}
                  <span className="text-[10px] opacity-60">
                    {sortKey === col.key ? (asc ? '▲' : '▼') : '⇅'}
                  </span>
                </button>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.talker} className="border-t border-border">
              <td className="py-2 pr-2"><span className="truncate">{r.name}</span></td>
              <td className="py-2 pr-2">{fmtDur(r.my_avg_reply_sec)}</td>
              <td className="py-2 pr-2 text-emerald-500">{fmtDur(r.my_fastest_sec)}</td>
              <td className="py-2 pr-2">{r.late_night_instant > 0 ? `${r.late_night_instant} 次` : '—'}</td>
              <td className="py-2 text-muted-foreground">{fmtDur(r.their_avg_reply_sec)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="mt-2 text-[11px] text-muted-foreground">
        只统计回复次数 ≥ 5 的联系人，最多 30 条；通话期间的来回消息不计入。
      </p>
    </div>
  )
}

// ── 共同群弹窗（功能4）─────────────────────────────
function CommonGroupsModal({ talker, name, onClose }: { talker: string; name: string; onClose: () => void }) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-commongroups', talker],
    queryFn: () => insightsApi.commonGroups(talker),
  })
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div className="max-h-[70vh] w-full max-w-md overflow-y-auto rounded-2xl border border-border bg-card p-5" onClick={(e) => e.stopPropagation()}>
        <div className="mb-3 flex items-center gap-2">
          <Users2 className="h-4 w-4 text-primary" />
          <h3 className="font-semibold">与 {name} 的共同群</h3>
          <button className="ml-auto text-muted-foreground hover:text-foreground" onClick={onClose}><X className="h-4 w-4" /></button>
        </div>
        {isLoading ? <div className="text-sm text-muted-foreground">加载中…</div> : (
          (data || []).length === 0 ? <div className="text-sm text-muted-foreground">没有共同群</div> : (
            <div className="space-y-1">
              <div className="mb-1 text-xs text-muted-foreground">共 {data!.length} 个共同群</div>
              {(data as CommonGroup[]).map((g) => (
                <div key={g.username} className="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
                  <span className="truncate">{g.name}</span>
                  <span className="ml-auto text-xs text-muted-foreground">{g.member_count} 人</span>
                </div>
              ))}
            </div>
          )
        )}
      </div>
    </div>
  )
}

export default function Insights() {
  const [year, setYear] = useState(CUR)
  const [picked, setPicked] = useState<InteractionRatio | null>(null)
  const sel = 'rounded-lg border border-border bg-background px-3 py-1.5 text-sm'

  return (
    <div className="h-full overflow-y-auto mx-auto max-w-5xl space-y-5 p-6">
      <div className="flex items-center gap-3">
        <h1 className="text-xl font-bold">关系洞察</h1>
        <select className={sel + ' ml-auto'} value={year} onChange={(e) => setYear(+e.target.value)}>
          {YEARS.map((y) => <option key={y} value={y}>{y} 年</option>)}
        </select>
      </div>

      <DailyReportCard />

      <Card title="日历热力图" icon={<CalendarDays className="h-4 w-4 text-primary" />}>
        <Heatmap year={year} />
      </Card>

      <YearCompare />

      <Card title="双向互动比" icon={<ArrowLeftRight className="h-4 w-4 text-primary" />}
        extra={<span className="text-xs text-muted-foreground">点联系人看共同群</span>}>
        <InteractionList year={year} onPick={setPicked} />
      </Card>

      <Card title="回复速度" icon={<Clock className="h-4 w-4 text-primary" />}>
        <ReplySpeedList year={year} />
      </Card>

      {picked && <CommonGroupsModal talker={picked.talker} name={picked.name} onClose={() => setPicked(null)} />}
    </div>
  )
}
