import { useState } from 'react';
import { ArrowRight, ChevronDown, Clock3, RefreshCw } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import type { SecurityEvent } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { date } from '../auth/forms';
import { events, outcomes } from './security-labels';

function EventLine({ item }: { item: SecurityEvent }) {
  return <>
    <Clock3 aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2"><span className="text-sm font-medium">{events[item.event_type] ?? item.event_type}</span>
        <Badge variant={item.outcome === 'success' ? 'secondary' : 'destructive'}>{outcomes[item.outcome] ?? item.outcome}</Badge>
      </div>
      <time className="mt-1 block text-xs text-muted-foreground" dateTime={item.created_at}>{date(item.created_at)}</time>
    </div>
  </>;
}

function EventHistory() {
  const { account, client } = useIdentity();
  const query = usePagedQuery({ queryKey: ['account', account?.id, 'security-events', 'history'], initialPageSize: 50,
    queryFn: ({ cursor, signal }) => client.securityEvents(cursor, signal) });
  return <div className="min-w-0 space-y-3">
    <div className="flex justify-end"><Button variant="ghost" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}><RefreshCw aria-hidden="true" />刷新记录</Button></div>
    <FormNotice error>{query.error?.message}</FormNotice>
    {query.isPending && <p role="status" className="text-sm">正在读取安全记录…</p>}
    {query.data?.items.length === 0 && <p className="text-sm text-muted-foreground">暂无安全记录。</p>}
    <ul className="divide-y" aria-label="近期安全记录">{query.data?.items.map(item => <li key={item.id}>
      <details className="group"><summary className="flex min-h-14 cursor-pointer list-none items-center gap-3 py-2">
        <EventLine item={item} /><ChevronDown aria-hidden="true" className="size-4 shrink-0 group-open:rotate-180" />
      </summary><dl className="mb-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 rounded-md bg-muted/50 p-3 text-xs leading-5">
        <dt>来源 IP</dt><dd>{item.source_ip || '服务器维护操作'}</dd><dt>客户端</dt><dd className="break-all">{item.user_agent || '未记录'}</dd>
      </dl></details>
    </li>)}</ul>
    <ListPagination label="近期安全记录" {...query.pagination} />
  </div>;
}

export function SecurityRecordsCard() {
  const { account, client } = useIdentity();
  const [open, setOpen] = useState(false);
  const latest = useQuery({ queryKey: ['account', account?.id, 'security-events', 'recent'],
    queryFn: ({ signal }) => client.securityEvents('', signal), select: result => result.items.slice(0, 3) });
  return <section className="account-surface" aria-labelledby="recent-security-title">
    <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
      <h2 id="recent-security-title" className="font-semibold">近期安全记录</h2>
      <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button size="sm" variant="ghost">查看记录<ArrowRight aria-hidden="true" /></Button></DialogTrigger>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader>
          <DialogTitle>近期安全记录</DialogTitle><DialogDescription>按发生时间排列，展开一条记录可查看来源。</DialogDescription>
        </DialogHeader>{open && <EventHistory />}</DialogContent>
      </Dialog>
    </div>
    {latest.error ? <FormNotice error>{latest.error.message}</FormNotice> : !latest.data?.length ?
      <p className="text-sm text-muted-foreground">{latest.isPending ? '正在读取安全记录…' : '暂无安全记录。'}</p> :
      <ul className="divide-y">{latest.data.map(item => <li className="flex items-center gap-3 py-2" key={item.id}><EventLine item={item} /></li>)}</ul>}
  </section>;
}
