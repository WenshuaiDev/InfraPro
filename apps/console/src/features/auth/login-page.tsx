import { Link, Navigate, useSearch } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { LogIn, LoaderCircle } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import { AuthCard } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { safeReturn } from '../../config/navigation';
import { forgetCredential, getCredential } from './credential';
import { MFALogin } from './mfa-login';
import { RecoveryCodes, useMFATask } from './mfa-shared';
import { FormNotice as Notice } from '@infrapro/ui/patterns/feedback';
import { loginSchema, setFormError } from './forms';

export function LoginPage({ recovery = false }: { recovery?: boolean }) {
  const identity = useIdentity();
  const search = useSearch({ strict: false });
  const form = useForm({ resolver: zodResolver(loginSchema), defaultValues: { username: '', password: '' } });
  const login = useMFATask();
  if (identity.recovery) return <AuthCard title="多因素认证已启用" description="请先保存账号恢复凭证。"><RecoveryCodes codes={identity.recovery.codes} onSaved={identity.dismissRecovery} /></AuthCard>;
  if (identity.phase === 'authenticating' && identity.challenge) return <AuthCard title={identity.challenge.status === 'mfa_required' ? '验证登录' : '设置验证器'} description="完成当前认证步骤后进入系统。"><MFALogin key={`${identity.challenge.challenge_id}:${identity.challenge.status}`} challenge={identity.challenge} /></AuthCard>;
  if (identity.phase === 'authenticated' && recovery && getCredential('mfa-recovery')) return <AuthCard title="当前账号已登录" description="恢复链接仅适用于待恢复账号。请核对目标账号；如需切换，请先退出当前登录。"><Button disabled={login.pending} onClick={() => void login.run(() => identity.client.logout(), () => { forgetCredential('mfa-recovery'); identity.signedOut(); })}>退出并重新打开恢复链接</Button></AuthCard>;
  if (identity.phase === 'authenticated') return <Navigate to={safeReturn(search.returnTo)} replace />;
  return <AuthCard title="欢迎回来" description="使用组织邀请创建的账号登录。" footer={<p className="text-center">没有账号？请联系已加入的同事获取邀请链接。</p>}>
    <div className="space-y-4">{recovery && <Notice>请先使用需要恢复 MFA 的账号和密码登录。恢复链接已从地址栏移除。</Notice>}<FormNotice>{search.notice === 'mfa_policy' ? '设置已生效，请重新登录完成 MFA。' : search.notice === 'created' ? '账号已创建，请使用新账号登录。' : search.notice === 'password' ? '密码已更新，请重新登录。' : undefined}</FormNotice>
      <Form {...form}><form className="grid gap-3" noValidate aria-busy={login.pending} onSubmit={form.handleSubmit(async (values) => { try { await login.run(() => identity.beginLogin(values), () => form.reset()); } catch (error) { setFormError(form, error); } })}>
        <FormInput control={form.control} name="username" label="用户名" autoComplete="username" autoCapitalize="none" spellCheck={false} />
        <FormInput control={form.control} name="password" type="password" label="密码" autoComplete="current-password" labelAction={<Link to="/forgot-password" className="text-sm font-medium text-muted-foreground hover:opacity-75">忘记密码</Link>} />
        <FormNotice error>{login.error?.message ?? form.formState.errors.root?.server?.message}</FormNotice>
        <Button className="mt-2" type="submit" disabled={login.pending}>{login.pending ? <LoaderCircle className="size-4 animate-spin" /> : <LogIn className="size-4" />}{login.pending ? '正在登录…' : '登录'}</Button>
      </form></Form>
    </div>
  </AuthCard>;
}
