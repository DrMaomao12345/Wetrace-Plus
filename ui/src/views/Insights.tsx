import { useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { insightsApi, type CommonGroup, type InteractionRatio, type ReplySpeed } from '@/api/insights'
import { CalendarDays, Users2, Clock, ArrowLeftRight, X } from 'lucide-react'

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
    const months: { week: number; label: string }[] = []
    let lastMonth = -1
    for (let d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
      const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
      const dayOfYear = Math.floor((d.getTime() - start.getTime()) / 86400000)
      const week = Math.floor((dayOfYear + firstOffset) / 7)
      const wd = d.getDay()
      cells.push({ date: iso, count: m.get(iso) || 0, week, wd })
      if (d.getMonth() !== lastMonth) {
        months.push({ week, label: `${d.getMonth() + 1}月` })
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
  const CS = 12, GAP = 3
  const weeks = Math.max(...cells.map((c) => c.week)) + 1
  const total = (data || []).reduce((s, d) => s + d.count, 0)

  if (isLoading) return <div className="text-sm text-muted-foreground">加载中…</div>
  return (
    <div className="overflow-x-auto">
      <div className="mb-1 text-xs text-muted-foreground">{year} 年共 {total.toLocaleString()} 条消息，{(data || []).length} 天有记录</div>
      <svg width={weeks * (CS + GAP) + 30} height={7 * (CS + GAP) + 20}>
        {months.map((mo, i) => (
          <text key={i} x={30 + mo.week * (CS + GAP)} y={10} className="fill-muted-foreground" style={{ fontSize: 10 }}>{mo.label}</text>
        ))}
        {['一', '三', '五'].map((w, i) => (
          <text key={w} x={0} y={20 + (i * 2 + 1) * (CS + GAP) + 9} className="fill-muted-foreground" style={{ fontSize: 9 }}>{w}</text>
        ))}
        {cells.map((c) => (
          <rect key={c.date} x={30 + c.week * (CS + GAP)} y={20 + c.wd * (CS + GAP)} width={CS} height={CS} rx={2}
            fill={color(c.count)}>
            <title>{c.date}: {c.count} 条</title>
          </rect>
        ))}
      </svg>
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
function ReplySpeedList({ year }: { year: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['insights-replyspeed', year],
    queryFn: () => insightsApi.replySpeed(year),
  })
  if (isLoading) return <div className="text-sm text-muted-foreground">加载中…</div>
  const rows = (data || []).filter((r: ReplySpeed) => r.my_reply_count >= 5).slice(0, 30)
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-muted-foreground">
            <th className="pb-2 font-normal">联系人</th>
            <th className="pb-2 font-normal">你平均回复</th>
            <th className="pb-2 font-normal">最快</th>
            <th className="pb-2 font-normal">深夜秒回</th>
            <th className="pb-2 font-normal">ta 平均回复</th>
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
    <div className="mx-auto max-w-5xl space-y-5 p-6">
      <div className="flex items-center gap-3">
        <h1 className="text-xl font-bold">关系洞察</h1>
        <select className={sel + ' ml-auto'} value={year} onChange={(e) => setYear(+e.target.value)}>
          {YEARS.map((y) => <option key={y} value={y}>{y} 年</option>)}
        </select>
      </div>

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
