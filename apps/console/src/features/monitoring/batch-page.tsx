import { useContext, useEffect, useRef, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { MonitoringImportBatch as Batch } from '@infrapro/api-client';
import { FileSpreadsheet, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@infrapro/ui/components/drawer';
import { useIsMobile } from '@infrapro/ui/hooks/use-mobile';
import { useMonitoring } from './context';
import { MonitoringError } from './common';
import { count, stateNames } from './format';
import { JobProgress } from './jobs-page';
import { UploadForm } from './upload-form';
import { MappingForm } from './mapping-form';
import { batchCapabilities, importDate, importFileSize, importFileType, importFilter, type ImportSearch, type ImportTab } from './import-search';
import { ImportStatus } from './import-status';
import { ImportOverview, ImportMappingSummary, ImportSectionTitle } from './import-overview';
import { ImportReport } from './import-report';
import { ImportHistory } from './import-history';
import { ImportReturnFocus } from './import-return-focus';
import { ImportCommandForm, ImportReadoptForm, ImportRevokeForm, type ImportCommand } from './import-actions';

type Action = { kind: ImportCommand | 'revoke' | 'readopt'; batch: Batch };
export function MonitoringBatchPage({ id, search }: { id: string; search: ImportSearch }) {
  return <BatchDrawer key={id} id={id} search={search} />;
}
function BatchDrawer({ id, search }: { id: string; search: ImportSearch }) {
  const m = useMonitoring(), cache = useQueryClient(), navigate = useNavigate(), mobile = useIsMobile();
  const returnFocus = useContext(ImportReturnFocus);
  const [busy, setBusy] = useState(false);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const result = useQuery({ queryKey: [...m.prefix, 'batch', id], queryFn: ({ signal }) => m.client.monitoringImport(m.scope, id, signal), enabled: m.available && m.canImportRead, refetchInterval: q => ['uploading', 'validating', 'preparing'].includes(q.state.data?.state ?? '') ? 2500 : 15000, refetchOnWindowFocus: 'always', retry: false });
  // Reverification hides cached content without discarding an in-progress form.
  // A real authority change remounts MonitoringBoundary and clears it.
  const b = result.data, verified = m.available && !result.error;
  const close = () => {
    if (busy) { toast('正在提交操作，请稍候。'); return; }
    returnFocus(id);
    void navigate({ to: m.paths.imports, search: importFilter(search), state: state => ({ ...state }), resetScroll: false });
  };
  const tab = search.tab ?? (b?.state === 'received' && b.mapping_revision === '0' ? 'mapping' : 'overview');
  const setTab = (value: ImportTab) => { void navigate({ to: m.paths.batch(id), search: { ...search, tab: value }, replace: true, state: state => ({ ...state }), resetScroll: false }); };
  const done = (batch?: Batch) => {
    if (batch) cache.setQueryData([...m.prefix, 'batch', batch.id], batch);
    void cache.invalidateQueries({ queryKey: [...m.prefix, 'imports'] });
    void cache.invalidateQueries({ queryKey: [...m.prefix, 'import-history'] });
    if (!alive.current) return;
    setBusy(false);
    if (batch && batch.id !== id) { void navigate({ to: m.paths.batch(batch.id), search: { ...importFilter(search), tab: 'overview' }, state: state => ({ ...state }), resetScroll: false }); }
    else void result.refetch();
  };
  return <Drawer open onOpenChange={open => { if (!open) close(); }} swipeDirection={mobile ? 'down' : 'right'} showSwipeHandle={mobile}><DrawerContent finalFocus={false} className="bg-background shadow-xl [--drawer-bleed-background:transparent] data-[swipe-axis=x]:h-[calc(100dvh-1.5rem)] data-[swipe-axis=x]:w-[min(48rem,calc(100vw-1.5rem))] data-[swipe-axis=x]:[--drawer-inset:0.75rem] data-[swipe-direction=right]:rounded-2xl data-[swipe-direction=right]:border data-[swipe-axis=y]:h-[94dvh] data-[swipe-axis=y]:max-h-[94dvh]">
    <DrawerHeader className="relative border-b px-6 pb-5 pt-6 pr-16 group-data-[swipe-axis=y]/drawer-popup:text-left"><div className="mb-3 flex items-center gap-2 text-xs text-muted-foreground"><FileSpreadsheet aria-hidden className="size-4" /><span>导入批次</span><span>/</span><span className="font-mono">{id.slice(0, 8).toUpperCase()}</span></div><DrawerTitle className="break-words text-xl font-semibold leading-7">{verified && b ? b.name : '批次详情'}</DrawerTitle><DrawerDescription className="mt-1.5 text-xs">{verified && b ? `${b.scope_clipped ? '当前授权部分' : `${importFileType(b)} · ${importFileSize(b.size_bytes)}`} · 导入于 ${importDate(b.created_at, true)}` : '查看导入状态、校验问题、文件映射和处理记录。'}</DrawerDescription><Button variant="ghost" size="icon" className="absolute right-3 top-3 size-10" aria-label="关闭批次详情" disabled={busy} onClick={close}><X aria-hidden /></Button></DrawerHeader>
    {result.isPending && <p role="status" className="p-6 text-sm">正在读取批次详情…</p>}{result.error && <div className="p-6"><MonitoringError error={result.error} retry={() => void result.refetch()} /></div>}{!m.available && <p role="alert" className="p-6 text-sm">当前工作空间暂不可用，输入已保留；恢复后可继续核对。</p>}
    {b && <div className={verified ? "flex min-h-0 flex-1 flex-col" : "hidden"}>{search.job ? <BatchExecution batch={b} job={search.job} generation={search.execution} openBatch={() => void navigate({ to: m.paths.batch(id), search: { ...importFilter(search), tab: 'overview' } })} /> : <BatchContent batch={b} tab={tab} setTab={setTab} busy={busy} setBusy={setBusy} done={done} close={close} />}</div>}
  </DrawerContent></Drawer>;
}
function BatchExecution({batch,job,generation,openBatch}:{batch:Batch;job:string;generation?:string;openBatch:()=>void}) {
  const m = useMonitoring();
  const query = useQuery({ queryKey:[...m.prefix,'job',job], queryFn:({signal})=>m.client.monitoringJob(m.scope,job,signal), retry:false });
  const result = query.error ? undefined : query.data;
  const matches = result && result.target_id===batch.id && ['validate','publish'].includes(result.kind) && (!generation || result.task_ref?.generation===generation);
  return <section className="min-h-0 space-y-4 overflow-y-auto p-6"><h2 className="font-semibold">本次批次处理结果</h2><MonitoringError error={query.error} retry={()=>void query.refetch()} />
    {query.isPending && <p role="status">正在核实作业回执…</p>}
    {matches && <><p className="text-sm">{result.kind==='validate'?'导入校验':'导入采用'} · {stateNames[result.state] ?? result.state}</p><JobProgress job={result} />
      <p className="break-all font-mono text-xs text-muted-foreground">{result.id}</p></>}
    {result && !matches && <p role="status">当前链接无法核实本次执行结果。</p>}
    <Button variant="outline" onClick={openBatch}>查看当前批次</Button></section>;
}
function BatchContent({ batch: b, tab, setTab, busy, setBusy, done, close }: { batch: Batch; tab: ImportTab; setTab: (tab: ImportTab) => void; busy: boolean; setBusy: (value: boolean) => void; done: (batch?: Batch) => void; close: () => void }) {
  const m = useMonitoring();
  const caps = batchCapabilities(b, m.canManage, m.canRevoke, m.canSource, m.available);
  const [action, setAction] = useState<Action | null>(null);
  const [editing, setEditing] = useState<Batch | null>(() => b.state === 'received' && b.mapping_revision === '0' && caps.mapping ? b : null);
  const scroll = useRef<HTMLDivElement>(null);
  const chooseTab = (value: ImportTab) => { if (!busy) { setAction(null); setTab(value); scroll.current?.scrollTo(0, 0); } };
  const begin = (kind: Action['kind']) => { setEditing(null); setAction({ kind, batch: b }); scroll.current?.scrollTo(0, 0); };
  const completed = (batch?: Batch) => { setAction(null); setEditing(null); done(batch); if (!batch || batch.id === b.id) setTab(batch?.state === 'needs_resolution' ? 'issues' : 'overview'); };
  const stillCurrent = action?.batch.revision === b.revision;
  const largeAction = action?.kind === 'revoke' || action?.kind === 'readopt';
  const mappingCurrent = !!editing && editing.revision === b.revision && caps.mapping;
  const tabClass = 'h-11 flex-none rounded-none border-x-0 border-t-0 border-b-2 px-0 pb-2 data-[state=active]:border-foreground data-[state=active]:bg-transparent data-[state=active]:shadow-none';
  return <><div className="flex flex-wrap items-center justify-between gap-2 px-6 py-4"><ImportStatus batch={b} /><span className="text-[11px] text-muted-foreground">最近更新 {importDate(b.updated_at)}</span></div>
    <Tabs value={tab} onValueChange={value => chooseTab(value as ImportTab)} className="min-h-0 flex-1 gap-0 overflow-hidden"><div className="shrink-0 border-b px-6"><TabsList aria-label="批次详情分类" className="h-11 w-full justify-start gap-4 rounded-none bg-transparent p-0">{([['overview', '概览'], ['issues', '校验问题'], ['mapping', '映射配置'], ['history', '处理记录']] as const).map(([value, label]) => <TabsTrigger key={value} value={value} disabled={busy} className={tabClass}>{label}{value === 'issues' && b.counts && b.counts.errors !== '0' && <Badge variant="secondary" className="px-1.5 py-0 text-[10px] text-destructive">{count(b.counts.errors)}</Badge>}</TabsTrigger>)}</TabsList></div>
      <div ref={scroll} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-5" data-import-detail-scroll>
        {action?.kind === 'revoke' ? <ImportRevokeForm batch={action.batch} available={stillCurrent && caps.revoke} done={() => completed()} cancel={() => setAction(null)} onPendingChange={setBusy} /> : action?.kind === 'readopt' ? <ImportReadoptForm batch={action.batch} available={stillCurrent && caps.readopt} done={completed} cancel={() => setAction(null)} onPendingChange={setBusy} /> : <>
          <TabsContent value="overview"><ImportOverview batch={b} source={caps.source} showIssues={() => chooseTab('issues')} /></TabsContent>
          <TabsContent value="issues"><ImportReport key={`${b.id}:${b.revision}`} batch={b} resolve={caps.resolve} done={completed} onPendingChange={setBusy} /></TabsContent>
          <TabsContent value="mapping" className="space-y-5">{b.scope_clipped ? <p className="text-sm text-muted-foreground">完整映射不在当前授权范围内。</p> : caps.upload ? <section className="space-y-4"><h3 className="text-sm font-semibold">上传并封存原文件</h3><p className="text-xs text-muted-foreground">已确认 {b.parts.length} 个分片，共 {count(String(b.parts.reduce((n, p) => n + BigInt(p.size_bytes), 0n)))} 字节。</p><UploadForm initial={b} done={batch => { setEditing(batch); done(batch); }} /></section> : <><ImportSectionTitle action={caps.mapping && <Button variant="outline" size="sm" disabled={busy} onClick={() => setEditing(editing ? null : b)}>{editing ? '取消编辑' : '查看 / 调整映射'}</Button>}>列与通道映射</ImportSectionTitle>{editing ? <>{!mappingCurrent && <p role="alert" className="text-sm text-destructive">批次状态或修订已变化，请取消编辑后重新核对。</p>}<MappingForm batch={editing} disabled={!mappingCurrent} done={completed} onPendingChange={setBusy} /></> : <ImportMappingSummary batch={b} />}</>}</TabsContent>
          <TabsContent value="history"><ImportHistory batch={b} /></TabsContent>
        </>}
      </div>
    </Tabs>
    <footer className="max-h-[55dvh] shrink-0 space-y-3 overflow-y-auto border-t bg-background px-6 py-4 pb-[max(1rem,env(safe-area-inset-bottom))]">{action && !largeAction ? <ImportCommandForm key={`${action.kind}:${action.batch.revision}`} batch={action.batch} action={action.kind as ImportCommand} available={stillCurrent && (action.kind === 'retry' ? caps.validate : caps[action.kind as 'validate' | 'publish' | 'cancel'])} done={completed} cancel={() => setAction(null)} onPendingChange={setBusy} /> : <div className="flex flex-wrap items-center justify-between gap-2"><div className="flex flex-wrap gap-2">{!largeAction && caps.cancel && <Button variant="ghost" size="sm" disabled={busy} onClick={() => begin('cancel')}>取消此批次</Button>}{!largeAction && caps.revoke && <Button variant="ghost" size="sm" disabled={busy} onClick={() => begin('revoke')}>预览撤销影响</Button>}</div><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy} onClick={close}>关闭</Button>{!largeAction && !editing && (caps.publish ? <Button disabled={busy} onClick={() => begin('publish')}>{b.counts?.proposed === '0' ? '确认完成，无新增数据' : '发布合格批次'}</Button> : caps.validate ? <Button disabled={busy} onClick={() => begin(b.state === 'failed' ? 'retry' : 'validate')}>{b.state === 'failed' ? '重新校验' : '开始全量校验'}</Button> : caps.mapping ? <Button disabled={busy} onClick={() => { chooseTab('mapping'); setEditing(b); }}>配置映射</Button> : caps.upload ? <Button onClick={() => chooseTab('mapping')}>继续上传</Button> : caps.readopt ? <Button disabled={busy} onClick={() => begin('readopt')}>从原观测重新采用</Button> : null)}</div></div>}</footer>
  </>;
}
