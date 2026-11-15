import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type TaskView = S<'TaskView'>;
export type TaskDetail = S<'TaskDetail'>;
export type TaskFilter = S<'TaskFilter'>;
export type TaskPage = S<'TaskPage'>;
export type TaskSummary = S<'TaskSummary'>;
export type TaskCommand = S<'TaskCommand'>;
export type TaskCommandResult = S<'TaskCommandResult'>;
export type TaskReference = S<'TaskRef'>;
export type TaskSourceReference = S<'TaskSourceRef'>;
export type TaskTarget = S<'TaskTarget'>;
export type TaskProgress = S<'TaskProgress'>;
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal, options?: MonitoringRequestOptions) => Promise<T>;
export function taskMethods(request: Request) {
 const query = (value: object) => new URLSearchParams(Object.entries(value).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString();
 const id = encodeURIComponent;
 return {
  tasks: (filter: TaskFilter = {}, signal?: AbortSignal) => request<TaskPage>(`/tasks?${query(filter)}`, 'GET', undefined, true, signal),
  taskSummary: (filter: TaskFilter = {}, signal?: AbortSignal) => request<TaskSummary>(`/tasks/summary?${query(filter)}`, 'GET', undefined, true, signal),
  taskCatalog: (filter: TaskFilter = {}, signal?: AbortSignal) => request<{ items: S<'TaskDefinition'>[] }>(`/tasks/catalog?${query(filter)}`, 'GET', undefined, true, signal),
  taskOperations: (filter: TaskFilter = {}, signal?: AbortSignal) => request<{ items: S<'TaskOperation'>[] }>(`/tasks/operations?${query(filter)}`, 'GET', undefined, true, signal),
  taskResolve: (ref: TaskSourceReference, signal?: AbortSignal) => request<TaskReference>(`/tasks/resolve?${query({ module: ref.source_module, kind: ref.source_kind, source_id: ref.source_root_id })}`, 'GET', undefined, true, signal),
  task: (value: string, generation?: string, signal?: AbortSignal) => request<TaskDetail>(`/tasks/${id(value)}?${query({ generation })}`, 'GET', undefined, true, signal),
  taskExecutions: (value: string, cursor?: string, signal?: AbortSignal) => request<S<'TaskExecutionPage'>>(`/tasks/${id(value)}/executions?${query({ cursor })}`, 'GET', undefined, true, signal),
  taskEvents: (value: string, generation: string, cursor?: string, signal?: AbortSignal) => request<S<'TaskEventPage'>>(`/tasks/${id(value)}/events?${query({ generation, cursor })}`, 'GET', undefined, true, signal),
  taskCommand: (value: string, input: TaskCommand, signal?: AbortSignal) => request<TaskCommandResult>(`/tasks/${id(value)}/commands`, 'POST', input, true, signal),
  taskTarget: (value: string, generation: string, kind = 'business', signal?: AbortSignal) => request<TaskTarget>(`/tasks/${id(value)}/target?${query({ generation, target_kind: kind })}`, 'GET', undefined, true, signal),
 };
}
