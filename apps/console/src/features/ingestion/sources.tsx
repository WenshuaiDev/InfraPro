import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Permissions as P,
  type IngestionSourceInput,
  type IngestionSourceDetail,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { ErrorState, FormNotice } from "@infrapro/ui/patterns/feedback";
import { DeviceField, DeviceSelect } from "../parsing/fields";
import { useParsing, useParsingWrite, WriteFeedback } from "../parsing/context";
import { Execution } from "./terminals";
import { Digest, Facts, Pages, State, Time } from "./common";
export function Sources({ initial }: { initial?: string }) {
  const c = useParsing(),
    [cursor, setCursor] = useState(""),
    [q, setQ] = useState(""),
    [selected, setSelected] = useState(initial),
    [create, setCreate] = useState(false),
    [nodes, setNodes] = useState(false);
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-sources", cursor, q],
    queryFn: ({ signal }) =>
      c.client.ingestionSources(c.scope, { cursor, q, limit: 25 }, signal),
  });
  const manage = c.scope.kind === "platform" && c.can(P.IngestionSourceManage);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-60 flex-1">
          <DeviceField
            name="source-search"
            label="搜索入口"
            value={q}
            onChange={(v) => {
              setQ(v);
              setCursor("");
            }}
          />
        </div>
        {manage && (
          <>
            <Button variant="outline" onClick={() => setNodes(true)}>
              云节点配置
            </Button>
            <Button disabled={!c.available} onClick={() => setCreate(true)}>
              新建接入入口
            </Button>
          </>
        )}
      </div>
      <DirectoryTable
        label="已分配接入入口"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="暂无获配入口。平台建立来源范围后，租户须确认准确修订。"
        columns={[
          {
            id: "name",
            label: "入口",
            cell: (v) => (
              <div>
                <Button
                  variant="link"
                  className="h-auto p-0"
                  onClick={() => setSelected(v.id)}
                >
                  {v.name}
                </Button>
                {v.synthetic && (
                  <p className="text-xs text-muted-foreground">模拟来源</p>
                )}
              </div>
            ),
          },
          {
            id: "revision",
            label: "配置／生效修订",
            cell: (v) => `${v.revision} / ${v.applied_revision ?? "未应用"}`,
          },
          {
            id: "reception",
            label: "云端接收",
            cell: (v) => <State value={v.state} />,
          },
          {
            id: "planning",
            label: "内网处理",
            cell: (v) => (
              <State
                value={
                  v.blocked
                    ? "blocked"
                    : v.planning_paused
                      ? "paused"
                      : "enabled"
                }
              />
            ),
          },
          { id: "error", label: "处理提示", cell: (v) => v.error_code || "—" },
        ]}
        pagination={
          <Pages
            cursor={cursor}
            next={query.data?.next_cursor}
            onChange={setCursor}
          />
        }
      />
      <ResponsivePanel
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected(undefined);
        }}
        title="接入入口详情"
        description="来源范围、租户确认和云端应用分别记录。"
        size="wide"
      >
        {selected && <SourceDetail id={selected} />}
      </ResponsivePanel>
      <ResponsivePanel
        open={create}
        onOpenChange={setCreate}
        title="新建接入入口"
        description="一个入口固定一个租户、来源项目、厂家及入口协议。混合项目上报暂不支持。"
      >
        {create && <SourceEditor done={() => setCreate(false)} />}
      </ResponsivePanel>
      <ResponsivePanel
        open={nodes}
        onOpenChange={setNodes}
        title="云节点登记"
        description="仅能选择部署方已批准的配置引用，密钥和网络地址由运维文件维护。"
      >
        {nodes && <Relays />}
      </ResponsivePanel>
    </div>
  );
}
function SourceDetail({ id }: { id: string }) {
  const c = useParsing(),
    w = useParsingWrite(),
    [reason, setReason] = useState(""),
    [edit, setEdit] = useState(false);
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-source", id],
    queryFn: ({ signal }) => c.client.ingestionSource(c.scope, id, signal),
    refetchInterval: 10000,
  });
  if (query.error) return <ErrorState message={query.error.message} />;
  const d = query.data,
    rev = d?.revisions?.[0];
  if (!d || !rev) return <p role="status">正在读取入口修订…</p>;
  const src = d.source,
    snap = rev.snapshot,
    manage = c.scope.kind === "platform" && c.can(P.IngestionSourceManage);
  if (edit) return <SourceEditor existing={d} done={() => setEdit(false)} />;
  const command = (action: string) =>
    void w.run(
      () =>
        c.client.ingestionSourceCommand(c.scope, id, {
          ...w.meta(
            "source-command",
            { id, revision: src.revision, action, reason },
            reason,
          ),
          expected_revision: src.revision,
          action,
          digest: rev.digest,
        }),
      action === "confirm"
        ? "准确修订已确认。"
        : "命令已记录，请核对云端实际应用状态。",
    );
  return (
    <div className="space-y-5">
      <h3 className="font-medium">{src.name}</h3>
      {src.synthetic && (
        <FormNotice>
          模拟来源。联调内层测量为示例，尚未声明真实硬件兼容。
        </FormNotice>
      )}
      <Facts
        items={[
          ["入口身份", <Digest value={id} />],
          ["来源项目", <Digest value={src.project_id} />],
          ["厂家范围", `${src.manufacturer_scope} · ${src.manufacturer_id}`],
          [
            "入口协议",
            `${snap.ingress_protocol_id} · ${snap.ingress_protocol_version}`,
          ],
          [
            "当前修订",
            `${rev.revision} · ${rev.confirmed ? "租户已确认" : "等待租户确认"}`,
          ],
          ["修订摘要", <Digest value={rev.digest} />],
          [
            "监听",
            `${snap.listener_transport} · ${snap.listener_address}:${snap.listener_port}`,
          ],
          ["认证来源", snap.producer_principal_ids?.join("、")],
          ["云端接收", <State value={src.state} />],
          ["已应用修订", src.applied_revision ?? "尚未应用"],
          [
            "内网处理",
            <State
              value={
                src.blocked
                  ? "blocked"
                  : src.planning_paused
                    ? "paused"
                    : "enabled"
              }
            />,
          ],
          [
            "状态观测",
            d.fresh ? (
              <Time value={d.observed_at} />
            ) : (
              <span>已过期或尚无观测，当前连接状态未知</span>
            ),
          ],
        ]}
      />
      {manage || c.can(P.TenantIngestionSourceConfirm) || c.can(P.TenantIngestionExecutionManage) ? (
        <div className="space-y-3 rounded-lg border p-4">
          <DeviceField
            name="source-command-reason"
            label="操作理由"
            value={reason}
            onChange={setReason}
          />
          <div className="flex flex-wrap gap-2">
            {c.scope.kind === "tenant" && !rev.confirmed && (
              <Button
                disabled={!reason.trim() || w.pending || !c.available}
                onClick={() => command("confirm")}
              >
                确认这份入口修订
              </Button>
            )}
            {c.scope.kind==='tenant' && c.can(P.TenantIngestionExecutionManage) && <Button variant="outline" disabled={!reason.trim()||w.pending||!c.available} onClick={()=>void w.run(()=>c.client.ingestionPlanning(c.scope,id,{...w.meta('planning',{id,reason,paused:src.planning_paused},reason),expected_revision:src.revision,action:src.planning_paused?'resume':'pause'}))}>{src.planning_paused?'恢复自动规划':'暂停自动规划'}</Button>}
            {manage && (
              <>
                <Button
                  variant="outline"
                  disabled={!c.available}
                  onClick={() => setEdit(true)}
                >
                  建立新修订
                </Button>
                {["publish", "pause", "resume", "block", "unblock"].map(
                  (action, n) => (
                    <Button
                      key={action}
                      variant={action === "block" ? "destructive" : "outline"}
                      disabled={
                        !reason.trim() ||
                        w.pending ||
                        !c.available ||
                        (action === "publish" && !rev.confirmed)
                      }
                      onClick={() => command(action)}
                    >
                      {
                        [
                          "发布已确认修订",
                          "暂停云端接收",
                          "恢复云端接收",
                          "立即阻断内网处理",
                          "解除内网阻断",
                        ][n]
                      }
                    </Button>
                  ),
                )}
              </>
            )}
          </div>
          <WriteFeedback value={w} />
        </div>
      ) : null}
      {c.scope.kind === "tenant" && <Execution source={id} />}
      <DirectoryTable
        label="入口修订与确认历史"
        items={d.revisions ?? []}
        rowId={(v) => v.revision}
        empty="暂无历史"
        columns={[
          { id: "revision", label: "修订", cell: (v) => v.revision },
          {
            id: "protocol",
            label: "入口协议",
            cell: (v) =>
              `${v.snapshot.ingress_protocol_id} · ${v.snapshot.ingress_protocol_version}`,
          },
          {
            id: "confirmed",
            label: "租户确认",
            cell: (v) => (v.confirmed ? "已确认" : "待确认"),
          },
          {
            id: "created",
            label: "创建时间",
            cell: (v) => <Time value={v.created_at} />,
          },
        ]}
      />
    </div>
  );
}
function SourceEditor({
  existing,
  done,
}: {
  existing?: IngestionSourceDetail;
  done: () => void;
}) {
  const c = useParsing(),
    w = useParsingWrite(),
    r = existing?.revisions?.[0]?.snapshot;
  const [input, setInput] = useState({
    name: existing?.source.name ?? "",
    tenant_id: r?.tenant_id ?? "",
    source_project_id: r?.source_project_id ?? "",
    manufacturer_id: r?.manufacturer_id ?? "",
    manufacturer_scope: r?.manufacturer_scope ?? "public",
    relay_id: r?.relay_id ?? "",
    ingress_protocol_id: r?.ingress_protocol_id ?? "",
    ingress_protocol_version: r?.ingress_protocol_version ?? "",
    listener_address: r?.listener_address ?? "127.0.0.1",
    listener_port: r?.listener_port ?? 19001,
    source_code: r?.source_code ?? "",
    principals: r?.producer_principal_ids?.join(",") ?? "",
    upstream_segregated: false,
    synthetic: r?.synthetic ?? false,
    exclusive: r?.exclusive ?? false,
    fixed_terminal: r?.fixed_terminal ?? "",
    idle_seconds: r?.idle_seconds ?? 180,
    budget_bytes: r?.budget_bytes ?? "1073741824",
    reason: "",
  });
  const set = <K extends keyof typeof input>(
    key: K,
    value: (typeof input)[K],
  ) => setInput((v) => ({ ...v, [key]: value }));
  const opts = useQuery({
      queryKey: [...c.prefix, "ingestion-source-options"],
      queryFn: ({ signal }) => c.client.ingestionSourceOptions(signal),
    }),
    nodes = useQuery({
      queryKey: [...c.prefix, "ingestion-relays"],
      queryFn: ({ signal }) => c.client.ingestionRelays(signal),
    }),
    protocols = useQuery({
      queryKey: [...c.prefix, "ingestion-enabled-protocols"],
      queryFn: ({ signal }) =>
        c.client.ingestionCapabilities(
          c.scope,
          "ingress-protocols",
          { state: "enabled", limit: 100 },
          signal,
        ),
    });
  const choices =
    protocols.data?.items.filter((v) => "transport_adapter" in v.entry) ?? [];
  const save = () =>
    void w.run(async () => {
      const { principals, ...rest } = input;
      const value: IngestionSourceInput = {
        ...rest,
        ...w.meta("source-save", input, input.reason),
        producer_principal_ids: principals
          .split(",")
          .map((v) => v.trim())
          .filter(Boolean),
        expected_revision: existing?.source.revision ?? "0",
      };
      await c.client.ingestionSaveSource(value, existing?.source.id);
      done();
    });
  return (
    <div className="space-y-4">
      {(opts.error || nodes.error || protocols.error) && (
        <ErrorState
          message={
            opts.error?.message ??
            nodes.error?.message ??
            protocols.error?.message ??
            "选项读取失败"
          }
        />
      )}
      <DeviceField
        name="source-name"
        label="入口名称"
        value={input.name}
        onChange={(v) => set("name", v)}
      />
      <DeviceSelect
        name="source-tenant"
        label="租户"
        value={input.tenant_id}
        disabled={!!existing}
        onChange={(v) => {
          set("tenant_id", v);
          set("source_project_id", "");
          set("manufacturer_id", "");
        }}
        options={[
          { value: "", label: "请选择租户" },
          ...(opts.data?.tenants ?? []).map((v) => ({
            value: v.id,
            label: v.name,
          })),
        ]}
      />
      <DeviceSelect
        name="source-project"
        label="来源项目"
        value={input.source_project_id}
        disabled={!!existing}
        onChange={(v) => set("source_project_id", v)}
        options={[
          { value: "", label: "请选择来源项目" },
          ...(opts.data?.projects ?? [])
            .filter((v) => v.tenant_id === input.tenant_id)
            .map((v) => ({ value: v.id, label: v.name })),
        ]}
      />
      <DeviceSelect
        name="source-maker"
        label="厂家及作用范围"
        value={input.manufacturer_id}
        disabled={!!existing}
        onChange={(v) => {
          set("manufacturer_id", v);
          set(
            "manufacturer_scope",
            opts.data?.manufacturers.find((m) => m.id === v)?.scope ?? "public",
          );
        }}
        options={[
          { value: "", label: "请选择厂家" },
          ...(opts.data?.manufacturers ?? [])
            .filter((v) => !v.tenant_id || v.tenant_id === input.tenant_id)
            .map((v) => ({
              value: v.id,
              label: `${v.name} · ${v.scope === "public" ? "公共" : "本租户"}`,
            })),
        ]}
      />
      <DeviceSelect
        name="source-relay"
        label="已登记云节点"
        value={input.relay_id}
        disabled={!!existing}
        onChange={(v) => set("relay_id", v)}
        options={[
          { value: "", label: "请选择节点" },
          ...(nodes.data?.items ?? []).map((v) => ({
            value: v.id,
            label: v.name,
          })),
        ]}
      />
      <DeviceSelect
        name="source-protocol"
        label="入口协议固定版本"
        value={input.ingress_protocol_id + "@" + input.ingress_protocol_version}
        onChange={(v) => {
          const [id = "", version = ""] = v.split("@");
          set("ingress_protocol_id", id);
          set("ingress_protocol_version", version);
        }}
        options={[
          { value: "@", label: "请选择已启用协议" },
          ...choices.flatMap((v) =>
            "transport_adapter" in v.entry
              ? [
                  {
                    value: v.entry.id + "@" + v.entry.version,
                    label: `${v.entry.name} · ${v.entry.version}`,
                  },
                ]
              : [],
          ),
        ]}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <DeviceField
          name="source-address"
          label="批准的监听 IP"
          value={input.listener_address}
          onChange={(v) => set("listener_address", v)}
        />
        <DeviceField
          name="source-port"
          label="监听端口"
          value={String(input.listener_port)}
          onChange={(v) => set("listener_port", Number(v))}
        />
      </div>
      <DeviceField
        name="source-code"
        label="报送路径代码"
        value={input.source_code}
        onChange={(v) => set("source_code", v)}
      />
      <DeviceField
        name="source-principals"
        label="已批准的报送身份（逗号分隔）"
        value={input.principals}
        onChange={(v) => set("principals", v)}
      />
      {input.ingress_protocol_id==='example.authenticated-reading'&&<><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={input.exclusive} onChange={e=>set('exclusive',e.target.checked)}/>已核对该认证来源独占一个固定终端</label><DeviceField name="source-fixed-terminal" label="独占来源固定终端（11 字节）" value={input.fixed_terminal} onChange={v=>set('fixed_terminal',v)}/></>}
      <div className="grid gap-3 sm:grid-cols-2">
        <DeviceField
          name="source-idle"
          label="连接空闲秒数"
          value={String(input.idle_seconds)}
          onChange={(v) => set("idle_seconds", Number(v))}
        />
        <DeviceField
          name="source-budget"
          label="云缓冲预算（字节）"
          value={input.budget_bytes}
          onChange={(v) => set("budget_bytes", v)}
        />
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={input.upstream_segregated}
          onChange={(e) => set("upstream_segregated", e.target.checked)}
        />
        已核对上游只向此入口报送该租户、项目和厂家的数据
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          disabled={!!existing}
          checked={input.synthetic}
          onChange={(e) => set("synthetic", e.target.checked)}
        />
        模拟来源
      </label>
      <DeviceField
        name="source-save-reason"
        label="创建／修订理由"
        value={input.reason}
        onChange={(v) => set("reason", v)}
      />
      <WriteFeedback value={w} />
      <Button
        disabled={
          w.pending ||
          !c.available ||
          !input.reason.trim() ||
          !input.name.trim() ||
          !input.tenant_id ||
          !input.source_project_id ||
          !input.manufacturer_id ||
          !input.ingress_protocol_id ||
          !input.upstream_segregated
        }
        onClick={save}
      >
        保存并等待租户确认
      </Button>
    </div>
  );
}
function Relays() {
  const c = useParsing(),
    w = useParsingWrite(),
    [id, setID] = useState(""),
    [name, setName] = useState(""),
    [reason, setReason] = useState("");
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-relays"],
    queryFn: ({ signal }) => c.client.ingestionRelays(signal),
  });
  return (
    <div className="space-y-4">
      <FormNotice>
        {query.data?.enabled
          ? "接入服务已配置。"
          : "接入默认关闭；部署方须提供私有配置文件并启动接入进程。"}
      </FormNotice>
      <DeviceSelect
        name="relay-ref"
        label="部署批准配置"
        value={id}
        onChange={setID}
        options={[
          { value: "", label: "请选择配置引用" },
          ...(query.data?.configured ?? []).map((v) => ({
            value: v.id,
            label: v.configuration_ref,
          })),
        ]}
      />
      <DeviceField
        name="relay-name"
        label="节点显示名称"
        value={name}
        onChange={setName}
      />
      <DeviceField
        name="relay-reason"
        label="登记理由"
        value={reason}
        onChange={setReason}
      />
      <Button
        disabled={!id || !name.trim() || !reason.trim() || w.pending}
        onClick={() =>
          void w.run(() =>
            c.client.ingestionCreateRelay({
              ...w.meta("relay", { id, name, reason }, reason),
              id,
              name,
              configuration_ref: query.data!.configured.find(
                (v) => v.id === id,
              )!.configuration_ref,
            }),
          )
        }
      >
        登记云节点
      </Button>
      <WriteFeedback value={w} />
      {query.error && <ErrorState message={query.error.message} />}
    </div>
  );
}
