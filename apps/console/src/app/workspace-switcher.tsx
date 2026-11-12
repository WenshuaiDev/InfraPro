import { useEffect, useRef, useState, type ComponentType, type KeyboardEvent } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { ArrowUpRight, Building2, Check, ChevronRight, ChevronsUpDown, FolderKanban, Layers3, Search, X } from 'lucide-react';
import type { WorkspaceEntry, ProjectContext, TenantContext } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import type { WorkspaceSwitcherHost } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { SidebarMenuButton } from '@infrapro/ui/components/sidebar';
import { Skeleton } from '@infrapro/ui/components/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/base-select';
import { brand } from '../config/brand';
import { safeReturn, workspaceScope } from '../config/navigation';
import { useWorkspacePicker, type WorkspaceFilter } from './workspace-picker-context';

type WorkspaceItemProps = {
  name: string; description: string; subtitle: string; icon: ComponentType<{ className?: string }>;
  current?: boolean; action?: string; badge?: string; reason?: string; onSelect: () => void;
};

function WorkspaceItem({ name, description, subtitle, icon: Icon, current, action, badge, reason, onSelect }: WorkspaceItemProps) {
  return <button type="button" data-workspace-item aria-label={`${name} ${description}${action ? ` ${action}` : ''}`} aria-current={current ? 'true' : undefined} onClick={onSelect} className={`flex min-h-16 w-full cursor-pointer items-center gap-3 rounded-lg p-2.5 text-start outline-offset-2 hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring ${current ? 'bg-accent' : ''}`}>
    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-muted-foreground"><Icon className="size-4" aria-hidden="true" /></span>
    <span className="grid min-w-0 flex-1 gap-1"><span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1"><span className="min-w-0 truncate text-sm font-medium" title={name}>{name}</span>{badge && <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">{badge}</span>}</span><span className="text-xs text-muted-foreground break-words">{subtitle}</span>{reason && <span className="text-xs text-muted-foreground break-words">{reason}</span>}</span>
    {current ? <><Check className="size-4 shrink-0" aria-hidden="true" /><span className="sr-only">当前工作空间</span></> : reason ? <span className="shrink-0 text-xs text-muted-foreground">{action}</span> : null}
  </button>;
}

function entryDescription(item: WorkspaceEntry) {
  return `${item.kind === 'tenant' ? '租户' : item.status === 'archived' ? '已归档' : '项目'} · ${item.code}${item.kind === 'project' ? ` · ${item.tenant_name}` : ''} · 本人角色：${item.roles.join('、')}`;
}

function WorkspaceLoading({ label }: { label: string }) {
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setVisible(true), 200);
    return () => clearTimeout(timer);
  }, []);
  return <div role="status" aria-label={label} className="px-2" aria-live="polite"><span className="sr-only">{label}</span>{visible && <div aria-hidden="true">{[0, 1, 2].map((index) => <div key={index} className="flex min-h-16 items-center gap-3 py-2.5"><Skeleton className="size-8 shrink-0 rounded-lg motion-reduce:animate-none" /><div className="flex-1 space-y-2"><Skeleton className="h-3 w-2/3 motion-reduce:animate-none" /><Skeleton className="h-2.5 w-4/5 motion-reduce:animate-none" /></div></div>)}</div>}</div>;
}

// Arrow navigation complements Dialog's native focus management; text fields keep editing keys.
function navigateItems(event: KeyboardEvent<HTMLDivElement>) {
  if (event.defaultPrevented || event.target instanceof HTMLElement && event.target.closest('[data-slot="select-trigger"], [data-slot="select-content"]')) return;
  if (event.target instanceof HTMLSelectElement || event.target instanceof HTMLTextAreaElement || (event.target instanceof HTMLElement && event.target.isContentEditable)) return;
  const input = event.target instanceof HTMLInputElement;
  if (input && event.key !== 'ArrowDown') return;
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
  const items = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[data-workspace-item]:not(:disabled)')].filter((item) => item.getClientRects().length);
  if (!items.length) return;
  const current = items.indexOf(document.activeElement as HTMLButtonElement);
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : current < 0 ? (event.key === 'ArrowUp' ? items.length - 1 : 0) : (current + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length;
  event.preventDefault();
  items[next]?.focus();
}

type Props = { path: string; tenant?: TenantContext; project?: ProjectContext; host: WorkspaceSwitcherHost };
export function WorkspaceSwitcher({ path, tenant, project, host }: Props) {
  const picker = useWorkspacePicker();
  const navigate = useNavigate();
  const selected = useRef(false);
  useEffect(() => { if (picker.state.open) selected.current = false; }, [picker.state.open]);
  const scope = workspaceScope(path);
  const single = picker.summary?.single;
  const current = single && single.kind === scope.kind && (single.kind === 'platform' || single.id === (single.kind === 'project' ? scope.projectId : scope.tenantId));
  const target = single && !current ? single : undefined;
  const name = target?.name ?? (scope.kind === 'project' ? project?.project.name ?? '项目工作空间' : scope.kind === 'tenant' ? tenant?.tenant.name ?? '租户工作台' : scope.kind === 'platform' ? '平台工作空间' : brand.name);
  const description = target ? (target.kind === 'project' ? `进入项目 · ${target.tenant_name}` : target.kind === 'tenant' ? '进入租户工作台' : '进入平台工作空间') : scope.kind === 'project' ? `项目 · ${project?.project.tenant_name ?? '所属租户'}` : scope.kind === 'tenant' ? '租户工作台' : scope.kind === 'platform' ? brand.name : picker.pending ? '正在核验工作空间…' : picker.summary?.mode === 'none' ? '暂无工作空间授权' : '选择工作空间';
  const kind = target?.kind ?? scope.kind;
  const Icon = kind === 'project' ? FolderKanban : kind === 'tenant' ? Building2 : kind === 'platform' ? Layers3 : null;
  const trigger = <><span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground [&>svg]:size-5">{Icon ? <Icon aria-hidden="true" /> : brand.logo}</span><span className="grid min-w-0 flex-1 text-start text-sm leading-tight"><span className="truncate font-semibold" title={name}>{name}</span><span className="truncate text-xs text-muted-foreground" title={description}>{description}</span></span>{picker.summary?.mode === 'multiple' && <ChevronsUpDown className="ms-auto size-4 shrink-0" aria-hidden="true" />}</>;
  const go = async (href: string) => {
    selected.current = true;
    picker.close();
    host.onSelected();
    if (path !== href) await navigate({ to: safeReturn(href) });
    requestAnimationFrame(() => document.getElementById('main')?.focus());
  };
  if (!picker.error && picker.summary?.mode !== 'multiple') {
    return single && !current ? <Button variant="ghost" className="h-12 w-full justify-start gap-2 px-2" aria-label={`进入工作空间：${single.name}`} onClick={() => picker.open()}>{trigger}</Button> : <div className="flex min-h-12 w-full min-w-0 items-center gap-2 px-2" aria-label={`当前工作空间：${name}`}>{trigger}</div>;
  }
  return <Dialog open={picker.state.open && host.ready} onOpenChange={(open) => { if (open) { selected.current = false; picker.open(); } else picker.close(); }}>
    <DialogTrigger asChild>{host.placement === 'sidebar'
      ? <SidebarMenuButton size="lg" aria-label={`切换工作空间：${name}`} tooltip={`切换工作空间：${name}`} className="min-w-0 data-[state=open]:bg-sidebar-accent">{trigger}</SidebarMenuButton>
      : <Button variant="ghost" aria-label={`切换工作空间：${name}`} className="h-12 w-full min-w-0 justify-start gap-2 px-1 sm:px-2">{trigger}</Button>}
    </DialogTrigger>
    {picker.state.open && host.ready && <PickerContent key={picker.state.filter} path={path} initialFilter={picker.state.filter} go={(href) => void go(href)} onCloseAutoFocus={(event) => { if (selected.current) event.preventDefault(); }} />}
  </Dialog>;
}

function PickerContent({ path, initialFilter, go, onCloseAutoFocus }: { path: string; initialFilter: WorkspaceFilter; go: (href: string) => void; onCloseAutoFocus: (event: Event) => void }) {
  const { account, client, phase } = useIdentity();
  const picker = useWorkspacePicker();
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState(initialFilter);
  // Radix handles Escape during capture; let the nested Base UI Select dismiss first.
  const [typeOpen, setTypeOpen] = useState(false);
  const [status, setStatus] = useState('all');
  const [selectedTenant, setSelectedTenant] = useState<string>();
  const search = useRef<HTMLInputElement>(null);
  // A null ref can make the nested portal resolve to body before this dialog mounts.
  // Publish the actual container so production effects retry once it is attached.
  const [dialog, setDialog] = useState<HTMLDivElement | null>(null);
  useEffect(() => { const timer = setTimeout(() => setQ(input.trim()), 250); return () => clearTimeout(timer); }, [input]);
  const scope = workspaceScope(path);
  const readTenants = filter !== 'projects';
  const readProjects = filter !== 'tenants';
  const tenants = useInfiniteQuery({
    queryKey: ['account', account?.id, 'workspaces', 'tenants', picker.revision, q], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => client.myWorkspaces({ kind: 'tenant', q, cursor: pageParam, limit: 50 }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: phase === 'authenticated' && readTenants, staleTime: 0, gcTime: 60_000, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 30_000,
  });
  const tenantItems = !readTenants || tenants.isError ? [] : tenants.data?.pages.flatMap((page) => page.items) ?? [];
  const active = selectedTenant === 'projects' ? undefined : tenantItems.find((item) => item.id === (selectedTenant ?? scope.tenantId)) ?? (selectedTenant ? undefined : tenantItems[0]);
  const dualPane = !input.trim() && filter === 'all' && (picker.summary?.tenant_count ?? 0) > 0;
  const tenantId = dualPane ? active?.id : undefined;
  const waitingForTenant = dualPane && selectedTenant !== 'projects' && tenants.isPending;
  const projects = useInfiniteQuery({
    queryKey: ['account', account?.id, 'workspaces', 'projects', picker.revision, q, status, tenantId], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => client.myWorkspaces({ kind: 'project', q, status, tenant_id: tenantId, cursor: pageParam, limit: 50 }, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: phase === 'authenticated' && readProjects && !waitingForTenant, staleTime: 0, gcTime: 60_000, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 30_000,
  });
  const projectItems = !readProjects || projects.isError ? [] : projects.data?.pages.flatMap((page) => page.items) ?? [];
  const ready = (!readTenants || tenants.isSuccess) && (!readProjects || projects.isSuccess);
  const row = (item: WorkspaceEntry) => <WorkspaceItem key={item.kind + item.id} name={item.name} description={entryDescription(item)} subtitle={`${item.code}${item.kind === 'project' ? ` · ${item.tenant_name}` : ''} · ${item.roles.join('、')}`} badge={item.status === 'archived' ? '已归档' : undefined} icon={item.kind === 'tenant' ? Building2 : FolderKanban} action={item.kind === 'tenant' ? '进入租户' : '进入项目'} current={item.kind === scope.kind && (item.kind === 'tenant' ? scope.tenantId : scope.projectId) === item.id} onSelect={() => go(item.default_path)} />;
  const projectStatus = readProjects && <><Label htmlFor="workspace-status" className="sr-only">项目状态</Label><select id="workspace-status" className="min-h-9 max-w-32 rounded-md bg-background px-2 text-xs" value={status} onChange={(event) => setStatus(event.target.value)}><option value="all">全部状态</option><option value="active">运行中</option><option value="archived">已归档</option></select></>;
  const moreTenants = readTenants && !tenants.isError && tenants.hasNextPage && <Button variant="ghost" className="w-full shrink-0" disabled={tenants.isFetchingNextPage} onClick={() => void tenants.fetchNextPage()}>{tenants.isFetchingNextPage ? '正在加载租户…' : '加载更多租户'}</Button>;
  const moreProjects = readProjects && !projects.isError && projects.hasNextPage && <Button variant="ghost" className="w-full shrink-0" disabled={projects.isFetchingNextPage} onClick={() => void projects.fetchNextPage()}>{projects.isFetchingNextPage ? '正在加载项目…' : '加载更多项目'}</Button>;
  const platform = picker.summary?.platform;
  const browse = (id: string, name: string, detail: string, Icon: typeof Building2) => <button key={id} type="button" data-workspace-item aria-label={id === 'projects' ? '浏览我的项目' : `浏览租户：${name}`} aria-pressed={(active?.id ?? 'projects') === id} onClick={() => setSelectedTenant(id)} className={`mb-1 flex min-h-16 w-full cursor-pointer items-center gap-2.5 rounded-lg border p-2.5 text-start outline-offset-2 hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring ${(active?.id ?? 'projects') === id ? 'border-border bg-background shadow-xs' : 'border-transparent'}`}><span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-muted-foreground"><Icon className="size-4" aria-hidden="true" /></span><span className="grid min-w-0 flex-1 gap-1"><span className="truncate text-xs font-medium" title={name}>{name}</span><span className="text-xs text-muted-foreground break-words">{detail}</span></span><ChevronRight className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" /></button>;
  const tenantLoading = readTenants && tenants.isPending && <WorkspaceLoading label="正在加载租户…" />;
  const projectList = <div aria-busy={readProjects && projects.isPending}>{readProjects && projects.isPending && <WorkspaceLoading label="正在加载项目…" />}{projectItems.map(row)}{readProjects && projects.isSuccess && !projectItems.length && <p className="px-2 py-8 text-center text-sm text-muted-foreground">{q || status !== 'all' ? '没有符合筛选条件的项目。' : active ? '当前租户没有获准进入的项目。' : '当前没有可进入的项目。'}</p>}{moreProjects}</div>;
  return <DialogContent ref={setDialog} aria-modal="true" className="flex h-[min(40rem,calc(100dvh-2rem))] max-h-[calc(100dvh-2rem)] max-w-[calc(100%-1.5rem)] flex-col gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-[54rem]" overlayClassName="bg-black/25 backdrop-blur-sm dark:bg-black/40 motion-reduce:animate-none" onOpenAutoFocus={(event) => { event.preventDefault(); search.current?.focus(); }} onCloseAutoFocus={onCloseAutoFocus} onKeyDown={navigateItems} onEscapeKeyDown={(event) => { if (typeOpen) event.preventDefault(); }}>
    <DialogHeader className="shrink-0 px-5 pt-6 pb-4 text-start sm:px-6"><DialogTitle className="text-xl">切换工作空间</DialogTitle><DialogDescription className="text-xs sm:text-sm">选择你已获授权的工作空间。</DialogDescription></DialogHeader>
    <div className="flex shrink-0 items-center gap-2 px-5 pb-4 sm:px-6">
      <div className="relative min-w-0 flex-1"><Label htmlFor="workspace-search" className="sr-only">搜索名称或编号</Label><Search className="pointer-events-none absolute top-3.5 start-3 size-4 text-muted-foreground" aria-hidden="true" /><Input ref={search} id="workspace-search" className="h-11 ps-9 pe-10" placeholder="搜索名称或编号" value={input} onChange={(event) => { setInput(event.target.value); if (!event.target.value.trim()) setQ(''); }} />{input && <Button size="icon" variant="ghost" className="absolute top-1 end-1 size-9" aria-label="清除" onClick={() => { setInput(''); setQ(''); search.current?.focus(); }}><X className="size-4" /></Button>}</div>
      <Label htmlFor="workspace-type" className="sr-only">工作空间类型</Label>
      <Select<WorkspaceFilter> value={filter} onValueChange={(value) => { if (value) setFilter(value); }} open={typeOpen} onOpenChange={setTypeOpen} modal={false} items={{ all: '全部类型', tenants: '租户', projects: '项目' }}>
        <SelectTrigger id="workspace-type" className="h-11 w-28 text-xs"><SelectValue /></SelectTrigger>
        <SelectContent container={dialog} align="end" alignItemWithTrigger={false}><SelectItem value="all">全部类型</SelectItem><SelectItem value="tenants">租户</SelectItem><SelectItem value="projects">项目</SelectItem></SelectContent>
      </Select>
    </div>
    <div aria-live="polite" className="shrink-0 px-5 empty:hidden sm:px-6">
      {picker.error && <Button variant="ghost" onClick={picker.refresh}>工作空间核验失败，点击重试</Button>}
      {readTenants && tenants.isError && <Button variant="ghost" disabled={tenants.isFetching} onClick={() => void tenants.refetch()}>租户加载失败，点击重试</Button>}
      {readProjects && projects.isError && <Button variant="ghost" disabled={projects.isFetching} onClick={() => void projects.refetch()}>项目加载失败，点击重试</Button>}
    </div>
    {dualPane ? <div className="flex min-h-0 flex-1 flex-col border-t md:flex-row">
      <div className="shrink-0 border-b bg-muted/30 px-5 py-3 md:hidden"><Label htmlFor="workspace-tenant" className="mb-1.5 block text-xs text-muted-foreground">选择租户</Label><select id="workspace-tenant" className="h-10 w-full min-w-0 rounded-md border bg-background px-2 text-sm" value={active?.id ?? 'projects'} onChange={(event) => setSelectedTenant(event.target.value)}><option value="projects">我的项目</option>{tenantItems.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>{moreTenants}</div>
      <div className="hidden min-h-0 w-72 shrink-0 flex-col overflow-y-auto overscroll-contain border-e bg-muted/30 p-2.5 md:flex" role="group" aria-label="租户列表" aria-busy={tenants.isPending}><h3 className="px-3 pt-1 pb-3 text-xs text-muted-foreground">我的租户</h3>{tenantLoading}{tenantItems.map((item) => browse(item.id, item.name, item.roles.join('、'), Building2))}{moreTenants}{(picker.summary?.project_count ?? 0) > 0 && <div className="mt-2 border-t pt-2">{browse('projects', '我的项目', '全部获准项目', FolderKanban)}</div>}</div>
      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-4 sm:px-5" role="group" aria-label={active?.name ?? '我的项目'}>
        {active && <div className="flex items-center justify-between gap-3 border-b px-1 py-4"><div className="min-w-0"><h3 className="text-sm font-semibold break-words">{active.name}</h3><p className="mt-1 text-xs text-muted-foreground break-words">{active.code} · {active.roles.join('、')}</p></div><Button data-workspace-item variant="outline" size="sm" className="shrink-0" aria-label={`${active.name} ${entryDescription(active)} 进入租户`} aria-current={scope.kind === 'tenant' && scope.tenantId === active.id ? 'true' : undefined} onClick={() => go(active.default_path)}>进入租户<ArrowUpRight className="size-3.5" aria-hidden="true" /></Button></div>}
        <div className="flex min-h-12 items-center justify-between gap-2 px-1"><h3 className="text-xs">{active ? '项目' : '我的项目'}<span className="ms-1.5 text-muted-foreground tabular-nums">{projects.isPending ? '—' : projects.hasNextPage ? `已加载 ${projectItems.length}` : projectItems.length}</span></h3>{projectStatus}</div>{projectList}
      </div>
    </div> : <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain border-t px-3 pb-3 sm:px-5" aria-label="工作空间列表">
      {readProjects && <div className="flex min-h-12 items-center justify-between px-2"><span className="text-xs text-muted-foreground">{q ? '搜索结果' : '我的项目'}</span>{projectStatus}</div>}
      {tenantLoading}{tenantItems.map(row)}{moreTenants}{projectList}{ready && filter === 'tenants' && !tenantItems.length && <p className="px-4 py-10 text-center text-sm text-muted-foreground">当前没有符合条件的租户工作空间。</p>}
    </div>}
    <div className="flex shrink-0 items-center justify-between gap-3 border-t px-4 py-2 sm:px-6"><div className="min-w-0 max-w-72 flex-1">{platform && <WorkspaceItem name="平台工作空间" description="平台管理与运营" subtitle="平台管理与运营" icon={Layers3} current={scope.kind === 'platform'} onSelect={() => go(platform.default_path)} />}</div><div className="flex shrink-0 gap-3 text-xs text-muted-foreground"><span className="hidden sm:inline">↑ ↓ 选择 · Enter 进入</span><span>Esc 关闭</span></div></div>
  </DialogContent>;
}
