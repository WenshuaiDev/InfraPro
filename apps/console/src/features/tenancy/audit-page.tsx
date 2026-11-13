import { useRef, useState, type RefObject } from 'react';
import { RefreshCw, Search, SlidersHorizontal, X } from 'lucide-react';
import { Permissions as P, type TenantAuditEvent } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useCursorList } from '../../hooks/use-cursor-list';
import { TenantAuditPanel } from './audit-panel';
import { tenantAuditActions, tenantAuditISO, tenantAuditRange } from './audit-presentation';
import { TenantAuditTable } from './audit-table';
import { useTenant } from './context';

type TimeFilters = { from: string; to: string; range: string };
const emptyFilters: TimeFilters = { from: '', to: '', range: '' };
const ranges = [{ value: '', label: '全部时间' }, { value: 'today', label: '今天' }, { value: 'week', label: '最近 7 天' }, { value: 'month', label: '最近 30 天' }, { value: 'custom', label: '自定义时间' }];

export function TenantAuditPage() {
  const ctx = useTenant(), { account } = useIdentity();
  if (!ctx.permissions.includes(P.TenantAuditRead)) return <FormNotice error>403 · 当前角色无权查看租户操作记录。</FormNotice>;
  return <TenantAuditContent key={`${account?.id}:${ctx.tenant.id}:${ctx.membership_id}`} />;
}

function TenantAuditContent() {
  const ctx = useTenant(), { account, client } = useIdentity();
  const [event, setEvent] = useState(''), [q, setQ] = useState(''), [pageSize, setPageSize] = useState(10);
  const [filters, setFilters] = useState(emptyFilters), [filterOpen, setFilterOpen] = useState(false);
  const [selected, setSelected] = useState<{ event: TenantAuditEvent; tab: string }>();
  const returnFocus = useRef<HTMLElement | null>(null), filterTrigger = useRef<HTMLElement | null>(null), rangeControl = useRef<HTMLDivElement>(null);
  const list = useCursorList({
    queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'audit', 'list', event, q, filters.from, filters.to, pageSize],
    enabled: ctx.available,
    load: (cursor, signal) => client.tenantAudit(ctx.tenant.id, { event_type: event, q: q.trim(), from: tenantAuditISO(filters.from), to: tenantAuditISO(filters.to), cursor, limit: pageSize }, signal),
  });
  const options = Object.entries(tenantAuditActions).filter(([key]) => !['engineering.category-save', 'engineering.category-delete'].includes(key) && (key !== 'tenant.project.governance' || ctx.permissions.includes(P.TenantProjectRead))).map(([value, label]) => ({ value, label }));
  const reset = () => { setQ(''); setEvent(''); setFilters(emptyFilters); };
  return <>
    <PageHeader title="租户操作记录" description={`${ctx.tenant.name} · 追溯租户内的管理变更与工程资源操作。`} actions={<Button variant="outline" className="max-sm:h-11" disabled={list.isFetching || !ctx.available} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin motion-reduce:animate-none' : undefined} aria-hidden="true" />刷新记录</Button>} />
    <div className="space-y-4">
      <TenantAuditTable items={list.items} loading={list.isPending} fetching={list.isFetching} error={list.error} retry={() => void list.refetch()} open={(value, trigger, tab = 'details') => { returnFocus.current = trigger; setSelected({ event: value, tab }); }} toolbar={<>
        <div className="relative w-full sm:w-72"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-tenant-audit-search aria-label="搜索操作或原因" placeholder="搜索原因或事件编码…" value={q} maxLength={200} className="h-9 pl-9 max-sm:h-11" onChange={(e) => setQ(e.target.value)} /></div>
        <ListFilter label="操作类型" caption="操作" value={event} onChange={setEvent} options={[{ value: '', label: '全部操作' }, ...options]} />
        <div ref={rangeControl}><ListFilter label="时间范围" caption="时间" value={filters.range} onChange={(range) => {
          if (range === 'custom') { filterTrigger.current = rangeControl.current?.querySelector('button') ?? null; setFilterOpen(true); }
          else setFilters(tenantAuditRange(range));
        }} options={ranges} /></div>
        <Button variant="outline" className="h-9 max-sm:h-11" onClick={(e) => { filterTrigger.current = e.currentTarget; setFilterOpen(true); }}><SlidersHorizontal aria-hidden="true" />更多筛选</Button>
        {(q || event || filters.from || filters.to) && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={reset}>重置<X aria-hidden="true" /></Button>}
      </>} />
      <ListPagination label="租户操作记录" count={list.items.length} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
      <p className="text-xs leading-5 text-muted-foreground">最新记录优先 · 时间按上海时区（Asia/Shanghai）显示{filters.range === 'custom' && ` · ${filters.from.replace('T', ' ') || '不限起始'} 至 ${filters.to.replace('T', ' ') || '不限结束'}`}</p>
    </div>
    {selected && <TenantAuditPanel key={`${selected.event.id}:${selected.tab}`} event={selected.event} tenantName={ctx.tenant.name} initialTab={selected.tab} close={() => setSelected(undefined)} returnFocus={returnFocus} />}
    {filterOpen && <TenantAuditFilters initial={filters} close={() => setFilterOpen(false)} apply={(next) => { setFilters(next); setFilterOpen(false); }} returnFocus={filterTrigger} />}
  </>;
}

function TenantAuditFilters({ initial, close, apply, returnFocus }: { initial: TimeFilters; close: () => void; apply: (value: TimeFilters) => void; returnFocus: RefObject<HTMLElement | null> }) {
  const [draft, setDraft] = useState(initial), [error, setError] = useState('');
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title="更多筛选" description="按操作发生时间缩小范围，开始与结束时间均使用上海时区。" returnFocus={returnFocus} fallbackFocusSelector="[data-tenant-audit-search]">
    <form className="space-y-6" onSubmit={(e) => {
      e.preventDefault();
      if ([draft.from, draft.to].some((value) => value && !Number.isFinite(new Date(`${value}+08:00`).getTime()))) { setError('请填写有效的日期和时间。'); return; }
      if (draft.from && draft.to && tenantAuditISO(draft.from) > tenantAuditISO(draft.to)) { setError('开始时间不能晚于结束时间。'); return; }
      apply({ ...draft, range: draft.from || draft.to ? (draft.from === initial.from && draft.to === initial.to ? initial.range : 'custom') : '' });
    }}>
      <div className="space-y-4"><h3 className="text-sm font-semibold">发生时间</h3>
        {[{ key: 'from', label: '开始时间（上海）' }, { key: 'to', label: '结束时间（上海）' }].map(({ key, label }) => <div key={key} className="space-y-2"><Label htmlFor={`tenant-audit-${key}`}>{label}</Label><Input id={`tenant-audit-${key}`} type="datetime-local" className="max-sm:h-11" value={draft[key as 'from' | 'to']} aria-invalid={Boolean(error)} aria-describedby={error ? 'tenant-audit-time-error' : undefined} onChange={(e) => { setError(''); setDraft({ ...draft, [key]: e.target.value }); }} /></div>)}
      </div>
      <div id="tenant-audit-time-error"><FormNotice error>{error}</FormNotice></div>
      <div className="flex flex-wrap gap-2"><Button type="submit" className="max-sm:h-11">应用筛选</Button><Button type="button" variant="outline" className="max-sm:h-11" onClick={() => { setDraft(emptyFilters); setError(''); }}>清空条件</Button><Button type="button" variant="ghost" className="max-sm:h-11" onClick={close}>取消</Button></div>
    </form>
  </ResponsivePanel>;
}
