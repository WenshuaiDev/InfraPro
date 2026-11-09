import { Fragment, createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { ApiError, createApiClient, type Account, type AccountHelp, type AuthContext, type PermissionCode } from './index';

type IdentityState = { phase: 'loading' | 'authenticated' | 'anonymous' | 'unavailable'; account?: Account; help?: AccountHelp; permissions: PermissionCode[]; authorization_revision?: string; authorizationEpoch: number };
export type IdentityValue = IdentityState & {
  client: ReturnType<typeof createApiClient>;
  reload: () => Promise<void>;
  signedIn: (account: Account) => Promise<void>;
  signedOut: () => void;
  can: (permission: PermissionCode) => boolean;
  canAll: (...permissions: PermissionCode[]) => boolean;
};
const IdentityContext = createContext<IdentityValue | null>(null);
export function IdentityProvider({ children, baseUrl = '/api/v1' }: { children: ReactNode; baseUrl?: string }) {
  const client = useMemo(() => createApiClient({ baseUrl, timeoutMs: 15_000 }), [baseUrl]);
  const [state, setState] = useState<IdentityState>({ phase: 'loading', permissions: [], authorizationEpoch: 0 });
  const generation = useRef(0);
  const pending = useRef<Promise<void> | undefined>(undefined);
  const revision = useRef<string | undefined>(undefined);
  const previous = useRef<AuthContext | undefined>(undefined);
  const accept = useCallback((value: AuthContext, help?: AccountHelp) => {
    const old = previous.current;
    if (old && (old.account.id !== value.account.id || old.permissions.some((p) => !value.permissions.includes(p)))) client.invalidateIdentity();
    previous.current = value;
    revision.current = value.authorization_revision;
    setState((old) => ({ ...old, ...value, help: help ?? old.help, phase: 'authenticated', authorizationEpoch: old.authorizationEpoch + (old.account?.id === value.account.id && old.permissions.some((p) => !value.permissions.includes(p)) ? 1 : 0) }));
  }, [client]);
  const signedOut = useCallback(() => { client.invalidateIdentity(); previous.current = undefined; generation.current++; pending.current = undefined; revision.current = undefined; setState((old) => ({ ...old, phase: 'anonymous', account: undefined, permissions: [] })); }, [client]);
  const reload = useCallback(() => {
    if (pending.current) return pending.current;
    const version = generation.current;
    const task = client.csrf().then(() => client.accountHelp()).then(async (help) => {
      try { const auth = await client.me(); if (version === generation.current) accept(auth, help); }
      catch (error) { if (error instanceof ApiError && error.status === 401) { if (version === generation.current) { signedOut(); setState((old) => ({ ...old, help })); } } else throw error; }
    }).catch(() => { if (version === generation.current) setState((old) => ({ ...old, phase: 'unavailable' })); }).finally(() => { if (pending.current === task) pending.current = undefined; });
    pending.current = task;
    return task;
  }, [client, accept, signedOut]);
  const signedIn = useCallback(async () => { client.invalidateIdentity(); previous.current = undefined; generation.current++; pending.current = undefined; await reload(); }, [client, reload]);
  useEffect(() => { void reload(); return client.onUnauthorized(signedOut); }, [client, reload, signedOut]);
  useEffect(() => client.onAuthorizationChange((next) => { if (!next || (next !== revision.current && (!revision.current || BigInt(next) > BigInt(revision.current)))) void reload(); }), [client, reload]);
  useEffect(() => {
    const focus = () => { if (document.visibilityState === 'visible') void reload(); };
    window.addEventListener('focus', focus);
    document.addEventListener('visibilitychange', focus);
    const timer = window.setInterval(focus, 30_000);
    return () => { clearInterval(timer); window.removeEventListener('focus', focus); document.removeEventListener('visibilitychange', focus); };
  }, [reload]);
  useEffect(() => {
    if (state.phase !== 'authenticated') return;
    let lastSent = 0;
    const activity = (event: Event) => {
      if (!event.isTrusted || document.visibilityState !== 'visible' || Date.now() - lastSent < 30_000) return;
      lastSent = Date.now();
      const version = generation.current;
      void client.activity().catch((error: unknown) => {
        if (version === generation.current && error instanceof ApiError && error.status === 503) setState((old) => ({ ...old, phase: 'unavailable' }));
      });
    };
    const events = ['pointerdown', 'keydown', 'scroll'];
    events.forEach((event) => window.addEventListener(event, activity, { passive: true }));
    return () => events.forEach((event) => window.removeEventListener(event, activity));
  }, [client, state.phase]);
  const can = (permission: PermissionCode) => state.permissions.includes(permission);
  // Loss of any permission destroys queries, drafts, credentials and pending consumer callbacks.
  // Unrelated revision changes and grants preserve drafts. Nothing is persisted to localStorage.
  return <IdentityContext value={{ ...state, client, reload, signedIn, signedOut, can, canAll: (...permissions) => permissions.every(can) }}>
    <Fragment key={`${state.account?.id ?? 'anonymous'}:${state.authorizationEpoch}`}>{children}</Fragment>
  </IdentityContext>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useIdentity() {
  const value = useContext(IdentityContext);
  if (!value) throw new Error('IdentityProvider is required');
  return value;
}
