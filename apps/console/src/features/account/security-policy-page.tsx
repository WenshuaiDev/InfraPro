import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import { ApiError, Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Switch } from '@infrapro/ui/components/switch';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { PasswordConfirmation } from './mfa-card';
import { useMFATask } from '../auth/mfa-shared';

export function SecurityPolicyPage() {
  const identity = useIdentity(); const { client, can } = identity; const navigate = useNavigate();
  const policy = useQuery({ queryKey: ['platform', 'security-policy'], queryFn: client.securityPolicy, retry: false });
  const impact = useQuery({ queryKey: ['platform', 'mfa-impact'], queryFn: client.securityPolicyImpact, retry: false });
  const [draft, setDraft] = useState<boolean>(); const [review, setReview] = useState<{ required: boolean; version: string }>();
  const [reason, setReason] = useState(''); const [notice, setNotice] = useState(''); const task = useMFATask();
  const reauth = task.error instanceof ApiError && task.error.code === 'reauthentication_required';
  const enabled = draft ?? policy.data?.require_mfa ?? false;
  return <><PageHeader title="平台安全策略" description="统一管理所有人员账号的多因素认证要求，适用于平台、租户、项目及尚未加入租户的账号。" />
    <section className="max-w-3xl space-y-6 rounded-xl border p-5 sm:p-6">
      {policy.isPending && <LoadingState />}<FormNotice error>{policy.error?.message}</FormNotice><FormNotice>{notice}</FormNotice>
      {policy.data && <><div className="flex items-center justify-between gap-6"><div className="space-y-2"><Label htmlFor="require-mfa">强制所有人员使用 MFA</Label><p className="text-sm text-muted-foreground">当前已生效：{policy.data.require_mfa ? '开启' : '关闭'}。拨动开关后需保存。</p></div><Switch id="require-mfa" checked={enabled} disabled={!can(P.PlatformSecurityPolicyUpdate) || task.pending} onCheckedChange={setDraft} /></div>
        <p className="text-sm text-muted-foreground">开启后，旧的仅密码会话将失效，未绑定用户需要在下次登录时完成绑定。关闭后，已启用的个人验证器和恢复码仍然保留。</p>
        {can(P.PlatformSecurityPolicyUpdate) ? <Button disabled={task.pending || enabled === policy.data.require_mfa} onClick={() => { setReason(''); task.clearError(); setReview({ required: enabled, version: policy.data!.revision }); }}>保存策略</Button> : <FormNotice>你具有查看权限，不能修改策略或重置账号 MFA。</FormNotice>}
        <p className="text-xs text-muted-foreground">修改人：{policy.data.updated_by_label} · 版本 {policy.data.revision} · 最后更新 {new Date(policy.data.updated_at).toLocaleString()}</p></>}
      {impact.data && <div className="space-y-3 border-t pt-5"><h2 className="font-medium">影响预览</h2><dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">{[['活跃账号', impact.data.active_accounts], ['已绑定', impact.data.enabled_accounts], ['未绑定', impact.data.unenrolled_accounts], ['待恢复', impact.data.reset_required_accounts], ['可能受影响会话', impact.data.potential_password_sessions]].map(([label, value]) => <div key={label}><dt className="text-sm text-muted-foreground">{label}</dt><dd className="mt-1 text-xl font-semibold tabular-nums">{value}</dd></div>)}</dl><p className="text-xs text-muted-foreground">按全局账号去重。会话数是尚未撤销且未绝对到期的密码会话上限，可能包含空闲过期会话。统计时间：{new Date(impact.data.computed_at).toLocaleString()}</p></div>}
      <FormNotice error>{impact.error?.message}</FormNotice><div className="flex gap-4"><Button variant="outline" onClick={() => { setDraft(undefined); void policy.refetch(); void impact.refetch(); }}>核对最新状态</Button>{can(P.PlatformAuditRead) && <Link to="/access/audit" className="self-center text-sm underline">查看平台审计</Link>}</div>
    </section>
    <Dialog open={!!review} onOpenChange={(open) => { if (!open && !task.pending) { setReview(undefined); setDraft(undefined); } }}><DialogContent><DialogHeader><DialogTitle>{review?.required ? '开启平台强制 MFA？' : '关闭平台强制 MFA？'}</DialogTitle><DialogDescription>{review?.required ? '设置提交后，未完成 MFA 的会话需要重新登录，包括你自己的仅密码会话。无需在保存前绑定验证器。' : '解除强制不会删除任何个人 MFA 配置，也不会恢复此前已失效的会话。'}</DialogDescription></DialogHeader>
      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (!review) return; void task.run(() => client.updateSecurityPolicy({ require_mfa: review.required, expected_version: review.version, reason }), async (result) => {
        setReview(undefined); setDraft(undefined);
        if (result.session_action === 'sign_in_again') { identity.signedOut(); await navigate({ to: '/login', search: { notice: 'mfa_policy' }, replace: true }); }
        else { setNotice(result.changed ? '安全策略已生效。' : '策略未变化。'); await policy.refetch(); await impact.refetch(); }
      }); }}><Label htmlFor="policy-reason">变更原因</Label><Input id="policy-reason" value={reason} onChange={(event) => setReason(event.target.value)} maxLength={500} /><p className="text-xs text-muted-foreground">填写业务原因，请勿填写密码、动态码或恢复凭证。</p><FormNotice error>{task.error?.message}</FormNotice><Button disabled={task.pending || reauth || !reason.trim()}>{task.pending ? '正在保存…' : '确认保存'}</Button></form>
      {reauth && <PasswordConfirmation onConfirmed={task.clearError} />}
    </DialogContent></Dialog>
  </>;
}
