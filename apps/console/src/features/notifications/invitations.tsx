import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { useNotifications } from './provider';
import { RefreshError } from './common';
import { time } from './labels';
const states: Record<string, string> = { pending: '等待接受', accepted: '已接受', expired: '已过期', revoked: '已撤销', replaced: '已替换', invalidated: '已失效' };
export function MyTenantInvitations() {
 const { client } = useIdentity(), n = useNotifications();
 const [cursor, setCursor] = useState<string>();
 const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'invitations', cursor], queryFn: ({ signal }) => client.myTenantInvitations({ cursor }, signal), refetchInterval: 5_000, refetchIntervalInBackground: false });
 return <section className="mx-auto max-w-3xl space-y-5"><Button asChild variant="outline" className="min-h-11"><Link to="/notifications" search={{}}>返回通知中心</Link></Button><PageHeader title="收到的租户邀请" description="仅显示发给当前账号的邀请。加入租户需要你明确接受。" /><RefreshError error={query.error} retry={() => { void query.refetch(); }} />{query.isPending && <p role="status">正在加载邀请…</p>}{query.data && !query.data.items.length && <p className="rounded-lg border border-dashed p-6">暂无收到的邀请</p>}<ul className="space-y-3">{query.data?.items.map(v => <li key={v.invitation_id}><Link to="/notifications/invitations/$invitationId" params={{ invitationId: v.invitation_id }} className="block min-h-16 rounded-lg border p-4 hover:bg-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"><p className="break-words font-medium">{v.preview?.tenant.name ?? '租户邀请'} · {states[v.status] ?? '状态已更新'}</p><p className="mt-1 text-sm text-muted-foreground">有效期至 {time(v.expires_at)}</p></Link></li>)}</ul><div className="flex gap-3">{cursor && <Button variant="outline" onClick={() => setCursor(undefined)}>第一页</Button>}{query.data?.next_cursor && <Button variant="outline" onClick={() => setCursor(query.data!.next_cursor)}>下一页</Button>}</div></section>;
}
export function MyTenantInvitation({ id }: { id: string }) {
 const { client, reload } = useIdentity(), n = useNotifications();
 const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'invitation', id], queryFn: ({ signal }) => client.myTenantInvitation(id, signal), retry: false });
 const accept = useMutation({ mutationFn: () => { if (!query.data?.can_accept) throw Error('邀请当前不可接受，请重新查询。'); return client.acceptMyTenantInvitation(id, query.data.version); }, onSuccess: async () => { await Promise.all([query.refetch(), n.refresh(), reload()]); }, onError: () => { void query.refetch(); } });
 const v = query.data;
 return <section className="mx-auto max-w-3xl space-y-5"><Button asChild variant="outline" className="min-h-11"><Link to="/notifications/invitations">返回收到的邀请</Link></Button><PageHeader title="租户邀请" description="阅读通知不会改变成员关系。" /><RefreshError error={query.error ?? accept.error} retry={() => { void query.refetch(); }} />{query.isPending && <p role="status">正在核验邀请…</p>}{v && <><p className="font-medium">{states[v.status] ?? '状态已更新'}</p>{v.preview && <dl className="space-y-3 rounded-lg border p-4"><div><dt className="text-sm text-muted-foreground">租户</dt><dd className="break-words">{v.preview.tenant.name}</dd></div><div><dt className="text-sm text-muted-foreground">邀请人</dt><dd>{v.preview.inviter.display_name}</dd></div><div><dt className="text-sm text-muted-foreground">加入角色</dt><dd>{v.preview.role}</dd></div></dl>}<p className="text-sm">有效期至 {time(v.expires_at)}</p>{v.blocked_reason && <p role="status">租户目前不可用，暂时无法加入。</p>}{v.can_accept && <Button className="min-h-11" disabled={accept.isPending} onClick={() => accept.mutate()}>{accept.isPending ? '正在确认…' : '接受邀请并加入租户'}</Button>}{v.status === 'accepted' && <p role="status">已接受邀请。可在工作空间切换器查看当前可进入的租户。</p>}</>}</section>;
}
