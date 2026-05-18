import { request } from "@/lib/request";

export interface AnnualOverview {
  total_messages: number;
  sent_messages: number;
  received_messages: number;
  total_contacts: number;
  active_contacts: number;
  total_chatrooms: number;
  active_chatrooms: number;
  first_message_date: string;
  last_message_date: string;
  active_days: number;
}

export interface AnnualHighlights {
  busiest_day: { date: string; count: number };
  quietest_day: { date: string; count: number };
  longest_streak: number;
  late_night_count: number;
  earliest_message_time: string;
  latest_message_time: string;
}

export interface ContactWordCountStat {
  talker: string;
  sentChars: number;
  recvChars: number;
  totalChars: number;
}

export interface WordCountStat {
  total_chars: number;
  sent_chars: number;
  recv_chars: number;
  contacts: ContactWordCountStat[] | null;
}

export interface OverviewDeltas {
  total_messages: number | null;
  sent_messages: number | null;
  received_messages: number | null;
  active_contacts: number | null;
  active_chatrooms: number | null;
  active_days: number | null;
}

export interface AnnualReport {
  year: number;
  data_version?: string;
  overview: AnnualOverview;
  overview_deltas?: OverviewDeltas | null;
  top_contacts: Array<{
    talker: string;
    name: string;
    avatar: string;
    isGroup: boolean;
    messageCount: number;
    sentCount: number;
    recvCount: number;
  }>;
  monthly_trend: Array<{ month: number; count: number }>;
  past_years_monthly_avg: Array<{ month: number; count: number }>;
  weekday_distribution: Array<{ weekday: number; count: number }>;
  hourly_distribution: Array<{ hour: number; count: number }>;
  message_types: Record<string, number>;
  highlights: AnnualHighlights;
}

export interface TZSegment {
  start_date: string;   // "2024-02-01"
  end_date: string;     // "2024-02-19"
  tz_offset: number;    // 分钟，东正西负（UTC+8 → 480）
}

export const reportApi = {
  getAnnualReport: (year: number, defaultTzOffset: number, tzSegments: TZSegment[] = [], excludeTalkers: string[] = []) =>
    request.post<AnnualReport>("/api/v1/report/annual", {
      year,
      default_tz_offset: defaultTzOffset,
      tz_segments: tzSegments,
      exclude_talkers: excludeTalkers,
    }),

  // 自定义范围的「往年月度趋势平均」
  getPastMonthlyAvg: (from: number, to: number, tzOffsetMinutes: number) =>
    request.get<Array<{ month: number; count: number }>>(
      "/api/v1/report/past_monthly_avg",
      { from, to, tz_offset: tzOffsetMinutes }
    ),

  // 字数统计（按消息内容字符数）
  getWordCounts: (year: number, defaultTzOffset: number, tzSegments: TZSegment[] = [], excludeTalkers: string[] = []) =>
    request.post<WordCountStat>("/api/v1/report/word_count", {
      year,
      default_tz_offset: defaultTzOffset,
      tz_segments: tzSegments,
      exclude_talkers: excludeTalkers,
    }),

  // 服务端持久化的「排除联系人」名单（网页与移动端共用）
  getExcludeTalkers: () =>
    request.get<{ talkers: string[] }>("/api/v1/report/exclude_talkers"),

  saveExcludeTalkers: (talkers: string[]) =>
    request.post<{ status: string }>("/api/v1/report/exclude_talkers", { talkers }),

  // 局部刷新：拿到当前生效的往年月均 + 往年同期 overview 平均
  getReportBaseline: (year: number, tzOffsetMinutes: number) =>
    request.get<{
      past_start_year: number;
      past_end_year: number;
      past_years_monthly_avg: Array<{ month: number; count: number }>;
      past_overview_avg: AnnualOverview | null;
    }>("/api/v1/report/baseline", { year, tz_offset: tzOffsetMinutes }),
};
