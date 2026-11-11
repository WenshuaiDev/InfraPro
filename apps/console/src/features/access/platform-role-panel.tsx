import { useState, type RefObject } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Permissions as P, type Permission, type PermissionCode, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Textarea } from '@infrapro/ui/components/textarea';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { Toggle } from '@infrapro/ui/components/toggle';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { PermissionCodeToggle, PermissionInfo, RolePermissionHelp, RolePermissionGroups } from './permission-help';
import { groupPermissions } from './permission-groups';
import { RoleMutationForm } from './role-mutation-form';
import { RoleActions, RoleStatus, type RoleAction } from './role-table';

export type PlatformRoleMode = 'details' | 'update' | 'permissions' | 'status' | 'archive';
const titles = { details: '角色详情', update: '编辑名称与说明', permissions: '配置角色权限', status: '变更角色状态', archive: '删除角色' };

export function PlatformRolePanel({ id, initialMode = 'details', close, returnFocus }: { id: string; initialMode?: PlatformRoleMode; close: () => void; returnFocus: RefObject<HTMLElement | null> }) {
  const { client, account, canAll } = useIdentity();
  const [mode, setMode] = useState(initialMode);
  const [busy, setBusy] = useState(false);
  const detail = useQuery({ queryKey: ['access', account?.id, 'role', id], queryFn: ({ signal }) => client.role(id, signal) });
  const role = detail.data;
  const operationAllowed = role && !role.builtin_key && (mode === 'update' ? canAll(P.PlatformRoleUpdate) : mode === 'permissions' ? canAll(P.PlatformPermissionRead, P.PlatformRoleSetPermissions) : mode === 'status' ? canAll(P.PlatformRoleSetStatus) : mode === 'archive' ? canAll(P.PlatformRoleDelete) : true);
  const cancel = () => setMode('details');
  return <ResponsivePanel open onOpenChange={(open) => { if (!open && !busy) close(); }} title={mode === 'details' ? role?.name ?? '角色详情' : titles[mode]} description="平台角色 · 绑定平台账号" returnFocus={returnFocus}>
    {detail.error ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : !role ? <p role="status">正在读取角色…</p> : <section aria-label="角色详情" className="space-y-5">
      {mode === 'details' ? <PlatformRoleDetails role={role} edit={setMode} /> : !operationAllowed ? <><FormNotice error>当前角色或操作权限已不可用，请重新核对。</FormNotice><Button variant="outline" onClick={cancel}>返回角色详情</Button></> : mode === 'permissions' ? <PlatformPermissionForm key={`${role.id}:permissions`} role={role} complete={cancel} cancel={cancel} onPendingChange={setBusy} /> : <PlatformRoleForm key={`${role.id}:${mode}`} role={role} mode={mode} complete={mode === 'archive' ? close : cancel} cancel={cancel} onPendingChange={setBusy} />}
    </section>}
  </ResponsivePanel>;
}

export function CreatePlatformRolePanel({ close, returnFocus }: { close: () => void; returnFocus: RefObject<HTMLElement | null> }) {
  const [busy, setBusy] = useState(false);
  return <ResponsivePanel open onOpenChange={(open) => { if (!open && !busy) close(); }} title="创建角色" description="先填写角色资料，创建后配置权限并分配账号。" returnFocus={returnFocus}><PlatformRoleForm mode="create" complete={close} cancel={close} onPendingChange={setBusy} /></ResponsivePanel>;
}

function PlatformRoleForm({ role, mode, complete, cancel, onPendingChange }: { role?: Role; mode: 'create' | 'update' | 'status' | 'archive'; complete: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const { client } = useIdentity();
  const [name, setName] = useState(role?.name ?? '');
  const [description, setDescription] = useState(role?.description ?? '');
  const fields = mode === 'create' || mode === 'update';
  const label = mode === 'create' ? '创建角色' : mode === 'update' ? '保存角色资料' : mode === 'archive' ? '确认删除角色' : role?.status === 'active' ? '确认停用角色' : '确认启用角色';
  return <RoleMutationForm label={label} cancel={cancel} complete={complete} onPendingChange={onPendingChange} disabled={mode === 'archive' && !!role?.account_count} submit={async (reason) => {
    if (fields && (!name.trim() || Array.from(name.trim()).length > 50 || Array.from(description).length > 500)) throw new Error('角色名称须为 1–50 个字符，说明最多 500 个字符。');
    if (mode === 'create') return client.createRole({ name: name.trim(), description, reason });
    if (!role) return;
    if (mode === 'update') return client.updateRole(role.id, { name: name.trim(), description, reason, expected_version: role.version });
    if (mode === 'status') return client.setRoleStatus(role.id, { expected_version: role.version, status: role.status === 'active' ? 'disabled' : 'active', reason });
    return client.archiveRole(role.id, { expected_version: role.version, reason });
  }}>
    {fields ? <><div className="space-y-2"><Label htmlFor="platform-role-name">角色名称</Label><Input id="platform-role-name" required value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：邀请办理员" /></div><div className="space-y-2"><Label htmlFor="platform-role-description">角色说明</Label><Textarea id="platform-role-description" value={description} onChange={(event) => setDescription(event.target.value)} placeholder="说明这个角色负责的工作" rows={3} /></div>{mode === 'create' && <p className="text-sm text-muted-foreground">新角色默认启用，初始为空权限，不会自动绑定账号。</p>}</> : <div className="space-y-3 rounded-lg border p-4"><p className="font-medium">{role?.name}</p><p className="text-sm">当前绑定 {role?.account_count} 个账号。</p><p className="text-sm text-muted-foreground">{mode === 'archive' ? '仅可删除没有任何账号绑定的角色，包括已停用账号。历史授权和审计记录保留。' : role?.status === 'active' ? '停用后立即撤销此角色贡献的权限，保留绑定；失去邀请资格的未使用邀请永久失效。' : '启用后恢复正常账号的角色权限，已失效的邀请不会恢复。'}</p>{mode === 'archive' && !!role?.account_count && <FormNotice>请先解除全部账号绑定后再删除。</FormNotice>}</div>}
  </RoleMutationForm>;
}

function PlatformRoleDetails({ role, edit }: { role: Role; edit: (mode: PlatformRoleMode) => void }) {
  const { client, can, canAll } = useIdentity();
  const [showCodes, setShowCodes] = useState(false);
  const catalog = useQuery({ queryKey: ['access', 'permissions'], queryFn: ({ signal }) => client.permissions(signal), enabled: can(P.PlatformPermissionRead) });
  const actions: RoleAction[] = !role.builtin_key ? [
    ...(can(P.PlatformRoleUpdate) ? [{ label: '编辑名称与说明', run: () => edit('update') }] : []),
    ...(canAll(P.PlatformRoleRead, P.PlatformPermissionRead, P.PlatformRoleSetPermissions) ? [{ label: '配置角色权限', run: () => edit('permissions') }] : []),
    ...(can(P.PlatformRoleSetStatus) ? [{ label: role.status === 'active' ? '停用角色' : '启用角色', run: () => edit('status') }] : []),
    ...(can(P.PlatformRoleDelete) ? [{ label: '删除角色', run: () => edit('archive'), destructive: true, disabled: role.account_count > 0 }] : []),
  ] : [];
  return <><div className="flex items-center gap-3"><Badge variant="outline">{role.builtin_key ? '内置角色' : '自定义角色'}</Badge><RoleStatus status={role.status} /><div className="ml-auto">{actions.length > 0 && <RoleActions name={role.name} actions={actions} />}</div></div><p className="break-words text-sm text-muted-foreground">{role.description || '暂无说明'}</p>
    <div className="grid grid-cols-2 gap-3 rounded-lg border bg-muted/20 p-4"><div><p className="text-xs text-muted-foreground">已配置权限</p><p className="mt-1 text-xl font-semibold tabular-nums">{role.permission_codes.length}</p></div><div><p className="text-xs text-muted-foreground">绑定账号</p><p className="mt-1 text-xl font-semibold tabular-nums">{role.account_count}</p></div></div>
    <Tabs defaultValue="permissions"><TabsList className="w-full"><TabsTrigger value="permissions">角色权限</TabsTrigger><TabsTrigger value="profile">基本资料</TabsTrigger></TabsList>
      <TabsContent value="permissions" className="space-y-4 pt-4"><div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-medium">角色权限（{role.permission_codes.length}）</h3>{!role.builtin_key && canAll(P.PlatformRoleRead, P.PlatformPermissionRead, P.PlatformRoleSetPermissions) && <Button variant="outline" onClick={() => edit('permissions')}>配置角色权限</Button>}</div><RolePermissionHelp scope="platform" /><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} />{catalog.error && <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} />}{!role.permission_codes.length && <FormNotice>该角色暂不授予操作能力。</FormNotice>}<RolePermissionGroups codes={role.permission_codes} catalog={catalog.data} scope="platform" showCodes={showCodes} /></TabsContent>
      <TabsContent value="profile" className="space-y-4 pt-4"><dl className="space-y-4 text-sm"><div><dt className="text-muted-foreground">角色名称</dt><dd className="mt-1 break-words font-medium">{role.name}</dd></div><div><dt className="text-muted-foreground">角色说明</dt><dd className="mt-1 break-words">{role.description || '暂无说明'}</dd></div><div><dt className="text-muted-foreground">适用范围</dt><dd className="mt-1">平台账号</dd></div></dl>{role.builtin_key ? <FormNotice>内置角色由系统维护，不可修改名称、权限、状态或删除。</FormNotice> : can(P.PlatformRoleUpdate) && <Button variant="outline" onClick={() => edit('update')}>编辑名称与说明</Button>}</TabsContent>
    </Tabs>
  </>;
}

function PlatformPermissionForm({ role, complete, cancel, onPendingChange }: { role: Role; complete: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const { client, can } = useIdentity();
  const catalog = useQuery({ queryKey: ['access', 'permissions'], queryFn: ({ signal }) => client.permissions(signal), enabled: can(P.PlatformPermissionRead) });
  if (catalog.error) return <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} />;
  if (!catalog.data) return <p role="status">正在读取权限说明…</p>;
  return <PermissionEditor role={role} catalog={catalog.data} complete={complete} cancel={cancel} onPendingChange={onPendingChange} />;
}

function PermissionEditor({ role, catalog, complete, cancel, onPendingChange }: { role: Role; catalog: Permission[]; complete: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const { client, can } = useIdentity();
  const [selected, setSelected] = useState<PermissionCode[]>(role.permission_codes);
  const [search, setSearch] = useState('');
  const [showCodes, setShowCodes] = useState(false);
  const [selectedOnly, setSelectedOnly] = useState(false);
  const visible = catalog.filter((p) => (p.scope === 'self' || p.scope === 'platform') && p.lifecycle === 'active' && (!selectedOnly || selected.includes(p.code)) && `${p.name} ${p.code} ${p.description}`.toLowerCase().includes(search.toLowerCase()));
  const add = selected.filter((p) => !role.permission_codes.includes(p)); const remove = role.permission_codes.filter((p) => !selected.includes(p));
  const names = (codes: PermissionCode[]) => codes.map((code) => catalog.find((p) => p.code === code)?.name ?? code).join('、') || '无';
  const unmatched = [
    { writes: [P.AccountProfileUpdate], read: P.AccountProfileRead },
    { writes: [P.PlatformAccountAssignRoles, P.PlatformAccountDisable, P.PlatformAccountEnable], read: P.PlatformAccountRead },
    { writes: [P.PlatformRoleCreate, P.PlatformRoleUpdate, P.PlatformRoleSetPermissions, P.PlatformRoleSetStatus, P.PlatformRoleDelete], read: P.PlatformRoleRead },
    { writes: [P.PlatformTenantCreate, P.PlatformTenantUpdate, P.PlatformTenantTransferAdmin], read: P.PlatformTenantRead },
    { writes: [P.PlatformInvitationCreate, P.PlatformInvitationRevoke, P.PlatformInvitationRegenerate], read: P.PlatformInvitationRead },
  ].some(({ writes, read }) => writes.some((p) => selected.includes(p)) && !selected.includes(read));
  const outOfScope = role.permission_codes.some((code) => catalog.some((p) => p.code === code && p.lifecycle === 'active') && !can(code));
  const toggle = (codes: PermissionCode[], checked: boolean) => setSelected((old) => checked ? [...new Set([...old, ...codes])] : old.filter((code) => !codes.includes(code)));
  return <RoleMutationForm label="保存权限配置" cancel={cancel} complete={complete} onPendingChange={onPendingChange} disabled={outOfScope || (!add.length && !remove.length)} submit={(reason) => client.setRolePermissions(role.id, { expected_version: role.version, permission_codes: selected, reason })}>
    <RolePermissionHelp scope="platform" /><div className="space-y-3"><Label htmlFor="role-permission-search">搜索本角色可配置的权限</Label><Input id="role-permission-search" value={search} placeholder="搜索名称、说明或编码…" onChange={(event) => setSearch(event.target.value)} /><div className="flex flex-wrap items-center justify-between gap-2"><Toggle variant="outline" pressed={selectedOnly} onPressedChange={setSelectedOnly}>仅看已选（{selected.length}）</Toggle><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} /></div></div>
    {outOfScope && <FormNotice>角色包含超出本人范围的权限，当前配置只读。</FormNotice>}
    {groupPermissions(visible).map(({ name: group, items }) => { const allowed = items.filter((p) => can(p.code)).map((p) => p.code); const count = allowed.filter((p) => selected.includes(p)).length; return <fieldset key={group} className="space-y-2 rounded-lg border p-3"><legend className="px-1 text-sm font-medium">{group}</legend><label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><Checkbox disabled={outOfScope || !allowed.length} checked={count === allowed.length && count > 0 ? true : count ? 'indeterminate' : false} onCheckedChange={(value) => toggle(allowed, value === true)} />全选此分组当前筛选结果（{allowed.length} 项可授权）</label>{items.map((p) => <label key={p.code} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md p-2 hover:bg-muted/40"><Checkbox className="mt-1" disabled={outOfScope || !can(p.code)} checked={selected.includes(p.code)} onCheckedChange={(value) => toggle([p.code], value === true)} /><span className="min-w-0 text-sm"><PermissionInfo permission={p} showCode={showCodes} />{!can(p.code) && <span>超出本人权限</span>}</span></label>)}</fieldset>; })}
    {!visible.length && <p className="py-4 text-center text-sm text-muted-foreground">没有符合条件的权限。</p>}
    {role.permission_codes.filter((code) => !catalog.some((p) => p.code === code && p.lifecycle === 'active')).map((code) => <label key={code} className="flex items-center gap-3 break-all text-sm"><Checkbox checked={selected.includes(code)} onCheckedChange={(value) => toggle([code], value === true)} />{code}（已退役，请移除后保存）</label>)}
    <div className="space-y-2 rounded-lg bg-muted/40 p-4 text-sm"><p>新增：{names(add)}</p><p>移除：{names(remove)}</p><p>影响 {role.account_count} 个绑定账号；{role.status === 'disabled' ? '角色当前停用，启用后生效。' : '保存后即时生效。'}</p>{unmatched && <p className="font-medium">缺少配套查看权限，使用者可能无法进入相应页面。请核对后保存。</p>}</div>
  </RoleMutationForm>;
}
