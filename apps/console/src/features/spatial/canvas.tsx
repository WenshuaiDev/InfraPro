import { lazy, Suspense, useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type {
  SpatialAsset,
  SpatialVersion,
  SpatialObjectRef,
  SpatialElement,
  SpatialSettings,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { ObjectHistory } from "../audit/object-history";
import { useSpatial } from "./context";
import { allSpatialPages } from "./facility-model";
import { Field, TextField, Failure, Info } from "./common";
import { inputClass, objectKey, canPlace, type Pick } from "./state";
const Layout = lazy(() => import("./layout-viewer")),
  Model = lazy(() => import("./model-viewer")),
  MapView = lazy(() => import("./map-viewer"));
export function ResourceMap({
  owner,
  select,
  hover,
  bbox,
  onBBox,
  children,
  facility,
  compact = false,
}: {
  owner?: SpatialObjectRef;
  select: (o: SpatialObjectRef) => void;
  hover: (o?: SpatialObjectRef) => void;
  bbox: string;
  onBBox: (s: string) => void;
  children?: React.ReactNode;
  facility?: string;
  compact?: boolean;
}) {
  const m = useSpatial(),
    [kind, setKind] = useState("facility");
  const q = useInfiniteQuery({
    queryKey: [...m.prefix, "map-objects", bbox, kind, facility],
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      m.client.spatialObjects(
        m.scope,
        {
          kind,
          facility_id: facility,
          bbox: bbox || undefined,
          cursor: pageParam,
          limit: 100,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available,
    retry: false,
  });
  const focused = useQuery({
    queryKey: [...m.prefix, "map-focused", owner],
    queryFn: ({ signal }) => m.client.spatialObject(m.scope, owner!, signal),
    enabled: !!owner && m.available,
    retry: false,
  });
  const objects = [
    ...new Map(
      [
        ...(q.data?.pages.flatMap((p) => p.items) ?? []),
        ...(focused.data ? [focused.data] : []),
      ].map((o) => [o.id, o]),
    ).values(),
  ];
  return (
    <>
      <Field label="地理位置图层">
        <select
          className={inputClass}
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="facility">设施位置</option>
          <option value="component">构件位置</option>
          <option value="monitoring_section">断面位置</option>
          <option value="borehole">孔位位置</option>
          <option value="point">测点位置</option>
        </select>
      </Field>
      <div
        className={
          compact
            ? "spatial-canvas-stage"
            : "relative h-[560px] overflow-hidden rounded-xl border"
        }
      >
        <Suspense fallback={<Info>正在加载地图…</Info>}>
          <MapView
            objects={objects}
            features={[]}
            selected={owner}
            onSelect={select}
            onHover={hover}
            onBBox={onBBox}
          />
        </Suspense>
        {children}
      </div>
      <Failure error={q.error} />
      <p className="text-xs text-muted-foreground">
        本次已载入 {objects.length} 个可读对象，其中{" "}
        {
          objects.filter((o) => o.longitude != null && o.latitude != null)
            .length
        }{" "}
        个具备有效地理位置。
      </p>
      {q.hasNextPage && (
        <Button
          variant="outline"
          disabled={q.isFetchingNextPage}
          onClick={() => void q.fetchNextPage()}
        >
          载入当前范围更多对象
        </Button>
      )}
    </>
  );
}
export function VersionCanvas({
  version,
  asset,
  owner,
  element,
  elements,
  pageID,
  viewID,
  mainChoices,
  onPage,
  onView,
  pick,
  hover,
  bbox,
  onBBox,
  children,
  compact = false,
}: {
  version: SpatialVersion;
  asset: SpatialAsset;
  owner?: SpatialObjectRef;
  element?: string;
  elements: SpatialElement[];
  pageID?: string;
  viewID?: string;
  mainChoices: Record<string, string>;
  onPage: (id: string) => void;
  onView: (id: string) => void;
  pick: (p: Pick) => void;
  hover: (o?: SpatialObjectRef) => void;
  bbox: string;
  onBBox: (s: string) => void;
  children?: React.ReactNode;
  compact?: boolean;
}) {
  const m = useSpatial(),
    [settings, setSettings] = useState<SpatialSettings>(version.settings),
    [viewRevision, setViewRevision] = useState("0"),
    [markerQuery, setMarkerQuery] = useState(""),
    [activeOnly, setActiveOnly] = useState(false);
  const queryClient = useQueryClient();
  const page =
    version.manifest.pages?.find(
      (p) => p.id === (pageID || settings.page_id),
    ) ?? version.manifest.pages?.[0];
  const placements = useInfiniteQuery({
    queryKey: [
      ...m.prefix,
      "placements",
      version.id,
      page?.id,
      markerQuery,
      activeOnly,
    ],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      m.client.spatialPlacements(
        m.scope,
        version.id,
        {
          page_id: page?.id,
          cursor: pageParam,
          limit: 100,
          q: markerQuery,
          state: activeOnly ? "normal" : undefined,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available && !!version.manifest_hash,
    retry: false,
  });
  const links = useInfiniteQuery({
    queryKey: [
      ...m.prefix,
      "component-links",
      version.id,
      page?.id,
      markerQuery,
      activeOnly,
    ],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      m.client.spatialComponentLinks(
        m.scope,
        version.id,
        {
          page_id: page?.id,
          cursor: pageParam,
          limit: 100,
          q: markerQuery,
          state: activeOnly ? "normal" : undefined,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available && !!version.manifest_hash,
    retry: false,
  });
  useEffect(() => {
    if (
      compact &&
      links.hasNextPage &&
      !links.isFetching &&
      !links.isError &&
      (links.data?.pages.length ?? 0) < 10
    )
      void links.fetchNextPage();
  }, [compact, links]);
  const features = useInfiniteQuery({
    queryKey: [...m.prefix, "features", version.id, bbox],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      m.client.spatialFeatures(
        m.scope,
        version.id,
        { bbox: bbox || undefined, cursor: pageParam, limit: 100 },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled:
      m.available && asset.kind === "geolayer" && !!version.manifest_hash,
    retry: false,
  });
  const view = useQuery({
    queryKey: [...m.prefix, "view", viewID],
    queryFn: ({ signal }) => m.client.spatialView(m.scope, viewID!, signal),
    enabled: !!viewID && m.scope.kind === "project" && m.available,
    retry: false,
  });
  const [observedView, setObservedView] = useState("");
  const viewStamp = `${viewID}:${view.data?.revision}`;
  if (
    view.data &&
    view.data.version_id === version.id &&
    observedView !== viewStamp
  ) {
    setObservedView(viewStamp);
    setSettings(view.data.settings);
    setViewRevision(view.data.revision);
  }
  const saveView = useMutation({
    mutationFn: async () => {
      const id = viewID ?? version.id;
      const result = await m.client.spatialSaveView(m.scope, id, {
        request_id: crypto.randomUUID(),
        expected_revision: viewRevision,
        version_id: version.id,
        settings: {
          ...settings,
          page_id: page?.id,
          main_channels: { ...settings.main_channels, ...mainChoices },
        },
      });
      setViewRevision(result.revision);
      await queryClient.invalidateQueries({
        queryKey: [...m.prefix, "view", id],
      });
      onView(id);
    },
  });
  const selectedPoints = useQuery({
    queryKey: [...m.prefix, "selected-placements", version.id, owner],
    queryFn: ({ signal }) =>
      allSpatialPages((cursor) =>
        m.client.spatialPlacements(
          m.scope,
          version.id,
          { owner_id: owner!.id, kind: owner!.kind, limit: 100, cursor },
          signal,
        ),
      ),
    enabled: canPlace(owner?.kind) && m.available,
    retry: false,
  });
  const selectedLinks = useQuery({
    queryKey: [...m.prefix, "selected-links", version.id, owner],
    queryFn: ({ signal }) =>
      allSpatialPages((cursor) =>
        m.client.spatialComponentLinks(
          m.scope,
          version.id,
          { owner_id: owner!.id, kind: owner!.kind, limit: 100, cursor },
          signal,
        ),
      ),
    enabled: owner?.kind === "component" && m.available,
    retry: false,
  });
  const pointItems = [
    ...new Map(
      [
        ...(placements.data?.pages.flatMap((p) => p.items) ?? []),
        ...(selectedPoints.data?.items ?? []),
      ].map((p) => [p.id, p]),
    ).values(),
  ];
  const linkItems = [
    ...new Map(
      [
        ...(links.data?.pages.flatMap((p) => p.items) ?? []),
        ...(selectedLinks.data?.items ?? []),
      ].map((p) => [p.id, p]),
    ).values(),
  ];
  const locatedOwner = useRef("");
  useEffect(() => {
    const key = owner ? objectKey(owner) : "";
    if (!key || locatedOwner.current === key) return;
    const targets =
      canPlace(owner?.kind)
        ? selectedPoints.data?.items
        : selectedLinks.data?.items;
    if (!targets) return;
    locatedOwner.current = key;
    if (
      asset.kind === "layout" &&
      targets.length &&
      !targets.some((p) => p.page_id === page?.id) &&
      targets[0]?.page_id
    )
      onPage(targets[0]?.page_id);
  }, [
    owner,
    selectedPoints.data,
    selectedLinks.data,
    asset.kind,
    page?.id,
    onPage,
  ]);
  const scene = {
    placements: pointItems.filter(
      (p) =>
        (!activeOnly || !p.object?.retired) &&
        (!markerQuery ||
          `${p.object?.name} ${p.object?.code}`.includes(markerQuery)),
    ),
    links: linkItems.filter(
      (p) =>
        (!activeOnly || !p.object?.retired) &&
        (!markerQuery ||
          `${p.object?.name} ${p.object?.code}`.includes(markerQuery)),
    ),
    selected: owner,
    element,
    settings,
    onPick: pick,
    onHover: hover,
    onHiddenElements: (hidden_elements: string[]) =>
      setSettings((s) => ({ ...s, hidden_elements })),
    onCamera: m.canSaveView
      ? (camera: NonNullable<SpatialSettings["camera"]>) =>
          setSettings((s) => ({ ...s, camera }))
      : undefined,
  };
  const modelFile = version.manifest.files?.find(
    (f) => f.media_type === "model/gltf-binary",
  );
  if (m.scope.kind === "project" && viewID && view.isPending)
    return <Info>正在读取项目视图…</Info>;
  return (
    <>
      {view.data && view.data.version_id !== version.id && (
        <Info>此保存视图属于另一固定版本，当前按本版本默认视图显示。</Info>
      )}
      {version.state === "draft" && <Info>此版本为草稿，尚未向项目发布。</Info>}
      {asset.status === "retired" && (
        <Info>资料已退役，保留当前获准版本供历史查询。</Info>
      )}
      <div
        className={
          compact
            ? "spatial-canvas-tools"
            : "flex flex-wrap items-end gap-3 rounded-lg border bg-card p-3"
        }
      >
        {asset.kind === "layout" && (
          <Field label="图纸页码">
            <select
              className={inputClass}
              value={page?.id ?? ""}
              onChange={(e) => onPage(e.target.value)}
            >
              {version.manifest.pages?.map((p) => (
                <option key={p.id} value={p.id}>
                  第 {p.number} 页
                </option>
              ))}
            </select>
          </Field>
        )}
        {asset.kind !== "geolayer" && (
          <div className="flex flex-wrap items-end gap-3">
            {compact ? (
              <input
                aria-label="搜索当前图面的测点与关联"
                placeholder="搜索图面测点与构件"
                className="spatial-search"
                value={markerQuery}
                onChange={(event) => setMarkerQuery(event.target.value)}
              />
            ) : (
              <TextField
                label="搜索当前图面的测点与关联"
                value={markerQuery}
                onChange={setMarkerQuery}
              />
            )}
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={activeOnly}
                onChange={(e) => setActiveOnly(e.target.checked)}
              />
              隐藏已退役对象
            </label>
          </div>
        )}
      </div>
      <div
        className={
          compact
            ? "spatial-canvas-stage"
            : "relative h-[560px] overflow-hidden rounded-xl border"
        }
      >
        <Suspense fallback={<Info>正在加载空间视图…</Info>}>
          {asset.kind === "layout" && page ? (
            <Layout key={page.id} page={page} version={version.id} {...scene} />
          ) : asset.kind === "model" && modelFile ? (
            <Model
              version={version.id}
              fileID={modelFile.id}
              elements={elements}
              {...scene}
            />
          ) : asset.kind === "geolayer" ? (
            <MapView
              objects={[]}
              features={features.data?.pages.flatMap((p) => p.items) ?? []}
              selected={owner}
              onSelect={(owner) => pick({ owner })}
              onHover={hover}
              onBBox={onBBox}
              basemap={settings.basemap}
              onBasemap={(basemap) => setSettings((s) => ({ ...s, basemap }))}
            />
          ) : (
            <div className="p-5">
              <Info>原件尚未完成处理，请在资料管理中上传并处理。</Info>
            </div>
          )}
        </Suspense>
        {children}
      </div>
      <Failure
        error={
          placements.error ??
          links.error ??
          features.error ??
          view.error ??
          saveView.error
        }
      />
      {pointItems.some((p) => p.basis_changed) ||
      linkItems.some((p) => p.basis_changed) ? (
        <Info>
          部分对象的工程位置或归属已变化；此固定版本仍保留原锚点，请核对后创建新版。
        </Info>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {placements.hasNextPage && (
          <Button
            variant="outline"
            onClick={() => void placements.fetchNextPage()}
          >
            载入更多测点
          </Button>
        )}
        {links.hasNextPage && (
          <Button variant="outline" onClick={() => void links.fetchNextPage()}>
            载入更多构件关联
          </Button>
        )}
        {features.hasNextPage && (
          <Button
            variant="outline"
            onClick={() => void features.fetchNextPage()}
          >
            载入更多图形
          </Button>
        )}
        {m.canSaveView && version.state === "published" && (
          <Button
            variant="outline"
            disabled={saveView.isPending || view.isError || !m.available}
            onClick={() => saveView.mutate()}
          >
            保存项目默认视图
          </Button>
        )}
      </div>
      {saveView.isSuccess && <Info>项目默认视图已保存。</Info>}
      {!compact &&
        owner &&
        (canPlace(owner.kind)
          ? selectedPoints.data?.items.length === 0
          : owner.kind === "component"
            ? selectedLinks.data?.items.length === 0
            : false) && (
          <Info>
            当前固定版本没有此对象的可读挂载；可通过“空间位置”查找其他载体。
          </Info>
        )}
      {!compact && (
        <p className="text-xs text-muted-foreground" aria-label="空间图例">
          蓝色：业务标记；橙色／金色：已选择；灰色：已退役。颜色不表示结构安全等级。
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        {asset.kind === "geolayer"
          ? `已载入 ${features.data?.pages.reduce((n, p) => n + p.items.length, 0) ?? 0} 个地理图形。业务关联按当前完整资源权限显示。`
          : `已载入 ${pointItems.length} 个测点、${linkItems.length} 个构件关联。标签、位置和监测状态来自当前可读对象。`}
      </p>
      {!compact && (
        <details className="text-sm">
          <summary className="cursor-pointer py-2 text-muted-foreground">
            资料与版本操作记录
          </summary>
          <div className="flex flex-wrap gap-2 py-2">
            <ObjectHistory objectType="spatial_asset" objectId={asset.id} />
            <ObjectHistory
              objectType="spatial_version"
              objectId={version.id}
              label="此版本操作历史"
            />
          </div>
        </details>
      )}
    </>
  );
}
