import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { ArrowRight, CircleDot, Cpu, Link2 } from 'lucide-react';
import { Permissions as P, type DeviceOutput } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { cn } from '@infrapro/ui/lib/utils';
import { useDevices } from '../devices/context';
import { useBindingDirectory, BindingFilters, FacilityFilter } from './directory';
import { CandidatePicker } from './candidates';
import { BindingControls, BindingDialog, type BindingEdit } from './editor';
import { BindingStatus, ChannelIdentity, CurrentEquipment, DesignMeaning, PageButtons, PairSummary } from './shared';
import { targetOf, type BindingSearch, type BindingRow } from './model';

export function BindingWorkspace({ search, change }: { search: BindingSearch; change: (value: BindingSearch) => void }) {
  const { client, scope, prefix, can, available } = useDevices();
  const [message, setMessage] = useState('');
  const [edit, setEdit] = useState<BindingEdit>();
  const [selection, setSelection] = useState<{ channel: string; output: DeviceOutput }>();
  const list = useBindingDirectory({ q: search.q?.trim(), facility_id: search.facility, parent_id: search.point, state: search.state });
  const selected = useQuery({ queryKey: [...prefix, 'binding-row', search.channel], enabled: !!search.channel && can(P.TenantEngineeringResourceRead), queryFn: async ({ signal }) => {
    if (scope.kind !== 'tenant') throw new Error('需要租户工作台');
    const page = await client.deviceBindingDirectory(scope, { channel_id: search.channel, limit: 1 }, signal);
    if (!page.items[0]) throw new Error('业务通道不存在或不可访问。');
    return page.items[0];
  } });
  const current = useQuery({ queryKey: [...prefix, 'source', selected.data ? targetOf(selected.data) : null], enabled: !!selected.data, queryFn: ({ signal }) => client.deviceSource(scope, targetOf(selected.data!), signal) });
  const output = selection && selection.channel === search.channel ? selection.output : undefined;
  const counts = list.data?.pages[0]?.counts;
  const updateFilter = (next: Partial<BindingSearch>) => change({ ...search, ...next, channel: undefined });
  if (!can(P.TenantEngineeringResourceRead)) return <ErrorState message="没有工程资源查看权限，无法访问绑定目录。" />;
  return <main className="min-w-0 space-y-5"><header className="space-y-2"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Link2 className="size-6" aria-hidden="true" />测点与设备绑定</h1><p className="text-sm text-muted-foreground">先选业务通道，再选择提供观测数据的设备通道，核对后完成绑定。</p></header>
    <FacilityFilter value={search.facility} choose={facility => updateFilter({ facility, point: undefined })} />
    {search.point && <FormNotice>当前限定为指定测点。<Button variant="link" onClick={() => updateFilter({ point: undefined })}>查看其他测点</Button></FormNotice>}
    <BindingFilters search={search.q ?? ''} state={search.state} counts={counts} changeSearch={q => updateFilter({ q: q || undefined })} changeState={state => updateFilter({ state })} />
    {message && <div role="status" className="rounded-lg border bg-primary/5 p-4 text-sm">{message}</div>}
    <div className="grid min-w-0 items-start gap-5 xl:grid-cols-2"><section className="min-w-0 overflow-hidden rounded-xl border bg-card" aria-label="业务通道列表"><div className="space-y-1 border-b bg-muted/20 p-4"><h2 className="flex items-center gap-2 font-semibold"><CircleDot className="size-4" aria-hidden="true" />1. 选择业务通道</h2><p className="text-xs text-muted-foreground">{counts ? `当前范围 ${counts.all} 个直接通道 · ${counts.unbound} 个待绑定 · ${counts.attention} 个需处理` : '按设施和测点查找需要接入数据的业务通道'}</p></div><div className="max-h-[38rem] overflow-y-auto">{list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => { list.go(0); void list.refetch(); }} /> : !list.items.length ? <EmptyState title="没有符合条件的业务通道" description="可调整搜索或状态筛选；派生通道使用派生计算，无需绑定设备。" /> : list.items.map(row => <button key={row.channel_id} aria-pressed={search.channel === row.channel_id} className={cn('flex w-full cursor-pointer items-start justify-between gap-3 border-b border-l-2 border-l-transparent p-4 text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring', search.channel === row.channel_id && 'border-l-primary bg-primary/5')} onClick={() => change({ ...search, channel: row.channel_id })}><ChannelIdentity row={row} /><BindingStatus state={row.state} /></button>)}</div><div className="p-3"><PageButtons page={list.page} hasNext={list.hasNext} busy={list.isFetching} previous={() => list.go(list.page - 1)} next={() => void list.next()} label="通道" /></div></section>
      <section className="min-w-0 rounded-xl border bg-card" aria-label="设备通道配对"><div className="space-y-1 border-b bg-muted/20 p-4"><h2 className="flex items-center gap-2 font-semibold"><Cpu className="size-4" aria-hidden="true" />2. 选择设备通道</h2><p className="text-xs text-muted-foreground">{selected.data ? `为「${selected.data.name}」选择设备输出` : '选中业务通道后，显示符合设计要求的设备'}</p></div><div className="p-4">{!search.channel ? <EmptyState title="请先选择业务通道" description="左侧是监测设计，右侧是现场设备，通过绑定衔接。" /> : selected.isPending ? <LoadingState /> : selected.error || !selected.data ? <ErrorState message={selected.error?.message ?? '无法读取通道'} onRetry={() => void selected.refetch()} /> : <Pairing key={selected.data.channel_id} row={selected.data} edit={setEdit} output={output} choose={output => setSelection({ channel: selected.data!.channel_id, output })} />}</div></section>
    </div>
    {selected.data && current.data && <section className="space-y-3 rounded-xl border bg-card p-4" aria-label="待确认的配对"><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-semibold">3. 核对配对</h2><p className="text-xs text-muted-foreground">选择好双方，再核对变更原因和生效范围。</p></div><PairSummary row={selected.data} output={output} /><div className="flex justify-end"><Button className="min-h-11 max-sm:w-full" disabled={!available || !output || !(current.data.actions.bind || current.data.actions.switch)} onClick={() => setEdit({ row: selected.data!, output, action: current.data!.source.configured ? 'switch' : 'bind' })}>{current.data.source.configured ? '核对并更换' : '核对并绑定'}<ArrowRight className="size-4" aria-hidden="true" /></Button></div></section>}
    <p className="text-xs text-muted-foreground">绑定到具体的设备输出代次；设备安装位置独立维护。已绑定表示关系已建立，数据接收情况请在监测数据中查看。</p>
    {edit && <BindingDialog edit={edit} close={() => setEdit(undefined)} saved={text => { setMessage(text); setSelection(undefined); setEdit(undefined); }} />}
  </main>;
}
function Pairing({ row, edit, output, choose }: { row: BindingRow; edit: (value: BindingEdit) => void; output?: DeviceOutput; choose: (output: DeviceOutput) => void }) {
  const { client, scope, prefix } = useDevices();
  const target = targetOf(row);
  const current = useQuery({ queryKey: [...prefix, 'source', target], queryFn: ({ signal }) => client.deviceSource(scope, target, signal) });
  if (current.isPending) return <LoadingState />;
  if (current.error || !current.data) return <ErrorState message={current.error?.message ?? '无法读取当前绑定'} onRetry={() => void current.refetch()} />;
  const source = current.data;
  return <div className="space-y-4"><div className="space-y-3 rounded-lg border p-3"><p className="text-xs text-muted-foreground">当前绑定</p><CurrentEquipment source={source.source} /><div className="flex flex-wrap items-center gap-2"><Button variant="ghost" className="min-h-11" onClick={() => edit({ row })}>查看关系与历史</Button><Button variant="ghost" className="min-h-11" asChild><Link to="/t/$tenantId/resources/facilities/$facilityId" params={{ tenantId: scope.tenantId, facilityId: row.facility_id }} search={{ point: row.point_id, view: 'bindings' }}>在设施中查看</Link></Button>{source.actions.unbind && <BindingControls current={{ ...source, actions: { ...source.actions, bind: false, switch: false } }} choose={action => edit({ row, action })} />}</div></div><DesignMeaning row={row} current={source} /><CandidatePicker target={target} current={source} selected={output?.id} choose={choose} /></div>;
}
