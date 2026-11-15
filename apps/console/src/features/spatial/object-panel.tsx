import { lazy, Suspense, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Permissions as P,
  type SpatialObject,
  type SpatialObjectRef,
  type SpatialElement,
  type SpatialSummaries,
  type MonitoringRange,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { useSpatial } from "./context";
import { SpatialLink, Failure, Info, TextField } from "./common";
import { inputClass, kinds } from "./state";
const Trend = lazy(() => import("../monitoring/trend-chart"));
export function ObjectPanel({
  owner,
  element,
  version,
  onLocate,
  mainChannel,
  onMainChannel,
}: {
  owner?: SpatialObjectRef;
  element?: SpatialElement;
  version?: string;
  onLocate: () => void;
  mainChannel?: string;
  onMainChannel?: (id: string) => void;
}) {
  const m = useSpatial(),
    [tab, setTab] = useState<"monitoring" | "artifacts" | "source">(
      "monitoring",
    );
  const object = useQuery({
    queryKey: [...m.prefix, "object", owner],
    queryFn: ({ signal }) => m.client.spatialObject(m.scope, owner!, signal),
    enabled: m.available && !!owner,
    retry: false,
  });
  return (
    <aside
      className="min-w-0 space-y-4 rounded-xl border bg-card p-4"
      aria-label="空间对象详情"
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-lg font-semibold">
            {object.data?.name ?? (element?.name || "对象详情")}
          </h2>
          {object.data && (
            <p className="text-xs text-muted-foreground">
              {kinds[object.data.kind]} · {object.data.code}
              {object.data.retired ? " · 已退役" : ""}
            </p>
          )}
        </div>
        {object.data && (
          <Button size="sm" variant="outline" onClick={onLocate}>
            空间位置
          </Button>
        )}
      </div>
      <Failure error={object.error} />
      {!owner && !element && (
        <Info>
          从资源目录、图纸或模型中选择对象，查看当前身份可读的监测与资料。
        </Info>
      )}
      {owner && object.isPending && <Info>正在核验对象权限…</Info>}
      {object.data && (
        <>
          <p className="text-xs text-muted-foreground">
            {object.data.position_notice || "工程位置与空间挂载分别维护。"}
          </p>
          <SpatialLink
            className="text-sm underline"
            to={`${m.base}/resources/facilities/${object.data.facility_id}`}
            search={{
              component:
                object.data.kind === "component" ? object.data.id : undefined,
              point: object.data.kind === "point" ? object.data.id : undefined,
            }}
          >
            打开资源详情
          </SpatialLink>
          <nav
            className="flex flex-wrap gap-1 border-b pb-2"
            aria-label="对象详情内容"
          >
            {[
              ["monitoring", "监测数据"],
              ["source", "设备来源"],
              ["artifacts", "分析成果"],
            ].map(([key, label]) => (
              <Button
                key={key}
                size="sm"
                variant={tab === key ? "secondary" : "ghost"}
                onClick={() => setTab(key as typeof tab)}
              >
                {label}
              </Button>
            ))}
          </nav>
          {tab === "monitoring" ? (
            <OwnerMonitoring
              key={`${owner!.kind}:${owner!.id}`}
              object={object.data}
              mainChannel={mainChannel}
              onMainChannel={onMainChannel}
            />
          ) : tab === "source" ? (
            <OwnerSources key={owner!.id} object={object.data} />
          ) : (
            <RelatedArtifacts key={owner!.id} owner={owner!} />
          )}
        </>
      )}
      {element && (
        <section className="space-y-3 border-t pt-3">
          <h3 className="font-medium">源构件信息</h3>
          <dl className="grid grid-cols-[4rem_1fr] gap-2 text-sm">
            <dt>类型</dt>
            <dd className="break-all">{element.type}</dd>
            <dt>标识</dt>
            <dd className="break-all">{element.key}</dd>
            {element.type_key && (
              <>
                <dt>类型标识</dt>
                <dd className="break-all">{element.type_key}</dd>
              </>
            )}
          </dl>
          {!owner && (
            <Info>
              此源构件未关联当前可见的业务构件。源属性本身不赋予业务或监测数据权限。
            </Info>
          )}
          <details>
            <summary className="min-h-11 cursor-pointer py-3 text-sm">
              源属性与类型属性
            </summary>
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all text-xs">
              {JSON.stringify(
                { instance: element.properties, type: element.type_properties },
                null,
                2,
              )}
            </pre>
          </details>
        </section>
      )}
      {version && (
        <details className="border-t pt-3 text-xs text-muted-foreground"><summary className="cursor-pointer py-1">固定版本标识</summary><p className="break-all py-2">{version}</p></details>
      )}
    </aside>
  );
}
type Summary = SpatialSummaries["items"][number]["channels"][number];
function OwnerMonitoring({
  object,
  mainChannel,
  onMainChannel,
}: {
  object: SpatialObject;
  mainChannel?: string;
  onMainChannel?: (id: string) => void;
}) {
  const m = useSpatial(),
    [selected, setSelected] = useState(""),
    [refresh, setRefresh] = useState(0);
  const q = useQuery({
    queryKey: [
      ...m.prefix,
      "summary",
      object.kind,
      object.id,
      refresh,
      mainChannel,
    ],
    queryFn: ({ signal }) =>
      m.client.spatialSummaries(
        m.scope,
        [{ kind: object.kind, id: object.id }],
        signal,
        mainChannel
          ? { [`${object.kind}:${object.id}`]: mainChannel }
          : undefined,
      ),
    enabled: m.available,
    retry: false,
    refetchInterval: 30000,
  });
  const channels = q.data?.items[0]?.channels ?? [],
    current =
      channels.find((c) => c.channel.id === (selected || mainChannel)) ??
      channels[0];
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="font-medium">直接所属通道</h3>
        <Button
          variant="ghost"
          size="sm"
          disabled={q.isFetching}
          onClick={() => setRefresh((n) => n + 1)}
        >
          刷新
        </Button>
      </div>
      <Failure error={q.error} />
      {q.isPending ? (
        <Info>正在读取监测摘要…</Info>
      ) : !channels.length && !q.error ? (
        <Info>
          没有当前可读的直接所属通道。构件与设施不会自动汇总下属测点。
        </Info>
      ) : null}
      {channels.length > 0 && (
        <label className="grid gap-1 text-sm">
          主通道
          <select
            aria-label="主通道"
            className={inputClass}
            value={current?.channel.id ?? ""}
            onChange={(e) => {
              setSelected(e.target.value);
              onMainChannel?.(e.target.value);
            }}
          >
            {channels.map((c) => (
              <option key={c.channel.id} value={c.channel.id}>
                {c.channel.name} ·{" "}
                {c.channel.source_kind === "derived" ? "派生" : "直接"} ·{" "}
                {c.channel.unit}
              </option>
            ))}
          </select>
        </label>
      )}
      {current && (
        <ChannelSummary
          key={`${current.channel.id}:${refresh}`}
          value={current}
        />
      )}
      <p className="text-xs text-muted-foreground">
        {q.data?.refreshed_at
          ? `刷新于 ${new Date(q.data.refreshed_at).toLocaleTimeString()}。 `
          : ""}
        {q.data?.snapshot_note}
      </p>
      {q.data?.items[0]?.next_cursor && (
        <SpatialLink
          to={`${m.base}/monitoring`}
          search={{ point: object.kind === "point" ? object.id : undefined }}
          className="text-sm underline"
        >
          更多直接所属通道：进入监测工作台
        </SpatialLink>
      )}
    </section>
  );
}
function ChannelSummary({ value }: { value: Summary }) {
  const m = useSpatial(),
    c = value.channel,
    stat = value.latest.channels?.[0],
    derived = c.source_kind === "derived",
    row = derived ? stat?.latest_output : stat?.last,
    [range, setRange] = useState<MonitoringRange>(() => ({
      from: new Date(Date.now() - 24 * 3600000).toISOString(),
      to: new Date().toISOString(),
    })),
    [showTrend, setShowTrend] = useState(false);
  const trend = useQuery({
    queryKey: [...m.prefix, "trend", c.id, value.selection.id, range],
    queryFn: ({ signal }) =>
      m.client.monitoringQuery(
        m.scope,
        "trends",
        {
          selection_id: value.selection.id,
          range,
          timezone: "Asia/Shanghai",
          status: "active",
          limit: 1000,
        },
        signal,
      ),
    enabled: showTrend && m.available,
    retry: false,
  });
  const datum = row?.value ?? "—",
    time = row && ("output_at" in row ? row.output_at : row.observed_at),
    quality = row && ("data_quality" in row ? row.data_quality : row.quality);
  return (
    <div className="space-y-3">
      <div className="rounded-lg bg-muted/40 p-4">
        <p className="text-xs text-muted-foreground">
          {derived ? "当前输出" : "最新观测"} · {c.metric_name}
        </p>
        <p className={`mt-2 break-all font-mono ${String(datum).length > 18 ? "text-lg" : "text-2xl"} font-semibold tabular-nums`}>
          {datum}
          <span className="ml-2 font-sans text-sm font-normal">{c.unit}</span>
        </p>
        <p className="mt-2 text-xs">
          {time ? `时间 ${time}` : "当前没有可见数据"} · 质量{" "}
          {quality ?? "无数据"}
        </p>
        {value.latest.state !== "ready" && (
          <p className="text-xs">查询状态：{value.latest.state}</p>
        )}
        {derived && (
          <div className="mt-3 space-y-1 text-xs">
            <p>
              控制状态：{stat?.control_state ?? "未知"} · 健康：
              {stat?.health ?? "未知"}
            </p>
            <p>
              计算状态：{stat?.latest_output?.calculation_status ?? "尚无输出"}{" "}
              · 新鲜度：{stat?.latest_output?.freshness ?? "无输出"}
            </p>
            <p>
              上次有效结果：{stat?.last_valid_output?.value ?? "—"}（
              {stat?.last_valid_output?.output_at ?? "暂无"}）
            </p>
            <p>计算推进至：{stat?.computed_through ?? "尚未计算"}</p>
            <p>输入推进至：{stat?.input_through ?? "暂无输入"}</p>
            <p>待更新区间：{stat?.pending_ranges?.length ?? 0}</p>
            {!!stat?.latest_output?.reason_flags?.length && (
              <p>原因：{stat.latest_output.reason_flags.join("、")}</p>
            )}
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {c.observation_direction || "未填写观测方向"} ·{" "}
        {c.reference_basis || "未填写参考基准"}。数值保留服务端原始精度。
      </p>
      <div className="flex flex-wrap gap-3 text-sm">
        <SpatialLink
          className="underline"
          to={`${m.base}/monitoring`}
          search={{ point: c.point_id || undefined, channel: c.id }}
        >
          进入监测数据
        </SpatialLink>
        <SpatialLink
          className="underline"
          to={`${m.base}/monitoring/analysis`}
          search={{
            id: "new",
            view: "configs",
            owner: c.owner_id,
            owner_kind: c.owner_kind,
            facility: c.facility_id,
            channel: c.id,
            from: range.from,
            to: range.to,
          }}
        >
          新建历史分析
        </SpatialLink>
        {derived && (
          <SpatialLink
            className="underline"
            to={`${m.base}/monitoring/derived/${c.id}`}
          >
            派生配置与运行
          </SpatialLink>
        )}
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={() => setShowTrend(!showTrend)}
      >
        {showTrend ? "收起趋势" : "查看近期趋势"}
      </Button>
      {showTrend && (
        <>
          <div className="grid gap-2">
            <TextField
              label="开始时间（带时区）"
              value={range.from ?? ""}
              onChange={(from) => setRange({ ...range, from })}
            />
            <TextField
              label="结束时间（带时区）"
              value={range.to ?? ""}
              onChange={(to) => setRange({ ...range, to })}
            />
          </div>
          <Failure error={trend.error} />
          {trend.isFetching && <Info>正在查询所选时段…</Info>}
          {trend.data?.trends && (
            <Suspense fallback={<Info>正在加载趋势图…</Info>}>
              <Trend
                channels={[c]}
                series={trend.data.trends}
                range={range}
                timezone="Asia/Shanghai"
                onWindow={setRange}
                onWidth={() => {}}
              />
            </Suspense>
          )}
        </>
      )}
    </div>
  );
}
function RelatedArtifacts({ owner }: { owner: SpatialObjectRef }) {
  const m = useSpatial(),
    [descendants, setDescendants] = useState(false),
    [cursor, setCursor] = useState(""),
    [from, setFrom] = useState(""),
    [to, setTo] = useState("");
  const q = useQuery({
    queryKey: [...m.prefix, "related", owner, descendants, cursor, from, to],
    queryFn: ({ signal }) =>
      m.client.spatialRelated(
        m.scope,
        {
          owner,
          descendants,
          cursor,
          from: from || undefined,
          to: to || undefined,
          limit: 20,
        },
        signal,
      ),
    enabled: m.available,
    retry: false,
  });
  return (
    <section className="space-y-3">
      <h3 className="font-medium">使用此对象通道的固定成果</h3>
      <p className="text-xs text-muted-foreground">
        按成果保存的实际输入关联，当前项目只读取本项目的成果；输入权限会重新核验。
      </p>
      {owner.kind !== "point" && (
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={descendants}
            onChange={(e) => {
              setDescendants(e.target.checked);
              setCursor("");
            }}
          />
          明确包含下级对象
        </label>
      )}
      <details>
        <summary className="min-h-11 cursor-pointer py-3 text-sm">
          按输出时间筛选
        </summary>
        <TextField
          label="开始时间（带时区）"
          value={from}
          onChange={(v) => {
            setFrom(v);
            setCursor("");
          }}
        />
        <TextField
          label="结束时间（带时区）"
          value={to}
          onChange={(v) => {
            setTo(v);
            setCursor("");
          }}
        />
      </details>
      <Failure error={q.error} />
      {q.isPending ? (
        <Info>正在读取关联成果…</Info>
      ) : !q.data?.items.length && !q.error ? (
        <Info>暂无当前可读的关联成果。</Info>
      ) : null}
      {q.data?.items.map((a) => (
        <div
          key={a.version.id}
          className="space-y-2 rounded border p-3 text-sm"
        >
          <SpatialLink
            className="underline"
            to={`${m.base}/monitoring/analysis/${a.artifact.id}`}
            search={{ view: "artifacts", version: a.version.id }}
          >
            {a.artifact.name} · V{a.version.version_no}
          </SpatialLink>
          <p>
            {a.version.input_state === "current"
              ? "依据未变化"
              : a.version.input_state === "affected"
                ? "依据已变化，固定结果保留"
                : "原依据包含撤销"}
          </p>
          <p className="text-xs text-muted-foreground">
            生成于 {new Date(a.version.created_at).toLocaleString()}
          </p>
        </div>
      ))}
      <div className="flex gap-2">
        {cursor && (
          <Button variant="ghost" onClick={() => setCursor("")}>
            首页
          </Button>
        )}
        {q.data?.next_cursor && (
          <Button
            variant="outline"
            onClick={() => setCursor(q.data!.next_cursor)}
          >
            更多成果
          </Button>
        )}
      </div>
    </section>
  );
}
function OwnerSources({ object }: { object: SpatialObject }) {
  const m = useSpatial(),
    [channel, setChannel] = useState(""),
    [history, setHistory] = useState(false),
    [cursor, setCursor] = useState("");
  const list = useQuery({
    queryKey: [...m.prefix, "source-channels", object.id],
    queryFn: ({ signal }) =>
      m.client.monitoringDirectory(
        m.scope,
        {
          owner_kind: object.kind as "facility" | "component" | "point",
          owner_id: object.id,
          source_kind: "direct",
        },
        signal,
      ),
    enabled: m.available,
    retry: false,
  });
  const selected =
      list.data?.items.find((c) => c.id === channel) ?? list.data?.items[0],
    target = selected?.point_id
      ? {
          facility_id: selected.facility_id,
          point_id: selected.point_id,
          channel_id: selected.id,
        }
      : undefined;
  const q = useQuery({
      queryKey: [...m.prefix, "source", target],
      queryFn: ({ signal }) => m.client.deviceSource(m.scope, target!, signal),
      enabled: !!target && m.available,
      retry: false,
    }),
    past = useQuery({
      queryKey: [...m.prefix, "source-history", target, cursor],
      queryFn: ({ signal }) =>
        m.client.deviceSourceHistory(
          m.scope,
          target!,
          { cursor, limit: 10 },
          signal,
        ),
      enabled: history && !!target && m.available,
      retry: false,
    });
  const source = q.data?.source;
  return (
    <section className="space-y-3">
      <h3 className="font-medium">当前设备来源</h3>
      <Info>
        此处展示当前配置。历史观测的来源按观测时间及当时快照解释，不能用当前设备替代。
      </Info>
      <select
        aria-label="选择直接通道来源"
        className={inputClass}
        value={selected?.id ?? ""}
        onChange={(e) => {
          setChannel(e.target.value);
          setCursor("");
        }}
      >
        <option value="">选择通道</option>
        {list.data?.items.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </select>
      <Failure error={list.error ?? q.error} />
      {selected && !selected.point_id && (
        <Info>
          此通道直接属于设施或构件，尚无测点设备来源配置。观测依据请从监测数据查看。
        </Info>
      )}
      {source && !source.configured ? (
        <p className="text-sm">未配置来源</p>
      ) : source?.visibility === "restricted" ? (
        <p className="text-sm">已配置来源，详情受限</p>
      ) : source?.configured && source.visibility === "full" ? (
        <div className="space-y-2 text-sm">
          <p>
            {source.output?.device.name} · {source.output?.name}
          </p>
          <p>开始于 {source.started_at}</p>
          {source.output &&
            m.can(
              m.scope.kind === "tenant"
                ? P.TenantDeviceRead
                : P.ProjectDeviceRead,
            ) && (
              <SpatialLink
                className="underline"
                to={`${m.base}/devices/${source.output.device.id}`}
              >
                打开设备详情
              </SpatialLink>
            )}
        </div>
      ) : null}
      <Button
        variant="outline"
        size="sm"
        disabled={!selected}
        onClick={() => setHistory(!history)}
      >
        查看来源历史
      </Button>
      {history && (
        <>
          <Failure error={past.error} />
          {past.data?.items.map((s, i) => (
            <div key={i} className="rounded border p-3 text-xs">
              {s.visibility === "restricted" ? (
                "历史来源详情受限"
              ) : s.configured ? (
                <>
                  <p>
                    {s.output?.device.name} · {s.output?.name}
                  </p>
                  <p>
                    {s.started_at} — {s.ended_at ?? "当前"}
                  </p>
                  <details>
                    <summary className="min-h-11 cursor-pointer py-3">
                      当时的解释快照
                    </summary>
                    <pre className="max-h-48 overflow-auto whitespace-pre-wrap">
                      {JSON.stringify(
                        {
                          output: s.output_snapshot,
                          channel: s.channel_snapshot,
                        },
                        null,
                        2,
                      )}
                    </pre>
                  </details>
                </>
              ) : (
                "未配置来源"
              )}
            </div>
          ))}
          {past.data?.next_cursor && (
            <Button
              variant="outline"
              onClick={() => setCursor(past.data!.next_cursor)}
            >
              更早记录
            </Button>
          )}
        </>
      )}
    </section>
  );
}
export function HoverSummary({
  owner,
  mainChannel,
}: {
  owner: SpatialObjectRef;
  mainChannel?: string;
}) {
  const m = useSpatial();
  const q = useQuery({
    queryKey: [...m.prefix, "hover", owner, mainChannel],
    queryFn: ({ signal }) =>
      m.client.spatialSummaries(
        m.scope,
        [owner],
        signal,
        mainChannel
          ? { [`${owner.kind}:${owner.id}`]: mainChannel }
          : undefined,
      ),
    enabled: m.available,
    retry: false,
    staleTime: 10000,
    gcTime: 0,
  });
  const first =
      q.data?.items[0]?.channels.find((c) => c.channel.id === mainChannel) ??
      q.data?.items[0]?.channels[0],
    stat = first?.latest.channels?.[0],
    derived = first?.channel.source_kind === "derived",
    row = derived ? stat?.latest_output : stat?.last,
    time = row && ("output_at" in row ? row.output_at : row.observed_at),
    quality = row && ("data_quality" in row ? row.data_quality : row.quality);
  return (
    <div
      role="status"
      className="pointer-events-none absolute bottom-16 left-4 z-10 max-w-72 rounded-lg border bg-background/95 p-3 text-sm shadow-sm"
    >
      <p className="font-medium">{first?.channel.owner_name ?? "所选对象"}</p>
      <p>
        {q.isPending
          ? "正在读取摘要…"
          : q.error
            ? "摘要暂不可用"
            : first
              ? `${first.channel.name}：${(first.channel.source_kind === "derived" ? stat?.latest_output?.value : stat?.last?.value) ?? "暂无数据"} ${first.channel.unit}`
              : "没有当前可读的直接所属通道"}
      </p>
      {first && (
        <div className="mt-1 space-y-1 text-xs text-muted-foreground">
          <p>{time ? `时间 ${time}` : "当前没有可见数据"} · 质量 {quality ?? "无数据"}</p>
          <p>查询状态：{first.latest.state}</p>
          {derived && <p>计算：{stat?.latest_output?.calculation_status ?? "尚无输出"} · 新鲜度：{stat?.latest_output?.freshness ?? "无输出"} · 待更新区间：{stat?.pending_ranges?.length ?? 0}</p>}
        </div>
      )}
      <p className="mt-1 text-xs text-muted-foreground">点击固定到详情面板</p>
    </div>
  );
}
