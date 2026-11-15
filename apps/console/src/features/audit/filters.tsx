import { useState, type ReactNode } from 'react';
import type { AuditFilter } from '@infrapro/api-client';
import { CalendarDays, Search, SlidersHorizontal, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { categories, filterLabels, rangePreset, recentFilter, shanghaiInput, shanghaiISO, validateFilter } from './filter-state';
import { date, outcomes } from './presentation';

const selectClass = 'h-9 min-w-0 max-w-full rounded-md border bg-background px-3 text-sm max-sm:h-11';
type Props = { technical?: boolean; value: AuditFilter; onChange: (filter: AuditFilter) => void; children?: ReactNode };
export function AuditFilters({ value, onChange, technical = false, children }: Props) {
  const [advanced, setAdvanced] = useState(false);
  const active = Object.entries(value).filter(([key, v]) => !!v && !['cursor', 'limit'].includes(key)) as [keyof AuditFilter, string][];
  const precise = active.filter(([key]) => !['q', 'outcome', 'from', 'to'].includes(key));
  return <div className="space-y-3">
    <div className="flex flex-wrap items-center gap-2">
      <SearchBox key={value.q ?? ''} value={value.q ?? ''} search={q => onChange({ ...value, q: q.trim() || undefined, cursor: undefined })} />
      <label className="relative max-w-full"><span className="sr-only">时间范围</span><CalendarDays aria-hidden="true" className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground max-sm:top-3.5" /><select aria-label="时间范围" className={`${selectClass} pl-9`} value={rangePreset(value)} onChange={event => { if (event.target.value === 'custom') setAdvanced(true); else onChange(recentFilter(value, Number(event.target.value))); }}>
        {[1, 7, 30].map(days => <option key={days} value={days}>最近 {days} 天</option>)}<option value="custom">自定义时间</option>
      </select></label>
      <label><span className="sr-only">操作结果</span><select aria-label="操作结果" className={selectClass} value={value.outcome ?? ''} onChange={event => onChange({ ...value, outcome: event.target.value as AuditFilter['outcome'] || undefined, cursor: undefined })}><option value="">全部结果</option>{Object.entries(outcomes).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
      <Button variant="outline" className="h-9 max-sm:h-11" onClick={() => setAdvanced(true)}><SlidersHorizontal aria-hidden="true" />更多筛选{precise.length > 0 && <span className="rounded bg-muted px-1.5 text-xs">{precise.length}</span>}</Button>
      <div className="ml-auto flex items-center gap-2">{children}</div>
    </div>
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>上海时区</span><span aria-hidden="true">·</span><span>{value.from || value.to ? `${value.from ? date(value.from) : '默认起点'} 至 ${value.to ? date(value.to) : '现在'}` : '默认查询最近 7 天'}</span>
      {active.filter(([key]) => !['from', 'to'].includes(key)).map(([key, v]) => <span key={key} className="inline-flex max-w-full items-center gap-1 rounded-md border bg-muted/30 py-0.5 pl-2 text-foreground"><span className="min-w-0 break-words [overflow-wrap:anywhere]">{filterLabels[key]}：{key === 'outcome' ? outcomes[v] ?? v : key === 'category' ? categories[v as keyof typeof categories] ?? v : v}</span><Button size="icon" variant="ghost" className="size-6 shrink-0" aria-label={`移除${filterLabels[key]}筛选`} onClick={() => onChange({ ...value, [key]: undefined, cursor: undefined })}><X aria-hidden="true" className="size-3" /></Button></span>)}
      {active.length > 0 && <Button variant="ghost" className="h-7 px-2 text-xs" onClick={() => onChange({ limit: value.limit })}>重置<X aria-hidden="true" className="size-3" /></Button>}
    </div>
    {advanced && <AdvancedFilters value={value} technical={technical} close={() => setAdvanced(false)} apply={next => { onChange(next); setAdvanced(false); }} />}
  </div>;
}
function SearchBox({ value, search }: { value: string; search: (q: string) => void }) {
  const [draft, setDraft] = useState(value);
  return <form role="search" className="relative w-full sm:w-72" onSubmit={event => { event.preventDefault(); search(draft); }}><Search aria-hidden="true" className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground max-sm:top-3.5" /><Input aria-label="搜索当前获准内容" type="search" maxLength={100} value={draft} onChange={event => setDraft(event.target.value)} placeholder="搜索操作、对象或操作者…" className="h-9 pl-9 pr-14 max-sm:h-11" /><Button type="submit" size="sm" variant="ghost" className="absolute right-1 top-1 h-7 px-2 text-xs max-sm:top-2">搜索</Button></form>;
}
function AdvancedFilters({ value, technical, apply, close }: { value: AuditFilter; technical: boolean; apply: (value: AuditFilter) => void; close: () => void }) {
  const [draft, setDraft] = useState(value), [error, setError] = useState('');
  const text = (key: keyof AuditFilter, hint?: string) => <label className="grid min-w-0 gap-2 text-sm" key={key}>{filterLabels[key]}<Input value={String(draft[key] ?? '')} maxLength={200} placeholder={hint} onChange={event => setDraft({ ...draft, [key]: event.target.value.trim() || undefined })} /></label>;
  return <ResponsivePanel open onOpenChange={open => { if (!open) close(); }} title="筛选操作记录" description="组合条件缩小查询范围，时间使用上海时区。" footer={<><Button variant="outline" onClick={() => { setDraft({ limit: value.limit }); setError(''); }}>重置条件</Button><Button type="submit" form="audit-filters">应用筛选</Button></>}>
    <form id="audit-filters" className="space-y-6" onSubmit={event => { event.preventDefault(); const message = validateFilter(draft); if (message) { setError(message); return; } apply({ ...draft, cursor: undefined }); }}>
      <section className="space-y-4"><h3 className="text-sm font-semibold">时间与事件</h3><div className="grid gap-4 sm:grid-cols-2">
        {(['from', 'to'] as const).map(key => <label key={key} className="grid min-w-0 gap-2 text-sm">{filterLabels[key]}（上海时区）<Input type="datetime-local" step="1" value={shanghaiInput(draft[key])} onChange={event => setDraft({ ...draft, [key]: shanghaiISO(event.target.value) })} /></label>)}
        <label className="grid min-w-0 gap-2 text-sm">事件分类<select aria-label="事件分类" className={selectClass} value={draft.category ?? ''} onChange={event => setDraft({ ...draft, category: event.target.value as AuditFilter['category'] || undefined })}><option value="">全部分类</option>{Object.entries(categories).map(([v, label]) => <option key={v} value={v}>{label}</option>)}</select></label>
        {text('event_type', '例如：access.role.updated')}{text('object_type')}{text('actor_id')}
      </div><p className="text-xs leading-5 text-muted-foreground">单次最长 366 天。不填时间时查询最近 7 天。</p></section>
      <section className="space-y-4 border-t pt-5"><h3 className="text-sm font-semibold">对象与来源</h3><div className="grid gap-4 sm:grid-cols-2">{text('object_id')}{text('reference_id')}{text('task_module')}{text('task_kind')}{text('task_id')}{text('generation')}</div></section>
      {technical && <section className="space-y-4 border-t pt-5"><h3 className="text-sm font-semibold">技术追踪</h3><div className="grid gap-4 sm:grid-cols-2">{text('operation_id')}{text('request_id')}</div></section>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    </form>
  </ResponsivePanel>;
}
