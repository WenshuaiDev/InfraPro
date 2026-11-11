import { MemberAuthorization } from './member-authorization';
import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { Permissions as P, type TenantMember } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Badge } from '@infrapro/ui/components/badge';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from './context';
import { MembershipConfirm } from './membership-shared';
import { memberStatus, membershipDate } from './membership-utils';
import { TenantAudit } from './pages';

function MemberLink({ item }: { item: TenantMember }) { return <Link className="inline-block min-h-11 break-words font-medium underline" to="/t/$tenantId/members/$membershipId" params={{ tenantId: item.tenant_id, membershipId: item.id }}>{item.account.display_name}（{item.account.username}）</Link>; }
function Roles({ item }: { item: TenantMember }) { return <span>{item.roles.filter((r) => item.membership_status === 'ended' || !r.revoked_at).map((r) => r.name).join('、')}{!item.role_effective && ' · 当前不生效'}</span>; }
function MemberActions({ item }: { item: TenantMember }) {
  const ctx = useTenant(); const { client, account } = useIdentity(); const cache = useQueryClient(); const [target, setTarget] = useState(item);
  if (item.account.id === account?.id) return <p className="text-sm text-muted-foreground">不能通过成员管理操作自己</p>;
  if (item.is_current_administrator) return <p className="text-sm text-muted-foreground">当前管理员，请联系平台先交接</p>;
  if (item.roles.filter((r) => !r.revoked_at).some((r) => r.permission_codes.some((p) => !ctx.permissions.includes(p)))) return <p className="text-sm text-muted-foreground">目标配置超出本人权限范围，请联系当前管理员</p>;
  const actions = [
    { key: 'suspend' as const, title: '暂停成员', permission: P.TenantMemberSuspend, show: item.membership_status === 'active', effect: '将失去本租户访问，可恢复；其他租户与登录不受影响。' },
    { key: 'resume' as const, title: '恢复成员', permission: P.TenantMemberResume, show: item.membership_status === 'suspended', effect: '将恢复同一成员关系及保留的本租户角色；要求账号正常。' },
    { key: 'remove' as const, title: '移除成员', permission: P.TenantMemberRemove, show: item.membership_status !== 'ended', effect: '本次关系将结束并撤销角色；再次加入需要新邀请。' },
  ];
  return <div className="flex flex-wrap gap-2 md:justify-end">{actions.filter((a) => a.show && ctx.permissions.includes(a.permission)).map((a) => <MembershipConfirm prepare={() => setTarget(item)} key={a.key} title={a.title} description={`${ctx.tenant.name} · ${item.account.display_name}（${item.account.username}）。${a.effect}`} refresh={async () => { setTarget(await client.tenantMember(ctx.tenant.id, item.id)); }} submit={async (reason) => { await client.changeTenantMember(ctx.tenant.id, item.id, a.key, { expected_version: target.version, reason }); void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id] }); }}><p>核对状态：{memberStatus[target.membership_status]} · {target.account_available ? '账号可用' : '账号不可用'}</p></MembershipConfirm>)}</div>;
}
// TanStack renders cell functions as components: stable definitions preserve open dialogs on refetch.
const memberColumns: ColumnDef<TenantMember>[] = [{ id: 'account', size: 220, header: '成员', cell: ({ row }) => <MemberLink item={row.original} /> }, { id: 'role', size: 240, header: '角色', cell: ({ row }) => <Roles item={row.original} /> }, { id: 'state', size: 168, header: '成员 / 账号状态', cell: ({ row }) => <p>{memberStatus[row.original.membership_status]} · {row.original.account_available ? '账号可用' : '账号不可用'}</p> }, { id: 'joined', size: 176, header: '加入时间（上海）', cell: ({ row }) => membershipDate(row.original.joined_at) }, { id: 'changed', size: 176, header: '状态变更时间', cell: ({ row }) => membershipDate(row.original.status_changed_at) }, { id: 'actions', size: 224, header: '操作', cell: ({ row }) => <MemberActions key={row.original.id} item={row.original} /> }];
export function TenantMembersPage() {
  const ctx = useTenant(); const { client, account } = useIdentity(); const [q, setQ] = useState(''); const [status, setStatus] = useState(''); const [role, setRole] = useState(''); const allowed = ctx.permissions.includes(P.TenantMemberRead);
  const list = useInfiniteQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'members', q, status, role], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenantMembers(ctx.tenant.id, { q, status, role, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: allowed && ctx.available, refetchInterval: 30_000 });
  if (!allowed) return <FormNotice error>无权查看本租户成员。</FormNotice>;
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  return <><PageHeader title="成员管理" description={`${ctx.tenant.name} · 当前关系与历史关系分别保存`} actions={ctx.permissions.includes(P.TenantInvitationRead) ? <Button asChild><Link to="/t/$tenantId/invitations" params={{ tenantId: ctx.tenant.id }}>邀请成员</Link></Button> : undefined} />
    <div className="mb-5 flex flex-wrap items-end gap-4"><div className="w-full max-w-sm space-y-2"><Label htmlFor="members-search">搜索本租户成员</Label><Input id="members-search" value={q} onChange={(e) => setQ(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="member-status">成员状态</Label><select id="member-status" className="h-11 rounded-md border bg-background px-3" value={status} onChange={(e) => setStatus(e.target.value)}><option value="">当前成员</option><option value="active">有效</option><option value="suspended">已暂停</option><option value="ended">历史成员</option><option value="all">全部关系</option></select></div><div className="space-y-2"><Label htmlFor="member-role">角色</Label><select id="member-role" className="h-11 rounded-md border bg-background px-3" value={role} onChange={(e) => setRole(e.target.value)}><option value="">全部角色</option><option value="tenant_admin">管理员</option><option value="tenant_member">普通成员</option></select></div><Button variant="outline" onClick={() => { setQ(''); setStatus(''); setRole(''); }}>清除筛选</Button></div>
    <div className="hidden md:block"><DataTable minWidth={1080} label="租户成员列表" columns={memberColumns} data={items} getRowId={(m) => m.id} loading={list.isPending} error={list.error?.message} onRetry={() => void list.refetch()} emptyLabel="没有符合条件的成员" /></div>
    <div className="space-y-3 md:hidden">{list.isPending && <p role="status">正在读取成员…</p>}{list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}{items.map((m) => <article key={m.id} className="space-y-3 rounded-lg border p-4"><MemberLink item={m} /><p className="break-words text-sm"><Roles item={m} /></p><p>{memberStatus[m.membership_status]} · {m.account_available ? '账号可用' : '账号不可用'}</p><p className="text-sm">加入：{membershipDate(m.joined_at)}（上海）</p><p className="text-sm">状态变更：{membershipDate(m.status_changed_at)}</p><MemberActions key={m.id} item={m} /></article>)}{!list.isPending && !list.error && !items.length && <p>没有符合条件的成员。</p>}</div>
    {list.hasNextPage && <Button className="mt-4" variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多成员</Button>}
  </>;
}
export function TenantMemberPage({ id }: { id: string }) {
  const ctx = useTenant(); const { account, client } = useIdentity(); const allowed = ctx.permissions.includes(P.TenantMemberRead);
  const q = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'member', id], queryFn: ({ signal }) => client.tenantMember(ctx.tenant.id, id, signal), enabled: allowed && ctx.available });
  if (!allowed) return <FormNotice error>无权查看本租户成员。</FormNotice>;
  if (q.error) return <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />;
  if (!q.data) return <p role="status">正在读取成员…</p>;
  const m = q.data;
  return <><PageHeader title={`${m.account.display_name}（${m.account.username}）`} description={`${ctx.tenant.name} · 本次成员关系`} /><section className="space-y-4 rounded-xl border p-5"><Badge>{memberStatus[m.membership_status]} · {m.account_available ? '账号可用' : '账号不可用'}</Badge><p className="break-all text-sm">成员关系：{m.id}</p><p>加入时间：{membershipDate(m.joined_at)}（上海）</p><p>加入方式：{m.source_invitation_id ? '本人接受邀请' : '平台指定管理员'}</p>{m.source_invitation_id && ctx.permissions.includes(P.TenantInvitationRead) && <Link className="block underline" to="/t/$tenantId/invitations/$invitationId" params={{ tenantId: ctx.tenant.id, invitationId: m.source_invitation_id }}>查看来源邀请</Link>}<p>最近状态变化：{membershipDate(m.status_changed_at)} · {m.status_reason || '初次加入'}</p>{m.ended_at && <p>结束：{membershipDate(m.ended_at)} · {m.end_reason}</p>}<h2 className="font-semibold">本次角色授予记录</h2>{m.roles.map((r) => <p key={r.binding_id} className="break-words text-sm">{r.name} · 授予于 {membershipDate(r.assigned_at)} · {r.revoked_at ? `已撤销于 ${membershipDate(r.revoked_at)}` : m.role_effective && r.status === 'active' ? '当前生效' : '当前不生效'}</p>)}<MemberActions key={m.id} item={m} /></section><MemberAuthorization id={id} />{ctx.permissions.includes(P.TenantAuditRead) && <section className="mt-6"><h2 className="mb-3 font-semibold">成员状态记录</h2><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} objectType="tenant_member" objectId={id} /></section>}</>;
}
