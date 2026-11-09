import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Line, LineChart, Pie, PieChart, XAxis, YAxis } from 'recharts';
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, type ChartConfig } from './chart';
import { EmptyState, ErrorState, LoadingState } from '../patterns/feedback';

export type ChartDatum = Record<string, string | number>;
export type ChartSeries = { key: string; label: string; color?: string };
type Props = {
  kind: 'line' | 'area' | 'bar' | 'pie' | 'donut'; title: string; description?: string;
  data: ChartDatum[]; categoryKey: string; series: ChartSeries[];
  loading?: boolean; error?: string; onRetry?: () => void;
  formatValue?: (value: number) => string; formatCategory?: (value: string) => string;
};

export function SeriesChart({ kind, title, description, data, categoryKey, series, loading, error, onRetry, formatValue = (value) => value.toLocaleString('zh-CN'), formatCategory }: Props) {
  const config: ChartConfig = Object.fromEntries(series.map((item, index) => [item.key, { label: item.label, color: item.color ?? `var(--chart-${index % 5 + 1})` }]));
  const radial = kind === 'pie' || kind === 'donut';
  const firstSeries = series[0];
  const pieData = data.map((row, index) => ({ ...row, fill: `var(--chart-${index % 5 + 1})` }));
  const tooltip = <ChartTooltip content={<ChartTooltipContent formatter={(value, name) => <div className="flex w-full justify-between gap-4"><span className="text-muted-foreground">{config[String(name)]?.label ?? name}</span><span className="font-medium tabular-nums">{typeof value === 'number' ? formatValue(value) : String(value)}</span></div>} />} />;
  const axes = <><CartesianGrid vertical={false} /><XAxis dataKey={categoryKey} tickLine={false} axisLine={false} tickMargin={8} tickFormatter={formatCategory} /><YAxis width={48} tickLine={false} axisLine={false} tickFormatter={formatValue} /></>;
  const legend = <ChartLegend content={<ChartLegendContent />} />;
  let content;
  if (kind === 'line') content = <LineChart data={data} accessibilityLayer>{axes}{tooltip}{legend}{series.map((item, index) => <Line key={item.key} dataKey={item.key} type="monotone" stroke={`var(--color-${item.key})`} strokeWidth={2} strokeDasharray={index ? '5 3' : undefined} dot={{ r: 3 }} isAnimationActive={false} />)}</LineChart>;
  else if (kind === 'area') content = <AreaChart data={data} accessibilityLayer>{axes}{tooltip}{legend}{series.map((item) => <Area key={item.key} dataKey={item.key} type="monotone" stroke={`var(--color-${item.key})`} fill={`var(--color-${item.key})`} fillOpacity={0.16} isAnimationActive={false} />)}</AreaChart>;
  else if (kind === 'bar') content = <BarChart data={data} accessibilityLayer>{axes}{tooltip}{legend}{series.map((item) => <Bar key={item.key} dataKey={item.key} fill={`var(--color-${item.key})`} radius={4} isAnimationActive={false} />)}</BarChart>;
  else content = <PieChart accessibilityLayer>{tooltip}<Pie data={pieData} dataKey={firstSeries?.key ?? 'value'} nameKey={categoryKey} innerRadius={kind === 'donut' ? 55 : 0} outerRadius={90} label={({ name }) => String(name)} isAnimationActive={false}>{pieData.map((row, index) => <Cell key={index} fill={row.fill} />)}</Pie></PieChart>;
  return <figure className="min-w-0 space-y-3" aria-label={title}>
    <figcaption><h3 className="text-sm font-medium">{title}</h3>{description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}</figcaption>
    {loading ? <LoadingState /> : error ? <ErrorState message={error} onRetry={onRetry} /> : !data.length || !series.length ? <EmptyState title="暂无图表数据" /> : <>
      <ChartContainer config={config} className="h-72 w-full aspect-auto">{content}</ChartContainer>
      {radial && <ul className="flex flex-wrap justify-center gap-4 text-xs">{data.map((row, index) => <li key={index} className="flex items-center gap-1.5"><span className="size-2 rounded-sm" style={{ backgroundColor: `var(--chart-${index % 5 + 1})` }} />{String(row[categoryKey] ?? '')}：{formatValue(Number(row[firstSeries?.key ?? 'value'] ?? 0))}</li>)}</ul>}
      <table className="sr-only"><caption>{title}数据</caption><thead><tr><th scope="col">分类</th>{series.map((item) => <th scope="col" key={item.key}>{item.label}</th>)}</tr></thead><tbody>{data.map((row, index) => <tr key={index}><th scope="row">{String(row[categoryKey] ?? '')}</th>{series.map((item) => <td key={item.key}>{typeof row[item.key] === 'number' ? formatValue(row[item.key] as number) : String(row[item.key] ?? '')}</td>)}</tr>)}</tbody></table>
    </>}
  </figure>;
}
