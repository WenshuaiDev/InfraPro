import { useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import type { MonitoringImportBatch as Batch } from '@infrapro/api-client';
import { ShieldCheck, TriangleAlert } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useMonitoring } from './context';
import { useWriteRequest } from './write-request';
import { count, meta } from './format';
import { CursorButtons, Field, MonitoringError } from './common';
import { ImportNotice } from './import-overview';

export function ImportReport({ batch, resolve, done, onPendingChange }: { batch: Batch; resolve: boolean; done: (batch: Batch) => void; onPendingChange: (value: boolean) => void }) {
  const m = useMonitoring(), stable = useWriteRequest();
  const [checked, setChecked] = useState(false), [reason, setReason] = useState('');
  const issues = useCursorList({ queryKey: [...m.prefix, 'issues', batch.id, batch.revision], load: (cursor, signal) => m.client.monitoringIssues(m.scope, batch.id, cursor, signal), enabled: m.available && !!batch.counts && !batch.scope_clipped });
  const mutation = useMutation({ mutationFn: () => m.client.monitoringResolve(m.scope, batch.id, stable({ ...meta(batch.revision, reason), preview_hash: batch.validation_hash ?? '', confirm_same_observation: true })), onSuccess: done });
  useEffect(() => { onPendingChange(mutation.isPending); return () => onPendingChange(false); }, [onPendingChange, mutation.isPending]);
  if (batch.scope_clipped) return <p className="text-sm text-muted-foreground">完整校验报告不在当前授权范围内。</p>;
  const c = batch.counts;
  if (!c) return <div className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-lg border border-dashed"><ShieldCheck aria-hidden className="size-8 text-muted-foreground/60" /><p className="text-sm">完成全量校验后，可在这里查看问题。</p></div>;
  return <section className="space-y-5" aria-label="问题定位与重复核对"><ImportNotice batch={batch} /><div className="flex flex-wrap gap-2">{[['错误', c.errors], ['可疑', c.suspect], ['同观测重复', c.duplicates], ['已忽略', c.ignored]].map(([label, value]) => <Badge key={label} variant="outline" className="gap-2 rounded-md px-2.5 py-1 font-normal">{label}<span className="tabular-nums">{count(value)}</span></Badge>)}</div><MonitoringError error={issues.error} retry={() => void issues.refetch()} />{issues.isPending && <p role="status" className="py-8 text-center text-sm">正在读取校验报告…</p>}{!issues.error && <><div className="space-y-3">{issues.items.map((issue, i) => <article key={`${issue.line}:${issue.column}:${i}`} className="rounded-lg border p-4"><div className="flex items-center gap-2"><TriangleAlert aria-hidden className="size-4 shrink-0 text-amber-700 dark:text-amber-400" /><span className="break-all text-xs font-medium">第 {issue.line} 行{issue.column ? ` · ${issue.column} 列` : ''}</span></div><p className="mt-2 text-sm leading-6">{issue.message}</p>{issue.observed_at && <p className="mt-2 break-all text-xs text-muted-foreground">观测时间：{issue.observed_at}</p>}{issue.channel_id && <p className="mt-1 break-all text-xs text-muted-foreground">通道：{issue.channel_id}</p>}{issue.target_observation_id && <p className="mt-1 break-all text-xs text-muted-foreground">既有观测：{issue.target_observation_id}</p>}<p className="mt-2 break-all font-mono text-[10px] text-muted-foreground">{issue.code}</p></article>)}</div>{!issues.isPending && !issues.items.length && <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">当前报告没有需要处理的问题。</p>}<CursorButtons previous={issues.page ? () => issues.go(issues.page - 1) : undefined} next={issues.hasNext ? () => void issues.next() : undefined} busy={issues.isFetching} caption={`问题第 ${issues.page + 1} 页`} /></>}{resolve && <form className="space-y-4 rounded-lg border bg-muted/30 p-4" onSubmit={e => { e.preventDefault(); if (resolve && checked && reason.trim() && !mutation.isPending) mutation.mutate(); }}><h3 className="text-sm font-semibold">核对同观测重复</h3><p className="text-xs leading-6">此次固定核对集合共 {count(c.duplicates)} 条同观测重复。确认后忽略这些新候选，保留本次来源与原观测关联，不会新增观测或恢复既有撤销。</p><fieldset disabled={mutation.isPending} className="space-y-3"><label className="flex min-h-11 items-start gap-2 text-sm"><input type="checkbox" className="mt-1 size-4 shrink-0 accent-primary" checked={checked} onChange={e => setChecked(e.target.checked)} /><span>已核对完整集合，确认全部 {count(c.duplicates)} 条为同一观测，允许忽略。</span></label><Field label="核对理由" value={reason} onChange={setReason} required maxLength={2000} /><Button type="submit" disabled={!checked || !reason.trim() || !!issues.error}>确认同观测并忽略重复</Button></fieldset><MonitoringError error={mutation.error} /></form>}</section>;
}
