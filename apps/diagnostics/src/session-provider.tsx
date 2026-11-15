import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { DiagnosticsClient, DiagnosticsError, type Session } from '@infrapro/diagnostics-client';
import { SessionContext } from './session-context';
const client = new DiagnosticsClient();
export function SessionProvider({ children }: { children: ReactNode }) {
  const cache = useQueryClient();
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);
  const [unavailable, setUnavailable] = useState(false);
  const clear = useCallback((blocked = false) => { client.clear(); void cache.cancelQueries(); cache.clear(); setSession(undefined); setUnavailable(blocked); }, [cache]);
  const channel = useRef<BroadcastChannel | undefined>(undefined);
  useEffect(() => {
    const stop = client.onDenied((error) => { clear(error.code === 'auth_configuration_unavailable' || error.status === 403); });
    const controller = new AbortController();
    void client.session(controller.signal).then(({ data }) => { if (!controller.signal.aborted) setSession(data); }).catch((error: unknown) => { if (!controller.signal.aborted && error instanceof DiagnosticsError && error.code === 'auth_configuration_unavailable') setUnavailable(true); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { stop(); controller.abort(); };
  }, [clear]);
  useEffect(() => { if (typeof BroadcastChannel === 'undefined') return; const current = new BroadcastChannel('infrapro-diagnostics-session'); channel.current = current; current.onmessage = (event: MessageEvent<unknown>) => { if (event.data === 'logout' || event.data === 'auth-changed') clear(); }; return () => { current.close(); channel.current = undefined; }; }, [clear]);
  useEffect(() => {
    if (!session) return;
    let last = 0;
    const activity = (event: Event) => {
      if (!event.isTrusted || document.visibilityState !== 'visible' || Date.now() - last < 60000) return;
      last = Date.now(); void client.activity().catch(() => { /* The client handles authentication failure centrally. */ });
    };
    document.addEventListener('pointerdown', activity, { passive: true }); document.addEventListener('keydown', activity);
    return () => { document.removeEventListener('pointerdown', activity); document.removeEventListener('keydown', activity); };
  }, [session]);
  const login = useCallback(async (username: string, password: string) => { const { data } = await client.login(username, password); cache.clear(); setUnavailable(false); setSession(data); }, [cache]);
  const logout = useCallback(async () => { try { await client.logout(); } finally { clear(); channel.current?.postMessage('logout'); } }, [clear]);
  return <SessionContext value={{ client, session, loading, unavailable, login, logout }}>{children}</SessionContext>;
}
