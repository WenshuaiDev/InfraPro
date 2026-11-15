import { useEffect, useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import type { MonitoringImportBatch } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Field, MonitoringError } from './common';
import { useMonitoring } from './context';
import { transferFile } from '../files/upload-controller';
import { monitoringAdapter } from '../files/adapters/monitoring';
import { UploadPicker, UploadProgress } from '@infrapro/ui/patterns/files';
import { phases } from '../files/state';

export function UploadForm({ initial, done }: { initial?: MonitoringImportBatch; done: (batch: MonitoringImportBatch) => void }) {
  const { client, scope, paths, available } = useMonitoring();
  const [file, setFile] = useState<File | null>(null), [reason, setReason] = useState(''), [batch, setBatch] = useState(initial), [error, setError] = useState<Error | null>(null), [busy, setBusy] = useState(false), [phase, setPhase] = useState('等待文件'), [bytes, setBytes] = useState(0);
  const controller = useRef<AbortController | null>(null), adapter = useRef<ReturnType<typeof monitoringAdapter> | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const submit = async () => {
    if (!file || busy) return;
    const signal = new AbortController(); controller.current = signal; setError(null); setBusy(true);
    try {
      adapter.current ??= monitoringAdapter(client, scope, reason, batch, setBatch);
      const result = await transferFile(adapter.current, file, signal.signal, p => {setPhase(phases[p.phase]);setBytes(p.bytes);}); signal.signal.throwIfAborted();setBatch(result);done(result);
    } catch (e) { setError(e instanceof Error ? e : new Error('上传失败。')); if (e instanceof Error && e.name === 'AbortError') setPhase('上传已暂停，已确认分片保留'); }
    finally { setBusy(false); }
  };
  return <form className="space-y-4" onSubmit={e => { e.preventDefault(); void submit(); }}><UploadPicker label={batch?'重新选择原文件以续传':'历史监测文件'} accept=".csv,.xlsx" disabled={busy} hint="CSV ≤ 32 GiB；XLSX ≤ 512 MiB，分片大小 32 MiB。" onChange={value=>{setFile(value);if(!batch)adapter.current=null;}}/><Field label="来源说明 / 导入原因" value={reason} onChange={setReason} required disabled={busy} maxLength={2000} />{batch && <p className="break-all text-sm">已建立批次：<Link to={paths.batch(batch.id)} className="underline">{batch.name}</Link>。离开页面会暂停上传，可在批次详情继续。</p>}<UploadProgress phase={phase} bytes={bytes} total={file?.size??Number(batch?.size_bytes??0)} busy={busy} onPause={()=>controller.current?.abort()}/><MonitoringError error={error} /><div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy || !available || !file || !reason.trim()}>{batch ? '继续上传并封存' : '上传文件'}</Button>{busy && <Button type="button" variant="outline" onClick={() => controller.current?.abort()}>暂停上传</Button>}</div></form>;
}
