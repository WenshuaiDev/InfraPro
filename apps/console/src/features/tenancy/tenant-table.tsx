import { useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef, type VisibilityState } from '@tanstack/react-table';
import { MoreHorizontal, SlidersHorizontal } from 'lucide-react';
import { Permissions as P, type Tenant } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuCheckboxItem } from '@infrapro/ui/components/dropdown-menu';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { AdministratorAvailability, TenantListStatus, TenantTime } from './tenant-list-presentation';
import type { TenantPanelMode } from './platform-tenant-panel';

type OpenTenant = (tenant: Tenant, mode: Exclude<TenantPanelMode, 'create'>, trigger: HTMLElement | null) => void;
const columns: ColumnDef<Tenant>[] = [
  { id: 'tenant', header: '租户', size: 320, enableHiding: false },
  { id: 'status', header: '状态', size: 132, enableHiding: false },
  { id: 'administrator', header: '当前管理员', size: 220 },
  { id: 'availability', header: '管理员可用性', size: 180 },
  { id: 'updated', header: '更新时间', size: 160 },
  { id: 'actions', header: () => <span className="sr-only">操作</span>, enableHiding: false },
];

function TenantActions({ tenant, open }: { tenant: Tenant; open: OpenTenant }) {
  const { can } = useIdentity();
  const trigger = useRef<HTMLButtonElement>(null), openingPanel = useRef(false);
  const select = (mode: Exclude<TenantPanelMode, 'create'>) => { openingPanel.current = true; open(tenant, mode, trigger.current); };
  return <DropdownMenu><DropdownMenuTrigger asChild><Button ref={trigger} id={`tenant-${tenant.id}-actions`} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${tenant.name}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="w-44" onCloseAutoFocus={(event) => { if (openingPanel.current) { event.preventDefault(); openingPanel.current = false; } }}>
      <DropdownMenuLabel>租户操作</DropdownMenuLabel><DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => select('details')}>查看详情</DropdownMenuItem>
      {can(P.PlatformTenantUpdate) && <DropdownMenuItem onSelect={() => select('edit')}>编辑资料</DropdownMenuItem>}
      {can(P.PlatformTenantTransferAdmin) && <DropdownMenuItem onSelect={() => select('transfer')}>交接管理员</DropdownMenuItem>}
      <DropdownMenuSeparator /><DropdownMenuItem asChild><Link to="/platform/tenants/$tenantId" params={{ tenantId: tenant.id }}>打开完整档案</Link></DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
}

// Stable cell components preserve the opening button across background refreshes.
function TenantCell({ column, tenant, open }: { column: string; tenant: Tenant; open: OpenTenant }) {
  switch (column) {
    case 'tenant': return <div className="min-w-0 space-y-1"><Button id={`tenant-${tenant.id}-name`} variant="link" className="h-auto max-w-full justify-start p-0 text-left font-medium max-sm:min-h-11" title={tenant.name} onClick={(event) => open(tenant, 'details', event.currentTarget)}><span className="truncate">{tenant.name}</span></Button><p className="truncate text-xs text-muted-foreground" title={tenant.code}>{tenant.code}</p><div className="space-y-1 pt-1 lg:hidden"><p className="truncate text-xs text-muted-foreground" title={`${tenant.administrator.display_name}（${tenant.administrator.username}）`}>管理员：{tenant.administrator.display_name}</p>{!tenant.administrator_available && <AdministratorAvailability available={false} />}</div></div>;
    case 'status': return <TenantListStatus status={tenant.status} />;
    case 'administrator': return <div className="min-w-0 space-y-1"><p className="truncate" title={tenant.administrator.display_name}>{tenant.administrator.display_name}</p><p className="truncate text-xs text-muted-foreground" title={tenant.administrator.username}>@{tenant.administrator.username}</p></div>;
    case 'availability': return <AdministratorAvailability available={tenant.administrator_available} />;
    case 'updated': return <TenantTime value={tenant.updated_at} />;
    case 'actions': return <TenantActions tenant={tenant} open={open} />;
    default: return null;
  }
}

export function TenantTable({ items, loading, fetching, error, retry, filtered, reset, open, toolbar }: {
  items: Tenant[]; loading: boolean; fetching: boolean; error: Error | null; retry: () => void;
  filtered: boolean; reset: () => void; open: OpenTenant; toolbar: ReactNode;
}) {
  const [visibility, setVisibility] = useState<VisibilityState>({});
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data: items, columns, getRowId: (tenant) => tenant.id, getCoreRowModel: getCoreRowModel(), state: { columnVisibility: visibility }, onColumnVisibilityChange: setVisibility });
  const total = table.getVisibleLeafColumns().filter((column) => column.id !== 'actions').reduce((sum, column) => sum + column.getSize(), 0);
  const style = (id: string, size: number): CSSProperties | undefined => id === 'actions' ? undefined : { '--column-width': `${size / total * 100}%` } as CSSProperties;
  const columnClass = (id: string) => id === 'tenant' ? 'lg:w-(--column-width) lg:px-4' : id === 'actions' ? 'w-14 text-right' : id === 'status' ? 'w-24 lg:w-(--column-width) lg:px-4' : 'hidden lg:table-cell lg:w-(--column-width) lg:px-4';
  return <section aria-label="平台租户列表" className="space-y-4">
    <div className="flex flex-wrap items-center gap-2">{toolbar}<DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex"><SlidersHorizontal aria-hidden="true" />显示列</Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuLabel>显示列</DropdownMenuLabel><DropdownMenuSeparator />{table.getAllColumns().filter((column) => column.getCanHide()).map((column) => <DropdownMenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>{String(column.columnDef.header)}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu></div>
    {error && <ErrorState message={error.message} onRetry={retry} />}
    <div className="overflow-hidden rounded-lg border" aria-busy={loading || fetching}>
      <Table aria-label="平台租户列表" className="table-fixed"><TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={columnClass(header.id)} style={style(header.id, header.getSize())}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
        <TableBody>{!loading && table.getRowModel().rows.map((row) => <TableRow key={row.id}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={`py-3 ${columnClass(cell.column.id)}`}><TenantCell column={cell.column.id} tenant={row.original} open={open} /></TableCell>)}</TableRow>)}{(loading || !items.length) && <TableRow><TableCell colSpan={table.getVisibleLeafColumns().length} className="h-40 whitespace-normal text-center">{loading ? <LoadingState label="正在读取租户…" /> : error ? '暂时无法读取租户，请重试。' : <EmptyState title={filtered ? '没有符合筛选条件的租户' : '平台尚无租户'} description={filtered ? '试试其他名称、编号，或调整筛选条件。' : '创建租户档案并指定管理员后，即可在这里管理。'}>{filtered && <Button variant="outline" className="mt-2" onClick={reset}>清除筛选</Button>}</EmptyState>}</TableCell></TableRow>}</TableBody>
      </Table>
    </div>
  </section>;
}
