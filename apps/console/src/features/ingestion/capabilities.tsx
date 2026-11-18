import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useState } from "react";
import {
  Permissions as P,
  type IngestionCapability,
  type IngestionCapabilityKind,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { FormNotice } from "@infrapro/ui/patterns/feedback";
import { DeviceField, DeviceSelect } from "../parsing/fields";
import { useParsing, useParsingWrite, WriteFeedback } from "../parsing/context";
import { Digest, Facts, State } from "./common";
const kinds: { value: IngestionCapabilityKind; label: string }[] = [
  { value: "ingress-protocols", label: "入口协议" },
  { value: "identification-methods", label: "识别与解复用方法" },
  { value: "ingress-compatibilities", label: "精确兼容组合" },
];
function name(v: IngestionCapability) {
  const e = v.entry;
  return "name" in e
    ? e.name
    : "ingress_protocol_id" in e
      ? `${e.ingress_protocol_id} · ${e.measurement.id}`
      : "能力";
}
export function Capabilities() {
  const c = useParsing(),
    w = useParsingWrite(),
    [kind, setKind] = useState<IngestionCapabilityKind>("ingress-protocols"),
    [q, setQ] = useState(""),
    [selected, setSelected] = useState<IngestionCapability>(),
    [state, setState] = useState("enabled"),
    [reason, setReason] = useState("");
  const query = usePagedQuery({ initialPageSize: 50,
    queryKey: [...c.prefix, "ingestion-capabilities", kind, q],
    queryFn: ({ cursor, signal, limit }) => c.client.ingestionCapabilities(
        c.scope,
        kind,
        { cursor, q, limit },
        signal,
      ),
    enabled: c.canMethods,
  });
  return (
    <div className="space-y-4">
      <FormNotice>
        入口协议负责来源身份和输入切分；测量方法由设备的实际解析采用决定。方法定义及兼容组合由平台发布，租户只登记终端、关联与执行授权。
      </FormNotice>
      <div className="grid gap-3 sm:grid-cols-2">
        <DeviceSelect
          name="capability-kind"
          label="接入能力类型"
          value={kind}
          onChange={(v) => {
            setKind(v as IngestionCapabilityKind);
            }}
          options={kinds}
        />
        <DeviceField
          name="capability-search"
          label="搜索能力"
          value={q}
          onChange={(v) => {
            setQ(v);
            }}
        />
      </div>
      {c.scope.kind === "platform" && c.can(P.ParsingMethodManage) && (
        <Button
          disabled={w.pending || !c.available}
          onClick={() =>
            void w.run(async () => {
              const result = await c.client.ingestionSync(
                w.meta("ingestion-sync", {}),
              );
              if (result.state === "failed")
                throw Error(result.error_code || "接入能力服务不可用");
            }, "目录同步完成；请核对固定产物并启用。")
          }
        >
          同步接入能力目录
        </Button>
      )}
      <WriteFeedback value={w} />
      <DirectoryTable
        label="接入能力目录"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="尚无可用接入能力。请检查独立解析服务并同步目录。"
        columns={[
          {
            id: "name",
            label: "能力及用途",
            cell: (v) => (
              <Button
                variant="link"
                className="h-auto whitespace-normal p-0 text-left"
                onClick={() => {
                  setSelected(v);
                  setState(v.state);
                }}
              >
                {name(v)}
              </Button>
            ),
          },
          {
            id: "version",
            label: "固定版本",
            cell: (v) =>
              "version" in v.entry
                ? v.entry.version
                : "method" in v.entry
                  ? v.entry.method.version
                  : v.entry.ingress_protocol_version,
          },
          {
            id: "state",
            label: "状态",
            cell: (v) => <State value={v.state} />,
          },
          {
            id: "available",
            label: "产物覆盖",
            cell: (v) => (v.available ? "已保留" : "当前不可用"),
          },
        ]}
        pagination={
          <ListPagination label="接入能力" {...query.pagination} />
        }
      />
      <ResponsivePanel
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected(undefined);
        }}
        title="接入能力固定依据"
        description="版本、产物摘要与兼容组合分别保存，不修改旧测量清单。"
      >
        {selected && (
          <div className="space-y-5">
            <h3 className="font-medium">{name(selected)}</h3>
            {"purpose" in selected.entry ? (
              <Facts
                items={[
                  [
                    "用途",
                    selected.entry.purpose === "terminal_demultiplex"
                      ? "终端解复用"
                      : "来源识别",
                  ],
                  [
                    "输入",
                    `${selected.entry.input_unit} · ${selected.entry.transport}`,
                  ],
                  [
                    "执行产物",
                    <Digest value={selected.entry.method.artifact_digest} />,
                  ],
                  [
                    "清单",
                    <Digest value={selected.entry.method.manifest_digest} />,
                  ],
                  ["说明", selected.entry.release_notes],
                ]}
              />
            ) : "transport_adapter" in selected.entry ? (
              <Facts
                items={[
                  [
                    "入口协议",
                    `${selected.entry.id} · ${selected.entry.version}`,
                  ],
                  ["传输", selected.entry.transport],
                  [
                    "身份识别",
                    `${selected.entry.identification.id} · ${selected.entry.identification.version}`,
                  ],
                  [
                    "适配器",
                    `${selected.entry.transport_adapter.id} · ${selected.entry.transport_adapter.version}`,
                  ],
                  ["版本摘要", <Digest value={selected.entry.digest} />],
                ]}
              />
            ) : (
              <Facts
                items={[
                  [
                    "入口协议",
                    `${selected.entry.ingress_protocol_id} · ${selected.entry.ingress_protocol_version}`,
                  ],
                  [
                    "测量方法",
                    `${selected.entry.measurement.id} · ${selected.entry.measurement.version}`,
                  ],
                  ["输入视图", selected.entry.input_view],
                  ["解析传输", selected.entry.parse_transport],
                  ["组合摘要", <Digest value={selected.entry.digest} />],
                ]}
              />
            )}
            <State value={selected.state} />
            {c.scope.kind === "platform" && c.can(P.ParsingMethodManage) && (
              <>
                <DeviceSelect
                  name="capability-state"
                  label="能力状态"
                  value={state}
                  onChange={setState}
                  options={[
                    { value: "pending", label: "待启用" },
                    { value: "enabled", label: "启用" },
                    { value: "deprecated", label: "停止新采用" },
                    { value: "blocked", label: "阻断执行" },
                  ]}
                />
                <DeviceField
                  name="capability-reason"
                  label="状态变更理由"
                  value={reason}
                  onChange={setReason}
                />
                <Button
                  disabled={!reason.trim() || w.pending || !c.available}
                  onClick={() =>
                    void w.run(async () => {
                      await c.client.ingestionCapabilityState(
                        kind,
                        selected.id,
                        {
                          ...w.meta(
                            "capability-state",
                            { id: selected.id, state, reason },
                            reason,
                          ),
                          expected_revision: selected.revision,
                          state,
                        },
                      );
                      setSelected(undefined);
                    })
                  }
                >
                  保存能力状态
                </Button>
              </>
            )}
            <WriteFeedback value={w} />
          </div>
        )}
      </ResponsivePanel>
    </div>
  );
}
