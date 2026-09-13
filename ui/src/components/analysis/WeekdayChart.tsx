import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts';
import type { WeekdayStat } from '@/api';

interface Props {
  data: WeekdayStat[];
}

// 后端给的是 1=周一 … 7=周日，0 位留空（用 0 基数组会让周日变成 undefined）
const WEEKDAY_NAMES = ["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"];

export function WeekdayChart({ data }: Props) {
  const chartData = [...data]
    .sort((a, b) => a.weekday - b.weekday)
    .map(d => ({
      name: WEEKDAY_NAMES[d.weekday] || `Day ${d.weekday}`,
      count: d.count,
      weekday: d.weekday,
    }));

  return (
    <div className="h-[280px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" vertical={false} />
          <XAxis dataKey="name" fontSize={12} tickLine={false} axisLine={false} />
          <YAxis fontSize={12} tickLine={false} axisLine={false} />
          <Tooltip
            formatter={(value: any) => [value, '消息数']}
            contentStyle={{ borderRadius: '8px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
          />
          <Bar dataKey="count" radius={[4, 4, 0, 0]}>
            {chartData.map((d, i) => (
              <Cell key={i} fill={d.weekday >= 6 ? '#ec4899' : '#6366f1'} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
