import { useEffect, useRef, useState } from 'react';
import { PageHeader } from '@infrapro/layouts';
import { Input } from '@infrapro/ui/components/input';
import { reason, type Page, type Transition, type Self } from '@infrapro/diagnostics-client';
import { retrySnapshot, useDiagnosticQuery } from './hooks';
import { label, timestamp } from './format';
import { DataTable, Feedback, Pager } from './patterns';
export function History() {
  const [kind, setKind] = useState(''); const [target, setTarget] = useState(''); const [cursors, setCursors] = useState(['']); const [notice, setNotice] = useState(''); const retried = useRef(false);
  const query = useDiagnosticQuery<Page<Transition>>('/transitions', { target_kind: kind, target_id: target, limit: 50, cursor: cursors.at(-1) }, 10000); const data = query.data?.data;
  useEffect(() => { if (retrySnapshot(query.error) && !retried.current) { retried.current = true; setCursors(['']); setNotice('历史快照已过期，已返回首页。'); } }, [query.error]);
  const retention = useDiagnosticQuery<Self>('/self', {}, 30000).data?.data.retention;
  const stateText = (value: string) => value.split('/').slice(0, 5).map(label).join(' · ');
  const reset = () => { retried.current = false; setCursors(['']); };
  return <div className="space-y-5"><PageHeader title="有限历史" description="记录状态和覆盖变化，不记录每次心跳；最多保留 24 小时、10,000 条变化。资源趋势最多保留 6 小时。" /><div className="flex flex-wrap gap-3"><label className="grid gap-1 text-sm">目标类型<select className="min-h-11 rounded-md border bg-background px-3" value={kind} onChange={(e) => { setKind(e.target.value); reset(); }}><option value="">全部</option>{['node', 'replica', 'endpoint', 'dependency'].map((v) => <option key={v} value={v}>{label(v)}</option>)}</select></label><label className="grid gap-1 text-sm">目标标识<Input value={target} onChange={(e) => { setTarget(e.target.value); reset(); }} placeholder="精确匹配标识" /></label></div>{retention && <p className="text-sm text-muted-foreground">实际保留起点：{timestamp(retention.actual_from)} · 已淘汰 {retention.evicted_count} 条</p>}{notice && <p className="text-sm">{notice}</p>}<Feedback error={query.error} meta={query.data?.meta} pending={query.isPending} />{data && <><DataTable><thead><tr><th scope="col">时间</th><th scope="col">目标</th><th scope="col">变化</th><th scope="col">原因</th></tr></thead><tbody>{data.items.map((item) => <tr key={item.id}><td>{timestamp(item.at)}</td><td>{item.target_id}<p className="text-xs text-muted-foreground">{item.retired ? '已从当前清单移除' : label(item.target_kind)}</p></td><td className="max-w-80 break-words">{item.from_state ? stateText(item.from_state) : '尚无证据'} → {stateText(item.to_state)}</td><td>{reason(item.reason_code).title}</td></tr>)}{data.items.length === 0 && <tr><td colSpan={4}>当前保留范围和筛选下没有状态变化。</td></tr>}</tbody></DataTable><Pager total={data.total} next={data.next_cursor} hasPrevious={cursors.length > 1} onNext={() => setCursors((old) => [...old, data.next_cursor])} onPrevious={() => setCursors((old) => old.slice(0, -1))} onRefresh={() => { reset(); void query.refetch(); }} /></>}</div>;
}
