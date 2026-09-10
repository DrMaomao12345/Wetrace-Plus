import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { UserSettings, AppConfig } from '@/types'
import { APP_VERSION } from '@/version'

interface AppState {
  config: AppConfig
  settings: UserSettings
  isMobile: boolean
  sidebarCollapsed: boolean
  activeNav: string
  
  // Actions
  setMobile: (isMobile: boolean) => void
  toggleSidebar: () => void
  setActiveNav: (nav: string) => void
  updateSettings: (settings: Partial<UserSettings>) => void
  toggleTheme: () => void
}

const defaultConfig: AppConfig = {
  title: 'Chatlog Session',
  version: APP_VERSION.slice(1),
  apiBaseUrl: 'http://127.0.0.1:5030',
  apiTimeout: 30000,
  pageSize: 500,
  maxPageSize: 5000,
  enableDebug: false,
  enableMock: false,
}

const defaultSettings: UserSettings = {
  theme: 'light',
  language: 'zh-CN',
  fontSize: 'medium',
  messageDensity: 'comfortable',
  enterToSend: true,
  autoPlayVoice: false,
  showMessagePreview: true,
  showTimestamp: true,
  showAvatar: true,
  timeFormat: '24h',
  showMediaResources: true,
  disableServerPinning: false,
  // 默认给区间：预测越远越不准，色带能自己把这件事说出来
  forecastDisplay: 'band',
}

export const useAppStore = create<AppState>()(
  persist(
    (set, get) => ({
      config: defaultConfig,
      settings: defaultSettings,
      isMobile: false,
      sidebarCollapsed: false,
      activeNav: 'import',

      setMobile: (isMobile) => set({ isMobile }),
      
      toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
      
      setActiveNav: (nav) => set({ activeNav: nav }),
      
      updateSettings: (newSettings) => {
        set((state) => {
          const settings = { ...state.settings, ...newSettings }
          // Apply theme side-effect
          if (newSettings.theme) {
            const html = document.documentElement
            if (newSettings.theme === 'dark' || (newSettings.theme === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
              html.classList.add('dark')
            } else {
              html.classList.remove('dark')
            }
          }
          return { settings }
        })
      },

      toggleTheme: () => {
        const { settings, updateSettings } = get()
        const themes: Array<'light' | 'dark' | 'auto'> = ['light', 'dark', 'auto']
        const currentIndex = themes.indexOf(settings.theme)
        const nextIndex = (currentIndex + 1) % themes.length
        updateSettings({ theme: themes[nextIndex] })
      },
    }),
    {
      name: 'app-storage',
      partialize: (state) => ({ settings: state.settings }), // Only persist settings
      // zustand 默认的 merge 是**浅合并顶层**：持久化里的 settings 会整个替换掉
      // defaultSettings，于是每新增一个设置项，老用户读回来都是 undefined ——
      // 新加的 forecastDisplay 就这么静默变成了 undefined，图上直接少画一半东西。
      // 这里改成「默认值打底、持久化的值覆盖」，以后再加字段不用记得改这里。
      merge: (persisted, current) => {
        const p = persisted as { settings?: Partial<UserSettings> } | undefined
        return {
          ...current,
          settings: { ...current.settings, ...(p?.settings ?? {}) },
        }
      },
    }
  )
)
