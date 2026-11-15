import { Link } from '@tanstack/react-router';
import { AlertCircle, Bell, CheckCircle2, CircleHelp, Inbox, Layers, LockKeyhole, ShieldCheck, Users } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import type { NotificationFilter, NotificationItem } from '@infrapro/api-client';
import { categories, time } from './labels';
import { notificationDay, notificationHeading, notificationOutcome, notificationSource, notificationTime } from './presentation';

export function RefreshError({ error, stale, retry }: { error: Error | null; stale?: boolean; retry: () => void }) {
  return error ? <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
    <p>{stale ? '暂未更新，以下为上次加载的内容。' : '通知暂时无法加载。'} {error.message}</p>
    <Button variant="outline" className="mt-2 min-h-11" onClick={retry}>重新加载</Button>
  </div> : null;
}

export function NotificationIcon({ item, className }: { item: NotificationItem; className?: string }) {
  const failed = ['failed', 'missing'].includes(item.outcome), action = ['pending', 'validated', 'needs_action'].includes(item.outcome);
  const Icon = item.redacted ? LockKeyhole : item.category === 'security' ? ShieldCheck : item.category === 'organization' ? Users : item.category === 'updates' ? Layers : failed ? AlertCircle : action ? CircleHelp : CheckCircle2;
  return <span className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/60 text-muted-foreground', !item.redacted && (failed ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300' : action ? 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/50 dark:text-amber-300' : 'border-blue-100 bg-blue-50 text-blue-700 dark:border-blue-900 dark:bg-blue-950/50 dark:text-blue-300'), className)}><Icon className="size-4" aria-hidden="true" /></span>;
}

export function NotificationEmpty({ unread = false, searching = false }: { unread?: boolean; searching?: boolean }) {
  return <div className="flex min-h-52 flex-col items-center justify-center gap-3 px-5 py-10 text-center">
    <Inbox className="size-9 text-muted-foreground/60" aria-hidden="true" />
    <p className="font-medium">{searching ? '本页没有匹配的通知' : unread ? '当前筛选内没有未读通知' : '当前筛选下没有通知'}</p>
    <p className="text-sm text-muted-foreground">{searching ? '可以清空关键词，或翻页继续查找。' : '试试切换分类、阅读状态或来源范围。'}</p>
  </div>;
}

type RowsProps = { items: NotificationItem[]; filter?: NotificationFilter; selectedId?: string; onSelect?: (id: string) => void; compact?: boolean; unread?: boolean; searching?: boolean };
export function NotificationRows({ items, filter = {}, selectedId, onSelect, compact = false, unread, searching }: RowsProps) {
  if (!items.length) return <NotificationEmpty unread={unread} searching={searching} />;
  return <ul aria-label="通知条目" className="min-w-0">
    {items.map((item, index) => {
      const day = notificationDay(item.delivered_at);
      const selected = selectedId === item.id;
      const content = <>
        {compact && <NotificationIcon item={item} />}
        <div className="min-w-0 flex-1 space-y-1.5">
          {!compact && <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><Bell className="size-3.5" aria-hidden="true" /><span>{categories[item.category as keyof typeof categories] || '通知'}</span><time className="ms-auto" dateTime={item.delivered_at} title={time(item.delivered_at)}>{notificationTime(item.delivered_at)}</time></div>}
          <div className="flex items-start gap-2"><span className={cn('min-w-0 flex-1 break-words text-sm leading-6', item.read_at ? 'font-medium' : 'font-semibold')}>{notificationHeading(item)}</span><span className="mt-2 flex size-2 shrink-0 items-center justify-center">{!item.read_at && <span className="size-1.5 rounded-full bg-blue-600 dark:bg-blue-400" aria-hidden="true" />}<span className="sr-only">{item.read_at ? '已读' : '未读'}</span></span></div>
          <p className="line-clamp-2 break-words text-xs leading-5 text-muted-foreground">{item.body}</p>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><span className="min-w-0 flex-1 break-words">{notificationSource(item)}</span>{compact ? <time dateTime={item.delivered_at} title={time(item.delivered_at)}>{notificationTime(item.delivered_at)}</time> : <span>{notificationOutcome(item)}</span>}</div>
        </div>
      </>;
      const className = cn('relative flex w-full min-w-0 items-start gap-3 border-b px-5 py-4 text-start transition-colors hover:bg-accent/60 focus-visible:z-10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring', selected && 'bg-blue-50 before:absolute before:inset-y-0 before:start-0 before:w-0.5 before:bg-blue-600 dark:bg-blue-950/35', compact && 'px-4 py-3.5');
      return <li key={item.id} data-notification-id={item.id}>
        {(index === 0 || notificationDay(items[index - 1]!.delivered_at) !== day) && <p className="border-b bg-muted/25 px-5 py-2 text-xs font-medium text-muted-foreground">{day}</p>}
        {onSelect ? <button type="button" className={className} onClick={() => onSelect(item.id)} aria-label={`查看通知：${notificationHeading(item)}，${item.read_at ? '已读' : '未读'}`}>{content}</button> : <Link to="/notifications/$notificationId" params={{ notificationId: item.id }} search={filter} aria-current={selected ? 'page' : undefined} className={className}>{content}</Link>}
      </li>;
    })}
  </ul>;
}

export function NotificationScopeSelect({ filter, items, onChange }: { filter: NotificationFilter; items: NotificationItem[]; onChange: (value: Partial<NotificationFilter>) => void }) {
  const scopes = new Map<string, { tenant_id: string; project_id?: string; name: string }>();
  items.forEach(item => { if (!item.redacted && item.scope.tenant_id) scopes.set(`${item.scope.tenant_id}:${item.scope.project_id ?? ''}`, { tenant_id: item.scope.tenant_id, project_id: item.scope.project_id, name: item.scope_name || '工作空间' }); });
  const selected = filter.tenant_id ? `${filter.tenant_id}:${filter.project_id ?? ''}` : filter.scope ?? '';
  if (filter.tenant_id && !scopes.has(selected)) scopes.set(selected, { tenant_id: filter.tenant_id, project_id: filter.project_id, name: '所选工作空间' });
  return <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">来源范围<select aria-label="来源范围" className="min-h-11 w-full min-w-0 max-w-full rounded-md border bg-background px-2 text-sm text-foreground" value={selected} onChange={e => {
    const value = scopes.get(e.target.value);
    onChange({ tenant_id: value?.tenant_id, project_id: value?.project_id, scope: ['account', 'platform', 'tenant', 'project'].includes(e.target.value) ? e.target.value as NotificationFilter['scope'] : undefined });
  }}><option value="">全部工作空间</option><option value="account">账号通知</option><option value="platform">平台通知</option><option value="tenant">所有租户来源</option><option value="project">所有项目来源</option>{[...scopes].map(([key, value]) => <option key={key} value={key}>{value.name}</option>)}</select></label>;
}
