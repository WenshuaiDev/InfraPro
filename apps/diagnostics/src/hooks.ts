import { useContext, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { DiagnosticsError, type Envelope } from '@infrapro/diagnostics-client';
import { SessionContext } from './session-context';
export function useSession() { const context = useContext(SessionContext); if (!context) throw new Error('Session provider missing'); return context; }
function subscribeVisibility(listener: () => void) { document.addEventListener('visibilitychange', listener); return () => document.removeEventListener('visibilitychange', listener); }
export function useVisible() { return useSyncExternalStore(subscribeVisibility, () => document.visibilityState === 'visible', () => true); }
export function useDiagnosticQuery<T>(path: string, params: Record<string, string | number | undefined> = {}, interval = 5000) {
  const { client, session } = useSession(); const visible = useVisible();
  const query = useQuery<Envelope<T>, Error>({ queryKey: ['diagnostics', session?.user.id, path, params], enabled: !!session, queryFn: ({ signal }) => client.get<T>(path, params, signal), retry: false, refetchInterval: (query) => visible ? query.state.error ? 30000 : interval : false, refetchIntervalInBackground: false, refetchOnWindowFocus: true, staleTime: interval, gcTime: 60000 });
  const removed = query.error instanceof DiagnosticsError && query.error.code === 'target_removed';
  return { ...query, data: removed ? undefined : query.data };
}
export function retrySnapshot(error: unknown) { return error instanceof DiagnosticsError && error.code === 'snapshot_expired'; }
