import { useRoutePagination } from '../../hooks/use-route-pagination';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useEffect,useMemo,useRef,useState } from 'react';
import { useQuery,useQueryClient } from '@tanstack/react-query';
import { useLocation,useNavigate,useSearch } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError,type AuditScope,type AuditCapabilities } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { ArrowDownToLine, LockKeyhole, RefreshCw, ScrollText } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { EventTable } from './event-table';
import { EventDetail } from './event-detail';
import { ExportDialog } from './export-dialog';
import { ExportList } from './export-list';
import { ExportDetail } from './export-detail';
import { auditFilter,auditPath,auditSearch,type AuditSearch } from './search';
declare module '@tanstack/react-router' { interface HistoryState { auditTrail?: (string | null)[]; auditTrailFor?: string } }
export function AuditWorkspace({scope}:{scope:AuditScope}) {
 const{client,account,authorization_revision,authorizationEpoch,phase}=useIdentity(),cache=useQueryClient();const[epoch,setEpoch]=useState(0),scopeId=JSON.stringify(scope);
 const prefix=useMemo(()=>['audit',account?.id,scopeId] as const,[account?.id,scopeId]);
 const lastRevision=useRef(authorization_revision);
 const key=useMemo(()=>[...prefix,authorization_revision,authorizationEpoch,epoch],[prefix,authorization_revision,authorizationEpoch,epoch]);
 useEffect(()=>{const clear=()=>{void cache.cancelQueries({queryKey:prefix});cache.removeQueries({queryKey:prefix});setEpoch(v=>v+1);};const unsub=[client.onAuthorizationChange(value=>{if(!value||value!==lastRevision.current){lastRevision.current=value;clear();}}),client.onProjectRestriction(clear),client.onTenantRestriction((_id,code)=>{if(code!=='active')clear();})];return()=>{unsub.forEach(fn=>fn());void cache.cancelQueries({queryKey:prefix});cache.removeQueries({queryKey:prefix});};},[client,cache,prefix]);
 const caps=useQuery({queryKey:[...key,'capabilities'],queryFn:({signal})=>client.auditCapabilities(scope,signal),enabled:phase==='authenticated',retry:false,refetchInterval:15000,refetchIntervalInBackground:false,refetchOnWindowFocus:'always'});
 if(phase!=='authenticated')return null;
 if(caps.error)return <ErrorState title={caps.error instanceof ApiError&&[403,404].includes(caps.error.status??0)?'没有当前空间的审计读取权限':'审计服务暂不可用'} message={caps.error.message} onRetry={()=>void caps.refetch()}/>;
 if(!caps.data)return <p role="status">正在核验审计权限…</p>;
 const c=caps.data;if(!c.read&&!c.manage)return <p>没有当前空间的审计读取或导出管理权限。</p>;
 const stamp=[...key,c.membership_key,c.authorization_revision,c.projection_version,c.epoch];
 return <WorkspaceContent key={JSON.stringify(stamp)} scope={scope} caps={c} queryKey={stamp}/>;
}
function WorkspaceContent({ scope, caps, queryKey }: { scope: AuditScope; caps: AuditCapabilities; queryKey: readonly unknown[] }) {
  const { client } = useIdentity(), cache = useQueryClient(), navigate = useNavigate();
  const search = auditSearch(useSearch({ strict: false })), filter = { ...auditFilter(search), limit: search.limit ?? 10 };
  const historyState = useLocation({ select: location => location.state });
  const [exportOpen, setExportOpen] = useState(false), mountedKey = useRef(queryKey);
  useEffect(() => () => { void cache.cancelQueries({ queryKey: mountedKey.current }); cache.removeQueries({ queryKey: mountedKey.current }); }, [cache]);
  const exports = search.tab === 'exports' || !caps.read;
  const q = useQuery({ queryKey: [...queryKey, 'list', filter], queryFn: ({ signal }) => client.auditEvents(scope, filter, signal), enabled: caps.read && !exports, retry: false, refetchOnWindowFocus: 'always' });
  const obsolete = !!q.data && q.data.authorization_revision !== caps.authorization_revision;
  const items = obsolete || q.error ? [] : q.data?.items ?? [];
  const trailKey = JSON.stringify([queryKey, { ...filter, cursor: undefined }]);
  const previous = historyState.auditTrailFor === trailKey ? historyState.auditTrail ?? [] : [];
  const change = (next: AuditSearch) => void navigate({ to: auditPath(scope), search: next, state: state => ({ ...state }) });
  const applyFilter = (next: AuditSearch) => void navigate({ to: auditPath(scope), search: { ...next, cursor: undefined }, state: state => ({ ...state, auditTrail: [], auditTrailFor: undefined }) });
  const pagination = useRoutePagination({
    key: [...queryKey, 'list', { ...filter, cursor: undefined }], data: !obsolete && !q.error ? q.data : undefined, cursor: filter.cursor, trail: previous,
    load: (cursor, signal) => client.auditEvents(scope, { ...filter, cursor }, signal), move: (cursor, trail) => void navigate({ to: auditPath(scope), search: { ...search, cursor }, state: state => ({ ...state, auditTrail: trail, auditTrailFor: trailKey }) }),
    pageSize: filter.limit, setPageSize: size => applyFilter({ ...filter, limit: size }), fetching: q.isFetching,
  });
  const scopeName = scope.kind === 'platform' ? '平台' : scope.kind === 'tenant' ? '租户' : '项目';
  const title = scope.kind === 'platform' ? '平台审计' : `${scopeName}操作记录`;
  const refresh = <Button variant="outline" size="icon" className="size-9 max-sm:size-11" aria-label="刷新记录" disabled={q.isFetching} onClick={() => void q.refetch()}><RefreshCw aria-hidden="true" className={q.isFetching ? 'motion-safe:animate-spin' : ''} /></Button>;
  return <section className="min-w-0 space-y-5" data-audit-workspace={scope.kind}>
    <div className="flex flex-wrap items-start justify-between gap-3 [&>div:first-child]:mb-0"><PageHeader title={title} description={scope.kind === 'platform' ? '核实平台治理、账号权限与系统操作的历史记录。' : `追踪当前${scopeName}内的操作、实际变化与后续处理。`} />{caps.export && <Button className="h-9 max-sm:h-11" onClick={() => setExportOpen(true)}><ArrowDownToLine aria-hidden="true" />导出当前筛选</Button>}</div>
    <div className="flex flex-wrap items-center justify-between gap-3 border-b"><nav className="flex gap-6" aria-label="审计视图">{caps.read && <Button variant="ghost" aria-pressed={!exports} className={`h-11 rounded-none border-b-2 px-0 hover:bg-transparent ${!exports ? 'border-foreground' : 'border-transparent text-muted-foreground'}`} onClick={() => change({ ...search, tab: undefined, event: undefined, export_id: undefined, execution: undefined })}><ScrollText aria-hidden="true" />操作记录</Button>}<Button variant="ghost" aria-pressed={exports} className={`h-11 rounded-none border-b-2 px-0 hover:bg-transparent ${exports ? 'border-foreground' : 'border-transparent text-muted-foreground'}`} onClick={() => change({ ...search, tab: 'exports', event: undefined, export_id: undefined, execution: undefined })}><ArrowDownToLine aria-hidden="true" />导出任务</Button></nav><p className="mb-2 flex items-center gap-1.5 text-xs text-muted-foreground"><LockKeyhole aria-hidden="true" className="size-3.5" />仅当前{scopeName}获准记录</p></div>
    {exports ? <ExportList scope={scope} canManage={caps.manage} queryKey={queryKey} open={(id, generation) => change({ ...search, tab: 'exports', export_id: id, execution: generation })} /> : <>
      <EventTable items={items} loading={q.isPending} fetching={q.isFetching} error={q.error} filter={filter} technical={caps.technical} onChange={applyFilter} open={event => change({ ...search, event })} refresh={refresh}
        notice={<>{q.error && <ErrorState title={q.error instanceof ApiError && q.error.code === 'audit_query_budget_exceeded' ? '候选记录超出查询预算，请缩小范围' : '暂时无法核实记录'} message={q.error.message} onRetry={() => void q.refetch()} />}{q.error instanceof ApiError && q.error.code === 'audit_cursor_stale' && <Button onClick={() => applyFilter(filter)}>按当前权限回到第一页</Button>}{!q.error && q.data?.coverage === 'partial' && <p role="status" className="rounded-md border bg-muted/30 p-3 text-xs leading-5">部分来源仍在交接或历史字段不完整；当前结果不能代表完整记录。</p>}{obsolete && <p role="status">授权已变化，正在重新核验；旧记录已隐藏。</p>}</>}
        pagination={<ListPagination label="审计记录" {...pagination} pageSizes={[10, 25, 50, 100]} />} />
      <p className="text-xs leading-5 text-muted-foreground">记录按当前空间与权限展示。查看详情可核实字段变化、关联来源与处理过程。</p>
    </>}
    {search.event && caps.read && <EventDetail key={search.event} scope={scope} id={search.event} queryKey={queryKey} close={() => change({ ...search, event: undefined })} history={(objectType, objectId) => applyFilter({ ...filter, object_type: objectType, object_id: objectId, reference_id: undefined })} />}
    {search.export_id && <ExportDetail key={`${search.export_id}:${search.execution}`} scope={scope} id={search.export_id} generation={search.execution} queryKey={queryKey} close={() => change({ ...search, export_id: undefined, execution: undefined })} />}
    {exportOpen && <ExportDialog scope={scope} capabilities={caps} filter={filter} close={() => setExportOpen(false)} created={e => { setExportOpen(false); change({ ...search, tab: 'exports', export_id: e.id, execution: e.generation }); }} />}
  </section>;
}
