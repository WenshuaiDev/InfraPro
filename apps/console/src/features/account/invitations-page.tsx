import { useRef, useState } from 'react';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from '../../config/validation';
import { Copy, Plus, RefreshCw } from 'lucide-react';
import { ApiError, Permissions as P, type Invitation, type LinkResult } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@infrapro/ui/components/card';
import { Form } from '@infrapro/ui/components/form';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { date, setFormError } from '../auth/forms';
import { useOperation } from './use-operation';

const statuses = { pending: '待使用', used: '已使用', expired: '已过期', revoked: '已撤销' };
const schema = z.object({ note: z.string().refine((value) => Array.from(value).length <= 200, '用途备注不能超过 200 个字符。') });
const confirmationSchema = z.object({ current_password: z.string().min(1, '请输入当前密码。') });

export function InvitationsPage() {
  const { client, account, phase, can, canAll } = useIdentity();
  const queryClient = useQueryClient();
  const queryKey = ['account', account?.id, 'invitations'];
  const list = useInfiniteQuery({ queryKey, initialPageParam: '', queryFn: ({ pageParam, signal }) => client.invitations(pageParam, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled: phase === 'authenticated' });
  const [link, setLink] = useState<LinkResult>();
  const [copyMessage, setCopyMessage] = useState('');
  const [passwordConfirmed, setPasswordConfirmed] = useState(false);
  const [confirmation, setConfirmation] = useState<string | null>(null);
  const createButton = useRef<HTMLButtonElement>(null);
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { note: '' } });
  const passwordForm = useForm({ resolver: zodResolver(confirmationSchema), defaultValues: { current_password: '' } });
  const generate = useOperation({
    mutationFn: ({ id }: { id: string }) => id ? client.regenerateInvitation(id) : client.createInvitation(form.getValues('note')),
    onSuccess: async (result) => { setLink(result); setCopyMessage(''); setConfirmation(null); setPasswordConfirmed(false); passwordForm.reset(); await queryClient.invalidateQueries({ queryKey }); },
    onError: (error, { id }) => { if (error instanceof ApiError && error.code === 'reauthentication_required') setConfirmation(id); },
  });
  const confirmPassword = useOperation({ mutationFn: (password: string) => client.reauthenticate(password), onSuccess: () => { passwordForm.reset(); setPasswordConfirmed(true); generate.reset(); } });
  const revoke = useOperation({ mutationFn: (id: string) => client.revokeInvitation(id), onSuccess: async () => { setLink(undefined); await queryClient.invalidateQueries({ queryKey }); } });
  const busy = generate.isPending || revoke.isPending || confirmPassword.isPending;
  const needsConfirmation = generate.error instanceof ApiError && generate.error.code === 'reauthentication_required';
  const columns: ColumnDef<Invitation>[] = [
    { accessorKey: 'note', header: '用途备注', cell: ({ row }) => <span className="block min-w-32 max-w-xs whitespace-normal break-words font-medium">{row.original.note || '未填写备注'}</span> },
    { accessorKey: 'status', header: '状态', cell: ({ row }) => <Badge variant={row.original.status === 'pending' ? 'secondary' : 'outline'}>{statuses[row.original.status]}</Badge> },
    { accessorKey: 'created_at', header: '创建时间', cell: ({ row }) => date(row.original.created_at) },
    { accessorKey: 'expires_at', header: '有效期至', cell: ({ row }) => date(row.original.expires_at) },
    { id: 'actions', header: '操作', cell: ({ row: { original: item } }) => <div className="flex gap-2">
      {item.status !== 'used' && canAll(P.PlatformInvitationCreate, P.PlatformInvitationRegenerate) && <Button variant="outline" size="sm" disabled={busy} onClick={() => { generate.reset(); generate.mutate({ id: item.id }); }}>重新生成</Button>}
      {item.status === 'pending' && can(P.PlatformInvitationRevoke) && <Button variant="ghost" size="sm" className="text-destructive" disabled={busy} onClick={() => revoke.mutate(item.id)}>撤销</Button>}
      {item.status === 'used' && <span className="text-muted-foreground">已完成开户</span>}
    </div> },
  ];
  return <><PageHeader title="邀请管理" description="邀请同事加入，并查看自己发出的邀请。" />
    {can(P.PlatformInvitationCreate) && <Card className="mb-6 max-w-3xl"><CardHeader><CardTitle>邀请同事加入</CardTitle><CardDescription>每条链接仅能创建一个基础成员账号，不继承邀请人的角色。持有链接的人可以认领，请妥善保管；疑似泄露时立即撤销。</CardDescription></CardHeader><CardContent className="space-y-6">
      <Form {...form}><form className="space-y-4" noValidate aria-busy={generate.isPending} onSubmit={form.handleSubmit(async () => { try { await generate.mutateAsync({ id: '' }); } catch (error) { if (!(error instanceof ApiError && error.code === 'reauthentication_required')) setFormError(form, error); } })}>
        <FormInput control={form.control} name="note" label="用途备注" placeholder="例如：桥梁监测项目同事" hint="可选，最多 200 个字符。备注不绑定收件人身份。" />
        <FormNotice error>{form.formState.errors.root?.server?.message}</FormNotice><Button ref={createButton} type="submit" disabled={busy}><Plus />{generate.isPending ? '正在处理…' : '生成邀请链接'}</Button>
      </form></Form>
      {confirmation !== null && <section aria-labelledby="reauth-title" className="rounded-lg border bg-muted/30 p-4"><h2 id="reauth-title" className="font-medium">确认当前密码</h2><p className="mb-4 mt-1 text-sm text-muted-foreground">生成邀请需要最近 5 分钟内验证过密码。</p>
        {!passwordConfirmed && <Form {...passwordForm}><form className="space-y-4" noValidate onSubmit={passwordForm.handleSubmit(async (values) => { try { await confirmPassword.mutateAsync(values.current_password); } catch (error) { setFormError(passwordForm, error); } })}>
          <FormInput control={passwordForm.control} name="current_password" label="当前密码" type="password" autoComplete="current-password" autoFocus />
          <FormNotice error>{passwordForm.formState.errors.root?.server?.message}</FormNotice><div className="flex flex-wrap gap-2"><Button disabled={busy}>确认密码</Button><Button type="button" variant="outline" disabled={busy} onClick={() => { setConfirmation(null); setPasswordConfirmed(false); passwordForm.reset(); generate.reset(); createButton.current?.focus(); }}>取消</Button></div>
        </form></Form>}
        {passwordConfirmed && <div className="space-y-3"><FormNotice>密码已确认，请再次提交生成请求。</FormNotice><Button disabled={busy} onClick={() => generate.mutate({ id: confirmation })}>重新提交生成</Button></div>}
      </section>}
      <FormNotice error>{revoke.error?.message || (!needsConfirmation && generate.error?.message)}</FormNotice>
      {link && <section className="space-y-3 rounded-lg border bg-muted/30 p-4" aria-labelledby="invitation-result"><div><h2 id="invitation-result" className="font-medium">邀请链接已生成</h2><p className="mt-1 text-sm text-muted-foreground">有效至 {date(link.expires_at)}。完整链接仅在本次生成后显示。</p></div>
        <div className="grid gap-2"><Label htmlFor="invitation-url">可手动复制的完整链接</Label><Input id="invitation-url" value={link.url} readOnly onFocus={(event) => event.currentTarget.select()} /></div>
        <Button variant="outline" onClick={() => { if (!navigator.clipboard?.writeText) { setCopyMessage('自动复制不可用，请选中上方完整链接手动复制。'); return; } void navigator.clipboard.writeText(link.url).then(() => setCopyMessage('已复制邀请链接。'), () => setCopyMessage('自动复制失败，请选中上方完整链接手动复制。')); }}><Copy />复制链接</Button>
        <FormNotice>{copyMessage || '如自动复制不可用，可选中上方链接后手动复制。'}</FormNotice>
      </section>}
    </CardContent></Card>}
    <section className="space-y-4" aria-labelledby="invitations-title"><div className="flex items-center justify-between gap-3"><h2 id="invitations-title" className="text-lg font-medium">我的邀请</h2><Button variant="outline" disabled={list.isFetching || busy} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin' : undefined} />刷新列表</Button></div>
      <DataTable data={list.data?.pages.flatMap((page) => page.items) ?? []} columns={columns} getRowId={(row) => row.id} label="我的邀请" loading={list.isPending} error={list.error?.message} onRetry={() => void list.refetch()} emptyLabel="还没有邀请。生成链接后会在这里显示状态。" hasMore={list.hasNextPage} loadingMore={list.isFetchingNextPage} onLoadMore={() => void list.fetchNextPage()} loadMoreLabel="加载更多邀请" />
    </section>
  </>;
}
