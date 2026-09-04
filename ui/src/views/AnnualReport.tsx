import { useState, useRef, useMemo, useEffect } from "react"
import { useQuery } from "@tanstack/react-query"
import { useReportStream } from "@/hooks/useReportStream"
import { useCountUp } from "@/hooks/useCountUp"
import { reportApi, type AnnualReport, type AnnualOverview, type TZSegment, type WordCountStat } from "@/api/report"
import { sessionApi, systemApi } from "@/api"
import { galaxyApi, type RelationshipGraph } from "@/api/galaxy"
import { computeYearReview } from "@/lib/relationshipReview"
import { toast } from "sonner"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  CalendarDays,
  MessageSquare,
  Users,
  TrendingUp,
  Moon,
  Clock,
  Flame,
  Trophy,
  Plus,
  Trash2,
  ChevronDown,
  Search,
  X,
  Eye,
  EyeOff,
} from "lucide-react"
import { formatNumber } from "@/lib/utils"
import { mediaApi } from "@/api/media"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  PieChart,
  Pie,
  LineChart,
  Line,
} from "recharts"

const WEEKDAY_NAMES = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]

// 常用时区列表（偏移量单位：分钟，东正西负）
const TZ_OPTIONS: { label: string; offset: number }[] = [
  { label: "UTC-12", offset: -720 },
  { label: "UTC-11", offset: -660 },
  { label: "UTC-10", offset: -600 },
  { label: "UTC-9",  offset: -540 },
  { label: "UTC-8",  offset: -480 },
  { label: "UTC-7",  offset: -420 },
  { label: "UTC-6",  offset: -360 },
  { label: "UTC-5",  offset: -300 },
  { label: "UTC-4",  offset: -240 },
  { label: "UTC-3",  offset: -180 },
  { label: "UTC-2",  offset: -120 },
  { label: "UTC-1",  offset:  -60 },
  { label: "UTC",    offset:    0 },
  { label: "UTC+1",  offset:   60 },
  { label: "UTC+2",  offset:  120 },
  { label: "UTC+3",  offset:  180 },
  { label: "UTC+4",  offset:  240 },
  { label: "UTC+5",  offset:  300 },
  { label: "UTC+5:30 (印度)", offset: 330 },
  { label: "UTC+5:45 (尼泊尔)", offset: 345 },
  { label: "UTC+6",  offset:  360 },
  { label: "UTC+7",  offset:  420 },
  { label: "UTC+8 (北京/上海)", offset: 480 },
  { label: "UTC+9 (东京/首尔)", offset: 540 },
  { label: "UTC+9:30 (澳大利亚中部)", offset: 570 },
  { label: "UTC+10", offset:  600 },
  { label: "UTC+11", offset:  660 },
  { label: "UTC+12", offset:  720 },
  { label: "UTC+13", offset:  780 },
  { label: "UTC+14", offset:  840 },
]

type SegmentRow = TZSegment & { _id: string }

function newRow(year: number): SegmentRow {
  return { _id: Math.random().toString(36).slice(2), start_date: `${year}-01-01`, end_date: `${year}-12-31`, tz_offset: -new Date().getTimezoneOffset() }
}

type ReportParams = { year: number; defaultTz: number; segments: SegmentRow[]; excludeTalkers: string[] }

const STORAGE_KEY = "annual_report_tz_config_v2"

interface SavedConfig {
  defaultTz: number
  segments: SegmentRow[]
  excludeTalkers: string[]
}

const DEFAULT_CONFIG: SavedConfig = {
  defaultTz: 480,
  segments: [
    { _id: "s1", start_date: "2025-09-10", end_date: "2025-12-21", tz_offset: 0 },
    { _id: "s2", start_date: "2025-12-22", end_date: "2025-12-26", tz_offset: 540 },
    { _id: "s3", start_date: "2025-12-01", end_date: "2026-01-13", tz_offset: 480 },
    { _id: "s4", start_date: "2026-01-14", end_date: "2026-03-21", tz_offset: 60 },
    { _id: "s5", start_date: "2026-03-22", end_date: "2026-04-11", tz_offset: 480 },
    { _id: "s6", start_date: "2026-04-12", end_date: "2026-05-22", tz_offset: 60 },
  ],
  excludeTalkers: [],
}

function loadConfig(): SavedConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULT_CONFIG
    return JSON.parse(raw) as SavedConfig
  } catch {
    return DEFAULT_CONFIG
  }
}

function saveConfig(cfg: SavedConfig) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg))
  } catch {}
}

const PRIVACY_KEY = "annualReport.privacyMode"

/**
 * 私密模式下的姓名遮罩：保留首字，其余替换成圆点。
 * 保留首字是为了自己还能认出是谁 —— 全遮住的话排行榜就没法读了。
 * 圆点数量固定为 3，不随原名长度变化：长度本身也是识别信息
 *（「无敌虚的老抖 M 鬻」和「妈」遮完若长度不同，等于没遮）。
 */
function maskName(name: string | undefined | null): string {
  const s = (name ?? "").trim()
  if (!s) return "???"
  return Array.from(s)[0] + "•••"
}

export default function AnnualReportView() {
  const currentYear = new Date().getFullYear()
  const [inputYear, setInputYear] = useState(String(currentYear))
  const [defaultTz, setDefaultTz] = useState(() => loadConfig().defaultTz)
  const [segments, setSegments] = useState<SegmentRow[]>(() => loadConfig().segments)
  const [excludeTalkers, setExcludeTalkers] = useState<string[]>(() => loadConfig().excludeTalkers ?? [])
  const [galaxy, setGalaxy] = useState<RelationshipGraph | null>(null)
  const [showReview, setShowReview] = useState(true)
  useEffect(() => { galaxyApi.getGraph().then(setGalaxy).catch(() => setGalaxy(null)) }, [])
  // 私密模式：把联系人姓名和头像遮住，方便把报告截图/导 PDF 分享出去。
  // 存 localStorage，下次打开保持 —— 一旦有人习惯开着，默认关掉会造成意外泄露。
  const [privacyMode, setPrivacyMode] = useState(
    () => localStorage.getItem(PRIVACY_KEY) === "1"
  )
  useEffect(() => {
    localStorage.setItem(PRIVACY_KEY, privacyMode ? "1" : "0")
  }, [privacyMode])

  const [showTzPanel, setShowTzPanel] = useState(false)
  const [showExcludePanel, setShowExcludePanel] = useState(false)
  const [excludeSearch, setExcludeSearch] = useState("")
  const [savedTip, setSavedTip] = useState(false)
  const savedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // 已提交的参数，null = 未生成过
  const [params, setParams] = useState<ReportParams | null>(null)

  const { data: sessionsData } = useQuery({
    queryKey: ["sessions-for-exclude"],
    queryFn: () => sessionApi.getSessions({ limit: 5000 }),
    enabled: showExcludePanel,
  })

  const filteredSessions = useMemo(() => {
    if (!sessionsData?.items) return []
    const kw = excludeSearch.trim().toLowerCase()
    if (!kw) return sessionsData.items
    return sessionsData.items.filter(s =>
      (s.name || "").toLowerCase().includes(kw) ||
      (s.talkerName || "").toLowerCase().includes(kw) ||
      s.talker.toLowerCase().includes(kw)
    )
  }, [sessionsData, excludeSearch])

  // 排除名单：从服务端加载（与移动端共用同一份），失败则用本地缓存
  useEffect(() => {
    reportApi
      .getExcludeTalkers()
      .then((res) => {
        if (res && Array.isArray(res.talkers)) setExcludeTalkers(res.talkers)
      })
      .catch(() => {})
  }, [])

  const handleSaveConfig = () => {
    saveConfig({ defaultTz, segments, excludeTalkers })
    // 排除名单同步保存到服务端，移动端 / 其他设备共用
    reportApi.saveExcludeTalkers(excludeTalkers).catch(() => {})
    setSavedTip(true)
    if (savedTimerRef.current) clearTimeout(savedTimerRef.current)
    savedTimerRef.current = setTimeout(() => setSavedTip(false), 2000)
  }

  // 显示模式：progress = 进度条；stream = 数据流（边算边显示，默认）
  const [displayMode, setDisplayMode] = useState<"progress" | "stream">(() => {
    const v = localStorage.getItem("annual_report_display_mode")
    return v === "progress" ? "progress" : "stream"
  })
  useEffect(() => {
    localStorage.setItem("annual_report_display_mode", displayMode)
  }, [displayMode])

  // 流式生成
  const { state: stream, start: startStream } = useReportStream()

  // 多份缓存（按 (sig, dataVersion) 分别保存，无上限），跨会话生效
  const REPORT_CACHE_KEY = "annual_report_cache_v3"
  type CacheEntry = { sig: string; dataVersion: string; report: AnnualReport; ts: number }
  const [cacheList, setCacheList] = useState<CacheEntry[]>(() => {
    try {
      const raw = localStorage.getItem(REPORT_CACHE_KEY)
      if (!raw) return []
      const arr = JSON.parse(raw)
      if (!Array.isArray(arr)) return []
      // 清理污损条目：sig 里的 year 必须等于 report.year
      const cleaned = (arr as CacheEntry[]).filter((e) => {
        try {
          const sigObj = JSON.parse(e.sig)
          return sigObj.year === e.report?.year && !!e.dataVersion
        } catch {
          return false
        }
      })
      if (cleaned.length !== arr.length) {
        console.warn(`[Cache] 启动时清理了 ${arr.length - cleaned.length} 条污损缓存`)
        try { localStorage.setItem(REPORT_CACHE_KEY, JSON.stringify(cleaned)) } catch {}
      }
      return cleaned
    } catch {
      return []
    }
  })

  // 当前后端数据版本指纹
  const { data: dataVersionResp } = useQuery({
    queryKey: ["data-version"],
    queryFn: () => systemApi.getDataVersion(),
    staleTime: 30_000, // 30s 内复用
  })
  const currentDataVersion = dataVersionResp?.version ?? null

  // 命中缓存的条件：sig 配置相同 AND 数据指纹相同
  const findCache = (sig: string) => {
    const entry = cacheList.find(c => c.sig === sig && c.dataVersion === currentDataVersion)
    if (!entry) return undefined
    try {
      const sigObj = JSON.parse(sig)
      if (sigObj.year !== entry.report.year) {
        console.warn("[Cache] 检测到污损条目（sig.year != report.year），忽略", sigObj.year, "vs", entry.report.year)
        return undefined
      }
    } catch {}
    return entry
  }
  // 当前 params 对应的签名
  const currentSig = params ? JSON.stringify({
    year: params.year,
    defaultTz: params.defaultTz,
    segments: params.segments.map(({ _id, ...s }) => s),
    excludeTalkers: [...params.excludeTalkers].sort(),
  }) : null

  // 流式完成后写缓存 + localStorage
  // 注意：用 stream.params（流启动时绑定的参数）而非组件 params。
  // 这样即使用户中途切到别的（已缓存的）配置，旧流完成时仍能正确写回它自己那份缓存
  useEffect(() => {
    if (stream.status === "done" && stream.fullReport && stream.params) {
      const sp = stream.params
      // 防御：如果 stream.params.year 和 stream.fullReport.year 不一致，说明有竞态污染，拒绝写缓存
      if (sp.year !== stream.fullReport.year) {
        console.warn("[Cache] 拒绝写入：sp.year =", sp.year, "≠ fullReport.year =", stream.fullReport.year)
        return
      }
      const sig = JSON.stringify({
        year: sp.year,
        defaultTz: sp.defaultTzOffset,
        segments: sp.tzSegments,
        excludeTalkers: [...sp.excludeTalkers].sort(),
      })
      const dv = stream.fullReport.data_version || ""
      if (!dv) {
        console.warn("[Cache] 报告缺 data_version，跳过缓存")
        return
      }
      setCacheList(prev => {
        // 替换 (sig, dataVersion) 完全相同的旧条目；其他保留（同 sig 不同 dataVersion 视为不同条目）
        const filtered = prev.filter(c => !(c.sig === sig && c.dataVersion === dv))
        const next: CacheEntry[] = [{ sig, dataVersion: dv, report: stream.fullReport!, ts: Date.now() }, ...filtered]
        try { localStorage.setItem(REPORT_CACHE_KEY, JSON.stringify(next)) } catch (e) {
          console.warn("[Cache] localStorage 写入失败（可能配额已满）", e)
        }
        return next
      })
    }
  }, [stream.status, stream.fullReport, stream.params])

  // 流当前绑定的 sig（如果不匹配 currentSig 说明用户已切到别的配置）
  const streamSig = stream.params ? JSON.stringify({
    year: stream.params.year,
    defaultTz: stream.params.defaultTzOffset,
    segments: stream.params.tzSegments,
    excludeTalkers: [...stream.params.excludeTalkers].sort(),
  }) : null
  const streamMatchesCurrent = streamSig !== null && streamSig === currentSig

  // 拼出 data：流跟当前 sig 匹配且完成时用 fullReport；运行中流式模式且匹配取 partial；
  // 否则回落到 localStorage 缓存
  const data: AnnualReport | null = (streamMatchesCurrent ? stream.fullReport : null)
    ?? (streamMatchesCurrent && displayMode === "stream" && stream.status === "running"
      ? ({
          year: params?.year ?? currentYear,
          overview: stream.partial.overview ?? {
            total_messages: 0, sent_messages: 0, received_messages: 0,
            total_contacts: 0, active_contacts: 0,
            total_chatrooms: 0, active_chatrooms: 0,
            first_message_date: "", last_message_date: "", active_days: 0,
          },
          overview_deltas: stream.partial.overview_deltas,
          top_contacts: stream.partial.top_contacts ?? [],
          monthly_trend: stream.partial.monthly_trend ?? [],
          past_years_monthly_avg: stream.partial.past_years_monthly_avg ?? [],
          weekday_distribution: stream.partial.weekday_distribution ?? [],
          hourly_distribution: stream.partial.hourly_distribution ?? [],
          message_types: stream.partial.message_types ?? {},
          highlights: stream.partial.highlights ?? {
            busiest_day: { date: "", count: 0 },
            quietest_day: { date: "", count: 0 },
            longest_streak: 0,
            late_night_count: 0,
            earliest_message_time: "",
            latest_message_time: "",
          },
        } as AnnualReport)
      : null)
    ?? (currentSig ? findCache(currentSig)?.report ?? null : null)

  const isLoading = stream.status === "running" && displayMode === "progress"
  const error = stream.status === "error" ? stream.error : null

  const { data: effectiveStart } = useQuery({
    queryKey: ["effective-chat-start"],
    queryFn: () => systemApi.getEffectiveChatStart(),
  })

  const { data: liveBaseline } = useQuery({
    queryKey: ["report-baseline", params?.year, params?.defaultTz, effectiveStart?.year],
    queryFn: () => reportApi.getReportBaseline(params!.year, params!.defaultTz),
    enabled: !!params && stream.status === "done" && !!effectiveStart,
  })

  // 字数模式：任何地方（概览卡或排行行）需要字数都触发拉取，按 sig 缓存
  const [showChars, setShowChars] = useState(false)        // 概览卡切换
  const [topRowsNeedChars, setTopRowsNeedChars] = useState(false)  // 排行行需要字数
  const wordCountsEnabled = !!params && (showChars || topRowsNeedChars)
  const { data: wordCounts } = useQuery<WordCountStat>({
    queryKey: ["report-word-count", currentSig],
    queryFn: () => reportApi.getWordCounts(
      params!.year,
      params!.defaultTz,
      params!.segments.map(({ _id, ...s }) => s),
      params!.excludeTalkers
    ),
    enabled: wordCountsEnabled,
    staleTime: Infinity,
  })

  const handleGenerate = () => {
    const y = parseInt(inputYear)
    const year = y > 2000 && y <= currentYear ? y : currentYear
    const tzSegments = segments.map(({ _id, ...s }) => s)
    const sig = JSON.stringify({
      year,
      defaultTz,
      segments: tzSegments,
      excludeTalkers: [...excludeTalkers].sort(),
    })

    console.group("[Generate] click")
    console.log("inputYear:", inputYear, " parsed year:", year)
    console.log("defaultTz:", defaultTz)
    console.log("segments:", tzSegments)
    console.log("excludeTalkers:", excludeTalkers)
    console.log("computed sig:", sig)
    console.log("cached entries:")
    cacheList.forEach((c, i) => console.log(`  [${i}] sig=${c.sig}\n      report.year=${c.report.year}`))
    console.groupEnd()

    // 在缓存里找匹配的配置 → 直接复用
    const cached = findCache(sig)
    if (cached) {
      console.log("[Generate] CACHE HIT, reusing report.year =", cached.report.year)
      toast.info(`已复用 ${cached.report.year} 年的缓存报告（配置未变化）`)
      setParams({ year, defaultTz, segments, excludeTalkers })
      return
    }
    console.log("[Generate] cache miss, starting fresh stream")

    setParams({ year, defaultTz, segments, excludeTalkers })
    startStream({
      year,
      defaultTzOffset: defaultTz,
      tzSegments,
      excludeTalkers,
    })
  }

  const addSegment = () => {
    const year = params?.year ?? currentYear
    setSegments(prev => [...prev, newRow(year)])
  }
  const removeSegment = (id: string) => setSegments(prev => prev.filter(s => s._id !== id))
  const updateSegment = (id: string, patch: Partial<SegmentRow>) =>
    setSegments(prev => prev.map(s => s._id === id ? { ...s, ...patch } : s))

  if (isLoading) {
    const pct = stream.total > 0 ? Math.round((stream.current / stream.total) * 100) : 0
    const stepLabel: Record<string, string> = {
      overview: "统计概览数据",
      top_contacts: "计算亲密度排行",
      monthly_trend: "分析月度趋势",
      past_years_avg: "算往年月均",
      weekday_dist: "分析星期分布",
      hourly_dist: "分析小时分布",
      message_types: "统计消息类型",
      highlights: "提取年度亮点",
    }
    return (
      <div className="flex items-center justify-center h-full">
        <div className="flex flex-col items-center gap-5 max-w-md w-full px-6">
          <p className="text-base font-medium">正在生成 {params?.year} 年度报告</p>
          <div className="w-full">
            <div className="h-2 w-full bg-muted/50 rounded-full overflow-hidden">
              <div
                className="h-full bg-primary transition-all duration-300"
                style={{ width: `${pct}%` }}
              />
            </div>
            <div className="mt-2 flex justify-between text-xs text-muted-foreground">
              <span>{stepLabel[stream.step] || "准备中..."}</span>
              <span>{stream.current} / {stream.total} ({pct}%)</span>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center space-y-4">
          <p className="text-destructive font-medium text-sm">加载年度报告失败</p>
          <Button size="sm" onClick={handleGenerate}>重新生成</Button>
        </div>
      </div>
    )
  }

  // 配置区域（始终可见）
  const configPanel = (
    <div className="space-y-4 print-hide">
      <div className="flex items-center gap-2 flex-wrap">
        <Button variant="outline" size="sm" className="gap-1" onClick={() => window.print()}>
          导出 PDF
        </Button>
        <label className="flex cursor-pointer items-center gap-1 text-xs text-muted-foreground">
          <input type="checkbox" checked={showReview} onChange={(e) => setShowReview(e.target.checked)} className="accent-primary" />
          关系回顾
        </label>
        <Button
          variant={privacyMode ? "default" : "outline"}
          size="sm"
          className="gap-1"
          onClick={() => setPrivacyMode(p => !p)}
          title="遮住联系人姓名与头像，便于截图或导出 PDF 分享"
        >
          {privacyMode ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
          私密模式
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="gap-1"
          onClick={() => { setShowTzPanel(p => !p); setShowExcludePanel(false) }}
        >
          时区设置
          {segments.length > 0 && (
            <span className="ml-1 text-xs bg-primary/10 text-primary rounded px-1">{segments.length} 段</span>
          )}
          <ChevronDown className={`w-3.5 h-3.5 transition-transform ${showTzPanel ? "rotate-180" : ""}`} />
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="gap-1"
          onClick={() => { setShowExcludePanel(p => !p); setShowTzPanel(false) }}
        >
          排除设置
          {excludeTalkers.length > 0 && (
            <span className="ml-1 text-xs bg-destructive/10 text-destructive rounded px-1">{excludeTalkers.length}</span>
          )}
          <ChevronDown className={`w-3.5 h-3.5 transition-transform ${showExcludePanel ? "rotate-180" : ""}`} />
        </Button>
        <Input
          type="number"
          value={inputYear}
          onChange={(e) => setInputYear(e.target.value)}
          className={`w-24 h-9 ${
            params && parseInt(inputYear) !== params.year ? "border-amber-500 ring-1 ring-amber-500" : ""
          }`}
          min={2000}
          max={currentYear}
        />
        <Button
          size="sm"
          onClick={handleGenerate}
          className={
            params && parseInt(inputYear) !== params.year ? "bg-amber-600 hover:bg-amber-700" : ""
          }
        >
          生成报告
          {params && parseInt(inputYear) !== params.year && (
            <span className="ml-1 text-[10px] opacity-90">(待应用)</span>
          )}
        </Button>
        <div className="ml-auto flex items-center gap-1 text-xs">
          <span className="text-muted-foreground">显示模式</span>
          <button
            onClick={() => setDisplayMode("stream")}
            className={`h-7 px-2 rounded ${displayMode === "stream"
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"}`}
          >
            数据流
          </button>
          <button
            onClick={() => setDisplayMode("progress")}
            className={`h-7 px-2 rounded ${displayMode === "progress"
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"}`}
          >
            进度条
          </button>
          {cacheList.length > 0 && (
            <button
              onClick={() => {
                if (!confirm(`清空 ${cacheList.length} 份本地缓存？`)) return
                setCacheList([])
                try { localStorage.removeItem(REPORT_CACHE_KEY) } catch {}
                toast.info("缓存已清空")
              }}
              className="h-7 px-2 rounded text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              title={`本地缓存共 ${cacheList.length} 份`}
            >
              清缓存({cacheList.length})
            </button>
          )}
        </div>
      </div>

      {/* 排除联系人/群聊面板 */}
      {showExcludePanel && (
        <div className="border rounded-lg p-4 bg-muted/30 space-y-3">
          <p className="text-xs text-muted-foreground">勾选后，该联系人/群聊不计入亲密度排行。</p>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
            <Input
              value={excludeSearch}
              onChange={e => setExcludeSearch(e.target.value)}
              placeholder="搜索联系人或群聊..."
              className="h-8 pl-7 text-sm"
            />
          </div>
          {excludeTalkers.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {excludeTalkers.map(t => {
                const session = sessionsData?.items.find(s => s.talker === t)
                const label = session?.name || session?.talkerName || t
                return (
                  <span key={t} className="inline-flex items-center gap-1 text-xs bg-destructive/10 text-destructive rounded-full px-2 py-0.5">
                    {label}
                    <button onClick={() => setExcludeTalkers(prev => prev.filter(x => x !== t))}>
                      <X className="w-3 h-3" />
                    </button>
                  </span>
                )
              })}
            </div>
          )}
          <div className="max-h-52 overflow-y-auto border rounded-md divide-y bg-background">
            {filteredSessions.slice(0, 100).map(s => (
              <label key={s.talker} className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer text-sm">
                <input
                  type="checkbox"
                  className="accent-primary"
                  checked={excludeTalkers.includes(s.talker)}
                  onChange={e => {
                    setExcludeTalkers(prev =>
                      e.target.checked ? [...prev, s.talker] : prev.filter(x => x !== s.talker)
                    )
                  }}
                />
                <span className="truncate flex-1">{s.name || s.talkerName || s.talker}</span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {s.type === "group" ? "群聊" : s.type === "official" ? "公众号" : "私聊"}
                </span>
              </label>
            ))}
            {filteredSessions.length === 0 && (
              <div className="px-3 py-4 text-center text-xs text-muted-foreground">暂无数据</div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={handleSaveConfig}>保存配置</Button>
            {savedTip && <span className="text-xs text-green-600">已保存</span>}
          </div>
        </div>
      )}

      {/* 时区配置面板 */}
      {showTzPanel && (
        <div className="border rounded-lg p-4 bg-muted/30 space-y-3">
          <div className="flex items-center gap-3">
            <span className="text-sm text-muted-foreground w-24 shrink-0">默认时区</span>
            <select
              value={defaultTz}
              onChange={(e) => setDefaultTz(Number(e.target.value))}
              className="h-8 rounded-md border border-input bg-background px-2 text-sm"
            >
              {TZ_OPTIONS.map((tz) => (
                <option key={tz.offset} value={tz.offset}>{tz.label}</option>
              ))}
            </select>
            <span className="text-xs text-muted-foreground">（未被覆盖的日期使用此时区）</span>
          </div>

          {segments.length > 0 && (
            <div className="space-y-2">
              <div className="grid grid-cols-[1fr_1fr_180px_32px] gap-2 text-xs text-muted-foreground px-1">
                <span>开始日期</span><span>结束日期</span><span>该时段时区</span><span />
              </div>
              {segments.map((seg) => (
                <div key={seg._id} className="grid grid-cols-[1fr_1fr_180px_32px] gap-2 items-center">
                  <Input
                    type="date"
                    value={seg.start_date}
                    onChange={(e) => updateSegment(seg._id, { start_date: e.target.value })}
                    className="h-8 text-sm"
                  />
                  <Input
                    type="date"
                    value={seg.end_date}
                    onChange={(e) => updateSegment(seg._id, { end_date: e.target.value })}
                    className="h-8 text-sm"
                  />
                  <select
                    value={seg.tz_offset}
                    onChange={(e) => updateSegment(seg._id, { tz_offset: Number(e.target.value) })}
                    className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                  >
                    {TZ_OPTIONS.map((tz) => (
                      <option key={tz.offset} value={tz.offset}>{tz.label}</option>
                    ))}
                  </select>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-muted-foreground hover:text-destructive"
                    onClick={() => removeSegment(seg._id)}
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="gap-1" onClick={addSegment}>
              <Plus className="w-3.5 h-3.5" />
              添加时区段
            </Button>
            <Button size="sm" className="gap-1" onClick={handleSaveConfig}>
              保存配置
            </Button>
            {savedTip && (
              <span className="text-xs text-green-600">已保存</span>
            )}
          </div>
        </div>
      )}
    </div>
  )

  // 未生成过时显示引导界面
  if (!params) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="space-y-6 w-full max-w-2xl px-4">
          <div className="text-center">
            <h2 className="text-xl font-bold">年度社交报告</h2>
            <p className="text-sm text-muted-foreground mt-1">配置好参数后点击「生成报告」</p>
          </div>
          {configPanel}
        </div>
      </div>
    )
  }

  if (!data) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-muted-foreground text-sm">暂无数据</p>
      </div>
    )
  }

  return (
    <ScrollArea className="h-full">
      <div id="report-print" className="max-w-5xl mx-auto p-6 space-y-6 pb-20">
        {/* Header */}
        <div className="flex items-center justify-between flex-wrap gap-4">
          <div>
            <h2 className="text-2xl font-bold tracking-tight">
              {data?.year ?? params.year} 年度社交报告
            </h2>
            <p className="text-sm text-muted-foreground mt-1">
              你的微信年度数据回顾
            </p>
          </div>
          {configPanel}
        </div>

        {/* 流式生成时的进度条（只在用户停留在该流绑定的配置上时显示） */}
        {stream.status === "running" && streamMatchesCurrent && (
          <StreamProgressStrip current={stream.current} total={stream.total} step={stream.step} />
        )}
        {/* 后台还在跑别的配置的流：提示一下 */}
        {stream.status === "running" && !streamMatchesCurrent && (
          <div className="border rounded-lg bg-blue-500/5 px-4 py-2 text-xs text-muted-foreground">
            后台仍在生成 {stream.params?.year} 年报告（{stream.current}/{stream.total}），完成后自动写入缓存。
          </div>
        )}

        {/* Phase3 §14 关系回顾（来自关系星图，配置里可开关，随报告一起导出 PDF） */}
        {showReview && galaxy && (() => {
          const rv = computeYearReview(galaxy, String(data?.year ?? params.year))
          if (!rv || rv.top.length === 0) return null
          const quad: [string, number][] = [["新增重要关系", rv.newly.length], ["持续陪伴", rv.continued.length], ["重新恢复", rv.resumed.length], ["逐渐淡出", rv.faded.length]]
          return (
            <div className="rounded-2xl border bg-gradient-to-br from-primary/5 to-transparent p-5">
              <div className="mb-3 flex items-center gap-2">
                <span className="text-sm font-semibold">我的 {rv.year} 关系回顾</span>
                <span className="text-xs text-muted-foreground">来自关系星图{rv.stage ? " · " + rv.stage : ""}</span>
              </div>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {quad.map(([l, v]) => (
                  <div key={l} className="rounded-xl bg-muted/40 p-3"><div className="text-2xl font-bold">{v}</div><div className="text-xs text-muted-foreground">{l}</div></div>
                ))}
              </div>
              <div className="mt-3 text-xs text-muted-foreground">这一年陪伴你最多：<span className="text-foreground">{rv.top.map((t) => (privacyMode ? maskName(t.name) : t.name)).join("、")}</span></div>
              <div className="mt-1 text-xs text-muted-foreground">本年度活跃 {rv.activeMonths} 个月{rv.keywords.length ? " · 关键词：" + rv.keywords.join("、") : ""}</div>
            </div>
          )
        })()}

        {/* 区块渲染：流式运行中且该步未到达 → 骨架屏；其余情况 → 正常渲染（含缓存路径） */}
        {(() => {
          // 只有流跟当前 sig 匹配时，缺失的步骤才用骨架；否则用缓存数据，不显示骨架
          const isStreaming = stream.status === "running" && streamMatchesCurrent
          const skeleton = (step: any) => isStreaming && !stream.loadedSteps.has(step)
          return (
            <>
              {skeleton("overview") ? (
                <SectionSkeleton title="概览数据" rows={1} cols={6} />
              ) : (
                <AnnualOverviewCards
                  overview={data.overview}
                  deltas={
                    liveBaseline?.past_overview_avg
                      ? computeDeltasFromAvg(data.overview, liveBaseline.past_overview_avg)
                      : data.overview_deltas
                  }
                  wordCounts={wordCounts}
                  showChars={showChars}
                  onToggleChars={() => setShowChars(p => !p)}
                />
              )}

              {skeleton("highlights") ? (
                <SectionSkeleton title="年度亮点" rows={2} cols={3} />
              ) : (
                <AnnualHighlightsSection highlights={data.highlights} />
              )}

              {skeleton("monthly_trend") ? (
                <SectionSkeleton title="月度消息趋势" chart />
              ) : (
                <MonthlyTrendChart
                  data={data.monthly_trend}
                  year={data.year}
                  pastYearsAvg={liveBaseline?.past_years_monthly_avg ?? data.past_years_monthly_avg}
                  pastStartYear={liveBaseline?.past_start_year}
                  pastEndYear={liveBaseline?.past_end_year}
                />
              )}

              {skeleton("top_contacts") ? (
                <SectionSkeleton title="亲密度排行 TOP 10" rows={5} cols={2} />
              ) : (
                <TopContactsSection
                  privacyMode={privacyMode}
                  contacts={data.top_contacts}
                  wordCounts={wordCounts}
                  onNeedChars={() => setTopRowsNeedChars(true)}
                />
              )}

              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                {skeleton("weekday_dist") ? (
                  <SectionSkeleton title="星期分布" chart />
                ) : (
                  <WeekdayChart data={data.weekday_distribution} />
                )}
                {skeleton("hourly_dist") ? (
                  <SectionSkeleton title="24小时分布" chart />
                ) : (
                  <HourlyChart data={data.hourly_distribution} />
                )}
              </div>

              {skeleton("message_types") ? (
                <SectionSkeleton title="消息类型分布" chart />
              ) : (
                <MessageTypesChart types={data.message_types} />
              )}
            </>
          )
        })()}
      </div>
    </ScrollArea>
  )
}

// 由「往年同期 overview 平均」+「当前 overview」计算 OverviewDeltas
function computeDeltasFromAvg(
  current: AnnualReport["overview"],
  past: AnnualOverview
) {
  const calc = (cur: number, p: number): number | null =>
    p === 0 ? null : ((cur - p) / p) * 100
  return {
    total_messages: calc(current.total_messages, past.total_messages),
    sent_messages: calc(current.sent_messages, past.sent_messages),
    received_messages: calc(current.received_messages, past.received_messages),
    active_contacts: calc(current.active_contacts, past.active_contacts),
    active_chatrooms: calc(current.active_chatrooms, past.active_chatrooms),
    active_days: calc(current.active_days, past.active_days),
  }
}

function DeltaSup({ delta }: { delta: number | null | undefined }) {
  if (delta === null || delta === undefined) return null
  // 红色：减少 ≤ -10%；绿色：增长 ≥ +10%；其余灰色
  const sign = delta > 0 ? "+" : ""
  let cls = "text-muted-foreground"
  if (delta >= 10) cls = "text-green-600 dark:text-green-400"
  else if (delta <= -10) cls = "text-red-600 dark:text-red-400"
  return (
    <sup className={`ml-1 text-[10px] font-semibold ${cls}`}>
      {sign}{delta.toFixed(1)}%
    </sup>
  )
}

function AnimatedNumber({ target, suffix = "" }: { target: number; suffix?: string }) {
  const v = useCountUp(target, 800)
  return <>{formatNumber(v)}{suffix}</>
}

function AnnualOverviewCards({
  overview,
  deltas,
  wordCounts,
  showChars,
  onToggleChars,
}: {
  overview: AnnualReport["overview"]
  deltas?: AnnualReport["overview_deltas"] | null
  wordCounts?: WordCountStat
  showChars?: boolean
  onToggleChars?: () => void
}) {
  // 三张可切换的卡（消息总数 / 发送 / 接收）会在字数模式下显示字数
  const charsLoading = showChars && !wordCounts
  const showCharsReady = showChars && !!wordCounts

  type Stat = {
    label: string
    target: number
    suffix: string
    delta?: number | null
    icon: any
    color: string
    toggleable?: boolean
    sub?: string
  }
  const stats: Stat[] = [
    {
      label: showCharsReady ? "总字数" : "消息总数",
      target: showCharsReady ? wordCounts!.total_chars : overview.total_messages,
      suffix: showCharsReady ? " 字" : "",
      delta: showCharsReady ? null : deltas?.total_messages,
      icon: MessageSquare, color: "text-pink-500",
      toggleable: true,
      // 语音转写出来的字也算「说了多少字」，单独标出来有多少来自语音
      sub:
        showCharsReady && (wordCounts!.voice_chars ?? 0) > 0
          ? `其中语音转写 ${formatNumber(wordCounts!.voice_chars!)} 字`
          : undefined,
    },
    {
      label: showCharsReady ? "发送字数" : "发送消息",
      target: showCharsReady ? wordCounts!.sent_chars : overview.sent_messages,
      suffix: showCharsReady ? " 字" : "",
      delta: showCharsReady ? null : deltas?.sent_messages,
      icon: TrendingUp, color: "text-blue-500",
      toggleable: true,
    },
    {
      label: showCharsReady ? "接收字数" : "接收消息",
      target: showCharsReady ? wordCounts!.recv_chars : overview.received_messages,
      suffix: showCharsReady ? " 字" : "",
      delta: showCharsReady ? null : deltas?.received_messages,
      icon: TrendingUp, color: "text-violet-500",
      toggleable: true,
    },
    { label: "活跃联系人", target: overview.active_contacts, suffix: "", delta: deltas?.active_contacts, icon: Users, color: "text-green-500" },
    { label: "活跃群聊", target: overview.active_chatrooms, suffix: "", delta: deltas?.active_chatrooms, icon: Users, color: "text-orange-500" },
    { label: "活跃天数", target: overview.active_days, suffix: " 天", delta: deltas?.active_days, icon: CalendarDays, color: "text-cyan-500" },
  ]

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
      {stats.map((s) => (
        <Card
          key={s.label}
          className={s.toggleable ? "cursor-pointer hover:ring-1 hover:ring-primary/40 transition-shadow" : ""}
          onClick={s.toggleable ? onToggleChars : undefined}
          title={s.toggleable ? "点击切换：消息数 ↔ 字数" : undefined}
        >
          <CardContent className="p-4">
            <div className="flex items-center gap-2 mb-2">
              <s.icon className={`w-4 h-4 ${s.color}`} />
              <span className="text-xs text-muted-foreground">{s.label}</span>
            </div>
            <div className="text-xl font-bold">
              {s.toggleable && charsLoading ? (
                <span className="text-muted-foreground text-sm">加载中...</span>
              ) : (
                <>
                  <AnimatedNumber target={s.target} suffix={s.suffix} />
                  <DeltaSup delta={s.delta ?? null} />
                </>
              )}
            </div>
            {s.sub && !charsLoading && (
              <p className="mt-1 text-[11px] text-muted-foreground">{s.sub}</p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function AnnualHighlightsSection({ highlights }: { highlights: AnnualReport["highlights"] }) {
  const items = [
    { label: "最忙碌的一天", value: highlights.busiest_day.date, sub: `${formatNumber(highlights.busiest_day.count)} 条消息`, icon: Flame, color: "from-red-50 dark:from-red-950/20" },
    { label: "最安静的一天", value: highlights.quietest_day.date, sub: `${highlights.quietest_day.count} 条消息`, icon: Moon, color: "from-blue-50 dark:from-blue-950/20" },
    { label: "最长连续活跃", value: `${highlights.longest_streak} 天`, sub: "连续聊天记录", icon: Trophy, color: "from-yellow-50 dark:from-yellow-950/20" },
    { label: "深夜消息", value: formatNumber(highlights.late_night_count), sub: "23:00 - 05:00", icon: Moon, color: "from-purple-50 dark:from-purple-950/20" },
    { label: "最早消息", value: highlights.earliest_message_time, sub: "当日最早一条", icon: Clock, color: "from-green-50 dark:from-green-950/20" },
    { label: "最晚消息", value: highlights.latest_message_time, sub: "当日最晚一条", icon: Clock, color: "from-indigo-50 dark:from-indigo-950/20" },
  ]

  return (
    <div>
      <h3 className="text-lg font-bold mb-4 flex items-center gap-2">
        <Flame className="w-5 h-5 text-orange-500" />
        年度亮点
      </h3>
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
        {items.map((item) => (
          <Card key={item.label} className={`border-none shadow-sm bg-gradient-to-br ${item.color} to-white dark:to-background`}>
            <CardContent className="p-4">
              <div className="flex items-center gap-2 mb-2">
                <item.icon className="w-4 h-4 text-muted-foreground" />
                <span className="text-xs text-muted-foreground font-medium">{item.label}</span>
              </div>
              <div className="text-lg font-bold">{item.value}</div>
              <div className="text-xs text-muted-foreground">{item.sub}</div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}

function MonthlyTrendChart({
  data,
  year,
  pastYearsAvg,
  pastStartYear,
  pastEndYear,
}: {
  data: AnnualReport["monthly_trend"]
  /** 报告年份 —— 用来判断哪些月份还没到 */
  year: number
  pastYearsAvg: AnnualReport["past_years_monthly_avg"]
  pastStartYear?: number
  pastEndYear?: number
}) {
  const [showAvg, setShowAvg] = useState(true)

  const avgByMonth = new Array(13).fill(0)
  for (const s of (pastYearsAvg || [])) {
    if (s.month >= 1 && s.month <= 12) avgByMonth[s.month] = s.count
  }
  const hasAvg = avgByMonth.slice(1).some((v) => v > 0)
  const rangeText = pastStartYear && pastEndYear && pastStartYear <= pastEndYear
    ? `${pastStartYear}-${pastEndYear}年`
    : ""

  // 后端的 monthly_trend 恒定补满 1~12 月（report_singlepass.go），
  // 所以跑「今年」的报告时，还没到的月份会是一串 0 —— 画出来像年底突然断崖。
  // 给 null 让线直接停在当月：recharts 默认 connectNulls=false，不连、不画点，
  // Tooltip 的 filterNull 默认 true，也不会列出来。
  // 往年的报告 12 个月都是完整的，不裁。
  const now = new Date()
  const monthCap = year > now.getFullYear() ? 0
    : year === now.getFullYear() ? now.getMonth() + 1
    : 12

  const chartData = (data || []).map((d) => ({
    name: `${d.month}月`,
    count: d.month > monthCap ? null : d.count,
    avg: avgByMonth[d.month] || 0,
  }))

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 flex-wrap">
          <TrendingUp className="w-4 h-4 text-pink-500" />
          月度消息趋势
          {hasAvg && rangeText && (
            <span className="text-xs text-muted-foreground font-normal">
              · 平均值统计范围：{rangeText}
            </span>
          )}
          {hasAvg && (
            <label className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground font-normal cursor-pointer select-none">
              <input
                type="checkbox"
                checked={showAvg}
                onChange={(e) => setShowAvg(e.target.checked)}
                className="accent-primary"
              />
              显示往年月均
            </label>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[300px] w-full" style={{ minWidth: 0, minHeight: 0 }}>
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
              <XAxis dataKey="name" style={{ fontSize: "12px" }} />
              <YAxis style={{ fontSize: "12px" }} />
              <Tooltip
                contentStyle={{
                  borderRadius: "8px",
                  border: "none",
                  boxShadow: "0 4px 12px rgba(0,0,0,0.1)",
                }}
              />
              <Line
                type="monotone"
                dataKey="count"
                stroke="#ec4899"
                strokeWidth={2}
                dot={{ fill: "#ec4899", r: 4 }}
                name="消息数"
              />
              {hasAvg && showAvg && (
                <Line
                  type="monotone"
                  dataKey="avg"
                  stroke="#6366f1"
                  strokeWidth={2}
                  strokeDasharray="6 4"
                  dot={false}
                  name="往年月均"
                />
              )}
            </LineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}

function TopContactsSection({
  contacts,
  wordCounts,
  onNeedChars,
  privacyMode = false,
}: {
  contacts: AnnualReport["top_contacts"]
  wordCounts?: WordCountStat
  onNeedChars?: () => void
  /** 私密模式：遮住姓名与头像，便于分享 */
  privacyMode?: boolean
}) {
  const top10 = (contacts || []).slice(0, 10)
  const charMap = new Map<string, { sentChars: number; recvChars: number; totalChars: number }>()
  for (const c of (wordCounts?.contacts || [])) {
    charMap.set(c.talker, { sentChars: c.sentChars, recvChars: c.recvChars, totalChars: c.totalChars })
  }
  const [charsRows, setCharsRows] = useState<Set<string>>(new Set())
  const toggleRow = (talker: string) => {
    onNeedChars?.()  // 通知父组件可以拉字数了
    setCharsRows(prev => {
      const next = new Set(prev)
      if (next.has(talker)) next.delete(talker); else next.add(talker)
      return next
    })
  }

  return (
    <div>
      <h3 className="text-lg font-bold mb-4 flex items-center gap-2">
        <Trophy className="w-5 h-5 text-yellow-500" />
        亲密度排行 TOP 10
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {top10.map((contact, idx) => {
          const showChars = charsRows.has(contact.talker)
          const cm = charMap.get(contact.talker)
          const charsAvailable = !!wordCounts // word counts data has been loaded
          const charsLoading = showChars && !cm && !charsAvailable
          return (
            <Card key={contact.talker} className="border-none shadow-sm hover:shadow-md transition-shadow">
              <CardContent className="p-4 flex items-center gap-4">
                <div className="text-lg font-black text-muted-foreground/30 w-6 text-center">
                  {idx + 1}
                </div>
                <Avatar className="h-10 w-10 border shadow-sm">
                  {/* 私密模式下不加载头像 —— 只遮名字没意义，脸比名字更能认人 */}
                  {!privacyMode && (
                    <AvatarImage src={contact.avatar && (contact.avatar.startsWith('http') ? contact.avatar : mediaApi.getAvatarUrl(`avatar/${contact.talker}`))} />
                  )}
                  <AvatarFallback>{contact.name?.substring(0, 1) || "?"}</AvatarFallback>
                </Avatar>
                <div className="flex-1 min-w-0">
                  <div className="font-medium text-sm truncate flex items-center gap-1.5">
                    {privacyMode ? maskName(contact.name) : contact.name}
                    {contact.isGroup && (
                      <span className="text-[10px] bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 rounded px-1 shrink-0">群聊</span>
                    )}
                  </div>
                  <div
                    className="text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors select-none"
                    onClick={() => toggleRow(contact.talker)}
                    title="点击切换：消息数 ↔ 字数"
                  >
                    {charsLoading ? (
                      "加载字数..."
                    ) : showChars && cm ? (
                      <>发 {formatNumber(cm.sentChars)} 字 / 收 {formatNumber(cm.recvChars)} 字</>
                    ) : (
                      <>发送 {formatNumber(contact.sentCount)} / 接收 {formatNumber(contact.recvCount)}</>
                    )}
                  </div>
                </div>
                <div
                  className="text-right cursor-pointer hover:bg-muted/50 rounded px-2 py-1 transition-colors"
                  onClick={() => toggleRow(contact.talker)}
                  title="点击切换：消息数 ↔ 字数"
                >
                  <div className="text-lg font-bold text-primary">
                    {charsLoading ? "..." : showChars && cm
                      ? formatNumber(cm.totalChars)
                      : formatNumber(contact.messageCount)
                    }
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    {showChars && cm ? "总字数" : "总消息"}
                  </div>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}

function WeekdayChart({ data }: { data: AnnualReport["weekday_distribution"] }) {
  const chartData = (data || []).map((d) => ({
    name: WEEKDAY_NAMES[d.weekday] || `Day ${d.weekday}`,
    count: d.count,
  }))

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">星期分布</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[250px] w-full" style={{ minWidth: 0, minHeight: 0 }}>
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
              <XAxis dataKey="name" style={{ fontSize: "12px" }} />
              <YAxis style={{ fontSize: "12px" }} />
              <Tooltip />
              <Bar dataKey="count" radius={[4, 4, 0, 0]} name="消息数">
                {chartData.map((_, i) => (
                  <Cell key={i} fill={i === 0 || i === 6 ? "#ec4899" : "#6366f1"} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}

function HourlyChart({ data }: { data: AnnualReport["hourly_distribution"] }) {
  const chartData = (data || []).map((d) => ({
    name: `${d.hour}时`,
    count: d.count,
  }))

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">24小时分布</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[250px] w-full" style={{ minWidth: 0, minHeight: 0 }}>
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
              <XAxis dataKey="name" style={{ fontSize: "10px" }} interval={2} />
              <YAxis style={{ fontSize: "12px" }} />
              <Tooltip />
              <Bar dataKey="count" radius={[2, 2, 0, 0]} fill="#6366f1" name="消息数" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}

function MessageTypesChart({ types }: { types: Record<string, number> }) {
  const TYPE_LABELS: Record<string, string> = {
    text: "文本",
    image: "图片",
    voice: "语音",
    video: "视频",
    link: "链接",
    other: "其他",
  }
  const COLORS = ["#ec4899", "#6366f1", "#06b6d4", "#f59e0b", "#10b981", "#8b5cf6"]

  const pieData = Object.entries(types || {})
    .sort((a, b) => b[1] - a[1])
    .map(([key, value], i) => ({
      name: TYPE_LABELS[key] || key,
      value,
      color: COLORS[i % COLORS.length],
    }))

  const total = pieData.reduce((s, d) => s + d.value, 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">消息类型分布</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col md:flex-row items-center gap-8">
          <div className="h-[250px] w-[250px]" style={{ minWidth: 0, minHeight: 0 }}>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={pieData}
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={90}
                  paddingAngle={3}
                  dataKey="value"
                >
                  {pieData.map((entry, i) => (
                    <Cell key={i} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-3">
            {pieData.map((item) => (
              <div key={item.name} className="flex items-center gap-2">
                <div className="w-3 h-3 rounded-full shrink-0" style={{ backgroundColor: item.color }} />
                <span className="text-sm">{item.name}</span>
                <span className="text-sm font-bold ml-auto">{formatNumber(item.value)}</span>
                <span className="text-xs text-muted-foreground">
                  ({total > 0 ? ((item.value / total) * 100).toFixed(1) : 0}%)
                </span>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

const STEP_LABEL: Record<string, string> = {
  overview: "概览数据",
  top_contacts: "亲密度排行",
  monthly_trend: "月度趋势",
  past_years_avg: "往年月均",
  weekday_dist: "星期分布",
  hourly_dist: "小时分布",
  message_types: "消息类型",
  highlights: "年度亮点 + 同期对比",
}

function StreamProgressStrip({ current, total, step }: { current: number; total: number; step: string }) {
  const pct = total > 0 ? Math.round((current / total) * 100) : 0
  return (
    <div className="border rounded-lg bg-muted/30 px-4 py-3">
      <div className="flex items-center justify-between text-xs text-muted-foreground mb-1.5">
        <span>
          正在统计：
          <span className="text-foreground font-medium ml-1">
            {STEP_LABEL[step] || (current === 0 ? "准备中..." : "下一步...")}
          </span>
        </span>
        <span>{current} / {total} ({pct}%)</span>
      </div>
      <div className="h-1.5 w-full bg-background rounded-full overflow-hidden">
        <div className="h-full bg-primary transition-all duration-300" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

function SectionSkeleton({
  title, rows = 1, cols = 1, chart,
}: { title: string; rows?: number; cols?: number; chart?: boolean }) {
  return (
    <div>
      <h3 className="text-sm font-medium mb-3 text-muted-foreground flex items-center gap-2">
        <span className="inline-block w-2 h-2 bg-primary rounded-full animate-pulse" />
        {title} <span className="text-xs">加载中...</span>
      </h3>
      {chart ? (
        <div className="border rounded-lg p-6 bg-muted/20">
          <div className="h-[260px] w-full bg-muted/40 rounded animate-pulse" />
        </div>
      ) : (
        <div
          className={`grid gap-3`}
          style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        >
          {Array.from({ length: rows * cols }).map((_, i) => (
            <div key={i} className="border rounded-lg p-4 bg-muted/20">
              <div className="h-3 w-16 bg-muted/40 rounded animate-pulse mb-2" />
              <div className="h-6 w-24 bg-muted/40 rounded animate-pulse" />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
