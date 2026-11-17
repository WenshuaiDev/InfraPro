import { useEffect, useRef, useState } from 'react';
import type { EnrollmentSetup } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';

// Secrets and input codes never enter TanStack mutation/query caches or drafts.
// Only the active component owns the result; late responses are discarded.
// eslint-disable-next-line react-refresh/only-export-components
export function useMFATask() {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<Error>();
  const active = useRef(true); const busy = useRef(false);
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  async function run<T>(work: () => Promise<T>, complete?: (result: T) => void | Promise<void>) {
    if (busy.current) return;
    busy.current = true; setPending(true); setError(undefined);
    try { const result = await work(); if (active.current) await complete?.(result); }
    catch (failure) { if (active.current) setError(failure instanceof Error ? failure : new Error('操作未完成，请核对当前状态。')); }
    finally { busy.current = false; if (active.current) setPending(false); }
  }
  return { pending, error, clearError: () => setError(undefined), run };
}

export function RecoveryCodes({ codes, onSaved }: { codes: string[]; onSaved: () => void }) {
  const task = useMFATask();
  const contents = `InfraPro 账号恢复凭证\n每码只能使用一次，请与验证器分开保管。\n\n${codes.join('\n')}\n`;
  return <section aria-labelledby="recovery-codes-title" className="space-y-4 rounded-lg border p-4" data-sensitive="true">
    <h2 id="recovery-codes-title" className="font-semibold">保存恢复码</h2>
    <p className="text-sm text-muted-foreground">这 10 个恢复码仅展示一次。验证器不可用时，可用密码和其中一个恢复码登录。请妥善保存，勿发送给他人。</p>
    <ul aria-label="一次性恢复码" className="grid gap-2 font-mono text-sm">{codes.map((code) => <li key={code} className="break-all rounded bg-muted p-2">{code}</li>)}</ul>
    <div className="flex flex-wrap gap-2"><Button variant="outline" onClick={() => void task.run(() => navigator.clipboard.writeText(contents))}>复制恢复码</Button><Button variant="outline" onClick={() => {
      const url = URL.createObjectURL(new Blob([contents], { type: 'text/plain;charset=utf-8' })); const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'InfraPro-recovery-codes.txt'; anchor.click(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    }}>下载恢复码</Button></div>
    <FormNotice error>{task.error?.message}</FormNotice><Button onClick={onSaved}>我已保存，继续</Button>
  </section>;
}

export function EnrollmentFields({ setup, code, onCode }: { setup: EnrollmentSetup; code: string; onCode: (code: string) => void }) {
  return <div className="space-y-4" data-sensitive="true">
    <p className="text-sm text-muted-foreground">在验证器中扫描二维码，或手动输入密钥，然后输入新验证器的动态码。</p>
    <img src={`data:image/png;base64,${setup.qr_png_base64}`} alt="用于绑定当前账号验证器的二维码" width={256} height={256} className="mx-auto max-w-full rounded-lg bg-white" />
    <div><p className="text-sm font-medium">手工密钥 · {setup.issuer} / {setup.account_label}</p><code className="mt-2 block break-all rounded bg-muted p-3 text-sm select-all">{setup.secret}</code></div>
    <div className="space-y-2"><Label htmlFor="new-totp-code">新验证器动态码</Label><Input id="new-totp-code" name="new_totp_code" value={code} onChange={(event) => onCode(event.target.value)} inputMode="numeric" autoComplete="one-time-code" maxLength={6} placeholder="6 位数字" /></div>
    <p className="text-xs text-muted-foreground">绑定步骤到期：{new Date(setup.expires_at).toLocaleTimeString()}。扫码不会延长有效期。</p>
  </div>;
}
