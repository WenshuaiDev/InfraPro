import type { ReactNode } from 'react';
import type { MonitoringImportBatch as Batch } from '@infrapro/api-client';
import { Check, Copy, Download, FileSpreadsheet, Info } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@infrapro/ui/components/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { cn } from '@infrapro/ui/lib/utils';
import { DomainFiles } from '../files/domain-files';
import { TaskLink } from '../tasks/common';
import { useMonitoring } from './context';
import { count } from './format';
import { importDate, importFileSize } from './import-search';

export function ImportSectionTitle({ children, action }: { children: ReactNode; action?: ReactNode }) { return <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">{children}</h3>{action}</div>; }
export function ImportMeta({ label, children }: { label: string; children: ReactNode }) { return <div className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1.5 break-words text-sm">{children}</dd></div>; }
export function ImportNotice({ batch: b }: { batch: Batch }) {
  const text: Record<Batch['state'], string> = {
    uploading: '文件尚未封存。重新选择同一个源文件，可继续上传剩余分片。',
    received: '文件已封存。确认列与通道映射后，开始全量校验。',
    validating: '正在全量解析、质量检查和重复核对。可以离开详情，后台会继续处理。',
    needs_resolution: b.counts?.errors !== '0' ? '存在错误，整批不能发布。请核对错误位置与映射；若源文件有误，请修正后创建新批次。' : '发现同观测重复。核对完整集合并确认忽略后，才可继续发布。',
    ready: '全量校验已完成。确认采用范围后，发布合格批次。',
    preparing: '正在写入并逐副本核验。全部完成后，本批次将统一生效。',
    published: '此批次已经统一生效，可在监测浏览中核对数量与原值。',
    completed_noop: '完成，无新增数据。此次重复来源与核对记录已保留，不会重复计数。',
    failed: '处理未完成，请查看校验问题及关联任务，修正条件后重试。',
    cancelled: '此批次已取消，已保存的源文件和处理证据仍然保留。',
    revoked: '此批次的采用关系已撤销，原文件和原观测保留。重新采用须建立关联新批次并重新校验。',
  };
  return <div role="status" className="flex items-start gap-3 rounded-lg border bg-muted/30 p-4"><Info aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><div className="space-y-2 text-xs leading-6"><p>{b.scope_clipped ? '此处仅展示当前授权范围内的批次信息。' : text[b.state]}</p>{b.error_code && <p className="break-all text-destructive">处理结果：{b.error_code}</p>}</div></div>;
}
export function ImportOverview({ batch: b, source, showIssues }: { batch: Batch; source: boolean; showIssues: () => void }) {
  const m = useMonitoring(), c = b.counts;
  const completed = ['published', 'completed_noop', 'revoked'].includes(b.state) ? 4 : ['ready', 'preparing'].includes(b.state) ? 3 : b.mapping ? 2 : b.state === 'uploading' ? 0 : 1;
  return <div className="space-y-6"><ImportNotice batch={b} />{b.scope_clipped ? <section className="rounded-lg border p-4 text-sm leading-6"><p>可见采用 {count(b.visible_adopted_count)} 条。完整文件属性、总数和未授权通道不在当前范围内。</p>{b.visible_coverage?.map(cov => <p key={cov.channel_id} className="mt-3 break-all text-xs">通道 {cov.channel_id} · {count(cov.count)} 条<br />{cov.earliest} 至 {cov.latest}</p>)}</section> : <>
    <section><ImportSectionTitle>导入进度</ImportSectionTitle><ol className="mt-4 grid grid-cols-4 gap-2">{['文件封存', '配置映射', '全量校验', '统一生效'].map((label, i) => <li key={label} className="space-y-2"><div className="flex items-center gap-2"><span className={cn('flex size-6 shrink-0 items-center justify-center rounded-full text-[11px]', i < completed ? 'bg-primary text-primary-foreground' : 'border text-muted-foreground')}>{i < completed ? <Check aria-hidden className="size-3.5" /> : i + 1}</span>{i < 3 && <span className={cn('h-px w-full', i < completed - 1 ? 'bg-primary' : 'bg-border')} />}</div><span className="text-xs text-muted-foreground">{label}</span></li>)}</ol></section>
    <section><ImportSectionTitle action={<Button variant="link" className="h-7 p-0 text-xs" onClick={showIssues}>查看校验报告 →</Button>}>校验与采用</ImportSectionTitle><div className="mt-3 grid grid-cols-3 overflow-hidden rounded-lg border sm:grid-cols-5">{([['候选观测', c?.candidates], [b.state === 'published' ? '实际采用' : b.state === 'revoked' ? '撤销前采用' : '拟采用', c?.proposed], ['错误', c?.errors], ['可疑', c?.suspect], ['已忽略', c?.ignored]] as const).map(([label, value]) => <div key={label} className="border-r p-3 last:border-0"><p className="text-[11px] text-muted-foreground">{label}</p><p className={cn('mt-2 text-xl font-semibold tabular-nums', label === '错误' && value && value !== '0' && 'text-destructive', label === '可疑' && value && value !== '0' && 'text-amber-700 dark:text-amber-400')}>{value === undefined ? '—' : count(value)}</p></div>)}</div>{c && <details className="mt-3 text-xs"><summary className="cursor-pointer text-muted-foreground">更多校验计数</summary><dl className="mt-3 grid grid-cols-2 gap-3">{([['解析数据行', c.parsed_rows], ['全空行', c.blank_rows], ['缺测单元格', c.missing_cells], ['同观测重复', c.duplicates]] as const).map(([label, value]) => <div key={label}><dt className="text-muted-foreground">{label}</dt><dd className="mt-1 tabular-nums">{count(value)}</dd></div>)}</dl></details>}<p className="mt-3 text-xs leading-5 text-muted-foreground">错误会阻止整批生效；可疑值保留原值，默认不纳入数学统计。</p></section>
    <section><ImportSectionTitle>源文件</ImportSectionTitle><div className="mt-3">{b.source_file_id ? <DomainFiles files={[{ ref: { kind: 'monitoring.import.source', scope: m.scope, batchId: b.id }, display_name: b.name, media_type: b.name.toLowerCase().endsWith('.xlsx') ? 'XLSX' : 'CSV', size_bytes: b.size_bytes ?? '0', state: source ? 'available' : 'restricted', allowed_actions: source ? ['download'] : [] }]} actions={() => <Button variant="outline" size="sm" asChild><a href={m.client.monitoringSourceURL(m.scope, b.source_file_id!)} download><Download aria-hidden />下载完整原文件</a></Button>} /> : <div className="flex items-center gap-3 rounded-lg border p-4"><FileSpreadsheet aria-hidden className="size-5 text-muted-foreground" /><div className="min-w-0"><p className="truncate text-sm">{b.name}</p><p className="mt-1 text-xs text-muted-foreground">{importFileSize(b.size_bytes)} · 已上传 {b.parts.length} 个分片，等待封存</p></div></div>}</div></section>
  </>}<section><ImportSectionTitle>批次信息</ImportSectionTitle><dl className="mt-3 grid gap-x-6 gap-y-4 sm:grid-cols-2"><ImportMeta label="批次编号"><span className="flex items-start gap-1"><span className="break-all font-mono text-[11px]">{b.id}</span><Button variant="ghost" size="icon" className="size-7 shrink-0" aria-label="复制完整批次编号" onClick={() => { void navigator.clipboard.writeText(b.id).then(() => toast.success('已复制批次编号'), () => toast.error('复制失败，请手动选择编号')); }}><Copy aria-hidden className="size-3.5" /></Button></span></ImportMeta><ImportMeta label="创建时间">{importDate(b.created_at, true)}</ImportMeta>{!b.scope_clipped && <><ImportMeta label="映射版本">{b.mapping_revision === '0' ? '尚未配置' : `版本 ${b.mapping_revision}`}</ImportMeta><ImportMeta label="数据布局">{b.mapping ? { single: '单通道', long: '多通道长表', wide: '多通道宽表' }[b.mapping.mode] : '尚未配置'}</ImportMeta></>}</dl></section>{b.job_id && <TaskLink source={b.task_ref ?? { source_module: 'monitoring', source_kind: 'job', source_root_id: b.job_id }} />}</div>;
}

export function ImportMappingSummary({ batch }: { batch: Batch }) {
  const m = batch.mapping;
  if (!m) return <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">尚未配置映射，请先明确时间、数值列及目标通道。</p>;
  return <div className="space-y-5"><dl className="grid grid-cols-2 gap-4 rounded-lg border p-4"><ImportMeta label="文件布局">{{ single: '单通道', long: '多通道长表', wide: '多通道宽表' }[m.mode]}</ImportMeta><ImportMeta label="工作表">{m.sheet || 'CSV 文件'}</ImportMeta><ImportMeta label="时间列">{m.time_column}</ImportMeta><ImportMeta label="时间归属">{m.timezone}</ImportMeta><ImportMeta label="表头行">第 {m.header_row} 行</ImportMeta><ImportMeta label="宽表空值处理">{m.empty_wide === 'no_observation' ? '不产生观测' : '作为错误阻断'}</ImportMeta></dl><div className="overflow-hidden rounded-lg border"><Table aria-label="批次通道映射"><TableHeader><TableRow><TableHead>源列 / 别名</TableHead><TableHead>目标通道</TableHead><TableHead>单位</TableHead></TableRow></TableHeader><TableBody>{m.channels.map((ch, i) => <TableRow key={i}><TableCell className="whitespace-normal text-xs">{ch.alias || ch.value_column || m.value_column || '数值'}</TableCell><TableCell className="max-w-52 whitespace-normal break-all font-mono text-[11px]">{ch.channel_id}</TableCell><TableCell className="text-xs">{ch.unit || '—'}</TableCell></TableRow>)}</TableBody></Table></div></div>;
}
