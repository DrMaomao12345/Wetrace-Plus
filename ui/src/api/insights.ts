import { request } from '@/lib/request'

// 浏览器时区偏移（分钟，东正西负），UTC+8 → 480
const tz = () => -new Date().getTimezoneOffset()

export interface DayHeat { date: string; count: number }

/** 日历热力图月份下钻：该月和谁聊过、各聊了多少天 */
export interface MonthPartner {
  username: string
  name: string
  avatar: string
  is_group: boolean
  days: number
  messages: number
}
export interface MonthPartners {
  year: number
  month: number
  /** <=0 表示整月；>=1 表示只统计那一天 */
  day: number
  total_days: number
  total_msgs: number
  total_peers: number
  partners: MonthPartner[]
}
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

export interface DailyPartner {
  username: string
  name: string
  is_group: boolean
  messages: number
  sent: number
  recv: number
  last_time: number
  first_by_self: boolean
}

export interface DailyReport {
  date: string
  total_messages: number
  sent_messages: number
  recv_messages: number
  active_peers: number
  active_groups: number
  total_peers: number
  first_time: number
  last_time: number
  hourly: number[]
  partners: DailyPartner[]
  types: { type: number; name: string; count: number }[]
  sent_chars: number
  recv_chars: number
  voice_chars: number
  peak_hour: number
  peak_hour_count: number
  initiated_by_me: number
  initiated_by_them: number
  prev_day_total: number
  last_week_total: number
  streak_days: number
  streak_capped: boolean
}

export const insightsApi = {
  calendarHeatmap: (year: number) =>
    request.get<DayHeat[]>('/api/v1/analysis/calendar_heatmap', { year, tz_offset: tz() }),
  /** 今日报告。date 省略=服务端按用户时区取今天 */
  dailyReport: (date?: string, top = 20) =>
    request.get<DailyReport>('/api/v1/analysis/daily_report',
      { ...(date ? { date } : {}), tz_offset: tz(), top }),

  /** 热力图下钻。day 省略=整月，给了就只看那一天 */
  heatmapPartners: (year: number, month: number, day?: number, limit = 30) =>
    request.get<MonthPartners>('/api/v1/analysis/heatmap_partners',
      { year, month, ...(day ? { day } : {}), tz_offset: tz(), limit }),
  interactionRatios: (year: number, limit = 200) =>
    request.get<InteractionRatio[]>('/api/v1/analysis/interaction_ratios', { year, tz_offset: tz(), limit }),
  replySpeed: (year: number, limit = 200) =>
    request.get<ReplySpeed[]>('/api/v1/analysis/reply_speed', { year, tz_offset: tz(), limit }),
  commonGroups: (talker: string) =>
    request.get<CommonGroup[]>('/api/v1/analysis/common_groups', { talker }),
  yearCompare: (yearA: number, yearB: number) =>
    request.get<YearCompare>('/api/v1/report/year_compare', { year_a: yearA, year_b: yearB, tz_offset: tz() }),
}
