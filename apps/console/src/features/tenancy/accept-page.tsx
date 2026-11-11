import { useEffect, useState, useSyncExternalStore } from 'react';
import { Link } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, type TenantInvitationInspection } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { AuthCard } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { getCredential, getCredentialGeneration, forgetCredential, subscribeCredential } from '../auth/credential';
import { useOperation } from '../account/use-operation';
import { invitationStatus, memberStatus, membershipDate } from './membership-utils';

export function TenantAcceptPage() {
  const generation = useSyncExternalStore(subscribeCredential, () => getCredentialGeneration('tenant'));
  return <TenantAcceptForm key={generation} />;
}
function TenantAcceptForm() {
  const { account, client, phase, signedOut } = useIdentity(); const cache = useQueryClient();
  const token = useSyncExternalStore(subscribeCredential, () => getCredential('tenant'));
  const [inspection, setInspection] = useState<TenantInvitationInspection>(); const [error, setError] = useState(''); const [attempt, setAttempt] = useState(0); const [result, setResult] = useState<{ tenant_id: string; membership_id: string }>();
  const accept = useOperation({ mutationFn: async () => { if (!inspection?.can_accept) throw new Error('请先核验邀请。'); return client.acceptTenantInvitation({ token, expected_version: inspection.version }); }, onSuccess: (value) => { setResult(value); forgetCredential('tenant'); void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-tenants'] }); } });
  const logout = useOperation({ mutationFn: () => client.logout(), onSuccess: () => { forgetCredential('tenant'); signedOut(); } });
  useEffect(() => {
    if (!token || phase !== 'authenticated') return;
    const controller = new AbortController();
    void client.inspectTenantInvitation(token, controller.signal).then((value) => {
      if (controller.signal.aborted) return; setInspection(value); setError('');
      if (!value.can_accept && value.status !== 'accepted' && value.blocked_reason !== 'tenant_disabled') forgetCredential('tenant');
    }, (e: unknown) => {
      if (controller.signal.aborted) return; setError(e instanceof Error ? e.message : '邀请校验失败。');
      if (e instanceof ApiError && [403, 404].includes(e.status ?? 0)) forgetCredential('tenant');
    });return () => controller.abort();
  }, [account?.id, client, token, phase, attempt]);
  const cancel = <Link to="/tenants" className="underline" onClick={() => forgetCredential('tenant')}>取消并选择工作空间</Link>;
  if (result) return <AuthCard title="已加入租户" description="你已成为租户普通成员，可以查看本租户基本资料。"><Button asChild><Link to="/t/$tenantId/profile" params={{ tenantId: result.tenant_id }}>进入租户</Link></Button></AuthCard>;
  if (inspection?.status === 'accepted') return <AuthCard title="该邀请已经接受" description={`原成员关系当前${memberStatus[inspection.membership_status] ?? inspection.membership_status}，重试不会恢复或重复加入。`} footer={cancel}><p>{inspection.preview?.tenant.name}</p>{inspection.tenant_status === 'disabled' && <FormNotice>租户已停用，暂时无法进入。{inspection.availability?.status_notice}</FormNotice>}{inspection.membership_status === 'active' && inspection.tenant_status !== 'disabled' && inspection.preview && <Button asChild><Link to="/t/$tenantId/profile" params={{ tenantId: inspection.preview.tenant.id }}>进入租户</Link></Button>}</AuthCard>;
  if (!token) return <AuthCard title={error || inspection ? '邀请当前不可使用' : '请重新打开原邀请链接'} description={error || (inspection ? invitationStatus[inspection.status] : '刷新或关闭页面后不会保留凭证，请重新打开管理员交付的完整链接。')} footer={cancel}>{account && <div className="space-y-3"><p>若当前账号不匹配，请先退出，切换到受邀账号后重新打开原链接。</p><FormNotice error>{logout.error?.message}</FormNotice><Button variant="outline" disabled={logout.isPending} onClick={() => logout.mutate()}>退出当前账号</Button></div>}</AuthCard>;
  if (phase !== 'authenticated') return <AuthCard title="登录后确认加入" description="请使用受邀的已有账号登录。登录不会自动加入租户，还需要本人明确确认。"><Button asChild><Link to="/login" search={{ returnTo: '/tenant-invite' }}>登录并继续</Link></Button></AuthCard>;
  if (error) return <AuthCard title="暂时无法核验邀请" description={error}><Button onClick={() => setAttempt((n) => n + 1)}>重新核验</Button></AuthCard>;
  if (inspection?.blocked_reason === 'tenant_disabled') return <AuthCard title="租户已停用，暂时无法加入" description={inspection.availability?.status_notice} footer={cancel}><div className="space-y-4"><p className="break-words">{inspection.availability?.name} · {inspection.availability?.code}</p><p>原有效期至 {membershipDate(inspection.expires_at)}（上海）。停用不延长邀请有效期。</p><p>请联系维护人员，恢复后重新核验并由本人确认加入。</p><Button onClick={() => { accept.reset(); setAttempt((n) => n + 1); }}>重新检查</Button><Button disabled>确认加入</Button></div></AuthCard>;
  if (!inspection?.preview) return <p role="status">正在核验邀请…</p>;
  const preview = inspection.preview;
  return <AuthCard title="确认加入租户" description="请核对租户和当前账号，只有点击确认后才会建立成员关系。" footer={cancel}><div className="space-y-4"><p className="break-words">当前账号：{account?.display_name}（{account?.username}）</p><p className="break-words">租户：{preview.tenant.name} · {preview.tenant.code}</p><p className="break-words">发起人：{preview.inviter.display_name}（{preview.inviter.username}）</p><p>{preview.role}</p><p className="text-sm">有效至 {membershipDate(inspection.expires_at)}（上海）</p><FormNotice error>{accept.error?.message}</FormNotice>{accept.error && <Button variant="outline" onClick={() => { accept.reset(); setAttempt((n) => n + 1); }}>查验当前邀请结果</Button>}<Button disabled={accept.isPending || !!accept.error || !inspection.can_accept} onClick={() => accept.mutate()}>{accept.isPending ? '正在确认…' : '确认加入'}</Button></div></AuthCard>;
}
