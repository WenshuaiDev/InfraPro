import { DomainFiles } from "../files/domain-files";
import { lazy, Suspense, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type {
  SpatialDraft,
  SpatialPlacement,
  SpatialComponentLink,
  SpatialSettings,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import { UnsavedChanges } from "../derived-monitoring/common";
import { useSpatial } from "./context";
import { Field, TextField, Failure, Info } from "./common";
import { inputClass, type Pick, type SceneProps } from "./state";
const Layout = lazy(() => import("./layout-viewer")),
  Model = lazy(() => import("./model-viewer"));
export function DraftEditor({
  version,
  onClose,
  onReview,
}: {
  version: string;
  onClose: () => void;
  onReview: () => void;
}) {
  const m = useSpatial();
  const q = useQuery({
    queryKey: [...m.prefix, "draft", version],
    queryFn: ({ signal }) => m.client.spatialDraft(m.scope, version, signal),
    enabled: m.available,
    retry: false,
  });
  return (
    <>
      <Failure error={q.error} />
      {q.isPending ? (
        <Info>正在读取可编辑草稿…</Info>
      ) : (
        q.data && (
          <Editor
            key={`${q.data.version.id}:${q.data.version.revision}`}
            initial={q.data}
            onClose={onClose}
            onReview={onReview}
          />
        )
      )}
    </>
  );
}
function Editor({
  initial,
  onClose,
  onReview,
}: {
  initial: SpatialDraft;
  onClose: () => void;
  onReview: () => void;
}) {
  const m = useSpatial(),
    cache = useQueryClient(),
    [placements, setPlacements] = useState(initial.placements),
    [links, setLinks] = useState(initial.component_links),
    [features, setFeatures] = useState(initial.feature_links),
    [settings, setSettings] = useState(initial.version.settings),
    [clearCandidates, setClearCandidates] = useState(false),
    [pageID, setPageID] = useState(
      initial.version.settings.page_id ||
        initial.version.manifest.pages?.[0]?.id ||
        "",
    ),
    [pick, setPick] = useState<Pick>(),
    [mode, setMode] = useState<"point" | "component">("point"),
    [selected, setSelected] = useState(""),
    [query, setQuery] = useState(""),
    [coordinates, setCoordinates] = useState(""),
    [label, setLabel] = useState("12,-12"),
    [main, setMain] = useState(""),
    [featureKey, setFeatureKey] = useState(""),
    [localError, setLocalError] = useState<unknown>();
  const kind = initial.asset.kind,
    geolayer = kind === "geolayer";
  const targets = useInfiniteQuery({
    initialPageParam: "",
    queryKey: [...m.prefix, "edit-targets", initial.asset.id, mode, query],
    queryFn: ({ signal, pageParam }) =>
      m.client.spatialObjects(
        m.scope,
        {
          kind: geolayer ? "facility" : mode,
          facility_id: initial.asset.facility_id,
          q: query,
          state: "normal",
          cursor: pageParam,
          limit: 50,
        },
        signal,
      ),
    getNextPageParam: (p) => p.next_cursor || undefined,
    enabled: m.available,
    retry: false,
  });
  const targetItems = targets.data?.pages.flatMap((p) => p.items) ?? [];
  const selectedTarget = useQuery({
    queryKey: [...m.prefix, "edit-target", mode, selected, geolayer],
    queryFn: ({ signal }) =>
      m.client.spatialObject(
        m.scope,
        { kind: geolayer ? "facility" : mode, id: selected },
        signal,
      ),
    enabled: !!selected && m.available,
    retry: false,
  });
  const target =
    targetItems.find((o) => o.id === selected) ?? selectedTarget.data;
  const channels = useQuery({
    queryKey: [...m.prefix, "edit-channels", mode, selected],
    queryFn: ({ signal }) =>
      m.client.monitoringDirectory(
        m.scope,
        { owner_kind: geolayer ? "facility" : mode, owner_id: selected },
        signal,
      ),
    enabled: !!selected && m.available,
    retry: false,
  });
  const fields = { placements, links, features, settings, clearCandidates },
    dirty =
      JSON.stringify(fields) !==
      JSON.stringify({
        placements: initial.placements,
        links: initial.component_links,
        features: initial.feature_links,
        settings: initial.version.settings,
        clearCandidates: false,
      });
  const changeSettings = (patch: Partial<SpatialSettings>) =>
    setSettings((s) => ({ ...s, ...patch }));
  const save = useMutation({
    mutationFn: () =>
      m.client.spatialEditDraft(m.scope, initial.version.id, {
        request_id: crypto.randomUUID(),
        expected_revision: initial.version.revision,
        placements: placements
          .filter(
            (p) =>
              !initial.placements.some(
                (old) => JSON.stringify(old) === JSON.stringify(p),
              ),
          )
          .map((p) => ({
            ...p,
            id: initial.placements.some((old) => old.id === p.id) ? p.id : "",
            object: undefined,
          })),
        remove_placements: initial.placements
          .filter((p) => !placements.some((v) => v.id === p.id))
          .map((p) => p.id),
        component_links: links
          .filter(
            (p) =>
              !initial.component_links.some(
                (old) => JSON.stringify(old) === JSON.stringify(p),
              ),
          )
          .map((p) => ({
            ...p,
            id: initial.component_links.some((old) => old.id === p.id)
              ? p.id
              : "",
            object: undefined,
          })),
        remove_component_links: initial.component_links
          .filter((p) => !links.some((v) => v.id === p.id))
          .map((p) => p.id),
        feature_links: features,
        remove_feature_links: initial.feature_links
          .filter((p) => !features.some((v) => v.feature_key === p.feature_key))
          .map((p) => p.feature_key),
        settings,
        clear_candidates: clearCandidates,
      }),
    onSuccess: () => cache.invalidateQueries({ queryKey: m.prefix }),
  });
  const onPick = (v: Pick) => {
    setPick(v);
    if (v.page) setPageID(v.page);
    if (v.anchor)
      setCoordinates(v.anchor.map((n) => Number(n.toFixed(6))).join(","));
    if (v.owner) {
      setMode(v.owner.kind === "component" ? "component" : "point");
      setSelected(v.owner.id);
      const p = placements.find(
        (p) => p.point_id === v.owner?.id && (!v.page || p.page_id === v.page),
      );
      setMain("");
      const linked = links.find(
        (l) =>
          l.component_id === v.owner?.id &&
          (!v.element || l.element_key === v.element) &&
          (!v.page || l.page_id === v.page),
      );
      if (linked) {
        setMain(linked.main_channel_id ?? "");
        setPick({ ...v, region: linked.region, element: linked.element_key });
      }
      if (p) {
        setCoordinates(p.anchor.join(","));
        setLabel(p.label_offset.join(","));
        setMain(p.main_channel_id ?? "");
      }
    }
  };
  const add = () => {
    try {
      setLocalError(undefined);
      if (!target || target.retired) throw Error("请选择当前可用的业务对象。");
      if (geolayer) {
        if (!featureKey) throw Error("请选择 GeoJSON 图形。");
        setFeatures([
          ...features.filter((v) => v.feature_key !== featureKey),
          { feature_key: featureKey, facility_id: target.id },
        ]);
        return;
      }
      if (mode === "point") {
        if (
          coordinates.split(",").some((n) => !n.trim()) ||
          label.split(",").some((n) => !n.trim())
        )
          throw Error("请填写完整坐标与标签偏移。");
        const anchor = coordinates.split(",").map(Number),
          offset = label.split(",").map(Number);
        if (
          anchor.length !== (kind === "layout" ? 2 : 3) ||
          offset.length !== 2 ||
          [...anchor, ...offset].some((n) => !Number.isFinite(n))
        )
          throw Error("请填写正确维数的有限坐标。");
        if (kind === "layout" && anchor.some((n) => n < 0 || n > 1))
          throw Error("图纸坐标须在 0–1 范围。");
        const old = placements.find(
          (p) =>
            p.point_id === selected &&
            (kind !== "layout" || p.page_id === pageID),
        );
        const p: SpatialPlacement = {
          id: old?.id ?? crypto.randomUUID(),
          point_id: selected,
          page_id: kind === "layout" ? pageID : "",
          anchor,
          element_key:
            kind === "model" ? (pick?.element ?? old?.element_key ?? "") : "",
          label_offset: offset,
          main_channel_id: main || undefined,
          basis_hash: target.basis_hash,
          basis_changed: false,
          object: target,
        };
        setPlacements([...placements.filter((v) => v !== old), p]);
      } else {
        if (kind === "layout" && !pick?.region)
          throw Error("请在图纸上绘制构件区域，双击完成。");
        if (kind === "model" && !pick?.element)
          throw Error("请点击模型或选择源构件。");
        const old = links.find((l) =>
          kind === "model"
            ? l.element_key === pick!.element
            : l.component_id === selected && l.page_id === pageID,
        );
        const link: SpatialComponentLink = {
          id: old?.id ?? crypto.randomUUID(),
          component_id: selected,
          element_key: kind === "model" ? pick!.element! : "",
          page_id: kind === "layout" ? pageID : "",
          region: pick?.region ?? [],
          main_channel_id: main || undefined,
          basis_hash: target.basis_hash,
          basis_changed: false,
          object: target,
        };
        setLinks([...links.filter((l) => l !== old), link]);
      }
    } catch (e) {
      setLocalError(e);
    }
  };
  const scene: SceneProps = {
    placements,
    links,
    selected: target,
    element: pick?.element,
    settings,
    onPick,
    onHover: () => {},
    editing: true,
    placing: mode,
    onMove: (id, anchor, label) =>
      setPlacements((v) =>
        v.map((p) => (p.id === id ? { ...p, anchor, label_offset: label } : p)),
      ),
    onHiddenElements: (hidden_elements) => changeSettings({ hidden_elements }),
    onCamera: (camera) => changeSettings({ camera }),
  };
  const page = initial.version.manifest.pages?.find((p) => p.id === pageID),
    modelFile = initial.version.manifest.files?.find(
      (f) => f.media_type === "model/gltf-binary",
    );
  return (
    <section className="min-w-0 space-y-4" aria-label="编辑空间挂载">
      <UnsavedChanges dirty={dirty} />
      {modelFile && (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer py-2">当前模型浏览文件</summary>
          <DomainFiles
            files={[
              {
                ref: {
                  kind: "spatial.file",
                  scope: m.scope,
                  versionId: initial.version.id,
                  fileId: modelFile.id,
                },
                display_name: modelFile.name,
                media_type: modelFile.media_type,
                size_bytes: String(modelFile.size),
                sha256: modelFile.sha256,
                state: initial.version.content_available
                  ? "available"
                  : "missing",
                allowed_actions: [],
              },
            ]}
            actions={() => null}
          />
        </details>
      )}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">
            {initial.asset.name} · 编辑 V{initial.version.number} 草稿
          </h2>
          <p className="text-sm text-muted-foreground">
            选择业务对象 → 在画布放置或关联 → 保存草稿 → 核对发布。
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={dirty ? "secondary" : "outline"}>
            {dirty ? "有未保存修改" : "草稿已保存"}
          </Badge>
          <Button
            disabled={!dirty || save.isPending || !m.available}
            onClick={() => save.mutate()}
          >
            {save.isPending ? "正在保存…" : "保存草稿"}
          </Button>
          {m.canPublish && (
            <Button
              variant="outline"
              disabled={dirty || save.isPending}
              onClick={onReview}
            >
              核对与发布
            </Button>
          )}
          <Button variant="outline" disabled={save.isPending} onClick={onClose}>
            取消编辑
          </Button>
        </div>
      </div>
      <Failure error={save.error ?? localError} />
      <div className="grid min-w-0 items-start gap-4 xl:grid-cols-[240px_minmax(0,1fr)] 2xl:grid-cols-[240px_minmax(0,1fr)_280px]">
        <aside
          className="min-w-0 space-y-3 rounded-xl border bg-card p-3"
          aria-label="标注对象目录"
        >
          <h3 className="text-sm font-semibold">业务对象与标注</h3>{" "}
          {!geolayer && (
            <Field label="编辑对象类型">
              <select
                className={inputClass}
                value={mode}
                onChange={(e) => {
                  setMode(e.target.value as "point" | "component");
                  setSelected("");
                  setPick(undefined);
                }}
              >
                <option value="point">测点挂载</option>
                <option value="component">构件关联</option>
              </select>
            </Field>
          )}
          <TextField label="搜索可关联对象" value={query} onChange={setQuery} />
          <Field label="业务对象">
            <select
              className={inputClass}
              value={selected}
              onChange={(e) => {
                const id = e.target.value;
                setSelected(id);
                setMain("");
                setLabel("12,-12");
                const placed = placements.find(
                  (p) =>
                    p.point_id === id &&
                    (kind !== "layout" || p.page_id === pageID),
                );
                if (placed)
                  onPick({
                    owner: { kind: "point", id },
                    anchor: placed.anchor,
                    page: placed.page_id,
                    element: placed.element_key,
                  });
                else {
                  setCoordinates("");
                  setPick(undefined);
                }
              }}
            >
              <option value="">请选择对象</option>
              {selectedTarget.data &&
                !targetItems.some((o) => o.id === selected) && (
                  <option value={selected}>
                    {selectedTarget.data.name} · {selectedTarget.data.code}
                  </option>
                )}
              {targetItems.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name} · {o.code}
                </option>
              ))}
            </select>
          </Field>
          {targets.hasNextPage && (
            <Button
              variant="outline"
              size="sm"
              disabled={targets.isFetchingNextPage}
              onClick={() => void targets.fetchNextPage()}
            >
              载入更多可关联对象
            </Button>
          )}
          <Failure error={targets.error ?? selectedTarget.error} />
          <div className="max-h-72 space-y-2 overflow-auto border-t pt-3">
            <h3 className="font-medium">已挂载与关联</h3>
            {placements.map((p, i) => (
              <div
                key={p.id || `new-${i}`}
                className="flex items-center justify-between gap-2 text-sm"
              >
                <button
                  className="min-h-11 text-left underline"
                  onClick={() =>
                    onPick({
                      owner: { kind: "point", id: p.point_id },
                      page: p.page_id,
                      anchor: p.anchor,
                      element: p.element_key,
                    })
                  }
                >
                  {p.object?.name ?? "测点"}
                  {p.page_id ? ` · ${p.page_id}` : ""}
                  {p.basis_changed ? " · 依据变化" : ""}
                </button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() =>
                    setPlacements(placements.filter((v) => v !== p))
                  }
                >
                  移除
                </Button>
              </div>
            ))}
            {links.map((l, i) => (
              <div
                key={l.id || `new-link-${i}`}
                className="flex items-center justify-between gap-2 text-sm"
              >
                <button
                  type="button"
                  className="min-h-11 text-left underline"
                  onClick={() =>
                    onPick({
                      owner: { kind: "component", id: l.component_id },
                      page: l.page_id,
                      element: l.element_key,
                      region: l.region,
                    })
                  }
                >
                  {l.object?.name ?? "构件"} · {l.element_key || "图纸区域"}
                </button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setLinks(links.filter((v) => v !== l))}
                >
                  解除
                </Button>
              </div>
            ))}
            {features.map((f) => (
              <div
                key={f.feature_key}
                className="flex items-center justify-between text-sm"
              >
                <span>{f.feature_key}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setFeatures(features.filter((v) => v !== f))}
                >
                  解除
                </Button>
              </div>
            ))}
          </div>
        </aside>
        <div className="min-w-0 space-y-3">
          {kind === "layout" && (
            <Field label="编辑图页">
              <select
                className={inputClass}
                value={pageID}
                onChange={(e) => {
                  setPageID(e.target.value);
                  setPick(undefined);
                }}
              >
                {initial.version.manifest.pages?.map((p) => (
                  <option key={p.id} value={p.id}>
                    第 {p.number} 页 · {p.width}×{p.height}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <div className="h-[540px] overflow-hidden rounded-xl border bg-muted/20">
            <Suspense fallback={<Info>正在加载编辑视图…</Info>}>
              {kind === "layout" && page ? (
                <Layout
                  key={page.id}
                  page={page}
                  version={initial.version.id}
                  {...scene}
                />
              ) : kind === "model" ? (
                <Model
                  version={initial.version.id}
                  fileID={modelFile?.id}
                  elements={initial.version.manifest.elements ?? []}
                  {...scene}
                />
              ) : (
                <div className="p-4">
                  <Info>
                    图层坐标由 GeoJSON
                    原件提供。选择图形后关联设施；地理位置请在资源详情维护。
                  </Info>
                  <Field label="图层图形">
                    <select
                      className={inputClass}
                      value={featureKey}
                      onChange={(e) => setFeatureKey(e.target.value)}
                    >
                      <option value="">请选择图形</option>
                      {initial.version.manifest.features?.map((f) => (
                        <option key={f.id} value={f.id}>
                          {f.properties.name ?? f.id}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>
              )}
            </Suspense>
          </div>
          {kind === "model" && (
            <Field label="源构件（与点击模型等价）">
              <select
                className={inputClass}
                value={pick?.element ?? ""}
                onChange={(e) => setPick({ element: e.target.value })}
              >
                <option value="">请选择源构件</option>
                {initial.version.manifest.elements
                  ?.filter((e) => e.fragments.length)
                  .map((e) => (
                    <option key={e.key} value={e.key}>
                      {e.name} · {e.type}
                    </option>
                  ))}
              </select>
            </Field>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={settings.source_properties}
                onChange={(e) =>
                  changeSettings({ source_properties: e.target.checked })
                }
              />
              发布源属性（包含源文件中的信息）
            </label>
            {kind === "layout" && (
              <Button
                variant="outline"
                onClick={() => changeSettings({ page_id: pageID })}
              >
                将当前图页设为默认
              </Button>
            )}
          </div>
        </div>
        <aside
          className="min-w-0 space-y-3 rounded-xl border bg-card p-4 xl:col-start-2 2xl:col-start-auto"
          aria-label="标注属性"
        >
          <h3 className="text-sm font-semibold">标注属性</h3>
          {!geolayer && mode === "point" && (
            <>
              <TextField
                label={
                  kind === "layout"
                    ? "锚点 x,y（0–1，左上角为原点）"
                    : "锚点 x,y,z（视图坐标，米）"
                }
                value={coordinates}
                onChange={setCoordinates}
              />
              <TextField
                label="标签偏移 dx,dy（像素）"
                value={label}
                onChange={setLabel}
              />
              <Info>
                点击画面设置锚点。布设图中可拖动圆点或文字；移动文字只改变标签偏移。
              </Info>
            </>
          )}
          {!geolayer && mode === "component" && (
            <Info>
              {kind === "layout"
                ? "在图纸上逐点绘制区域，双击完成。"
                : "点击模型中的源构件，或从源构件列表选择。"}
              关联不会创建新的业务构件。
            </Info>
          )}
          <Field label="默认主通道（当前对象直接拥有）">
            <select
              className={inputClass}
              value={main}
              onChange={(e) => setMain(e.target.value)}
            >
              <option value="">按可读通道选择</option>
              {channels.data?.items.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} · {c.unit}
                </option>
              ))}
            </select>
          </Field>
          <Failure error={channels.error} />
          <Button
            className="w-full"
            disabled={
              !target || target.retired || save.isPending || !m.available
            }
            onClick={add}
          >
            应用到草稿
          </Button>
        </aside>
      </div>
      {initial.version.candidates.length > 0 && !clearCandidates && (
        <section className="space-y-3 border-t pt-4">
          <h3 className="font-medium">迁移候选 · 逐项核对</h3>
          <Info>
            候选尚未生效。仅稳定来源标识可用于比较，名称或空间邻近不能证明为同一构件。确认后使用上方“应用到草稿”。
          </Info>
          {initial.version.candidates.map((c, i) => (
            <div
              key={i}
              className="flex flex-wrap items-center justify-between gap-2 rounded border p-3 text-sm"
            >
              <span>
                {c.placement?.object?.name ??
                  c.component_link?.object?.name ??
                  `候选 ${i + 1}`}{" "}
                · {c.placement?.page_id || c.component_link?.page_id || "模型"}{" "}
                ·{" "}
                {(
                  {
                    needs_review: "待核对",
                    unmatched: "未找到原元素",
                    uncertain: "来源身份待确认",
                  } as Record<string, string>
                )[c.state] ?? c.state}{" "}
                · {c.reason}
              </span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  if (c.placement) {
                    if (c.placement.page_id) setPageID(c.placement.page_id);
                    setMain(c.placement.main_channel_id ?? "");
                    setMode("point");
                    setSelected(c.placement.point_id);
                    setCoordinates(c.placement.anchor.join(","));
                    setLabel(c.placement.label_offset.join(","));
                    setPick({ element: c.placement.element_key });
                  } else if (c.component_link) {
                    if (c.component_link.page_id)
                      setPageID(c.component_link.page_id);
                    setMain(c.component_link.main_channel_id ?? "");
                    setMode("component");
                    setSelected(c.component_link.component_id);
                    setPick({
                      element: c.component_link.element_key,
                      region: c.component_link.region,
                    });
                  }
                }}
              >
                载入核对
              </Button>
            </div>
          ))}
          <Button variant="outline" onClick={() => setClearCandidates(true)}>
            已核对全部候选，清空候选列表
          </Button>
        </section>
      )}
    </section>
  );
}
