import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useLocation } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { ListChecks } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import type { TaskSummary } from '@infrapro/api-client';
import { workspaceScope } from '../../config/navigation';
type ContextValue = { key: readonly unknown[]; summary?: TaskSummary; observeAuthorization: (value?: string) => void; refresh: () => Promise<void>; workspace: { tenant_id?: string; project_id?: string } };
const Context = createContext<ContextValue | null>(null);
export function TaskProvider({ children }: { children: ReactNode }) {
 const { client, account, authorization_revision, authorizationEpoch } = useIdentity(), cache = useQueryClient();
 const [epoch, setEpoch] = useState(0), revision = useRef(authorization_revision), vector = useRef<string | undefined>(undefined);
 const path = useLocation({ select: value => value.pathname }), current = workspaceScope(path);
 const browsingTasks = path.startsWith('/tasks') || path.endsWith('/monitoring/jobs');
 const [lastWorkspace, setLastWorkspace] = useState<{ tenant_id?: string; project_id?: string }>({});
 if (current.tenantId && (lastWorkspace.tenant_id !== current.tenantId || lastWorkspace.project_id !== current.projectId)) setLastWorkspace({ tenant_id: current.tenantId, project_id: current.projectId });
 const prefix = useMemo(() => ['tasks', account?.id] as const, [account?.id]);
 const key = useMemo(() => [...prefix, authorization_revision, authorizationEpoch, epoch] as const, [prefix, authorization_revision, authorizationEpoch, epoch]);
 const refresh = useCallback(async () => { await cache.invalidateQueries({ queryKey: prefix }); }, [cache, prefix]);
 const summary = useQuery({ queryKey: [...key, 'summary'], queryFn: ({ signal }) => client.taskSummary({}, signal), enabled: !browsingTasks, refetchInterval: 15_000, refetchIntervalInBackground: false, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', retry: false });
 // The center already qualifies its list; a global full-scope count must not
 // compete with that request for the same bounded metadata budget.
 useEffect(() => { if (browsingTasks) void cache.cancelQueries({queryKey:[...key,'summary']}); }, [browsingTasks, cache, key]);
 useEffect(() => {
  const clear = () => { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }); setEpoch(v => v + 1); };
  const subscriptions = [client.onTenantRestriction((_id, code) => { if (code !== 'active') clear(); }), client.onProjectRestriction(clear), client.onAuthorizationChange(value => { if (value && revision.current !== value) { revision.current = value; clear(); } })];
  return () => { subscriptions.forEach(fn => fn()); void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }); };
 }, [cache, client, prefix]);
 const observeAuthorization = useCallback((next?: string) => { if (next && vector.current && vector.current !== next) { void cache.cancelQueries({queryKey:prefix}); cache.removeQueries({queryKey:prefix}); setEpoch(v => v + 1); } if(next) vector.current=next; }, [cache, prefix]);
 useEffect(() => { if(!browsingTasks) observeAuthorization(summary.data?.authorization_revision); }, [browsingTasks, summary.data?.authorization_revision, observeAuthorization]);
 return <Context value={{ key, summary: summary.error || browsingTasks ? undefined : summary.data, observeAuthorization, refresh, workspace: current.tenantId ? { tenant_id: current.tenantId, project_id: current.projectId } : lastWorkspace }}>{children}</Context>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useTasks() { const value = useContext(Context); if (!value) throw Error('TaskProvider required'); return value; }
export function TaskButton() { const t = useTasks(); const count = t.summary?.counts_state === 'exact' ? t.summary.active : null; return <Button variant="ghost" className="min-h-11 min-w-11 gap-1" asChild><Link to="/tasks" search={{ mode: 'mine' }} aria-label={count != null ? `任务中心，${count} 项进行中` : '任务中心'}><ListChecks aria-hidden="true" /><span className="hidden sm:inline">任务</span>{count != null && count > 0 && <span className="text-xs tabular-nums">{count}</span>}</Link></Button>; }
