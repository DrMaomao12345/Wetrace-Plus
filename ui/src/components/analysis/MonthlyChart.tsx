import { useState, useMemo, useEffect } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import type { YearMonthStat, MonthlyStat } from '@/api';
import { ym, monthWindowOf, monthValue } from '@/lib/monthSeries';

interface Props {
  data: YearMonthStat[];
  top10Avg: MonthlyStat[];
}

// 不同年份的线色
const YEAR_COLORS = [
  '#ec4899', // 粉
  '#22c55e', // 绿
  '#3b82f6', // 蓝
  '#f59e0b', // 橙
  '#a855f7', // 紫
  '#ef4444', // 红
  '#0ea5e9', // 天蓝
  '#84cc16', // 黄绿
];

export function MonthlyChart({ data, top10Avg }: Props) {
  const currentYear = new Date().getFullYear();

  // 数据中出现过的所有年份（按升序）
  const availableYears = useMemo(() => {
    const set = new Set<number>();
    for (const s of data || []) {
      if (s.year >= 2000 && s.year <= 2100) set.add(s.year);
    }
    return Array.from(set).sort((a, b) => a - b);
  }, [data]);

  // 选中显示的年份。默认勾选当年；如果当年没数据，勾选最近一年
  const [selectedYears, setSelectedYears] = useState<Set<number>>(() => {
    const init = new Set<number>();
    init.add(currentYear);
    return init;
  });
  // 数据初次加载或年份列表变化时，确保至少选中一个有数据的年份
  useEffect(() => {
    setSelectedYears((prev) => {
      const hasAnyValid = Array.from(prev).some(y => availableYears.includes(y));
      if (hasAnyValid || availableYears.length === 0) return prev;
      const next = new Set<number>()
      // 优先当年；否则最近一年
      if (availableYears.includes(currentYear)) next.add(currentYear)
      else next.add(availableYears[availableYears.length - 1])
      return next
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [availableYears.join(",")]);

  const [showAvg, setShowAvg] = useState(true);

  // 颜色映射：稳定按年份升序分配
  const yearColor = (y: number): string => {
    const idx = availableYears.indexOf(y);
    return YEAR_COLORS[idx % YEAR_COLORS.length];
  };

  // 历史月均
  const avgByMonth = new Array(13).fill(0);
  for (const s of (top10Avg || [])) {
    if (s.month >= 1 && s.month <= 12) avgByMonth[s.month] = s.count;
  }
  const hasAvg = avgByMonth.slice(1).some((v) => v > 0);

  // 把数据按年-月聚合成 { year2026: [12], year2025: [12], ... }
  const yearMonthMap = useMemo(() => {
    const map: Record<number, number[]> = {};
    for (const y of availableYears) map[y] = new Array(13).fill(0);
    for (const s of (data || [])) {
      if (map[s.year] && s.month >= 1 && s.month <= 12) {
        map[s.year][s.month] += s.count;
      }
    }
    return map;
  }, [data, availableYears]);

  // 哪几个月是真实存在的 —— 口径统一在 lib/monthSeries，说明见那里
  const window = useMemo(() => monthWindowOf(data || []), [data]);

  // 构造 recharts 的 row 数据
  const chartData = [];
  for (let m = 1; m <= 12; m++) {
    const row: Record<string, any> = { name: `${m}月` };
    for (const y of availableYears) {
      row[`y${y}`] = monthValue(ym(y, m), window, yearMonthMap[y][m]);
    }
    row.avg = avgByMonth[m];
    chartData.push(row);
  }

  const toggleYear = (y: number) => {
    setSelectedYears((prev) => {
      const next = new Set(prev);
      if (next.has(y)) next.delete(y);
      else next.add(y);
      return next;
    });
  };

  return (
    <div className="space-y-2">
      {/* 顶部控制栏：年份勾选 + 平均线开关 */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap text-xs">
          <span className="text-muted-foreground">年份：</span>
          {availableYears.map((y) => (
            <label key={y} className="flex items-center gap-1 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={selectedYears.has(y)}
                onChange={() => toggleYear(y)}
                className="accent-primary"
              />
              <span style={{ color: selectedYears.has(y) ? yearColor(y) : undefined }} className="font-medium">
                {y}年
              </span>
            </label>
          ))}
          {availableYears.length === 0 && (
            <span className="text-muted-foreground">暂无数据</span>
          )}
        </div>
        {hasAvg && (
          <label className="flex items-center gap-1 text-xs text-muted-foreground cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showAvg}
              onChange={(e) => setShowAvg(e.target.checked)}
              className="accent-primary"
            />
            显示平均线
          </label>
        )}
      </div>

      {hasAvg && showAvg && (
        <div className="text-xs text-muted-foreground">
          虚线：亲密度 Top10 联系人过去年份的月均
        </div>
      )}

      <div className="h-[260px] w-full">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="name" fontSize={12} tickLine={false} axisLine={false} />
            <YAxis fontSize={12} tickLine={false} axisLine={false} />
            <Tooltip
              contentStyle={{ borderRadius: '8px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
            />
            <Legend verticalAlign="top" height={28} />
            {availableYears
              .filter((y) => selectedYears.has(y))
              .map((y) => (
                <Line
                  key={y}
                  type="monotone"
                  dataKey={`y${y}`}
                  stroke={yearColor(y)}
                  strokeWidth={2}
                  dot={{ fill: yearColor(y), r: 3 }}
                  name={`${y}年`}
                />
              ))}
            {hasAvg && showAvg && (
              <Line
                type="monotone"
                dataKey="avg"
                stroke="#6366f1"
                strokeWidth={2}
                strokeDasharray="6 4"
                dot={false}
                name="Top10 历史月均"
              />
            )}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
