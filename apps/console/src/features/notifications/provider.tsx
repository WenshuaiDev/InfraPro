import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocation } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import type { NotificationSummary } from '@infrapro/api-client';

type ContextValue = { key: readonly unknown[]; open: boolean; setOpen: (open: boolean) => void; summary?: NotificationSummary; error: Error | null; pending: boolean; refresh: () => Promise<void>; inboxViews: Map<string, { scrollTop: number; search: string }> };
const Context = createContext<ContextValue | null>(null);
export function NotificationProvider({ children }: { children: ReactNode }) {
 const { client, account, authorization_revision, authorizationEpoch } = useIdentity();
 const cache = useQueryClient();
 const [open, setOpen] = useState(false), [epoch, setEpoch] = useState(0);
 const seenRevision = useRef(authorization_revision);
 const path = useLocation({ select: location => location.pathname });
 const prefix = useMemo(() => ['notifications', account?.id] as const, [account?.id]);
 const key = useMemo(() => [...prefix, authorization_revision, authorizationEpoch, epoch] as const, [prefix, authorization_revision, authorizationEpoch, epoch]);
 // List/detail are separate shareable routes. Keep only their transient view
 // position here, scoped to the same account and authorization epoch as data.
 // eslint-disable-next-line react-hooks/exhaustive-deps -- Changing the authorization key intentionally discards view-only state.
 const inboxViews = useMemo(() => new Map<string, { scrollTop: number; search: string }>(), [key]);
 const summary = useQuery({ queryKey: [...key, 'summary'], queryFn: ({ signal }) => client.notificationSummary(signal), refetchInterval: open || path.includes('/notifications') ? 5_000 : 15_000, refetchIntervalInBackground: false, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', retry: 2, retryDelay: attempt => Math.min(1000 * 2 ** attempt, 15_000) });
 const refresh = useCallback(async () => { await cache.invalidateQueries({ queryKey: prefix }); }, [cache, prefix]);
 useEffect(() => {
  // Keep reconnect refresh attached to the account owner even when observers
  // have been replaced by an authorization/visibility change.
  const reconnect = () => { if (document.visibilityState === 'visible') void refresh(); };
  window.addEventListener('online', reconnect);
  return () => window.removeEventListener('online', reconnect);
 }, [refresh]);
 useEffect(() => {
  const clear = () => { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }); setEpoch(value => value + 1); setOpen(false); };
  const unsubscribe = [client.onTenantRestriction((_id, code) => { if (code !== 'active') clear(); }), client.onProjectRestriction(clear), client.onAuthorizationChange(revision => { if (revision && seenRevision.current !== revision) { seenRevision.current = revision; clear(); } })];
  return () => { unsubscribe.forEach(fn => fn()); void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }); };
 }, [cache, client, prefix]);
 return <Context value={{ key, open, setOpen, summary: summary.data, error: summary.error, pending: summary.isPending, refresh, inboxViews }}>{children}</Context>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useNotifications() { const value = useContext(Context); if (!value) throw Error('NotificationProvider required'); return value; }
