import { useEffect, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import type { ColumnDef } from '@tanstack/react-table';
import { Permissions as P, type Tenant } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Badge } from '@infrapro/ui/components/badge';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { tenantActions } from './labels';
import { TenantEditor } from './editor';
import { useTenant, useOptionalTenant } from './context';
import { TenantPermissions } from './shared';

const tenantDate = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
export function PlatformTenantsPage() {
  const { client, account, can } = useIdentity(); const [q, setQ] = useState(''); const [unavailable, setUnavailable] = useState(false);
  const read = can(P.PlatformTenantRead);
  const list = useInfiniteQuery({ queryKey: ['platform', account?.id, 'tenants', q, unavailable], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.tenants({ q, administrator_unavailable: unavailable, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: read });
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const columns: ColumnDef<Tenant>[] = [{ id: 'tenant', header: '租户', cell: ({ row }) => <TenantLink value={row.original} /> }, { id: 'administrator', header: '当前管理员', cell: ({ row }) => <p className="whitespace-normal">{row.original.administrator.display_name}（{row.original.administrator.username}）</p> }, { id: 'available', header: '管理员可用性', cell: ({ row }) => <Availability value={row.original} /> }];
  return <><PageHeader title="租户管理" description="平台管理 · 建立租户档案、指定和交接管理员。" actions={can(P.PlatformTenantCreate) ? <TenantEditor mode="create" trigger={<Button>创建租户</Button>} /> : undefined} />
    {!read ? <FormNotice>当前未配置租户查看权限。创建能力可单独使用；编辑及交接页面还需要配套的租户查看权限。</FormNotice> : <>
      <div className="mb-5 flex flex-wrap items-end gap-4"><div className="w-full max-w-md space-y-2"><Label htmlFor="tenant-search">搜索名称或当前 / 历史编号</Label><Input id="tenant-search" value={q} onChange={(e) => setQ(e.target.value)} /></div><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={unavailable} onChange={(e) => setUnavailable(e.target.checked)} />仅显示管理员不可用</label></div>
      <div className="hidden md:block"><DataTable label="平台租户列表" columns={columns} data={items} getRowId={(t) => t.id} loading={list.isPending} error={list.error?.message} onRetry={() => void list.refetch()} emptyLabel={q || unavailable ? '没有符合筛选条件的租户' : '平台尚无租户'} /></div>
      <div className="space-y-3 md:hidden">{list.isPending && <p role="status">正在读取租户…</p>}{list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}{items.map((t) => <article key={t.id} className="space-y-3 rounded-lg border p-4"><TenantLink value={t} /><p className="break-words text-sm">管理员：{t.administrator.display_name}（{t.administrator.username}）</p><Availability value={t} /></article>)}{!list.isPending && !list.error && !items.length && <p>{q || unavailable ? '没有符合筛选条件的租户' : '平台尚无租户'}</p>}</div>
      {list.hasNextPage && <Button className="mt-4" variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多租户</Button>}
    </>}
  </>;
}
function TenantLink({ value }: { value: Tenant }) { return <div className="min-w-0 space-y-1"><Link to="/platform/tenants/$tenantId" params={{ tenantId: value.id }} className="inline-block min-h-11 break-words font-medium underline underline-offset-4">{value.name}</Link><p className="break-all text-sm text-muted-foreground">{value.code}</p></div>; }
function Availability({ value }: { value: Tenant }) { return <Badge variant={value.administrator_available ? 'outline' : 'secondary'} className="whitespace-normal">{value.administrator_available ? '正常' : '管理员不可用，待处理'}</Badge>; }
export function MyTenantsPage() {
  const { account, client, authorization_revision } = useIdentity(); const [q, setQ] = useState('');
  const list = useInfiniteQuery({ queryKey: ['account', account?.id, 'my-tenants', q], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.myTenants({ q, cursor: pageParam }, signal), getNextPageParam: (p) => p.next_cursor || undefined, refetchInterval: 30_000, refetchOnWindowFocus: true });
  const { refetch } = list;
  useEffect(() => { void refetch(); }, [authorization_revision, refetch]);
  useEffect(() => { const refresh = () => { if (document.visibilityState === 'visible') void refetch(); }; window.addEventListener('focus', refresh); return () => window.removeEventListener('focus', refresh); }, [refetch]);
  return <><PageHeader title="我的租户" description="选择要进入的工作空间；各标签页可以独立使用不同租户。" /><div className="mb-5 max-w-md space-y-2"><Label htmlFor="my-tenant-search">搜索我的租户</Label><Input id="my-tenant-search" value={q} onChange={(e) => setQ(e.target.value)} /></div>{list.isPending && <p role="status">正在读取租户…</p>}{list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{list.data?.pages.flatMap((p) => p.items).map((t) => <article key={t.id} className="space-y-3 rounded-xl border bg-card p-5"><h2 className="break-words text-lg font-semibold">{t.name}</h2><p className="break-all text-sm text-muted-foreground">{t.code}</p><p className="text-sm">本人角色：{t.roles.map((r) => r.name).join('、')} · 仅在本租户生效</p><Button asChild><Link to="/t/$tenantId/profile" params={{ tenantId: t.id }}>进入租户</Link></Button></article>)}</div>
    {list.data && !list.error && !list.data.pages[0]?.items.length && <div className="space-y-3 rounded-lg border p-5"><p>{q ? '没有符合筛选条件的租户' : '尚未加入租户，请联系平台管理人员'}</p>{q && <Button variant="outline" onClick={() => setQ('')}>清除筛选</Button>}<Link to="/" className="block underline">返回账号首页</Link></div>}
    {list.hasNextPage && <Button className="mt-4" variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多租户</Button>}
  </>;
}
export function PlatformTenantPage({ id }: { id: string }) {
  const { account, client, can } = useIdentity();
  const detail = useQuery({ queryKey: ['platform', account?.id, 'tenant', id], queryFn: ({ signal }) => client.tenant(id, true, signal), enabled: can(P.PlatformTenantRead), refetchOnWindowFocus: true });
  if (!can(P.PlatformTenantRead)) return <FormNotice>打开详情还需要租户查看权限。编辑或交接权限不会自动包含查看权限。</FormNotice>;
  if (detail.error) return <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />;
  if (!detail.data) return <p role="status">正在读取租户档案…</p>;
  return <><PageHeader title={detail.data.name} description={`平台管理 · ${detail.data.code}`} /><TenantDetails value={detail.data} platform />{can(P.PlatformAuditRead) && <section className="mt-6"><h2 className="mb-4 text-lg font-semibold">平台租户操作记录</h2><TenantAudit id={id} platform /></section>}</>;
}
export function TenantProfilePage() {
  const ctx = useTenant(); const { account, client } = useIdentity();
  const query = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'profile'], queryFn: ({ signal }) => client.tenant(ctx.tenant.id, false, signal), enabled: ctx.available && ctx.permissions.includes(P.TenantProfileRead), refetchInterval: 30_000 });
  return <><PageHeader title="租户资料" description={`${ctx.tenant.name} · ${ctx.tenant.code}`} />{query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}{query.data ? <TenantDetails value={query.data} /> : <p role="status">正在读取资料…</p>}<div className="mt-6"><TenantPermissions /></div></>;
}
function TenantDetails({ value: t, platform = false }: { value: Tenant; platform?: boolean }) {
  const { can } = useIdentity(); const ctx = useOptionalTenant();
  return <section className="space-y-5 rounded-xl border p-5"><div className="flex flex-wrap gap-3">{(platform ? can(P.PlatformTenantUpdate) : ctx?.permissions.includes(P.TenantProfileUpdate)) && <TenantEditor mode="edit" tenant={t} platform={platform} trigger={<Button variant="outline">编辑资料</Button>} />}{platform && can(P.PlatformTenantTransferAdmin) && <TenantEditor mode="transfer" tenant={t} trigger={<Button variant="outline">交接管理员</Button>} />}</div><dl className="grid gap-5 sm:grid-cols-2"><div><dt className="text-sm text-muted-foreground">租户名称</dt><dd className="mt-1 break-words">{t.name}</dd></div><div><dt className="text-sm text-muted-foreground">租户编号</dt><dd className="mt-1 break-all">{t.code}</dd></div><div className="sm:col-span-2"><dt className="text-sm text-muted-foreground">说明</dt><dd className="mt-1 whitespace-pre-wrap break-words">{t.description || '暂无说明'}</dd></div><div><dt className="text-sm text-muted-foreground">当前管理员</dt><dd className="mt-1 break-words">{t.administrator.display_name}（{t.administrator.username}）</dd></div><div><dt className="mb-1 text-sm text-muted-foreground">管理员可用性</dt><dd><Availability value={t} /></dd></div><div><dt className="text-sm text-muted-foreground">创建时间（Asia/Shanghai）</dt><dd className="mt-1 text-sm">{tenantDate(t.created_at)}</dd></div><div><dt className="text-sm text-muted-foreground">修改时间（Asia/Shanghai）</dt><dd className="mt-1 text-sm">{tenantDate(t.updated_at)}</dd></div><div><dt className="text-sm text-muted-foreground">创建账号</dt><dd className="mt-1 break-all text-xs">{t.created_by}</dd></div><div><dt className="text-sm text-muted-foreground">最近修改账号</dt><dd className="mt-1 break-all text-xs">{t.updated_by}</dd></div></dl></section>;
}
export function TenantAuditPage() { const ctx = useTenant(); return <><PageHeader title="租户操作记录" description={`${ctx.tenant.name} · ${ctx.tenant.code} · Asia/Shanghai`} /><TenantAudit id={ctx.tenant.id} membership={ctx.membership_id} /></>; }
function TenantAudit({ id, platform = false, membership }: { id: string; platform?: boolean; membership?: string }) {
  const { account, client } = useIdentity(); const [event, setEvent] = useState(''); const [q, setQ] = useState(''); const [from, setFrom] = useState(''); const [to, setTo] = useState('');
  const prefix = platform ? ['platform', account?.id, 'tenant-audit', id] : ['tenant', account?.id, id, membership, 'audit'];
  const list = useInfiniteQuery({ queryKey: [...prefix, event, q, from, to], initialPageParam: '', queryFn: ({ pageParam, signal }) => { const filter = { event_type: event, q, from: from ? new Date(`${from}+08:00`).toISOString() : '', to: to ? new Date(`${to}+08:00`).toISOString() : '', cursor: pageParam, limit: 20 }; return platform ? client.accessAudit({ ...filter, tenant_id: id }, signal) : client.tenantAudit(id, filter, signal); }, getNextPageParam: (p) => p.next_cursor || undefined });
  return <section className="space-y-4"><div className="flex flex-wrap gap-4"><div className="space-y-2"><Label htmlFor="tenant-audit-q">搜索操作或原因</Label><Input id="tenant-audit-q" value={q} onChange={(e) => setQ(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="tenant-audit-event">事件类型</Label><select id="tenant-audit-event" className="h-11 rounded-md border bg-background px-3" value={event} onChange={(e) => setEvent(e.target.value)}><option value="">全部</option>{Object.entries(tenantActions).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></div><div className="space-y-2"><Label htmlFor="tenant-audit-from">开始时间（上海）</Label><Input id="tenant-audit-from" type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="tenant-audit-to">结束时间（上海）</Label><Input id="tenant-audit-to" type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></div></div>{list.isPending && <p role="status">正在读取操作记录…</p>}{list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}
    {list.data?.pages.flatMap((p) => p.items).map((ev) => <article key={ev.id} className="space-y-3 rounded-lg border p-4"><div className="flex flex-wrap justify-between gap-2"><h3 className="font-medium">{tenantActions[ev.event_type] ?? ev.event_type}</h3><time className="text-sm text-muted-foreground">{tenantDate(ev.created_at)}</time></div><p className="text-sm">{ev.actor_scope === 'tenant' ? '租户操作' : '平台操作'} · {ev.reason}</p><p className="break-all text-xs text-muted-foreground">操作者：{ev.actor_type === 'maintenance' ? '主机维护' : ev.actor_id}</p><details><summary className="cursor-pointer text-sm">查看前后内容与操作者快照</summary><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted/40 p-3 text-xs">{JSON.stringify(ev.details, null, 2)}</pre></details></article>)}
    {list.data && !list.error && !list.data.pages[0]?.items.length && <p>没有符合条件的操作记录。</p>}{list.hasNextPage && <Button variant="outline" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更多记录</Button>}
  </section>;
}
