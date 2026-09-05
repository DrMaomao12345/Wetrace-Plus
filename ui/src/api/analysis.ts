import { request } from "@/lib/request";

export interface HourlyStat {
  hour: number;
  count: number;
}

export interface DailyStat {
  date: string;
  count: number;
}

export interface WeekdayStat {
  weekday: number;
  count: number;
}

export interface MonthlyStat {
  month: number;
  count: number;
}

/** 月度预测的一个点。来自 GET /api/v1/analysis/forecast/:id */
export interface ForecastPoint {
  year: number;
  month: number;
  /** 上下界的平均 —— 让点始终落在色带正中 */
  point: number;
  lo: number;   // 10% 分位
  hi: number;   // 90% 分位
  /** 距当前月几个月，0 = 当前月 */
  horizon: number;
  /** 仅当前月有意义：截至今天已经发生的条数 */
  actual: number;
  has_actual: boolean;
}
export interface ForecastResult {
  points: ForecastPoint[];
  /** 最近 90 天零消息天占比 */
  zero_day_ratio: number;
  confidence: 'high' | 'medium' | 'low';
  basis_days: number;
}

export interface MessageTypeStat {
  type: number;
  count: number;
}

export interface MemberActivity {
  memberId: number;
  platformId: string;
  name: string;
  messageCount: number;
  avatar?: string;
}

export interface RepeatStat {
  content: string;
  count: number;
  memberName: string;
}

export interface YearMonthStat {
  year: number;
  month: number;
  count: number;
}

export interface CallStats {
  total_calls: number;
  completed_calls: number;
  missed_calls: number;
  voice_calls: number;
  video_calls: number;
  total_duration: number;
  longest_duration: number;
  avg_duration: number;
}

export interface PersonalTopContact {
  talker: string;
  name: string;
  avatar: string;
  messageCount: number;
  sentCount: number;
  recvCount: number;
  lastTime: number;
}

export const analysisApi = {
  getHourly: (id: string) => request.get<HourlyStat[]>(`/api/v1/analysis/hourly/${id}`),
  getDaily: (id: string) => request.get<DailyStat[]>(`/api/v1/analysis/daily/${id}`),
  getWeekday: (id: string) => request.get<WeekdayStat[]>(`/api/v1/analysis/weekday/${id}`),
  getMonthly: (id: string) => request.get<MonthlyStat[]>(`/api/v1/analysis/monthly/${id}`),
  getTypeDistribution: (id: string) => request.get<MessageTypeStat[]>(`/api/v1/analysis/type_distribution/${id}`),
  getMemberActivity: (id: string) => request.get<MemberActivity[]>(`/api/v1/analysis/member_activity/${id}`),
  getRepeat: (id: string) => request.get<RepeatStat[]>(`/api/v1/analysis/repeat/${id}`),
  getCallStats: (id: string) => request.get<CallStats>(`/api/v1/analysis/calls/${id}`),
  getYearlyMonthly: (id: string) => request.get<YearMonthStat[]>(`/api/v1/analysis/yearly_monthly/${id}`),
  /** 当月到今年 12 月的聊天量预测。tzOffsetMinutes 东正西负（UTC+8 → 480） */
  getForecast: (id: string, year: number, tzOffsetMinutes: number) =>
    request.get<ForecastResult>(`/api/v1/analysis/forecast/${id}`, { year, tz_offset: tzOffsetMinutes }),
  getTopContactsMonthlyAvg: (limit: number = 10) => request.get<MonthlyStat[]>(`/api/v1/analysis/top_contacts_monthly_avg`, { limit }),
  getPersonalTopContacts: () => request.get<PersonalTopContact[]>('/api/v1/analysis/personal/top_contacts'),
};
