import { useQuery } from "@tanstack/react-query";
import { analysisApi } from "@/api";

export function useAnalysis(talker: string) {
  const hourlyQuery = useQuery({
    queryKey: ["analysis", "hourly", talker],
    queryFn: () => analysisApi.getHourly(talker),
    enabled: !!talker,
    retry: 1,
  });

  const dailyQuery = useQuery({
    queryKey: ["analysis", "daily", talker],
    queryFn: () => analysisApi.getDaily(talker),
    enabled: !!talker,
    retry: 1,
  });

  const weekdayQuery = useQuery({
    queryKey: ["analysis", "weekday", talker],
    queryFn: () => analysisApi.getWeekday(talker),
    enabled: !!talker,
    retry: 1,
  });

  const monthlyQuery = useQuery({
    queryKey: ["analysis", "monthly", talker],
    queryFn: () => analysisApi.getMonthly(talker),
    enabled: !!talker,
    retry: 1,
  });

  const typeQuery = useQuery({
    queryKey: ["analysis", "types", talker],
    queryFn: () => analysisApi.getTypeDistribution(talker),
    enabled: !!talker,
    retry: 1,
  });

  const memberQuery = useQuery({
    queryKey: ["analysis", "members", talker],
    queryFn: () => analysisApi.getMemberActivity(talker),
    enabled: !!talker,
    retry: 1,
  });

  const repeatQuery = useQuery({
    queryKey: ["analysis", "repeat", talker],
    queryFn: () => analysisApi.getRepeat(talker),
    enabled: !!talker,
    retry: 1,
  });

  const callsQuery = useQuery({
    queryKey: ["analysis", "calls", talker],
    queryFn: () => analysisApi.getCallStats(talker),
    enabled: !!talker,
    retry: 1,
  });

  const yearlyMonthlyQuery = useQuery({
    queryKey: ["analysis", "yearly_monthly", talker],
    queryFn: () => analysisApi.getYearlyMonthly(talker),
    enabled: !!talker,
    retry: 1,
  });

  const top10MonthlyAvgQuery = useQuery({
    queryKey: ["analysis", "top10_monthly_avg"],
    queryFn: () => analysisApi.getTopContactsMonthlyAvg(10),
    enabled: !!talker,
    retry: 1,
  });

  const isInitialLoading = (hourlyQuery.isLoading && hourlyQuery.isFetching) ||
                           (dailyQuery.isLoading && dailyQuery.isFetching);

  return {
    hourly: hourlyQuery,
    daily: dailyQuery,
    weekday: weekdayQuery,
    monthly: monthlyQuery,
    types: typeQuery,
    members: memberQuery,
    repeat: repeatQuery,
    calls: callsQuery,
    yearlyMonthly: yearlyMonthlyQuery,
    top10MonthlyAvg: top10MonthlyAvgQuery,
    isLoading: isInitialLoading,
  };
}
