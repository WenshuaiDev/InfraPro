import { ClassificationBulk } from '../classification/bulk';
import { useEffect, useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ChevronLeft, ChevronRight, MapPin, Plus } from 'lucide-react';
import type { DeviceFilter, DeviceInstallation } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import { cn } from '@infrapro/ui/lib/utils';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from './context';
import { DeviceContent } from './detail-page';
import { DeviceEditor } from './device-editor';
import type { TenantDeviceSearch } from './search';
import { WorkspaceFilters } from './workspace-filters';

type Props = {
  id?: string;
  search: TenantDeviceSearch;
  onSelect: (id: string | undefined, search: TenantDeviceSearch) => void;
  onFilter: (search: TenantDeviceSearch) => void;
  onDetail: (search: TenantDeviceSearch) => void;
};

export function TenantDeviceWorkspace(props: Props) {
  const { canRead } = useDevices();
  if (!canRead) return <ErrorState title="没有设备查看权限" message="请联系当前范围的管理员配置相应角色。" />;
  return <Workspace {...props} />;
}

function Workspace({ id, search, onSelect, onFilter, onDetail }: Props) {
  const { scope, client, prefix, paths, available, canManage } = useDevices();
  const cache = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [pageSize, setPageSize] = useState(25);
  const listRef = useRef<HTMLElement>(null);
  const detailRef = useRef<HTMLDivElement>(null);
  const previousId = useRef(id);
  const listScroll = useRef(0);
  const filter: DeviceFilter = {category_id:search.category,category_scope:search.category_scope,category_group:search.category_group, q: search.q, manufacturer_id: search.manufacturer, product_id: search.product, source: search.source, status: search.status, installation: search.installation, facility_id: search.facility };
  const list = useCursorList({ queryKey: [...prefix, 'workspace-list', filter, pageSize], load: (cursor, signal) => client.devices(scope, { ...filter, cursor, limit: pageSize }, signal) });
  const activeId = id ?? list.items[0]?.id;
  const detail = useQuery({ queryKey: [...prefix, 'detail', activeId], enabled: !!activeId, queryFn: ({ signal }) => client.device(scope, activeId!, signal) });
  const filtered = !!(search.category || search.category_group || search.q || search.manufacturer || search.product || search.status || search.installation || search.facility || search.source && search.source !== 'all');
  const reset = () => onFilter({ tab: search.tab });

  useEffect(() => {
    if (window.matchMedia('(max-width: 1279px)').matches) {
      if (id) {
        window.scrollTo({ top: 0, behavior: 'instant' });
        detailRef.current?.focus({ preventScroll: true });
      } else if (previousId.current) {
        window.scrollTo({ top: listScroll.current, behavior: 'instant' });
        listRef.current?.querySelector<HTMLElement>(`[data-device-id="${previousId.current}"]`)?.focus({ preventScroll: true });
      }
    }
    previousId.current = id;
  }, [id]);

  return <section className="min-w-0 space-y-5">
    <PageHeader title="设备台账" description="查找设备，维护档案、输出与安装记录。" actions={canManage && <><ClassificationBulk scope={scope} kind="device" disabled={!available}/><Button id="create-device" disabled={!available} onClick={() => setCreating(true)}><Plus aria-hidden="true" />登记设备</Button></>} />
    <div className={cn(id && 'hidden xl:block')}><WorkspaceFilters search={search} onChange={(next) => onFilter({ ...next, output: undefined })} /></div>
    <div className="grid min-w-0 gap-5 xl:h-[calc(100dvh-18rem)] xl:min-h-[32rem] xl:grid-cols-[18rem_minmax(0,1fr)]">
      <aside aria-label="设备列表" className={cn('flex min-h-0 min-w-0 flex-col rounded-lg border', id && 'hidden xl:flex')}>
        <div className="flex items-center justify-between border-b px-4 py-3"><h2 className="text-sm font-semibold">{filtered ? '筛选结果' : '全部设备'}</h2><span className="text-xs text-muted-foreground" role="status">本页 {list.items.length} 台{list.isFetching && !list.isPending ? ' · 更新中' : ''}</span></div>
        <nav ref={listRef} aria-label="选择设备" aria-busy={list.isFetching} className="min-h-0 flex-1 overflow-y-auto p-2" onKeyDown={(event) => {
          if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
          const links = [...event.currentTarget.querySelectorAll<HTMLAnchorElement>('a[data-device-id]')];
          const index = links.indexOf(event.target as HTMLAnchorElement);
          if (index < 0) return;
          event.preventDefault();
          const next = event.key === 'Home' ? 0 : event.key === 'End' ? links.length - 1 : Math.max(0, Math.min(links.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)));
          links[next]?.focus();
        }}>
          {list.isPending ? <LoadingState label="正在加载设备…" /> : list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !list.items.length ? <EmptyState title={filtered ? '没有符合条件的设备' : '尚未登记设备'} description={filtered ? '调整筛选条件后再试。' : '登记设备后，在这里查看和维护。'}>{filtered && <Button variant="outline" size="sm" onClick={reset}>清空筛选</Button>}</EmptyState> : <ul className="space-y-1">{list.items.map((device) => <li key={device.id}>
            <Button asChild variant="ghost" className={cn('h-auto w-full justify-start rounded-md px-3 py-3 text-left whitespace-normal', activeId === device.id && 'bg-accent text-accent-foreground')}>
              <Link to={paths.device(device.id)} search={{ ...search, output: undefined }} resetScroll={false} data-device-id={device.id} aria-label={device.name} aria-current={activeId === device.id ? 'page' : undefined} onClick={() => { listScroll.current = window.scrollY; }}>
                <span className="min-w-0 flex-1 space-y-1.5">
                  <span className="flex items-start justify-between gap-2"><span className="line-clamp-2 break-words font-medium leading-5">{device.name}</span>{device.status === 'disabled' && <Badge variant="outline" className="shrink-0 text-xs">停用</Badge>}</span>
                  <span className="block truncate font-mono text-xs font-normal text-muted-foreground">{device.code}</span>
                  <span className="block truncate text-xs font-normal text-muted-foreground">{device.product.name} · v{device.product.number}</span>
                  <span className="flex items-center gap-1.5 text-xs font-normal text-muted-foreground"><MapPin className="size-3.5 shrink-0" aria-hidden="true" /><span className="truncate">{installationLabel(device.current_installation)}</span></span>
                </span>
              </Link>
            </Button>
          </li>)}</ul>}
        </nav>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t p-3 text-xs">
          <Select value={String(pageSize)} onValueChange={(value) => setPageSize(Number(value))} disabled={list.isFetching}><SelectTrigger aria-label="设备每页条数" className="h-9 w-24"><SelectValue /></SelectTrigger><SelectContent>{[25, 50, 100].map((size) => <SelectItem key={size} value={String(size)}>{size} 台 / 页</SelectItem>)}</SelectContent></Select>
          <div className="flex items-center gap-1" aria-label="设备分页"><Button variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label="上一页设备" disabled={!list.page || list.isFetching} onClick={() => list.go(list.page - 1)}><ChevronLeft aria-hidden="true" /></Button><span className="whitespace-nowrap tabular-nums" role="status">第 {list.page + 1} 页</span><Button variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label="下一页设备" disabled={!list.hasNext || list.isFetching} onClick={() => void list.next()}><ChevronRight aria-hidden="true" /></Button></div>
        </div>
      </aside>
      <div ref={detailRef} tabIndex={-1} role="region" aria-label="设备详情工作区" className={cn('min-h-0 min-w-0 space-y-4 focus:outline-none xl:overflow-y-auto xl:pr-1', !id && 'hidden xl:block')}>
        {id && <Button variant="ghost" size="sm" className="xl:hidden" onClick={() => onSelect(undefined, { ...search, output: undefined })}><ArrowLeft aria-hidden="true" />返回设备列表</Button>}
        {!activeId && list.isPending ? <LoadingState label="正在加载设备…" /> : !activeId ? <div className="flex h-full min-h-80 items-center justify-center rounded-lg border border-dashed"><EmptyState title={filtered ? '没有符合条件的设备' : '选择一台设备'} description={filtered ? '调整左侧列表的筛选条件。' : '设备档案、输出与安装记录将在这里展示。'} /></div> : detail.isPending ? <LoadingState label="正在加载设备详情…" /> : detail.error || !detail.data ? <ErrorState message={detail.error?.message ?? '无法读取设备'} onRetry={() => void detail.refetch()} /> : <>
          {id && !list.isPending && !list.error && !list.items.some((device) => device.id === id) && <FormNotice>当前设备不在本页列表中。你可以继续查看，或返回列表调整筛选。</FormNotice>}
          <DeviceContent key={activeId} device={detail.data} embedded search={search} onSearch={(next) => { const value = { ...search, tab: next.tab, output: next.output }; if (id) onDetail(value); else onSelect(activeId, value); }} onDeleted={() => onSelect(undefined, { ...search, output: undefined })} />
        </>}
      </div>
    </div>
    <ResponsivePanel open={creating} onOpenChange={setCreating} title="登记设备" description="采用一个已发布产品版本，固定输出将一并生成。" fallbackFocusSelector="#create-device">
      {creating && <DeviceEditor done={(result) => { void cache.invalidateQueries({ queryKey: prefix }); setCreating(false); onSelect(result.id, { tab: 'details' }); }} />}
    </ResponsivePanel>
  </section>;
}

function installationLabel(value: DeviceInstallation | null) {
  if (!value) return '无当前安装记录';
  if (value.visibility === 'restricted') return '安装目标详情受限';
  return value.target_kind === 'location' ? `独立位置 · ${value.location_description}` : `${value.target_name || '历史安装目标'}${value.target_status === 'retired' ? '（已退役）' : ''}`;
}
