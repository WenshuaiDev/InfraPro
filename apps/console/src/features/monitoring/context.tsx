import { createContext, useCallback, useContext, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Permissions as P, type MonitoringScope, type PermissionCode } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { useTenant } from '../tenancy/context';
import { useProject } from '../projects/context';
import { projectContextKey } from '../projects/scope';

type Range = { scope: MonitoringScope; prefix: readonly unknown[]; available: boolean; archived: boolean; can: (permission: PermissionCode) => boolean };
const Context = createContext<Range | null>(null);
export function TenantMonitoring({ children }: { children: ReactNode }) {
  const t = useTenant(); const { account } = useIdentity();
  return <MonitoringBoundary value={{ scope: { kind: 'tenant', tenantId: t.tenant.id }, prefix: ['tenant', account?.id, t.tenant.id, t.membership_id, 'monitoring', t.permissions.join(',')], available: t.available, archived: false, can: p => t.permissions.includes(p) }}>{children}</MonitoringBoundary>;
}
export function ProjectMonitoring({ children }: { children: ReactNode }) {
  const p = useProject();
  return <MonitoringBoundary value={{ scope: { kind: 'project', tenantId: p.project.tenant_id, projectId: p.project.id }, prefix: [...p.key, 'monitoring', p.resource_access_revision], available: p.available, archived: p.project.status === 'archived', can: p.can }}>{children}</MonitoringBoundary>;
}
function MonitoringBoundary({ value, children }: { value: Range; children: ReactNode }) {
  const { client, account } = useIdentity(); const cache = useQueryClient();
  const [notice, setNotice] = useState(false);
  const seenRevision = useRef(false);
  const tenant = value.scope.tenantId, project = value.scope.kind === 'project' ? value.scope.projectId : undefined;
  const subscribe = useCallback((notify: () => void) => client.onMonitoringRestriction((t, p, code) => {
    if (t !== tenant || p !== project) return;
    if (code !== 'revision' || seenRevision.current) setNotice(true);
    seenRevision.current = true;
    notify();
    if (code !== 'revision') void cache.invalidateQueries({ queryKey: project ? projectContextKey(account?.id, tenant, project) : ['tenant', account?.id, tenant, 'context'] });
  }), [account?.id, cache, client, tenant, project]);
  const revision = useSyncExternalStore(subscribe, () => client.monitoringRevision(tenant, project));
  const prefix = [...value.prefix, revision], stamp = JSON.stringify(prefix);
  useEffect(() => {
    const queryKey: unknown[] = JSON.parse(stamp);
    return () => { void cache.cancelQueries({ queryKey }); cache.removeQueries({ queryKey }); };
  }, [cache, stamp]);
  useEffect(() => () => client.invalidateMonitoringScope(tenant, project), [client, tenant, project]);
  // A lightweight metadata poll discovers revocation while a fixed snapshot is idle.
  useQuery({ queryKey: [...prefix, 'qualification'], queryFn: ({ signal }) => client.monitoringJobs(value.scope, undefined, signal), enabled: value.available, refetchInterval: 30_000, refetchOnWindowFocus: 'always', retry: false });
  return <Context value={{ ...value, prefix }}>{notice && <p role="status" className="mb-4 rounded-lg border bg-muted/50 p-3 text-sm">数据或授权已变化，已清空此前选择与缓存。请重新选择当前获准的范围。</p>}<div key={`${JSON.stringify(value.prefix)}:${revision.split(':').at(-1)}`}>{children}</div></Context>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useMonitoring() {
  const range = useContext(Context), identity = useIdentity();
  if (!range) throw new Error('Explicit monitoring scope required');
  const { scope, can, archived } = range, tenant = scope.kind === 'tenant';
  const base = tenant ? `/t/${scope.tenantId}` : `/t/${scope.tenantId}/p/${scope.projectId}`;
  return { ...identity, ...range,
    canRead: can(tenant ? P.TenantMonitoringDataRead : P.ProjectMonitoringDataRead),
    canNavigate: can(tenant ? P.TenantEngineeringResourceRead : P.ProjectEngineeringResourceRead),
    canHistory: can(tenant ? P.TenantMonitoringDataHistoryRead : P.ProjectMonitoringDataHistoryRead),
    canExport: can(tenant ? P.TenantMonitoringDataExport : P.ProjectMonitoringDataExport),
    canImportRead: can(tenant ? P.TenantMonitoringImportsRead : P.ProjectMonitoringImportsRead),
    canManage: !archived && can(tenant ? P.TenantMonitoringImportsManage : P.ProjectMonitoringImportsManage),
    canRevoke: !archived && can(tenant ? P.TenantMonitoringBatchesRevoke : P.ProjectMonitoringBatchesRevoke),
    canSource: can(tenant ? P.TenantMonitoringSourceRead : P.ProjectMonitoringSourceRead),
    paths: { root: `${base}/monitoring`, imports: `${base}/monitoring/imports`, batch: (id: string) => `${base}/monitoring/imports/${encodeURIComponent(id)}`, jobs: `${base}/monitoring/jobs` },
  };
}
