import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Ban, Check, Copy, Info, Link2, MailPlus, RotateCcw } from 'lucide-react';
import { ApiError, Permissions as P, type Invitation, type LinkResult } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle, AlertDialogDescription, AlertDialogFooter, AlertDialogCancel } from '@infrapro/ui/components/alert-dialog';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@infrapro/ui/components/dialog';
import { Form } from '@infrapro/ui/components/form';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@infrapro/ui/components/sheet';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { z } from '../../config/validation';
import { setFormError } from '../auth/forms';
import { useOperation } from './use-operation';
import { invitationDate, invitationTitle } from './invitation-utils';
import { InvitationRules, InvitationStatus } from './invitation-ui';

const noteSchema = z.object({ note: z.string().refine((value) => Array.from(value).length <= 200, '用途备注不能超过 200 个字符。') });
const passwordSchema = z.object({ current_password: z.string().min(1, '请输入当前密码。') });
type DialogActions = { close: () => void; refresh: () => void; restoreFocus: (event: Event) => void };

export function CreateInvitationDialog({ original, close, refresh, restoreFocus, onGenerated }: DialogActions & { original?: Invitation; onGenerated: () => void }) {
  const { client } = useIdentity();
  const [link, setLink] = useState<LinkResult>();
  const [copyMessage, setCopyMessage] = useState('');
  const [copied, setCopied] = useState(false);
  const [stage, setStage] = useState<'form' | 'password' | 'confirmed'>('form');
  const form = useForm({ resolver: zodResolver(noteSchema), defaultValues: { note: original?.note ?? '' } });
  const password = useForm({ resolver: zodResolver(passwordSchema), defaultValues: { current_password: '' } });
  const generate = useOperation({
    mutationFn: (note: string) => original ? client.regenerateInvitation(original.id) : client.createInvitation(note),
    onSuccess: (value) => { setLink(value); setCopyMessage(''); setCopied(false); password.reset(); onGenerated(); refresh(); },
    onError: (error) => { if (error instanceof ApiError && error.code === 'reauthentication_required') { password.reset(); setStage('password'); } else setFormError(form, error); refresh(); },
  });
  const verify = useOperation({
    mutationFn: (value: string) => client.reauthenticate(value),
    onSuccess: () => { password.reset(); form.clearErrors(); generate.reset(); setStage('confirmed'); },
    onError: (error) => setFormError(password, error),
  });
  const busy = generate.isPending || verify.isPending;
  function submit() { form.clearErrors(); generate.mutate(form.getValues('note')); }
  async function copy() {
    if (!link) return;
    if (!navigator.clipboard?.writeText) { setCopyMessage('自动复制不可用，请选中上方完整链接手动复制。'); return; }
    try { await navigator.clipboard.writeText(link.url); setCopied(true); setCopyMessage('已复制邀请链接。'); }
    catch { setCopyMessage('自动复制失败，请选中上方完整链接手动复制。'); }
  }
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) close(); }}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-[490px]" showCloseButton={!busy} onCloseAutoFocus={restoreFocus} onEscapeKeyDown={(e) => { if (busy) e.preventDefault(); }} onPointerDownOutside={(e) => { if (busy || link) e.preventDefault(); }} onInteractOutside={(e) => { if (busy || link) e.preventDefault(); }}>
    <DialogHeader><DialogTitle className="flex items-center gap-2.5"><MailPlus className="size-5" aria-hidden="true" />{original ? '重新生成邀请链接' : '创建开户邀请'}</DialogTitle><DialogDescription className="leading-6">生成一条链接，邀请同事创建基础成员账号。</DialogDescription></DialogHeader>
    {link ? <section className="space-y-5" aria-labelledby="invitation-result"><div className="flex items-start gap-3"><div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"><Check className="size-5" aria-hidden="true" /></div><div><h3 id="invitation-result" className="font-semibold">邀请链接已生成</h3><p className="mt-1 text-sm text-muted-foreground">有效至 {invitationDate(link.expires_at)}（北京时间）</p></div></div>
      <div className="space-y-2"><Label htmlFor="invitation-url">可手动复制的完整链接</Label><Input id="invitation-url" value={link.url} readOnly onFocus={(e) => e.currentTarget.select()} /><Button className="w-full" onClick={() => void copy()}>{copied ? <Check /> : <Copy />}{copied ? '已复制链接' : '复制链接'}</Button></div>
      <FormNotice>{copyMessage}</FormNotice><div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm leading-6 text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200"><p className="font-medium">关闭前，请先复制链接</p><p>完整链接仅在本次生成后显示。请私下发给需要开户的同事。</p></div><p className="break-words text-sm text-muted-foreground">用途备注：{form.getValues('note') || '未填写备注'}</p><Button variant="outline" className="w-full" onClick={close}>完成</Button>
    </section> : stage === 'password' ? <section className="space-y-4" aria-labelledby="reauth-title"><div className="rounded-lg bg-muted/50 p-4"><h3 id="reauth-title" className="font-medium">确认当前密码</h3><p className="mt-1 text-sm leading-6 text-muted-foreground">生成邀请需要最近 5 分钟内验证过密码。验证后请再次提交生成请求。</p></div><Form {...password}><form className="space-y-4" noValidate aria-busy={verify.isPending} onSubmit={password.handleSubmit((values) => { password.clearErrors(); verify.mutate(values.current_password); })}><FormInput control={password.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" autoFocus disabled={busy} /><FormNotice error>{password.formState.errors.root?.server?.message}</FormNotice><div className="flex justify-end gap-2"><Button type="button" variant="outline" disabled={busy} onClick={close}>取消</Button><Button disabled={busy}>{verify.isPending ? '正在确认…' : '确认密码'}</Button></div></form></Form></section> : <Form {...form}><form className="space-y-5" noValidate aria-busy={generate.isPending} onSubmit={form.handleSubmit(submit)}>
      <FormInput control={form.control} name="note" label="用途备注" placeholder="例如：桥梁监测项目同事" readOnly={!!original} disabled={busy} hint={original ? '重新生成会沿用原备注，原记录保留，旧链接随即失效。' : '可选，最多 200 个字符。便于后续查找，不绑定收件人身份。'} /><InvitationRules />{stage === 'confirmed' && <FormNotice>密码已确认，请再次提交生成请求。</FormNotice>}<FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><div className="flex justify-end gap-2"><Button variant="outline" type="button" disabled={busy} onClick={close}>取消</Button><Button disabled={busy}><Link2 />{generate.isPending ? '正在生成…' : stage === 'confirmed' ? '重新提交生成' : '生成邀请链接'}</Button></div>
    </form></Form>}
  </DialogContent></Dialog>;
}

export function RevokeInvitationDialog({ item, close, refresh, restoreFocus }: DialogActions & { item: Invitation }) {
  const { client } = useIdentity();
  const revoke = useOperation({ mutationFn: () => client.revokeInvitation(item.id), onSuccess: () => { refresh(); close(); }, onError: () => refresh() });
  return <AlertDialog open onOpenChange={(open) => { if (!open && !revoke.isPending) close(); }}><AlertDialogContent onCloseAutoFocus={restoreFocus} onEscapeKeyDown={(e) => { if (revoke.isPending) e.preventDefault(); }}><AlertDialogHeader><AlertDialogTitle>撤销这条邀请？</AlertDialogTitle><AlertDialogDescription className="break-words leading-7">“{invitationTitle(item)}” 的链接将立即失效，对方无法继续使用该链接开户。邀请记录会保留。</AlertDialogDescription></AlertDialogHeader><FormNotice error>{revoke.error?.message}</FormNotice><AlertDialogFooter><AlertDialogCancel disabled={revoke.isPending}>保留邀请</AlertDialogCancel><Button variant="destructive" disabled={revoke.isPending} onClick={() => revoke.mutate()}>{revoke.isPending ? '正在撤销…' : '确认撤销'}</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}

export function InvitationDetail({ id, close, restoreFocus, regenerate, revoke }: Omit<DialogActions, 'refresh'> & { id: string; regenerate: (item: Invitation) => void; revoke: (item: Invitation) => void }) {
  const { client, account, can, canAll } = useIdentity();
  const detail = useQuery({ queryKey: ['account', account?.id, 'invitations', 'detail', id], queryFn: ({ signal }) => client.invitation(id, signal), refetchInterval: 30_000 });
  const item = detail.data;
  const descriptions = { pending: '等待对方完成开户', used: '这条邀请已完成开户', expired: '这条邀请已超过有效期', revoked: '这条邀请已被撤销' };
  return <Sheet open onOpenChange={(open) => { if (!open) close(); }}><SheetContent className="w-full overflow-y-auto sm:max-w-[480px]" onCloseAutoFocus={restoreFocus}><SheetHeader className="border-b px-6 pb-5 pt-7"><SheetTitle>邀请详情</SheetTitle><SheetDescription>查看这条邀请的状态与可用操作。</SheetDescription></SheetHeader><div className="px-6 pb-8 pt-2">
    {detail.isPending ? <LoadingState /> : detail.error ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : item && <><div className="flex items-center justify-between gap-3"><span className="text-xs text-muted-foreground">邀请记录</span><InvitationStatus status={item.status} /></div><h3 className="mt-5 break-words text-xl font-semibold tracking-tight">{invitationTitle(item)}</h3><p className="mt-2 text-sm text-muted-foreground">{descriptions[item.status]}</p><dl className="mt-7 grid grid-cols-[5rem_minmax(0,1fr)] gap-x-4 gap-y-4 border-y py-6 text-sm"><dt className="text-muted-foreground">邀请编号</dt><dd className="break-all text-xs leading-5">{item.id}</dd><dt className="text-muted-foreground">创建时间</dt><dd className="tabular-nums">{invitationDate(item.created_at)}</dd><dt className="text-muted-foreground">有效期至</dt><dd className="tabular-nums">{invitationDate(item.expires_at)}</dd><dt className="text-muted-foreground">开户身份</dt><dd>基础成员<span className="block text-xs leading-6 text-muted-foreground">不继承邀请人角色</span></dd><dt className="text-muted-foreground">使用次数</dt><dd>仅限 1 次</dd></dl><p className="mt-2 text-xs text-muted-foreground">时间均为北京时间</p>
      <div className="mt-6 flex gap-3 rounded-lg bg-muted/50 p-4 text-sm leading-6"><Info className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><div><p className="font-medium">{item.status === 'used' ? '邀请已使用，无需继续操作' : '完整链接仅在生成时显示'}</p><p className="mt-1 text-muted-foreground">{item.status === 'used' ? '如需邀请其他同事，请创建一条新的邀请。' : '需要再次交付时，请重新生成链接。原记录会保留，旧链接随即失效。'}</p></div></div><div className="mt-6 flex flex-wrap gap-3">{item.status !== 'used' && canAll(P.PlatformInvitationCreate, P.PlatformInvitationRegenerate) && <Button onClick={() => regenerate(item)}><RotateCcw />重新生成链接</Button>}{item.status === 'pending' && can(P.PlatformInvitationRevoke) && <Button variant="outline" className="text-destructive" onClick={() => revoke(item)}><Ban />撤销邀请</Button>}</div>
    </>}
  </div></SheetContent></Sheet>;
}
