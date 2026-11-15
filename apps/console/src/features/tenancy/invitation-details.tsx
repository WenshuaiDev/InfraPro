import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Info } from 'lucide-react';
import { Permissions as P, type TenantInvitationCreated } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ObjectHistory } from '../audit/object-history';
import { useTenant } from './context';
import { InvitationActions } from './invitation-actions';
import { InvitationLink } from './membership-shared';
import { TenantInvitationStatus } from './membership-ui';
import { membershipDate } from './membership-utils';
import { TenantAudit } from './pages';

export function TenantInvitationDetails({ id }: { id: string }) {
  const ctx = useTenant(); const { account, client } = useIdentity(); const [link, setLink] = useState<TenantInvitationCreated>(); const allowed = ctx.permissions.includes(P.TenantInvitationRead);
  const q = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'invitation', id], queryFn: ({ signal }) => client.tenantInvitation(ctx.tenant.id, id, signal), enabled: allowed && ctx.available, refetchInterval: 30_000 });
  if (!allowed) return <FormNotice error>无权查看本租户成员邀请。</FormNotice>;
  if (q.error) return <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />;
  if (!q.data) return <LoadingState label="正在读取邀请…" />;
  const i = q.data;
  return <>
    <div className="flex flex-wrap items-center justify-between gap-3"><span className="text-xs text-muted-foreground">邀请记录</span><TenantInvitationStatus status={i.status} /></div>
    <h3 className="mt-5 break-words text-xl font-semibold tracking-tight">{i.target.display_name}</h3><p className="mt-2 break-all text-sm text-muted-foreground">{i.target.username}</p>
    <dl className="mt-7 grid grid-cols-[5rem_minmax(0,1fr)] gap-x-4 gap-y-4 border-y py-6 text-sm">
      <dt className="text-muted-foreground">邀请编号</dt><dd className="break-all text-xs leading-5">{i.id}</dd>
      <dt className="text-muted-foreground">发起人</dt><dd className="break-words">{i.inviter.display_name}<span className="mt-1 block break-all text-xs text-muted-foreground">{i.inviter.username}</span></dd>
      <dt className="text-muted-foreground">创建时间</dt><dd className="tabular-nums">{membershipDate(i.created_at)}</dd>
      <dt className="text-muted-foreground">有效期至</dt><dd className="tabular-nums">{membershipDate(i.expires_at)}</dd>
      <dt className="text-muted-foreground">加入身份</dt><dd>租户普通成员<span className="block text-xs leading-6 text-muted-foreground">仅限受邀账号本人接受</span></dd>
      {i.closed_at && <><dt className="text-muted-foreground">结束时间</dt><dd className="break-words">{membershipDate(i.closed_at)}<span className="mt-1 block text-xs text-muted-foreground">{i.close_reason}</span></dd></>}
    </dl>
    <p className="mt-2 text-xs text-muted-foreground">时间均为北京时间。</p>
    <div className="mt-5 space-y-3 text-sm">{i.accepted_membership_id && ctx.permissions.includes(P.TenantMemberRead) && <Link className="block underline underline-offset-4" to="/t/$tenantId/members/$membershipId" params={{ tenantId: ctx.tenant.id, membershipId: i.accepted_membership_id }}>查看已加入的成员关系（{i.membership_status}）</Link>}{[[i.replaces_id, '查看原邀请'], [i.replaced_by_id, '查看替代邀请']].filter(([id]) => id).map(([id, label]) => <Link key={id} className="block underline underline-offset-4" to="/t/$tenantId/invitations/$invitationId" params={{ tenantId: ctx.tenant.id, invitationId: id! }}>{label}</Link>)}</div>
    <div className="mt-6 flex gap-3 rounded-lg bg-muted/50 p-4 text-sm leading-6"><Info className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><div><p className="font-medium">完整链接仅在生成时显示</p><p className="mt-1 text-muted-foreground">需要再次交付时，请重新生成链接。原记录会保留，旧链接随即失效。</p></div></div>
    <div className="mt-6"><InvitationActions key={i.id} item={i} done={setLink} /></div>
    <div className="mt-6"><ObjectHistory objectType="tenant_invitation" objectId={id} /></div>
    {ctx.permissions.includes(P.TenantAuditRead) && <details className="mt-5 border-t pt-4"><summary className="cursor-pointer text-sm font-semibold">邀请操作记录</summary><div className="mt-4"><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} objectType="tenant_invitation" objectId={id} /></div></details>}
    <InvitationLink value={link} close={() => setLink(undefined)} />
  </>;
}
