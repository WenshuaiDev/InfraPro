import { useWriteRequest } from './write-request';
import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useMutation } from '@tanstack/react-query';
import type { MonitoringQuery } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Choice, Field, MonitoringError } from './common';
import { count, meta, rangeLabel } from './format';
import { useMonitoring } from './context';

export function ExportForm({ query, channelCount, estimate, kind = 'details' }: { query: MonitoringQuery; channelCount: number; estimate?: string; kind?: 'details' | 'statistics' }) {
  const { client, scope, paths, available } = useMonitoring();
  const [format, setFormat] = useState('parquet'), [csvMode, setCSVMode] = useState('spreadsheet_safe'), [reason, setReason] = useState(''), [allHistory, setAllHistory] = useState(false);
  const stableRequest=useWriteRequest();
  const result = useMutation({ mutationFn: () => client.monitoringExport(scope, stableRequest({ ...meta('0', reason), query: allHistory ? { ...query, range: undefined, view_range: undefined, snapshot_id: undefined, all_history: true, cursor: undefined } : { ...query, cursor: undefined }, kind, format: format as 'csv' | 'parquet', ...(format === 'csv' ? { csv_mode: csvMode as 'exact' | 'spreadsheet_safe' } : {}) })) });
  if (result.data) return <div className="space-y-3"><p role="status">导出任务已创建，可以离开本页。全部分卷校验完成后可下载，结果保留 7 天。</p><Button asChild><Link to={paths.jobs}>查看任务与下载</Link></Button></div>;
  return <form className="space-y-4" onSubmit={e => { e.preventDefault(); result.mutate(); }}><div className="space-y-1 rounded-lg bg-muted/50 p-3 text-sm"><p>{kind === 'statistics' ? '基础统计表' : '精确原始明细'} · {channelCount} 个通道</p><p className="break-words">{allHistory ? '获准的全部历史' : (query.view_range ?? query.range) ? rangeLabel((query.view_range ?? query.range)!) : '获准的全部历史'}</p><p>时区 {query.timezone ?? 'Asia/Shanghai'} · 质量 {(query.qualities ?? ['good', 'unmarked']).join('、')}</p><p>{query.status === 'history' ? '包含撤销历史' : '当前在用观测'}</p>{!allHistory && <p>当前筛选样本 {count(estimate)} · 固定快照 {query.snapshot_id?.slice(0, 8) ?? '创建时固定'}</p>}</div><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={allHistory} onChange={e => setAllHistory(e.target.checked)} />改为获准的全部历史（创建新快照）</label><Choice label="导出格式" value={format} onChange={setFormat} options={[{ value: 'parquet', label: 'Parquet 2.6 · 保留原始类型和纳秒精度' }, { value: 'csv', label: 'UTF-8 CSV' }]} />{format === 'csv' && <><Choice label="CSV 模式" value={csvMode} onChange={setCSVMode} options={[{ value: 'spreadsheet_safe', label: '表格阅览 · 对公式样式文本做可逆转义' }, { value: 'exact', label: '精确交换 · 保留规范化原文' }]} /><p className="text-xs text-muted-foreground">精确交换模式需按文本或明确类型导入，电子表格自动识别可能损失大整数、纳秒和小数精度。阅览模式的转义规则随清单下载。</p></>}<Field label="导出用途 / 原因" value={reason} onChange={setReason} required maxLength={2000} /><MonitoringError error={result.error} /><Button type="submit" disabled={!available || result.isPending || !reason.trim()}>{result.isPending ? '正在创建…' : '创建后台导出任务'}</Button><p className="text-xs text-muted-foreground">后续追加不改变此次结果；相关撤销或授权变化会使结果失效。每次下载重新核验权限。</p></form>;
}
