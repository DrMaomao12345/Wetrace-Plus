import { useAppStore } from "@/stores/app"
import { cn } from "@/lib/utils"
import { APP_VERSION } from "@/version"
import {
  MessageSquare, Moon, Sun, Monitor, Search, UploadCloud,
  ImageIcon, BarChart3, Sparkles, Users, Settings, Shield,
  ChevronDown, CalendarDays, Heart, Cloud, BrainCircuit, PlayCircle, Clock, History, Network, Newspaper,
} from "lucide-react"
import { useNavigate, useLocation } from "react-router-dom"
import { useState, useEffect } from "react"

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
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())

  const navEntries: NavEntry[] = [
    { key: 'import', icon: UploadCloud, label: '导入', path: '/import' },
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
          onClick={() => handleNavClick('import', '/import')}
        >
          <UploadCloud className="w-5 h-5" />
          <span className="font-semibold text-sm">Wetrace Plus</span>
        </div>
      </div>

      {/* Main nav */}
      <nav className="flex-1 min-h-0 overflow-y-auto overscroll-contain px-2 flex flex-col gap-0.5">
        {navEntries.map((entry) =>
          isGroup(entry) ? renderNavGroup(entry) : renderNavItem(entry)
        )}
      </nav>

      {/* Bottom section: utility buttons + version */}
      <div className="mx-2 mt-auto flex flex-col border-t border-border px-2 pt-2">
        <div className="flex items-center justify-center gap-1">
          <button
            onClick={() => handleNavClick('settings', '/settings')}
            aria-label="设置"
            className={cn(
              "flex h-8 w-8 items-center justify-center rounded-lg transition-colors",
              isActive('settings')
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            )}
            title="设置"
          >
            <Settings className="h-4 w-4" />
          </button>
          <button
            onClick={toggleTheme}
            aria-label="切换显示模式"
            className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="切换显示模式"
          >
            <ThemeIcon className="w-4 h-4" />
          </button>
        </div>

        <button
          onClick={() => navigate("/changelog")}
          className="w-full py-1.5 text-center text-[10px] text-muted-foreground/40 transition-colors hover:text-muted-foreground"
          title="查看更新日志"
        >
          {APP_VERSION}
        </button>
      </div>

    </div>
  )
}
