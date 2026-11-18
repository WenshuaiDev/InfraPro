import { useId, type ReactNode } from 'react';
import { ApiError } from '@infrapro/api-client';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
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
