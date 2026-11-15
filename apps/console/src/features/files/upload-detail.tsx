import { useRef, useState, type RefObject } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import type { FileReconfirmInput, FileRevisionCommand } from '@infrapro/api-client';
import { ArrowLeft, Info, MoreHorizontal, Upload } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { FileState } from '@infrapro/ui/patterns/files';
import { toast } from 'sonner';
import { ObjectHistory } from '../audit/object-history';
import { useFiles } from './context';
import { useFilePanelView } from './use-panel-view';
import { FileUploadForm } from './upload-form';
import { FixedFileTaskLink } from './task-link';
import { fileDate, fileSize } from './file-format';
import { fileStates, fileReason, type FileSearch } from './state';

type UploadView = 'overview' | 'resume' | 'cancel' | 'reconfirm' | 'technical';

export function UploadDetail({ id, job, generation, onSearch, onClose, returnFocus }: {
  id?: string; job?: string; generation?: string; onSearch: (search: FileSearch) => void;
  onClose: () => void; returnFocus: RefObject<HTMLElement | null>;
}) {
  const f = useFiles(), [reason, setReason] = useState('');
  const { view, setView, contentRef, openFromMenu, onMenuCloseAutoFocus } = useFilePanelView<UploadView>('overview');
  const jobQuery = useQuery({ queryKey: [...f.prefix, 'job', job, generation], queryFn: ({ signal }) => f.client.fileJob(f.scope, job!, generation, signal), enabled: !!job, refetchInterval: query => ['queued', 'running', 'retry_wait', 'cancelling'].includes(query.state.data?.state ?? '') ? 2000 : 30_000, retry: false });
  const uploadID = id ?? jobQuery.data?.upload_id;
  const uploadQuery = useQuery({ queryKey: [...f.prefix, 'upload', uploadID], queryFn: ({ signal }) => f.client.fileUpload(f.scope, uploadID!, signal), enabled: !!uploadID, refetchInterval: 10_000, retry: false });
  const linkedJob = uploadQuery.data?.job;
  const currentJobQuery = useQuery({ queryKey: [...f.prefix, 'job', linkedJob?.id, generation ?? linkedJob?.generation], queryFn: ({ signal }) => f.client.fileJob(f.scope, linkedJob!.id, generation ?? linkedJob!.generation, signal), enabled: !job && !!linkedJob, refetchInterval: query => ['queued', 'running', 'retry_wait', 'cancelling'].includes(query.state.data?.state ?? '') ? 2000 : 30_000, retry: false });
  const error = jobQuery.error ?? uploadQuery.error ?? currentJobQuery.error;
  const current = error ? undefined : uploadQuery.data, task = job ? jobQuery.data : currentJobQuery.data;
  const can = (kind: string) => !!current?.allowed_actions.some(action => action.kind === kind && action.enabled);
  const receipt = useRef<{ key: string; epoch: string; input: FileRevisionCommand | FileReconfirmInput } | null>(null);
  const mutation = useMutation({
    mutationFn: async (kind: 'cancel' | 'reconfirm') => {
      if (!current || !f.capabilities.data || f.capabilities.error || !can(kind)) throw Error('当前状态不允许此操作，请重新核对。');
      const key = JSON.stringify({ kind, reason: reason.trim(), upload: current.id });
      // Retry the exact request after a lost response, even if polling has refreshed the session.
      if (receipt.current?.key !== key) {
        const attachment = kind === 'reconfirm' && current.attachment_id ? await f.client.fileAttachment(f.scope, current.attachment_id) : undefined;
        const body = kind === 'cancel' ? { reason: reason.trim(), expected_revision: current.revision } : { reason: reason.trim(), expected_generation: current.job!.generation, expected_control_revision: current.job!.control_revision, expected_attachment_revision: attachment?.revision ?? null };
        receipt.current = { key, epoch: f.capabilities.data.restore_epoch, input: { ...body, request_id: crypto.randomUUID() } };
      }
      if (receipt.current.epoch !== f.capabilities.data.restore_epoch) throw Error('恢复代次已变化，请重新打开上传记录。');
      return kind === 'cancel' ? f.client.fileCancelUpload(f.scope, current.id, receipt.current.input as FileRevisionCommand, receipt.current.epoch) : f.client.fileReconfirm(f.scope, current.id, receipt.current.input as FileReconfirmInput, receipt.current.epoch);
    },
    onSuccess: async result => { await f.changed(); toast.success(view === 'cancel' ? '上传已取消' : '已重新提交处理'); setView('overview'); setReason(''); onSearch({ tab: 'uploads', upload: current?.id, job: result.job_id ?? current?.job?.id, generation: result.job_generation ?? current?.job?.generation }); },
  });
  const status = task?.state ?? current?.job?.state ?? current?.state ?? '';
  const resultButton = task?.result_attachment_id && <Button variant="outline" onClick={() => onSearch({ attachment: task.result_attachment_id!, revision: task.result_revision_id ?? undefined })}>查看本代附件修订</Button>;
  const footer = current && view === 'overview' ? <div className="flex flex-wrap items-center justify-end gap-2">{resultButton}{can('resume') && <Button onClick={() => setView('resume')}><Upload aria-hidden="true" />继续上传</Button>}{can('reconfirm') && <Button onClick={() => setView('reconfirm')}>重新确认绑定</Button>}<DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="icon" aria-label="上传更多操作"><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" onCloseAutoFocus={onMenuCloseAutoFocus} onEscapeKeyDown={event => event.stopPropagation()}><DropdownMenuItem onSelect={() => openFromMenu('technical')}><Info aria-hidden="true" />技术信息与处理报告</DropdownMenuItem>{can('cancel') && <DropdownMenuItem variant="destructive" onSelect={() => openFromMenu('cancel')}>取消未提交的上传</DropdownMenuItem>}</DropdownMenuContent></DropdownMenu></div> : undefined;
  return <ResponsivePanel open onOpenChange={open => { if (!open) { if (mutation.isPending) toast.info('操作正在提交，请稍候。'); else onClose(); } }} title={current?.display_name ?? '上传与待处理'} description={current ? `${fileSize(current.size_bytes)} · ${fileDate(current.created_at, true)}` : '查看上传进度和处理结果'} returnFocus={returnFocus} fallbackFocusSelector="#files-add" footer={footer}>
    {error ? <div className="space-y-3"><FileState error>{error.message}</FileState><Button variant="outline" onClick={() => { if (job) void jobQuery.refetch(); if (uploadID) void uploadQuery.refetch(); if (!job && linkedJob) void currentJobQuery.refetch(); }}>重新读取上传</Button></div> : !current ? <FileState>正在读取上传与后台任务…</FileState> : <div ref={contentRef} className="space-y-5">
      {view !== 'overview' && <Button variant="ghost" size="sm" className="-ml-2" disabled={mutation.isPending} onClick={() => setView('overview')}><ArrowLeft aria-hidden="true" />返回概览</Button>}
      {view === 'overview' && <><Badge variant={['failed', 'needs_action'].includes(status) ? 'destructive' : 'secondary'}>{fileStates[status] ?? status}</Badge><dl className="grid grid-cols-2 gap-5 text-sm"><div className="col-span-2"><dt className="text-xs text-muted-foreground">原始文件名</dt><dd className="mt-1.5 break-words">{current.filename}</dd></div><div><dt className="text-xs text-muted-foreground">文件大小</dt><dd className="mt-1.5">{fileSize(current.size_bytes)}</dd></div><div><dt className="text-xs text-muted-foreground">上传时间</dt><dd className="mt-1.5">{fileDate(current.created_at, true)}</dd></div></dl>{current.state === 'receiving' && <div className="space-y-2"><p className="text-sm">已接收 {current.parts.length} / {current.part_count} 片</p><progress aria-label="已接收上传分片" className="h-2 w-full" value={current.parts.length} max={Math.max(1, current.part_count)} /><p className="text-xs text-muted-foreground">选择同一文件，可继续传输剩余部分。</p></div>}{current.description && <p className="whitespace-pre-wrap break-words text-sm leading-6">{current.description}</p>}{task?.error_code && <FileState error>{fileReason(task.error_code)}</FileState>}{current.prepared_until && <FileState>待绑定内容保留至 {fileDate(current.prepared_until, true)}。重新确认将根据当前附件版本创建新一代任务。</FileState>}</>}
      {view === 'resume' && <FileUploadForm key={current.id} initial={current} done={result => onSearch({ tab: 'uploads', upload: current.id, job: result.job_id ?? undefined, generation: result.job_generation ?? undefined })} />}
      {(view === 'cancel' || view === 'reconfirm') && <form className="space-y-4" onSubmit={event => { event.preventDefault(); if (reason.trim()) mutation.mutate(view); }}><h3 className="font-semibold">{view === 'cancel' ? '取消未提交的上传' : '按当前版本重新确认绑定'}</h3><p className="text-sm leading-6 text-muted-foreground">{view === 'cancel' ? '取消后不再继续此上传，已绑定附件不受影响。' : '核对原文件和当前附件后，使用已有内容重新提交检查与绑定。'}</p><div className="space-y-2"><Label htmlFor="upload-control-reason">操作原因</Label><Input id="upload-control-reason" value={reason} maxLength={500} required disabled={mutation.isPending} onChange={event => setReason(event.target.value)} /></div>{mutation.error && <FileState error>{mutation.error.message}</FileState>}<div className="flex flex-wrap gap-2"><Button type="submit" variant={view === 'cancel' ? 'destructive' : 'default'} disabled={!reason.trim() || mutation.isPending || !can(view)}>{mutation.isPending ? '正在提交…' : view === 'cancel' ? '取消未提交的上传' : '按当前版本重新确认绑定'}</Button><Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => setView('overview')}>返回</Button></div></form>}
      {view === 'technical' && <section className="space-y-5"><h3 className="font-semibold">技术信息与处理报告</h3><dl className="space-y-4 text-sm">{[['上传会话', current.id], ['SHA-256', current.sha256], ['分片接收', `${current.parts.length} / ${current.part_count}`]].map(([label, value]) => <div key={label}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1.5 break-all font-mono text-xs leading-5">{value}</dd></div>)}</dl><div className="flex flex-wrap gap-2"><ObjectHistory objectType="file_upload" objectId={current.id} />{current.job && <FixedFileTaskLink job={current.job.id} generation={generation ?? current.job.generation} />}</div>{task && <div className="space-y-2 border-t pt-4"><h4 className="text-sm font-medium">本代处理报告</h4><pre className="overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/40 p-3 text-xs leading-5">{JSON.stringify(task.report, null, 2)}</pre></div>}</section>}
    </div>}
  </ResponsivePanel>;
}
