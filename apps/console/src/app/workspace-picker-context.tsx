import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, useLocation, useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { WorkspaceSummary } from '@infrapro/api-client';
import { relativePath, safeReturn, workspaceScope } from '../config/navigation';
import { Button } from '@infrapro/ui/components/button';

export type WorkspaceFilter = 'all' | 'tenants' | 'projects';
type WorkspacePickerController = {
  state: { open: boolean; filter: WorkspaceFilter };
  open: (filter?: WorkspaceFilter) => void;
  close: () => void;
  summary?: WorkspaceSummary;
  revision?: string;
  pending: boolean;
  error: Error | null;
  refresh: () => void;
};
const PickerContext = createContext<WorkspacePickerController | null>(null);

export function WorkspacePickerProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<WorkspacePickerController['state']>({ open: false, filter: 'all' });
  const path = useLocation({ select: (location) => location.pathname });
  const { account, client, authorization_revision } = useIdentity();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const summary = useQuery({ queryKey: ['account', account?.id, 'workspaces', 'summary'], queryFn: ({ signal }) => client.myWorkspaces({ limit: 1 }, signal), staleTime: 0, retry: false, refetchInterval: 30_000, refetchOnWindowFocus: 'always' });
  const intent = useRef(0);
  useEffect(() => () => { intent.current++; }, [path]);
  const close = useCallback(() => { intent.current++; setState({ open: false, filter: 'all' }); }, []);
  const { refetch } = summary;
  const open = useCallback((filter: WorkspaceFilter = 'all') => {
    const request = ++intent.current;
    void refetch().then((result) => {
      if (request !== intent.current) return;
      if (result.error) { setState({ open: true, filter }); return; }
      const entry = result.data?.summary.single;
      if (entry) { close(); void navigate({ to: safeReturn(entry.default_path) }); }
      else setState({ open: result.data?.summary.mode === 'multiple', filter });
    });
  }, [refetch, close, navigate]);
  // Reset before descendant effects consume a new home intent; search cleanup keeps it open.
  const [previousPath, setPreviousPath] = useState(path);
  if (previousPath !== path) {
    setPreviousPath(path);
    setState({ open: false, filter: 'all' });
  }
  useEffect(() => {
    const refresh = () => {
      void cache.invalidateQueries({ queryKey: ['account', account?.id, 'workspaces'] });
      void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-tenants'] });
      void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-projects'] });
    };
    refresh();
    const tenant = client.onTenantRestriction(refresh);
    const project = client.onProjectRestriction(refresh);
    return () => { tenant(); project(); };
  }, [account?.id, authorization_revision, cache, client]);
  const mode = summary.error ? undefined : summary.data?.summary.mode;
  // A permission refresh can remove the need for a picker without changing a valid page.
  if (state.open && !summary.isFetching && mode && mode !== 'multiple') setState({ open: false, filter: 'all' });
  const refresh = useCallback(() => { if (state.open) open(state.filter); else void refetch(); }, [state.open, state.filter, open, refetch]);
  // A fresh entry summary may arrive before the identity refresh it triggered.
  // Use the latest known revision so an open picker never reuses older grants.
  const entryRevision = summary.data?.authorization_revision;
  const revision = authorization_revision && (!entryRevision || BigInt(authorization_revision) > BigInt(entryRevision)) ? authorization_revision : entryRevision;
  const value = useMemo(() => ({ state, open, close, summary: summary.error ? undefined : summary.data?.summary, revision, pending: summary.isPending || !summary.isFetchedAfterMount, error: summary.error, refresh }), [state, open, close, summary.data, summary.error, summary.isPending, summary.isFetchedAfterMount, revision, refresh]);
  return <PickerContext value={value}>{children}</PickerContext>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useWorkspacePicker() {
  const value = useContext(PickerContext);
  if (!value) throw new Error('WorkspacePickerProvider is required');
  return value;
}

export function WorkspacePickerButton({ filter = 'all', children }: { filter?: WorkspaceFilter; children: ReactNode }) {
  const picker = useContext(PickerContext);
  const path = useLocation({ select: (location) => location.pathname });
  const scope = workspaceScope(relativePath(path));
  const single = picker?.summary?.single;
  if (single && single.kind === scope.kind && (single.kind === 'platform' || single.id === (single.kind === 'project' ? scope.projectId : scope.tenantId))) return null;
  if (picker?.summary?.mode === 'none') return null;
  if (picker?.summary?.mode === 'single') return <Button variant="outline" onClick={() => picker.open()}>进入{picker.summary.single?.name}</Button>;
  return picker ? <Button variant="outline" onClick={() => picker.open(filter)}>{children}</Button> : <Button variant="outline" asChild><Link to={filter === 'projects' ? '/projects' : '/tenants'}>{children}</Link></Button>;
}
