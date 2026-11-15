import { useState } from 'react';
import { ArrowLeft, ArrowRight, Bell, CheckCheck, Settings2, X } from 'lucide-react';
import { Link, useLocation } from '@tanstack/react-router';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Popover, PopoverClose, PopoverContent, PopoverDescription, PopoverTitle, PopoverTrigger } from '@infrapro/ui/components/popover';
import { cn } from '@infrapro/ui/lib/utils';
import { useNotifications } from './provider';
import { NotificationRows, RefreshError } from './common';
import { NotificationDetail } from './detail';
import { relativePath, workspaceScope } from '../../config/navigation';

export function NotificationBell() {
  const context = useNotifications();
  return <BellContent key={JSON.stringify(context.key)} />;
}

function BellContent() {
  const { client } = useIdentity(), context = useNotifications();
  const scope = workspaceScope(relativePath(useLocation({ select: location => location.pathname })));
  const [unreadOnly, setUnreadOnly] = useState(false), [currentScope, setCurrentScope] = useState(false), [selectedId, setSelectedId] = useState<string>();
  const filter: NotificationFilter = { status: unreadOnly ? 'unread' : 'all', ...(currentScope && scope.tenantId ? { tenant_id: scope.tenantId, project_id: scope.projectId } : {}) };
  const list = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: [...context.key, 'list', { ...filter, limit: 8 }], queryFn: ({ signal }) => client.notifications({ ...filter, limit: 8 }, signal), enabled: context.open, refetchInterval: context.open ? 5_000 : false, refetchIntervalInBackground: false });
  const read = useMutation({ mutationFn: () => { if (!list.data) throw Error('请先加载通知。'); return client.readNotifications(filter, list.data.watermark); }, onSuccess: context.refresh });
  const count = context.summary?.unread;
  const label = context.error ? '通知，未读数暂未更新' : count === undefined ? '通知，正在加载未读数' : `通知，${count} 条未读`;
  const setOpen = (open: boolean) => { context.setOpen(open); if (!open) setSelectedId(undefined); };
  return <><span className="sr-only" role="status" aria-live="polite">{label}</span><Popover open={context.open} onOpenChange={setOpen}>
    <PopoverTrigger render={<Button variant="ghost" size="icon" className={cn('relative size-11', context.open && 'bg-accent')} aria-label={label} />}>
      <Bell className="size-5" aria-hidden="true" />{count !== undefined && count > 0 && <span aria-hidden="true" className="absolute end-0 top-0 min-w-5 rounded-full bg-primary px-1 text-xs text-primary-foreground">{count > 99 ? '99+' : count}</span>}{context.error && <span aria-hidden="true" className="absolute end-0 top-0 text-xs">!</span>}
    </PopoverTrigger>
    <PopoverContent aria-label="通知快捷面板">
      <div className="shrink-0 px-5 pt-4 pb-3">
        <div className="flex items-center justify-between gap-2"><PopoverTitle className="flex flex-wrap items-center gap-2 text-lg font-semibold">通知 {count !== undefined && <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">{count} 条未读</span>}</PopoverTitle><div className="flex gap-1"><Button asChild variant="ghost" size="icon" className="size-11"><Link to="/notifications/preferences" onClick={() => setOpen(false)} aria-label="通知接收设置"><Settings2 aria-hidden="true" /></Link></Button><PopoverClose render={<Button variant="ghost" size="icon" className="size-11" aria-label="关闭通知" />}><X aria-hidden="true" /></PopoverClose></div></div>
        <PopoverDescription className="sr-only">个人通知箱。仅打开列表不会标记已读。</PopoverDescription>
        <label className="sr-only" htmlFor="notification-quick-scope">通知来源范围</label><select id="notification-quick-scope" value={currentScope && scope.tenantId ? 'current' : 'all'} onChange={e => { setCurrentScope(e.target.value === 'current'); setSelectedId(undefined); }} className="min-h-11 max-w-full rounded-md bg-background px-1 text-sm text-muted-foreground"><option value="all">全部工作空间</option>{scope.tenantId && <option value="current">当前工作空间</option>}</select>
      </div>
      {selectedId ? <NotificationDetail id={selectedId} compact onNavigate={() => setOpen(false)} back={<Button variant="ghost" className="min-h-11" onClick={() => setSelectedId(undefined)}><ArrowLeft aria-hidden="true" />返回列表</Button>} /> : <>
        <div className="flex shrink-0 flex-wrap items-center gap-4 border-b px-5"><div className="flex gap-4" role="group" aria-label="快捷通知阅读状态">{[['全部', false], ['未读', true]].map(([name, value]) => <button key={String(name)} type="button" aria-pressed={unreadOnly === value} className={cn('min-h-11 border-b-2 border-transparent px-1 text-sm text-muted-foreground', unreadOnly === value && 'border-foreground font-medium text-foreground')} onClick={() => setUnreadOnly(Boolean(value))}>{name}</button>)}</div><Button variant="ghost" size="sm" className="ms-auto min-h-11 px-1 text-xs" aria-label="当前筛选全部已读" disabled={!list.data || list.isFetching || read.isPending || !!list.error} onClick={() => read.mutate()}><CheckCheck aria-hidden="true" />全部已读</Button></div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain"><div className="empty:hidden">{(list.error || context.error || read.error) && <div className="p-3"><RefreshError error={list.error ?? context.error ?? read.error} stale={!!list.data} retry={() => { void context.refresh(); read.reset(); }} /></div>}</div>{list.isPending ? <p role="status" className="px-5 py-12 text-center text-sm text-muted-foreground">正在加载通知…</p> : list.data && <NotificationRows items={list.data.items} compact unread={unreadOnly} onSelect={setSelectedId} />}</div>
      </>}
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t bg-muted/20 px-4 py-2"><span className="text-xs text-muted-foreground">{selectedId ? '详情加载后标记已读' : '仅打开列表不会标记已读'}</span><Button asChild variant="ghost" className="min-h-11 px-2 text-sm">{selectedId ? <Link to="/notifications/$notificationId" params={{ notificationId: selectedId }} search={filter} onClick={() => setOpen(false)}>在通知中心查看<ArrowRight aria-hidden="true" /></Link> : <Link to="/notifications" search={filter} onClick={() => setOpen(false)}>查看全部通知<ArrowRight aria-hidden="true" /></Link>}</Button></div>
    </PopoverContent>
  </Popover></>;
}
