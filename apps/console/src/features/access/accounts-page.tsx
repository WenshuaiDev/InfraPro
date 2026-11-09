import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { Permissions as P, type ManagedAccount, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { MutationDialog } from './mutation-dialog';
import { AuthorizationView } from './authorization-view';
function RolePicker({ account, roles }: { account: ManagedAccount; roles: Role[] }) {
  const { client, can } = useIdentity(); const [ids, setIds] = useState(account.roles.map((r) => r.id));
  const available = [...new Map([...account.roles, ...roles].map((r) => [r.id, r])).values()];
  const before = account.roles.map((r) => r.id); const added = ids.filter((id) => !before.includes(id)); const removed = before.filter((id) => !ids.includes(id));
  const names = (values: string[]) => values.map((id) => available.find((r) => r.id === id)?.name ?? id).join('、') || '无';
  return <section className="space-y-4"><h3 className="font-medium">配置绑定角色</h3><div className="grid gap-2 sm:grid-cols-2">{available.map((r) => { const outside = r.permission_codes.some((p) => !can(p)); const disabled = outside || (r.status !== 'active' && !before.includes(r.id)); return <label key={r.id} className="flex min-h-11 items-start gap-3 rounded-lg border p-3"><Checkbox disabled={disabled} checked={ids.includes(r.id)} onCheckedChange={(v) => setIds((old) => v ? [...old, r.id] : old.filter((id) => id !== r.id))} /><span className="min-w-0 text-sm"><span className="font-medium">{r.name}</span><span className="block text-muted-foreground">{r.status === 'disabled' ? '已停用，不贡献权限；原绑定可保留或解除。' : `${r.permission_codes.length} 项权限`}{outside ? ' · 超出本人授权范围' : ''}</span></span></label>; })}</div><p className="text-sm">新增：{names(added)}；解除：{names(removed)}</p>{!ids.length && <FormNotice>保存后该账号将没有业务操作权限，仍可登录并退出。</FormNotice>}
    <MutationDialog title="保存账号角色" description={`为 ${account.display_name} 更新角色绑定。权限按启用角色的并集计算。`} trigger={<Button disabled={!added.length && !removed.length}>保存角色绑定</Button>} submit={(reason) => client.assignRoles(account.id, { expected_version: account.management_version, role_ids: ids, reason })}><p className="text-sm">新增：{names(added)}</p><p className="text-sm">解除：{names(removed)}</p>{!ids.length && <FormNotice>该账号将失去全部业务操作权限。</FormNotice>}</MutationDialog>
  </section>;
}
function AccountDetail({ id, close }: { id: string; close: () => void }) {
  const { client, can, canAll } = useIdentity();
  const detail = useQuery({ queryKey: ['access', 'account', id], queryFn: ({ signal }) => client.account(id, signal) });
  const auth = useQuery({ queryKey: ['access', 'authorization', id], queryFn: ({ signal }) => client.authorization(id, signal) });
  const roles = useInfiniteQuery({ queryKey: ['access', 'role-options'], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.roles({ cursor: pageParam, limit: 100 }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: can(P.PlatformRoleRead) });
  const account = detail.data;
  if (detail.error) return <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />;
  if (!account) return <p>正在读取账号…</p>;
  const enabled = account.status === 'active';
  return <section className="space-y-6 rounded-lg border p-4 sm:p-6" aria-label="账号授权详情"><div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-xl font-semibold">{account.display_name}</h2><p className="mt-1 text-sm text-muted-foreground">{account.username} · {enabled ? '正常' : '停用'}</p></div><Button variant="ghost" onClick={close}>关闭详情</Button></div>
    {can(enabled ? P.PlatformAccountDisable : P.PlatformAccountEnable) && <MutationDialog title={enabled ? '停用账号' : '恢复账号'} description={enabled ? '将撤销该账号的会话和未使用链接。最后完整管理账号受保护。' : '恢复登录资格，历史会话和链接不会恢复。'} trigger={<Button variant="outline">{enabled ? '停用账号' : '恢复账号'}</Button>} submit={(reason) => client.setAccountStatus(account.id, !enabled, { expected_version: account.management_version, reason })} />}
    {canAll(P.PlatformAccountRead, P.PlatformRoleRead, P.PlatformAccountAssignRoles) && roles.data && <><RolePicker key={account.id} account={account} roles={roles.data.pages.flatMap((p) => p.items)} />{roles.hasNextPage && <Button variant="outline" disabled={roles.isFetchingNextPage} onClick={() => void roles.fetchNextPage()}>加载更多可选角色</Button>}</>}
    {can(P.PlatformAccountAssignRoles) && !can(P.PlatformRoleRead) && <FormNotice>角色分配表单还需要角色查看权限。</FormNotice>}{roles.error && <FormNotice error>{roles.error.message}</FormNotice>}{auth.error && <FormNotice error>{auth.error.message}</FormNotice>}{auth.data && <AuthorizationView value={auth.data} />}
  </section>;
}
export function AccountsPage() {
  const { client, can } = useIdentity(); const [search, setSearch] = useState(''); const [status, setStatus] = useState(''); const [role, setRole] = useState(''); const [selected, setSelected] = useState<string>();
  const roleOptions = useInfiniteQuery({ queryKey: ['access', 'account-role-filter'], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.roles({ limit: 100, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: can(P.PlatformRoleRead) });
  const list = useInfiniteQuery({ queryKey: ['access', 'accounts', search, status, role], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.accounts({ q: search, status, role_id: role, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined });
  const columns: ColumnDef<ManagedAccount>[] = [{ id: 'account', header: '账号', cell: ({ row }) => <div><Button variant="link" className="h-auto whitespace-normal text-left" onClick={() => setSelected(row.original.id)}>{row.original.display_name}</Button><p className="text-xs text-muted-foreground">{row.original.username}</p></div> }, { accessorKey: 'status', header: '状态', cell: ({ row }) => <Badge variant="outline">{row.original.status === 'active' ? '正常' : '停用'}</Badge> }, { id: 'roles', header: '绑定角色', cell: ({ row }) => <div className="flex max-w-lg flex-wrap gap-2">{row.original.roles.map((r) => <Badge key={r.id} variant="secondary">{r.name}{r.status === 'disabled' ? '（停用）' : ''}</Badge>)}{!row.original.roles.length && <span className="text-muted-foreground">无角色</span>}</div> }];
  return <><PageHeader title="账号管理" description="查看账号状态，配置角色及只读有效权限。" /><div className="mb-4 flex flex-wrap gap-4"><div className="space-y-2"><Label htmlFor="account-search">搜索用户名或显示名称</Label><Input id="account-search" value={search} onChange={(e) => setSearch(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="account-status">账号状态</Label><select id="account-status" value={status} onChange={(e) => setStatus(e.target.value)} className="h-10 rounded-md border bg-background px-3"><option value="">全部</option><option value="active">正常</option><option value="disabled">停用</option></select></div>{can(P.PlatformRoleRead) && <div className="space-y-2"><Label htmlFor="account-role">绑定角色</Label><select id="account-role" value={role} onChange={(e) => setRole(e.target.value)} className="h-10 max-w-64 rounded-md border bg-background px-3"><option value="">全部</option>{roleOptions.data?.pages.flatMap((p) => p.items).map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}</select>{roleOptions.hasNextPage && <Button variant="ghost" onClick={() => void roleOptions.fetchNextPage()}>加载更多角色</Button>}</div>}</div><DataTable label="账号列表" data={list.data?.pages.flatMap((p) => p.items) ?? []} columns={columns} getRowId={(r) => r.id} loading={list.isPending} error={list.error?.message} onRetry={() => void list.refetch()} hasMore={list.hasNextPage} loadingMore={list.isFetchingNextPage} onLoadMore={() => void list.fetchNextPage()} />{selected && <div className="mt-6"><AccountDetail key={selected} id={selected} close={() => setSelected(undefined)} /></div>}</>;
}
