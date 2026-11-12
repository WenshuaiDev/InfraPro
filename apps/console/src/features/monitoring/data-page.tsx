import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { PanelLeft, RefreshCw, Download } from 'lucide-react';
import type { MonitoringAnalyticResult, MonitoringOverview, MonitoringQuery, MonitoringRange, MonitoringSelection } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { useMonitoring } from './context';
import { Choice, Field, MonitoringError, MonitoringTabs } from './common';
import { ChannelPicker, type SelectedChannel } from './channel-picker';
import { count, formatNS, MAX_NS, MIN_NS, parseNS, qualityNames, rangeLabel, recentRange, shiftRange, zoomRange } from './format';
import { useAnalytic, useTrendTiles, querySlot } from './query-hooks';
import { AnalyticState, DetailTable, LatestTable, StatisticsTable } from './data-tables';
import { ExportForm } from './export-form';
const TrendChart = lazy(() => import('./trend-chart'));

export function MonitoringDataPage({initialPoint,initialChannel}:{initialPoint?:string;initialChannel?:string}) {
  const m = useMonitoring(); const [selected, setSelected] = useState<SelectedChannel[]>([]), [picker, setPicker] = useState(false), [generation, setGeneration] = useState(0);
  const [quality, setQuality] = useState('default'), [timezone, setTimezone] = useState('Asia/Shanghai');
  const ids = selected.map(c => c.id).sort();
  const selection = useQuery({ queryKey: [...m.prefix, 'selection', ids, generation], queryFn: ({ signal }) => m.client.monitoringSelect(m.scope, ids.length ? { channel_ids: ids } : { all_workspace: true }, signal), enabled: m.available && m.canRead, staleTime: Infinity, gcTime: 120000, retry: false });
  if (!m.canRead) return <ErrorState title="没有监测数据查看权限" message="请在当前工作空间配置监测数据查看权限。" />;
  const chooser = <ChannelPicker selected={selected} onChange={setSelected} autoSelect initialPoint={initialPoint} initialChannel={initialChannel} />;
  const qualities: MonitoringQuery['qualities'] = quality === 'all' ? ['good', 'unmarked', 'suspect', 'bad'] : quality === 'suspect' ? ['suspect'] : quality === 'bad' ? ['bad'] : ['good', 'unmarked'];
  return <section className="min-w-0 space-y-5"><PageHeader title="监测数据" description="按测点与通道查看历史观测，比较趋势并核对精确原值。" actions={<div className="flex flex-wrap gap-2"><Button id="monitoring-picker" variant="outline" className="lg:hidden" onClick={() => setPicker(true)}><PanelLeft aria-hidden />测点与通道</Button><Button variant="outline" onClick={() => setGeneration(g => g + 1)} disabled={selection.isFetching || !m.available}><RefreshCw aria-hidden />刷新数据</Button></div>} /><MonitoringTabs current="data" /><div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(15rem,19rem)_minmax(0,1fr)]"><aside className="hidden min-w-0 self-start rounded-xl border p-4 lg:block">{chooser}</aside><div className="min-w-0 space-y-5"><div className="grid gap-3 sm:grid-cols-2"><Choice label="参与查询的质量" value={quality} onChange={setQuality} options={[{ value: 'default', label: '默认：正常 + 未标注' }, { value: 'all', label: '全部质量（含可疑与无效）' }, { value: 'suspect', label: '仅可疑' }, { value: 'bad', label: '仅无效' }]} /><Choice label="展示与日历统计时区" value={timezone} onChange={setTimezone} options={['Asia/Shanghai', 'UTC', 'America/New_York', 'Europe/London', 'Asia/Tokyo'].map(value => ({ value, label: value }))} /></div><MonitoringError error={selection.error} retry={() => void selection.refetch()} />{selection.isPending && <p role="status">正在建立获准数据选择…</p>}{selection.data && <SelectedData key={`${selection.data.id}:${quality}:${timezone}`} selection={selection.data} channels={selected} qualities={qualities} timezone={timezone} />}</div></div><ResponsivePanel open={picker} onOpenChange={setPicker} title="测点与通道" description="搜索设施或测点，选择通道加入比较；已有选择会保留。" fallbackFocusSelector="#monitoring-picker">{picker && chooser}<Button className="mt-4 w-full" onClick={() => setPicker(false)}>查看已选 {selected.length} 个通道</Button></ResponsivePanel></section>;
}
function SelectedData({ selection, channels, qualities, timezone }: { selection: MonitoringSelection; channels: SelectedChannel[]; qualities: MonitoringQuery['qualities']; timezone: string }) {
  const { paths, canManage } = useMonitoring();
  const fullQuery: MonitoringQuery = { selection_id: selection.id, all_history: true, qualities, timezone };
  const full = useAnalytic('overview', fullQuery);
  const [customRange, setCustomRange] = useState<MonitoringRange | null>(null);
  const latest = full.data?.overview?.latest;
  const range = useMemo(() => customRange ?? (latest ? recentRange(latest) : null), [customRange, latest]);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const changeRange = useCallback((value: MonitoringRange) => { clearTimeout(timer.current); timer.current = setTimeout(() => setCustomRange(value), 150); }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  return <><div className="space-y-3"><div className="flex flex-wrap items-center justify-between gap-2"><h2 className="text-base font-semibold">{channels.length ? '已选通道' : '工作空间'} · 全历史概况</h2><span className="text-xs text-muted-foreground">{count(selection.channel_count)} 个获准通道 · 不随视窗移动</span></div><MonitoringError error={full.error} retry={() => void full.refetch()} /><AnalyticState result={full.data} loading={full.isPending} />{full.data?.state === 'ready' && full.data.overview && <OverviewCards overview={full.data.overview} />}{full.data?.state === 'ready' && full.data.overview && <NewDataNotice query={fullQuery} fixed={full.data} />}{full.data?.state === 'ready' && <p className="text-xs text-muted-foreground">查询时间 {new Date(full.dataUpdatedAt).toLocaleString()} · 发布水位 {full.data.snapshot.watermark}{full.error ? ' · 保留上次完整结果，请重试' : ''}</p>}</div>{channels.length === 0 ? <div className="rounded-xl border border-dashed p-8 text-center"><h2 className="font-medium">选择测点，开始查看数据</h2><p className="mt-2 text-sm text-muted-foreground">可把多个测点的兼容通道加入比较。无需绑定设备。</p></div> : full.data?.state === 'ready' ? <><LatestTable query={{ ...fullQuery, snapshot_id: full.data.snapshot.id }} channels={channels} />{!range ? <div className="rounded-lg border p-8 text-center"><p>所选通道尚无在用数据。</p>{canManage && <Button className="mt-3" asChild><Link to={paths.imports}>导入历史数据</Link></Button>}</div> : <><TimeWindow range={range} latest={latest ?? undefined} earliest={full.data.overview?.earliest ?? undefined} onChange={changeRange} /><WindowData snapshot={full.data.snapshot.id} selection={selection.id} channels={channels} range={range} qualities={qualities} timezone={timezone} onWindow={changeRange} /></>}</> : null}</>;
}
function NewDataNotice({query,fixed}:{query:MonitoringQuery;fixed:MonitoringAnalyticResult}) {
  const {client,scope,prefix,available}=useMonitoring(), cache=useQueryClient();
  const key=[...prefix,'new-data',fixed.snapshot.id];
  const fresh=useQuery({queryKey:key,enabled:available,queryFn:({signal})=>querySlot(signal,()=>{
    const previous=cache.getQueryData<MonitoringAnalyticResult>(key);
    return client.monitoringQuery(scope,'overview',{...query,...(previous?.state==='pending'?{snapshot_id:previous.snapshot.id}:{})},signal);
  }),refetchInterval:state=>state.state.data?.state==='pending'?1500:30000,staleTime:30000,retry:false});
  const changed=fresh.data?.state==='ready'&&fixed.overview&&fresh.data.overview&&JSON.stringify(fixed.overview)!==JSON.stringify(fresh.data.overview);
  return changed?<p role="status" className="rounded-lg border bg-muted/50 p-3 text-sm">有新数据。当前窗口仍保留固定快照；使用“刷新数据”查看最新结果。</p>:null;
}
function OverviewCards({ overview, compact = false }: { overview: MonitoringOverview; compact?: boolean }) {
  const items = [['累计曾发布观测', overview.ever_published_count, '按原观测去重，撤销后仍保留'], ['当前在用观测', overview.active_count, '已采用且未撤销，包含全部质量'], ['当前筛选样本', overview.sample_count, '仅纳入所选质量的在用样本']];
  return <div className="space-y-3"><div className="grid gap-3 sm:grid-cols-3">{items.map(([label, value, hint]) => <div key={label} className={`min-w-0 rounded-xl border bg-card ${compact ? 'p-3' : 'p-4'}`}><h3 className="text-sm font-medium text-muted-foreground">{label}</h3><p className={`my-2 break-all font-semibold tabular-nums ${compact ? 'text-xl' : 'text-2xl'}`}>{count(value)}</p><p className="text-xs text-muted-foreground">{hint}</p></div>)}</div><div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">{Object.entries(overview.quality_counts).map(([q, n]) => <span key={q}>{qualityNames[q]} {count(n)}</span>)}</div><p className="break-words text-xs text-muted-foreground">当前覆盖：{overview.earliest ?? '无数据'} 至 {overview.latest ?? '无数据'}</p></div>;
}
function TimeWindow({ range, latest, earliest, onChange }: { range: MonitoringRange; latest?: string; earliest?: string; onChange: (range: MonitoringRange) => void }) {
  const key = rangeLabel(range);
  const [draft, setDraft] = useState({ key, from: range.from ?? '', to: range.to ?? '' }), [error, setError] = useState<Error | null>(null);
  const from = draft.key === key ? draft.from : range.from ?? '', to = draft.key === key ? draft.to : range.to ?? '';
  const setFrom = (value: string) => setDraft({ key, from: value, to }), setTo = (value: string) => setDraft({ key, from, to: value });
  const duration = (range.to ? parseNS(range.to) : MAX_NS) - (range.from ? parseNS(range.from) : MIN_NS);
  return <section className="space-y-3 rounded-xl border p-4"><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={!latest} onClick={() => latest && onChange(recentRange(latest))}>返回最新 24 小时</Button><Button variant="ghost" onClick={() => onChange(recentRange(new Date().toISOString()))}>相对当前时间</Button><Button variant="ghost" disabled={!earliest || !latest} onClick={() => { if (earliest && latest) onChange({ from: earliest, to: parseNS(latest) === MAX_NS ? null : formatNS(parseNS(latest) + 1n) }); }}>全部历史范围</Button></div><form className="space-y-3" onSubmit={e => { e.preventDefault(); try { const start = from ? parseNS(from) : MIN_NS, end = to ? parseNS(to) : MAX_NS; if (end <= start || !from && !to) throw new Error('请填写至少一个端点，结束必须晚于开始。'); setError(null); onChange({ from: from || null, to: to || null }); } catch (e) { setError(e instanceof Error ? e : new Error('时间格式无效。')); } }}><div className="grid gap-3 md:grid-cols-2"><Field label="窗口开始（含）" value={from} onChange={setFrom} hint="RFC 3339 时间，可带 9 位小数；留空为不限开始。" /><Field label="窗口结束（不含）" value={to} onChange={setTo} hint="例如 2020-01-02T00:00:00+08:00；留空为不限结束。" /></div><div className="flex flex-wrap gap-2"><Button type="submit">跳转到日期范围</Button><Button type="button" variant="outline" onClick={() => onChange(shiftRange(range, -duration / 2n))}>向前半窗</Button><Button type="button" variant="outline" onClick={() => onChange(shiftRange(range, duration / 2n))}>向后半窗</Button><Button type="button" variant="outline" onClick={() => onChange(zoomRange(range, 1n, 2n))}>放大一倍</Button><Button type="button" variant="outline" onClick={() => onChange(zoomRange(range, 2n, 1n))}>缩小一倍</Button></div><MonitoringError error={error} /></form></section>;
}
function WindowData({ snapshot, selection, channels, range, qualities, timezone, onWindow }: { snapshot: string; selection: string; channels: SelectedChannel[]; range: MonitoringRange; qualities: MonitoringQuery['qualities']; timezone: string; onWindow: (range: MonitoringRange) => void }) {
  const { canHistory, canExport, client, scope, prefix } = useMonitoring();
  const [tab, setTab] = useState('trends'), [status, setStatus] = useState<'active' | 'history'>('active'), [exporting, setExporting] = useState(false), [period, setPeriod] = useState<'none' | 'day' | 'month' | 'year'>('none'), [digits, setDigits] = useState(12);
  const historical = useAnalytic('overview', status === 'history' ? { selection_id: selection, all_history: true, qualities, timezone, status } : undefined);
  const fixed = status === 'history' ? historical.data?.snapshot.id : snapshot;
  const query: MonitoringQuery | undefined = fixed ? { selection_id: selection, all_history: true, snapshot_id: fixed, view_range: range, qualities, timezone, status } : undefined;
  const overview = useAnalytic('overview', query), ready = overview.data?.state === 'ready';
  const renewal = useQuery({ queryKey: [...prefix, 'snapshot-renewal', fixed], queryFn: ({ signal }) => { if (!query?.snapshot_id) throw new Error('缺少快照。'); return client.monitoringRenew(scope, query.snapshot_id, query, signal); }, enabled: ready && !!query, refetchInterval: 5 * 60_000, staleTime: 5 * 60_000, retry: false });
  return <section className="min-w-0 space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap items-center gap-3"><h2 className="font-semibold">当前窗口</h2><Button variant="ghost" size="sm" disabled={!ready || overview.isFetching} onClick={() => void overview.refetch()}>重读当前快照</Button></div>{canHistory && <label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={status === 'history'} onChange={e => setStatus(e.target.checked ? 'history' : 'active')} />包含撤销历史（需历史权限）</label>}</div>
    <MonitoringError error={historical.error} /><MonitoringError error={overview.error} retry={() => void overview.refetch()} /><MonitoringError error={renewal.error} />
    <AnalyticState result={overview.data} loading={overview.isPending} />
    {ready && overview.data && <div className="rounded-xl border p-4"><h3 className="mb-3 text-sm font-medium">窗口内在用观测与质量数量</h3>{overview.data.overview && <OverviewCards overview={overview.data.overview} compact />}{status === 'history' && <p className="mt-3 text-xs">以下明细与统计包含撤销历史；上方三类数量仍按当前在用定义，统计样本 N 单独展示。</p>}<p className="mt-3 break-all text-xs text-muted-foreground">固定快照 {fixed} · 水位 {overview.data.snapshot.watermark} · 最近完成 {new Date(overview.dataUpdatedAt).toLocaleString()}{overview.error ? ' · 当前读取失败，保留上次完整结果' : ''}</p></div>}
    <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3"><div className="flex gap-2" aria-label="监测展示方式">{([['trends', '趋势'], ['details', '明细'], ['statistics', '统计']] as const).map(([value, label]) => <Button key={value} variant={tab === value ? 'secondary' : 'ghost'} aria-pressed={tab === value} onClick={() => setTab(value)}>{label}</Button>)}</div>{canExport && <Button id="monitoring-export" variant="outline" disabled={!ready || !query || !!renewal.error} onClick={() => setExporting(true)}><Download aria-hidden />导出当前{tab === 'statistics' ? '统计' : '明细'}</Button>}</div>
    {query && tab === 'trends' && <WindowTrend key={fixed} query={query} channels={channels} range={range} timezone={timezone} onWindow={onWindow} />}
    {query && tab === 'details' && <DetailTable key={`${fixed}:${rangeLabel(range)}`} query={query} channels={channels} />}
    {query && tab === 'statistics' && <StatisticsTable key={`${fixed}:${rangeLabel(range)}`} query={query} channels={channels} period={period} digits={digits} onPeriod={setPeriod} onDigits={setDigits} />}
    <ResponsivePanel open={exporting} onOpenChange={setExporting} title="创建后台导出" description="核对当前范围、质量及格式后创建；任务完成后在任务中心下载。" fallbackFocusSelector="#monitoring-export">{exporting && query && <ExportForm query={{ ...query, ...(tab === 'statistics' && period !== 'none' ? { period } : {}), digits }} channelCount={channels.length} estimate={overview.data?.overview?.sample_count} kind={tab === 'statistics' ? 'statistics' : 'details'} />}</ResponsivePanel>
  </section>;
}
function WindowTrend({ query, channels, range, timezone, onWindow }: { query: MonitoringQuery; channels: SelectedChannel[]; range: MonitoringRange; timezone: string; onWindow: (range: MonitoringRange) => void }) {
  const [width, setWidth] = useState(960); const trend = useTrendTiles(query, range, width, channels.length);
  return <div className="min-h-80 space-y-3"><MonitoringError error={trend.error} retry={trend.retry} />{trend.failures.length>0&&<p className="break-words text-sm">未完整加载的时间块：{trend.failures.map(rangeLabel).join("；")}</p>}{(trend.pending || !trend.display && !trend.error) && <AnalyticState result={trend.pending} loading={!trend.display && !trend.pending} />}<p role="status" className="text-xs text-muted-foreground">当前视窗时间块 {trend.cached} / {trend.tiles.length} 已就绪{!trend.complete && trend.display ? ` · ${trend.error ? "当前读取失败" : "正在加载"}，显示上次完整窗口（${rangeLabel(trend.display.range)}）` : ""}。完整时间块含窗口边缘数据；当前窗口精确数量以上方概况为准。</p>{trend.display && <Suspense fallback={<p role="status">正在加载图表…</p>}><TrendChart channels={channels} series={trend.display.series} range={trend.display.range} timezone={timezone} onWindow={onWindow} onWidth={setWidth} /></Suspense>}</div>;
}
