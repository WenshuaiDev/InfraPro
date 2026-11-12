import { useEffect, useRef, useState } from 'react';
import { createFileRoute, Navigate } from '@tanstack/react-router';
import { HomePage } from '../../features/account/home-page';
import { useWorkspacePicker } from '../../app/workspace-picker-context';
import { safeReturn } from '../../config/navigation';
import { ErrorState } from '@infrapro/ui/patterns/feedback';

type HomeSearch = { workspace?: 'tenants' | 'projects'; view?: 'account' };
export const Route = createFileRoute('/_authenticated/')({
  validateSearch: (search: Record<string, unknown>): HomeSearch => ({ workspace: search.workspace === 'tenants' || search.workspace === 'projects' ? search.workspace : undefined, view: search.view === 'account' ? 'account' : undefined }),
  component: function HomeRoute() {
    const { workspace, view } = Route.useSearch();
    const navigate = Route.useNavigate();
    const { open, summary, pending, error, refresh } = useWorkspacePicker();
    const consumed = useRef<string | undefined>(undefined);
    const [landing, setLanding] = useState<string | null>();
    if (landing === undefined && !pending && summary && !error) setLanding(workspace || view === 'account' ? null : summary.single?.default_path ?? null);
    useEffect(() => {
      if (!workspace) { consumed.current = undefined; return; }
      if (pending) return;
      if (consumed.current === workspace) return;
      consumed.current = workspace;
      open(workspace);
      void navigate({ to: '/', search: { view: 'account' }, replace: true });
    }, [workspace, pending, open, navigate]);
    if (!workspace && view !== 'account' && landing !== null) {
      if (pending) return <p role="status">正在核验工作空间…</p>;
      if (error) return <ErrorState title="工作空间核验失败" message="暂时无法确认可进入的工作空间，请重试。" onRetry={refresh} />;
      if (landing) return <Navigate to={safeReturn(landing)} replace />;
    }
    return <HomePage />;
  },
});
