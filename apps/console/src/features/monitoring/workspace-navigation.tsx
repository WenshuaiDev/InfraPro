import { useEffect, useId, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { ResourceTreeRef } from '@infrapro/api-client';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { ProjectResources, TenantResources } from '../resources/context';
import { ResourceTree } from '../resources/resource-tree';
import { navigationPath, useResourceTree } from '../resources/resource-tree-query';
import { MonitoringError } from './common';
import { useMonitoring } from './context';
import type { SelectedChannel } from './channel-picker';

// eslint-disable-next-line react-refresh/only-export-components
export function useCurrentChannel(point?: string, channel?: string) {
  const m = useMonitoring();
  return useQuery({
    queryKey: [...m.prefix, 'current-channel', point, channel], enabled: !!channel && m.available && m.canRead && m.canNavigate, retry: false,
    queryFn: async ({ signal }) => {
      const channels = await m.client.monitoringDirectory(m.scope, { channel_id: channel }, signal);
      const current = channels.items[0];
      if (!current || point && current.point_id !== point) throw new Error('此通道在当前工作空间不可读，请从目录选择其他通道。');
      return current;
    },
  });
}

type Props = { current?: SelectedChannel & { facility_id: string }; initialPoint?: string; select: (point: string | undefined, channel: string) => void };
export function MonitoringNavigator(props: Props) {
  const { scope, canNavigate } = useMonitoring();
  if (!canNavigate) return <p className="p-4 text-sm text-muted-foreground">查看目录需要工程资源查看权限。</p>;
  const Boundary = scope.kind === 'tenant' ? TenantResources : ProjectResources;
  return <Boundary><NavigationTree {...props} /></Boundary>;
}
function NavigationTree({ current, initialPoint, select }: Props) {
  const m = useMonitoring(), searchID = useId();
  const [search, setSearch] = useState(''), [debounced, setDebounced] = useState('');
  useEffect(() => { const timer = setTimeout(() => setDebounced(search.trim()), 200); return () => clearTimeout(timer); }, [search]);
  const project = m.scope.kind === 'project';
  const roots = useResourceTree({ q: debounced || undefined, status: 'all' }, m.available);
  const point = useQuery({ queryKey: [...m.prefix, 'navigation-point', initialPoint], queryFn: ({ signal }) => m.client.monitoringPoints(m.scope, { point_id: initialPoint }, signal), enabled: !current && !!initialPoint && m.available, retry: false });
  const facility = current?.facility_id ?? point.data?.items[0]?.facility_id;
  const targetPoint = current?.point_id ?? point.data?.items[0]?.id;
  const detail = useQuery({
    queryKey: [...m.prefix, 'navigation-path', facility, targetPoint, current?.id], enabled: !!facility && (!!current || !!targetPoint) && m.available, retry: false,
    queryFn: ({ signal }) => m.client.resourceDetail(m.scope, current ? { kind: 'channel', facility, point: targetPoint, id: current.id } : { kind: 'point', facility, id: targetPoint }, signal),
  });
  const path = detail.data ? navigationPath(detail.data) : [];
  const choose = (node: ResourceTreeRef) => { if (node.kind === 'channel') select(node.parent_kind === 'point' ? node.parent_id ?? undefined : undefined, node.id); };
  return <>
    <div className="space-y-2 border-b p-3"><Label htmlFor={searchID} className="sr-only">{project ? '搜索项目设施' : '搜索设施、构件、测点或通道'}</Label><Input id={searchID} placeholder={project ? '搜索项目设施' : '搜索设施、构件、测点或通道'} value={search} onChange={e => setSearch(e.target.value)} /></div>
    <ResourceTree title="设施构件树" query={roots} searching={!project && !!debounced} filtered={!!debounced} selected={current?.id} path={project && debounced ? [] : path} select={choose} expandOnSelect />
    {(point.error || detail.error) && <div className="p-3"><MonitoringError error={point.error ?? detail.error} retry={() => { void point.refetch(); void detail.refetch(); }} /></div>}
  </>;
}
