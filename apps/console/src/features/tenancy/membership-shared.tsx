import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError, type TenantInvitationCreated } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { AlertDialog, AlertDialogTrigger, AlertDialogContent, AlertDialogHeader, AlertDialogTitle, AlertDialogDescription, AlertDialogCancel } from '@infrapro/ui/components/alert-dialog';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@infrapro/ui/components/dialog';
import { z } from '../../config/validation';
import { useOperation } from '../account/use-operation';
import { useTenant } from './context';

const reasonSchema = z.object({ reason: z.string().trim().refine((v) => Array.from(v).length >= 1 && Array.from(v).length <= 500, '请填写 1–500 个字符的操作原因。') });
export function ManagementForm({ submit, refresh, children, label = '确认操作', cancel, disabled, onPendingChange, stickyActions }: { submit: (reason: string) => Promise<void>; refresh?: () => Promise<void>; children?: ReactNode; label?: string; cancel?: ReactNode; disabled?: boolean; onPendingChange?: (pending: boolean) => void; stickyActions?: boolean }) {
  const ctx = useTenant(); const { client, phase } = useIdentity(); const [password, setPassword] = useState(''); const [notice, setNotice] = useState('');
  const form = useForm({ resolver: zodResolver(reasonSchema), defaultValues: { reason: '' } });
  const action = useOperation({ mutationFn: submit });
  const error = action.error; const reauth = error instanceof ApiError && error.code === 'reauthentication_required';
  const conflict = error instanceof ApiError && ['version_conflict', 'member_state_conflict', 'invitation_not_pending', 'idempotency_conflict', 'impact_changed', 'request_conflict'].includes(error.code ?? '');
  const unknown = error instanceof ApiError && !error.status;
  const verify = useOperation({ mutationFn: () => client.reauthenticate(password), onSuccess: () => { setPassword(''); setNotice('密码已确认，请核对内容后再次确认。'); action.reset(); } });
  const reload = useOperation({ mutationFn: refresh, onSuccess: () => { action.reset(); setNotice('已加载最新状态，请核对影响后再次确认。'); } });
  const pending = action.isPending || verify.isPending;
  useEffect(() => { onPendingChange?.(pending); return () => onPendingChange?.(false); }, [pending, onPendingChange]);
  if (error instanceof ApiError && ([404].includes(error.status ?? 0) || error.code === 'permission_denied')) return <FormNotice error>当前操作权限或目标已不可用，原表单已清理。请关闭后重新进入。</FormNotice>;
  return <div className="space-y-4"><Form {...form}><form className="space-y-4" onSubmit={form.handleSubmit(({ reason }) => action.mutate(reason))}>
    <fieldset disabled={action.isPending} className="min-w-0 space-y-4">{children}<FormInput control={form.control} name="reason" label="操作原因" hint="仅供本租户治理审计，不向受邀人展示。" /></fieldset>
    {!ctx.available && <FormNotice error>服务暂不可用，输入已保留，请重新核验后提交。</FormNotice>}
    <FormNotice error>{error?.message ?? reload.error?.message}</FormNotice><FormNotice>{notice}</FormNotice>
    {unknown && <FormNotice>结果尚未确认。请关闭后查看对应成员或邀请记录；生成链接的响应丢失时，可明确重新生成。</FormNotice>}
    {(conflict || unknown) && refresh && <Button type="button" variant="outline" disabled={reload.isPending} onClick={() => reload.mutate()}>刷新状态并重新核对</Button>}
    <div className={stickyActions ? "sticky -bottom-6 -mx-5 flex justify-end gap-2 border-t bg-background/95 px-5 py-4 backdrop-blur sm:-mx-6 sm:px-6" : "flex flex-wrap gap-3"}>{cancel}<Button type="submit" disabled={disabled || phase !== 'authenticated' || !ctx.available || action.isPending || reauth || conflict || unknown}>{action.isPending ? '正在处理…' : label}</Button></div>
  </form></Form>{reauth && <form className="space-y-3 rounded-md border p-3" onSubmit={(e) => { e.preventDefault(); verify.mutate(); }}><Label htmlFor="membership-password">当前密码</Label><Input id="membership-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /><FormNotice error>{verify.error?.message}</FormNotice><Button disabled={verify.isPending}>确认密码</Button></form>}</div>;
}
export function MembershipConfirm({ title, description, submit, refresh, children, prepare }: { title: string; description: string; submit: (reason: string) => Promise<void>; refresh: () => Promise<void>; children?: ReactNode; prepare?: () => void }) {
  const [open, setOpen] = useState(false); const ctx = useTenant();
  return <AlertDialog open={open} onOpenChange={(next) => { if (next) prepare?.(); setOpen(next); }}><AlertDialogTrigger asChild><Button variant="outline" disabled={!ctx.available}>{title}</Button></AlertDialogTrigger><AlertDialogContent className="max-h-[85dvh] overflow-y-auto"><AlertDialogHeader><AlertDialogTitle>{title}</AlertDialogTitle><AlertDialogDescription>{description}</AlertDialogDescription></AlertDialogHeader>{open && <ManagementForm submit={async (reason) => { await submit(reason); setOpen(false); }} refresh={refresh} cancel={<AlertDialogCancel>取消</AlertDialogCancel>}>{children}</ManagementForm>}</AlertDialogContent></AlertDialog>;
}
export function InvitationLink({ value, close }: { value?: TenantInvitationCreated; close: () => void }) {
  const [notice, setNotice] = useState(''); const ctx = useTenant();
  return <Dialog open={!!value} onOpenChange={(open) => { if (!open) { setNotice(''); close(); } }}><DialogContent><DialogHeader><DialogTitle>{value?.link ? '成员邀请链接已生成' : '已确认此前邀请结果'}</DialogTitle><DialogDescription>链接只在本次生成后展示。关闭后无法取回，可在邀请详情明确重新生成。</DialogDescription></DialogHeader>{value && <div className="min-w-0 space-y-4"><p className="break-words">{ctx.tenant.name} · {ctx.tenant.code}</p><p className="break-words">受邀账号：{value.invitation.target.display_name}（{value.invitation.target.username}）</p><p>租户普通成员 · 可查看本租户基本资料</p><p className="text-sm">有效至 {new Date(value.invitation.expires_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}（上海）</p>{value.link ? <><Label htmlFor="tenant-invite-link">完整邀请链接</Label><Input id="tenant-invite-link" value={value.link} readOnly onFocus={(e) => e.currentTarget.select()} /><Button variant="outline" onClick={() => { if (!navigator.clipboard?.writeText) { setNotice('请选中上方链接手动复制。'); return; } void navigator.clipboard.writeText(value.link!).then(() => setNotice('已复制邀请链接。'), () => setNotice('请选中上方链接手动复制。')); }}>复制链接</Button></> : <p>此前结果已存在，不能重取明文链接，请查看邀请记录后重新生成。</p>}<FormNotice>{notice}</FormNotice><Button onClick={close}>关闭链接</Button></div>}</DialogContent></Dialog>;
}
// Async continuations must not store one-time links after an account, member or route unmount.
// eslint-disable-next-line react-refresh/only-export-components
export function useLivePage() { const live = useRef(true); useEffect(() => { live.current = true; return () => { live.current = false; }; }, []); return live; }
