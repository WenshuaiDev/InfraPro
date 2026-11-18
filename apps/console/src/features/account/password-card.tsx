import { useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { KeyRound } from 'lucide-react';
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { passwordSchema, setFormError } from '../auth/forms';
import { useOperation } from './use-operation';

export function PasswordCard() {
  const { client, help, signedOut } = useIdentity();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const min = help?.password_min ?? 8; const max = help?.password_max ?? 128;
  const schema = z.object({ current_password: z.string().min(1, '请输入当前密码。'), new_password: passwordSchema(min, max), confirm: z.string() }).refine((values) => values.new_password === values.confirm, { path: ['confirm'], message: '两次输入的密码不一致。' });
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { current_password: '', new_password: '', confirm: '' } });
  const password = useWatch({ control: form.control, name: 'new_password' });
  const save = useOperation({ mutationFn: (values: z.infer<typeof schema>) => client.changePassword({ current_password: values.current_password, new_password: values.new_password }), onSuccess: () => { form.reset(); signedOut(); void navigate({ to: '/login', search: { notice: 'password' }, replace: true }); } });
  return <section className="account-surface flex flex-col items-start gap-3" aria-labelledby="password-card-title">
    <KeyRound aria-hidden="true" className="size-6 text-muted-foreground" /><h2 id="password-card-title" className="font-semibold">登录密码</h2>
    <p className="text-sm text-muted-foreground">更改后，所有设备都需要重新登录。</p>
    <Dialog open={open} onOpenChange={value => { if (save.isPending) return; setOpen(value); if (!value) { form.reset(); save.reset(); } }}>
      <DialogTrigger asChild><Button variant="outline" className="mt-auto">修改密码</Button></DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto" showCloseButton={!save.isPending}><DialogHeader>
        <DialogTitle>修改密码</DialogTitle><DialogDescription>提交后将退出所有登录会话。已发出的有效邀请可以继续使用。</DialogDescription>
      </DialogHeader>
        <Form {...form}><form className="space-y-5" noValidate aria-busy={save.isPending} onSubmit={form.handleSubmit(async (values) => { try { await save.mutateAsync(values); } catch (error) { setFormError(form, error); } })}>
          <FormInput control={form.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" />
          <FormInput control={form.control} name="new_password" label="新密码" type="password" autoComplete="new-password" hint={`${min}–${max} 个字符，允许空格和中文。当前 ${Array.from(password).length} 个字符。`} />
          <FormInput control={form.control} name="confirm" label="确认新密码" type="password" autoComplete="new-password" />
          <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><Button disabled={save.isPending}>{save.isPending ? '正在修改…' : '修改密码并退出'}</Button>
        </form></Form>
      </DialogContent>
    </Dialog>
  </section>;
}
