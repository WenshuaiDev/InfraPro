import { useRef, useState } from 'react';
import { Plus, Search, X } from 'lucide-react';
import { Permissions as P, type Tenant } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useCursorList } from '../access/use-role-list';
import { PlatformTenantPanel, type TenantPanelMode } from './platform-tenant-panel';
import { TenantTable } from './tenant-table';

export function PlatformTenantsPage() {
  const { account } = useIdentity();
  return <PlatformTenantList key={account?.id} />;
}

function PlatformTenantList() {
  const { client, account, can } = useIdentity();
  const [q, setQ] = useState(''), [status, setStatus] = useState(''), [availability, setAvailability] = useState('');
  const [pageSize, setPageSize] = useState(10), [notice, setNotice] = useState('');
  const [selected, setSelected] = useState<{ id?: string; mode: TenantPanelMode }>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const read = can(P.PlatformTenantRead);
  const list = useCursorList({
    queryKey: ['platform', account?.id, 'tenants', q, status, availability, pageSize], enabled: read,
    load: (cursor, signal) => client.tenants({ q, status, administrator_unavailable: availability === 'unavailable', cursor, limit: pageSize }, signal),
  });
  const open = (tenant: Tenant, mode: Exclude<TenantPanelMode, 'create'>, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelected({ id: tenant.id, mode }); };
  const reset = () => { setQ(''); setStatus(''); setAvailability(''); };
  const filtered = Boolean(q || status || availability);
  return <>
    <PageHeader title="租户管理" description="管理租户档案与使用状态，指定和交接管理员。" actions={can(P.PlatformTenantCreate) ? <Button onClick={(event) => { returnFocus.current = event.currentTarget; setSelected({ mode: 'create' }); }}><Plus aria-hidden="true" />创建租户</Button> : undefined} />
    {notice && <div className="mb-4"><FormNotice>{notice}</FormNotice></div>}
    {!read ? <FormNotice>当前未配置租户查看权限。创建能力可单独使用；编辑及交接页面还需要配套的租户查看权限。</FormNotice> : <Tabs value={status || 'all'} onValueChange={(value) => setStatus(value === 'all' ? '' : value)} className="gap-5">
      <TabsList aria-label="租户状态" className="max-sm:h-auto"><TabsTrigger value="all" className="max-sm:min-h-11">全部租户</TabsTrigger><TabsTrigger value="active" className="max-sm:min-h-11">已启用</TabsTrigger><TabsTrigger value="disabled" className="max-sm:min-h-11">已停用</TabsTrigger></TabsList>
      <TabsContent value={status || 'all'} className="space-y-4">
        <TenantTable items={list.items} loading={list.isPending} fetching={list.isFetching} error={list.error} retry={() => void list.refetch()} filtered={filtered} reset={reset} open={open} toolbar={<>
          <div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input id="tenant-search" data-tenant-list-search aria-label="搜索名称或当前 / 历史编号" placeholder="搜索租户名称、当前或历史编号…" maxLength={200} className="h-9 pl-9 max-sm:h-11" value={q} onChange={(event) => setQ(event.target.value)} /></div>
          <ListFilter label="管理员可用性" caption="管理员" value={availability} onChange={setAvailability} options={[{ value: '', label: '全部管理员' }, { value: 'unavailable', label: '管理员不可用' }]} />
          {filtered && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={reset}>重置<X aria-hidden="true" /></Button>}
        </>} />
        <ListPagination label="平台租户列表" count={list.items.length} unit="个租户" page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
      </TabsContent>
    </Tabs>}
    {selected && <PlatformTenantPanel key={`${selected.id ?? 'new'}:${selected.mode}`} id={selected.id} initialMode={selected.mode} close={() => setSelected(undefined)} complete={(message) => { setNotice(message); setSelected(undefined); }} returnFocus={returnFocus} />}
  </>;
}
