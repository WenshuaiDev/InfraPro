import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link, useLocation } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';

export type WorkspaceFilter = 'all' | 'tenants' | 'projects';
type WorkspacePickerController = {
  state: { open: boolean; filter: WorkspaceFilter };
  open: (filter?: WorkspaceFilter) => void;
  close: () => void;
};
const PickerContext = createContext<WorkspacePickerController | null>(null);

export function WorkspacePickerProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<WorkspacePickerController['state']>({ open: false, filter: 'all' });
  const path = useLocation({ select: (location) => location.pathname });
  const { account, client, authorization_revision } = useIdentity();
  const cache = useQueryClient();
  const close = useCallback(() => setState({ open: false, filter: 'all' }), []);
  const open = useCallback((filter: WorkspaceFilter = 'all') => setState({ open: true, filter }), []);
  // Reset before descendant effects consume a new home intent; search cleanup keeps it open.
  const [previousPath, setPreviousPath] = useState(path);
  if (previousPath !== path) {
    setPreviousPath(path);
    setState({ open: false, filter: 'all' });
  }
  useEffect(() => {
    const refresh = () => {
      void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-tenants'] });
      void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-projects'] });
    };
    refresh();
    const tenant = client.onTenantRestriction(refresh);
    const project = client.onProjectRestriction(refresh);
    return () => { tenant(); project(); };
  }, [account?.id, authorization_revision, cache, client]);
  const value = useMemo(() => ({ state, open, close }), [state, open, close]);
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
  return picker ? <Button variant="outline" onClick={() => picker.open(filter)}>{children}</Button> : <Button variant="outline" asChild><Link to={filter === 'projects' ? '/projects' : '/tenants'}>{children}</Link></Button>;
}
