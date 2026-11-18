import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { Link } from '@tanstack/react-router';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { useNotifications } from './provider';
import { RefreshError } from './common';
import { NotificationSettingsLayout } from './settings-layout';
import { time } from './labels';

const states: Record<string, string> = { pending: '等待接受', accepted: '已接受', expired: '已过期', revoked: '已撤销', replaced: '已替换', invalidated: '已失效' };

export function MyTenantInvitations() {
  const { client } = useIdentity(), n = useNotifications();
  const query = usePagedQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'invitations'], queryFn: ({ cursor, signal }) => client.myTenantInvitations({ cursor }, signal), refetchInterval: 5_000, refetchIntervalInBackground: false });
  return <NotificationSettingsLayout title="收到的租户邀请" description="查看发给当前账号的邀请，接受后即可加入对应租户。">
    <RefreshError error={query.error} retry={() => { void query.refetch(); }} />
    {query.isPending && <p role="status">正在加载邀请…</p>}
    {query.data && !query.data.items.length && <p className="rounded-lg border border-dashed p-6 text-sm text-muted-foreground">暂无收到的邀请</p>}
    <ul className="divide-y">{query.data?.items.map(v => <li key={v.invitation_id}>
      <Link to="/notifications/invitations/$invitationId" params={{ invitationId: v.invitation_id }} className="block min-h-16 rounded-md px-2 py-3 hover:bg-accent">
        <div className="flex flex-wrap items-center justify-between gap-2"><span className="break-words font-medium">{v.preview?.tenant.name ?? '租户邀请'}</span><span className="text-sm">{states[v.status] ?? '状态已更新'}</span></div>
        <p className="mt-1 text-sm text-muted-foreground">有效期至 {time(v.expires_at)}</p>
      </Link>
    </li>)}</ul>
    {query.data && <ListPagination label="收到的租户邀请" {...query.pagination} />}
  </NotificationSettingsLayout>;
}

export function MyTenantInvitation({ id }: { id: string }) {
  const { client, reload } = useIdentity(), n = useNotifications();
  const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'invitation', id], queryFn: ({ signal }) => client.myTenantInvitation(id, signal), retry: false });
  const accept = useMutation({
    mutationFn: () => {
      if (!query.data?.can_accept) throw Error('邀请当前不可接受，请重新查询。');
      return client.acceptMyTenantInvitation(id, query.data.version);
    },
    onSuccess: async () => { await Promise.all([query.refetch(), n.refresh(), reload()]); },
    onError: () => { void query.refetch(); },
  });
  const invitation = query.error ? undefined : query.data;
  return <NotificationSettingsLayout title="租户邀请" description="确认租户和角色后，可以接受邀请。" invitation>
    <RefreshError error={query.error ?? accept.error} retry={() => { void query.refetch(); }} />
    {query.isPending && <p role="status">正在核验邀请…</p>}
    {invitation && <>
      <p className="font-medium">{states[invitation.status] ?? '状态已更新'}</p>
      {invitation.preview && <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-3 rounded-lg bg-muted/40 p-4 text-sm">
        <dt className="text-muted-foreground">租户</dt><dd>{invitation.preview.tenant.name}</dd>
        <dt className="text-muted-foreground">邀请人</dt><dd>{invitation.preview.inviter.display_name}</dd>
        <dt className="text-muted-foreground">加入角色</dt><dd>{invitation.preview.role}</dd>
      </dl>}
      <p className="text-sm">有效期至 {time(invitation.expires_at)}</p>
      {invitation.blocked_reason && <p role="status">租户目前不可用，暂时无法加入。</p>}
      {invitation.can_accept && <Button className="min-h-11" disabled={accept.isPending} onClick={() => accept.mutate()}>{accept.isPending ? '正在确认…' : '接受邀请并加入租户'}</Button>}
      {invitation.status === 'accepted' && <p role="status">已接受邀请。可在工作空间切换器查看当前可进入的租户。</p>}
    </>}
  </NotificationSettingsLayout>;
}
