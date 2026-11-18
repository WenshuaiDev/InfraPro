import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { ChevronDown, Laptop, RefreshCw } from 'lucide-react';
import { Permissions as P, type Session } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useArrayPagination } from '@infrapro/ui/hooks/use-array-pagination';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { date } from '../auth/forms';
import { useOperation } from './use-operation';
import { clientLabel } from './security-labels';

type SessionAction = { kind: 'current' | 'others' | 'all' } | { kind: 'revoke'; id: string };
const methods = { password: '密码登录', password_totp: '密码 + 验证器', password_recovery_code: '密码 + 恢复码' };
const actionLabels = { current: '退出当前会话', revoke: '撤销会话', others: '退出其他设备', all: '退出全部设备' };
const descriptions = {
  current: '当前浏览器需要重新登录。',
  revoke: '此会话将失效，使用它的客户端需要重新登录。',
  others: '其他会话将失效，当前浏览器保持登录。',
  all: '包括当前浏览器在内的所有登录会话都会失效，需要重新登录。',
};

function SessionInfo({ session }: { session: Session }) {
  return <details className="group min-w-0 flex-1">
    <summary className="flex min-h-12 cursor-pointer list-none items-center gap-3">
      <Laptop aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1"><span className="flex flex-wrap items-center gap-2 text-sm font-medium">{clientLabel(session.user_agent)}
        <Badge variant={session.current ? 'secondary' : 'outline'}>{session.current ? '当前会话' : '其他会话'}</Badge>
      </span><span className="mt-1 block text-xs text-muted-foreground">最近活动 {date(session.last_activity_at)}</span></span>
      <ChevronDown aria-hidden="true" className="size-4 shrink-0 group-open:rotate-180" />
    </summary>
    <dl className="mb-1 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 rounded-md bg-muted/50 p-3 text-xs leading-5">
      <dt>登录方式</dt><dd>{methods[session.authentication_method]}</dd><dt>来源 IP</dt><dd>{session.ip || '未知'}</dd>
      <dt>创建时间</dt><dd>{date(session.created_at)}</dd><dt>最晚到期</dt><dd>{date(session.expires_at)}</dd>
      <dt>客户端</dt><dd className="break-all">{session.user_agent || '未知'}</dd>
    </dl>
  </details>;
}

export function SessionsCard() {
  const { client, account, can, signedOut } = useIdentity();
  const canRead = can(P.AccountSessionRead), canRevoke = can(P.AccountSessionRevoke);
  const cache = useQueryClient(), navigate = useNavigate();
  const key = ['account', account?.id, 'sessions'];
  const query = useQuery({ queryKey: key, queryFn: ({ signal }) => client.sessions(signal), enabled: canRead });
  const sessions = [...(query.data?.items ?? [])].sort((a, b) => Number(b.current) - Number(a.current) || b.last_activity_at.localeCompare(a.last_activity_at));
  const current = sessions.find(item => item.current);
  const page = useArrayPagination(sessions, 10);
  const [open, setOpen] = useState(false), [confirm, setConfirm] = useState<string>();
  const refresh = () => Promise.all([cache.invalidateQueries({ queryKey: key }), cache.invalidateQueries({ queryKey: ['account', account?.id, 'security-events'] })]);
  const action = useOperation({
    mutationFn: async (target: SessionAction) => {
      // The session cookie may have changed in another tab since the list was read.
      if (target.kind === 'current') await client.logout();
      else if (target.kind === 'all') await client.logoutAll();
      else if (target.kind === 'revoke') await client.revokeSession(target.id);
      else for (const item of sessions.filter(item => !item.current)) await client.revokeSession(item.id);
    },
    onSuccess: async (_, target) => {
      setConfirm(undefined);
      if (target.kind === 'current' || target.kind === 'all') { signedOut(); void navigate({ to: '/login', replace: true }); }
      else await refresh();
    },
    // A bulk action can partially succeed. A retry should use the remaining sessions.
    onError: async () => { await refresh(); },
  });
  function revokeButton(target: SessionAction, disabled = false) {
    const id = target.kind === 'revoke' ? target.id : target.kind;
    const label = actionLabels[target.kind];
    return <ConfirmDialog trigger={<Button size="sm" variant="outline" className={target.kind === 'all' ? 'text-destructive' : undefined} disabled={disabled || action.isPending}>{label}</Button>}
      open={confirm === id} onOpenChange={next => { action.reset(); setConfirm(next ? id : undefined); }}
      title={label + '？'} description={descriptions[target.kind]} confirmLabel={'确认' + label} destructive pending={action.isPending}
      error={action.error?.message} onConfirm={() => action.mutate(target)} />;
  }
  return <section className="account-surface flex flex-col items-start justify-between gap-5" aria-labelledby="session-card-title">
    <h2 id="session-card-title" className="flex items-center gap-2 font-semibold"><Laptop aria-hidden="true" className="size-5" />{canRead ? '当前会话' : '会话管理'}</h2>
    <div><p className="text-xl font-medium">{!canRead ? '管理登录状态' : query.error ? '暂时无法读取' : current ? clientLabel(current.user_agent) : query.isPending ? '正在读取…' : '未找到当前会话'}</p>
      {current && canRead && !query.error && <p className="mt-1 text-sm text-muted-foreground">最近活动 {date(current.last_activity_at)}</p>}
    </div>
    <Dialog open={open} onOpenChange={next => { if (!action.isPending) setOpen(next); }}>
      <DialogTrigger asChild><Button variant="outline">管理会话</Button></DialogTrigger>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader>
        <DialogTitle>登录会话</DialogTitle><DialogDescription>{canRead ? '浏览器、来源 IP 和活动时间可帮助你辨认登录会话。' : '退出全部设备后需要重新登录。'}</DialogDescription>
      </DialogHeader>
      {canRead && <><div className="flex justify-end"><Button variant="ghost" size="sm" disabled={query.isFetching || action.isPending} onClick={() => void refresh()}><RefreshCw aria-hidden="true" />刷新会话与记录</Button></div>
        <FormNotice error>{query.error?.message}</FormNotice>
        {query.isPending && <p role="status" className="text-sm">正在读取会话…</p>}
        {query.data && !sessions.length && <p className="text-sm text-muted-foreground">没有可显示的登录会话。</p>}
        <ul className="min-w-0 divide-y" aria-label="登录会话">{page.items.map(session => <li key={session.id} className="flex flex-wrap items-start gap-2 py-2">
          <SessionInfo session={session} />
          {(session.current || canRevoke) && <div className="py-2">{revokeButton(session.current ? { kind: 'current' } : { kind: 'revoke', id: session.id })}</div>}
        </li>)}</ul>
        {query.data && <ListPagination label="登录会话" {...page.pagination} />}
      </>}
      {canRevoke && <div className="flex flex-wrap gap-2 border-t pt-4">
        {canRead && revokeButton({ kind: 'others' }, !sessions.some(item => !item.current))}
        {revokeButton({ kind: 'all' })}
      </div>}
      </DialogContent>
    </Dialog>
  </section>;
}
