import { useState, type ReactNode } from 'react';
import type { AuditEvent } from '@infrapro/api-client';
import { ArrowDown, CheckCircle2, ChevronRight, CircleAlert, Columns3, Search } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem } from '@infrapro/ui/components/dropdown-menu';
import { Skeleton } from '@infrapro/ui/components/skeleton';
import { AuditFilters } from './filters';
import type { AuditFilter } from '@infrapro/api-client';
import { categories } from './filter-state';
import { date, outcomes, phases } from './presentation';

export function Outcome({ value }: { value: string }) {
  const Icon = value === 'success' ? CheckCircle2 : CircleAlert;
  const color = value === 'success' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300' : ['failure', 'denied'].includes(value) ? 'bg-red-50 text-red-700 dark:bg-red-950/50 dark:text-red-300' : ['unknown', 'conflict', 'rate_limited'].includes(value) ? 'bg-amber-50 text-amber-800 dark:bg-amber-950/50 dark:text-amber-300' : 'bg-muted text-muted-foreground';
  return <span className={`inline-flex max-w-full items-center gap-1 rounded-md px-2 py-1 text-xs leading-4 ${color}`}><Icon className="size-3.5 shrink-0" aria-hidden="true" /><span>{outcomes[value] ?? '结果待核实'}</span></span>;
}
const optional = { time: '发生时间', actor: '操作者', phase: '处理阶段' };
export function EventTable({ items, loading, fetching, error, open, filter, technical, onChange, refresh, notice, pagination }: {
  items: AuditEvent[]; loading: boolean; fetching: boolean; error: Error | null; open: (id: string) => void;
  filter: AuditFilter; technical: boolean; onChange: (value: AuditFilter) => void; refresh: ReactNode; notice: ReactNode; pagination: ReactNode;
}) {
  const [hidden, setHidden] = useState<string[]>([]);
  const show = (id: string) => !hidden.includes(id);
  const cells = 'whitespace-normal px-3 py-3 text-xs lg:px-4';
  return <section aria-label="审计事件" className="min-w-0 space-y-4">
    <AuditFilters value={filter} technical={technical} onChange={onChange}><DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="icon" className="size-9 max-sm:size-11" aria-label="显示列"><Columns3 aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end">{Object.entries(optional).map(([id, label]) => <DropdownMenuCheckboxItem key={id} checked={show(id)} onSelect={event => event.preventDefault()} onCheckedChange={checked => setHidden(checked ? hidden.filter(key => key !== id) : [...hidden, id])}>{label}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu>{refresh}</AuditFilters>
    {notice}
    <div className="overflow-hidden rounded-lg border"><Table aria-label="审计事件" aria-busy={fetching} className="table-fixed"><TableHeader><TableRow className="bg-muted/40 hover:bg-muted/40">
      {show('time') && <TableHead className="hidden w-36 px-4 text-xs text-muted-foreground xl:table-cell" aria-sort="descending"><span className="flex items-center gap-1">发生时间<ArrowDown className="size-3" aria-hidden="true" /></span></TableHead>}
      <TableHead className="px-3 text-xs text-muted-foreground lg:px-4">操作 / 对象</TableHead>
      {show('actor') && <TableHead className="hidden w-[18%] px-4 text-xs text-muted-foreground lg:table-cell">操作者</TableHead>}
      {show('phase') && <TableHead className="hidden w-[16%] px-4 text-xs text-muted-foreground xl:table-cell">处理阶段</TableHead>}
      <TableHead className="w-28 px-3 text-xs text-muted-foreground sm:w-32 lg:px-4">操作结果</TableHead><TableHead className="hidden w-20 px-4 text-right text-xs text-muted-foreground lg:table-cell">操作</TableHead>
    </TableRow></TableHeader><TableBody>
      {loading ? Array.from({ length: 6 }, (_, i) => <TableRow key={i}><TableCell colSpan={6} className="h-16 px-4"><Skeleton className="h-4 w-3/4" /></TableCell></TableRow>) : (error ? [] : items).map(e => <TableRow key={e.id}>
        {show('time') && <TableCell className={`${cells} hidden tabular-nums xl:table-cell`}><time dateTime={e.occurred_at}><span className="block text-muted-foreground">{new Date(e.occurred_at).toLocaleDateString('zh-CN', { timeZone: 'Asia/Shanghai' })}</span><span className="mt-1 block">{new Date(e.occurred_at).toLocaleTimeString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}</span></time></TableCell>}
        <TableCell className={cells}><Button data-audit-record={e.id} variant="link" className="h-auto min-h-7 max-w-full max-sm:min-h-11 justify-start whitespace-normal p-0 text-left text-sm leading-5" onClick={() => open(e.id)}>{e.action_label}</Button><p className="mt-0.5 truncate text-xs text-muted-foreground" title={e.object.label || '对象名称未记录或受限'}>{e.object.label || '对象名称未记录或受限'}</p>{show('actor') && <p className="mt-1 text-xs text-muted-foreground lg:hidden">{e.actor.name || '操作者未记录'}</p>}{show('phase') && <p className="mt-1 text-xs text-muted-foreground xl:hidden">{phases[e.phase] ?? e.phase}</p>}{show('time') && <p className="mt-1 text-[11px] tabular-nums text-muted-foreground xl:hidden">{date(e.occurred_at)}</p>}</TableCell>
        {show('actor') && <TableCell className={`${cells} hidden lg:table-cell`}><span className="flex min-w-0 items-center gap-2"><span aria-hidden="true" className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] text-muted-foreground">{(e.actor.name || '未').slice(0, 1)}</span><span className="break-words">{e.actor.name || '未记录'}</span></span></TableCell>}
        {show('phase') && <TableCell className={`${cells} hidden xl:table-cell`}>{phases[e.phase] ?? e.phase}<p className="mt-1 text-[11px] text-muted-foreground">{categories[e.category] ?? e.category}</p></TableCell>}
        <TableCell className={cells}><Outcome value={e.outcome} /></TableCell><TableCell className={`${cells} hidden text-right lg:table-cell`}><Button variant="ghost" size="sm" className="h-8 px-1 text-xs text-muted-foreground" aria-label={`查看${e.action_label}详情`} onClick={() => open(e.id)}>详情<ChevronRight aria-hidden="true" className="size-3.5" /></Button></TableCell>
      </TableRow>)}
      {!loading && (!items.length || error) && <TableRow><TableCell colSpan={6}><div className="flex min-h-56 flex-col items-center justify-center gap-3 text-center"><Search aria-hidden="true" className="size-7 text-muted-foreground" /><p className="font-medium">{error ? '无法核实记录，请重新查询。' : '当前筛选下没有可见记录。'}</p><p className="text-xs text-muted-foreground">{error ? '核实成功后将重新显示记录。' : '试试调整关键词、时间或操作结果。'}</p>{!error && <Button variant="outline" size="sm" onClick={() => onChange({ limit: filter.limit })}>清空筛选</Button>}</div></TableCell></TableRow>}
    </TableBody></Table></div>{pagination}
  </section>;
}
