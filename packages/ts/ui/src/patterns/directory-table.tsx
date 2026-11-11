import { useRef, useState, type ReactNode } from 'react';
import { MoreHorizontal, Search, SlidersHorizontal, X } from 'lucide-react';
import { Button } from '../components/button';
import { Input } from '../components/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/table';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../components/dropdown-menu';
import { ErrorState, LoadingState } from './feedback';

export type DirectoryColumn<T> = {
  id: string;
  label: string;
  cell: (item: T) => ReactNode;
  secondary?: boolean;
  className?: string;
};

/** The same compact controls and responsive table treatment used by role management. */
export function DirectoryTable<T>({
  label,
  items,
  columns,
  rowId,
  loading = false,
  error,
  retry,
  empty = '暂无记录',
  toolbar,
  pagination,
}: {
  label: string;
  items: T[];
  columns: DirectoryColumn<T>[];
  rowId: (item: T) => string;
  loading?: boolean;
  error?: string;
  retry?: () => void;
  empty?: string;
  toolbar?: ReactNode;
  pagination?: ReactNode;
}) {
  const [hidden, setHidden] = useState<string[]>([]);
  const visible = columns.filter((column) => !hidden.includes(column.id));
  const optional = columns.filter((column) => column.secondary);
  return (
    <section aria-label={label} className="min-w-0 space-y-4">
      {(toolbar || optional.length > 0) && (
        <div className="flex flex-wrap items-center gap-2">
          {toolbar}
          {optional.length > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex">
                  <SlidersHorizontal aria-hidden="true" />
                  显示列
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>显示列</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {optional.map((column) => (
                  <DropdownMenuCheckboxItem
                    key={column.id}
                    checked={!hidden.includes(column.id)}
                    onCheckedChange={(show) =>
                      setHidden(show ? hidden.filter((id) => id !== column.id) : [...hidden, column.id])
                    }
                  >
                    {column.label}
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      )}
      {error && <ErrorState title="暂时无法读取列表" message={error} onRetry={retry} />}
      {loading ? (
        <LoadingState label={`正在读取${label}…`} />
      ) : (
        <div className="overflow-hidden rounded-lg border">
          <Table aria-label={label} className="w-full table-fixed">
            <TableHeader>
              <TableRow>
                {visible.map((column) => (
                  <TableHead
                    key={column.id}
                    className={`${column.secondary ? 'hidden lg:table-cell' : ''} ${column.className ?? ''} lg:px-4`}
                  >
                    {column.id === 'actions' ? <span className="sr-only">操作</span> : column.label}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.length ? (
                items.map((item) => (
                  <TableRow key={rowId(item)}>
                    {visible.map((column) => (
                      <TableCell
                        key={column.id}
                        className={`${column.secondary ? 'hidden lg:table-cell' : ''} ${column.className ?? ''} break-words whitespace-normal lg:px-4`}
                      >
                        {column.cell(item)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={visible.length} className="h-32 text-center text-muted-foreground">
                    {empty}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
      {pagination}
    </section>
  );
}

export function DirectorySearch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="relative w-full sm:w-64">
      <Search
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        className="h-9 pl-9 max-sm:h-11"
        aria-label={label}
        placeholder={`${label}…`}
        maxLength={200}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

export function DirectoryReset({ onClick }: { onClick: () => void }) {
  return (
    <Button variant="ghost" className="h-9 px-2 max-sm:h-11" onClick={onClick}>
      重置
      <X aria-hidden="true" />
    </Button>
  );
}

export function DirectoryActions({
  name,
  actions,
  id,
}: {
  name: string;
  id?: string;
  actions: {
    label: string;
    run: (trigger: HTMLButtonElement | null) => void;
    disabled?: boolean;
    opensPanel?: boolean;
  }[];
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const opening = useRef(false);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          id={id}
          ref={trigger}
          variant="ghost"
          size="icon"
          className="size-9 max-sm:size-11"
          aria-label={`${name}的操作`}
        >
          <MoreHorizontal aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          if (opening.current) {
            event.preventDefault();
            opening.current = false;
          }
        }}
      >
        <DropdownMenuLabel>操作</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {actions.map((action) => (
          <DropdownMenuItem
            key={action.label}
            disabled={action.disabled}
            onSelect={() => {
              opening.current = !!action.opensPanel;
              action.run(trigger.current);
            }}
          >
            {action.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
