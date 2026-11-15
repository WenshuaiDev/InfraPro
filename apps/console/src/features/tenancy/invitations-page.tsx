import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { ArrowDown, CalendarRange, Link2, LockKeyhole, RefreshCw, Search, ShieldCheck, X } from 'lucide-react';
import { Permissions as P, type TenantInvitation, type TenantInvitationCreated } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Popover, PopoverContent, PopoverTrigger } from '@infrapro/ui/components/popover';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@infrapro/ui/components/sheet';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { InvitationEmpty } from '../account/invitation-ui';
import { useTenant } from './context';
import { CreateMemberInvitation, InvitationActions } from './invitation-actions';
import { TenantInvitationDetails } from './invitation-details';
import { InvitationLink } from './membership-shared';
import { TenantInvitationStatus } from './membership-ui';
import { invitationStatus, membershipDate } from './membership-utils';

const tabClass = 'h-11 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none dark:data-[state=active]:bg-transparent';

export function TenantInvitationsPage({ invitationId }: { invitationId?: string } = {}) {
  const ctx = useTenant(); const { account, client } = useIdentity(); const cache = useQueryClient(); const navigate = useNavigate();
  const [search, setSearch] = useState(''), [q, setQ] = useState(''), [status, setStatus] = useState('all');
  const [from, setFrom] = useState(''), [to, setTo] = useState(''), [link, setLink] = useState<TenantInvitationCreated>();
  const [selection, setSelection] = useState<string>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const selected = selection ?? invitationId;
  useEffect(() => { const timer = setTimeout(() => setQ(search.trim()), 250); return () => clearTimeout(timer); }, [search]);
  const allowed = ctx.permissions.includes(P.TenantInvitationRead);
  const rangeError = from && to && from > to ? '创建开始时间不能晚于结束时间。' : '';
  const list = useInfiniteQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'invitations', q, status, from, to], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenantInvitations(ctx.tenant.id, { q, status: status === 'all' ? '' : status, from: from ? new Date(`${from}+08:00`).toISOString() : '', to: to ? new Date(`${to}+08:00`).toISOString() : '', cursor: pageParam }, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: allowed && ctx.available && !rangeError, refetchInterval: 30_000 });
  const done = useCallback((value: TenantInvitationCreated) => { setLink(value); void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id] }); }, [account?.id, cache, ctx.tenant.id, ctx.membership_id]);
  const open = useCallback((id: string, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelection(id); }, []);
  const close = () => { setSelection(undefined); if (invitationId) void navigate({ to: '/t/$tenantId/invitations', params: { tenantId: ctx.tenant.id } }); };
  const reset = () => { setSearch(''); setQ(''); setStatus('all'); setFrom(''); setTo(''); };
  // Keep cell component identities stable while list/context refreshes preserve open action forms.
  const columns = useMemo<ColumnDef<TenantInvitation>[]>(() => [
    { id: 'target', size: 300, header: '受邀账号', cell: ({ row: { original: item } }) => <div className="flex items-center gap-3"><span className="flex size-8 shrink-0 items-center justify-center rounded-md border text-muted-foreground"><Link2 className="size-4" aria-hidden="true" /></span><div className="min-w-0"><Button variant="link" className="h-auto min-h-8 max-w-full justify-start whitespace-normal break-words px-0 text-left font-medium" onClick={(event) => open(item.id, event.currentTarget)}>{item.target.display_name}</Button><p className="break-all text-xs text-muted-foreground">{item.target.username}</p></div></div> },
    { id: 'status', size: 126, header: '状态', cell: ({ row }) => <TenantInvitationStatus status={row.original.status} /> },
    { id: 'inviter', size: 160, header: '发起人', cell: ({ row }) => <div className="space-y-1"><p>{row.original.inviter.display_name}</p><p className="break-all text-xs text-muted-foreground">{row.original.inviter.username}</p></div> },
    { id: 'created', size: 184, header: () => <span className="inline-flex items-center gap-1">创建时间<ArrowDown className="size-3" aria-hidden="true" /></span>, cell: ({ row }) => <time className="text-muted-foreground tabular-nums" dateTime={row.original.created_at}>{membershipDate(row.original.created_at)}</time> },
    { id: 'expires', size: 184, header: '有效期至', cell: ({ row }) => <time className="text-muted-foreground tabular-nums" dateTime={row.original.expires_at}>{membershipDate(row.original.expires_at)}</time> },
    { id: 'actions', size: 64, header: '操作', cell: ({ row }) => <InvitationActions item={row.original} done={done} details={(trigger) => open(row.original.id, trigger)} /> },
  ], [done, open]);
  if (!allowed) return <FormNotice error>无权查看本租户成员邀请。</FormNotice>;
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const filtered = !!search.trim() || status !== 'all' || !!from || !!to;
  return <>
    <PageHeader title="成员邀请" description={`${ctx.tenant.name} · 邀请已有账号加入，并管理本租户的邀请。`} actions={ctx.permissions.includes(P.TenantInvitationCreate) ? <CreateMemberInvitation done={done} /> : undefined} />
    <p className="mb-5 flex items-start gap-2 text-sm leading-6 text-muted-foreground"><ShieldCheck className="mt-1 size-4 shrink-0" aria-hidden="true" /><span>受邀人确认后成为租户普通成员；邀请仅限本人接受，有效期为 24 小时。</span></p>
    <Tabs value={status} onValueChange={setStatus} className="gap-0">
      <div className="mb-5 overflow-x-auto border-b pb-px"><TabsList aria-label="邀请状态" className="h-auto w-max gap-5 rounded-none bg-transparent p-0"><TabsTrigger value="all" className={tabClass}>全部邀请</TabsTrigger>{Object.entries(invitationStatus).map(([key, label]) => <TabsTrigger key={key} value={key} className={tabClass}>{label}</TabsTrigger>)}</TabsList></div>
      <TabsContent value={status} aria-busy={list.isFetching || search.trim() !== q}>
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-invitation-search aria-label="搜索受邀账号" placeholder="搜索受邀用户名或显示名称…" value={search} maxLength={200} onChange={(event) => setSearch(event.target.value)} className="pl-9 pr-10" />{search && <Button variant="ghost" size="icon" className="absolute right-0 top-0" aria-label="清空搜索" onClick={() => { setSearch(''); setQ(''); }}><X className="size-3.5" aria-hidden="true" /></Button>}</div>
          <Popover><PopoverTrigger render={<Button variant="outline" className="border-dashed" />}><CalendarRange aria-hidden="true" />创建时间{(from || to) && <Badge variant="secondary">已筛选</Badge>}</PopoverTrigger><PopoverContent align="start" className="w-80 gap-4 p-4"><div className="space-y-2"><Label htmlFor="invitation-from">创建开始时间（上海）</Label><Input id="invitation-from" type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} /></div><div className="space-y-2"><Label htmlFor="invitation-to">创建结束时间（上海）</Label><Input id="invitation-to" type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} /></div><FormNotice error>{rangeError}</FormNotice><Button variant="outline" onClick={() => { setFrom(''); setTo(''); }}>清除时间</Button></PopoverContent></Popover>
          {filtered && <Button variant="ghost" onClick={reset}>重置<X aria-hidden="true" /></Button>}
          <Button className="sm:ml-auto" variant="outline" disabled={list.isFetching || !!rangeError} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin' : undefined} aria-hidden="true" />刷新列表</Button>
        </div>
        {rangeError ? <FormNotice error>{rangeError}</FormNotice> : list.isPending ? <LoadingState label="正在读取邀请…" /> : list.error && !items.length ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !items.length ? <InvitationEmpty filtered={filtered} reset={reset} /> : <DataTable label="租户邀请列表" columns={columns} data={items} getRowId={(item) => item.id} error={list.error?.message} onRetry={() => void list.refetch()} hasMore={list.hasNextPage} loadingMore={list.isFetchingNextPage} onLoadMore={() => void list.fetchNextPage()} loadMoreLabel="加载更多邀请" />}
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground"><span role="status">已显示 {items.length} 条{filtered ? '符合条件的邀请' : '邀请'}</span><span className="flex items-center gap-1.5"><LockKeyhole className="size-3" aria-hidden="true" />仅显示本租户邀请 · 时间为北京时间</span></div>
      </TabsContent>
    </Tabs>
    {selected && <Sheet open onOpenChange={(open) => { if (!open) close(); }}><SheetContent className="w-full overflow-y-auto sm:max-w-[480px]" onCloseAutoFocus={(event) => { const target = returnFocus.current?.isConnected ? returnFocus.current : document.querySelector<HTMLElement>('[data-invitation-search]'); if (target) { event.preventDefault(); target.focus(); } }}><SheetHeader className="border-b px-6 pb-5 pt-7"><SheetTitle>成员邀请详情</SheetTitle><SheetDescription>{ctx.tenant.name} · 查看邀请状态与可用操作。</SheetDescription></SheetHeader><div className="px-6 pb-8 pt-2"><TenantInvitationDetails key={selected} id={selected} /></div></SheetContent></Sheet>}
    <InvitationLink value={link} close={() => setLink(undefined)} />
  </>;
}

export function TenantInvitationPage({ id }: { id: string }) { return <TenantInvitationsPage invitationId={id} />; }
