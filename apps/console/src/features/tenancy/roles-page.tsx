import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { ApiError, Permissions as P, type TenantRoleDefinition } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { RoleTable, RoleStatus, RoleActions, type RoleAction } from '../access/role-table';
import { useRoleList } from '../access/use-role-list';
import { PermissionCodeToggle, RolePermissionGroups, RolePermissionHelp } from '../access/permission-help';
import { useTenant } from './context';
import { TenantRoleForm, type TenantRoleMode } from './role-editor';
import { TenantAudit } from './pages';

const titles = { create: '创建租户角色', copy: '复制为本租户角色', update: '编辑名称与说明', permissions: '配置角色权限', status: '变更角色状态', archive: '删除租户角色' };

export function TenantRolesPage() {
  const ctx = useTenant(); const { client, account } = useIdentity(); const navigate = useNavigate();
  const [q, setQ] = useState(''); const [source, setSource] = useState(''); const [status, setStatus] = useState(''); const [pageSize, setPageSize] = useState(10);
  const [operation, setOperation] = useState<{ mode: TenantRoleMode; role?: TenantRoleDefinition }>();
  const [busy, setBusy] = useState(false); const returnFocus = useRef<HTMLElement | null>(null);
  const allowed = ctx.permissions.includes(P.TenantRoleRead);
  const list = useRoleList({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'roles', q, source, status, pageSize], load: (cursor, signal) => client.tenantRoles(ctx.tenant.id, { q, source, status, cursor, limit: pageSize }, signal), enabled: allowed && ctx.available });
  const open = (mode: TenantRoleMode, role: TenantRoleDefinition | undefined, trigger: HTMLElement | null) => { returnFocus.current = trigger; setOperation({ mode, role }); };
  const close = () => { if (!busy) setOperation(undefined); };
  if (!allowed) return <FormNotice error>无权查看租户角色目录。本人授权可在租户资料页查看。</FormNotice>;
  if (list.error instanceof ApiError && [403, 404].includes(list.error.status ?? 0)) return <FormNotice error>角色列表当前不可访问，请重新核验租户身份。</FormNotice>;
  const actions = (role: TenantRoleDefinition): RoleAction[] => [
    { label: '查看详情', run: () => void navigate({ to: '/t/$tenantId/roles/$roleId', params: { tenantId: ctx.tenant.id, roleId: role.id } }) },
    ...(role.can_copy ? [{ label: '复制为本租户角色', run: (trigger: HTMLElement | null) => open('copy', role, trigger) }] : []),
    ...(role.can_edit ? [
      { label: '编辑名称与说明', run: (trigger: HTMLElement | null) => open('update', role, trigger) },
      { label: '配置角色权限', run: (trigger: HTMLElement | null) => open('permissions', role, trigger) },
      { label: role.status === 'active' ? '停用角色' : '启用角色', run: (trigger: HTMLElement | null) => open('status', role, trigger) },
      { label: '删除租户角色', run: (trigger: HTMLElement | null) => open('archive', role, trigger), destructive: true },
    ] : []),
  ];
  return <>
    <PageHeader title="租户角色" description={`${ctx.tenant.name} · 为成员安排职责，查看角色权限及授权影响。`} actions={ctx.permissions.includes(P.TenantRoleCreate) ? <Button onClick={(event) => open('create', undefined, event.currentTarget)}><Plus aria-hidden="true" />创建租户角色</Button> : undefined} />
    <RoleTable label="租户角色列表" items={list.items} q={q} setQ={setQ} status={status} setStatus={setStatus} source={source} setSource={setSource} sourceName={(role) => role.owner_tenant_id ? '本租户自定义' : '平台预置'} boundCount={(role) => role.member_count} renderName={(role) => <Link id={`role-${role.id}-name`} className="inline-block max-w-full truncate font-medium text-primary hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-ring" to="/t/$tenantId/roles/$roleId" params={{ tenantId: ctx.tenant.id, roleId: role.id }}>{role.name}</Link>} actions={actions} loading={list.isPending} error={list.error} retry={() => void list.refetch()} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
    {operation && <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title={titles[operation.mode]} description={`${ctx.tenant.name} · ${operation.role?.name ?? '创建后不自动绑定成员'}`} returnFocus={returnFocus}><TenantRoleForm key={`${operation.mode}:${operation.role?.id ?? 'new'}`} mode={operation.mode} initial={operation.role} finish={() => setOperation(undefined)} cancel={close} onPendingChange={setBusy} /></ResponsivePanel>}
  </>;
}

export function TenantRolePage({ id }: { id: string }) {
  const ctx = useTenant(); const { client, account } = useIdentity(); const navigate = useNavigate();
  const [mode, setMode] = useState<TenantRoleMode>(); const [busy, setBusy] = useState(false);
  const detailRef = useRef<HTMLDivElement>(null); const previousMode = useRef(mode);
  useEffect(() => { if (previousMode.current && !mode) detailRef.current?.querySelector<HTMLButtonElement>('[data-role-actions], button')?.focus(); previousMode.current = mode; }, [mode]);
  const allowed = ctx.permissions.includes(P.TenantRoleRead);
  const q = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'role', id], queryFn: ({ signal }) => client.tenantRole(ctx.tenant.id, id, signal), enabled: allowed && ctx.available });
  const close = () => { if (!busy) void navigate({ to: '/t/$tenantId/roles', params: { tenantId: ctx.tenant.id } }); };
  const cancel = () => { if (!busy) setMode(undefined); };
  return <ResponsivePanel open fallbackFocusSelector="[data-role-list-search]" onOpenChange={(open) => { if (!open) close(); }} title={mode ? titles[mode] : q.data?.name ?? '租户角色详情'} description={`${ctx.tenant.name} · ${q.data?.owner_tenant_id ? '本租户自定义' : '平台预置角色'}`}>
    {!allowed ? <FormNotice error>无权查看租户角色。</FormNotice> : q.error ? <ErrorState message={q.error.message} onRetry={() => void q.refetch()} /> : !q.data ? <p role="status">正在读取角色…</p> : mode ? <TenantRoleForm key={`${id}:${mode}`} mode={mode} initial={q.data} finish={() => { if (mode === 'archive') void navigate({ to: '/t/$tenantId/roles', params: { tenantId: ctx.tenant.id } }); else setMode(undefined); }} cancel={cancel} onPendingChange={setBusy} /> : <div ref={detailRef}><TenantRoleDetails role={q.data} edit={setMode} /></div>}
  </ResponsivePanel>;
}

function TenantRoleDetails({ role: r, edit }: { role: TenantRoleDefinition; edit: (mode: TenantRoleMode) => void }) {
  const ctx = useTenant(); const { client, account } = useIdentity(); const [tab, setTab] = useState('permissions'); const [showCodes, setShowCodes] = useState(false);
  const prefix = ['tenant', account?.id, ctx.tenant.id, ctx.membership_id];
  const catalog = useQuery({ queryKey: [...prefix, 'permissions'], queryFn: ({ signal }) => client.tenantPermissions(ctx.tenant.id, signal), enabled: ctx.available && ctx.permissions.includes(P.TenantPermissionRead) });
  const members = useInfiniteQuery({ queryKey: [...prefix, 'role-members', r.id], queryFn: ({ pageParam, signal }) => client.tenantRoleMembers(ctx.tenant.id, r.id, { cursor: pageParam }, signal), initialPageParam: '', getNextPageParam: (page) => page.next_cursor || undefined, enabled: ctx.available && tab === 'members' && ctx.permissions.includes(P.TenantMemberRead) });
  const actions: RoleAction[] = r.can_edit ? [
    { label: '编辑名称与说明', run: () => edit('update') },
    { label: r.status === 'active' ? '停用角色' : '启用角色', run: () => edit('status') },
    { label: '删除租户角色', run: () => edit('archive'), destructive: true },
  ] : [];
  return <section className="space-y-5" aria-label="租户角色详情">
    <div className="flex flex-wrap items-center gap-3"><Badge variant="outline">{r.owner_tenant_id ? '本租户自定义' : '平台预置'}</Badge><RoleStatus status={r.status} /><span className="text-sm text-muted-foreground">版本 {r.version}</span><div className="ml-auto">{actions.length > 0 && <RoleActions name={r.name} actions={actions} />}</div></div>
    <p className="break-words text-sm text-muted-foreground">{r.description || '暂无说明'}</p>
    <div className="grid grid-cols-3 gap-3 rounded-lg border bg-muted/20 p-4">{[['角色权限', r.permission_codes.length], ['绑定成员', r.member_count], ['当前生效', r.effective_member_count]].map(([label, value]) => <div key={label}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-xl font-semibold tabular-nums">{value}</p></div>)}</div>
    {r.can_copy && <Button variant="outline" onClick={() => edit('copy')}>复制为本租户角色</Button>}
    <Tabs value={tab} onValueChange={setTab} className="gap-4"><TabsList className="w-full"><TabsTrigger value="permissions">角色权限</TabsTrigger><TabsTrigger value="profile">基本资料</TabsTrigger>{ctx.permissions.includes(P.TenantMemberRead) && <TabsTrigger value="members">绑定成员</TabsTrigger>}{ctx.permissions.includes(P.TenantAuditRead) && <TabsTrigger value="audit">操作记录</TabsTrigger>}</TabsList>
      <TabsContent value="permissions" className="space-y-4"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-medium">角色权限（{r.permission_codes.length}）</h3>{r.can_edit && <Button variant="outline" onClick={() => edit('permissions')}>配置角色权限</Button>}</div><RolePermissionHelp scope="tenant" /><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} />{catalog.error && <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} />}{!r.permission_codes.length && <FormNotice>该角色暂不授予操作能力。</FormNotice>}<RolePermissionGroups codes={r.permission_codes} catalog={catalog.data} scope="tenant" showCodes={showCodes} /></TabsContent>
      <TabsContent value="profile" className="space-y-4"><dl className="space-y-4 text-sm"><div><dt className="text-muted-foreground">角色名称</dt><dd className="mt-1 break-words font-medium">{r.name}</dd></div><div><dt className="text-muted-foreground">角色说明</dt><dd className="mt-1 break-words">{r.description || '暂无说明'}</dd></div></dl>{!r.owner_tenant_id && <FormNotice>{r.can_copy ? '预置角色由平台维护，可直接分配，也可复制为独立角色。' : '管理员角色受保护，仅可通过创建租户或管理员交接流程任命。'}</FormNotice>}{r.source_role_id && <p className="text-sm text-muted-foreground">复制自来源角色的版本 {r.source_role_version}，副本独立维护。</p>}{r.can_edit && <Button variant="outline" onClick={() => edit('update')}>编辑名称与说明</Button>}</TabsContent>
      {ctx.permissions.includes(P.TenantMemberRead) && <TabsContent value="members" className="space-y-4"><h3 className="font-medium">本租户绑定成员</h3>{members.isPending && <p role="status">正在读取绑定成员…</p>}{members.error && <ErrorState message={members.error.message} onRetry={() => void members.refetch()} />}<ul className="divide-y rounded-lg border">{members.data?.pages.flatMap((page) => page.items).map((m) => <li key={m.membership_id} className="space-y-1 p-4"><Link className="inline-block min-h-11 break-words font-medium text-primary underline" to="/t/$tenantId/members/$membershipId" params={{ tenantId: ctx.tenant.id, membershipId: m.membership_id }}>{m.display_name}（{m.username}）</Link><p className="text-xs text-muted-foreground">{m.membership_status === 'active' ? '有效成员' : '已暂停'} · {m.account_status === 'active' ? '账号可用' : '账号不可用'}</p></li>)}</ul>{!members.isPending && !members.error && !members.data?.pages[0]?.items.length && <p className="text-sm text-muted-foreground">该角色尚未绑定成员。</p>}{members.hasNextPage && <Button variant="outline" disabled={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>加载更多绑定成员</Button>}</TabsContent>}
      {ctx.permissions.includes(P.TenantAuditRead) && <TabsContent value="audit" className="space-y-4"><h3 className="font-medium">角色变更记录</h3><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} objectType="tenant_role" objectId={r.id} /></TabsContent>}
    </Tabs>
  </section>;
}
