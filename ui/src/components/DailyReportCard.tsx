import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Sun, Send, Inbox, Type, ChevronDown, ChevronRight } from 'lucide-react'
import { insightsApi } from '@/api/insights'

/** 今日报告：当天的收发概况、时段分布、聊得最多的人。
 *
 *  方向判定（哪条是我发的）在后端逐字沿用年度报告那套，
 *  所以这里的「发/收」与年度报告、日历热力图三处口径一致。
 *
 *  置于聊天页顶部，**可折叠** —— 聊天页的主角是聊天，
 *  报告不该长期占着垂直空间；折叠状态记在 localStorage。
 */
export function DailyReportCard() {
  // 回看：0=今天，1=昨天…最多 30 天
  const [back, setBack] = useState(0)
  const [open, setOpen] = useState(() => localStorage.getItem('dailyReportOpen') !== '0')
  const [showAllPartners, setShowAllPartners] = useState(false)

  const date = useMemo(() => {
    const d = new Date()
    d.setDate(d.getDate() - back)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  }, [back])

  const { data, isLoading } = useQuery({
    queryKey: ['insights-daily-report', date],
    // 一天的会话数很少，直接全取，这样「展开」是真的全部而不是前 20
    queryFn: () => insightsApi.dailyReport(date, 200),
    staleTime: 60 * 1000,
  })

  // 换一天就收起，否则看下一天时莫名其妙是展开的
  useEffect(() => { setShowAllPartners(false) }, [date])

  const shownPartners = showAllPartners ? data?.partners ?? [] : (data?.partners ?? []).slice(0, 6)

  const toggle = () => {
    setOpen((v) => {
      localStorage.setItem('dailyReportOpen', v ? '0' : '1')
      return !v
    })
  }

  const hhmm = (u: number) =>
    u ? new Date(u * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : '—'

  // 超过「昨天」必须把具体日期写出来 —— 只说「3 天前」读者还得自己数
  const label =
    back === 0 ? '今日报告'
      : back === 1 ? `昨日报告 · ${date.slice(5)}`
        : `${back} 天前 · ${date}`

  const maxHour = data ? Math.max(1, ...data.hourly) : 1

  return (
    // relative：展开的面板绝对定位挂在这层下方，浮在内容之上 ——
    // 这样展开/收起都不会把下面的会话列表顶来顶去
    <div className="relative z-30 flex-shrink-0 border-b border-border bg-background">
      <div className="flex items-center gap-2 px-4 py-2">
        <button onClick={toggle} className="flex items-center gap-1.5 text-sm font-semibold hover:text-primary">
          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          <Sun className="h-4 w-4 text-primary" />
          {label}
        </button>

        {/* 收起时把最要紧的数字留在标题行，一眼能看到 */}
        {!open && data && data.total_messages > 0 && (
          <span className="truncate text-xs text-muted-foreground">
            {data.total_messages} 条 · 发 {data.sent_messages} / 收 {data.recv_messages} · {data.total_peers} 个会话
          </span>
        )}

        <div className="ml-auto flex flex-shrink-0 items-center gap-1.5">
          <button
            className="rounded-md border border-border px-2 py-0.5 text-xs hover:bg-accent disabled:opacity-40"
            onClick={() => setBack((b) => b + 1)} disabled={back >= 30}>← 前一天</button>
          <button
            className="rounded-md border border-border px-2 py-0.5 text-xs hover:bg-accent disabled:opacity-40"
            onClick={() => setBack((b) => Math.max(0, b - 1))} disabled={back === 0}>后一天 →</button>
        </div>
      </div>

      {open && (
        // absolute + top-full：紧贴标题栏下沿浮出，不占文档流高度。
        // 自带滚动 + overscroll-contain，列表滚到底不会把滚动链传给背后的会话列表。
        <div
          className="absolute inset-x-0 top-full overflow-y-auto border-b border-border bg-background px-4 pb-3 shadow-lg"
          style={{ maxHeight: '70vh', overscrollBehavior: 'contain' }}
        >
          {isLoading && <div className="py-4 text-center text-sm text-muted-foreground">加载中…</div>}

          {data && data.total_messages === 0 && (
            <div className="py-4 text-center text-sm text-muted-foreground">{date} 没有聊天记录</div>
          )}

          {data && data.total_messages > 0 && (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
                <Stat label="消息总数" value={data.total_messages.toLocaleString()}
                  sub={<Delta cur={data.total_messages} prev={data.prev_day_total} unit="较昨天" />} />
                <Stat label="我发出" value={data.sent_messages.toLocaleString()} icon={<Send className="h-3 w-3" />} />
                <Stat label="我收到" value={data.recv_messages.toLocaleString()} icon={<Inbox className="h-3 w-3" />} />
                <Stat label="今日字数" value={(data.sent_chars + data.recv_chars).toLocaleString()}
                  icon={<Type className="h-3 w-3" />}
                  sub={data.voice_chars > 0 ? `含语音转写 ${data.voice_chars.toLocaleString()}` : undefined} />
                <Stat label="活跃会话" value={`${data.total_peers}`}
                  sub={`${data.active_peers} 私聊 · ${data.active_groups} 群`} />
              </div>

              {/* 一句话把最值得注意的三件事说完，省得逐个读图 */}
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg bg-muted/30 px-3 py-2 text-xs">
                <span>
                  最活跃 <b className="tabular-nums">{String(data.peak_hour).padStart(2, '0')}:00</b>
                  <span className="text-muted-foreground">（{data.peak_hour_count} 条）</span>
                </span>
                <span>
                  我主动开口 <b className="tabular-nums">{data.initiated_by_me}</b>
                  <span className="text-muted-foreground"> / 对方先来 {data.initiated_by_them}</span>
                </span>
                <span>
                  连续 <b className="tabular-nums">{data.streak_days}{data.streak_capped ? '+' : ''}</b> 天有记录
                </span>
                <span className="text-muted-foreground">
                  上周同日 {data.last_week_total.toLocaleString()} 条
                </span>
              </div>

              <div className="text-xs text-muted-foreground">
                {hhmm(data.first_time)} 起，最后一条 {hhmm(data.last_time)}
                {data.types.length > 0 && (
                  <span className="ml-2">· {data.types.slice(0, 4).map((t) => `${t.name} ${t.count}`).join(' · ')}</span>
                )}
              </div>

              <div>
                <div className="mb-1 text-xs text-muted-foreground">时段分布</div>
                {/* 24 个格子的热力图，色阶与日历热力图完全一致，
                    两处放在同一页面时视觉语言统一 */}
                <div className="flex gap-[3px]">
                  {data.hourly.map((n, h) => (
                    <div key={h} className="group relative flex-1">
                      <div className="aspect-square w-full rounded-sm transition-transform hover:scale-110"
                        title={`${String(h).padStart(2, '0')}:00–${String(h).padStart(2, '0')}:59 · ${n} 条`}
                        style={{ background: hourColor(n, maxHour) }} />
                    </div>
                  ))}
                </div>
                <div className="mt-1 flex justify-between text-[10px] text-muted-foreground">
                  <span>0</span><span>6</span><span>12</span><span>18</span><span>23</span>
                </div>
              </div>

              <div>
                <div className="mb-1.5 text-xs text-muted-foreground">聊得最多</div>
                <div className="space-y-1">
                  {shownPartners.map((p) => {
                    const pct = (p.messages / data.partners[0].messages) * 100
                    return (
                      <div key={p.username} className="flex items-center gap-2 text-xs">
                        <span className="w-36 shrink-0 truncate" title={p.name}>
                          {p.is_group && <span className="mr-1 text-muted-foreground">[群]</span>}
                          {p.name}
                          {/* 谁先开的口 —— 小箭头比多一列文字省地方 */}
                          <span className="ml-1 text-[10px] text-muted-foreground"
                            title={p.first_by_self ? '今天是我先开的口' : '今天是对方先来的'}>
                            {p.first_by_self ? '↗' : '↘'}
                          </span>
                        </span>
                        {/* 条形：让「谁聊得多」一眼可比，而不是逐个读数字 */}
                        <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted/40">
                          <div className="h-full rounded-full bg-primary/70" style={{ width: `${pct}%` }} />
                        </div>
                        <span className="w-20 shrink-0 text-right tabular-nums text-muted-foreground">
                          发{p.sent} / 收{p.recv}
                        </span>
                        <span className="w-14 shrink-0 text-right font-medium tabular-nums">{p.messages} 条</span>
                      </div>
                    )
                  })}
                </div>
                {data.partners.length > 6 && (
                  <button
                    onClick={() => setShowAllPartners((v) => !v)}
                    className="mt-1.5 text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                  >
                    {showAllPartners
                      ? '收起'
                      : `仅显示前 6 位，共 ${data.total_peers} 个会话 · 点击展开`}
                  </button>
                )}

              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** 时段热力图的色阶。与日历热力图（Insights.tsx 的 color()）逐档一致：
 *  无消息用灰，其余按占当日峰值的比例分四档。 */
function hourColor(n: number, max: number) {
  if (n <= 0) return 'rgba(148,163,184,0.15)'
  const r = max > 0 ? n / max : 0
  const level = r > 0.66 ? 0.95 : r > 0.33 ? 0.7 : r > 0.1 ? 0.45 : 0.25
  return `rgba(236,72,153,${level})`
}

/** 与前一天的增减。没有前一天数据时不显示，避免「↑100%」这种噪声。 */
function Delta({ cur, prev, unit }: { cur: number; prev: number; unit: string }) {
  if (!prev) return null
  const diff = cur - prev
  if (diff === 0) return <span className="text-muted-foreground">{unit}持平</span>
  const pct = Math.round((diff / prev) * 100)
  return (
    <span className={diff > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}>
      {diff > 0 ? '↑' : '↓'}{Math.abs(pct)}% {unit}
    </span>
  )
}

/** 概览小格子 */
function Stat({ label, value, sub, icon }: { label: string; value: string; sub?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/20 px-3 py-2">
      <div className="flex items-center gap-1 text-[11px] text-muted-foreground">{icon}{label}</div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
      {sub && <div className="text-[10px] text-muted-foreground">{sub}</div>}
    </div>
  )
}
