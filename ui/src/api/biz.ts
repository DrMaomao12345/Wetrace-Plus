import { request } from "@/lib/request"

export interface BizAccount {
  talker: string
  name: string
  avatar: string
  type: string
  type_label: string
  push_count: number
  first_time: number
  last_time: number
  active_months: number
  monthly_avg: number
  lifetime_last: number
}

export interface BizOverview {
  followed_total: number
  pushing_accounts: number
  silent_accounts: number
  total_pushes: number
  daily_avg: number
  peak_hour: number
  busiest_date: string
  busiest_count: number
  subscription_cnt: number
  service_cnt: number
}

export interface BizProfile {
  year: number
  overview: BizOverview
  top_accounts: BizAccount[] | null
  silent_top: BizAccount[] | null
  monthly: { month: string; count: number }[] | null
  hourly: { hour: number; count: number }[] | null
  title_keywords: { text: string; count: number }[] | null
  has_data: boolean
}

export const bizApi = {
  /** year=0 表示全部历史 */
  getProfile: (year: number, withTitles = true) =>
    request.get<BizProfile>("/api/v1/biz/profile", {
      year,
      with_titles: withTitles ? 1 : 0,
    }),
}
