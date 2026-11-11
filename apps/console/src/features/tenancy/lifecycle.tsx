import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, Permissions as P, type Tenant } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';

export function TenantStatus({ status }: { status: string }) { return <Badge variant={status === 'active' ? 'outline' : 'secondary'}>{status === 'active' ? '已启用' : '已停用'}</Badge>; }
export function TenantLifecycle({ tenant, blocked = false, retry }: { tenant: Tenant; blocked?: boolean; retry?: () => void }) {
  const { can } = useIdentity(); const [open, setOpen] = useState(false); const [intent, setIntent] = useState<'disable' | 'enable'>('disable'); const [notice, setNotice] = useState(''); const pendingRef = useRef(false); const cancel = useRef<HTMLButtonElement>(null);
  const action = tenant.status === 'active' ? 'disable' : 'enable'; const label = action === 'disable' ? '停用租户' : '恢复租户';
  return <section className="space-y-4 rounded-xl border p-5"><div className="flex flex-wrap items-center gap-3"><h2 className="text-lg font-semibold">租户状态</h2><TenantStatus status={tenant.status} /></div><dl className="space-y-3 text-sm"><div><dt className="text-muted-foreground">最近状态变更时间</dt><dd>{tenant.status_changed_at ? new Date(tenant.status_changed_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '尚无启停记录'}</dd></div>{tenant.status_change_reason && <div><dt className="text-muted-foreground">平台内部原因</dt><dd className="whitespace-pre-wrap break-words">{tenant.status_change_reason}</dd></div>}{tenant.status_notice && <div><dt className="text-muted-foreground">租户可见说明</dt><dd className="whitespace-pre-wrap break-words">{tenant.status_notice}</dd></div>}</dl>
    {can(action === 'disable' ? P.PlatformTenantDisable : P.PlatformTenantEnable) && <Dialog open={open} onOpenChange={(value) => { if (!pendingRef.current) { if (value) setIntent(action); setOpen(value); } }}><DialogTrigger asChild><Button disabled={blocked} variant={action === 'disable' ? 'destructive' : 'default'}>{label}</Button></DialogTrigger><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl" onOpenAutoFocus={(e) => { e.preventDefault(); cancel.current?.focus(); }}><DialogHeader><DialogTitle>{intent === 'disable' ? '停用租户' : '恢复租户'}</DialogTitle><DialogDescription>先核对对象与影响，再明确确认。取消不会改变租户状态。</DialogDescription></DialogHeader>{open && <StatusForm tenant={tenant} blocked={blocked} retry={retry} action={intent} pendingRef={pendingRef} done={() => { setOpen(false); setNotice(intent === 'disable' ? '租户已停用。' : '租户已恢复。'); }} />}<Button ref={cancel} variant="outline" onClick={() => { if (!pendingRef.current) setOpen(false); }}>取消</Button></DialogContent></Dialog>}{notice && <p role="status">{notice}</p>}
  </section>;
}
function StatusForm({ tenant, action, pendingRef, done, blocked, retry }: { blocked: boolean; retry?: () => void; tenant: Tenant; action: 'disable' | 'enable'; pendingRef: React.RefObject<boolean>; done: () => void }) {
  const { account, client } = useIdentity(); const cache = useQueryClient(); const [reason, setReason] = useState(''); const [notice, setNotice] = useState('该租户已被平台停用，暂时无法使用。请联系维护人员。'); const [password, setPassword] = useState(''); const [fields, setFields] = useState<Record<string, string>>({}); const [review, setReview] = useState(false);
  const preview = useQuery({ queryKey: ['platform', account?.id, 'tenant-status-preview', tenant.id, action], queryFn: ({ signal }) => client.previewTenantStatus(tenant.id, action, signal), staleTime: 0, retry: false });
  const p = preview.data;
  const mutation = useOperation({ mutationFn: async () => {
    if (blocked || !p || preview.error || !p.can_submit) throw new Error('请先成功读取并核对操作影响。');
    const errors: Record<string, string> = {};
    if (!reason.trim() || Array.from(reason.trim()).length > 500) errors.reason = '请填写 1–500 个字符的内部操作原因。';
    if (action === 'disable' && (!notice.trim() || Array.from(notice.trim()).length > 500)) errors.tenant_notice = '请填写 1–500 个字符的租户可见说明。';
    setFields(errors); if (Object.keys(errors).length) throw new Error('请检查标记的字段。');
    pendingRef.current = true;
    return client.changeTenantStatus(tenant.id, action, { expected_version: p.expected_version, preview_hash: p.preview_hash, reason: reason.trim(), ...(action === 'disable' ? { tenant_notice: notice.trim() } : {}) });
  }, onSuccess: () => { for (const key of [['platform', account?.id], ['account', account?.id, 'my-tenants'], ['tenant-availability', account?.id, tenant.id]]) void cache.invalidateQueries({ queryKey: key }); done(); }, onError: (e) => { if (e instanceof ApiError && e.fields) setFields(e.fields); }, onSettled: () => { pendingRef.current = false; } });
  const reauth = mutation.error instanceof ApiError && mutation.error.code === 'reauthentication_required';
  const mustReview = mutation.error instanceof ApiError && (!mutation.error.status || ['version_conflict', 'preview_changed', 'tenant_status_unchanged', 'tenant_restore_blocked'].includes(mutation.error.code ?? ''));
  const refresh = async () => { mutation.reset(); setReview(true); await preview.refetch(); };
  const confirm = useOperation({ mutationFn: () => client.reauthenticate(password), onSuccess: () => { setPassword(''); void refresh(); } });
  return <div className="space-y-4">{blocked && <FormNotice error>租户状态暂不可确认，输入已保留。{retry && <Button type="button" variant="outline" onClick={retry}>重新读取租户资料</Button>}</FormNotice>}{preview.isFetching && <p role="status">正在读取操作影响…</p>}<FormNotice error>{preview.error?.message}</FormNotice>{p && <section aria-label="租户启停影响" className="space-y-2 rounded-lg bg-muted/40 p-4 text-sm"><p className="break-words font-medium">{p.name} · {p.code}</p><p>{p.current_status === 'active' ? '已启用 → 已停用' : '已停用 → 已启用'} · 版本 {p.expected_version}</p><p>当前成员 {p.impact.active_members} 人，暂停成员 {p.impact.suspended_members} 人；待领取邀请 {p.impact.pending_invitations} 条。</p><p>当前可执行业务 {p.impact.current_business_members} 人；恢复后按当前资格可访问 {p.impact.eligible_members} 人。</p><p>受限原因：账号不可用 {p.impact.unavailable_accounts} 人、成员暂停 {p.impact.suspended_members} 人、无有效权限 {p.impact.no_permissions} 人（原因可能重叠）。</p><p>管理员：{p.administrator.display_name} · {p.administrator_available && p.administrator_valid ? '可用' : '不可用，请先交接管理员'}</p>{p.blocked_reasons.length > 0 && <FormNotice error>{p.blocked_reasons.includes('tenant_status_unchanged') ? '状态已变化，请关闭并核对最新档案。' : '管理员不可用，暂不能恢复；请先在档案页交接管理员。'}</FormNotice>}</section>}
    <p className="text-sm">{action === 'disable' ? '普通租户业务读写将停止；其他租户、个人账号和平台治理继续按原权限使用。' : '恢复使用原租户身份；已暂停、移除、停用或撤销的权限不会恢复。'}成员、角色和历史保留。待领取邀请继续按原到期时间计算，不延期、不自动接受。</p>
    <fieldset disabled={mutation.isPending} className="space-y-4"><div className="space-y-2"><Label htmlFor="lifecycle-reason">平台内部操作原因</Label><Input id="lifecycle-reason" value={reason} onChange={(e) => setReason(e.target.value)} aria-invalid={!!fields.reason} aria-describedby="lifecycle-reason-hint" /><p id="lifecycle-reason-hint" className="text-sm text-muted-foreground">仅平台治理及完整审计可见。</p><FormNotice error>{fields.reason}</FormNotice></div>{action === 'disable' && <div className="space-y-2"><Label htmlFor="lifecycle-notice">租户可见说明</Label><textarea id="lifecycle-notice" rows={3} className="w-full rounded-md border bg-background p-3 text-sm" value={notice} onChange={(e) => setNotice(e.target.value)} aria-invalid={!!fields.tenant_notice} /><p className="text-sm text-muted-foreground">展示给该租户成员和合法邀请目标，请勿填写内部信息。</p><FormNotice error>{fields.tenant_notice}</FormNotice></div>}</fieldset>
    <FormNotice error>{mutation.error?.message}</FormNotice>{review && <FormNotice>已重新读取影响，输入已保留，请核对上方当前数据后再次确认。</FormNotice>}{mustReview && <FormNotice>提交结果或影响需要重新核对。请查看最新状态及平台操作记录，不要自动重试。</FormNotice>}
    {(mustReview || preview.error) && <Button variant="outline" disabled={preview.isFetching} onClick={() => void refresh()}>重新预览并核对</Button>}
    {reauth && <form className="space-y-3 rounded-md border p-3" onSubmit={(e) => { e.preventDefault(); confirm.mutate(); }}><Label htmlFor="lifecycle-password">当前密码</Label><Input id="lifecycle-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /><FormNotice error>{confirm.error?.message}</FormNotice><Button disabled={confirm.isPending}>确认密码</Button></form>}
    <Button variant={action === 'disable' ? 'destructive' : 'default'} disabled={blocked || !p?.can_submit || !!preview.error || preview.isFetching || mutation.isPending || reauth || mustReview} onClick={() => mutation.mutate()}>{mutation.isPending ? '正在提交…' : action === 'disable' ? '确认停用租户' : '确认恢复租户'}</Button>
  </div>;
}
