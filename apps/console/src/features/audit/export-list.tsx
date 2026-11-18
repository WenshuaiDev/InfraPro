import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useState } from 'react';
import { useIdentity } from '@infrapro/api-client/react';
import type { AuditScope } from '@infrapro/api-client';
import { ChevronRight, Download, RefreshCw, SlidersHorizontal } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { TaskStatus } from '../tasks/common';
import { shanghaiInput, shanghaiISO } from './filter-state';
import { date } from './presentation';

type State = '' | 'queued' | 'running' | 'retry_wait' | 'cancelling' | 'succeeded' | 'failed' | 'cancelled';
export function ExportList({ scope, queryKey, open, canManage }: { canManage: boolean; scope: AuditScope; queryKey: readonly unknown[]; open: (id: string, generation: string) => void }) {
  const { client } = useIdentity();
  const [mode, setMode] = useState<'mine' | 'managed'>('mine'), [state, setState] = useState<State>('');
  const [window, setWindow] = useState({ from: '', to: '' }), [editing, setEditing] = useState(false);
  const q = usePagedQuery({ queryKey: [...queryKey, 'exports', mode, state, window], retry: false, refetchInterval: 10000, refetchIntervalInBackground: false, queryFn: ({ cursor, signal }) => client.auditExports(scope, { cursor, mode, state: state || undefined, from: window.from || undefined, to: window.to || undefined }, signal) });
  const items = q.error ? [] : q.data?.items ?? [];
  return <section className="space-y-4" aria-label="审计导出任务">
    <p className="rounded-lg bg-muted/40 px-4 py-3 text-xs leading-5 text-muted-foreground">导出固定申请时的查询范围。下载时重新核验权限；管理视图可查看和处理任务，仅本人申请的成果可以下载。</p>
    <div className="flex flex-wrap items-center gap-2">{canManage && <label><span className="sr-only">导出范围</span><select aria-label="导出范围" className={selectClass} value={mode} onChange={event => setMode(event.target.value as 'mine' | 'managed')}><option value="mine">本人申请</option><option value="managed">当前空间管理</option></select></label>}<label><span className="sr-only">执行状态</span><select aria-label="执行状态" className={selectClass} value={state} onChange={event => setState(event.target.value as State)}><option value="">全部状态</option>{Object.entries({ queued: '排队中', running: '执行中', retry_wait: '等待自动重试', cancelling: '正在取消', succeeded: '已完成', failed: '失败', cancelled: '已取消' }).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label><Button variant="outline" className="h-9 max-sm:h-11" onClick={() => setEditing(true)}><SlidersHorizontal aria-hidden="true" />申请时间</Button>{(window.from || window.to || state) && <Button variant="ghost" className="h-9" onClick={() => { setWindow({ from: '', to: '' }); setState(''); }}>重置</Button>}<Button variant="outline" size="icon" className="ml-auto size-9 max-sm:size-11" aria-label="刷新导出任务" disabled={q.isFetching} onClick={() => void q.refetch()}><RefreshCw aria-hidden="true" className={q.isFetching ? 'motion-safe:animate-spin' : ''} /></Button></div>
    {(window.from || window.to) && <p className="text-xs text-muted-foreground">申请时间：{window.from ? date(window.from) : '不限'} 至 {window.to ? date(window.to) : '不限'} · 上海时区</p>}
    {q.error && <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />}
    {q.isPending && <p role="status" className="py-12 text-center text-sm text-muted-foreground">正在核实导出任务…</p>}
    {!q.isPending && !q.error && !items.length && <div className="rounded-lg border p-12 text-center text-sm text-muted-foreground">当前没有可见导出任务。</div>}
    {items.length > 0 && <ul className="divide-y overflow-hidden rounded-lg border">{items.map(e => <li key={`${e.id}:${e.generation}`} className="flex flex-wrap items-center gap-4 p-4"><span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted/50 text-muted-foreground"><Download aria-hidden="true" className="size-5" /></span><div className="min-w-0 flex-1"><Button variant="link" className="h-auto min-h-7 max-w-full max-sm:min-h-11 justify-start whitespace-normal p-0 text-left text-sm" onClick={() => open(e.id, e.generation)}>审计导出 · {date(e.created_at)}</Button><p className="mt-1 text-xs leading-5 text-muted-foreground">第 {e.generation} 代 · {e.owned ? '本人申请' : '管理视图，正文不可下载'}{e.row_count != null && ` · ${Number(e.row_count).toLocaleString()} 条记录`}</p></div><div className="flex flex-wrap items-center gap-3 max-sm:w-full max-sm:justify-between"><TaskStatus task={e} /><Button variant="outline" size="sm" className="h-9 max-sm:h-11" onClick={() => open(e.id, e.generation)}>查看详情<ChevronRight aria-hidden="true" className="size-3.5" /></Button></div></li>)}</ul>}
    <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><p>{!q.isPending && !q.error && `已显示 ${items.length} 项导出任务`}</p>{q.data && <ListPagination label="导出" {...q.pagination} />}</div>
    {editing && <ExportTime value={window} close={() => setEditing(false)} apply={next => { setWindow(next); setEditing(false); }} />}
  </section>;
}
const selectClass = 'h-9 max-w-full rounded-md border bg-background px-3 text-sm max-sm:h-11';
function ExportTime({ value, close, apply }: { value: { from: string; to: string }; close: () => void; apply: (value: { from: string; to: string }) => void }) {
  const [draft, setDraft] = useState(value), [error, setError] = useState('');
  return <ResponsivePanel open onOpenChange={open => { if (!open) close(); }} title="筛选导出申请时间" description="按上海时区查询，不填时不限制对应时间边界。" footer={<><Button variant="outline" onClick={() => { setDraft({ from: '', to: '' }); setError(''); }}>重置条件</Button><Button type="submit" form="audit-export-time">应用筛选</Button></>}><form id="audit-export-time" className="space-y-4" onSubmit={event => { event.preventDefault(); if (draft.from && draft.to && Date.parse(draft.from) >= Date.parse(draft.to)) { setError('结束时间必须晚于开始时间。'); return; } apply(draft); }}>{(['from', 'to'] as const).map(key => <label className="grid gap-2 text-sm" key={key}>申请{key === 'from' ? '开始' : '结束'}时间（上海时区）<Input type="datetime-local" step="1" value={shanghaiInput(draft[key])} onChange={event => setDraft({ ...draft, [key]: shanghaiISO(event.target.value) ?? '' })} /></label>)}{error && <p role="alert" className="text-sm text-destructive">{error}</p>}</form></ResponsivePanel>;
}
