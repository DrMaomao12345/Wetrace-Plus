import { SessionList } from "@/components/chat/SessionList"
import { openSafe } from "@/lib/openSafe"
import { MessageList } from "@/components/chat/MessageList"
import { useAppStore } from "@/stores/app"
import { cn } from "@/lib/utils"
import { DailyReportCard } from "@/components/DailyReportCard"
import { useChat } from "@/hooks/useChat"
import { RefreshCw, ArrowLeft, Smile, PlusCircle, Mic, Download, Sparkles, Images, BrainCircuit, MessageSquareQuote, MoreHorizontal, Type } from "lucide-react"
import { Button } from "@/components/ui/button"
import { mediaApi } from "@/api"
import { toast } from "sonner"
import { aiApi } from "@/api/ai"
import { createPortal } from "react-dom"
import { useState, useMemo, useRef, useCallback, useEffect } from "react"
import { useSessions } from "@/hooks/useSession"
import { useMessages } from "@/hooks/useChatLog"
import { AnalysisPanel } from "@/components/analysis/AnalysisPanel"
import { ExportModal } from "@/components/chat/ExportModal"
import { AISummaryModal } from "@/components/ai/AISummaryModal"
import { SUMMARY_RETRY_KEY } from "@/views/AISummaryHistory"
import { AISimulateChat } from "@/components/ai/AISimulateChat"
import { SessionGalleryModal } from "@/components/chat/SessionGalleryModal"

export default function Chat() {
  const isMobile = useAppStore((state) => state.isMobile)
  const { activeTalker, setActiveTalker } = useChat()
  const [showAnalysis, setShowAnalysis] = useState(false)
  const [showExportModal, setShowExportModal] = useState(false)
  
  // AI States
  const [showAISummary, setShowAISummary] = useState(false)
  const [aiSummary, setAiSummary] = useState("")
  const [aiSummaryError, setAiSummaryError] = useState("")
  const [lastSummaryHistoryId] = useState("")
  const [isSummarizing, setIsSummarizing] = useState(false)
  const [summaryInitialPrompt, setSummaryInitialPrompt] = useState("")
  const [summaryInitialTimeRange, setSummaryInitialTimeRange] = useState("")
  const [showAISimulate, setShowAISimulate] = useState(false)
  const [showSessionGallery, setShowSessionGallery] = useState(false)
  const [showMoreMenu, setShowMoreMenu] = useState(false)
  const moreButtonRef = useRef<HTMLButtonElement>(null)
  const [menuPos, setMenuPos] = useState({ top: 0, right: 0 })

  // Batch voice transcription
  const [isBatchTranscribing, setIsBatchTranscribing] = useState(false)
  const [batchProgress, setBatchProgress] = useState<{ done: number; total: number } | null>(null)
  const batchPollRef = useRef<number | null>(null)

  const openMoreMenu = useCallback(() => {
    if (moreButtonRef.current) {
      const rect = moreButtonRef.current.getBoundingClientRect()
      setMenuPos({
        top: rect.bottom + 4,
        right: window.innerWidth - rect.right,
      })
    }
    setShowMoreMenu(true)
  }, [])

  const isGroupChat = useMemo(() => {
    return activeTalker?.endsWith('@chatroom')
  }, [activeTalker])

  // 从历史页「前往重试」跳转过来时，自动打开弹窗并触发总结
  useEffect(() => {
    const raw = sessionStorage.getItem(SUMMARY_RETRY_KEY)
    if (!raw) return
    sessionStorage.removeItem(SUMMARY_RETRY_KEY)
    try {
      const { talker, time_range, prompt, retry_of } = JSON.parse(raw)
      if (talker) {
        setActiveTalker(talker)
        setSummaryInitialPrompt(prompt || "")
        setSummaryInitialTimeRange(time_range || "")
        setShowAISummary(true)
        // 延迟一帧等 activeTalker 更新后再触发
        setTimeout(() => {
          handleAISummarize(time_range || undefined, prompt || undefined, retry_of || undefined)
        }, 100)
      }
    } catch {}
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleAISummarize = async (timeRange?: string, customPrompt?: string, retryOf?: string) => {
    if (!activeTalker) return
    setIsSummarizing(true)
    setAiSummaryError("")
    setAiSummary("")
    try {
      const res = await aiApi.summarize({
        talker: activeTalker,
        time_range: timeRange,
        custom_prompt: customPrompt,
        retry_of: retryOf,
      })
      setAiSummary(res)
    } catch (err: any) {
      const msg = err?.message || "AI 总结失败，请检查后端 AI 配置是否正确。"
      setAiSummaryError(msg)
      // lastSummaryHistoryId 由后端写入历史后前端无法直接获取 ID，
      // 这里用时间戳近似标记（history 页可精确查看）
    } finally {
      setIsSummarizing(false)
    }
  }

  const stopBatchPoll = () => {
    if (batchPollRef.current !== null) {
      clearInterval(batchPollRef.current)
      batchPollRef.current = null
    }
  }

  const handleBatchTranscribe = async () => {
    if (!activeTalker || isBatchTranscribing) return
    setShowMoreMenu(false)
    try {
      await mediaApi.transcribeSession(activeTalker)
      setIsBatchTranscribing(true)
      setBatchProgress({ done: 0, total: 0 })
      toast.info("批量转文字任务已启动，请稍候...")

      batchPollRef.current = window.setInterval(async () => {
        try {
          const status = await mediaApi.transcribeSessionStatus()
          setBatchProgress({ done: status.done, total: status.total })
          if (!status.running) {
            stopBatchPoll()
            setIsBatchTranscribing(false)
            setBatchProgress(null)
            if (status.errors > 0) {
              toast.warning(`转文字完成：${status.done - status.errors} 成功，${status.errors} 失败`)
            } else {
              toast.success(`转文字完成！共处理 ${status.done} 条语音消息`)
            }
          }
        } catch {
          stopBatchPoll()
          setIsBatchTranscribing(false)
          setBatchProgress(null)
          toast.error("获取转文字状态失败")
        }
      }, 2000)
    } catch (err: any) {
      toast.error("启动转文字失败: " + (err?.message || "未知错误"))
    }
  }

  // Clean up poll timer on unmount
  useEffect(() => () => stopBatchPoll(), [])

  const { data: sessions = [] } = useSessions()
  const { data: allMessages = [] } = useMessages(activeTalker)

  const contactAvatar = useMemo(() => {
    if (!activeTalker) return ""
    const session = sessions.find(s => s.talker === activeTalker)
    if (session?.avatar) return session.avatar
    
    // Fallback: find from messages
    const msg = allMessages.find(m => m.sender === activeTalker)
    return msg?.smallHeadURL || msg?.bigHeadURL || mediaApi.getAvatarUrl(`avatar/${activeTalker}`)
  }, [activeTalker, sessions, allMessages])

  const selfAvatar = useMemo(() => {
    const msg = allMessages.find(m => m.isSelf)
    if (msg) return msg.smallHeadURL || msg.bigHeadURL || mediaApi.getAvatarUrl(`avatar/${msg.sender}`)
    return ""
  }, [allMessages])
  
  const displayName = useMemo(() => {
    if (!activeTalker) return ""
    const session = sessions.find(s => s.talker === activeTalker)
    return session ? (session.name || session.talkerName) : activeTalker
  }, [activeTalker, sessions])

  const handleExportRequest = (
    type: string,
    range: { type: 'all' | 'custom', start?: string, end?: string },
    opts?: { fillToNow?: boolean },
  ) => {
    if (!activeTalker) return

    let timeRangeParam = ''
    if (range.type === 'custom' && range.start && range.end) {
      timeRangeParam = `&time_range=${range.start}~${range.end}`
    }

    if (type === 'monthly_csv' || type === 'monthly_xlsx') {
      // 月度统计是全量汇总表，不接 time_range —— 掐一段就看不出「什么时候加上的」
      const fmt = type === 'monthly_xlsx' ? 'xlsx' : 'csv'
      // fillToNow 缺省为 true，只有明确关掉时才带上参数
      const fill = opts?.fillToNow === false ? '&fill_to_now=0' : ''
      const url = `/api/v1/export/monthly_stats?talker=${encodeURIComponent(activeTalker)}&name=${encodeURIComponent(displayName)}&format=${fmt}${fill}`
      openSafe(url)
    } else if (type === 'json') {
      const url = `/api/v1/messages?talker_id=${activeTalker}&limit=1000000${timeRangeParam}`
      const a = document.createElement('a')
      a.href = url
      a.download = `messages_${activeTalker}.json`
      a.style.display = 'none'
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
    } else if (type === 'forensic') {
      const url = `/api/v1/export/forensic?talker=${activeTalker}&name=${encodeURIComponent(displayName)}${timeRangeParam}`
      openSafe(url)
    } else {
      const formatParam = type !== 'html' ? `&format=${type}` : ''
      const url = `/api/v1/export/chat?talker=${activeTalker}&name=${encodeURIComponent(displayName)}${formatParam}${timeRangeParam}`
      openSafe(url)
    }
  }
  
  return (
    <div className="flex h-full w-full flex-col">
      {/* 今日报告置顶。可折叠 —— 聊天页的主角是聊天，不该被报告长期占着高度 */}
      <DailyReportCard />

      <div className="flex min-h-0 flex-1 w-full">
      <div className={cn(
        "flex-shrink-0 border-r border-border bg-background transition-all duration-300", 
        isMobile 
          ? (activeTalker ? "w-0 overflow-hidden" : "w-full") 
          : "w-[320px]"
      )}>
        <SessionList />
      </div>
      
      <div className={cn(
        "flex-1 bg-muted/30 flex flex-col h-full overflow-hidden",
        isMobile && !activeTalker && "hidden"
      )}>
        {activeTalker ? (
          <>
            <div className="h-14 flex-shrink-0 border-b border-border/30 bg-background/50 backdrop-blur-md flex items-center justify-between px-4">
              <div className="flex items-center gap-2">
                {isMobile && (
                  <Button 
                    variant="ghost" 
                    size="icon" 
                    className="w-8 h-8"
                    onClick={() => setActiveTalker('')}
                  >
                    <ArrowLeft className="w-5 h-5" />
                  </Button>
                )}
                <h2 className="font-medium text-sm truncate">{displayName}</h2>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="gap-2 text-primary hover:bg-primary/10"
                  onClick={() => setShowAISummary(true)}
                  title="AI 总结最近对话"
                >
                  <BrainCircuit className="w-4 h-4" />
                  <span className="text-xs font-bold">AI 总结</span>
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  className="gap-2 text-muted-foreground hover:text-primary"
                  onClick={() => setShowExportModal(true)}
                  title="导出聊天记录"
                >
                  <Download className="w-4 h-4" />
                  <span className="text-xs">导出</span>
                </Button>

                {/* Batch transcribe progress badge */}
                {isBatchTranscribing && batchProgress && (
                  <span className="text-xs text-muted-foreground flex items-center gap-1">
                    <RefreshCw className="w-3 h-3 animate-spin" />
                    {batchProgress.done}/{batchProgress.total}
                  </span>
                )}

                {/* More dropdown */}
                <Button
                  ref={moreButtonRef}
                  variant="ghost"
                  size="sm"
                  className="gap-1 text-muted-foreground hover:text-primary"
                  onClick={openMoreMenu}
                  title="更多操作"
                >
                  <MoreHorizontal className="w-4 h-4" />
                  <span className="text-xs">更多</span>
                </Button>
                {showMoreMenu && createPortal(
                  <>
                    <div className="fixed inset-0 z-[140]" onClick={() => setShowMoreMenu(false)} />
                    <div className="fixed z-[150] bg-card border rounded-lg shadow-lg py-1 w-44" style={{ top: menuPos.top, right: menuPos.right }}>
                        {/* AI 功能 */}
                        <div className="px-3 py-1 text-[10px] text-muted-foreground font-medium uppercase tracking-wider">AI 功能</div>
                        {!isGroupChat && (
                          <button
                            className="w-full px-3 py-2 text-sm text-left hover:bg-muted/50 transition-colors flex items-center gap-2"
                            onClick={() => { setShowAISimulate(true); setShowMoreMenu(false) }}
                          >
                            <MessageSquareQuote className="w-4 h-4 text-primary" />
                            模拟对话
                          </button>
                        )}
                        <button
                          className="w-full px-3 py-2 text-sm text-left hover:bg-muted/50 transition-colors flex items-center gap-2"
                          onClick={() => { setShowAnalysis(true); setShowMoreMenu(false) }}
                        >
                          <Sparkles className="w-4 h-4 text-primary" />
                          会话分析
                        </button>

                        <div className="border-t my-1" />
                        {/* 媒体 */}
                        <div className="px-3 py-1 text-[10px] text-muted-foreground font-medium uppercase tracking-wider">媒体</div>
                        <button
                          className="w-full px-3 py-2 text-sm text-left hover:bg-muted/50 transition-colors flex items-center gap-2"
                          onClick={() => { setShowSessionGallery(true); setShowMoreMenu(false) }}
                        >
                          <Images className="w-4 h-4" />
                          查看图片
                        </button>
                        <button
                          className="w-full px-3 py-2 text-sm text-left hover:bg-muted/50 transition-colors flex items-center gap-2"
                          onClick={async () => {
                            if (!activeTalker) return
                            setShowMoreMenu(false)
                            
                            const voiceIds = allMessages
                              .filter(msg => msg.type === 34 && msg.contents?.voice)
                              .map(msg => String(msg.contents!.voice))

                            if (voiceIds.length === 0) {
                              toast.error("当前页面未发现语音消息")
                              return
                            }

                            const loadingToast = toast.loading(`正在准备导出 ${voiceIds.length} 条语音...`)
                            try {
                              const blob = await mediaApi.exportVoices({
                                talker: activeTalker,
                                name: displayName,
                                ids: voiceIds
                              })
                              
                              const url = window.URL.createObjectURL(blob)
                              const a = document.createElement('a')
                              a.href = url
                              a.download = `voices_${displayName}_${new Date().toISOString().split('T')[0]}.zip`
                              document.body.appendChild(a)
                              a.click()
                              window.URL.revokeObjectURL(url)
                              document.body.removeChild(a)
                              
                              toast.success("语音导出成功", { id: loadingToast })
                            } catch (err: any) {
                              console.error("Export voices failed:", err)
                              toast.error(`导出失败: ${err.message}`, { id: loadingToast })
                            }
                          }}
                        >
                          <Mic className="w-4 h-4" />
                          导出语音
                        </button>
                        <button
                          className="w-full px-3 py-2 text-sm text-left hover:bg-muted/50 transition-colors flex items-center gap-2 disabled:opacity-50"
                          onClick={handleBatchTranscribe}
                          disabled={isBatchTranscribing}
                        >
                          <Type className="w-4 h-4" />
                          {isBatchTranscribing && batchProgress
                            ? `转文字 ${batchProgress.done}/${batchProgress.total}`
                            : "一键转文字"}
                        </button>

                      </div>
                  </>,
                  document.body
                )}
              </div>
            </div>
            
            <div className="flex-1 overflow-hidden relative">
              <MessageList />
              {showAISimulate && (
                <AISimulateChat 
                  talker={activeTalker} 
                  displayName={displayName} 
                  contactAvatar={contactAvatar}
                  selfAvatar={selfAvatar}
                  onClose={() => setShowAISimulate(false)} 
                />
              )}
            </div>

            {/* Dummy Input Area */}
            {!showAISimulate && (
              <div className="flex-shrink-0 bg-background border-t border-border/30 px-6 py-6 pb-safe">
                <div className="flex items-center gap-4">
                  <Button variant="ghost" size="icon" className="shrink-0 text-muted-foreground rounded-full hover:bg-muted h-10 w-10">
                    <Mic className="w-6 h-6" />
                  </Button>
                  
                  <div className="flex-1 bg-muted/50 border border-border/50 rounded-lg h-12 px-4 flex items-center text-muted-foreground/60 text-base cursor-not-allowed select-none">
                    只读模式，无法发送消息
                  </div>

                  <Button variant="ghost" size="icon" className="shrink-0 text-muted-foreground rounded-full hover:bg-muted h-10 w-10">
                    <Smile className="w-6 h-6" />
                  </Button>
                  <Button variant="ghost" size="icon" className="shrink-0 text-muted-foreground rounded-full hover:bg-muted h-10 w-10">
                    <PlusCircle className="w-6 h-6" />
                  </Button>
                </div>
              </div>
            )}
          </>
        ) : (
          !isMobile && (
            <div className="flex items-center justify-center h-full text-muted-foreground flex-col gap-4">
              <div className="w-24 h-24 bg-muted rounded-full flex items-center justify-center">
                <svg xmlns="http://www.w3.org/2000/svg" className="w-12 h-12 text-muted-foreground/50" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>
              </div>
              <p>选择一个会话开始浏览</p>
            </div>
          )
        )}
      </div>

      {showAnalysis && activeTalker && (
        <AnalysisPanel 
          talker={activeTalker} 
          onClose={() => setShowAnalysis(false)} 
        />
      )}

      <ExportModal 
        isOpen={showExportModal}
        onClose={() => setShowExportModal(false)}
        onExport={handleExportRequest}
      />

      <AISummaryModal
        isOpen={showAISummary}
        onClose={() => { setShowAISummary(false); setAiSummaryError(""); setAiSummary(""); setSummaryInitialPrompt(""); setSummaryInitialTimeRange("") }}
        summary={aiSummary}
        isLoading={isSummarizing}
        error={aiSummaryError}
        lastHistoryId={lastSummaryHistoryId}
        initialPrompt={summaryInitialPrompt}
        initialTimeRange={summaryInitialTimeRange}
        onSummarize={handleAISummarize}
      />

      {activeTalker && (
        <SessionGalleryModal
          talker={activeTalker}
          displayName={displayName}
          isOpen={showSessionGallery}
          onClose={() => setShowSessionGallery(false)}
        />
      )}
    </div>
      </div>
  )
}
