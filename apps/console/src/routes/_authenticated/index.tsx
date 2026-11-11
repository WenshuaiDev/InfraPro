import { useEffect, useRef } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { HomePage } from '../../features/account/home-page';
import { useWorkspacePicker } from '../../app/workspace-picker-context';

type HomeSearch = { workspace?: 'tenants' | 'projects' };
export const Route = createFileRoute('/_authenticated/')({
  validateSearch: (search: Record<string, unknown>): HomeSearch => ({ workspace: search.workspace === 'tenants' || search.workspace === 'projects' ? search.workspace : undefined }),
  component: function HomeRoute() {
    const { workspace } = Route.useSearch();
    const navigate = Route.useNavigate();
    const { open } = useWorkspacePicker();
    const consumed = useRef<string | undefined>(undefined);
    useEffect(() => {
      if (!workspace) { consumed.current = undefined; return; }
      if (consumed.current === workspace) return;
      consumed.current = workspace;
      open(workspace);
      void navigate({ to: '/', search: {}, replace: true });
    }, [workspace, open, navigate]);
    return <HomePage />;
  },
});
