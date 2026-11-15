import { Link } from '@tanstack/react-router';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationPreferences } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { useNotifications } from './provider';
import { RefreshError } from './common';
export function NotificationPreferencesPage() {
 const { client } = useIdentity(), n = useNotifications();
 const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'preferences'], queryFn: ({ signal }) => client.notificationPreferences(signal) });
 const save = useMutation({ mutationFn: (value: NotificationPreferences) => client.saveNotificationPreferences(value), onSuccess: n.refresh, onError: () => { void query.refetch(); } });
 return <section className="mx-auto max-w-3xl space-y-5"><Button asChild variant="outline" className="min-h-11"><Link to="/notifications" search={{}}>返回通知中心</Link></Button><PageHeader title="通知接收设置" description="设置随账号保存，在所有设备生效。" /><p className="rounded-lg border p-4">账号安全、本人资格变化和直接发给你的租户邀请始终接收。</p><RefreshError error={query.error ?? save.error} retry={() => { void query.refetch(); }} />{query.data && <fieldset disabled={save.isPending} className="space-y-3">{(['tasks', 'updates'] as const).map(key => <label key={key} className="flex min-h-14 cursor-pointer items-center gap-3 rounded-lg border p-4"><input type="checkbox" checked={save.isPending ? (save.variables?.[key] ?? query.data![key]) : query.data![key]} onChange={e => save.mutate({ ...query.data!, [key]: e.target.checked })} className="size-5 accent-primary" /><span>{key === 'tasks' ? '任务结果' : '业务更新'}</span></label>)}</fieldset>}<p className="text-sm text-muted-foreground">关闭后保留历史通知，重新开启不会补发关闭期间的消息。通知不会自动删除，可通过归档整理。</p>{save.isPending && <p role="status">正在保存…</p>}</section>;
}
