import { useState } from 'react';
import { Permissions as P, type PermissionCode, type TenantPermission, type TenantAuthorization, type TenantAuthorizationImpact } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Toggle } from '@infrapro/ui/components/toggle';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { PermissionCodeToggle, PermissionInfo, RolePermissionHelp } from '../access/permission-help';
import { tenantPermissionSections } from '../access/permission-groups';

export function TenantPermissionPicker({ catalog, selected, onChange }: { catalog: TenantPermission[]; selected: PermissionCode[]; onChange: (value: PermissionCode[]) => void }) {
  const [search, setSearch] = useState('');
  const [showCodes, setShowCodes] = useState(false);
  const [selectedOnly, setSelectedOnly] = useState(false);
  const toggle = (codes: PermissionCode[], checked: boolean) => onChange(checked ? [...new Set([...selected, ...codes])] : selected.filter((p) => !codes.includes(p)));
  const pairs = [{ writes: [P.TenantProfileUpdate], read: P.TenantProfileRead }, { writes: [P.TenantProjectCreate], read: P.TenantProjectRead }, { writes: [P.TenantMemberSuspend, P.TenantMemberResume, P.TenantMemberRemove], read: P.TenantMemberRead }, { writes: [P.TenantInvitationCreate, P.TenantInvitationRevoke, P.TenantInvitationRegenerate], read: P.TenantInvitationRead }];
  const missing = pairs.filter(({ writes, read }) => writes.some((p) => selected.includes(p)) && !selected.includes(read));
  return <div className="min-w-0 space-y-4"><RolePermissionHelp scope="tenant" /><h3 className="font-medium">可配置业务权限</h3><div className="flex flex-wrap items-center justify-between gap-2"><Toggle variant="outline" pressed={selectedOnly} onPressedChange={setSelectedOnly}>仅看已选（{selected.length}）</Toggle><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} /></div><Label htmlFor="tenant-permission-search">搜索权限</Label><Input id="tenant-permission-search" value={search} onChange={(e) => setSearch(e.target.value)} /><p className="text-sm">已选择 {selected.length} 项权限。搜索保留已选项；分组选择覆盖该组全部可选权限。</p>
    {tenantPermissionSections.map(({ title, part }) => { const all = catalog.filter((p) => p.tenant_custom_role_assignable && p.code.split('.')[1] === part); const visible = all.filter((p) => (!selectedOnly || selected.includes(p.code)) && `${p.name} ${p.code} ${p.description}`.toLowerCase().includes(search.toLowerCase())); if (!visible.length) return null; const allowed = all.filter((p) => p.selectable).map((p) => p.code); const count = allowed.filter((p) => selected.includes(p)).length; return <fieldset key={part} className="min-w-0 space-y-2 rounded-lg border p-3"><legend className="px-1 font-medium">{title}</legend><label className="flex min-h-11 cursor-pointer items-center gap-3"><Checkbox checked={count === allowed.length && count > 0 ? true : count ? 'indeterminate' : false} disabled={!allowed.length} onCheckedChange={(v) => toggle(allowed, v === true)} />选择本组全部可选项（{allowed.length} 项）</label>{visible.map((p) => <label key={p.code} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md p-2 hover:bg-muted/40"><Checkbox className="mt-1" checked={selected.includes(p.code)} disabled={!p.selectable && !selected.includes(p.code)} onCheckedChange={(v) => toggle([p.code], v === true)} /><span className="min-w-0 text-sm"><PermissionInfo permission={p} showCode={showCodes} />{!p.selectable && <span className="block">不可新增</span>}</span></label>)}</fieldset>; })}
    {selected.filter((code) => !catalog.some((p) => p.code === code && p.selectable)).map((code) => <label key={code} className="flex min-h-11 items-center gap-3 break-all"><Checkbox checked onCheckedChange={() => toggle([code], false)} />{code}（已退役或不可配置，请明确移除）</label>)}
    {!selected.length && <FormNotice>该角色暂不授予操作能力。</FormNotice>}
    {missing.map(({ read }) => <FormNotice key={read}>缺少配套查看权限：{catalog.find((p) => p.code === read)?.name}，使用者可能无法进入相关页面。<Button type="button" variant="link" onClick={() => toggle([read], true)}>添加配套查看权限</Button></FormNotice>)}
    <details className="rounded-lg border p-3"><summary className="min-h-11 cursor-pointer font-medium">当前管理员保留能力（不可配置）</summary><p className="mb-3 text-sm text-muted-foreground">以下能力由当前租户管理员持有，不可下放给自定义角色。</p><ul className="space-y-3">{catalog.filter((p) => !p.tenant_custom_role_assignable).map((p) => <li key={p.code} className="text-sm"><PermissionInfo permission={p} showCode={showCodes} /></li>)}</ul></details>
  </div>;
}
export function TenantAuthorizationView({ value }: { value: TenantAuthorization }) {
  const names = (codes: PermissionCode[]) => codes.join('、') || '无';
  return <section className="min-w-0 space-y-4" aria-label="授权解释"><h2 className="text-lg font-semibold">角色与有效权限</h2><p>成员：{value.membership_status === 'active' ? '有效' : value.membership_status === 'suspended' ? '已暂停' : '已结束'} · 账号：{value.account_status === 'active' ? '可用' : '不可用'}</p>
    <div className="grid gap-3 sm:grid-cols-2">{value.roles.map((r) => <article key={r.id} className="min-w-0 rounded-lg border p-3"><p className="break-words font-medium">{r.name} · v{r.version}</p><p className="text-sm">{r.owner_tenant_id ? '本租户自定义' : '平台预置'} · {r.status === 'active' ? '启用' : '停用'}{(value.inactive_reasons.length > 0 || r.status !== 'active') && ' · 已配置，当前不生效'}</p></article>)}</div>
    {!value.roles.length && <FormNotice>当前未获业务授权，请联系租户管理员。</FormNotice>}
    <h3 className="font-medium">当前有效权限（{value.permissions.length}）</h3><ul className="space-y-2 text-sm">{value.permissions.map((p) => <li key={p} className="break-words"><code className="break-all">{p}</code><span className="block text-muted-foreground">全部来源：{value.sources[p]?.map((id) => value.roles.find((r) => r.id === id)?.name ?? id).join('、')}</span></li>)}</ul>
    {value.inactive_reasons.length > 0 && <p className="break-all text-sm">账号及成员恢复后权限：{names(value.restorable_permissions)}。停用角色仍不生效。</p>}
  </section>;
}
export function TenantImpactView({ value, more }: { value: TenantAuthorizationImpact; more?: () => void }) {
  return <section className="min-w-0 space-y-3 rounded-lg border bg-muted/30 p-4" aria-label="变更影响预览"><h3 className="font-semibold">变更影响预览</h3><dl className="grid grid-cols-2 gap-3 text-sm">{[['绑定成员', value.bound_members], ['当前有效成员', value.effective_members], ['实际增权人数', value.gaining_members], ['实际失权人数', value.losing_members], ['恢复后受影响人数', value.restorable_members], ['将失效邀请', value.invalidated_invitations]].map(([name, n]) => <div key={name}><dt className="text-muted-foreground">{name}</dt><dd className="text-lg font-semibold">{n}</dd></div>)}</dl>{value.blocked_reasons.map((r) => <FormNotice error key={r}>{r}</FormNotice>)}
    {value.members.items.map((m) => <details key={m.before.membership_id} className="min-w-0 rounded border p-3"><summary className="cursor-pointer break-words">{m.before.display_name}（{m.before.username}）</summary><div className="mt-2 space-y-2 break-all text-sm"><p>新增角色：{m.added_role_ids.map((id) => m.after.roles.find((r) => r.id === id)?.name).join('、') || '无'}</p><p>移除角色：{m.removed_role_ids.map((id) => m.before.roles.find((r) => r.id === id)?.name).join('、') || '无'}</p><p>实际新增权限：{m.added_permissions.join('、') || '无'}</p><p>实际失去权限：{m.removed_permissions.join('、') || '无'}</p><p>其他来源保留权限：{m.retained_permissions.join('、') || '无'}</p><p>恢复后权限：{m.after.restorable_permissions.join('、') || '无'}</p></div></details>)}{value.members.next_cursor && more && <Button type="button" variant="outline" onClick={more}>加载更多影响成员</Button>}
  </section>;
}
