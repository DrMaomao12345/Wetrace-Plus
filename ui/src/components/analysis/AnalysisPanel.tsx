import { useState } from "react";
import { useAnalysis } from "@/hooks/useAnalysis";
import {
  X, BarChart3, TrendingUp, MessageSquare,
  Calendar, PieChart as PieIcon, Search, Loader2, Phone,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { HourlyChart } from "./HourlyChart";
import { DailyChart } from "./DailyChart";
import { WeekdayChart } from "./WeekdayChart";
import { MonthlyChart } from "./MonthlyChart";
import { TypePieChart } from "./TypePieChart";
import { TalkerExtras } from "./TalkerExtras";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { searchApi, type CallStats } from "@/api";
import { toast } from "sonner";

interface Props {
  talker: string;
  onClose: () => void;
}

interface CountResult {
  keyword: string;
  messageCount: number;
  totalOccurrences: number;
}

export function AnalysisPanel({ talker, onClose }: Props) {
  const { hourly, daily, weekday, types, calls, yearlyMonthly, top10MonthlyAvg, isLoading } = useAnalysis(talker);

  // 搜索统计 state
  const [searchKw, setSearchKw] = useState("");
  const [counting, setCounting] = useState(false);
  const [countResult, setCountResult] = useState<CountResult | null>(null);

  const handleCount = async () => {
    const kw = searchKw.trim();
    if (!kw) return;
    setCounting(true);
    setCountResult(null);
    try {
      // 1. 用 search 拿到包含关键词的所有消息（最多 5000 条）
      const res = await searchApi.search({ keyword: kw, talker, limit: 5000 });
      // 2. 客户端逐条统计字符出现次数
      let total = 0;
      for (const m of res.items) {
        if (!m.content) continue;
        // 转义正则特殊字符
        const escaped = kw.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
        const matches = m.content.match(new RegExp(escaped, "g"));
        if (matches) total += matches.length;
      }
      setCountResult({ keyword: kw, messageCount: res.total, totalOccurrences: total });
    } catch (e: any) {
      toast.error("统计失败: " + (e?.message || ""));
    } finally {
      setCounting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-background flex flex-col animate-in fade-in slide-in-from-bottom-4 duration-300">
      <div className="h-14 border-b flex items-center justify-between px-6 bg-card/50 backdrop-blur-md sticky top-0 z-10">
        <div className="flex items-center gap-2">
          <BarChart3 className="w-5 h-5 text-pink-500" />
          <h2 className="font-semibold text-lg">数据分析报告</h2>
        </div>
        <Button variant="ghost" size="icon" onClick={onClose} className="rounded-full">
          <X className="w-5 h-5" />
        </Button>
      </div>

      <ScrollArea className="flex-1">
        <div className="max-w-6xl mx-auto p-6 space-y-6 pb-20">
          {isLoading ? (
            <div className="flex items-center justify-center h-64">
              <div className="flex flex-col items-center gap-4">
                <div className="w-10 h-10 border-4 border-pink-500 border-t-transparent rounded-full animate-spin" />
                <p className="text-muted-foreground animate-pulse">正在深度挖掘数据中...</p>
              </div>
            </div>
          ) : (
            <>
              {/* 顶部统计卡 */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Card>
                  <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                    <CardTitle className="text-sm font-medium">总消息数</CardTitle>
                    <MessageSquare className="h-4 w-4 text-muted-foreground" />
                  </CardHeader>
                  <CardContent>
                    <div className="text-2xl font-bold">
                      {(daily.data?.reduce((sum, d) => sum + d.count, 0) || 0).toLocaleString()}
                    </div>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                    <CardTitle className="text-sm font-medium">活跃天数</CardTitle>
                    <TrendingUp className="h-4 w-4 text-muted-foreground" />
                  </CardHeader>
                  <CardContent>
                    <div className="text-2xl font-bold">{daily.data?.length || 0} 天</div>
                  </CardContent>
                </Card>
              </div>

              {/* 日历热力图 / 语音 / 互动与回复 */}
              <TalkerExtras talker={talker} />

              {/* 通话统计 */}
              {calls.data && calls.data.total_calls > 0 && (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Phone className="w-4 h-4 text-pink-500" />
                      通话统计
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <CallStatsView stats={calls.data} />
                  </CardContent>
                </Card>
              )}

              {/* 关键词统计 */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Search className="w-4 h-4 text-pink-500" />
                    关键词出现次数统计
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-2">
                    <Input
                      placeholder="输入要统计的关键词，例如 ?"
                      value={searchKw}
                      onChange={(e) => setSearchKw(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Enter") handleCount() }}
                      className="h-9 max-w-sm"
                    />
                    <Button size="sm" onClick={handleCount} disabled={counting || !searchKw.trim()}>
                      {counting ? <Loader2 className="w-4 h-4 animate-spin" /> : "统计"}
                    </Button>
                  </div>
                  {countResult && (
                    <div className="mt-4 grid grid-cols-2 gap-4 max-w-md">
                      <div className="rounded-lg border p-3">
                        <div className="text-xs text-muted-foreground">包含的消息数</div>
                        <div className="text-2xl font-bold mt-1">{countResult.messageCount.toLocaleString()}</div>
                      </div>
                      <div className="rounded-lg border p-3 bg-pink-50/50 dark:bg-pink-950/20">
                        <div className="text-xs text-muted-foreground">「{countResult.keyword}」出现总次数</div>
                        <div className="text-2xl font-bold mt-1 text-pink-600 dark:text-pink-400">
                          {countResult.totalOccurrences.toLocaleString()}
                        </div>
                      </div>
                    </div>
                  )}
                  <p className="text-xs text-muted-foreground mt-3">
                    最多统计最近 5000 条匹配消息中的字符出现次数
                  </p>
                </CardContent>
              </Card>

              {/* 24小时活跃 */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <BarChart3 className="w-4 h-4 text-pink-500" />
                    24小时活跃分布
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <HourlyChart data={hourly.data || []} />
                </CardContent>
              </Card>

              {/* 星期分布 + 月度趋势 */}
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Calendar className="w-4 h-4 text-pink-500" />
                      星期分布
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <WeekdayChart data={weekday.data || []} />
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <TrendingUp className="w-4 h-4 text-pink-500" />
                      月度消息趋势
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <MonthlyChart data={yearlyMonthly.data || []} top10Avg={top10MonthlyAvg.data || []} />
                  </CardContent>
                </Card>
              </div>

              {/* 每日消息趋势 */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <TrendingUp className="w-4 h-4 text-pink-500" />
                    每日消息趋势
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <DailyChart data={daily.data || []} />
                </CardContent>
              </Card>

              {/* 消息类型分布 */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <PieIcon className="w-4 h-4 text-pink-500" />
                    消息类型分布
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <TypePieChart data={types.data || []} />
                </CardContent>
              </Card>

            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

function formatDuration(sec: number): string {
  if (sec <= 0) return "0秒";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const parts: string[] = [];
  if (h > 0) parts.push(`${h}小时`);
  if (m > 0) parts.push(`${m}分`);
  if (s > 0 || parts.length === 0) parts.push(`${s}秒`);
  return parts.join("");
}

function CallStatsView({ stats }: { stats: CallStats }) {
  const items = [
    { label: "总通话次数", value: stats.total_calls.toLocaleString(), accent: "text-foreground" },
    { label: "已接通", value: stats.completed_calls.toLocaleString(), accent: "text-green-600 dark:text-green-400" },
    { label: "未接通/取消", value: stats.missed_calls.toLocaleString(), accent: "text-muted-foreground" },
    { label: "语音通话", value: stats.voice_calls.toLocaleString(), accent: "text-blue-600 dark:text-blue-400" },
    { label: "视频通话", value: stats.video_calls.toLocaleString(), accent: "text-violet-600 dark:text-violet-400" },
    { label: "通话总时长", value: formatDuration(stats.total_duration), accent: "text-pink-600 dark:text-pink-400" },
    { label: "最长一次", value: formatDuration(stats.longest_duration), accent: "text-foreground" },
    { label: "平均时长", value: formatDuration(stats.avg_duration), accent: "text-foreground" },
  ];
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      {items.map((it) => (
        <div key={it.label} className="rounded-lg border p-3">
          <div className="text-xs text-muted-foreground">{it.label}</div>
          <div className={`text-lg font-bold mt-1 ${it.accent}`}>{it.value}</div>
        </div>
      ))}
    </div>
  );
}

