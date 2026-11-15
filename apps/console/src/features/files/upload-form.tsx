import { useEffect, useRef, useState } from 'react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Textarea } from '@infrapro/ui/components/textarea';
import { Label } from '@infrapro/ui/components/label';
import { FileState, UploadPicker, UploadProgress } from '@infrapro/ui/patterns/files';
import type { FileUpload, FileCommandResult } from '@infrapro/api-client';
import { useFiles } from './context';
import { ordinaryAdapter } from './adapters/ordinary';
import { transferFile, type UploadProgress as Progress } from './upload-controller';
import { phases } from './state';

export function FileUploadForm({ initial, attachment, done }: { initial?: FileUpload; attachment?: { id: string; revision: string }; done: (value: FileCommandResult) => void }) {
  const f = useFiles(), [file, setFile] = useState<File | null>(null), [name, setName] = useState(initial?.display_name ?? ''), [description, setDescription] = useState(initial?.description ?? ''), [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false), [error, setError] = useState<Error | null>(null), [progress, setProgress] = useState<Progress>({ phase: 'paused', bytes: 0, total: 0 }), [upload, setUpload] = useState(initial), [locked, setLocked] = useState(false);
  const abort = useRef<AbortController | null>(null), adapter = useRef<ReturnType<typeof ordinaryAdapter> | null>(null);
  // A submitted session can still need an exact replay after a lost response.
  // Keep the in-progress adapter's receipt until it succeeds or the form closes.
  const resumable = locked || !initial || initial.allowed_actions.some(action => action.kind === 'resume' && action.enabled);
  useEffect(() => () => abort.current?.abort(), []);
  const submit = async () => {
    if (!file || !f.capabilities.data || busy) return;
    const controller = new AbortController(); abort.current = controller; setBusy(true); setLocked(true); setError(null);
    try {
      adapter.current ??= ordinaryAdapter(f.client, f.scope, f.capabilities.data, { reason, displayName: name, description, upload, attachment }, value => { if (!controller.signal.aborted) setUpload(value); });
      const result = await transferFile(adapter.current, file, controller.signal, setProgress);
      controller.signal.throwIfAborted(); await f.changed(); controller.signal.throwIfAborted(); done(result);
    } catch (error) {
      if (!controller.signal.aborted) setError(error instanceof Error ? error : Error('传输失败，请重试。'));
      else setProgress(value => ({ ...value, phase: 'paused' }));
    } finally { setBusy(false); }
  };
  return <form className="space-y-5" onSubmit={event => { event.preventDefault(); void submit(); }}>
    <UploadPicker label={upload ? '重新选择原文件以续传' : '选择普通附件'} accept=".png,.jpg,.jpeg,.pdf,.txt,.csv,.docx,.xlsx,.pptx" hint="支持图片、PDF、文本和 Office 文档，单文件最多 100 MiB。" disabled={busy || !resumable} onChange={value => { setFile(value); if (!upload) adapter.current = null; }} />
    {!attachment && <><div className="space-y-2"><Label htmlFor="file-name">显示名称</Label><Input id="file-name" value={name} placeholder={file?.name ?? '留空使用文件名'} maxLength={200} disabled={busy || !!upload} onChange={event => { setName(event.target.value); adapter.current = null; }} /></div><div className="space-y-2"><Label htmlFor="file-description">说明 <span className="font-normal text-muted-foreground">（选填）</span></Label><Textarea id="file-description" rows={3} value={description} maxLength={1000} disabled={busy || !!upload} onChange={event => { setDescription(event.target.value); adapter.current = null; }} /></div></>}
    <div className="space-y-2"><Label htmlFor="file-reason">操作原因</Label><Input id="file-reason" value={reason} required maxLength={500} disabled={busy || locked} onChange={event => setReason(event.target.value)} /></div>
    {upload && <p className="text-xs leading-5 text-muted-foreground">离开后可在「上传与待处理」中续传。提交检查后，后台会继续处理。</p>}
    {(busy || locked) && <UploadProgress phase={phases[progress.phase]} bytes={progress.bytes} total={file?.size ?? progress.total} busy={busy} onPause={() => abort.current?.abort()} />}
    {error && <FileState error>{error.message} 可用同一文件重试，已确认的分片会跳过。</FileState>}
    {!resumable && !busy && <FileState>此上传已不可续传，请返回概览核对最新状态。</FileState>}
    <Button type="submit" disabled={busy || !file || !reason.trim() || !f.capabilities.data?.can_manage || !resumable}>{upload ? '继续传输并提交检查' : attachment ? '上传替换文件' : '上传并提交检查'}</Button>
  </form>;
}
