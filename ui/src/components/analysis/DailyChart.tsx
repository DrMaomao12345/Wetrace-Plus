import { useState } from 'react';
import { AreaChart, Area, ReferenceLine, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import type { DailyStat } from '@/api';
import { formatNumber } from '@/lib/utils';

interface Props {
  data: DailyStat[];
}

export function DailyChart({ data }: Props) {
  const [showAvg, setShowAvg] = useState(true);

  // 平均：跨"有数据的天数"取均值（更能反映活跃日均，比把所有日历天分母更有意义）
  const total = (data || []).reduce((s, d) => s + d.count, 0);
  const days = data?.length || 0;
  const avg = days > 0 ? Math.round(total / days) : 0;
  const max = (data || []).reduce((m, d) => d.count > m ? d.count : m, 0);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3 flex-wrap text-xs">
        <div className="flex items-center gap-4 text-muted-foreground">
          <span>活跃天数 <span className="font-bold text-foreground">{days}</span></span>
          <span>日均 <span className="font-bold text-foreground">{formatNumber(avg)}</span></span>
          <span>最高单日 <span className="font-bold text-foreground">{formatNumber(max)}</span></span>
        </div>
        <label className="flex items-center gap-1 cursor-pointer select-none text-muted-foreground">
          <input
            type="checkbox"
            checked={showAvg}
            onChange={(e) => setShowAvg(e.target.checked)}
            className="accent-primary"
          />
          显示日均参考线
        </label>
      </div>

      <div className="h-[300px] w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id="colorCount" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#ec4899" stopOpacity={0.1} />
                <stop offset="95%" stopColor="#ec4899" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis
              dataKey="date"
              fontSize={12}
              tickLine={false}
              axisLine={false}
              minTickGap={30}
            />
            <YAxis
              fontSize={12}
              tickLine={false}
              axisLine={false}
            />
            <Tooltip
              contentStyle={{ borderRadius: '8px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
            />
            {showAvg && avg > 0 && (
              <ReferenceLine
                y={avg}
                stroke="#6366f1"
                strokeWidth={2}
                strokeDasharray="6 4"
                label={{ value: `日均 ${formatNumber(avg)}`, position: "right", fill: "#6366f1", fontSize: 11 }}
              />
            )}
            <Area
              type="monotone"
              dataKey="count"
              stroke="#ec4899"
              fillOpacity={1}
              fill="url(#colorCount)"
              strokeWidth={2}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
