import { ChevronLeft, ChevronRight, ChevronsLeft, PlusCircle } from 'lucide-react';
import { Badge } from '../components/badge';
import { Button } from '../components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '../components/dropdown-menu';
import { Pagination, PaginationContent, PaginationItem } from '../components/pagination';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/select';

export function ListFilter({ label, caption = label, value, onChange, options }: { label: string; caption?: string; value: string; onChange: (value: string) => void; options: { value: string; label: string }[] }) {
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="h-9 border-dashed max-sm:h-11" aria-label={label}><PlusCircle aria-hidden="true" />{caption}{value && <><span className="mx-1 h-4 border-l" /><Badge variant="secondary">{options.find((item) => item.value === value)?.label}</Badge></>}</Button></DropdownMenuTrigger><DropdownMenuContent align="start" className="max-h-80 overflow-y-auto"><DropdownMenuLabel>{label}</DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuRadioGroup value={value} onValueChange={onChange}>{options.map((option) => <DropdownMenuRadioItem key={option.value} value={option.value}>{option.label}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuContent></DropdownMenu>;
}

type PaginationProps = {
  label: string; count: number; unit?: string; page: number; pageCount: number; hasNext: boolean; fetching: boolean;
  go: (page: number) => void; next: () => void; pageSize: number; setPageSize: (size: number) => void; pageSizes?: number[];
};

export function ListPagination({ label, count, unit = '条记录', page, pageCount, hasNext, fetching, go, next, pageSize, setPageSize, pageSizes = [10, 20, 50] }: PaginationProps) {
  return <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
    <div className="flex items-center gap-2 text-muted-foreground"><span>每页</span><Select disabled={fetching} value={String(pageSize)} onValueChange={(value) => setPageSize(Number(value))}><SelectTrigger aria-label="每页条数" className="h-9 w-20 max-sm:h-11"><SelectValue /></SelectTrigger><SelectContent>{pageSizes.map((size) => <SelectItem key={size} value={String(size)}>{size}</SelectItem>)}</SelectContent></Select><span className="hidden sm:inline">本页 {count} {unit}</span></div>
    <Pagination aria-label={`${label}分页`} className="mx-0 w-auto"><PaginationContent>
      <PaginationItem><Button variant="outline" size="icon" className="size-9 max-sm:size-11" aria-label="第一页" disabled={page === 0 || fetching} onClick={() => go(0)}><ChevronsLeft aria-hidden="true" /></Button></PaginationItem>
      <PaginationItem><Button variant="outline" size="icon" className="size-9 max-sm:size-11" aria-label="上一页" disabled={page === 0 || fetching} onClick={() => go(page - 1)}><ChevronLeft aria-hidden="true" /></Button></PaginationItem>
      <PaginationItem><span className="px-3 tabular-nums" role="status">第 {page + 1} 页{!hasNext && pageCount > 0 ? ' · 末页' : ''}</span></PaginationItem>
      <PaginationItem><Button variant="outline" size="icon" className="size-9 max-sm:size-11" aria-label="下一页" disabled={!hasNext || fetching} onClick={next}><ChevronRight aria-hidden="true" /></Button></PaginationItem>
    </PaginationContent></Pagination>
  </div>;
}
