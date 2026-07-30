import { request } from '@/lib/request'

// 浏览器时区偏移（分钟，东正西负），UTC+8 → 480
const tz = () => -new Date().getTimezoneOffset()

export interface DayHeat { date: string; count: number }
export interface InteractionRatio {
  talker: string; name: string; avatar: string
  sent_count: number; recv_count: number
  my_initiations: number; their_initiations: number
  total: number; last_time: number
}
export interface ReplySpeed {
  talker: string; name: string; avatar: string
  my_avg_reply_sec: number; my_fastest_sec: number; my_slowest_sec: number; my_reply_count: number
  their_avg_reply_sec: number; their_reply_count: number
  late_night_instant: number; ignored_by_them: number
}
export interface CommonGroup { username: string; name: string; avatar: string; member_count: number }
export interface AnnualOverviewLite {
  total_messages: number; sent_messages: number; received_messages: number
  active_contacts: number; active_chatrooms: number; active_days: number
}
export interface ContactYearDelta {
  talker: string; name: string; avatar: string; is_group: boolean
  count_a: number; count_b: number; delta: number
}
export interface YearCompare {
  year_a: number; year_b: number
  overview_a: AnnualOverviewLite; overview_b: AnnualOverviewLite
  faded_out: ContactYearDelta[]; newly_active: ContactYearDelta[]
  rising: ContactYearDelta[]; falling: ContactYearDelta[]
}

export const insightsApi = {
  calendarHeatmap: (year: number) =>
    request.get<DayHeat[]>('/api/v1/analysis/calendar_heatmap', { year, tz_offset: tz() }),
  interactionRatios: (year: number, limit = 200) =>
    request.get<InteractionRatio[]>('/api/v1/analysis/interaction_ratios', { year, tz_offset: tz(), limit }),
  replySpeed: (year: number, limit = 200) =>
    request.get<ReplySpeed[]>('/api/v1/analysis/reply_speed', { year, tz_offset: tz(), limit }),
  commonGroups: (talker: string) =>
    request.get<CommonGroup[]>('/api/v1/analysis/common_groups', { talker }),
  yearCompare: (yearA: number, yearB: number) =>
    request.get<YearCompare>('/api/v1/report/year_compare', { year_a: yearA, year_b: yearB, tz_offset: tz() }),
}
