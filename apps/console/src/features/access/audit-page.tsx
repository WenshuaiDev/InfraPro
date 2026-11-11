import { useRef, useState, type RefObject } from 'react';
import { RefreshCw, Search, SlidersHorizontal, X } from 'lucide-react';
import { Permissions as P, type AccessAuditEvent } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { AuditPanel } from './audit-panel';
import { auditActions, auditCategories, auditOutcomes, eventCategory } from './audit-presentation';
import { AuditTable } from './audit-table';
import { useCursorList } from './use-role-list';

type ExtraFilters = { from: string; to: string; target: string; tenant: string; range: string };
const emptyFilters: ExtraFilters = { from: '', to: '', target: '', tenant: '', range: '' };
const ranges = [{ value: '', label: '全部时间' }, { value: 'today', label: '今天' }, { value: 'week', label: '最近 7 天' }, { value: 'month', label: '最近 30 天' }, { value: 'custom', label: '自定义时间' }];
const iso = (value: string) => value ? new Date(value).toISOString() : '';
function localDateTime(date: Date) { const pad = (value: number) => String(value).padStart(2, '0'); return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`; }

export function AuditPage() {
  const { client, account, can } = useIdentity(), canTenant = can(P.PlatformTenantRead);
  const [category, setCategory] = useState(''), [event, setEvent] = useState(''), [q, setQ] = useState(''), [outcome, setOutcome] = useState('');
  const [filters, setFilters] = useState(emptyFilters), [pageSize, setPageSize] = useState(10), [filterOpen, setFilterOpen] = useState(false);
  const viewerKey = `${account?.id}:${canTenant}`;
  const [selected, setSelected] = useState<{ event: AccessAuditEvent; tab: string; viewerKey: string }>();
  const returnFocus = useRef<HTMLElement | null>(null), filterTrigger = useRef<HTMLElement | null>(null);
  const tenant = canTenant ? filters.tenant : '';
  const list = useCursorList({ queryKey: ['access', 'audit', viewerKey, category, event, q, outcome, filters.from, filters.to, filters.target, tenant, pageSize], load: (cursor, signal) => client.accessAudit({ category, event_type: event, q, outcome, from: iso(filters.from), to: iso(filters.to), object_id: filters.target, tenant_id: tenant, cursor, limit: pageSize }, signal) });
  const open = (value: AccessAuditEvent, trigger: HTMLElement | null, tab = 'details') => { returnFocus.current = trigger; setSelected({ event: value, tab, viewerKey }); };
  const reset = () => { setQ(''); setEvent(''); setOutcome(''); setCategory(''); setFilters(emptyFilters); };
  const setRange = (range: string) => {
    if (range === 'custom') { filterTrigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; setFilterOpen(true); return; }
    const start = new Date();
    if (range === 'today') start.setHours(0, 0, 0, 0);
    else start.setDate(start.getDate() - (range === 'week' ? 7 : 30));
    setFilters({ ...filters, range, from: range ? localDateTime(start) : '', to: '' });
  };
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return <>
    <PageHeader title="平台审计" description="追溯谁在什么时候，对什么做了什么，以及操作结果。" actions={<Button variant="outline" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw className={list.isFetching ? 'animate-spin' : undefined} aria-hidden="true" />刷新记录</Button>} />
    <Tabs value={category || 'all'} onValueChange={(value) => { setCategory(value === 'all' ? '' : value); setEvent(''); }} className="gap-5">
      <TabsList className="grid h-auto w-full grid-cols-2 sm:flex sm:w-fit">{auditCategories.map((item) => <TabsTrigger key={item.value} value={item.value || 'all'} className="max-sm:min-h-11">{item.label}</TabsTrigger>)}</TabsList>
      <TabsContent value={category || 'all'} className="space-y-4">
        <AuditTable items={list.items} open={open} loading={list.isPending} fetching={list.isFetching} error={list.error} retry={() => void list.refetch()} toolbar={<>
          <div className="relative w-full sm:w-64"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input data-audit-search aria-label="搜索名称或原因" placeholder="搜索名称、用户名或原因…" value={q} maxLength={200} className="h-9 pl-9 max-sm:h-11" onChange={(e) => setQ(e.target.value)} /></div>
          <ListFilter label="操作结果" caption="结果" value={outcome} onChange={setOutcome} options={auditOutcomes} />
          <ListFilter label="操作类型" caption="操作" value={event} onChange={setEvent} options={[{ value: '', label: '全部操作' }, ...Object.entries(auditActions).filter(([key]) => (!category || eventCategory(key) === category) && (canTenant || !key.startsWith('tenant.'))).map(([value, label]) => ({ value, label }))]} />
          <ListFilter label="时间范围" caption="时间" value={filters.range} onChange={setRange} options={ranges} />
          <Button variant="outline" className="h-9 max-sm:h-11" onClick={(e) => { filterTrigger.current = e.currentTarget; setFilterOpen(true); }}><SlidersHorizontal aria-hidden="true" />更多筛选{(filters.target || tenant) && <Badge variant="secondary">{Number(Boolean(filters.target)) + Number(Boolean(tenant))}</Badge>}</Button>
          {(q || event || outcome || category || filters.from || filters.to || filters.target || tenant) && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={reset}>重置<X aria-hidden="true" /></Button>}
        </>} />
        <ListPagination label="平台审计记录" count={list.items.length} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
        <p className="text-xs leading-5 text-muted-foreground">按发生时间从新到旧排列 · 时间按 {timezone} 显示{filters.range === 'custom' && ` · ${filters.from.replace('T', ' ') || '不限起始'} 至 ${filters.to.replace('T', ' ') || '不限结束'}`}</p>
      </TabsContent>
    </Tabs>
    {selected?.viewerKey === viewerKey && <AuditPanel key={`${selected.event.id}:${selected.tab}`} event={selected.event} initialTab={selected.tab} close={() => setSelected(undefined)} returnFocus={returnFocus} />}
    {filterOpen && <AuditFilters key={viewerKey} initial={filters} canTenant={canTenant} close={() => setFilterOpen(false)} apply={(next) => { setFilters(next); setFilterOpen(false); }} returnFocus={filterTrigger} />}
  </>;
}

function AuditFilters({ initial, canTenant, close, apply, returnFocus }: { initial: ExtraFilters; canTenant: boolean; close: () => void; apply: (filters: ExtraFilters) => void; returnFocus: RefObject<HTMLElement | null> }) {
  const [draft, setDraft] = useState(initial), [error, setError] = useState('');
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title="更多筛选" description="按时间缩小范围，或用已知 ID 精确定位记录。" returnFocus={returnFocus} fallbackFocusSelector="[data-audit-search]">
    <form className="space-y-6" onSubmit={(e) => {
      e.preventDefault();
      if ([draft.from, draft.to].some((value) => value && !Number.isFinite(new Date(value).getTime()))) { setError('请填写有效的日期和时间。'); return; }
      if (draft.from && draft.to && new Date(draft.from) > new Date(draft.to)) { setError('开始时间不能晚于结束时间。'); return; }
      if (canTenant && draft.tenant.trim() && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(draft.tenant.trim())) { setError('请输入完整的租户 ID，或使用列表搜索租户名称。'); return; }
      apply({ ...draft, target: draft.target.trim(), tenant: canTenant ? draft.tenant.trim() : '', range: draft.from || draft.to ? (draft.from === initial.from && draft.to === initial.to ? initial.range : 'custom') : '' });
    }}>
      <div className="space-y-4"><h3 className="text-sm font-semibold">发生时间</h3><div className="space-y-2"><Label htmlFor="audit-from">开始时间</Label><Input id="audit-from" type="datetime-local" value={draft.from} onChange={(e) => { setError(''); setDraft({ ...draft, from: e.target.value }); }} /></div><div className="space-y-2"><Label htmlFor="audit-to">结束时间</Label><Input id="audit-to" type="datetime-local" value={draft.to} onChange={(e) => { setError(''); setDraft({ ...draft, to: e.target.value }); }} /></div></div>
      <div className="space-y-4 border-t pt-5"><h3 className="text-sm font-semibold">精确定位</h3><p className="text-sm text-muted-foreground">通常直接搜索名称即可。排查问题时，可从记录的技术信息复制 ID。</p><div className="space-y-2"><Label htmlFor="audit-target">对象或关联账号 ID</Label><Input id="audit-target" value={draft.target} maxLength={200} onChange={(e) => setDraft({ ...draft, target: e.target.value })} /></div>{canTenant && <div className="space-y-2"><Label htmlFor="audit-tenant">租户 ID</Label><Input id="audit-tenant" value={draft.tenant} maxLength={36} onChange={(e) => { setError(''); setDraft({ ...draft, tenant: e.target.value }); }} /></div>}</div>
      <FormNotice error>{error}</FormNotice>
      <div className="flex flex-wrap gap-2"><Button type="submit">应用筛选</Button><Button type="button" variant="outline" onClick={() => { setDraft(emptyFilters); setError(''); }}>清空条件</Button><Button type="button" variant="ghost" onClick={close}>取消</Button></div>
    </form>
  </ResponsivePanel>;
}
