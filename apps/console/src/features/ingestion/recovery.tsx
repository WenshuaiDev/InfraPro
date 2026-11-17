import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Permissions as P,
  type IngestionMetadata,
  type IngestionRecovery,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { FormNotice, ErrorState } from "@infrapro/ui/patterns/feedback";
import { DeviceField, DeviceSelect } from "../parsing/fields";
import { useParsing, useParsingWrite, WriteFeedback } from "../parsing/context";
import { Facts, Pages, State, Time } from "./common";
export function Unmatched() {
  const c = useParsing(),
    w = useParsingWrite(),
    [cursor, setCursor] = useState(""),
    [errorCode, setError] = useState(""),
    [selected, setSelected] = useState<IngestionMetadata[]>([]),
    [open, setOpen] = useState(false),
    [closeReason, setCloseReason] = useState("");
  const platform = c.scope.kind === "platform",
    manage = c.can(
      platform
        ? P.IngestionQuarantineManage
        : P.TenantIngestionUnmatchedProcess,
    );
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-unmatched", cursor, errorCode],
    queryFn: ({ signal }) =>
      c.client.ingestionUnmatched(
        c.scope,
        { cursor, error_code: errorCode, limit: 50 },
        signal,
      ),
  });
  const integrity=useQuery({queryKey:[...c.prefix,'ingestion-integrity'],queryFn:({signal})=>c.client.ingestionIntegrity(signal),enabled:platform,refetchInterval:10000});
  return (
    <div className="space-y-4">
      <FormNotice>
        此处只显示最小接收元数据。登记新终端不自动开放历史正文；恢复时固定接收清单，逐单元重新核验归属。
      </FormNotice>
      {platform&&<><DirectoryTable label="包装完整性冲突" items={integrity.data??[]} rowId={v=>v.id} loading={integrity.isPending} error={integrity.error?.message} empty="没有包装冲突" columns={[{id:'receipt',label:'云收据',cell:v=><code className="break-all text-xs">{v.receipt_id}</code>},{id:'code',label:'原因',cell:v=>v.error_code},{id:'count',label:'次数',cell:v=>v.occurrences},{id:'time',label:'最后发现',cell:v=><Time value={v.last_seen_at}/>}]}/>{manage&&<EpochConfirmation/>}</>}
      <DeviceField
        name="unmatched-error"
        label="错误代码筛选"
        value={errorCode}
        onChange={(v) => {
          setError(v);
          setCursor("");
          setSelected([]);
        }}
      />
      <DirectoryTable
        label="未匹配与隔离接收元数据"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="没有待处理接收记录"
        columns={[
          {
            id: "select",
            label: "选择",
            cell: (v) =>
              manage ? (
                <input
                  type="checkbox"
                  aria-label={`选择接收 ${v.receipt_id}`}
                  checked={selected.some((x) => x.id === v.id)}
                  onChange={(e) =>
                    setSelected((x) =>
                      e.target.checked
                        ? [...x, v]
                        : x.filter((a) => a.id !== v.id),
                    )
                  }
                />
              ) : null,
          },
          {
            id: "receipt",
            label: "云端收据",
            cell: (v) => (
              <code className="break-all text-xs">{v.receipt_id}</code>
            ),
          },
          {
            id: "time",
            label: "接收时间",
            cell: (v) => <Time value={v.received_at} />,
          },
          {
            id: "state",
            label: "保管状态",
            cell: (v) => <State value={v.state} />,
          },
          {
            id: "error",
            label: "原因",
            cell: (v) => v.error_code || "单元待关联",
          },
          { id: "size", label: "原文字节", cell: (v) => v.byte_length },
        ]}
        pagination={
          <Pages
            cursor={cursor}
            next={query.data?.next_cursor}
            onChange={setCursor}
          />
        }
      />
      {manage && (
        <div className="space-y-3">
          <p className="text-sm">
            已固定选择 {selected.length} 份接收记录（最多 1000）
          </p>
          <Button
            disabled={
              !selected.length || selected.length > 1000 || !c.available
            }
            onClick={() => setOpen(true)}
          >
            {platform ? "建立传输对账作业" : "建立固定范围补处理"}
          </Button>
          {!platform && (
            <>
              <DeviceField
                name="unmatched-close-reason"
                label="仅记录处置结论的理由"
                value={closeReason}
                onChange={setCloseReason}
              />
              <Button
                variant="outline"
                disabled={
                  !selected.length ||
                  !closeReason.trim() ||
                  w.pending ||
                  !c.available
                }
                onClick={() =>
                  void w.run(async () => {
                    for (const v of selected)
                      await c.client.ingestionCloseUnmatched(
                        c.scope,
                        v.id,
                        w.meta(
                          "unmatched-close",
                          { id: v.id, closeReason },
                          closeReason,
                        ),
                      );
                    setSelected([]);
                  }, "处置已记录；原文及原失败证据保留。")
                }
              >
                记录处置结论
              </Button>
            </>
          )}
          <WriteFeedback value={w} />
        </div>
      )}
      <ResponsivePanel
        open={open}
        onOpenChange={setOpen}
        title={platform ? "传输对账" : "固定范围补处理"}
        description="预览与提交均复验当前权限、来源及设备资格。"
      >
        {open && (
          <RecoveryEditor
            receipts={selected.map((v) => v.id)}
            source={
              selected.every((v) => v.source_id === selected[0]?.source_id)
                ? selected[0]?.source_id
                : undefined
            }
            done={() => {
              setOpen(false);
              setSelected([]);
            }}
          />
        )}
      </ResponsivePanel>
    </div>
  );
}
export function RecoveryEditor({
  receipts = [],
  units = [],
  device,
  source,
  done,
}: {
  receipts?: string[];
  units?: string[];
  device?: string;
  source?: string;
  done: () => void;
}) {
  const c = useParsing(),
    w = useParsingWrite(),
    platform = c.scope.kind === "platform",
    [kind, setKind] = useState(
      platform ? "reconcile" : units.length ? "reparse" : "backfill",
    ),
    [reason, setReason] = useState(""),
    [config, setConfig] = useState(""),
    [identification,setIdentification]=useState(""),
    [terminal, setTerminal] = useState(""),
    [association, setAssociation] = useState(""),
    [prepared, setPrepared] = useState(false),
    [task, setTask] = useState("");
  const configs = useQuery({
      queryKey: [...c.prefix, "ingestion-recovery-configs", device],
      queryFn: ({ signal }) =>
        c.client.parsingDeviceConfigs(c.scope, device!, "", signal),
      enabled: !!device && c.canMethods,
    }),
    terminals = useQuery({
      queryKey: [...c.prefix, "ingestion-recovery-terminals", source],
      queryFn: ({ signal }) =>
        c.client.ingestionTerminals(
          c.scope,
          { source_id: source, limit: 100 },
          signal,
        ),
      enabled: !platform && !!source && c.can(P.TenantIngestionTerminalRead),
    }),
    associations = useQuery({
      queryKey: [...c.prefix, "ingestion-recovery-associations", terminal],
      queryFn: ({ signal }) =>
        c.client.ingestionAssociations(c.scope, terminal, signal),
      enabled: !!terminal,
    });
  const methods=useQuery({queryKey:[...c.prefix,'recovery-identifications'],queryFn:({signal})=>c.client.ingestionCapabilities(c.scope,'identification-methods',{limit:100},signal),enabled:!platform&&c.canMethods});
  const selectedMethod=methods.data?.items.find(v=>v.id===identification)?.entry;
  const input = () => ({
    ...w.meta(
      "recovery",
      { receipts, units, kind, config, association, identification, reason },
      reason,
    ),
    kind,
    ...(kind==="reidentify"&&selectedMethod&&"method" in selectedMethod?{identification:selectedMethod.method}:{}),
    receipt_ids: receipts,
    unit_ids: units,
    config_revision_id: config,
    association_id: association,
  });
  return (
    <div className="space-y-4">
      <Facts
        items={[
          [
            "固定范围",
            `${receipts.length || units.length} ${units.length ? "个核验单元" : "份接收记录"}`,
          ],
          [
            "范围身份",
            <ul className="max-h-32 overflow-auto font-mono text-xs">
              {[...receipts, ...units].map((v) => (
                <li key={v}>{v}</li>
              ))}
            </ul>,
          ],
        ]}
      />
      <DeviceSelect
        name="recovery-kind"
        label="处理方式"
        value={kind}
        onChange={(v) => {
          setKind(v);
          setPrepared(false);
        }}
        options={
          platform
            ? [{ value: "reconcile", label: "校验原始来源修订并传输对账" }]
            : [
                {
                  value: units.length ? "reparse" : "backfill",
                  label: units.length ? "重新解析" : "补处理明确历史范围",
                },
                { value: "reidentify", label: "重新建立识别决定" },
              ]
        }
      />
      {kind==='reidentify'&&!platform&&<DeviceSelect name="recovery-identification" label="来源识别固定版本" value={identification} onChange={v=>{setIdentification(v);setPrepared(false)}} options={[{value:'',label:'保留原入口修订的识别版本'},...(methods.data?.items??[]).flatMap(v=>'purpose' in v.entry&&v.entry.purpose==='source_identify'&&v.available&&v.state!=='blocked'?[{value:v.id,label:`${v.entry.name} · ${v.entry.method.version}`}]:[])]}/>}
      {device && (
        <DeviceSelect
          name="recovery-config"
          label="测量配置依据"
          value={config}
          onChange={(v) => {
            setConfig(v);
            setPrepared(false);
          }}
          options={[
            { value: "", label: "使用当前设备实际采用" },
            ...(configs.data?.items ?? []).map((v) => ({
              value: v.id,
              label: `${v.name} · ${v.snapshot.method.version} · 修订 ${v.number}`,
            })),
          ]}
        />
      )}
      {source && !platform && (
        <>
          <DeviceSelect
            name="recovery-terminal"
            label="历史补处理关联（可选）"
            value={terminal}
            onChange={(v) => {
              setTerminal(v);
              setAssociation("");
              setPrepared(false);
            }}
            options={[
              { value: "", label: "保留原接收区间匹配规则" },
              ...(terminals.data?.items ?? []).map((v) => ({
                value: v.id,
                label: `${v.name} · ${v.terminal_key}`,
              })),
            ]}
          />
          {terminal && (
            <DeviceSelect
              name="recovery-association"
              label="已登记的准确设备关联"
              value={association}
              onChange={(v) => {
                setAssociation(v);
                setPrepared(false);
              }}
              options={[
                { value: "", label: "请选择已核验关联" },
                ...(associations.data ?? [])
                  .filter((v) => v.enabled)
                  .map((v) => ({
                    value: v.id,
                    label: `${v.child_key || "整路"} · ${v.device_id} · 序号 ${v.start_sequence}`,
                  })),
              ]}
            />
          )}
        </>
      )}
      <DeviceField
        name="recovery-reason"
        label="人工处理理由"
        value={reason}
        onChange={(v) => {
          setReason(v);
          setPrepared(false);
        }}
      />
      <FormNotice>
        此作业创建真实任务和新处理代次。取消、重试及执行提交均重新核验权限；原决定和结果保持不变。
      </FormNotice>
      <div className="flex gap-2">
        <Button
          variant="outline"
          disabled={!reason.trim() || w.pending || !c.available}
          onClick={() =>
            void w.run(async () => {
              await c.client.ingestionRecover(c.scope, input(), true);
              setPrepared(true);
            }, "固定清单与当前资格已预览。")
          }
        >
          预览固定清单
        </Button>
        {prepared && (
          <Button
            disabled={w.pending || !c.available}
            onClick={() =>
              void w.run(async () => {
                const result = await c.client.ingestionRecover(
                  c.scope,
                  input(),
                );
                setTask(result.task_ref?.task_id ?? "");
                setPrepared(false);
              }, "恢复作业已提交，可在任务中心查看和控制。")
            }
          >
            提交恢复作业
          </Button>
        )}
      </div>
      <WriteFeedback value={w} />
      {task && (
        <a className="text-sm underline" href={`/tasks/${task}`}>
          打开恢复任务
        </a>
      )}
      <Button variant="ghost" onClick={done}>
        关闭
      </Button>
    </div>
  );
}
export function Recovery({ initial,generation }: { initial?: string;generation?:string }) {
  const c = useParsing(),
    [selected, setSelected] = useState(initial);
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-recovery"],
    queryFn: ({ signal }) => c.client.ingestionRecoveries(c.scope, signal),
    refetchInterval: 10000,
  });
  const version=useQuery({queryKey:[...c.prefix,"ingestion-recovery-generation",selected,generation],queryFn:({signal})=>c.client.ingestionRecoveryGeneration(c.scope,selected!,generation!,signal),enabled:!!selected&&!!generation});
  const item = generation?version.data:query.data?.find((v) => v.id === selected);
  return (
    <div className="space-y-4">
      <DirectoryTable
        label="人工恢复作业"
        items={query.error ? [] : (query.data ?? [])}
        rowId={(v) => v.id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="没有当前可读的恢复作业"
        columns={[
          {
            id: "kind",
            label: "恢复方式",
            cell: (v) => (
              <Button
                variant="link"
                className="h-auto p-0"
                onClick={() => setSelected(v.id)}
              >
                {(
                  {
                    backfill: "历史补处理",
                    reidentify: "重新识别",
                    reparse: "重新解析",
                    reconcile: "传输对账",
                    confirm_epoch: "恢复时间线确认",
                  } as Record<string, string>
                )[v.kind] ?? v.kind}
              </Button>
            ),
          },
          {
            id: "state",
            label: "状态",
            cell: (v) => <State value={v.state} />,
          },
          { id: "generation", label: "代次", cell: (v) => v.generation },
          {
            id: "count",
            label: "固定范围",
            cell: (v) =>
              `${v.input.receipt_ids?.length || v.input.unit_ids?.length || 0} 条`,
          },
          { id: "reason", label: "人工理由", cell: (v) => v.reason },
        ]}
      />
      <ResponsivePanel
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected(undefined);
        }}
        title="恢复作业明细"
        description="历史结果与当前执行资格分开保存。"
      >
        {item ? (
          <RecoveryDetail item={item} />
        ) : (
          <ErrorState
            message={
              query.isPending
                ? "正在核验作业范围…"
                : "作业不存在或当前不可访问。"
            }
          />
        )}
      </ResponsivePanel>
    </div>
  );
}
function RecoveryDetail({ item }: { item: IngestionRecovery }) {
  return (
    <div className="space-y-4">
      <Facts
        items={[
          ["作业 ID", <code>{item.id}</code>],
          ["状态", <State value={item.state} />],
          ["代次／修订", `${item.generation} / ${item.revision}`],
          ["理由", item.reason],
        ]}
      />
      <DirectoryTable
        label="固定清单逐项结果"
        items={item.items ?? []}
        rowId={(v) => v.receipt_id}
        empty="作业尚未开始"
        columns={[
          {
            id: "receipt",
            label: "接收记录",
            cell: (v) => (
              <code className="break-all text-xs">{v.receipt_id}</code>
            ),
          },
          {
            id: "state",
            label: "结果",
            cell: (v) => <State value={v.state} />,
          },
          { id: "error", label: "原因", cell: (v) => v.error_code || "—" },
        ]}
      />
      <a className="text-sm underline" href="/tasks">
        到任务中心查看取消、重试及执行历史
      </a>
    </div>
  );
}
export function Operations() {
  const c = useParsing(),
    w = useParsingWrite(),
    [reason, setReason] = useState("");
  const query = useQuery({
    queryKey: [...c.prefix, "ingestion-operations"],
    queryFn: ({ signal }) => c.client.ingestionOperations(c.scope, {}, signal),
    refetchInterval: 10000,
  });
  return (
    <div className="space-y-4">
      <FormNotice>
        连接、接收、业务测量和解析成功分别观测。云端信息过期时显示未知，不能据此断言所有设备离线。
      </FormNotice>
      <DirectoryTable
        label="接入运行状态"
        items={query.error ? [] : (query.data?.items ?? [])}
        rowId={(v) => v.source_id}
        loading={query.isPending}
        error={query.error?.message}
        retry={() => void query.refetch()}
        empty="尚无接入运行记录"
        columns={[
          {
            id: "source",
            label: "入口接收／解析",
            cell: (v) => (
              <div className="space-y-1">
                <State value={v.source_state} />
                <State value={v.planning_paused ? "paused" : "enabled"} />
              </div>
            ),
          },
          {
            id: "connection",
            label: "连接观测",
            cell: (v) => (
              <div>
                <State value={v.connection_state} />
                <small className="block">
                  <Time value={v.observed_at} />
                </small>
              </div>
            ),
          },
          {
            id: "receive",
            label: "最近接收",
            cell: (v) => <Time value={v.last_received_at} />,
          },
          {
            id: "measurement",
            label: "最近业务测量",
            cell: (v) => <Time value={v.last_measurement_at} />,
          },
          {
            id: "parse",
            label: "最近解析成功",
            cell: (v) => <Time value={v.last_parsed_at} />,
          },
          {
            id: "queue",
            label: "待处理／未匹配",
            cell: (v) => `${v.queued} / ${v.unmatched}`,
          },
        ]}
      />
      {!!query.data?.changes.length && (
        <>
          <DeviceField
            name="change-cancel-reason"
            label="取消待应用关联变更的理由"
            value={reason}
            onChange={setReason}
          />
          <DirectoryTable
            label="关联水位命令"
            items={query.data.changes}
            rowId={(v) => v.id}
            empty="无关联命令"
            columns={[
              {
                id: "id",
                label: "命令",
                cell: (v) => <code className="text-xs">{v.id}</code>,
              },
              {
                id: "state",
                label: "状态",
                cell: (v) => <State value={v.state} />,
              },
              { id: "error", label: "原因", cell: (v) => v.error_code || "—" },
              {
                id: "cancel",
                label: "操作",
                cell: (v) =>
                  v.state === "pending" &&
                  c.can(P.TenantIngestionTerminalManage) ? (
                    <Button
                      variant="outline"
                      disabled={!reason.trim() || w.pending}
                      onClick={() =>
                        void w.run(() =>
                          c.client.ingestionCancelChange(
                            c.scope,
                            v.id,
                            w.meta(
                              "cancel-change",
                              { id: v.id, reason },
                              reason,
                            ),
                          ),
                        )
                      }
                    >
                      取消待应用变更
                    </Button>
                  ) : null,
              },
            ]}
          />
          <WriteFeedback value={w} />
        </>
      )}
    </div>
  );
}

function EpochConfirmation(){const c=useParsing(),w=useParsingWrite(),[relay,setRelay]=useState(''),[epoch,setEpoch]=useState(''),[reason,setReason]=useState('');return <details className="rounded-lg border p-4"><summary className="cursor-pointer text-sm">恢复后确认云缓冲时间线</summary><div className="mt-4 space-y-3"><FormNotice>先完成云、内网和对象存储对账，再确认准确的新 epoch；旧关联不会自动用于新时间线。</FormNotice><DeviceField name="epoch-relay" label="云节点 ID" value={relay} onChange={setRelay}/><DeviceField name="epoch-id" label="新 epoch" value={epoch} onChange={setEpoch}/><DeviceField name="epoch-reason" label="对账依据与理由" value={reason} onChange={setReason}/><WriteFeedback value={w}/><Button disabled={!relay||!epoch||!reason.trim()||w.pending} onClick={()=>void w.run(()=>c.client.ingestionRecover(c.scope,{...w.meta('epoch',{relay,epoch,reason},reason),kind:'confirm_epoch',relay_id:relay,epoch,receipt_ids:[],unit_ids:[]}))}>记录时间线确认任务</Button></div></details>}
