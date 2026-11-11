import { useState } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import type { ColumnDef } from '@tanstack/react-table';
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { RefreshCw } from 'lucide-react';
import { Permissions as P, type SecurityEvent, type Session } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { Separator } from '@infrapro/ui/components/separator';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { AccountSettings } from '../../app/workspace-shell';
import { date, passwordSchema, setFormError } from '../auth/forms';
import { useOperation } from './use-operation';

const events: Record<string, string> = { account_authorization_changed: '本人角色权限变化', initial_role_bound: '开户角色绑定', account_roles_changed: '本人角色绑定变化', bootstrap: '首次账号创建', login: '登录', logout: '退出登录', logout_all: '退出全部设备', session_revoked: '会话撤销', password_changed: '修改密码', password_reset: '密码重置', reset_link_created: '生成重置链接', reauthentication: '密码确认', invitation_created: '创建邀请', invitation_revoked: '撤销邀请', invitation_accepted: '邀请已被接受', account_created: '账号创建', account_disabled: '账号停用', account_emergency_disabled: '应急停用', account_enabled: '账号恢复', profile_updated: '资料修改' };
const outcomes: Record<string, string> = { success: '成功', failure: '失败', rate_limited: '尝试受限' };

function PasswordForm() {
  const { client, help, signedOut } = useIdentity();
  const navigate = useNavigate();
  const min = help?.password_min ?? 8; const max = help?.password_max ?? 128;
  const schema = z.object({ current_password: z.string().min(1, '请输入当前密码。'), new_password: passwordSchema(min, max), confirm: z.string() }).refine((values) => values.new_password === values.confirm, { path: ['confirm'], message: '两次输入的密码不一致。' });
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { current_password: '', new_password: '', confirm: '' } });
  const password = useWatch({ control: form.control, name: 'new_password' });
  const save = useOperation({ mutationFn: (values: z.infer<typeof schema>) => client.changePassword({ current_password: values.current_password, new_password: values.new_password }), onSuccess: () => { form.reset(); signedOut(); void navigate({ to: '/login', search: { notice: 'password' }, replace: true }); } });
  return <section className="max-w-2xl space-y-5" aria-labelledby="password-title"><div><h2 id="password-title" className="text-lg font-medium">修改密码</h2><p className="mt-1 text-sm text-muted-foreground">修改后所有设备都需要重新登录，已发出的有效邀请会保留。</p></div>
    <Form {...form}><form className="space-y-5" noValidate aria-busy={save.isPending} onSubmit={form.handleSubmit(async (values) => { try { await save.mutateAsync(values); } catch (error) { setFormError(form, error); } })}>
      <FormInput control={form.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" />
      <FormInput control={form.control} name="new_password" label="新密码" type="password" autoComplete="new-password" hint={`${min}–${max} 个字符，允许空格和中文。当前 ${Array.from(password).length} 个字符。`} />
      <FormInput control={form.control} name="confirm" label="确认新密码" type="password" autoComplete="new-password" />
      <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><Button disabled={save.isPending}>{save.isPending ? '正在修改…' : '修改密码并退出'}</Button>
    </form></Form>
  </section>;
}

type SessionAction = { kind: 'current' | 'others' | 'all' } | { kind: 'revoke'; id: string };

export function SecurityPage() {
  const { client, account, phase, signedOut, can } = useIdentity();
  const queryClient = useQueryClient();
  const sessionsKey = ['account', account?.id, 'sessions'];
  const eventsKey = ['account', account?.id, 'security-events'];
  const sessions = useQuery({ queryKey: sessionsKey, queryFn: ({ signal }) => client.sessions(signal), enabled: phase === 'authenticated' && can(P.AccountSessionRead) });
  const records = useInfiniteQuery({ queryKey: eventsKey, initialPageParam: '', queryFn: ({ pageParam, signal }) => client.securityEvents(pageParam, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: phase === 'authenticated' && can(P.AccountSecurityEventRead) });
  const [confirmAll, setConfirmAll] = useState(false);
  const navigate = useNavigate();
  const action = useOperation({
    mutationFn: async (input: SessionAction) => {
      // The current cookie can have changed in another tab since this list loaded.
      if (input.kind === 'current') await client.logout();
      else if (input.kind === 'all') await client.logoutAll();
      else if (input.kind === 'revoke') await client.revokeSession(input.id);
      else for (const session of sessions.data?.items.filter((item) => !item.current) ?? []) await client.revokeSession(session.id);
    },
    onSuccess: async (_, input) => {
      if (input.kind === 'all' || input.kind === 'current') { signedOut(); void navigate({ to: '/login', replace: true }); }
      else await Promise.all([queryClient.invalidateQueries({ queryKey: sessionsKey }), queryClient.invalidateQueries({ queryKey: eventsKey })]);
    },
    // A batch may partially succeed; refresh so the user can retry only what remains.
    onError: async () => { await queryClient.invalidateQueries({ queryKey: sessionsKey }); },
  });
  const sessionColumns: ColumnDef<Session>[] = [
    { id: 'device', size: 320, header: '会话', cell: ({ row: { original: session } }) => <div className="min-w-48 max-w-sm space-y-2 whitespace-normal break-words"><Badge variant={session.current ? 'secondary' : 'outline'}>{session.current ? '当前会话' : '其他会话'}</Badge><p className="text-sm">{session.user_agent || '未知浏览器'}</p><p className="text-xs text-muted-foreground">IP：{session.ip}</p></div> },
    { id: 'time', size: 272, header: '活动时间', cell: ({ row: { original: session } }) => <dl className="space-y-1 text-xs"><div><dt className="inline text-muted-foreground">创建：</dt><dd className="inline">{date(session.created_at)}</dd></div><div><dt className="inline text-muted-foreground">最近活动：</dt><dd className="inline">{date(session.last_activity_at)}</dd></div><div><dt className="inline text-muted-foreground">最晚到期：</dt><dd className="inline">{date(session.expires_at)}</dd></div></dl> },
    { id: 'actions', size: 176, header: '操作', cell: ({ row: { original: session } }) => (session.current || can(P.AccountSessionRevoke)) ? <Button variant="outline" size="sm" className="text-destructive" disabled={action.isPending} onClick={() => action.mutate(session.current ? { kind: 'current' } : { kind: 'revoke', id: session.id })}>{session.current ? '退出当前会话' : '撤销会话'}</Button> : null },
  ];
  const eventColumns: ColumnDef<SecurityEvent>[] = [
    { accessorKey: 'event_type', size: 184, header: '操作', cell: ({ row }) => events[row.original.event_type] || row.original.event_type },
    { accessorKey: 'outcome', size: 104, header: '结果', cell: ({ row }) => <Badge variant={row.original.outcome === 'success' ? 'outline' : 'destructive'}>{outcomes[row.original.outcome] || row.original.outcome}</Badge> },
    { accessorKey: 'created_at', size: 184, header: '时间', cell: ({ row }) => date(row.original.created_at) },
    { id: 'source', size: 320, header: '来源', cell: ({ row }) => <div className="min-w-56 max-w-sm whitespace-normal break-words"><p>{row.original.source_ip || '服务器维护操作'}</p><p className="mt-1 text-xs text-muted-foreground">{row.original.user_agent}</p></div> },
  ];
  return <><PageHeader title="账号安全" description="管理密码、登录会话与近期安全记录。" /><AccountSettings>
    {can(P.AccountPasswordUpdate) && <><PasswordForm /><Separator /></>}
    {can(P.AccountSessionRead) && <section className="space-y-4" aria-labelledby="sessions-title"><div className="flex flex-wrap items-center justify-between gap-3"><h2 id="sessions-title" className="text-lg font-medium">登录会话</h2><Button variant="outline" disabled={sessions.isFetching || records.isFetching || action.isPending} onClick={() => { if (can(P.AccountSessionRead)) void sessions.refetch(); if (can(P.AccountSecurityEventRead)) void records.refetch(); }}><RefreshCw />刷新会话与记录</Button></div><p className="text-sm text-muted-foreground">以下浏览器和 IP 信息仅用于辨认会话，不能准确识别物理设备或定位。</p>
      <FormNotice error>{!confirmAll && action.error?.message}</FormNotice>
      <DataTable data={sessions.data?.items ?? []} columns={sessionColumns} getRowId={(row) => row.id} label="登录会话" loading={sessions.isPending} error={sessions.error?.message} onRetry={() => void sessions.refetch()} emptyLabel="没有可显示的登录会话。" />
      {can(P.AccountSessionRevoke) && <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={action.isPending || !sessions.data?.items.some((session) => !session.current)} onClick={() => action.mutate({ kind: 'others' })}>退出其他设备</Button>
        <ConfirmDialog trigger={<Button variant="destructive" disabled={action.isPending}>退出全部设备</Button>} open={confirmAll} onOpenChange={(open) => { action.reset(); setConfirmAll(open); }} title="退出全部设备？" description="包括当前浏览器在内的所有登录会话都会失效，需要重新登录。" confirmLabel="确认退出全部设备" destructive pending={action.isPending} error={action.error?.message} onConfirm={() => action.mutate({ kind: 'all' })} />
      </div>}
    </section>}<Separator />
    {can(P.AccountSessionRevoke) && !can(P.AccountSessionRead) && <ConfirmDialog trigger={<Button variant="destructive" disabled={action.isPending}>退出全部设备</Button>} open={confirmAll} onOpenChange={setConfirmAll} title="退出全部设备？" description="包括当前浏览器在内的所有登录会话都会失效。" confirmLabel="确认退出全部设备" destructive pending={action.isPending} error={action.error?.message} onConfirm={() => action.mutate({ kind: 'all' })} />}
    {can(P.AccountSecurityEventRead) && <section className="space-y-4" aria-labelledby="events-title"><h2 id="events-title" className="text-lg font-medium">近期安全记录</h2><DataTable data={records.data?.pages.flatMap((page) => page.items) ?? []} columns={eventColumns} getRowId={(row) => String(row.id)} label="近期安全记录" loading={records.isPending} error={records.error?.message} onRetry={() => void records.refetch()} emptyLabel="暂无安全记录。" hasMore={records.hasNextPage} loadingMore={records.isFetchingNextPage} onLoadMore={() => void records.fetchNextPage()} loadMoreLabel="加载更多记录" /></section>}
  </AccountSettings></>;
}
