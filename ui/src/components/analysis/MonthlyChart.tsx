import { useState, useMemo, useEffect } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import type { YearMonthStat, MonthlyStat } from '@/api';

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

  // —— 时间边界 ——
  // 图上的 0 有两种完全不同的含义，必须分开：
  //   · **真 0**：那个月确实一条没聊 —— 该画，那是信息
  //   · **假 0**：那个月压根不存在 —— 要么联系人还没加上（第一条消息之前），
  //     要么这个月还没到（未来）。后端只按 GROUP BY 返回有数据的月份，
  //     这里补的 0 全是假的，画出来就成了「贴着 X 轴的一条平线」和「年底跌到 0」。
  // 假 0 一律给 null：recharts 默认 connectNulls=false，线会断开、点也不画，
  // Tooltip 的 filterNull 默认 true，悬浮时也不会列出这些年份。
  const firstYM = useMemo(() => {
    let min = Infinity;
    for (const s of (data || [])) {
      if (s.count > 0 && s.month >= 1 && s.month <= 12) {
        min = Math.min(min, s.year * 12 + s.month);
      }
    }
    return min;
  }, [data]);
  // 「现在」按浏览器时区算，和 api/insights.ts 里的 tz() 同口径。
  // 当月是**已经开始**的月份，照常显示（哪怕只过了几天）。
  const nowYM = currentYear * 12 + (new Date().getMonth() + 1);

  // 构造 recharts 的 row 数据
  const chartData = [];
  for (let m = 1; m <= 12; m++) {
    const row: Record<string, any> = { name: `${m}月` };
    for (const y of availableYears) {
      const ym = y * 12 + m;
      row[`y${y}`] = (ym < firstYM || ym > nowYM) ? null : yearMonthMap[y][m];
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
