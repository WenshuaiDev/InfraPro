import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Activity } from 'lucide-react';
import { AuthLayout, AuthCard } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import type { Bootstrap } from '@infrapro/diagnostics-client';
import { useSession } from './hooks';
export function Login() {
  const { client, login, unavailable } = useSession(); const [bootstrap, setBootstrap] = useState<Bootstrap>();
  const [username, setUsername] = useState(''); const [password, setPassword] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const [retry, setRetry] = useState(0); const errorSummary = useRef<HTMLDivElement>(null);
  useEffect(() => { const controller = new AbortController(); void client.bootstrap(controller.signal).then(({ data }) => { setBootstrap(data); setError(''); }).catch((error: unknown) => { if (!(error instanceof DOMException && error.name === 'AbortError')) setError('无法连接独立诊断服务，请重试连接。'); }); return () => controller.abort(); }, [client, retry]);
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setError(''); if (!username || !password) { setError('请输入独立诊断账号和密码。'); errorSummary.current?.focus(); return; }
    setBusy(true); try { await login(username, password); setPassword(''); } catch (error) { setError(error instanceof Error ? error.message : '登录失败'); setPassword(''); errorSummary.current?.focus(); await client.bootstrap().then(({ data }) => setBootstrap(data)).catch(() => {}); } finally { setBusy(false); }
  };
  return <AuthLayout brand={{ name: 'InfraPro · 运行诊断', logo: <Activity /> }}><AuthCard title="独立诊断账号登录" description="查看本实例的进程、依赖和基础资源。诊断账号由服务器本地维护。" footer="只读诊断 · 独立地址 · 独立身份">
    <form onSubmit={(event) => { void submit(event); }} className="space-y-5">
      <div ref={errorSummary} tabIndex={-1} id="login-errors" className={error || unavailable ? 'rounded border border-destructive/40 p-3 text-sm text-destructive' : 'sr-only'}>{error || (unavailable ? '诊断账号配置暂不可用，当前访问已关闭。' : '')}</div>
      {bootstrap && !bootstrap.initialized && <p className="rounded border p-3 text-sm">诊断账号尚未初始化。请在服务器本地使用账号初始化命令完成设置。</p>}
      <div className="space-y-2"><Label htmlFor="username">诊断账号</Label><Input id="username" autoComplete="username" maxLength={64} value={username} onChange={(event) => setUsername(event.target.value)} aria-describedby={error ? 'login-errors' : undefined} required /></div>
      <div className="space-y-2"><Label htmlFor="password">密码</Label><Input id="password" type="password" autoComplete="current-password" maxLength={128} value={password} onChange={(event) => setPassword(event.target.value)} aria-describedby={error ? 'login-errors' : undefined} required /></div>
      <Button type="submit" disabled={busy || !bootstrap?.login_allowed} className="min-h-11 w-full">{busy ? '正在验证…' : '登录诊断'}</Button>
      {!bootstrap && error && <Button type="button" variant="outline" className="min-h-11 w-full" onClick={() => setRetry((value) => value + 1)}>重试连接</Button>}
    </form>
  </AuthCard></AuthLayout>;
}
