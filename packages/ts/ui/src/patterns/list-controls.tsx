import { useEffect } from 'react';
import { PlusCircle } from 'lucide-react';
import { Badge } from '../components/badge';
import { Button } from '../components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '../components/dropdown-menu';
import { Pagination, PaginationContent, PaginationEllipsis, PaginationItem, PaginationLink, PaginationNext, PaginationPrevious } from '../components/pagination';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/select';

export function ListFilter({ label, caption = label, value, onChange, options }: { label: string; caption?: string; value: string; onChange: (value: string) => void; options: { value: string; label: string }[] }) {
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="h-9 border-dashed max-sm:h-11" aria-label={label}><PlusCircle aria-hidden="true" />{caption}{value && <><span className="mx-1 h-4 border-l" /><Badge variant="secondary">{options.find((item) => item.value === value)?.label}</Badge></>}</Button></DropdownMenuTrigger><DropdownMenuContent align="start" className="max-h-80 overflow-y-auto"><DropdownMenuLabel>{label}</DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuRadioGroup value={value} onValueChange={onChange}>{options.map((option) => <DropdownMenuRadioItem key={option.value} value={option.value}>{option.label}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuContent></DropdownMenu>;
}

export type ListPage = {
  page: number;
  pageSize: number;
  total: number | undefined;
  go: (page: number) => void;
  setPageSize: (size: number) => void;
  fetching?: boolean;
  hasNext?: boolean;
  countError?: boolean;
  retryCount?: () => void;
};

function pageNumbers(current: number, last: number) {
  const neighbors = current <= 2 ? [2, 3] : current >= last - 1 ? [last - 2, last - 1] : [current - 1, current, current + 1];
  const numbers = [1, ...neighbors.filter(n => n > 1 && n < last), last];
  const sorted = [...new Set(numbers)].sort((a, b) => a - b);
  const result: (string | number)[] = [];
  for (const n of sorted) {
    const previous = result.at(-1);
    if (typeof previous === 'number' && n - previous > 1) result.push(n - previous === 2 ? n - 1 : `gap-${n}`);
    result.push(n);
  }
  return result;
}

export function ListPagination({ label, page, total, pageSize, go, setPageSize, fetching, hasNext, countError, retryCount,
  pageSizes = [10, 20, 50, 100], unit = '条', previousLabel, nextLabel }: ListPage & {
  label: string; pageSizes?: readonly number[]; unit?: string; previousLabel?: string; nextLabel?: string;
}) {
  const current = page + 1;
  const last = total === undefined ? current + Number(!!hasNext) : Math.max(1, Math.ceil(total / pageSize));
  useEffect(() => {
    if (total !== undefined && !fetching && current > last) go(last - 1);
  }, [current, last, total, fetching, go]);
  const sizes = [...new Set([...pageSizes, pageSize])].sort((a, b) => a - b);
  return <div className="@container/pager min-w-0">
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 text-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground">
        <span className="whitespace-nowrap tabular-nums">{total !== undefined ? `共 ${total} ${unit}` : countError ? '总数暂不可用' : '正在统计总数…'}</span>
        {countError && retryCount && <Button type="button" size="sm" variant="link" onClick={retryCount}>重试统计</Button>}
        <div className="flex items-center gap-1.5"><span>每页</span>
          <Select value={String(pageSize)} onValueChange={value => setPageSize(Number(value))}>
            <SelectTrigger aria-label="每页条数" className="h-9 w-auto min-w-16 max-sm:min-h-11"><SelectValue /></SelectTrigger>
            <SelectContent>{sizes.map(size => <SelectItem key={size} value={String(size)}>{size}</SelectItem>)}</SelectContent>
          </Select>
          <span>{unit}</span>
        </div>
      </div>
      <Pagination aria-label={`${label}分页`}>
        <span className="sr-only" role="status">第 {current} 页{total !== undefined && current === last ? ' · 末页' : ''}</span>
        <PaginationContent>
          <PaginationItem><PaginationPrevious aria-label={previousLabel ?? '上一页'} disabled={page === 0 || fetching} onClick={() => go(page - 1)} /></PaginationItem>
          {pageNumbers(current, last).map(n => <PaginationItem key={n}>{typeof n === 'number'
            ? <PaginationLink isActive={current === n} aria-label={n === 1 ? '第一页' : `第 ${n} 页`} disabled={fetching} onClick={() => go(n - 1)}>{n}</PaginationLink>
            : <PaginationEllipsis />}</PaginationItem>)}
          <PaginationItem><PaginationNext aria-label={nextLabel ?? '下一页'} disabled={current >= last || fetching} onClick={() => go(page + 1)} /></PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  </div>;
}
