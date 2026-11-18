import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { readSpatialMetadata } from "./metadata-reader";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Permissions as P,
  type SpatialAccessInput,
  type SpatialPreview,
  type SpatialGrant,
} from "@infrapro/api-client";
import { Plus, ArrowLeft, ShieldCheck } from "lucide-react";
import { Button } from "@infrapro/ui/components/button";
import { Badge } from "@infrapro/ui/components/badge";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { useTenant } from "../tenancy/context";
import { useSpatial } from "./context";
import { SpatialLink, Failure, Field, Info, TextField } from "./common";
import { inputClass } from "./state";
import { PreviewFacts } from "./preview-facts";

export function SpatialAccessPage({
  project,
  showEnded,
  onShowEnded,
}: {
  project: string;
  showEnded: boolean;
  onShowEnded: (value: boolean) => void;
}) {
  const m = useSpatial(),
    tenant = useTenant(),
    cache = useQueryClient(),
    canRead = tenant.permissions.includes(P.TenantProjectSpatialAccessRead),
    canManage = tenant.permissions.includes(P.TenantProjectSpatialAccessManage),
    [q, setQ] = useState(""),
    [asset, setAsset] = useState(""),
    [version, setVersion] = useState(""),
    [source, setSource] = useState(false),
    [defaultView, setDefaultView] = useState(false),
    [reason, setReason] = useState(""),
    [open, setOpen] = useState(false),
    [endTarget, setEndTarget] = useState(""),
    [notice, setNotice] = useState(""),
    [stage, setStage] = useState<{
      input: SpatialAccessInput;
      preview: SpatialPreview;
    }>();
  const grants = useQuery({
    queryKey: [...m.prefix, "access", project],
    queryFn: ({ signal }) =>
      m.client.spatialAccess(tenant.tenant.id, project, signal),
    enabled: canRead && tenant.available,
    retry: false,
  });
  const summary = useQuery({
    queryKey: [...m.prefix, "access-project", project],
    queryFn: ({ signal }) =>
      m.client.projectSummary(tenant.tenant.id, project, signal),
    enabled: canRead && m.can(P.TenantProjectRead) && m.available,
    retry: false,
  });
  const assets = usePagedQuery({ initialPageSize: 30, queryKey: [...m.prefix, "grant-assets", q], enabled: canManage && m.canRead && m.available && open && !endTarget, retry: false, queryFn: ({ signal, cursor , limit}) =>
      m.client.spatialAssets(
        m.scope,
        { q, state: "normal", limit, cursor },
        signal,
      ) });
  const selectedAsset = useQuery({
    queryKey: [...m.prefix, "asset", asset],
    queryFn: ({ signal }) => m.client.spatialAsset(m.scope, asset, signal),
    enabled: !!asset && m.canRead && open && m.available,
    retry: false,
  });
  const versions = usePagedQuery({ initialPageSize: 30, queryKey: [...m.prefix, "grant-versions", asset], enabled: !!asset && m.canRead && m.available && open && !endTarget, retry: false, queryFn: ({ signal, cursor , limit}) =>
      m.client.spatialVersions(
        m.scope,
        asset,
        { state: "published", limit, cursor },
        signal,
      ) });
  const command = useMutation({
    mutationFn: async (action: string) => {
      if (!grants.data) throw Error("请先读取当前授权。");
      if (action === "confirm") {
        if (!stage) return;
        await m.client.spatialAccessSave(tenant.tenant.id, project, {
          ...stage.input,
          preview_hash: stage.preview.preview_hash,
        });
        setStage(undefined);
        setOpen(false);
        setNotice("空间资料授权已更新。");
        await cache.invalidateQueries({ queryKey: m.prefix });
        return;
      }
      const input: SpatialAccessInput = {
        request_id: crypto.randomUUID(),
        expected_revision: grants.data.revision,
        reason,
        add:
          action === "add"
            ? [
                {
                  version_id: version,
                  source_download: source,
                  is_default: defaultView,
                },
              ]
            : [],
        end: action === "add" ? [] : [action],
      };
      const preview = await m.client.spatialAccessPreview(
        tenant.tenant.id,
        project,
        input,
      );
      setStage({ input, preview });
    },
  });
  const start = (g?: SpatialGrant, end = false) => {
    command.reset();
    setReason("");
    setQ("");
    setAsset(g?.asset_id ?? "");
    setVersion(g?.version_id ?? "");
    setSource(g?.source_download ?? false);
    setDefaultView(g?.is_default ?? false);
    setEndTarget(end ? g!.id : "");
    setOpen(true);
  };
  if (!canRead) return <Info>当前身份没有项目空间授权读取权限。</Info>;
  const records =
      grants.data?.grants.filter((g) => showEnded || !g.ended_at) ?? [],
    assetItems = assets.data?.items ?? [];
  return (
    <section className="min-w-0 space-y-5">
      <div className="flex flex-wrap gap-4 text-sm">
        <SpatialLink
          className="inline-flex items-center gap-1 text-muted-foreground hover:underline"
          to={`${m.base}/spatial`}
          search={{ section: "assets" }}
        >
          <ArrowLeft className="size-4" />
          返回资料与版本
        </SpatialLink>
        <SpatialLink
          className="text-muted-foreground hover:underline"
          to={`${m.base}/projects/${project}`}
        >
          项目治理
        </SpatialLink>
      </div>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">项目空间资料授权</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {summary.data?.name ?? "当前项目"} ·
            逐份选择可浏览的固定版本，单独控制原件下载。
          </p>
        </div>
        {canManage && m.canRead && (
          <Button onClick={() => start()} disabled={!m.available}>
            <Plus className="size-4" />
            授权空间资料
          </Button>
        )}
      </header>
      <div className="grid gap-3 md:grid-cols-3">
        {[
          {
            title: "图纸与模型",
            text: "获准查看整份图面、几何及已发布的源属性。",
          },
          {
            title: "原件下载",
            text: "额外授权后可下载原始文件，可能包含更多来源内容。",
          },
          {
            title: "业务与监测",
            text: "测点、构件、设备和监测数据仍按各自权限读取。",
          },
        ].map((v) => (
          <div key={v.title} className="rounded-xl border bg-card p-4">
            <h2 className="flex items-center gap-2 text-sm font-semibold">
              <ShieldCheck className="size-4 text-primary" />
              {v.title}
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {v.text}
            </p>
          </div>
        ))}
      </div>
      <Info>
        设施资料需先关联到项目。结束设施关联会结束该次空间授权；重新关联后须重新授权。归档项目可查看和撤权，不能新增或扩权。
      </Info>
      {notice && <Info>{notice}</Info>}
      <Failure error={grants.error} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-semibold">
          固定版本授权记录{" "}
          <span className="ml-2 text-sm font-normal text-muted-foreground">
            {records.length} 项
          </span>
        </h2>
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={showEnded}
            onChange={(e) => onShowEnded(e.target.checked)}
          />
          显示已结束授权
        </label>
      </div>
      {grants.isPending ? (
        <Info>正在读取授权…</Info>
      ) : !records.length ? (
        <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          暂无{showEnded ? "" : "有效的"}空间资料授权。项目授权固定到具体版本。
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                {[
                  "空间资料 / 固定版本",
                  "呈现内容",
                  "原件下载",
                  "项目默认",
                  "状态",
                  "操作",
                ].map((v) => (
                  <th key={v} className="p-3 font-medium whitespace-nowrap">
                    {v}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y">
              {records.map((g) => (
                <tr key={g.id} className="hover:bg-muted/20">
                  <td className="p-3">
                    <GrantIdentity grant={g} />
                  </td>
                  <td className="p-3">
                    {g.ended_at ? "已结束" : "整份已发布内容"}
                  </td>
                  <td className="p-3">
                    {g.source_download ? "允许" : "不允许"}
                  </td>
                  <td className="p-3">{g.is_default ? "默认版本" : "—"}</td>
                  <td className="p-3">
                    <Badge variant={g.ended_at ? "outline" : "secondary"}>
                      {g.ended_at ? "已结束" : "有效"}
                    </Badge>
                    {g.ended_at && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {new Date(g.ended_at).toLocaleString()}
                      </p>
                    )}
                  </td>
                  <td className="p-3">
                    {canManage && !g.ended_at && (
                      <div className="flex gap-1">
                        {m.canRead && (
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => start(g)}
                          >
                            调整授权
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => start(g, true)}
                        >
                          结束授权
                        </Button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <ResponsivePanel
        open={open}
        onOpenChange={(value) => {
          if (!command.isPending) setOpen(value);
        }}
        title={endTarget ? "结束空间资料授权" : "授权或调整固定版本"}
        description={summary.data?.name ?? "当前项目的空间资料权限"}
        footer={
          <div className="flex gap-2">
            <Button
              disabled={
                !reason.trim() ||
                (!endTarget && !version) ||
                command.isPending ||
                !m.available
              }
              onClick={() => command.mutate(endTarget || "add")}
            >
              {command.isPending
                ? "正在预览…"
                : endTarget
                  ? "预览结束授权"
                  : "预览授权或调整"}
            </Button>
            <Button
              variant="outline"
              disabled={command.isPending}
              onClick={() => setOpen(false)}
            >
              取消
            </Button>
          </div>
        }
      >
        <div className="space-y-4">
          {endTarget ? (
            <Info>
              结束后项目将不能继续浏览或下载这个固定版本，其他授权保持不变。
            </Info>
          ) : m.canRead ? (
            <>
              <TextField label="搜索资料" value={q} onChange={setQ} />
              <Field label="资料">
                <select
                  className={inputClass}
                  value={asset}
                  onChange={(e) => {
                    setAsset(e.target.value);
                    setVersion("");
                  }}
                >
                  <option value="">请选择资料</option>
                  {selectedAsset.data &&
                    !assetItems.some((a) => a.id === asset) && (
                      <option value={asset}>{selectedAsset.data.name}</option>
                    )}
                  {assetItems.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name} · {a.code}
                    </option>
                  ))}
                </select>
              </Field>
              {assets.data && (
                <ListPagination label="可授权资料" {...assets.pagination} />
              )}
              <Field label="明确选择已发布版本">
                <select
                  className={inputClass}
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                >
                  <option value="">请选择固定版本</option>
                  {versions.data?.items
                    .filter((v) => v.state === "published")
                    .map((v) => (
                      <option key={v.id} value={v.id}>
                        V{v.number} · {v.change_note || "已发布"}
                      </option>
                    ))}
                </select>
              </Field>
              {versions.data && (
                <ListPagination label="已发布版本" {...versions.pagination} />
              )}
              <label className="flex min-h-11 items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={source}
                  onChange={(e) => setSource(e.target.checked)}
                />
                额外允许下载原件
              </label>
              <label className="flex min-h-11 items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={defaultView}
                  onChange={(e) => setDefaultView(e.target.checked)}
                />
                设为此项目的该资料默认版本
              </label>
              <Failure
                error={assets.error ?? versions.error ?? selectedAsset.error}
              />
            </>
          ) : (
            <Info>选择新增资料还需要空间资料读取权限。</Info>
          )}
          <TextField
            label="本次授权变更原因"
            value={reason}
            onChange={setReason}
          />
          <Failure error={command.error} />
        </div>
      </ResponsivePanel>
      <ResponsivePanel
        open={!!stage}
        onOpenChange={(value) => {
          if (!value && !command.isPending) setStage(undefined);
        }}
        title="空间授权变更预览"
        description="核对固定版本、下载权限与影响后确认。"
        footer={
          stage && (
            <div className="flex gap-2">
              <Button
                disabled={
                  !stage.preview.can_submit || command.isPending || !m.available
                }
                onClick={() => command.mutate("confirm")}
              >
                {command.isPending ? "正在提交…" : "确认授权变更"}
              </Button>
              <Button
                variant="outline"
                disabled={command.isPending}
                onClick={() => setStage(undefined)}
              >
                返回调整
              </Button>
            </div>
          )
        }
      >
        {stage && (
          <div className="space-y-4">
            <p className="text-sm">
              新增或调整 {stage.input.add.length} 个固定版本，结束{" "}
              {stage.input.end.length}{" "}
              项授权。此操作不会授予业务资源、数据或设备权限。
            </p>
            {stage.input.add.length > 0 && (
              <dl className="grid grid-cols-2 gap-3 rounded-lg border p-4 text-sm">
                <dt className="text-muted-foreground">原件下载</dt>
                <dd>
                  {stage.input.add[0]?.source_download ? "允许" : "不允许"}
                </dd>
                <dt className="text-muted-foreground">项目默认</dt>
                <dd>
                  {stage.input.add[0]?.is_default
                    ? "设为此资料默认版本"
                    : "保持默认设置"}
                </dd>
              </dl>
            )}
            <p className="rounded-lg bg-muted p-3 text-sm">
              原因：{stage.input.reason}
            </p>
            <PreviewFacts preview={stage.preview} />
            <Failure error={command.error} />
          </div>
        )}
      </ResponsivePanel>
    </section>
  );
}
function GrantIdentity({ grant }: { grant: SpatialGrant }) {
  const m = useSpatial();
  const asset = useQuery({
    queryKey: [...m.prefix, "asset", grant.asset_id],
    queryFn: ({ signal }) =>
      readSpatialMetadata(signal, () =>
        m.client.spatialAsset(m.scope, grant.asset_id, signal),
      ),
    enabled: m.canRead && m.available,
    retry: false,
  });
  const version = useQuery({
    queryKey: [...m.prefix, "manifest", grant.version_id],
    queryFn: ({ signal }) =>
      readSpatialMetadata(signal, () =>
        m.client.spatialManifest(m.scope, grant.version_id, signal),
      ),
    enabled: m.canRead && m.available,
    retry: false,
  });
  return (
    <div className="space-y-1">
      <p className="font-medium">{asset.data?.name ?? "空间资料"}</p>
      {m.canRead ? (
        <SpatialLink
          className="text-primary underline underline-offset-4"
          to={`${m.base}/spatial`}
          search={{ asset: grant.asset_id, version: grant.version_id }}
        >
          查看资料版本{" "}
          {version.data
            ? `V${version.data.number}`
            : grant.version_id.slice(0, 8)}
        </SpatialLink>
      ) : (
        <p className="text-muted-foreground">当前身份无资料浏览权限</p>
      )}
      {asset.data && (
        <p className="text-xs text-muted-foreground">{asset.data.code}</p>
      )}
    </div>
  );
}
