import { useState, type ComponentType } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Building2, Check, ChevronsUpDown, FolderKanban, Layers3, LayoutGrid } from 'lucide-react';
import type { MyTenant, ProjectContext, TenantContext } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { SidebarMenuButton, useSidebar } from '@infrapro/ui/components/sidebar';
import { brand } from '../config/brand';
import { platformNavigation, safeReturn, workspaceScope } from '../config/navigation';

type WorkspaceItemProps = {
  name: string;
  description: string;
  icon: ComponentType<{ className?: string }>;
  current?: boolean;
  disabled?: boolean;
  onSelect: () => void;
};
type TenantGroup = { id: string; name: string; tenant?: MyTenant; projects: ProjectContext[] };

function WorkspaceItem({ name, description, icon: Icon, current, disabled, onSelect }: WorkspaceItemProps) {
  return <DropdownMenuItem disabled={disabled} onSelect={onSelect} aria-current={current ? 'true' : undefined} className={`min-h-12 cursor-pointer gap-3 rounded-md p-2 ${current ? 'bg-accent' : ''}`}>
    <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background"><Icon className="size-4" aria-hidden="true" /></span>
    <span className="grid min-w-0 flex-1 gap-0.5"><span className="truncate font-medium" title={name}>{name}</span><span className="truncate text-xs text-muted-foreground" title={description}>{description}</span></span>
    {current && <><Check className="size-4 text-foreground" aria-hidden="true" /><span className="sr-only">当前工作空间</span></>}
  </DropdownMenuItem>;
}

export function WorkspaceSwitcher({ path, tenant, project }: { path: string; tenant?: TenantContext; project?: ProjectContext }) {
  const { account, client, can } = useIdentity();
  const { isMobile, setOpenMobile } = useSidebar();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const scope = workspaceScope(path);
  const platformEntry = platformNavigation(path, can)[0]?.items[0];
  const tenants = useInfiniteQuery({
    queryKey: ['account', account?.id, 'my-tenants', 'workspace-switcher'],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => client.myTenants({ cursor: pageParam, limit: 50 }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: open, staleTime: 0, refetchOnWindowFocus: true,
  });
  const projects = useInfiniteQuery({
    queryKey: ['account', account?.id, 'my-projects', 'workspace-switcher'],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => client.myProjects({ cursor: pageParam, limit: 50, status: 'all' }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: open, staleTime: 0, refetchOnWindowFocus: true,
  });
  const tenantItems = tenants.isError ? [] : tenants.data?.pages.flatMap((page) => page.items) ?? [];
  const projectItems = projects.isError ? [] : projects.data?.pages.flatMap((page) => page.items) ?? [];
  // Project membership can exist without tenant business permissions. Its parent
  // remains a grouping label unless the tenant list independently permits entry.
  const groups = new Map<string, TenantGroup>(tenantItems.map((item) => [item.id, { id: item.id, name: item.name, tenant: item, projects: [] }]));
  for (const item of projectItems) {
    const group: TenantGroup = groups.get(item.project.tenant_id) ?? { id: item.project.tenant_id, name: item.project.tenant_name, projects: [] };
    group.projects.push(item);
    groups.set(group.id, group);
  }
  const name = scope.kind === 'project' ? project?.project.name ?? '项目工作空间' : scope.kind === 'tenant' ? tenant?.tenant.name ?? '租户工作台' : scope.kind === 'platform' ? '平台工作空间' : brand.name;
  const description = scope.kind === 'project' ? `项目 · ${project?.project.tenant_name ?? tenant?.tenant.name ?? '所属租户'}` : scope.kind === 'tenant' ? '租户工作台' : scope.kind === 'platform' ? brand.name : '选择工作空间';
  const Icon = scope.kind === 'project' ? FolderKanban : scope.kind === 'tenant' ? Building2 : scope.kind === 'platform' ? Layers3 : null;
  const go = (href: string) => {
    setOpen(false);
    setOpenMobile(false);
    void navigate({ to: safeReturn(href) });
  };
  // The mobile sidebar is already a modal Sheet. A second modal would retain
  // its pointer lock when selecting a workspace closes both layers together.
  return <DropdownMenu modal={!isMobile} open={open} onOpenChange={setOpen}>
    <DropdownMenuTrigger asChild>
      <SidebarMenuButton size="lg" aria-label={`切换工作空间：${name}`} tooltip={`切换工作空间：${name}`} className="min-w-0 data-[state=open]:bg-sidebar-accent">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground [&>svg]:size-5">{Icon ? <Icon aria-hidden="true" /> : brand.logo}</span>
        <span className="grid min-w-0 flex-1 text-start text-sm leading-tight"><span className="truncate font-semibold" title={name}>{name}</span><span className="truncate text-xs text-muted-foreground" title={description}>{description}</span></span>
        <ChevronsUpDown className="ms-auto size-4 shrink-0" aria-hidden="true" />
      </SidebarMenuButton>
    </DropdownMenuTrigger>
    <DropdownMenuContent aria-label="切换工作空间" align="start" side={isMobile ? 'bottom' : 'right'} sideOffset={8} collisionPadding={12} className="w-80 max-w-[calc(100vw-1.5rem)] rounded-lg p-1.5">
      <DropdownMenuLabel className="text-xs text-muted-foreground">切换工作空间</DropdownMenuLabel>
      {platformEntry && <><DropdownMenuGroup aria-label="平台"><DropdownMenuLabel className="text-xs text-muted-foreground">平台</DropdownMenuLabel><WorkspaceItem name="平台工作空间" description="平台管理与运营" icon={Layers3} current={scope.kind === 'platform'} onSelect={() => go(platformEntry.href)} /></DropdownMenuGroup><DropdownMenuSeparator /></>}
      <DropdownMenuGroup aria-label="租户与项目">
        <DropdownMenuLabel className="text-xs text-muted-foreground">租户与项目</DropdownMenuLabel>
        {(tenants.isPending || projects.isPending) && <p role="status" className="px-2 py-3 text-sm text-muted-foreground">正在加载工作空间…</p>}
        {tenants.isError && <DropdownMenuItem className="min-h-11 cursor-pointer" disabled={tenants.isFetching} onSelect={(event) => { event.preventDefault(); void tenants.refetch(); }}>租户加载失败，点击重试</DropdownMenuItem>}
        {projects.isError && <DropdownMenuItem className="min-h-11 cursor-pointer" disabled={projects.isFetching} onSelect={(event) => { event.preventDefault(); void projects.refetch(); }}>项目加载失败，点击重试</DropdownMenuItem>}
        {[...groups.values()].map((group) => <DropdownMenuGroup key={group.id} aria-label={group.name} className="border-b border-border py-1.5 last:border-b-0">
          <WorkspaceItem name={group.name} description={group.tenant ? group.tenant.status === 'disabled' ? '租户已停用' : group.tenant.can_enter ? `租户 · ${group.tenant.code}` : '租户暂不可进入' : '所属租户'} icon={Building2} current={scope.kind === 'tenant' && scope.tenantId === group.id} disabled={!group.tenant?.can_enter} onSelect={() => go(`/t/${group.id}/profile`)} />
          {group.projects.length > 0 && <div className="ms-6 mt-1 space-y-1 border-s ps-2">{group.projects.map((item) => <WorkspaceItem key={item.project.id} name={item.project.name} description={!item.can_enter ? '项目暂不可进入' : `${item.project.status === 'archived' ? '已归档 · ' : '项目 · '}${item.project.code}`} icon={FolderKanban} disabled={!item.can_enter} current={scope.kind === 'project' && scope.tenantId === group.id && scope.projectId === item.project.id} onSelect={() => go(`/t/${group.id}/p/${item.project.id}/profile`)} />)}</div>}
        </DropdownMenuGroup>)}
        {tenants.isSuccess && projects.isSuccess && !groups.size && <p className="px-2 py-4 text-sm text-muted-foreground">尚未加入租户或项目。</p>}
        {tenants.hasNextPage && <DropdownMenuItem disabled={tenants.isFetchingNextPage} className="min-h-11 cursor-pointer" onSelect={(event) => { event.preventDefault(); void tenants.fetchNextPage(); }}>{tenants.isFetchingNextPage ? '正在加载租户…' : '加载更多租户'}</DropdownMenuItem>}
        {projects.hasNextPage && <DropdownMenuItem disabled={projects.isFetchingNextPage} className="min-h-11 cursor-pointer" onSelect={(event) => { event.preventDefault(); void projects.fetchNextPage(); }}>{projects.isFetchingNextPage ? '正在加载项目…' : '加载更多项目'}</DropdownMenuItem>}
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuGroup aria-label="查看全部">
        <DropdownMenuItem className="min-h-11 cursor-pointer" onSelect={() => go('/tenants')}><Building2 aria-hidden="true" />我的租户<span className="ms-auto text-xs text-muted-foreground">查看全部与状态</span></DropdownMenuItem>
        <DropdownMenuItem className="min-h-11 cursor-pointer" onSelect={() => go('/projects')}><LayoutGrid aria-hidden="true" />我的项目<span className="ms-auto text-xs text-muted-foreground">查看全部与状态</span></DropdownMenuItem>
      </DropdownMenuGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
