import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Permissions as P,
  type PermissionCode,
  type SpatialScope,
} from "@infrapro/api-client";
import { useIdentity } from "@infrapro/api-client/react";
import { useTenant } from "../tenancy/context";
import { useProject } from "../projects/context";
type ScopeValue = {
  scope: SpatialScope;
  prefix: readonly unknown[];
  available: boolean;
  archived: boolean;
  can: (p: PermissionCode) => boolean;
};
const Context = createContext<ScopeValue | null>(null);
export function TenantSpatial({ children }: { children: ReactNode }) {
  const t = useTenant(),
    { account } = useIdentity();
  return (
    <Boundary
      value={{
        scope: { kind: "tenant", tenantId: t.tenant.id },
        prefix: [
          "tenant",
          account?.id,
          t.tenant.id,
          t.membership_id,
          "spatial",
          t.permissions.join(","),
        ],
        available: t.available,
        archived: false,
        can: (p) => t.permissions.includes(p),
      }}
    >
      {children}
    </Boundary>
  );
}
export function ProjectSpatial({ children }: { children: ReactNode }) {
  const p = useProject();
  return (
    <Boundary
      value={{
        scope: {
          kind: "project",
          tenantId: p.project.tenant_id,
          projectId: p.project.id,
        },
        prefix: [...p.key, "spatial", p.resource_access_revision],
        available: p.available,
        archived: p.project.status === "archived",
        can: p.can,
      }}
    >
      {children}
    </Boundary>
  );
}
function Boundary({
  value,
  children,
}: {
  value: ScopeValue;
  children: ReactNode;
}) {
  const { client } = useIdentity(),
    cache = useQueryClient(),
    [notice, setNotice] = useState(false),
    tenant = value.scope.tenantId,
    project =
      value.scope.kind === "project" ? value.scope.projectId : undefined;
  const subscribe = useCallback(
    (notify: () => void) =>
      client.onSpatialRestriction((t, p, code) => {
        if (t === tenant && p === project) {
          if (code !== "revision") setNotice(true);
          notify();
        }
      }),
    [client, tenant, project],
  );
  const revision = useSyncExternalStore(subscribe, () =>
      client.spatialRevision(tenant, project),
    ),
    epoch = revision.split(":").at(-1),
    prefix = [...value.prefix, epoch],
    stamp = JSON.stringify(prefix);
  useEffect(() => {
    const queryKey: unknown[] = JSON.parse(stamp);
    return () => {
      void cache.cancelQueries({ queryKey });
      cache.removeQueries({ queryKey });
    };
  }, [cache, stamp]);
  useEffect(
    () => () => client.invalidateSpatialScope(tenant, project),
    [client, tenant, project],
  );
  const canRead = value.can(
    project ? P.ProjectSpatialView : P.TenantSpatialRead,
  );
  useQuery({
    queryKey: [...prefix, "qualification"],
    queryFn: ({ signal }) =>
      client.spatialAssets(value.scope, { limit: 1 }, signal),
    enabled: value.available && canRead,
    refetchInterval: 15000,
    refetchOnWindowFocus: "always",
    retry: false,
  });
  return (
    <Context value={{ ...value, prefix }}>
      {notice && (
        <p role="status" className="mb-3 rounded-lg border p-3 text-sm">
          授权或资料已变化，旧画面和选择已清空。请重新选择可访问的对象。
        </p>
      )}
      <div key={stamp}>{children}</div>
    </Context>
  );
}
// eslint-disable-next-line react-refresh/only-export-components
export function useSpatial() {
  const v = useContext(Context),
    identity = useIdentity();
  if (!v) throw Error("Spatial scope required");
  const tenant = v.scope.kind === "tenant";
  return {
    ...v,
    ...identity,
    can: v.can,
    canRead: v.can(tenant ? P.TenantSpatialRead : P.ProjectSpatialView),
    canManage: tenant && v.can(P.TenantSpatialManage),
    canEdit: tenant && v.can(P.TenantSpatialPlacementManage),
    canPublish: tenant && v.can(P.TenantSpatialPublish),
    canSaveView: !tenant && !v.archived && v.can(P.ProjectSpatialViewsManage),
    base: `/t/${v.scope.tenantId}${v.scope.kind === "project" ? `/p/${v.scope.projectId}` : ""}`,
  };
}
