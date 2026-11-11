// THROWAWAY: one searchable entry for workspaces. Uses existing read APIs only.
import { useState, type ComponentType } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useLocation, useNavigate } from '@tanstack/react-router';
import { Building2, Check, ChevronsUpDown, FolderKanban, House, Layers3 } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import type { MyTenant, ProjectContext, TenantContext } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { platformNavigation, safeReturn, workspaceScope } from '../config/navigation';

type Group = { id: string; name: string; tenant?: MyTenant; projects: ProjectContext[] };
function Item({ name, description, icon: Icon, current, disabled, onSelect }: { name: string; description: string; icon: ComponentType<{ className?: string }>; current?: boolean; disabled?: boolean; onSelect: () => void }) {
  return <DropdownMenuItem onSelect={onSelect} disabled={disabled} aria-current={current ? 'true' : undefined} className="min-h-14 cursor-pointer gap-3 rounded-lg px-3 py-2"><Icon className="size-4 shrink-0" /><span className="grid min-w-0 flex-1 gap-1"><span className="break-words font-medium">{name}</span><span className="text-xs text-muted-foreground">{description}</span></span>{current && <Check className="size-4 shrink-0" />}</DropdownMenuItem>;
}

export function WorkspaceSwitcherPrototype({ path, tenant, project }: { path: string; tenant?: TenantContext; project?: ProjectContext }) {
  const { account, client, can } = useIdentity();
  const search = useLocation({ select: (location) => location.search });
  const navigate = useNavigate();
  const [localOpen, setLocalOpen] = useState(false);
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<'all' | 'tenant' | 'project'>('all');
  const open = localOpen || Boolean(search.picker);
  const mode = search.picker ?? filter;
  const scope = workspaceScope(path);
  const platform = platformNavigation(path, can)[0]?.items[0];
  const tenants = useInfiniteQuery({ queryKey: ['account', account?.id, 'my-tenants', 'navigation-prototype', q], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.myTenants({ cursor: pageParam, limit: 50, q }, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: open, staleTime: 0 });
  const projects = useInfiniteQuery({ queryKey: ['account', account?.id, 'my-projects', 'navigation-prototype', q], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.myProjects({ cursor: pageParam, limit: 50, status: 'all', q }, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: open, staleTime: 0 });
  const tenantItems = tenants.isError ? [] : tenants.data?.pages.flatMap((page) => page.items) ?? [];
  const projectItems = projects.isError ? [] : projects.data?.pages.flatMap((page) => page.items) ?? [];
  const groups = new Map<string, Group>(tenantItems.map((item) => [item.id, { id: item.id, name: item.name, tenant: item, projects: [] }]));
  for (const item of projectItems) { const group = groups.get(item.project.tenant_id) ?? { id: item.project.tenant_id, name: item.project.tenant_name, projects: [] }; group.projects.push(item); groups.set(group.id, group); }
  const visible = [...groups.values()].filter((group) => mode === 'project' ? group.projects.length : mode === 'tenant' ? group.tenant : true);
  const name = scope.kind === 'project' ? project?.project.name ?? '项目工作空间' : scope.kind === 'tenant' ? tenant?.tenant.name ?? '租户工作空间' : scope.kind === 'platform' ? '平台工作空间' : '选择工作空间';
  const detail = scope.kind === 'project' ? project?.project.tenant_name ?? '项目' : scope.kind === 'tenant' ? '租户' : scope.kind === 'platform' ? '平台' : '租户 / 项目 / 平台';
  const Icon = scope.kind === 'project' ? FolderKanban : scope.kind === 'tenant' ? Building2 : Layers3;
  const go = (href: string) => { setLocalOpen(false); setQ(''); void navigate({ to: safeReturn(href), search: { variant: search.variant ?? 'A' } }); };
  const changeOpen = (next: boolean) => { setLocalOpen(next); if (!next) { setQ(''); if (search.picker) void navigate({ to: path, search: { variant: search.variant ?? 'A' }, replace: true }); } };
  return <DropdownMenu open={open} onOpenChange={changeOpen}>
    <DropdownMenuTrigger asChild><Button variant="ghost" aria-label={`切换工作空间：${name}`} className="h-14 w-full min-w-0 justify-start gap-2 px-2 text-start"><span className="hidden size-8 shrink-0 items-center justify-center rounded-md border bg-muted/40 sm:flex"><Icon className="size-4" /></span><span className="grid min-w-0 flex-1 gap-0.5"><span className="truncate text-sm font-medium" title={name}>{name}</span><span className="truncate text-xs font-normal text-muted-foreground" title={detail}>{detail}</span></span><ChevronsUpDown className="size-4 shrink-0" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="start" side="bottom" sideOffset={8} collisionPadding={12} aria-label="选择工作空间" className="w-[420px] max-w-[calc(100vw-24px)] rounded-xl p-2">
      <DropdownMenuLabel className="px-3 py-2">切换工作空间</DropdownMenuLabel>
      <div className="space-y-3 px-2 pb-3"><label htmlFor="prototype-workspace-query" className="sr-only">搜索租户或项目名称、编号</label><Input id="prototype-workspace-query" placeholder="搜索名称或编号…" value={q} onChange={(event) => setQ(event.target.value)} onKeyDown={(event) => { if (event.key === 'ArrowDown') { event.preventDefault(); event.currentTarget.closest('[role="menu"]')?.querySelector<HTMLElement>('[role="menuitem"]:not([data-disabled])')?.focus(); } if (event.key !== 'Escape') event.stopPropagation(); }} /><div className="flex gap-1" aria-label="工作空间类型">{(['all', 'tenant', 'project'] as const).map((value) => <Button key={value} size="sm" variant={mode === value ? 'secondary' : 'ghost'} aria-pressed={mode === value} onClick={() => { setFilter(value); if (search.picker) { setLocalOpen(true); void navigate({ to: path, search: { variant: search.variant ?? 'A' }, replace: true }); } }}>{value === 'all' ? '全部' : value === 'tenant' ? '租户' : '项目'}</Button>)}</div></div>
      <div className="max-h-[min(55vh,480px)] overflow-y-auto">
        {!q && mode === 'all' && <><Item name="首页" description="返回个人账号首页" icon={House} current={path === '/'} onSelect={() => go('/')} />{platform && <Item name="平台工作空间" description="平台管理与运营" icon={Layers3} current={scope.kind === 'platform'} onSelect={() => go(platform.href)} />}<DropdownMenuSeparator /></>}
        {(tenants.isPending || projects.isPending) && <p role="status" className="p-3 text-sm text-muted-foreground">正在加载工作空间…</p>}
        {(tenants.isError || projects.isError) && <DropdownMenuItem onSelect={(event) => { event.preventDefault(); void tenants.refetch(); void projects.refetch(); }}>加载失败，点击重试</DropdownMenuItem>}
        {visible.map((group) => <DropdownMenuGroup key={group.id} aria-label={group.name} className="border-b py-1 last:border-0">
          {mode !== 'project' ? <Item name={group.name} description={group.tenant ? group.tenant.status === 'disabled' ? '已停用 · 查看状态' : group.tenant.can_enter ? `租户 · ${group.tenant.code}` : '租户暂不可进入' : '所属租户 · 无租户入口'} icon={Building2} current={scope.kind === 'tenant' && scope.tenantId === group.id} disabled={!group.tenant || (!group.tenant.can_enter && group.tenant.status !== 'disabled')} onSelect={() => go(`/t/${group.id}/profile`)} /> : <DropdownMenuLabel className="break-words text-xs font-normal text-muted-foreground">{group.name}</DropdownMenuLabel>}
          {mode !== 'tenant' && <div className={mode === 'all' ? 'ms-5 border-s ps-2' : ''}>{group.projects.map((item) => <Item key={item.project.id} name={item.project.name} description={`${item.project.status === 'archived' ? '已归档' : '项目'} · ${item.project.code}${item.can_enter ? '' : ' · 暂不可进入'}`} icon={FolderKanban} current={scope.projectId === item.project.id} disabled={!item.can_enter} onSelect={() => go(`/t/${group.id}/p/${item.project.id}/profile`)} />)}</div>}
        </DropdownMenuGroup>)}
        {tenants.isSuccess && projects.isSuccess && !visible.length && <p className="p-4 text-sm text-muted-foreground">{q ? '没有匹配的工作空间。' : '尚未加入租户或项目。'}</p>}
        {tenants.hasNextPage && mode !== 'project' && <DropdownMenuItem disabled={tenants.isFetchingNextPage} onSelect={(event) => { event.preventDefault(); void tenants.fetchNextPage(); }}>加载更多租户</DropdownMenuItem>}
        {projects.hasNextPage && mode !== 'tenant' && <DropdownMenuItem disabled={projects.isFetchingNextPage} onSelect={(event) => { event.preventDefault(); void projects.fetchNextPage(); }}>加载更多项目</DropdownMenuItem>}
      </div>
    </DropdownMenuContent>
  </DropdownMenu>;
}
