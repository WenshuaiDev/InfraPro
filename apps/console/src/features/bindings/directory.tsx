import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Permissions as P, type DeviceBindingFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListFilter } from '@infrapro/ui/patterns/list-controls';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from '../devices/context';
import { stateLabels, type BindingRow } from './model';

// eslint-disable-next-line react-refresh/only-export-components
export function useBindingDirectory(filter: DeviceBindingFilter) {
  const { client, scope, prefix, can } = useDevices();
  return useCursorList({ queryKey: [...prefix, 'binding-directory', filter], enabled: scope.kind === 'tenant' && can(P.TenantEngineeringResourceRead),
    load: (cursor, signal) => { if (scope.kind !== 'tenant') throw new Error('请在租户工作台查看绑定目录'); return client.deviceBindingDirectory(scope, { ...filter, cursor, limit: 10 }, signal); } });
}
export function BindingFilters({ search, state, changeSearch, changeState, counts }: { search: string; state?: BindingRow['state']; changeSearch: (v: string) => void; changeState: (v?: BindingRow['state']) => void; counts?: Record<string, number> }) {
  return <div className="flex flex-wrap items-center gap-3"><DirectorySearch label="搜索设施、测点或业务通道" value={search} onChange={changeSearch} /><ListFilter label="绑定状态" caption="状态" value={state ?? 'all'} onChange={v => changeState(v === 'all' ? undefined : v as BindingRow['state'])} options={Object.entries(stateLabels).map(([value, label]) => ({ value, label: `${label}${counts ? ` · ${counts[value] ?? 0}` : ''}` }))} /></div>;
}
export function FacilityFilter({ value, choose }: { value?: string; choose: (id?: string) => void }) {
  const { client, scope, prefix } = useDevices();
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const selected = useQuery({ queryKey: [...prefix, 'binding-facility', value], enabled: !!value, queryFn: ({ signal }) => client.resourceDetail(scope, { kind: 'facility', id: value, facility: value }, signal) });
  const list = useInfiniteQuery({ queryKey: [...prefix, 'binding-facilities', search], enabled: open, initialPageParam: '', queryFn: ({ pageParam, signal }) => client.resourceTree(scope.tenantId, { q: search, status: 'all', limit: 15, cursor: pageParam }, signal), getNextPageParam: page => page.next_cursor || undefined });
  return <details open={open} onToggle={event => setOpen(event.currentTarget.open)} className="rounded-lg border bg-card"><summary className="min-h-11 cursor-pointer px-4 py-3 text-sm">设施范围：{value ? selected.data?.name ?? '指定设施' : '全部设施'}</summary><div className="space-y-3 border-t p-4"><DirectorySearch label="搜索设施范围" value={search} onChange={setSearch} /><Button className="min-h-11" variant={!value ? 'secondary' : 'outline'} onClick={() => { choose(); setOpen(false); }}>全部设施</Button>{list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{list.data?.pages.flatMap(page => page.items).map(facility => <Button key={facility.id} variant={value === facility.id ? 'secondary' : 'outline'} className="h-auto min-h-11 justify-start whitespace-normal text-left" onClick={() => { choose(facility.id); setOpen(false); }}>{facility.name} · {facility.code}</Button>)}</div>}{list.hasNextPage && <Button variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多设施</Button>}</div></details>;
}
