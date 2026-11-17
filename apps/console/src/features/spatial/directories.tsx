import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Permissions as P,
  type SpatialObjectRef,
  type SpatialElement,
  type ResourceTreeRef,
} from "@infrapro/api-client";
import { TenantResources, ProjectResources } from "../resources/context";
import { ResourceTree } from "../resources/resource-tree";
import {
  navigationPath,
  useResourceTree,
} from "../resources/resource-tree-query";
import { useSpatial } from "./context";
import { TextField, Info } from "./common";
export function SpatialDirectory({
  selected,
  facility,
  select,
  compact = false,
}: {
  selected?: SpatialObjectRef;
  facility?: string;
  compact?: boolean;
  select: (o: SpatialObjectRef, facility?: string) => void;
}) {
  const m = useSpatial();
  if (
    !m.can(
      m.scope.kind === "tenant"
        ? P.TenantEngineeringResourceRead
        : P.ProjectEngineeringResourceRead,
    )
  )
    return <Info>查看业务目录需要资源读取权限。</Info>;
  const Boundary =
    m.scope.kind === "tenant" ? TenantResources : ProjectResources;
  return (
    <Boundary>
      <Directory selected={selected} facility={facility} select={select} compact={compact} />
    </Boundary>
  );
}
function Directory({
  selected,
  facility,
  select,
  compact = false,
}: {
  selected?: SpatialObjectRef;
  facility?: string;
  compact?: boolean;
  select: (o: SpatialObjectRef, facility?: string) => void;
}) {
  const m = useSpatial(),
    [q, setQ] = useState("");
  const roots = useResourceTree(
    { q: q || undefined, status: "all" },
    m.available,
  );
  const detail = useQuery({
    queryKey: [...m.prefix, "tree-path", selected, facility],
    queryFn: ({ signal }) =>
      m.client.resourceDetail(
        m.scope,
        {
          kind: selected!.kind as "facility" | "component" | "monitoring_section" | "borehole" | "point",
          facility: facility!,
          id: selected!.id,
        },
        signal,
      ),
    enabled: !!selected && !!facility && m.available,
    retry: false,
  });
  const choose = (node: ResourceTreeRef) => {
    if (node.kind !== "channel")
      select({ kind: node.kind, id: node.id }, node.facility_id);
  };
  return (
    <>
      <div className="p-3">
        <>{compact ? <input className="spatial-search" aria-label="搜索资源目录" placeholder="搜索设施、构件、测点" value={q} onChange={event => setQ(event.target.value)} /> : <TextField label="搜索资源目录" value={q} onChange={setQ} />}</>
      </div>
      <ResourceTree
        query={roots}
        searching={!!q && m.scope.kind === "tenant"}
        filtered={!!q}
        selected={selected?.id}
        path={detail.data ? navigationPath(detail.data) : []}
        select={choose}
        expandOnSelect={!compact}
        leafKind={compact ? "point" : undefined}
        title="设施、构件与测点"
      />
      {!compact && <p className="p-3 text-xs text-muted-foreground">
        上下文祖先仅用于导航；选择后会重新核验完整对象权限。
      </p>}
    </>
  );
}
export function SourceTree({
  elements,
  selected,
  select,
}: {
  elements: SpatialElement[];
  selected?: string;
  select: (key: string) => void;
}) {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const byKey = new Map(elements.map((e) => [e.key, e]));
  const children = new Map<string, SpatialElement[]>();
  for (const e of elements) {
    const parent = e.parent && byKey.has(e.parent) ? e.parent : "";
    children.set(parent, [...(children.get(parent) ?? []), e]);
  }
  const rows: { element: SpatialElement; depth: number }[] = [],
    visited = new Set<string>();
  const visit = (e: SpatialElement, depth: number, hidden = false) => {
    if (visited.has(e.key)) return;
    visited.add(e.key);
    if (!hidden) rows.push({ element: e, depth });
    for (const child of children.get(e.key) ?? [])
      visit(child, depth + 1, hidden || collapsed.has(e.key));
  };
  for (const root of children.get("") ?? []) visit(root, 0);
  for (const element of elements)
    if (!visited.has(element.key)) visit(element, 0);
  return (
    <ul aria-label="源模型构件树" className="space-y-1">
      {rows.map(({ element: e, depth }) => (
        <li
          key={e.key}
          style={{ paddingLeft: Math.min(depth, 6) * 12 }}
          className="flex items-start"
        >
          {!!children.get(e.key)?.length && (
            <button
              type="button"
              className="min-h-11 min-w-8 rounded hover:bg-muted focus-visible:ring-2"
              aria-label={`${collapsed.has(e.key) ? "展开" : "收起"} ${e.name || e.type}`}
              aria-expanded={!collapsed.has(e.key)}
              onClick={() =>
                setCollapsed((old) => {
                  const next = new Set(old);
                  if (next.has(e.key)) next.delete(e.key);
                  else next.add(e.key);
                  return next;
                })
              }
            >
              {collapsed.has(e.key) ? "▸" : "▾"}
            </button>
          )}
          <button
            type="button"
            className={`min-h-11 min-w-0 flex-1 rounded p-2 text-left text-sm hover:bg-muted focus-visible:ring-2 ${selected === e.key ? "bg-primary/10 font-medium" : ""}`}
            aria-current={selected === e.key ? "true" : undefined}
            onClick={() => select(e.key)}
          >
            {e.name || e.type}
            <span className="block text-xs text-muted-foreground">
              {e.type}
              {e.type_key ? " · 有类型定义" : ""}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
