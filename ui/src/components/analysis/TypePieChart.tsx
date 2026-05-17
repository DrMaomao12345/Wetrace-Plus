import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts';
import type { MessageTypeStat } from '@/api';
import { formatNumber } from '@/lib/utils';

interface Props {
  data: MessageTypeStat[];
}

const COLORS = [
  '#ec4899', '#6366f1', '#06b6d4', '#f59e0b', '#10b981',
  '#8b5cf6', '#ef4444', '#14b8a6', '#f97316', '#3b82f6',
];

const TYPE_NAMES: Record<number, string> = {
  1: '文本',
  3: '图片',
  34: '语音',
  42: '名片',
  43: '视频',
  47: '表情',
  48: '位置',
  49: '链接/文件',
  50: '通话',
  99: '其他',
  10000: '系统',
};

export function TypePieChart({ data }: Props) {
  const sorted = [...data].sort((a, b) => b.count - a.count);
  const total = sorted.reduce((s, d) => s + d.count, 0);

  const chartData = sorted.map((item, i) => ({
    name: TYPE_NAMES[item.type] || `类型 ${item.type}`,
    value: item.count,
    percent: total > 0 ? (item.count / total) * 100 : 0,
    color: COLORS[i % COLORS.length],
  }));

  if (chartData.length === 0) {
    return <div className="h-[250px] flex items-center justify-center text-muted-foreground text-sm">暂无数据</div>;
  }

  return (
    <div className="flex flex-col md:flex-row items-center gap-8">
      <div className="h-[250px] w-[250px] shrink-0">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={chartData}
              cx="50%"
              cy="50%"
              innerRadius={60}
              outerRadius={90}
              paddingAngle={3}
              dataKey="value"
            >
              {chartData.map((entry, i) => (
                <Cell key={i} fill={entry.color} />
              ))}
            </Pie>
            <Tooltip
              formatter={(value: any, _name, ctx: any) => {
                const p = ctx?.payload?.percent ?? 0;
                return [`${formatNumber(value)} (${p.toFixed(1)}%)`, ctx?.payload?.name];
              }}
              contentStyle={{ borderRadius: '8px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <div className="flex-1 grid grid-cols-2 gap-x-8 gap-y-3 w-full">
        {chartData.map((item) => (
          <div key={item.name} className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full shrink-0" style={{ backgroundColor: item.color }} />
            <span className="text-sm">{item.name}</span>
            <span className="text-sm font-bold ml-auto">{formatNumber(item.value)}</span>
            <span className="text-xs text-muted-foreground w-12 text-right">
              {item.percent.toFixed(1)}%
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
