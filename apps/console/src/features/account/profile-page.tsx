import { useQuery } from '@tanstack/react-query';
import { Permissions as P } from '@infrapro/api-client';
import { toast } from 'sonner';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { setFormError } from '../auth/forms';
import { AccountCenterLayout } from './account-center-layout';
import { ProfileIdentityCard } from './avatar-editor';
import { useOperation } from './use-operation';

const schema = z.object({
  display_name: z.string().trim().refine(value => {
    const length = Array.from(value).length;
    return length >= 1 && length <= 50;
  }, '请输入 1–50 个字符的显示名称。'),
});

function EditProfile({ name }: { name: string }) {
  const { client, reload } = useIdentity();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { display_name: name } });
  const save = useOperation({
    mutationFn: (value: z.infer<typeof schema>) => client.updateProfile(value.display_name),
    onSuccess: async updated => {
      form.reset({ display_name: updated.display_name });
      await reload();
      toast.success('个人资料已保存。');
    },
  });
  return <section className="account-surface" aria-labelledby="edit-profile-title">
    <h2 id="edit-profile-title" className="mb-5 font-semibold">编辑资料</h2>
    <Form {...form}><form className="max-w-lg space-y-4" noValidate aria-busy={save.isPending} onSubmit={form.handleSubmit(async value => {
      try { await save.mutateAsync(value); } catch (error) { setFormError(form, error); }
    })}>
      <FormInput control={form.control} name="display_name" label="显示名称" readOnly={save.isPending} autoComplete="nickname" hint="1–50 个字符。此名称会显示在各工作空间中。" />
      <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice>
      <Button type="submit" disabled={save.isPending || !form.formState.isDirty}>{save.isPending ? '正在保存…' : '保存资料'}</Button>
    </form></Form>
  </section>;
}

export function ProfilePage() {
  const { client, account, can, authorization_revision } = useIdentity();
  const profile = useQuery({ queryKey: ['account', account?.id, 'profile', authorization_revision], queryFn: ({ signal }) => client.profile(signal) });
  const roles = profile.data?.authorization.roles.filter(role => role.binding_scope === 'platform');
  return <AccountCenterLayout title="个人资料">
    <div className="grid items-start gap-5 md:grid-cols-[minmax(0,.9fr)_minmax(0,1.3fr)]">
      <ProfileIdentityCard />
      <div className="grid min-w-0 gap-4">
        {account && can(P.AccountProfileUpdate) && <EditProfile key={account.id} name={account.display_name} />}
        <section className="account-surface" aria-labelledby="platform-roles-title">
          <h2 id="platform-roles-title" className="mb-3 font-semibold">平台角色</h2>
          {profile.error ? <ErrorState message={profile.error.message} onRetry={() => void profile.refetch()} /> :
            !roles ? <p role="status" className="text-sm text-muted-foreground">正在读取平台角色…</p> :
            roles.length ? <ul className="flex flex-wrap gap-2">{roles.map(role => <li key={role.id}><Badge variant="secondary" className="max-w-full whitespace-normal">{role.name}{role.status === 'disabled' && '（已停用）'}</Badge></li>)}</ul> :
            <p className="text-sm text-muted-foreground">尚未绑定平台角色</p>}
        </section>
      </div>
    </div>
  </AccountCenterLayout>;
}
