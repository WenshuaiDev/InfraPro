import { useRef, useState } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { Permissions as P, type TenantAuthorization, type TenantAuthorizationImpact } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogTrigger, DialogClose } from '@infrapro/ui/components/dialog';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from './context';
import { ManagementForm, useLivePage } from './membership-shared';
import { TenantAuthorizationView, TenantImpactView } from './role-components';
import { useOperation } from '../account/use-operation';

export function MemberAuthorization({ id }: { id: string }) {
  const ctx = useTenant(); const { client, account } = useIdentity(); const [open, setOpen] = useState(false);
  const q = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'authorization', id], queryFn: ({ signal }) => client.tenantMemberAuthorization(ctx.tenant.id, id, signal), enabled: ctx.available && ctx.permissions.includes(P.TenantMemberRead) });
  if (q.error) return <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />;
  if (!q.data) return <p role="status">正在读取授权…</p>;
  const value = q.data;
  return <section className="mt-6 space-y-4 rounded-xl border p-4 sm:p-6"><TenantAuthorizationView value={value} />{ctx.permissions.includes(P.TenantMemberAssignRoles) && !value.is_current_administrator && value.membership_status !== 'ended' && <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button>配置成员角色</Button></DialogTrigger><DialogContent className="max-h-[88dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>配置成员角色</DialogTitle><DialogDescription>{value.display_name}（{value.username}）· 只影响本次成员关系。可选择多个角色或明确清空。</DialogDescription></DialogHeader>{open && <MemberRoleForm initial={value} finish={() => setOpen(false)} />}</DialogContent></Dialog>}</section>;
}
function MemberRoleForm({ initial, finish }: { initial: TenantAuthorization; finish: () => void }) {
  const ctx = useTenant(); const { client, account, reload } = useIdentity(); const cache = useQueryClient(); const live = useLivePage(); const [value, setValue] = useState(initial); const [selected, setSelected] = useState(initial.roles.map((r) => r.id)); const [impact, setImpact] = useState<TenantAuthorizationImpact>();
  const roles = useInfiniteQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'role-options'], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenantRoles(ctx.tenant.id, { cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: ctx.available });
  const items = [...new Map([...value.roles, ...(roles.data?.pages.flatMap((p) => p.items) ?? [])].map((r) => [r.id, r])).values()];
  const selectionEpoch = useRef(0);
  const input = () => ({ expected_membership_version: value.membership_version, role_ids: selected });
  const preview = useOperation({ mutationFn: async () => { const epoch = selectionEpoch.current; const p = await client.previewTenantMemberRoles(ctx.tenant.id, value.membership_id, input()); if (live.current && epoch === selectionEpoch.current) setImpact(p); } });
  const change = (next: string[]) => { selectionEpoch.current++; setSelected(next); setImpact(undefined); };
  const finalPermissions = new Set(items.filter((r) => selected.includes(r.id) && r.status === 'active').flatMap((r) => r.permission_codes));
  return <ManagementForm label="确认成员授权" disabled={!impact || impact.blocked_reasons.length > 0} refresh={async () => { const current = await client.tenantMemberAuthorization(ctx.tenant.id, value.membership_id); if (live.current) { setValue(current); setImpact(undefined); } await roles.refetch(); }} cancel={<DialogClose asChild><Button type="button" variant="outline">取消</Button></DialogClose>} submit={async (reason) => {
    if (!impact) throw new Error('请先预览影响。'); await client.assignTenantMemberRoles(ctx.tenant.id, value.membership_id, { ...input(), preview_hash: impact.preview_hash, role_versions: impact.role_versions, reason }); if (live.current) { void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id] }); void reload(); finish(); }
  }}>
    <p>已选 {selected.length} 个角色 · 启用角色配置权限并集 {finalPermissions.size} 项</p>{value.membership_status === 'suspended' && <FormNotice>成员已暂停，本次配置用于恢复后的职责，当前不生效。</FormNotice>}{value.account_status !== 'active' && <FormNotice>账号不可用：只允许保留或撤销已有绑定，不能新增。</FormNotice>}
    <Button type="button" variant="outline" onClick={() => change([])}>清空全部角色</Button>{!selected.length && <FormNotice>关系仍保留，但不再具有任何业务操作能力。</FormNotice>}
    <div className="space-y-3">{items.map((r) => { const held = value.roles.some((v) => v.id === r.id); const blocked = r.assignment_policy !== 'manual' || (!held && (r.status !== 'active' || value.account_status !== 'active')); return <div key={r.id} className="min-w-0 rounded-lg border p-3"><label className="flex min-h-11 cursor-pointer items-start gap-3"><Checkbox className="mt-1" disabled={blocked} checked={selected.includes(r.id)} onCheckedChange={(v) => change(v === true ? [...selected, r.id] : selected.filter((id) => id !== r.id))} /><span className="min-w-0 break-words"><span className="font-medium">{r.name}</span><span className="block text-sm">{r.owner_tenant_id ? '本租户自定义' : '平台预置'} · v{r.version} · {r.status === 'active' ? '启用' : '停用，可保留或撤销'}{r.assignment_policy !== 'manual' && ' · 仅管理员交接'}</span></span></label><details><summary className="min-h-11 cursor-pointer text-sm">查看权限（{r.permission_codes.length}）</summary><ul className="space-y-1 break-all text-xs">{r.permission_codes.map((p) => <li key={p}>{p}</li>)}</ul></details></div>; })}</div>
    {roles.hasNextPage && <Button type="button" variant="outline" disabled={roles.isFetchingNextPage} onClick={() => void roles.fetchNextPage()}>加载更多可选角色</Button>}<FormNotice error>{roles.error?.message ?? preview.error?.message}</FormNotice>
    <Button type="button" variant="outline" disabled={preview.isPending} onClick={() => preview.mutate()}>{preview.isPending ? '正在计算…' : '预览成员授权影响'}</Button>{impact && <TenantImpactView value={impact} />}
  </ManagementForm>;
}
