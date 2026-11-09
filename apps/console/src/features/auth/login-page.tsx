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
import { useOperation } from '../account/use-operation';
import { loginSchema, setFormError } from './forms';

export function LoginPage() {
  const identity = useIdentity();
  const search = useSearch({ from: '/_auth/login' });
  const form = useForm({ resolver: zodResolver(loginSchema), defaultValues: { username: '', password: '' } });
  const login = useOperation({ mutationFn: identity.client.login, onSuccess: identity.signedIn });
  if (identity.phase === 'authenticated') return <Navigate to={safeReturn(search.returnTo)} replace />;
  return <AuthCard title="欢迎回来" description="使用组织邀请创建的账号登录。" footer={<p className="text-center">没有账号？请联系已加入的同事获取邀请链接。</p>}>
    <div className="space-y-4"><FormNotice>{search.notice === 'created' ? '账号已创建，请使用新账号登录。' : search.notice === 'password' ? '密码已更新，请重新登录。' : undefined}</FormNotice>
      <Form {...form}><form className="grid gap-3" noValidate aria-busy={login.isPending} onSubmit={form.handleSubmit(async (values) => { try { await login.mutateAsync(values); } catch (error) { setFormError(form, error); } })}>
        <FormInput control={form.control} name="username" label="用户名" autoComplete="username" autoCapitalize="none" spellCheck={false} />
        <FormInput control={form.control} name="password" type="password" label="密码" autoComplete="current-password" labelAction={<Link to="/forgot-password" className="text-sm font-medium text-muted-foreground hover:opacity-75">忘记密码</Link>} />
        <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice>
        <Button className="mt-2" type="submit" disabled={login.isPending}>{login.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <LogIn className="size-4" />}{login.isPending ? '正在登录…' : '登录'}</Button>
      </form></Form>
    </div>
  </AuthCard>;
}
