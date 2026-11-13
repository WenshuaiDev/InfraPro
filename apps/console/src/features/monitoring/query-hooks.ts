import { mergeTrendTiles } from './tiles';
import { viewportTiles } from './format';
import { useEffect, useState } from 'react';
import { useQuery, useQueryClient, useQueries, type Query, type QueryClient } from '@tanstack/react-query';
import type { MonitoringAnalyticResult, MonitoringQuery, MonitoringRange, MonitoringTrendSeries } from '@infrapro/api-client';
import { useMonitoring } from './context';

// Bound interactive query concurrency even when statistics, details and trends
// mount together. Aborted queued work never occupies a connection.
let active = 0;
const queue: (() => void)[] = [];
export async function querySlot<T>(signal: AbortSignal, run: () => Promise<T>): Promise<T> {
  if (signal.aborted) throw signal.reason;
  await new Promise<void>((resolve, reject) => {
    const start = () => { signal.removeEventListener('abort', abort); active++; resolve(); };
    const abort = () => { const i = queue.indexOf(start); if (i >= 0) queue.splice(i, 1); reject(signal.reason); };
    if (active < 3) start(); else { queue.push(start); signal.addEventListener('abort', abort, { once: true }); }
  });
  try { if (signal.aborted) throw signal.reason; return await run(); }
  finally { active--; queue.shift()?.(); }
}
// Each completed refresh creates a new snapshot; only unfinished jobs reuse one.
export function useLiveLatest(selection: string | undefined, timezone: string, enabled: boolean) {
  const { client, scope, prefix, available } = useMonitoring(), cache = useQueryClient();
  const queryKey = [...prefix, 'live-latest', selection, timezone];
  return useQuery({ queryKey, enabled: available && !!selection, retry: false, staleTime: 30_000, gcTime: 120_000,
    queryFn: ({ signal }) => querySlot(signal, () => {
      const previous = cache.getQueryData<MonitoringAnalyticResult>(queryKey);
      return client.monitoringQuery(scope, 'latest', { selection_id: selection!, all_history: true, qualities: ['good', 'unmarked', 'suspect', 'bad'], timezone, limit: 50, ...(previous?.state === 'pending' ? { snapshot_id: previous.snapshot.id } : {}) }, signal);
    }), refetchInterval: q => q.state.data?.state === 'pending' ? 1500 : enabled ? 30_000 : false,
    refetchOnWindowFocus: enabled ? 'always' : false,
  });
}
export function useAnalytic(kind: 'overview' | 'latest' | 'statistics' | 'trends', input: MonitoringQuery | undefined, generation = 0) {
  const { client, scope, prefix, available } = useMonitoring(), cache = useQueryClient();
  const queryKey = [...prefix, 'analytic', kind, input, generation];
  const query = useQuery({ queryKey, enabled: available && !!input, queryFn: ({ signal }) => querySlot(signal, () => {
    if (!input) throw new Error('请选择通道。');
    const previous = cache.getQueryData<MonitoringAnalyticResult>(queryKey);
    return client.monitoringQuery(scope, kind, { ...input, ...(previous ? { snapshot_id: previous.snapshot.id } : {}) }, signal);
  }), refetchInterval: q => q.state.data?.state === 'pending' ? 1500 : false, staleTime: 120_000, gcTime: 120_000, retry: false });
  useEffect(() => {
    if (!query.data || kind !== 'trends') return;
    const entries = cache.getQueryCache().findAll({ queryKey: [...prefix, 'analytic', 'trends'] }).sort((a, b) => b.state.dataUpdatedAt - a.state.dataUpdatedAt);
    let bytes = 0, points = 0;
    for (const entry of entries) {
      const data = entry.state.data as MonitoringAnalyticResult | undefined;
      if (!data) continue;
      bytes += JSON.stringify(data).length * 2;
      points += (data.trends ?? []).reduce((n, series) => n + series.buckets.reduce((m, bucket) => m + bucket.points.length + bucket.states.length, 0), 0);
      if ((bytes > 32 * 1024 * 1024 || points > 80_000) && !entry.getObserversCount()) cache.removeQueries({ queryKey: entry.queryKey, exact: true });
    }
  }, [cache, kind, prefix, query.data]);
  return query;
}

const trendAccess = new WeakMap<QueryClient, Map<string,number>>();
function trimTrendCache(cache: QueryClient, prefix: readonly unknown[]) {
  const access = trendAccess.get(cache) ?? new Map<string,number>(); trendAccess.set(cache,access);
  const entries = cache.getQueryCache().findAll({ queryKey: [...prefix, 'trend-tile'] });
  for(const entry of entries) if(entry.getObserversCount()) access.set(entry.queryHash,Date.now());
  for(const key of access.keys()) if(!cache.getQueryCache().get(key)) access.delete(key);
  entries.sort((a,b)=>(access.get(b.queryHash)??b.state.dataUpdatedAt)-(access.get(a.queryHash)??a.state.dataUpdatedAt));
  let bytes = 0, points = 0;
  for (const entry of entries) {
    const data = entry.state.data as MonitoringAnalyticResult | undefined;
    if (!data) continue;
    bytes += JSON.stringify(data).length * 2;
    points += (data.trends ?? []).reduce((n, s) => n + s.buckets.reduce((m, b) => m + b.points.length + b.states.length, 0), 0);
    if ((bytes > 32 * 1024 * 1024 || points > 80_000) && !entry.getObserversCount()) cache.removeQueries({ queryKey: entry.queryKey, exact: true });
  }
}
export function useTrendTiles(query: MonitoringQuery, range: MonitoringRange, width: number, curves: number) {
  const { client, scope, prefix, available } = useMonitoring(), cache = useQueryClient();
  const plan = viewportTiles(range, width, curves);
  const queries = plan.tiles.map(tile => ({ ...query, view_range: tile.range, plot_width_px: plan.plotWidth, plot_point_budget: plan.budget }));
  const keys = queries.map(input => [...prefix, 'trend-tile', input]);
  const readyForPrefetch = plan.tiles.every((tile, i) => !tile.visible || cache.getQueryData<MonitoringAnalyticResult>(keys[i]!)?.state === 'ready');
  const results = useQueries({ queries: queries.map((input, index) => ({
    queryKey: keys[index]!, enabled: available && (plan.tiles[index]!.visible || readyForPrefetch),
    queryFn: ({signal}: {signal: AbortSignal}) => querySlot(signal, () => client.monitoringQuery(scope, 'trends', input, signal)),
    staleTime: Infinity, gcTime: 120000, retry: false,
    refetchInterval: (q: Query<MonitoringAnalyticResult>) => q.state.data?.state === 'pending' ? 1500 : false,
  })) });
  const visible = results.filter((_, index) => plan.tiles[index]!.visible);
  const complete = visible.every(r => r.data?.state === 'ready');
  const [previous, setPrevious] = useState<{range: MonitoringRange; series: MonitoringTrendSeries[]; at: number; stamp:string} | null>(null);
  const signature = JSON.stringify(keys.filter((_, index) => plan.tiles[index]!.visible));
  const completedAt = complete ? Math.max(...visible.map(r => r.dataUpdatedAt)) : 0;
  const stamp = `${signature}:${JSON.stringify(range)}:${completedAt}`;
  if (complete && previous?.stamp !== stamp) setPrevious({range, series:mergeTrendTiles(visible.map(r => r.data!.trends ?? [])), at:completedAt,stamp});
  const changedAt = Math.max(...results.map(r=>r.dataUpdatedAt));
  const prefixStamp=JSON.stringify(prefix);
  useEffect(() => { trimTrendCache(cache, JSON.parse(prefixStamp)); }, [cache, prefixStamp, changedAt,signature]);
  const error = visible.find(r => r.error)?.error ?? null;
  const pending = visible.find(r => r.data?.state === 'pending')?.data;
  return { display: previous, complete, error, pending, fetching: visible.some(r => r.isFetching),
    failures: plan.tiles.filter((t,i)=>t.visible&&results[i]?.error).map(t=>t.range),
    tiles: plan.tiles.filter(t => t.visible), cached: visible.filter(r => r.data?.state === 'ready').length,
    retry: () => { for (const r of visible) void r.refetch(); },
  };
}
