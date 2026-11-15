import type { ReactNode } from 'react';
import { useMonitoring } from './context';

export function MonitoringPageHeader({ title, description, actions }: { title: string; description: string; actions?: ReactNode }) {
  const { archived } = useMonitoring();
  return <header className="min-w-0 space-y-3">
    <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
      <div className="min-w-0 space-y-1"><h1 className="break-words text-2xl font-bold tracking-tight">{title}</h1><p className="max-w-prose text-sm leading-6 text-muted-foreground">{description}</p></div>
      {actions && <div className="flex min-w-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
    {archived && <p role="status" className="rounded-lg border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">项目已归档 · 可读取和导出获准数据</p>}
  </header>;
}
