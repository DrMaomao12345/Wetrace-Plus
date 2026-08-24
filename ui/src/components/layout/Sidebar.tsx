import { useAppStore } from "@/stores/app"
import { cn } from "@/lib/utils"
import {
  MessageSquare, RefreshCw, Moon, Sun, Monitor, Search, Key,
  ImageIcon, BarChart3, Sparkles, Users, Settings, Shield,
  ChevronDown, CalendarDays, Heart, Cloud, BrainCircuit, PlayCircle, Clock, History, Network, Newspaper,
} from "lucide-react"
import { useNavigate, useLocation } from "react-router-dom"
import { useState, useEffect } from "react"
import { systemApi, mediaApi } from "@/api"
import { toast } from "sonner"
import { KeyManagerModal } from "./KeyManagerModal"
import { usePlatform } from "@/hooks/usePlatform"
import { ImageCacheManager } from "../chat/ImageCacheManager"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"

type NavItem = {
  key: string
  icon: React.ComponentType<{ className?: string }>
  label: string
  path: string
  /** 当前平台不支持时置灰，鼠标悬停给出原因 */
  disabled?: boolean
  disabledReason?: string
}

type NavGroup = {
  key: string
  icon: React.ComponentType<{ className?: string }>
  label: string
  children: NavItem[]
}

type NavEntry = NavItem | NavGroup

function isGroup(entry: NavEntry): entry is NavGroup {
  return 'children' in entry
}

export function Sidebar() {
  const { activeNav, setActiveNav, toggleTheme, settings } = useAppStore()
  const navigate = useNavigate()
  const location = useLocation()
  const [isSyncing, setIsSyncing] = useState(false)
  const [showKeyManager, setShowKeyManager] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [confirmPreload, setConfirmPreload] = useState(false)
  // 图片相关入口是否可用 —— 与消息气泡共用同一份判断
  const { imagesUnavailable: isMac, hasDbKey } = usePlatform()

  const navEntries: NavEntry[] = [
    { key: 'chat', icon: MessageSquare, label: '聊天', path: '/chat' },
    { key: 'contacts', icon: Users, label: '联系人', path: '/contacts' },
    { key: 'contact-reminder', icon: Clock, label: '联系提醒', path: '/contact-reminder' },
    { key: 'gallery', icon: ImageIcon, label: '图片', path: '/gallery' },
    {
      key: 'analysis',
      icon: BarChart3,
      label: '分析',
      children: [
        { key: 'report', icon: CalendarDays, label: '年度报告', path: '/report' },
        { key: 'insights', icon: Network, label: '关系洞察', path: '/insights' },
        { key: 'galaxy', icon: Sparkles, label: '关系星图', path: '/galaxy' },
        { key: 'biz', icon: Newspaper, label: '公众号画像', path: '/biz' },
        { key: 'sentiment', icon: Heart, label: '情感分析', path: '/sentiment' },
        { key: 'wordcloud', icon: Cloud, label: '词云', path: '/wordcloud' },
        { key: 'replay', icon: PlayCircle, label: '对话回放', path: '/replay' },
      ],
    },
    {
      key: 'ai',
      icon: Sparkles,
      label: 'AI工具',
      children: [
        { key: 'ai-tools', icon: BrainCircuit, label: 'AI工具箱', path: '/ai-tools' },
        { key: 'ai-summary-history', icon: History, label: '总结历史', path: '/ai-summary-history' },
      ],
    },
    { key: 'search', icon: Search, label: '搜索', path: '/search' },
    { key: 'monitor', icon: Shield, label: '监控', path: '/monitor' },
  ]

  const settingsItem: NavItem = { key: 'settings', icon: Settings, label: '设置', path: '/settings' }

  // Auto-expand groups that contain the active route
  useEffect(() => {
    const path = location.pathname
    for (const entry of navEntries) {
      if (isGroup(entry)) {
        const match = entry.children.some((child) => path.startsWith(child.path))
        if (match) {
          setExpandedGroups((prev) => {
            const next = new Set(prev)
            next.add(entry.key)
            return next
          })
        }
      }
    }
  }, [location.pathname])

  const toggleGroup = (key: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const handleNavClick = (key: string, path: string) => {
    setActiveNav(key)
    navigate(path)
  }

  const handleFullCache = async () => {
    try {
      await mediaApi.startCache('all')
      window.dispatchEvent(new CustomEvent('image-cache-start'))
      toast.success("全量图片预加载任务已启动，你可以在右下角查看进度。")
    } catch (err) {
      console.error("Failed to start full cache:", err)
      toast.error("启动缓存任务失败")
    }
  }

  const handleSync = async () => {
    try {
      setIsSyncing(true)
      await systemApi.triggerSync()
      toast.info("正在同步数据，数据量较大时可能需要几分钟，请耐心等待...")

      // Poll sync status every 2 seconds, max 150 times (5 min timeout)
      const MAX_POLLS = 150
      const POLL_INTERVAL = 2000
      let pollCount = 0

      const pollTimer = setInterval(async () => {
        pollCount++
        try {
          const status = await systemApi.getSyncStatus()

          if (!status.is_syncing) {
            clearInterval(pollTimer)
            setIsSyncing(false)
            if (status.last_sync_status === "success") {
              toast.success("数据同步成功！")
              window.location.reload()
            } else {
              toast.error("同步失败: " + (status.last_sync_status || "未知错误"))
            }
            return
          }

          if (pollCount >= MAX_POLLS) {
            clearInterval(pollTimer)
            setIsSyncing(false)
            toast.error("同步超时（已等待5分钟），请稍后重试或检查日志。")
          }
        } catch {
          clearInterval(pollTimer)
          setIsSyncing(false)
          toast.error("获取同步状态失败。")
        }
      }, POLL_INTERVAL)
    } catch (error: any) {
      console.error("Sync trigger failed:", error)
      const message = error.message || "启动同步失败，请检查日志。"
      toast.error(message)
      setIsSyncing(false)
    }
  }

  const isActive = (key: string) => activeNav === key
  const isGroupActive = (group: NavGroup) => group.children.some((c) => activeNav === c.key)

  const ThemeIcon = settings.theme === 'dark' ? Moon : settings.theme === 'light' ? Sun : Monitor

  const renderNavItem = (item: NavItem, indent = false) => (
    <button
      key={item.key}
      onClick={() => { if (!item.disabled) handleNavClick(item.key, item.path) }}
      disabled={item.disabled}
      title={item.disabled ? item.disabledReason : undefined}
      className={cn(
        "w-full h-9 shrink-0 flex items-center gap-3 rounded-lg px-3 text-sm transition-colors",
        indent && "pl-9",
        item.disabled
          ? "text-muted-foreground/35 cursor-not-allowed"
          : isActive(item.key)
            ? "bg-primary/10 text-primary font-medium"
            : "text-muted-foreground hover:bg-muted hover:text-foreground"
      )}
    >
      <item.icon className="w-4 h-4 shrink-0" />
      <span className="truncate">{item.label}</span>
      {item.disabled && <span className="ml-auto text-[10px] shrink-0">不可用</span>}
    </button>
  )

  const renderNavGroup = (group: NavGroup) => {
    const expanded = expandedGroups.has(group.key)
    const groupActive = isGroupActive(group)

    return (
      <div key={group.key} className="shrink-0">
        <button
          onClick={() => toggleGroup(group.key)}
          className={cn(
            "w-full h-9 shrink-0 flex items-center gap-3 rounded-lg px-3 text-sm transition-colors",
            groupActive
              ? "text-primary font-medium"
              : "text-muted-foreground hover:bg-muted hover:text-foreground"
          )}
        >
          <group.icon className="w-4 h-4 shrink-0" />
          <span className="truncate flex-1 text-left">{group.label}</span>
          <ChevronDown
            className={cn(
              "w-3.5 h-3.5 shrink-0 transition-transform duration-200",
              expanded && "rotate-180"
            )}
          />
        </button>
        <div
          className={cn(
            "overflow-hidden transition-all duration-200",
            expanded ? "max-h-[500px] opacity-100 mt-0.5" : "max-h-0 opacity-0"
          )}
        >
          <div className="flex flex-col gap-0.5">
            {group.children.map((child) => renderNavItem(child, true))}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="w-[180px] h-full bg-background border-r border-border flex flex-col py-3 z-50">
      {/* Logo / Brand */}
      <div className="px-4 mb-4">
        <div
          className="h-9 flex items-center gap-2 cursor-pointer text-primary"
          onClick={() => handleNavClick('chat', '/chat')}
        >
          <MessageSquare className="w-5 h-5" />
          <span className="font-semibold text-sm">WeTrace Pro</span>
        </div>
      </div>

      {/* Main nav */}
      <nav className="flex-1 min-h-0 overflow-y-auto overscroll-contain px-2 flex flex-col gap-0.5">
        {navEntries.map((entry) =>
          isGroup(entry) ? renderNavGroup(entry) : renderNavItem(entry)
        )}
      </nav>

      {/* Bottom section: settings + utility buttons */}
      <div className="mt-auto px-2 flex flex-col gap-0.5 pt-2 border-t border-border mx-2">
        {renderNavItem(settingsItem)}

        <button
          onClick={() => navigate("/changelog")}
          className="text-[10px] text-muted-foreground/40 hover:text-muted-foreground text-center py-1 w-full transition-colors"
          title="查看更新日志"
        >
          v2.5.2
        </button>

        <div className="flex items-center gap-1 mt-1 px-1">
          <button
            onClick={() => setShowKeyManager(true)}
            className={cn(
              "flex-1 h-8 flex items-center justify-center gap-1 rounded-lg transition-colors",
              hasDbKey
                ? "text-emerald-500 hover:bg-emerald-500/10"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
            title={hasDbKey ? "密钥已获取，点击查看" : "获取微信密钥"}
          >
            <Key className="w-4 h-4" />
            {hasDbKey && <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />}
          </button>
          <button
            onClick={() => setConfirmPreload(true)}
            className="flex-1 h-8 flex items-center justify-center rounded-lg transition-colors text-muted-foreground hover:bg-muted hover:text-foreground"
            title={isMac
              ? "预加载全量图片（macOS 上 2025 年 5 月之后的加密图片会跳过）"
              : "预加载全量图片"}
          >
            <ImageIcon className="w-4 h-4" />
          </button>
          <button
            onClick={handleSync}
            disabled={isSyncing}
            className="flex-1 h-8 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            title="重新同步数据"
          >
            <RefreshCw className={cn("w-4 h-4", isSyncing && "animate-spin")} />
          </button>
          <button
            onClick={toggleTheme}
            className="flex-1 h-8 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            title="切换主题"
          >
            <ThemeIcon className="w-4 h-4" />
          </button>
        </div>
      </div>

      {showKeyManager && (
        <KeyManagerModal onClose={() => setShowKeyManager(false)} />
      )}

      <ConfirmDialog
        open={confirmPreload}
        title="预加载全部图片？"
        description={
          <>
            会把所有聊天图片解密后写入本地缓存，图片多时可能跑很久、期间占用较多 CPU。
            <br />
            跑起来之后可以随时在右下角的进度条上中断；已处理的部分会保留，下次继续不会重来。
          </>
        }
        confirmText="开始预加载"
        onCancel={() => setConfirmPreload(false)}
        onConfirm={() => {
          setConfirmPreload(false)
          handleFullCache()
        }}
      />

      <ImageCacheManager />
    </div>
  )
}
