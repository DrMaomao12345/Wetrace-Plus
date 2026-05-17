import { createPortal } from "react-dom"
import { X, Loader2, BrainCircuit, Calendar, ChevronDown, ChevronUp, SlidersHorizontal, RotateCcw, Square, RefreshCw, Copy, Check } from "lucide-react"
import { Button } from "../ui/button"
import { useState, useEffect } from "react"
import { Input } from "../ui/input"
import { Label } from "../ui/label"
import { systemApi } from "@/api/system"
import { aiApi } from "@/api/ai"

interface AISummaryModalProps {
  isOpen: boolean
  onClose: () => void
  summary: string
  isLoading: boolean
  error?: string
  lastHistoryId?: string
  initialPrompt?: string
  initialTimeRange?: string
  onSummarize: (timeRange?: string, customPrompt?: string, retryOf?: string) => void
}

export function AISummaryModal({ isOpen, onClose, summary, isLoading, error, lastHistoryId, initialPrompt, initialTimeRange, onSummarize }: AISummaryModalProps) {
  const [startDate, setStartDate] = useState("")
  const [endDate, setEndDate] = useState("")
  const [showRange, setShowRange] = useState(false)
  const [showPromptEditor, setShowPromptEditor] = useState(false)
  const [customPrompt, setCustomPrompt] = useState("")
  const [defaultPrompt, setDefaultPrompt] = useState("")
  const [cancelling, setCancelling] = useState(false)
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    if (!summary) return
    navigator.clipboard.writeText(summary).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  // 从历史重试跳转过来时，预填提示词和时间范围
  useEffect(() => {
    if (isOpen) {
      if (initialPrompt) {
        setCustomPrompt(initialPrompt)
        setShowPromptEditor(true)
      }
      if (initialTimeRange && initialTimeRange.includes("~")) {
        const [s, e] = initialTimeRange.split("~")
        setStartDate(s || "")
        setEndDate(e || "")
        setShowRange(true)
      }
    }
  }, [isOpen, initialPrompt, initialTimeRange])

  useEffect(() => {
    if (isOpen && defaultPrompt === "") {
      systemApi.getAIPrompts().then((res) => {
        const p = res.prompts?.summarize || ""
        setDefaultPrompt(p)
      }).catch(() => {})
    }
  }, [isOpen])

  const handleCancel = async () => {
    setCancelling(true)
    try { await aiApi.cancelSummarize() } catch {}
    setCancelling(false)
  }

  if (!isOpen) return null

  const effectivePrompt = customPrompt.trim() !== "" ? customPrompt : undefined

  const handleSummarize = (retryOf?: string) => {
    const timeRange = showRange && startDate && endDate ? `${startDate}~${endDate}` : undefined
    onSummarize(timeRange, effectivePrompt, retryOf)
  }

  const handleResetPrompt = () => {
    setCustomPrompt("")
  }

  const isUsingCustomPrompt = customPrompt.trim() !== ""

  return (createPortal(
    <div className="fixed inset-0 z-[150] flex items-center justify-center bg-black/50 backdrop-blur-sm animate-in fade-in duration-200 p-4">
      <div
        className="bg-background border shadow-2xl rounded-xl p-6 w-full max-w-[700px] max-h-[90vh] flex flex-col animate-in zoom-in-95 duration-200 relative"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4 border-b pb-4 shrink-0">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center">
              <BrainCircuit className="w-5 h-5 text-primary" />
            </div>
            <div>
              <h3 className="text-lg font-bold">AI 会话总结</h3>
              <p className="text-xs text-muted-foreground">根据历史聊天记录智能生成概要</p>
            </div>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="rounded-full">
            <X className="h-5 w-5" />
          </Button>
        </div>

        <div className="mb-4 space-y-3 shrink-0">
          {/* 时间范围 */}
          <div className="flex items-center justify-between">
            <Label className="text-sm font-medium">时间范围</Label>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={() => setShowRange(!showRange)}
            >
              <Calendar className="w-3 h-3" />
              {showRange ? "取消自定义" : "自定义范围"}
            </Button>
          </div>

          {showRange && (
            <div className="grid grid-cols-2 gap-3 animate-in slide-in-from-top-2 duration-200">
              <div className="space-y-1">
                <Label className="text-[10px] text-muted-foreground uppercase">开始日期</Label>
                <Input
                  type="date"
                  value={startDate}
                  onChange={(e) => setStartDate(e.target.value)}
                  className="h-8 text-xs"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-[10px] text-muted-foreground uppercase">结束日期</Label>
                <Input
                  type="date"
                  value={endDate}
                  onChange={(e) => setEndDate(e.target.value)}
                  className="h-8 text-xs"
                />
              </div>
            </div>
          )}

          {/* 提示词编辑器 */}
          <div className="border rounded-lg overflow-hidden">
            <button
              className="w-full flex items-center justify-between px-3 py-2 text-xs font-medium text-muted-foreground hover:bg-muted/50 transition-colors"
              onClick={() => setShowPromptEditor(!showPromptEditor)}
            >
              <span className="flex items-center gap-1.5">
                <SlidersHorizontal className="w-3.5 h-3.5" />
                本次提示词
                {isUsingCustomPrompt && (
                  <span className="bg-primary/15 text-primary px-1.5 py-0.5 rounded text-[10px]">已自定义</span>
                )}
              </span>
              {showPromptEditor ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
            </button>

            {showPromptEditor && (
              <div className="px-3 pb-3 space-y-2 border-t animate-in slide-in-from-top-1 duration-150">
                <div className="flex items-center justify-between pt-2">
                  <span className="text-[11px] text-muted-foreground">
                    留空则使用全局默认提示词，仅对本次有效
                  </span>
                  {isUsingCustomPrompt && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 text-[11px] gap-1 text-muted-foreground"
                      onClick={handleResetPrompt}
                    >
                      <RotateCcw className="w-3 h-3" />
                      恢复默认
                    </Button>
                  )}
                </div>
                <textarea
                  className="w-full h-32 text-xs font-mono rounded-md border bg-muted/30 px-3 py-2 resize-none focus:outline-none focus:ring-1 focus:ring-primary placeholder:text-muted-foreground/60"
                  placeholder={defaultPrompt || "在此输入自定义提示词，将替换默认的总结指令..."}
                  value={customPrompt}
                  onChange={(e) => setCustomPrompt(e.target.value)}
                />
              </div>
            )}
          </div>

          {/* 生成 / 取消按钮 */}
          <div className="flex gap-2">
            <Button
              className="flex-1 h-9 gap-2"
              onClick={() => handleSummarize()}
              disabled={isLoading || (showRange && (!startDate || !endDate))}
            >
              {isLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <BrainCircuit className="w-4 h-4" />}
              {isLoading ? "生成中..." : showRange ? "总结该时间段" : "生成对话总结"}
            </Button>
            {isLoading && (
              <Button
                variant="outline"
                className="h-9 gap-1 text-destructive border-destructive/40 hover:bg-destructive/10"
                onClick={handleCancel}
                disabled={cancelling}
              >
                <Square className="w-3.5 h-3.5" />
                中止
              </Button>
            )}
          </div>
        </div>

        <div className="flex-1 min-h-[240px] overflow-hidden bg-muted/20 rounded-xl border border-border/50 flex flex-col">
          {isLoading ? (
            <div className="flex flex-col items-center justify-center h-60 gap-4">
              <Loader2 className="w-10 h-10 animate-spin text-primary" />
              <p className="text-sm text-muted-foreground">AI 正在阅读聊天记录并生成总结...</p>
            </div>
          ) : error ? (
            <div className="flex flex-col items-center justify-center h-60 gap-3 px-6">
              <div className="w-10 h-10 rounded-full bg-destructive/10 flex items-center justify-center">
                <X className="w-5 h-5 text-destructive" />
              </div>
              <p className="text-sm font-medium text-destructive">总结失败</p>
              <p className="text-xs text-muted-foreground text-center break-all">{error}</p>
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5 mt-1"
                onClick={() => handleSummarize(lastHistoryId)}
              >
                <RefreshCw className="w-3.5 h-3.5" />
                重试
              </Button>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto custom-scrollbar flex flex-col">
              {summary && (
                <div className="flex justify-end px-4 pt-3 shrink-0">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 text-xs gap-1.5 text-muted-foreground hover:text-foreground"
                    onClick={handleCopy}
                  >
                    {copied ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5" />}
                    {copied ? "已复制" : "复制全文"}
                  </Button>
                </div>
              )}
              <div className="text-sm leading-relaxed whitespace-pre-wrap text-foreground/90 px-5 pb-5 pt-2">
                {summary || "点击「生成对话总结」开始"}
              </div>
            </div>
          )}
        </div>

        <style>{`
          .custom-scrollbar::-webkit-scrollbar { width: 6px; }
          .custom-scrollbar::-webkit-scrollbar-track { background: transparent; }
          .custom-scrollbar::-webkit-scrollbar-thumb { background: rgba(0,0,0,0.1); border-radius: 10px; }
          .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: rgba(0,0,0,0.2); }
          .dark .custom-scrollbar::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.1); }
          .dark .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: rgba(255,255,255,0.2); }
        `}</style>

        <div className="flex justify-end mt-4 border-t pt-4 shrink-0">
          <Button variant="ghost" onClick={onClose}>关闭</Button>
        </div>
      </div>
      <div className="absolute inset-0 -z-10" onClick={onClose} />
    </div>,
    document.body
  ))
}
