import { useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef, type VisibilityState } from '@tanstack/react-table';
import { MoreHorizontal, SlidersHorizontal } from 'lucide-react';
import type { TenantAuditEvent } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { tenantAuditAction as actionLabel } from './audit-presentation';
import { tenantAuditActor, tenantAuditScope, tenantAuditSummary, tenantAuditTime } from './audit-presentation';

export type OpenTenantAudit = (event: TenantAuditEvent, trigger: HTMLElement | null, tab?: string) => void;

function AuditActions({ event, open }: { event: TenantAuditEvent; open: OpenTenantAudit }) {
  const trigger = useRef<HTMLButtonElement>(null), opening = useRef(false);
  return <DropdownMenu>
    <DropdownMenuTrigger asChild><Button id={`tenant-audit-${event.id}-menu`} ref={trigger} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${actionLabel(event)}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" onCloseAutoFocus={(e) => { if (opening.current) { e.preventDefault(); opening.current = false; } }}>
      <DropdownMenuLabel>操作记录</DropdownMenuLabel><DropdownMenuSeparator />
      {[{ tab: 'details', label: '查看详情' }, { tab: 'technical', label: '查看技术信息' }].map((item) => <DropdownMenuItem key={item.tab} onSelect={() => { opening.current = true; open(event, trigger.current, item.tab); }}>{item.label}</DropdownMenuItem>)}
    </DropdownMenuContent>
  </DropdownMenu>;
}

function AuditCell({ column, event, open }: { column: string; event: TenantAuditEvent; open: OpenTenantAudit }) {
  const time = tenantAuditTime(event.created_at), actor = tenantAuditActor(event);
  switch (column) {
    case 'time': return <time dateTime={event.created_at} className="block whitespace-nowrap text-xs tabular-nums"><span className="block">{time.day}</span><span className="mt-1 block text-muted-foreground">{time.time}</span></time>;
    case 'event': return <div className="min-w-0 space-y-1.5">
      <Button id={`tenant-audit-${event.id}-name`} variant="link" className="h-auto max-w-full justify-start whitespace-normal p-0 text-left font-medium max-sm:min-h-11" onClick={(e) => open(event, e.currentTarget)}>{actionLabel(event)}</Button>
      <p className="line-clamp-2 whitespace-normal break-words text-xs leading-5 text-muted-foreground" title={tenantAuditSummary(event)}>{tenantAuditSummary(event)}</p>
      <div className="space-y-1 whitespace-normal break-words text-xs leading-5 lg:hidden"><p>{actor.name}</p><p className="text-muted-foreground"><time dateTime={event.created_at}>{time.day} {time.time}</time></p><p className="line-clamp-2 text-muted-foreground">{event.reason || '未填写原因'}</p></div>
    </div>;
    case 'actor': return <div className="min-w-0 space-y-1"><p className="truncate" title={actor.name}>{actor.name}</p><p className="truncate text-xs text-muted-foreground" title={actor.caption}>{actor.caption}</p></div>;
    case 'scope': return <Badge variant="outline" className="whitespace-nowrap font-normal">{tenantAuditScope(event)}</Badge>;
    case 'reason': return <p className="line-clamp-2 whitespace-normal break-words text-xs leading-5" title={event.reason}>{event.reason || '未填写'}</p>;
    case 'actions': return <AuditActions event={event} open={open} />;
    default: return null;
  }
}

export function TenantAuditTable({ items, open, toolbar, loading, fetching, error, retry }: {
  items: TenantAuditEvent[]; open: OpenTenantAudit; toolbar: ReactNode; loading: boolean; fetching: boolean; error?: Error | null; retry: () => void;
}) {
  const [visibility, setVisibility] = useState<VisibilityState>({});
  const columns: ColumnDef<TenantAuditEvent>[] = [
    { id: 'time', header: '发生时间', size: 140 },
    { id: 'event', header: '操作', size: 290, enableHiding: false },
    { id: 'actor', header: '操作者', size: 180 },
    { id: 'scope', header: '操作来源', size: 112, enableHiding: false },
    { id: 'reason', header: '操作原因', size: 290 },
    { id: 'actions', header: () => <span className="sr-only">查看记录</span>, enableHiding: false },
  ];
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data: items, columns, getRowId: (row) => row.id, getCoreRowModel: getCoreRowModel(), state: { columnVisibility: visibility }, onColumnVisibilityChange: setVisibility });
  const total = table.getVisibleLeafColumns().filter((column) => column.id !== 'actions').reduce((sum, column) => sum + column.getSize(), 0);
  const style = (id: string, size: number) => id === 'actions' ? undefined : { '--column-width': `${size / total * 100}%` } as CSSProperties;
  const classes = (id: string) => id === 'actions' ? 'w-12 text-right lg:w-14' : id === 'scope' ? 'w-24 lg:w-(--column-width) lg:px-4' : id === 'event' ? 'lg:w-(--column-width) lg:px-4' : 'hidden lg:table-cell lg:w-(--column-width) lg:px-4';
  return <section aria-label="租户操作记录列表" className="space-y-4">
    <div className="flex flex-wrap items-center gap-2">{toolbar}
      <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex"><SlidersHorizontal aria-hidden="true" />显示列</Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end"><DropdownMenuLabel>显示列</DropdownMenuLabel><DropdownMenuSeparator />{table.getAllColumns().filter((column) => column.getCanHide()).map((column) => <DropdownMenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>{String(column.columnDef.header)}</DropdownMenuCheckboxItem>)}</DropdownMenuContent>
      </DropdownMenu>
    </div>
    {error && <ErrorState message={error.message} onRetry={retry} />}
    <div className="overflow-hidden rounded-lg border" aria-busy={loading || fetching}>
      <Table aria-label="租户操作记录" className="table-fixed">
        <TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={classes(header.id)} style={style(header.id, header.getSize())}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
        <TableBody>
          {!loading && table.getRowModel().rows.map((row) => <TableRow key={row.id}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={`py-3 ${classes(cell.column.id)}`} style={style(cell.column.id, cell.column.getSize())}><AuditCell column={cell.column.id} event={row.original} open={open} /></TableCell>)}</TableRow>)}
          {(loading || !items.length) && <TableRow><TableCell colSpan={table.getVisibleLeafColumns().length}>{loading ? <LoadingState label="正在读取操作记录…" /> : error ? <p className="p-8 text-center text-sm text-muted-foreground">暂时无法读取记录，请重试。</p> : <EmptyState title="没有符合条件的记录" description="试试其他关键词、调整时间范围，或重置筛选条件。" />}</TableCell></TableRow>}
        </TableBody>
      </Table>
    </div>
  </section>;
}
