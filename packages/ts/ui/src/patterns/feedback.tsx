import type { ReactNode } from 'react';
import { CircleAlert, Inbox, LoaderCircle } from 'lucide-react';
import { Button } from '../components/button';
import { cn } from '../lib/utils';

export function FormNotice({ error = false, children }: { error?: boolean; children?: ReactNode }) {
  if (!children) return null;
  return <p role={error ? 'alert' : 'status'} className={cn('rounded-md border px-3 py-2 text-sm', error ? 'border-destructive/20 bg-destructive/5 text-destructive' : 'bg-muted/50 text-foreground')}>{children}</p>;
}

export function LoadingState({ label = '正在加载…' }: { label?: string }) {
  return <div role="status" className="flex min-h-32 items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" aria-hidden="true" />{label}</div>;
}

export function EmptyState({ title = '暂无数据', description, children }: { title?: string; description?: string; children?: ReactNode }) {
  return <div className="flex min-h-40 flex-col items-center justify-center gap-2 p-6 text-center"><Inbox className="mb-1 size-8 text-muted-foreground/60" aria-hidden="true" /><p className="text-sm font-medium">{title}</p>{description && <p className="max-w-sm text-sm text-muted-foreground">{description}</p>}{children}</div>;
}

export function ErrorState({ title = '加载失败', message, onRetry, retryLabel = '重新加载' }: { title?: string; message: string; onRetry?: () => void; retryLabel?: string }) {
  return <div className="flex min-h-40 flex-col items-center justify-center gap-3 p-6 text-center" role="alert"><CircleAlert className="size-7 text-destructive" aria-hidden="true" /><h2 className="font-semibold">{title}</h2><p className="max-w-md text-sm text-muted-foreground">{message}</p>{onRetry && <Button type="button" variant="outline" onClick={onRetry}>{retryLabel}</Button>}</div>;
}
