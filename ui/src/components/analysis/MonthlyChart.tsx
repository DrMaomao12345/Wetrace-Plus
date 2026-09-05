import { useState, useMemo, useEffect } from 'react';
import { ComposedChart, Line, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import type { YearMonthStat, MonthlyStat, ForecastResult } from '@/api';
import { ym, monthWindowOf, monthValue } from '@/lib/monthSeries';
import { useAppStore } from '@/stores/app';

/** 有回测支撑的最远预测距离。再往后只留色带，不画点。 */
const TRUSTED_HORIZON = 3;

const CONFIDENCE_LABEL: Record<string, string> = {
  high: '较高', medium: '一般', low: '较低',
};

interface Props {
  data: YearMonthStat[];
  top10Avg: MonthlyStat[];
  /** 当月到今年 12 月的预测；没有就不画 */
  forecast?: ForecastResult | null;
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

export function MonthlyChart({ data, top10Avg, forecast }: Props) {
  const forecastDisplay = useAppStore((st) => st.settings.forecastDisplay);
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

  // 预测只对**今年**有意义，且今年得被勾上；往年是既成事实，不需要预测
  const fcByMonth = useMemo(() => {
    const map = new Map<number, ForecastResult['points'][number]>();
    if (!forecast || !selectedYears.has(currentYear)) return map;
    for (const p of forecast.points) {
      if (p.year === currentYear) map.set(p.month, p);
    }
    return map;
  }, [forecast, selectedYears, currentYear]);
  const hasForecast = fcByMonth.size > 0;
  const currentMonth = new Date().getMonth() + 1;

  // 构造 recharts 的 row 数据
  const chartData = [];
  for (let m = 1; m <= 12; m++) {
    const row: Record<string, any> = { name: `${m}月` };
    for (const y of availableYears) {
      // 画预测时，实线只到**最后一个完整月**。
      // 当月才过了几天，它的部分值画在实线上就是一个假的断崖 —— 用户会读成
      // 「不聊了」，而实际只是月份没过完。那个数字没丢，进了 Tooltip 的
      // 「截至今天已 N 条」，月末预计值由空心点接手。
      const partialCurrentMonth = hasForecast && y === currentYear && m >= currentMonth;
      row[`y${y}`] = partialCurrentMonth ? null : monthValue(ym(y, m), window, yearMonthMap[y][m]);
    }
    row.avg = avgByMonth[m];

    // 把色带的左端锚在**最后一个完整月**的实际值上（零宽度），
    // 否则色带会从 9 月凭空开始、和绿线断开，看不出是同一条故事线
    const isAnchor = hasForecast && m === currentMonth - 1 && availableYears.includes(currentYear);
    if (isAnchor && forecastDisplay === 'band') {
      const v = yearMonthMap[currentYear][m];
      row.fBand = [v, v];
    }

    const f = fcByMonth.get(m);
    if (f) {
      // 色带用 [下界, 上界] 的区间 Area；只显示点时不给 band
      row.fBand = forecastDisplay === 'band' ? [f.lo, f.hi] : null;
      // 超过有回测支撑的距离就只留色带 —— 那些月份没人验证过，不该给一个具体数字
      row.fPoint = f.horizon <= TRUSTED_HORIZON ? f.point : null;
      row.__fc = f;
    } else if (!isAnchor) {
      row.fBand = null;
      row.fPoint = null;
    } else {
      row.fPoint = null;
    }
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

      {hasForecast && (
        <div className="text-xs text-muted-foreground">
          {forecastDisplay === 'band' ? '色带与空心点' : '空心点'}：{currentYear} 年剩余月份的预计聊天量
          {forecast?.confidence && <>（置信度 <span className="text-foreground">{CONFIDENCE_LABEL[forecast.confidence] ?? forecast.confidence}</span>）</>}
          ，{TRUSTED_HORIZON} 个月以后只给范围、不给具体数字
        </div>
      )}

      <div className="h-[260px] w-full">
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="name" fontSize={12} tickLine={false} axisLine={false} />
            <YAxis fontSize={12} tickLine={false} axisLine={false} />
            <Tooltip
              contentStyle={{ borderRadius: '8px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null;
                const row: any = payload[0]?.payload ?? {};
                const f = row.__fc as ForecastResult['points'][number] | undefined;
                // 真实值只列出来自年份线和平均线的项，预测另起一段单独说明
                const real = payload.filter(
                  (e: any) => e.dataKey !== 'fBand' && e.dataKey !== 'fPoint' && e.value != null
                );
                return (
                  <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-xl">
                    <div className="mb-1 font-semibold text-foreground">{label}</div>
                    {real.map((e: any) => (
                      <div key={e.dataKey} className="flex items-center gap-2">
                        <span className="inline-block h-2 w-2 rounded-full" style={{ background: e.color }} />
                        <span className="text-muted-foreground">{e.name}</span>
                        <span className="ml-auto font-medium text-foreground">
                          {Number(e.value).toLocaleString()}
                        </span>
                      </div>
                    ))}
                    {f && (
                      <div className="mt-1.5 border-t border-border pt-1.5">
                        {/* 明确写「预测」两个字 —— 光给月份和数字会被当成已经发生的事 */}
                        <div className="font-medium text-foreground">
                          预测：约 {f.point.toLocaleString()} 条
                          {f.horizon > TRUSTED_HORIZON && '（仅范围）'}
                        </div>
                        <div className="text-muted-foreground">
                          80% 区间：{f.lo.toLocaleString()}～{f.hi.toLocaleString()} 条
                        </div>
                        <div className="text-muted-foreground">
                          距当前：{f.horizon} 个月
                          {forecast?.confidence && ` · 置信度${CONFIDENCE_LABEL[forecast.confidence] ?? ''}`}
                        </div>
                        {f.has_actual && (
                          <div className="text-muted-foreground">
                            截至今天已 {f.actual.toLocaleString()} 条
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              }}
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

            {/* 预测：半透明色带 = 80% 区间。线型留给「基准/平均」，这里不用虚线，
                免得和上面那条「历史月均」撞语义 */}
            {hasForecast && forecastDisplay === 'band' && (
              <Area
                type="monotone"
                dataKey="fBand"
                stroke="none"
                fill={yearColor(currentYear)}
                fillOpacity={0.16}
                isAnimationActive={false}
                activeDot={false}
                legendType="none"
                name="预测区间"
                connectNulls={false}
              />
            )}
            {/* 空心点：只在有回测支撑的距离内画 */}
            {hasForecast && (
              <Line
                type="monotone"
                dataKey="fPoint"
                stroke={yearColor(currentYear)}
                strokeWidth={0}
                dot={{ fill: 'transparent', stroke: yearColor(currentYear), strokeWidth: 2, r: 4 }}
                activeDot={{ fill: 'transparent', stroke: yearColor(currentYear), strokeWidth: 2, r: 6 }}
                isAnimationActive={false}
                name="预计"
                connectNulls={false}
              />
            )}
          </ComposedChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
