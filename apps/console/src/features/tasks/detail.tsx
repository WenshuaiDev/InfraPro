import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError, type TaskCommand, type TaskView } from '@infrapro/api-client';
import { ArrowUpRight, CircleAlert, Copy, RotateCcw, Square } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@infrapro/ui/components/button';
import { Textarea } from '@infrapro/ui/components/textarea';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { Skeleton } from '@infrapro/ui/components/skeleton';
import { reasonNames } from '../monitoring/detail-format';
import { TaskHistory } from '../audit/task-history';
import { useTasks } from './provider';
import { TaskError, TaskProgress, TaskStatus, taskDestination, taskStates } from './common';

export function TaskActions({ task, initialAction }: { task: TaskView; initialAction?: 'cancel' | 'retry' }) {
  const { client } = useIdentity(), t = useTasks();
  const [action, setAction] = useState<'cancel' | 'retry'>(initialAction ?? ((task.actions.find(value => value.enabled) ?? task.actions.find(value => value.kind === 'retry'))?.kind === 'retry' ? 'retry' : 'cancel'));
  const [editing, setEditing] = useState(!!initialAction), [reason, setReason] = useState(''), [hasPendingRequest, setHasPendingRequest] = useState(false);
  const pendingCommand = useRef<TaskCommand | null>(null);
  const command = useMutation({
    mutationFn: () => {
      pendingCommand.current ??= { request_id: crypto.randomUUID(), action, reason, expected_generation: task.generation, expected_control_revision: task.control_revision };
      setHasPendingRequest(true);
      return client.taskCommand(task.id, pendingCommand.current);
    },
    onSuccess: () => { pendingCommand.current = null; setHasPendingRequest(false); setEditing(false); void t.refresh(); },
    onError: error => {
      // Only a definitive rejection releases the frozen command. A network error
      // or server failure can follow a successful commit and must replay its ID.
      if (error instanceof ApiError && error.status && error.status >= 400 && error.status < 500) { pendingCommand.current = null; setHasPendingRequest(false); }
    },
    retry: false,
  });
  const capability = task.actions.find(value => value.kind === action);
  const replayPending = !!command.error && hasPendingRequest;
  if ((!task.actions.length || task.actions.every(value => !value.enabled) && ['succeeded', 'cancelled'].includes(task.state)) && !hasPendingRequest && !command.data && !editing) return null;
  const reasonRequired = capability?.requires_reason || action === 'retry';
  return <section className="space-y-3 border-t pt-5"><h3 className="text-sm font-semibold">任务操作</h3><div className="flex flex-wrap gap-2">{task.actions.map(value => <Button type="button" key={value.kind} variant={editing && action === value.kind ? 'secondary' : 'outline'} disabled={!value.enabled || hasPendingRequest || command.isPending} title={value.enabled ? undefined : '当前状态或权限不支持此操作'} onClick={() => { setAction(value.kind === 'retry' ? 'retry' : 'cancel'); setEditing(true); command.reset(); }}>{value.kind === 'retry' ? <RotateCcw aria-hidden="true" /> : <Square aria-hidden="true" />}{value.kind === 'retry' ? '重试此任务' : '取消此任务'}</Button>)}</div>
    {(editing || hasPendingRequest || task.actions.every(value => !value.enabled)) && <form className="space-y-3 rounded-lg border bg-muted/25 p-4" onSubmit={event => { event.preventDefault(); if ((capability?.enabled || replayPending) && !command.isPending) command.mutate(); }}>
      <p className="text-sm leading-6">{action === 'retry' ? task.type === 'spatial.process' ? '使用未变化的原件重新处理，由当前操作人执行；原发起人与旧错误保留。' : '使用固定输入和原执行资格开始新一代，旧执行记录保留。' : `取消“${task.title}”第 ${task.generation} 代。运行中的任务可能需要等待工作进程确认停止。`}</p>
      <label className="grid gap-2 text-sm">操作理由{!reasonRequired && '（可选）'}<Textarea aria-label="操作理由" className="min-h-24 bg-background" placeholder={reasonRequired ? '说明本次操作的原因…' : '补充本次取消的原因…'} value={reason} onChange={event => { setReason(event.target.value); command.reset(); }} maxLength={2000} required={reasonRequired} disabled={hasPendingRequest || command.isPending || !capability?.enabled} /></label>
      {replayPending && <p role="status" className="text-xs leading-5">上次请求尚未确认结果。再次提交会使用原请求编号核对，不重复创建执行。</p>}
      {!capability?.enabled && !replayPending && <p className="text-xs leading-5 text-muted-foreground">{capability?.reason_code === 'execution_principal_unavailable' ? '原执行账号或委托已失效。请回原业务页面，按当前资格重新创建任务。' : capability?.reason_code === 'attempts_or_input_unavailable' ? '已达到原任务尝试上限，或原上传内容已不可用于重试。请回大屏页面核对。' : capability?.reason_code === 'input_changed' ? '原件、草稿或处理条件已变化，当前条件不允许重试。请回原业务页面核对。' : '当前状态不支持此操作，请核实任务或返回原业务处理。'}</p>}
      <TaskError error={command.error} />
      {command.error instanceof ApiError && command.error.code === 'task_revision_changed' && <Button type="button" variant="outline" onClick={() => { command.reset(); void t.refresh(); }}>核对最新状态，保留理由</Button>}
      <div className="flex flex-wrap gap-2"><Button type="submit" variant={action === 'cancel' ? 'destructive' : 'default'} disabled={(!capability?.enabled && !replayPending) || command.isPending || (!!reasonRequired && !reason.trim())}>{command.isPending ? '正在提交…' : action === 'retry' ? '确认重试' : '确认取消'}</Button><Button type="button" variant="ghost" disabled={hasPendingRequest || command.isPending} onClick={() => setEditing(false)}>暂不操作</Button></div>
    </form>}
    {command.data && <p role="status" className="text-sm">{command.data.command_state === 'requested' ? '取消请求已提交，等待执行停止。' : '操作已提交。'}</p>}
  </section>;
}
export function TaskDetailContent({ id, execution, initialAction, onExecution }: { id: string; execution?: string; initialAction?: 'cancel' | 'retry'; onExecution: (value: string) => void }) {
  const { client } = useIdentity(), t = useTasks(), navigate = useNavigate();
  const query = useQuery({ queryKey: [...t.key, 'detail', id, execution], queryFn: ({ signal }) => client.task(id, execution, signal), refetchInterval: value => value.state.data && ['queued', 'running', 'waiting', 'retry_wait', 'cancelling'].includes(value.state.data.state) ? 3000 : 15000, refetchIntervalInBackground: false, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', retry: false });
  const task = query.error ? undefined : query.data;
  const history = usePagedQuery({ queryKey: [...t.key, 'executions', id], queryFn: ({ signal, cursor }) => client.taskExecutions(id, cursor, signal), enabled: !!task, retry: false });
  const events = usePagedQuery({ queryKey: [...t.key, 'events', id, task?.generation], queryFn: ({ signal, cursor }) => client.taskEvents(id, task?.generation ?? '1', cursor, signal), enabled: !!task, retry: false });
  const target = useMutation({ mutationFn: (kind: string) => client.taskTarget(id, task?.generation ?? '1', kind), onSuccess: (value, purpose) => { const destination = taskDestination(value, purpose); if (destination) void navigate(destination); } });
  return <div className="min-w-0 space-y-5"><TaskError error={query.error} retry={() => void query.refetch()} />{query.isPending && <div role="status" className="space-y-4"><span className="sr-only">正在核实任务…</span><Skeleton className="h-6 w-3/4" /><Skeleton className="h-4 w-1/2" /><Skeleton className="h-24 w-full" /></div>}{task && <>
    <div className="space-y-3"><TaskStatus task={task} /><h2 className="break-words text-xl font-semibold tracking-tight">{task.title}</h2><p className="text-sm leading-6 text-muted-foreground">{task.scope_label}</p><div className="flex min-w-0 items-center gap-1"><code className="min-w-0 truncate text-[11px] text-muted-foreground" title={task.id}>{task.id}</code><Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label="复制任务编号" onClick={() => { void navigator.clipboard.writeText(task.id).then(() => toast.success('已复制任务编号'), () => toast.error('复制失败，请选择编号复制。')); }}><Copy aria-hidden="true" className="size-3.5" /></Button></div><div className="rounded-lg border bg-muted/20 p-4"><TaskProgress phase={task.phase} progress={task.progress} /></div>
      {task.generation !== task.current_generation && <p className="rounded-md border bg-muted/40 p-3 text-xs leading-5">正在查看第 {task.generation} 代。当前为第 {task.current_generation} 代；历史失败不会被当前状态覆盖。<Button variant="link" className="h-auto px-1 py-0 text-xs" onClick={() => onExecution(task.current_generation)}>查看当前代次</Button></p>}
      {task.history_state === 'syncing' && <p role="status" className="text-xs text-muted-foreground">当前来源状态已核实，执行历史正在同步。</p>}
      {task.history_incomplete && <p className="text-xs text-muted-foreground">部分旧历史未保存，以下仅显示有证据的记录。</p>}
      {task.cleanup_state && <p className="text-xs text-muted-foreground">取消已阻止成果发布，临时资源清理状态待核实。</p>}
      {task.error_code && <div className="space-y-2 rounded-lg border border-amber-200 bg-amber-50/50 p-4 text-amber-900 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200"><p className="flex items-center gap-2 text-sm font-medium"><CircleAlert aria-hidden="true" className="size-4" />失败或待处理原因</p><p className="text-xs leading-5">{task.error_code === 'execution_principal_unavailable' ? '原执行资格已失效，请在原业务页面重新申请。' : task.error_code === 'partial_execution_restricted' ? '部分执行信息受限，无法确认整组结果。' : reasonNames[task.error_code] ?? task.error_code}</p><details className="text-xs"><summary className="cursor-pointer py-1">诊断信息</summary><p className="break-all pt-2">原因码：{task.error_code}</p></details></div>}
    </div>
    <section className="space-y-3"><h3 className="text-sm font-semibold">结果与下一步</h3><p className="text-xs leading-5 text-muted-foreground">{task.type === 'spatial.process' || task.type === 'display.package' ? '处理完成后，请到原页面配置并明确发布。' : task.type === 'monitoring.import.validate' ? '校验完成后，请到批次页面检查并确认采用。' : task.next_step === 'completed_noop' ? '本次处理完成，没有新增需要采用的数据。' : '结果是否可用与历史执行状态分别核验。'}</p><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={!task.result_readable || target.isPending} onClick={() => target.mutate('business')}>打开原业务详情<ArrowUpRight aria-hidden="true" /></Button>{task.result_state === 'available' && <Button disabled={!task.result_readable || target.isPending} onClick={() => target.mutate('result')}>查看结果</Button>}</div><TaskError error={target.error} />{target.data && !target.data.available && <p role="status" className="text-xs">当前结果不可访问，请回原业务重新核实或申请。</p>}</section>
    <TaskActions key={execution ?? 'current'} task={task} initialAction={initialAction} />
    <Tabs defaultValue="overview" className="border-t pt-5"><TabsList className="w-full"><TabsTrigger value="overview">概览</TabsTrigger><TabsTrigger value="history">执行记录</TabsTrigger></TabsList><TabsContent value="overview" className="space-y-5 pt-4"><dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-4 gap-y-4 text-xs"><dt className="text-muted-foreground">发起人</dt><dd>{task.actor_label || '历史未记录'}</dd><dt className="text-muted-foreground">提交时间</dt><dd className="tabular-nums">{new Date(task.created_at).toLocaleString('zh-CN')}</dd><dt className="text-muted-foreground">执行代次</dt><dd>第 {task.generation} 代</dd><dt className="text-muted-foreground">执行身份</dt><dd>{task.selected_execution.executor_kind === 'person' ? '人员账号' : task.selected_execution.executor_kind === 'none' ? '展示分组' : '委托执行'}</dd><dt className="text-muted-foreground">最近核实</dt><dd className="tabular-nums">{new Date(task.observed_at).toLocaleString('zh-CN')}</dd></dl><details className="rounded-md border px-3 text-xs"><summary className="cursor-pointer py-3">本代身份编号</summary><p className="break-all pb-3 leading-6">发起操作账号：{task.selected_execution.requested_by || '历史未记录'}<br />实际执行账号：{task.selected_execution.executor_ref || '无人员执行器'}</p></details><TaskHistory task={task} />{(task.relations.length > 0 || task.relations_restricted) && <section className="space-y-3"><h3 className="text-sm font-semibold">关联运行</h3>{task.relations_restricted && <p className="text-xs text-muted-foreground">部分执行信息受限。</p>}{task.relations.map(child => <div key={child.id} className="space-y-2 rounded-md border p-3"><Link className="inline-block py-1 text-sm underline" to="/tasks/$taskId" params={{ taskId: child.id }}>{child.title}</Link><TaskStatus task={child} /></div>)}</section>}</TabsContent>
      <TabsContent value="history" className="space-y-6 pt-4"><section className="space-y-3"><h3 className="text-sm font-semibold">执行代次</h3><TaskError error={history.error} retry={() => void history.refetch()} />{history.isPending && <p className="text-xs text-muted-foreground">正在加载执行历史…</p>}<div className="flex flex-wrap gap-2">{!history.error && history.data?.items.map(item => <Button variant={item.generation === task.generation ? 'secondary' : 'outline'} key={item.generation} size="sm" onClick={() => { onExecution(item.generation); }}>第 {item.generation} 代 · {taskStates[item.state] ?? item.state}</Button>)}</div>{history.data && <ListPagination label="任务执行历史" {...history.pagination} />}</section><section className="space-y-4"><h3 className="text-sm font-semibold">状态记录</h3><TaskError error={events.error} retry={() => void events.refetch()} />{events.isPending && <p className="text-xs text-muted-foreground">正在加载状态记录…</p>}<ol className="ml-1 border-l pl-5">{!events.error && events.data?.items.map(item => { const data = item.details as Record<string, string>; return <li key={item.id} className="relative pb-5 text-xs"><span className="absolute -left-[25px] top-1 size-2 rounded-full border bg-background" /><p className="font-medium">{taskStates[data.state ?? ''] ?? '状态更新'}</p><p className="mt-1 text-muted-foreground">{new Date(item.occurred_at).toLocaleString('zh-CN')}</p>{data.reason && <p className="mt-2 break-words leading-5 text-muted-foreground">理由：{data.reason}</p>}</li>; })}</ol>{!events.error && events.data?.items.length === 0 && <p className="text-xs text-muted-foreground">暂无已记录的状态变化。</p>}{events.data && <ListPagination label="任务状态记录" {...events.pagination} />}</section><details className="rounded-md border px-3"><summary className="cursor-pointer py-3 text-xs">工作进程尝试 · {task.attempts.length}</summary>{task.attempts_truncated && <p className="text-xs text-muted-foreground">仅显示最近 100 次尝试；完整状态变化可在执行记录中向前翻阅。</p>}{task.attempts.map(attempt => <p key={`${attempt.unit_key}:${attempt.source_fence}`} className="break-words py-2 text-xs leading-5">尝试 {attempt.attempt_no} · {taskStates[attempt.outcome] ?? (attempt.outcome === 'superseded' ? '已被新租约接替' : attempt.outcome)}<br /><span className="text-muted-foreground">{new Date(attempt.started_at).toLocaleString('zh-CN')}</span></p>)}</details></TabsContent>
    </Tabs>
  </>}</div>;
}
