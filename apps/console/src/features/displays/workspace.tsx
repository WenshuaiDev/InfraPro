import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Monitor, Plus, Search, Settings2, ArrowUpRight, Package, CircleAlert } from 'lucide-react';
import { Permissions as P, type DisplayInstance, type DisplayList } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Badge } from '@infrapro/ui/components/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@infrapro/ui/components/dialog';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useProject } from '../projects/context';
import { InstancePanel } from './detail';
import { requestUUID } from './crypto';
import { configChanges, displaySummary, matchesDisplay, type DisplayFilter, type DisplayView } from './presentation';

export function DisplaysWorkspace({ instanceID, packageID, onSelect }: { instanceID?: string; packageID?: string; onSelect: (id?: string) => void }) {
  const project = useProject(), { client } = useIdentity(), cache = useQueryClient();
  const scope = { tenantId: project.project.tenant_id, projectId: project.project.id }, key = [...project.key, 'displays'];
  const [dialog, setDialog] = useState<'create' | 'settings' | null>(null);
  const [name, setName] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const [search, setSearch] = useState(''), [filter, setFilter] = useState<DisplayFilter>('all'), [limit, setLimit] = useState(12);
  const [view, setView] = useState<DisplayView>('概览');
  const returnFocus = useRef<HTMLElement | null>(null);
  const state = useQuery({ queryKey: key, queryFn: ({ signal }) => client.displayList(scope, signal), enabled: project.available && project.can(P.ProjectDisplayView), refetchInterval: 15000, staleTime: 0, refetchOnWindowFocus: 'always', retry: false });
  const active = project.available && project.project.status === 'active';
  const data = !state.error ? state.data : undefined;
  const items = data?.items.filter(item => matchesDisplay(item, search, filter)) ?? [];
  const createAllowed = active && project.can(P.ProjectDisplayManage) && data?.settings.enabled;
  function open(id: string, initialView: DisplayView, trigger: HTMLElement) {
    returnFocus.current = trigger; setView(initialView); onSelect(id);
  }
  function openDialog(next: 'create' | 'settings') { setError(''); setName(''); setDialog(next); }
  async function command(path: string, body: object, method = 'POST') {
    if (!project.available) return;
    setBusy(true); setError('');
    try {
      const result = await client.displayCommand(scope, path, { request_id: requestUUID(), reason: '大屏管理', ...body }, method);
      await cache.invalidateQueries({ queryKey: key });
      setDialog(null);
      if (path === '/instances') { setView('概览'); onSelect(result.id); }
    } catch (e) { setError(e instanceof Error ? e.message : '操作失败'); }
    finally { setBusy(false); }
  }
  if (!project.can(P.ProjectDisplayView)) return <p>您没有项目大屏查看权限。</p>;
  return <section className="space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-4">
      <div><h1 className="text-2xl font-semibold">项目大屏</h1><p className="mt-1.5 text-sm text-muted-foreground">{data ? `${data.items.length} 个大屏 · ${data.items.filter(i => i.current_release_id && i.state === 'enabled').length} 个已发布` : '大屏与访问管理'}</p></div>
      <div className="flex gap-2">
        {data && project.can(P.ProjectDisplaySettings) && <Button variant="outline" size="icon" className="size-10" aria-label="项目大屏设置" onClick={() => openDialog('settings')}><Settings2 aria-hidden="true" /></Button>}
        {project.can(P.ProjectDisplayManage) && <Button className="h-10" disabled={!createAllowed} onClick={() => openDialog('create')}><Plus aria-hidden="true" />新建大屏</Button>}
      </div>
    </header>
    {state.error ? <ErrorState message={state.error.message} onRetry={() => void state.refetch()} /> : !data ? <LoadingState label="正在读取大屏…" /> : <>
      {!data.settings.enabled && <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/50 p-4 text-sm"><span className="flex items-center gap-2"><CircleAlert className="size-4" aria-hidden="true" />项目大屏已停用</span>{project.can(P.ProjectDisplaySettings) && <Button variant="outline" size="sm" disabled={!active} onClick={() => openDialog('settings')}>启用项目大屏</Button>}</div>}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-48 flex-1 sm:max-w-xs"><Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" aria-hidden="true" /><Input id="display-search" className="h-10 pl-9" aria-label="搜索大屏名称" placeholder="搜索大屏名称" value={search} onChange={e => { setSearch(e.target.value); setLimit(12); }} /></div>
        <select aria-label="筛选大屏状态" className="h-10 min-w-32 rounded-md border bg-background px-3 py-2 text-sm focus-visible:outline-ring" value={filter} onChange={e => { setFilter(e.target.value as DisplayFilter); setLimit(12); }}>
          <option value="all">全部状态</option><option value="published">已发布</option><option value="unpublished">未发布</option><option value="disabled">已停用</option><option value="archived">已归档</option>
        </select><p className="text-xs text-muted-foreground sm:ml-auto">共 {items.length} 个大屏</p>
      </div>
      {items.length ? <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))] gap-5">{items.slice(0, limit).map(item => <DisplayCard key={item.id} instance={item} list={data} selected={instanceID === item.id} onOpen={open} />)}</div> : <div className="rounded-xl border border-dashed"><EmptyState title={data.items.length ? '没有匹配的大屏' : '还没有大屏'} description={data.items.length ? '试试其他名称或状态。' : undefined}>{data.items.length ? <Button variant="outline" onClick={() => { setSearch(''); setFilter('all'); }}>清除筛选</Button> : createAllowed && <Button onClick={() => openDialog('create')}><Plus aria-hidden="true" />新建大屏</Button>}</EmptyState></div>}
      {items.length > limit && <div className="text-center"><Button variant="outline" onClick={() => setLimit(limit + 12)}>加载更多大屏</Button></div>}
      {instanceID && <InstancePanel key={`${instanceID}:${packageID ?? ''}`} id={instanceID} list={data} packageID={packageID} initialView={view} close={() => onSelect()} returnFocus={returnFocus} />}
    </>}
    <Dialog open={dialog !== null} onOpenChange={open => { if (!open && !busy) setDialog(null); }}><DialogContent>
      <DialogHeader><DialogTitle>{dialog === 'create' ? '新建大屏' : '项目大屏设置'}</DialogTitle><DialogDescription>{dialog === 'create' ? project.project.name : data?.settings.enabled ? '停用后，此项目的所有大屏和固定终端将停止运行。' : '启用此项目的大屏管理与访问。'}</DialogDescription></DialogHeader>
      <FormNotice error>{error}</FormNotice>
      {dialog === 'create' ? <form className="space-y-5" onSubmit={e => { e.preventDefault(); if (createAllowed) void command('/instances', { name: name.trim(), expected_revision: '0' }); }}>
        <label className="block space-y-2 text-sm font-medium"><span>大屏名称</span><Input value={name} onChange={e => setName(e.target.value)} maxLength={100} required placeholder="例如：桥梁运行总览" disabled={busy} /></label>
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={() => setDialog(null)}>取消</Button><Button disabled={busy || !name.trim() || !createAllowed}>{busy ? '正在创建…' : '创建大屏'}</Button></DialogFooter>
      </form> : <DialogFooter><Button variant="outline" disabled={busy} onClick={() => setDialog(null)}>取消</Button><Button variant={data?.settings.enabled ? 'destructive' : 'default'} disabled={busy || !project.available || !data || !project.can(P.ProjectDisplaySettings) || (!active && !data.settings.enabled)} onClick={() => { if (data) void command('/settings', { expected_revision: data.settings.revision, enabled: !data.settings.enabled }, 'PUT'); }}>{busy ? '正在保存…' : data?.settings.enabled ? '停用项目大屏' : '确认启用'}</Button></DialogFooter>}
    </DialogContent></Dialog>
  </section>;
}

function DisplayCard({ instance, list, selected, onOpen }: { instance: DisplayInstance; list: DisplayList; selected: boolean; onOpen: (id: string, view: DisplayView, trigger: HTMLElement) => void }) {
  const project = useProject(), { client } = useIdentity();
  const detail = useQuery({ queryKey: [...project.key, 'displays', instance.id], queryFn: ({ signal }) => client.displayDetail({ tenantId: project.project.tenant_id, projectId: project.project.id }, instance.id, signal), enabled: project.available && project.can(P.ProjectDisplayView), refetchInterval: selected ? false : 30000, staleTime: 10000, retry: false });
  const data = !detail.error ? detail.data : undefined, summary = data && displaySummary(data, list.settings.enabled);
  // The list can be newer than the card's cached detail after another user's write.
  const fresh = data?.instance.revision === instance.revision;
  const canOpen = project.available && fresh && summary?.canOpen;
  const changes = summary && configChanges(summary.liveConfig, summary.draft);
  const draftText = !summary?.management ? '访问入口' : summary.draft ? summary.liveConfig ? changes?.length ? `${changes.length} 项未发布更改` : '与线上配置一致' : '有未发布草稿' : '尚无工作草稿';
  const status = detail.error ? '读取失败' : !fresh ? '正在更新' : summary?.status ?? '正在读取';
  const program = summary?.livePackage ?? summary?.draftPackage;
  return <article className="flex min-w-0 flex-col overflow-hidden rounded-xl border bg-card shadow-xs" aria-label={instance.name}>
    <button className="flex h-36 items-center justify-center gap-4 border-b bg-muted/45 px-5 text-left transition-colors hover:bg-muted/70 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring" aria-label={`查看 ${instance.name}`} onClick={e => onOpen(instance.id, '概览', e.currentTarget)}>
      <span className="flex size-14 shrink-0 items-center justify-center rounded-xl border bg-background"><Monitor className="size-7 text-muted-foreground" aria-hidden="true" /></span><span className="min-w-0"><span className="block truncate text-sm font-medium">{program?.manifest.name || program?.manifest.app_key || '项目大屏'}</span><span className="mt-1 block text-xs text-muted-foreground">{program ? `程序 ${program.manifest.version}` : summary?.management ? '尚未配置程序' : '独立访问入口'}</span></span>
    </button>
    <div className="flex-1 space-y-5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-2"><button className="min-w-0 break-words text-left font-semibold hover:underline focus-visible:outline-ring" onClick={e => onOpen(instance.id, '概览', e.currentTarget)}>{instance.name}</button><Badge variant="outline" className={canOpen ? 'border-emerald-600/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : ''}>{status}</Badge></div>
      {detail.error ? <div className="space-y-2 text-sm"><p className="text-destructive">无法读取大屏信息</p><Button size="sm" variant="outline" onClick={() => void detail.refetch()}>重试</Button></div> : <dl className="grid grid-cols-2 gap-3 text-sm"><div><dt className="text-xs text-muted-foreground">线上程序</dt><dd className="mt-1.5 flex items-center gap-1.5"><Package className="size-3.5 text-muted-foreground" aria-hidden="true" />{summary?.livePackage?.manifest.version ?? (instance.current_release_id ? '已发布' : '—')}</dd></div><div><dt className="text-xs text-muted-foreground">固定终端</dt><dd className="mt-1.5">{data?.terminals ? `${data.terminals.filter(t => t.state !== 'revoked').length} 台` : '—'}</dd></div></dl>}
      <div className="flex min-h-9 items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground"><span>{draftText}</span>{summary?.management && <Button size="sm" variant="ghost" className="h-8 px-2" onClick={e => onOpen(instance.id, '概览', e.currentTarget)}>查看</Button>}</div>
    </div>
    <footer className="flex gap-2 border-t px-5 py-4">
      {canOpen && data ? <Button asChild className="min-h-10 flex-1"><a href={data.access_url} target="_blank" rel="noreferrer">打开大屏<ArrowUpRight aria-hidden="true" /></a></Button> : <Button className="min-h-10 flex-1" disabled>打开大屏<ArrowUpRight aria-hidden="true" /></Button>}
      <Button id={`display-manage-${instance.id}`} variant="outline" className="min-h-10" onClick={e => onOpen(instance.id, '概览', e.currentTarget)}>{summary?.management ? '管理' : '详情'}</Button>
      {project.can(P.ProjectDisplayManage) && <Button variant="ghost" size="icon" className="size-10" aria-label={`设置 ${instance.name}`} onClick={e => onOpen(instance.id, '设置', e.currentTarget)}><Settings2 aria-hidden="true" /></Button>}
    </footer>
  </article>;
}
