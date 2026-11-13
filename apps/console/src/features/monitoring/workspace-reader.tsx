import { lazy, Suspense, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Activity, CalendarDays, ChartNoAxesCombined, ChevronLeft, ChevronRight, Download, Pause, Play, RefreshCw, Settings2 } from 'lucide-react';
import type { MonitoringAnalyticResult, MonitoringQuery, MonitoringRange } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useMonitoring } from './context';
import { Choice, MonitoringError } from './common';
import type { SelectedChannel } from './channel-picker';
import { AnalyticState, DetailTable } from './data-tables';
import { compatibility, MAX_NS, MIN_NS, parseNS, qualityNames, rangeLabel, shiftRange, zoomRange } from './format';
import { useAnalytic, useLiveLatest, useTrendTiles } from './query-hooks';
import { ExportForm } from './export-form';
import { displayTime, latestRange, observedAge, qualityOptions, selectedQualities } from './workspace-model';
import { MonitoringOverviewPanel } from './workspace-overview';
import { DerivedLatest } from './workspace-derived-latest';
import { MonitoringTimeRange } from './workspace-time-range';

const TrendChart = lazy(() => import('./trend-chart'));
function useComplete(result: MonitoringAnalyticResult | undefined, scope: string, readAt: number) {
  const [last, setLast] = useState<{ scope: string; result: MonitoringAnalyticResult; readAt: number }>();
  if (result?.state === 'ready' && (last?.scope !== scope || last.result !== result)) setLast({ scope, result, readAt });
  return result?.state === 'ready' ? { result, readAt } : last?.scope === scope ? last : undefined;
}

export function MonitoringReader({ channels, comparing = false }: { channels: SelectedChannel[]; comparing?: boolean }) {
  const m = useMonitoring(), id = useId();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 30_000); return () => clearInterval(timer); }, []);
  const [quality, setQuality] = useState('default'), [timezone, setTimezone] = useState('Asia/Shanghai'), [status, setStatus] = useState<'active' | 'history'>('active');
  const [live, setLive] = useState(true), [minutes, setMinutes] = useState(1440), [customRange, setCustomRange] = useState<MonitoringRange | null>(null), [generation, setGeneration] = useState(0);
  const [tab, setTab] = useState('trends'), [overview, setOverview] = useState(false), [timeOpen, setTimeOpen] = useState(false), [settings, setSettings] = useState(false), [exporting, setExporting] = useState(false);
  const ids = channels.map(c => c.id).sort();
  const selection = useQuery({ queryKey: [...m.prefix, 'selection', ids], queryFn: ({ signal }) => m.client.monitoringSelect(m.scope, { channel_ids: ids }, signal), enabled: m.available && m.canRead, staleTime: 120_000, retry: false });
  const latest = useLiveLatest(selection.data?.id, timezone, live);
  const latestScope = JSON.stringify([m.prefix, selection.data?.id, timezone]);
  const latestComplete = useComplete(latest.data, latestScope, latest.dataUpdatedAt);
  const latestReady = latestComplete?.result;
  const watermark = latestReady?.snapshot.watermark;
  const seen = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (watermark && seen.current && watermark !== seen.current && !customRange) setGeneration(value => value + 1);
    seen.current = watermark;
  }, [watermark, customRange]);
  const qualities = useMemo(() => selectedQualities(quality), [quality]);
  const base: MonitoringQuery | undefined = selection.data ? { selection_id: selection.data.id, all_history: true, qualities, timezone, status } : undefined;
  const full = useAnalytic('overview', base, generation);
  const fullReady = useComplete(full.data, JSON.stringify([m.prefix, base]), full.dataUpdatedAt)?.result;
  const lastTime = fullReady?.overview?.latest;
  const range = customRange ?? (lastTime ? latestRange(lastTime, minutes) : null);
  const query = fullReady && base ? { ...base, snapshot_id: fullReady.snapshot.id, ...(range ? { view_range: range } : {}) } : undefined;
  const renewal = useQuery({ queryKey: [...m.prefix, 'snapshot-renewal', fullReady?.snapshot.id], enabled: !!fullReady && !!base && m.available, staleTime: 5 * 60_000, refetchInterval: 5 * 60_000, retry: false, queryFn: ({ signal }) => m.client.monitoringRenew(m.scope, fullReady!.snapshot.id, base!, signal) });
  const refresh = () => { void latest.refetch(); setGeneration(value => value + 1); };
  const follow = () => { setCustomRange(null); setGeneration(value => value + 1); };
  const changeRange = (next: MonitoringRange) => setCustomRange(next);
  const changed = !!customRange && !!watermark && !!fullReady && watermark !== fullReady.snapshot.watermark;
  const duration = range ? (range.to ? parseNS(range.to) : MAX_NS) - (range.from ? parseNS(range.from) : MIN_NS) : 0n;
  return <div className="min-w-0 space-y-4" aria-label={comparing ? '多通道比较工作区' : '当前通道数据'}>
    <MonitoringError error={selection.error} retry={() => void selection.refetch()} />
    <section className="min-w-0 rounded-lg border bg-card" aria-label="最新观测">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3"><div className="flex items-center gap-2"><Activity className="size-4 text-muted-foreground" aria-hidden /><h2 className="font-semibold">最新观测</h2><span className="text-xs text-muted-foreground">{comparing ? `${channels.length} 个通道` : '不随历史范围改变'}</span></div><div className="flex flex-wrap gap-1"><Button size="sm" variant="ghost" aria-pressed={live} onClick={() => setLive(value => !value)}>{live ? <Pause aria-hidden /> : <Play aria-hidden />}{live ? '自动更新 · 30 秒' : '开启自动更新'}</Button><Button size="sm" variant="ghost" aria-label="刷新监测数据" disabled={latest.isFetching || !selection.data} onClick={refresh}><RefreshCw aria-hidden />刷新</Button></div></div>
      <div className="p-4">{latestReady && latest.data && !['ready', 'pending'].includes(latest.data.state) && <AnalyticState result={latest.data} />}<MonitoringError error={latest.error} retry={() => void latest.refetch()} />{latest.error && latestReady && <p className="mb-3 text-xs text-muted-foreground">最新读取失败，保留上次完整观测；以下数值可能已过时。</p>}{!latestReady ? <AnalyticState result={latest.data} loading={selection.isPending || latest.isPending} /> : <><div className={comparing ? 'grid gap-3 md:grid-cols-2 2xl:grid-cols-3' : ''}>{channels.map(c => {
        const row = latestReady.channels?.find(row => row.channel_id === c.id);
        if (c.source_kind === 'derived') return <DerivedLatest key={c.id} channel={c} row={row} timezone={timezone} comparing={comparing} />;
        const last = row?.last?.source_kind === 'direct' ? row.last : undefined;
        const index = channels.filter(peer => compatibility(peer) === compatibility(c)).findIndex(peer => peer.id === c.id);
        return <article key={c.id} className={comparing ? 'min-w-0 rounded-md border p-3' : 'grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(16rem,1fr)]'}>
          <div className="min-w-0"><p className="mb-1 break-words text-xs text-muted-foreground">{comparing ? c.owner_name : `${c.facility_name} / ${c.owner_name}`}</p><h3 className="flex flex-wrap items-center gap-2 font-medium">{comparing && <span className="size-2 shrink-0 rounded-full" style={{ background: `var(--chart-${index % 5 + 1})` }} />}{c.name}<span className="text-xs font-normal text-muted-foreground">{c.code}</span></h3><p className="my-2 break-all font-semibold tabular-nums"><span className={comparing ? 'text-2xl' : 'text-3xl'}>{last ? last.value_kind === 'enum' ? last.value || '（空文本）' : last.value.replace(/(\.\d*?[1-9])0+$|\.0+$/, '$1') : '—'}</span><span className="ml-2 text-sm font-normal text-muted-foreground">{c.unit}</span></p></div>
          <div className="min-w-0 space-y-2 self-center text-sm">{last ? <><p className="text-muted-foreground">观测时间 <time className="ml-1 font-medium text-foreground tabular-nums" dateTime={last.observed_at}>{displayTime(last.observed_at, timezone)}</time></p><div className="flex flex-wrap items-center gap-2"><Badge variant={last.quality === 'bad' ? 'destructive' : 'outline'}>{qualityNames[last.quality]}</Badge><span className="text-xs text-muted-foreground">{observedAge(last.observed_at, now)}</span>{!qualities?.includes(last.quality) && <span className="text-xs text-muted-foreground">未纳入当前质量筛选</span>}</div></> : <p className="text-muted-foreground">暂无在用观测。无数据不表示值为零。</p>}
            <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">通道信息与精确原值</summary><div className="mt-2 space-y-1 break-all"><p>{c.metric_name} · {c.observation_direction || '方向未填'} · {c.reference_basis || '基准未填'}</p><p>{c.timestamp_meaning === 'interval_end' ? '区间结束时刻' : '观测时刻'} · {timezone}</p>{last && <><p>精确时间：{last.observed_at}</p><p>精确原值：{last.value} {c.unit}</p>{last.quality_reason && <p>质量说明：{last.quality_reason}</p>}</>}<p>可读范围：{c.read_ranges.map(rangeLabel).join('；')}</p></div></details>
          </div>
        </article>;
      })}</div><p className="mt-2 text-xs text-muted-foreground">读取于 {latestComplete?.readAt ? new Date(latestComplete.readAt).toLocaleTimeString('zh-CN', { timeZone: timezone }) : '—'} · {timezone}{latest.isFetching ? ' · 正在更新' : ''}</p></> }</div>
    </section>
    <section className="min-w-0 overflow-hidden rounded-lg border" aria-label="趋势与监测明细">
      <div className="flex flex-wrap items-center gap-2 border-b bg-muted/20 p-3"><div className="flex flex-wrap gap-1" aria-label="快捷时间范围">{[[60, '近 1 小时'], [360, '近 6 小时'], [1440, '近 24 小时']].map(([value, label]) => <Button key={value} size="sm" variant={!customRange && minutes === value ? 'secondary' : 'ghost'} aria-pressed={!customRange && minutes === value} disabled={!lastTime} onClick={() => { setMinutes(Number(value)); setCustomRange(null); setGeneration(g => g + 1); }}>{label}</Button>)}</div><Button id={`${id}-time`} size="sm" variant="outline" disabled={!range} onClick={() => setTimeOpen(true)}><CalendarDays aria-hidden />自定义时间</Button><Button size="sm" variant="ghost" disabled={!lastTime} onClick={follow}>回到最新</Button><Button id={`${id}-settings`} size="sm" variant="ghost" className="ml-auto" onClick={() => setSettings(true)}><Settings2 aria-hidden />查询设置</Button></div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2"><div className="flex gap-1" aria-label="监测展示方式">{[['trends', '趋势'], ['details', '明细']].map(([value, label]) => <Button key={value} variant={tab === value ? 'secondary' : 'ghost'} size="sm" aria-pressed={tab === value} onClick={() => setTab(value!)}>{label}</Button>)}</div><div className="flex flex-wrap gap-1"><Button id={`${id}-overview`} size="sm" variant="ghost" disabled={!query} onClick={() => setOverview(true)}><ChartNoAxesCombined aria-hidden />数据概况</Button>{m.canExport && <Button id={`${id}-export`} size="sm" variant="ghost" disabled={!query || !range || !!renewal.error} onClick={() => setExporting(true)}><Download aria-hidden />导出当前明细</Button>}</div></div>
      <div className="min-w-0 space-y-3 p-4">
        <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground"><span>{range ? `${displayTime(range.from, timezone, false)} — ${displayTime(range.to, timezone, false)}` : '等待当前通道数据'} · {customRange ? '历史范围' : '跟随最新覆盖'}</span><span>{qualityOptions.find(q => q.value === quality)?.label} · {timezone}{status === 'history' && ' · 含撤销历史'}</span></div>
        <MonitoringError error={full.error} retry={() => void full.refetch()} /><MonitoringError error={renewal.error} retry={() => void renewal.refetch()} />
        {fullReady && full.data && !['ready', 'pending'].includes(full.data.state) && <AnalyticState result={full.data} />}
        {full.error && fullReady && <p role="status" className="text-sm text-muted-foreground">当前读取失败，保留上次完整结果。</p>}
        {changed && <div role="status" className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted p-3 text-sm"><span>有新数据，最新值已更新；历史视窗保持不变。</span><Button size="sm" variant="outline" onClick={follow}>查看最新趋势</Button></div>}
        {!fullReady ? <AnalyticState result={full.data} loading={full.isPending} /> : !range ? <p className="py-12 text-center text-sm text-muted-foreground">所选通道暂无数据覆盖。</p> : query && (tab === 'trends' ? <WindowTrend key={`${query.snapshot_id}:${quality}:${timezone}:${status}`} query={query} channels={channels} range={range} timezone={timezone} onWindow={changeRange} /> : <DetailTable key={`${query.snapshot_id}:${rangeLabel(range)}`} query={query} channels={channels} />)}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2 text-xs text-muted-foreground"><span>{comparing ? '兼容通道同图比较，图例可显隐。' : '精确原值可在明细中核对。'}</span><div className="flex flex-wrap gap-1"><Button size="sm" variant="ghost" disabled={!range} onClick={() => range && changeRange(shiftRange(range, -duration))}><ChevronLeft aria-hidden />前一时段</Button><Button size="sm" variant="ghost" disabled={!range} onClick={() => range && changeRange(shiftRange(range, duration))}>后一时段<ChevronRight aria-hidden /></Button><Button size="sm" variant="ghost" disabled={!range} onClick={() => range && changeRange(zoomRange(range, 1n, 2n))}>放大</Button></div></div>
    </section>
    <ResponsivePanel open={settings} onOpenChange={setSettings} title="查询设置" description="影响趋势、明细和统计；最新观测始终展示最新在用记录。" fallbackFocusSelector={`[id="${id}-settings"]`}>
      <div className="space-y-5"><Choice label="参与查询的质量" value={quality} onChange={setQuality} options={qualityOptions} /><Choice label="展示与日历统计时区" value={timezone} onChange={setTimezone} options={['Asia/Shanghai', 'UTC', 'America/New_York', 'Europe/London', 'Asia/Tokyo'].map(value => ({ value, label: value }))} />{m.canHistory && <label className="flex min-h-11 items-center gap-3 text-sm"><Checkbox checked={status === 'history'} onCheckedChange={checked => setStatus(checked === true ? 'history' : 'active')} />包含撤销历史</label>}<Button onClick={() => setSettings(false)}>完成设置</Button></div>
    </ResponsivePanel>
    <ResponsivePanel open={timeOpen} onOpenChange={setTimeOpen} title="查阅时间范围" description="选择日期时间，也可输入纳秒精度的时间；查看历史不影响最新值。" fallbackFocusSelector={`[id="${id}-time"]`}>{timeOpen && range && <MonitoringTimeRange range={range} earliest={fullReady?.overview?.earliest} latest={lastTime} onChange={next => { changeRange(next); setTimeOpen(false); }} />}</ResponsivePanel>
    <ResponsivePanel open={overview} onOpenChange={setOverview} title="数据概况" description={`统计对象：${channels.length} 个所选通道。范围、质量口径与查阅视图一致。`} size="wide" fallbackFocusSelector={`[id="${id}-overview"]`}>{overview && query && fullReady && <MonitoringOverviewPanel query={query} channels={channels} full={fullReady} />}</ResponsivePanel>
    <ResponsivePanel open={exporting} onOpenChange={setExporting} title="创建后台导出" description="核对范围与质量后创建，完成后在任务中心下载。" fallbackFocusSelector={`[id="${id}-export"]`}>{exporting && query && <ExportForm query={query} channelCount={channels.length} />}</ResponsivePanel>
  </div>;
}

function WindowTrend({ query, channels, range, timezone, onWindow }: { query: MonitoringQuery; channels: SelectedChannel[]; range: MonitoringRange; timezone: string; onWindow: (range: MonitoringRange) => void }) {
  const [width, setWidth] = useState(960);
  const trend = useTrendTiles(query, range, width, channels.length);
  return <div className="min-h-80 min-w-0 space-y-3"><MonitoringError error={trend.error} retry={trend.retry} />{!trend.display && <AnalyticState result={trend.pending} loading={!trend.pending} />}{!trend.complete && trend.display && <p role="status" className="text-xs text-muted-foreground">{trend.error ? '读取失败' : '正在读取新范围'}，暂时保留上一完整窗口：{rangeLabel(trend.display.range)}</p>}{trend.display && <Suspense fallback={<p role="status">正在加载图表…</p>}><TrendChart channels={channels} series={trend.display.series} range={trend.display.range} timezone={timezone} onWindow={onWindow} onWidth={setWidth} /></Suspense>}</div>;
}
