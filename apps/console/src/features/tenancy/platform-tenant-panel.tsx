import { useRef, useState, type RefObject } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowUpRight, Pencil, UserRoundCog } from 'lucide-react';
import { ApiError, Permissions as P, type Tenant } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { TenantEditorForm } from './editor';
import { AdministratorAvailability, TenantListStatus, TenantTime } from './tenant-list-presentation';

export type TenantPanelMode = 'details' | 'create' | 'edit' | 'transfer';
const titles = { details: '租户详情', create: '创建租户', edit: '编辑租户资料', transfer: '交接管理员' };
const descriptions = { details: '查看租户档案、使用状态和当前管理员。', create: '建立租户档案，并指定负责管理该租户的账号。', edit: '更新名称、编号和说明，变更原因将记入审计。', transfer: '原管理员将失去本租户访问与管理能力；其账号及其他租户权限不受影响。' };

export function PlatformTenantPanel({ id, initialMode, close, complete, returnFocus }: { id?: string; initialMode: TenantPanelMode; close: () => void; complete: (message: string) => void; returnFocus: RefObject<HTMLElement | null> }) {
  const { account, client, can } = useIdentity();
  const [mode, setMode] = useState(initialMode), canClose = useRef(() => true);
  const read = can(P.PlatformTenantRead);
  const detail = useQuery({ queryKey: ['platform', account?.id, 'tenant', id], queryFn: ({ signal }) => client.tenant(id!, true, signal), enabled: !!id && read, refetchOnWindowFocus: true });
  const allowed = mode === 'create' ? can(P.PlatformTenantCreate) : read && (mode === 'edit' ? can(P.PlatformTenantUpdate) : mode === 'transfer' ? can(P.PlatformTenantTransferAdmin) : true);
  const dismiss = () => { if (mode === 'details' || canClose.current()) close(); };
  const cancel = () => { if (canClose.current()) { if (mode === 'create') close(); else setMode('details'); } };
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) dismiss(); }} title={titles[mode]} description={descriptions[mode]} returnFocus={returnFocus} fallbackFocusSelector="[data-tenant-list-search]">
    {mode === 'details' && detail.error && detail.data && <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} />}
    {!allowed ? <FormNotice error>当前操作权限已不可用，请关闭面板并核对权限。</FormNotice> : mode !== 'create' && detail.error && (!detail.data || (detail.error instanceof ApiError && [401, 403, 404].includes(detail.error.status ?? 0))) ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : mode !== 'create' && !detail.data ? <LoadingState label="正在读取租户档案…" /> : mode === 'details' ? detail.data && <TenantSummary tenant={detail.data} edit={setMode} /> : <TenantEditorForm key={`${id ?? 'new'}:${mode}`} mode={mode} tenant={detail.data} platform blocked={!!detail.error} retry={() => void detail.refetch()} canCloseRef={canClose} done={complete} cancel={cancel} />}
  </ResponsivePanel>;
}

function TenantSummary({ tenant, edit }: { tenant: Tenant; edit: (mode: TenantPanelMode) => void }) {
  const { can } = useIdentity();
  return <section aria-label="租户详情" className="space-y-6">
    <div className="space-y-3"><div className="flex flex-wrap items-center gap-3"><h2 className="min-w-0 break-words text-xl font-semibold">{tenant.name}</h2><TenantListStatus status={tenant.status} /></div><p className="break-all text-sm text-muted-foreground">{tenant.code}</p><p className="whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">{tenant.description || '暂无说明'}</p></div>
    <section className="space-y-4 rounded-lg border p-4"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="text-sm font-medium">当前管理员</h3><AdministratorAvailability available={tenant.administrator_available} /></div><div className="space-y-1"><p className="break-words font-medium">{tenant.administrator.display_name}</p><p className="break-all text-sm text-muted-foreground">@{tenant.administrator.username}</p></div>{!tenant.administrator_available && <p className="text-sm text-destructive">管理员账号当前不可用，请交接给可用账号以恢复管理职责。</p>}</section>
    <dl className="grid grid-cols-2 gap-4 border-b pb-5 text-sm"><div><dt className="mb-2 text-muted-foreground">创建时间</dt><dd><TenantTime value={tenant.created_at} /></dd></div><div><dt className="mb-2 text-muted-foreground">更新时间</dt><dd><TenantTime value={tenant.updated_at} /></dd></div></dl>
    {tenant.status === 'disabled' && <FormNotice>{tenant.status_notice || '该租户已停用，成员暂时无法使用租户业务。'}</FormNotice>}
    <div className="flex flex-wrap gap-2">{can(P.PlatformTenantUpdate) && <Button variant="outline" onClick={() => edit('edit')}><Pencil aria-hidden="true" />编辑资料</Button>}{can(P.PlatformTenantTransferAdmin) && <Button variant="outline" onClick={() => edit('transfer')}><UserRoundCog aria-hidden="true" />交接管理员</Button>}</div>
    <div className="space-y-2 border-t pt-5"><Button variant="outline" asChild><Link to="/platform/tenants/$tenantId" params={{ tenantId: tenant.id }}>打开完整档案<ArrowUpRight aria-hidden="true" /></Link></Button><p className="text-xs leading-5 text-muted-foreground">在完整档案中管理租户启停{can(P.PlatformAuditRead) ? '、查看操作记录' : ''}。时间按 Asia/Shanghai 显示。</p></div>
  </section>;
}
