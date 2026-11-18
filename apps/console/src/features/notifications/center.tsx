import { useLayoutEffect, useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, CheckCheck, ChevronRight, Mail, Search, Settings2, UserPlus, X } from 'lucide-react';
import { toast } from 'sonner';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useRoutePagination } from '../../hooks/use-route-pagination';
import { AccountCenterLayout } from '../account/account-center-layout';
import { useNotifications } from './provider';
import { NotificationRows, NotificationScopeSelect, RefreshError } from './common';
import { NotificationDetail } from './detail';
import { categories } from './labels';
import { notificationHeading, notificationSource } from './presentation';

export function NotificationPage({ id, filter }: { id: string; filter: NotificationFilter }) {
  return <AccountCenterLayout title="通知中心">
    <div className="overflow-hidden rounded-2xl border bg-card">
      <NotificationDetail id={id} back={<Button asChild variant="ghost" className="min-h-11"><Link to="/notifications" search={filter} resetScroll={false}><ArrowLeft aria-hidden="true" />返回通知列表</Link></Button>} />
    </div>
  </AccountCenterLayout>;
}

function NotificationOverview({ unread, failed }: { unread?: number; failed: boolean }) {
  const shortcuts = [
    { to: '/notifications/invitations', label: '收到的租户邀请', hint: '查看邀请并加入租户', icon: UserPlus },
    { to: '/notifications/preferences', label: '通知接收设置', hint: '选择要接收的通知类型', icon: Settings2 },
  ] as const;
  return <div className="mb-5 grid gap-4 md:grid-cols-[minmax(0,.9fr)_minmax(0,1.3fr)]">
    <section className="account-cover grid grid-cols-[minmax(0,1fr)_auto] items-end gap-x-3 gap-y-2 md:flex md:flex-col md:items-start md:justify-between md:gap-3" aria-labelledby="unread-title">
      <h2 id="unread-title" className="account-cover-muted col-span-2 flex items-center gap-2 text-sm font-medium"><Mail aria-hidden="true" className="size-4" />全部工作空间的未读通知</h2>
      <p className="flex items-baseline gap-2"><strong className="text-4xl font-semibold tabular-nums">{unread ?? '—'}</strong><span className="account-cover-muted text-sm">{unread == null ? (failed ? '暂时无法读取' : '正在读取…') : failed ? '条 · 上次更新' : '条'}</span></p>
      <Link to="/notifications" search={{ status: 'unread' }} className="inline-flex min-h-10 items-center gap-2 rounded-sm text-sm font-medium hover:underline">查看未读通知<ArrowRight className="size-4" aria-hidden="true" /></Link>
    </section>
    <div className="account-surface grid grid-cols-2 content-center gap-2 md:grid-cols-1 md:gap-0 md:divide-y">
      {shortcuts.map(({ to, label, hint, icon: Icon }) => <Link key={to} to={to} aria-label={label} className="group flex min-h-11 items-center gap-2 rounded-sm md:min-h-16 md:gap-4 md:px-1 md:py-3">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-muted text-[var(--account-ink)] md:size-10"><Icon aria-hidden="true" className="size-4 md:size-5" /></span>
        <div className="min-w-0 flex-1"><span className="text-sm font-medium group-hover:underline md:text-base">{label}</span><p className="mt-1 hidden text-xs text-muted-foreground md:block">{hint}</p></div>
        <ChevronRight aria-hidden="true" className="hidden size-4 shrink-0 text-muted-foreground md:block" />
      </Link>)}
    </div>
  </div>;
}

export function NotificationCenter({ filter }: { filter: NotificationFilter }) {
  const n = useNotifications();
  return <AccountCenterLayout title="通知中心">
    <NotificationOverview unread={n.summary?.unread} failed={!!n.error} />
    <Inbox key={JSON.stringify([...n.key, filter])} filter={filter} />
  </AccountCenterLayout>;
}

function Inbox({ filter }: { filter: NotificationFilter }) {
  const { client } = useIdentity(), n = useNotifications(), navigate = useNavigate();
  const viewKey = JSON.stringify(filter);
  const [search, setSearch] = useState(() => n.inboxViews.get(viewKey)?.search ?? '');
  const list = useRef<HTMLElement>(null);
  const remember = (value: { search: string; scrollTop: number; focusId?: string }) => {
    if (!n.inboxViews.has(viewKey) && n.inboxViews.size >= 20) n.inboxViews.delete(n.inboxViews.keys().next().value!);
    n.inboxViews.set(viewKey, value);
  };
  function find(value: string) { setSearch(value); remember({ search: value, scrollTop: 0 }); }
  const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'list', filter],
    queryFn: ({ signal }) => client.notifications(filter, signal), refetchInterval: 5_000, refetchIntervalInBackground: false });
  const ready = !!query.data;
  useLayoutEffect(() => {
    const view = n.inboxViews.get(viewKey);
    if (!ready || !view?.focusId) return;
    const frame = requestAnimationFrame(() => {
      const row = list.current?.querySelector<HTMLElement>('[data-notification-id="' + CSS.escape(view.focusId!) + '"] a');
      (row ?? list.current)?.focus({ preventScroll: true });
      window.scrollTo(0, view.scrollTop);
      n.inboxViews.set(viewKey, { search: view.search, scrollTop: 0 });
    });
    return () => cancelAnimationFrame(frame);
  }, [ready, n.inboxViews, viewKey]);
  const read = useMutation({ mutationFn: () => {
    if (!query.data || search.trim()) throw Error('请先清空本页查找，再按当前筛选全部已读。');
    return client.readNotifications({ ...filter, cursor: undefined }, query.data.watermark);
  }, onSuccess: async result => { await n.refresh(); toast.success('当前筛选内的 ' + result.changed + ' 条通知已标记已读'); } });
  const change = (next: Partial<NotificationFilter>, keepCursor = false) => {
    const updated = { ...filter, ...next };
    if (!keepCursor) updated.cursor = undefined;
    find('');
    n.inboxViews.delete(JSON.stringify(updated));
    read.reset();
    void navigate({ to: '/notifications', search: updated });
  };
  const items = query.data?.items ?? [];
  const pagination = useRoutePagination({
    key: [...n.key, 'list', { ...filter, cursor: undefined }], data: !query.error ? query.data : undefined, cursor: filter.cursor, trail: [],
    loadIndex: async signal => {
      const result = await client.notificationPageIndex(filter, signal);
      if (result.total == null) throw new Error('暂时无法读取通知总数。');
      return { total: result.total, bookmarks: (result.page_cursors ?? []).map((cursor, i) => ({ cursor, offset: (i + 1) * (filter.limit ?? 30) })) };
    },
    load: (cursor, signal) => client.notifications({ ...filter, cursor }, signal), move: cursor => change({ cursor }, true),
    pageSize: filter.limit ?? 30, setPageSize: size => change({ limit: size }), fetching: query.isFetching,
  });
  const needle = search.trim().toLocaleLowerCase();
  const shown = needle ? items.filter(item => [notificationHeading(item), item.title, item.body, notificationSource(item)].join(' ').toLocaleLowerCase().includes(needle)) : items;
  const selectClass = 'min-h-11 w-full min-w-0 rounded-md border bg-background px-2 text-sm text-foreground';
  return <section ref={list} tabIndex={-1} aria-label="通知列表" className="min-w-0 rounded-2xl border bg-card outline-none">
    <div className="space-y-4 border-b p-4 sm:p-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">收件箱</h2><Button variant="outline" className="min-h-11" disabled={!query.data || query.isFetching || read.isPending || filter.status === 'archived' || !!needle || !!query.error} onClick={() => read.mutate()}><CheckCheck aria-hidden="true" />当前筛选全部已读</Button>
      </div>
      <div className="grid grid-cols-2 items-end gap-3 lg:grid-cols-[1fr_1fr_1.4fr_1.6fr]">
        <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">阅读状态<select value={filter.status ?? 'all'} onChange={e => change({ status: e.target.value as NotificationFilter['status'] })} className={selectClass}><option value="all">全部未归档</option><option value="unread">未读</option><option value="archived">已归档</option></select></label>
        <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">通知分类<select value={filter.category ?? ''} onChange={e => change({ category: e.target.value ? e.target.value as NotificationFilter['category'] : undefined })} className={selectClass}><option value="">全部分类</option>{Object.entries(categories).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
        <NotificationScopeSelect filter={filter} items={items} onChange={change} />
        <div className="flex min-w-0 items-center gap-2 rounded-md border bg-background ps-3"><Search className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><input aria-label="查找本页通知" placeholder="查找本页通知…" value={search} onChange={e => find(e.target.value)} className="min-h-11 min-w-0 flex-1 bg-transparent pe-2 text-sm" />{search && <Button size="icon" variant="ghost" className="size-10 shrink-0" aria-label="清空本页查找" onClick={() => find('')}><X aria-hidden="true" /></Button>}</div>
      </div>
      {needle && <p role="status" className="text-xs text-muted-foreground">本页匹配 {shown.length} / {items.length} 条。清空查找后可按当前筛选全部已读。</p>}
      <RefreshError error={n.error ?? read.error ?? query.error} stale={!!query.data} retry={() => { read.reset(); void n.refresh(); }} />
    </div>
    {query.isPending ? <p role="status" className="p-8 text-center text-sm text-muted-foreground">正在加载通知…</p> : query.data && <NotificationRows items={shown} filter={filter} unread={filter.status === 'unread'} searching={!!needle} rememberItem={focusId => remember({ search, focusId, scrollTop: window.scrollY })} />}
    <div className="border-t p-3 sm:px-5"><ListPagination label="通知" {...pagination} pageSizes={[10, 20, 30, 50, 100]} /></div>
  </section>;
}
