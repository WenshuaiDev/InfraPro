import { useState } from 'react';
import type { ReactNode } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { TaskFilter } from '@infrapro/api-client';
import { CirclePlus, Search, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Badge } from '@infrapro/ui/components/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem, DropdownMenuLabel, DropdownMenuSeparator } from '@infrapro/ui/components/dropdown-menu';
import { useTasks } from './provider';
import { TaskError } from './common';

type Props = { filter: TaskFilter; types: { type: string; title: string }[]; onChange: (value: Partial<TaskFilter>) => void; children?: ReactNode };
export function TaskToolbar({ filter, types, onChange, children }: Props) {
  const selected = filter.type?.split(',').filter(Boolean) ?? [];
  const filtered = !!(filter.q || filter.group || filter.type || filter.from || filter.to || filter.tenant_id || filter.scope_kind);
  return <div className="space-y-3"><div className="flex flex-wrap items-center gap-2">
    <TaskSearchBox key={filter.q ?? ''} value={filter.q ?? ''} onSearch={q => onChange({ q: q.trim() || undefined })} />
    <Select value={filter.group ?? 'all'} onValueChange={value => onChange({ group: value === 'all' ? undefined : value as TaskFilter['group'] })}><SelectTrigger aria-label="任务状态" className="h-9 w-auto min-w-28"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">进行中</SelectItem><SelectItem value="attention">需处理</SelectItem><SelectItem value="ended">已结束</SelectItem></SelectContent></Select>
    <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="h-9 border-dashed"><CirclePlus aria-hidden="true" />任务类型{selected.length > 0 && <Badge variant="secondary" className="px-1.5">{selected.length}</Badge>}</Button></DropdownMenuTrigger><DropdownMenuContent align="start" className="w-56"><DropdownMenuLabel>任务类型 · 可多选</DropdownMenuLabel><DropdownMenuSeparator />{types.map(item => <DropdownMenuCheckboxItem key={item.type} checked={selected.includes(item.type)} onSelect={event => event.preventDefault()} onCheckedChange={checked => { const next = checked ? [...selected, item.type] : selected.filter(v => v !== item.type); onChange({ type: next.sort().join(',') || undefined }); }}>{item.title}</DropdownMenuCheckboxItem>)}{!types.length && <p className="p-2 text-xs text-muted-foreground">当前范围没有可选类型</p>}</DropdownMenuContent></DropdownMenu>
    <WorkspaceFilter filter={filter} onChange={onChange} />
    <Select value={filter.from || filter.to ? 'bounded' : 'all'} onValueChange={value => onChange({ from: value === 'all' ? undefined : new Date(Date.now() - Number(value) * 86400000).toISOString(), to: undefined })}><SelectTrigger aria-label="提交时间范围" className="h-9 w-auto min-w-28"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部时间</SelectItem>{(filter.from || filter.to) && <SelectItem value="bounded" disabled>已限定时段</SelectItem>}<SelectItem value="1">最近一天</SelectItem><SelectItem value="7">最近一周</SelectItem><SelectItem value="30">最近一月</SelectItem></SelectContent></Select>
    {filtered && <Button variant="ghost" className="h-9 px-2 text-muted-foreground" onClick={() => onChange({ q: undefined, group: undefined, type: undefined, from: undefined, to: undefined, tenant_id: undefined, project_id: undefined, scope_kind: undefined })}>重置<X aria-hidden="true" /></Button>}
    <div className="ml-auto">{children}</div>
  </div>{(filter.from || filter.to) && <p className="text-xs text-muted-foreground">提交时间：{filter.from ? new Date(filter.from).toLocaleString() : '不限'} 至 {filter.to ? new Date(filter.to).toLocaleString() : '现在'}</p>}</div>;
}
function TaskSearchBox({ value, onSearch }: { value: string; onSearch: (value: string) => void }) {
  const [query, setQuery] = useState(value);
  return <form className="relative flex w-full gap-2 sm:w-72" role="search" onSubmit={event => { event.preventDefault(); onSearch(query); }}><Search aria-hidden="true" className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="检索任务" value={query} onChange={event => setQuery(event.target.value)} maxLength={100} placeholder="搜索任务名称或编号…" className="pr-12 pl-9" /><Button type="submit" variant="ghost" size="sm" className="absolute right-1 top-1 h-7 px-2 text-xs">搜索</Button></form>;
}
function WorkspaceFilter({ filter, onChange }: Pick<Props, 'filter' | 'onChange'>) {
  const { client } = useIdentity(), t = useTasks();
  const workspaces = useInfiniteQuery({ queryKey: [...t.key, 'workspaces'], initialPageParam: undefined as string | undefined, queryFn: ({ signal, pageParam }) => client.myWorkspaces({ limit: 100, cursor: pageParam }, signal), getNextPageParam: page => page.next_cursor, retry: false });
  const items = workspaces.error ? [] : workspaces.data?.pages.flatMap(page => page.items).filter(item => item.kind !== 'platform') ?? [];
  const value = filter.project_id ?? filter.tenant_id ?? (filter.scope_kind === 'platform' ? 'platform' : 'all');
  return <div className="max-w-full"><Select value={value} onValueChange={selected => {
    if (selected === 'more') { void workspaces.fetchNextPage(); return; }
    if (selected === 'current') { onChange({ scope_kind: undefined, ...t.workspace }); return; }
    const item = items.find(item => item.id === selected);
    onChange({ scope_kind: selected === 'platform' ? 'platform' : undefined, tenant_id: item?.kind === 'tenant' ? item.id : item?.tenant_id, project_id: item?.kind === 'project' ? item.id : undefined });
  }}><SelectTrigger aria-label="任务工作空间" className="h-9 max-w-64"><SelectValue placeholder="工作空间" /></SelectTrigger><SelectContent><SelectItem value="all">全部工作空间</SelectItem><SelectItem value="platform">平台导出</SelectItem>{t.workspace.tenant_id && <SelectItem value="current">当前工作空间</SelectItem>}{value !== 'all' && value !== 'platform' && !items.some(item => item.id === value) && <SelectItem value={value}>指定工作空间</SelectItem>}{items.map(item => <SelectItem key={item.id} value={item.id}>{item.tenant_name ? `${item.tenant_name} / ` : ''}{item.name}</SelectItem>)}{workspaces.hasNextPage && <SelectItem value="more" disabled={workspaces.isFetchingNextPage}>{workspaces.isFetchingNextPage ? '正在加载…' : '加载更多工作空间'}</SelectItem>}</SelectContent></Select>{workspaces.error && <TaskError error={workspaces.error} retry={() => void workspaces.refetch()} />}</div>;
}
