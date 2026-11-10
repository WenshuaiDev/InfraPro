import { useEffect, useRef, useState, type ReactElement } from 'react';
import { useBlocker, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError, Permissions as P, type Tenant, type TenantAccount, type TenantResult } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { toast } from 'sonner';
import { useOptionalTenant } from './context';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { z } from '../../config/validation';
import { useOperation } from '../account/use-operation';
import { setFormError } from '../auth/forms';
import { CandidatePicker } from './shared';

const fields = z.object({
  name: z.string().transform((v) => v.normalize('NFKC').trim()).refine((v) => Array.from(v).length >= 1 && Array.from(v).length <= 100, '名称需为 1–100 个字符。'),
  code: z.string().trim().toLowerCase().regex(/^[a-z0-9][a-z0-9_-]{2,31}$/, '编号需为 3–32 位字母、数字、短横线或下划线，首位为字母或数字。'),
  description: z.string().refine((v) => Array.from(v).length <= 1000, '说明最多 1000 个字符。'),
  reason: z.string().trim().min(1, '请填写变更原因。').refine((v) => Array.from(v).length <= 500, '原因最多 500 个字符。'),
});
// getRandomValues also works on the supported ordinary HTTP deployment.
function requestUUID() { const b = crypto.getRandomValues(new Uint8Array(16)); b[6] = (b[6]! & 15) | 64; b[8] = (b[8]! & 63) | 128; const h = Array.from(b, (v) => v.toString(16).padStart(2, '0')).join(''); return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`; }
type Mode = 'create' | 'edit' | 'transfer';
const titles = { create: '创建租户', edit: '编辑租户资料', transfer: '交接管理员' };
export function TenantEditor({ mode, tenant, platform = true, trigger }: { mode: Mode; tenant?: Tenant; platform?: boolean; trigger: ReactElement }) {
  const [open, setOpen] = useState(false); const [notice, setNotice] = useState(''); const canCloseRef = useRef(() => true);
  return <><Dialog open={open} onOpenChange={(v) => { if (!v && !canCloseRef.current()) return; setOpen(v); }}><DialogTrigger asChild>{trigger}</DialogTrigger><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{titles[mode]}</DialogTitle><DialogDescription>{mode === 'transfer' ? '原管理员将失去本租户访问与管理能力；其账号及其他租户权限不受影响。' : '名称、编号和说明将保存为租户档案；编号修改后永久保留原归属。'}</DialogDescription></DialogHeader>
    {open && <Editor mode={mode} tenant={tenant} platform={platform} canCloseRef={canCloseRef} done={(message) => { setOpen(false); setNotice(message); }} />}
  </DialogContent></Dialog>{notice && <p className="text-sm" role="status">{notice}</p>}</>;
}
function Editor({ mode, tenant, platform, done, canCloseRef }: { mode: Mode; tenant?: Tenant; platform: boolean; done: (message: string) => void; canCloseRef: React.RefObject<() => boolean> }) {
  const { client, account, can, phase } = useIdentity(); const tenantContext = useOptionalTenant(); const available = phase === 'authenticated' && (platform || tenantContext?.available === true); const cache = useQueryClient(); const navigate = useNavigate();
  const form = useForm({ resolver: zodResolver(fields), defaultValues: { name: tenant?.name ?? '', code: tenant?.code ?? '', description: tenant?.description ?? '', reason: '' } });
  const [candidate, setCandidate] = useState<TenantAccount>(); const [requestId, setRequestId] = useState(() => requestUUID()); const [currentVersion, setCurrentVersion] = useState(tenant?.version ?? '');
  const [latest, setLatest] = useState<Tenant>(); const [password, setPassword] = useState(''); const [confirmed, setConfirmed] = useState(false); const completed = useRef(false);
  const contextStamp = [account?.id, tenant?.id, tenantContext?.membership_id, tenantContext?.epoch].join(':');
  const change = useOperation({ mutationFn: async () => {
    if (!available) throw new Error('租户状态暂不可确认，请等待服务恢复；输入已保留。');
    if (mode !== 'transfer' && !await form.trigger()) throw new Error('请检查表单中标记的字段。');
    const values = form.getValues(); const reason = values.reason.trim(); if (!reason || Array.from(reason).length > 500) { form.setError('reason', { message: '请填写 1–500 个字符的变更原因。' }); throw new Error('请填写有效的变更原因。'); }
    if (mode !== 'edit' && !candidate) throw new Error('请先选择接任账号。');
    // Keep the successful result tied to the exact submitted intent, not later form state.
    const submitted = { ...values, administrator: candidate ? { ...candidate } : undefined };
    let result: TenantResult;
    if (mode === 'create') result = await client.createTenant({ ...values, reason, administrator_account_id: candidate!.id, client_request_id: requestId });
    else {
      if (!tenant) throw new Error('租户信息不可用。');
      result = mode === 'transfer'
        ? await client.transferTenant(tenant.id, { expected_version: currentVersion, new_administrator_account_id: candidate!.id, reason })
        : await client.updateTenant(tenant.id, platform, { ...values, reason, expected_version: currentVersion });
    }
    return { result, submitted, stamp: contextStamp };
  }, onSuccess: ({ result, submitted, stamp }) => {
    if (stamp !== contextStamp) return;
    completed.current = true; form.reset();
    for (const prefix of [['platform', account?.id], ['tenant', account?.id], ['account', account?.id, 'my-tenants'], ['access']]) void cache.invalidateQueries({ queryKey: prefix });
    toast.success(mode === 'create' ? '租户已创建。' : '变更已保存。');
    done(mode === 'create' ? `租户已创建${result.replayed ? '（已确认此前提交结果）' : ''}：${submitted.name.normalize('NFKC').trim()}（${submitted.code.trim().toLowerCase()}）；管理员：${submitted.administrator?.display_name}（${submitted.administrator?.username}）；租户 ID：${result.tenant_id}。` : '变更已保存。');
    if (mode === 'create' && can(P.PlatformTenantRead)) void navigate({ to: '/platform/tenants/$tenantId', params: { tenantId: result.tenant_id } });
  } });
  const dirty = form.formState.isDirty || !!candidate; const pending = change.isPending;
  const leaveText = pending ? '操作正在处理中，离开不会撤销服务端写入。请稍后回到该租户核实结果。仍要离开吗？' : '有尚未保存的输入，放弃后离开吗？';
  useEffect(() => { canCloseRef.current = () => completed.current || !(dirty || pending) || window.confirm(leaveText); }, [canCloseRef, dirty, pending, leaveText]);
  const blocker = useBlocker({ shouldBlockFn: () => !completed.current && (dirty || pending), withResolver: true, enableBeforeUnload: dirty || pending });
  const reauth = change.error instanceof ApiError && change.error.code === 'reauthentication_required';
  const conflict = change.error instanceof ApiError && change.error.code === 'version_conflict';
  const unknown = change.error instanceof ApiError && !change.error.status;
  const confirm = useOperation({ mutationFn: () => client.reauthenticate(password), onSuccess: () => { setPassword(''); setConfirmed(true); change.reset(); } });
  const refresh = useOperation({ mutationFn: async () => { if (!tenant) return; const v = await client.tenant(tenant.id, platform); setLatest(v); setCurrentVersion(v.version); change.reset(); } });
  return <><Form {...form}><form className="space-y-4" onSubmit={(e) => { e.preventDefault(); void change.mutateAsync().catch((error: unknown) => setFormError(form, error)); }}>
    {!available && <FormNotice error>租户状态暂不可确认，输入已保留，恢复后可继续提交。</FormNotice>}
    <fieldset disabled={pending} className="space-y-4">{mode !== 'transfer' && <><FormInput control={form.control} name="name" label="租户名称" /><FormInput control={form.control} name="code" label="租户编号" hint="3–32 位，大小写视为同一编号。" /><FormInput control={form.control} name="description" label="租户说明" /></>}
    {mode === 'transfer' && tenant && <p className="rounded-md bg-muted p-3 text-sm">当前管理员：{tenant.administrator.display_name}（{tenant.administrator.username}）</p>}
    {mode !== 'edit' && <CandidatePicker value={candidate} onChange={setCandidate} />}
    <FormInput control={form.control} name="reason" label="变更原因" hint="本次操作及原因将记入审计。" />
    </fieldset><FormNotice error>{change.error?.message ?? refresh.error?.message}</FormNotice>{confirmed && <FormNotice>密码已确认，请核对表单后再次提交。</FormNotice>}
    {unknown && <FormNotice>提交结果尚未确认。{mode === 'create' ? '再次提交将沿用原请求编号，请保持内容不变；也可在平台列表查验。' : '请先加载最新内容，核实此前操作是否生效，再确认提交。'}</FormNotice>}
    {(conflict || unknown) && mode !== 'create' && <Button type="button" variant="outline" disabled={refresh.isPending} onClick={() => refresh.mutate()}>加载最新内容并比较</Button>}
    {latest && <section className="space-y-2 rounded-md border p-3 text-sm"><h3 className="font-medium">服务器最新内容（本地输入已保留）</h3><p>{latest.name} · {latest.code}</p><p>{latest.description || '暂无说明'}</p><p>当前管理员：{latest.administrator.display_name}（{latest.administrator.username}）</p><p>请比较后再次确认提交。</p></section>}
    {mode === 'create' && change.error instanceof ApiError && change.error.code === 'idempotency_conflict' && <Button type="button" variant="outline" onClick={() => { setRequestId(requestUUID()); change.reset(); }}>使用新请求编号重新创建</Button>}
    <Button disabled={!available || pending || reauth || conflict || (unknown && mode !== 'create')}>{pending ? '正在提交，请稍候…' : '确认提交'}</Button>
  </form></Form>
    {reauth && <form className="mt-4 space-y-3 rounded-lg border p-4" onSubmit={(e) => { e.preventDefault(); confirm.mutate(); }}><label className="block text-sm font-medium" htmlFor="tenant-password">当前密码</label><input id="tenant-password" className="h-11 w-full rounded-md border bg-background px-3" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /><FormNotice error>{confirm.error?.message}</FormNotice><Button disabled={confirm.isPending}>确认密码</Button></form>}
    <Dialog open={blocker.status === 'blocked'} onOpenChange={(v) => { if (!v && blocker.status === 'blocked') blocker.reset(); }}><DialogContent><DialogHeader><DialogTitle>{pending ? '操作仍在处理中' : '放弃未保存的输入？'}</DialogTitle><DialogDescription>{leaveText}</DialogDescription></DialogHeader><div className="flex flex-wrap gap-3"><Button onClick={() => { if (blocker.status === 'blocked') blocker.reset(); }}>继续编辑</Button><Button variant="outline" onClick={() => { if (blocker.status === 'blocked') blocker.proceed(); }}>放弃后离开</Button></div></DialogContent></Dialog>
  </>;
}
