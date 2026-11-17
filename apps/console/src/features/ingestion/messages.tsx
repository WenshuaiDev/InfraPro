import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Permissions as P, type IngestionFilters } from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { ErrorState, FormNotice } from "@infrapro/ui/patterns/feedback";
import { DeviceField, DeviceSelect } from "../parsing/fields";
import { useParsing, useParsingWrite, WriteFeedback } from "../parsing/context";
import { Digest, Facts, Pages, State, Time } from "./common";
import { RecoveryEditor } from "./recovery";
export function Messages({
  source,
  device,
}: {
  source?: string;
  device?: string;
}) {
  const c = useParsing(),
    [filter, setFilter] = useState<IngestionFilters>({
      source_id: source ?? "",
      device_id: device ?? "",
      limit: 25,
    }),
    [selected, setSelected] = useState("");
  const update = (key: keyof IngestionFilters, value: string) =>
    setFilter((f) => ({ ...f, [key]: value, cursor: "" }));
  const sources = useQuery({
    queryKey: [...c.prefix, "ingestion-source-choices"],
    queryFn: ({ signal }) =>
      c.client.ingestionSources(c.scope, { limit: 100 }, signal),
    enabled: c.can(P.TenantIngestionSourceRead) && c.can(P.TenantProjectRead),
  });
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-messages", filter],
    queryFn: ({ signal }) =>
      c.client.ingestionMessages(c.scope, filter, signal),
  });
  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <DeviceSelect
          name="message-source"
          label="来源入口"
          value={filter.source_id ?? ""}
          onChange={(v) => update("source_id", v)}
          options={[
            { value: "", label: "全部可见入口" },
            ...(sources.data?.items ?? []).map((v) => ({
              value: v.id,
              label: v.name,
            })),
          ]}
        />
        <DeviceSelect
          name="message-state"
          label="处理状态"
          value={filter.state ?? ""}
          onChange={(v) => update("state", v)}
          options={[
            { value: "", label: "全部状态" },
            ...[
              "parsed",
              "planned",
              "control",
              "blocked",
              "awaiting_configuration",
              "failed",
            ].map((v) => ({
              value: v,
              label:
                (
                  {
                    parsed: "解析完成",
                    planned: "已规划",
                    control: "控制包",
                    blocked: "已阻断",
                    awaiting_configuration: "待配置",
                    failed: "失败",
                  } as Record<string, string>
                )[v] ?? v,
            })),
          ]}
        />
        <DeviceField
          name="message-version"
          label="测量方法版本"
          value={filter.method_version ?? ""}
          onChange={(v) => update("method_version", v)}
        />
        <DeviceField
          name="message-receipt"
          label="接收 ID"
          value={filter.receipt_id ?? ""}
          onChange={(v) => update("receipt_id", v)}
        />
        <DeviceField
          name="message-from"
          label="接收时间起（RFC3339）"
          value={filter.from ?? ""}
          onChange={(v) => update("from", v)}
        />
        <DeviceField
          name="message-error"
          label="错误代码"
          value={filter.error_code ?? ""}
          onChange={(v) => update("error_code", v)}
        />
      </div>
      <DirectoryTable
        label="已核验原始报文单元"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="暂无符合筛选的已核验单元。未匹配数据在独立区域保管。"
        columns={[
          {
            id: "identity",
            label: "终端／子地址",
            cell: (v) => (
              <div>
                <Button
                  variant="link"
                  className="h-auto p-0 font-mono"
                  onClick={() => setSelected(v.id)}
                >
                  {v.terminal_key}
                  {v.child_key ? ` / ${v.child_key}` : ""}
                </Button>
                {v.synthetic && (
                  <p className="text-xs text-muted-foreground">模拟来源</p>
                )}
              </div>
            ),
          },
          {
            id: "received",
            label: "接收时间",
            cell: (v) => <Time value={v.received_at} />,
          },
          {
            id: "protocol",
            label: "入口／测量版本",
            cell: (v) => (
              <div>
                {v.ingress_version}
                <small className="block">
                  {v.method_version ?? "尚未规划"}
                </small>
              </div>
            ),
          },
          {
            id: "state",
            label: "处理／确认",
            cell: (v) => (
              <div className="space-y-1">
                <State value={v.state} />
                <small className="block">{v.acknowledgement}</small>
              </div>
            ),
          },
          { id: "size", label: "单元字节", cell: (v) => v.byte_length ?? "—" },
          { id: "error", label: "处理提示", cell: (v) => v.error_code || "—" },
        ]}
        pagination={
          <Pages
            cursor={filter.cursor ?? ""}
            next={query.data?.next_cursor}
            onChange={(cursor) => setFilter((f) => ({ ...f, cursor }))}
          />
        }
      />
      <ResponsivePanel
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected("");
        }}
        title="接收、识别与解析证据"
        description="原始接收与派生单元分别保留摘要，所有偏移均标明其父字节空间。"
        size="workspace"
      >
        {selected && <MessageEvidence id={selected} />}
      </ResponsivePanel>
    </div>
  );
}
export function MessageEvidence({ id }: { id: string }) {
  const c = useParsing(),
    w = useParsingWrite(),
    [raw, setRaw] = useState<Uint8Array>(),
    [range, setRange] = useState<{ start: number; end: number }>(),
    [recover, setRecover] = useState(false);
  const query = useQuery({
      queryKey: [...c.prefix, "ingestion-message", id],
      queryFn: ({ signal }) => c.client.ingestionMessage(c.scope, id, signal),
    }),
    decisions = useQuery({
      queryKey: [...c.prefix, "ingestion-decisions", id],
      queryFn: ({ signal }) => c.client.ingestionDecisions(c.scope, id, signal),
      enabled: !!query.data,
    }),
    results = useQuery({
      queryKey: [...c.prefix, "ingestion-results", id],
      queryFn: ({ signal }) => c.client.ingestionResults(c.scope, id, signal),
      enabled: !!query.data?.can_results,
    });
  if (query.error) return <ErrorState message={query.error.message} />;
  const data = query.data;
  if (!data) return <p role="status">正在核验单元权限…</p>;
  const unit = data.unit,
    receipt = data.receipt;
  const save = (blob: Blob, name: string) => { const url = URL.createObjectURL(blob); const a = document.createElement("a"); a.href = url; a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); };
  const content = () =>
    void w.run(async () => {
      const blob = await c.client.ingestionContent(c.scope, id);
      setRaw(new Uint8Array(await blob.arrayBuffer()));
    }, "已读取获准单元；完整批次根不对普通接口开放。");
  return (
    <div className="space-y-6">
      {receipt.envelope.synthetic && (
        <FormNotice>
          模拟报文，联调测量示例；业务质量未评定，结果尚未正式采用为监测观测。
        </FormNotice>
      )}
      <div className="grid gap-6 xl:grid-cols-2">
        <Facts
          items={[
            ["终端／子地址", `${unit.terminal_key} ${unit.child_key}`],
            ["来源项目", receipt.project_id],
            [
              "接收身份",
              <Digest
                value={`${receipt.relay_id} / ${receipt.epoch} / ${receipt.receipt_id}`}
              />,
            ],
            [
              "来源修订／序号",
              `${receipt.source_revision} / ${receipt.source_sequence}`,
            ],
            [
              "接收／保存",
              <>
                <Time value={receipt.received_at} />
                <br />
                <Time value={receipt.stored_at} />
              </>,
            ],
            ["实际报送传输", receipt.envelope.transport],
            [
              "入口协议",
              `${receipt.envelope.ingress_protocol_id} · ${receipt.envelope.ingress_protocol_version}`,
            ],
            ["原接收摘要", <Digest value={receipt.envelope.sha256} />],
          ]}
        />
        <Facts
          items={[
            ["单元身份", <Digest value={unit.id} />],
            ["处理状态", <State value={unit.state} />],
            ["处理提示", unit.error_code || "无"],
            ["输入视图", `${unit.view.kind} · ${unit.view.input_unit}`],
            ["parse_transport", unit.view.parse_transport],
            [
              "单元字节／摘要",
              <>
                {unit.view.byte_length} 字节
                <br />
                <Digest value={unit.view.sha256} />
              </>,
            ],
            ["设备", unit.device_id],
            ["关联依据", unit.association_id],
          ]}
        />
      </div>
      <div className="flex flex-wrap gap-3">
        {data.can_raw && (
          <Button variant="outline" disabled={w.pending} onClick={content}>
            读取已核验单元原文
          </Button>
        )}
        {data.can_raw && <Button variant="outline" disabled={w.pending} onClick={() => void w.run(async () => save(await c.client.ingestionContent(c.scope, id), `ingestion-unit-${id}.bin`))}>下载已核验单元</Button>}
        {data.can_results && <Button variant="outline" disabled={w.pending} onClick={() => void w.run(async () => save(new Blob([JSON.stringify(await c.client.ingestionResults(c.scope, id), null, 2)], { type: "application/json" }), `ingestion-results-${id}.json`))}>下载解析结果</Button>}
        {data.can_reprocess && (
          <Button disabled={!c.available} onClick={() => setRecover(true)}>
            固定此单元重新处理
          </Button>
        )}
      </div>
      <WriteFeedback value={w} />
      {raw && (
        <div className="space-y-2">
          <p className="text-sm">
            当前字节空间：获准单元。
            {range
              ? `选中 [${range.start}, ${range.end})`
              : "点击测量字段可定位字节。"}
          </p>
          <div
            className="max-h-80 overflow-auto rounded border bg-muted/20 p-3 font-mono text-xs"
            aria-label="获准单元十六进制"
          >
            {Array.from({ length: Math.ceil(raw.length / 16) }, (_, row) => (
              <div key={row} className="flex min-w-max gap-3">
                <span className="text-muted-foreground">
                  {(row * 16).toString(16).padStart(6, "0")}
                </span>
                <span>
                  {Array.from(raw.slice(row * 16, row * 16 + 16), (b, n) => (
                    <span
                      key={n}
                      className={
                        range &&
                        row * 16 + n >= range.start &&
                        row * 16 + n < range.end
                          ? "bg-primary text-primary-foreground"
                          : ""
                      }
                    >
                      {b.toString(16).padStart(2, "0")}{" "}
                    </span>
                  ))}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
      <section className="space-y-3">
        <h3 className="font-semibold">单元还原依据</h3>
        {unit.view.transforms?.map((v, n) => (
          <Facts
            key={n}
            items={[
              ["规则", v.rule],
              [
                "父空间／偏移",
                `${n === 0 ? "原接收字节" : "上一转换输出"} · [${v.range.start}, ${v.range.end})`,
              ],
              ["输出摘要", <Digest value={v.output_sha256} />],
            ]}
          />
        ))}
      </section>
      <DirectoryTable
        label="不可变识别决定历史"
        tableClassName="min-w-[40rem]"
        items={decisions.data ?? []}
        rowId={(v) => v.unit_id}
        loading={decisions.isPending}
        error={decisions.error?.message}
        empty="没有可读识别历史"
        columns={[
          { id: "generation", label: "决定代次", cell: (v) => v.generation },
          {
            id: "method",
            label: "固定识别方法",
            cell: (v) => `${v.method.method.id} · ${v.method.method.version}`,
          },
          {
            id: "state",
            label: "状态",
            cell: (v) => <State value={v.decision_state} />,
          },
          { id: "error", label: "提示", cell: (v) => v.decision_error || "—" },
        ]}
      />
      <section className="space-y-3">
        <h3 className="font-semibold">固定生产解析计划</h3>
        {data.plans?.map((p) => (
          <div className="space-y-2 rounded-lg border p-4" key={p.id}>
            <State value={p.state} />
            <Facts
              items={[
                [
                  "方法",
                  `${p.snapshot.parsing.entry.method_id} · ${p.snapshot.parsing.entry.method_version}`,
                ],
                [
                  "产物摘要",
                  <Digest value={p.snapshot.parsing.entry.artifact_digest} />,
                ],
                [
                  "设备采用／配置",
                  `${p.adoption_id} / ${p.snapshot.parsing.config_revision_id}`,
                ],
                [
                  "精确兼容声明",
                  <Digest value={p.snapshot.compatibility.digest} />,
                ],
                ["重试／fence", `${p.attempts} / ${p.fence}`],
                ["输入摘要", <Digest value={p.snapshot.input.sha256} />],
              ]}
            />
          </div>
        ))}
      </section>
      {data.can_results ? (
        <section className="space-y-4">
          <h3 className="font-semibold">解析结果与字段定位</h3>
          {results.error && <ErrorState message={results.error.message} />}
          {results.data?.map((v) => (
            <div key={v.id} className="space-y-4 rounded-lg border p-4">
              <p className="text-sm">
                {v.response.method.id} · {v.response.method.version} ·{" "}
                {v.status}
              </p>
              <FormNotice>
                业务质量未评定 · 未正式采用。
                {!data.can_raw ? "当前权限不包含原文和原始字段。" : ""}
              </FormNotice>
              <DirectoryTable
                label={`测量结果 ${v.response.method.version}`}
                tableClassName="min-w-[40rem]"
                items={(v.response.records ?? []).flatMap((record) =>
                  (record.measurements ?? []).map((m, n) => ({
                    ...m,
                    id: `${record.index}-${n}`,
                    observed: record.observed_at,
                  })),
                )}
                rowId={(m) => m.id}
                empty="该响应不包含测量值"
                columns={[
                  {
                    id: "key",
                    label: "测量字段",
                    cell: (m) => (
                      <Button
                        variant="link"
                        className="h-auto max-w-full break-all whitespace-normal p-0 text-left"
                        disabled={!data.can_raw}
                        onClick={() => {
                          setRange(m.location.range);
                          if (!raw) content();
                        }}
                      >
                        {m.output_key}
                      </Button>
                    ),
                  },
                  {
                    id: "value",
                    label: "精确数值",
                    cell: (m) => (
                      <code>
                        {m.value ?? "缺失"} {m.unit}
                      </code>
                    ),
                  },
                  {
                    id: "time",
                    label: "观测时间",
                    cell: (m) => <Time value={m.observed} />,
                  },
                  {
                    id: "evidence",
                    label: "单元内位置",
                    cell: (m) =>
                      data.can_raw
                        ? `[${m.location.range.start}, ${m.location.range.end}) · ${m.location.json_pointer ?? ""}`
                        : "无原文权限",
                  },
                ]}
              />
              <details>
                <summary className="cursor-pointer text-sm underline">
                  查看产品解析步骤
                </summary>
                <ul className="mt-2 space-y-1 text-xs">
                  {v.response.steps?.map((s) => (
                    <li key={s.id}>
                      {s.id} · {s.rule_id} · {s.status}
                      {data.can_raw
                        ? ` · [${s.range.start}, ${s.range.end})`
                        : ""}
                    </li>
                  ))}
                </ul>
              </details>
            </div>
          ))}
        </section>
      ) : (
        <FormNotice>当前角色可查看报文元数据，未获准读取解析结果。</FormNotice>
      )}
      <ResponsivePanel
        open={recover}
        onOpenChange={setRecover}
        title="固定单元恢复作业"
        description="创建新的决定和结果，保留所有历史证据。"
      >
        {recover && (
          <RecoveryEditor
            units={[id]}
            device={unit.device_id ?? undefined}
            done={() => setRecover(false)}
          />
        )}
      </ResponsivePanel>
    </div>
  );
}
