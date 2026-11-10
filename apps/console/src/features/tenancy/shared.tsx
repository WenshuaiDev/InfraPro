import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type TenantAccount } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from './context';

export function CandidatePicker({ value, onChange }: { value?: TenantAccount; onChange: (value: TenantAccount) => void }) {
  const { account, client } = useIdentity(); const [q, setQ] = useState('');
  const list = useInfiniteQuery({ queryKey: ['platform', account?.id, 'tenant-candidates', q], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenantCandidates({ q, cursor: pageParam, limit: 20 }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: Array.from(q.trim()).length >= 2 });
  return <section className="space-y-3"><Label htmlFor="tenant-admin-search">搜索接任账号</Label><Input id="tenant-admin-search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="输入至少 2 个字符" /><p className="text-sm text-muted-foreground">仅选择已有正常账号；将授予本租户管理员权限。</p>
    {value && <p className="rounded-md bg-muted p-3 text-sm" role="status">已选择：{value.display_name}（{value.username}）</p>}
    {list.isFetching && <p role="status">正在查找账号…</p>}{list.error && <FormNotice error>{list.error.message}<Button type="button" variant="link" onClick={() => void list.refetch()}>重试搜索</Button></FormNotice>}
    {Array.from(q.trim()).length >= 2 && list.data && <div className="max-h-52 space-y-2 overflow-y-auto">{list.data.pages.flatMap((p) => p.items).map((a) => <Button key={a.id} type="button" variant={value?.id === a.id ? 'secondary' : 'outline'} className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" onClick={() => onChange(a)}>{a.display_name}（{a.username}）</Button>)}{!list.data.pages[0]?.items.length && <p className="text-sm">没有符合条件的正常账号</p>}{list.hasNextPage && <Button type="button" variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多账号</Button>}</div>}
  </section>;
}
export function TenantPermissions() {
  const ctx = useTenant(); const labels = { [P.TenantProfileRead]: '查看租户资料', [P.TenantProfileUpdate]: '修改租户资料', [P.TenantAuditRead]: '查看租户操作记录' };
  return <details className="rounded-lg border p-4"><summary className="cursor-pointer font-medium">当前租户权限</summary><div className="mt-4 space-y-3 text-sm"><p>{ctx.tenant.name} · {ctx.tenant.code}</p><p className="break-all text-muted-foreground">成员关系：{ctx.membership_id}</p><p>角色：{ctx.roles.map((r) => `${r.name}（租户范围）`).join('、')}</p><ul className="space-y-2">{ctx.permissions.map((p) => <li key={p} className="break-words">{labels[p as keyof typeof labels] ?? p} · 来源：{ctx.permission_sources[p]?.map((id) => ctx.roles.find((r) => r.id === id)?.name ?? id).join('、')}</li>)}</ul></div></details>;
}
