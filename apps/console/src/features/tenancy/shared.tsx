import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type TenantAdministratorCandidate } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from './context';

export function CandidatePicker({ value, onChange, tenantId }: { value?: TenantAdministratorCandidate; onChange: (value: TenantAdministratorCandidate) => void; tenantId?: string }) {
  const { account, client } = useIdentity(); const [q, setQ] = useState('');
  const list = useInfiniteQuery({ queryKey: ['platform', account?.id, 'tenant-candidates', tenantId, q], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenantCandidates({ q, tenant_id: tenantId, cursor: pageParam, limit: 20 }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: Array.from(q.trim()).length >= 2 });
  return <section className="space-y-3"><Label htmlFor="tenant-admin-search">搜索接任账号</Label><Input id="tenant-admin-search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="输入至少 2 个字符" /><p className="text-sm text-muted-foreground">仅选择已有正常账号；将授予本租户管理员权限。</p>
    {value && <p className="rounded-md bg-muted p-3 text-sm" role="status">已选择：{value.display_name}（{value.username}）{tenantId && <span className="block">{value.transfer_mode === 'reuse' ? '接任已有成员：保留成员关系和加入时间，改授管理员角色。' : value.transfer_mode === 'resume_required' ? '该成员已暂停，需先恢复或另选账号。' : '将新建成员关系并授予管理员角色。'}原管理员的成员关系将结束。{value.replaced_roles.length > 0 && <span className="block">接任人原角色将全部撤销：{value.replaced_roles.join('、')}。</span>}</span>}</p>}
    {list.isFetching && <p role="status">正在查找账号…</p>}{list.error && <FormNotice error>{list.error.message}<Button type="button" variant="link" onClick={() => void list.refetch()}>重试搜索</Button></FormNotice>}
    {Array.from(q.trim()).length >= 2 && list.data && <div className="max-h-52 space-y-2 overflow-y-auto">{list.data.pages.flatMap((p) => p.items).map((a) => <Button key={a.id} type="button" variant={value?.id === a.id ? 'secondary' : 'outline'} className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" onClick={() => onChange(a)}>{a.display_name}（{a.username}）</Button>)}{!list.data.pages[0]?.items.length && <p className="text-sm">没有符合条件的正常账号</p>}{list.hasNextPage && <Button type="button" variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多账号</Button>}</div>}
  </section>;
}
export function TenantPermissions() {
  const ctx = useTenant(); const labels = { [P.TenantProfileRead]: '查看租户资料', [P.TenantProfileUpdate]: '修改租户资料', [P.TenantAuditRead]: '查看租户操作记录', [P.TenantMemberRead]: '查看租户成员', [P.TenantMemberSuspend]: '暂停租户成员', [P.TenantMemberResume]: '恢复租户成员', [P.TenantMemberRemove]: '移除租户成员', [P.TenantInvitationRead]: '查看成员邀请', [P.TenantInvitationCreate]: '创建成员邀请', [P.TenantInvitationRevoke]: '撤销成员邀请', [P.TenantInvitationRegenerate]: '重新生成成员邀请' };
  return <details className="rounded-lg border p-4"><summary className="cursor-pointer font-medium">当前租户权限</summary><div className="mt-4 space-y-3 text-sm"><p>{ctx.tenant.name} · {ctx.tenant.code}</p><p className="break-all text-muted-foreground">成员关系：{ctx.membership_id}</p><p>角色：{ctx.roles.map((r) => `${r.name}（租户范围）`).join('、')}</p><ul className="space-y-2">{ctx.permissions.map((p) => <li key={p} className="break-words">{labels[p as keyof typeof labels] ?? p} · 来源：{ctx.permission_sources[p]?.map((id) => ctx.roles.find((r) => r.id === id)?.name ?? id).join('、')}</li>)}</ul></div></details>;
}
