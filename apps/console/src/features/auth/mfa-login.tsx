import { useEffect, useState } from 'react';
import { ApiError, type EnrollmentSetup, type LoginChallenge, type LoginResult } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { EnrollmentFields, useMFATask } from './mfa-shared';
import { forgetCredential, getCredential } from './credential';

export function MFALogin({ challenge }: { challenge: LoginChallenge }) {
  const identity = useIdentity(); const { client } = identity;
  const task = useMFATask();
  const [method, setMethod] = useState<'totp' | 'recovery_code'>('totp');
  const [code, setCode] = useState(''); const [setup, setSetup] = useState<EnrollmentSetup>();
  const [grant, setGrant] = useState(() => getCredential('mfa-recovery'));
  const id = challenge.challenge_id!;
  const enrollment = challenge.status === 'mfa_enrollment_required'; const recovery = challenge.status === 'mfa_recovery_required';
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer); }, []);
  const expired = !!challenge.expires_at && new Date(challenge.expires_at).getTime() <= now;
  const invalid = task.error instanceof ApiError && ['mfa_challenge_invalid', 'mfa_challenge_completed'].includes(task.error.code ?? '');
  async function done(result: LoginResult) { setCode(''); setGrant(''); setSetup(undefined); if (result.status !== 'mfa_recovery_required') forgetCredential('mfa-recovery'); await identity.acceptLogin(result); }
  return <div className="space-y-4">
    <p className="text-sm">正在验证 <strong>{challenge.account_label}</strong></p>
    <FormNotice>{recovery ? '该账号已进入受控恢复。请使用管理员提供的专用恢复凭证，随后重新绑定验证器。' : enrollment ? '请完成验证器绑定后进入系统。' : '输入验证器中的动态码，或使用一个尚未使用的恢复码。'}</FormNotice>
    <form className="space-y-4" aria-busy={task.pending} onSubmit={(event) => {
      event.preventDefault();
      if (recovery) void task.run(async () => { try { return await client.mfaRecoveryClaim({ challenge_id: id, recovery_token: grant }); } catch (error) { if (error instanceof ApiError && error.code === 'mfa_code_invalid') { forgetCredential('mfa-recovery'); setGrant(''); } throw error; } }, done);
      else if (enrollment && setup) void task.run(() => client.mfaLoginEnrollmentConfirm({ challenge_id: id, new_totp_code: code }), done);
      else if (!enrollment) void task.run(() => client.mfaVerify({ challenge_id: id, method, code }), done);
    }}>
      {recovery ? <div className="space-y-2"><Label htmlFor="mfa-recovery-token">MFA 恢复凭证</Label><Input id="mfa-recovery-token" type="password" value={grant} onChange={(event) => setGrant(event.target.value)} autoComplete="off" /><p className="text-xs text-muted-foreground">仅使用专用恢复链接中的凭证；邀请和密码重置链接不能用于此处。</p></div> : enrollment ? setup ? <EnrollmentFields setup={setup} code={code} onCode={setCode} /> : <Button type="button" disabled={task.pending || expired} onClick={() => void task.run(() => client.mfaLoginEnrollmentStart(id), setSetup)}>显示绑定二维码</Button> : <>
        <div className="space-y-2"><Label htmlFor="mfa-code">{method === 'totp' ? '动态码' : '恢复码'}</Label><Input id="mfa-code" name="code" value={code} onChange={(event) => setCode(event.target.value)} inputMode={method === 'totp' ? 'numeric' : 'text'} autoComplete={method === 'totp' ? 'one-time-code' : 'off'} maxLength={method === 'totp' ? 6 : 64} /></div>
        <Button type="button" variant="link" onClick={() => { setMethod(method === 'totp' ? 'recovery_code' : 'totp'); setCode(''); task.clearError(); }}>{method === 'totp' ? '使用恢复码' : '使用验证器动态码'}</Button>
      </>}
      <FormNotice error>{expired ? '认证步骤已过期，请重新登录。' : task.error?.message}</FormNotice>
      {(!enrollment || setup) && <Button className="w-full" disabled={task.pending || expired || invalid}>{task.pending ? '正在验证…' : recovery ? '认领恢复凭证' : enrollment ? '确认绑定' : '验证并登录'}</Button>}
    </form>
    {invalid && <Button variant="outline" disabled={task.pending} onClick={() => void task.run(identity.reload)}>核对登录状态</Button>}
    <Button variant="ghost" disabled={task.pending} onClick={() => void task.run(() => client.logout(), () => { forgetCredential('mfa-recovery'); identity.signedOut(); })}>退出认证 / 切换账号</Button>
  </div>;
}
