import { Link } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { AccountAvatar } from '../access/account-table';
import { ObjectHistory } from '../audit/object-history';
import { MemberAuthorization } from './member-authorization';
import { MemberActions } from './member-actions';
import { MemberStatus } from './membership-ui';
import { useTenant } from './context';
import { membershipDate } from './membership-utils';
import { TenantAudit } from './pages';

export function TenantMemberDetails({ id }: { id: string }) {
  const ctx = useTenant(); const { account, client } = useIdentity(); const allowed = ctx.permissions.includes(P.TenantMemberRead);
  const q = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'member', id], queryFn: ({ signal }) => client.tenantMember(ctx.tenant.id, id, signal), enabled: allowed && ctx.available });
  if (!allowed) return <FormNotice error>无权查看本租户成员。</FormNotice>;
  if (q.error) return <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />;
  if (!q.data) return <LoadingState label="正在读取成员…" />;
  const m = q.data;
  return <section className="space-y-5">
    <div className="flex flex-wrap items-start gap-3"><AccountAvatar name={m.account.display_name} /><div className="min-w-0 flex-1 space-y-1"><h2 className="break-words font-semibold">{m.account.display_name}</h2><p className="break-all text-sm text-muted-foreground">{m.account.username}</p><MemberStatus item={m} /></div><ObjectHistory objectType="tenant_member" objectId={id} /></div>
    <dl className="grid gap-3 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2">
      <div className="sm:col-span-2"><dt className="text-muted-foreground">成员关系</dt><dd className="mt-1 break-all text-xs">{m.id}</dd></div>
      <div><dt className="text-muted-foreground">加入时间</dt><dd className="mt-1 tabular-nums">{membershipDate(m.joined_at)}</dd></div>
      <div><dt className="text-muted-foreground">加入方式</dt><dd className="mt-1">{m.source_invitation_id ? '本人接受邀请' : '平台指定管理员'}</dd></div>
      <div className="sm:col-span-2"><dt className="text-muted-foreground">最近状态变化</dt><dd className="mt-1 break-words">{membershipDate(m.status_changed_at)} · {m.status_reason || '初次加入'}</dd></div>
      {m.ended_at && <div className="sm:col-span-2"><dt className="text-muted-foreground">关系结束</dt><dd className="mt-1 break-words">{membershipDate(m.ended_at)} · {m.end_reason}</dd></div>}
    </dl>
    <p className="text-xs text-muted-foreground">时间均为北京时间。</p>
    {m.source_invitation_id && ctx.permissions.includes(P.TenantInvitationRead) && <Link className="block text-sm underline underline-offset-4" to="/t/$tenantId/invitations/$invitationId" params={{ tenantId: ctx.tenant.id, invitationId: m.source_invitation_id }}>查看来源邀请</Link>}
    <MemberActions key={m.id} item={m} />
    <Tabs defaultValue="roles" className="gap-4">
      <TabsList className="w-full"><TabsTrigger value="roles" className="flex-1 max-sm:text-xs">绑定角色</TabsTrigger><TabsTrigger value="authorization" className="flex-1 max-sm:text-xs">授权详情</TabsTrigger>{ctx.permissions.includes(P.TenantAuditRead) && <TabsTrigger value="history" className="flex-1 max-sm:text-xs">状态记录</TabsTrigger>}</TabsList>
      <TabsContent value="roles" className="space-y-4"><div className="flex items-center justify-between gap-3"><h2 className="text-sm font-semibold">本次角色授予记录</h2><Badge variant="secondary">{m.roles.length} 个</Badge></div>
        <ul className="space-y-3">{m.roles.map((role) => <li key={role.binding_id} className="space-y-2 rounded-lg border p-4"><div className="flex flex-wrap items-start justify-between gap-2"><h3 className="min-w-0 break-words text-sm font-medium">{role.name}</h3><Badge variant="outline">{role.revoked_at ? '已撤销' : m.role_effective && role.status === 'active' ? '当前生效' : '当前不生效'}</Badge></div><p className="text-xs text-muted-foreground">授予于 {membershipDate(role.assigned_at)}</p>{role.revoked_at && <p className="text-xs text-muted-foreground">已撤销于 {membershipDate(role.revoked_at)}</p>}</li>)}</ul>
        {!m.roles.length && <EmptyState title="尚未绑定租户角色" description="成员关系仍保留，可在授权详情中查看权限和配置角色。" />}
      </TabsContent>
      <TabsContent value="authorization"><MemberAuthorization id={id} /></TabsContent>
      {ctx.permissions.includes(P.TenantAuditRead) && <TabsContent value="history"><h2 className="mb-3 text-sm font-semibold">成员状态记录</h2><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} objectType="tenant_member" objectId={id} /></TabsContent>}
    </Tabs>
  </section>;
}
