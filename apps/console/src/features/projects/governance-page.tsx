import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useRef } from 'react';
import { Link } from '@tanstack/react-router';
import { Plus, Search, SlidersHorizontal, X } from 'lucide-react';
import { Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useTenant } from '../tenancy/context';
import { GovernanceCard, GovernanceOverview } from './governance-cards';
import { hasGovernanceAccess, ProjectGovernancePanel } from './governance-detail';
import type { GovernanceSearch, GovernanceView } from './governance-search';
import { retryScopeRead } from './scope';

type Props = { search: GovernanceSearch; onSearch: (search: GovernanceSearch) => void };
export function ProjectGovernancePage(props: Props) {
  const ctx = useTenant();
  return ctx.permissions.includes(P.TenantProjectRead) ? <GovernanceWorkspace {...props} /> : <p>没有项目治理读取权限。独立项目授权可通过左上角工作空间切换器进入项目。</p>;
}

function GovernanceWorkspace({ search, onSearch }: Props) {
  const ctx = useTenant(), { client, account } = useIdentity();
  const returnFocus = useRef<HTMLElement | null>(null), status = search.status ?? 'all';
  const list = usePagedQuery({ queryKey: ['project-governance', account?.id, ctx.tenant.id, ctx.membership_id, 'list', search.q ?? '', status, search.unavailable ?? false, search.owner ?? ''], enabled: ctx.available, staleTime: 0, retry: retryScopeRead, refetchInterval: 30_000, refetchOnWindowFocus: 'always', queryFn: async ({ cursor, limit, signal }) => { const response = await client.projectGovernance(ctx.tenant.id, { q: search.q, status, cursor, limit, administrator_unavailable: search.unavailable, owner_tenant_membership_id: search.owner }, signal); return { ...response, total: response.overview?.filtered_total }; } });
  const data = ctx.available && !list.error ? list.data : undefined;
  const overview = data?.overview, projects = data?.items.map(item => item.project) ?? [];
  const create = ctx.permissions.includes(P.TenantProjectCreate), access = hasGovernanceAccess(ctx.permissions);
  const filtered = !!search.q || status !== 'all' || !!search.owner || !!search.unavailable;
  const update = (next: Partial<GovernanceSearch>) => onSearch({ ...search, ...next });
  const clear = () => onSearch({ project: search.project, view: search.view, status: 'all' });
  const open = (id: string, view: GovernanceView, trigger: HTMLElement) => { returnFocus.current = trigger; update({ project: id, view }); };
  const selectedOwner = overview?.owners.find(owner => owner.tenant_membership_id === search.owner);
  const createButton = create ? <Button asChild disabled={!ctx.available}><Link to="/t/$tenantId/projects/new" params={{ tenantId: ctx.tenant.id }}><Plus aria-hidden="true" />创建项目</Link></Button> : null;
  return <section className="space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-4"><div><h1 className="text-2xl font-semibold">项目治理</h1><p className="mt-2 text-sm text-muted-foreground">以项目为中心，查看资源规模、管理团队与计划周期。</p></div>{createButton}</header>
    <GovernanceOverview value={overview} search={search} onFilter={filter => onSearch({ project: search.project, view: search.view, status: filter === 'active' ? 'active' : 'all', unavailable: filter === 'attention' ? true : undefined })} />
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-48 flex-1 space-y-1.5 lg:max-w-md"><Label htmlFor="project-governance-search" className="sr-only">搜索项目名称或编号</Label><div className="relative"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-4 text-muted-foreground" /><Input id="project-governance-search" value={search.q ?? ''} maxLength={100} placeholder="搜索项目名称或编号" className="h-10 bg-background pl-9" onChange={event => update({ q: event.target.value || undefined })} /></div></div>
        <div><Label className="sr-only" htmlFor="project-governance-status">项目状态</Label><select id="project-governance-status" className="h-10 rounded-md border bg-background px-3 text-sm" value={status} onChange={event => { const value = event.target.value; update({ status: value === 'active' || value === 'archived' ? value : 'all' }); }}><option value="all">全部状态</option><option value="active">运行中</option><option value="archived">已归档</option></select></div>
        <div className="min-w-0"><Label className="sr-only" htmlFor="project-governance-owner">项目负责人</Label><select id="project-governance-owner" className="h-10 max-w-56 rounded-md border bg-background px-3 text-sm" value={search.owner ?? ''} onChange={event => update({ owner: event.target.value || undefined })}><option value="">全部负责人</option>{search.owner && !selectedOwner && <option value={search.owner}>已选负责人</option>}{overview?.owners.map(owner => <option value={owner.tenant_membership_id} key={owner.tenant_membership_id}>{owner.display_name || owner.username}</option>)}</select></div>
        <label className="flex min-h-10 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={!!search.unavailable} aria-label="仅管理员不可用或空缺" onChange={event => update({ unavailable: event.target.checked || undefined })} /><SlidersHorizontal className="size-3.5 text-muted-foreground" aria-hidden="true" />仅需关注</label>
        <span className="ml-auto self-center text-xs text-muted-foreground">{overview ? '租户范围 · ' + overview.total + ' 个项目' : '正在读取项目统计'}</span>
      </div>
      {filtered && <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span>{overview ? '匹配 ' + overview.filtered_total + ' 个项目' : '正在筛选项目'}</span>{search.owner && selectedOwner && <span>负责人：{selectedOwner.display_name}</span>}<Button variant="ghost" size="sm" onClick={clear}><X className="size-3.5" aria-hidden="true" />清除筛选</Button></div>}
    </div>
    {list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !data ? <LoadingState label="正在加载项目…" /> : !projects.length ? <><div className="rounded-xl border bg-card"><EmptyState title={filtered ? '没有匹配的项目' : '从第一个项目开始'} description={filtered ? '试试其他名称、编号或筛选条件。' : '创建项目并指定负责人和管理员，建立项目业务空间。'}>{filtered ? <Button variant="outline" onClick={clear}>清除全部筛选</Button> : createButton}</EmptyState></div><ListPagination label="项目治理" {...list.pagination} /></> : <>
      <div className={'grid gap-5 md:grid-cols-2 ' + (projects.length > 2 || list.pagination.hasNext ? '2xl:grid-cols-3' : '')}>{projects.map(project => <GovernanceCard key={project.id} project={project} hasAccess={access} onOpen={open} onOwner={owner => update({ owner })} />)}</div>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground"><p role="status">已显示 {projects.length} / {overview?.filtered_total ?? projects.length} 个项目 · 按创建时间排序</p>{list.data && <ListPagination label="项目治理" {...list.pagination} />}</div>
      <p className="text-xs leading-6 text-muted-foreground">规模按各项目的有效授权范围展示，跨项目可能重复；“—”表示无查看权限。计划周期不代表完成进度。</p>
    </>}
    {search.project && <ProjectGovernancePanel key={search.project} project={search.project} view={search.view ?? 'summary'} setView={view => update({ view })} close={() => update({ project: undefined, view: undefined })} returnFocus={returnFocus} />}
  </section>;
}
