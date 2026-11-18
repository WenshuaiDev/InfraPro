import { Navigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ApiError } from '@infrapro/api-client';
import { useMonitoring } from '../monitoring/context';
import { TaskError } from './common';
import { workspaceTaskFilter } from './search';

export function LegacyTaskLocation({ id, kind, execution }: { id: string; kind: 'job' | 'run'; execution?: string }) {
  const m = useMonitoring();
  const filter = workspaceTaskFilter(m.scope);
  const automatic = [filter.tenant_id, filter.project_id].filter(Boolean).join('/');
  const target = useQuery({
    queryKey: [...m.prefix, 'legacy-task-location', kind, id, execution], retry: false,
    queryFn: async ({ signal }) => {
      const source = { source_module: kind === 'run' ? 'derived' : 'monitoring', source_kind: kind, source_root_id: id };
      try {
        const ref = await m.client.taskResolve(source, signal);
        if (ref.task_id) {
          const task = await m.client.task(ref.task_id, execution ?? ref.generation, signal);
          if (task.scope.tenant_id === filter.tenant_id && (task.scope.project_id || undefined) === filter.project_id) {
            return { task: task.id, generation: task.generation };
          }
        }
      } catch (error) {
        if (!(error instanceof ApiError) || (![403,404].includes(error.status ?? 0) && error.code !== 'index_pending')) throw error;
      }
      if (kind === 'job') {
        const job = await m.client.monitoringJob(m.scope, id, signal);
        if (job.task_ref?.task_id) return { task: job.task_ref.task_id, generation: execution ?? job.task_ref.generation };
        if (job.kind !== 'derive') throw Error('当前范围无法定位这项旧任务。');
        const run = await m.client.derivedRun(m.scope, job.target_id, signal);
        if (run.mode === 'automatic') return { run: run.id };
        if (run.job.task_ref?.task_id) return { task: run.job.task_ref.task_id, generation: execution ?? run.job.task_ref.generation };
      } else {
        const run = await m.client.derivedRun(m.scope, id, signal);
        if (run.mode === 'automatic') return { run: run.id };
        if (run.job.task_ref?.task_id) return { task: run.job.task_ref.task_id, generation: execution ?? run.job.task_ref.generation };
      }
      throw Error('当前范围无法定位这项旧任务。');
    },
  });
  if (target.error) return <TaskError error={target.error} retry={() => void target.refetch()} />;
  if (target.data?.run) return <Navigate replace to="/tasks" search={{ ...filter, operations: 'open', automatic, run: target.data.run }} />;
  if (target.data?.task) return <Navigate replace to="/tasks/$taskId" params={{ taskId: target.data.task }} search={{ ...filter, execution: target.data.generation }} />;
  return <p role="status">正在定位本次执行…</p>;
}
