import { DomainFiles } from "../files/domain-files";
import { ObjectHistory } from "../audit/object-history";
import { spatialAdapter } from "../files/adapters/spatial";
import { transferFile } from "../files/upload-controller";
import { phases } from "../files/state";
import { UploadProgress, UploadPicker } from "@infrapro/ui/patterns/files";
import { SourceTaskPanel } from "../tasks/source-panel";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  SpatialAsset,
  SpatialVersion,
  SpatialPreview,
  SpatialControlInput,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { PreviewFacts } from "./preview-facts";
import { useSpatial } from "./context";
import { Field, TextField, Failure, Info } from "./common";
import { assetKinds, inputClass } from "./state";
type Props = {
  asset?: SpatialAsset;
  version?: SpatialVersion;
  onSelect: (asset: string, version?: string) => void;
  onEdit: () => void;
  facility?: string;
};
export function AssetManager(props: Props) {
  const m = useSpatial(),
    [tab, setTab] = useState("versions");
  const steps = [
    { name: "登记资料", done: !!props.asset },
    { name: "上传与处理", done: !!props.version?.manifest_hash },
    { name: "标注与核对", done: props.version?.state === "published" },
    { name: "发布版本", done: props.version?.state === "published" },
  ];
  return (
    <section className="min-w-0 space-y-5" aria-label="空间资料管理">
      <ol
        className="grid grid-cols-2 gap-2 sm:grid-cols-4"
        aria-label="资料准备流程"
      >
        {steps.map((step, i) => (
          <li
            key={step.name}
            className={`rounded-lg border p-3 text-sm ${step.done ? "border-primary/30 bg-primary/5" : "bg-muted/30"}`}
          >
            <span className="mb-2 block text-xs text-muted-foreground">
              0{i + 1}
            </span>
            {step.name}
          </li>
        ))}
      </ol>
      {!props.asset ? (
        <AssetForm
          key="new"
          facility={props.facility}
          onSelect={props.onSelect}
        />
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{assetKinds[props.asset.kind]}</Badge>
            <Badge variant="outline">
              {props.asset.status === "retired" ? "已退役" : "正常"}
            </Badge>
            <span className="text-sm text-muted-foreground">
              {props.asset.code}
            </span>
            <ObjectHistory objectType="spatial_asset" objectId={props.asset.id} />
            {props.version && (
              <ObjectHistory
                objectType="spatial_version"
                objectId={props.version.id}
                label="此版本操作历史"
              />
            )}
          </div>
          <nav className="flex gap-2 border-b pb-2" aria-label="资料管理内容">
            {[
              ["versions", "版本与原件"],
              ["info", "资料信息"],
              ["history", "变更记录"],
            ].map(([key, label]) => (
              <Button
                key={key}
                size="sm"
                variant={tab === key ? "secondary" : "ghost"}
                onClick={() => setTab(key!)}
              >
                {label}
              </Button>
            ))}
          </nav>
          {tab === "info" ? (
            <AssetForm
              key={props.asset.id}
              asset={props.asset}
              onSelect={(id) => props.onSelect(id, props.version?.id)}
            />
          ) : tab === "history" ? (
            <History asset={props.asset.id} />
          ) : (
            <>
              <Versions {...props} asset={props.asset} />
              {props.version && (
                <div className="space-y-4 rounded-xl border p-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-semibold">
                      当前查看 V{props.version.number}
                    </h3>
                    <Badge variant="secondary">
                      {props.version.state === "draft"
                        ? "草稿"
                        : props.version.state === "published"
                          ? "已发布"
                          : "已撤回"}
                    </Badge>
                    {props.asset.current_version_id === props.version.id && (
                      <Badge variant="outline">租户当前版本</Badge>
                    )}
                  </div>
                  <VersionFiles
                    key={props.version.id}
                    version={props.version}
                    onUpdated={() =>
                      props.onSelect(props.asset!.id, props.version!.id)
                    }
                  />
                  {props.version.state === "draft" &&
                    !!props.version.manifest_hash && (
                      <div className="space-y-3 rounded-lg bg-muted/40 p-4">
                        <p className="text-sm">
                          浏览文件已就绪。请在标注工作区核对图页、模型、测点位置与构件关联，再发布供项目使用。
                        </p>
                        {props.version.candidates.length > 0 && (
                          <Info>
                            还有 {props.version.candidates.length}{" "}
                            项迁移候选需要核对。
                          </Info>
                        )}
                        {m.canEdit && props.asset.status === "normal" && (
                          <Button onClick={props.onEdit}>
                            编辑挂载、关联与默认视图
                          </Button>
                        )}
                      </div>
                    )}
                </div>
              )}
              {(m.canManage || m.canPublish) && (
                <Lifecycle {...props} asset={props.asset} />
              )}
              <p className="text-xs leading-5 text-muted-foreground">
                已发布版本保留固定图模与标注。新版本发布后，项目仍使用各自明确获准的版本。
              </p>
            </>
          )}
        </>
      )}
    </section>
  );
}
function AssetForm({
  asset,
  facility,
  onSelect,
}: {
  asset?: SpatialAsset;
  facility?: string;
  onSelect: Props["onSelect"];
}) {
  const m = useSpatial(),
    cache = useQueryClient(),
    [name, setName] = useState(asset?.name ?? ""),
    [code, setCode] = useState(asset?.code ?? ""),
    [kind, setKind] = useState(asset?.kind ?? "layout"),
    [category, setCategory] = useState(asset?.category ?? "plan"),
    [fid, setFID] = useState(asset?.facility_id ?? facility ?? ""),
    [source, setSource] = useState(asset?.source_note ?? ""),
    [q, setQ] = useState("");
  const facilities = useQuery({
    queryKey: [...m.prefix, "asset-facilities", q],
    queryFn: ({ signal }) =>
      m.client.spatialObjects(
        m.scope,
        { kind: "facility", q, limit: 100 },
        signal,
      ),
    enabled: !asset && m.canManage,
  });
  const save = useMutation({
    mutationFn: () =>
      m.client.spatialSaveAsset(m.scope, asset?.id, {
        request_id: crypto.randomUUID(),
        expected_revision: asset?.revision,
        kind,
        facility_id: fid || undefined,
        name,
        code,
        category,
        source_note: source,
      }),
    onSuccess: async (r) => {
      await cache.invalidateQueries({ queryKey: m.prefix });
      onSelect(r.id);
    },
  });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
      className="space-y-3"
    >
      <fieldset
        disabled={save.isPending || !m.canManage || !m.available}
        className="grid gap-3 sm:grid-cols-2"
      >
        <TextField label="资料名称" value={name} onChange={setName} />
        <TextField
          label="资料编码"
          value={code}
          onChange={setCode}
          disabled={!!asset}
        />
        <Field label="资料类型">
          <select
            className={inputClass}
            disabled={!!asset}
            value={kind}
            onChange={(e) => setKind(e.target.value)}
          >
            {Object.entries(assetKinds).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </Field>
        <Field label="资料分类">
          <select
            className={inputClass}
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            {Object.entries({
              general: "通用",
              plan: "平面布设",
              elevation: "立面布设",
              section: "断面",
              monitoring_section: "监测剖面",
              other: "其他",
            }).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </Field>
        {!asset && (
          <>
            <TextField label="搜索所属设施" value={q} onChange={setQ} />
            <Field
              label={kind === "geolayer" ? "所属设施（可选）" : "所属设施"}
            >
              <select
                className={inputClass}
                value={fid}
                onChange={(e) => setFID(e.target.value)}
              >
                <option value="">
                  {kind === "geolayer" ? "租户级图层" : "请选择设施"}
                </option>
                {facilities.data?.items.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.name} · {f.code}
                  </option>
                ))}
              </select>
            </Field>
          </>
        )}
        <div className="sm:col-span-2">
          <TextField
            label="来源、版权及用途说明"
            value={source}
            onChange={setSource}
          />
        </div>
        {m.canManage && (
          <Button type="submit">
            {save.isPending ? "正在保存…" : "保存资料信息"}
          </Button>
        )}
      </fieldset>
      <Failure error={save.error ?? facilities.error} />
    </form>
  );
}
function Versions({
  asset,
  version,
  onSelect,
}: Props & { asset: SpatialAsset }) {
  const m = useSpatial(),
    cache = useQueryClient(),
    [cursor, setCursor] = useState(""),
    [note, setNote] = useState(""),
    [newSource, setNewSource] = useState(true);
  const list = useQuery({
    queryKey: [...m.prefix, "versions", asset.id, cursor],
    queryFn: ({ signal }) =>
      m.client.spatialVersions(
        m.scope,
        asset.id,
        { cursor, limit: 20 },
        signal,
      ),
  });
  const create = useMutation({
    mutationFn: () =>
      m.client.spatialCreateVersion(m.scope, asset.id, {
        request_id: crypto.randomUUID(),
        expected_revision: asset.revision,
        base_version_id:
          version?.state === "published" ? version.id : undefined,
        new_source: newSource,
        change_note: note,
      }),
    onSuccess: async (r) => {
      await cache.invalidateQueries({ queryKey: m.prefix });
      onSelect(asset.id, r.version_id || r.id);
    },
  });
  return (
    <div className="space-y-3">
      <h3 className="font-medium">固定版本</h3>
      <ul className="divide-y rounded-lg border">
        {list.data?.items.map((v) => (
          <li key={v.id}>
            <button
              type="button"
              className="flex min-h-11 w-full flex-wrap items-center justify-between gap-2 p-3 text-left hover:bg-muted focus-visible:ring-2"
              aria-current={version?.id === v.id ? "true" : undefined}
              onClick={() => onSelect(asset.id, v.id)}
            >
              <span>
                V{v.number} ·{" "}
                {
                  (
                    {
                      draft: "草稿",
                      published: "已发布",
                      withdrawn: "已撤回",
                    } as Record<string, string>
                  )[v.state]
                }
                {asset.current_version_id === v.id ? " · 当前版" : ""}
              </span>
              <span className="text-xs text-muted-foreground">
                {v.change_note || "未填写变更说明"}
              </span>
            </button>
          </li>
        ))}
      </ul>
      <div className="flex gap-2">
        {cursor && (
          <Button variant="ghost" onClick={() => setCursor("")}>
            回到首页
          </Button>
        )}
        {list.data?.next_cursor && (
          <Button
            variant="ghost"
            onClick={() => setCursor(list.data!.next_cursor)}
          >
            更多版本
          </Button>
        )}
      </div>
      {m.canManage && asset.status === "normal" && (
        <details
          open={!version || undefined}
          className="rounded-lg border bg-muted/20 p-3"
        >
          <summary className="min-h-11 cursor-pointer py-3 font-medium">
            {version ? "创建下一个草稿" : "创建首个版本草稿"}
          </summary>
          <div className="space-y-3">
            <TextField label="版本变更说明" value={note} onChange={setNote} />
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={newSource}
                onChange={(e) => setNewSource(e.target.checked)}
              />
              更换原始文件（已有挂载仅作为待核对候选）
            </label>
            <Info>
              {version?.state === "published"
                ? `以 V${version.number} 为基版。`
                : "从空白草稿开始；只有已发布版本可作为复制基版。"}
              同一原件可复制关联；更换原件后须逐项确认。项目授权保持原固定版本。
            </Info>
            <Button
              disabled={
                create.isPending ||
                !m.available ||
                (!newSource && version?.state !== "published")
              }
              onClick={() => create.mutate()}
            >
              创建草稿
            </Button>
          </div>
        </details>
      )}
      <Failure error={create.error ?? list.error} />
    </div>
  );
}
function VersionFiles({
  version,
  onUpdated,
}: {
  version: SpatialVersion;
  onUpdated: () => void;
}) {
  const m = useSpatial(),
    cache = useQueryClient(),
    [file, setFile] = useState<File>(),
    [progress, setProgress] = useState(""),
    [uploadID, setUploadID] = useState(version.latest_upload_id ?? ""),
    [jobID, setJobID] = useState(version.latest_job_id ?? ""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(),
    controller = useRef<AbortController | null>(null),
    completedJob = useRef(""),
    adapter = useRef<ReturnType<typeof spatialAdapter> | null>(null),
    [uploadedBytes, setUploadedBytes] = useState(0);
  const job = useQuery({
    queryKey: [...m.prefix, "job", jobID],
    queryFn: ({ signal }) => m.client.spatialJob(m.scope, jobID, signal),
    enabled: !!jobID,
    refetchInterval: (q) =>
      ["queued", "running"].includes(q.state.data?.state ?? "queued")
        ? 1200
        : false,
    retry: false,
  });
  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => {
    if (
      job.data?.state === "succeeded" &&
      completedJob.current !== `${job.data.id}:${job.data.revision}`
    ) {
      completedJob.current = `${job.data.id}:${job.data.revision}`;
      void cache.invalidateQueries({
        queryKey: [...m.prefix, "manifest", version.id],
      });
      void cache.invalidateQueries({
        queryKey: [...m.prefix, "versions", version.asset_id],
      });
      // Completion refreshes this version. It must not navigate after the user
      // has already followed an audit, task or other source link.
    }
  }, [job.data, cache, m.prefix, version.id, version.asset_id]);
  const upload = async () => {
    if (!file) return;
    const abort = new AbortController();
    controller.current = abort;
    setBusy(true);
    setError(undefined);
    try {
      adapter.current ??= spatialAdapter(
        m.client,
        m.scope,
        version,
        uploadID,
        setUploadID,
      );
      await transferFile(adapter.current, file, abort.signal, (p) => {
        setProgress(phases[p.phase]);
        setUploadedBytes(p.bytes);
      });
      abort.signal.throwIfAborted();
      adapter.current = null;
      setProgress("原件已封存。");
      setUploadID("");
      await cache.invalidateQueries({ queryKey: m.prefix });
      onUpdated();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const action = useMutation({
    mutationFn: async (
      op: "process" | "cancel" | "retry" | "cancel-upload",
    ) => {
      if (op === "process") {
        const current = await m.client.spatialManifest(m.scope, version.id);
        const result = await m.client.spatialProcess(m.scope, version.id, {
          request_id: crypto.randomUUID(),
          expected_revision: current.revision,
        });
        setJobID(result.id);
      } else if (op === "cancel-upload") {
        controller.current?.abort();
        const current = await m.client.spatialUpload(m.scope, uploadID);
        await m.client.spatialCancelUpload(m.scope, uploadID, {
          request_id: crypto.randomUUID(),
          expected_revision: current.revision,
        });
        setUploadID("");
        adapter.current = null;
        setProgress("上传已取消。");
      } else if (job.data) {
        await m.client.spatialControlJob(m.scope, job.data.id, op, {
          request_id: crypto.randomUUID(),
          expected_revision: job.data.revision,
        });
        await job.refetch();
      }
      await cache.invalidateQueries({
        queryKey: [...m.prefix, "manifest", version.id],
      });
    },
  });
  return (
    <div className="space-y-3 border-t pt-4">
      <h3 className="font-medium">原件与处理</h3>
      <p className="break-all text-xs text-muted-foreground">
        {version.source?.name ?? "尚未封存原件"}
        {version.manifest_hash
          ? ` · 校验摘要 ${version.manifest_hash.slice(0, 16)}…`
          : ""}
      </p>
      {version.state === "draft" && m.canManage && (
        <>
          <UploadPicker
            label="选择空间原件"
            accept=".png,.jpg,.jpeg,.pdf,.glb,.ifc,.geojson,.json"
            hint="PNG、JPEG、PDF、GLB、IFC、GeoJSON；最大 256 MiB。"
            disabled={busy || !!version.source?.name}
            onChange={(value) => {
              setFile(value ?? undefined);
              if (!uploadID) adapter.current = null;
            }}
          />
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={!file || busy || !!version.source?.name || !m.available}
              onClick={() => void upload()}
            >
              {uploadID ? "继续上传并校验" : "上传并校验"}
            </Button>
            {uploadID && (
              <Button
                variant="outline"
                disabled={action.isPending}
                onClick={() => action.mutate("cancel-upload")}
              >
                取消上传
              </Button>
            )}
            <Button
              variant="outline"
              disabled={
                !version.source?.name ||
                busy ||
                action.isPending ||
                ["queued", "running"].includes(job.data?.state ?? "")
              }
              onClick={() => action.mutate("process")}
            >
              开始处理原件
            </Button>
          </div>
        </>
      )}
      {version.source?.name && (
        <DomainFiles
          files={[
            {
              ref: {
                kind: "spatial.source",
                scope: m.scope,
                versionId: version.id,
              },
              display_name: version.source.name,
              media_type: version.format,
              size_bytes: String(version.source.size ?? 0),
              sha256: version.source.sha256,
              state: version.can_download_source ? "available" : "restricted",
              allowed_actions: version.can_download_source ? ["download"] : [],
            },
          ]}
          actions={() => (
            <Button variant="outline" asChild>
              <a
                href={m.client.spatialFileURL(m.scope, version.id, "source")}
                download
              >
                下载原始文件
              </a>
            </Button>
          )}
        />
      )}
      {jobID && (
        <SourceTaskPanel
          source={{
            source_module: "spatial",
            source_kind: "job",
            source_root_id: jobID,
          }}
        />
      )}
      {progress && (
        <UploadProgress
          phase={progress}
          bytes={uploadedBytes}
          total={file?.size ?? 0}
          busy={busy}
          onPause={() => controller.current?.abort()}
        />
      )}
      <Failure error={error ?? action.error ?? job.error} />
    </div>
  );
}
function Lifecycle({
  asset,
  version,
  onSelect,
}: Props & { asset: SpatialAsset }) {
  const m = useSpatial(),
    cache = useQueryClient(),
    [deleteOpen, setDeleteOpen] = useState(false),
    [reason, setReason] = useState(""),
    [replacement, setReplacement] = useState(""),
    [stage, setStage] = useState<{
      kind: "assets" | "versions";
      target: string;
      action: "publish" | "withdraw" | "current" | "retire";
      input: SpatialControlInput;
      preview: SpatialPreview;
    }>();
  const versions = useQuery({
    queryKey: [...m.prefix, "published", asset.id],
    queryFn: ({ signal }) =>
      m.client.spatialVersions(
        m.scope,
        asset.id,
        { state: "published", limit: 100 },
        signal,
      ),
    enabled: version?.state === "published",
  });
  const command = useMutation({
    mutationFn: async (
      action:
        | "publish"
        | "withdraw"
        | "current"
        | "retire"
        | "restore"
        | "confirm"
        | "validate"
        | "delete",
    ) => {
      if (action === "validate") {
        if (!version) return;
        await m.client.spatialValidate(m.scope, version.id, {
          request_id: crypto.randomUUID(),
          expected_revision: version.revision,
        });
        return "当前草稿校验通过。";
      }
      if (action === "delete") {
        if (!version) return;
        await m.client.spatialDeleteDraft(m.scope, version.id, {
          request_id: crypto.randomUUID(),
          expected_revision: version.revision,
          reason,
        });
        setDeleteOpen(false);
        await cache.invalidateQueries({ queryKey: m.prefix });
        onSelect(asset.id);
        return "草稿已删除。";
      }
      if (action === "confirm" && stage) {
        await m.client.spatialControl(
          m.scope,
          stage.kind,
          stage.target,
          stage.action,
          { ...stage.input, preview_hash: stage.preview.preview_hash },
        );
        setStage(undefined);
      } else if (action === "restore") {
        await m.client.spatialControl(m.scope, "assets", asset.id, "restore", {
          request_id: crypto.randomUUID(),
          expected_revision: asset.revision,
          reason,
        });
      } else if (action !== "confirm") {
        const kind =
            action === "publish" || action === "withdraw"
              ? "versions"
              : "assets",
          target = kind === "versions" ? version?.id : asset.id;
        if (!target) return;
        const input = {
          request_id: crypto.randomUUID(),
          expected_revision:
            kind === "versions" ? version!.revision : asset.revision,
          reason,
          version_id: action === "current" ? version?.id : undefined,
          replacement_version_id:
            action === "withdraw" ? replacement || undefined : undefined,
        };
        const preview = await m.client.spatialPreview(
          m.scope,
          kind,
          target,
          action,
          input,
        );
        setStage({ kind, target, action, input, preview });
        return;
      }
      await cache.invalidateQueries({ queryKey: m.prefix });
      return "状态已更新。";
    },
  });
  return (
    <div className="space-y-3 border-t pt-4">
      <h3 className="font-medium">发布与生命周期</h3>
      <TextField label="本次变更原因" value={reason} onChange={setReason} />
      {version?.state === "published" && (
        <Field label="撤回当前版时的替代版本">
          <select
            className={inputClass}
            value={replacement}
            onChange={(e) => setReplacement(e.target.value)}
          >
            <option value="">撤回后不设当前版</option>
            {versions.data?.items
              .filter((v) => v.id !== version.id && v.state === "published")
              .map((v) => (
                <option key={v.id} value={v.id}>
                  V{v.number}
                </option>
              ))}
          </select>
        </Field>
      )}
      <div className="flex flex-wrap gap-2">
        {version?.state === "draft" && m.canPublish && (
          <>
            <Button
              variant="outline"
              disabled={command.isPending}
              onClick={() => command.mutate("validate")}
            >
              校验草稿
            </Button>
            <Button
              disabled={command.isPending || !reason.trim()}
              onClick={() => command.mutate("publish")}
            >
              预览发布
            </Button>
          </>
        )}
        {version?.state === "draft" && m.canManage && (
          <Button
            variant="outline"
            disabled={command.isPending || !reason.trim()}
            onClick={() => setDeleteOpen(true)}
          >
            删除未使用草稿
          </Button>
        )}
        {version?.state === "published" && m.canPublish && (
          <>
            <Button
              variant="outline"
              disabled={command.isPending || !reason.trim()}
              onClick={() => command.mutate("current")}
            >
              设为当前版
            </Button>
            <Button
              variant="outline"
              disabled={command.isPending || !reason.trim()}
              onClick={() => command.mutate("withdraw")}
            >
              预览撤回
            </Button>
          </>
        )}
        {m.canManage && (
          <Button
            variant="outline"
            disabled={command.isPending || !reason.trim()}
            onClick={() =>
              command.mutate(asset.status === "normal" ? "retire" : "restore")
            }
          >
            {asset.status === "normal" ? "预览退役资料" : "恢复资料"}
          </Button>
        )}
      </div>
      <ResponsivePanel
        open={!!stage}
        onOpenChange={(open) => {
          if (!open && !command.isPending) setStage(undefined);
        }}
        title="确认空间资料变更"
        description="核对版本与影响，确认后执行本次变更。"
        footer={
          stage && (
            <div className="flex gap-2">
              <Button
                disabled={
                  !stage.preview.can_submit || command.isPending || !m.available
                }
                onClick={() => command.mutate("confirm")}
              >
                {command.isPending ? "正在提交…" : "确认变更"}
              </Button>
              <Button
                variant="outline"
                disabled={command.isPending}
                onClick={() => setStage(undefined)}
              >
                取消
              </Button>
            </div>
          )
        }
      >
        {stage && (
          <div className="space-y-4">
            <h3 className="font-semibold">
              {
                {
                  publish: "发布版本",
                  withdraw: "撤回版本",
                  current: "设为当前版本",
                  retire: "退役资料",
                }[stage.action]
              }{" "}
              · {asset.name}
              {version ? ` · V${version.number}` : ""}
            </h3>
            <p className="text-sm leading-6">
              {stage.action === "publish"
                ? "发布后文件、挂载和默认视图将固定。项目继续使用各自获准的版本。"
                : stage.action === "withdraw"
                  ? "该版本将不可再浏览，相关项目授权同时结束。替代当前版本不会自动授权给项目。"
                  : stage.action === "retire"
                    ? "停止新版本、新标注与新增授权，已有获准历史读取和变更记录保留。"
                    : "更改租户当前展示版本，项目授权仍固定到原来获准的版本。"}
            </p>
            <p className="rounded-lg bg-muted p-3 text-sm">
              变更原因：{stage.input.reason}
            </p>
            <PreviewFacts preview={stage.preview} />
            <Failure error={command.error} />
          </div>
        )}
      </ResponsivePanel>
      <ResponsivePanel
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!command.isPending) setDeleteOpen(open);
        }}
        title="删除未使用草稿"
        description="此操作删除当前草稿及其未发布标注，不影响已发布版本。"
        footer={
          <div className="flex gap-2">
            <Button
              variant="destructive"
              disabled={command.isPending || !m.available}
              onClick={() => command.mutate("delete")}
            >
              确认删除草稿
            </Button>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              取消
            </Button>
          </div>
        }
      >
        <p className="text-sm">
          {asset.name} · V{version?.number} · {reason}
        </p>
        <Failure error={command.error} />
      </ResponsivePanel>
      {command.data && <Info>{command.data}</Info>}
      <Failure error={command.error} />
    </div>
  );
}
function History({ asset }: { asset: string }) {
  const m = useSpatial(),
    [cursor, setCursor] = useState("");
  const q = useQuery({
    queryKey: [...m.prefix, "history", asset, cursor],
    queryFn: ({ signal }) =>
      m.client.spatialHistory(m.scope, asset, { cursor, limit: 20 }, signal),
    enabled: m.available,
  });
  const names: Record<string, string> = {
    "assets.create": "登记空间资料",
    "assets.update": "更新资料信息",
    "versions.create": "创建版本草稿",
    "drafts.update": "保存标注草稿",
    "versions.publish": "发布版本",
    "versions.withdraw": "撤回版本",
    "assets.current": "设置当前版本",
    "assets.retire": "退役资料",
    "assets.restore": "恢复资料",
    "drafts.delete": "删除草稿",
    "uploads.complete": "完成原件上传",
  };
  return (
    <section className="space-y-3">
      <h3 className="font-medium">资料变更记录</h3>
      <Failure error={q.error} />
      {q.isPending && <Info>正在读取变更记录…</Info>}
      {q.data?.items.map((item, i) => (
        <article key={i} className="space-y-2 rounded-lg border p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium">
              {names[String(item.action)] ??
                String(item.action ?? "空间资料变更")}
            </span>
            <time className="text-xs text-muted-foreground">
              {typeof item.created_at === "string"
                ? new Date(item.created_at).toLocaleString()
                : ""}
            </time>
          </div>
          {typeof item.reason === "string" && (
            <p className="text-sm text-muted-foreground">{item.reason}</p>
          )}
          <details>
            <summary className="cursor-pointer py-2 text-xs text-muted-foreground">
              查看记录字段
            </summary>
            <pre className="max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-3 text-xs">
              {JSON.stringify(item, null, 2)}
            </pre>
          </details>
        </article>
      ))}
      {q.data && !q.data.items.length && <Info>暂无变更记录。</Info>}
      {q.data?.next_cursor && (
        <Button
          variant="outline"
          onClick={() => setCursor(q.data!.next_cursor)}
        >
          更多记录
        </Button>
      )}
      {cursor && (
        <Button variant="ghost" onClick={() => setCursor("")}>
          回到首批记录
        </Button>
      )}
    </section>
  );
}
