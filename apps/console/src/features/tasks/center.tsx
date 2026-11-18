import { useEffect, useMemo } from 'react';
import { useLocation, useMatch, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError, type TaskFilter } from '@infrapro/api-client';
import { Activity, ArrowDown, ArrowUp, CheckCheck, ChevronRight, CircleAlert, Clock3, Download, FileInput, FlaskConical, Inbox, Layers3, RefreshCw, Workflow } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { useIsMobile } from '@infrapro/ui/hooks/use-mobile';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useRoutePagination } from '../../hooks/use-route-pagination';
import { TaskDetailContent } from './detail';
import { TaskOperations } from './operations';
import { TaskError, TaskStatus } from './common';
import { TaskToolbar } from './toolbar';
import { useTasks } from './provider';
import type { TaskSearch } from './search';
import './center.css';

declare module '@tanstack/react-router' { interface HistoryState { taskTrail?: (string | null)[]; taskTrailFor?: string } }
const groups = [
  { field: 'total', label: '全部任务', group: undefined, icon: Layers3 },
  { field: 'active', label: '进行中', group: 'active', icon: Clock3 },
  { field: 'attention', label: '需处理', group: 'attention', icon: CircleAlert },
  { field: 'ended', label: '已结束', group: 'ended', icon: CheckCheck },
] as const;
const time = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });

export function TaskCenter({ filter: raw, embedded = false }: { filter: TaskSearch; embedded?: boolean }) {
  const { client } = useIdentity(), tasks = useTasks(), navigate = useNavigate(), mobile = useIsMobile();
  const match = useMatch({ from: '/_authenticated/tasks/$taskId', shouldThrow: false });
  const selected = match?.params.taskId;
  const state = useLocation({ select: location => location.state });
  const { source, scope_kind, mode = 'mine', tenant_id, project_id, type, group, from, to, q, cursor, limit = 10, sort } = raw;
  const scope = useMemo(() => ({ source, scope_kind, mode, tenant_id, project_id }), [source, scope_kind, mode, tenant_id, project_id]);
  const criteria = useMemo(() => ({ ...scope, type, from, to, q }), [scope, type, from, to, q]);
  const filter = { ...criteria, group, cursor, limit, sort };
  const list = useQuery({ queryKey: [...tasks.key, 'list', filter], queryFn: ({ signal }) => client.tasks(filter, signal), retry: false,
    refetchInterval: query => query.state.data?.items.some(item => ['queued','running','waiting','retry_wait','cancelling'].includes(item.state)) ? 3000 : 15000,
    refetchIntervalInBackground: false, refetchOnWindowFocus: 'always' });
  const summary = useQuery({ queryKey: [...tasks.key, 'filtered-summary', criteria], queryFn: ({ signal }) => client.taskSummary(criteria, signal), refetchInterval: 30000, retry: false });
  const catalog = useQuery({ queryKey: [...tasks.key, 'catalog', scope], queryFn: ({ signal }) => client.taskCatalog(scope, signal), retry: false });
  const { observeAuthorization } = tasks;
  useEffect(() => { observeAuthorization(list.data?.authorization_revision); }, [list.data?.authorization_revision, observeAuthorization]);
  const trailKey = JSON.stringify({ ...filter, cursor: undefined });
  function change(value: Partial<TaskFilter>) {
    void navigate({ to: '/tasks', search: { ...filter, ...value, cursor: undefined, back: raw.back }, state: s => ({ ...s, taskTrail: [], taskTrailFor: undefined }) });
  }
  const pagination = useRoutePagination({
    key: [...tasks.key, 'list', { ...filter, cursor: undefined }], data: list.error ? undefined : list.data, cursor,
    trail: state.taskTrailFor === trailKey ? state.taskTrail ?? [] : [],
    load: (next, signal) => client.tasks({ ...filter, cursor: next }, signal),
    move: (next, trail) => void navigate({ to: '/tasks', search: { ...filter, cursor: next, back: raw.back }, state: s => ({ ...s, taskTrail: trail, taskTrailFor: trailKey }) }),
    pageSize: limit, setPageSize: size => change({ limit: size }), fetching: list.isFetching,
  });
  function select(id: string, execution?: string) {
    void navigate({ to: '/tasks/$taskId', params: { taskId: id }, search: { ...filter, back: raw.back, execution }, state: s => ({ ...s }), resetScroll: false });
  }
  function overlay(value: Partial<TaskSearch>) {
    const options = { search: { ...raw, ...value }, state: (s: typeof state) => ({ ...s }), resetScroll: false };
    if (selected) void navigate({ ...options, to: '/tasks/$taskId', params: { taskId: selected } });
    else void navigate({ ...options, to: '/tasks' });
  }
  const items = list.error ? [] : list.data?.items ?? [];
  const activeID = selected ?? (!mobile ? items[0]?.id : undefined);
  const detail = activeID && <TaskDetailContent key={activeID} id={activeID} execution={selected ? raw.execution : undefined} initialAction={raw.action} onExecution={generation => select(activeID, generation)} />;
  const counts = !summary.error && summary.data?.counts_state === 'exact' ? summary.data : undefined;
  return <section className="min-w-0 space-y-5">
    <header className="flex flex-wrap items-center justify-between gap-3">{!embedded && <PageHeader title="任务中心" description="跟进任务进度、处理异常，查看本次执行的成果。" />}<div className="flex gap-2">
      <Button variant="outline" onClick={() => overlay({ operations: 'open' })}><Activity aria-hidden />运行概况</Button>
      <Button variant="outline" disabled={list.isFetching} onClick={() => void tasks.refresh()}><RefreshCw aria-hidden />刷新</Button></div></header>
    <Tabs value={mode} onValueChange={value => change({ mode: value === 'managed' ? 'managed' : 'mine' })}><TabsList aria-label="任务视图"><TabsTrigger value="mine">我的任务</TabsTrigger><TabsTrigger value="managed">管理视图</TabsTrigger></TabsList></Tabs>
    <div className="task-counts" aria-label="任务统计">{groups.map(({ field, label, group: value, icon: Icon }) => <button type="button" key={field} aria-pressed={value === group} onClick={() => change({ group: value })}>
      <span><Icon aria-hidden size={16} />{label}</span><strong>{counts?.[field]?.toLocaleString() ?? '—'}</strong>
    </button>)}</div>
    {!counts && !summary.isPending && <p role="status" className="text-xs text-muted-foreground">当前授权范围的完整统计暂时无法核实。</p>}
    <TaskToolbar filter={filter} types={catalog.error ? [] : catalog.data?.items ?? []} onChange={change} />
    <TaskError error={list.error ?? catalog.error ?? summary.error} retry={() => void tasks.refresh()} />
    {list.error instanceof ApiError && list.error.code === 'task_cursor_stale' && <Button onClick={() => change({})}>按当前权限回到首页</Button>}
    {list.data?.coverage === 'partial' && !list.error && <p role="status" className="text-xs text-muted-foreground">索引正在同步，部分历史任务尚未列出。</p>}
    <div className="task-panes"><section aria-label="任务列表" className="task-inbox">
      <header className="task-pane-heading"><h2>任务列表</h2><Button aria-label="按提交时间排序" size="sm" variant="ghost" onClick={() => change({ sort: sort === 'oldest' ? undefined : 'oldest' })}>提交时间{sort === 'oldest' ? <ArrowUp /> : <ArrowDown />}</Button></header>
      <div className="task-items">{items.map(task => {
        const Icon = task.type.includes('export') ? Download : task.type.includes('import') ? FileInput : task.type.startsWith('analysis.') ? FlaskConical : Workflow;
        return <button type="button" id={`task-${task.id}`} key={task.id} aria-pressed={activeID === task.id} className="task-list-item" onClick={() => select(task.id, task.generation)}>
          <span className="task-type-icon"><Icon size={17} aria-hidden /></span><span className="min-w-0 space-y-1"><strong className="block text-sm">{task.title}</strong><span className="block truncate text-xs text-muted-foreground">{task.scope_label} · {task.id.slice(0, 8).toUpperCase()}</span>
          <span className="flex flex-wrap items-center gap-2 pt-1"><TaskStatus task={task} hideResult /><time className="text-xs text-muted-foreground" dateTime={task.created_at}>{time.format(new Date(task.created_at))}</time></span></span><ChevronRight size={15} aria-hidden /></button>;
      })}{!items.length && <p role="status" className="p-8 text-center text-sm text-muted-foreground">{list.isPending ? '正在读取任务…' : list.error ? '任务列表暂时不可用' : '当前筛选下没有任务'}</p>}</div>
    </section>{!mobile && <section className="task-inspector" aria-label="任务详情"><header className="task-pane-heading"><h2>任务详情</h2></header><div className="task-detail-body">{detail || <div className="grid justify-items-center gap-3 py-12 text-sm text-muted-foreground"><Inbox aria-hidden />选择一项任务查看详情</div>}</div></section>}</div>
    <ListPagination label="任务" {...pagination} pageSizes={[10,25,50,100]} />
    {mobile && selected && !raw.operations && <ResponsivePanel open title="任务详情" description="本次执行的进度、操作和结果" fallbackFocusSelector={`#task-${selected}`} onOpenChange={open => { if (!open) void navigate({ to: '/tasks', replace: true, search: { ...filter, back: raw.back }, state: s => ({ ...s }), resetScroll: false }); }}>{detail}</ResponsivePanel>}
    <ResponsivePanel open={raw.operations === 'open'} title="运行概况" description="按工作空间查看后台运行和处理异常。" size="wide" onOpenChange={open => { if (!open) overlay({ operations: undefined, automatic: undefined, run: undefined }); }}>{raw.operations && <TaskOperations search={raw} change={overlay} />}</ResponsivePanel>
  </section>;
}
