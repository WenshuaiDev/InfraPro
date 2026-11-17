import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Permissions as P, type IngestionTerminal } from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { FormNotice, ErrorState } from "@infrapro/ui/patterns/feedback";
import { DeviceField, DeviceSelect } from "../parsing/fields";
import { useParsing, useParsingWrite, WriteFeedback } from "../parsing/context";
import { Facts, Pages, State, Time } from "./common";
export function Terminals({
  source,
  initial,
}: {
  source?: string;
  initial?: string;
}) {
  const c = useParsing(),
    [cursor, setCursor] = useState(""),
    [q, setQ] = useState(""),
    [sourceID, setSource] = useState(source ?? ""),
    [selected, setSelected] = useState(initial),
    [create, setCreate] = useState(false);
  const sources = useQuery({
      queryKey: [...c.prefix, "ingestion-source-choices"],
      queryFn: ({ signal }) =>
        c.client.ingestionSources(c.scope, { limit: 100 }, signal),
      enabled: c.can(P.TenantIngestionSourceRead) && c.can(P.TenantProjectRead),
    }),
    query = useQuery({
      queryKey: [...c.prefix, "ingestion-terminals", cursor, q, sourceID],
      queryFn: ({ signal }) =>
        c.client.ingestionTerminals(
          c.scope,
          { cursor, q, source_id: sourceID, limit: 25 },
          signal,
        ),
    });
  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <DeviceField
          name="terminal-search"
          label="终端名称或完整外部编号"
          value={q}
          onChange={(v) => {
            setQ(v);
            setCursor("");
          }}
        />
        <DeviceSelect
          name="terminal-source"
          label="来源入口"
          value={sourceID}
          onChange={(v) => {
            setSource(v);
            setCursor("");
          }}
          options={[
            { value: "", label: "全部获配入口" },
            ...(sources.data?.items ?? []).map((v) => ({
              value: v.id,
              label: v.name,
            })),
          ]}
        />
      </div>
      {c.can(P.TenantIngestionTerminalManage) &&
        c.can(P.TenantIngestionSourceRead) && (
          <Button disabled={!c.available} onClick={() => setCreate(true)}>
            登记接入终端
          </Button>
        )}
      <DirectoryTable
        label="接入终端"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="尚未登记终端。外部编号只在其来源命名空间内唯一。"
        columns={[
          {
            id: "name",
            label: "终端与原始身份",
            cell: (v) => (
              <div>
                <Button
                  variant="link"
                  className="h-auto p-0"
                  onClick={() => setSelected(v.id)}
                >
                  {v.name}
                </Button>
                <p className="font-mono text-xs">{v.terminal_key}</p>
              </div>
            ),
          },
          {
            id: "source",
            label: "入口",
            cell: (v) =>
              sources.data?.items?.find((s) => s.id === v.source_id)?.name ??
              v.source_id,
          },
          {
            id: "mode",
            label: "关联方式",
            cell: (v) =>
              v.mode === "multiple" ? "多设备／子地址" : "单设备整路",
          },
          { id: "revision", label: "修订", cell: (v) => v.revision },
          {
            id: "state",
            label: "状态",
            cell: (v) => <State value={v.enabled ? "enabled" : "paused"} />,
          },
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
        open={create}
        onOpenChange={setCreate}
        title="登记接入终端"
        description="原始编号保留前导零；登记不会自动开放早期未匹配报文。"
      >
        {create && (
          <TerminalEditor source={sourceID} done={() => setCreate(false)} />
        )}
      </ResponsivePanel>
      <ResponsivePanel
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected(undefined);
        }}
        title="终端关联与执行授权"
        description="关联按云端接收序号区间生效；设备采用保持独立。"
        size="wide"
      >
        {selected && <TerminalDetail id={selected} />}
      </ResponsivePanel>
    </div>
  );
}
function TerminalEditor({
  source,
  done,
  existing,
}: {
  source: string;
  done: () => void;
  existing?:IngestionTerminal;
}) {
  const c = useParsing(),
    w = useParsingWrite(),
    [sourceID, setSource] = useState(source),
    [key, setKey] = useState(existing?.terminal_key??""),
    [name, setName] = useState(existing?.name??""),
    [mode, setMode] = useState(existing?.mode??"single"),
    [enabled,setEnabled]=useState(existing?.enabled??true),
    [demux, setDemux] = useState(existing?.demultiplexer?`${existing.demultiplexer.id}@${existing.demultiplexer.version}`:""),
    [reason, setReason] = useState("");
  const sources = useQuery({
      queryKey: [...c.prefix, "ingestion-source-choices"],
      queryFn: ({ signal }) =>
        c.client.ingestionSources(c.scope, { limit: 100 }, signal),
    }),
    methods = useQuery({
      queryKey: [...c.prefix, "ingestion-demultiplexers"],
      queryFn: ({ signal }) =>
        c.client.ingestionCapabilities(
          c.scope,
          "identification-methods",
          { state: "enabled", limit: 100 },
          signal,
        ),
      enabled: c.canMethods,
    });
  return (
    <div className="space-y-4">
      <DeviceSelect
        name="new-terminal-source"
        disabled={!!existing}
        label="已确认入口"
        value={sourceID}
        onChange={setSource}
        options={[
          { value: "", label: "请选择入口" },
          ...(sources.data?.items ?? []).map((v) => ({
            value: v.id,
            label: v.name,
          })),
        ]}
      />
      <DeviceField
        name="terminal-key"
        disabled={!!existing}
        label="原始终端编号（11 字节）"
        value={key}
        onChange={setKey}
      />
      <DeviceField
        name="terminal-name"
        label="终端名称"
        value={name}
        onChange={setName}
      />
      <DeviceSelect
        name="terminal-mode"
        label="终端结构"
        value={mode}
        onChange={setMode}
        options={[
          { value: "single", label: "整路关联一台设备" },
          { value: "multiple", label: "按子地址关联多台设备" },
        ]}
      />
      {mode === "multiple" && (
        <DeviceSelect
          name="terminal-demux"
          label="固定解复用方法"
          value={demux}
          onChange={setDemux}
          options={[
            { value: "", label: "请选择已启用方法" },
            ...(existing?.demultiplexer && !methods.data?.items.some(v=>"method" in v.entry && v.entry.method.id===existing.demultiplexer?.id && v.entry.method.version===existing.demultiplexer?.version) ? [{value:`${existing.demultiplexer.id}@${existing.demultiplexer.version}`,label:`当前固定方法 · ${existing.demultiplexer.id} · ${existing.demultiplexer.version}`}] : []),
            ...(methods.data?.items ?? []).flatMap((v) =>
              "purpose" in v.entry && v.entry.purpose === "terminal_demultiplex"
                ? [
                    {
                      value: `${v.entry.method.id}@${v.entry.method.version}`,
                      label: `${v.entry.name} · ${v.entry.method.version}`,
                    },
                  ]
                : [],
            ),
          ]}
        />
      )}
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/>启用终端</label>
      <DeviceField
        name="terminal-reason"
        label="登记／修订理由"
        value={reason}
        onChange={setReason}
      />
      <WriteFeedback value={w} />
      <Button
        disabled={
          !sourceID ||
          key.length !== 11 ||
          !name.trim() ||
          !reason.trim() ||
          (mode === "multiple" && !demux) ||
          w.pending ||
          !c.available
        }
        onClick={() =>
          void w.run(async () => {
            const entry = methods.data?.items.find(
              (v) => "method" in v.entry && `${v.entry.method.id}@${v.entry.method.version}`===demux,
            )?.entry;
            const input = {
              ...w.meta(
                "terminal-create",
                { sourceID, key, name, mode, demux, reason },
                reason,
              ),
              expected_revision:existing?.revision,enabled,
              source_id: sourceID,
              terminal_key: key,
              name,
              mode,
              ...(entry && "purpose" in entry
                ? { demultiplexer: entry.method }
                : existing?.demultiplexer && demux===`${existing.demultiplexer.id}@${existing.demultiplexer.version}` && mode==="multiple" ? {demultiplexer:existing.demultiplexer} : {}),
            };
            if(existing)await c.client.ingestionReviseTerminal(c.scope,existing.id,input);else await c.client.ingestionCreateTerminal(c.scope,input);
            done();
          })
        }
      >
        {existing?"保存终端新修订":"登记终端"}
      </Button>
    </div>
  );
}
function TerminalDetail({ id }: { id: string }) {
  const c = useParsing(),
    w = useParsingWrite(),
    [reason, setReason] = useState(""),
    [device, setDevice] = useState(""),
    [q, setQ] = useState(""),
    [child, setChild] = useState(""),
    [view,setView]=useState("complete_frame"),[editing,setEditing]=useState(false),
    [prepared, setPrepared] = useState(false);
  const query = useQuery({
      queryKey: [...c.prefix, "ingestion-terminal", id],
      queryFn: ({ signal }) => c.client.ingestionTerminal(c.scope, id, signal),
    }),
    associations = useQuery({
      queryKey: [...c.prefix, "ingestion-associations", id],
      queryFn: ({ signal }) =>
        c.client.ingestionAssociations(c.scope, id, signal),
      refetchInterval: 10000,
    });
  const history=useQuery({queryKey:[...c.prefix,"ingestion-terminal-history",id],queryFn:({signal})=>c.client.ingestionTerminalHistory(c.scope,id,signal)});
  const devices = useQuery({
    queryKey: [...c.prefix, "ingestion-device-candidates", q],
    queryFn: ({ signal }) => {
      if (c.scope.kind === "platform") throw Error("租户范围必需");
      return c.client.devices(c.scope, { q, limit: 100 }, signal);
    },
    enabled:
      c.can(P.TenantDeviceRead) && c.can(P.TenantIngestionTerminalManage),
  });
  if (query.error) return <ErrorState message={query.error.message} />;
  const t = query.data;
  if (!t) return <p role="status">正在读取终端…</p>;
  if(editing)return <><Button variant="outline" onClick={()=>setEditing(false)}>返回终端</Button><TerminalEditor source={t.source_id} existing={t} done={()=>setEditing(false)}/></>;
  const input = () => ({
    ...w.meta(
      "associate",
      { id, device, child, revision: t.revision, reason },
      reason,
    ),
    expected_revision: t.revision,
    device_id: device,
    child_key: t.mode === "single" ? "" : child,
    input_view: t.mode === "single" ? view : "json_reading",
  });
  return (
    <div className="space-y-5">
      <Facts
        items={[
          ["终端", t.name],
          ["原始身份", <code>{t.terminal_key}</code>],
          ["修订", t.revision],
          ["结构", t.mode === "single" ? "单设备整路" : "多设备子地址"],
          [
            "解复用",
            t.demultiplexer
              ? `${t.demultiplexer.id} · ${t.demultiplexer.version}`
              : "无需解复用",
          ],
        ]}
      />
      {c.can(P.TenantIngestionTerminalManage)&&<Button variant="outline" onClick={()=>setEditing(true)}>修订终端</Button>}
      <details><summary className="cursor-pointer text-sm">终端修订历史（{history.data?.length??0}）</summary><ul className="space-y-2 py-3 text-sm">{history.data?.map(v=><li key={v.revision}>修订 {v.revision} · {v.name} · {v.mode==='single'?'单路':'多路'} · {v.demultiplexer?.version??'无需解复用'} · {v.enabled?'启用':'停用'}</li>)}</ul></details>
      <DirectoryTable
        label="按接收序号生效的关联历史"
        items={associations.data ?? []}
        rowId={(v) => v.id}
        loading={associations.isPending}
        error={associations.error?.message}
        empty="暂无关联；新报文将保持待关联状态。"
        columns={[
          {
            id: "device",
            label: "设备／子地址",
            cell: (v) => (
              <div>
                {devices.data?.items?.find((d) => d.id === v.device_id)?.name ??
                  v.device_id}
                <small className="block">{v.child_key || "整路"}</small>
              </div>
            ),
          },
          {
            id: "range",
            label: "云端序号区间",
            cell: (v) => (
              <span>
                {v.start_sequence} ≤ N{" "}
                {v.end_sequence ? `< ${v.end_sequence}` : "至后续切换"}
                <small className="block break-all">epoch {v.epoch}</small>
              </span>
            ),
          },
          {
            id: "state",
            label: "关联状态",
            cell: (v) => <State value={v.enabled ? "enabled" : "paused"} />,
          },
          {
            id: "created",
            label: "建立时间",
            cell: (v) => <Time value={v.created_at} />,
          },
          {
            id: "action",
            label: "操作",
            cell: (v) =>
              c.can(P.TenantIngestionTerminalManage) ? (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!reason.trim() || w.pending || !c.available}
                  onClick={() =>
                    void w.run(() =>
                      c.client.ingestionAssociationState(c.scope, v.id, {
                        ...w.meta(
                          "association-state",
                          { id: v.id, enabled: !v.enabled, reason },
                          reason,
                        ),
                        expected_revision: t.revision,
                        action: v.enabled ? "disable" : "enable",
                      }),
                    )
                  }
                >
                  {v.enabled ? "停用关联" : "恢复关联"}
                </Button>
              ) : null,
          },
        ]}
      />
      {c.can(P.TenantIngestionTerminalManage) && (
        <div className="space-y-3 rounded-lg border p-4">
          <h3 className="font-medium">建立或切换关联</h3>
          <FormNotice>
            提交后暂停该入口规划，取得云端持久水位 N，再让新关联从 N+1
            起生效。来源项目必须已获准使用所选设备。
          </FormNotice>
          <DeviceField
            name="association-device-search"
            label="搜索设备"
            value={q}
            onChange={setQ}
          />
          <DeviceSelect
            name="association-device"
            label="目标设备"
            value={device}
            onChange={(v) => {
              setDevice(v);
              setPrepared(false);
            }}
            options={[
              { value: "", label: "请选择设备" },
              ...(devices.data?.items ?? []).map((v) => ({
                value: v.id,
                label: `${v.name} · ${v.code} · ${v.product.name}`,
              })),
            ]}
          />
          {t.mode === "single" && <DeviceSelect name="association-view" label="设备解析输入视图" value={view} onChange={v=>{setView(v);setPrepared(false)}} options={[{value:"complete_frame",label:"完整 DDP 帧"},{value:"json_reading",label:"独立 reading JSON"}]}/>}
          {t.mode === "multiple" && (
            <DeviceField
              name="association-child"
              label="子地址"
              value={child}
              onChange={(v) => {
                setChild(v);
                setPrepared(false);
              }}
            />
          )}
          <DeviceField
            name="association-reason"
            label="关联／状态操作理由"
            value={reason}
            onChange={(v) => {
              setReason(v);
              setPrepared(false);
            }}
          />
          <div className="flex gap-2">
            <Button
              variant="outline"
              disabled={!device || !reason.trim() || w.pending || !c.available}
              onClick={() =>
                void w.run(async () => {
                  await c.client.ingestionAssociate(c.scope, id, input(), true);
                  setPrepared(true);
                }, "设备实际采用和入口兼容性已核验。")
              }
            >
              预览关联
            </Button>
            {prepared && (
              <Button
                disabled={w.pending || !c.available}
                onClick={() =>
                  void w.run(async () => {
                    await c.client.ingestionAssociate(c.scope, id, input());
                    setPrepared(false);
                  }, "关联意图已提交；请在运行概况核对水位应用。")
                }
              >
                确认按水位切换
              </Button>
            )}
          </div>
        </div>
      )}
      <WriteFeedback value={w} />
      {c.can(P.TenantIngestionSourceRead) && <Execution terminal={t} />}
    </div>
  );
}
export function Execution({
  terminal,
  source,
}: {
  terminal?: IngestionTerminal;
  source?: string;
}) {
  const c = useParsing(),
    w = useParsingWrite(),
    [reason, setReason] = useState(""),
    [budget, setBudget] = useState("10000");
  const sourceID = terminal?.source_id ?? source ?? "";
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-grants", sourceID],
    queryFn: ({ signal }) =>
      c.client.ingestionGrants(c.scope, { source_id: sourceID }, signal),
    enabled: !!sourceID,
  });
  const grant = query.data?.find((g) =>
    terminal ? g.terminal_id === terminal.id : !g.terminal_id,
  );
  return (
    <div className="space-y-3 rounded-lg border p-4">
      <h3 className="font-medium">
        {terminal ? "终端测量执行授权" : "入口识别与自动规划授权"}
      </h3>
      <p className="text-sm text-muted-foreground">
        授权以租户机器任务运行，独立于创建人的后续账号状态。接收、识别和测量执行分别控制。
      </p>
      <State value={grant?.enabled ? "enabled" : "paused"} />
      {c.can(P.TenantIngestionExecutionManage) && (
        <>
          <DeviceField
            name={`grant-budget-${terminal?.id ?? "source"}`}
            label="处理队列预算（条）"
            value={budget}
            onChange={setBudget}
          />
          <DeviceField
            name={`grant-reason-${terminal?.id ?? "source"}`}
            label="授权变更理由"
            value={reason}
            onChange={setReason}
          />
          <Button
            variant={grant?.enabled ? "outline" : "default"}
            disabled={!sourceID || !reason.trim() || w.pending || !c.available}
            onClick={() =>
              void w.run(() =>
                c.client.ingestionSaveGrant(c.scope, {
                  ...w.meta(
                    "grant",
                    {
                      sourceID,
                      terminal: terminal?.id,
                      enabled: !grant?.enabled,
                      reason,
                      budget,
                    },
                    reason,
                  ),
              source_id: sourceID,
                  terminal_id: terminal?.id,
                  expected_revision: grant?.generation ?? "0",
                  identify: !terminal,
                  measure: !!terminal,
                  enabled: !grant?.enabled,
                  budget: Number(budget),
                }),
              )
            }
          >
            {grant?.enabled ? "暂停自动处理" : "启用自动处理"}
          </Button>
        </>
      )}
      <WriteFeedback value={w} />
      {query.error && <ErrorState message={query.error.message} />}
    </div>
  );
}
