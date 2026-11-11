import { useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef, type VisibilityState } from '@tanstack/react-table';
import { CheckCircle2, CircleDashed, MoreHorizontal, Search, SlidersHorizontal, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuCheckboxItem } from '@infrapro/ui/components/dropdown-menu';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';

export type RoleRow = { id: string; name: string; description: string; status: string; permission_codes: string[] };
export type RoleAction = { label: string; run: (trigger: HTMLButtonElement | null) => void; disabled?: boolean; destructive?: boolean };

export function RoleActions({ name, actions, id }: { name: string; actions: RoleAction[]; id?: string }) {
  const trigger = useRef<HTMLButtonElement>(null);
  const openingPanel = useRef(false);
  return <DropdownMenu><DropdownMenuTrigger asChild><Button id={id} ref={trigger} data-role-actions variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${name}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="w-48" onCloseAutoFocus={(event) => { if (openingPanel.current) { event.preventDefault(); openingPanel.current = false; } }}><DropdownMenuLabel>角色操作</DropdownMenuLabel><DropdownMenuSeparator />{actions.map((action) => <DropdownMenuItem key={action.label} disabled={action.disabled} className={action.destructive ? 'text-destructive focus:text-destructive' : undefined} onSelect={() => { openingPanel.current = true; action.run(trigger.current); }}>{action.label}</DropdownMenuItem>)}</DropdownMenuContent>
  </DropdownMenu>;
}

export function RoleFilter({ label, value, onChange, options }: { label: string; value: string; onChange: (value: string) => void; options: { value: string; label: string }[] }) {
  return <ListFilter label={label} caption={label.replace('角色', '')} value={value} onChange={onChange} options={options} />;
}

export function RoleStatus({ status }: { status: string }) {
  const Icon = status === 'active' ? CheckCircle2 : CircleDashed;
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-sm"><Icon className="size-4 text-muted-foreground" aria-hidden="true" />{status === 'active' ? '启用' : '停用'}</span>;
}

type Props<T extends RoleRow> = {
  items: T[]; label: string; q: string; setQ: (q: string) => void; status: string; setStatus: (value: string) => void;
  source?: string; setSource?: (value: string) => void;
  sourceName: (row: T) => string; boundCount?: (row: T) => number;
  renderName: (row: T) => ReactNode; actions: (row: T) => RoleAction[];
  loading: boolean; error?: Error | null; retry: () => void;
  page: number; pageCount: number; hasNext: boolean; fetching: boolean; go: (page: number) => void; next: () => void;
  pageSize: number; setPageSize: (size: number) => void;
};

// Keep a stable React component type for each cell. Inline column renderers passed
// to flexRender remount focused buttons whenever a background query rerenders.
function RoleCell<T extends RoleRow>({ column, row, sourceName, boundCount, renderName, actions }: Pick<Props<T>, 'sourceName' | 'boundCount' | 'renderName' | 'actions'> & { column: string; row: T }) {
  switch (column) {
    case 'name': return <div className="min-w-0 space-y-1"><div className="truncate font-medium" title={row.name}>{renderName(row)}</div><p className="truncate text-xs text-muted-foreground" title={row.description}>{row.description || '暂无说明'}</p><p className="text-xs text-muted-foreground lg:hidden">{sourceName(row)} · {row.permission_codes.length} 项权限{boundCount && ` · ${boundCount(row)} 人绑定`}</p></div>;
    case 'source': return <Badge variant="outline" className="whitespace-nowrap font-normal">{sourceName(row)}</Badge>;
    case 'status': return <RoleStatus status={row.status} />;
    case 'permissions': return <span className="tabular-nums">{row.permission_codes.length}</span>;
    case 'members': return <span className="tabular-nums">{boundCount?.(row)}</span>;
    case 'actions': return <RoleActions id={`role-${row.id}-actions`} name={row.name} actions={actions(row)} />;
    default: return null;
  }
}

export function RoleTable<T extends RoleRow>({ items, label, q, setQ, status, setStatus, source = '', setSource, sourceName, boundCount, renderName, actions, loading, error, retry, page, pageCount, hasNext, fetching, go, next, pageSize, setPageSize }: Props<T>) {
  const [visibility, setVisibility] = useState<VisibilityState>({});
  const columns: ColumnDef<T>[] = [
    { id: 'name', header: '角色', size: 320, enableHiding: false },
    { id: 'source', header: '来源', size: 160 },
    { id: 'status', header: '状态', size: 128, enableHiding: false },
    { id: 'permissions', header: '权限数', size: 112 },
    ...(boundCount ? [{ id: 'members', header: '绑定人数', size: 112 }] : []),
    { id: 'actions', header: () => <span className="sr-only">操作</span>, enableHiding: false },
  ];
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data: items, columns, getRowId: (row) => row.id, getCoreRowModel: getCoreRowModel(), state: { columnVisibility: visibility }, onColumnVisibilityChange: setVisibility });
  const totalSize = table.getVisibleLeafColumns().filter((column) => column.id !== 'actions').reduce((total, column) => total + column.getSize(), 0);
  // Fixed table layout redistributes these proportions around the 56px action menu.
  // Use percentages: mixed percentage/length calc() widths are ignored by tables.
  const columnStyle = (id: string, size: number): CSSProperties | undefined => id === 'actions' ? undefined : { '--column-width': `${size / totalSize * 100}%` } as CSSProperties;
  const columnClass = (id: string) => id === 'name' ? 'lg:w-(--column-width) lg:px-4' : id === 'actions' ? 'w-14 text-right' : id === 'status' ? 'w-24 lg:w-(--column-width) lg:px-4' : `hidden lg:table-cell lg:w-(--column-width) lg:px-4 ${id === 'permissions' || id === 'members' ? 'text-right' : ''}`;
  const reset = () => { setQ(''); setStatus(''); setSource?.(''); };
  return <section aria-label={label} className="space-y-4">
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative w-full sm:w-64"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-role-list-search className="h-9 pl-9 max-sm:h-11" aria-label="搜索角色名称" placeholder="搜索角色名称…" value={q} maxLength={200} onChange={(event) => setQ(event.target.value)} /></div>
      <RoleFilter label="角色状态" value={status} onChange={setStatus} options={[{ value: '', label: '全部状态' }, { value: 'active', label: '启用' }, { value: 'disabled', label: '停用' }]} />
      {setSource && <RoleFilter label="角色来源" value={source} onChange={setSource} options={[{ value: '', label: '全部来源' }, { value: 'preset', label: '平台预置' }, { value: 'custom', label: '本租户自定义' }]} />}
      {(q || status || source) && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={reset}>重置<X aria-hidden="true" /></Button>}
      <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex"><SlidersHorizontal aria-hidden="true" />显示列</Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuLabel>显示列</DropdownMenuLabel><DropdownMenuSeparator />{table.getAllColumns().filter((column) => column.getCanHide()).map((column) => <DropdownMenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>{String(column.columnDef.header)}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu>
    </div>
    {error && <ErrorState message={error.message} onRetry={retry} />}
    <div className="overflow-hidden rounded-lg border" aria-busy={loading || fetching}>
      <Table aria-label={label} className="table-fixed"><TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={columnClass(header.id)} style={columnStyle(header.id, header.getSize())}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
        <TableBody>{!loading && table.getRowModel().rows.map((row) => <TableRow key={row.id}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={`py-3 ${columnClass(cell.column.id)}`}><RoleCell column={cell.column.id} row={row.original} sourceName={sourceName} boundCount={boundCount} renderName={renderName} actions={actions} /></TableCell>)}</TableRow>)}{(loading || !items.length) && <TableRow><TableCell colSpan={table.getVisibleLeafColumns().length} className="h-40 text-center">{loading ? <LoadingState /> : error ? '暂时无法读取角色，请重试。' : '没有符合条件的角色。'}</TableCell></TableRow>}</TableBody>
      </Table>
    </div>
    <ListPagination label={label} count={items.length} unit="个角色" page={page} pageCount={pageCount} hasNext={hasNext} fetching={fetching} go={go} next={next} pageSize={pageSize} setPageSize={setPageSize} />
  </section>;
}
