import { useRef, useState } from 'react';
import { RefreshCw, Search, X } from 'lucide-react';
import { Permissions as P, type ManagedAccount } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { AccountPanel, type AccountMode } from './account-panel';
import { AccountRoleFilter } from './account-role-filter';
import { AccountTable } from './account-table';
import { useCursorList } from './use-role-list';

export function AccountsPage() {
  const { client, account, can } = useIdentity();
  const [q, setQ] = useState(''), [status, setStatus] = useState(''), [role, setRole] = useState('');
  const [pageSize, setPageSize] = useState(10);
  const [selected, setSelected] = useState<{ id: string; mode: AccountMode; viewer: string }>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const roleFilter = can(P.PlatformRoleRead) ? role : '';
  const list = useCursorList({ queryKey: ['access', account?.id, 'accounts', q, status, roleFilter, pageSize], load: (cursor, signal) => client.accounts({ q, status, role_id: roleFilter, cursor, limit: pageSize }, signal) });
  const open = (target: ManagedAccount, mode: AccountMode, trigger: HTMLElement | null) => {
    returnFocus.current = trigger;
    setSelected({ id: target.id, mode, viewer: account?.id ?? '' });
  };
  const reset = () => { setQ(''); setStatus(''); setRole(''); };
  return <>
    <PageHeader title="账号管理" description="管理平台账号的登录状态、角色绑定与有效权限。" actions={<Button variant="outline" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin' : undefined} aria-hidden="true" />刷新账号</Button>} />
    <Tabs value={status || 'all'} onValueChange={(value) => setStatus(value === 'all' ? '' : value)} className="gap-5">
      <TabsList className="max-sm:grid max-sm:h-auto max-sm:w-full max-sm:grid-cols-3">
        <TabsTrigger value="all" className="max-sm:min-h-11">全部账号</TabsTrigger><TabsTrigger value="active" className="max-sm:min-h-11">正常</TabsTrigger><TabsTrigger value="disabled" className="max-sm:min-h-11">已停用</TabsTrigger>
      </TabsList>
      <TabsContent value={status || 'all'} className="space-y-4">
        <AccountTable items={list.items} selectedId={selected?.id} open={open} loading={list.isPending} fetching={list.isFetching} error={list.error} retry={() => void list.refetch()} toolbar={<>
          <div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-account-search aria-label="搜索用户名或显示名称" placeholder="搜索用户名或显示名称…" className="h-9 pl-9 max-sm:h-11" value={q} maxLength={200} onChange={(event) => setQ(event.target.value)} /></div>
          {can(P.PlatformRoleRead) && <AccountRoleFilter value={roleFilter} onChange={setRole} />}
          {(q || status || roleFilter) && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={reset}>重置<X aria-hidden="true" /></Button>}
        </>} />
        <ListPagination label="账号列表" unit="个账号" count={list.items.length} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
      </TabsContent>
    </Tabs>
    {selected && selected.viewer === account?.id && <AccountPanel key={`${selected.id}:${selected.mode}`} id={selected.id} initialMode={selected.mode} close={() => setSelected(undefined)} returnFocus={returnFocus} />}
  </>;
}
