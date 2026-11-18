import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useEffect, useState } from 'react';
import { Check, ChevronDown, ChevronRight, FolderTree, Search } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { useMonitoring } from './context';
import { MonitoringError } from './common';
import type { SelectedChannel } from './channel-picker';
import { channelAppearance } from './trend-model';

const ownerNames = { facility: '设施', component: '构件', monitoring_section: '监测断面', borehole: '孔位', point: '测点' };
const ownerKey = (channel: SelectedChannel) => `${channel.owner_kind}:${channel.owner_id}`;

export function ComparisonChannelPicker({ selected, current, onChange }: { selected: SelectedChannel[]; current?: SelectedChannel; onChange: (channels: SelectedChannel[]) => void }) {
  const m = useMonitoring();
  const [search, setSearch] = useState(''), [debounced, setDebounced] = useState(''), [onlySelected, setOnlySelected] = useState(false);
  const [source, setSource] = useState<'' | 'direct' | 'derived'>(''), [owner, setOwner] = useState<'' | 'facility' | 'component' | 'point'>('');
  const [expanded, setExpanded] = useState<string[]>(() => [...new Set([...selected, ...(current ? [current] : [])].flatMap(channel => [channel.facility_id, ownerKey(channel)]))]);
  useEffect(() => { const timer = setTimeout(() => setDebounced(search.trim()), 200); return () => clearTimeout(timer); }, [search]);
  const directory = usePagedQuery({ queryKey: [...m.prefix, 'comparison-directory', debounced, source, owner], enabled: m.available && m.canNavigate && m.canRead && !onlySelected, staleTime: 30_000, retry: false, queryFn: ({ cursor, signal }) => m.client.monitoringDirectory(m.scope, { cursor: cursor || undefined, search: debounced || undefined, source_kind: source || undefined, owner_kind: owner || undefined }, signal) });
  const loaded = directory.data?.items ?? [];
  const items = onlySelected ? selected.filter(channel => (!source || channel.source_kind === source) && (!owner || channel.owner_kind === owner) && `${channel.name} ${channel.code} ${channel.facility_name} ${channel.owner_name} ${channel.owner_code}`.toLocaleLowerCase().includes(debounced.toLocaleLowerCase())) : [...new Map([...(current && !debounced && !source && !owner ? [current] : []), ...loaded].map(channel => [channel.id, channel])).values()];
  const facilities = [...new Set(items.map(channel => channel.facility_id))];
  const toggle = (key: string) => setExpanded(value => value.includes(key) ? value.filter(item => item !== key) : [...value, key]);
  return <section className="comparison-directory" aria-label="比较通道目录">
    <div className="flex items-center justify-between gap-2 px-4 py-4"><h2 className="flex items-center gap-2 text-sm font-semibold"><FolderTree className="size-4" aria-hidden />选择通道</h2><span className="text-xs text-muted-foreground">{selected.length} / 50</span></div>
    <div className="space-y-3 px-4 pb-3">
      <div className="relative"><Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" aria-hidden /><Input className="pl-9" aria-label="搜索比较通道" placeholder="通道、设施、所属对象或编号" maxLength={120} value={search} onChange={event => setSearch(event.target.value)} /></div>
      <div className="grid grid-cols-2 gap-2"><label className="text-xs text-muted-foreground">来源类型<select className="comparison-select mt-1 w-full" value={source} onChange={event => setSource(event.target.value as typeof source)}><option value="">全部来源</option><option value="direct">直接观测</option><option value="derived">派生通道</option></select></label><label className="text-xs text-muted-foreground">所属对象<select className="comparison-select mt-1 w-full" value={owner} onChange={event => setOwner(event.target.value as typeof owner)}><option value="">全部对象</option><option value="facility">设施</option><option value="component">构件</option><option value="point">测点</option></select></label></div>
      <div className="flex gap-2" aria-label="通道目录范围"><Button size="sm" variant={onlySelected ? 'ghost' : 'secondary'} aria-pressed={!onlySelected} onClick={() => setOnlySelected(false)}>全部通道</Button><Button size="sm" variant={onlySelected ? 'secondary' : 'ghost'} aria-pressed={onlySelected} onClick={() => setOnlySelected(true)}>已选 {selected.length}</Button></div>
    </div>
    <div className="comparison-tree">
      {!onlySelected && <><MonitoringError error={directory.error} retry={() => void directory.refetch()} />{directory.isPending && <p role="status" className="p-3 text-sm text-muted-foreground">正在加载通道…</p>}</>}
      {facilities.map(facility => {
        const children = items.filter(channel => channel.facility_id === facility), open = expanded.includes(facility) || !!debounced || onlySelected;
        return <div key={facility}><button type="button" className="comparison-tree-heading" aria-expanded={open} onClick={() => toggle(facility)}>{open ? <ChevronDown /> : <ChevronRight />}<span>{children[0]!.facility_name}</span><small>{children.length}</small></button>{open && [...new Set(children.map(ownerKey))].map(key => {
          const peers = children.filter(channel => ownerKey(channel) === key), channel = peers[0]!, opened = expanded.includes(key) || !!debounced || onlySelected;
          return <div key={key} className="comparison-owner"><button type="button" className="comparison-tree-heading text-muted-foreground" aria-expanded={opened} onClick={() => toggle(key)}>{opened ? <ChevronDown /> : <ChevronRight />}<span>{channel.owner_name}<small className="ml-1 font-normal">{ownerNames[channel.owner_kind]}</small></span><small>{peers.filter(peer => selected.some(item => item.id === peer.id)).length || ''}</small></button>{opened && peers.map(channel => {
            const checked = selected.some(item => item.id === channel.id);
            return <label key={channel.id} className={`comparison-channel ${checked ? 'is-selected' : ''}`}><input type="checkbox" className="size-4 shrink-0 accent-primary" checked={checked} disabled={!checked && selected.length >= 50} aria-label={`${channel.owner_name} ${channel.name} ${channel.code}`} onChange={() => onChange(checked ? selected.filter(item => item.id !== channel.id) : [...selected, channel])} /><span className="min-w-0 flex-1"><strong className="block break-words text-xs font-medium">{channel.name}</strong><small className="mt-1 block break-all text-[11px] text-muted-foreground">{channel.source_kind === 'derived' ? '派生' : '直接观测'} · {channel.unit || (channel.value_kind === 'enum' ? '状态' : '无单位')}{channel.status !== 'normal' && ' · 历史资源'}</small></span>{checked && <span aria-hidden style={{ color: channelAppearance(channel).color }}>{channelAppearance(channel).mark}</span>}</label>;
          })}</div>;
        })}</div>;
      })}
      {!items.length && (onlySelected || !directory.isPending && !directory.error) && <p className="p-3 text-xs text-muted-foreground">{onlySelected && !selected.length ? '尚未选择通道。切换到全部通道开始比较。' : '没有符合条件的可读通道。试试其他名称或编号。'}</p>}
      {!onlySelected && directory.data && <ListPagination label="通道" {...directory.pagination} />}
    </div>
    <div className="comparison-directory-footer"><span className="flex items-center gap-1"><Check className="size-3" aria-hidden />{selected.length >= 50 ? '已达 50 个通道上限' : '勾选即加入趋势图'}</span><Button size="sm" variant="ghost" disabled={!selected.length} onClick={() => onChange([])}>清空选择</Button></div>
  </section>;
}
