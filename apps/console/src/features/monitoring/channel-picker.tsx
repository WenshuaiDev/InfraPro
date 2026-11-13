import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { MonitoringChannel, MonitoringPoint } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useMonitoring } from './context';
import { CursorButtons, MonitoringError } from './common';
import { rangeLabel } from './format';
import { PositionView } from '../location-fields/position-editor';

export type SelectedChannel = MonitoringChannel;
type PickerProps = { selected: SelectedChannel[]; onChange: (channels: SelectedChannel[]) => void; maximum?: number; autoSelect?: boolean; initialPoint?: string; initialChannel?: string; sourceKind?: 'direct' | 'derived' };
const ownerNames = { facility: '设施', component: '构件', point: '测点' };
const selectClass = 'min-h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';
export function ChannelPicker({ selected, onChange, maximum = 50, autoSelect = false, initialPoint, initialChannel, sourceKind }: PickerProps) {
  const { available, canNavigate, canRead } = useMonitoring();
  const [mode, setMode] = useState<'all' | 'points'>(initialPoint ? 'points' : 'all');
  if (!canNavigate || !canRead) return <p className="text-sm">选择通道需要当前工作空间的资源查看及监测数据查看权限。</p>;
  return <section className="min-w-0 space-y-4" aria-label="选择监测通道">
    <h2 className="font-semibold">选择监测通道</h2>
    <div className="flex flex-wrap gap-2" aria-label="通道浏览方式">
      <Button size="sm" variant={mode === 'all' ? 'secondary' : 'outline'} aria-pressed={mode === 'all'} onClick={() => setMode('all')}>全部对象</Button>
      <Button size="sm" variant={mode === 'points' ? 'secondary' : 'outline'} aria-pressed={mode === 'points'} onClick={() => setMode('points')}>按测点浏览</Button>
    </div>
    {available && (mode === 'all' ? <DirectoryChannels selected={selected} onChange={onChange} maximum={maximum} initialChannel={initialChannel} sourceKind={sourceKind} /> : <PointPicker selected={selected} onChange={onChange} maximum={maximum} autoSelect={autoSelect} initialPoint={initialPoint} initialChannel={initialChannel} sourceKind={sourceKind} />)}
    {selected.length > 0 && <div className="space-y-2 border-t pt-4">
      <div className="flex items-center justify-between gap-2"><h3 className="text-sm font-medium">已选 {selected.length} / {maximum}</h3><Button size="sm" variant="ghost" onClick={() => onChange([])}>清空选择</Button></div>
      <ul className="space-y-1">{selected.map(c => <li key={c.id} className="flex items-start justify-between gap-2 text-sm"><span className="min-w-0 break-words">{c.owner_name} · {c.name}<span className="block text-xs text-muted-foreground">{c.source_kind === 'derived' ? '派生' : '直接观测'} · {c.metric_name} · {c.unit || '枚举'}</span></span><Button size="sm" variant="ghost" aria-label={`移除 ${c.owner_name} ${c.name}`} onClick={() => onChange(selected.filter(v => v.id !== c.id))}>移除</Button></li>)}</ul>
    </div>}
  </section>;
}
function DirectoryChannels({ selected, onChange, maximum = 50, initialChannel, sourceKind }: PickerProps) {
  const { client, scope, prefix } = useMonitoring();
  const [search, setSearch] = useState(''), [debounced, setDebounced] = useState('');
  const [source, setSource] = useState<'' | 'direct' | 'derived'>(sourceKind ?? '');
  const [owner, setOwner] = useState<'' | 'facility' | 'component' | 'point'>('');
  const initialized = useRef(false);
  useEffect(() => { const timer = setTimeout(() => setDebounced(search), 200); return () => clearTimeout(timer); }, [search]);
  const channels = useCursorList({ queryKey: [...prefix, 'channel-directory', debounced, source, owner], load: (cursor, signal) => client.monitoringDirectory(scope, { cursor, search: debounced, source_kind: source || undefined, owner_kind: owner || undefined }, signal) });
  const linked = useQuery({ queryKey: [...prefix, 'linked-directory-channel', initialChannel], queryFn: ({ signal }) => client.monitoringDirectory(scope, { channel_id: initialChannel }, signal), enabled: !!initialChannel, retry: false });
  useEffect(() => {
    if (!initialized.current && linked.data) {
      initialized.current = true;
      if (selected.length === 0 && linked.data.items[0] && (!sourceKind || linked.data.items[0].source_kind === sourceKind)) onChange([linked.data.items[0]]);
    }
  }, [linked.data, selected.length, onChange, sourceKind]);
  return <div className="min-w-0 space-y-3">
    <Input aria-label="搜索通道或所属对象" placeholder="通道、设施、构件或测点名称" maxLength={120} value={search} onChange={e => setSearch(e.target.value)} />
    <div className="grid grid-cols-2 gap-2">
      <label className="space-y-1 text-xs">来源类型<select className={selectClass} value={source} disabled={!!sourceKind} onChange={e => setSource(e.target.value as typeof source)}><option value="">全部来源</option><option value="direct">直接观测</option><option value="derived">派生通道</option></select></label>
      <label className="space-y-1 text-xs">所属对象<select className={selectClass} value={owner} onChange={e => setOwner(e.target.value as typeof owner)}><option value="">全部对象</option><option value="facility">设施</option><option value="component">构件</option><option value="point">测点</option></select></label>
    </div>
    <MonitoringError error={linked.error} />
    {initialChannel && linked.data && !linked.data.items.length && <p className="text-xs">链接通道在当前范围不可读，请重新选择。</p>}
    <MonitoringError error={channels.error} retry={() => void channels.refetch()} />
    {channels.isPending && <p role="status">正在加载通道…</p>}
    <div className="max-h-96 space-y-2 overflow-y-auto" aria-label="对象通道列表">{channels.items.map(c => {
      const checked = selected.some(v => v.id === c.id);
      return <label key={c.id} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
        <input type="checkbox" className="mt-1 size-4 accent-primary" checked={checked} disabled={!checked && selected.length >= maximum} onChange={() => onChange(checked ? selected.filter(v => v.id !== c.id) : [...selected, c])} />
        <span className="min-w-0 text-sm"><span className="block break-words font-medium">{c.name} · {c.code}</span><span className="block break-words text-xs text-muted-foreground">{c.source_kind === 'derived' ? '派生' : '直接观测'} · {ownerNames[c.owner_kind]}：{c.owner_name}</span><span className="block break-words text-xs text-muted-foreground">{c.facility_name} · {c.metric_name} · {c.unit || '枚举'}{c.status !== 'normal' && ' · 历史资源'}</span>{c.read_ranges.some(r => r.from || r.to) && <span className="block text-xs">可读：{c.read_ranges.map(rangeLabel).join('；')}</span>}</span>
      </label>;
    })}</div>
    {!channels.isPending && !channels.error && !channels.items.length && <p className="text-sm text-muted-foreground">没有符合条件的可读通道。</p>}
    <CursorButtons previous={channels.page > 0 ? () => channels.go(channels.page - 1) : undefined} next={channels.hasNext ? () => void channels.next() : undefined} busy={channels.isFetching} caption={`通道第 ${channels.page + 1} 页`} />
  </div>;
}
function PointPicker({ selected, onChange, maximum = 50, autoSelect = false, initialPoint, initialChannel, sourceKind }: PickerProps) {
  const { client, scope, prefix } = useMonitoring();
  const [search, setSearch] = useState(''), [debounced, setDebounced] = useState(''), [point, setPoint] = useState<MonitoringPoint | null>(null);
  const initial = useQuery({ queryKey: [...prefix, 'initial-point', initialPoint], queryFn: ({ signal }) => client.monitoringPoints(scope, { point_id: initialPoint }, signal), enabled: !!initialPoint, retry: false });
  const initialized = useRef(false);
  useEffect(() => { if (!initialized.current && initial.data?.items[0]) { initialized.current = true; setPoint(initial.data.items[0]); } }, [initial.data]);
  useEffect(() => { const timer = setTimeout(() => setDebounced(search), 200); return () => clearTimeout(timer); }, [search]);
  const points = useCursorList({ queryKey: [...prefix, 'points', debounced], load: (cursor, signal) => client.monitoringPoints(scope, { cursor, search: debounced }, signal) });
  return <div className="min-w-0 space-y-3">
    <Input aria-label="搜索设施、测点名称或编号" placeholder="设施、测点名称或编号" value={search} onChange={e => setSearch(e.target.value)} />
    <MonitoringError error={points.error} retry={() => void points.refetch()} />
    {points.isPending && <p role="status">正在加载测点…</p>}
    <div className="max-h-72 space-y-1 overflow-y-auto" aria-label="测点列表">{points.items.map(p => <Button key={p.id} variant={point?.id === p.id ? 'secondary' : 'ghost'} className="h-auto min-h-11 w-full justify-start whitespace-normal px-3 py-2 text-left" aria-pressed={point?.id === p.id} onClick={() => setPoint(p)}><span className="min-w-0"><span className="block break-words font-medium">{p.name} <span className="text-xs text-muted-foreground">{p.code}</span></span><span className="block break-words text-xs text-muted-foreground">{p.facility_name}{p.status !== 'normal' ? ' · 历史资源' : ''}</span></span></Button>)}</div>
    {!points.isPending && !points.items.length && !points.error && <p className="text-sm text-muted-foreground">没有符合条件的可读测点。</p>}
    <CursorButtons previous={points.page > 0 ? () => points.go(points.page - 1) : undefined} next={points.hasNext ? () => void points.next() : undefined} busy={points.isFetching} caption={`测点第 ${points.page + 1} 页`} />
    {point && <PointChannels key={point.id} point={point} selected={selected} onChange={onChange} maximum={maximum} autoSelect={autoSelect} initialChannel={point.id === initialPoint ? initialChannel : undefined} sourceKind={sourceKind} />}
  </div>;
}
function PointChannels({ point, selected, onChange, maximum, autoSelect, initialChannel, sourceKind }: { point: MonitoringPoint; selected: SelectedChannel[]; onChange: (channels: SelectedChannel[]) => void; maximum: number; autoSelect: boolean; initialChannel?: string; sourceKind?: 'direct' | 'derived' }) {
  const { client, scope, prefix } = useMonitoring(); const [search, setSearch] = useState(''); const auto = useRef(false);
  const channels = useCursorList({ queryKey: [...prefix, 'point-channels', point.id], load: (cursor, signal) => client.monitoringChannels(scope, point.id, cursor, signal) });
  const linkedChannel=useQuery({queryKey:[...prefix,'linked-channel',point.id,initialChannel],queryFn:({signal})=>client.monitoringChannelByID(scope,point.id,initialChannel!,signal),enabled:!!initialChannel,retry:false});
  const withPoint = (c: MonitoringChannel): SelectedChannel => c;
  useEffect(() => {
    if (auto.current || !channels.data || initialChannel && !linkedChannel.data) return;
    auto.current = true;
    const linked=linkedChannel.data?.items[0];
    if(initialChannel){if(linked&&selected.length===0&&(!sourceKind||linked.source_kind===sourceKind))onChange([withPoint(linked)]);return;}
    if (autoSelect && selected.length === 0 && channels.items.length === 1 && !channels.hasNext && channels.items[0]!.value_kind !== 'enum' && (!sourceKind || channels.items[0]!.source_kind === sourceKind)) onChange([withPoint(channels.items[0]!)]);
    // Automatic selection happens only on the first completed page for this point.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channels.data,linkedChannel.data]);
  const items = channels.items.filter(c => (!sourceKind || c.source_kind === sourceKind) && `${c.name} ${c.code} ${c.metric_name}`.toLowerCase().includes(search.toLowerCase()));
  return <section className="min-w-0 space-y-3 rounded-lg border p-3"><h3 className="break-words text-sm font-semibold">{point.name} 的通道</h3><details className="text-xs"><summary className="cursor-pointer">测点位置</summary><PositionView position={point.position}/></details><Input aria-label="筛选本页通道" placeholder="筛选本页通道" value={search} onChange={e => setSearch(e.target.value)} /><MonitoringError error={linkedChannel.error}/>{initialChannel&&linkedChannel.data&&!linkedChannel.data.items.length&&<p className="text-xs">链接通道在当前范围不可读，请重新选择。</p>}<MonitoringError error={channels.error} retry={() => void channels.refetch()} />{channels.isPending && <p role="status">正在加载通道…</p>}<div className="max-h-80 space-y-2 overflow-y-auto">{items.map(c => { const checked = selected.some(v => v.id === c.id); return <label key={c.id} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"><input type="checkbox" className="mt-1 size-4 accent-primary" checked={checked} disabled={!checked && selected.length >= maximum} onChange={() => onChange(checked ? selected.filter(v => v.id !== c.id) : [...selected, withPoint(c)])} /><span className="min-w-0 text-sm"><span className="block break-words font-medium">{c.name} · {c.code}</span><span className="block break-words text-xs text-muted-foreground">{c.metric_name} · {c.unit || '枚举'} · {c.timestamp_meaning === 'observation_time' ? '观测时刻' : '区间结束时刻'}</span><span className="block break-words text-xs text-muted-foreground">{c.observation_direction || '方向未填'} · {c.reference_basis || '基准未填'}</span>{c.read_ranges.some(r => r.from || r.to) && <span className="block break-words text-xs">可读：{c.read_ranges.map(rangeLabel).join('；')}</span>}</span></label>; })}</div><CursorButtons previous={channels.page > 0 ? () => channels.go(channels.page - 1) : undefined} next={channels.hasNext ? () => void channels.next() : undefined} busy={channels.isFetching} caption={`通道第 ${channels.page + 1} 页`} /></section>;
}
