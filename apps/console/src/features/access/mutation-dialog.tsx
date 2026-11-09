import { useState, type ReactElement, type ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { z } from '../../config/validation';
import { useOperation } from '../account/use-operation';
import { setFormError } from '../auth/forms';
const schema = z.object({ reason: z.string().trim().min(1, '请填写变更原因。').refine((v) => Array.from(v).length <= 500, '最多 500 个字符。') });
export function MutationDialog({ title, description, trigger, children, submit, onComplete, disabled }: { title: string; description: string; trigger: ReactElement; children?: ReactNode; submit: (reason: string) => Promise<unknown>; onComplete?: () => void; disabled?: boolean }) {
  const [open, setOpen] = useState(false);
  const { client, reload } = useIdentity(); const cache = useQueryClient();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { reason: '' } });
  const password = useForm({ defaultValues: { current_password: '' } });
  const [confirmed, setConfirmed] = useState(false);
  const change = useOperation({ mutationFn: submit, onSuccess: async () => { await reload(); await cache.invalidateQueries({ queryKey: ['access'] }); setOpen(false); form.reset(); onComplete?.(); } });
  const confirm = useOperation({ mutationFn: (value: string) => client.reauthenticate(value), onSuccess: () => { password.reset(); setConfirmed(true); change.reset(); } });
  const reauth = change.error instanceof ApiError && change.error.code === 'reauthentication_required';
  const conflict = change.error instanceof ApiError && change.error.code === 'version_conflict';
  const blocked = change.error instanceof ApiError && ['last_platform_manager', 'role_in_use'].includes(change.error.code ?? '');
  return <Dialog open={open} onOpenChange={(value) => { if (change.isPending || confirm.isPending) return; setOpen(value); if (!value) { password.reset(); change.reset(); confirm.reset(); setConfirmed(false); } }}><DialogTrigger asChild>{trigger}</DialogTrigger><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{title}</DialogTitle><DialogDescription>{description}</DialogDescription></DialogHeader>
    {children}
    <Form {...form}><form className="space-y-4" onSubmit={form.handleSubmit(async ({ reason }) => { try { await change.mutateAsync(reason); } catch (error) { setFormError(form, error); } })}>
      <FormInput control={form.control} name="reason" label="变更原因" hint="1–500 个字符，将记录在平台审计中。" />
      <FormNotice error>{change.error?.message}</FormNotice>{confirmed && <FormNotice>密码已确认，请检查变更后再次点击提交。</FormNotice>}
      {conflict && <Button type="button" variant="outline" onClick={() => { void cache.invalidateQueries({ queryKey: ['access'] }); setOpen(false); onComplete?.(); }}>重新加载并比较</Button>}
      <Button disabled={disabled || change.isPending || reauth || blocked || conflict}>{change.isPending ? '正在提交…' : '确认提交变更'}</Button>
    </form></Form>
    {reauth && <Form {...password}><form className="space-y-4 rounded-lg border p-4" onSubmit={password.handleSubmit(async ({ current_password }) => { try { await confirm.mutateAsync(current_password); } catch (error) { setFormError(password, error); } })}><FormInput control={password.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" /><FormNotice error>{confirm.error?.message}</FormNotice><Button disabled={confirm.isPending}>确认密码</Button></form></Form>}
  </DialogContent></Dialog>;
}
