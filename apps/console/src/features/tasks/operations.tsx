import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Activity, ArrowLeft, ChevronRight } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { TenantBoundary } from '../tenancy/context';
import { ProjectBoundary } from '../projects/context';
import { ProjectMonitoring, TenantMonitoring } from '../monitoring/context';
import { stateNames } from '../monitoring/format';
import { reasonNames } from '../monitoring/detail-format';
import { reportTime } from '../derived-monitoring/analysis-format';
import { RunPanel } from '../derived-monitoring/run-panel';
import { useDerived } from '../derived-monitoring/common';
import { TaskError } from './common';
import { useTasks } from './provider';
import type { TaskSearch } from './search';

const modules: Record<string, [string, string]> = { monitoring: ['监测维护','monitoring'], derived: ['自动计算','monitoring/derived'], spatial: ['空间处理','spatial'], display: ['大屏包处理','displays'], files: ['文件检查与绑定','files'], audit: ['审计导出','audit'] };
type Props = { search: TaskSearch; change: (value: Partial<TaskSearch>) => void };

export function TaskOperations({ search, change }: Props) {
  const { client } = useIdentity(), tasks = useTasks();
  const filter = { tenant_id: search.tenant_id, project_id: search.project_id, scope_kind: search.scope_kind };
  const query = useQuery({ queryKey: [...tasks.key, 'operations', filter], queryFn: ({ signal }) => client.taskOperations(filter, signal), enabled: !search.automatic, refetchInterval: 30000, retry: false });
  if (search.automatic) {
    const [tenant, project] = search.automatic.split('/');
    if (!tenant) return null;
    const runs = <AutomaticRuns key={search.automatic} runID={search.run} select={id => change({ run: id })} />;
    return <div className="space-y-4"><Button variant="ghost" onClick={() => change(search.run ? { run: undefined } : { automatic: undefined })}><ArrowLeft aria-hidden />{search.run ? '返回自动运行列表' : '返回运行概况'}</Button>
      <TenantBoundary id={tenant}>{project ? <ProjectBoundary tenant={tenant} project={project} hideHeader><ProjectMonitoring>{runs}</ProjectMonitoring></ProjectBoundary> : <TenantMonitoring>{runs}</TenantMonitoring>}</TenantBoundary></div>;
  }
  const items = query.error ? [] : query.data?.items ?? [];
  return <div className="space-y-5"><TaskError error={query.error} retry={() => void query.refetch()} />
    {query.isPending && <p role="status">正在读取运行概况…</p>}
    {[...items].sort((a, b) => Number(b.source_module === 'derived') - Number(a.source_module === 'derived')).map(item => {
      const [label, path] = modules[item.source_module] ?? [item.source_module, ''];
      const key = [item.scope.tenant_id, item.scope.project_id].filter(Boolean).join('/');
      return <section key={item.source_module + key} className="space-y-3 rounded-xl border p-4"><div className="flex flex-wrap items-center justify-between gap-2"><div><h3 className="font-semibold">{label}</h3><p className="text-xs text-muted-foreground">{item.scope_label}</p></div>
        {item.source_module === 'derived' ? <Button variant="outline" onClick={() => change({ automatic: key, run: undefined })}>查看自动运行<ChevronRight aria-hidden /></Button> : path && <Button asChild variant="link"><a href={item.scope.kind === 'platform' ? '/access/audit' : `/t/${item.scope.tenant_id}${item.scope.project_id ? '/p/' + item.scope.project_id : ''}/${path}`}>打开来源工作区</a></Button>}</div>
        {item.state === 'available' ? <dl className="grid grid-cols-3 gap-3">{[['排队',item.queued],['运行',item.running],['失败',item.failed]].map(([name,value]) => <div key={name}><dt className="text-xs text-muted-foreground">{name}</dt><dd className="text-xl font-semibold">{value ?? '—'}</dd></div>)}</dl> : <p>暂时无法核实</p>}</section>;
    })}{!query.isPending && !query.error && !items.length && <p>当前范围没有可查看的运行概况。</p>}</div>;
}

function AutomaticRuns({ runID, select }: { runID?: string; select: (id: string) => void }) {
  const m = useDerived();
  const query = usePagedQuery({ queryKey: [...m.prefix, 'automatic-runs'], queryFn: ({ cursor, limit, signal }) => m.client.derivedRuns(m.scope, { mode: 'automatic', cursor, limit }, signal), enabled: !runID && m.allows('runs.read'), retry: false, refetchInterval: 10000 });
  if (!m.allows('runs.read')) return <p role="status">当前身份没有计算运行读取权限。</p>;
  if (runID) return <RunPanel key={runID} id={runID} automaticOnly />;
  return <section className="min-w-0 space-y-3"><h3 className="flex items-center gap-2 font-semibold"><Activity size={17} aria-hidden />自动计算运行</h3><TaskError error={query.error} retry={() => void query.refetch()} />
    {query.isPending && <p role="status">正在读取自动运行…</p>}{!query.error && query.data?.items.map(run => <button key={run.id} type="button" className="grid w-full gap-1 rounded-lg border p-3 text-left hover:bg-muted" onClick={() => select(run.id)}>
      <strong className="text-sm">{stateNames[run.job.state] ?? run.job.state} · {run.id.slice(0,8).toUpperCase()}</strong>
      <span className="text-sm">{reportTime(run.output_range.from_ns)} 至 {run.output_range.to_ns ? reportTime(run.output_range.to_ns) : '持续计算'}</span>
      {run.job.error_code && <span className="text-xs text-destructive">{reasonNames[run.job.error_code] ?? run.job.error_code}</span>}</button>)}
    {query.data && !query.error && <>{!query.data.items.length && <p className="text-sm">此页没有可查看的自动运行。</p>}<ListPagination label="自动运行" {...query.pagination} /></>}
  </section>;
}
