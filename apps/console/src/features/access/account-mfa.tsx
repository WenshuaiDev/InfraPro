import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ApiError, Permissions as P, type MFAResult } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { PasswordConfirmation } from '../account/mfa-card';
import { useMFATask } from '../auth/mfa-shared';

export function AccountMFASection({ id }: { id: string }) {
  const { client, account, can } = useIdentity(); const task = useMFATask();
  const query = useQuery({ queryKey: ['access', account?.id, 'account-mfa', id], queryFn: () => client.accountMFA(id), retry: false });
  const [review, setReview] = useState<string>(); const [reason, setReason] = useState(''); const [result, setResult] = useState<MFAResult>();
  const reauth = task.error instanceof ApiError && task.error.code === 'reauthentication_required';
  return <section className="space-y-3 rounded-lg border p-4" aria-label="账号 MFA 状态">
    <h3 className="font-medium">多因素认证</h3><p className="text-sm">{query.isError ? '暂时无法核实认证状态' : query.isPending ? '正在读取…' : query.data?.state === 'enabled' ? '已启用验证器' : query.data?.state === 'reset_required' ? '等待受控恢复' : '未启用'}</p><FormNotice error>{query.error?.message}</FormNotice>
    {query.isError && <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>重新读取认证状态</Button>}
    {id === account?.id ? <p className="text-sm text-muted-foreground">请前往账号安全页更换自己的验证器。</p> : can(P.PlatformAccountMfaReset) && query.data && query.data.state !== 'disabled' && !review && !result && <Button variant="outline" disabled={query.isError || query.isFetching} onClick={() => { setReview(query.data!.factor_version); setReason(''); }}>重置该账号 MFA</Button>}
    {review && <><FormNotice>仅在核验本人身份后重置。目标的旧验证器、恢复码及全部会话将失效；账号状态、角色和平台策略保持不变。</FormNotice><form className="space-y-3" onSubmit={(event) => { event.preventDefault(); if (query.isError || query.isFetching) return; void task.run(() => client.resetAccountMFA(id, { expected_factor_version: review, reason }), async (out) => { setResult(out); setReview(undefined); await query.refetch(); }); }}><Label htmlFor="mfa-reset-reason">重置原因</Label><Input id="mfa-reset-reason" value={reason} maxLength={500} onChange={(event) => setReason(event.target.value)} /><p className="text-xs text-muted-foreground">仅填写业务原因，勿填写凭证。</p><FormNotice error>{task.error?.message}</FormNotice><div className="flex flex-wrap gap-2"><Button disabled={task.pending || reauth || query.isError || query.isFetching || !reason.trim()}>确认重置</Button><Button type="button" variant="ghost" disabled={task.pending} onClick={() => setReview(undefined)}>取消</Button></div></form>{reauth && <PasswordConfirmation onConfirmed={task.clearError} />}</>}
    {result && <div className="space-y-3" data-sensitive="true"><FormNotice>{result.secret_unavailable ? '原重置已完成，恢复链接不重复展示。若未保存，可关闭此结果并主动重新发放。' : '恢复链接只展示一次。请通过可信渠道交付给已核验身份的用户。'}</FormNotice>{result.recovery_url && <><Label htmlFor="mfa-reset-link">专用 MFA 恢复链接</Label><Input id="mfa-reset-link" value={result.recovery_url} readOnly /><p className="text-xs text-muted-foreground">到期：{result.expires_at && new Date(result.expires_at).toLocaleString()}</p><Button variant="outline" onClick={() => void task.run(() => navigator.clipboard.writeText(result.recovery_url!))}>复制恢复链接</Button></>}<FormNotice error>{task.error?.message}</FormNotice><Button variant="ghost" onClick={() => setResult(undefined)}>关闭结果</Button></div>}
  </section>;
}
