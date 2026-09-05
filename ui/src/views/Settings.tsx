import { useState, useEffect, useMemo } from "react"
import { QRCodeSVG } from "qrcode.react"
import { cn } from "@/lib/utils"
import { useAppStore } from "@/stores/app"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { systemApi, sessionApi, mediaApi } from "@/api"
import { toast } from "sonner"
import type {
  AIConfigUpdate,
  SyncConfigUpdate,
  BackupConfigUpdate,
  TTSConfigUpdate,
  WhisperScanResult,
} from "@/api/system"
import { ScrollArea } from "@/components/ui/scroll-area"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { StatsScopeSection } from "@/components/settings/StatsScopeSection"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import {
  TrendingUp,
  Bot,
  RefreshCw,
  Lock,
  HardDrive,
  Loader2,
  CheckCircle,
  XCircle,
  MessageSquare,
  RotateCcw,
  Search,
  X,
  Mic,
  FolderOpen,
  CalendarRange,
  Smartphone,
} from "lucide-react"

/* ============================================================
 * AI Provider Presets
 * ============================================================ */
const AI_PRESETS = [
  {
    name: "OpenAI",
    provider: "openai",
    base_url: "https://api.openai.com/v1",
    model: "gpt-4o",
  },
  {
    name: "DeepSeek",
    provider: "deepseek",
    base_url: "https://api.deepseek.com/v1",
    model: "deepseek-chat",
  },
  {
    name: "Gemini",
    provider: "google",
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai",
    model: "gemini-2.0-flash",
  },
  {
    name: "Claude",
    provider: "anthropic",
    base_url: "https://api.anthropic.com/v1",
    model: "claude-sonnet-4-6",
  },
  {
    name: "Ollama",
    provider: "ollama",
    base_url: "http://localhost:11434/v1",
    model: "llama3",
  },
  {
    name: "月之暗面",
    provider: "moonshot",
    base_url: "https://api.moonshot.cn/v1",
    model: "moonshot-v1-8k",
  },
  {
    name: "通义千问",
    provider: "qwen",
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    model: "qwen-turbo",
  },
  {
    name: "智谱",
    provider: "zhipu",
    base_url: "https://open.bigmodel.cn/api/paas/v4",
    model: "glm-4-flash",
  },
]

/* ============================================================
 * AI Config Section
 * ============================================================ */
function AIConfigSection() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<AIConfigUpdate>({
    enabled: false,
    provider: "openai",
    model: "",
    base_url: "",
    api_key: "",
  })
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "error">("idle")
  const [showPromptsDialog, setShowPromptsDialog] = useState(false)

  const { data: config, isLoading } = useQuery({
    queryKey: ["ai-config"],
    queryFn: () => systemApi.getAIConfig(),
  })

  useEffect(() => {
    if (config) {
      setForm({
        enabled: config.enabled,
        provider: config.provider || "openai",
        model: config.model || "",
        base_url: config.base_url || "",
        api_key: "",
      })
    }
  }, [config])

  const updateMutation = useMutation({
    mutationFn: (data: AIConfigUpdate) => systemApi.updateAIConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-config"] })
      toast.success("AI 配置已保存")
    },
    onError: (err: Error) => toast.error("保存失败: " + err.message),
  })

  const handleTest = async () => {
    setTestStatus("testing")
    try {
      // 把表单当前的值传过去测 —— 否则测的是上次保存的旧配置，
      // 改了模型/Key 不先保存就点测试会一直失败，且看不出原因
      await systemApi.testAIConfig(form)
      setTestStatus("success")
      toast.success("AI 连接测试成功")
    } catch (err) {
      setTestStatus("error")
      toast.error("连接失败: " + (err as Error).message)
    }
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center justify-center">
          <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Bot className="w-4 h-4 text-primary" />
          AI 大模型配置
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium leading-none">启用 AI 功能</label>
          <Switch
            checked={form.enabled}
            onCheckedChange={(checked) => setForm((f) => ({ ...f, enabled: checked }))}
          />
        </div>

        {form.enabled && (
          <>
            {/* Provider presets */}
            <div className="space-y-2">
              <label className="text-sm font-medium leading-none">快速选择服务商</label>
              <div className="flex flex-wrap gap-1.5">
                {AI_PRESETS.map((p) => {
                  const active = form.provider === p.provider && form.base_url === p.base_url
                  const hasSavedKey = !!config?.provider_keys_masked?.[p.provider]
                  return (
                    <button
                      key={p.name}
                      type="button"
                      onClick={() => setForm((f) => ({ ...f, provider: p.provider, base_url: p.base_url, model: p.model, api_key: "" }))}
                      className={cn(
                        "px-2.5 py-1 rounded-md text-xs border transition-colors relative",
                        active
                          ? "bg-primary text-primary-foreground border-primary"
                          : "bg-background text-muted-foreground border-border hover:border-primary hover:text-primary"
                      )}
                    >
                      {p.name}
                      {hasSavedKey && (
                        <span className={cn(
                          "absolute -top-1 -right-1 w-2 h-2 rounded-full",
                          active ? "bg-white" : "bg-green-500"
                        )} title="已保存 Key" />
                      )}
                    </button>
                  )
                })}
              </div>
              <p className="text-xs text-muted-foreground">绿色圆点表示该服务商已保存 Key，每个服务商 Key 独立保存</p>
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">模型名称</label>
              <Input
                value={form.model || ""}
                onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
                placeholder="gpt-4o / deepseek-chat / gemini-2.0-flash"
                className="h-9"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">API 地址</label>
              <Input
                value={form.base_url || ""}
                onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))}
                placeholder="https://api.openai.com/v1"
                className="h-9"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">API Key</label>
              <Input
                type="password"
                value={form.api_key || ""}
                onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
                placeholder={
                  config?.provider_keys_masked?.[form.provider || ""]
                    ? `已保存（${config.provider_keys_masked[form.provider!]}），留空不修改`
                    : "输入该服务商的 API Key"
                }
                className="h-9"
              />
            </div>
          </>
        )}

        <div className="flex items-center gap-2 pt-2">
          <Button size="sm" onClick={() => updateMutation.mutate(form)} disabled={updateMutation.isPending}>
            {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存配置
          </Button>
          {form.enabled && (
            <>
              <Button variant="outline" size="sm" onClick={handleTest} disabled={testStatus === "testing"}>
                {testStatus === "testing" && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
                {testStatus === "success" && <CheckCircle className="w-4 h-4 text-green-500 mr-1" />}
                {testStatus === "error" && <XCircle className="w-4 h-4 text-destructive mr-1" />}
                测试连接
              </Button>
              <Button variant="outline" size="sm" onClick={() => setShowPromptsDialog(true)}>
                <MessageSquare className="w-4 h-4 mr-1" />
                修改默认提示词
              </Button>
            </>
          )}
        </div>
      </CardContent>

      {showPromptsDialog && (
        <AIPromptsDialog onClose={() => setShowPromptsDialog(false)} />
      )}
    </Card>
  )
}

/* ============================================================
 * AI Prompts Dialog (Tab-based)
 * ============================================================ */
const PROMPT_LABELS: Record<string, string> = {
  summarize: "聊天总结",
  simulate: "模拟对话",
  sentiment: "情感分析",
  summary: "结构化摘要",
  extract_todos: "待办提取",
  extract_info: "关键信息抽取",
}

const PROMPT_KEYS = Object.keys(PROMPT_LABELS)

function AIPromptsDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const [prompts, setPrompts] = useState<Record<string, string>>({})
  const [defaults, setDefaults] = useState<Record<string, string>>({})
  const [activeTab, setActiveTab] = useState(PROMPT_KEYS[0])

  const { data, isLoading } = useQuery({
    queryKey: ["ai-prompts"],
    queryFn: () => systemApi.getAIPrompts(),
  })

  useEffect(() => {
    if (data) {
      setPrompts(data.prompts || {})
      setDefaults(data.defaults || {})
    }
  }, [data])

  const updateMutation = useMutation({
    mutationFn: (p: Record<string, string>) => systemApi.updateAIPrompts(p),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ai-prompts"] })
      toast.success("AI 提示词配置已保存")
      onClose()
    },
    onError: (err: Error) => toast.error("保存失败: " + err.message),
  })

  const handleReset = (key: string) => {
    if (defaults[key]) {
      setPrompts((prev) => ({ ...prev, [key]: defaults[key] }))
    }
  }

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-200">
      <div
        className="bg-background border shadow-2xl rounded-2xl w-full max-w-2xl flex flex-col overflow-hidden animate-in zoom-in-95 duration-200 max-h-[85vh]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-6 py-4 border-b flex items-center justify-between">
          <div>
            <h3 className="font-bold text-lg">AI 提示词配置</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              自定义各 AI 功能的提示词，留空将使用默认值
            </p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="rounded-full">
            <X className="h-5 w-5" />
          </Button>
        </div>

        {isLoading ? (
          <div className="p-12 flex items-center justify-center">
            <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <>
            {/* Tabs */}
            <div className="flex border-b px-4 overflow-x-auto">
              {PROMPT_KEYS.map((key) => (
                <button
                  key={key}
                  onClick={() => setActiveTab(key)}
                  className={`px-3 py-2.5 text-sm whitespace-nowrap border-b-2 transition-colors ${
                    activeTab === key
                      ? "border-primary text-primary font-medium"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {PROMPT_LABELS[key]}
                </button>
              ))}
            </div>

            {/* Content */}
            <div className="flex-1 overflow-y-auto p-6 space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  模板变量：模拟对话支持 {"{{target_name}}"} 和 {"{{history}}"}；情感分析支持 {"{{monthly_texts}}"}；关键信息抽取支持 {"{{types_hint}}"}。
                </p>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs text-muted-foreground shrink-0"
                  onClick={() => handleReset(activeTab)}
                >
                  <RotateCcw className="w-3 h-3 mr-1" />
                  重置默认
                </Button>
              </div>
              <textarea
                className="w-full min-h-[260px] rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 resize-y"
                value={prompts[activeTab] || ""}
                onChange={(e) =>
                  setPrompts((prev) => ({ ...prev, [activeTab]: e.target.value }))
                }
                placeholder={defaults[activeTab]?.slice(0, 200) + "..."}
              />
            </div>

            {/* Footer */}
            <div className="px-6 py-4 border-t flex items-center justify-end gap-2">
              <Button variant="outline" size="sm" onClick={onClose}>
                取消
              </Button>
              <Button
                size="sm"
                onClick={() => updateMutation.mutate(prompts)}
                disabled={updateMutation.isPending}
              >
                {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
                保存提示词
              </Button>
            </div>
          </>
        )}
      </div>
      <div className="absolute inset-0 -z-10" onClick={onClose} />
    </div>
  )
}

/* ============================================================
 * Sync Config Section
 * ============================================================ */
function SyncConfigSection() {
  const queryClient = useQueryClient()
  const [enabled, setEnabled] = useState(false)
  const [interval, setInterval] = useState(30)

  const { data: config, isLoading } = useQuery({
    queryKey: ["sync-config"],
    queryFn: () => systemApi.getSyncConfig(),
    refetchInterval: (query) => query.state.data?.is_syncing ? 2000 : false,
  })

  useEffect(() => {
    if (config) {
      setEnabled(config.enabled)
      setInterval(config.interval_minutes)
    }
  }, [config])

  const updateMutation = useMutation({
    mutationFn: (data: SyncConfigUpdate) => systemApi.updateSyncConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sync-config"] })
      toast.success("同步配置已保存")
    },
    onError: (err: Error) => toast.error("保存失败: " + err.message),
  })

  const syncMutation = useMutation({
    mutationFn: () => systemApi.triggerSync(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sync-config"] })
      toast.success("同步已触发")
    },
    onError: (err: Error) => toast.error("同步失败: " + err.message),
  })

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center justify-center">
          <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <RefreshCw className="w-4 h-4 text-primary" />
          自动同步
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-md bg-muted/50 p-3 text-xs text-muted-foreground">
          💡 获取密钥后，点击"立即同步"按钮将微信数据解密并导入到本地数据库
        </div>
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium leading-none">启用自动同步</label>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>

        {enabled && (
          <div className="space-y-1.5">
            <label className="text-sm font-medium leading-none">同步间隔（分钟）</label>
            <Input
              type="number"
              min={5}
              max={1440}
              value={interval}
              onChange={(e) => setInterval(Number(e.target.value))}
              className="h-9 w-32"
            />
            <p className="text-xs text-muted-foreground">最小 5 分钟，最大 1440 分钟（24小时）</p>
          </div>
        )}

        {config?.last_sync_time && (
          <div className="text-xs text-muted-foreground">
            上次同步: {new Date(config.last_sync_time).toLocaleString()}
            {config.last_sync_status && ` (${config.last_sync_status})`}
          </div>
        )}

        <div className="flex items-center gap-2 pt-2">
          <Button
            size="sm"
            onClick={() => updateMutation.mutate({ enabled, interval_minutes: interval })}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存配置
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => syncMutation.mutate()}
            disabled={syncMutation.isPending || config?.is_syncing}
          >
            {(syncMutation.isPending || config?.is_syncing) && (
              <Loader2 className="w-4 h-4 animate-spin mr-1" />
            )}
            立即同步
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

/* ============================================================
 * Password Section
 * ============================================================ */
function PasswordSection() {
  const [oldPwd, setOldPwd] = useState("")
  const [newPwd, setNewPwd] = useState("")
  const [confirmPwd, setConfirmPwd] = useState("")
  const [disablePwd, setDisablePwd] = useState("")

  const { data: status, isLoading, refetch } = useQuery({
    queryKey: ["password-status"],
    queryFn: () => systemApi.getPasswordStatus(),
  })

  const setMutation = useMutation({
    mutationFn: () => systemApi.setPassword(oldPwd, newPwd),
    onSuccess: () => {
      toast.success("密码已设置")
      setOldPwd("")
      setNewPwd("")
      setConfirmPwd("")
      refetch()
    },
    onError: (err: Error) => toast.error("设置失败: " + err.message),
  })

  const disableMutation = useMutation({
    mutationFn: () => systemApi.disablePassword(disablePwd),
    onSuccess: () => {
      toast.success("密码保护已关闭")
      setDisablePwd("")
      refetch()
    },
    onError: (err: Error) => toast.error("关闭失败: " + err.message),
  })

  const handleSetPassword = () => {
    if (newPwd.length < 4) {
      toast.warning("密码至少 4 位")
      return
    }
    if (newPwd !== confirmPwd) {
      toast.warning("两次输入的密码不一致")
      return
    }
    setMutation.mutate()
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center justify-center">
          <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Lock className="w-4 h-4 text-primary" />
          密码保护
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs text-muted-foreground">
          {status?.enabled ? "密码保护已开启，每次打开应用需要输入密码。" : "密码保护未开启。"}
        </p>

        {status?.enabled ? (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">输入当前密码以关闭保护</label>
              <Input
                type="password"
                value={disablePwd}
                onChange={(e) => setDisablePwd(e.target.value)}
                placeholder="当前密码"
                className="h-9 w-64"
              />
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => disableMutation.mutate()}
              disabled={!disablePwd || disableMutation.isPending}
            >
              {disableMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
              关闭密码保护
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">设置新密码</label>
              <Input
                type="password"
                value={newPwd}
                onChange={(e) => setNewPwd(e.target.value)}
                placeholder="新密码（至少 4 位）"
                className="h-9 w-64"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">确认密码</label>
              <Input
                type="password"
                value={confirmPwd}
                onChange={(e) => setConfirmPwd(e.target.value)}
                placeholder="再次输入密码"
                className="h-9 w-64"
              />
            </div>
            <Button
              size="sm"
              onClick={handleSetPassword}
              disabled={!newPwd || !confirmPwd || setMutation.isPending}
            >
              {setMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
              启用密码保护
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/* ============================================================
 * Backup Config Section
 * ============================================================ */
function BackupConfigSection() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<BackupConfigUpdate>({
    enabled: false,
    interval_hours: 24,
    backup_path: "",
    format: "html",
  })
  const [selectedSessionIds, setSelectedSessionIds] = useState<string[]>([])
  const [backupAll, setBackupAll] = useState(true)
  const [sessionSearch, setSessionSearch] = useState("")

  const { data: config, isLoading } = useQuery({
    queryKey: ["backup-config"],
    queryFn: () => systemApi.getBackupConfig(),
  })

  // Fetch session list for selective backup
  const { data: sessionData } = useQuery({
    queryKey: ["backup-sessions"],
    queryFn: () => sessionApi.getSessions({ limit: 10000 }),
  })

  const filteredSessions = useMemo(() => {
    if (!sessionData?.items) return []
    if (!sessionSearch.trim()) return sessionData.items
    const kw = sessionSearch.trim().toLowerCase()
    return sessionData.items.filter(
      (s) =>
        (s.name || "").toLowerCase().includes(kw) ||
        (s.talkerName || "").toLowerCase().includes(kw) ||
        s.talker.toLowerCase().includes(kw)
    )
  }, [sessionData, sessionSearch])

  useEffect(() => {
    if (config) {
      setForm({
        enabled: config.enabled,
        interval_hours: config.interval_hours,
        backup_path: config.backup_path,
        format: config.format || "html",
      })
    }
  }, [config])

  const updateMutation = useMutation({
    mutationFn: (data: BackupConfigUpdate) => systemApi.updateBackupConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["backup-config"] })
      toast.success("备份配置已保存")
    },
    onError: (err: Error) => toast.error("保存失败: " + err.message),
  })

  const backupMutation = useMutation({
    mutationFn: (sessionIds?: string[]) => systemApi.runBackup(sessionIds),
    onSuccess: () => toast.success("备份任务已启动"),
    onError: (err: Error) => toast.error("备份失败: " + err.message),
  })

  const handleRunBackup = () => {
    if (backupAll) {
      backupMutation.mutate(undefined)
    } else {
      if (selectedSessionIds.length === 0) {
        toast.warning("请至少选择一个会话")
        return
      }
      backupMutation.mutate(selectedSessionIds)
    }
  }

  const handleToggleSession = (id: string) => {
    setSelectedSessionIds((prev) =>
      prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id]
    )
  }

  const handleSelectAllFiltered = () => {
    const ids = filteredSessions.map((s) => s.talker)
    setSelectedSessionIds((prev) => {
      const set = new Set(prev)
      ids.forEach((id) => set.add(id))
      return Array.from(set)
    })
  }

  const handleDeselectAllFiltered = () => {
    const ids = new Set(filteredSessions.map((s) => s.talker))
    setSelectedSessionIds((prev) => prev.filter((id) => !ids.has(id)))
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center justify-center">
          <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <HardDrive className="w-4 h-4 text-primary" />
          自动备份
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium leading-none">启用自动备份</label>
          <Switch
            checked={form.enabled}
            onCheckedChange={(checked) => setForm((f) => ({ ...f, enabled: checked }))}
          />
        </div>

        {form.enabled && (
          <>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">备份间隔（小时）</label>
              <Input
                type="number"
                min={1}
                value={form.interval_hours}
                onChange={(e) => setForm((f) => ({ ...f, interval_hours: Number(e.target.value) }))}
                className="h-9 w-32"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">备份保存路径</label>
              <Input
                value={form.backup_path}
                onChange={(e) => setForm((f) => ({ ...f, backup_path: e.target.value }))}
                placeholder="/path/to/backups"
                className="h-9"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium leading-none">备份格式</label>
              <Input
                value={form.format || "html"}
                onChange={(e) => setForm((f) => ({ ...f, format: e.target.value }))}
                placeholder="html / txt / csv"
                className="h-9 w-32"
              />
            </div>
          </>
        )}

        {config?.last_backup_time && (
          <div className="text-xs text-muted-foreground">
            上次备份: {new Date(config.last_backup_time).toLocaleString()}
            {config.last_backup_status && ` (${config.last_backup_status})`}
          </div>
        )}

        {/* Session selection for manual backup */}
        <div className="space-y-2 border rounded-md p-3">
          <label className="text-sm font-medium leading-none">手动备份范围</label>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                name="backup-scope"
                checked={backupAll}
                onChange={() => setBackupAll(true)}
                className="accent-primary"
              />
              备份所有会话
            </label>
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                name="backup-scope"
                checked={!backupAll}
                onChange={() => setBackupAll(false)}
                className="accent-primary"
              />
              选择会话备份
            </label>
          </div>

          {!backupAll && (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                  <Input
                    value={sessionSearch}
                    onChange={(e) => setSessionSearch(e.target.value)}
                    placeholder="搜索会话..."
                    className="h-8 pl-7 text-xs"
                  />
                </div>
                <Button variant="ghost" size="sm" className="h-8 text-xs px-2" onClick={handleSelectAllFiltered}>
                  全选
                </Button>
                <Button variant="ghost" size="sm" className="h-8 text-xs px-2" onClick={handleDeselectAllFiltered}>
                  取消全选
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                已选择 {selectedSessionIds.length} 个会话
                {sessionData?.items ? ` / 共 ${sessionData.items.length} 个` : ""}
              </p>
              <div className="max-h-48 overflow-y-auto border rounded-md divide-y">
                {filteredSessions.map((s) => (
                  <label
                    key={s.talker}
                    className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer text-sm"
                  >
                    <input
                      type="checkbox"
                      checked={selectedSessionIds.includes(s.talker)}
                      onChange={() => handleToggleSession(s.talker)}
                      className="accent-primary"
                    />
                    <span className="truncate">{s.name || s.talkerName || s.talker}</span>
                    {s.type && (
                      <span className="ml-auto text-xs text-muted-foreground shrink-0">
                        {s.type === "group" ? "群聊" : s.type === "private" ? "私聊" : s.type === "official" ? "公众号" : ""}
                      </span>
                    )}
                  </label>
                ))}
                {filteredSessions.length === 0 && (
                  <div className="px-3 py-4 text-center text-xs text-muted-foreground">
                    {sessionSearch ? "未找到匹配的会话" : "暂无会话数据"}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2 pt-2">
          <Button
            size="sm"
            onClick={() => updateMutation.mutate(form)}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存配置
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleRunBackup}
            disabled={backupMutation.isPending}
          >
            {backupMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            {backupAll ? "立即备份全部" : `立即备份 (${selectedSessionIds.length})`}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

/* ============================================================
 * TTS Voice-to-Text Config Section
 * ============================================================ */
function TTSConfigSection() {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<TTSConfigUpdate>({
    enabled: false,
    provider: "openai",
    base_url: "",
    api_key: "",
    model: "whisper-1",
    local_mode: false,
    local_binary: "",
    local_model: "",
    auto: false,
  })

  const { data: config, isLoading } = useQuery({
    queryKey: ["tts-config"],
    queryFn: () => systemApi.getTTSConfig(),
  })

  useEffect(() => {
    if (config) {
      setForm({
        enabled: config.enabled,
        provider: config.provider || "openai",
        base_url: config.base_url || "",
        api_key: "",
        model: config.model || "whisper-1",
        local_mode: (config as any).local_mode || false,
        local_binary: (config as any).local_binary || "",
        local_model: (config as any).local_model || "",
        auto: (config as any).auto || false,
      })
    }
  }, [config])

  const updateMutation = useMutation({
    mutationFn: (data: TTSConfigUpdate) => systemApi.updateTTSConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tts-config"] })
      toast.success("语音转文字配置已保存")
    },
    onError: (err: Error) => toast.error("保存失败: " + err.message),
  })

  // 本地模式打开时自动在常见位置找一遍 whisper.cpp 和模型，省得用户自己翻路径
  const [scan, setScan] = useState<WhisperScanResult | null>(null)
  const [scanning, setScanning] = useState(false)
  const [scanErr, setScanErr] = useState("")

  const runScan = async () => {
    setScanning(true)
    setScanErr("")
    try {
      setScan(await systemApi.scanWhisperLocal())
    } catch (e: any) {
      setScanErr(e?.message || "查找失败")
    } finally {
      setScanning(false)
    }
  }

  useEffect(() => {
    if (form.enabled && form.local_mode && !scan && !scanning) {
      runScan()
    }
  }, [form.enabled, form.local_mode])

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center justify-center">
          <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Mic className="w-4 h-4 text-primary" />
          语音转文字 (Whisper)
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium leading-none">启用语音转文字</label>
          <Switch
            checked={form.enabled}
            onCheckedChange={(checked) => setForm((f) => ({ ...f, enabled: checked }))}
          />
        </div>

        {form.enabled && (
          <>
            {/* 自动转写 */}
            <div className="flex items-center justify-between">
              <div>
                <label className="text-sm font-medium leading-none">自动转文字</label>
                <p className="text-xs text-muted-foreground mt-0.5">
                  每次数据同步后自动把新语音转成文字；转出来的字数会计入年度报告
                </p>
              </div>
              <Switch
                checked={form.auto || false}
                onCheckedChange={(v) => setForm((f) => ({ ...f, auto: v }))}
              />
            </div>

            {/* 一次性把历史语音全部转写 */}
            <BatchTranscribePanel />

            {/* 模式切换 */}
            <div className="flex items-center justify-between">
              <div>
                <label className="text-sm font-medium leading-none">本地模型模式</label>
                <p className="text-xs text-muted-foreground mt-0.5">使用本地 whisper.cpp 识别，无需联网，识别后自动关闭模型</p>
              </div>
              <Switch
                checked={form.local_mode || false}
                onCheckedChange={(v) => setForm((f) => ({ ...f, local_mode: v }))}
              />
            </div>

            {form.local_mode ? (
              /* 本地模式：配置 whisper.cpp 路径 */
              <div className="space-y-3 border rounded-md p-3 bg-muted/30">
                {/* 自动查找本机的 whisper.cpp 与模型 */}
                <div className="rounded-md border border-border bg-background/60 p-3 space-y-2">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">本机查找</span>
                    {scanning && (
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Loader2 className="w-3 h-3 animate-spin" />
                        正在扫描常见安装位置…
                      </span>
                    )}
                    <Button size="sm" variant="outline" className="ml-auto h-7 text-xs"
                      onClick={runScan} disabled={scanning}>
                      重新查找
                    </Button>
                  </div>

                  {scanErr && <p className="text-xs text-destructive">{scanErr}</p>}

                  {!scanning && scan && (
                    <>
                      {(scan.binaries?.length ?? 0) > 0 ? (
                        <div className="space-y-1">
                          <p className="text-xs text-muted-foreground">
                            找到 {scan.binaries!.length} 个可执行文件，点击填入：
                          </p>
                          {scan.binaries!.map((b) => (
                            <button key={b.path}
                              onClick={() => setForm((f) => ({ ...f, local_binary: b.path }))}
                              className={cn(
                                "w-full text-left rounded px-2 py-1 font-mono text-[11px] hover:bg-muted",
                                form.local_binary === b.path && "bg-primary/10 text-primary",
                              )}>
                              {b.path}
                              <span className="ml-2 font-sans text-[10px] text-muted-foreground">来自 {b.source}</span>
                            </button>
                          ))}
                        </div>
                      ) : (
                        <p className="text-xs text-amber-600 dark:text-amber-500">
                          没找到 whisper.cpp 可执行文件。macOS 可以直接
                          <span className="font-mono"> brew install whisper-cpp </span>
                          安装，或到 GitHub Releases 下载预编译版本。
                        </p>
                      )}

                      {(scan.models?.length ?? 0) > 0 ? (
                        <div className="space-y-1">
                          <p className="text-xs text-muted-foreground">
                            找到 {scan.models!.length} 个模型，点击填入（越大越准、越慢）：
                          </p>
                          {scan.models!.map((m) => (
                            <button key={m.path}
                              onClick={() => setForm((f) => ({ ...f, local_model: m.path }))}
                              className={cn(
                                "w-full text-left rounded px-2 py-1 font-mono text-[11px] hover:bg-muted",
                                form.local_model === m.path && "bg-primary/10 text-primary",
                              )}>
                              {m.name}
                              <span className="ml-2 font-sans text-[10px] text-muted-foreground">
                                {m.size_mb ? `${m.size_mb.toFixed(0)} MB · ` : ""}{m.source}
                              </span>
                            </button>
                          ))}
                        </div>
                      ) : (
                        <div className="rounded bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-400 space-y-1">
                          <p className="font-medium">没有在本机找到任何 Whisper 模型（ggml-*.bin）</p>
                          <p>
                            中文推荐下载 <span className="font-mono">ggml-medium.bin</span>（约 1.5 GB），
                            想快一点用 <span className="font-mono">ggml-base.bin</span>（约 142 MB）：
                          </p>
                          <p className="font-mono break-all">
                            huggingface.co/ggerganov/whisper.cpp/tree/main
                          </p>
                          <p>下载后放到下面任一目录，再点「重新查找」即可自动识别。</p>
                        </div>
                      )}

                      <details className="text-[11px] text-muted-foreground">
                        <summary className="cursor-pointer">查看已搜索的 {scan.searched?.length ?? 0} 个位置</summary>
                        <div className="mt-1 space-y-0.5 font-mono">
                          {(scan.searched || []).map((d) => <div key={d}>{d}</div>)}
                        </div>
                      </details>
                    </>
                  )}
                </div>

                <div className="space-y-1.5">
                  <label className="text-sm font-medium leading-none">whisper.cpp 可执行文件路径</label>
                  <Input
                    value={form.local_binary || ""}
                    onChange={(e) => setForm((f) => ({ ...f, local_binary: e.target.value }))}
                    placeholder="C:\whisper\whisper-cli.exe"
                    className="h-9 font-mono text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium leading-none">模型文件路径（.bin）</label>
                  <Input
                    value={form.local_model || ""}
                    onChange={(e) => setForm((f) => ({ ...f, local_model: e.target.value }))}
                    placeholder="C:\whisper\models\ggml-base.bin"
                    className="h-9 font-mono text-xs"
                  />
                </div>
                <div className="rounded-md bg-muted p-3 text-xs text-muted-foreground space-y-1">
                  <p className="font-medium text-foreground">使用说明</p>
                  <p>1. 前往 <span className="font-mono">github.com/ggerganov/whisper.cpp/releases</span> 下载预编译二进制（Windows 选 <span className="font-mono">whisper-cli.exe</span>）</p>
                  <p>2. 前往 <span className="font-mono">huggingface.co/ggerganov/whisper.cpp</span> 下载模型文件（推荐 <span className="font-mono">ggml-base.bin</span>，中文建议用 <span className="font-mono">ggml-medium.bin</span>）</p>
                  <p>3. 填入两者的完整路径后保存即可</p>
                  <p>4. 每次识别时自动启动模型进程，识别完立即退出，不占内存</p>
                  <p>5. 识别结果永久保存，再次打开聊天时自动显示</p>
                </div>
              </div>
            ) : (
              /* API 模式：调用远程 Whisper 接口 */
              <>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium leading-none">模型名称</label>
                  <Input
                    value={form.model || ""}
                    onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
                    placeholder="whisper-1"
                    className="h-9"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium leading-none">API 地址</label>
                  <Input
                    value={form.base_url || ""}
                    onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))}
                    placeholder="https://api.openai.com/v1"
                    className="h-9"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium leading-none">API Key</label>
                  <Input
                    type="password"
                    value={form.api_key || ""}
                    onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
                    placeholder={config?.api_key_masked || "输入 API Key"}
                    className="h-9"
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  支持 OpenAI Whisper 兼容接口。识别结果永久保存，再次打开聊天时自动显示。
                </p>
              </>
            )}
          </>
        )}

        <div className="flex items-center gap-2 pt-2">
          <Button
            size="sm"
            onClick={() => updateMutation.mutate(form)}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存配置
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

/* ============================================================
 * Data Directory Section
 * ============================================================ */
const TZ_OPTIONS_SETTINGS: { label: string; offset: number }[] = [
  { label: "UTC-12", offset: -720 }, { label: "UTC-11", offset: -660 }, { label: "UTC-10", offset: -600 },
  { label: "UTC-9", offset: -540 }, { label: "UTC-8", offset: -480 }, { label: "UTC-7", offset: -420 },
  { label: "UTC-6", offset: -360 }, { label: "UTC-5", offset: -300 }, { label: "UTC-4", offset: -240 },
  { label: "UTC-3", offset: -180 }, { label: "UTC-2", offset: -120 }, { label: "UTC-1", offset: -60 },
  { label: "UTC", offset: 0 }, { label: "UTC+1", offset: 60 }, { label: "UTC+2", offset: 120 },
  { label: "UTC+3", offset: 180 }, { label: "UTC+4", offset: 240 }, { label: "UTC+5", offset: 300 },
  { label: "UTC+5:30 (印度)", offset: 330 }, { label: "UTC+6", offset: 360 }, { label: "UTC+7", offset: 420 },
  { label: "UTC+8 (北京/上海)", offset: 480 }, { label: "UTC+9 (东京/首尔)", offset: 540 },
  { label: "UTC+10", offset: 600 }, { label: "UTC+11", offset: 660 }, { label: "UTC+12", offset: 720 },
  { label: "UTC+13", offset: 780 }, { label: "UTC+14", offset: 840 },
]

/* ============================================================
 * Mobile Pairing Section — iOS App 配对
 * ============================================================ */
const PAIR_URL_STORAGE = "mobile_pair_public_url"

function fmtTime(sec: number): string {
  if (!sec) return "从未"
  return new Date(sec * 1000).toLocaleString()
}

function MobilePairingSection() {
  const queryClient = useQueryClient()
  // 公网访问地址（手机要能连到的地址），存 localStorage。默认用当前网页地址。
  const [publicURL, setPublicURL] = useState<string>(() => {
    const saved = localStorage.getItem(PAIR_URL_STORAGE)
    if (saved && saved.trim()) return saved
    const origin = window.location.origin
    if (origin.includes("127.0.0.1") || origin.includes("localhost")) return ""
    return origin
  })
  const [newLabel, setNewLabel] = useState("")
  // 当前展开显示二维码的记录 id
  const [qrFor, setQrFor] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ["mobile-pairings"],
    queryFn: () => systemApi.listMobilePairings(),
  })
  const pairings = data?.pairings ?? []

  const createMutation = useMutation({
    mutationFn: (label: string) => systemApi.createMobilePairing(label),
    onSuccess: (rec) => {
      toast.success("已创建配对")
      setNewLabel("")
      setQrFor(rec.id)   // 创建后自动展开新记录的二维码
      queryClient.invalidateQueries({ queryKey: ["mobile-pairings"] })
    },
    onError: (e: Error) => toast.error("创建失败: " + e.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => systemApi.deleteMobilePairing(id),
    onSuccess: () => {
      toast.success("已删除，该设备立即失效")
      queryClient.invalidateQueries({ queryKey: ["mobile-pairings"] })
    },
    onError: (e: Error) => toast.error("删除失败: " + e.message),
  })

  const renameMutation = useMutation({
    mutationFn: (v: { id: string; label: string }) => systemApi.renameMobilePairing(v.id, v.label),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["mobile-pairings"] }),
    onError: (e: Error) => toast.error("重命名失败: " + e.message),
  })

  const saveURL = (v: string) => {
    setPublicURL(v)
    localStorage.setItem(PAIR_URL_STORAGE, v.trim())
  }

  const cleanURL = publicURL.trim().replace(/\/+$/, "")
  const qrValue = (token: string) =>
    JSON.stringify({ url: cleanURL, token })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Smartphone className="w-4 h-4 text-primary" />
          iOS App 配对
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-md bg-muted/50 p-3 text-xs text-muted-foreground space-y-1">
          <p>📱 每次「新建配对」生成一条独立记录（对应一台设备），可在下方历史里随时查看二维码、重命名、删除。</p>
          <p>手机/电脑不在同一网络时，先用 Tailscale / Cloudflare Tunnel / frp 把本服务暴露出去。</p>
        </div>

        {/* 公网地址 */}
        <div className="space-y-1.5">
          <label className="text-sm font-medium leading-none">公网访问地址</label>
          <Input
            value={publicURL}
            onChange={(e) => saveURL(e.target.value)}
            placeholder="https://your-pc.tailxxxx.ts.net:5200"
            className="h-9 font-mono text-xs"
          />
        </div>

        {/* 新建配对 */}
        <div className="flex items-center gap-2">
          <Input
            value={newLabel}
            onChange={(e) => setNewLabel(e.target.value)}
            placeholder="备注名（如：我的 iPhone）"
            className="h-9 text-sm"
          />
          <Button
            size="sm"
            onClick={() => createMutation.mutate(newLabel.trim() || "新配对")}
            disabled={createMutation.isPending}
          >
            {createMutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            新建配对
          </Button>
        </div>

        {/* 历史记录列表 */}
        <div className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">
            配对历史（{pairings.length}）
          </div>
          {isLoading ? (
            <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
          ) : pairings.length === 0 ? (
            <p className="text-xs text-muted-foreground">还没有配对记录，点上方「新建配对」</p>
          ) : (
            pairings.map((p) => (
              <div key={p.id} className="border rounded-lg p-3 space-y-2">
                <div className="flex items-center gap-2">
                  <div className="flex-1 min-w-0">
                    <input
                      defaultValue={p.label}
                      onBlur={(e) => {
                        const v = e.target.value.trim()
                        if (v && v !== p.label) renameMutation.mutate({ id: p.id, label: v })
                      }}
                      className="bg-transparent text-sm font-medium w-full outline-none border-b border-transparent focus:border-border"
                    />
                    <div className="text-[11px] text-muted-foreground mt-0.5">
                      创建 {fmtTime(p.created_at)} · 最后访问 {fmtTime(p.last_seen_at)}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    className="text-xs"
                    onClick={() => setQrFor(qrFor === p.id ? null : p.id)}
                  >
                    {qrFor === p.id ? "收起" : "二维码"}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-destructive hover:text-destructive"
                    onClick={() => {
                      if (confirm(`删除「${p.label}」？该设备将立即无法访问。`)) {
                        deleteMutation.mutate(p.id)
                      }
                    }}
                  >
                    <X className="w-4 h-4" />
                  </Button>
                </div>

                {qrFor === p.id && (
                  cleanURL ? (
                    <div className="flex flex-col items-center gap-2 pt-1">
                      <div className="bg-white p-3 rounded-lg">
                        <QRCodeSVG value={qrValue(p.token)} size={200} level="M" />
                      </div>
                      <p className="text-xs text-muted-foreground">用 iOS App 扫描此码</p>
                      <details className="text-xs text-muted-foreground w-full">
                        <summary className="cursor-pointer">手动配对信息</summary>
                        <div className="mt-1 font-mono break-all bg-muted/50 p-2 rounded">
                          <div>地址: {cleanURL}</div>
                          <div>Token: {p.token}</div>
                        </div>
                      </details>
                    </div>
                  ) : (
                    <p className="text-xs text-amber-600 dark:text-amber-400">
                      ⚠ 请先填写上方「公网访问地址」，二维码才能生成
                    </p>
                  )
                )}
              </div>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function DefaultTimezoneSection() {
  const queryClient = useQueryClient()
  const [offset, setOffset] = useState<number | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ["default-timezone"],
    queryFn: () => systemApi.getDefaultTimezone(),
  })

  useEffect(() => {
    if (data && offset === null) {
      // 服务端没保存过 → 用浏览器本地时区作为默认显示
      const guess = data.has_key ? data.offset : -new Date().getTimezoneOffset()
      setOffset(guess)
    }
  }, [data])

  const mutation = useMutation({
    mutationFn: (off: number) => systemApi.updateDefaultTimezone(off),
    onSuccess: () => {
      toast.success("已保存，联系人侧分析数据会按新时区计算")
      queryClient.invalidateQueries({ queryKey: ["default-timezone"] })
      // 让所有联系人侧分析查询重新拉取
      queryClient.invalidateQueries({ queryKey: ["analysis"] })
    },
    onError: (e: Error) => toast.error("保存失败: " + e.message),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <CalendarRange className="w-4 h-4 text-primary" />
          默认时区
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          联系人统计中的「24 小时活跃分布 / 星期分布 / 月度趋势 / 每日趋势」按此时区聚合数据。
          年度报告有自己独立的时区分段配置，不受此项影响。
        </p>
        <div className="flex items-center gap-2">
          <label className="text-sm w-20 shrink-0">时区</label>
          <select
            value={offset ?? -new Date().getTimezoneOffset()}
            onChange={(e) => setOffset(Number(e.target.value))}
            className="h-9 rounded-md border border-input bg-background px-2 text-sm"
            disabled={isLoading}
          >
            {TZ_OPTIONS_SETTINGS.map((tz) => (
              <option key={tz.offset} value={tz.offset}>{tz.label}</option>
            ))}
          </select>
          <Button
            size="sm"
            onClick={() => offset !== null && mutation.mutate(offset)}
            disabled={mutation.isPending || offset === null}
          >
            {mutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存
          </Button>
        </div>
        {data?.has_key && (
          <p className="text-xs text-muted-foreground">
            当前生效：<span className="font-bold text-foreground">UTC{data.offset >= 0 ? "+" : ""}{Math.floor(data.offset / 60)}{data.offset % 60 ? `:${Math.abs(data.offset % 60).toString().padStart(2, "0")}` : ""}</span>
          </p>
        )}
        {!data?.has_key && (
          <p className="text-xs text-muted-foreground">
            未配置，目前使用服务器本地时区
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function EffectiveChatStartSection() {
  const queryClient = useQueryClient()
  const [yearInput, setYearInput] = useState<string>("")

  const { data, isLoading } = useQuery({
    queryKey: ["effective-chat-start"],
    queryFn: () => systemApi.getEffectiveChatStart(),
  })

  useEffect(() => {
    if (data?.year && !yearInput) setYearInput(String(data.year))
  }, [data])

  const mutation = useMutation({
    mutationFn: (year: number) => systemApi.updateEffectiveChatStart(year),
    onSuccess: () => {
      toast.success("已保存，年度报告平均值会自动重算")
      queryClient.invalidateQueries({ queryKey: ["effective-chat-start"] })
      // 不去 invalidate 整份报告（昂贵），只让 baseline 缓存失效，自动局部刷新
      queryClient.invalidateQueries({ queryKey: ["report-baseline"] })
      queryClient.invalidateQueries({ queryKey: ["past-monthly-avg"] })
    },
    onError: (e: Error) => toast.error("保存失败: " + e.message),
  })

  const handleSave = () => {
    const y = parseInt(yearInput)
    if (!y || y < 2000 || y > 2100) {
      toast.error("请输入 2000-2100 之间的年份")
      return
    }
    mutation.mutate(y)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <CalendarRange className="w-4 h-4 text-primary" />
          有效聊天记录起始时间
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          年度报告中「往年同期对比」「往年月均参考线」等计算的起始年份。
          早于此年份的数据可能不完整或缺失，将被排除在对比统计外。
        </p>
        <div className="flex items-center gap-2">
          <label className="text-sm w-20 shrink-0">起始年份</label>
          <Input
            type="number"
            value={yearInput}
            onChange={(e) => setYearInput(e.target.value)}
            placeholder={isLoading ? "加载中..." : "2023"}
            className="h-9 w-32"
            min={2000}
            max={2100}
          />
          <Button size="sm" onClick={handleSave} disabled={mutation.isPending || !yearInput}>
            {mutation.isPending && <Loader2 className="w-4 h-4 animate-spin mr-1" />}
            保存
          </Button>
        </div>
        {data?.year && (
          <p className="text-xs text-muted-foreground">
            当前生效：<span className="font-bold text-foreground">{data.year}</span> 年起
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function DataDirSection() {
  const { data, isLoading } = useQuery({
    queryKey: ["data-dir"],
    queryFn: () => systemApi.getDataDir(),
  })

  const openMutation = useMutation({
    mutationFn: () => systemApi.openDataDir(),
    onError: () => toast.error("无法打开文件夹"),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <FolderOpen className="w-4 h-4 text-primary" />
          本地数据目录
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          应用数据库、缓存、导出文件均保存在此目录。
        </p>
        <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-3 py-2">
          {isLoading ? (
            <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
          ) : (
            <span className="font-mono text-xs break-all flex-1 select-all">
              {data?.path ?? "—"}
            </span>
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5"
          onClick={() => openMutation.mutate()}
          disabled={openMutation.isPending || isLoading}
        >
          <FolderOpen className="w-4 h-4" />
          在文件管理器中打开
        </Button>
      </CardContent>
    </Card>
  )
}

/* ============================================================
 * Main Settings View
 * ============================================================ */
/* ============================================================
 * 月度趋势图的预测怎么画
 * ============================================================ */
function ForecastDisplaySection() {
  const forecastDisplay = useAppStore((st) => st.settings.forecastDisplay)
  const updateSettings = useAppStore((st) => st.updateSettings)
  const [showInfo, setShowInfo] = useState(false)

  const options: { id: 'band' | 'point'; label: string; desc: string }[] = [
    { id: 'band', label: '显示区间', desc: '半透明色带 + 空心点' },
    { id: 'point', label: '只显示一个数据点', desc: '仅空心点' },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <TrendingUp className="w-4 h-4 text-primary" />
          聊天频率预测
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs text-muted-foreground">
          联系人的月度趋势图会把当月到今年 12 月的预计聊天量画出来。三个月以后的部分
          没有回测支撑，只保留色带、不画点。
        </p>

        <div className="space-y-2">
          <div className="flex items-center gap-1.5">
            <label className="text-sm font-medium leading-none">预测的画法</label>
            {/* 小感叹号：讲清楚两种画法各自的代价 */}
            <div className="relative flex items-center">
              <button
                type="button"
                onClick={() => setShowInfo((v) => !v)}
                onMouseEnter={() => setShowInfo(true)}
                onMouseLeave={() => setShowInfo(false)}
                aria-label="两种画法的优劣"
                className="flex h-4 w-4 items-center justify-center rounded-full border border-muted-foreground/50 text-[10px] font-bold leading-none text-muted-foreground transition-colors hover:border-primary hover:text-primary"
              >
                !
              </button>
              {showInfo && (
                <div className="absolute left-6 top-1/2 z-50 w-[300px] -translate-y-1/2 rounded-lg border border-border bg-popover p-3 text-xs leading-relaxed shadow-xl">
                  <div className="mb-1.5">
                    <span className="font-semibold text-foreground">显示区间</span>
                    <span className="text-muted-foreground">
                      　看得出「越往后越不准」—— 12 月的色带会明显比当月宽，不会把预测
                      当成承诺。代价是图更满，同时勾选多个年份时容易糊。
                    </span>
                  </div>
                  <div>
                    <span className="font-semibold text-foreground">只显示点</span>
                    <span className="text-muted-foreground">
                      　干净，好和历史线比。代价是看不出不确定性 —— 11 月那个点会显得和
                      当月一样可信，实际上差很远。
                    </span>
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2">
            {options.map((o) => (
              <button
                key={o.id}
                onClick={() => updateSettings({ forecastDisplay: o.id })}
                className={cn(
                  "rounded-lg border p-3 text-left transition-all",
                  forecastDisplay === o.id
                    ? "border-primary bg-primary/5 ring-1 ring-primary"
                    : "border-border hover:bg-muted/50"
                )}
              >
                <div className="text-sm font-medium">{o.label}</div>
                <div className="text-xs text-muted-foreground">{o.desc}</div>
              </button>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default function SettingsView() {
  return (
    <ScrollArea className="h-full">
      <div className="max-w-3xl mx-auto p-6 space-y-6 pb-20">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">设置</h2>
          <p className="text-sm text-muted-foreground mt-1">管理应用配置</p>
        </div>

        <DataDirSection />
        <MobilePairingSection />
        <DefaultTimezoneSection />
        <StatsScopeSection />
        <EffectiveChatStartSection />
        <ForecastDisplaySection />
        <AIConfigSection />
        <TTSConfigSection />
        <SyncConfigSection />
        <PasswordSection />
        <BackupConfigSection />
      </div>
    </ScrollArea>
  )
}
/* ============================================================
 * 批量语音转写：进度、当前正在转谁、可中断
 * ============================================================ */
function BatchTranscribePanel() {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof mediaApi.transcribeSessionStatus>> | null>(null)
  const [confirmStart, setConfirmStart] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  // 全部转完时的提示，以及可重试的「本地无文件」条数
  const [allDone, setAllDone] = useState<{ missing: number } | null>(null)

  // 只在任务运行时轮询，闲置时降到低频，避免白占资源
  useEffect(() => {
    let timer: number | undefined
    let alive = true
    const tick = async () => {
      try {
        const s = await mediaApi.transcribeSessionStatus()
        if (!alive) return
        setStatus(s)
        timer = window.setTimeout(tick, s.running ? 1000 : 10000)
      } catch {
        if (alive) timer = window.setTimeout(tick, 10000)
      }
    }
    tick()
    return () => {
      alive = false
      if (timer) window.clearTimeout(timer)
    }
  }, [])

  const running = status?.running ?? false
  const total = status?.total ?? 0
  const done = status?.done ?? 0
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0

  return (
    <div className="rounded-md border border-input px-3 py-2 space-y-2">
      <div className="flex items-center justify-between">
        <div className="min-w-0">
          <div className="text-sm">
            转写全部历史语音
            {running && <span className="ml-2 text-xs text-primary">正在转写…</span>}
            {!running && status?.canceled && (
              <span className="ml-2 text-xs text-muted-foreground">已中断</span>
            )}
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">
            扫描所有会话里尚未转写的语音，后台逐条处理。微信自己转过的会自动跳过。
          </p>
        </div>
        {running ? (
          <Button size="sm" variant="outline" onClick={() => setConfirmStop(true)}>
            中断
          </Button>
        ) : (
          <Button size="sm" variant="outline" onClick={() => setConfirmStart(true)}>
            开始
          </Button>
        )}
      </div>

      {allDone && !running && (
        <p className="text-[11px] text-muted-foreground">
          全部语音都已转写完毕。
          {allDone.missing > 0 && (
            <>
              {` 另有 ${allDone.missing} 条本地没有语音文件（微信没下载过）已跳过，`}
              <button
                className="underline underline-offset-2 hover:text-foreground"
                onClick={async () => {
                  try {
                    const r = await mediaApi.transcribeSession("", true)
                    if (r?.status === "nothing_to_do") toast.info(r.message)
                    else {
                      setAllDone(null)
                      toast.success("已开始重试")
                    }
                  } catch (e: any) {
                    toast.error(e?.message || "启动失败")
                  }
                }}
              >
                重试这些
              </button>
            </>
          )}
        </p>
      )}

      {(running || done > 0) && (
        <div className="space-y-1.5">
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary transition-all duration-500"
              style={{ width: `${pct}%` }}
            />
          </div>
          <div className="flex items-center justify-between text-[11px] text-muted-foreground">
            <span>
              {done} / {total}（{pct}%）
              {(status?.skipped ?? 0) > 0 && ` · 已跳过 ${status?.skipped}`}
              {(status?.missing ?? 0) > 0 && ` · 本地无文件 ${status?.missing}`}
              {(status?.errors ?? 0) > 0 && ` · 失败 ${status?.errors}`}
            </span>
            {running && status?.current_name && (
              <span className="truncate max-w-[45%]">正在转：{status.current_name}</span>
            )}
          </div>
          {status?.last_text && (
            <p className="truncate text-[11px] text-muted-foreground/80" title={status.last_text}>
              最近一条：{status.last_text}
            </p>
          )}
        </div>
      )}

      <ConfirmDialog
        open={confirmStart}
        title="转写全部历史语音？"
        description={
          <>
            会把所有还没转写的语音逐条跑一遍本地识别，数量多时可能要跑很久、期间占用 CPU。
            <br />
            过程中可以随时中断，已转好的会保存下来，下次继续不会重来。
          </>
        }
        confirmText="开始转写"
        onCancel={() => setConfirmStart(false)}
        onConfirm={async () => {
          setConfirmStart(false)
          try {
            const r = await mediaApi.transcribeSession("")
            if (r?.status === "nothing_to_do") {
              setAllDone({ missing: r.missing ?? 0 })
              toast.success(r.message || "全部语音都已转写完毕")
            } else {
              setAllDone(null)
              toast.success("已开始转写")
            }
          } catch (e: any) {
            toast.error(e?.message || "启动失败")
          }
        }}
      />

      <ConfirmDialog
        open={confirmStop}
        danger
        title="中断转写？"
        description={
          <>
            已经转好的 {done} 条会保留，下次继续时自动跳过，不会白做。
            <br />
            剩余 {Math.max(0, total - done)} 条这次就不处理了。
          </>
        }
        confirmText="中断"
        cancelText="继续跑"
        onCancel={() => setConfirmStop(false)}
        onConfirm={async () => {
          setConfirmStop(false)
          try {
            await mediaApi.stopTranscribeSession()
            toast.info("正在中断…")
          } catch (e: any) {
            toast.error(e?.message || "中断失败")
          }
        }}
      />
    </div>
  )
}
