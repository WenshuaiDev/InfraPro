import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  MapPinned,
  Layers,
  Box,
  FolderTree,
  LayoutDashboard,
  FileImage,
  Crosshair,
  FolderOpen,
  Upload,
  X,
} from "lucide-react";
import { type SpatialObjectRef, type SpatialAsset } from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { useSpatial } from "./context";
import { FacilitySpatialWorkspace } from "./facility-workspace";
import { AssetManager } from "./asset-manager";
import { DraftEditor } from "./draft-editor";
import { ObjectPanel, HoverSummary } from "./object-panel";
import { Failure, Info, TextField } from "./common";
import {
  assetKinds,
  canPlace,
  inputClass,
  objectKey,
  sectionForKind,
  type SpatialSearch,
  type SpatialSection,
  type Pick,
} from "./state";
import { SpatialDirectory, SourceTree } from "./directories";
import { ResourceMap, VersionCanvas } from "./canvas";
import {
  FacilityScope,
  SpatialOverview,
  AssetLibrary,
} from "./workspace-catalog";
type Props = {
  search: SpatialSearch;
  onSearch: (s: SpatialSearch) => void;
  facility?: string;
};
export function SpatialWorkspace(props: Props) {
  const m = useSpatial();
  if (!m.canRead) return <Info>当前身份没有空间浏览权限。</Info>;
  return m.scope.kind === "tenant" ? <FacilitySpatialWorkspace {...props} /> : <Workspace {...props} />;
}
function Workspace({ search, onSearch, facility }: Props) {
  const m = useSpatial(),
    [assetQuery, setAssetQuery] = useState(""),
    [tab, setTab] = useState<"resources" | "assets" | "elements">("resources"),
    [elementKey, setElementKey] = useState(""),
    [resolveElement, setResolveElement] = useState(""),
    [mainChoices, setMainChoices] = useState<Record<string, string>>({}),
    [elementQuery, setElementQuery] = useState(""),
    [hover, setHover] = useState<SpatialObjectRef>(),
    [locating, setLocating] = useState(false),
    [notice, setNotice] = useState(""),
    [mapRange, setMapRange] = useState({
      version: search.version ?? "",
      bbox: "",
    }),
    timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined),
    resolvedElement = useRef("");
  const bbox = mapRange.version === (search.version ?? "") ? mapRange.bbox : "";
  const setBBox = useCallback(
    (bbox: string) => setMapRange({ version: search.version ?? "", bbox }),
    [search.version],
  );
  const owner = useMemo(
    () =>
      search.owner && search.owner_kind
        ? { kind: search.owner_kind, id: search.owner }
        : undefined,
    [search.owner, search.owner_kind],
  );
  const section: SpatialSection =
    search.section ??
    (search.mode === "manage"
      ? "assets"
      : search.asset || search.version
        ? "layouts"
        : search.owner
          ? "gis"
          : "overview");
  const desiredKind =
    section === "layouts"
      ? "layout"
      : section === "models"
        ? "model"
        : section === "gis"
          ? "geolayer"
          : undefined;
  const manage = search.mode === "manage";
  const assets = useInfiniteQuery({
    queryKey: [...m.prefix, "assets", assetQuery, facility, desiredKind],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      m.client.spatialAssets(
        m.scope,
        {
          q: assetQuery,
          facility_id: facility,
          kind: desiredKind,
          cursor: pageParam,
          limit: 25,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available && section !== "overview" && section !== "assets",
    retry: false,
  });
  const manifest = useQuery({
    queryKey: [...m.prefix, "manifest", search.version],
    queryFn: ({ signal }) =>
      m.client.spatialManifest(m.scope, search.version!, signal),
    enabled: !!search.version && m.available,
    retry: false,
  });
  // Fixed-version links from audit/tasks need no guessed parent ID. Resolve it
  // from the authorized manifest without navigating away from another page.
  const editing =
    search.mode === "edit" &&
    m.canEdit &&
    manifest.data?.state === "draft" &&
    !!manifest.data.manifest_hash;
  const activeAsset =
    manifest.data && manifest.data.id === search.version
      ? manifest.data.asset_id
      : search.asset;
  const asset = useQuery({
    queryKey: [...m.prefix, "asset", activeAsset],
    queryFn: ({ signal }) =>
      m.client.spatialAsset(m.scope, activeAsset!, signal),
    enabled: !!activeAsset && m.available,
    retry: false,
  });
  const versions = useQuery({
    queryKey: [...m.prefix, "versions", activeAsset],
    queryFn: ({ signal }) =>
      m.client.spatialVersions(m.scope, activeAsset!, { limit: 100 }, signal),
    enabled: !!activeAsset && m.available,
    retry: false,
  });
  const elements = useInfiniteQuery({
    queryKey: [...m.prefix, "elements", search.version, elementQuery],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      m.client.spatialElements(
        m.scope,
        search.version!,
        { q: elementQuery, cursor: pageParam, limit: 100 },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled:
      !!search.version &&
      asset.data?.kind === "model" &&
      !!manifest.data?.manifest_hash &&
      m.available,
    retry: false,
  });
  const element = useQuery({
    queryKey: [...m.prefix, "element", search.version, elementKey],
    queryFn: ({ signal }) =>
      m.client.spatialElements(
        m.scope,
        search.version!,
        { element_key: elementKey, limit: 1 },
        signal,
      ),
    enabled: !!search.version && !!elementKey && m.available,
    retry: false,
  });
  const object = useQuery({
    queryKey: [...m.prefix, "object", owner],
    queryFn: ({ signal }) => m.client.spatialObject(m.scope, owner!, signal),
    enabled: !!owner && m.available,
    retry: false,
  });
  const locations = useQuery({
    queryKey: [...m.prefix, "locations", owner],
    queryFn: ({ signal }) => m.client.spatialLocate(m.scope, [owner!], signal),
    enabled: !!owner && m.available,
    retry: false,
  });
  const chooseAsset = useCallback(
    (id: string, version?: string) => {
      setElementKey("");
      setHover(undefined);
      onSearch({
        ...search,
        asset: id,
        version,
        section:
          search.section ?? (search.mode === "manage" ? "assets" : undefined),
        page: undefined,
        view: undefined,
      });
    },
    [onSearch, search],
  );
  useEffect(() => {
    if (search.asset && !search.version && asset.data && versions.data) {
      const id =
        asset.data.current_version_id ||
        versions.data.items.find((v) => v.state !== "withdrawn")?.id;
      if (id) onSearch({ ...search, version: id });
    }
  }, [search, onSearch, asset.data, versions.data]);
  useEffect(() => {
    if (!search.section && asset.data && search.mode !== "manage")
      onSearch({ ...search, section: sectionForKind(asset.data.kind) });
  }, [search, asset.data, onSearch]);
  const select = (o: SpatialObjectRef) => {
    setLocating(false);
    setElementKey("");
    onSearch({
      ...search,
      owner: o.id,
      owner_kind: o.kind,
      facility: search.facility,
    });
  };
  const hoverObject = useCallback((o?: SpatialObjectRef) => {
    clearTimeout(timer.current);
    if (!o) {
      setHover(undefined);
      return;
    }
    timer.current = setTimeout(() => setHover(o), 220);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  const pick = (p: Pick) => {
    resolvedElement.current = "";
    setResolveElement(p.element && !p.owner ? p.element : "");
    if (p.element) setElementKey(p.element);
    if (p.owner) {
      setLocating(false);
      onSearch({ ...search, owner: p.owner.id, owner_kind: p.owner.kind });
    } else if (p.element) {
      onSearch({ ...search, owner: undefined, owner_kind: undefined });
    }
  };
  const share = async () => {
    try {
      await navigator.clipboard.writeText(location.href);
      setNotice(
        search.version
          ? "固定版本链接已复制，打开时会重新核验权限。"
          : "当前页面链接已复制，打开时会重新核验权限。",
      );
    } catch {
      setNotice("请复制地址栏中的当前页面链接。");
    }
  };
  const loadedElements = elements.data?.pages.flatMap((p) => p.items) ?? [];
  const resolvedLinks = useQuery({
    queryKey: [...m.prefix, "source-links", search.version, resolveElement],
    queryFn: ({ signal }) =>
      m.client.spatialComponentLinks(
        m.scope,
        search.version!,
        { element_key: resolveElement, limit: 1 },
        signal,
      ),
    enabled: !!resolveElement && !!search.version && m.available,
    retry: false,
  });
  useEffect(() => {
    const stamp = `${search.version}:${resolveElement}`;
    if (
      !resolveElement ||
      !resolvedLinks.data ||
      resolvedElement.current === stamp
    )
      return;
    resolvedElement.current = stamp;
    const link = resolvedLinks.data.items[0];
    if (link)
      onSearch({
        ...search,
        owner: link.component_id,
        owner_kind: "component",
      });
  }, [resolveElement, resolvedLinks.data, onSearch, search]);
  const defaultViewID = search.view || search.version;
  const projectView = useQuery({
    queryKey: [...m.prefix, "view", defaultViewID],
    queryFn: ({ signal }) =>
      m.client.spatialView(m.scope, defaultViewID!, signal),
    enabled: m.scope.kind === "project" && !!defaultViewID && m.available,
    retry: false,
  });
  const selectedPlacement = useQuery({
    queryKey: [...m.prefix, "selected-placements", search.version, owner],
    queryFn: ({ signal }) =>
      m.client.spatialPlacements(
        m.scope,
        search.version!,
        { owner_id: owner!.id, kind: owner!.kind, limit: 100 },
        signal,
      ),
    enabled: !!search.version && canPlace(owner?.kind) && m.available,
    retry: false,
  });
  const selectedLink = useQuery({
    queryKey: [...m.prefix, "selected-links", search.version, owner],
    queryFn: ({ signal }) =>
      m.client.spatialComponentLinks(
        m.scope,
        search.version!,
        { owner_id: owner!.id, kind: owner!.kind, limit: 100 },
        signal,
      ),
    enabled: !!search.version && owner?.kind === "component" && m.available,
    retry: false,
  });
  const mainKey = owner ? objectKey(owner) : "";
  const choicePrefix = `${search.version}:`;
  const viewChoices = Object.fromEntries(
    Object.entries(mainChoices)
      .filter(([key]) => key.startsWith(choicePrefix))
      .map(([key, id]) => [key.slice(choicePrefix.length), id]),
  );
  const currentMain =
    viewChoices[mainKey] ??
    (projectView.data && projectView.data.version_id === search.version
      ? projectView.data.settings.main_channels?.[mainKey]
      : undefined) ??
    manifest.data?.settings.main_channels?.[mainKey] ??
    selectedPlacement.data?.items.find(
      (p) => !search.page || p.page_id === search.page,
    )?.main_channel_id ??
    selectedLink.data?.items[0]?.main_channel_id;
  const cache = useQueryClient();
  const clone = useMutation({
    mutationFn: () =>
      m.client.spatialCreateVersion(m.scope, asset.data!.id, {
        request_id: crypto.randomUUID(),
        expected_revision: asset.data!.revision,
        base_version_id: manifest.data!.id,
        new_source: false,
        change_note: "维护测点标注与构件关联",
      }),
    onSuccess: async (r) => {
      await cache.invalidateQueries({ queryKey: m.prefix });
      onSearch({
        ...search,
        section: "placements",
        mode: "edit",
        version: r.version_id || r.id,
      });
    },
  });
  const go = (next: SpatialSection, target?: SpatialAsset) => {
    setElementKey("");
    setHover(undefined);
    setLocating(false);
    const located =
      owner && !target
        ? locations.data?.items.find(
            (l) => sectionForKind(l.asset.kind) === next,
          )
        : undefined;
    const keep =
      !target &&
      (next === "placements" ||
        next === "assets" ||
        (asset.data && sectionForKind(asset.data.kind) === next));
    onSearch({
      ...search,
      section: next,
      mode: undefined,
      q: undefined,
      kind: undefined,
      status: undefined,
      asset:
        target?.id ?? located?.asset.id ?? (keep ? activeAsset : undefined),
      version:
        target?.current_version_id ??
        located?.version_id ??
        (keep ? search.version : undefined),
      page: located?.page_id || undefined,
      view: undefined,
    });
  };
  const importAsset = () =>
    onSearch({
      ...search,
      section: "assets",
      mode: "manage",
      asset: undefined,
      version: undefined,
      page: undefined,
      view: undefined,
    });
  const openManager = (a?: SpatialAsset) =>
    onSearch({
      ...search,
      section: "assets",
      mode: "manage",
      asset: a?.id ?? activeAsset,
      version: a?.current_version_id ?? search.version,
    });
  const sections = [
    { key: "overview", label: "空间总览", icon: LayoutDashboard },
    { key: "gis", label: "GIS 地图", icon: MapPinned },
    { key: "layouts", label: "布设图", icon: FileImage },
    { key: "models", label: "BIM / 三维", icon: Box },
    { key: "placements", label: "测点标注", icon: Crosshair },
    { key: "assets", label: "资料与版本", icon: FolderOpen },
  ] as const;
  const loadedAssets = assets.data?.pages.flatMap((p) => p.items) ?? [];
  const managerVersion = manifest.error
    ? versions.data?.items.find((v) => v.id === search.version)
    : (manifest.data ??
      versions.data?.items.find((v) => v.id === search.version));
  const versionReady =
    search.version &&
    manifest.data &&
    asset.data &&
    manifest.data.asset_id === asset.data.id &&
    !manifest.error &&
    !asset.error;
  const hasSelection = !!owner || !!elementKey;
  const hoverCard = hover && (
    <HoverSummary
      key={objectKey(hover)}
      owner={hover}
      mainChannel={
        viewChoices[objectKey(hover)] ??
        projectView.data?.settings.main_channels?.[objectKey(hover)] ??
        manifest.data?.settings.main_channels?.[objectKey(hover)]
      }
    />
  );
  return (
    <section className="min-w-0 space-y-5" aria-label="工程空间工作台">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <MapPinned className="size-6 text-primary" aria-hidden />
            <h1 className="text-2xl font-semibold">工程空间</h1>
            <Badge variant="outline">
              {m.scope.kind === "tenant" ? "租户空间" : "项目空间"}
            </Badge>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            查看设施位置、布设图和三维模型，在空间中定位测点与监测数据。
          </p>
        </div>
        <div className="flex max-w-full flex-wrap items-center gap-2">
          <FacilityScope
            value={facility}
            onChange={(id) =>
              onSearch({
                section,
                facility: id,
                owner: section === "gis" ? id : undefined,
                owner_kind: section === "gis" && id ? "facility" : undefined,
              })
            }
          />
          <Button variant="outline" onClick={() => void share()}>
            复制当前视图链接
          </Button>
          {m.canManage && (
            <Button onClick={importAsset}>
              <Upload className="size-4" aria-hidden />
              导入空间资料
            </Button>
          )}
        </div>
      </header>
      <nav
        aria-label="工程空间功能"
        className="flex max-w-full gap-1 overflow-x-auto border-b"
      >
        {sections.map(({ key, label, icon: Icon }) => (
          <button
            type="button"
            key={key}
            aria-current={section === key ? "page" : undefined}
            onClick={() => go(key)}
            className={`flex min-h-12 shrink-0 items-center gap-2 border-b-2 px-4 text-sm font-medium transition-colors focus-visible:ring-2 ${section === key ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:bg-muted hover:text-foreground"}`}
          >
            <Icon className="size-4" aria-hidden />
            {label}
          </button>
        ))}
      </nav>
      {notice && <Info>{notice}</Info>}
      {section === "overview" ? (
        <SpatialOverview
          facility={facility}
          go={go}
          importAsset={importAsset}
        />
      ) : section === "assets" ? (
        <AssetLibrary
          search={search}
          onSearch={onSearch}
          onOpen={(a) =>
            onSearch({
              ...search,
              mode: "manage",
              asset: a.id,
              version: a.current_version_id || undefined,
            })
          }
          onBrowse={(a) => go(sectionForKind(a.kind), a)}
        />
      ) : editing && search.version ? (
        <DraftEditor
          key={search.version}
          version={search.version}
          onClose={() =>
            onSearch({ ...search, section: "placements", mode: undefined })
          }
          onReview={() =>
            onSearch({ ...search, section: "assets", mode: "manage" })
          }
        />
      ) : (
        <>
          {section === "placements" && (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-card p-4">
              <div>
                <h2 className="font-semibold">测点标注与构件关联</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  选择图纸或模型，在草稿中维护位置，核对后发布。已发布版本保持不变。
                </p>
              </div>
              {versionReady &&
              manifest.data.state === "draft" &&
              m.canEdit &&
              !!manifest.data.manifest_hash ? (
                <Button onClick={() => onSearch({ ...search, mode: "edit" })}>
                  编辑挂载、关联与默认视图
                </Button>
              ) : versionReady &&
                manifest.data.state === "published" &&
                m.canManage &&
                m.canEdit &&
                asset.data.status !== "retired" ? (
                <Button
                  disabled={clone.isPending}
                  onClick={() => clone.mutate()}
                >
                  {clone.isPending ? "正在创建…" : "基于此版本创建标注草稿"}
                </Button>
              ) : null}
              <Failure error={clone.error} />
            </div>
          )}
          <div
            className={`grid min-w-0 items-start gap-4 ${hasSelection ? "2xl:grid-cols-[240px_minmax(0,1fr)_340px] xl:grid-cols-[220px_minmax(0,1fr)]" : "xl:grid-cols-[250px_minmax(0,1fr)]"}`}
          >
            <aside
              className="min-w-0 overflow-hidden rounded-xl border bg-card"
              aria-label="空间目录"
            >
              <div className="space-y-3 border-b p-3">
                <h2 className="text-sm font-semibold">
                  {section === "layouts"
                    ? "布设图目录"
                    : section === "models"
                      ? "模型目录"
                      : section === "placements"
                        ? "标注载体"
                        : "地图与参考图层"}
                </h2>
                <TextField
                  label="搜索空间资料"
                  value={assetQuery}
                  onChange={setAssetQuery}
                />
                {section === "gis" && (
                  <Button
                    className="w-full justify-start"
                    variant={!activeAsset ? "secondary" : "ghost"}
                    onClick={() =>
                      onSearch({
                        ...search,
                        asset: undefined,
                        version: undefined,
                        page: undefined,
                        view: undefined,
                      })
                    }
                  >
                    资源地理位置
                  </Button>
                )}
                <div className="max-h-56 space-y-1 overflow-auto">
                  <Failure error={assets.error} />
                  {assets.isPending ? (
                    <Info>正在读取资料…</Info>
                  ) : !loadedAssets.length ? (
                    <Info>
                      {section === "gis"
                        ? "暂无参考图层，可查看资源地理位置。"
                        : "暂无匹配的图模资料。"}
                    </Info>
                  ) : (
                    loadedAssets.map((a) => (
                      <button
                        key={a.id}
                        type="button"
                        aria-current={activeAsset === a.id ? "true" : undefined}
                        className={`w-full rounded-md p-2 text-left text-sm hover:bg-muted focus-visible:ring-2 ${activeAsset === a.id ? "bg-primary/10 text-primary" : ""}`}
                        onClick={() =>
                          chooseAsset(a.id, a.current_version_id || undefined)
                        }
                      >
                        <span className="block font-medium">{a.name}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {assetKinds[a.kind]}
                          {a.status === "retired" ? " · 已退役" : ""}
                        </span>
                      </button>
                    ))
                  )}
                  {assets.hasNextPage && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={assets.isFetchingNextPage}
                      onClick={() => void assets.fetchNextPage()}
                    >
                      更多资料
                    </Button>
                  )}
                </div>
              </div>
              <nav
                className="flex flex-wrap gap-1 border-b p-2"
                aria-label="目录内容"
              >
                <Button
                  size="sm"
                  variant={tab !== "elements" ? "secondary" : "ghost"}
                  onClick={() => setTab("resources")}
                >
                  <FolderTree className="size-4" aria-hidden />
                  资源
                </Button>
                {asset.data?.kind === "model" && (
                  <Button
                    size="sm"
                    variant={tab === "elements" ? "secondary" : "ghost"}
                    onClick={() => setTab("elements")}
                  >
                    <Box className="size-4" aria-hidden />
                    源构件
                  </Button>
                )}
              </nav>
              <div className="max-h-80 overflow-auto xl:max-h-[520px]">
                {tab !== "elements" || asset.data?.kind !== "model" ? (
                  <SpatialDirectory
                    selected={owner}
                    facility={object.data?.facility_id ?? facility}
                    select={select}
                  />
                ) : (
                  <div className="space-y-3 p-3">
                    <TextField
                      label="搜索源构件名称或标识"
                      value={elementQuery}
                      onChange={setElementQuery}
                    />
                    <Failure error={elements.error} />
                    <SourceTree
                      elements={loadedElements}
                      selected={elementKey}
                      select={(key) => pick({ element: key })}
                    />
                    {elements.hasNextPage && (
                      <Button
                        variant="outline"
                        disabled={elements.isFetchingNextPage}
                        onClick={() => void elements.fetchNextPage()}
                      >
                        载入更多源构件
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </aside>
            <div className="min-w-0 space-y-3">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="mr-auto min-w-0 break-words font-semibold">
                  {asset.data?.name ??
                    (section === "gis"
                      ? "资源地理位置"
                      : "选择一份空间资料开始浏览")}
                </h2>
                {activeAsset && (
                  <label className="flex max-w-full items-center gap-2 text-sm">
                    <span className="shrink-0">固定版本</span>
                    <select
                      className={`${inputClass} !w-auto max-w-full`}
                      value={search.version ?? ""}
                      onChange={(e) => chooseAsset(activeAsset, e.target.value)}
                    >
                      <option value="">请选择版本</option>
                      {manifest.data &&
                        !versions.data?.items.some(
                          (v) => v.id === manifest.data.id,
                        ) && (
                          <option value={manifest.data.id}>
                            V{manifest.data.number} ·{" "}
                            {manifest.data.state === "published"
                              ? "已发布"
                              : manifest.data.state === "draft"
                                ? "草稿"
                                : "已撤回"}
                          </option>
                        )}
                      {versions.data?.items.map((v) => (
                        <option key={v.id} value={v.id}>
                          V{v.number} ·{" "}
                          {v.state === "draft"
                            ? "草稿"
                            : v.state === "withdrawn"
                              ? "已撤回"
                              : "已发布"}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
                {activeAsset && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => openManager()}
                  >
                    管理资料
                  </Button>
                )}
              </div>
              <Failure
                error={asset.error ?? versions.error ?? manifest.error}
              />
              {versionReady ? (
                <VersionCanvas
                  key={search.version}
                  version={manifest.data}
                  asset={asset.data}
                  owner={owner}
                  element={elementKey}
                  elements={loadedElements}
                  pageID={search.page}
                  viewID={defaultViewID}
                  mainChoices={viewChoices}
                  onPage={(page) => onSearch({ ...search, page })}
                  onView={(view) => onSearch({ ...search, view })}
                  pick={pick}
                  hover={hoverObject}
                  onBBox={setBBox}
                  bbox={bbox}
                >
                  {hoverCard}
                </VersionCanvas>
              ) : !activeAsset && !search.version && section === "gis" ? (
                <ResourceMap
                  owner={owner}
                  select={(o) => select(o)}
                  hover={hoverObject}
                  bbox={bbox}
                  onBBox={setBBox}
                >
                  {hoverCard}
                </ResourceMap>
              ) : !manifest.error && !asset.error ? (
                <div className="flex min-h-96 flex-col items-center justify-center gap-4 rounded-xl border border-dashed bg-muted/20 p-6 text-center">
                  <Layers
                    className="size-12 text-muted-foreground"
                    strokeWidth={1}
                  />
                  <h3 className="font-medium">
                    {manifest.isFetching
                      ? "正在读取固定版本…"
                      : activeAsset
                        ? "这份资料还没有可浏览的版本"
                        : "从左侧选择布设图或模型"}
                  </h3>
                  <p className="max-w-lg text-sm leading-6 text-muted-foreground">
                    {activeAsset
                      ? "打开资料与版本，创建草稿、上传原件并完成处理后即可预览。"
                      : "图纸支持多页浏览和测点标注；BIM 与三维模型支持构件查询和空间定位。"}
                  </p>
                  {m.canManage && (
                    <Button
                      variant="outline"
                      onClick={() =>
                        activeAsset ? openManager() : importAsset()
                      }
                    >
                      {activeAsset ? "准备资料版本" : "导入空间资料"}
                    </Button>
                  )}
                </div>
              ) : null}
              {owner && (
                <section
                  className="space-y-2 rounded-lg border p-3"
                  aria-label="可用空间位置"
                >
                  <div className="flex items-center gap-3">
                    <h3 className="text-sm font-medium">同一对象的空间位置</h3>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setLocating(!locating)}
                    >
                      {locating ? "收起位置" : "查看图纸 / 模型位置"}
                    </Button>
                  </div>
                  {locating && (
                    <>
                      <Failure error={locations.error} />
                      {locations.isPending ? (
                        <Info>正在查找可读挂载…</Info>
                      ) : !locations.data?.items.length ? (
                        <Info>
                          当前没有可读的布设图或模型挂载。工程位置仍保留在资源详情。
                        </Info>
                      ) : (
                        <div className="flex flex-wrap gap-2">
                          {locations.data.items.map((l, i) => (
                            <Button
                              key={`${l.version_id}:${l.page_id}:${i}`}
                              className="h-auto min-h-11 whitespace-normal"
                              variant="outline"
                              onClick={() => {
                                setElementKey(l.element_key ?? "");
                                onSearch({
                                  ...search,
                                  section: sectionForKind(l.asset.kind),
                                  asset: l.asset.id,
                                  version: l.version_id,
                                  page: l.page_id || undefined,
                                  view: undefined,
                                });
                                setLocating(false);
                              }}
                            >
                              {l.asset.name} · V{l.number}
                              {l.page_id ? ` · ${l.page_id}` : ""}
                            </Button>
                          ))}
                        </div>
                      )}
                    </>
                  )}
                </section>
              )}
            </div>
            {hasSelection && (
              <div className="min-w-0 space-y-2 xl:col-start-2 2xl:col-start-auto">
                <div className="flex justify-end">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setElementKey("");
                      onSearch({
                        ...search,
                        owner: undefined,
                        owner_kind: undefined,
                      });
                    }}
                  >
                    <X className="size-4" />
                    收起对象详情
                  </Button>
                </div>
                <ObjectPanel
                  key={`${search.version ?? "map"}:${owner ? objectKey(owner) : elementKey}`}
                  owner={owner}
                  element={element.data?.items[0]}
                  version={search.version}
                  mainChannel={currentMain}
                  onMainChannel={(id) =>
                    setMainChoices((previous) => ({
                      ...previous,
                      [choicePrefix + mainKey]: id,
                    }))
                  }
                  onLocate={() => setLocating(true)}
                />
              </div>
            )}
          </div>
        </>
      )}
      <ResponsivePanel
        open={manage}
        onOpenChange={(open) => {
          if (!open) onSearch({ ...search, mode: undefined });
        }}
        title={asset.data ? `${asset.data.name} · 资料与版本` : "导入空间资料"}
        description={
          asset.data
            ? "管理原件、处理任务、固定版本与发布记录。"
            : "先登记资料信息，再创建版本、上传原件并处理。"
        }
        size="wide"
      >
        {manage &&
          (activeAsset ? (
            asset.data ? (
              <AssetManager
                key={activeAsset}
                asset={asset.data}
                version={managerVersion}
                facility={facility}
                onSelect={chooseAsset}
                onEdit={() =>
                  onSearch({ ...search, section: "placements", mode: "edit" })
                }
              />
            ) : (
              <>
                <Failure error={asset.error} />
                {asset.isPending && <Info>正在读取资料…</Info>}
              </>
            )
          ) : m.canManage ? (
            <AssetManager
              key="new"
              facility={facility}
              onSelect={chooseAsset}
              onEdit={() =>
                onSearch({ ...search, section: "placements", mode: "edit" })
              }
            />
          ) : (
            <Info>当前身份不可新建资料。</Info>
          ))}
      </ResponsivePanel>
    </section>
  );
}
