import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type EnrollmentSetup } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
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
  const reauth = task.error instanceof ApiError && task.error.code === 'reauthentication_required';
  const labels = { enable: '启用多因素认证', replace: '更换验证器', disable: '关闭多因素认证', regenerate: '重建恢复码' };
  async function refresh() { await cache.invalidateQueries({ queryKey: ['account', account?.id] }); await identity.reload(); }
  function close() { setMode(undefined); setSetup(undefined); setCode(''); task.clearError(); }
  function submit() {
    const current = query.data; if (!current || !mode) return;
    if (mode === 'enable' || mode === 'replace') {
      if (!setup) void task.run(() => client.mfaEnrollmentStart({ mode, expected_factor_version: current.factor_version }), setSetup);
      else void task.run(() => client.mfaEnrollmentConfirm({ enrollment_id: setup.enrollment_id, new_totp_code: code }), async (result) => { setCodes(result.recovery_codes); close(); await refresh(); });
    } else if (mode === 'disable') void task.run(() => client.mfaDisable({ expected_factor_version: current.factor_version }), async () => { setNotice('多因素认证已关闭，当前会话已更新，其他设备已退出。'); close(); await refresh(); });
    else void task.run(() => client.mfaRegenerate({ expected_factor_version: current.factor_version, expected_recovery_generation: current.recovery_generation }), async (result) => { setCodes(result.recovery_codes); setNotice(result.secret_unavailable ? '原操作已完成。恢复码只展示一次；如未保存，请核对状态后主动重建。' : '旧恢复码已全部失效，现有登录会话保持有效。'); close(); await refresh(); });
  }
  return <section aria-labelledby="mfa-title" className="space-y-4">
    <div className="flex flex-wrap items-center gap-3"><h2 id="mfa-title" className="text-lg font-medium">多因素认证</h2>{query.data && <Badge variant={query.data.state === 'enabled' ? 'secondary' : 'outline'}>{query.data.state === 'enabled' ? '本人已启用' : query.data.state === 'reset_required' ? '等待恢复' : '本人未启用'}</Badge>}</div>
    <p className="text-sm text-muted-foreground">使用验证器保护每次登录。登录后的管理操作沿用密码确认；更换验证器只需验证新配置。</p>
    {query.isPending && <LoadingState label="正在读取认证状态…" />}<FormNotice error>{query.error?.message}</FormNotice>
    {query.data && <><p className="text-sm">平台要求：{query.data.policy_required ? '所有人员必须启用' : '用户自主选择'}</p>{query.data.state === 'enabled' && <p className="text-sm text-muted-foreground">剩余恢复码：{query.data.recovery_codes_remaining} / 10</p>}
      {!codes && <div className="flex flex-wrap gap-2">{query.data.state === 'disabled' ? <Button onClick={() => setMode('enable')}>启用多因素认证</Button> : <>
        <Button variant="outline" onClick={() => setMode('replace')}>更换验证器</Button><Button variant="outline" onClick={() => setMode('regenerate')}>重建恢复码</Button><Button variant="outline" disabled={!query.data.can_disable} onClick={() => setMode('disable')}>关闭 MFA</Button>
      </>}</div>}{query.data.policy_required && <p className="text-sm text-muted-foreground">平台强制已开启，当前不能关闭个人 MFA。</p>}</>}
    <FormNotice>{notice}</FormNotice>{codes && <RecoveryCodes codes={codes} onSaved={() => setCodes(undefined)} />}
    <Button variant="ghost" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新认证状态</Button>
    <Dialog open={!!mode} onOpenChange={(open) => { if (open || task.pending) return; if (setup) void task.run(() => client.mfaEnrollmentCancel(setup.enrollment_id), close); else close(); }}><DialogContent className="max-h-[90dvh] overflow-y-auto"><DialogHeader><DialogTitle>{mode ? labels[mode] : '多因素认证'}</DialogTitle><DialogDescription>{mode === 'disable' ? '关闭后新登录只验证密码，其他设备会退出。' : mode === 'regenerate' ? '旧恢复码将立即失效，新恢复码只展示一次。' : '确认新验证器后，原验证器、恢复码和其他设备会话将失效。取消或输入错误不会改变原配置。'}</DialogDescription></DialogHeader>
      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); submit(); }}>
        {setup && <EnrollmentFields setup={setup} code={code} onCode={setCode} />}<FormNotice error>{task.error?.message}</FormNotice>
        <Button disabled={task.pending || reauth}>{task.pending ? '正在处理…' : setup ? '确认新验证器' : mode === 'enable' || mode === 'replace' ? '开始设置' : '确认操作'}</Button>
      </form>{reauth && <PasswordConfirmation onConfirmed={() => { task.clearError(); setNotice('密码已确认，请再次提交原操作。'); }} />}
    </DialogContent></Dialog>
  </section>;
}
