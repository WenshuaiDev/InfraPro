import { useId, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { MonitoringAnalyticResult, MonitoringOverview, MonitoringQuery } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { AnalyticState, StatisticsTable } from './data-tables';
import { Choice, MonitoringError } from './common';
import { useAnalytic } from './query-hooks';
import type { SelectedChannel } from './channel-picker';
import { count, qualityNames, rangeLabel } from './format';
import { displayTime, qualityOptions, selectedQualities } from './workspace-model';
import { ExportForm } from './export-form';
import { useMonitoring } from './context';

export function WorkspaceOverview() {
  const m = useMonitoring();
  const [quality, setQuality] = useState('default'), [generation, setGeneration] = useState(0);
  const selection = useQuery({ queryKey: [...m.prefix, 'selection', 'all-workspace'], queryFn: ({ signal }) => m.client.monitoringSelect(m.scope, { all_workspace: true }, signal), enabled: m.available, staleTime: 120_000, retry: false });
  const qualities = selectedQualities(quality);
  const result = useAnalytic('overview', selection.data ? { selection_id: selection.data.id, all_history: true, qualities, timezone: 'Asia/Shanghai' } : undefined, generation);
  const data = result.data?.state === 'ready' ? result.data.overview : undefined;
  return <div className="space-y-5">
    <div className="flex flex-wrap items-center justify-between gap-3"><p className="text-sm">{count(selection.data?.channel_count)} 个获准通道 · 全部历史</p><Button variant="outline" size="sm" disabled={selection.isFetching || result.isFetching} onClick={() => setGeneration(value => value + 1)}>刷新统计</Button></div>
    <div className="max-w-sm"><Choice label="样本质量口径" value={quality} onChange={setQuality} options={qualityOptions} /></div>
    <MonitoringError error={selection.error} retry={() => void selection.refetch()} /><MonitoringError error={result.error} retry={() => void result.refetch()} />
    <AnalyticState result={result.data} loading={selection.isPending || result.isPending} />
    {data && <>
      <section className="space-y-3"><h3 className="font-semibold">{data.derived ? '直接观测记录' : '观测记录'}</h3><div className="grid gap-4 sm:grid-cols-3">{[['累计曾发布', data.ever_published_count, '按唯一观测累计，撤销不减少'], ['当前在用', data.active_count, '已采用且未撤销，包含全部质量'], ['已撤销且未在用', (BigInt(data.ever_published_count) - BigInt(data.active_count)).toString(), '重新采用后不重复累计']].map(([label, value, hint]) => <div key={label} className="rounded-lg border p-4"><p className="text-sm text-muted-foreground">{label}</p><p className="my-2 text-2xl font-semibold tabular-nums">{count(value)}</p><p className="text-xs text-muted-foreground">{hint}</p></div>)}</div></section>
      <section className="space-y-4 border-t pt-5"><p className="text-sm">{data.derived ? '纳入的直接观测样本' : '纳入的在用样本'} <strong className="mx-2 text-xl font-semibold tabular-nums">{count(data.sample_count)}</strong> 条</p><div className="grid gap-3 sm:grid-cols-4">{(['good', 'unmarked', 'suspect', 'bad'] as const).map(q => <div key={q} className="flex items-center justify-between gap-2 rounded-md bg-muted/40 p-3 text-sm"><span>{qualityNames[q]} <span className="text-xs text-muted-foreground">{qualities?.includes(q) ? '纳入' : '筛除'}</span></span><strong className="font-medium tabular-nums">{count(data.quality_counts[q])}</strong></div>)}</div></section>
      <DerivedCounts counts={data.derived} /><section className="space-y-3 border-t pt-5"><h3 className="font-semibold">{data.derived ? '观测与派生输出覆盖' : '在用数据覆盖'}</h3><dl className="grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-muted-foreground">最早观测</dt><dd className="mt-1 tabular-nums">{displayTime(data.earliest, 'Asia/Shanghai')}</dd></div><div><dt className="text-muted-foreground">最近观测</dt><dd className="mt-1 tabular-nums">{displayTime(data.latest, 'Asia/Shanghai')}</dd></div></dl><p className="text-xs text-muted-foreground">Asia/Shanghai · 覆盖包含全部质量；各通道仅统计当前获准范围。</p><details className="text-xs text-muted-foreground"><summary className="cursor-pointer">精确覆盖时间</summary><p className="mt-2 break-all">{data.earliest ?? '—'} 至 {data.latest ?? '—'}</p></details></section>
      <p className="text-xs text-muted-foreground">统计完成于 {displayTime(new Date(result.dataUpdatedAt).toISOString(), 'Asia/Shanghai')}</p>
    </>}
  </div>;
}

export function MonitoringOverviewPanel({ query, channels, full }: { query: MonitoringQuery; channels: SelectedChannel[]; full: MonitoringAnalyticResult }) {
  const { canExport } = useMonitoring(), id = useId();
  const [scope, setScope] = useState('window'), [period, setPeriod] = useState<'none' | 'day' | 'month' | 'year'>('none'), [digits, setDigits] = useState(12), [exporting, setExporting] = useState(false);
  const input = { ...query, view_range: scope === 'window' ? query.view_range : undefined };
  const result = useAnalytic('overview', input), data = result.data?.overview, timezone = query.timezone ?? 'Asia/Shanghai';
  return <div className="min-w-0 space-y-5">
    <div className="flex flex-wrap justify-between gap-3"><div className="flex flex-wrap gap-1" aria-label="统计范围"><Button size="sm" variant={scope === 'window' ? 'secondary' : 'ghost'} aria-pressed={scope === 'window'} onClick={() => setScope('window')}>当前查阅范围</Button><Button size="sm" variant={scope === 'history' ? 'secondary' : 'ghost'} aria-pressed={scope === 'history'} onClick={() => setScope('history')}>所选通道全部历史</Button></div><span className="self-center text-xs text-muted-foreground">{channels.length} 个所选通道 · {timezone}</span></div>
    <div className="space-y-2 rounded-md bg-muted/40 p-3 text-sm"><p className="break-words">{scope === 'window' && query.view_range ? `${displayTime(query.view_range.from, timezone)} — ${displayTime(query.view_range.to, timezone)}` : '所选通道的全部获准历史'}</p><p className="text-xs text-muted-foreground">质量：{query.qualities?.map(q => qualityNames[q]).join(' + ')} · {query.status === 'history' ? '包含撤销历史，统计 N 可包含已撤销样本' : '仅在用观测'}</p>{scope === 'window' && query.view_range && <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">精确起止时间</summary><p className="mt-2 break-all">{rangeLabel(query.view_range)}</p></details>}</div>
    <MonitoringError error={result.error} retry={() => void result.refetch()} /><AnalyticState result={result.data} loading={result.isPending} />
    {data && result.data?.state === 'ready' && <>
      <div className="grid gap-4 border-b pb-5 sm:grid-cols-3"><div><p className="text-sm text-muted-foreground">{data.derived ? '纳入的直接观测样本' : '纳入的在用样本'}</p><p className="my-2 text-2xl font-semibold tabular-nums">{count(data.sample_count)} <span className="text-sm font-normal">条</span></p><p className="text-xs text-muted-foreground">在用 {count(data.active_count)} 条，质量筛除 {count((BigInt(data.active_count) - BigInt(data.sample_count)).toString())} 条</p></div><div><p className="text-sm text-muted-foreground">最早在用覆盖</p><p className="my-2 text-sm font-medium tabular-nums">{displayTime(data.earliest, timezone)}</p><p className="text-xs text-muted-foreground">覆盖包含全部质量</p></div><div><p className="text-sm text-muted-foreground">最近在用覆盖</p><p className="my-2 text-sm font-medium tabular-nums">{displayTime(data.latest, timezone)}</p><p className="text-xs text-muted-foreground">属于当前统计范围</p></div></div>
      <DerivedCounts counts={data.derived} /><section className="space-y-3"><h3 className="font-semibold">逐通道统计</h3><StatisticsTable key={`${scope}:${query.snapshot_id}`} query={input} channels={channels} period={period} digits={digits} onPeriod={setPeriod} onDigits={setDigits} /></section>
      <section className="space-y-3 border-t pt-5"><div className="flex flex-wrap justify-between gap-2"><h3 className="font-semibold">{data.derived ? '范围内直接观测的质量' : '范围内在用观测的质量'}</h3><span className="text-xs text-muted-foreground">共 {count(data.active_count)} 条，包含被筛除的记录</span></div><div className="flex flex-wrap gap-x-5 gap-y-2 text-sm">{(['good', 'unmarked', 'suspect', 'bad'] as const).map(q => <span key={q}>{qualityNames[q]} <strong className="font-medium tabular-nums">{count(data.quality_counts[q] ?? '0')}</strong><span className="ml-2 text-xs text-muted-foreground">{query.qualities?.includes(q) ? '纳入' : '筛除'}</span></span>)}</div></section>
    </>}
    <details className="border-t pt-4"><summary className="cursor-pointer text-sm text-muted-foreground">{full.overview?.derived ? '直接观测全历史记录与生命周期' : '全历史记录与生命周期'}</summary><div className="mt-3 space-y-3 rounded-md bg-muted/40 p-4 text-sm"><p className="text-xs text-muted-foreground">当前所选通道的全部历史，独立于上方时间范围；不按质量筛除。</p><dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-2"><dt>累计曾发布的唯一观测</dt><dd className="tabular-nums">{count(full.overview?.ever_published_count)}</dd><dt>当前在用观测</dt><dd className="tabular-nums">{count(full.overview?.active_count)}</dd><dt>已撤销且当前未在用</dt><dd className="tabular-nums">{full.overview ? count((BigInt(full.overview.ever_published_count) - BigInt(full.overview.active_count)).toString()) : '—'}</dd></dl><p className="text-xs text-muted-foreground">撤销后保留观测身份；重新采用不会重复累计。当前数值统计是否包含撤销记录以上方查询口径为准。</p></div></details>
    {canExport && <Button id={`${id}-statistics-export`} variant="outline" disabled={result.data?.state !== 'ready'} onClick={() => setExporting(true)}>导出当前统计</Button>}
    <ResponsivePanel open={exporting} onOpenChange={setExporting} title="导出统计结果" description="导出使用当前统计范围、质量和分组口径。" fallbackFocusSelector={`[id="${id}-statistics-export"]`}>{exporting && <ExportForm query={{ ...input, digits, ...(period === 'none' ? {} : { period }) }} channelCount={channels.length} kind="statistics" estimate={data?.sample_count} />}</ResponsivePanel>
  </div>;
}


function DerivedCounts({ counts }: { counts?: MonitoringOverview['derived'] }) {
  if (!counts) return null;
  return <section className="space-y-3" aria-label="派生输出数量"><h3 className="font-semibold">派生输出</h3><div className="grid gap-3 sm:grid-cols-3">{[['逻辑输出', counts.logical_output_count], ['有效结果', counts.valid_result_count], ['无结果', counts.no_result_count]].map(([label, value]) => <div key={label} className="min-w-0 rounded-lg border p-3"><p className="text-sm text-muted-foreground">{label}</p><p className="my-2 break-all text-2xl font-semibold tabular-nums">{count(value)}</p></div>)}</div><p className="text-xs text-muted-foreground">只统计当前正式集合，不重复累计历史代次；无结果有明确的计算状态，不作为零值。逐通道统计 N 还会排除不符合质量口径的结果。</p></section>;
}
