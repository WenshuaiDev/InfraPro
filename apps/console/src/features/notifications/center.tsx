import { useLayoutEffect, useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Archive, ArrowLeft, CheckCheck, Inbox, Layers, Mail, Search, Settings2, ShieldCheck, UserPlus, Users, Workflow, X } from 'lucide-react';
import { toast } from 'sonner';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { useNotifications } from './provider';
import { NotificationRows, NotificationScopeSelect, RefreshError } from './common';
import { NotificationDetail } from './detail';
import { categories } from './labels';
import { notificationHeading, notificationSource } from './presentation';

type Props = { filter: NotificationFilter; selectedId?: string };
export function NotificationCenter(props: Props) {
  const n = useNotifications();
  return <CenterContent key={JSON.stringify([...n.key, props.filter])} {...props} />;
}

function CenterContent({ filter, selectedId }: Props) {
  const { client } = useIdentity(), n = useNotifications(), navigate = useNavigate();
  const viewKey = JSON.stringify(filter);
  const [search, setSearch] = useState(() => n.inboxViews.get(viewKey)?.search ?? '');
  const listElement = useRef<HTMLDivElement>(null), restoredPosition = useRef(false);
  const remember = (value: Partial<{ scrollTop: number; search: string }>) => {
    if (!n.inboxViews.has(viewKey) && n.inboxViews.size >= 20) n.inboxViews.delete(n.inboxViews.keys().next().value!);
    n.inboxViews.set(viewKey, { scrollTop: 0, search: '', ...n.inboxViews.get(viewKey), ...value });
  };
  const find = (value: string) => { setSearch(value); remember({ search: value, scrollTop: 0 }); if (listElement.current) listElement.current.scrollTop = 0; };
  const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...n.key, 'list', filter], queryFn: ({ signal }) => client.notifications(filter, signal), refetchInterval: 5_000, refetchIntervalInBackground: false });
  useLayoutEffect(() => {
    const element = listElement.current;
    if (query.data && element?.getClientRects().length && !restoredPosition.current) {
      element.scrollTop = n.inboxViews.get(viewKey)?.scrollTop ?? 0;
      restoredPosition.current = true;
    }
  }, [query.data, n.inboxViews, viewKey]);
  const read = useMutation({ mutationFn: () => {
    if (!query.data || search.trim()) throw Error('请先清空本页查找，再按当前筛选全部已读。');
    return client.readNotifications({ ...filter, cursor: undefined }, query.data.watermark);
  }, onSuccess: async result => { await n.refresh(); toast.success(`当前筛选内的 ${result.changed} 条通知已标记已读`); } });
  const change = (next: Partial<NotificationFilter>, keepCursor = false) => {
    find(''); read.reset();
    void navigate({ to: '/notifications', search: { ...filter, ...next, ...(!keepCursor ? { cursor: undefined } : {}) } });
  };
  const items = query.data?.items ?? [];
  const needle = search.trim().toLocaleLowerCase();
  const shown = needle ? items.filter(item => `${notificationHeading(item)} ${item.title} ${item.body} ${notificationSource(item)}`.toLocaleLowerCase().includes(needle)) : items;
  const categoryIcons = { tasks: Workflow, organization: Users, updates: Layers, security: ShieldCheck };
  const statusItems = [{ value: 'all', label: '收件箱', Icon: Inbox }, { value: 'unread', label: '未读通知', Icon: Mail }, { value: 'archived', label: '已归档', Icon: Archive }] as const;
  const navClass = (active: boolean) => cn('flex min-h-11 w-full items-center gap-2.5 rounded-md px-3 text-start text-sm transition-colors hover:bg-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring', active ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950/50 dark:text-blue-300' : 'text-muted-foreground');
  return <section className="flex min-w-0 flex-col gap-5" aria-label="通知中心">
    <div className="flex flex-wrap items-start justify-between gap-4"><div className="space-y-1"><h1 className="text-2xl font-bold tracking-tight">通知中心</h1><p className="text-sm text-muted-foreground">你的全部工作空间，一个收件箱。{n.summary && <span className="ms-2">{n.summary.unread} 条未读</span>}</p></div><Button variant="outline" className="min-h-11" disabled={!query.data || query.isFetching || read.isPending || filter.status === 'archived' || !!needle || !!query.error} onClick={() => read.mutate()}><CheckCheck aria-hidden="true" />当前筛选全部已读</Button></div>
    <RefreshError error={n.error ?? read.error} stale={!!query.data} retry={() => { void n.refresh(); read.reset(); }} />
    <div className="grid min-w-0 overflow-hidden rounded-xl border bg-background lg:h-[calc(100dvh-14rem)] lg:min-h-[32rem] lg:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)] xl:grid-cols-[11rem_22rem_minmax(0,1fr)]">
      <nav aria-label="通知分类导航" className="hidden min-h-0 flex-col gap-1 overflow-y-auto border-e bg-muted/20 p-3 xl:flex">
        {statusItems.map(({ value, label, Icon }) => <button key={value} type="button" aria-pressed={(filter.status ?? 'all') === value} className={navClass((filter.status ?? 'all') === value)} onClick={() => change({ status: value })}><Icon className="size-4 shrink-0" aria-hidden="true" />{label}</button>)}
        <p className="px-3 pt-6 pb-2 text-xs text-muted-foreground">按分类查看</p>
        <button type="button" aria-pressed={!filter.category} className={navClass(!filter.category)} onClick={() => change({ category: undefined })}><Inbox className="size-4 shrink-0" aria-hidden="true" />全部分类</button>
        {Object.entries(categories).map(([key, label]) => { const Icon = categoryIcons[key as keyof typeof categoryIcons]; return <button key={key} type="button" aria-pressed={filter.category === key} className={navClass(filter.category === key)} onClick={() => change({ category: key as NotificationFilter['category'] })}><Icon className="size-4 shrink-0" aria-hidden="true" />{label}</button>; })}
        <div className="mt-auto space-y-1 border-t pt-3"><Link to="/notifications/invitations" className={navClass(false)}><UserPlus className="size-4 shrink-0" aria-hidden="true" /><span className="whitespace-normal">收到的租户邀请</span></Link><Link to="/notifications/preferences" className={navClass(false)}><Settings2 className="size-4 shrink-0" aria-hidden="true" />接收设置</Link></div>
      </nav>
      <section aria-label="通知列表" className={cn('min-h-0 min-w-0 flex-col lg:flex lg:border-e', selectedId ? 'hidden' : 'flex')}>
        <div className="shrink-0 space-y-3 border-b p-4">
          <div className="grid grid-cols-2 gap-2 xl:hidden"><label className="grid min-w-0 gap-1 text-xs text-muted-foreground">阅读状态<select aria-label="阅读状态" value={filter.status ?? 'all'} onChange={e => change({ status: e.target.value as NotificationFilter['status'] })} className="min-h-11 min-w-0 rounded-md border bg-background px-2 text-sm text-foreground"><option value="all">全部未归档</option><option value="unread">未读</option><option value="archived">已归档</option></select></label><label className="grid min-w-0 gap-1 text-xs text-muted-foreground">通知分类<select aria-label="通知分类" value={filter.category ?? ''} onChange={e => change({ category: e.target.value ? e.target.value as NotificationFilter['category'] : undefined })} className="min-h-11 min-w-0 rounded-md border bg-background px-2 text-sm text-foreground"><option value="">全部分类</option>{Object.entries(categories).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label></div>
          <NotificationScopeSelect filter={filter} items={items} onChange={change} />
          <div className="flex min-w-0 items-center gap-2 rounded-md border bg-muted/20 ps-3"><Search className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><input aria-label="查找本页通知" placeholder="查找本页通知…" value={search} onChange={e => find(e.target.value)} className="min-h-11 min-w-0 flex-1 bg-transparent py-2 text-sm outline-offset-2" />{search && <Button size="icon" variant="ghost" className="size-11" aria-label="清空本页查找" onClick={() => find('')}><X aria-hidden="true" /></Button>}</div>
          <div className="flex flex-wrap gap-1.5 text-xs text-muted-foreground" aria-live="polite"><span>{filter.category ? categories[filter.category] : '全部分类'} · {filter.status === 'unread' ? '未读' : filter.status === 'archived' ? '已归档' : '未归档'}</span><span className="ms-auto">{query.isPending ? '加载中…' : needle ? `本页匹配 ${shown.length} / ${items.length} 条` : `本页 ${items.length} 条`}</span></div>
          {needle && <p className="text-xs leading-5 text-muted-foreground">关键词仅查找本页。清空后可按当前来源和分类全部已读。</p>}
          <div className="flex flex-wrap gap-x-4 gap-y-2 xl:hidden"><Link to="/notifications/invitations" className="inline-flex min-h-11 items-center text-xs underline underline-offset-4">收到的租户邀请</Link><Link to="/notifications/preferences" className="inline-flex min-h-11 items-center text-xs underline underline-offset-4">接收设置</Link></div>
        </div>
        <div ref={listElement} className="min-h-0 flex-1 overflow-y-auto overscroll-contain" onScroll={e => { if (restoredPosition.current) remember({ scrollTop: e.currentTarget.scrollTop }); }}>
          {query.error && <div className="p-3"><RefreshError error={query.error} stale={!!query.data} retry={() => { void n.refresh(); }} /></div>}
          {query.isPending ? <p role="status" className="px-5 py-12 text-center text-sm text-muted-foreground">正在加载通知…</p> : query.data && <NotificationRows items={shown} filter={filter} selectedId={selectedId} unread={filter.status === 'unread'} searching={!!needle} />}
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t px-3 py-2"><span className="text-xs text-muted-foreground">仅打开列表不会标记已读</span><div className="flex gap-1">{filter.cursor && <Button variant="ghost" className="min-h-11 px-2 text-xs" onClick={() => change({ cursor: undefined })}>回到第一页</Button>}{query.data?.next_cursor && <Button variant="ghost" className="min-h-11 px-2 text-xs" onClick={() => change({ cursor: query.data?.next_cursor }, true)}>下一页</Button>}</div></div>
      </section>
      <div className={cn('min-h-0 min-w-0 flex-col lg:flex', selectedId ? 'flex' : 'hidden')}>
        {selectedId ? <NotificationDetail id={selectedId} back={<Button asChild variant="ghost" className="min-h-11"><Link to="/notifications" search={filter}><ArrowLeft aria-hidden="true" />返回通知列表</Link></Button>} /> : <div className="flex min-h-64 flex-1 flex-col items-center justify-center gap-3 p-8 text-center"><Mail className="size-10 text-muted-foreground/50" aria-hidden="true" /><h2 className="font-medium">选择一条通知</h2><p className="text-sm leading-6 text-muted-foreground">详情会在这里展开。<br />详情加载成功后，该通知才会标记已读。</p></div>}
      </div>
    </div>
    <p className="text-xs leading-5 text-muted-foreground">全部已读仅处理当前来源、分类和阅读状态下已经收到的未归档通知；新收到的消息保持未读。</p>
  </section>;
}
