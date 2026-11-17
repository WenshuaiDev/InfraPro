import { useEffect, useMemo, useRef, useState } from 'react';
import { init, use as registerCharts, type EChartsCoreOption } from 'echarts/core';
import { LineChart, ScatterChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, DataZoomComponent, AriaComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import { X } from 'lucide-react';
import type { MonitoringRange, MonitoringTrendSeries } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import type { SelectedChannel } from './channel-picker';
import { count, formatNS, MAX_NS, shiftRange } from './format';
import { buildTrendModel, channelAppearance, type TrendScale } from './trend-model';

registerCharts([LineChart, ScatterChart, GridComponent, TooltipComponent, DataZoomComponent, AriaComponent, CanvasRenderer]);
type Props = {
  channels: SelectedChannel[]; series: MonitoringTrendSeries[]; range: MonitoringRange; timezone: string;
  onWindow: (range: MonitoringRange) => void; onWidth: (width: number) => void;
  scale?: TrendScale; hidden?: string[]; onToggle?: (id: string) => void; onRemove?: (id: string) => void;
};

export default function TrendChart({ channels, series, range, timezone, onWindow, onWidth, scale = 'actual', hidden: controlledHidden, onToggle, onRemove }: Props) {
  const [localHidden, setLocalHidden] = useState<string[]>([]);
  const hidden = controlledHidden ?? localHidden;
  const model = useMemo(() => buildTrendModel(channels, series, range, scale), [channels, series, range, scale]);
  const toggle = (id: string) => onToggle ? onToggle(id) : setLocalHidden(value => value.includes(id) ? value.filter(item => item !== id) : [...value, id]);
  return <section className="min-w-0 space-y-3" aria-label="趋势图">
    <div className="flex max-h-40 flex-wrap gap-2 overflow-y-auto" aria-label="图例开关">{channels.map(channel => {
      const appearance = channelAppearance(channel), muted = hidden.includes(channel.id), empty = !model.lines.find(line => line.id === channel.id)?.data.some(point => point.value[1] !== null);
      return <div key={channel.id} className={`flex max-w-full items-center rounded-md border ${muted ? 'opacity-50' : 'bg-muted/30'}`}>
        <Button className="h-auto min-h-10 min-w-0 whitespace-normal text-left" size="sm" variant="ghost" aria-label={`${muted ? '显示' : '隐藏'} ${channel.owner_name} ${channel.name}`} aria-pressed={!muted} onClick={() => toggle(channel.id)}>
          <span aria-hidden className="w-5 shrink-0 text-center" style={{ color: appearance.color, borderBottom: `2px ${appearance.lineType} currentColor` }}>{appearance.mark}</span>
          <span className="min-w-0 break-words"><span className="block">{channel.name}</span><span className="block text-[11px] font-normal text-muted-foreground">{channel.owner_name} · {channel.unit || (channel.value_kind === 'enum' ? '状态' : '无单位')}{empty && ' · 当前窗口无样本'}</span></span>
        </Button>
        {onRemove && <Button size="icon" variant="ghost" className="mr-1 size-7 shrink-0" aria-label={`移除 ${channel.owner_name} ${channel.name}`} onClick={() => onRemove(channel.id)}><X className="size-3.5" /></Button>}
      </div>;
    })}</div>
    <CanvasPlot model={model} hidden={hidden} range={range} timezone={timezone} onWindow={onWindow} onWidth={onWidth} />
    <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground"><span>已选 {channels.length} 个通道 · 显示 {channels.filter(channel => !hidden.includes(channel.id)).length} 个通道</span><span>{scale === 'relative' ? '各通道按当前窗口样本幅度展示' : '同一时间轴 · 同单位共用纵轴'}{channels.some(channel => channel.value_kind === 'enum') && ' · 状态通道以离散点展示'}</span></div>
    <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">图表说明与样本信息</summary><p className="my-2">所选通道同时显示；不同单位分别标尺，方向与基准保留各自定义。直接观测用实线，派生结果用虚线。空值、已知断点及摘要仅显示实际观测点，不补零。悬停查看精确原值，明细页可键盘读取等价数据。按住 Ctrl 滚轮缩放，横向拖动平移；也可使用时间按钮。</p>{scale === 'relative' && <p className="my-2">相对幅度以各通道当前窗口的可见样本最小值、最大值映射到 0–100%，恒定值显示为 50%；摘要采样可能影响幅度，仅用于比较变化形态。提示和明细始终保留原值。</p>}<div className="flex flex-wrap gap-x-4 gap-y-1">{channels.map(channel => { const source = series.find(item => item.channel_id === channel.id); return <span key={channel.id}>{channel.name}：{source ? `${source.summary ? `摘要 · ${source.resolution}` : '原始点'} · 时间块样本 ${count(source.sample_count)}` : '当前窗口无样本'}</span>; })}</div></details>
  </section>;
}

function CanvasPlot({ model, hidden, range, timezone, onWindow, onWidth }: { model: ReturnType<typeof buildTrendModel>; hidden: string[]; range: MonitoringRange; timezone: string; onWindow: Props['onWindow']; onWidth: Props['onWidth'] }) {
  const element = useRef<HTMLDivElement>(null), callback = useRef({ onWindow, onWidth });
  useEffect(() => { callback.current = { onWindow, onWidth }; }, [onWindow, onWidth]);
  const { start, end } = model;
  useEffect(() => {
    const node = element.current;
    if (!node) return;
    const left = model.axes[0]?.kind === 'category' ? 100 : 72, right = 24 + Math.max(0, model.axes.length - 1) * 78;
    const lines = model.lines.filter(line => !hidden.includes(line.id));
    const description = `监测趋势，${lines.length} 条通道，共用一张趋势图。明细提供精确原值和时间。`;
    const chart = init(node, undefined, { renderer: 'canvas', width: Math.max(260, node.clientWidth), height: Math.max(288, node.clientHeight) });
    const render = () => {
      const styles = getComputedStyle(node), foreground = styles.color;
      const axisDate = new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
      const option: EChartsCoreOption = {
        animation: false, backgroundColor: 'transparent', textStyle: { color: foreground, fontFamily: styles.fontFamily },
        aria: { enabled: true, label: { description } },
        grid: { top: 36, left, right, bottom: 48, containLabel: false },
        xAxis: { type: 'value', min: 0, max: Number(end - start) / 1e9, splitNumber: node.clientWidth - left - right < 500 ? 2 : 5, axisLabel: { color: foreground, hideOverlap: true, formatter: (value: number) => axisDate.format(new Date(Number(start / 1000000n) + value * 1000)) }, splitLine: { show: false } },
        yAxis: model.axes.map((axis, index) => ({ type: axis.kind, name: axis.name, nameTextStyle: { color: foreground, fontSize: 11 }, position: index === 0 ? 'left' : 'right', offset: index > 0 ? (index - 1) * 78 : 0, scale: true, ...(axis.kind === 'category' ? { data: axis.states } : axis.key === 'relative' ? { min: 0, max: 100 } : {}), axisLabel: { color: foreground, width: index === 0 && axis.kind === 'category' ? 85 : 68, overflow: 'truncate', hideOverlap: true }, splitLine: { show: index === 0, lineStyle: { color: foreground, opacity: .12 } } })),
        tooltip: { trigger: 'item', renderMode: 'richText', confine: true, formatter: (point: unknown) => point && typeof point === 'object' && 'name' in point ? String(point.name) : '' },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none', zoomOnMouseWheel: 'ctrl', moveOnMouseWheel: false, moveOnMouseMove: false, throttle: 150 }],
        series: lines.map(line => {
          const appearance = channelAppearance(line.channel), color = styles.getPropertyValue(`--chart-${appearance.colorIndex}`).trim();
          return { id: line.id, name: line.name, yAxisIndex: line.axisIndex, type: line.channel.value_kind === 'enum' ? 'scatter' : 'line', connectNulls: false, showSymbol: true, lineStyle: { width: 1.8, color, type: appearance.lineType }, symbol: appearance.symbol, symbolSize: line.summary || line.channel.value_kind === 'enum' ? 6 : 3, itemStyle: { color }, data: line.data, progressive: 1000, clip: true };
        }),
      };
      chart.setOption(option, true);
      node.dataset.seriesCount = String(lines.length);
      node.dataset.channelIds = lines.map(line => line.id).join(',');
    };
    render();
    let timer: ReturnType<typeof setTimeout>;
    const observer = new ResizeObserver(() => {
      if (!node.clientWidth || !node.clientHeight) return;
      chart.resize({ width: node.clientWidth, height: node.clientHeight }); render(); clearTimeout(timer);
      timer = setTimeout(() => callback.current.onWidth(Math.max(160, Math.min(4096, Math.round(node.clientWidth - left - right)))), 150);
    });
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
    const down = (event: PointerEvent) => { if (event.button === 0) drag = { x: event.clientX, y: event.clientY, pointer: event.pointerId }; };
    const up = (event: PointerEvent) => {
      const previous = drag; drag = undefined;
      if (!previous || previous.pointer !== event.pointerId || Math.abs(event.clientX - previous.x) < 8 || Math.abs(event.clientX - previous.x) < Math.abs(event.clientY - previous.y)) return;
      const shift = (end - start) * BigInt(Math.round((previous.x - event.clientX) * 1000)) / BigInt(Math.max(1, node.clientWidth - left - right) * 1000);
      callback.current.onWindow(shiftRange(range, shift));
    };
    node.addEventListener('pointerdown', down); window.addEventListener('pointerup', up);
    return () => { clearTimeout(timer); observer.disconnect(); theme.disconnect(); node.removeEventListener('pointerdown', down); window.removeEventListener('pointerup', up); chart.dispose(); };
  }, [model, hidden, end, range, start, timezone]);
  return <div className="min-w-0 space-y-2">{model.axes.filter(axis => axis.baseline !== 0n).map(axis => <p key={axis.key} className="break-all text-xs text-muted-foreground">{axis.unit} 纵轴基准：{axis.baselineValue}；坐标近似，提示与明细保留精确原值。</p>)}<div className="overflow-x-auto rounded-lg border bg-card"><div ref={element} style={{ minWidth: Math.max(260, 280 + (model.axes.length - 1) * 78) }} className="h-64 touch-pan-y sm:h-72 xl:h-80" role="img" aria-label={`监测趋势，${model.lines.filter(line => !hidden.includes(line.id)).length} 条通道，共用一张趋势图。明细提供精确原值和时间。`} /></div></div>;
}
