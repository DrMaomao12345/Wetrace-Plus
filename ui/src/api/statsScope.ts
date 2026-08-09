import { request } from "@/lib/request"
import type { TalkerTag } from "@/types"

/** 可以单独覆盖统计范围的模块 key，与后端 model.StatsModule 对应 */
export type StatsModule =
  | "report"
  | "insights"
  | "galaxy"
  | "dashboard"
  | "wordcloud"
  | "reminder"
  | "biz"

export interface StatsScope {
  /** 全局默认：哪些类型参与统计 */
  global: Record<string, boolean>
  /** 出现在这里的模块用自己的开关覆盖全局；没出现则继承全局 */
  modules?: Partial<Record<StatsModule, Record<string, boolean>>>
  /** 单个会话的手动类型覆盖 */
  overrides?: Record<string, TalkerTag>
}

export interface LabeledOption {
  key: string
  label: string
}

export interface StatsScopeResponse {
  scope: StatsScope
  talker_types: LabeledOption[]
  modules: LabeledOption[]
}

export interface TalkerTagItem {
  talker: string
  name: string
  auto: TalkerTag
  effective: TalkerTag
  override: boolean
}

export interface TalkerTagsResponse {
  items: TalkerTagItem[]
  counts: { key: TalkerTag; label: string; count: number }[]
}

export const statsScopeApi = {
  getScope: () => request.get<StatsScopeResponse>("/api/v1/stats/scope"),

  saveScope: (scope: StatsScope) =>
    request.post<{ status: string }>("/api/v1/stats/scope", scope),

  getTalkerTags: (params?: { keyword?: string; type?: string }) =>
    request.get<TalkerTagsResponse>("/api/v1/stats/talker_tags", params),

  /** type 传空字符串表示恢复自动分类 */
  setTalkerTag: (talker: string, type: TalkerTag | "") =>
    request.post<{ status: string }>("/api/v1/stats/talker_tags", { talker, type }),
}
