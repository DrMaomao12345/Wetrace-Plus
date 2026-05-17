import { useState, useMemo } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "react-router-dom"
import { aiApi, type SummaryHistoryItem } from "@/api/ai"
import { useSessions } from "@/hooks/useSession"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import {
  BrainCircuit, Trash2, ChevronDown, ChevronUp,
  CheckCircle2, XCircle, RefreshCw, MessageSquare, SlidersHorizontal,
  Loader2, StopCircle, Copy, Check,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { toast } from "sonner"

export const SUMMARY_RETRY_KEY = "wetrace_summary_retry"

function formatDate(iso: string) {
  const d = new Date(iso)
  return d.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
}

function getYear(iso: string) {
  return new Date(iso).getFullYear().toString()
}

export default function AISummaryHistoryView() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { data: sessions = [] } = useSessions()
  const { data: history = [], isLoading } = useQuery({
    queryKey: ["ai-summary-history"],
    queryFn: () => aiApi.getSummaryHistory(),
    refetchInterval: (query) => {
      const data = query.state.data as SummaryHistoryItem[] | undefined
      return data?.some((item) => item.status === "running") ? 2000 : false
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => aiApi.deleteSummaryHistory(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-summary-history"] })
      toast.success("已删除")
    },
    onError: () => toast.error("删除失败"),
  })

  const cancelMutation = useMutation({
    mutationFn: () => aiApi.cancelSummarize(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-summary-history"] })
      toast.success("已中止总结")
    },
    onError: () => toast.error("中止失败"),
  })

  const sessionName = (talker: string) =>
    sessions.find((s) => s.talker === talker)?.name || talker

  const handleRetry = (item: SummaryHistoryItem) => {
    sessionStorage.setItem(SUMMARY_RETRY_KEY, JSON.stringify({
      talker: item.talker,
      time_range: item.time_range,
      prompt: item.prompt_used,
      retry_of: item.id,
    }))
    navigate("/chat")
  }

  // 按年份分组，最新在前
  const grouped = useMemo(() => {
    const map = new Map<string, SummaryHistoryItem[]>()
    for (const item of history) {
      const year = getYear(item.created_at)
      if (!map.has(year)) map.set(year, [])
      map.get(year)!.push(item)
    }
    return Array.from(map.entries()).sort((a, b) => Number(b[0]) - Number(a[0]))
  }, [history])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        加载中...
      </div>
    )
  }

  return (
    <ScrollArea className="h-full">
      <div className="max-w-3xl mx-auto p-6 space-y-6 pb-20">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">AI 总结历史</h2>
          <p className="text-sm text-muted-foreground mt-1">
            共 {history.length} 条记录
          </p>
        </div>

        {history.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24 gap-4">
            <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
              <BrainCircuit className="w-8 h-8 text-muted-foreground/30" />
            </div>
            <p className="text-muted-foreground text-sm">暂无总结记录</p>
          </div>
        ) : (
          grouped.map(([year, items]) => (
            <div key={year} className="space-y-3">
              <div className="flex items-center gap-2">
                <span className="text-xs font-bold text-muted-foreground uppercase tracking-wider">{year}</span>
                <div className="flex-1 h-px bg-border" />
                <span className="text-xs text-muted-foreground">{items.length} 条</span>
              </div>
              {items.map((item) => (
                <HistoryCard
                  key={item.id}
                  item={item}
                  sessionName={sessionName(item.talker)}
                  onDelete={() => deleteMutation.mutate(item.id)}
                  onRetry={() => handleRetry(item)}
                  onCancel={() => cancelMutation.mutate()}
                  deleting={deleteMutation.isPending}
                  cancelling={cancelMutation.isPending}
                />
              ))}
            </div>
          ))
        )}
      </div>
    </ScrollArea>
  )
}

function HistoryCard({
  item,
  sessionName,
  onDelete,
  onRetry,
  onCancel,
  deleting,
  cancelling,
}: {
  item: SummaryHistoryItem
  sessionName: string
  onDelete: () => void
  onRetry: () => void
  onCancel: () => void
  deleting: boolean
  cancelling: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const [showPrompt, setShowPrompt] = useState(false)
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    if (!item.summary) return
    navigator.clipboard.writeText(item.summary).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  const isRunning = item.status === "running"
  const isSuccess = item.status === "success"
  const isFailed = item.status === "failed"

  return (
    <Card className={cn(
      "transition-all hover:shadow-md",
      isRunning && "border-primary/40 bg-primary/5",
      isFailed && "border-destructive/40 bg-destructive/5"
    )}>
      <CardContent className="p-4 space-y-3">
        {/* 头部：状态、会话名、时间、操作 */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            {isRunning ? (
              <Loader2 className="w-4 h-4 text-primary shrink-0 animate-spin" />
            ) : isSuccess ? (
              <CheckCircle2 className="w-4 h-4 text-green-500 shrink-0" />
            ) : (
              <XCircle className="w-4 h-4 text-destructive shrink-0" />
            )}
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{sessionName}</span>
                {isRunning && (
                  <span className="text-[10px] bg-primary/10 text-primary px-1.5 py-0.5 rounded-full">
                    进行中
                  </span>
                )}
                {item.retry_count > 0 && (
                  <span className="text-[10px] bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400 px-1.5 py-0.5 rounded-full">
                    第 {item.retry_count} 次重试
                  </span>
                )}
              </div>
              <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                <span>{formatDate(item.created_at)}</span>
                {item.msg_count > 0 && (
                  <span className="flex items-center gap-0.5">
                    <MessageSquare className="w-3 h-3" />
                    {item.msg_count} 条消息
                  </span>
                )}
                {item.time_range && <span>范围: {item.time_range}</span>}
              </div>
            </div>
          </div>
          {isRunning ? (
            <Button
              variant="ghost"
              size="icon"
              className="w-7 h-7 shrink-0 text-muted-foreground hover:text-destructive"
              onClick={onCancel}
              disabled={cancelling}
              title="中止总结"
            >
              <StopCircle className="w-3.5 h-3.5" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              className="w-7 h-7 shrink-0 text-muted-foreground hover:text-destructive"
              onClick={onDelete}
              disabled={deleting}
            >
              <Trash2 className="w-3.5 h-3.5" />
            </Button>
          )}
        </div>

        {/* 进行中提示 */}
        {isRunning && (
          <div className="text-xs text-primary/70 bg-primary/5 rounded px-3 py-2 flex items-center gap-2">
            <Loader2 className="w-3 h-3 animate-spin shrink-0" />
            AI 正在分析聊天记录，请稍候…
          </div>
        )}

        {/* 失败原因 */}
        {isFailed && item.error && (
          <div className="text-xs text-destructive bg-destructive/10 rounded px-3 py-2 break-all">
            {item.error}
          </div>
        )}

        {/* 提示词折叠 */}
        {item.prompt_used && (
          <button
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => setShowPrompt(!showPrompt)}
          >
            <SlidersHorizontal className="w-3 h-3" />
            使用的提示词
            {showPrompt ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
          </button>
        )}
        {showPrompt && (
          <div className="text-xs font-mono bg-muted/50 rounded px-3 py-2 whitespace-pre-wrap max-h-32 overflow-y-auto">
            {item.prompt_used}
          </div>
        )}

        {/* 总结内容折叠 */}
        {isSuccess && item.summary && (
          <>
            <div className="flex items-center justify-between gap-2">
              <button
                className="flex items-center gap-1 text-xs text-primary hover:underline"
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? (
                  <><ChevronUp className="w-3 h-3" />收起</>
                ) : (
                  <><ChevronDown className="w-3 h-3" />展开全文</>
                )}
              </button>
              <button
                className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
                onClick={handleCopy}
              >
                {copied ? <Check className="w-3 h-3 text-green-500" /> : <Copy className="w-3 h-3" />}
                {copied ? "已复制" : "复制"}
              </button>
            </div>
            <div
              className={cn(
                "text-sm text-foreground/80 leading-relaxed whitespace-pre-wrap",
                !expanded && "line-clamp-3"
              )}
            >
              {item.summary}
            </div>
          </>
        )}

        {/* 重试入口（失败时） */}
        {isFailed && (
          <div className="pt-1">
            <Button
              size="sm"
              variant="outline"
              className="h-7 text-xs gap-1.5"
              onClick={onRetry}
            >
              <RefreshCw className="w-3 h-3" />
              前往重试
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
