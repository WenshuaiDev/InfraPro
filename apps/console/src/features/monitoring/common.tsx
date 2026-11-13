import { useId, type ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { ApiError, type PermissionCode } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { useMonitoring } from './context';

export function MonitoringTabs({ current }: { current: 'data' | 'imports' | 'jobs' }) {
  const m = useMonitoring();
  return <nav aria-label="监测数据导航" className="flex flex-wrap gap-2 border-b pb-3">{[[m.canRead, 'data', '监测浏览', m.paths.root], [m.canImportRead, 'imports', '导入与批次', m.paths.imports], [m.canRead||m.canImportRead, 'jobs', '任务中心', m.paths.jobs], [m.can(`${m.scope.kind}.derived.channels.read` as PermissionCode),'derived','派生通道',`${m.paths.root}/derived`], [m.can(`${m.scope.kind}.derived.analysis.read` as PermissionCode)||m.can(`${m.scope.kind}.derived.analysis.results.read` as PermissionCode),'analysis','历史分析',`${m.paths.root}/analysis`]].map(([show, key, label, to]) => show && <Button key={String(key)} variant={current === key ? 'secondary' : 'ghost'} asChild><Link to={String(to)} aria-current={current === key ? 'page' : undefined}>{label}</Link></Button>)}{m.archived && <span className="self-center text-sm text-muted-foreground">项目已归档 · 可读取和导出获准数据</span>}</nav>;
}
export function Field({ label, value, onChange, hint, children, ...props }: { label: string; value?: string; onChange?: (value: string) => void; hint?: string; children?: ReactNode; disabled?: boolean; required?: boolean; type?: string; placeholder?: string; maxLength?: number; step?: number | string }) {
  const id = useId();
  return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label>{children ?? <Input id={id} value={value ?? ''} onChange={e => onChange?.(e.target.value)} aria-describedby={hint ? `${id}-hint` : undefined} {...props} />}{hint && <p id={`${id}-hint`} className="text-xs text-muted-foreground">{hint}</p>}</div>;
}
export function Choice({ label, value, onChange, options, disabled }: { label: string; value: string; onChange: (value: string) => void; options: { value: string; label: string }[]; disabled?: boolean }) {
  const id = useId();
  return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label><Select value={value} onValueChange={onChange} disabled={disabled}><SelectTrigger id={id} className="min-h-11 w-full"><SelectValue /></SelectTrigger><SelectContent>{options.map(o => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent></Select></div>;
}
export function MonitoringError({ error, retry }: { error: Error | null; retry?: () => void }) {
  if (!error || error.name === 'AbortError') return null;
  return <ErrorState title={error instanceof ApiError && ['snapshot_stale', 'selection_stale'].includes(error.code ?? '') ? '数据范围已变化，请重新查询' : '未能完成请求'} message={error.message} onRetry={retry} />;
}
export function CursorButtons({ previous, next, busy, caption }: { previous?: () => void; next?: () => void; busy?: boolean; caption?: string }) {
  return <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-sm text-muted-foreground" role="status">{caption}</span><div className="flex gap-2"><Button variant="outline" disabled={busy || !previous} onClick={previous}>上一页</Button><Button variant="outline" disabled={busy || !next} onClick={next}>下一页</Button></div></div>;
}
