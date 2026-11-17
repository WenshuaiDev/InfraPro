import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Permissions as P, type DeviceBindingFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Popover, PopoverContent, PopoverTrigger } from '@infrapro/ui/components/popover';
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
// eslint-disable-next-line react-refresh/only-export-components
export function useBindingTreeDirectory(filter: DeviceBindingFilter) {
  const { client, scope, prefix, can } = useDevices();
  return useInfiniteQuery({ queryKey: [...prefix, 'binding-tree-directory', filter], enabled: scope.kind === 'tenant' && can(P.TenantEngineeringResourceRead), initialPageParam: '',
    queryFn: ({ pageParam, signal }) => { if (scope.kind !== 'tenant') throw new Error('请在租户工作台查看绑定目录'); return client.deviceBindingDirectory(scope, { ...filter, cursor: pageParam, limit: 50 }, signal); },
    getNextPageParam: page => page.next_cursor || undefined });
}
export function BindingFilters({ search, state, changeSearch, changeState, counts }: { search: string; state?: BindingRow['state']; changeSearch: (v: string) => void; changeState: (v?: BindingRow['state']) => void; counts?: Record<string, number> }) {
  return <div className="flex flex-wrap items-center gap-2"><div className="min-w-0 flex-1 [&>div]:w-full"><DirectorySearch label="搜索设施、构件、测点或业务通道" value={search} onChange={changeSearch} /></div><ListFilter label="绑定状态" caption="状态" value={state ?? 'all'} onChange={v => changeState(v === 'all' ? undefined : v as BindingRow['state'])} options={Object.entries(stateLabels).map(([value, label]) => ({ value, label: `${label}${counts ? ` · ${counts[value] ?? 0}` : ''}` }))} /></div>;
}
export function FacilityFilter({ value, choose }: { value?: string; choose: (id?: string) => void }) {
  const { client, scope, prefix } = useDevices();
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const selected = useQuery({ queryKey: [...prefix, 'binding-facility', value], enabled: !!value, queryFn: ({ signal }) => client.resourceDetail(scope, { kind: 'facility', id: value, facility: value }, signal) });
  const list = useInfiniteQuery({ queryKey: [...prefix, 'binding-facilities', search], enabled: open, initialPageParam: '', queryFn: ({ pageParam, signal }) => client.resourceTree(scope.tenantId, { q: search, status: 'all', limit: 15, cursor: pageParam }, signal), getNextPageParam: page => page.next_cursor || undefined });
  return <Popover open={open} onOpenChange={setOpen}><PopoverTrigger render={<Button variant="outline" className="h-auto min-h-10 max-w-full whitespace-normal text-left" />}>设施范围：{value ? selected.data?.name ?? '指定设施' : '全部设施'}</PopoverTrigger><PopoverContent className="w-80"><div className="space-y-3 overflow-y-auto p-4"><h2 className="text-sm font-semibold">选择设施范围</h2><DirectorySearch label="搜索设施范围" value={search} onChange={setSearch} /><Button className="min-h-10" variant={!value ? 'secondary' : 'outline'} onClick={() => { choose(); setOpen(false); }}>全部设施</Button>{list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : <div className="grid gap-1">{list.data?.pages.flatMap(page => page.items).map(facility => <Button key={facility.id} variant={value === facility.id ? 'secondary' : 'ghost'} className="h-auto min-h-10 justify-start whitespace-normal text-left" onClick={() => { choose(facility.id); setOpen(false); }}>{facility.name} · {facility.code}</Button>)}</div>}{list.hasNextPage && <Button variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多设施</Button>}</div></PopoverContent></Popover>;
}
