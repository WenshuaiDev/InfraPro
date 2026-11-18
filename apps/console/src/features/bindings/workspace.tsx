import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, Info, Link2, LocateFixed } from 'lucide-react';
import { Permissions as P, type DeviceOutput } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useDevices } from '../devices/context';
import { useBindingTreeDirectory, BindingFilters, FacilityFilter } from './directory';
import { CandidatePicker } from './candidates';
import { BindingDialog, type BindingEdit } from './editor';
import { BindingStatus, ChannelIdentity, DesignMeaning } from './shared';
import { CurrentBinding } from './current-binding';
import { BindingTree } from './tree';
import { bindingLocation, targetOf, visibleOutput, type BindingSearch } from './model';
import './workspace.css';

export function BindingWorkspace({ search, change }: { search: BindingSearch; change: (value: BindingSearch) => void }) {
  const { client, scope, prefix, can, available, canRead, canSource } = useDevices();
  const [message, setMessage] = useState('');
  const [edit, setEdit] = useState<BindingEdit>();
  const [meaning, setMeaning] = useState(false);
  const [selection, setSelection] = useState<{ channel: string; sourceRevision: string; output: DeviceOutput }>();
  const [locateVersion, setLocateVersion] = useState(0);
  const treeScroll = useRef<HTMLDivElement>(null);
  const list = useBindingTreeDirectory({ q: search.q?.trim(), facility_id: search.facility, parent_id: search.point, state: search.state });
  const rows = list.data?.items ?? [];
  const selected = useQuery({ queryKey: [...prefix, 'binding-row', search.channel], enabled: !!search.channel && can(P.TenantEngineeringResourceRead), queryFn: async ({ signal }) => {
    if (scope.kind !== 'tenant') throw new Error('需要租户工作台');
    const page = await client.deviceBindingDirectory(scope, { channel_id: search.channel, limit: 1 }, signal);
    if (!page.items[0]) throw new Error('业务通道不存在或不可访问。');
    return page.items[0];
  } });
  const row = !selected.error ? selected.data : undefined;
  const current = useQuery({ queryKey: [...prefix, 'source', row ? targetOf(row) : null], enabled: !!row, queryFn: ({ signal }) => client.deviceSource(scope, targetOf(row!), signal) });
  const source = !current.error ? current.data : undefined;
  const output = selection?.channel === search.channel && selection?.sourceRevision === source?.source_revision ? selection?.output : undefined;
  const counts = list.data?.counts;
  const currentOutput = source ? visibleOutput(source.source) : undefined;
  const updateFilter = (next: Partial<BindingSearch>) => { setSelection(undefined); change({ ...search, ...next, channel: undefined }); };
  const canReview = available && canRead && canSource && !!output && !!row && !!source && !current.isFetching && !selected.isFetching && (source.source.configured ? source.actions.switch : source.actions.bind);
  const locate = () => {
    if (!row) return;
    setLocateVersion(value => value + 1);
    requestAnimationFrame(() => {
      const leaf = treeScroll.current?.querySelector<HTMLElement>(`[data-binding-channel="${row.channel_id}"]`);
      leaf?.scrollIntoView({ block: 'nearest' });
      leaf?.focus({ preventScroll: true });
    });
  };
  if (!can(P.TenantEngineeringResourceRead)) return <ErrorState message="没有工程资源查看权限，无法访问绑定目录。" />;
  return <div className="@container/binding min-w-0 space-y-4">
    <header className="flex flex-wrap items-start justify-between gap-3"><div className="space-y-2"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Link2 className="size-6" aria-hidden="true" />测点与设备绑定</h1><p className="text-sm text-muted-foreground">按设施层级定位业务通道，查看当前设备，按需建立或调整绑定关系。</p></div><FacilityFilter value={search.facility} choose={facility => updateFilter({ facility, point: undefined })} /></header>
    {message && <div role="status" className="rounded-lg border bg-primary/5 px-4 py-2 text-sm">{message}</div>}
    <section className="binding-workbench overflow-hidden rounded-xl border bg-card" aria-label="绑定工作台">
      <div className="binding-columns">
        <section className="binding-tree-panel" aria-label="业务通道列表">
          <div className="binding-panel-heading"><h2 className="flex items-center gap-2 text-sm font-semibold"><span className="binding-step">1</span>选择业务通道</h2><Button size="sm" variant="ghost" className="min-h-9 gap-1 px-2 text-xs" disabled={!row} onClick={locate}><LocateFixed className="size-3.5" aria-hidden="true" />定位已选</Button></div>
          <div className="shrink-0 space-y-2 border-b p-3"><BindingFilters search={search.q ?? ''} state={search.state} counts={counts} changeSearch={q => updateFilter({ q: q || undefined })} changeState={state => updateFilter({ state })} />{search.point && <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>当前限定为指定测点</span><Button variant="link" size="sm" className="h-auto min-h-6 p-0 text-xs" onClick={() => updateFilter({ point: undefined })}>查看其他测点</Button></div>}</div>
          <div ref={treeScroll} className="binding-tree-scroll" tabIndex={0} role="region" aria-label="业务通道层级列表">
            {row && !rows.some(item => item.channel_id === row.channel_id) && <div className="border-b"><p className="px-3 pt-3 text-xs text-muted-foreground">当前定位 · 不在已加载结果中</p><BindingTree key={`selected:${row.channel_id}:${locateVersion}`} rows={[row]} selected={row.channel_id} choose={next => change({ ...search, channel: next.channel_id })} expandMatches /></div>}
            {list.isPending ? <LoadingState /> : list.error && !rows.length ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !rows.length ? <EmptyState title="没有符合条件的业务通道" description="可调整搜索或状态筛选；派生通道无需绑定设备。" /> : <BindingTree key={`${search.q ?? ''}:${search.state ?? ''}:${search.facility ?? ''}:${search.point ?? ''}:${locateVersion}`} rows={rows} selected={search.channel} choose={next => { setSelection(undefined); change({ ...search, channel: next.channel_id }); }} expandMatches={!!search.q || !!search.state || !!search.point} />}
            {list.error && !!rows.length && <ErrorState title="更多通道暂时无法读取" message={list.error.message} onRetry={() => void list.refetch()} />}
          </div>
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t bg-muted/20 px-3 py-2 text-xs text-muted-foreground">{list.data && <div className="w-full"><ListPagination label="业务通道" {...list.pagination} /></div>}</div>
        </section>
        <section className="binding-device-panel" aria-label="设备通道配对">
          <div className="binding-panel-heading"><h2 className="flex items-center gap-2 text-sm font-semibold"><span className="binding-step">2</span>选择设备通道</h2>{row && <BindingStatus state={row.state} />}</div>
          {!search.channel ? <EmptyState title="请先选择业务通道" description="展开左侧的设施、构件和测点，选择需要绑定的业务通道。" /> : selected.isPending ? <LoadingState /> : !row ? <ErrorState message={selected.error?.message ?? '无法读取通道'} onRetry={() => void selected.refetch()} /> : <>
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 p-3"><ChannelIdentity row={row} /><Button variant="ghost" size="sm" className="min-h-9 px-2 text-xs" disabled={!source} onClick={() => setMeaning(true)}>设计要求<Info className="size-3.5" aria-hidden="true" /></Button></div>
            {current.isPending ? <LoadingState /> : !source ? <ErrorState message={current.error?.message ?? '无法读取当前绑定'} onRetry={() => void current.refetch()} /> : <><CurrentBinding row={row} current={source} edit={setEdit} /><div className="binding-candidates"><CandidatePicker key={row.channel_id} target={targetOf(row)} current={source} selected={output?.id} choose={next => setSelection({ channel: row.channel_id, sourceRevision: source.source_revision, output: next })} fill /></div></>}
          </>}
        </section>
      </div>
      <footer className="binding-review border-t bg-muted/20 px-4 py-3" aria-label="待确认的配对"><div className="flex items-center gap-2"><span className="binding-step">3</span><h2 className="whitespace-nowrap text-sm font-semibold">核对配对</h2></div><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">{row ? <><strong className="break-words font-medium">{row.point_name} · {row.name}</strong><ArrowRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><span className="break-all">{output ? `${output.device.name} · ${output.device.code}` : currentOutput ? `当前：${currentOutput.device.code}` : source?.source.configured ? '当前设备详情受限' : '请选择设备通道'}</span>{output && <Badge variant="outline">待确认</Badge>}</> : <span className="text-muted-foreground">选择业务通道和设备通道后，在此核对配对。</span>}</div>{row && <p className="mt-1 break-words text-xs text-muted-foreground">{output ? (currentOutput ? `将更换当前设备 ${currentOutput.device.code}，核对原因和生效范围后提交。` : '将建立新的设备绑定，核对后提交。') : source?.source.configured ? '当前关系已建立；如需更换，请选择上方候选设备。' : '只有确认提交后，设备绑定才会生效。'}</p>}</div><div className="flex shrink-0 items-center justify-end gap-2">{output && <Button variant="ghost" size="sm" className="min-h-10" onClick={() => setSelection(undefined)}>取消选择</Button>}<Button className="min-h-10" disabled={!canReview} onClick={() => { if (row && source && output) setEdit({ row, output, action: source.source.configured ? 'switch' : 'bind' }); }}>{source?.source.configured ? '核对并更换' : '核对并绑定'}<ArrowRight className="size-4" aria-hidden="true" /></Button></div></footer>
    </section>
    <p className="flex items-start gap-2 text-xs text-muted-foreground"><Info className="size-3.5 shrink-0" aria-hidden="true" />绑定到具体的设备输出代次；设备安装位置独立维护。“已绑定”仅表示关系已建立，数据接收情况请在监测数据中查看。</p>
    {edit && <BindingDialog edit={edit} close={() => setEdit(undefined)} saved={text => { setMessage(text); setSelection(undefined); setEdit(undefined); }} />}
    {meaning && row && source && <Dialog open onOpenChange={setMeaning}><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle>设计要求 · 观测含义</DialogTitle><DialogDescription>{[...bindingLocation(row), row.name].join(' / ')}</DialogDescription></DialogHeader><DesignMeaning row={row} current={source} expanded /></DialogContent></Dialog>}
  </div>;
}
