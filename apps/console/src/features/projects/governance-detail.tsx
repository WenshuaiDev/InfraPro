import type { RefObject } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowUpRight, Boxes, CalendarDays, Cpu, Map, ShieldCheck, TriangleAlert, Waves } from 'lucide-react';
import { Permissions as P, type PermissionCode, type ProjectSummary } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useTenant } from '../tenancy/context';
import { ObjectHistory } from '../audit/object-history';
import { ProjectActionButton } from './actions';
import { ProjectScale, ProjectStatus } from './governance-cards';
import { projectDate, plannedEndNotice, type GovernanceView } from './governance-search';
import { retryScopeRead } from './scope';

const entries = [
  { label: '资源授权', icon: Boxes, description: '查看和管理设施、构件、测点与通道的授权范围。', to: '/t/$tenantId/resource-access/$projectId', permissions: [P.TenantEngineeringResourceRead, P.TenantEngineeringResourceAuthorizationRead] },
  { label: '设备授权', icon: Cpu, description: '查看和管理设备与设备输出的授权范围。', to: '/t/$tenantId/projects/$projectId/device-access', permissions: [P.TenantDeviceRead, P.TenantDeviceAuthorizationRead] },
  { label: '空间资料授权', icon: Map, description: '查看和管理图纸、地图、模型等空间资料的授权。', to: '/t/$tenantId/projects/$projectId/spatial-access', permissions: [P.TenantProjectSpatialAccessRead] },
  { label: '监测数据授权', icon: Waves, description: '查看和管理项目可访问的监测数据范围。', to: '/t/$tenantId/projects/$projectId/monitoring-access', permissions: [P.TenantMonitoringAuthorizationRead] },
] as const;

// eslint-disable-next-line react-refresh/only-export-components
export const hasGovernanceAccess = (permissions: readonly PermissionCode[]) => entries.some(entry => entry.permissions.every(code => permissions.includes(code)));

function useGovernanceDetail(project: string) {
  const ctx = useTenant(), { account, client } = useIdentity();
  return useQuery({
    queryKey: ['project-governance', account?.id, ctx.tenant.id, ctx.membership_id, project],
    queryFn: ({ signal }) => client.projectSummary(ctx.tenant.id, project, signal),
    enabled: ctx.available && ctx.permissions.includes(P.TenantProjectRead),
    staleTime: 0, retry: retryScopeRead, refetchInterval: 30_000, refetchOnWindowFocus: 'always',
  });
}

function GovernanceAccess({ project }: { project: ProjectSummary }) {
  const ctx = useTenant();
  const allowed = entries.filter(entry => entry.permissions.every(code => ctx.permissions.includes(code)));
  return <section className="space-y-4" aria-label="项目授权入口">
    <div><h3 className="font-medium">项目访问授权</h3><p className="mt-2 text-xs leading-5 text-muted-foreground">各类授权分别控制项目可访问的范围。</p></div>
    {allowed.length ? <div className="divide-y overflow-hidden rounded-xl border">{allowed.map(entry => <Link key={entry.label} to={entry.to} params={{ tenantId: ctx.tenant.id, projectId: project.id }} className="flex min-h-20 items-center gap-3 p-4 hover:bg-muted/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring">
      <entry.icon aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" /><div className="min-w-0 flex-1"><h4 className="text-sm font-medium">{entry.label}</h4><p className="mt-1 text-xs leading-5 text-muted-foreground">{entry.description}</p></div><ArrowUpRight aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
    </Link>)}</div> : <p className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground">当前没有这些授权范围的查看权限。</p>}
  </section>;
}

function GovernanceSummary({ project: p }: { project: ProjectSummary }) {
  const ctx = useTenant(), attention = !p.administrator.available, plan = plannedEndNotice(p.planned_end, p.status);
  return <div className="space-y-6">
    {attention ? <section className="space-y-3 rounded-lg border border-amber-200 bg-amber-50 p-4 text-amber-900 dark:border-amber-900 dark:bg-amber-950/25 dark:text-amber-200" aria-label="项目管理问题">
      <h3 className="flex items-center gap-2 text-sm font-medium"><TriangleAlert className="size-4 shrink-0" aria-hidden="true" />{p.management_gap ? '管理员空缺，需要恢复管理' : '管理员不可用，需要恢复管理'}</h3>
      <p className="text-xs leading-5">指定可履职的项目管理员，恢复项目管理职责。</p>
      {ctx.permissions.includes(P.TenantProjectRecoverAdmin) && <ProjectActionButton tenant={ctx.tenant.id} project={p.id} action="administrator-recovery" label="恢复项目管理员" disabled={!ctx.available} />}
    </section> : <p className="flex items-center gap-2 rounded-lg bg-muted/50 p-4 text-sm"><ShieldCheck aria-hidden="true" className="size-4 shrink-0 text-emerald-700 dark:text-emerald-300" />管理员可履职，当前没有管理空缺。</p>}
    <dl className="grid grid-cols-2 gap-x-5 gap-y-6 text-sm">
      <div><dt className="text-xs text-muted-foreground">项目负责人</dt><dd className="mt-2 break-words">{p.owner.display_name || p.owner.username}{!p.owner.available && <span className="text-amber-700 dark:text-amber-300">（不可用）</span>}</dd></div>
      <div><dt className="text-xs text-muted-foreground">项目管理员</dt><dd className="mt-2 break-words">{p.management_gap ? '待指定' : p.administrator.display_name || p.administrator.username}{!p.management_gap && !p.administrator.available && <span className="text-amber-700 dark:text-amber-300">（不可用）</span>}</dd></div>
      <div className="col-span-2"><dt className="flex items-center gap-2 text-xs text-muted-foreground"><CalendarDays aria-hidden="true" className="size-3.5" />计划周期</dt><dd className="mt-2 tabular-nums">{projectDate(p.planned_start)} — {projectDate(p.planned_end)}</dd>{plan && <dd className="mt-2 text-xs text-amber-700 dark:text-amber-300">{plan}，请核对后续计划。</dd>}</div>
    </dl>
    <section className="space-y-4 border-y py-5"><h3 className="text-sm font-medium">项目规模</h3><ProjectScale statistics={p.statistics} /><p className="text-xs leading-5 text-muted-foreground">按有效授权统计设施、测点和设备；当前成员含暂停成员。{p.statistics && Object.values(p.statistics).some(value => value === null) && '“—”表示无查看权限。'}</p></section>
    <dl className="grid grid-cols-2 gap-4 text-xs"><div><dt className="text-muted-foreground">创建时间</dt><dd className="mt-2 break-words">{projectTime(p.created_at)}</dd></div><div><dt className="text-muted-foreground">最近更新</dt><dd className="mt-2 break-words">{projectTime(p.updated_at)}</dd></div></dl>
    {p.status === 'archived' && <p className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground">项目已归档，保留治理摘要与当前授权范围供查询。</p>}
    <p className="text-xs leading-6 text-muted-foreground">业务资料由项目成员权限控制。项目成员可通过工作空间切换器进入项目。</p>
  </div>;
}

function projectTime(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '未记录';
}

export function ProjectGovernancePanel({ project, view, setView, close, returnFocus }: { project: string; view: GovernanceView; setView: (view: GovernanceView) => void; close: () => void; returnFocus: RefObject<HTMLElement | null> }) {
  const ctx = useTenant(), detail = useGovernanceDetail(project);
  const p = ctx.available && !detail.error ? detail.data : undefined;
  const access = hasGovernanceAccess(ctx.permissions);
  return <ResponsivePanel open onOpenChange={open => { if (!open) close(); }} title={p?.name ?? '项目治理详情'} description={p?.code ?? '查看管理状态、计划周期和项目授权范围。'} returnFocus={returnFocus} fallbackFocusSelector="#project-governance-search">
    {detail.error ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : !p ? <LoadingState label="正在读取项目治理摘要…" /> : <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3"><ProjectStatus status={p.status} /><ObjectHistory objectType="project" objectId={p.id} label="操作历史" variant="ghost" /></div>
      <div className="flex gap-2 border-b pb-3" role="group" aria-label="项目治理详情内容">
        <Button variant={view === 'summary' ? 'secondary' : 'ghost'} aria-pressed={view === 'summary'} onClick={() => setView('summary')}>治理摘要</Button>
        {access && <Button variant={view === 'access' ? 'secondary' : 'ghost'} aria-pressed={view === 'access'} onClick={() => setView('access')}>授权概览</Button>}
      </div>
      {view === 'access' ? <GovernanceAccess project={p} /> : <GovernanceSummary project={p} />}
    </div>}
  </ResponsivePanel>;
}

export function ProjectGovernanceDetail({ project }: { project: string }) {
  const ctx = useTenant(), detail = useGovernanceDetail(project);
  if (!ctx.permissions.includes(P.TenantProjectRead)) return <p>没有项目治理读取权限。</p>;
  const p = ctx.available && !detail.error ? detail.data : undefined;
  return <section className="mx-auto max-w-4xl space-y-6">
    <Button asChild variant="ghost"><Link to="/t/$tenantId/projects" params={{ tenantId: ctx.tenant.id }}><ArrowLeft aria-hidden="true" />返回项目治理</Link></Button>
    {detail.error ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : !p ? <LoadingState label="正在读取项目治理摘要…" /> : <>
      <div className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="text-2xl font-semibold break-words">{p.name}</h1><p className="mt-2 text-sm break-all text-muted-foreground">{p.code}</p></div><ProjectStatus status={p.status} /></div>
      <div className="rounded-xl border bg-card p-5 sm:p-6"><GovernanceSummary project={p} /></div>
      <GovernanceAccess project={p} /><ObjectHistory objectType="project" objectId={p.id} />
    </>}
  </section>;
}
