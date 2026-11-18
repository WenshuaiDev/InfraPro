import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { readSpatialMetadata } from "./metadata-reader";
import { useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Permissions as P, type SpatialAsset } from "@infrapro/api-client";
import {
  ArrowRight,
  Box,
  FileImage,
  MapPinned,
  Crosshair,
  Upload,
  FolderOpen,
  Users,
} from "lucide-react";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { useSpatial } from "./context";
import {
  Failure,
  Field,
  Info,
  SpatialLink,
  TextField,
  useBlobURL,
  useSpatialFile,
} from "./common";
import {
  assetKinds,
  inputClass,
  type SpatialSearch,
  type SpatialSection,
} from "./state";

export function FacilityScope({
  value,
  onChange,
}: {
  value?: string;
  onChange: (id?: string) => void;
}) {
  const m = useSpatial(),
    [open, setOpen] = useState(false),
    [q, setQ] = useState("");
  const selected = useQuery({
    queryKey: [...m.prefix, "facility-scope", value],
    queryFn: ({ signal }) =>
      m.client.spatialObject(m.scope, { kind: "facility", id: value! }, signal),
    enabled: !!value && m.available,
    retry: false,
  });
  const list = usePagedQuery({ initialPageSize: 30, queryKey: [...m.prefix, "facility-picker", q], enabled: open && m.available, retry: false, queryFn: ({ signal, cursor , limit}) =>
      m.client.spatialObjects(
        m.scope,
        { kind: "facility", q, cursor, limit },
        signal,
      ) });
  return (
    <>
      <Button
        variant="outline"
        className="max-w-full"
        onClick={() => setOpen(true)}
      >
        <MapPinned className="size-4 shrink-0" />
        <span className="truncate">
          {value ? (selected.data?.name ?? "当前设施") : "全部设施"}
        </span>
      </Button>
      <ResponsivePanel
        open={open}
        onOpenChange={setOpen}
        title="选择设施范围"
        description="按设施查看图纸、模型与标注资料。GIS 地图仍可浏览当前权限内的地理位置。"
      >
        <div className="space-y-4">
          <TextField label="搜索设施名称或编码" value={q} onChange={setQ} />
          <Button
            variant={!value ? "secondary" : "outline"}
            onClick={() => {
              onChange(undefined);
              setOpen(false);
            }}
          >
            全部设施
          </Button>
          <Failure error={list.error} />
          {list.isPending && <Info>正在读取设施…</Info>}
          {list.data?.items
            .map((o) => (
              <button
                type="button"
                key={o.id}
                className="flex min-h-14 w-full items-center justify-between gap-3 rounded-lg border p-3 text-left text-sm hover:bg-muted focus-visible:ring-2"
                onClick={() => {
                  onChange(o.id);
                  setOpen(false);
                }}
              >
                <span>
                  {o.name}
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {o.code}
                    {o.retired ? " · 已退役" : ""}
                  </span>
                </span>
                {o.id === value && <Badge variant="secondary">当前</Badge>}
              </button>
            ))}
          {!list.isPending &&
            !list.error &&
            !list.data?.items.length && <Info>没有匹配的设施。</Info>}
          {list.data && (
            <ListPagination label="设施" {...list.pagination} />
          )}
        </div>
      </ResponsivePanel>
    </>
  );
}

function LayoutThumbnail({ asset }: { asset?: SpatialAsset }) {
  const m = useSpatial(),
    version = asset?.current_version_id;
  const manifest = useQuery({
    queryKey: [...m.prefix, "manifest", version],
    queryFn: ({ signal }) =>
      m.client.spatialManifest(m.scope, version!, signal),
    enabled: !!version && m.available,
    retry: false,
  });
  const first = manifest.data?.manifest.pages[0],
    file = first?.file_id;
  const blob = useSpatialFile(version ?? "", file),
    url = useBlobURL(blob.data);
  return url ? (
    <img
      src={url}
      alt={`${asset?.name}首页预览`}
      className="h-full w-full object-contain p-3"
    />
  ) : (
    <FileImage className="size-16 text-primary/60" strokeWidth={1} />
  );
}

export function SpatialOverview({
  facility,
  go,
  importAsset,
}: {
  facility?: string;
  go: (section: SpatialSection, asset?: SpatialAsset) => void;
  importAsset: () => void;
}) {
  const m = useSpatial();
  const assets = useInfiniteQuery({
    queryKey: [...m.prefix, "overview-assets", facility],
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      m.client.spatialAssets(
        m.scope,
        { facility_id: facility, cursor: pageParam, limit: 30 },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available,
    retry: false,
  });
  const items = assets.data?.pages.flatMap((p) => p.items) ?? [],
    usable = items.filter((a) => a.status !== "retired");
  const cards = [
    {
      section: "gis" as const,
      title: "GIS 地图",
      lead: "从地理位置找到设施",
      description: "查看设施、构件和测点的位置，叠加已发布的参考图层。",
      icon: MapPinned,
      kind: "geolayer",
      tone: "bg-emerald-50 dark:bg-emerald-950/30",
    },
    {
      section: "layouts" as const,
      title: "布设图",
      lead: "在图纸上查看测点",
      description: "浏览平面、立面与断面图，按图页定位测点和构件。",
      icon: FileImage,
      kind: "layout",
      tone: "bg-blue-50 dark:bg-blue-950/30",
    },
    {
      section: "models" as const,
      title: "BIM / 三维",
      lead: "从模型理解监测位置",
      description: "查看模型构件和源属性，剖切、隔离并联动监测数据。",
      icon: Box,
      kind: "model",
      tone: "bg-indigo-50 dark:bg-indigo-950/30",
    },
  ];
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">从哪里开始查看？</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            先选空间载体，再选设施、构件或测点。切换图纸与模型时可继续定位同一个对象。
          </p>
        </div>
        {m.scope.kind === "project" && (
          <Badge variant="outline">项目已授权的固定版本</Badge>
        )}
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        {cards.map((c) => {
          const Icon = c.icon,
            a = usable.find((a) => a.kind === c.kind && a.current_version_id);
          return (
            <article
              key={c.section}
              className="min-w-0 overflow-hidden rounded-xl border bg-card"
            >
              <div
                className={`flex h-40 items-center justify-center border-b ${c.tone}`}
                aria-hidden={c.kind !== "layout"}
              >
                {c.kind === "layout" ? (
                  <LayoutThumbnail asset={a} />
                ) : (
                  <Icon className="size-16 text-primary/60" strokeWidth={1} />
                )}
              </div>
              <div className="space-y-3 p-5">
                <div className="flex items-center gap-2">
                  <Icon className="size-5 text-primary" />
                  <h3 className="font-semibold">{c.title}</h3>
                </div>
                <p className="text-base font-medium">{c.lead}</p>
                <p className="min-h-10 text-sm leading-6 text-muted-foreground">
                  {c.description}
                </p>
                <Button
                  variant="outline"
                  className="w-full justify-between"
                  onClick={() =>
                    go(c.section, c.section === "gis" ? undefined : a)
                  }
                >
                  进入{c.title}
                  <ArrowRight className="size-4" />
                </Button>
              </div>
            </article>
          );
        })}
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.65fr)]">
        <section className="min-w-0 rounded-xl border bg-card">
          <div className="flex items-center justify-between border-b p-4">
            <h3 className="font-semibold">当前范围内的空间资料</h3>
            <Button size="sm" variant="ghost" onClick={() => go("assets")}>
              查看全部
              <ArrowRight className="size-4" />
            </Button>
          </div>
          <Failure error={assets.error} />
          {assets.isPending ? (
            <Info>正在读取空间资料…</Info>
          ) : !items.length ? (
            <div className="space-y-3 p-6">
              <p className="text-sm text-muted-foreground">
                尚无可读的图纸、模型或参考图层。设施位置仍可在 GIS 地图查看。
              </p>
              {m.canManage && (
                <Button onClick={importAsset}>导入第一份空间资料</Button>
              )}
            </div>
          ) : (
            <div className="divide-y">
              {items.slice(0, 6).map((a) => (
                <button
                  type="button"
                  key={a.id}
                  className="flex min-h-16 w-full items-center justify-between gap-3 px-4 py-3 text-left hover:bg-muted/50 focus-visible:ring-2"
                  onClick={() =>
                    go(
                      a.kind === "layout"
                        ? "layouts"
                        : a.kind === "model"
                          ? "models"
                          : "gis",
                      a,
                    )
                  }
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">
                      {a.name}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {assetKinds[a.kind]} · {a.code}
                    </span>
                  </span>
                  <Badge variant="outline">
                    {a.status === "retired"
                      ? "已退役"
                      : a.current_version_id
                        ? "有当前版本"
                        : "待准备版本"}
                  </Badge>
                </button>
              ))}
            </div>
          )}
        </section>
        <section className="space-y-3 rounded-xl border bg-card p-5">
          <h3 className="font-semibold">维护空间资料</h3>
          <p className="text-sm leading-6 text-muted-foreground">
            导入图纸或模型 → 处理原件 → 标注与核对 → 发布版本 → 授权项目使用。
          </p>
          {m.canManage && (
            <Button
              variant="outline"
              className="w-full justify-start"
              onClick={importAsset}
            >
              <Upload className="size-4" />
              导入空间资料
            </Button>
          )}
          <Button
            variant="outline"
            className="w-full justify-start"
            onClick={() => go("placements")}
          >
            <Crosshair className="size-4" />
            {m.canEdit ? "维护测点标注与构件关联" : "查看测点标注"}
          </Button>
          <Button
            variant="outline"
            className="w-full justify-start"
            onClick={() => go("assets")}
          >
            <FolderOpen className="size-4" />
            资料、版本与处理任务
          </Button>
          <ProjectAccessEntry />
        </section>
      </div>
    </div>
  );
}

export function AssetLibrary({
  search,
  onSearch,
  onOpen,
  onBrowse,
}: {
  search: SpatialSearch;
  onSearch: (s: SpatialSearch) => void;
  onOpen: (asset: SpatialAsset) => void;
  onBrowse: (asset: SpatialAsset) => void;
}) {
  const m = useSpatial();
  const list = usePagedQuery({ initialPageSize: 25, queryKey: [
      ...m.prefix,
      "library",
      search.facility,
      search.q,
      search.kind,
      search.status,
    ], enabled: m.available, retry: false, queryFn: ({ signal, cursor , limit}) =>
      m.client.spatialAssets(
        m.scope,
        {
          facility_id: search.facility,
          q: search.q,
          kind: search.kind,
          state: search.status,
          cursor,
          limit,
        },
        signal,
      ) });
  const items = list.data?.items ?? [];
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">资料与版本</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            在这里导入、处理、发布图模，以及管理历史版本和项目授权。
          </p>
        </div>
        <ProjectAccessEntry />
      </div>
      <div className="grid items-end gap-3 rounded-xl border bg-card p-4 sm:grid-cols-[minmax(180px,1fr)_180px_150px]">
        <TextField
          label="搜索资料名称或编码"
          value={search.q ?? ""}
          onChange={(q) => onSearch({ ...search, q: q || undefined })}
        />
        <Field label="资料类型筛选">
          <select
            className={inputClass}
            value={search.kind ?? ""}
            onChange={(e) =>
              onSearch({ ...search, kind: e.target.value || undefined })
            }
          >
            <option value="">全部类型</option>
            {Object.entries(assetKinds).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </Field>
        <Field label="资料状态筛选">
          <select
            className={inputClass}
            value={search.status ?? ""}
            onChange={(e) =>
              onSearch({ ...search, status: e.target.value || undefined })
            }
          >
            <option value="">全部状态</option>
            <option value="normal">正常</option>
            <option value="retired">已退役</option>
          </select>
        </Field>
      </div>
      <Failure error={list.error} />
      {list.isPending ? (
        <Info>正在读取资料…</Info>
      ) : !items.length ? (
        <Info>没有符合当前范围的空间资料。可调整设施或筛选条件。</Info>
      ) : (
        <div className="overflow-x-auto rounded-xl border">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="p-3 font-medium">资料</th>
                <th className="p-3 font-medium">类型</th>
                <th className="p-3 font-medium">当前版本</th>
                <th className="p-3 font-medium">状态</th>
                <th className="p-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((a) => (
                <tr key={a.id} className="hover:bg-muted/30">
                  <td className="p-3">
                    <button
                      type="button"
                      className="text-left font-medium hover:underline"
                      onClick={() => onOpen(a)}
                    >
                      {a.name}
                    </button>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {a.code}
                    </p>
                  </td>
                  <td className="p-3 whitespace-nowrap">
                    {assetKinds[a.kind]}
                  </td>
                  <td className="p-3">
                    <CurrentVersion asset={a} />
                  </td>
                  <td className="p-3">
                    <Badge variant="outline">
                      {a.status === "retired" ? "已退役" : "正常"}
                    </Badge>
                  </td>
                  <td className="p-3">
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onBrowse(a)}
                      >
                        浏览
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => onOpen(a)}
                      >
                        资料与版本
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {list.data && (
        <ListPagination label="资料" {...list.pagination} />
      )}
      <p className="text-xs text-muted-foreground">
        已载入 {items.length} 份资料{list.pagination.hasNext ? "，还有更多结果" : ""}
        。项目使用明确授权的版本，后续发布不会自动升级项目版本。
      </p>
    </section>
  );
}
function CurrentVersion({ asset }: { asset: SpatialAsset }) {
  const m = useSpatial(),
    version = useQuery({
      queryKey: [...m.prefix, "manifest", asset.current_version_id],
      queryFn: ({ signal }) =>
        readSpatialMetadata(signal, () =>
          m.client.spatialManifest(m.scope, asset.current_version_id!, signal),
        ),
      enabled: !!asset.current_version_id && m.available,
      retry: false,
    });
  return (
    <span className="whitespace-nowrap">
      {!asset.current_version_id
        ? "未设置"
        : version.data
          ? `V${version.data.number} · 已发布`
          : version.error
            ? "版本不可读"
            : "正在读取…"}
    </span>
  );
}
export function ProjectAccessEntry() {
  const m = useSpatial(),
    [open, setOpen] = useState(false),
    [q, setQ] = useState("");
  const allowed =
    m.scope.kind === "tenant" &&
    m.can(P.TenantProjectRead) &&
    m.can(P.TenantProjectSpatialAccessRead);
  const list = usePagedQuery({ initialPageSize: 25, queryKey: [...m.prefix, "access-projects", q], enabled: allowed && open && m.available, retry: false, queryFn: ({ signal, cursor , limit}) =>
      m.client.projectGovernance(
        m.scope.tenantId,
        { q, status: "all", limit, cursor },
        signal,
      ) });
  if (!allowed) return null;
  return (
    <>
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Users className="size-4" />
        项目空间授权
      </Button>
      <ResponsivePanel
        open={open}
        onOpenChange={setOpen}
        title="选择项目空间授权"
        description="按项目管理可浏览的固定版本与原件下载权限。"
      >
        <div className="space-y-4">
          <TextField label="搜索授权项目" value={q} onChange={setQ} />
          <Failure error={list.error} />
          {list.isPending && <Info>正在读取项目…</Info>}
          {list.data?.items
            .map(({ project: p }) => (
              <SpatialLink
                key={p.id}
                className="block rounded-lg border p-4 text-sm hover:bg-muted"
                to={`${m.base}/projects/${p.id}/spatial-access`}
              >
                <span className="font-medium">{p.name}</span>
                <span className="mt-1 block text-muted-foreground">
                  {p.code} · {p.status === "archived" ? "已归档" : "运行中"}
                </span>
              </SpatialLink>
            ))}
          {!list.isPending &&
            !list.error &&
            !list.data?.items.length && (
              <Info>没有符合条件的项目。</Info>
            )}
          {list.data && (
            <ListPagination label="项目" {...list.pagination} />
          )}
        </div>
      </ResponsivePanel>
    </>
  );
}
