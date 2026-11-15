import { useRef, useState, type ReactNode } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { CalendarDays, FolderOpen, Search, Trash2, Upload, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Badge } from '@infrapro/ui/components/badge';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { FileState } from '@infrapro/ui/patterns/files';
import { useFiles } from './context';
import { fileStates, type FileSearch } from './state';
import { fileDate, fileSize, type AttachmentView } from './file-format';
import { AttachmentsTable } from './attachments-table';
import { AttachmentDetail } from './attachment-detail';
import { UploadDetail } from './upload-detail';
import { FileUploadForm } from './upload-form';

export function FilesWorkspace({ search, onSearch, compact = false, compactTitle = '资料附件', attachmentLayout = 'table' }: { search: FileSearch; onSearch: (value: FileSearch) => void; compact?: boolean; compactTitle?: ReactNode; attachmentLayout?: 'table' | 'list' }) {
  const f = useFiles(), caps = f.capabilities.data, tab = search.tab ?? 'active';
  const Heading = compact ? 'h2' : 'h1', compactList = compact && attachmentLayout === 'list';
  const [q, setQ] = useState(''), [from, setFrom] = useState(''), [to, setTo] = useState('');
  const [dateOpen, setDateOpen] = useState(false), [dateDraft, setDateDraft] = useState({ from: '', to: '' });
  const [adding, setAdding] = useState(false), [managed, setManaged] = useState(false), [initialView, setInitialView] = useState<AttachmentView>('overview');
  const returnFocus = useRef<HTMLElement | null>(null);
  const attachments = useInfiniteQuery({
    queryKey: [...f.prefix, 'attachments', tab, q, from, to, caps?.version], initialPageParam: '',
    queryFn: ({ signal, pageParam }) => f.client.fileAttachments(f.scope, { state: tab === 'trash' ? 'trashed' : 'active', q, cursor: pageParam, created_from: from ? new Date(`${from}T00:00:00+08:00`).toISOString() : undefined, created_to: to ? new Date(`${to}T23:59:59.999+08:00`).toISOString() : undefined }, signal),
    getNextPageParam: page => page.next_cursor || undefined, enabled: !!caps && tab !== 'uploads', refetchInterval: 30_000, retry: false,
  });
  const uploads = useInfiniteQuery({
    queryKey: [...f.prefix, 'uploads', managed, caps?.version], initialPageParam: '',
    queryFn: ({ signal, pageParam }) => f.client.fileUploads(f.scope, { view: managed ? 'managed' : 'mine', cursor: pageParam }, signal),
    getNextPageParam: page => page.next_cursor || undefined, enabled: !!caps && tab === 'uploads', refetchInterval: 10_000, retry: false,
  });
  const files = attachments.data?.pages.flatMap(page => page.items) ?? [], sessions = uploads.data?.pages.flatMap(page => page.items) ?? [];
  const query = tab === 'uploads' ? uploads : attachments;
  const captureFocus = () => { returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; };
  const closeDetail = () => { setInitialView('overview'); onSearch({ tab }); };
  const clearFilters = () => { setQ(''); setFrom(''); setTo(''); };
  if (f.capabilities.error) return <div className="space-y-3"><FileState error>{f.capabilities.error.message}</FileState><Button variant="outline" onClick={() => void f.capabilities.refetch()}>重新核验附件资格</Button></div>;
  if (!caps) return <FileState>正在核验附件资格…</FileState>;
  return <section className={`min-w-0 ${compactList ? 'space-y-3' : 'space-y-6'}`}>
    <header className="flex flex-wrap items-center justify-between gap-3"><div><div className="flex flex-wrap items-baseline gap-2"><Heading className={compact ? compactList ? 'text-base font-semibold' : 'text-lg font-semibold' : 'text-2xl font-semibold tracking-tight'}>{compact ? compactTitle : '文件与附件'}</Heading>{compactList && !query.isPending && !query.error && <span className="text-xs font-normal text-muted-foreground">{query.hasNextPage ? '已加载 ' : ''}{files.length} 份</span>}</div>{!compactList && <p className="mt-1.5 text-sm text-muted-foreground">{f.scope.project ? '项目资料' : '租户资料'}，在这里查找、查看和管理附件。</p>}</div>{caps.can_manage && caps.accept_uploads && <Button id="files-add" variant={compactList ? 'ghost' : 'default'} size={compactList ? 'sm' : 'default'} onClick={() => { captureFocus(); setAdding(true); }}><Upload aria-hidden="true" />添加附件</Button>}</header>
    {caps.status === 'archived' && <FileState>项目已归档，保留授权读取，附件管理已关闭。</FileState>}
    {caps.mode !== 'online' && <FileState>文件服务正在维护，当前不可提交修改。</FileState>}
    {!caps.accept_uploads && caps.mode === 'online' && caps.status !== 'archived' && <FileState>新上传已关闭；已有资料仍按当前权限读取。</FileState>}
    {!compact && <Tabs value={tab} onValueChange={value => { setInitialView('overview'); onSearch({ tab: value as FileSearch['tab'] }); }}><TabsList aria-label="文件分类" className="h-11 w-full justify-start gap-5 rounded-none border-b bg-transparent p-0">{[{ key: 'active', label: '普通附件', icon: FolderOpen }, { key: 'uploads', label: '上传与待处理', icon: Upload }, ...(caps.can_view_trash ? [{ key: 'trash', label: '回收站', icon: Trash2 }] : [])].map(item => <TabsTrigger key={item.key} value={item.key} className="h-11 flex-none rounded-none border-0 border-b-2 border-transparent px-0 pb-3 text-muted-foreground shadow-none data-[state=active]:border-foreground data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"><item.icon aria-hidden="true" />{item.label}</TabsTrigger>)}</TabsList></Tabs>}
    {!compact && tab !== 'uploads' && <div className="flex flex-wrap items-center gap-2"><div className="relative min-w-48 flex-1 sm:max-w-80"><Search aria-hidden="true" className="absolute top-2.5 left-3 size-4 text-muted-foreground" /><Input id="files-search" aria-label="搜索附件名称" placeholder="搜索附件名称…" value={q} maxLength={100} onChange={event => setQ(event.target.value)} className="h-9 pr-9 pl-9" />{q && <button aria-label="清空搜索" className="absolute top-2 right-2 rounded p-0.5 text-muted-foreground hover:text-foreground" onClick={() => setQ('')}><X className="size-4" /></button>}</div><Button variant="outline" size="sm" className="border-dashed" onClick={() => { setDateDraft({ from, to }); setDateOpen(true); }}><CalendarDays aria-hidden="true" />{from || to ? `${from || '不限'} — ${to || '不限'}` : '创建日期'}</Button>{(q || from || to) && <Button variant="ghost" size="sm" onClick={clearFilters}>重置<X aria-hidden="true" /></Button>}</div>}
    {tab === 'uploads' && caps.can_manage && <div className="flex items-center gap-2"><Checkbox id="files-managed" checked={managed} onCheckedChange={value => setManaged(value === true)} /><Label htmlFor="files-managed">查看本空间可管理的会话</Label></div>}
    {query.error ? <div className="space-y-3 rounded-lg border p-6"><FileState error>{query.error.message}</FileState><Button variant="outline" onClick={() => void query.refetch()}>重新加载列表</Button></div> : query.isPending ? <div role="status" className="space-y-3 rounded-lg border p-6"><span className="sr-only">正在读取文件列表…</span>{[1, 2, 3].map(key => <div key={key} className="h-12 animate-pulse rounded bg-muted" />)}</div> : tab === 'uploads' ? <>
      {sessions.length ? <div className="overflow-hidden rounded-lg border"><Table aria-label="上传记录"><TableHeader><TableRow className="bg-muted/30"><TableHead className="pl-4">文件名称</TableHead><TableHead>状态</TableHead><TableHead className="hidden sm:table-cell">大小</TableHead><TableHead className="hidden lg:table-cell">上传时间</TableHead><TableHead className="w-16"><span className="sr-only">操作</span></TableHead></TableRow></TableHeader><TableBody>{sessions.map(item => <TableRow key={item.id}><TableCell className="max-w-0 py-4 pl-4"><button id={`upload-open-${item.id}`} className="max-w-full truncate rounded text-left font-medium outline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" aria-label={`查看上传 ${item.display_name}`} onClick={() => { captureFocus(); onSearch({ tab: 'uploads', upload: item.id, job: item.job?.id, generation: item.job?.generation }); }}>{item.display_name}</button></TableCell><TableCell><Badge variant={['failed', 'needs_action'].includes(item.job?.state ?? item.state) ? 'destructive' : 'secondary'}>{fileStates[item.job?.state ?? item.state] ?? item.state}</Badge></TableCell><TableCell className="hidden text-muted-foreground sm:table-cell">{fileSize(item.size_bytes)}</TableCell><TableCell className="hidden text-muted-foreground lg:table-cell">{fileDate(item.created_at, true)}</TableCell><TableCell><Button variant="ghost" size="sm" onClick={() => { returnFocus.current = document.getElementById(`upload-open-${item.id}`); onSearch({ tab: 'uploads', upload: item.id, job: item.job?.id, generation: item.job?.generation }); }}>查看与处理</Button></TableCell></TableRow>)}</TableBody></Table></div> : <EmptyFiles text="暂无上传会话。" />}
    </> : files.length ? <AttachmentsTable items={files} layout={attachmentLayout} onOpen={(file, view = 'overview') => { returnFocus.current = document.getElementById(`file-open-${file.id}`); setInitialView(view); onSearch({ tab, attachment: file.id }); }} /> : <EmptyFiles text={q || from || to ? '没有找到匹配的附件。' : tab === 'trash' ? '回收站为空。' : '尚未添加普通附件。'} reset={q || from || to ? clearFilters : undefined} />}
    {!query.isPending && !query.error && (!compactList || query.hasNextPage) && <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><span>{query.hasNextPage ? '已加载' : '共'} {tab === 'uploads' ? sessions.length : files.length} {tab === 'uploads' ? '条上传记录' : '个附件'}</span>{query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? '正在加载…' : tab === 'uploads' ? '加载更多上传' : '加载更多附件'}</Button>}</div>}
    {search.attachment ? <AttachmentDetail key={search.attachment} id={search.attachment} revision={search.revision} initialView={initialView} onSearch={onSearch} onClose={closeDetail} returnFocus={returnFocus} /> : (search.upload || search.job) && <UploadDetail key={`${search.upload ?? search.job}:${search.generation ?? ''}`} id={search.upload} job={search.job} generation={search.generation} onSearch={onSearch} onClose={closeDetail} returnFocus={returnFocus} />}
    <ResponsivePanel open={adding} onOpenChange={open => { setAdding(open); if (!open) void f.changed(); }} title="添加附件" description="上传文件，供当前空间的成员按权限查阅。" returnFocus={returnFocus} fallbackFocusSelector="#files-add">{adding && <FileUploadForm done={result => { setAdding(false); onSearch({ tab: 'uploads', upload: result.upload_id ?? undefined, job: result.job_id ?? undefined, generation: result.job_generation ?? undefined }); }} />}</ResponsivePanel>
    <Dialog open={dateOpen} onOpenChange={setDateOpen}><DialogContent><DialogHeader><DialogTitle>筛选创建日期</DialogTitle><DialogDescription>按北京时间筛选，留空表示不限。</DialogDescription></DialogHeader><form className="space-y-5" onSubmit={event => { event.preventDefault(); if (dateDraft.from && dateDraft.to && dateDraft.from > dateDraft.to) return; setFrom(dateDraft.from); setTo(dateDraft.to); setDateOpen(false); }}><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="files-from">开始日期</Label><Input id="files-from" type="date" value={dateDraft.from} max={dateDraft.to || undefined} onChange={event => setDateDraft(value => ({ ...value, from: event.target.value }))} /></div><div className="space-y-2"><Label htmlFor="files-to">结束日期</Label><Input id="files-to" type="date" value={dateDraft.to} min={dateDraft.from || undefined} onChange={event => setDateDraft(value => ({ ...value, to: event.target.value }))} /></div></div><DialogFooter><Button type="button" variant="outline" onClick={() => { setFrom(''); setTo(''); setDateOpen(false); }}>清除日期</Button><Button type="submit">应用筛选</Button></DialogFooter></form></DialogContent></Dialog>
  </section>;
}

function EmptyFiles({ text, reset }: { text: string; reset?: () => void }) {
  return <div className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-lg border border-dashed p-6 text-center"><FolderOpen aria-hidden="true" className="size-8 stroke-1 text-muted-foreground" /><FileState>{text}</FileState>{reset && <Button variant="outline" size="sm" onClick={reset}>清除筛选</Button>}</div>;
}
