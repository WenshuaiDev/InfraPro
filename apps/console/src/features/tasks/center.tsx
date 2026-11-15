import { useCallback, useEffect, useMemo, useState } from 'react';
import { Outlet, useLocation, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError, type TaskFilter, type TaskView } from '@infrapro/api-client';
import { Activity, RefreshCw } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useTasks } from './provider';
import { TaskError } from './common';
import { TaskTable } from './task-table';
import type { TaskSearch } from './search';

declare module '@tanstack/react-router' { interface HistoryState { taskTrail?: (string | null)[]; taskTrailFor?: string } }
const noTypes: { type: string; title: string }[] = [];
export function TaskCenter({ filter: raw, embedded = false }: { filter: TaskSearch; embedded?: boolean }) {
  const { client } = useIdentity(), t = useTasks(), navigate = useNavigate();
  const historyState = useLocation({ select: location => location.state });
  const { source, scope_kind, mode = 'mine', tenant_id, project_id, type, group, from, to, q, cursor, limit = 10, sort } = raw;
  const filter: TaskFilter = useMemo(() => ({ source, scope_kind, mode, tenant_id, project_id, type, group, from, to, q, cursor, limit, sort }), [source, scope_kind, mode, tenant_id, project_id, type, group, from, to, q, cursor, limit, sort]);
  const trailKey = JSON.stringify({ ...filter, cursor: undefined });
  const previous = historyState.taskTrailFor === trailKey ? historyState.taskTrail ?? [] : [];
  const [operationsOpen, setOperationsOpen] = useState(false);
  const query = useQuery({ queryKey: [...t.key, 'list', filter], queryFn: ({ signal }) => client.tasks(filter, signal), refetchInterval: value => value.state.data?.items.some(item => ['queued', 'running', 'waiting', 'retry_wait', 'cancelling'].includes(item.state)) ? 3000 : 15000, refetchIntervalInBackground: false, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', retry: false });
  const observeAuthorization = t.observeAuthorization;
  useEffect(() => { observeAuthorization(query.data?.authorization_revision); }, [query.data?.authorization_revision, observeAuthorization]);
  const catalog = useQuery({ queryKey: [...t.key, 'catalog', source, scope_kind, mode, tenant_id, project_id], queryFn: ({ signal }) => client.taskCatalog({ source, scope_kind, mode, tenant_id, project_id }, signal), retry: false });
  const change = useCallback((value: Partial<TaskFilter>) => { void navigate({ to: '/tasks', search: { ...filter, ...value, cursor: undefined }, state: state => ({ ...state, taskTrail: [], taskTrailFor: undefined }) }); }, [filter, navigate]);
  const open = useCallback((task: TaskView, action?: 'cancel' | 'retry') => { void navigate({ to: '/tasks/$taskId', params: { taskId: task.id }, search: { ...filter, action }, state: state => ({ ...state }) }); }, [filter, navigate]);
  const items = query.error ? [] : query.data?.items ?? [];
  return <section className="min-w-0 space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-3">{!embedded && <PageHeader title="任务中心" description="跟进任务进度、处理异常，在这里找到执行结果。" />}<div className="ml-auto flex items-center gap-2"><Button variant="outline" className="h-9" onClick={() => setOperationsOpen(true)}><Activity aria-hidden="true" />运行概况</Button><Button variant="outline" className="h-9" disabled={query.isFetching} onClick={() => void t.refresh()}><RefreshCw aria-hidden="true" className={query.isFetching ? 'motion-safe:animate-spin' : ''} />刷新</Button></div></div>
    <div className="flex flex-wrap items-center justify-between gap-3"><Tabs value={mode} onValueChange={value => change({ mode: value === 'managed' ? 'managed' : 'mine' })}><TabsList aria-label="任务视图"><TabsTrigger value="mine" className="px-4">我的任务</TabsTrigger><TabsTrigger value="managed" className="px-4">管理视图</TabsTrigger></TabsList></Tabs><p className="text-xs text-muted-foreground">{mode === 'managed' ? '仅展示管理权限内的任务' : '汇总本人在各工作空间可见的任务'}</p></div>
    <TaskTable items={items} filter={filter} types={catalog.error ? noTypes : catalog.data?.items ?? noTypes} loading={query.isPending} fetching={query.isFetching} error={!!query.error} next={!!query.data?.next_cursor} previous={!!cursor && previous.length > 0} page={!cursor ? 1 : previous.length ? previous.length + 1 : undefined} onChange={change} onOpen={open}
      onNext={() => { if (query.data?.next_cursor) void navigate({ to: '/tasks', search: { ...filter, cursor: query.data.next_cursor }, state: state => ({ ...state, taskTrail: [...previous, cursor ?? null], taskTrailFor: trailKey }) }); }}
      onPrevious={() => { void navigate({ to: '/tasks', search: { ...filter, cursor: previous.at(-1) ?? undefined }, state: state => ({ ...state, taskTrail: previous.slice(0, -1), taskTrailFor: trailKey }) }); }}
      notice={<><TaskError error={query.error ?? catalog.error} retry={() => void t.refresh()} />{query.error instanceof ApiError && query.error.code === 'task_cursor_stale' && <Button onClick={() => change({})}>按当前权限回到首页</Button>}{!query.error && query.data?.coverage === 'partial' && <p role="status" className="rounded-md border bg-muted/30 p-3 text-xs leading-5">任务索引正在准备或同步，当前列表尚不代表完整历史。</p>}</>} />
    <p aria-live="polite" className="text-xs text-muted-foreground">{query.isFetching ? '正在核实最新状态…' : !query.error && query.data ? `最近核实 ${new Date(query.data.observed_at).toLocaleTimeString('zh-CN')} · 自动更新` : ''}</p>
    <ResponsivePanel open={operationsOpen} onOpenChange={setOperationsOpen} title="运行概况" description="查看获准来源的内部处理状态。">{operationsOpen && <Operations filter={{ tenant_id, project_id, scope_kind }} />}</ResponsivePanel>
    {!embedded && <Outlet />}
  </section>;
}
function Operations({ filter }: { filter: TaskFilter }) {
  const { client } = useIdentity(), t = useTasks();
  const query = useQuery({ queryKey: [...t.key, 'operations', filter], queryFn: ({ signal }) => client.taskOperations(filter, signal), refetchInterval: 15000, refetchIntervalInBackground: false, retry: false });
  const modules: Record<string, { title: string; path: string }> = { monitoring: { title: '监测维护', path: 'monitoring' }, derived: { title: '自动计算', path: 'monitoring/derived-runs' }, spatial: { title: '空间处理', path: 'spatial' }, display: { title: '大屏包处理', path: 'displays' }, files: { title: '文件检查与绑定', path: 'files' }, audit: { title: '审计导出', path: 'audit' } };
  return <div className="space-y-4"><p className="text-sm leading-6 text-muted-foreground">自动计算和内部维护不列入我的任务。通知投递诊断由通知运维入口提供。</p><TaskError error={query.error} retry={() => void query.refetch()} />{query.isPending && <p role="status" className="text-sm">正在核实运行状态…</p>}{!query.error && query.data?.items.map(item => <section key={`${item.source_module}:${item.scope.tenant_id}:${item.scope.project_id}`} className="space-y-3 rounded-lg border p-4"><div><h3 className="font-medium">{modules[item.source_module]?.title ?? item.source_module}</h3><p className="mt-1 text-xs text-muted-foreground">{item.scope_label}</p></div>{item.state === 'available' ? <dl className="grid grid-cols-3 gap-3">{[['排队', item.queued], ['运行', item.running], ['失败', item.failed]].map(([label, value]) => <div key={label}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 text-xl font-semibold tabular-nums">{value ?? '—'}</dd></div>)}</dl> : <p className="text-sm">暂时无法核实</p>}<div className="flex flex-wrap items-center justify-between gap-2"><p className="text-xs text-muted-foreground">核实 {new Date(item.observed_at).toLocaleTimeString('zh-CN')}</p>{modules[item.source_module] && <Button variant="link" size="sm" className="px-0" asChild><a href={item.scope.kind === 'platform' ? '/access/audit' : `/t/${item.scope.tenant_id}${item.scope.project_id ? `/p/${item.scope.project_id}` : ''}/${modules[item.source_module]?.path}`}>打开来源工作区</a></Button>}</div></section>)}{!query.error && query.data?.items.length === 0 && <p className="py-8 text-center text-sm text-muted-foreground">当前范围没有可查看的运行概况。</p>}</div>;
}
