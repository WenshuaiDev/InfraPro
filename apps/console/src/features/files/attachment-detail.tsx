import { useEffect, useState, type RefObject } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { ArrowLeft, Download, Info, MoreHorizontal, Pencil, RotateCcw, Trash2, Upload } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { FileState } from '@infrapro/ui/patterns/files';
import { toast } from 'sonner';
import { ObjectHistory } from '../audit/object-history';
import { useFiles } from './context';
import { useFilePanelView } from './use-panel-view';
import { fileStates, type FileSearch } from './state';
import { fileAction, fileContentAllowed, fileDate, fileSize, type AttachmentView } from './file-format';
import { AttachmentActionForm } from './attachment-actions';
import { AttachmentVersions } from './attachment-versions';
import { AttachmentPreview } from './attachment-visual';
import { FileUploadForm } from './upload-form';
import { FixedFileTaskLink } from './task-link';
import { documentFormats } from './preview/model';

export function AttachmentDetail({ id, revision, initialView = 'overview', onSearch, onClose, returnFocus }: {
  id: string; revision?: string; initialView?: AttachmentView; onSearch: (search: FileSearch) => void;
  onClose: () => void; returnFocus: RefObject<HTMLElement | null>;
}) {
  const f = useFiles(), [busy, setBusy] = useState(false), [versionsOpen, setVersionsOpen] = useState(false);
  const { view, setView, contentRef, openFromMenu, onMenuCloseAutoFocus } = useFilePanelView<AttachmentView>(initialView);
  const detail = useQuery({ queryKey: [...f.prefix, 'attachment', id], queryFn: ({ signal }) => f.client.fileAttachment(f.scope, id, signal), refetchInterval: busy ? false : 30_000, retry: false });
  const file = detail.error ? undefined : detail.data;
  const history = useInfiniteQuery({
    queryKey: [...f.prefix, 'attachment', id, 'history'], initialPageParam: '',
    queryFn: ({ signal, pageParam }) => f.client.fileRevisions(f.scope, id, pageParam, signal), getNextPageParam: page => page.next_cursor || undefined,
    enabled: !!file && (versionsOpen || !!revision && revision !== file.current_revision.id), retry: false,
  });
  const revisions = history.error && !history.isFetchNextPageError ? [] : history.data?.pages.flatMap(page => page.items) ?? [];
  const selected = !revision || revision === file?.current_revision.id ? file?.current_revision : revisions.find(item => item.id === revision);
  const { hasNextPage, isFetching, error: historyError, fetchNextPage } = history;
  // A fixed revision from a task or notification may be beyond the first page.
  useEffect(() => {
    if (file && revision && !selected && hasNextPage && !isFetching && !historyError) void fetchNextPage();
  }, [file, revision, selected, hasNextPage, isFetching, historyError, fetchNextPage]);
  const isHistorical = !!file && !!selected && selected.id !== file.current_revision.id;
  const downloadable = !!file && !!selected && !f.capabilities.error && fileContentAllowed(f.capabilities.data, file, selected);
  const goRevision = (value?: string) => onSearch({ tab: file?.status === 'trashed' ? 'trash' : 'active', attachment: id, revision: value });
  const readView = ['overview', 'technical'].includes(view);
  const actions = file && <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="icon" aria-label="附件更多操作" disabled={busy}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" onCloseAutoFocus={onMenuCloseAutoFocus} onEscapeKeyDown={event => event.stopPropagation()}>
    {fileAction(file, 'rename') && <DropdownMenuItem onSelect={() => openFromMenu('edit')}><Pencil aria-hidden="true" />编辑信息</DropdownMenuItem>}
    {fileAction(file, 'replace') && f.capabilities.data?.accept_uploads && <DropdownMenuItem onSelect={() => openFromMenu('replace')}><Upload aria-hidden="true" />替换正文</DropdownMenuItem>}
    {isHistorical && fileAction(file, 'revert') && selected?.file_state === 'available' && <DropdownMenuItem onSelect={() => openFromMenu('revert')}><RotateCcw aria-hidden="true" />还原此版本</DropdownMenuItem>}
    <DropdownMenuItem onSelect={() => openFromMenu('technical')}><Info aria-hidden="true" />技术信息与处理记录</DropdownMenuItem>
    {(fileAction(file, 'trash') || fileAction(file, 'restore')) && <DropdownMenuSeparator />}
    {fileAction(file, 'trash') && <DropdownMenuItem variant="destructive" onSelect={() => openFromMenu('trash')}><Trash2 aria-hidden="true" />移入回收站</DropdownMenuItem>}
    {fileAction(file, 'restore') && <DropdownMenuItem onSelect={() => openFromMenu('restore')}><RotateCcw aria-hidden="true" />恢复附件</DropdownMenuItem>}
  </DropdownMenuContent></DropdownMenu>;
  const footer = file && readView ? <div className="flex items-center justify-between gap-3"><span className="text-xs text-muted-foreground">{isHistorical ? '历史版本' : '当前版本'} {selected?.revision_no ?? file.current_revision.revision_no}</span><div className="flex items-center gap-2">{downloadable && selected && <Button asChild><a href={f.client.fileDownloadURL(f.scope, id, selected.id)} download><Download aria-hidden="true" />下载原件</a></Button>}{actions}</div></div> : undefined;
  return <ResponsivePanel open size={selected && documentFormats.includes(selected.format) && view === 'overview' && downloadable ? 'wide' : 'default'} onOpenChange={open => { if (!open) { if (busy) toast.info('操作正在提交，请稍候。'); else onClose(); } }} title={file?.display_name ?? '附件详情'} description={selected ? `${selected.format.toUpperCase()} · ${fileSize(selected.size_bytes)} · 版本 ${selected.revision_no}` : '查看附件内容与文件信息'} returnFocus={returnFocus} fallbackFocusSelector="#files-search, #files-add" footer={footer}>
    {detail.error ? <div className="space-y-3"><FileState error>{detail.error.message}</FileState><Button variant="outline" onClick={() => void detail.refetch()}>重新读取附件</Button></div> : !file ? <FileState>正在读取附件…</FileState> : <div ref={contentRef} className="space-y-5">
      {view !== 'overview' && <Button variant="ghost" size="sm" className="-ml-2" disabled={busy} onClick={() => setView('overview')}><ArrowLeft aria-hidden="true" />返回附件详情</Button>}
      {view === 'overview' && <div className="space-y-5">
        <AttachmentVersions attachment={file} selected={selected} items={revisions} open={versionsOpen} onOpenChange={setVersionsOpen} loading={history.isPending} error={history.error} hasMore={history.hasNextPage} loadingMore={history.isFetchingNextPage} onMore={() => void history.fetchNextPage()} onRetry={() => void (history.isFetchNextPageError ? history.fetchNextPage() : history.refetch())} onSelect={item => goRevision(item.id === file.current_revision.id ? undefined : item.id)} onRestore={() => setView('revert')} />
        {!selected ? <div className="space-y-3"><FileState error={!!history.error}>{history.error?.message ?? (history.isFetching || history.hasNextPage ? '正在定位历史版本…' : '未找到指定的历史版本。')}</FileState>{history.error && <Button variant="outline" onClick={() => void history.refetch()}>重试读取历史</Button>}<Button variant="outline" onClick={() => goRevision()}>返回最新版</Button></div> : <>
          <AttachmentPreview key={selected.id} attachment={file} revision={selected} />
          <dl className="grid w-full grid-cols-2 gap-x-6 gap-y-5 text-sm">{[['文件类型', `${selected.format.toUpperCase()} 文件`], ['文件大小', fileSize(selected.size_bytes)], ['创建时间', fileDate(file.created_at, true)], ['更新时间', fileDate(file.updated_at, true)]].map(([label, value]) => <div key={label} className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1.5 break-words tabular-nums">{value}</dd></div>)}</dl>
          {file.description && <section className="space-y-2 border-t pt-5"><h3 className="text-xs text-muted-foreground">说明</h3><p className="whitespace-pre-wrap break-words text-sm leading-6">{file.description}</p></section>}
        </>}
        {file.status !== 'active' && <FileState>{fileStates[file.status] ?? file.status}{file.purge_after ? `，可恢复至 ${fileDate(file.purge_after, true)}。` : '。'}</FileState>}
      </div>}
      {view === 'technical' && <section className="space-y-5"><div><h3 className="font-semibold">技术信息</h3><p className="mt-1 text-xs text-muted-foreground">用于核验文件或排查问题。</p></div><dl className="space-y-4 text-sm">{[['附件编号', file.id], ['附件状态', fileStates[file.status] ?? file.status], ...(selected ? [['版本编号', selected.id], ['原始文件名', selected.original_name], ['媒体类型', selected.media_type], ['正文状态', fileStates[selected.file_state] ?? selected.file_state], ['预览方式', documentFormats.includes(selected.format) ? '浏览器文档阅读器' : '服务器图片预览'], ['图片预览产物状态', fileStates[selected.preview_state] ?? selected.preview_state], ['原始字节数', selected.size_bytes], ['SHA-256', selected.sha256], ['本版本操作者', selected.created_by], ...(selected.reverted_from_revision_id ? [['还原来源版本编号', selected.reverted_from_revision_id]] : [])] : [])].map(([label, value]) => <div key={label}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1.5 break-all font-mono text-xs leading-5">{value}</dd></div>)}</dl><div className="flex flex-wrap gap-2 border-t pt-4"><ObjectHistory objectType="file_attachment" objectId={id} />{selected && <ObjectHistory objectType="file_revision" objectId={id} filter={{ reference_id: selected.id }} label="此修订操作历史" />}{selected?.job_id && selected.job_generation && <FixedFileTaskLink job={selected.job_id} generation={selected.job_generation} />}</div></section>}
      {view === 'replace' && (fileAction(file, 'replace') && f.capabilities.data?.accept_uploads ? <section className="space-y-4"><div><h3 className="font-semibold">替换正文</h3><p className="mt-1 text-sm text-muted-foreground">上传新文件，当前文件将保留在版本记录中。</p></div><FileUploadForm attachment={{ id, revision: file.revision }} done={result => onSearch({ tab: 'uploads', upload: result.upload_id ?? undefined, job: result.job_id ?? undefined, generation: result.job_generation ?? undefined })} /></section> : <FileState>当前状态不允许替换附件。</FileState>)}
      {(view === 'edit' || view === 'trash' || view === 'restore' || view === 'revert') && <AttachmentActionForm key={`${view}:${view === 'revert' ? selected?.id ?? '' : ''}`} attachment={file} kind={view === 'edit' ? 'update' : view} selected={selected} onBusy={setBusy} onCancel={() => setView('overview')} onRefresh={() => { setView('overview'); void detail.refetch(); }} onDone={() => { toast.success({ edit: '附件信息已保存', trash: '附件已移入回收站', restore: '附件已恢复', revert: '已还原为新版本' }[view]); if (view === 'trash' || view === 'restore') onClose(); else { setView('overview'); if (view === 'revert') goRevision(); } }} />}
    </div>}
  </ResponsivePanel>;
}
