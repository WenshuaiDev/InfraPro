import type { ReactNode } from 'react';
import { cn } from '@infrapro/ui/lib/utils';

export function PageContent({ children, className }: { children: ReactNode; className?: string }) {
  return <main id="main" tabIndex={-1} className={cn('min-w-0 px-4 py-6 @7xl/content:mx-auto @7xl/content:w-full @7xl/content:max-w-7xl focus:outline-none', className)}>{children}</main>;
}

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: ReactNode }) {
  return <div className="mb-6 flex flex-wrap items-start justify-between gap-4"><div className="space-y-1"><h1 className="text-2xl font-bold tracking-tight">{title}</h1>{description && <p className="text-sm text-muted-foreground">{description}</p>}</div>{actions && <div className="flex items-center gap-2">{actions}</div>}</div>;
}
