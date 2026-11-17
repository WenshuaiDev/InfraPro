import type { MonitoringChannel, MonitoringRange, MonitoringTrendSeries } from '@infrapro/api-client';
import { calculationStatusNames, observationKind, observationQuality, observationRevoked, observationTime, plotCoefficient, type DetailRow } from './detail-format';
import { MAX_NS, MIN_NS, parseNS, qualityNames } from './format';
import { plotBaseline, plotObservations } from './trend-points';

export type TrendScale = 'actual' | 'relative';
const symbols = ['circle', 'rect', 'triangle', 'diamond', 'pin', 'roundRect'];
const marks = ['●', '■', '▲', '◆', '⬟', '▰'];

// Channel identity, rather than its current index or compatibility group, owns
// its appearance. Removing or hiding a peer cannot recolor surviving curves.
export function channelAppearance(channel: Pick<MonitoringChannel, 'id' | 'source_kind'>) {
  let hash = 2166136261;
  for (const character of channel.id) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619) >>> 0;
  const symbol = Math.floor(hash / 5) % symbols.length;
  return { color: `var(--chart-${hash % 5 + 1})`, colorIndex: hash % 5 + 1, symbol: symbols[symbol]!, mark: marks[symbol]!, lineType: channel.source_kind === 'derived' ? 'dashed' as const : 'solid' as const };
}
export type TrendAxis = { key: string; kind: 'value' | 'category'; unit: string; name: string; precision: number; baseline: bigint; baselineValue?: string; states: string[] };
export type TrendDatum = { value: [number | null, number | string | null]; name: string };
export type TrendLine = { id: string; name: string; axisIndex: number; channel: MonitoringChannel; summary: boolean; data: TrendDatum[] };

// Metadata differences remain in channel details and tooltips; they never
// exclude a selected series from the plot. Only units determine numeric axes.
export function buildTrendModel(channels: MonitoringChannel[], series: MonitoringTrendSeries[], range: MonitoringRange, scale: TrendScale = 'actual') {
  const start = range.from ? parseNS(range.from) : MIN_NS, end = range.to ? parseNS(range.to) : MAX_NS;
  const byID = new Map(series.map(item => [item.channel_id, item]));
  const axisKey = (channel: MonitoringChannel) => channel.value_kind === 'enum' ? 'enum' : scale === 'relative' ? 'relative' : `unit:${channel.unit}`;
  const grouped = new Map<string, MonitoringChannel[]>();
  for (const channel of channels) { const key = axisKey(channel); grouped.set(key, [...(grouped.get(key) ?? []), channel]); }
  const axes: TrendAxis[] = [...grouped].map(([key, peers]) => {
    const rows = peers.flatMap(channel => byID.get(channel.id)?.buckets.flatMap(bucket => bucket.points) ?? []);
    const numeric = rows.filter(row => observationKind(row) !== 'enum' && row.value !== null);
    const precision = numeric.reduce((digits, row) => Math.max(digits, row.value?.split('.')[1]?.length ?? 0), 12);
    const baseline = scale === 'relative' ? 0n : plotBaseline(numeric[0], precision);
    const states = [...new Set(peers.flatMap(channel => byID.get(channel.id)?.buckets.flatMap(bucket => [...bucket.states.map(state => state.value), ...bucket.points.filter(row => observationKind(row) === 'enum' && row.value !== null).map(row => row.value!)]) ?? []))].sort();
    const unit = peers[0]!.value_kind === 'enum' ? '状态' : peers[0]!.unit || '无单位';
    return { key, kind: key === 'enum' ? 'category' : 'value', unit, precision, baseline, baselineValue: baseline ? numeric[0]?.value ?? undefined : undefined, states, name: key === 'enum' ? '状态' : key === 'relative' ? '相对幅度 (%)' : baseline ? `${unit} · 相对基准` : unit };
  });
  const lines: TrendLine[] = channels.map(channel => {
    const source = byID.get(channel.id), axisIndex = axes.findIndex(axis => axis.key === axisKey(channel)), axis = axes[axisIndex]!;
    const rows = source ? plotObservations(source) : [];
    let low: bigint | undefined, high: bigint | undefined;
    if (scale === 'relative' && channel.value_kind !== 'enum') for (const row of rows) {
      if (!row || row.value === null) continue;
      const at = parseNS(observationTime(row)!);
      if (at < start || at >= end && end !== MAX_NS) continue;
      const value = plotCoefficient(row.value, axis.precision);
      low = low === undefined || value < low ? value : low;
      high = high === undefined || value > high ? value : high;
    }
    const observation = (row: DetailRow | null): TrendDatum => {
      if (!row || row.value === null) return { value: [null, null], name: '' };
      const at = observationTime(row)!;
      const numeric = observationKind(row) === 'enum' ? undefined : plotCoefficient(row.value, axis.precision);
      const value = numeric === undefined ? row.value : scale === 'relative' ? low === undefined || high === undefined || high === low ? 50 : Number(numeric - low) / Number(high - low) * 100 : Number(numeric - axis.baseline) / 10 ** axis.precision;
      return { value: [Number(parseNS(at) - start) / 1e9, value], name: `${channel.facility_name} / ${channel.owner_name} · ${channel.name}\n${at}\n原值 ${row.value || '（空文本）'} ${channel.unit}\n质量 ${qualityNames[observationQuality(row)]}${observationRevoked(row) ? ' · 已撤销' : ''}${row.source_kind === 'derived' ? ` · ${calculationStatusNames[row.calculation_status]}` : ''}\n${channel.observation_direction || '方向未填写'} · ${channel.reference_basis || '基准未填写'}\n${channel.timestamp_meaning === 'interval_end' ? '区间结束时刻' : '观测时刻'}` };
    };
    const data = channel.value_kind === 'enum' ? source?.buckets.flatMap(bucket => bucket.states.length ? bucket.states.map(state => ({ value: [Number(parseNS(state.first_at) - start) / 1e9, state.value] as [number, string], name: `${channel.owner_name} · ${channel.name}\n状态：${state.value || '（空文本）'}\n样本 ${state.count} · 占比 ${state.sample_proportion}\n首次 ${state.first_at}\n末次 ${state.last_at}` })) : bucket.points.map(observation)) ?? [] : rows.map(observation);
    return { id: channel.id, name: `${channel.owner_name} · ${channel.name}`, axisIndex, channel, summary: source?.summary ?? false, data };
  });
  return { axes, lines, start, end };
}
