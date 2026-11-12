import type { MonitoringTrendSeries } from '@infrapro/api-client';
export function mergeTrendTiles(tiles: MonitoringTrendSeries[][]): MonitoringTrendSeries[] {
  const merged = new Map<string, MonitoringTrendSeries>();
  for (const series of tiles) for (const part of series) {
    const current = merged.get(part.channel_id);
    if (!current) { merged.set(part.channel_id, { ...part, buckets: [...part.buckets] }); continue; }
    current.buckets.push(...part.buckets);
    current.sample_count = (BigInt(current.sample_count) + BigInt(part.sample_count)).toString();
    current.range = { from: current.range.from, to: part.range.to };
    current.summary ||= part.summary;
    current.resolution = [...new Set([...current.resolution.split(' / '), part.resolution])].join(' / ');
  }
  return Array.from(merged.values());
}
