import { useEffect, useRef, useState } from 'react';
import { init, use as registerCharts, type EChartsCoreOption } from 'echarts/core';
import { LineChart, ScatterChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, DataZoomComponent, AriaComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { MonitoringRange, MonitoringTrendSeries } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import type { SelectedChannel } from './channel-picker';
import { plotBaseline, plotObservations } from './trend-points';
import { compatibility, count, formatNS, MAX_NS, MIN_NS, parseNS, shiftRange, qualityNames } from './format';
import { observationTime, observationKind, observationQuality, observationRevoked, plotCoefficient, calculationStatusNames, type DetailRow } from './detail-format';
registerCharts([LineChart, ScatterChart, GridComponent, TooltipComponent, DataZoomComponent, AriaComponent, CanvasRenderer]);
const symbols = ['circle', 'rect', 'triangle', 'diamond', 'pin', 'roundRect'];
const marks = ['●', '■', '▲', '◆', '⬟', '▰'];

export default function TrendChart({ channels, series, range, timezone, onWindow, onWidth }: { channels: SelectedChannel[]; series: MonitoringTrendSeries[]; range: MonitoringRange; timezone: string; onWindow: (range: MonitoringRange) => void; onWidth: (width: number) => void }) {
  const grouped = new Map<string, SelectedChannel[]>();
  for (const channel of channels) { const key = compatibility(channel); grouped.set(key, [...(grouped.get(key) ?? []), channel]); }
  const groups = Array.from(grouped.values());
  const [groupIndex, setGroupIndex] = useState(0), [page, setPage] = useState(0), [hidden, setHidden] = useState<string[]>([]);
  const group = groups[Math.min(groupIndex, groups.length - 1)] ?? [], visible = group.slice(page * 6, (page + 1) * 6);
  const plotted = series.filter(s => visible.some(c => c.id === s.channel_id));
  return <section className="min-w-0 space-y-3" aria-label="趋势图"><div className="flex flex-wrap gap-2" aria-label="兼容通道分组">{groups.length === 1 ? <p className="self-center text-xs text-muted-foreground">{group[0]?.metric_name} · {group[0]?.unit || '枚举'} · {group.length > 1 ? `${group.length} 个通道同图比较` : '历史趋势'}</p> : groups.map((g, i) => <Button key={compatibility(g[0]!)} className="h-auto min-h-8 max-w-full whitespace-normal break-all text-left" size="sm" variant={i === groupIndex ? 'secondary' : 'outline'} aria-pressed={i === groupIndex} onClick={() => { setGroupIndex(i); setPage(0); }}>{g[0]!.metric_name} · {g[0]!.unit || '枚举'} · 第 {i + 1} 组（{g.length} 个通道{g.length > 1 ? '同图' : ''}）</Button>)}</div>{groups.length > 1 && group[0] && <p className="text-xs text-muted-foreground">{group[0].observation_direction || '观测方向未填写'} · {group[0].reference_basis || '参考基准未填写'} · {group[0].timestamp_meaning === 'observation_time' ? '观测时刻' : '区间结束时刻'}{groups.length > 1 && '；不同指标、单位、方向或基准分开显示。'}</p>}<div className="flex flex-wrap gap-2" aria-label="图例开关">{visible.map((c, i) => <Button key={c.id} className="h-auto min-h-8 max-w-full whitespace-normal text-left" size="sm" variant={hidden.includes(c.id) ? 'outline' : 'secondary'} aria-pressed={!hidden.includes(c.id)} onClick={() => setHidden(h => h.includes(c.id) ? h.filter(id => id !== c.id) : [...h, c.id])}><span aria-hidden style={{ color: `var(--chart-${(page * 6 + i) % 5 + 1})` }}>{marks[i]}</span><span className="min-w-0 break-all">{c.owner_name} · {c.name}</span></Button>)}</div><CanvasPlot colorOffset={page * 6} channels={visible} series={plotted.filter(s => !hidden.includes(s.channel_id))} range={range} timezone={timezone} onWindow={onWindow} onWidth={onWidth} /><details className="text-xs text-muted-foreground"><summary className="cursor-pointer">图表说明与样本信息</summary><p className="my-2">兼容通道共用时间轴与纵轴；原始观测与有效派生结果可连线，空值、已知断点及摘要仅显示实际观测点，不补零。悬停查看精确原值，明细页可键盘读取等价数据。按住 Ctrl 滚轮缩放，横向拖动平移；也可使用上方时间按钮。</p><div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">{plotted.map(s => <span key={s.channel_id}>{channels.find(c => c.id === s.channel_id)?.name}：{s.summary ? `摘要 · ${s.resolution}` : '原始点'} · 时间块样本 {count(s.sample_count)}</span>)}</div></details>{group.length > 6 && <div className="flex items-center gap-3"><Button variant="outline" disabled={page === 0} onClick={() => setPage(page - 1)}>上一组曲线</Button><span className="text-sm">每图最多 6 条，第 {page + 1} / {Math.ceil(group.length / 6)} 页</span><Button variant="outline" disabled={(page + 1) * 6 >= group.length} onClick={() => setPage(page + 1)}>下一组曲线</Button></div>}</section>;
}
function CanvasPlot({ colorOffset, channels, series, range, timezone, onWindow, onWidth }: { colorOffset: number; channels: SelectedChannel[]; series: MonitoringTrendSeries[]; range: MonitoringRange; timezone: string; onWindow: (range: MonitoringRange) => void; onWidth: (width: number) => void }) {
  const element = useRef<HTMLDivElement>(null), callback = useRef({ onWindow, onWidth });
  useEffect(() => { callback.current = { onWindow, onWidth }; }, [onWindow, onWidth]);
  const start = range.from ? parseNS(range.from) : MIN_NS, end = range.to ? parseNS(range.to) : MAX_NS;
  const numeric = series.flatMap(s => s.buckets.flatMap(b => b.points)).filter(p => observationKind(p) !== 'enum' && p.value !== null);
  const first = numeric[0], precision = Math.max(12, ...numeric.map(p => (p.value?.split('.')[1] ?? '').length));
  const baseline = plotBaseline(first, precision), isEnum = channels[0]?.value_kind === 'enum';
  useEffect(() => {
    const node = element.current;
    if (!node) return;
    const chart = init(node, undefined, { renderer: 'canvas', width: Math.max(160, node.clientWidth), height: Math.max(288, node.clientHeight) });
    const render = () => {
      const styles = getComputedStyle(node), foreground = styles.color;
      const colors = Array.from({ length: 6 }, (_, i) => styles.getPropertyValue(`--chart-${(colorOffset + i) % 5 + 1}`).trim());
      const stateValues = [...new Set(series.flatMap(s => s.buckets.flatMap(b => [...b.states.map(v => v.value), ...b.points.filter(v => observationKind(v) === 'enum').map(v => v.value)])))].sort();
      const axisDate = new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
      const option: EChartsCoreOption = {
        animation: false, backgroundColor: 'transparent', textStyle: { color: foreground, fontFamily: styles.fontFamily },
        aria: { enabled: true, label: { description: `监测趋势，${channels.length} 条通道。图下明细提供精确原值和时间。` } },
        grid: { top: 24, left: isEnum ? 100 : 72, right: 24, bottom: 56, containLabel: false },
        xAxis: { type: 'value', min: 0, max: Number(end - start) / 1e9, splitNumber: node.clientWidth < 600 ? 2 : 5, axisLabel: { color: foreground, hideOverlap: true, formatter: (v: number) => axisDate.format(new Date(Number(start / 1000000n) + v * 1000)) }, splitLine: { show: false } },
        yAxis: isEnum ? { type: 'category', data: stateValues, axisLabel: { color: foreground, width: 85, overflow: 'truncate' } } : { type: 'value', scale: true, name: baseline ? `相对基准变化（${channels[0]?.unit ?? ''}）` : channels[0]?.unit ?? '', nameTextStyle: { color: foreground }, axisLabel: { color: foreground }, splitLine: { lineStyle: { color: foreground, opacity: .12 } } },
        tooltip: { trigger: 'item', renderMode: 'richText', confine: true, formatter: (p: unknown) => p && typeof p === 'object' && 'name' in p ? String(p.name) : '' },
        dataZoom: [{ type: 'inside', filterMode: 'none', zoomOnMouseWheel: 'ctrl', moveOnMouseWheel: false, moveOnMouseMove: false, throttle: 150 }],
        series: series.map(s => {
          const index = channels.findIndex(c => c.id === s.channel_id), c = channels[index];
          const observation = (p: DetailRow | null) => p?.value != null ? ({ value: [Number(parseNS(observationTime(p)!) - start) / 1e9, observationKind(p) === 'enum' ? p.value : Number(plotCoefficient(p.value, precision) - baseline) / 10 ** precision], name: `${c?.owner_name} · ${c?.name}\n${observationTime(p)}\n原值 ${p.value} ${c?.unit ?? ''}\n质量 ${qualityNames[observationQuality(p)]}${observationRevoked(p) ? ' · 已撤销' : ''}${p.source_kind === 'derived' ? ` · ${calculationStatusNames[p.calculation_status]}` : ''}` }) : ({ value: [null, null], name: '' });
          const data = isEnum ? s.buckets.flatMap(b => b.states.length ? b.states.map(v => ({ value: [Number(parseNS(v.first_at) - start) / 1e9, v.value], name: `${c?.owner_name} · ${c?.name}\n状态：${v.value}\n样本 ${v.count} · 占比 ${v.sample_proportion}\n首次 ${v.first_at}\n末次 ${v.last_at}` })) : b.points.map(observation)) : plotObservations(s).map(observation);
          return { type: isEnum ? 'scatter' : 'line', connectNulls: false, showSymbol: true, lineStyle: { width: 1.5, color: colors[index % 6] }, name: `${c?.owner_name} · ${c?.name}`, symbol: symbols[index % 6], symbolSize: s.summary ? 6 : 3, itemStyle: { color: colors[index % 6] }, data, progressive: 1000, clip: true };
        }),
      };
      chart.setOption(option, true);
    };
    render();
    let timer: ReturnType<typeof setTimeout>;
    const observer = new ResizeObserver(() => { if (!node.clientWidth || !node.clientHeight) return; chart.resize({ width: node.clientWidth, height: node.clientHeight }); render(); clearTimeout(timer); timer = setTimeout(() => callback.current.onWidth(Math.max(160, Math.min(4096, Math.round(node.clientWidth)))), 150); });
    observer.observe(node);
    const theme = new MutationObserver(render); theme.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] });
    chart.on('datazoom', (event: unknown) => {
      if (!event || typeof event !== 'object') return;
      const value = 'batch' in event && Array.isArray(event.batch) ? event.batch[0] as unknown : event;
      if (!value || typeof value !== 'object' || !('start' in value) || !('end' in value) || typeof value.start !== 'number' || typeof value.end !== 'number') return;
      const duration = end - start, from = start + duration * BigInt(Math.round(value.start * 10000)) / 1000000n, to = start + duration * BigInt(Math.round(value.end * 10000)) / 1000000n;
      if (to > from) callback.current.onWindow({ from: formatNS(from), to: to === MAX_NS ? null : formatNS(to) });
    });
    let drag: { x: number; y: number; pointer: number } | undefined;
    const down = (e: PointerEvent) => { if (e.button === 0) drag = { x: e.clientX, y: e.clientY, pointer: e.pointerId }; };
    const up = (e: PointerEvent) => { const d = drag; drag = undefined; if (!d || d.pointer !== e.pointerId || Math.abs(e.clientX - d.x) < 8 || Math.abs(e.clientX - d.x) < Math.abs(e.clientY - d.y)) return; const shift = (end - start) * BigInt(Math.round((d.x - e.clientX) * 1000)) / BigInt(Math.max(1, node.clientWidth - 100) * 1000); callback.current.onWindow(shiftRange(range, shift)); };
    node.addEventListener('pointerdown', down); window.addEventListener('pointerup', up);
    return () => { clearTimeout(timer); observer.disconnect(); theme.disconnect(); node.removeEventListener('pointerdown', down); window.removeEventListener('pointerup', up); chart.dispose(); };
  }, [baseline, channels, colorOffset, end, isEnum, precision, range, series, start, timezone]);
  return <div className="min-w-0 space-y-2">{!isEnum && baseline !== 0n && <p className="break-all text-xs text-muted-foreground">纵轴基准：{first?.value ?? '0'} {channels[0]?.unit}；坐标近似，提示与明细保留精确原值。</p>}<div ref={element} className="h-72 min-w-0 touch-pan-y rounded-lg border bg-card sm:h-80" role="img" aria-label="监测趋势，使用时间按钮缩放和平移，切换明细读取精确原值" /></div>;
}
