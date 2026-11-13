import { useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { PanelLeft, Layers } from 'lucide-react';
import type { DeviceFilter, DeviceTreeNode } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, DirectorySearch, DirectoryReset } from '@infrapro/ui/patterns/directory-table';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from './context';
import { DeviceTree } from './tree';

export type DeviceListSearch = { manufacturer?: string; product?: string; device?: string; q?: string; status?: 'enabled' | 'disabled'; installation?: 'recorded' | 'unrecorded'; source?: 'all' | 'public' | 'private'; tab?: 'devices' | 'outputs' };
type ListProps = { search: DeviceListSearch; onSearch: (value: DeviceListSearch) => void };

export function DevicesPage({ search, onSearch }: ListProps) {
  const { paths, canRead } = useDevices();
  const navigate = useNavigate();
  const [treeOpen, setTreeOpen] = useState(false);
  if (!canRead) return <ErrorState title="没有设备查看权限" message="请联系当前范围的管理员配置相应角色。" />;
  const selected = !!(search.manufacturer || search.product || search.device);
  const clearSelection = () => onSearch({ ...search, manufacturer: undefined, product: undefined, device: undefined });
  const select = (node: DeviceTreeNode) => {
    if (node.kind === 'manufacturer') onSearch({ ...search, manufacturer: node.id, product: undefined, device: undefined });
    if (node.kind === 'product') onSearch({ ...search, product: node.id, device: undefined });
    if (node.kind === 'device') {
      if (node.visibility === 'full') void navigate({ to: paths.device(node.id) });
      else onSearch({ ...search, device: node.id, tab: 'outputs' });
    }
    if (node.kind === 'output') void navigate({ to: paths.output(node.id) });
    setTreeOpen(false);
  };
  const tree = <DeviceTree selection={search} onSelect={select} filter={{ q: search.q, source: search.source, status: search.status }} />;
  return <section className="min-w-0 space-y-6">
    <PageHeader title="项目设备" description="查看本项目获准的设备档案与具体输出。"
      actions={<Button id="device-directory" variant="outline" className="lg:hidden" onClick={() => setTreeOpen(true)}><PanelLeft aria-hidden="true" />目录</Button>} />
    <div className="grid min-w-0 gap-6 lg:grid-cols-[17rem_minmax(0,1fr)]">
      <aside aria-label="设备筛选目录" className="hidden min-w-0 space-y-4 border-r pr-5 lg:block">
        <h2 className="font-semibold">设备目录</h2>
        <Button variant={selected ? 'ghost' : 'secondary'} className="w-full justify-start" aria-pressed={!selected} onClick={clearSelection}><Layers aria-hidden="true" />全部设备</Button>
        <div className="max-h-[65vh] overflow-y-auto">{tree}</div>
      </aside>
      <div className="min-w-0 space-y-5">
        <Tabs value={search.tab ?? 'devices'} onValueChange={(tab) => onSearch({ ...search, tab: tab as DeviceListSearch['tab'] })} className="min-w-0 gap-5">
          <TabsList aria-label="设备读取范围"><TabsTrigger value="devices">设备档案</TabsTrigger><TabsTrigger value="outputs">获准输出</TabsTrigger></TabsList>
          {selected && <SelectionNotice search={search} clear={clearSelection} />}
          <TabsContent value="devices"><DeviceTable search={search} onSearch={onSearch} /></TabsContent>
          <TabsContent value="outputs"><ProjectOutputs search={search} onSearch={onSearch} /></TabsContent>
        </Tabs>

      </div>
    </div>
    <ResponsivePanel open={treeOpen} onOpenChange={setTreeOpen} title="设备目录" description="选择厂家、产品、设备或获准输出。" fallbackFocusSelector="#device-directory">
      {treeOpen && <div className="space-y-4"><Button variant={selected ? 'ghost' : 'secondary'} className="w-full justify-start" onClick={() => { clearSelection(); setTreeOpen(false); }}><Layers aria-hidden="true" />全部设备</Button>{tree}</div>}
    </ResponsivePanel>
  </section>;
}

function SelectionNotice({ search, clear }: { search: DeviceListSearch; clear: () => void }) {
  return <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted/40 px-3 py-2"><p className="text-sm">已按{search.device ? '设备' : search.product ? '产品' : '厂家'}筛选</p><Button size="sm" variant="ghost" onClick={clear}>清除目录选择</Button></div>;
}

function DeviceFilters({ search, onSearch, outputs = false }: ListProps & { outputs?: boolean }) {
  return <>
    <DirectorySearch label={outputs ? '搜索获准输出或设备编号' : '搜索设备名称、编号或序列号'} value={search.q ?? ''} onChange={(q) => onSearch({ ...search, q })} />
    <ListFilter label="产品来源" value={search.source ?? 'all'} onChange={(source) => onSearch({ ...search, source: source as DeviceListSearch['source'], manufacturer: undefined, product: undefined, device: undefined })} options={[{ value: 'all', label: '全部来源' }, { value: 'public', label: '平台公共' }, { value: 'private', label: '本租户私有' }]} />
    <ListFilter label="设备状态" value={search.status ?? ''} onChange={(status) => onSearch({ ...search, status: (status || undefined) as DeviceListSearch['status'] })} options={[{ value: '', label: '全部状态' }, { value: 'enabled', label: '启用' }, { value: 'disabled', label: '停用' }]} />
    {!outputs && <ListFilter label="安装记录" value={search.installation ?? ''} onChange={(value) => onSearch({ ...search, installation: (value || undefined) as DeviceListSearch['installation'] })} options={[{ value: '', label: '全部设备' }, { value: 'recorded', label: '有当前安装记录' }, { value: 'unrecorded', label: '无当前安装记录' }]} />}
    {(search.q || search.status || search.installation || search.source && search.source !== 'all') && <DirectoryReset onClick={() => onSearch({ tab: search.tab })} />}
  </>;
}

function DeviceTable({ search, onSearch }: ListProps) {
  const { client, scope, paths, prefix } = useDevices();
  const [size, setSize] = useState(25);
  const filter: DeviceFilter = { source: search.source, q: search.q, status: search.status, installation: search.installation, manufacturer_id: search.manufacturer, product_id: search.product };
  const query = useCursorList({ queryKey: [...prefix, 'list', filter, size], load: (cursor, signal) => client.devices(scope, { ...filter, limit: size, cursor }, signal) });
  return <DirectoryTable label="设备列表" items={query.items} rowId={(value) => value.id} loading={query.isPending} error={query.error?.message} retry={() => void query.refetch()}
    empty="暂无整台设备档案授权；可切换到获准输出。" toolbar={<DeviceFilters search={search} onSearch={onSearch} />}
    columns={[
      { id: 'name', label: '设备', cell: (value) => <div className="space-y-1"><Link to={paths.device(value.id)} className="break-words font-medium underline-offset-4 hover:underline">{value.name}</Link><p className="break-all font-mono text-xs text-muted-foreground">{value.code}</p><p className="text-xs text-muted-foreground lg:hidden">{value.product.name} · v{value.product.number}</p></div> },
      { id: 'product', label: '采用产品', secondary: true, cell: (value) => <div className="space-y-1"><p>{value.product.name} <span className="text-muted-foreground">· v{value.product.number}</span></p><p className="text-xs text-muted-foreground">{value.product.manufacturer_name} · {value.product.model}</p><Badge variant="outline">{value.product.source === 'public' ? '平台公共' : '本租户私有'}</Badge></div> },
      { id: 'status', label: '状态', className: 'w-20 lg:w-24', cell: (value) => <Badge variant={value.status === 'enabled' ? 'secondary' : 'outline'}>{value.status === 'enabled' ? '启用' : '停用'}</Badge> },
    ]} pagination={<ListPagination pageSizes={[25, 50, 100]} label="设备" count={query.items.length} page={query.page} pageCount={query.pageCount} hasNext={!!query.hasNext} fetching={query.isFetching} go={query.go} next={() => void query.next()} pageSize={size} setPageSize={setSize} />} />;
}

function ProjectOutputs({ search, onSearch }: ListProps) {
  const { client, scope, paths, prefix } = useDevices();
  const [size, setSize] = useState(25);
  const filter = { source: search.source, q: search.q, device_id: search.device, manufacturer_id: search.manufacturer, product_id: search.product, status: search.status };
  const query = useCursorList({ queryKey: [...prefix, 'outputs', filter, size], load: (cursor, signal) => { if (scope.kind !== 'project') throw new Error('项目范围无效。'); return client.projectDeviceOutputs(scope, { ...filter, cursor, limit: size }, signal); } });
  return <div className="space-y-4"><p className="text-sm text-muted-foreground">仅列出明确授予本项目的输出；设备档案授权不会自动包含输出。</p><DirectoryTable label="获准设备输出" items={query.items} rowId={(value) => value.id} loading={query.isPending} error={query.error?.message} retry={() => void query.refetch()} empty="暂无明确授予本项目的输出。设备档案授权不会自动授予输出。"
    toolbar={<DeviceFilters search={search} onSearch={onSearch} outputs />}
    columns={[
      { id: 'name', label: '输出', cell: (value) => <div className="space-y-1"><Link to={paths.output(value.id)} className="break-words font-medium underline-offset-4 hover:underline">{value.name}</Link><p className="text-xs text-muted-foreground">第 {value.generation} 代</p><p className="text-xs text-muted-foreground lg:hidden">{value.device.name} · {value.device.code}</p></div> },
      { id: 'device', label: '所属设备', secondary: true, cell: (value) => <div className="space-y-1"><p>{value.device.name}</p><p className="break-all font-mono text-xs text-muted-foreground">{value.device.code}</p><p className="text-xs text-muted-foreground">{value.device.product_name} · {value.device.product_model}</p></div> },
      { id: 'state', label: '状态', className: 'w-24 lg:w-32', cell: (value) => <div className="space-y-1"><Badge variant={value.ended_at || value.device_status === 'disabled' ? 'outline' : 'secondary'}>{value.ended_at ? '已结束' : value.device_status === 'disabled' ? '设备停用' : '当前代次'}</Badge>{value.ended_at && <p className="text-xs text-muted-foreground">仅供历史查看</p>}</div> },
    ]} pagination={<ListPagination pageSizes={[25, 50, 100]} label="输出" count={query.items.length} page={query.page} pageCount={query.pageCount} hasNext={!!query.hasNext} fetching={query.isFetching} go={query.go} next={() => void query.next()} pageSize={size} setPageSize={setSize} />} /></div>;
}
