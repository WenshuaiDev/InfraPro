import { CheckCheck, CircleCheck, CircleDashed, CircleSlash, CircleX, Clock3, FileCheck2, LoaderCircle, RotateCcw, Upload, TriangleAlert, type LucideIcon } from 'lucide-react';
import type { MonitoringImportBatch } from '@infrapro/api-client';
import { cn } from '@infrapro/ui/lib/utils';
import { count, stateNames } from './format';

const states: Record<MonitoringImportBatch['state'], [LucideIcon, string]> = {
  uploading: [Upload, 'text-blue-700 dark:text-blue-400'], received: [CircleDashed, 'text-muted-foreground'],
  validating: [LoaderCircle, 'text-blue-700 dark:text-blue-400'], needs_resolution: [TriangleAlert, 'text-amber-700 dark:text-amber-400'],
  ready: [FileCheck2, 'text-blue-700 dark:text-blue-400'], preparing: [Clock3, 'text-blue-700 dark:text-blue-400'],
  published: [CircleCheck, 'text-emerald-700 dark:text-emerald-400'], completed_noop: [CheckCheck, 'text-muted-foreground'],
  failed: [CircleX, 'text-destructive'], cancelled: [CircleSlash, 'text-muted-foreground'], revoked: [RotateCcw, 'text-muted-foreground'],
};
export function ImportStatus({ batch }: { batch: MonitoringImportBatch }) {
  const [Icon, color] = states[batch.state];
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-xs"><Icon aria-hidden className={cn('size-4 shrink-0', color)} />{batch.state === 'completed_noop' ? '完成，无新增' : stateNames[batch.state]}</span>;
}
export function ImportQuality({ batch }: { batch: MonitoringImportBatch }) {
  if (batch.scope_clipped) return <span className="text-xs text-muted-foreground">部分可见</span>;
  if (!batch.counts || ['uploading', 'received', 'validating'].includes(batch.state)) return <span className="text-xs text-muted-foreground">{batch.state === 'validating' ? '校验中' : '尚未校验'}</span>;
  const c = batch.counts;
  return <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs tabular-nums">
    {c.errors !== '0' && <span className="text-destructive">{count(c.errors)} 错误</span>}
    {c.suspect !== '0' && <span className="text-amber-700 dark:text-amber-400">{count(c.suspect)} 可疑</span>}
    {c.duplicates !== '0' && <span className="text-muted-foreground">{count(c.duplicates)} 重复</span>}
    {c.errors === '0' && c.suspect === '0' && c.duplicates === '0' && <span className="text-muted-foreground">无异常</span>}
  </span>;
}
