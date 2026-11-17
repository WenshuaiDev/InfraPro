import { useMutation } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import type { TaskProgress as Progress, TaskSourceReference, TaskTarget, TaskView } from '@infrapro/api-client';
import { CircleCheck, CircleAlert, CircleDashed, Clock3, LoaderCircle, CircleX } from 'lucide-react';
import { cn } from '@infrapro/ui/lib/utils';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { safeReturn } from '../../config/navigation';
// eslint-disable-next-line react-refresh/only-export-components
export const taskStates: Record<string, string> = { queued: '排队中', running: '执行中', waiting: '等待上游', retry_wait: '等待自动重试', cancelling: '正在取消', needs_action: '需要处理', succeeded: '已完成', failed: '失败', cancelled: '已取消', unknown: '待核实' };
const phases: Record<string, string> = { queued: '等待工作进程', preparing: '准备中', validating: '校验中', computing: '处理中', writing: '写入中', waiting: '等待依赖', sealing: '封存中', completed: '处理完成', failed: '需要检查', cancelled: '停止处理', unknown: '执行信息受限' };
const results: Record<string, string> = { none: '尚无结果', preparing: '结果准备中', available: '结果可查看', invalidated: '结果已失效', expired: '结果已过期', missing: '结果缺失', restricted: '结果访问受限', unknown: '结果待核实' };
export function TaskResult({ state }: { state: TaskView['result_state'] }) { return <span className="text-xs text-muted-foreground">{results[state]}</span>; }
export function TaskStatus({ task, hideResult = false }: { task: Pick<TaskView, 'state' | 'result_state'>; hideResult?: boolean }) {
 const failed = task.state === 'failed', attention = task.state === 'needs_action', active = ['running', 'cancelling'].includes(task.state), success = task.state === 'succeeded';
 const Icon = failed || attention ? CircleAlert : success ? CircleCheck : active ? LoaderCircle : task.state === 'cancelled' ? CircleX : task.state === 'unknown' ? CircleDashed : Clock3;
 return <div className="flex flex-wrap items-center gap-2"><Badge variant="outline" className={cn('gap-1.5 whitespace-nowrap rounded-md px-2 py-1 text-[11px] font-medium', failed && 'border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300', attention && 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300', active && 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-300', success && 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300', !failed && !attention && !active && !success && 'bg-muted/40 text-muted-foreground')}><Icon aria-hidden="true" className={cn('size-3', active && 'motion-safe:animate-spin')} />{taskStates[task.state] ?? '待核实'}</Badge>{!hideResult && <TaskResult state={task.result_state} />}</div>;
}
export function TaskProgress({ phase, progress, compact = false }: { phase: string; progress: Progress; compact?: boolean }) {
 let percent: number | undefined;
 if (progress.completed && progress.total && /^[0-9]+$/.test(progress.completed) && /^[1-9][0-9]*$/.test(progress.total)) percent = Math.min(100, Number(BigInt(progress.completed) * 100n / BigInt(progress.total)));
 const unit = ({ rows: '行', bytes: '字节', files: '个文件', units: '个单元' } as Record<string, string>)[progress.unit ?? ''] ?? '';
 const showProgress = percent !== undefined && !['completed', 'failed', 'cancelled', 'unknown'].includes(phase);
 return <div className="space-y-2 whitespace-normal break-words text-xs text-muted-foreground"><p className="leading-5">{phases[phase] ?? '处理中'}{progress.completed && ` · ${compact ? '' : '已处理 '}${progress.completed} ${unit}`}</p>{showProgress && <div className="flex items-center gap-2"><div role="progressbar" aria-label="当前阶段进度" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent} className="h-1.5 w-full max-w-48 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary transition-[width] motion-reduce:transition-none" style={{ width: `${percent}%` }} /></div><span className="tabular-nums">{percent}%</span></div>}</div>;
}
export function TaskError({ error, retry }: { error: Error | null; retry?: () => void }) { return error ? <FormNotice error><span className="break-words">{error.message}</span>{retry && <Button className="ml-2 min-h-11" variant="outline" onClick={retry}>重新核实</Button>}</FormNotice> : null; }
export function TaskLink({ source, label = '查看任务' }: { source: TaskSourceReference; label?: string }) {
 const { client } = useIdentity(), navigate = useNavigate();
 const resolve = useMutation({ mutationFn: () => client.taskResolve(source), onSuccess: ref => { if (ref.task_id) void navigate({ to: '/tasks/$taskId', params: { taskId: ref.task_id }, search: { execution: ref.generation } }); } });
 return <span className="inline-flex flex-wrap items-center gap-2"><Button className="min-h-11" variant="outline" disabled={resolve.isPending} onClick={() => resolve.mutate()}>{resolve.isPending ? '正在定位…' : label}</Button><TaskError error={resolve.error} /></span>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function taskDestination(target: TaskTarget): { to: string; search: Record<string, string> } | undefined {
 if(target.available&&target.kind==='ingestion_recovery'&&target.generation&&/^[1-9][0-9]*$/.test(target.generation)&&target.id&&/^[0-9a-f-]{36}$/i.test(target.id)&&target.scope.kind!=='project'){return {to:target.scope.kind==='platform'?'/platform/ingestion':`/t/${target.scope.tenant_id}/ingestion`,search:{recovery:target.id,generation:target.generation}};}
 if(target.available&&target.kind==='parsing_run'&&target.id&&/^[0-9a-f-]{36}$/i.test(target.id)&&target.generation&&/^[1-9][0-9]*$/.test(target.generation)&&target.scope.kind!=='project'){return {to:target.scope.kind==='platform'?'/platform/parsing':`/t/${target.scope.tenant_id}/parsing`,search:{run:target.id,generation:target.generation}};}
 if(target.available&&target.kind==='audit_export'&&target.id&&/^[0-9a-f-]{36}$/i.test(target.id)&&target.generation&&/^[1-9][0-9]*$/.test(target.generation)){const s=target.scope;const to=s.kind==='platform'?'/access/audit':`/t/${s.tenant_id}${s.project_id?`/p/${s.project_id}`:''}/audit`;return {to,search:{tab:'exports',export_id:target.id,execution:target.generation}};}
 if (!target.available || !target.id || !/^[0-9a-f-]{36}$/i.test(target.id) || !/^[0-9a-f-]{36}$/i.test(target.scope.tenant_id) || target.scope.project_id && !/^[0-9a-f-]{36}$/i.test(target.scope.project_id)) return;
 const base = `/t/${target.scope.tenant_id}${target.scope.project_id ? `/p/${target.scope.project_id}` : ''}`;
 let to: string, search: Record<string, string> = {};
 switch (target.kind) {
  case 'file_upload': to = `${base}/files`; search = { tab: 'uploads', upload: target.id, job: target.parameters?.job ?? '', generation: target.generation ?? '' }; break;
  case 'file_attachment': to = `${base}/files`; search = { attachment: target.id, revision: target.parameters?.revision ?? '' }; break;
  case 'file_host': to = `${base}/files`; break;
  case 'monitoring_import': to = `${base}/monitoring/imports/${target.id}`; break;
  case 'monitoring_query': to = `${base}/monitoring/jobs`; search = { job: target.parameters?.job_id ?? target.id, result: 'query' }; break;
  case 'monitoring_export': case 'monitoring_job': to = `${base}/monitoring/jobs`; search = { job: target.parameters?.job_id ?? target.id, result: 'files' }; break;
  case 'derived_run': to = `${base}/monitoring/derived-runs`; search = { id: target.id }; break;
  case 'analysis_export': to = `${base}/monitoring/derived-runs`; search = { export_id: target.id }; break;
  case 'derived_evidence_export': to = `${base}/monitoring/derived-runs`; search = { evidence_export_id: target.id }; break;
  case 'spatial_version': to = `${base}/spatial`; search = { mode: 'manage', version: target.id }; break;
  case 'display': to = `${base}/displays`; search = { instance: target.id }; break;
  default: return;
 }
 return safeReturn(to) === to ? { to, search } : undefined;
}
