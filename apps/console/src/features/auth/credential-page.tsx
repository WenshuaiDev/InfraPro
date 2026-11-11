import { useEffect, useState, useSyncExternalStore } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { useIdentity } from '@infrapro/api-client/react';
import { AuthCard } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';
import { forgetCredential, getCredential, subscribeCredential } from './credential';
import { date, passwordSchema, setFormError } from './forms';

export function CredentialPage({ reset = false }: { reset?: boolean }) {
  const token = useSyncExternalStore(subscribeCredential, () => getCredential(reset ? 'reset' : 'invitation'));
  return <CredentialForm key={token} token={token} reset={reset} />;
}

function CredentialForm({ token, reset }: { token: string; reset: boolean }) {
  const { client, account, signedOut, help } = useIdentity();
  const navigate = useNavigate();
  const [expires, setExpires] = useState('');
  const [loadError, setLoadError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const min = help?.password_min ?? 8; const max = help?.password_max ?? 128;
  const schema = z.object({ username: reset ? z.string() : z.string().regex(/^[A-Za-z0-9_-]{3,32}$/, '用户名需为 3–32 位英文字母、数字、下划线或短横线。'), display_name: z.string().refine((value) => Array.from(value).length <= 50, '显示名称不能超过 50 个字符。'), password: passwordSchema(min, max), confirm: z.string() }).refine((value) => value.password === value.confirm, { path: ['confirm'], message: '两次输入的密码不一致。' });
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { username: '', display_name: '', password: '', confirm: '' } });
  const password = useWatch({ control: form.control, name: 'password' });
  const submit = useOperation({ mutationFn: async (values: z.infer<typeof schema>) => { if (reset) await client.completeReset({ token, password: values.password }); else await client.acceptInvitation({ token, username: values.username, display_name: values.display_name, password: values.password }); }, onSuccess: () => {
    forgetCredential(reset ? 'reset' : 'invitation'); form.reset(); if (reset) signedOut(); void navigate({ to: '/login', search: { notice: reset ? 'password' : 'created' }, replace: true });
  } });
  const logout = useOperation({ mutationFn: () => client.logout(), onSuccess: signedOut });
  useEffect(() => {
    if (!token || (account && !reset)) return;
    let active = true;
    const inspect = reset ? client.inspectReset : client.inspectInvitation;
    void inspect(token).then((result) => { if (active) { setExpires(result.expires_at); setLoadError(''); } }, (error: unknown) => { if (active) setLoadError(error instanceof Error ? error.message : '链接校验失败。'); });
    return () => { active = false; };
  }, [client, token, reset, account, attempt]);
  const back = <Link to="/login" className="text-sm underline underline-offset-4">返回登录</Link>;
  if (!token) return <AuthCard title="请重新打开原链接" description="页面刷新后不会保留链接凭证。请重新打开邀请人或维护人员交付的完整链接。" footer={back} />;
  if (account && !reset) return <AuthCard title="先退出当前账号" description={`当前登录身份：${account.display_name}（${account.username}）。创建新账号前需要退出。`}><div className="space-y-3"><FormNotice error>{logout.error?.message}</FormNotice><Button variant="outline" disabled={logout.isPending} onClick={() => logout.mutate()}>退出并继续</Button></div></AuthCard>;
  if (loadError) return <AuthCard title="暂时无法使用此链接" description={`可重试校验；链接已失效时，请联系${reset ? '维护人员' : '邀请人'}获取新链接。`} footer={back}><div className="space-y-3"><FormNotice error>{loadError}</FormNotice><Button variant="outline" onClick={() => setAttempt((value) => value + 1)}>重新校验</Button></div></AuthCard>;
  if (!expires) return <LoadingState label="正在校验链接…" />;
  return <AuthCard title={reset ? '设置新密码' : '创建你的账号'} description={`链接有效至 ${date(expires)}，成功提交后只能使用一次。`} footer={back}>
    <Form {...form}><form className="grid gap-4" noValidate aria-busy={submit.isPending} onSubmit={form.handleSubmit(async (values) => { try { await submit.mutateAsync(values); } catch (error) { setFormError(form, error); } })}>
      {!reset && <><FormInput control={form.control} name="username" label="用户名" autoComplete="username" autoCapitalize="none" hint="3–32 位英文字母、数字、下划线或短横线，创建后不能修改。" /><FormInput control={form.control} name="display_name" label="显示名称" autoComplete="nickname" hint="可使用中文，留空使用用户名。" /></>}
      <FormInput control={form.control} name="password" type="password" label="新密码" autoComplete="new-password" hint={`${min}–${max} 个字符，允许空格和中文。当前 ${Array.from(password).length} 个字符。`} />
      <FormInput control={form.control} name="confirm" type="password" label="确认新密码" autoComplete="new-password" />
      <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><Button type="submit" disabled={submit.isPending}>{submit.isPending ? '正在提交…' : reset ? '重置密码' : '创建账号'}</Button>
    </form></Form>
  </AuthCard>;
}
