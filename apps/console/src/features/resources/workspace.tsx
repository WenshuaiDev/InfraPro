import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ApiError, type ResourceTreeRef } from '@infrapro/api-client';
import { ArrowLeft, Plus } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListFilter } from '@infrapro/ui/patterns/list-controls';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useResources } from './context';
import { ResourceEditor } from './editor';
import { ResourceHistory } from './history';
import { ResourceTree } from './resource-tree';
import { navigationPath, navigationRef, useResourceTree } from './resource-tree-query';
import { WorkspaceDetail } from './workspace-detail';
import { WorkspaceContext } from './workspace-context';
import { resourceSelection, selectedTarget, treeFilter, type FacilityWorkspaceSearch } from './workspace-search';

type Props = { facilityId?: string; search: FacilityWorkspaceSearch; navigate: (id: string | undefined, search: FacilityWorkspaceSearch, replace?: boolean) => void };
export function FacilityWorkspace(props: Props) {
  const { canRead, prefix } = useResources();
  if (!canRead) return <ErrorState title="没有工程资源查看权限" message="请联系租户管理员配置相应角色。" />;
  return <Workspace key={JSON.stringify(prefix)} {...props} />;
}

function Workspace({ facilityId, search, navigate }: Props) {
  const { client, scope, prefix, canCreateFacility, canHistory, available } = useResources();
  const [creating, setCreating] = useState(false);
  const [lastSelection, setLastSelection] = useState<string>();
  const detailRef = useRef<HTMLDivElement>(null);
  const treeRef = useRef<HTMLElement>(null);
  const mobileScroll = useRef(0);
  const showDetail = !!facilityId && !search.directory;
  const previousDetail = useRef(showDetail);
  const roots = useResourceTree(treeFilter(search));
  const first = roots.items[0];
  const activeFacility = facilityId ?? first?.facility_id;
  const target = activeFacility ? selectedTarget(activeFacility, facilityId ? search : first ? resourceSelection(first) : {}) : undefined;
  const activeId = target?.id;
  const detail = useQuery({
    queryKey: [...prefix, 'detail', target?.kind, activeFacility, target?.point, activeId],
    queryFn: ({ signal }) => client.resourceDetail(scope, target!, signal),
    enabled: !!target && !(first?.deleted_at && !facilityId), retry: false,
  });
  const facility = useQuery({
    queryKey: [...prefix, 'detail', 'facility', activeFacility],
    queryFn: ({ signal }) => client.resourceDetail(scope, { kind: 'facility', id: activeFacility }, signal),
    enabled: !!activeFacility && target?.kind !== 'facility', retry: false,
  });
  const facilityData = target?.kind === 'facility' ? detail.data : facility.data;
  const selected = detail.data;
  const missing = detail.error instanceof ApiError && detail.error.status === 404;
  const contextPath = useQuery({
    queryKey: [...prefix, 'tree', 'path', activeFacility, activeId],
    queryFn: ({ signal }) => client.resourceTree(scope, { facility_id: activeFacility, node_id: activeId, status: 'all' }, signal),
    enabled: scope.kind === 'project' && missing && !!activeFacility && !!activeId,
    retry: false,
  });
  const contextNode = missing ? contextPath.data?.items.find((node) => node.id === activeId && node.kind === target?.kind) : undefined;
  const filters = { q: search.q, type: search.type, status: search.status, deleted: search.deleted };
  const select = (node: ResourceTreeRef) => {
    if (!showDetail) mobileScroll.current = window.scrollY;
    setLastSelection(node.id);
    navigate(node.facility_id, { ...filters, ...resourceSelection(node), ...(scope.kind === 'tenant' && search.view === 'bindings' && !node.deleted_at && (node.kind === 'facility' || node.kind === 'point') ? { view: 'bindings' as const } : {}) });
  };
  const activeView = search.view ?? (!facilityId && first?.deleted_at ? 'changes' : undefined);
  const view = (next: FacilityWorkspaceSearch['view']) => {
    if (selected) navigate(selected.facility_id ?? selected.id, { ...filters, ...resourceSelection(navigationRef(selected)), view: next });
  };
  const filtered = !!(search.q?.trim() || search.type || search.status || search.deleted);
  const clear = () => navigate(facilityId, { ...search, q: undefined, type: undefined, status: undefined, deleted: undefined }, true);
  const type = facilityData?.definitions.find((v) => v.id === facilityData.type_version_id);
  const displayed = selected ?? contextNode;
  const outsideFilter = !!displayed && (search.deleted === 'only' || !!search.type && type?.definition_id !== search.type || !!search.status && (search.q?.trim() ? displayed.status : facilityData?.status) !== search.status || !!search.q?.trim() && !`${displayed.name}\n${displayed.code}`.toLowerCase().includes(search.q.trim().toLowerCase()));
  const path = !outsideFilter && !search.q?.trim() ? selected && !detail.error ? navigationPath(selected) : contextNode ? [...contextNode.ancestors, contextNode] : [] : [];
  const selectedStamp = `${activeFacility}/${activeId}`;
  useEffect(() => {
    if (window.matchMedia('(max-width: 1023px)').matches) {
      if (showDetail) { window.scrollTo({ top: 0, behavior: 'instant' }); detailRef.current?.focus({ preventScroll: true }); }
      else if (previousDetail.current) {
        window.scrollTo({ top: mobileScroll.current, behavior: 'instant' });
        treeRef.current?.querySelector<HTMLElement>(`[data-node="${lastSelection}"]`)?.focus({ preventScroll: true });
      }
    }
    previousDetail.current = showDetail;
  }, [showDetail, selectedStamp, lastSelection]);
  // Authorization errors must not leave a previously cached detail on screen.
  const restricted = [roots.error, detail.error, contextPath.error, target?.kind !== 'facility' ? facility.error : null].find((error) => error instanceof ApiError && [401, 403].includes(error.status ?? 0));
  if (restricted) return <ErrorState title="当前资源不可访问" message={restricted.message} />;
  const failure = detail.error?.message ?? (target?.kind !== 'facility' ? facility.error?.message : undefined);
  const blocked = !!detail.error || target?.kind !== 'facility' && !!facility.error || !facilityData;
  const retry = () => { void detail.refetch(); if (target?.kind !== 'facility') void facility.refetch(); void roots.refetch(); };
  return <section className="min-w-0 space-y-5">
    <PageHeader title={scope.kind === 'project' ? '项目设施' : '设施管理'} description={scope.kind === 'project' ? '查看和维护本项目获准使用的设施、构件、断面、孔位、测点与通道。' : '在同一处维护设施、构件、断面、孔位、测点与通道。'} actions={canCreateFacility && <Button id="create-facility" disabled={!available} onClick={() => setCreating(true)}><Plus aria-hidden="true" />新增设施</Button>} />
    <div className={cn('flex flex-wrap items-center gap-2', showDetail && 'hidden lg:flex')}>
      <DirectorySearch label="搜索设施、构件、断面、孔位、测点或通道" value={search.q ?? ''} onChange={(q) => navigate(facilityId, { ...search, q: q || undefined }, true)} />
      <ListFilter label="设施类型" caption="类型" value={search.type ?? ''} onChange={(type) => navigate(facilityId, { ...search, type: type || undefined }, true)} options={[{ value: '', label: '全部类型' }, ...(roots.summary?.types ?? []).map((type) => ({ value: type.id, label: type.name }))]} />
      <ListFilter label={search.q?.trim() ? '匹配资源状态' : '设施状态'} caption="状态" value={search.deleted ? 'deleted' : search.status ?? 'all'} onChange={(status) => navigate(facilityId, { ...search, status: status === 'normal' || status === 'retired' ? status : undefined, deleted: status === 'deleted' ? 'only' : undefined }, true)} options={[{ value: 'all', label: '全部未删除' }, { value: 'normal', label: '正常' }, { value: 'retired', label: '已退役' }, ...(scope.kind === 'tenant' ? [{ value: 'deleted', label: '已删除记录' }] : [])]} />
      {filtered && <Button variant="ghost" size="sm" onClick={clear}>重置筛选</Button>}
      <span className="ml-auto text-xs text-muted-foreground" role="status">{roots.summary ? search.q?.trim() && !search.deleted ? `匹配 ${roots.summary.total} 个资源，保留祖先路径` : `共 ${roots.summary.total} 座设施` : '正在读取…'}{roots.isFetching && !roots.isPending && ' · 更新中'}</span>
    </div>
    <div className="grid min-w-0 gap-4 lg:h-[calc(100dvh-18rem)] lg:min-h-[32rem] lg:grid-cols-[18rem_minmax(0,1fr)]">
      <aside ref={treeRef} aria-label="设施资源树" className={cn('flex min-h-[24rem] min-w-0 flex-col overflow-hidden rounded-lg border lg:min-h-0', showDetail && 'hidden lg:flex')}>
        <ResourceTree query={roots} searching={!!search.q?.trim() && !search.deleted} filtered={filtered} selected={activeId} path={path} select={select} />
      </aside>
      <div ref={detailRef} id="resource-workspace-anchor" tabIndex={-1} aria-label="资源详情工作区" className={cn('min-h-0 min-w-0 space-y-3 outline-none lg:overflow-y-auto lg:pr-1', !showDetail && 'hidden lg:block')}>
        <Button variant="ghost" className="-ml-3 lg:hidden" onClick={() => { if (activeId) setLastSelection(activeId); navigate(activeFacility, { ...search, directory: true }); }}><ArrowLeft aria-hidden="true" />返回资源目录</Button>
        {outsideFilter && <FormNotice>正在查看的对象不符合当前目录筛选。<Button size="sm" variant="link" onClick={clear}>清除目录筛选</Button></FormNotice>}
        {!activeFacility ? <EmptyState title="选择一个资源开始查看" description="从左侧目录打开设施，或搜索下级构件、测点与业务通道。" /> : (scope.kind === 'tenant' && activeView === 'changes' && (missing || !selected && !!first?.deleted_at)) ? <article className="space-y-4 rounded-lg border p-4"><h2 className="text-xl font-semibold">{first?.id === activeFacility ? first.name : '设施变更记录'}</h2><p className="text-sm text-muted-foreground">已删除资源保留身份和变更历史。</p>{canHistory && <ResourceHistory facility={activeFacility} />}</article> : contextNode ? <WorkspaceContext key={contextNode.id} node={contextNode} select={select} /> : missing && scope.kind === 'project' && contextPath.isPending ? <LoadingState label="正在定位已授权资源…" /> : missing && contextPath.error ? <ErrorState message={contextPath.error.message} onRetry={() => void contextPath.refetch()} /> : missing ? <ErrorState title="资源已不可访问" message="资源不存在或不可访问。" onRetry={retry} /> : !selected && detail.error ? <ErrorState message={detail.error.message} onRetry={retry} /> : !selected ? <LoadingState label="正在读取资源详情…" /> : <WorkspaceDetail key={`${selected.kind}:${selected.id}`} resource={selected} facility={facilityData} view={activeView} onView={view} select={select} onDeleted={(parent) => { if (parent) select(parent); else navigate(undefined, filters); }} blocked={blocked} failure={failure} retry={retry} />}
      </div>
    </div>
    <ResponsivePanel open={creating} onOpenChange={setCreating} title="新增设施" description="选择已发布的设施类型，填写工程资料。" fallbackFocusSelector="#create-facility">
      {creating && <ResourceEditor target={{ kind: 'facility' }} done={(result) => { setCreating(false); navigate(result.id, filters); }} />}
    </ResponsivePanel>
  </section>;
}
