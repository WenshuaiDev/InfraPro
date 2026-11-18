import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type EnrollmentSetup } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ArrowRight, ShieldCheck } from 'lucide-react';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { EnrollmentFields, RecoveryCodes, useMFATask } from '../auth/mfa-shared';

export function PasswordConfirmation({ onConfirmed }: { onConfirmed: () => void }) {
  const { client } = useIdentity(); const [password, setPassword] = useState(''); const task = useMFATask();
  return <form className="space-y-3 rounded-lg border p-4" onSubmit={(event) => { event.preventDefault(); void task.run(() => client.reauthenticate(password), () => { setPassword(''); onConfirmed(); }); }}>
    <Label htmlFor="mfa-current-password">当前密码</Label><Input id="mfa-current-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} />
    <FormNotice error>{task.error?.message}</FormNotice><Button disabled={task.pending}>{task.pending ? '正在确认…' : '确认密码'}</Button>
    <p className="text-sm text-muted-foreground">确认后请重新提交原操作。</p>
  </form>;
}

export function MFACard() {
  const identity = useIdentity(); const { client, account } = identity; const cache = useQueryClient();
  const query = useQuery({ queryKey: ['account', account?.id, 'mfa'], queryFn: client.mfa, retry: false });
  const task = useMFATask(); const [mode, setMode] = useState<'enable' | 'replace' | 'disable' | 'regenerate'>();
  const [setup, setSetup] = useState<EnrollmentSetup>(); const [code, setCode] = useState(''); const [codes, setCodes] = useState<string[]>();
  const [notice, setNotice] = useState('');
  const [open, setOpen] = useState(false);
  const reauth = task.error instanceof ApiError && task.error.code === 'reauthentication_required';
  const labels = { enable: '启用多因素认证', replace: '更换验证器', disable: '关闭多因素认证', regenerate: '重建恢复码' };
  async function refresh() { await cache.invalidateQueries({ queryKey: ['account', account?.id] }); await identity.reload(); }
  function close() { setMode(undefined); setSetup(undefined); setCode(''); task.clearError(); }
  function submit() {
    const current = query.data; if (!current || !mode) return;
    if (mode === 'enable' || mode === 'replace') {
      if (!setup) void task.run(() => client.mfaEnrollmentStart({ mode, expected_factor_version: current.factor_version }), setSetup);
      else void task.run(() => client.mfaEnrollmentConfirm({ enrollment_id: setup.enrollment_id, new_totp_code: code }), async (result) => { setCodes(result.recovery_codes?.length ? result.recovery_codes : undefined); setNotice(result.secret_unavailable ? '此操作已完成。若尚未保存恢复码，可重新生成。' : '新验证器已启用。'); close(); await refresh(); });
    } else if (mode === 'disable') void task.run(() => client.mfaDisable({ expected_factor_version: current.factor_version }), async () => { setNotice('多因素认证已关闭，当前会话已更新，其他设备已退出。'); close(); await refresh(); });
    else void task.run(() => client.mfaRegenerate({ expected_factor_version: current.factor_version, expected_recovery_generation: current.recovery_generation }), async (result) => { setCodes(result.recovery_codes?.length ? result.recovery_codes : undefined); setNotice(result.secret_unavailable ? '原操作已完成。恢复码只展示一次；如未保存，请核对状态后主动重建。' : '旧恢复码已全部失效，现有登录会话保持有效。'); close(); await refresh(); });
  }
  const status = query.error ? undefined : query.data;
  const stateLabel = status ? { enabled: '已启用', disabled: '未启用', reset_required: '等待恢复' }[status.state] : '';
  const policy = status?.policy_required ? '平台要求全员启用' : '平台允许自主选择';
  function dismiss() {
    if ((task.pending && mode) || codes) return;
    const finish = () => { close(); setOpen(false); };
    if (setup) void task.run(() => client.mfaEnrollmentCancel(setup.enrollment_id), finish);
    else finish();
  }
  return <section aria-labelledby="mfa-title" className="account-cover flex min-h-52 flex-col items-start justify-between gap-5">
    <h2 id="mfa-title" className="account-cover-muted flex items-center gap-2 text-sm font-medium"><ShieldCheck aria-hidden="true" className="size-5" />登录保护</h2>
    <div><p className="text-2xl font-semibold">{status ? '多因素认证' + stateLabel : query.error ? '认证状态暂时无法读取' : '正在读取认证状态…'}</p>
      {status && <p className="account-cover-muted mt-1 text-sm">{policy}</p>}
    </div>
    <Dialog open={open} onOpenChange={next => { if (next) { setOpen(true); setNotice(''); } else dismiss(); }}>
      <DialogTrigger asChild><Button className="bg-white text-slate-900 hover:bg-slate-100">管理验证器<ArrowRight aria-hidden="true" /></Button></DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto" showCloseButton={!(task.pending && mode) && !codes}>
        <DialogHeader><DialogTitle>{mode ? labels[mode] : '管理验证器'}</DialogTitle>
          <DialogDescription>{mode === 'disable' ? '关闭后使用密码登录，其他设备需要重新登录。' :
            mode === 'regenerate' ? '旧恢复码将立即失效，新恢复码只展示一次。' :
            mode === 'replace' ? '确认新验证器后，原验证器、恢复码和其他设备会话将失效。' :
            mode === 'enable' ? '用验证器扫描二维码，验证成功后保存恢复码。' : '使用密码和验证器共同保护登录。'}</DialogDescription>
        </DialogHeader>
        <FormNotice>{notice}</FormNotice>
        {codes ? <RecoveryCodes codes={codes} onSaved={() => setCodes(undefined)} /> : mode ? <>
          <form className="space-y-4" onSubmit={event => { event.preventDefault(); submit(); }}>
            {setup && <EnrollmentFields setup={setup} code={code} onCode={setCode} />}
            <FormNotice error>{task.error?.message}</FormNotice>
            <Button disabled={task.pending || reauth}>{task.pending ? '正在处理…' : setup ? '确认新验证器' : mode === 'enable' || mode === 'replace' ? '开始设置' : '确认操作'}</Button>
          </form>
          {reauth && <PasswordConfirmation onConfirmed={() => { task.clearError(); setNotice('密码已确认，请再次提交原操作。'); }} />}
        </> : <>
          <FormNotice error>{query.error?.message}</FormNotice>
          {query.isPending && <p role="status" className="text-sm">正在读取认证状态…</p>}
          {status && <div className="space-y-4">
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-2 rounded-lg bg-muted/50 p-4 text-sm">
              <dt className="text-muted-foreground">验证器</dt><dd>{stateLabel}</dd>
              <dt className="text-muted-foreground">平台策略</dt><dd>{policy}</dd>
              {status.state === 'enabled' && <><dt className="text-muted-foreground">剩余恢复码</dt><dd>{status.recovery_codes_remaining} / 10</dd></>}
            </dl>
            <fieldset disabled={task.pending} className="flex flex-wrap gap-2"><legend className="sr-only">验证器操作</legend>{status.state === 'disabled' ? <Button onClick={() => setMode('enable')}>启用多因素认证</Button> : <>
              <Button variant="outline" onClick={() => setMode('replace')}>更换验证器</Button>
              <Button variant="outline" onClick={() => setMode('regenerate')}>重建恢复码</Button>
              <Button variant="outline" disabled={!status.can_disable} onClick={() => setMode('disable')}>关闭 MFA</Button>
            </>}</fieldset>
          </div>}
          <Button variant="ghost" className="justify-self-start" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新认证状态</Button>
        </>}
      </DialogContent>
    </Dialog>
  </section>;
}
