import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { RefreshCw, Search, UserPlus, X } from 'lucide-react';
import { Permissions as P, type TenantMember } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, type DirectoryColumn } from '@infrapro/ui/patterns/directory-table';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useTenant } from './context';
import { MemberActions, MemberMenu } from './member-actions';
import { useDirectoryAction } from './directory-action';
import { TenantMemberDetails } from './member-details';
import { MemberRoles, MemberStatus, MembershipIdentity } from './membership-ui';
import { membershipDate } from './membership-utils';

const statuses = [['current', '当前成员'], ['active', '有效'], ['suspended', '已暂停'], ['ended', '历史成员'], ['all', '全部关系']] as const;

export function TenantMembersPage({ membershipId }: { membershipId?: string } = {}) {
  const ctx = useTenant(); const { client, account } = useIdentity(); const navigate = useNavigate();
  const [q, setQ] = useState(''), [status, setStatus] = useState('current'), [role, setRole] = useState('');
  const [pageSize, setPageSize] = useState(10), [selection, setSelection] = useState<string>();
  const selected = selection ?? membershipId;
  const returnFocus = useRef<HTMLElement | null>(null);
  const allowed = ctx.permissions.includes(P.TenantMemberRead);
  const list = useCursorList({ pageSize, queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'members', q, status, role, pageSize], load: (cursor, signal) => client.tenantMembers(ctx.tenant.id, { q, status: status === 'current' ? '' : status, role, cursor, limit: pageSize }, signal), enabled: allowed, paused: !ctx.available, staleTime: 0, refetchInterval: 30_000 });
  const action = useDirectoryAction(list.items, ctx.available, list);
  const open = (id: string, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelection(id); };
  const close = () => { setSelection(undefined); if (membershipId) void navigate({ to: '/t/$tenantId/members', params: { tenantId: ctx.tenant.id } }); };
  const columns: DirectoryColumn<TenantMember>[] = [
    { id: 'account', label: '成员', cell: (item) => <MembershipIdentity avatar={item.account} target={{ kind: 'tenant', tenant: ctx.tenant.id, id: item.id }} name={item.account.display_name} username={item.account.username} onClick={(trigger) => open(item.id, trigger)}><p className="truncate text-xs text-muted-foreground lg:hidden">{item.roles.filter((role) => item.membership_status === 'ended' || !role.revoked_at).map((role) => role.name).join('、') || '未绑定角色'}</p></MembershipIdentity> },
    { id: 'status', label: '成员 / 账号状态', className: 'w-28 lg:w-36', cell: (item) => <MemberStatus item={item} /> },
    { id: 'roles', label: '绑定角色', secondary: true, cell: (item) => <MemberRoles item={item} onMore={(trigger) => open(item.id, trigger)} /> },
    { id: 'joined', label: '加入时间', secondary: true, className: 'lg:w-40', cell: (item) => <time className="text-sm tabular-nums text-muted-foreground" dateTime={item.joined_at} title={`${membershipDate(item.joined_at)}（北京时间）`}>{membershipDate(item.joined_at)}</time> },
    { id: 'changed', label: '状态变更时间', secondary: true, className: 'lg:w-40', cell: (item) => <time className="text-sm tabular-nums text-muted-foreground" dateTime={item.status_changed_at}>{membershipDate(item.status_changed_at)}</time> },
    { id: 'actions', label: '操作', className: 'w-14 text-right', cell: (item) => <MemberMenu item={item} select={action.select} details={(trigger) => open(item.id, trigger)} /> },
  ];
  if (!allowed) return <FormNotice error>无权查看本租户成员。</FormNotice>;
  return <>
    <PageHeader title="成员管理" description={`${ctx.tenant.name} · 管理成员状态、角色绑定与历史关系。`} actions={<><Button variant="outline" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw aria-hidden="true" className={list.isFetching ? 'animate-spin' : undefined} />刷新成员</Button>{ctx.permissions.includes(P.TenantInvitationRead) && <Button asChild><Link to="/t/$tenantId/invitations" params={{ tenantId: ctx.tenant.id }}><UserPlus aria-hidden="true" />邀请成员</Link></Button>}</>} />
    <Tabs value={status} onValueChange={setStatus} className="gap-5">
      <TabsList aria-label="成员状态" className="max-sm:grid max-sm:h-auto max-sm:w-full max-sm:grid-cols-3">{statuses.map(([value, label]) => <TabsTrigger key={value} value={value} className="max-sm:min-h-11">{label}</TabsTrigger>)}</TabsList>
      <TabsContent value={status} className="space-y-4" aria-busy={list.isFetching}>
        <DirectoryTable label="租户成员列表" items={list.items} rowId={(item) => item.id} columns={columns} loading={list.isPending} error={list.error?.message} retry={() => void list.refetch()} empty="没有符合条件的成员" toolbar={<>
          <div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-member-search aria-label="搜索本租户成员" placeholder="搜索用户名或显示名称…" className="h-9 pl-9 max-sm:h-11" maxLength={200} value={q} onChange={(event) => setQ(event.target.value)} /></div>
          <ListFilter label="角色" value={role} onChange={setRole} options={[{ value: '', label: '全部角色' }, { value: 'tenant_admin', label: '管理员' }, { value: 'tenant_member', label: '普通成员' }]} />
          {(q || status !== 'current' || role) && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={() => { setQ(''); setStatus('current'); setRole(''); }}>重置<X aria-hidden="true" /></Button>}
        </>} />
        <ListPagination label="成员列表" {...list.pagination} setPageSize={setPageSize} />
        <p className="text-xs text-muted-foreground">时间均为北京时间；历史成员保留本次关系及角色授予记录。</p>
      </TabsContent>
    </Tabs>
    {action.item && action.dialog && <MemberActions key={action.item.id + action.dialog.command} item={action.item} dialog={action.dialog} />}
    {selected && <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title="成员详情" description={`${ctx.tenant.name} · 查看本次成员关系与授权情况。`} returnFocus={returnFocus} fallbackFocusSelector="[data-member-search]"><TenantMemberDetails key={selected} id={selected} /></ResponsivePanel>}
  </>;
}

export function TenantMemberPage({ id }: { id: string }) { return <TenantMembersPage membershipId={id} />; }
