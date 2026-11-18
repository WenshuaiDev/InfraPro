import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { ArrowRight, CircleDot, Cpu, FolderTree } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { cn } from '@infrapro/ui/lib/utils';
import { useCursorList } from '../../hooks/use-cursor-list';
import { TenantDevices, useDevices } from '../devices/context';
import { BindingDialog, type BindingEdit } from './editor';
import { BindingFilters, useBindingDirectory } from './directory';
import { BindingStatus, ChannelIdentity, CurrentEquipment } from './shared';
import type { BindingRow } from './model';

type Props = { facility: string; point?: string; name: string; blocked: boolean };
export function FacilityBindings(props: Props) { return <TenantDevices><FacilityContent key={`${props.facility}:${props.point ?? ''}`} {...props} /></TenantDevices>; }
function FacilityContent({ facility, point, name, blocked }: Props) {
  const { scope, canRead, canSource, available } = useDevices();
  const [selectedPoint, setPoint] = useState(point);
  const [search, setSearch] = useState('');
  const [state, setState] = useState<BindingRow['state']>();
  const [edit, setEdit] = useState<BindingEdit>();
  const [message, setMessage] = useState('');
  const list = useBindingDirectory({ facility_id: facility, parent_id: selectedPoint, q: search.trim(), state });
  const counts = list.data?.counts;
  const groups = [...new Map(list.items.map(row => [row.point_id, row.point_name]))];
  return <section className="min-w-0 space-y-4" aria-label="设施设备绑定"><header className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="font-semibold">{name} · 设备绑定</h3><p className="mt-1 text-sm text-muted-foreground">对照监测设计，为业务通道选择现场设备。</p></div><Button asChild variant="outline" className="min-h-11"><Link to="/t/$tenantId/bindings" params={{ tenantId: scope.tenantId }} search={{ facility, point: selectedPoint }}>打开统一绑定入口<ArrowRight className="size-4" aria-hidden="true" /></Link></Button></header>{message && <div role="status" className="rounded-lg border bg-primary/5 p-3 text-sm">{message}</div>}
    <div className={cn('grid min-w-0 items-start gap-4', !point && '2xl:grid-cols-[13rem_minmax(0,1fr)]')}>{!point && <PointList facility={facility} selected={selectedPoint} choose={setPoint} />}<div className="min-w-0 space-y-4"><BindingFilters search={search} state={state} changeSearch={setSearch} changeState={setState} counts={counts} />{counts && <p className="text-xs text-muted-foreground" role="status">当前范围 {counts.all} 个直接通道 · {counts.unbound} 个待绑定 · {counts.attention} 个需处理</p>}
      {list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => { list.go(0); void list.refetch(); }} /> : !list.items.length ? <EmptyState title="暂无符合条件的业务通道" description="请先在监测设计中添加直接业务通道，或调整当前筛选。派生通道使用计算配置。" /> : groups.map(([id, pointName]) => <section key={id} className="space-y-3" aria-label={pointName}><h4 className="flex items-center gap-2 text-sm font-semibold"><CircleDot className="size-4" aria-hidden="true" />{pointName}</h4>{list.items.filter(row => row.point_id === id).map(row => <article key={row.channel_id} className="space-y-4 rounded-xl border bg-card p-4"><div className="flex items-start justify-between gap-2"><ChannelIdentity row={row} location={false} /><BindingStatus state={row.state} /></div><div className="grid min-w-0 gap-4 rounded-lg border bg-muted/10 p-4 lg:grid-cols-2"><div className="min-w-0 space-y-2"><p className="flex items-center gap-2 text-xs text-muted-foreground"><CircleDot className="size-4" aria-hidden="true" />监测设计 · 业务通道</p><p className="text-sm">{row.metric_name}{row.unit_symbol && ` · ${row.unit_symbol}`}</p><p className="break-words text-xs text-muted-foreground">{row.point_name} / {row.code}</p></div><div className="min-w-0 space-y-2 border-t pt-4 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-4"><p className="flex items-center gap-2 text-xs text-muted-foreground"><Cpu className="size-4" aria-hidden="true" />现场实施 · 设备通道</p><CurrentEquipment source={row.source} /></div></div><div className="flex flex-wrap gap-2">{canRead && canSource && row.state !== 'restricted' && <>{row.state !== 'retired' && <Button className="min-h-11" variant={row.source.configured ? 'outline' : 'default'} disabled={blocked || !available} onClick={() => setEdit({ row, action: row.source.configured ? 'switch' : 'bind' })}>{row.source.configured ? '更换设备' : '绑定设备'}</Button>}{row.source.configured && <Button className="min-h-11" variant="ghost" disabled={blocked || !available} onClick={() => setEdit({ row, action: 'unbind' })}>解除绑定</Button>}</>}<Button className="min-h-11" variant="ghost" onClick={() => setEdit({ row })}>设计要求与绑定历史</Button></div></article>)}</section>)}
      <ListPagination label="通道" {...list.pagination} previousLabel="上一页通道" nextLabel="下一页通道" /></div></div>
    {edit && <BindingDialog edit={edit} close={() => setEdit(undefined)} saved={text => { setEdit(undefined); setMessage(text); }} />}
  </section>;
}
function PointList({ facility, selected, choose }: { facility: string; selected?: string; choose: (point?: string) => void }) {
  const { client, scope, prefix } = useDevices();
  const [search, setSearch] = useState('');
  const list = useCursorList({ initialPageSize: 10, queryKey: [...prefix, 'binding-points', facility, search], load: (cursor, signal, limit) => client.resourceList(scope, { kind: 'point', facility }, { q: search, limit, cursor }, signal) });
  return <aside className="space-y-3 rounded-lg border bg-muted/15 p-3" aria-label="选择测点"><h3 className="flex items-center gap-2 text-sm font-semibold"><FolderTree className="size-4" aria-hidden="true" />按测点配置</h3><DirectorySearch label="搜索测点" value={search} onChange={setSearch} /><Button className="min-h-11 w-full justify-start" variant={!selected ? 'secondary' : 'ghost'} onClick={() => choose()}>全部测点</Button>{list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : <div className="grid gap-1 sm:grid-cols-2 2xl:grid-cols-1">{list.items.map(item => <Button key={item.id} className="h-auto min-h-11 justify-start whitespace-normal text-left" variant={selected === item.id ? 'secondary' : 'ghost'} aria-pressed={selected === item.id} onClick={() => choose(item.id)}>{item.name}</Button>)}</div>}{list.data && <ListPagination label="测点" {...list.pagination} />}</aside>;
}
