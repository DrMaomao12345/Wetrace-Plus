import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { request } from "@/lib/request"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { CalendarDays, Mic, ArrowLeftRight } from "lucide-react"

const CUR = new Date().getFullYear()
const YEARS = Array.from({ length: CUR - 2021 }, (_, i) => CUR - i)

interface VoiceStats {
  total_count: number
  sent_count: number
  recv_count: number
  total_duration_ms: number
  sent_duration_ms: number
  recv_duration_ms: number
  avg_duration_ms: number
  sent_avg_duration_ms: number
  recv_avg_duration_ms: number
  longest_duration_ms: number
  longest_at: number
  longest_is_self: boolean
  without_duration: number
}

interface TalkerExtrasData {
  year: number
  calendar_heatmap: { date: string; count: number }[] | null
  voice: VoiceStats | null
  interaction: {
    sent_count: number
    recv_count: number
    my_initiations: number
    their_initiations: number
  } | null
  reply_speed: {
    my_avg_reply_sec: number
    my_reply_count: number
    my_fastest_sec: number
    my_slowest_sec: number
    their_avg_reply_sec: number
    their_reply_count: number
    late_night_instant: number
    ignored_by_them: number
  } | null
  call_windows: number
}

function fmtDur(ms: number): string {
  if (!ms || ms <= 0) return "—"
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)} 秒`
  const m = s / 60
  if (m < 60) return `${m.toFixed(1)} 分钟`
  return `${(m / 60).toFixed(1)} 小时`
}

function fmtSec(sec: number): string {
  if (!sec || sec <= 0) return "—"
  if (sec < 60) return `${sec} 秒`
  if (sec < 3600) return `${Math.round(sec / 60)} 分钟`
  return `${(sec / 3600).toFixed(1)} 小时`
}

/** 紧凑版日历热力图：一列一周，和 GitHub 贡献图一致 */
function Heatmap({ year, data }: { year: number; data: { date: string; count: number }[] }) {
  const { cells, max, months } = useMemo(() => {
    const m = new Map(data.map((d) => [d.date, d.count]))
    let max = 0
    for (const c of m.values()) if (c > max) max = c

    const start = new Date(year, 0, 1)
    const end = new Date(year, 11, 31)
    const firstOffset = start.getDay()
    const cells: { date: string; count: number; week: number; wd: number }[] = []
    const months: { week: number; label: string }[] = []
    let lastMonth = -1
    for (const d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
      const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
      const dayOfYear = Math.floor((d.getTime() - start.getTime()) / 86400000)
      const week = Math.floor((dayOfYear + firstOffset) / 7)
      cells.push({ date: iso, count: m.get(iso) || 0, week, wd: d.getDay() })
      if (d.getMonth() !== lastMonth) {
        months.push({ week, label: `${d.getMonth() + 1}月` })
        lastMonth = d.getMonth()
      }
    }
    return { cells, max, months }
  }, [data, year])

  const color = (c: number) => {
    if (c <= 0) return "var(--muted)"
    const t = Math.min(1, Math.log(1 + c) / Math.log(1 + Math.max(1, max)))
    return `rgba(59,130,246,${0.15 + t * 0.85})` // 蓝色系，和其它图表一致
  }
  const weeks = Math.max(...cells.map((c) => c.week)) + 1

  return (
    <div className="overflow-x-auto">
      <div style={{ minWidth: weeks * 12 + 24 }}>
        <div className="relative mb-1 h-3">
          {months.map((mo) => (
            <span key={mo.label} className="absolute text-[9px] text-muted-foreground" style={{ left: mo.week * 12 }}>
              {mo.label}
            </span>
          ))}
        </div>
        <div className="relative" style={{ height: 7 * 12 }}>
          {cells.map((c) => (
            <div
              key={c.date}
              title={`${c.date}　${c.count} 条`}
              className="absolute rounded-[2px]"
              style={{
                left: c.week * 12,
                top: c.wd * 12,
                width: 10,
                height: 10,
                background: color(c.count),
              }}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-lg border border-border p-3">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className="mt-0.5 text-lg font-semibold tabular-nums">{value}</div>
      {sub && <div className="text-[10px] text-muted-foreground">{sub}</div>}
    </div>
  )
}

export function TalkerExtras({ talker }: { talker: string }) {
  const [year, setYear] = useState(CUR)

  const { data, isLoading } = useQuery({
    queryKey: ["talker-extras", talker, year],
    queryFn: () =>
      request.get<TalkerExtrasData>(`/api/v1/analysis/extras/${encodeURIComponent(talker)}`, { year }),
    enabled: !!talker,
  })

  if (isLoading) {
    return <div className="py-6 text-center text-sm text-muted-foreground">正在统计…</div>
  }
  if (!data) return null

  const v = data.voice
  const ir = data.interaction
  const rs = data.reply_speed
  const heat = data.calendar_heatmap ?? []
  const activeDays = heat.length

  return (
    <div className="space-y-4">
      {/* 日历热力图 */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="flex items-center gap-2 text-base">
            <CalendarDays className="h-4 w-4 text-blue-500" />
            日历热力图
          </CardTitle>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{activeDays} 天有聊天</span>
            <select
              value={year}
              onChange={(e) => setYear(Number(e.target.value))}
              className="h-7 rounded-md border border-input bg-background px-1.5 text-xs"
            >
              {YEARS.map((y) => (
                <option key={y} value={y}>
                  {y} 年
                </option>
              ))}
            </select>
          </div>
        </CardHeader>
        <CardContent>
          {activeDays === 0 ? (
            <p className="text-sm text-muted-foreground">{year} 年没有聊天记录。</p>
          ) : (
            <Heatmap year={year} data={heat} />
          )}
        </CardContent>
      </Card>

      {/* 语音统计 */}
      {v && v.total_count > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Mic className="h-4 w-4 text-blue-500" />
              语音消息
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
              <Stat label="总条数" value={`${v.total_count}`} sub={`我 ${v.sent_count} · ta ${v.recv_count}`} />
              <Stat
                label="总时长"
                value={fmtDur(v.total_duration_ms)}
                sub={`我 ${fmtDur(v.sent_duration_ms)} · ta ${fmtDur(v.recv_duration_ms)}`}
              />
              <Stat
                label="平均每条"
                value={fmtDur(v.avg_duration_ms)}
                sub={`我 ${fmtDur(v.sent_avg_duration_ms)} · ta ${fmtDur(v.recv_avg_duration_ms)}`}
              />
              <Stat
                label="最长一条"
                value={fmtDur(v.longest_duration_ms)}
                sub={v.longest_at ? `${v.longest_is_self ? "我" : "ta"}发于 ${new Date(v.longest_at * 1000).toLocaleDateString()}` : undefined}
              />
            </div>
            {v.without_duration > 0 && (
              <p className="text-[11px] text-muted-foreground">
                其中 {v.without_duration} 条解析不出时长，未计入平均值。
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {/* 互动与回复 */}
      {(ir || rs) && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ArrowLeftRight className="h-4 w-4 text-blue-500" />
              互动与回复
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {ir && (
              <>
                <div className="flex h-5 overflow-hidden rounded">
                  <div
                    className="flex items-center justify-center bg-blue-500/80 text-[10px] text-white"
                    style={{ width: `${(ir.sent_count / Math.max(1, ir.sent_count + ir.recv_count)) * 100}%` }}
                  >
                    我 {ir.sent_count}
                  </div>
                  <div
                    className="flex items-center justify-center bg-slate-400/70 text-[10px] text-white"
                    style={{ width: `${(ir.recv_count / Math.max(1, ir.sent_count + ir.recv_count)) * 100}%` }}
                  >
                    ta {ir.recv_count}
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <Stat label="我主动发起对话" value={`${ir.my_initiations} 次`} />
                  <Stat label="ta 主动发起对话" value={`${ir.their_initiations} 次`} />
                </div>
              </>
            )}
            {rs && (
              <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
                <Stat label="我的平均回复" value={fmtSec(rs.my_avg_reply_sec)} sub={`${rs.my_reply_count} 次`} />
                <Stat label="ta 的平均回复" value={fmtSec(rs.their_avg_reply_sec)} sub={`${rs.their_reply_count} 次`} />
                <Stat label="我最快回复" value={fmtSec(rs.my_fastest_sec)} />
                <Stat label="深夜秒回" value={`${rs.late_night_instant} 次`} sub="0–5 点 2 分钟内" />
              </div>
            )}
            {data.call_windows > 0 && (
              <p className="text-[11px] text-muted-foreground">
                已剔除 {data.call_windows} 段通话期间的消息 —— 通话中的来回不算回复。
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
