import { useState } from 'react';
import { ClassificationFilter } from './classification-filter';
import { useQuery } from '@tanstack/react-query';
import { Check, PlusCircle } from 'lucide-react';
import { Permissions } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryReset, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from './context';
import type { TenantDeviceSearch } from './search';

type Picker = 'manufacturer' | 'product' | 'facility';
const labels: Record<Picker, string> = { manufacturer: '厂家', product: '产品', facility: '所属设施' };
type Props = { search: TenantDeviceSearch; onChange: (value: TenantDeviceSearch) => void };

export function WorkspaceFilters({ search, onChange }: Props) {
  const { client, scope, prefix, can } = useDevices();
  const [picker, setPicker] = useState<Picker>();
  const canReadResources = can(Permissions.TenantEngineeringResourceRead);
  const manufacturer = useQuery({ queryKey: [...prefix, 'filter-manufacturer', search.manufacturer], enabled: !!search.manufacturer, queryFn: ({ signal }) => client.deviceTree(scope, { level: 'manufacturer', manufacturer_id: search.manufacturer }, signal) });
  const product = useQuery({ queryKey: [...prefix, 'filter-product', search.manufacturer, search.product], enabled: !!search.manufacturer && !!search.product, queryFn: ({ signal }) => client.deviceTree(scope, { level: 'product', parent_id: search.manufacturer, product_id: search.product }, signal) });
  const facility = useQuery({ queryKey: [...prefix, 'filter-facility', search.facility], enabled: canReadResources && !!search.facility, queryFn: ({ signal }) => client.resourceDetail(scope, { kind: 'facility', id: search.facility }, signal) });
  const names = { manufacturer: manufacturer.data?.items[0]?.name, product: product.data?.items[0]?.name, facility: facility.data?.name };
  const filtered = !!(search.category || search.category_group || search.q || search.manufacturer || search.product || search.facility || search.status || search.installation || search.source && search.source !== 'all');
  const choose = (kind: Picker, value?: string) => {
    onChange({ ...search, [kind]: value,
      ...(kind === 'manufacturer' ? { product: undefined } : {}),
      ...(kind === 'facility' && value ? { installation: 'resource' as const } : {}),
    });
    setPicker(undefined);
  };
  return <div className="space-y-3" aria-label="设备筛选">
    <div className="flex flex-wrap items-center gap-2">
      <ClassificationFilter search={search} onChange={onChange}/>
      <DirectorySearch label="搜索设备名称、编号或序列号" value={search.q ?? ''} onChange={(q) => onChange({ ...search, q: q || undefined })} />
      <ListFilter label="设备状态" value={search.status ?? ''} onChange={(status) => onChange({ ...search, status: (status || undefined) as TenantDeviceSearch['status'] })} options={[{ value: '', label: '全部状态' }, { value: 'enabled', label: '启用' }, { value: 'disabled', label: '停用' }]} />
      <ListFilter label="安装位置" value={search.installation ?? ''} onChange={(value) => onChange({ ...search, installation: (value || undefined) as TenantDeviceSearch['installation'], facility: undefined })} options={[{ value: '', label: '全部位置' }, { value: 'recorded', label: '有当前安装记录' }, { value: 'resource', label: '设施 / 构件 / 断面 / 孔位 / 测点' }, { value: 'location', label: '独立位置' }, { value: 'unrecorded', label: '无当前安装记录' }]} />
      {(['facility', 'manufacturer', 'product'] as const).filter((kind) => kind !== 'facility' || canReadResources || search.facility).map((kind) => <Button key={kind} id={`device-filter-${kind}`} variant="outline" className="h-9 max-w-full border-dashed max-sm:h-11" disabled={kind === 'product' && !search.manufacturer || kind === 'facility' && !canReadResources} aria-label={labels[kind]} title={kind === 'product' && !search.manufacturer ? '先选择厂家，再筛选产品' : names[kind]} onClick={() => setPicker(kind)}><PlusCircle aria-hidden="true" />{labels[kind]}{search[kind] && <><span className="mx-1 h-4 border-l" /><Badge variant="secondary" className="max-w-36 truncate">{names[kind] || '已选择'}</Badge></>}</Button>)}
      <ListFilter label="产品来源" value={search.source ?? ''} onChange={(value) => onChange({ ...search, source: (value || undefined) as TenantDeviceSearch['source'], manufacturer: undefined, product: undefined })} options={[{ value: '', label: '全部来源' }, { value: 'public', label: '平台公共' }, { value: 'private', label: '本租户私有' }]} />
      {filtered && <DirectoryReset onClick={() => onChange({ tab: search.tab })} />}
    </div>
    {picker && <ResponsivePanel open onOpenChange={(open) => { if (!open) setPicker(undefined); }} title={`筛选${labels[picker]}`} description={picker === 'facility' ? '包含当前安装在该设施及其构件、断面、孔位、测点的设备。' : picker === 'manufacturer' ? '选择厂家后，可以继续按产品缩小范围。' : '选择当前厂家下的产品，涵盖该产品的所有版本。'} fallbackFocusSelector={`#device-filter-${picker}`}>
      <FilterChoices kind={picker} search={search} choose={(id) => choose(picker, id)} />
    </ResponsivePanel>}
  </div>;
}

function FilterChoices({ kind, search, choose }: { kind: Picker; search: TenantDeviceSearch; choose: (id?: string) => void }) {
  const { client, scope, prefix } = useDevices();
  const [q, setQ] = useState('');
  const [pageSize, setPageSize] = useState(25);
  const list = useCursorList({ queryKey: [...prefix, 'filter-choices', kind, search.manufacturer, search.source, q, pageSize], load: async (cursor, signal) => {
    if (kind === 'facility') {
      const page = await client.resourceList(scope, { kind: 'facility' }, { q, status: 'all', cursor, limit: pageSize }, signal);
      return { ...page, items: page.items.map((item) => ({ id: item.id, name: item.name, code: item.code, secondary: item.status === 'retired' ? '已退役' : '设施' })) };
    }
    const page = await client.deviceTree(scope, { level: kind, parent_id: kind === 'product' ? search.manufacturer : undefined, source: search.source, q, cursor, limit: pageSize }, signal);
    return { ...page, items: page.items.map((item) => ({ id: item.id, name: item.name, code: item.code, secondary: item.source === 'public' ? '平台公共' : '本租户私有' })) };
  } });
  return <div className="space-y-4">
    <DirectorySearch label={`搜索${labels[kind]}名称或编号`} value={q} onChange={setQ} />
    <Button variant="outline" className="w-full justify-between" onClick={() => choose()}>{kind === 'facility' ? '不限设施' : `全部${labels[kind]}`}{!search[kind] && <Check aria-hidden="true" />}</Button>
    {list.isPending ? <LoadingState /> : list.error ? <ErrorState message={list.error.message} onRetry={() => void list.refetch()} /> : !list.items.length ? <EmptyState title="没有匹配的选项" description="请尝试其他名称或编号。" /> : <ul className="space-y-2">{list.items.map((item) => <li key={item.id}><Button variant={search[kind] === item.id ? 'secondary' : 'ghost'} className="h-auto min-h-14 w-full justify-between text-left whitespace-normal" onClick={() => choose(item.id)}><span className="min-w-0 space-y-1"><span className="block break-words">{item.name}</span><span className="block text-xs font-normal text-muted-foreground">{item.code} · {item.secondary}</span></span>{search[kind] === item.id && <Check className="shrink-0" aria-hidden="true" />}</Button></li>)}</ul>}
    <ListPagination label={`${labels[kind]}筛选`} pageSizes={[25, 50, 100]} pageSize={pageSize} setPageSize={setPageSize} count={list.items.length} page={list.page} pageCount={list.pageCount} hasNext={!!list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} />
  </div>;
}
