import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/table';
import { Button } from '../components/button';
import { EmptyState, ErrorState, LoadingState } from './feedback';
import { cn } from '../lib/utils';

type Props<T> = {
  data: T[]; columns: ColumnDef<T>[]; getRowId: (row: T) => string; label: string;
  loading?: boolean; error?: string; onRetry?: () => void; emptyLabel?: string;
  hasMore?: boolean; loadingMore?: boolean; onLoadMore?: () => void; loadMoreLabel?: string;
  /** ColumnDef.size sets the width proportions; minWidth keeps dense tables readable. */
  minWidth?: number;
};

export function DataTable<T>({ data, columns, getRowId, label, loading, error, onRetry, emptyLabel, hasMore, loadingMore, onLoadMore, loadMoreLabel = '加载更多', minWidth }: Props<T>) {
  // TanStack Table is intentionally used through its documented mutable table interface.
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data, columns, getRowId, getCoreRowModel: getCoreRowModel() });
  const sized = columns.some((column) => column.size !== undefined);
  const totalSize = table.getTotalSize();
  const actionColumn = (id: string) => id === 'actions';
  if (loading) return <LoadingState />;
  if (error && !data.length) return <ErrorState message={error} onRetry={onRetry} />;
  return <div className="min-w-0 space-y-4">
    {error && <ErrorState message={error} onRetry={onRetry} />}
    <div className="overflow-hidden rounded-md border"><Table aria-label={label} className={sized ? 'table-fixed' : undefined} style={{ minWidth: minWidth ?? (sized ? totalSize : undefined) }}>
      <TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={cn('px-4', actionColumn(header.column.id) && 'text-right')} style={sized ? { width: `${header.getSize() / totalSize * 100}%` } : undefined}>{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
      <TableBody>{table.getRowModel().rows.map((row) => <TableRow key={row.id}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={cn('px-4 py-3 align-top', sized && 'whitespace-normal break-words', actionColumn(cell.column.id) && 'text-right')}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>)}</TableRow>)}
        {!data.length && <TableRow><TableCell colSpan={columns.length}><EmptyState title={emptyLabel} /></TableCell></TableRow>}
      </TableBody>
    </Table></div>
    {hasMore && onLoadMore && <div className="flex justify-center"><Button variant="outline" disabled={loadingMore} onClick={onLoadMore}>{loadingMore ? '正在加载…' : loadMoreLabel}</Button></div>}
  </div>;
}
