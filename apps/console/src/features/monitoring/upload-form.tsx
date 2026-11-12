import { useEffect, useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import type { MonitoringImportBatch, MonitoringImportInput, MonitoringUploadComplete } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Field, MonitoringError } from './common';
import { count, meta } from './format';
import { useMonitoring } from './context';
import type { FileHashes, HashMessage } from './hash.worker';

function hashFile(file: File, signal: AbortSignal, progress: (bytes: number) => void): Promise<FileHashes> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(new URL('./hash.worker.ts', import.meta.url), { type: 'module' });
    const finish = () => { signal.removeEventListener('abort', abort); worker.terminate(); };
    const abort = () => { finish(); reject(new DOMException('上传已暂停', 'AbortError')); };
    signal.addEventListener('abort', abort, { once: true });
    worker.onmessage = (event: MessageEvent<HashMessage>) => { const m = event.data; if (m.kind === 'progress') progress(m.bytes); if (m.kind === 'done') { finish(); resolve(m.result); } if (m.kind === 'error') { finish(); reject(new Error(m.message)); } };
    worker.onerror = () => { finish(); reject(new Error('文件摘要工作线程失败，请重试。')); };
    if (signal.aborted) abort(); else worker.postMessage({ file });
  });
}
export function UploadForm({ initial, done }: { initial?: MonitoringImportBatch; done: (batch: MonitoringImportBatch) => void }) {
  const { client, scope, paths, available } = useMonitoring();
  const [file, setFile] = useState<File | null>(null), [reason, setReason] = useState(''), [batch, setBatch] = useState(initial), [error, setError] = useState<Error | null>(null), [busy, setBusy] = useState(false), [phase, setPhase] = useState('等待文件'), [bytes, setBytes] = useState(0);
  const controller = useRef<AbortController | null>(null), createInput = useRef<MonitoringImportInput | null>(null), completeInput = useRef<MonitoringUploadComplete | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const submit = async () => {
    if (!file || busy) return;
    const signal = new AbortController(); controller.current = signal; setError(null); setBusy(true);
    try {
      const ext = file.name.toLowerCase().split('.').pop(), maximum = ext === 'xlsx' ? 512 * 1024 ** 2 : 32 * 1024 ** 3;
      if (!['csv', 'xlsx'].includes(ext ?? '') || file.size === 0 || file.size > maximum) throw new Error('CSV 最大 32 GiB，XLSX 最大 512 MiB；不接受空文件。');
      if (batch && (batch.name !== file.name || batch.size_bytes !== String(file.size))) throw new Error('续传须重新选择同名、同大小的原文件；各分片还会核对摘要。');
      setPhase('建立上传会话'); setBytes(0);
      let current = batch ? await client.monitoringImport(scope, batch.id, signal.signal) : null;
      if (!current) { createInput.current ??= { ...meta('0', reason), name: file.name, size_bytes: String(file.size) }; current = await client.monitoringCreateImport(scope, createInput.current, signal.signal); setBatch(current); }
      if (current.state !== 'uploading') { if (current.state === 'received') { done(current); return; } throw new Error('此批次已结束上传，请刷新状态。'); }
      setPhase('读取本机文件并计算完整摘要');
      const hashes = await hashFile(file, signal.signal, setBytes);
      let uploaded = 0;
      for (const receipt of current.parts) { const part = hashes.parts[receipt.part - 1]; if (!part || part.sha256 !== receipt.sha256 || String(part.size) !== receipt.size_bytes) throw new Error(`第 ${receipt.part} 分片与已上传内容不一致；请使用原文件或新建批次。`); uploaded += part.size; }
      setPhase('上传并等待分片收据'); setBytes(uploaded);
      for (const part of hashes.parts) {
        if (current.parts.some(p => p.part === part.part)) continue;
        const start = (part.part - 1) * 32 * 1024 * 1024;
        await client.monitoringUploadPart(scope, current.id, current.revision, part.part, part.sha256, file.slice(start, start + part.size), signal.signal);
        uploaded += part.size; setBytes(uploaded);
      }
      setPhase('服务端封存并核验完整文件');
      completeInput.current ??= { ...meta(current.revision, reason), sha256: hashes.sha256 };
      const result = await client.monitoringCompleteUpload(scope, current.id, completeInput.current, signal.signal); setBatch(result); done(result);
    } catch (e) { setError(e instanceof Error ? e : new Error('上传失败。')); if (e instanceof Error && e.name === 'AbortError') setPhase('上传已暂停，已确认分片保留'); }
    finally { setBusy(false); }
  };
  return <form className="space-y-4" onSubmit={e => { e.preventDefault(); void submit(); }}><div className="space-y-2"><Label htmlFor="monitoring-file">{batch ? '重新选择原文件以续传' : '历史监测文件'}</Label><Input id="monitoring-file" type="file" accept=".csv,.xlsx" disabled={busy} onChange={e => { setFile(e.target.files?.[0] ?? null); if (!batch) { createInput.current = null; completeInput.current = null; } }} /><p className="text-xs text-muted-foreground">CSV ≤ 32 GiB；XLSX ≤ 512 MiB，每批选择一个工作表。分片大小 32 MiB，最多同时保留 3 个上传会话。</p></div><Field label="来源说明 / 导入原因" value={reason} onChange={setReason} required disabled={busy} maxLength={2000} />{batch && <p className="break-all text-sm">已建立批次：<Link to={paths.batch(batch.id)} className="underline">{batch.name}</Link>。离开页面会暂停上传，可在批次详情继续。</p>}<div className="rounded-lg bg-muted/50 p-3 text-sm" role="status"><p>{phase}</p><p className="mt-1 tabular-nums">{count(String(bytes))} / {count(file ? String(file.size) : batch?.size_bytes)} 字节</p>{busy && <progress className="mt-2 w-full" value={bytes} max={Math.max(1, file?.size ?? 1)} aria-label={`${phase}的已处理字节`} />}</div><MonitoringError error={error} /><div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy || !available || !file || !reason.trim()}>{batch ? '继续上传并封存' : '上传文件'}</Button>{busy && <Button type="button" variant="outline" onClick={() => controller.current?.abort()}>暂停上传</Button>}</div></form>;
}
