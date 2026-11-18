import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ClassificationCard } from '../classification/assignment';
import { ProductParsing } from '../parsing/configs';
import { ObjectHistory } from '../audit/object-history';
import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Copy, FilePenLine, GitBranch, Info, Package, Plus } from 'lucide-react';
import type { DeviceProductVersion } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryActions } from '@infrapro/ui/patterns/directory-table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { EmptyState, LoadingState, ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { DeviceSelect } from '../devices/fields';
import { DeviceIssues } from '../devices/command';
import { ProductEditor } from './editor';
import { useDeviceCatalog } from './context';
import { ProductAction, type ActionMode } from './product-action';
import { ProductDiff, ProductInformation, ProductOutputs } from './product-content';

type Panel = { mode: 'edit' | 'copy' | ActionMode; version?: DeviceProductVersion };
const catalogOperationNames: Record<string, string> = { create: '新建产品', copy: '复制产品', 'draft-save': '保存草稿', 'draft-create': '创建草稿', 'draft-delete': '丢弃草稿', publish: '发布版本', state: '变更版本状态', delete: '删除产品' };
const formatDate = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });

export function DeviceProductPage({ id, source = 'private' }: { id: string; source?: 'public' | 'private' }) {
  const c = useDeviceCatalog(source);
  if (!c.read) return <ErrorState message="没有此设备产品目录的读取权限。" />;
  return <ProductBody key={`${JSON.stringify(c.prefix)}:${id}`} id={id} source={source} />;
}

function ProductBody({ id, source }: { id: string; source: 'public' | 'private' }) {
  const c = useDeviceCatalog(source);
  const cache = useQueryClient();
  const navigate = useNavigate();
  const [selected, setSelected] = useState('');
  const [compare, setCompare] = useState('');
  const [tab, setTab] = useState('outputs');
  const [panel, setPanel] = useState<Panel>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const query = usePagedQuery({
    queryKey: [...c.prefix, 'product', id], initialPageSize: 25,
    queryFn: async ({ signal, cursor, limit }) => {
      if (c.owned) {
        const product = await c.client.deviceProduct(c.writeScope, id, { cursor, limit }, signal);
        return { product, items: product.versions, next_cursor: product.next_cursor };
      }
      const result = await c.client.devicePublicVersions(c.tenant!.tenant.id, id, { cursor, limit }, signal);
      return { ...result, product: undefined };
    },
  });
  const product = query.data?.product;
  const versions = query.data?.items ?? [];
  const viewingDraft = !!product?.draft && (selected === 'draft' || !versions.length);
  const version = versions.find((item) => item.id === selected) ?? versions[0];
  const content = viewingDraft ? product!.draft!.content : version?.content;
  const definitions = [...new Map(versions.flatMap((item) => item.definitions).map((item) => [item.id, item])).values()];
  const compareOptions = versions.filter((item) => viewingDraft || item.id !== version?.id);
  const previous = compareOptions.find((item) => item.id === compare) ?? compareOptions[0];
  const name = product?.name ?? version?.content.name ?? '产品';
  const openPanel = (mode: Panel['mode'], trigger: HTMLElement | null) => { returnFocus.current = trigger; setPanel({ mode, version }); };
  const done = () => { setPanel(undefined); void cache.invalidateQueries({ queryKey: c.prefix }); };
  const panelTitle = ({ edit: '编辑产品草稿', copy: '复制产品', publish: '校验并发布', draft: '创建产品草稿', draft_delete: '丢弃草稿', state: version?.enabled ? '停用产品版本' : '启用产品版本', delete: '删除产品' })[panel?.mode ?? 'edit'];

  return <section className="min-w-0 space-y-6"><ObjectHistory objectType="product" objectId={id} scope={c.tenant?{kind:"tenant",tenant_id:c.tenant.tenant.id}:{kind:"platform"}}/>
    <Button asChild variant="ghost" size="sm" className="-ml-3 w-fit"><Link to={c.base} search={{ source }}><ArrowLeft aria-hidden="true" />返回设备产品目录</Link></Button>
    {query.isPending ? <LoadingState /> : query.error ? <ErrorState message={query.error.message} onRetry={() => void query.refetch()} /> : <>
      <PageHeader title={name} description={product ? `${product.code} · ${product.manufacturer_name}` : `${version?.manufacturer_name ?? '平台公共目录'} · 固定发布版本`}
        actions={<div className="flex flex-wrap gap-2">
          {!viewingDraft && version && c.edit && <Button id="copy-product" variant="outline" disabled={!c.available} onClick={(event) => openPanel('copy', event.currentTarget)}><Copy aria-hidden="true" />复制为{c.tenant ? '本租户' : ''}新产品</Button>}
          {c.owned && c.edit && product && !versions.length && <DirectoryActions id="product-actions" name={name} actions={[{ label: '删除产品', disabled: !c.available, opensPanel: true, run: (trigger) => openPanel('delete', trigger) }]} />}
        </div>} />
      <div className="grid min-w-0 overflow-hidden rounded-xl border lg:grid-cols-[12rem_minmax(0,1fr)]">
        <aside id="product-versions" tabIndex={-1} aria-label="产品版本" className="min-w-0 space-y-4 border-b bg-muted/20 p-4 lg:border-r lg:border-b-0">
          <h2 className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><GitBranch className="size-4" aria-hidden="true" />产品版本</h2>
          <div className="flex gap-2 overflow-x-auto pb-1 lg:flex-col">
            {product?.draft && <Button variant={viewingDraft ? 'secondary' : 'ghost'} className="h-auto min-w-36 justify-start whitespace-normal px-3 py-3 text-left lg:min-w-0" aria-pressed={viewingDraft} onClick={() => setSelected('draft')} aria-label="查看当前草稿"><span className="w-full space-y-2"><span className="flex items-center justify-between gap-2"><span>当前草稿</span><Badge variant="outline">未发布</Badge></span><span className="block text-xs font-normal text-muted-foreground">尚未发布的修改</span></span></Button>}
            {versions.map((item, index) => <Button key={item.id} variant={!viewingDraft && version?.id === item.id ? 'secondary' : 'ghost'} aria-pressed={!viewingDraft && version?.id === item.id} aria-label={`查看发布版本 v${item.number}`} onClick={() => setSelected(item.id)} className="h-auto min-w-36 justify-start whitespace-normal px-3 py-3 text-left lg:min-w-0"><span className="w-full space-y-2"><span className="flex items-center justify-between gap-2"><span>v{item.number}{query.pagination.page === 0 && index === 0 ? ' · 最新' : ''}</span><Badge variant="outline">{item.enabled ? '启用' : '停用'}</Badge></span><span className="block text-xs font-normal text-muted-foreground">{formatDate(item.published_at)}</span></span></Button>)}
          </div>
          {query.data && <ListPagination label="版本" {...query.pagination} />}
          <p className="hidden text-xs leading-relaxed text-muted-foreground lg:block">发布时间：北京时间<br />发布内容固定保留。</p>
        </aside>
        <div className="min-w-0 space-y-6 p-4 sm:p-6">
          {content ? <>
{!viewingDraft&&version&&<ClassificationCard scopeKey={c.prefix} key={version.id} scope={c.writeScope} kind="product_version" id={version.id} value={version.classification} canManage={c.owned&&c.edit&&c.available} canHistory/>}
            <div className="flex flex-wrap items-start justify-between gap-4 border-b pb-5">
              <div className="space-y-2"><div className="flex flex-wrap items-center gap-2">{viewingDraft ? <FilePenLine className="size-4 text-muted-foreground" aria-hidden="true" /> : <Package className="size-4 text-muted-foreground" aria-hidden="true" />}<h2 className="font-semibold">{viewingDraft ? '当前草稿' : `发布版本 v${version!.number}`}</h2><Badge variant={viewingDraft || !version?.enabled ? 'outline' : 'secondary'}>{viewingDraft ? '未发布' : version!.enabled ? '启用' : '停用'}</Badge></div><p className="text-sm text-muted-foreground">{viewingDraft ? '草稿发布后才会成为新的可采用版本。' : `发布于 ${formatDate(version!.published_at)}（北京时间）`}</p></div>
              <div className="flex flex-wrap items-center gap-2">
                {viewingDraft && c.owned && <>
                  {c.edit && <Button id="edit-product" variant="outline" disabled={!c.available} onClick={(event) => openPanel('edit', event.currentTarget)}>编辑草稿</Button>}
                  {c.publish && <Button id="publish-product" disabled={!c.available} onClick={(event) => openPanel('publish', event.currentTarget)}>校验并发布</Button>}
                  {c.edit && <DirectoryActions id="draft-actions" name="当前草稿" actions={[{ label: '丢弃草稿', disabled: !c.available, opensPanel: true, run: (trigger) => openPanel('draft_delete', trigger) }]} />}
                </>}
                {!viewingDraft && c.owned && version && <>
                  {!product?.draft && c.edit && <Button id="new-product-draft" disabled={!c.available} onClick={(event) => openPanel('draft', event.currentTarget)}><Plus aria-hidden="true" />从所选版本创建草稿</Button>}
                  {product?.draft && <Button variant="outline" onClick={() => setSelected('draft')}>查看草稿</Button>}
                  {c.state && <DirectoryActions id="version-actions" name={`发布版本 v${version.number}`} actions={[{ label: version.enabled ? '停用此版本' : '启用此版本', disabled: !c.available, opensPanel: true, run: (trigger) => openPanel('state', trigger) }]} />}
                </>}
              </div>
            </div>
            {!viewingDraft && version && <DeviceIssues issues={version.issues} />}
            <Tabs value={tab} onValueChange={setTab} className="min-w-0 gap-5">
              <div className="min-w-0 overflow-x-auto"><TabsList aria-label="产品详情"><TabsTrigger value="outputs">输出槽 <Badge variant="secondary" className="px-1.5 py-0">{content.output_slots.length}</Badge></TabsTrigger><TabsTrigger value="info">产品信息</TabsTrigger>{!viewingDraft && <TabsTrigger value="parsing">解析配置</TabsTrigger>}<TabsTrigger value="compare">版本比较</TabsTrigger>{c.owned && <TabsTrigger value="history">变更记录</TabsTrigger>}</TabsList></div>
              <TabsContent value="outputs"><ProductOutputs key={viewingDraft ? `draft:${product!.draft!.revision}` : version!.id} content={content} definitions={definitions} /></TabsContent>
              {!viewingDraft && version && <TabsContent value="parsing"><ProductParsing key={version.id} product={id} version={version.id} slots={content.output_slots} definitions={definitions} source={source}/></TabsContent>}
              <TabsContent value="info"><ProductInformation content={content} /></TabsContent>
              <TabsContent value="compare" className="space-y-5">{previous ? <><DeviceSelect name="compare-version" label="对比基准版本" value={previous.id} onChange={setCompare} options={compareOptions.map((item) => ({ value: item.id, label: `v${item.number}` }))} /><ProductDiff before={previous.content} after={content} definitions={definitions} beforeLabel={`v${previous.number} · 基准`} afterLabel={viewingDraft ? '当前草稿' : `v${version!.number} · 当前`} /></> : <EmptyState title="暂无可比较的版本" description="发布另一个版本或创建新草稿后，可在这里对照变化。" />}</TabsContent>
              {c.owned && <TabsContent value="history"><CatalogHistory product={id} /></TabsContent>}
            </Tabs>
            <p className="flex items-start gap-2 text-sm leading-relaxed text-muted-foreground"><Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />发布新版本不会自动更新已有设备采用的版本。</p>
          </> : <EmptyState title="暂无可查看的产品内容" description="当前没有草稿或可见的发布版本。" />}
        </div>
      </div>
    </>}
    <ResponsivePanel open={!!panel} onOpenChange={(open) => { if (!open) setPanel(undefined); }} title={panelTitle} description="发布版本的内容固定保留。新草稿和新的发布版本不自动修改已有设备。" returnFocus={returnFocus} fallbackFocusSelector="#product-versions">
      {panel?.mode === 'edit' && product ? <ProductEditor key={product.draft?.revision} product={product} done={() => { done(); setSelected('draft'); }} />
        : panel?.mode === 'copy' && panel.version ? <ProductEditor copy={panel.version} done={(result) => { done(); void navigate({ to: `${c.base}/${result.id}`, search: { source: 'private' } }); }} />
          : panel && panel.mode !== 'edit' && panel.mode !== 'copy' && product && <ProductAction key={`${panel.mode}:${panel.version?.id ?? id}`} mode={panel.mode} product={product} version={panel.version} done={() => {
            done();
            if (panel.mode === 'delete') void navigate({ to: c.base, search: { source } });
            else if (panel.mode === 'draft') setSelected('draft');
            else if (panel.mode === 'publish' || panel.mode === 'draft_delete') setSelected('');
          }} />}
    </ResponsivePanel>
  </section>;
}

function CatalogHistory({ product }: { product: string }) {
  const c = useDeviceCatalog();
  const query = usePagedQuery({ initialPageSize: 25, queryKey: [...c.prefix, 'history', product], queryFn: ({ signal, cursor , limit}) => c.client.deviceCatalogChanges(c.writeScope, { product_id: product, cursor, limit }, signal) });
  const events = query.data?.items ?? [];
  return <section className="space-y-4" aria-label="产品变更记录"><h3 className="font-semibold">产品变更记录</h3>
    {query.isPending ? <LoadingState label="正在读取变更记录…" /> : query.error ? <ErrorState message={query.error.message} onRetry={() => void query.refetch()} /> : events.length ? <ol className="space-y-4">{events.map((event) => <li key={event.id} className="space-y-2 rounded-lg border p-4"><p className="break-words text-sm font-medium">{catalogOperationNames[event.operation.split(':').at(-1) ?? ''] ?? '产品变更'} · {event.actor_name}</p><p className="text-xs text-muted-foreground">{formatDate(event.changed_at)}（北京时间）</p><p className="whitespace-pre-wrap break-words text-sm">{event.reason || '未填写原因'}</p><details><summary className="cursor-pointer text-sm text-muted-foreground">查看变更前后</summary><pre className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify({ operation: event.operation, before: event.before, after: event.after }, null, 2)}</pre></details></li>)}</ol> : <EmptyState title="暂无产品变更记录" />}
    {query.data && <ListPagination label="记录" {...query.pagination} />}
    {query.isFetching && <FormNotice>正在读取更早记录…</FormNotice>}
  </section>;
}
