import { timestamp } from './format';
import { Link } from '@tanstack/react-router';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { RefreshCw } from 'lucide-react';
import { reason, type Overview as OverviewData } from '@infrapro/diagnostics-client';
import { useDiagnosticQuery } from './hooks';
import { CoverageText, DataTable, Feedback, LiveRegion, Panel, Status } from './patterns';
export function Overview() {
  const query = useDiagnosticQuery<OverviewData>('/overview'); const data = query.data?.data; const old = !!query.error;
  return <div className="space-y-6"><PageHeader title="运行诊断" description={data?.instance.display_name ?? '独立查看本实例运行证据'} actions={<Button variant="outline" onClick={() => { if (!query.isFetching) void query.refetch(); }} aria-disabled={query.isFetching} aria-busy={query.isFetching}><RefreshCw className="size-4" />刷新</Button>} />
    <Feedback error={query.error} meta={query.data?.meta} pending={query.isPending} />
    {data && <>
      <div className="flex flex-wrap justify-between gap-2 text-sm text-muted-foreground"><span>最后刷新：{timestamp(query.data?.meta.server_time)}</span><span>只读 · 前台每 5 秒刷新</span></div>
      {query.data?.meta.diagnostics_state && Object.values(query.data.meta.diagnostics_state).some((value) => value !== 'ok') && <div className="rounded-lg border border-amber-600/40 bg-amber-50 p-4 text-sm text-amber-950 dark:bg-amber-950 dark:text-amber-100">诊断自身存在退化或配置缺项。<Link to="/self" className="ms-2 underline">查看诊断自身</Link></div>}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{[['期望常驻副本', data.summary.expected_replicas], ['新鲜可用副本', old ? '待确认' : data.summary.ready_replicas], ['已确认不可用', old ? '历史数据' : data.summary.failed_replicas], ['未知副本', data.summary.unknown_replicas]].map(([title, value]) => <Panel key={title} title={String(title)}><p className="text-3xl font-semibold tabular-nums">{value}</p></Panel>)}</div>
      <Panel title="观察覆盖"><div className="flex flex-wrap items-center justify-between gap-4"><CoverageText value={data.coverage} stale={old} /><p className="text-sm">观察器在线 {old ? '待确认' : data.observer_summary.online}/{data.observer_summary.expected}</p></div><p className="text-sm text-muted-foreground">失败的检查也属于有效证据；未配置、无权限和过期会显示覆盖缺口。</p></Panel>
      {data.resource_warnings.length > 0 && <Panel title={old ? '历史资源提示' : '资源提示'}><ul className="space-y-2 text-sm">{data.resource_warnings.map((warning) => <li key={`${warning.probe_id}-${warning.reason_code}-${warning.dimension_id}`}><Link to="/nodes/$nodeId" params={{ nodeId: warning.node_id }} className="underline">{warning.node_id}</Link> · {reason(warning.reason_code).title} {warning.dimension_id && `（${warning.dimension_id}）`}</li>)}</ul></Panel>}
      <Panel title="需要关注的组件"><DataTable><thead><tr><th scope="col">组件</th><th scope="col">状态</th><th scope="col">副本可用性</th><th scope="col">覆盖</th></tr></thead><tbody>{data.abnormal_components.map((component) => <tr key={component.id}><td><Link to="/components/$componentId" params={{ componentId: component.id }}>{component.display_name}</Link></td><td><Status value={component.display_state} stale={old} /></td><td>{component.ready_replicas}/{component.expected_replicas} 可用 · {component.failed_replicas} 不可用 · {component.unknown_replicas} 未知</td><td><CoverageText value={component.coverage} stale={old} /></td></tr>)}{data.abnormal_components.length === 0 && <tr><td colSpan={4}>当前没有已记录的异常组件。仍请核对上方观察覆盖。</td></tr>}</tbody></DataTable><Link to="/components" className="text-sm underline">查看全部组件与未启用项</Link></Panel>
      <Panel title="最近状态变化"><ul className="space-y-3 text-sm">{data.recent_transitions.slice(0, 8).map((transition) => <li key={transition.id} className="flex flex-wrap gap-x-4 gap-y-1"><time className="text-muted-foreground">{timestamp(transition.at)}</time><span>{transition.target_id}</span><span>{reason(transition.reason_code).title}</span></li>)}{data.recent_transitions.length === 0 && <li className="text-muted-foreground">尚无状态变化。每次心跳不会新增记录。</li>}</ul><Link to="/history" className="text-sm underline">查看有限历史</Link></Panel>
      <LiveRegion message={old ? '诊断连接中断，正在显示历史数据。' : `运行诊断：${data.summary.failed_replicas} 个副本不可用，${data.summary.unknown_replicas} 个未知，${data.coverage.unavailable} 项观察缺口。`} />
    </>}
  </div>;
}
