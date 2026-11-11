import { useEffect, useRef, useState, type ComponentType } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Building2, Check, ChevronsUpDown, FolderKanban, Layers3 } from 'lucide-react';
import type { MyTenant, ProjectContext, TenantContext } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import type { WorkspaceSwitcherHost } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { SidebarMenuButton } from '@infrapro/ui/components/sidebar';
import { brand } from '../config/brand';
import { platformNavigation, safeReturn, workspaceScope } from '../config/navigation';
import { useWorkspacePicker, type WorkspaceFilter } from './workspace-picker-context';

type WorkspaceItemProps = {
  name: string;
  description: string;
  icon: ComponentType<{ className?: string }>;
  current?: boolean;
  action?: string;
  onSelect: () => void;
};
type TenantGroup = { id: string; name: string; tenant?: MyTenant; projects: ProjectContext[] };

function WorkspaceItem({ name, description, icon: Icon, current, action, onSelect }: WorkspaceItemProps) {
  return <DropdownMenuItem onSelect={onSelect} aria-current={current ? 'true' : undefined} className={`min-h-12 cursor-pointer gap-3 rounded-md p-2 ${current ? 'bg-accent' : ''}`}>
    <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background"><Icon className="size-4" aria-hidden="true" /></span>
    <span className="grid min-w-0 flex-1 gap-0.5"><span className="truncate font-medium" title={name}>{name}</span><span className="text-xs text-muted-foreground break-words">{description}</span>{action && <span className="text-xs font-medium underline">{action}</span>}</span>
    {current && <><Check className="size-4 text-foreground" aria-hidden="true" /><span className="sr-only">当前工作空间</span></>}
  </DropdownMenuItem>;
}

function projectDescription(item: ProjectContext) {
  const auth = item.authorization;
  const labels: Record<string, string> = { tenant_disabled: '租户已停用', account_unavailable: '账号不可用', membership_suspended: '项目成员已暂停', membership_ended: '项目关系已结束', tenant_membership_suspended: '租户成员已暂停', tenant_membership_ended: '租户关系已结束', no_project_permissions: '尚未获项目业务授权' };
  const fallback = auth.tenant_status === 'disabled' ? '租户已停用' : auth.tenant_membership_status === 'suspended' ? '租户成员已暂停' : auth.membership_status === 'suspended' ? '项目成员已暂停' : !item.can_enter ? '尚未获项目业务授权' : '';
  const reason = auth.inactive_reasons.map((code) => labels[code] ?? code).join('、') || fallback;
  return `${item.project.status === 'archived' ? '已归档' : '项目'} · ${item.project.code} · ${item.project.tenant_name} · 本人角色：${auth.roles.map((role) => role.name).join('、') || '未获业务授权'}${reason ? ` · ${reason}` : ''}`;
}
function tenantDescription(item: MyTenant) {
  const reason = item.status === 'disabled' || item.restriction_code === 'tenant_disabled' ? '已停用' : item.restriction_code === 'membership_suspended' ? '成员已暂停' : !item.can_enter ? '暂不可进入' : '';
  return `租户 · ${item.code} · 本人角色：${item.roles.map((role) => role.name).join('、') || '未获业务授权'}${reason ? ` · ${reason}` : ''}`;
}

type Props = { path: string; tenant?: TenantContext; project?: ProjectContext; host: WorkspaceSwitcherHost };
export function WorkspaceSwitcher({ path, tenant, project, host }: Props) {
  const picker = useWorkspacePicker();
  const navigate = useNavigate();
  const selected = useRef(false);
  const scope = workspaceScope(path);
  const name = scope.kind === 'project' ? project?.project.name ?? '项目工作空间' : scope.kind === 'tenant' ? tenant?.tenant.name ?? '租户工作台' : scope.kind === 'platform' ? '平台工作空间' : brand.name;
  const description = scope.kind === 'project' ? `项目 · ${project?.project.tenant_name ?? '所属租户'}` : scope.kind === 'tenant' ? '租户工作台' : scope.kind === 'platform' ? brand.name : '选择工作空间';
  const Icon = scope.kind === 'project' ? FolderKanban : scope.kind === 'tenant' ? Building2 : scope.kind === 'platform' ? Layers3 : null;
  const trigger = <><span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground [&>svg]:size-5">{Icon ? <Icon aria-hidden="true" /> : brand.logo}</span><span className="grid min-w-0 flex-1 text-start text-sm leading-tight"><span className="truncate font-semibold" title={name}>{name}</span><span className="truncate text-xs text-muted-foreground" title={description}>{description}</span></span><ChevronsUpDown className="ms-auto size-4 shrink-0" aria-hidden="true" /></>;
  const go = async (href: string) => {
    selected.current = true;
    picker.close();
    host.onSelected();
    if (path !== href) await navigate({ to: safeReturn(href) });
    // Navigation can replace the old trigger. Focus the new page, not a detached node.
    requestAnimationFrame(() => document.getElementById('main')?.focus());
  };
  return <DropdownMenu modal={!host.nestedInDrawer} open={picker.state.open && host.ready} onOpenChange={(open) => { if (open) { selected.current = false; picker.open(); } else picker.close(); }}>
    <DropdownMenuTrigger asChild>{host.placement === 'sidebar'
      ? <SidebarMenuButton size="lg" aria-label={`切换工作空间：${name}`} tooltip={`切换工作空间：${name}`} className="min-w-0 data-[state=open]:bg-sidebar-accent">{trigger}</SidebarMenuButton>
      : <Button variant="ghost" aria-label={`切换工作空间：${name}`} className="h-12 w-full min-w-0 justify-start gap-2 px-1 sm:px-2">{trigger}</Button>}
    </DropdownMenuTrigger>
    {picker.state.open && host.ready && <PickerContent key={picker.state.filter} path={path} initialFilter={picker.state.filter} host={host} go={(href) => void go(href)} onCloseAutoFocus={(event) => { if (selected.current) event.preventDefault(); }} />}
  </DropdownMenu>;
}

function PickerContent({ path, initialFilter, host, go, onCloseAutoFocus }: { path: string; initialFilter: WorkspaceFilter; host: WorkspaceSwitcherHost; go: (href: string) => void; onCloseAutoFocus: (event: Event) => void }) {
  const { account, client, can, phase } = useIdentity();
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState(initialFilter);
  const [status, setStatus] = useState('all');
  const search = useRef<HTMLInputElement>(null);
  const initialFocus = useRef(false);
  useEffect(() => { const timer = setTimeout(() => setQ(input), 250); return () => clearTimeout(timer); }, [input]);
  const scope = workspaceScope(path);
  const platformEntry = platformNavigation(path, can)[0]?.items[0];
  const readTenants = filter !== 'projects';
  const readProjects = filter !== 'tenants';
  const tenants = useInfiniteQuery({
    queryKey: ['account', account?.id, 'my-tenants', 'workspace-picker', q, filter],
    initialPageParam: '', queryFn: ({ pageParam, signal }) => client.myTenants({ q, cursor: pageParam, limit: 50 }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: phase === 'authenticated' && readTenants, staleTime: 0, gcTime: 0, refetchOnWindowFocus: 'always', refetchInterval: readTenants ? 30_000 : false,
  });
  const projects = useInfiniteQuery({
    queryKey: ['account', account?.id, 'my-projects', 'workspace-picker', q, status, filter],
    initialPageParam: '', queryFn: ({ pageParam, signal }) => client.myProjects({ q, status, cursor: pageParam, limit: 50 }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: phase === 'authenticated' && readProjects, staleTime: 0, gcTime: 0, refetchOnWindowFocus: 'always', refetchInterval: readProjects ? 30_000 : false,
  });
  const tenantItems = !readTenants || tenants.isError ? [] : tenants.data?.pages.flatMap((page) => page.items) ?? [];
  const projectItems = !readProjects || projects.isError ? [] : projects.data?.pages.flatMap((page) => page.items) ?? [];
  // A parent obtained from a project is only a label, never an inferred tenant grant.
  const groups = new Map<string, TenantGroup>(tenantItems.map((item) => [item.id, { id: item.id, name: item.name, tenant: item, projects: [] }]));
  const seenProjects = new Set<string>();
  for (const item of projectItems) {
    if (seenProjects.has(item.project.id)) continue;
    seenProjects.add(item.project.id);
    const group: TenantGroup = groups.get(item.project.tenant_id) ?? { id: item.project.tenant_id, name: item.project.tenant_name, projects: [] };
    group.projects.push(item); groups.set(group.id, group);
  }
  return <DropdownMenuContent aria-labelledby={undefined} aria-label="切换工作空间" align="start" side={host.placement === 'header' || host.nestedInDrawer ? 'bottom' : 'right'} sideOffset={8} collisionPadding={12} className="w-96 max-w-[calc(100vw-1.5rem)] rounded-lg p-1.5" onCloseAutoFocus={onCloseAutoFocus} onFocusCapture={() => { if (!initialFocus.current) { initialFocus.current = true; queueMicrotask(() => search.current?.focus()); } }}>
    <DropdownMenuLabel className="text-xs text-muted-foreground">切换工作空间</DropdownMenuLabel>
    <div className="space-y-3 px-2 pb-3" onKeyDown={(event) => { if (event.key !== 'Escape') event.stopPropagation(); if (event.key === 'ArrowDown' && event.target === search.current) { event.preventDefault(); event.currentTarget.closest('[role=menu]')?.querySelector<HTMLElement>('[role=menuitem]:not([aria-disabled=true])')?.focus(); } }}>
      <div className="space-y-1"><Label htmlFor="workspace-search">搜索名称或编号</Label><div className="flex gap-1"><Input ref={search} id="workspace-search" value={input} onChange={(event) => { setInput(event.target.value); if (!event.target.value) setQ(''); }} />{input && <Button variant="ghost" onClick={() => { setInput(''); setQ(''); search.current?.focus(); }}>清除</Button>}</div></div>
      <div className="flex gap-3"><div className="min-w-0 flex-1 space-y-1"><Label htmlFor="workspace-type">工作空间类型</Label><select id="workspace-type" className="h-11 w-full rounded-md border bg-background px-2 text-sm" value={filter} onChange={(event) => setFilter(event.target.value as WorkspaceFilter)}><option value="all">全部</option><option value="tenants">租户</option><option value="projects">项目</option></select></div>
        {readProjects && <div className="min-w-0 flex-1 space-y-1"><Label htmlFor="workspace-status">项目状态</Label><select id="workspace-status" className="h-11 w-full rounded-md border bg-background px-2 text-sm" value={status} onChange={(event) => setStatus(event.target.value)}><option value="all">全部</option><option value="active">运行中</option><option value="archived">已归档</option></select></div>}
      </div>
    </div>
    {filter === 'all' && platformEntry && <><DropdownMenuGroup aria-label="平台"><DropdownMenuLabel className="text-xs text-muted-foreground">平台</DropdownMenuLabel><WorkspaceItem name="平台工作空间" description="平台管理与运营" icon={Layers3} current={scope.kind === 'platform'} onSelect={() => go(platformEntry.href)} /></DropdownMenuGroup><DropdownMenuSeparator /></>}
    <DropdownMenuGroup aria-label="租户与项目">
      <DropdownMenuLabel className="text-xs text-muted-foreground">租户与项目</DropdownMenuLabel>
      {((readTenants && tenants.isPending) || (readProjects && projects.isPending)) && <p role="status" className="px-2 py-3 text-sm text-muted-foreground">正在加载工作空间…</p>}
      {readTenants && tenants.isError && <DropdownMenuItem className="min-h-11 cursor-pointer" disabled={tenants.isFetching} onSelect={(event) => { event.preventDefault(); void tenants.refetch(); }}>租户加载失败，点击重试</DropdownMenuItem>}
      {readProjects && projects.isError && <DropdownMenuItem className="min-h-11 cursor-pointer" disabled={projects.isFetching} onSelect={(event) => { event.preventDefault(); void projects.refetch(); }}>项目加载失败，点击重试</DropdownMenuItem>}
      {[...groups.values()].map((group) => <DropdownMenuGroup key={group.id} aria-label={group.name} className="border-b border-border py-1.5 last:border-b-0">
        {group.tenant ? <WorkspaceItem name={group.name} description={tenantDescription(group.tenant)} icon={Building2} current={scope.kind === 'tenant' && scope.tenantId === group.id} action={group.tenant.can_enter ? '进入租户' : '查看状态'} onSelect={() => go(`/t/${group.id}/profile`)} /> : <DropdownMenuLabel className="break-words text-sm">{group.name}<span className="ms-2 text-xs font-normal text-muted-foreground">所属租户</span></DropdownMenuLabel>}
        {group.projects.length > 0 && <div className="ms-6 mt-1 space-y-1 border-s ps-2">{group.projects.map((item) => <WorkspaceItem key={item.project.id} name={item.project.name} description={projectDescription(item)} icon={FolderKanban} action={item.can_enter ? '进入项目' : '查看本人状态'} current={scope.kind === 'project' && scope.tenantId === group.id && scope.projectId === item.project.id} onSelect={() => go(`/t/${group.id}/p/${item.project.id}/profile`)} />)}</div>}
      </DropdownMenuGroup>)}
      {(!readTenants || tenants.isSuccess) && (!readProjects || projects.isSuccess) && !groups.size && <p className="px-2 py-4 text-sm text-muted-foreground">{q ? '没有符合筛选条件的工作空间。' : filter === 'tenants' ? '尚未加入租户，请联系平台管理人员' : filter === 'projects' ? '当前没有可显示的项目。' : '尚未加入租户或项目。'}</p>}
      {readTenants && !tenants.isError && tenants.hasNextPage && <DropdownMenuItem disabled={tenants.isFetchingNextPage} className="min-h-11 cursor-pointer" onSelect={(event) => { event.preventDefault(); void tenants.fetchNextPage(); }}>{tenants.isFetchingNextPage ? '正在加载租户…' : '加载更多租户'}</DropdownMenuItem>}
      {readProjects && !projects.isError && projects.hasNextPage && <DropdownMenuItem disabled={projects.isFetchingNextPage} className="min-h-11 cursor-pointer" onSelect={(event) => { event.preventDefault(); void projects.fetchNextPage(); }}>{projects.isFetchingNextPage ? '正在加载项目…' : '加载更多项目'}</DropdownMenuItem>}
    </DropdownMenuGroup>
  </DropdownMenuContent>;
}
