import { useQuery } from '@tanstack/react-query';
import { Permissions as P } from '@infrapro/api-client';
import { AuthorizationView } from '../access/authorization-view';
import { toast } from 'sonner';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { AccountSettings } from '../../app/workspace-shell';
import { date, setFormError } from '../auth/forms';
import { useOperation } from './use-operation';

const schema = z.object({ display_name: z.string().refine((value) => { const length = Array.from(value.trim()).length; return length >= 1 && length <= 50; }, '请输入 1–50 个字符的显示名称。') });

export function ProfilePage() {
  const { account, client, reload, can, authorization_revision } = useIdentity();
  const profile = useQuery({ queryKey: ['account', account?.id, 'profile', authorization_revision], queryFn: ({ signal }) => client.profile(signal) });
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { display_name: account?.display_name ?? '' } });
  const save = useOperation({ mutationFn: (values: z.infer<typeof schema>) => client.updateProfile(values.display_name), onSuccess: async () => { await reload(); toast.success('个人资料已保存。'); } });
  return <><PageHeader title="个人资料" description="查看和更新你的账号资料。" /><AccountSettings>
    <section className="max-w-2xl space-y-6"><div><h2 className="text-lg font-medium">基本信息</h2><p className="mt-1 text-sm text-muted-foreground">显示名称用于在工作空间中识别你的身份。</p></div>
      <dl className="grid gap-4 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2"><div><dt className="text-muted-foreground">用户名</dt><dd className="mt-1 font-medium">{account?.username}（创建后固定）</dd></div><div><dt className="text-muted-foreground">创建时间</dt><dd className="mt-1">{account && date(account.created_at)}</dd></div></dl>
      {can(P.AccountProfileUpdate) && <Form {...form}><form className="space-y-5" noValidate aria-busy={save.isPending} onSubmit={form.handleSubmit(async (values) => { try { await save.mutateAsync(values); } catch (error) { setFormError(form, error); } })}>
        <FormInput control={form.control} name="display_name" label="显示名称" autoComplete="nickname" hint="1–50 个字符，可使用中文。" /><FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><Button type="submit" disabled={save.isPending}>{save.isPending ? '正在保存…' : '保存资料'}</Button>
      </form></Form>}
      {profile.error && <FormNotice error>{profile.error.message}</FormNotice>}
      {profile.data && <AuthorizationView value={profile.data.authorization} />}
    </section>
  </AccountSettings></>;
}
