import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Archive, Building2, ExternalLink, Mail, MailOpen } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationChange, NotificationItem } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { NotificationHistory } from '../audit/task-history';
import { useNotifications } from './provider';
import { NotificationIcon, RefreshError } from './common';
import { categories, time, currentStates, unavailableReasons } from './labels';
import { notificationDestination } from './targets';
import { notificationHeading, notificationOutcome, notificationSource } from './presentation';

type Props = { id: string; back: ReactNode; compact?: boolean; onNavigate?: () => void };
export function NotificationDetail(props: Props) {
  const n = useNotifications();
  return <DetailContent key={JSON.stringify([...n.key, props.id])} {...props} />;
}

function DetailContent({ id, back, compact, onNavigate }: Props) {
  const { client } = useIdentity(), n = useNotifications(), cache = useQueryClient(), navigate = useNavigate();
  const key = [...n.key, 'detail', id];
  const query = useQuery({ refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', queryKey: key, queryFn: ({ signal }) => client.notification(id, signal), retry: false });
  const attempted = useRef(false), active = useRef(true), targetRequest = useRef<AbortController | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);
  useEffect(() => { active.current = true; return () => { active.current = false; targetRequest.current?.abort(); }; }, []);
  const change = useMutation({
    mutationFn: (input: Omit<NotificationChange, 'expected_revision'>) => {
      if (!query.data) throw Error('请先加载通知。');
      return client.changeNotification(id, { ...input, expected_revision: query.data.revision });
    },
    onMutate: () => key,
    onSuccess: async (value, _input, startedKey) => {
      if (startedKey) {
        const current = cache.getQueryData<NotificationItem>(startedKey);
        // A late response can update only its original, still-valid cache entry.
        if (current?.id === value.id && BigInt(current.revision) <= BigInt(value.revision)) cache.setQueryData(startedKey, value);
      }
      await n.refresh();
    },
    onError: () => { if (active.current) void query.refetch(); },
  });
  const { mutate } = change;
  useEffect(() => {
    if (query.data && !query.error && !attempted.current) {
      attempted.current = true;
      if (!query.data.read_at) mutate({ read: true });
    }
  }, [query.data, query.error, mutate]);
  const target = useMutation({ mutationFn: async () => {
    targetRequest.current?.abort();
    const controller = new AbortController(); targetRequest.current = controller;
    const value = await client.notificationTarget(id, controller.signal);
    if (!active.current || controller.signal.aborted) return;
    const destination = notificationDestination(value);
    if (!destination) throw Error('目标当前不可用，可能已过期、删除或失去访问资格。请刷新通知查看状态。');
    onNavigate?.();
    await navigate(destination);
  } });
  const item = query.error ? undefined : query.data;
  const busy = change.isPending || query.isFetching;
  return <section aria-label="通知详情" className="flex min-h-0 min-w-0 flex-1 flex-col">
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-2.5">
      {back}
      {item && <Button variant="ghost" className="min-h-11" disabled={busy} onClick={() => change.mutate({ read: !item.read_at })}>{item.read_at ? <Mail aria-hidden="true" /> : <MailOpen aria-hidden="true" />}{item.read_at ? '恢复未读' : '标记已读'}</Button>}
    </div>
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
      <div className={compact ? 'space-y-5 p-5' : 'space-y-6 p-5 sm:p-7'}>
        <RefreshError error={query.error} retry={() => { void query.refetch(); }} />
        {query.isPending && <p role="status" className="py-10 text-center text-sm text-muted-foreground">正在加载通知详情…</p>}
        {item && <>
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2"><NotificationIcon item={item} /><span className="rounded-md bg-muted px-2 py-1 text-xs font-medium">{notificationOutcome(item)}</span><span className="text-xs text-muted-foreground">{categories[item.category as keyof typeof categories]}</span></div>
            <h2 className="break-words text-xl leading-relaxed font-semibold tracking-tight">{notificationHeading(item)}</h2>
            <p className="flex items-start gap-2 text-xs leading-5 text-muted-foreground"><Building2 className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" /><span className="break-words">{notificationSource(item)}</span></p>
          </div>
          <p className="break-words text-sm leading-7">{item.body}</p>
          <dl className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3 rounded-lg border bg-muted/20 p-4 text-sm">
            <dt className="text-muted-foreground">通知事项</dt><dd className="break-words">{item.title}</dd>
            <dt className="text-muted-foreground">阅读状态</dt><dd>{item.archived_at ? '已归档' : item.read_at ? '已读' : '未读'}</dd>
            {item.current_state && <><dt className="text-muted-foreground">当前业务状态</dt><dd>{currentStates[item.current_state] || '状态已更新'}</dd></>}
            {item.reason_code && !item.redacted && <><dt className="text-muted-foreground">结果原因</dt><dd className="break-words">{item.reason_code === 'validation_requires_resolution' ? '校验发现需处理的问题，请查看原导入报告。' : item.reason_code === 'job_cancelled' ? '任务取消已生效。' : item.reason_code}</dd></>}
          </dl>
          {item.legacy_task_reference && <p className="rounded-lg bg-muted p-3 text-xs leading-6 text-muted-foreground">旧通知未记录执行代次；业务入口显示当前状态，不能据此还原当时的执行。</p>}
          {item.redacted && <p className="rounded-lg bg-muted p-4 text-sm leading-6">来源内容当前不可访问，已隐藏工作空间和业务详情。你仍可管理这条个人回执。</p>}
          {!item.target_available && !item.redacted && <p className="text-sm leading-6 text-muted-foreground">{unavailableReasons[item.unavailable_reason ?? ''] || '当前业务状态已变化，暂无可用操作入口。'}</p>}
          <RefreshError error={change.error} retry={() => { void query.refetch(); }} />
          <RefreshError error={target.error} retry={() => { void query.refetch(); }} />
          <div className="flex flex-wrap gap-2">
            {item.target_available && !item.redacted && <Button className="min-h-11" disabled={target.isPending || busy} onClick={() => target.mutate()}><ExternalLink aria-hidden="true" />{target.isPending ? '正在核验访问资格…' : '打开业务页面'}</Button>}
            <Button variant="outline" className="min-h-11" disabled={busy} onClick={() => change.mutate({ archived: !item.archived_at })}><Archive aria-hidden="true" />{item.archived_at ? '取消归档' : '归档'}</Button>
          </div>
          <p className="text-xs leading-6 text-muted-foreground">阅读和归档仅改变通知状态。下载、接受邀请或任务处理在业务页面完成。</p>
          <div className="space-y-1 border-t pt-4 text-xs leading-6 text-muted-foreground"><p>发生时间：{time(item.occurred_at)}</p><p>收到时间：{time(item.delivered_at)}</p></div>
          {item.task_id && !item.redacted && <details className="rounded-lg border p-3" onToggle={e => setHistoryOpen(e.currentTarget.open)}><summary className="min-h-8 cursor-pointer text-sm font-medium">任务操作记录</summary>{historyOpen && <div className="mt-3 min-w-0"><NotificationHistory taskId={item.task_id} generation={item.task_generation} /></div>}</details>}
        </>}
      </div>
    </div>
  </section>;
}
