import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useContext, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { DiagnosticsError, type Page, type Meta, type Envelope } from '@infrapro/diagnostics-client';
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

export function useDiagnosticPages<T>(path: string, params: Record<string, string | undefined>, generation: number) {
  const { client, session } = useSession();
  const visible = useVisible();
  return usePagedQuery<Page<T> & { meta: Meta }>({
    queryKey: ['diagnostics', session?.user.id, path, params, generation], enabled: !!session,
    initialPageSize: 50, retry: false, refetchInterval: visible ? 10_000 : false, gcTime: 60_000,
    queryFn: async ({ cursor, signal, limit }) => {
      const result = await client.get<Page<T>>(path, { ...params, cursor, limit }, signal);
      return { ...result.data, meta: result.meta };
    },
  });
}
