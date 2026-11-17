import { useCallback, useMemo, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Box,
  ChevronRight,
  Crosshair,
  FileImage,
  FolderOpen,
  Info as InfoIcon,
  Layers,
  Link2,
  MapPinned,
  Upload,
} from "lucide-react";
import {
  Permissions as P,
  type SpatialElement,
  type SpatialObjectRef,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@infrapro/ui/components/dialog";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { navigationPath } from "../resources/resource-navigation";
import { useSpatial } from "./context";
import { AssetManager } from "./asset-manager";
import { DraftEditor } from "./draft-editor";
import { ResourceMap, VersionCanvas } from "./canvas";
import { SpatialDirectory, SourceTree } from "./directories";
import { ObjectPanel } from "./object-panel";
import { Failure, Info } from "./common";
import {
  assetKinds,
  kinds,
  sectionForKind,
  type SpatialSearch,
  type Pick,
} from "./state";
import {
  facilityAssets,
  directLocations,
  allSpatialPages,
} from "./facility-model";
import "./facility-workspace.css";

type Props = {
  search: SpatialSearch;
  onSearch: (search: SpatialSearch) => void;
  facility?: string;
};
type Panel = "import" | "manage" | "detail" | "elements" | "locations";
const views = [
  { key: "layouts", kind: "layout", name: "布设图", Icon: FileImage },
  { key: "models", kind: "model", name: "BIM / 三维", Icon: Box },
  { key: "gis", kind: "geolayer", name: "GIS 地图", Icon: MapPinned },
] as const;

/** Tenant resource selection owns the workspace; assets never become a second navigation tree. */
export function FacilitySpatialWorkspace({
  search,
  onSearch,
  facility: fixedFacility,
}: Props) {
  const m = useSpatial(),
    cache = useQueryClient();
  const pickRequest = useRef(0);
  const [panel, setPanel] = useState<Panel>();
  const [sharedGIS, setSharedGIS] = useState(false);
  const [element, setElement] = useState("");
  const [elementQuery, setElementQuery] = useState("");
  const [bbox, setBBox] = useState("");
  const [notice, setNotice] = useState("");
  const [dirty, setDirty] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const [mainChoices, setMainChoices] = useState<Record<string, string>>({});
  const owner = useMemo<SpatialObjectRef | undefined>(
    () =>
      search.owner && search.owner_kind
        ? { id: search.owner, kind: search.owner_kind }
        : undefined,
    [search.owner, search.owner_kind],
  );
  const object = useQuery({
    queryKey: [...m.prefix, "object", owner],
    queryFn: ({ signal }) => m.client.spatialObject(m.scope, owner!, signal),
    enabled: !!owner && m.available,
    retry: false,
  });
  const linkedVersion = useQuery({
    queryKey: [...m.prefix, "entry-version", search.version],
    queryFn: ({ signal }) =>
      m.client.spatialManifest(m.scope, search.version!, signal),
    enabled: !!search.version && !search.asset && m.available,
    retry: false,
  });
  const linkedAssetID = search.asset || linkedVersion.data?.asset_id;
  const linkedAsset = useQuery({
    queryKey: [...m.prefix, "asset", linkedAssetID],
    queryFn: ({ signal }) =>
      m.client.spatialAsset(m.scope, linkedAssetID!, signal),
    enabled: !!linkedAssetID && m.available,
    retry: false,
  });
  const facilities = useQuery({
    queryKey: [...m.prefix, "workspace-facilities"],
    queryFn: ({ signal }) =>
      m.client.spatialObjects(
        m.scope,
        { kind: "facility", limit: 100 },
        signal,
      ),
    enabled:
      !owner &&
      !search.facility &&
      !fixedFacility &&
      !linkedAssetID &&
      !search.version &&
      m.available,
  });
  const facility =
    fixedFacility ||
    object.data?.facility_id ||
    (owner?.kind === "facility" ? owner.id : undefined) ||
    search.facility ||
    linkedAsset.data?.facility_id ||
    facilities.data?.items[0]?.id;
  const facilityObject = useQuery({
    queryKey: [...m.prefix, "facility-title", facility],
    queryFn: ({ signal }) =>
      m.client.spatialObject(
        m.scope,
        { kind: "facility", id: facility! },
        signal,
      ),
    enabled: !!facility && m.available,
  });
  const selected =
    owner ?? (facility ? { kind: "facility", id: facility } : undefined);
  const detail = useQuery({
    queryKey: [...m.prefix, "workspace-path", selected, facility],
    queryFn: ({ signal }) =>
      m.client.resourceDetail(
        m.scope,
        {
          kind: selected!.kind as "facility" | "component" | "monitoring_section" | "borehole" | "point",
          id: selected!.id,
          facility: facility!,
        },
        signal,
      ),
    enabled:
      !!selected &&
      !!facility &&
      m.available &&
      m.can(P.TenantEngineeringResourceRead),
    retry: false,
  });
  const path = detail.data ? navigationPath(detail.data) : [];
  const assetsQuery = useInfiniteQuery({
    queryKey: [...m.prefix, "facility-assets", facility, sharedGIS],
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      m.client.spatialAssets(
        m.scope,
        {
          facility_id: sharedGIS ? undefined : facility,
          kind: sharedGIS ? "geolayer" : undefined,
          cursor: pageParam,
          limit: 100,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: !!facility && m.available,
  });
  const assets = facilityAssets(
    assetsQuery.data?.pages.flatMap((p) => p.items) ?? [],
    facility,
    sharedGIS,
  );
  const section = sharedGIS
    ? "gis"
    : views.some((v) => v.key === search.section)
      ? search.section
      : linkedAsset.data
        ? sectionForKind(linkedAsset.data.kind)
        : "models";
  const kind = views.find((v) => v.key === section)?.kind ?? "model";
  const candidates = assets.filter((a) => a.kind === kind);
  const resourceMap =
    section === "gis" &&
    (search.map === "resources" || (!candidates.length && !search.asset));
  const asset = resourceMap
    ? undefined
    : linkedAsset.data &&
        (linkedAsset.data.facility_id === facility ||
          (!linkedAsset.data.facility_id &&
            linkedAsset.data.kind === "geolayer"))
      ? linkedAsset.data
      : (candidates.find(
          (a) => a.current_version_id && a.status !== "retired",
        ) ?? candidates[0]);
  const versions = useQuery({
    queryKey: [...m.prefix, "versions", asset?.id],
    queryFn: ({ signal }) =>
      m.client.spatialVersions(m.scope, asset!.id, { limit: 100 }, signal),
    enabled: !!asset && m.available,
  });
  const versionID =
    search.version ||
    asset?.current_version_id ||
    versions.data?.items.find((v) => v.state !== "withdrawn")?.id;
  const manifest = useQuery({
    queryKey: [...m.prefix, "manifest", versionID],
    queryFn: ({ signal }) =>
      m.client.spatialManifest(m.scope, versionID!, signal),
    enabled: !!versionID && m.available,
    retry: false,
  });
  const version =
    manifest.data?.asset_id === asset?.id ? manifest.data : undefined;
  const elementsQuery = useQuery({
    queryKey: [...m.prefix, "workspace-elements", version?.id],
    queryFn: ({ signal }) =>
      allSpatialPages<SpatialElement>((cursor) =>
        m.client.spatialElements(
          m.scope,
          version!.id,
          { limit: 100, cursor },
          signal,
        ),
      ),
    enabled: asset?.kind === "model" && !!version?.manifest_hash && m.available,
    retry: false,
  });
  const elements = elementsQuery.data?.items ?? [];
  const locations = useQuery({
    queryKey: [...m.prefix, "workspace-locations", selected],
    queryFn: ({ signal }) =>
      m.client.spatialLocate(m.scope, [selected!], signal),
    enabled: !!selected && m.available,
    retry: false,
  });
  const activeLocations = directLocations(
    locations.data?.items ?? [],
    selected,
    version?.id,
  );
  const ownLocation =
    selected?.kind !== "facility" && activeLocations.length > 0;
  const editing = search.mode === "edit" && !!version && m.canEdit;
  const managing = search.mode === "manage" || panel === "manage";
  const update = (next: Partial<SpatialSearch>) =>
    onSearch({ ...search, variant: undefined, ...next });
  const choose = (ref: SpatialObjectRef, fid?: string) => {
    pickRequest.current++;
    const nextFacility = fid || (ref.kind === "facility" ? ref.id : facility);
    const changed = nextFacility !== facility;
    setElement("");
    setNotice("");
    if (changed) {
      setSharedGIS(false);
      setBBox("");
    }
    update({
      owner: ref.id,
      owner_kind: ref.kind,
      facility: nextFacility,
      ...(changed
        ? {
            asset: undefined,
            version: undefined,
            page: undefined,
            view: undefined,
            map: undefined,
          }
        : {}),
    });
  };
  const chooseAsset = (id: string, vid?: string) => {
    pickRequest.current++;
    if (id === "resources") {
      setBBox("");
      update({
        asset: undefined,
        version: undefined,
        map: "resources",
        section: "gis",
      });
      return;
    }
    const next = assets.find((a) => a.id === id);
    setElement("");
    setBBox("");
    update({
      asset: id,
      version: vid,
      page: undefined,
      view: undefined,
      map: undefined,
      section: next ? sectionForKind(next.kind) : section,
    });
  };
  const pick = (value: Pick) => {
    setElement(value.element ?? "");
    if (value.owner) {
      choose(value.owner);
      setElement(value.element ?? "");
    } else if (value.element) {
      const request = ++pickRequest.current;
      void m.client
        .spatialComponentLinks(m.scope, version!.id, {
          element_key: value.element,
          limit: 100,
        })
        .then((result) => {
          if (request !== pickRequest.current) return;
          if (result.items[0]?.object) choose(result.items[0].object);
          setElement(value.element ?? "");
        })
        .catch((error) => setNotice(error.message));
    }
  };
  const edit = useMutation({
    mutationFn: async () => {
      if (!asset || !version) throw new Error("请先选择已完成处理的资料。");
      if (version.state === "draft") return version.id;
      const existing = versions.data?.items.find(
        (v) => v.state === "draft" && v.base_version_id === version.id,
      );
      if (existing) return existing.id;
      const result = await m.client.spatialCreateVersion(m.scope, asset.id, {
        request_id: crypto.randomUUID(),
        expected_revision: asset.revision,
        base_version_id: version.id,
        new_source: false,
        change_note: "从设施树调整空间关联",
      });
      return result.version_id || result.id;
    },
    onSuccess: async (next) => {
      await cache.invalidateQueries({ queryKey: m.prefix });
      setPanel(undefined);
      setDirty(false);
      update({ asset: asset!.id, version: next, mode: "edit" });
    },
  });
  const closeEditor = useCallback(() => {
    if (dirty) setConfirmClose(true);
    else onSearch({ ...search, mode: undefined });
  }, [dirty, onSearch, search]);
  const copyLink = () => {
    const url = new URL(window.location.href);
    const resolved = {
      facility,
      asset: asset?.id,
      version: version?.id,
      section,
      owner_kind: selected?.kind,
      owner: selected?.id,
    };
    for (const [key, value] of Object.entries(resolved))
      if (value) url.searchParams.set(key, value);
    url.searchParams.delete("mode");
    url.searchParams.delete("variant");
    void navigator.clipboard
      .writeText(url.toString())
      .then(() => setNotice("当前视图链接已复制。"))
      .catch(() => setNotice("浏览器未允许复制，请复制地址栏链接。"));
  };
  const selectedName = owner
    ? (object.data?.name ?? "正在读取对象…")
    : (facilityObject.data?.name ?? "选择设施");
  const label =
    resourceMap || (asset?.kind === "geolayer" && selected?.kind !== "facility")
      ? "地理位置见资源详情"
      : selected?.kind === "facility"
        ? "设施资料"
        : ownLocation
          ? "当前资料已定位"
          : "当前资料待定位";
  return (
    <div className="facility-spatial" aria-label="工程空间工作台">
      <header className="spatial-heading">
        <div>
          <h1>
            <MapPinned aria-hidden="true" />
            工程空间
          </h1>
          <p>沿设施层级查看空间资料，定位构件与测点。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={copyLink}>
            <Link2 className="size-4" />
            复制链接
          </Button>
          {m.canManage && (
            <Button
              disabled={!facility || !m.available}
              onClick={() => setPanel("import")}
            >
              <Upload className="size-4" />
              导入设施资料
            </Button>
          )}
        </div>
      </header>
      {notice && (
        <p className="spatial-notice" role="status">
          {notice}
        </p>
      )}
      {search.mode === "edit" && !m.canEdit && (
        <Info>当前账号没有空间关联编辑权限。</Info>
      )}
      <Failure
        error={
          object.error ??
          facilityObject.error ??
          linkedVersion.error ??
          linkedAsset.error ??
          edit.error
        }
      />
      <div className="spatial-workbench">
        <aside className="spatial-resource-panel" aria-label="工程资源树">
          <SpatialDirectory
            compact
            selected={selected}
            facility={facility}
            select={choose}
          />
        </aside>
        <section className="spatial-main-panel" aria-label="设施空间资料">
          <div className="spatial-context">
            <nav aria-label="当前资源路径" className="spatial-breadcrumb">
              {path.length ? (
                path.map((node, i) => (
                  <span key={node.id}>
                    {i > 0 && <ChevronRight className="size-3" />}
                    <button onClick={() => choose(node, node.facility_id)}>
                      {node.name}
                    </button>
                  </span>
                ))
              ) : (
                <span>{facilityObject.data?.name ?? "请选择左侧设施"}</span>
              )}
            </nav>
            <div className="spatial-selection">
              <div>
                <strong>{selectedName}</strong>
                <Badge variant="outline">
                  {kinds[selected?.kind ?? "facility"]}
                </Badge>
                <span
                  className={
                    ownLocation ? "spatial-located" : "text-muted-foreground"
                  }
                >
                  {label}
                </span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                disabled={!selected}
                onClick={() => setPanel("detail")}
              >
                <InfoIcon className="size-4" />
                对象详情
              </Button>
            </div>
          </div>
          <div className="spatial-viewbar">
            <div role="tablist" aria-label="空间视图">
              {views.map(({ key, name, Icon, kind: viewKind }) => (
                <button
                  role="tab"
                  aria-selected={section === key}
                  key={key}
                  onClick={() => {
                    setSharedGIS(false);
                    setElement("");
                    setBBox("");
                    update({
                      section: key,
                      asset: undefined,
                      version: undefined,
                      page: undefined,
                      view: undefined,
                      map: undefined,
                    });
                  }}
                >
                  <Icon className="size-4" />
                  {name}
                  <span>
                    {assets.filter((a) => a.kind === viewKind).length}
                  </span>
                </button>
              ))}
            </div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setSharedGIS(!sharedGIS);
                setBBox("");
                update({
                  map: undefined,
                  section: "gis",
                  asset: undefined,
                  version: undefined,
                  page: undefined,
                });
              }}
            >
              {sharedGIS ? "返回设施 GIS" : "共享 GIS"}
            </Button>
          </div>
          <div className="spatial-assetbar">
            <select
              aria-label="当前空间资料"
              value={resourceMap ? "resources" : (asset?.id ?? "")}
              onChange={(event) => chooseAsset(event.target.value)}
            >
              <option value="" disabled>
                {sharedGIS
                  ? "共享参考图层"
                  : candidates.length
                    ? "选择资料"
                    : "当前设施暂无此类资料"}
              </option>
              {section === "gis" && (
                <option value="resources">资源地理位置</option>
              )}
              {candidates.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                  {a.status === "retired" ? " · 已退役" : ""}
                </option>
              ))}
              {asset && !candidates.some((a) => a.id === asset.id) && (
                <option value={asset.id}>{asset.name}</option>
              )}
            </select>
            {version && (
              <Badge variant="outline">
                V{version.number} ·{" "}
                {version.state === "published"
                  ? "已发布"
                  : version.state === "draft"
                    ? "草稿"
                    : "已撤回"}
              </Badge>
            )}
            <Button
              size="sm"
              variant="ghost"
              disabled={!asset}
              onClick={() => setPanel("manage")}
            >
              <FolderOpen className="size-4" />
              资料与版本
            </Button>
            {asset?.kind === "model" && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setPanel("elements")}
              >
                <Layers className="size-4" />
                模型实体
              </Button>
            )}
          </div>
          <Failure
            error={
              assetsQuery.error ??
              versions.error ??
              manifest.error ??
              elementsQuery.error
            }
          />
          {assetsQuery.hasNextPage && (
            <Button
              variant="ghost"
              size="sm"
              disabled={assetsQuery.isFetchingNextPage}
              onClick={() => void assetsQuery.fetchNextPage()}
            >
              加载更多资料
            </Button>
          )}
          <div className="spatial-view-content">
            {resourceMap ? (
              <ResourceMap
                compact
                facility={facility}
                owner={selected}
                select={choose}
                hover={() => {}}
                bbox={bbox}
                onBBox={setBBox}
              />
            ) : assetsQuery.isPending || (manifest.isPending && !!versionID) ? (
              <Info>正在读取设施空间资料…</Info>
            ) : asset && version ? (
              <VersionCanvas
                compact
                key={version.id}
                asset={asset}
                version={version}
                elements={elements}
                owner={selected}
                element={element}
                mainChoices={mainChoices}
                pageID={search.page}
                onPage={(page) => update({ page })}
                onView={() => {}}
                pick={pick}
                hover={() => {}}
                bbox={bbox}
                onBBox={setBBox}
              />
            ) : (
              <div className="spatial-empty">
                <FolderOpen className="size-10" />
                <h2>
                  {sharedGIS
                    ? "暂无共享 GIS 图层"
                    : `为${facilityObject.data?.name ?? "当前设施"}准备${assetKinds[kind]}`}
                </h2>
                <p>资料导入设施后，可沿左侧资源树逐层关联构件与测点。</p>
                {m.canManage && (
                  <Button
                    disabled={!facility}
                    onClick={() => setPanel("import")}
                  >
                    导入空间资料
                  </Button>
                )}
              </div>
            )}
          </div>
          <footer className="spatial-bindingbar">
            <div>
              <strong>
                {resourceMap
                  ? "地理坐标直接来自当前设施的资源"
                  : !asset?.facility_id && asset?.kind === "geolayer"
                    ? "共享 GIS 参考图层"
                    : asset?.kind === "geolayer"
                      ? "当前设施的 GIS 图层"
                      : selected?.kind === "facility"
                        ? "资料归属此设施"
                        : ownLocation
                          ? `当前版本有 ${activeLocations.length} 处显式定位`
                          : "可查看设施资料 · 此对象尚未在当前版本定位"}
              </strong>
              <p>
                {selected?.kind === "facility"
                  ? "展开左侧设施，选择构件或测点建立空间关联。"
                  : asset?.kind === "geolayer"
                    ? "GIS 图形关联设施；构件与测点的位置在资源详情中维护。"
                    : "构件关联图纸区域或模型实体；测点使用独立空间坐标。"}
              </p>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={!selected}
                onClick={() => setPanel("locations")}
              >
                全部空间关联
              </Button>
              {m.canEdit &&
                !resourceMap &&
                (asset?.kind !== "geolayer" ||
                  selected?.kind === "facility") && (
                  <Button
                    size="sm"
                    disabled={
                      !version?.manifest_hash ||
                      !m.available ||
                      edit.isPending ||
                      asset?.status === "retired" ||
                      (selected?.kind === "facility" &&
                        asset?.kind !== "geolayer")
                    }
                    onClick={() => edit.mutate()}
                  >
                    <Crosshair className="size-4" />
                    {edit.isPending
                      ? "准备草稿…"
                      : ownLocation
                        ? "调整定位"
                        : "建立关联"}
                  </Button>
                )}
            </div>
          </footer>
        </section>
      </div>
      <ResponsivePanel
        open={!!panel || managing}
        onOpenChange={(open) => {
          if (!open) {
            setPanel(undefined);
            if (search.mode === "manage") update({ mode: undefined });
          }
        }}
        title={
          panel === "import"
            ? `导入资料 · ${facilityObject.data?.name ?? "设施"}`
            : panel === "detail"
              ? "对象详情"
              : panel === "elements"
                ? "模型实体"
                : panel === "locations"
                  ? "全部空间关联"
                  : "资料与版本"
        }
        description={
          panel === "import"
            ? "登记资料、上传原件、核对空间关联后发布。"
            : selectedName
        }
        size={
          panel === "detail" || panel === "elements" || panel === "locations"
            ? "default"
            : "wide"
        }
      >
        {panel === "detail" ? (
          <ObjectPanel
            owner={selected}
            element={elements.find((e) => e.key === element)}
            version={version?.id}
            onLocate={() => setPanel("locations")}
            mainChannel={
              selected
                ? mainChoices[`${selected.kind}:${selected.id}`]
                : undefined
            }
            onMainChannel={(id) =>
              selected &&
              setMainChoices((current) => ({
                ...current,
                [`${selected.kind}:${selected.id}`]: id,
              }))
            }
          />
        ) : panel === "elements" ? (
          <>
            <input
              className="spatial-search"
              aria-label="搜索模型实体"
              placeholder="名称或 IFC 类型"
              value={elementQuery}
              onChange={(e) => setElementQuery(e.target.value)}
            />
            <p className="my-3 text-xs text-muted-foreground">
              {elements.length} 个实体 · 选中后在画布中高亮
            </p>
            <SourceTree
              elements={elements.filter((e) =>
                `${e.name} ${e.type}`
                  .toLowerCase()
                  .includes(elementQuery.toLowerCase()),
              )}
              selected={element}
              select={(key) => {
                pick({ element: key });
                setPanel(undefined);
              }}
            />
          </>
        ) : panel === "locations" ? (
          <>
            <Failure error={locations.error} />
            {locations.isPending ? (
              <Info>正在查找空间关联…</Info>
            ) : !locations.data?.items.length ? (
              <Info>
                此对象尚无已发布的空间关联。可以在当前设施的资料上建立定位。
              </Info>
            ) : (
              locations.data.items.map((l, i) => (
                <button
                  className="spatial-location"
                  key={`${l.version_id}:${i}`}
                  onClick={() => {
                    setSharedGIS(!l.asset.facility_id);
                    update({
                      asset: l.asset.id,
                      map: undefined,
                      version: l.version_id,
                      section: sectionForKind(l.asset.kind),
                      page: l.page_id || undefined,
                      mode: undefined,
                    });
                    setElement(l.element_key ?? "");
                    setPanel(undefined);
                  }}
                >
                  <strong>{l.asset.name}</strong>
                  <span>
                    {assetKinds[l.asset.kind]} · V{l.number}{" "}
                    {l.version_id === l.asset.current_version_id
                      ? "· 当前版本"
                      : "· 历史版本"}
                  </span>
                </button>
              ))
            )}
          </>
        ) : (
          <AssetManager
            key={panel === "import" ? "new" : asset?.id}
            facility={facility}
            asset={panel === "import" ? undefined : asset}
            version={panel === "import" ? undefined : version}
            onSelect={(id, vid) => {
              chooseAsset(id, vid);
              setPanel("manage");
            }}
            onEdit={() => edit.mutate()}
          />
        )}
      </ResponsivePanel>
      <ResponsivePanel
        open={editing}
        onOpenChange={(open) => {
          if (!open) closeEditor();
        }}
        title={`空间关联 · ${selectedName}`}
        description="保存草稿后核对发布，已发布版本保持原有关系。"
        size="workspace"
      >
        {editing && version && (
          <DraftEditor
            version={version.id}
            initialOwner={selected}
            onDirtyChange={setDirty}
            onClose={closeEditor}
            onReview={() => {
              update({ mode: "manage" });
              setPanel("manage");
            }}
          />
        )}
      </ResponsivePanel>
      <Dialog open={confirmClose} onOpenChange={setConfirmClose}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃未保存的修改？</DialogTitle>
            <DialogDescription>
              已保存的草稿会保留，本次尚未保存的调整将丢弃。
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setConfirmClose(false)}>
              继续编辑
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmClose(false);
                setDirty(false);
                update({ mode: undefined });
              }}
            >
              放弃修改
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
