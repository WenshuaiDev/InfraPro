import { useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef, type VisibilityState } from '@tanstack/react-table';
import { MoreHorizontal, SlidersHorizontal } from 'lucide-react';
import type { AccessAuditEvent } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { actionLabel, actorLabel, auditSummary, auditTime, objectLabel, scopeLabel, targetLabel } from './audit-presentation';
import { AuditOutcome } from './audit-panel';

type OpenEvent = (event: AccessAuditEvent, trigger: HTMLElement | null, tab?: string) => void;
function AuditActions({ event, open }: { event: AccessAuditEvent; open: OpenEvent }) {
  const trigger = useRef<HTMLButtonElement>(null), opening = useRef(false);
  return <DropdownMenu><DropdownMenuTrigger asChild><Button id={`audit-${event.id}-menu`} ref={trigger} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${actionLabel(event)}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" onCloseAutoFocus={(e) => { if (opening.current) { e.preventDefault(); opening.current = false; } }}><DropdownMenuLabel>审计记录</DropdownMenuLabel><DropdownMenuSeparator />{[{ tab: 'details', label: '查看详情' }, { tab: 'technical', label: '查看技术信息' }].map((item) => <DropdownMenuItem key={item.tab} onSelect={() => { opening.current = true; open(event, trigger.current, item.tab); }}>{item.label}</DropdownMenuItem>)}</DropdownMenuContent></DropdownMenu>;
}

function AuditCell({ column, event, open }: { column: string; event: AccessAuditEvent; open: OpenEvent }) {
  switch (column) {
    case 'time': { const time = auditTime(event.created_at); return <time dateTime={event.created_at} className="block whitespace-nowrap text-xs tabular-nums"><span className="block">{time.day}</span><span className="mt-1 block text-muted-foreground">{time.time}</span></time>; }
    case 'event': { const time = auditTime(event.created_at); return <div className="min-w-0 space-y-1.5"><Button id={`audit-${event.id}-name`} variant="link" className="h-auto max-w-full justify-start whitespace-normal p-0 text-left font-medium" onClick={(e) => open(event, e.currentTarget)}>{actionLabel(event)}</Button><p className="line-clamp-2 whitespace-normal break-words text-xs leading-5 text-muted-foreground" title={auditSummary(event)}>{auditSummary(event)}</p><div className="space-y-1 whitespace-normal break-words text-xs leading-5 lg:hidden"><p>{actorLabel(event)} → {targetLabel(event)}</p><p className="text-muted-foreground">{scopeLabel(event)} · {time.day} {time.time}</p></div></div>; }
    case 'actor': return <div className="min-w-0 space-y-1"><p className="truncate" title={actorLabel(event)}>{actorLabel(event)}</p><p className="truncate text-xs text-muted-foreground">{event.display?.actor_username ? `@${event.display.actor_username}` : event.actor_type === 'release' ? '自动执行' : event.actor_type === 'maintenance' ? '维护命令' : event.actor_id ? '账号操作' : '身份未确认'}</p></div>;
    case 'target': return <div className="min-w-0 space-y-1"><p className="truncate" title={targetLabel(event)}>{targetLabel(event)}</p><p className="truncate text-xs text-muted-foreground" title={`${objectLabel(event)} · ${scopeLabel(event)}`}>{objectLabel(event)} · {scopeLabel(event)}</p></div>;
    case 'outcome': return <AuditOutcome event={event} />;
    case 'reason': return <p className="line-clamp-2 whitespace-normal break-words text-xs leading-5" title={event.reason}>{event.reason || '未填写'}</p>;
    case 'actions': return <AuditActions event={event} open={open} />;
    default: return null;
  }
}

export function AuditTable({ items, open, toolbar, loading, fetching, error, retry }: { items: AccessAuditEvent[]; open: OpenEvent; toolbar: ReactNode; loading: boolean; fetching: boolean; error?: Error | null; retry: () => void }) {
  const [visibility, setVisibility] = useState<VisibilityState>({ reason: false });
  const columns: ColumnDef<AccessAuditEvent>[] = [
    { id: 'time', header: '发生时间', size: 148 },
    { id: 'event', header: '操作', size: 320, enableHiding: false },
    { id: 'actor', header: '操作者', size: 160 },
    { id: 'target', header: '影响对象', size: 230 },
    { id: 'outcome', header: '结果', size: 80, enableHiding: false },
    { id: 'reason', header: '原因', size: 230 },
    { id: 'actions', header: () => <span className="sr-only">查看记录</span>, enableHiding: false },
  ];
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data: items, columns, getRowId: (row) => row.id, getCoreRowModel: getCoreRowModel(), state: { columnVisibility: visibility }, onColumnVisibilityChange: setVisibility });
  const total = table.getVisibleLeafColumns().filter((column) => column.id !== 'actions').reduce((sum, column) => sum + column.getSize(), 0);
  const style = (id: string, size: number) => id === 'actions' ? undefined : { '--column-width': `${size / total * 100}%` } as CSSProperties;
  const classes = (id: string) => id === 'actions' ? 'w-12 text-right lg:w-14' : id === 'outcome' ? 'w-[4.5rem] lg:w-(--column-width) lg:px-4' : id === 'event' ? 'lg:w-(--column-width) lg:px-4' : 'hidden lg:table-cell lg:w-(--column-width) lg:px-4';
  return <section aria-label="平台审计记录" className="space-y-4">
    <div className="flex flex-wrap items-center gap-2">{toolbar}<DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex"><SlidersHorizontal aria-hidden="true" />显示列</Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuLabel>显示列</DropdownMenuLabel><DropdownMenuSeparator />{table.getAllColumns().filter((column) => column.getCanHide()).map((column) => <DropdownMenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>{String(column.columnDef.header)}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu></div>
    {error && <ErrorState message={error.message} onRetry={retry} />}
    <div className="overflow-hidden rounded-lg border" aria-busy={loading || fetching}>
      <Table aria-label="平台审计记录" className="table-fixed"><TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={classes(header.id)} style={style(header.id, header.getSize())}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
        <TableBody>{!loading && table.getRowModel().rows.map((row) => <TableRow key={row.id}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={`py-3 ${classes(cell.column.id)}`} style={style(cell.column.id, cell.column.getSize())}><AuditCell column={cell.column.id} event={row.original} open={open} /></TableCell>)}</TableRow>)}{(loading || !items.length) && <TableRow><TableCell colSpan={table.getVisibleLeafColumns().length}>{loading ? <LoadingState label="正在读取审计记录…" /> : error ? <p className="p-8 text-center text-sm text-muted-foreground">暂时无法读取记录，请重试。</p> : <EmptyState title="没有符合条件的记录" description="试试其他名称、调整时间范围，或重置筛选条件。" />}</TableCell></TableRow>}</TableBody>
      </Table>
    </div>
  </section>;
}
