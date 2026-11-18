import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { TaskReference, TaskSourceReference } from '@infrapro/api-client';
import { useTasks } from './provider';
import { TaskError, TaskLink, TaskProgress, TaskStatus } from './common';
import { TaskActions } from './detail';

// Business details keep their specialized reports; state and commands use the
// same current authorization and capability response as the global center.
export function SourceTaskPanel({ source, controls = true }: { source: TaskSourceReference & Partial<TaskReference>; controls?: boolean }) {
 const { client } = useIdentity(), t = useTasks();
 const resolve = useQuery({ queryKey: [...t.key, 'resolve', source], queryFn: ({ signal }) => client.taskResolve(source, signal), enabled: !source.task_id, retry: false, refetchInterval: q => q.state.data ? false : 3000, refetchIntervalInBackground: false });
 const id = source.task_id ?? resolve.data?.task_id;
 const query = useQuery({ queryKey: [...t.key, 'detail', id, source.generation], queryFn: ({ signal }) => client.task(id!, source.generation, signal), enabled: !!id, retry: false, refetchInterval: q => q.state.data && ['queued', 'running', 'waiting', 'retry_wait', 'cancelling'].includes(q.state.data.state) ? 3000 : 15000, refetchIntervalInBackground: false });
 const task = query.error || resolve.error ? undefined : query.data;
 return <section className="space-y-3 rounded-lg border p-3" aria-label="统一任务状态">{source.task_id && <TaskLink source={source} />}<TaskError error={resolve.error ?? query.error} retry={() => void (id ? query.refetch() : resolve.refetch())} />{task && <><TaskStatus task={task} /><TaskProgress phase={task.phase} progress={task.progress} />{!source.task_id && <TaskLink source={resolve.data ?? source} />}{controls && <TaskActions task={task} />}</>}{!task && !resolve.error && !query.error && <p role="status" className="text-sm">正在核实任务…</p>}</section>;
}
