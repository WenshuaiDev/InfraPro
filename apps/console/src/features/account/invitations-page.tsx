import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { ArrowDown, Ban, Info, Link2, LockKeyhole, MoreHorizontal, Plus, RefreshCw, RotateCcw, Search, X } from 'lucide-react';
import { Permissions as P, type Invitation } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { CreateInvitationDialog, InvitationDetail, RevokeInvitationDialog } from './invitation-dialogs';
import { invitationDate, invitationStatuses, invitationTitle } from './invitation-utils';
import { InvitationEmpty, InvitationExpiry, InvitationRules, InvitationStatus } from './invitation-ui';

type Status = Invitation['status'] | 'all';
type Modal = { kind: 'create'; original?: Invitation } | { kind: 'detail'; id: string } | { kind: 'revoke'; item: Invitation };
const tabClass = 'h-11 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none dark:data-[state=active]:bg-transparent';

export function InvitationsPage() {
  const { account } = useIdentity();
  return <InvitationsContent key={account?.id} />;
}

function InvitationsContent() {
  const { client, account, phase, can, canAll } = useIdentity();
  const cache = useQueryClient();
  const [status, setStatus] = useState<Status>('all');
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  const [modal, setModal] = useState<Modal>();
  const createButton = useRef<HTMLButtonElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  useEffect(() => { const timer = setTimeout(() => setQuery(search.trim()), 250); return () => clearTimeout(timer); }, [search]);
  const queryKey = ['account', account?.id, 'invitations'];
  const list = useInfiniteQuery({ queryKey: [...queryKey, 'list', status, query], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.invitations({ cursor: pageParam, q: query, status: status === 'all' ? '' : status }, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: phase === 'authenticated' && can(P.PlatformInvitationRead), refetchInterval: 30_000 });
  const rows = list.data?.pages.flatMap((page) => page.items) ?? [];
  const summary = list.data?.pages[0];
  const filtered = status !== 'all' || !!search.trim();
  const firstUse = summary?.counts.all === 0 && !filtered && !list.error;
  const createAllowed = can(P.PlatformInvitationCreate);
  const regenerateAllowed = canAll(P.PlatformInvitationCreate, P.PlatformInvitationRegenerate);
  const revokeAllowed = can(P.PlatformInvitationRevoke);
  function refresh() { void cache.invalidateQueries({ queryKey }); }
  function reset() { setSearch(''); setQuery(''); setStatus('all'); }
  function openCreate() { setModal({ kind: 'create' }); }
  function restoreFocus(event: Event) { event.preventDefault(); (returnFocus.current?.isConnected ? returnFocus.current : createButton.current)?.focus(); }
  function rememberFocus(element: HTMLElement) { returnFocus.current = element; }
  const columns: ColumnDef<Invitation>[] = [
    { accessorKey: 'note', size: 300, header: '用途备注', cell: ({ row: { original: item } }) => <div className="flex items-center gap-3"><span className="flex size-8 shrink-0 items-center justify-center rounded-md border text-muted-foreground"><Link2 className="size-4" aria-hidden="true" /></span><div className="min-w-0"><Button variant="link" className="h-auto min-h-8 max-w-full justify-start whitespace-normal break-words px-0 text-left font-medium" onFocus={(e) => rememberFocus(e.currentTarget)} onClick={() => setModal({ kind: 'detail', id: item.id })}>{invitationTitle(item)}</Button><p className="text-xs text-muted-foreground">{item.id.slice(0, 8)}</p></div></div> },
    { accessorKey: 'status', size: 126, header: '状态', cell: ({ row }) => <InvitationStatus status={row.original.status} /> },
    { accessorKey: 'created_at', size: 184, header: () => <span className="inline-flex items-center gap-1">创建时间<ArrowDown className="size-3" aria-hidden="true" /></span>, cell: ({ row }) => <time className="text-muted-foreground tabular-nums" dateTime={row.original.created_at}>{invitationDate(row.original.created_at)}</time> },
    { accessorKey: 'expires_at', size: 184, header: '有效期至', cell: ({ row }) => <InvitationExpiry item={row.original} checkedAt={list.dataUpdatedAt} /> },
    { id: 'actions', size: 64, header: '操作', cell: ({ row: { original: item } }) => <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`${invitationTitle(item)}的操作`} onFocus={(e) => rememberFocus(e.currentTarget)}><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-44" onCloseAutoFocus={(e) => { if (modal) e.preventDefault(); }}><DropdownMenuLabel>邀请操作</DropdownMenuLabel><DropdownMenuItem onSelect={() => setModal({ kind: 'detail', id: item.id })}><Info />查看详情</DropdownMenuItem>{item.status !== 'used' && regenerateAllowed && <DropdownMenuItem onSelect={() => setModal({ kind: 'create', original: item })}><RotateCcw />重新生成链接</DropdownMenuItem>}{item.status === 'pending' && revokeAllowed && <><DropdownMenuSeparator /><DropdownMenuItem variant="destructive" onSelect={() => setModal({ kind: 'revoke', item })}><Ban />撤销邀请</DropdownMenuItem></>}</DropdownMenuContent></DropdownMenu> },
  ];
  const controls = <div className="mb-4 flex flex-wrap items-center justify-between gap-3"><div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input aria-label="搜索用途备注" placeholder="搜索用途备注…" value={search} onChange={(e) => setSearch(e.target.value)} maxLength={400} className="pl-9 pr-10" />{search && <Button variant="ghost" size="icon" className="absolute right-0 top-0" aria-label="清空搜索" onClick={() => { setSearch(''); setQuery(''); }}><X className="size-3.5" /></Button>}</div><Button variant="outline" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin' : undefined} />刷新列表</Button></div>;
  const content = <>{controls}{list.isPending ? <LoadingState label="正在读取邀请…" /> : list.error && !rows.length ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !rows.length ? <InvitationEmpty filtered reset={reset} /> : <DataTable label="我的邀请" data={rows} columns={columns} getRowId={(row) => row.id} error={list.error?.message} onRetry={() => void list.refetch()} hasMore={list.hasNextPage} loadingMore={list.isFetchingNextPage} onLoadMore={() => void list.fetchNextPage()} loadMoreLabel="加载更多邀请" />}{summary && <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground"><span role="status">已显示 {rows.length} 条，共 {summary.total} 条{filtered ? '符合条件的邀请' : '邀请'}</span><span className="flex items-center gap-1.5"><LockKeyhole className="size-3" aria-hidden="true" />仅显示你发出的邀请 · 时间为北京时间</span></div>}</>;
  return <><PageHeader title="账号开户邀请" description="邀请同事创建账号，并管理你发出的邀请。" actions={createAllowed ? <Button ref={createButton} onFocus={(e) => rememberFocus(e.currentTarget)} onClick={openCreate}><Plus />创建邀请</Button> : undefined} />
    {firstUse ? <><InvitationEmpty create={createAllowed ? openCreate : undefined} /><div className="mt-5"><InvitationRules compact /></div></> : <><div className="mb-5"><InvitationRules compact /></div><Tabs value={status} onValueChange={(value) => setStatus(value as Status)} className="gap-0"><div className="mb-5 overflow-x-auto border-b pb-px"><TabsList aria-label="邀请状态" className="h-auto w-max gap-5 rounded-none bg-transparent p-0"><TabsTrigger value="all" className={tabClass}>全部邀请<span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{summary?.counts.all ?? '—'}</span></TabsTrigger>{Object.entries(invitationStatuses).map(([key, label]) => <TabsTrigger key={key} value={key} className={tabClass}>{label}<span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{summary?.counts[key as Invitation['status']] ?? '—'}</span></TabsTrigger>)}</TabsList></div>{(['all', ...Object.keys(invitationStatuses)] as Status[]).map((value) => <TabsContent key={value} value={value} aria-busy={list.isFetching || search.trim() !== query}>{value === status && content}</TabsContent>)}</Tabs></>}
    {modal?.kind === 'create' && (modal.original ? regenerateAllowed : createAllowed) && <CreateInvitationDialog original={modal.original} close={() => setModal(undefined)} refresh={refresh} restoreFocus={restoreFocus} onGenerated={reset} />}
    {modal?.kind === 'revoke' && revokeAllowed && <RevokeInvitationDialog item={modal.item} close={() => setModal(undefined)} refresh={refresh} restoreFocus={restoreFocus} />}
    {modal?.kind === 'detail' && <InvitationDetail id={modal.id} close={() => setModal(undefined)} restoreFocus={restoreFocus} regenerate={(item) => setModal({ kind: 'create', original: item })} revoke={(item) => setModal({ kind: 'revoke', item })} />}
  </>;
}
