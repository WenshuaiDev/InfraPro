import { useEffect, useState, type ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { z } from '../../config/validation';
import { useOperation } from '../account/use-operation';
import { setFormError } from '../auth/forms';

const schema = z.object({ reason: z.string().trim().min(1, '请填写变更原因。').refine((value) => Array.from(value).length <= 500, '最多 500 个字符。') });

export function RoleMutationForm({ children, submit, complete, cancel, disabled, label, onPendingChange }: {
  children: ReactNode; submit: (reason: string) => Promise<unknown>; complete: () => void; cancel: () => void;
  disabled?: boolean; label: string; onPendingChange: (pending: boolean) => void;
}) {
  const { client, reload } = useIdentity();
  const cache = useQueryClient();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { reason: '' } });
  const password = useForm({ defaultValues: { current_password: '' } });
  const [confirmed, setConfirmed] = useState(false);
  const change = useOperation({ mutationFn: submit, onSuccess: async () => { await reload(); await cache.invalidateQueries({ queryKey: ['access'] }); complete(); } });
  const confirm = useOperation({ mutationFn: (value: string) => client.reauthenticate(value), onSuccess: () => { password.reset(); setConfirmed(true); change.reset(); } });
  const reauth = change.error instanceof ApiError && change.error.code === 'reauthentication_required';
  const conflict = change.error instanceof ApiError && change.error.code === 'version_conflict';
  const blocked = change.error instanceof ApiError && ['last_platform_manager', 'role_in_use', 'permission_denied'].includes(change.error.code ?? '');
  const pending = change.isPending || confirm.isPending;
  useEffect(() => { onPendingChange(pending); return () => onPendingChange(false); }, [pending, onPendingChange]);
  return <div className="space-y-5"><Form {...form}><form className="space-y-5" onSubmit={form.handleSubmit(async ({ reason }) => { try { await change.mutateAsync(reason); } catch (error) { setFormError(form, error); } })}>
    <fieldset className="min-w-0 space-y-5" disabled={pending}>{children}<FormInput control={form.control} name="reason" label="变更原因" hint="将记录在平台审计中。" /></fieldset>
    <FormNotice error>{change.error?.message}</FormNotice>{confirmed && <FormNotice>密码已确认，请核对变更后再次提交。</FormNotice>}
    {conflict && <Button type="button" variant="outline" onClick={() => { void cache.invalidateQueries({ queryKey: ['access'] }); cancel(); }}>重新加载并比较</Button>}
    <div className="sticky -bottom-6 -mx-5 flex justify-end gap-2 border-t bg-background/95 px-5 py-4 backdrop-blur sm:-mx-6 sm:px-6"><Button type="button" variant="outline" disabled={pending} onClick={cancel}>取消</Button><Button disabled={disabled || pending || reauth || blocked || conflict}>{change.isPending ? '正在提交…' : label}</Button></div>
  </form></Form>{reauth && <Form {...password}><form className="space-y-4 rounded-lg border p-4" onSubmit={password.handleSubmit(async ({ current_password }) => { try { await confirm.mutateAsync(current_password); } catch (error) { setFormError(password, error); } })}><FormInput control={password.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" /><FormNotice error>{confirm.error?.message}</FormNotice><Button disabled={confirm.isPending}>确认密码</Button></form></Form>}</div>;
}
