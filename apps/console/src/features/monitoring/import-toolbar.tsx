import { useState, type ReactNode } from 'react';
import { CirclePlus, Search, X } from 'lucide-react';
import type { MonitoringImportFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Badge } from '@infrapro/ui/components/badge';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem, DropdownMenuLabel, DropdownMenuSeparator } from '@infrapro/ui/components/dropdown-menu';
import { importStates } from './import-search';
import { stateNames } from './format';

export function ImportToolbar({ filter, change, children }: { filter: MonitoringImportFilter; change: (filter: Partial<MonitoringImportFilter>) => void; children?: ReactNode }) {
  const states = filter.states?.split(',') ?? [], formats = filter.formats?.split(',') ?? [];
  const toggle = (list: string[], value: string) => (list.includes(value) ? list.filter(v => v !== value) : [...list, value]).sort().join(',') || undefined;
  return <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex min-w-0 flex-wrap items-center gap-2">
    <ImportSearchBox key={filter.q ?? ''} value={filter.q ?? ''} search={q => change({ q: q || undefined })} />
    <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="sm" className="h-9 border-dashed"><CirclePlus aria-hidden />状态{states.length > 0 && <Badge variant="secondary" className="rounded-sm px-1.5">{states.length}</Badge>}</Button></DropdownMenuTrigger><DropdownMenuContent align="start" className="w-52"><DropdownMenuLabel>处理状态 · 可多选</DropdownMenuLabel><DropdownMenuSeparator />{importStates.map(state => <DropdownMenuCheckboxItem key={state} checked={states.includes(state)} onSelect={e => e.preventDefault()} onCheckedChange={() => change({ states: toggle(states, state) })}>{stateNames[state]}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu>
    <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="sm" className="h-9 border-dashed"><CirclePlus aria-hidden />文件类型{formats.length > 0 && <Badge variant="secondary" className="rounded-sm px-1.5">{formats.join(' · ').toUpperCase()}</Badge>}</Button></DropdownMenuTrigger><DropdownMenuContent align="start"><DropdownMenuLabel>原文件类型</DropdownMenuLabel><DropdownMenuSeparator />{['csv', 'xlsx'].map(format => <DropdownMenuCheckboxItem key={format} checked={formats.includes(format)} onSelect={e => e.preventDefault()} onCheckedChange={() => change({ formats: toggle(formats, format) })}>{format.toUpperCase()}</DropdownMenuCheckboxItem>)}<p className="max-w-48 p-2 text-xs leading-5 text-muted-foreground">部分可见批次不展示原文件类型。</p></DropdownMenuContent></DropdownMenu>
    {(filter.q || states.length > 0 || formats.length > 0) && <Button variant="ghost" size="sm" onClick={() => change({ q: undefined, states: undefined, formats: undefined })}>重置<X aria-hidden /></Button>}
  </div>{children}</div>;
}
function ImportSearchBox({ value, search }: { value: string; search: (q: string) => void }) {
  const [draft, setDraft] = useState(value);
  // Submit explicitly so list refreshes and the URL never interrupt typing/IME.
  return <form className="relative flex w-full sm:w-72" role="search" onSubmit={e => { e.preventDefault(); search(draft.trim()); }}><Search aria-hidden className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input id="import-search" aria-label="搜索文件名或批次编号" placeholder="搜索文件名或批次编号…" value={draft} onChange={e => setDraft(e.target.value)} maxLength={200} className="h-9 pl-9 pr-14" /><Button variant="ghost" type="submit" size="sm" className="absolute right-1 top-1 h-7 px-2 text-xs">搜索</Button></form>;
}
