import { ClassificationBulk } from '../classification/bulk';
import { ObjectHistory } from '../audit/object-history';
import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, Factory, Layers, Package, Plus, Settings2 } from 'lucide-react';
import type { DeviceManufacturer } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@infrapro/ui/components/card';
import { Separator } from '@infrapro/ui/components/separator';
import { DirectoryReset, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListPagination, ListFilter } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDeviceCatalog } from './context';
import { ManufacturerEditor } from './manufacturer-editor';
import { ManufacturerManager, type ManufacturerMode } from './manufacturer-manager';
import { ProductEditor } from './editor';

export type CatalogSearch = { source?: 'public' | 'private'; q?: string; manufacturer?: string; status?: 'enabled' | 'disabled' | 'draft' };
type Panel = { kind: 'manufacturers' } | { kind: 'manufacturer'; initial?: DeviceManufacturer; mode?: ManufacturerMode } | { kind: 'product' };

export function DeviceCatalogPage({ search, onSearch }: { search: CatalogSearch; onSearch: (value: CatalogSearch) => void }) {
  const defaultCatalog = useDeviceCatalog();
  const source = search.source ?? (defaultCatalog.tenant && !defaultCatalog.read ? 'public' : 'private');
  const c = useDeviceCatalog(source);
  return c.read ? <CatalogBody key={JSON.stringify(c.prefix)} search={{ ...search, source }} onSearch={onSearch} /> : <ErrorState message="当前范围没有该设备产品目录的读取权限。" />;
}

function CatalogBody({ search, onSearch }: { search: CatalogSearch; onSearch: (value: CatalogSearch) => void }) {
  const c = useDeviceCatalog(search.source ?? 'private');
  const navigate = useNavigate();
  const cache = useQueryClient();
  const [manufacturerSearch, setManufacturerSearch] = useState('');
  const [size, setSize] = useState(25);
  const [panel, setPanel] = useState<Panel>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const manufacturers = useCursorList({ initialPageSize: 25,
    queryKey: [...c.prefix, 'manufacturers', manufacturerSearch],
    load: (cursor, signal, limit) => c.client.deviceManufacturers(c.scope, { q: manufacturerSearch, cursor, limit }, signal),
  });
  const products = useCursorList({ pageSize: size,
    queryKey: [...c.prefix, 'products', search, size],
    load: (cursor, signal) => c.client.deviceProducts(c.scope, { q: search.q, manufacturer_id: search.manufacturer, status: search.status, cursor, limit: size }, signal),
  });
  const manufacturer = manufacturers.items.find((item) => item.id === search.manufacturer);
  const manufacturerName = manufacturer?.name ?? products.items.find((item) => item.manufacturer_id === search.manufacturer)?.manufacturer_name;
  const editManufacturer = (initial?: DeviceManufacturer, mode: ManufacturerMode = 'edit') => setPanel({ kind: 'manufacturer', initial, mode });
  const selectManufacturer = (id?: string) => onSearch({ ...search, manufacturer: id });
  const refresh = () => void cache.invalidateQueries({ queryKey: c.prefix });
  const panelTitle = panel?.kind === 'product' ? '创建产品草稿' : panel?.kind === 'manufacturer'
    ? !panel.initial ? '新建厂家' : panel.mode === 'state' ? panel.initial.status === 'active' ? '停用厂家' : '启用厂家' : panel.mode === 'delete' ? '删除厂家' : '编辑厂家'
    : '管理厂家';

  return <section className="space-y-6">
    <PageHeader title={c.tenant ? '设备产品目录' : '公共设备产品'} description="按厂家浏览产品，维护产品草稿与设备可采用的发布版本。"
      actions={c.edit && <>{c.owned&&<ClassificationBulk scope={c.writeScope} kind="product_version" disabled={!c.available}/>}<Button id="create-product" disabled={!c.available} onClick={(event) => { returnFocus.current = event.currentTarget; setPanel({ kind: 'product' }); }}><Plus aria-hidden="true" />{c.owned ? '新建产品' : '新增本租户产品'}</Button></>} />
    {c.tenant && <div className="flex flex-wrap gap-2" aria-label="产品目录来源"><Button variant={search.source !== 'public' ? 'secondary' : 'ghost'} aria-pressed={search.source !== 'public'} onClick={() => onSearch({ source: 'private' })}>本租户私有</Button><Button variant={search.source === 'public' ? 'secondary' : 'ghost'} aria-pressed={search.source === 'public'} onClick={() => onSearch({ source: 'public' })}>平台公共目录</Button></div>}
    <div className="grid min-w-0 gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <aside className="min-w-0 space-y-4 border-b pb-5 lg:border-r lg:border-b-0 lg:pr-5 lg:pb-0" aria-label="厂家目录">
        <div className="flex items-center justify-between gap-2"><h2 className="font-semibold">厂家目录</h2>
          {c.owned && c.manufacturerManage && <Button id="manage-manufacturers" variant="ghost" size="sm" disabled={!c.available} onClick={(event) => { returnFocus.current = event.currentTarget; setPanel({ kind: 'manufacturers' }); }}><Settings2 aria-hidden="true" />管理厂家</Button>}
        </div>
        <div className="[&>div]:w-full"><DirectorySearch label="搜索厂家" value={manufacturerSearch} onChange={setManufacturerSearch} /></div>
        <Button variant={!search.manufacturer ? 'secondary' : 'ghost'} className="w-full justify-start" aria-pressed={!search.manufacturer} onClick={() => selectManufacturer()}><Layers aria-hidden="true" />全部厂家</Button>
        {manufacturers.isPending ? <LoadingState label="正在读取厂家…" /> : manufacturers.error ? <ErrorState message={manufacturers.error.message} onRetry={() => void manufacturers.refetch()} /> : <div className="max-h-56 space-y-1 overflow-y-auto lg:max-h-[50vh]">
          {manufacturers.items.length ? manufacturers.items.map((item) => <Button key={item.id} variant={search.manufacturer === item.id ? 'secondary' : 'ghost'} aria-pressed={search.manufacturer === item.id}
            className="h-auto min-h-11 w-full items-start justify-start gap-2 px-3 py-3 whitespace-normal text-left" onClick={() => selectManufacturer(item.id)}>
            <Factory className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><span className="min-w-0 space-y-1"><span className="block break-words">{item.name}</span><span className="block break-all text-xs font-normal text-muted-foreground">{item.status === 'archived' ? '已停用 · 既有产品保留' : item.code}</span></span>
          </Button>) : <p className="px-3 py-5 text-sm text-muted-foreground">{manufacturerSearch ? '没有匹配的厂家。' : '暂无厂家。'}</p>}
        </div>}
        <ListPagination label="厂家" {...manufacturers.pagination} />
      </aside>
      <section className="min-w-0 space-y-4" aria-label="设备产品">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0"><h2 className="break-words text-lg font-semibold">{search.manufacturer ? manufacturerName ?? '已选厂家产品' : '全部产品'}</h2>{search.manufacturer&&<ObjectHistory objectType="manufacturer" objectId={search.manufacturer}/> }<p className="mt-1 text-sm text-muted-foreground">{manufacturer?.status === 'archived' ? '厂家已停用，既有产品的版本状态独立管理。' : search.manufacturer ? '查看该厂家的产品与发布版本。' : '跨厂家查找产品，查看最新发布版本与草稿状态。'}</p></div>
          {search.manufacturer && <Button variant="ghost" size="sm" onClick={() => selectManufacturer()}>清除厂家筛选</Button>}
        </div>
        <div className="flex flex-wrap items-center gap-2"><DirectorySearch label="搜索产品名称、编号或型号" value={search.q ?? ''} onChange={(q) => onSearch({ ...search, q })} />
          {c.owned && <ListFilter label="产品状态" value={search.status ?? ''} onChange={(status) => onSearch({ ...search, status: (status || undefined) as CatalogSearch['status'] })} options={[{ value: '', label: '全部产品' }, { value: 'draft', label: '有草稿' }, { value: 'enabled', label: '有启用版本' }, { value: 'disabled', label: '全部版本停用' }]} />}
          {(search.q || search.status) && <DirectoryReset onClick={() => onSearch({ ...search, q: undefined, status: undefined })} />}
        </div>
        {products.error && <ErrorState title="暂时无法读取产品" message={products.error.message} onRetry={() => void products.refetch()} />}
        {products.isPending ? <LoadingState label="正在读取设备产品…" /> : products.items.length ? <div className="grid min-w-0 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
          {products.items.map((item) => <Link key={item.id} to={c.base + '/' + item.id} search={{ source: search.source }} aria-label={item.name || item.code} className="group min-w-0 rounded-xl focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring">
            <Card className="h-full gap-4 py-5 shadow-none transition-colors group-hover:border-primary/40 motion-reduce:transition-none">
              <CardHeader className="gap-4 px-5"><div className="flex min-w-0 items-center justify-between gap-3"><span className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground"><Package className="size-5" aria-hidden="true" /></span><span className="break-all text-right text-xs text-muted-foreground">{item.model || '未填写型号'}</span></div><div className="min-w-0 space-y-1"><CardTitle className="break-words text-base leading-relaxed">{item.name || item.code}</CardTitle><p className="break-all font-mono text-xs text-muted-foreground">{item.code}</p></div></CardHeader>
              <CardContent className="min-w-0 flex-1 px-5"><p className="break-words text-sm text-muted-foreground">{item.manufacturer_name}</p></CardContent>
              <div className="px-5"><Separator /></div>
              <CardFooter className="justify-between gap-3 px-5"><div className="flex flex-wrap items-center gap-2">{item.latest_version_number !== null ? <><span className="text-sm tabular-nums">v{item.latest_version_number}</span><Badge variant={item.latest_version_enabled ? 'secondary' : 'outline'}>{item.latest_version_enabled ? '启用' : '停用'}</Badge></> : <span className="text-sm text-muted-foreground">尚未发布</span>}{item.has_draft && <Badge variant="outline">有草稿</Badge>}</div><ArrowRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /></CardFooter>
            </Card>
          </Link>)}
        </div> : !products.error && <EmptyState title="没有符合条件的产品" description={c.owned ? '可以调整搜索与筛选条件，或新建产品。' : '公共目录仅显示可采用的已发布产品版本。'} />}
        <ListPagination label="产品" {...products.pagination} pageSizes={[25, 50, 100]} setPageSize={setSize} />
      </section>
    </div>
    <ResponsivePanel open={!!panel} onOpenChange={(open) => { if (!open) setPanel(undefined); }} title={panelTitle}
      description={panel?.kind === 'product' ? '先保存草稿，再校验并发布。' : panel?.kind === 'manufacturer' ? '厂家编号创建后保持固定。' : '集中维护厂家资料和状态，不影响当前产品筛选。'}
      returnFocus={returnFocus} fallbackFocusSelector={panel?.kind === 'product' ? '#create-product' : '#manage-manufacturers'}>
      {panel?.kind === 'manufacturers' ? <ManufacturerManager onEdit={editManufacturer} onSelect={(item) => { selectManufacturer(item.id); setPanel(undefined); }} />
        : panel?.kind === 'manufacturer' ? <div className="space-y-5"><Button autoFocus variant="ghost" size="sm" onClick={() => setPanel({ kind: 'manufacturers' })}><ArrowLeft aria-hidden="true" />返回厂家列表</Button><ManufacturerEditor key={`${panel.initial?.id ?? 'new'}:${panel.mode}`} initial={panel.initial} mode={panel.mode} done={() => { if (panel.mode === 'delete' && panel.initial?.id === search.manufacturer) selectManufacturer(); setPanel({ kind: 'manufacturers' }); refresh(); }} /></div>
          : panel?.kind === 'product' && <ProductEditor initialManufacturer={search.manufacturer ? { id: search.manufacturer, source: search.source ?? 'private' } : undefined} done={(result) => { setPanel(undefined); refresh(); void navigate({ to: `${c.base}/${result.id}`, search: { source: 'private' } }); }} />}
    </ResponsivePanel>
  </section>;
}
