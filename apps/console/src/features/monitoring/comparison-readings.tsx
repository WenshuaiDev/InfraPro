import { useId, useState } from 'react';
import { Activity, CircleHelp } from 'lucide-react';
import type { MonitoringAnalyticResult, MonitoringChannelStatistic, MonitoringQuery } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@infrapro/ui/components/table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { PositionView } from '../location-fields/position-editor';
import type { SelectedChannel } from './channel-picker';
import { MonitoringError } from './common';
import { AnalyticState } from './data-tables';
import { calculationStatusNames, observationQuality, observationTime, reasonNames } from './detail-format';
import { qualityNames, rangeLabel } from './format';
import { useAnalytic } from './query-hooks';
import { channelAppearance } from './trend-model';
import { DerivedLatest } from './workspace-derived-latest';
import { displayTime, observedAge } from './workspace-model';

const valueText = (value: string | null | undefined) => value == null ? '—' : value.replace(/(\.\d*?[1-9])0+$|\.0+$/, '$1');

export function ComparisonReadings({ channels, latest, query, timezone, now, qualities, hidden }: { channels: SelectedChannel[]; latest?: MonitoringAnalyticResult; query?: MonitoringQuery; timezone: string; now: number; qualities?: string[]; hidden: string[] }) {
  const id = useId(), [inspected, setInspected] = useState<string>();
  const statistics = useAnalytic('statistics', query?.view_range ? { ...query, limit: 50 } : undefined);
  const channel = channels.find(item => item.id === inspected), row = latest?.channels?.find(item => item.channel_id === inspected);
  const stats = statistics.data?.state === 'ready' ? statistics.data.channels : undefined;
  return <section className="min-w-0 overflow-hidden rounded-lg border bg-card" aria-label="通道读数">
    <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3"><h2 className="flex items-center gap-2 text-sm font-semibold"><Activity className="size-4" aria-hidden />通道读数</h2><span className="text-xs text-muted-foreground">最新观测不随历史范围改变 · 极值按当前窗口统计</span></div>
    <div className="px-4"><MonitoringError error={statistics.error} retry={() => void statistics.refetch()} />{query?.view_range && statistics.data?.state !== 'ready' && <AnalyticState result={statistics.data} loading={statistics.isPending} />}</div>
    <Table aria-label="比较通道读数"><TableHeader><TableRow>{['通道 / 所属对象', '最新观测', '当前窗口最小', '当前窗口最大', '质量 / 计算状态', ''].map((text, index) => <TableHead key={index}>{text}</TableHead>)}</TableRow></TableHeader><TableBody>{channels.map(channel => {
      const latestRow = latest?.channels?.find(item => item.channel_id === channel.id), last = channel.source_kind === 'derived' ? latestRow?.latest_output : latestRow?.last;
      const statistic = stats?.find(item => item.channel_id === channel.id), at = last && observationTime(last), quality = last && observationQuality(last);
      const numeric = channel.value_kind !== 'enum';
      return <TableRow key={channel.id} className={hidden.includes(channel.id) ? 'opacity-60' : ''}>
        <TableCell className="max-w-64 whitespace-normal"><button id={`${id}-${channel.id}`} type="button" className="flex items-start gap-2 text-left" onClick={() => setInspected(channel.id)}><span aria-hidden style={{ color: channelAppearance(channel).color }}>{channelAppearance(channel).mark}</span><span><strong className="block break-words text-xs font-medium">{channel.name}</strong><small className="mt-1 block text-[11px] text-muted-foreground">{channel.owner_name} · {channel.source_kind === 'derived' ? '派生' : '直接观测'}{hidden.includes(channel.id) && ' · 曲线已隐藏'}</small></span></button></TableCell>
        <TableCell className="tabular-nums"><span className="font-medium">{last ? last.value === null ? '无结果' : numeric ? valueText(last.value) : last.value || '（空文本）' : latest ? channel.source_kind === 'derived' ? '尚无输出' : '暂无观测' : '读取中…'}</span> <span className="text-xs text-muted-foreground">{channel.unit}</span>{at && <time className="mt-1 block text-[11px] text-muted-foreground" dateTime={at}>{displayTime(at, timezone)}</time>}</TableCell>
        <TableCell className="font-mono text-xs tabular-nums">{!numeric ? '不适用' : stats ? valueText(statistic?.minimum?.value) : statistics.error ? '读取失败' : query?.view_range ? '计算中…' : '—'}</TableCell>
        <TableCell className="font-mono text-xs tabular-nums">{!numeric ? '不适用' : stats ? valueText(statistic?.maximum?.value) : statistics.error ? '读取失败' : query?.view_range ? '计算中…' : '—'}</TableCell>
        <TableCell className="max-w-56 whitespace-normal text-xs">{last ? <><Badge variant={quality === 'bad' ? 'destructive' : 'outline'}>{qualityNames[quality!]}</Badge>{last.source_kind === 'derived' && <p className="mt-1">{calculationStatusNames[last.calculation_status]}{last.reason_flags.length > 0 && ` · ${last.reason_flags.map(reason => reasonNames[reason] ?? reason).join('、')}`}</p>}{at && <p className="mt-1 text-muted-foreground">{observedAge(at, now)}</p>}{quality && qualities && !qualities.includes(quality) && <p className="mt-1 text-muted-foreground">未纳入当前质量筛选</p>}</> : '—'}{!!latestRow?.pending_ranges?.length && <p className="mt-1">部分范围等待更新</p>}</TableCell>
        <TableCell><Button size="icon" variant="ghost" aria-label={`查看 ${channel.owner_name} ${channel.name} 信息`} onClick={() => setInspected(channel.id)}><CircleHelp className="size-4" /></Button></TableCell>
      </TableRow>;
    })}</TableBody></Table>
    {(latest?.coverage?.some(part => part.state !== 'current') || statistics.data?.coverage?.some(part => part.state !== 'current')) && <details className="border-t px-4 py-3 text-xs text-muted-foreground"><summary className="cursor-pointer">部分范围尚未计算或等待更新 · 查看计算覆盖</summary><div className="mt-3 space-y-3"><p>最新观测覆盖</p><AnalyticState result={latest} />{statistics.data?.state === 'ready' && <><p>当前窗口统计覆盖</p><AnalyticState result={statistics.data} /></>}</div></details>}
    <ResponsivePanel open={!!channel} onOpenChange={open => { if (!open) setInspected(undefined); }} title="通道信息与精确读数" description={channel ? `${channel.facility_name} / ${channel.owner_name}` : ''} fallbackFocusSelector={`[id="${id}-${inspected}"]`}>
      {channel && <div className="space-y-5">{channel.source_kind === 'derived' ? <DerivedLatest channel={channel} row={row} timezone={timezone} comparing /> : <DirectReading channel={channel} row={row} timezone={timezone} />}
        <dl className="space-y-3 break-all text-sm"><div><dt className="text-xs text-muted-foreground">通道编号 / 指标</dt><dd>{channel.code} · {channel.metric_name}</dd></div><div><dt className="text-xs text-muted-foreground">方向 / 基准</dt><dd>{channel.observation_direction || '方向未填'} · {channel.reference_basis || '基准未填'}</dd></div><div><dt className="text-xs text-muted-foreground">时间含义</dt><dd>{channel.timestamp_meaning === 'interval_end' ? '区间结束时刻' : '观测时刻'} · {timezone}</dd></div><div><dt className="text-xs text-muted-foreground">可读范围</dt><dd>{channel.read_ranges.map(rangeLabel).join('；')}</dd></div></dl>
        <details className="text-sm"><summary className="min-h-11 cursor-pointer py-3">所属对象位置</summary><PositionView position={channel.position} /></details>
      </div>}
    </ResponsivePanel>
  </section>;
}

function DirectReading({ channel, row, timezone }: { channel: SelectedChannel; row?: MonitoringChannelStatistic; timezone: string }) {
  const last = row?.last?.source_kind === 'direct' ? row.last : undefined;
  return <div className="space-y-3 rounded-lg border p-4"><h3 className="font-semibold">{channel.name}</h3>{last ? <><p className="break-all font-mono text-xl">{last.value || '（空文本）'} <span className="text-sm">{channel.unit}</span></p><p className="text-sm">{displayTime(last.observed_at, timezone)} · {qualityNames[last.quality]}</p><p className="break-all text-xs text-muted-foreground">精确时间：{last.observed_at}</p>{last.quality_reason && <p className="break-words text-sm">质量说明：{last.quality_reason}</p>}</> : <p className="text-sm text-muted-foreground">暂无在用观测。无数据不表示值为零。</p>}</div>;
}
