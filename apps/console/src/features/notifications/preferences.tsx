import { useQuery, useMutation } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationPreferences } from '@infrapro/api-client';
import { useNotifications } from './provider';
import { RefreshError } from './common';
import { NotificationSettingsLayout } from './settings-layout';

export function NotificationPreferencesPage() {
  const { client } = useIdentity(), n = useNotifications();
  const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'preferences'], queryFn: ({ signal }) => client.notificationPreferences(signal) });
  const save = useMutation({ mutationFn: (value: NotificationPreferences) => client.saveNotificationPreferences(value), onSuccess: n.refresh, onError: () => { void query.refetch(); } });
  const values = save.isPending ? save.variables : query.data;
  return <NotificationSettingsLayout title="通知接收设置" description="设置会同步到当前账号的所有设备。">
    <RefreshError error={query.error ?? save.error} retry={() => { save.reset(); void query.refetch(); }} />
    {query.isPending && <p role="status" className="text-sm">正在读取接收设置…</p>}
    {values && <fieldset disabled={save.isPending || !!query.error} className="divide-y rounded-lg border">
      <legend className="sr-only">可选通知类型</legend>
      {(['tasks', 'updates'] as const).map(key => <label key={key} className="flex min-h-14 cursor-pointer items-center justify-between gap-3 px-4 py-3">
        <span className="text-sm font-medium">{key === 'tasks' ? '任务结果' : '业务更新'}</span>
        <input type="checkbox" checked={values[key]} onChange={e => save.mutate({ ...values, [key]: e.target.checked })} className="size-5 accent-primary" />
      </label>)}
    </fieldset>}
    <p className="text-sm leading-6 text-muted-foreground">账号安全、本人资格变化和直接发给你的租户邀请始终接收。关闭可选通知后，历史消息会保留，关闭期间的消息不会补发。</p>
    {save.isPending && <p role="status" className="text-sm">正在保存…</p>}
  </NotificationSettingsLayout>;
}
