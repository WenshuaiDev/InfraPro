import { useState, type RefObject } from 'react';
import { Link } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Building2, ShieldCheck } from 'lucide-react';
import { Permissions as P, type ManagedAccount } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { AccountAvatar, AccountStatus } from './account-table';
import { AccountRolesForm } from './account-roles-form';
import { AccountPermissions } from './account-permissions';
import { RoleMutationForm } from './role-mutation-form';

export type AccountMode = 'details' | 'roles' | 'status';

export function AccountPanel({ id, initialMode, close, returnFocus }: { id: string; initialMode: AccountMode; close: () => void; returnFocus: RefObject<HTMLElement | null> }) {
  const { client, account: viewer, can, canAll } = useIdentity();
  const [mode, setMode] = useState(initialMode), [busy, setBusy] = useState(false);
  const detail = useQuery({ queryKey: ['access', viewer?.id, 'account', id], queryFn: ({ signal }) => client.account(id, signal) });
  const account = detail.data;
  const allowed = mode === 'details' || (mode === 'roles' ? canAll(P.PlatformRoleRead, P.PlatformAccountAssignRoles) : account && can(account.status === 'active' ? P.PlatformAccountDisable : P.PlatformAccountEnable));
  const cancel = () => setMode('details');
  const title = mode === 'roles' ? '配置账号角色' : mode === 'status' ? '变更账号状态' : account?.display_name ?? '账号详情';
  return <ResponsivePanel open onOpenChange={(open) => { if (!open && !busy) close(); }} title={title} description={account ? `${account.username} · 平台账号` : '查看账号资料与授权情况。'} returnFocus={returnFocus} fallbackFocusSelector="[data-account-search]">
    {detail.error && <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />}
    {!account ? !detail.error && <LoadingState label="正在读取账号…" /> : <section aria-label="账号授权详情" className="space-y-5">
      {!allowed ? <><FormNotice error>当前操作权限已不可用，请重新核对。</FormNotice><Button variant="outline" onClick={cancel}>返回账号详情</Button></> : mode === 'details' ? <AccountDetails account={account} edit={setMode} /> : mode === 'roles' ? <AccountRolesForm account={account} complete={cancel} cancel={cancel} onPendingChange={setBusy} /> : <AccountStatusForm account={account} complete={cancel} cancel={cancel} onPendingChange={setBusy} />}
    </section>}
  </ResponsivePanel>;
}

function AccountDetails({ account, edit }: { account: ManagedAccount; edit: (mode: AccountMode) => void }) {
  const { can, canAll } = useIdentity();
  const canAssign = canAll(P.PlatformRoleRead, P.PlatformAccountAssignRoles);
  const canTenants = can(P.PlatformTenantRead);
  const [tab, setTab] = useState('roles');
  const enabled = account.status === 'active';
  return <>
    <div className="flex items-start gap-3"><AccountAvatar name={account.display_name} /><div className="min-w-0 flex-1 space-y-1"><p className="break-words font-medium">{account.display_name}</p><AccountStatus status={account.status} /></div></div>
    <dl className="grid gap-3 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2"><div><dt className="text-muted-foreground">用户名</dt><dd className="mt-1 break-all">{account.username}</dd></div><div><dt className="text-muted-foreground">创建时间</dt><dd className="mt-1"><time dateTime={account.created_at}>{new Date(account.created_at).toLocaleString('zh-CN')}</time></dd></div></dl>
    {!enabled && <FormNotice>账号已停用，不能登录或执行操作。恢复账号不会恢复历史会话和邀请链接。</FormNotice>}
    <div className="flex flex-wrap gap-2">{canAssign && <Button variant="outline" onClick={() => edit('roles')}><ShieldCheck aria-hidden="true" />配置角色</Button>}{can(enabled ? P.PlatformAccountDisable : P.PlatformAccountEnable) && <Button variant="outline" className={enabled ? 'text-destructive hover:text-destructive' : undefined} onClick={() => edit('status')}>{enabled ? '停用账号' : '恢复账号'}</Button>}</div>
    <Tabs value={tab === 'tenants' && !canTenants ? 'roles' : tab} onValueChange={setTab} className="gap-4">
      <TabsList className="w-full"><TabsTrigger className="flex-1 max-sm:text-xs" value="roles">绑定角色</TabsTrigger><TabsTrigger className="flex-1 max-sm:text-xs" value="permissions">有效权限</TabsTrigger>{canTenants && <TabsTrigger className="flex-1 max-sm:text-xs" value="tenants">租户任职</TabsTrigger>}</TabsList>
      <TabsContent value="roles" className="space-y-4">
        <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">绑定的平台角色</h3><Badge variant="secondary">{account.roles.length} 个</Badge></div>
        <p className="text-sm leading-6 text-muted-foreground">本人及平台权限由启用角色共同授予。租户和项目角色在对应工作空间管理。</p>
        <ul className="space-y-3">{account.roles.map((role) => <li key={role.id} className="space-y-2 rounded-lg border p-4"><div className="flex flex-wrap items-start justify-between gap-2"><h4 className="min-w-0 break-words text-sm font-medium">{role.name}</h4><Badge variant="outline">{role.status === 'active' ? '启用' : '停用'}</Badge></div><p className="break-words text-sm text-muted-foreground">{role.description || '暂无角色说明。'}</p><p className="text-xs text-muted-foreground">{role.status === 'disabled' ? '已停用，不贡献权限；原绑定仍保留。' : `${role.permission_codes.length} 项配置权限`}</p></li>)}</ul>
        {!account.roles.length && <EmptyState title="尚未绑定平台角色" description="可通过配置角色授予本人账号和平台范围的操作权限。" />}
        {can(P.PlatformAccountAssignRoles) && !can(P.PlatformRoleRead) && <FormNotice>配置角色还需要角色查看权限。</FormNotice>}
      </TabsContent>
      <TabsContent value="permissions"><AccountPermissions id={account.id} disabled={!enabled} /></TabsContent>
      {canTenants && <TabsContent value="tenants"><AccountTenants id={account.id} /></TabsContent>}
    </Tabs>
  </>;
}

function AccountTenants({ id }: { id: string }) {
  const { client, account } = useIdentity();
  const query = useInfiniteQuery({ queryKey: ['access', account?.id, 'tenant-memberships', id], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.accountTenants(id, { cursor: pageParam }, signal), getNextPageParam: (page) => page.next_cursor || undefined });
  const memberships = query.data?.pages.flatMap((page) => page.items) ?? [];
  return <section aria-label="当前租户任职" className="space-y-4"><h3 className="text-sm font-semibold">当前租户任职</h3><p className="text-sm text-muted-foreground">查看当前任职和租户状态；成员授权在租户工作空间管理。</p>
    {query.isPending && <LoadingState label="正在读取任职…" />}{query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}
    <ul className="space-y-3">{memberships.map((tenant) => <li key={tenant.id} className="space-y-2 rounded-lg border p-4"><div className="flex items-start gap-2"><Building2 aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><Link to="/platform/tenants/$tenantId" params={{ tenantId: tenant.id }} className="min-w-0 break-words text-sm font-medium underline underline-offset-4">{tenant.name}</Link></div><p className="break-all text-xs text-muted-foreground">{tenant.code} · 租户{tenant.status === 'active' ? '已启用' : '已停用'}</p><div className="flex flex-wrap gap-1.5">{tenant.roles.map((role) => <Badge key={role.id} variant="secondary" className="max-w-full whitespace-normal break-words">{role.name}</Badge>)}{!tenant.roles.length && <span className="text-sm text-muted-foreground">未配置租户角色</span>}</div></li>)}</ul>
    {!query.isPending && !query.error && !memberships.length && <EmptyState title="没有当前租户任职" />}
    {query.hasNextPage && <Button variant="outline" disabled={query.isFetching} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? '正在加载…' : '加载更多任职'}</Button>}
  </section>;
}

function AccountStatusForm({ account, complete, cancel, onPendingChange }: { account: ManagedAccount; complete: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const { client } = useIdentity();
  // Keep the version of the state the operator reviewed, even if a query refreshes.
  const [target] = useState(account);
  const enabled = target.status === 'active';
  return <RoleMutationForm label={enabled ? '确认停用账号' : '确认恢复账号'} complete={complete} cancel={cancel} onPendingChange={onPendingChange} submit={(reason) => client.setAccountStatus(target.id, !enabled, { expected_version: target.management_version, reason })}>
    <div className="space-y-3 rounded-lg border p-4"><h3 className="break-words font-medium">{target.display_name}</h3><p className="text-sm">账号状态：{enabled ? '正常 → 停用' : '停用 → 正常'}</p><p className="text-sm leading-6 text-muted-foreground">{enabled ? '停用后将立即撤销该账号的会话和未使用链接，并禁止登录。最后完整管理账号受保护；担任租户管理员时需先完成交接。' : '恢复后可重新登录，原有角色绑定继续保留。历史会话和已失效链接不会恢复。'}</p></div>
  </RoleMutationForm>;
}
