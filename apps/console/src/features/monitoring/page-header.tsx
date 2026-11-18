import { Link } from '@tanstack/react-router';
import { ListChecks } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { workspaceTaskFilter } from '../tasks/search';
import type { ReactNode } from 'react';
import { useMonitoring } from './context';

export function MonitoringPageHeader({ title, description, actions, workspaceTasks }: { title: string; description: string; actions?: ReactNode; workspaceTasks?: boolean }) {
  const { archived, scope } = useMonitoring();
  return <header className="min-w-0 space-y-3">
    <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
      <div className="min-w-0 space-y-1"><h1 className="break-words text-2xl font-bold tracking-tight">{title}</h1><p className="max-w-prose text-sm leading-6 text-muted-foreground">{description}</p></div>
      {(actions || workspaceTasks) && <div className="flex min-w-0 flex-wrap items-center gap-2">{workspaceTasks && <Button asChild variant="outline" className="h-9"><Link to="/tasks" search={workspaceTaskFilter(scope)}><ListChecks aria-hidden />查看本空间任务</Link></Button>}{actions}</div>}
    </div>
    {archived && <p role="status" className="rounded-lg border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">项目已归档 · 可读取和导出获准数据</p>}
  </header>;
}
