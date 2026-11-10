import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Permissions as P, type Permission, type PermissionCode, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Form } from '@infrapro/ui/components/form';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { z } from '../../config/validation';
import { MutationDialog } from './mutation-dialog';

const detailsSchema = z.object({ name: z.string().trim().min(1, '请填写角色名称。').refine((v) => Array.from(v).length <= 50, '最多 50 字符。'), description: z.string().refine((v) => Array.from(v).length <= 500, '说明最多 500 字符。') });
function RoleDetailsForm({ role }: { role?: Role }) {
  const { client } = useIdentity();
  const form = useForm({ resolver: zodResolver(detailsSchema), defaultValues: { name: role?.name ?? '', description: role?.description ?? '' } });
  const values = useWatch({ control: form.control });
  return <MutationDialog title={role ? '编辑角色资料' : '创建空角色'} description={role ? '名称和说明变更不会改变角色权限。' : '新角色默认启用，创建后单独配置权限。'} trigger={<Button variant={role ? 'outline' : 'default'}>{role ? '编辑名称与说明' : '创建角色'}</Button>} submit={async (reason) => { if (!await form.trigger()) throw new Error('请检查角色名称和说明。'); const input = { ...form.getValues(), reason }; if (role) return client.updateRole(role.id, { ...input, expected_version: role.version }); return client.createRole(input); }} onComplete={() => form.reset(role ? form.getValues() : { name: '', description: '' })}>
    <Form {...form}><div className="space-y-4"><FormInput control={form.control} name="name" label="角色名称" /><FormInput control={form.control} name="description" label="角色说明" />{role && <p className="text-sm text-muted-foreground">名称：{role.name} → {values.name}</p>}</div></Form>
  </MutationDialog>;
}
function PermissionEditor({ role, catalog }: { role: Role; catalog: Permission[] }) {
  const { client, can } = useIdentity();
  const [selected, setSelected] = useState<PermissionCode[]>(role.permission_codes);
  const [search, setSearch] = useState('');
  const visible = catalog.filter((p) => p.scope !== 'tenant' && p.lifecycle === 'active' && `${p.name} ${p.code} ${p.description}`.toLowerCase().includes(search.toLowerCase()));
  const add = selected.filter((p) => !role.permission_codes.includes(p)); const remove = role.permission_codes.filter((p) => !selected.includes(p));
  const names = (codes: PermissionCode[]) => codes.map((c) => catalog.find((p) => p.code === c)?.name ?? c).join('、') || '无';
  const unmatched = [
    { writes: [P.AccountProfileUpdate], read: P.AccountProfileRead },
    { writes: [P.PlatformAccountAssignRoles, P.PlatformAccountDisable, P.PlatformAccountEnable], read: P.PlatformAccountRead },
    { writes: [P.PlatformRoleCreate, P.PlatformRoleUpdate, P.PlatformRoleSetPermissions, P.PlatformRoleSetStatus, P.PlatformRoleDelete], read: P.PlatformRoleRead },
    { writes: [P.PlatformTenantCreate, P.PlatformTenantUpdate, P.PlatformTenantTransferAdmin], read: P.PlatformTenantRead },
    { writes: [P.PlatformInvitationCreate, P.PlatformInvitationRevoke, P.PlatformInvitationRegenerate], read: P.PlatformInvitationRead },
  ].some(({ writes, read }) => writes.some((p) => selected.includes(p)) && !selected.includes(read));
  const outOfScope = role.permission_codes.some((p) => catalog.some((c) => c.code === p && c.lifecycle === 'active') && !can(p));
  const toggle = (codes: PermissionCode[], checked: boolean) => setSelected((old) => checked ? [...new Set([...old, ...codes])] : old.filter((p) => !codes.includes(p)));
  return <section className="space-y-4"><h3 className="font-medium">配置权限</h3><div className="max-w-lg space-y-2"><Label htmlFor="role-permission-search">搜索本角色可配置的权限</Label><Input id="role-permission-search" value={search} onChange={(e) => setSearch(e.target.value)} /></div>
    {outOfScope && <FormNotice>角色包含超出本人范围的权限，当前配置只读。</FormNotice>}
    {[...new Set(visible.map((p) => p.group))].map((group) => { const items = visible.filter((p) => p.group === group); const allowed = items.filter((p) => can(p.code)).map((p) => p.code); const count = allowed.filter((p) => selected.includes(p)).length; return <fieldset key={group} className="space-y-3 rounded-lg border p-4"><legend className="px-1 font-medium">{group}</legend><label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><Checkbox disabled={outOfScope || !allowed.length} checked={count === allowed.length && count > 0 ? true : count > 0 ? 'indeterminate' : false} onCheckedChange={(v) => toggle(allowed, v === true)} />全选此分组当前筛选结果（{allowed.length} 项可授权）</label>{items.map((p) => <label key={p.code} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md p-2 hover:bg-muted/40"><Checkbox className="mt-1" disabled={outOfScope || !can(p.code)} checked={selected.includes(p.code)} onCheckedChange={(v) => toggle([p.code], v === true)} /><span className="min-w-0 space-y-1"><span className="flex flex-wrap items-center gap-2 text-sm font-medium">{p.name}<Badge variant="outline">{p.scope === 'self' ? '本人' : '平台'}</Badge>{p.risk === 'high' && <Badge variant="secondary">敏感操作</Badge>}{!can(p.code) && <span>超出本人权限</span>}</span><span className="block text-sm text-muted-foreground">{p.description}</span><code className="block break-all text-xs text-muted-foreground">{p.code}</code></span></label>)}</fieldset>; })}
    {role.permission_codes.filter((p) => !catalog.some((c) => c.code === p && c.lifecycle === 'active')).map((p) => <label key={p} className="flex items-center gap-3 text-sm"><Checkbox checked={selected.includes(p)} onCheckedChange={(v) => toggle([p], v === true)} />{p}（已退役，请移除后保存）</label>)}
    <div className="rounded-lg bg-muted/40 p-4 text-sm"><p>新增：{names(add)}</p><p>移除：{names(remove)}</p><p>影响 {role.account_count} 个绑定账号；{role.status === 'disabled' ? '角色当前停用，启用后生效。' : '保存后即时生效。'}</p>{unmatched && <p className="mt-2 font-medium">缺少配套查看权限，使用者可能无法进入相应页面。系统不会自动补充权限。</p>}</div>
    <MutationDialog title="保存角色权限" description={`影响 ${role.account_count} 个绑定账号，请确认增加、移除的权限。`} trigger={<Button disabled={outOfScope || (!add.length && !remove.length)}>保存权限配置</Button>} submit={(reason) => client.setRolePermissions(role.id, { expected_version: role.version, permission_codes: selected, reason })}><p className="text-sm">新增：{names(add)}</p><p className="text-sm">移除：{names(remove)}</p>{unmatched && <FormNotice>你选择保留缺少配套查看权限的配置，请确认此安排符合用途。</FormNotice>}</MutationDialog>
  </section>;
}
function RoleDetail({ id, close }: { id: string; close: () => void }) {
  const { client, can, canAll } = useIdentity();
  const detail = useQuery({ queryKey: ['access', 'role', id], queryFn: ({ signal }) => client.role(id, signal) });
  const catalog = useQuery({ queryKey: ['access', 'permissions'], queryFn: ({ signal }) => client.permissions(signal), enabled: can(P.PlatformPermissionRead) });
  const role = detail.data;
  if (detail.error) return <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />;
  if (!role) return <p>正在读取角色…</p>;
  return <section className="space-y-6 rounded-lg border p-4 sm:p-6" aria-label="角色详情"><div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-xl font-semibold">{role.name}</h2><p className="mt-1 text-sm text-muted-foreground">{role.description || '暂无说明'} · {role.account_count} 个绑定账号</p></div><Button variant="ghost" onClick={close}>关闭详情</Button></div>
    {role.builtin_key ? <FormNotice>内置角色由系统维护，不可修改名称、权限、状态或删除。</FormNotice> : <div className="flex flex-wrap gap-3">{can(P.PlatformRoleUpdate) && <RoleDetailsForm role={role} />}{can(P.PlatformRoleSetStatus) && <MutationDialog title={role.status === 'active' ? '停用角色' : '启用角色'} description={`影响 ${role.account_count} 个账号：${role.status === 'active' ? '立即移除此角色贡献的权限，保留绑定；失去邀请资格的待用链接永久失效。' : '恢复原绑定对有效权限的贡献，不恢复历史邀请。'}`} trigger={<Button variant="outline">{role.status === 'active' ? '停用角色' : '启用角色'}</Button>} submit={(reason) => client.setRoleStatus(role.id, { expected_version: role.version, status: role.status === 'active' ? 'disabled' : 'active', reason })} />}{can(P.PlatformRoleDelete) && <div><MutationDialog title="删除角色" description="角色将被归档，历史审计记录保留。" trigger={<Button variant="outline" disabled={role.account_count > 0}>删除角色</Button>} submit={(reason) => client.archiveRole(role.id, { expected_version: role.version, reason })} onComplete={close} />{role.account_count > 0 && <p className="mt-1 text-xs text-muted-foreground">需先解除全部账号绑定（含停用账号）。</p>}</div>}</div>}
    {!role.builtin_key && canAll(P.PlatformRoleRead, P.PlatformPermissionRead, P.PlatformRoleSetPermissions) && catalog.data ? <PermissionEditor key={role.id} role={role} catalog={catalog.data} /> : <div><h3 className="mb-3 font-medium">角色权限（{role.permission_codes.length}）</h3><ul className="space-y-2 text-sm">{role.permission_codes.map((code) => <li key={code} className="break-all">{catalog.data?.find((p) => p.code === code)?.name ?? code} <code className="text-xs text-muted-foreground">{code}</code></li>)}</ul>{!role.builtin_key && can(P.PlatformRoleSetPermissions) && !can(P.PlatformPermissionRead) && <FormNotice>编辑表单还需要权限目录查看权限。</FormNotice>}</div>}
    {catalog.error && <FormNotice error>{catalog.error.message}</FormNotice>}
  </section>;
}
export function RolesPage() {
  const { client, can } = useIdentity(); const [search, setSearch] = useState(''); const [status, setStatus] = useState(''); const [selected, setSelected] = useState<string>();
  const list = useInfiniteQuery({ queryKey: ['access', 'roles', search, status], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.roles({ q: search, status, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined });
  const columns: ColumnDef<Role>[] = [{ accessorKey: 'name', header: '角色', cell: ({ row }) => <Button variant="link" className="h-auto whitespace-normal text-left" onClick={() => setSelected(row.original.id)}>{row.original.name}</Button> }, { id: 'kind', header: '类型', cell: ({ row }) => row.original.builtin_key ? '内置' : '自定义' }, { accessorKey: 'status', header: '状态', cell: ({ row }) => <Badge variant="outline">{row.original.status === 'active' ? '启用' : '停用'}</Badge> }, { id: 'permissions', header: '权限数', cell: ({ row }) => row.original.permission_codes.length }, { accessorKey: 'account_count', header: '绑定人数' }];
  return <><PageHeader title="角色管理" description="角色是权限的集合，账号可绑定多个角色。" actions={can(P.PlatformRoleCreate) ? <RoleDetailsForm /> : undefined} /><div className="mb-4 flex flex-wrap gap-4"><div className="space-y-2"><Label htmlFor="role-search">搜索角色名称</Label><Input id="role-search" value={search} onChange={(e) => setSearch(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="role-status">角色状态</Label><select id="role-status" value={status} onChange={(e) => setStatus(e.target.value)} className="h-10 rounded-md border bg-background px-3"><option value="">全部</option><option value="active">启用</option><option value="disabled">停用</option></select></div></div><DataTable label="角色列表" data={list.data?.pages.flatMap((p) => p.items) ?? []} columns={columns} getRowId={(r) => r.id} loading={list.isPending} error={list.error?.message} onRetry={() => void list.refetch()} hasMore={list.hasNextPage} loadingMore={list.isFetchingNextPage} onLoadMore={() => void list.fetchNextPage()} />{selected && <div className="mt-6"><RoleDetail key={selected} id={selected} close={() => setSelected(undefined)} /></div>}</>;
}
