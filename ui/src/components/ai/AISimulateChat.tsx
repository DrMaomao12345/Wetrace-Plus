import { useState, useRef, useEffect } from "react"
import {
  aiApi,
  type AISimulateTurn,
  type ContactMemoryStatusResponse,
} from "@/api/ai"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  AlertCircle,
  Bot,
  BrainCircuit,
  CheckCircle2,
  Loader2,
  RefreshCw,
  RotateCcw,
  Send,
  Trash2,
  X,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { EmojiText } from "../chat/EmojiText"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { toast } from "sonner"

interface Message {
  id: number
  role: 'user' | 'assistant'
  content: string
  failed?: boolean
}

const MAX_CONVERSATION_TURNS = 24

interface AISimulateChatProps {
  talker: string
  displayName: string
  contactAvatar?: string
  selfAvatar?: string
  onClose: () => void
}

export function AISimulateChat({ talker, displayName, contactAvatar, selfAvatar, onClose }: AISimulateChatProps) {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [memoryState, setMemoryState] = useState<ContactMemoryStatusResponse | null>(null)
  const [isMemoryLoading, setIsMemoryLoading] = useState(true)
  const [memoryAction, setMemoryAction] = useState<'rebuild' | 'delete' | null>(null)
  const [memoryError, setMemoryError] = useState("")
  const [showDeleteMemoryConfirm, setShowDeleteMemoryConfirm] = useState(false)
  const [memoryLoadVersion, setMemoryLoadVersion] = useState(0)
  const scrollRef = useRef<HTMLDivElement>(null)
  const requestControllerRef = useRef<AbortController | null>(null)
  const memoryActionControllerRef = useRef<AbortController | null>(null)
  const messageIDRef = useRef(0)

  useEffect(() => {
    requestControllerRef.current?.abort()
    requestControllerRef.current = null
    memoryActionControllerRef.current?.abort()
    memoryActionControllerRef.current = null
    setMessages([])
    setInput("")
    setIsLoading(false)
  }, [talker])

  useEffect(() => {
    const controller = new AbortController()
    setMemoryState(null)
    setMemoryError("")
    setIsMemoryLoading(true)
    setMemoryAction(null)
    setShowDeleteMemoryConfirm(false)

    aiApi.getContactMemory(talker, controller.signal)
      .then(setMemoryState)
      .catch((error: unknown) => {
        if (controller.signal.aborted) return
        setMemoryError((error as Error)?.message || "暂时无法读取联系人记忆")
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsMemoryLoading(false)
      })

    return () => controller.abort()
  }, [talker, memoryLoadVersion])

  useEffect(() => {
    if (memoryState?.status !== 'generating') return

    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | undefined

    const poll = async () => {
      controller = new AbortController()
      try {
        const nextState = await aiApi.getContactMemory(talker, controller.signal)
        if (stopped) return
        setMemoryState(nextState)
        setMemoryError("")
        if (nextState.status === 'generating') {
          timer = setTimeout(poll, 2000)
        }
      } catch (error) {
        if (stopped || controller.signal.aborted) return
        setMemoryError((error as Error)?.message || "记忆状态更新失败")
        timer = setTimeout(poll, 4000)
      }
    }

    timer = setTimeout(poll, 2000)
    return () => {
      stopped = true
      if (timer) clearTimeout(timer)
      controller?.abort()
    }
  }, [memoryState?.status, talker])

  useEffect(() => () => {
    requestControllerRef.current?.abort()
    memoryActionControllerRef.current?.abort()
  }, [])

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  const handleSend = async () => {
    if (!input.trim() || isLoading) return

    const userMsg = input.trim()
    const userMessageID = ++messageIDRef.current
    const conversation: AISimulateTurn[] = messages
      .filter(message => !message.failed)
      .slice(-MAX_CONVERSATION_TURNS)
      .map(({ role, content }) => ({ role, content }))

    setInput("")
    setMessages(prev => [...prev, { id: userMessageID, role: 'user', content: userMsg }])
    setIsLoading(true)

    const controller = new AbortController()
    requestControllerRef.current = controller

    try {
      const res = await aiApi.simulate({
        talker,
        message: userMsg,
        conversation,
        response_mode: 'text',
      }, controller.signal)

      if (controller.signal.aborted || requestControllerRef.current !== controller) return
      setMessages(prev => [...prev, {
        id: ++messageIDRef.current,
        role: 'assistant',
        content: res,
      }])
      // 首次进入时，状态查询和第一条回复可能并行完成。即使这里尚未拿到
      // memoryState，也要再查一次，才能接上后端自动启动的建立任务。
      if (!memoryState || memoryState.status === 'missing' || memoryState.status === 'stale') {
        setMemoryLoadVersion(version => version + 1)
      }
    } catch (err) {
      const errorCode = (err as { code?: string } | null)?.code
      if (controller.signal.aborted || errorCode === 'ERR_CANCELED') return
      console.error("AI simulation failed:", err)
      setMessages(prev => [
        ...prev.map(message => message.id === userMessageID ? { ...message, failed: true } : message),
        {
          id: ++messageIDRef.current,
          role: 'assistant',
          content: "本轮生成失败，请检查 AI 配置后重试。",
          failed: true,
        },
      ])
    } finally {
      if (requestControllerRef.current === controller) {
        requestControllerRef.current = null
        setIsLoading(false)
      }
    }
  }

  const handleReset = () => {
    requestControllerRef.current?.abort()
    requestControllerRef.current = null
    setMessages([])
    setInput("")
    setIsLoading(false)
  }

  const handleRebuildMemory = async () => {
    if (memoryAction || memoryState?.status === 'generating') return
    setMemoryAction('rebuild')
    setMemoryError("")
    const controller = new AbortController()
    memoryActionControllerRef.current = controller
    try {
      const nextState = await aiApi.rebuildContactMemory(talker, controller.signal)
      if (controller.signal.aborted || memoryActionControllerRef.current !== controller) return
      setMemoryState(nextState)
      toast.success(memoryState?.exists ? "正在更新联系人记忆" : "正在建立联系人记忆")
    } catch (error) {
      if (controller.signal.aborted) return
      const message = (error as Error)?.message || "无法开始建立联系人记忆"
      setMemoryError(message)
      toast.error(message)
    } finally {
      if (memoryActionControllerRef.current === controller) {
        memoryActionControllerRef.current = null
        setMemoryAction(null)
      }
    }
  }

  const handleDeleteMemory = async () => {
    if (memoryAction) return
    setShowDeleteMemoryConfirm(false)
    setMemoryAction('delete')
    setMemoryError("")
    const controller = new AbortController()
    memoryActionControllerRef.current = controller
    try {
      await aiApi.deleteContactMemory(talker, controller.signal)
      if (controller.signal.aborted || memoryActionControllerRef.current !== controller) return
      setMemoryState({ exists: false, status: 'missing' })
      toast.success("联系人记忆已删除，聊天记录不受影响")
    } catch (error) {
      if (controller.signal.aborted) return
      const message = (error as Error)?.message || "删除联系人记忆失败"
      setMemoryError(message)
      toast.error(message)
    } finally {
      if (memoryActionControllerRef.current === controller) {
        memoryActionControllerRef.current = null
        setMemoryAction(null)
      }
    }
  }

  const handleClose = () => {
    requestControllerRef.current?.abort()
    requestControllerRef.current = null
    memoryActionControllerRef.current?.abort()
    memoryActionControllerRef.current = null
    onClose()
  }

  return (
    <div className="absolute inset-0 bg-background flex flex-col z-50 animate-in slide-in-from-right duration-300">
      <div className="h-14 border-b flex items-center justify-between px-4 bg-muted/20">
        <div className="flex items-center gap-2">
          <Bot className="w-5 h-5 text-primary" />
          <h3 className="font-medium text-sm">与 {displayName} (AI 模拟) 对话</h3>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={handleReset}
            disabled={messages.length === 0 && !isLoading}
            title="重新开始模拟会话"
            aria-label="重新开始模拟会话"
          >
            <RotateCcw className="w-4 h-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={handleClose} aria-label="关闭模拟会话">
            <X className="w-5 h-5" />
          </Button>
        </div>
      </div>

      <ScrollArea className="flex-1 p-4" ref={scrollRef}>
        <div className="flex flex-col gap-4">
          <div className="bg-muted/50 p-3 rounded-lg text-xs text-muted-foreground italic">
            本次会话会连续理解前文，并参考文字及已转写的历史语音。图片、文件等只会显示为类型标记，不会把内容交给 AI。所有回复均为 AI 模拟，不代表 {displayName} 本人。
          </div>
          <ContactMemoryCard
            state={memoryState}
            loading={isMemoryLoading}
            action={memoryAction}
            error={memoryError}
            onRebuild={handleRebuildMemory}
            onRefresh={() => setMemoryLoadVersion(version => version + 1)}
            onDelete={() => setShowDeleteMemoryConfirm(true)}
          />
          {messages.map((msg) => (
            <div key={msg.id} className={cn(
              "flex gap-3 max-w-[85%]",
              msg.role === 'user' ? "ml-auto flex-row-reverse" : "mr-auto",
              msg.failed && "opacity-60"
            )}>
              <Avatar className="w-8 h-8 shrink-0 rounded-md">
                <AvatarImage src={msg.role === 'user' ? selfAvatar : contactAvatar} />
                <AvatarFallback className="rounded-md bg-primary text-primary-foreground text-[10px]">
                  {msg.role === 'user' ? "我" : displayName.slice(0, 1)}
                </AvatarFallback>
              </Avatar>
              <div className={cn(
                "p-3 rounded-2xl text-sm leading-relaxed",
                msg.role === 'user' ? "bg-primary text-primary-foreground rounded-tr-none" : "bg-muted rounded-tl-none",
                msg.failed && msg.role === 'assistant' && "text-destructive"
              )}>
                <EmojiText text={msg.content} />
              </div>
            </div>
          ))}
          {isLoading && (
            <div className="flex gap-3 max-w-[85%] mr-auto">
              <Avatar className="w-8 h-8 shrink-0 rounded-md">
                <AvatarImage src={contactAvatar} />
                <AvatarFallback className="rounded-md bg-muted text-[10px]">
                  {displayName.slice(0, 1)}
                </AvatarFallback>
              </Avatar>
              <div className="bg-muted p-3 rounded-2xl rounded-tl-none flex items-center">
                <Loader2 className="w-4 h-4 animate-spin" />
              </div>
            </div>
          )}
        </div>
      </ScrollArea>

      <div className="p-4 border-t bg-background">
        <form 
          className="flex gap-2" 
          onSubmit={(e) => {
            e.preventDefault()
            handleSend()
          }}
        >
          <Input 
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="输入消息，由 AI 模拟对方回复..."
            disabled={isLoading}
            className="flex-1"
          />
          <Button type="submit" size="icon" disabled={isLoading || !input.trim()}>
            <Send className="w-4 h-4" />
          </Button>
        </form>
      </div>

      <ConfirmDialog
        open={showDeleteMemoryConfirm}
        title="删除联系人记忆？"
        description="删除后无法恢复这份 AI 记忆，但不会删除任何聊天记录。需要时可以重新建立。"
        confirmText="删除记忆"
        danger
        onConfirm={handleDeleteMemory}
        onCancel={() => setShowDeleteMemoryConfirm(false)}
      />
    </div>
  )
}

function ContactMemoryCard({
  state,
  loading,
  action,
  error,
  onRebuild,
  onRefresh,
  onDelete,
}: {
  state: ContactMemoryStatusResponse | null
  loading: boolean
  action: 'rebuild' | 'delete' | null
  error: string
  onRebuild: () => void
  onRefresh: () => void
  onDelete: () => void
}) {
  const status = state?.status
  const isGenerating = status === 'generating'
  const isReady = status === 'ready'
  const isStale = status === 'stale'
  const isFailed = status === 'failed'
  const isBusy = loading || action !== null || isGenerating

  let title = "还没有联系人记忆"
  let description = "建立后，AI 会记住对方的表达方式，下次启动也会更快。"
  let actionLabel = "建立记忆"

  if (loading) {
    title = "正在查看联系人记忆…"
    description = "这不会影响你继续模拟对话。"
  } else if (!state && error) {
    title = "暂时无法读取联系人记忆"
    description = "可以稍后再试，当前仍可继续模拟对话。"
    actionLabel = "再试一次"
  } else if (isGenerating) {
    title = "正在学习聊天记录…"
    description = "完成后会自动用于后续回复，现在也可以继续对话。"
    actionLabel = "正在建立"
  } else if (isReady) {
    title = "联系人记忆已就绪"
    description = state?.memory?.profile.summary || "AI 会结合这份记忆，更稳定地还原对方的表达习惯。"
    actionLabel = "重新建立"
  } else if (isStale) {
    title = "有新的聊天记录可以学习"
    description = state?.memory?.profile.summary || "现有记忆仍可使用，更新后会更贴近对方最近的表达。"
    actionLabel = "更新记忆"
  } else if (isFailed) {
    title = "联系人记忆建立失败"
    description = state?.error || "可以重新尝试，当前仍可继续模拟对话。"
    actionLabel = "重新尝试"
  }

  const Icon = loading || isGenerating
    ? Loader2
    : isReady
      ? CheckCircle2
      : isFailed || (!state && error)
        ? AlertCircle
        : BrainCircuit

  const updatedAt = (isReady || isStale) && state?.memory?.updated_at
    ? new Date(state.memory.updated_at).toLocaleString("zh-CN", {
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : ""
  const traits = (state?.memory?.profile.traits ?? []).slice(0, 4)
  const tones = (state?.memory?.profile.speaking_style.tone ?? []).slice(0, 4)

  return (
    <div className={cn(
      "rounded-lg border p-3 text-xs",
      isReady && "border-green-500/30 bg-green-500/5",
      isStale && "border-amber-500/30 bg-amber-500/5",
      isFailed && "border-destructive/30 bg-destructive/5",
    )}>
      <div className="flex items-start gap-2.5">
        <Icon className={cn(
          "mt-0.5 h-4 w-4 shrink-0",
          (loading || isGenerating) && "animate-spin text-primary",
          isReady && "text-green-600 dark:text-green-400",
          isStale && "text-amber-600 dark:text-amber-400",
          (isFailed || (!state && error)) && "text-destructive",
        )} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <div className="font-medium text-foreground">{title}</div>
              {updatedAt && <div className="mt-0.5 text-[10px] text-muted-foreground">更新于 {updatedAt}</div>}
            </div>
            {!loading && (
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 px-2.5 text-xs"
                  disabled={isBusy}
                  onClick={!state && error ? onRefresh : onRebuild}
                >
                  {action === 'rebuild' || isGenerating ? (
                    <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  ) : (
                    <RefreshCw className="mr-1 h-3 w-3" />
                  )}
                  {actionLabel}
                </Button>
                {state?.exists && !isGenerating && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-muted-foreground hover:text-destructive"
                    disabled={action !== null}
                    onClick={onDelete}
                    title="删除联系人记忆"
                  >
                    {action === 'delete' ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Trash2 className="h-3.5 w-3.5" />
                    )}
                    <span className="sr-only">删除联系人记忆</span>
                  </Button>
                )}
              </div>
            )}
          </div>
          <p className="mt-1.5 line-clamp-3 leading-relaxed text-muted-foreground">{description}</p>
          {(traits.length > 0 || tones.length > 0) && (
            <div className="mt-2 space-y-1.5">
              {traits.length > 0 && <MemoryTagRow label="特点" tags={traits} />}
              {tones.length > 0 && <MemoryTagRow label="语气" tags={tones} />}
            </div>
          )}
          {error && state && status !== 'failed' && (
            <p className="mt-1.5 text-destructive">{error}</p>
          )}
        </div>
      </div>
    </div>
  )
}

function MemoryTagRow({ label, tags }: { label: string; tags: string[] }) {
  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="mr-0.5 text-[10px] text-muted-foreground/70">{label}</span>
      {tags.map(tag => (
        <span key={tag} className="rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
          {tag}
        </span>
      ))}
    </div>
  )
}
