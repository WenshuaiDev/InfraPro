import { TenantLifecycle } from './lifecycle';
import { TenantAuthorizationView } from './role-components';
import { useState } from 'react';
import { WorkspacePickerButton } from '../../app/workspace-picker-context';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { ApiError, Permissions as P, type Tenant, type TenantProfile } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Badge } from '@infrapro/ui/components/badge';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { tenantActions } from './labels';
import { TenantEditor } from './editor';
import { useTenant, useOptionalTenant } from './context';
import { TenantPermissions } from './shared';

const tenantDate = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
function Availability({ value }: { value: TenantProfile }) { return <Badge variant={value.administrator_available ? 'outline' : 'secondary'} className="whitespace-normal">{value.administrator_available ? '正常' : '管理员不可用，待处理'}</Badge>; }
export function PlatformTenantPage({ id }: { id: string }) {
  const { account, client, can } = useIdentity();
  const detail = useQuery({ queryKey: ['platform', account?.id, 'tenant', id], queryFn: ({ signal }) => client.tenant(id, true, signal), enabled: can(P.PlatformTenantRead), refetchOnWindowFocus: true });
  if (!can(P.PlatformTenantRead)) return <FormNotice>打开详情还需要租户查看权限。编辑或交接权限不会自动包含查看权限。</FormNotice>;
  if (detail.error && (!detail.data || (detail.error instanceof ApiError && [401, 403, 404].includes(detail.error.status ?? 0)))) return <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />;
  if (!detail.data) return <p role="status">正在读取租户档案…</p>;
  return <><PageHeader title={detail.data.name} description={`平台管理 · ${detail.data.code}`} /><div className="space-y-5">{detail.error && <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />}<TenantLifecycle key={detail.data.id} tenant={detail.data} blocked={!!detail.error} retry={() => void detail.refetch()} /><TenantDetails value={detail.data} platform blocked={!!detail.error} retry={() => void detail.refetch()} /></div>{can(P.PlatformAuditRead) && <section className="mt-6"><h2 className="mb-4 text-lg font-semibold">平台租户操作记录</h2><TenantAudit id={id} platform /></section>}</>;
}
export function TenantProfilePage() {
  const ctx = useTenant(); const { account, client } = useIdentity();
  const query = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'profile'], queryFn: ({ signal }) => client.tenant(ctx.tenant.id, false, signal), enabled: ctx.available && ctx.permissions.includes(P.TenantProfileRead), refetchInterval: 30_000 });
  if (!ctx.permissions.includes(P.TenantProfileRead)) return <><PageHeader title="本人租户授权" description={ctx.tenant.name} /><FormNotice>当前未获资料查看权限，请联系租户管理员。</FormNotice><TenantAuthorizationView value={ctx.authorization} /><WorkspacePickerButton filter="tenants">重新选择租户</WorkspacePickerButton></>;
  return <><PageHeader title="租户资料" description={`${ctx.tenant.name} · ${ctx.tenant.code}`} />{query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}{query.data ? <TenantDetails value={query.data} /> : <p role="status">正在读取资料…</p>}<div className="mt-6"><TenantPermissions /></div></>;
}
function TenantDetails({ value: t, platform = false, blocked = false, retry }: { value: TenantProfile | Tenant; platform?: boolean; blocked?: boolean; retry?: () => void }) {
  const { can } = useIdentity(); const ctx = useOptionalTenant();
  return <section className="space-y-5 rounded-xl border p-5"><div className="flex flex-wrap gap-3">{(platform ? can(P.PlatformTenantUpdate) : ctx?.permissions.includes(P.TenantProfileUpdate)) && <TenantEditor mode="edit" tenant={t} platform={platform} blocked={blocked} retry={retry} trigger={<Button variant="outline">编辑资料</Button>} />}{platform && can(P.PlatformTenantTransferAdmin) && <TenantEditor mode="transfer" tenant={t} blocked={blocked} retry={retry} trigger={<Button variant="outline">交接管理员</Button>} />}</div><dl className="grid gap-5 sm:grid-cols-2"><div><dt className="text-sm text-muted-foreground">租户名称</dt><dd className="mt-1 break-words">{t.name}</dd></div><div><dt className="text-sm text-muted-foreground">租户编号</dt><dd className="mt-1 break-all">{t.code}</dd></div><div className="sm:col-span-2"><dt className="text-sm text-muted-foreground">说明</dt><dd className="mt-1 whitespace-pre-wrap break-words">{t.description || '暂无说明'}</dd></div><div><dt className="text-sm text-muted-foreground">当前管理员</dt><dd className="mt-1 break-words">{t.administrator.display_name}（{t.administrator.username}）</dd></div><div><dt className="mb-1 text-sm text-muted-foreground">管理员可用性</dt><dd><Availability value={t} /></dd></div>{'created_at' in t && <div><dt className="text-sm text-muted-foreground">创建时间（Asia/Shanghai）</dt><dd className="mt-1 text-sm">{tenantDate(t.created_at)}</dd></div>}<div><dt className="text-sm text-muted-foreground">修改时间（Asia/Shanghai）</dt><dd className="mt-1 text-sm">{tenantDate(t.updated_at)}</dd></div>{'created_by' in t && <div><dt className="text-sm text-muted-foreground">创建账号</dt><dd className="mt-1 break-all text-xs">{t.created_by}</dd></div>}{'updated_by' in t && <div><dt className="text-sm text-muted-foreground">最近修改账号</dt><dd className="mt-1 break-all text-xs">{t.updated_by}</dd></div>}</dl></section>;
}
export function TenantAuditPage() { const ctx = useTenant(); if (!ctx.permissions.includes(P.TenantAuditRead)) return <FormNotice error>403 · 当前角色无权查看租户操作记录。</FormNotice>; return <><PageHeader title="租户操作记录" description={`${ctx.tenant.name} · ${ctx.tenant.code} · Asia/Shanghai`} /><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} /></>; }
export function TenantAudit({ id, platform = false, membership, objectType, objectId }: { id: string; platform?: boolean; membership?: string; objectType?: string; objectId?: string }) {
  const { account, client } = useIdentity(); const [event, setEvent] = useState(''); const [q, setQ] = useState(''); const [from, setFrom] = useState(''); const [to, setTo] = useState('');
  const prefix = platform ? ['platform', account?.id, 'tenant-audit', id] : ['tenant', account?.id, id, membership, 'audit'];
  const list = useInfiniteQuery({ queryKey: [...prefix, event, q, from, to, objectType, objectId], initialPageParam: '', queryFn: ({ pageParam, signal }) => { const filter = { object_type: objectType, object_id: objectId, event_type: event, q, from: from ? new Date(`${from}+08:00`).toISOString() : '', to: to ? new Date(`${to}+08:00`).toISOString() : '', cursor: pageParam, limit: 20 }; return platform ? client.accessAudit({ ...filter, tenant_id: id }, signal) : client.tenantAudit(id, filter, signal); }, getNextPageParam: (p) => p.next_cursor || undefined });
  return <section className="space-y-4"><div className="flex flex-wrap gap-4"><div className="space-y-2"><Label htmlFor="tenant-audit-q">搜索操作或原因</Label><Input id="tenant-audit-q" value={q} onChange={(e) => setQ(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="tenant-audit-event">事件类型</Label><select id="tenant-audit-event" className="h-11 rounded-md border bg-background px-3" value={event} onChange={(e) => setEvent(e.target.value)}><option value="">全部</option>{Object.entries(tenantActions).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></div><div className="space-y-2"><Label htmlFor="tenant-audit-from">开始时间（上海）</Label><Input id="tenant-audit-from" type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="tenant-audit-to">结束时间（上海）</Label><Input id="tenant-audit-to" type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></div></div>{list.isPending && <p role="status">正在读取操作记录…</p>}{list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}
    {list.data?.pages.flatMap((p) => p.items).map((ev) => <article key={ev.id} className="space-y-3 rounded-lg border p-4"><div className="flex flex-wrap justify-between gap-2"><h3 className="font-medium">{tenantActions[ev.event_type] ?? ev.event_type}</h3><time className="text-sm text-muted-foreground">{tenantDate(ev.created_at)}</time></div><p className="text-sm">{ev.actor_scope === 'tenant' ? '租户操作' : '平台操作'} · {ev.reason}</p><p className="break-all text-xs text-muted-foreground">操作者：{ev.actor_type === 'maintenance' ? '主机维护' : ev.actor_type === 'release' ? '发布同步' : ev.actor_id}</p><details><summary className="cursor-pointer text-sm">查看前后内容与操作者快照</summary><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted/40 p-3 text-xs">{JSON.stringify(ev.details, null, 2)}</pre></details></article>)}
    {list.data && !list.error && !list.data.pages[0]?.items.length && <p>没有符合条件的操作记录。</p>}{list.hasNextPage && <Button variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多记录</Button>}
  </section>;
}
