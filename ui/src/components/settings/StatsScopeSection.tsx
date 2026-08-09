import { useEffect, useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { statsScopeApi } from "@/api"
import type { StatsModule, StatsScope } from "@/api/statsScope"
import type { TalkerTag } from "@/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Filter, Loader2, RotateCcw, Search, Tags } from "lucide-react"

/** 「只统计真人 + 群聊」预设:剔掉公众号 / 服务号 / 系统账号 */
const PERSON_ONLY: Record<string, boolean> = {
  friend: true,
  group: true,
  work: true,
  stranger: true,
  subscription: false,
  service: false,
  system: false,
  other: false,
}

function allOn(keys: string[]): Record<string, boolean> {
  return Object.fromEntries(keys.map((k) => [k, true]))
}

function sameTypes(a: Record<string, boolean>, b: Record<string, boolean>, keys: string[]) {
  return keys.every((k) => Boolean(a[k]) === Boolean(b[k]))
}

export function StatsScopeSection() {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<StatsScope | null>(null)
  const [keyword, setKeyword] = useState("")
  const [tagSearch, setTagSearch] = useState("")
  const [showTagEditor, setShowTagEditor] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ["stats-scope"],
    queryFn: () => statsScopeApi.getScope(),
  })

  useEffect(() => {
    if (data?.scope && draft === null) setDraft(data.scope)
  }, [data])

  const typeKeys = useMemo(() => (data?.talker_types ?? []).map((t) => t.key), [data])

  const saveMutation = useMutation({
    mutationFn: (scope: StatsScope) => statsScopeApi.saveScope(scope),
    onSuccess: () => {
      toast.success("统计范围已保存,相关统计会按新范围重新计算")
      // 所有依赖统计结果的查询都得重来
      queryClient.invalidateQueries()
    },
    onError: (e: Error) => toast.error("保存失败: " + e.message),
  })

  const save = (next: StatsScope) => {
    setDraft(next)
    saveMutation.mutate(next)
  }

  const setGlobal = (key: string, value: boolean) => {
    if (!draft) return
    save({ ...draft, global: { ...draft.global, [key]: value } })
  }

  const toggleModuleOverride = (mod: StatsModule, enabled: boolean) => {
    if (!draft) return
    const modules = { ...(draft.modules ?? {}) }
    if (enabled) {
      // 开启覆盖时以当前全局作为起点,用户再单独调
      modules[mod] = { ...draft.global }
    } else {
      delete modules[mod]
    }
    save({ ...draft, modules })
  }

  const setModuleType = (mod: StatsModule, key: string, value: boolean) => {
    if (!draft) return
    const current = draft.modules?.[mod] ?? draft.global
    const modules = { ...(draft.modules ?? {}), [mod]: { ...current, [key]: value } }
    save({ ...draft, modules })
  }

  if (isLoading || !draft || !data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Filter className="w-4 h-4 text-primary" />
            统计范围
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin" />
            加载中…
          </div>
        </CardContent>
      </Card>
    )
  }

  const isPersonOnly = sameTypes(draft.global, PERSON_ONLY, typeKeys)
  const isAllOn = typeKeys.every((k) => draft.global[k] !== false)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Filter className="w-4 h-4 text-primary" />
          统计范围
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <p className="text-xs text-muted-foreground">
          每个聊天都会被自动打上类型标签(联系人 / 群聊 / 订阅号 / 服务号 / 企业微信 / 陌生人 / 系统账号)。
          关掉某个类型,它的消息就不再计入统计。
          <span className="block mt-1">
            注:该过滤对微信 4.x（macOS 当前使用的格式）逐会话生效;旧版 v3 单表数据库只能靠下方的手动标签与年度报告的排除名单。
          </span>
        </p>

        {/* 预设 */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">快捷预设</span>
          <Button
            size="sm"
            variant={isPersonOnly ? "default" : "outline"}
            onClick={() => save({ ...draft, global: { ...PERSON_ONLY } })}
          >
            只统计真人 + 群聊
          </Button>
          <Button
            size="sm"
            variant={isAllOn ? "default" : "outline"}
            onClick={() => save({ ...draft, global: allOn(typeKeys) })}
          >
            全部统计
          </Button>
        </div>

        {/* 全局开关 */}
        <div className="space-y-2">
          <div className="text-sm font-medium">全局默认</div>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
            {data.talker_types.map((t) => (
              <label
                key={t.key}
                className="flex items-center justify-between gap-2 rounded-md border border-input px-3 py-2"
              >
                <span className="text-sm">{t.label}</span>
                <Switch
                  checked={draft.global[t.key] !== false}
                  onCheckedChange={(v) => setGlobal(t.key, v)}
                />
              </label>
            ))}
          </div>
        </div>

        {/* 分模块覆盖 */}
        <div className="space-y-2">
          <div className="text-sm font-medium">按统计模块单独设置</div>
          <p className="text-xs text-muted-foreground">
            默认所有模块跟随上面的全局设置。打开开关后,该模块使用自己的一套类型范围。
          </p>
          <div className="space-y-2">
            {data.modules.map((m) => {
              const mod = m.key as StatsModule
              const override = draft.modules?.[mod]
              return (
                <div key={m.key} className="rounded-md border border-input">
                  <div className="flex items-center justify-between px-3 py-2">
                    <span className="text-sm">{m.label}</span>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-muted-foreground">
                        {override ? "单独设置" : "跟随全局"}
                      </span>
                      <Switch
                        checked={Boolean(override)}
                        onCheckedChange={(v) => toggleModuleOverride(mod, v)}
                      />
                    </div>
                  </div>
                  {override && (
                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 px-3 pb-3">
                      {data.talker_types.map((t) => (
                        <label key={t.key} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="h-4 w-4"
                            checked={override[t.key] !== false}
                            onChange={(e) => setModuleType(mod, t.key, e.target.checked)}
                          />
                          {t.label}
                        </label>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        {/* 手动改单个聊天的标签 */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <div className="text-sm font-medium flex items-center gap-2">
              <Tags className="w-4 h-4 text-muted-foreground" />
              手动调整聊天标签
            </div>
            <Button size="sm" variant="outline" onClick={() => setShowTagEditor((v) => !v)}>
              {showTagEditor ? "收起" : "展开"}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            自动分类判错时可以在这里改。比如把某个服务号当成联系人统计,或把一个不想统计的联系人标成「其他」再关掉该类型。
          </p>
          {showTagEditor && (
            <TalkerTagEditor
              keyword={keyword}
              tagSearch={tagSearch}
              setKeyword={setKeyword}
              setTagSearch={setTagSearch}
              talkerTypes={data.talker_types}
            />
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function TalkerTagEditor({
  keyword,
  tagSearch,
  setKeyword,
  setTagSearch,
  talkerTypes,
}: {
  keyword: string
  tagSearch: string
  setKeyword: (v: string) => void
  setTagSearch: (v: string) => void
  talkerTypes: { key: string; label: string }[]
}) {
  const queryClient = useQueryClient()

  const { data, isFetching } = useQuery({
    queryKey: ["talker-tags", tagSearch],
    queryFn: () => statsScopeApi.getTalkerTags({ keyword: tagSearch }),
  })

  const mutation = useMutation({
    mutationFn: ({ talker, type }: { talker: string; type: TalkerTag | "" }) =>
      statsScopeApi.setTalkerTag(talker, type),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["talker-tags"] })
      queryClient.invalidateQueries({ queryKey: ["stats-scope"] })
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
    },
    onError: (e: Error) => toast.error("保存失败: " + e.message),
  })

  const items = data?.items ?? []

  return (
    <div className="space-y-3">
      {/* 各类型数量概览 */}
      <div className="flex flex-wrap gap-2">
        {(data?.counts ?? []).map((c) => (
          <span
            key={c.key}
            className="text-xs rounded-full border border-input px-2 py-0.5 text-muted-foreground"
          >
            {c.label} {c.count}
          </span>
        ))}
      </div>

      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") setTagSearch(keyword.trim())
            }}
            placeholder="搜索聊天名称或 ID,回车搜索"
            className="pl-8"
          />
        </div>
        <Button size="sm" onClick={() => setTagSearch(keyword.trim())}>
          搜索
        </Button>
      </div>

      <div className="max-h-80 overflow-y-auto rounded-md border border-input divide-y divide-border">
        {isFetching && items.length === 0 && (
          <div className="px-3 py-6 text-center text-sm text-muted-foreground">加载中…</div>
        )}
        {!isFetching && items.length === 0 && (
          <div className="px-3 py-6 text-center text-sm text-muted-foreground">没有匹配的聊天</div>
        )}
        {items.slice(0, 300).map((item) => (
          <div key={item.talker} className="flex items-center gap-2 px-3 py-2">
            <div className="min-w-0 flex-1">
              <div className="text-sm truncate">{item.name}</div>
              <div className="text-[11px] text-muted-foreground truncate">{item.talker}</div>
            </div>
            {item.override && (
              <span className="text-[11px] text-amber-600 dark:text-amber-500 shrink-0">已手动指定</span>
            )}
            <select
              value={item.effective}
              onChange={(e) =>
                mutation.mutate({ talker: item.talker, type: e.target.value as TalkerTag })
              }
              className="h-8 rounded-md border border-input bg-background px-2 text-xs shrink-0"
            >
              {talkerTypes.map((t) => (
                <option key={t.key} value={t.key}>
                  {t.label}
                </option>
              ))}
            </select>
            {item.override && (
              <Button
                size="sm"
                variant="ghost"
                title="恢复自动分类"
                onClick={() => mutation.mutate({ talker: item.talker, type: "" })}
              >
                <RotateCcw className="w-3.5 h-3.5" />
              </Button>
            )}
          </div>
        ))}
      </div>
      {items.length > 300 && (
        <p className="text-xs text-muted-foreground">
          仅显示前 300 条,用搜索缩小范围(共 {items.length} 条)。
        </p>
      )}
    </div>
  )
}
