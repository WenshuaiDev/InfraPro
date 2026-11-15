import { FacilityBindings } from '../bindings/facility';
import { ObjectHistory } from '../audit/object-history';
import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import type { ResourceDetail, ResourceKind, ResourceTarget, ResourceTreeNode, ResourceTreeRef } from '@infrapro/api-client';
import { ChevronDown, ChevronRight, FolderTree, Pencil, Plus } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListFilter } from '@infrapro/ui/patterns/list-controls';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ChannelSource } from '../devices/source-panel';
import { FacilityUsage } from '../resource-access/pages';
import { useResources } from './context';
import { OwnerDerivedChannels } from './owner-derived-channels';
import { ResourceEditor } from './editor';
import { ResourceHistory } from './history';
import { LifecycleForm } from './lifecycle';
import { ObjectActions, ResourceInfo } from './resource-details';
import { ResourceIcon, TreeMore } from './resource-tree';
import { resourceLabels } from './resource-labels';
import { childFilter, navigationPath, navigationRef, useResourceTree } from './resource-tree-query';
import type { FacilityWorkspaceSearch } from './workspace-search';

type Panel = { mode: 'create' | 'edit' | 'retire' | 'restore' | 'delete'; target: ResourceTarget; resource?: ResourceDetail; owner?: ResourceDetail };
type Props = { resource: ResourceDetail; facility?: ResourceDetail; view: FacilityWorkspaceSearch['view']; onView: (view: FacilityWorkspaceSearch['view']) => void; select: (node: ResourceTreeRef) => void; onDeleted: (parent?: ResourceTreeRef) => void; blocked: boolean; failure?: string; retry: () => void };
const modeLabels = { create: '新增', edit: '编辑', retire: '退役', restore: '恢复', delete: '删除' };
const tabClass = 'h-full rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 data-[state=active]:border-foreground data-[state=active]:shadow-none';

export function WorkspaceDetail({ resource, facility, view, onView, select, onDeleted, blocked, failure, retry }: Props) {
  const { scope, available, canManage, canHistory, canGovern, canMonitor, canDerivedRead } = useResources();
  const navigate = useNavigate();
  const [panel, setPanel] = useState<Panel>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const node = navigationRef(resource);
  const path = navigationPath(resource);
  const target = { kind: resource.kind, id: resource.id, facility: node.facility_id, point: resource.point_id };
  const children = useResourceTree(childFilter(node), resource.kind !== 'channel');
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState<'normal' | 'retired' | 'all'>('all');
  const filtered = !!query.trim() || status !== 'all';
  const filteredChildren = useResourceTree({ ...childFilter(node), q: query.trim() || undefined, status }, resource.kind !== 'channel' && view === 'children' && filtered);
  const list = filtered ? filteredChildren : children;
  const currentView = view === 'bindings' && (scope.kind !== 'tenant' || resource.deleted_at || resource.kind !== 'facility' && resource.kind !== 'point') || view === 'source' && resource.kind !== 'channel' || view === 'children' && resource.kind === 'channel' || view === 'usage' && resource.kind !== 'facility' ? 'details' : view ?? 'details';
  const type = resource.definitions.find((v) => v.kind === 'type-definitions');
  const category = type?.content.components?.find((c) => c.key === resource.category_key)?.name;
  const disabled = blocked || !canManage || !available;
  const openPanel = (p: Panel, trigger: HTMLElement | null) => { returnFocus.current = trigger; setPanel(p); };
  const create = (kind: 'component' | 'point' | 'channel', trigger: HTMLElement | null) => openPanel({ mode: 'create', target: { kind, facility: node.facility_id, point: kind === 'channel' ? node.id : undefined }, owner: resource }, trigger);
  const canAdd = canManage ? (['component', 'point', 'channel'] as const).filter((kind) => resource.actions[`create_${kind}`]) : [];
  const derived = resource.kind === 'channel' && resource.source_kind === 'derived';
  const derivedPath: string = `${scope.kind === 'project' ? `/t/${scope.tenantId}/p/${scope.projectId}` : `/t/${scope.tenantId}`}/monitoring/derived/${resource.id}`;
  const source = () => derived ? <section className="space-y-3"><h3 className="font-semibold">派生计算</h3><p className="text-sm text-muted-foreground">此通道通过固定版本的规则和输入绑定计算。可在派生通道详情中查看配置、最新输出及历史依据。</p>{canDerivedRead && <Button asChild variant="outline"><Link to={derivedPath}>查看派生计算</Link></Button>}</section> : <ChannelSource target={{ facility_id: node.facility_id, point_id: resource.point_id!, channel_id: resource.id }} />;
  const counts = children.summary?.counts;
  const bindingEntry = scope.kind === 'tenant' && (resource.kind === 'facility' || resource.kind === 'point') && !resource.deleted_at;
  return <>
    <article className="min-w-0 overflow-hidden rounded-lg border bg-card" aria-label="当前资源详情">
      <div className="space-y-4 p-4 sm:p-5">
        <nav aria-label="当前资源路径" className="flex flex-wrap items-center gap-x-1 gap-y-1 text-xs text-muted-foreground"><FolderTree className="mr-1 size-3.5" aria-hidden="true" />{path.map((item, index) => <span className="flex min-w-0 items-center gap-1" key={item.id}>{index > 0 && <ChevronRight className="size-3 shrink-0" aria-hidden="true" />}<button type="button" onClick={() => select(item)} aria-current={item.id === node.id ? 'location' : undefined} className="min-h-8 cursor-pointer rounded text-left break-words hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring">{item.name}</button></span>)}</nav>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-1 items-start gap-3"><ResourceIcon kind={node.kind} /><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h2 className="break-words text-xl font-semibold tracking-tight">{resource.name}</h2><ObjectHistory objectType={node.kind} objectId={resource.id}/><Badge variant="outline" className="font-normal">{resource.status === 'normal' ? '正常' : '已退役'}</Badge></div><p className="mt-1 break-all text-xs text-muted-foreground">{resource.code} · {category ?? resourceLabels[node.kind]}{type && ` · ${type.content.name} V${type.number}`}</p></div></div>
          <div className="flex flex-wrap items-center gap-2">
            {bindingEntry && <Button size="sm" variant="outline" disabled={blocked} onClick={() => onView('bindings')}>设备绑定</Button>}
            {resource.kind!=='channel'&&<Button size="sm" variant="outline" disabled={blocked} onClick={()=>void navigate({to:scope.kind==='project'?'/t/$tenantId/p/$projectId/spatial':'/t/$tenantId/spatial',params:{tenantId:scope.tenantId,...(scope.kind==='project'?{projectId:scope.projectId}:{})},search:{facility:node.facility_id,owner:resource.id,owner_kind:resource.kind}})}>查看空间位置</Button>}
            {canMonitor && (resource.kind === 'point' || resource.kind === 'channel') && <Button size="sm" variant="outline" disabled={blocked || !available} onClick={() => void navigate({ to: scope.kind === 'project' ? '/t/$tenantId/p/$projectId/monitoring' : '/t/$tenantId/monitoring', params: { tenantId: scope.tenantId, ...(scope.kind === 'project' ? { projectId: scope.projectId } : {}) }, search: { point: resource.kind === 'point' ? resource.id : resource.point_id, channel: resource.kind === 'channel' ? resource.id : undefined } })}>查看监测数据</Button>}
            {canManage && <>
            {resource.actions.edit && <Button size="sm" variant="outline" disabled={disabled} onClick={(event) => openPanel({ mode: 'edit', target, resource }, event.currentTarget)}><Pencil className="size-3.5" aria-hidden="true" />编辑资料</Button>}
            {!!canAdd.length && <AddResource kinds={canAdd} disabled={disabled} create={create} />}
            <ObjectActions resource={resource} disabled={disabled} choose={(mode, trigger) => openPanel({ mode, target, resource }, trigger)} />
            </>}
          </div>
        </div>
        {failure && <FormNotice error>暂时无法读取，输入已保留。{failure} <Button size="sm" variant="link" onClick={retry}>重新读取资源</Button></FormNotice>}
        {resource.kind !== 'channel' && <div className={`grid ${resource.kind === 'point' ? 'grid-cols-2' : 'grid-cols-2 sm:grid-cols-4'} divide-x rounded-md border bg-muted/20 py-3`} aria-label="直属资源数量">{(resource.kind === 'point' ? [['channel', '业务通道']] : [['component', '直属构件'], ['point', '直属测点'], ['channel', '直属通道']]).map(([kind, label]) => <div className="px-3 sm:px-4" key={kind}><span className="block text-xs text-muted-foreground sm:inline">{label}</span><span className="text-lg font-semibold tabular-nums sm:ml-3">{counts ? counts[kind as ResourceKind] : '—'}</span></div>)}<div className="px-3 sm:px-4"><span className="block text-xs text-muted-foreground sm:inline">直属合计</span><span className="text-lg font-semibold tabular-nums sm:ml-3">{children.summary?.total ?? '—'}</span></div></div>}
      </div>
      <Tabs value={currentView} onValueChange={(value) => onView(value as FacilityWorkspaceSearch['view'])} className="gap-0">
        <div className="overflow-x-auto border-b px-4 sm:px-5"><TabsList className="h-11 gap-5 rounded-none bg-transparent p-0" aria-label="资源详情视图">
          <TabsTrigger className={tabClass} value="details">基本资料</TabsTrigger>
          {resource.kind !== 'channel' && <TabsTrigger className={tabClass} value="children">{resource.kind === 'point' ? '业务通道' : '下级资源'}</TabsTrigger>}
          {resource.kind === 'channel' && <TabsTrigger className={tabClass} value="source">{derived ? '派生计算' : '数据来源'}</TabsTrigger>}
          {bindingEntry && <TabsTrigger className={tabClass} value="bindings">设备绑定</TabsTrigger>}
          {canGovern && resource.kind === 'facility' && <TabsTrigger className={tabClass} value="usage">项目使用</TabsTrigger>}
          {canHistory && <TabsTrigger className={tabClass} value="changes">{resource.kind === 'facility' ? '设施变更记录' : '变更记录'}</TabsTrigger>}
        </TabsList></div>
        <TabsContent value="details" className="space-y-5 p-4 sm:p-5">
          <ResourceInfo resource={resource} showIdentity={false} />
          {resource.kind !== 'channel' && <OwnerDerivedChannels owner={resource} />}
          {resource.kind === 'channel' ? source() : <div className="space-y-3 border-t pt-4"><div className="flex items-center justify-between gap-2"><h3 className="text-sm font-semibold">直属资源 <span className="ml-1 font-normal text-muted-foreground">{children.summary?.total ?? '—'}</span></h3><Button variant="ghost" size="sm" onClick={() => onView('children')}>查看全部<ChevronRight className="size-3.5" aria-hidden="true" /></Button></div>{children.isPending ? <LoadingState label="正在读取直属资源…" /> : children.error ? <ErrorState message={children.error.message} onRetry={() => void children.refetch()} /> : <ChildrenList items={children.items.slice(0, 4)} select={select} empty={resource.kind === 'point' ? '暂无业务通道' : '暂无直属资源'} />}</div>}
        </TabsContent>
        <TabsContent value="children" className="space-y-4 p-4 sm:p-5">
          <p className="text-sm text-muted-foreground">直属于「{resource.name}」的{resource.kind === 'point' ? '业务通道' : '构件、测点与派生通道'}，点击名称继续向下浏览。</p>
          <div className="flex flex-wrap items-center gap-2"><DirectorySearch label="搜索直属资源名称或编号" value={query} onChange={setQuery} /><ListFilter label="直属资源状态" caption="状态" value={status} onChange={(value) => setStatus(value as typeof status)} options={[{ value: 'all', label: '全部状态' }, { value: 'normal', label: '正常' }, { value: 'retired', label: '已退役' }]} />{filtered && <Button variant="ghost" size="sm" onClick={() => { setQuery(''); setStatus('all'); }}>重置下级筛选</Button>}</div>
          {list.isPending ? <LoadingState /> : list.error && !list.isFetchNextPageError ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : <><ChildrenList items={list.items} select={select} empty={filtered ? '没有符合条件的直属资源' : resource.kind === 'point' ? '暂无业务通道' : '暂无直属资源'} /><div className="text-xs text-muted-foreground" role="status">已显示 {list.items.length} / {list.summary?.total ?? 0} 项</div><TreeMore query={list} /></>}
        </TabsContent>
        {bindingEntry && <TabsContent value="bindings" className="p-4 sm:p-5"><FacilityBindings facility={node.facility_id} point={resource.kind === 'point' ? resource.id : undefined} name={resource.name} blocked={blocked} /></TabsContent>}
        {resource.kind === 'channel' && <TabsContent value="source" className="p-4 sm:p-5">{source()}</TabsContent>}
        {resource.kind === 'facility' && <TabsContent value="usage" className="p-4 sm:p-5">{canGovern ? <FacilityUsage facility={node.facility_id} /> : <ErrorState message="没有项目资源授权查看权限。" />}</TabsContent>}
        <TabsContent value="changes" className="p-4 sm:p-5">{canHistory ? <ResourceHistory facility={node.facility_id} resource={resource.kind === 'facility' ? undefined : { kind: resource.kind, id: resource.id }} /> : <ErrorState message="没有资源历史查看权限。" />}</TabsContent>
      </Tabs>
    </article>
    <ResponsivePanel open={!!panel} onOpenChange={(open) => { if (!open) setPanel(undefined); }} title={panel ? `${modeLabels[panel.mode]}${resourceLabels[panel.target.kind]}` : '资源操作'} description={panel?.mode === 'create' ? `归属：${path.map((item) => item.name).join(' / ')}` : `维护「${resource.name}」的工程资料。`} returnFocus={returnFocus} fallbackFocusSelector="#resource-workspace-anchor">
      {panel && (panel.mode === 'create' || panel.mode === 'edit' ? <ResourceEditor key={`${panel.mode}:${panel.target.kind}:${panel.target.id ?? 'new'}`} blocked={blocked} target={panel.target} initial={panel.resource} facility={facility} owner={panel.owner} done={(result) => { const saved = panel; setPanel(undefined); select({ ...node, id: result.id, facility_id: result.facility_id, kind: saved.target.kind, parent_id: saved.target.kind === 'channel' ? saved.target.point! : node.parent_id, deleted_at: null }); }} /> : <LifecycleForm blocked={blocked} target={panel.target} resource={panel.resource!} structure={facility?.structure_revision} action={panel.mode} done={() => { const deleted = panel.mode === 'delete'; setPanel(undefined); if (deleted) onDeleted(path.at(-2)); }} />)}
    </ResponsivePanel>
  </>;
}

function AddResource({ kinds, disabled, create }: { kinds: ('component' | 'point' | 'channel')[]; disabled: boolean; create: (kind: 'component' | 'point' | 'channel', trigger: HTMLElement | null) => void }) {
  const trigger = useRef<HTMLButtonElement>(null);
  const pending = useRef<(typeof kinds)[number] | undefined>(undefined);
  if (kinds.length === 1) return <Button size="sm" disabled={disabled} onClick={(event) => create(kinds[0]!, event.currentTarget)}><Plus className="size-3.5" aria-hidden="true" />{kinds[0] === 'channel' ? '添加通道' : kinds[0] === 'point' ? '添加测点' : '新增构件'}</Button>;
  return <DropdownMenu><DropdownMenuTrigger asChild><Button ref={trigger} size="sm" disabled={disabled}><Plus className="size-3.5" aria-hidden="true" />添加下级<ChevronDown className="size-3.5" aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent onCloseAutoFocus={(event) => { const kind = pending.current; pending.current = undefined; if (kind) { event.preventDefault(); create(kind, trigger.current); } }}>{kinds.map((kind) => <DropdownMenuItem key={kind} onSelect={() => { pending.current = kind; }}><ResourceIcon kind={kind} small />{kind === 'component' ? '新增构件' : '添加测点'}</DropdownMenuItem>)}</DropdownMenuContent></DropdownMenu>;
}

function ChildrenList({ items, select, empty }: { items: ResourceTreeNode[]; select: (node: ResourceTreeRef) => void; empty: string }) {
  if (!items.length) return <EmptyState title={empty} />;
  return <div className="overflow-hidden rounded-md border"><Table aria-label="直属资源"><TableHeader><TableRow className="bg-muted/30"><TableHead className="pl-3">名称 / 编号</TableHead><TableHead>类型</TableHead><TableHead className="pr-3 text-right">状态</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.id}><TableCell className="max-w-0 pl-3"><button type="button" className="flex min-h-11 w-full cursor-pointer items-center gap-2 rounded text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring" onClick={() => select(item)}><ResourceIcon kind={item.kind} small /><span className="min-w-0 whitespace-normal"><span className="block break-words text-sm font-medium">{item.name}</span><span className="block break-all text-xs text-muted-foreground">{item.code}</span>{item.metric_name && <span className="block text-xs text-muted-foreground">{item.metric_name}{item.unit_symbol && ` · ${item.unit_symbol}`}</span>}</span></button></TableCell><TableCell className="w-20 whitespace-normal text-xs text-muted-foreground">{resourceLabels[item.kind]}</TableCell><TableCell className="w-20 pr-3 text-right"><Badge variant="outline" className="font-normal">{item.status === 'normal' ? '正常' : '已退役'}</Badge></TableCell></TableRow>)}</TableBody></Table></div>;
}
