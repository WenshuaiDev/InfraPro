import { Navigate } from '@tanstack/react-router';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { MonitoringPageHeader } from '../monitoring/page-header';
import { TaskError } from '../tasks/common';
import { LegacyTaskLocation } from '../tasks/legacy-location';
import { workspaceTaskFilter } from '../tasks/search';
import { ObservationRows } from './channel-history';
import { useDerived } from './common';
import { RunPanel } from './run-panel';
import { AnalysisExportTask } from './export-tasks';
import type { DerivedSearch } from './search';

export function DerivedRunsPage({ id, export_id, evidence_export_id, view, version, execution }: DerivedSearch) {
  const m = useDerived();
  if (export_id || evidence_export_id) return <section className="space-y-4"><MonitoringPageHeader workspaceTasks title="固定版本导出" description="查看本次导出和可下载的文件。" /><AnalysisExportTask id={(export_id ?? evidence_export_id)!} generation={!!evidence_export_id} /></section>;
  if (id && view === 'generation' && version) return <FixedGeneration run={id} version={version} />;
  if (id && view === 'result') return <section className="space-y-4"><MonitoringPageHeader workspaceTasks title="计算运行详情" description="查看本次运行的输出和执行依据。" /><RunPanel key={id} id={id} /></section>;
  if (id) return <LegacyTaskLocation kind="run" id={id} execution={execution} />;
  const filter = workspaceTaskFilter(m.scope);
  return <Navigate replace to="/tasks" search={{ ...filter, source: 'derived' }} />;
}
function FixedGeneration({ run, version }: { run: string; version: string }) {
  const m = useDerived();
  const result = usePagedQuery({ queryKey: [...m.prefix, 'task-generation', run, version], queryFn: async ({ signal, cursor, limit }) => {
    const data = await m.client.derivedGenerationDetails(m.scope, version, { cursor, limit }, signal);
    if (data.generation.run_id !== run) throw Error('该发布代次与所选运行不匹配。');
    return data;
  }, retry: false });
  return <section className="space-y-4"><MonitoringPageHeader workspaceTasks title="本次发布结果" description="查看所选运行发布的固定派生结果。" /><TaskError error={result.error} retry={() => void result.refetch()} />{result.isPending && <p role="status">正在读取发布结果…</p>}{!result.error && result.data && <><p className="break-all text-xs text-muted-foreground">发布代次 {version}</p><ObservationRows items={result.data.items} /><ListPagination label="发布结果" {...result.pagination} /></>}</section>;
}
