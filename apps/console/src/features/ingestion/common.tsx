import type { ReactNode } from "react";
import { Badge } from "@infrapro/ui/components/badge";
const labels: Record<string, string> = {
  active: "已生效",
  enabled: "已启用",
  pending: "待启用",
  publishing: "等待云端应用",
  awaiting_tenant_confirmation: "待租户确认",
  paused: "已暂停",
  blocked: "已阻断",
  failed: "失败",
  stored: "原文已保存",
  identified: "已识别",
  awaiting_identity: "待识别",
  awaiting_association: "待关联",
  parser_unconfigured: "待配置解析",
  quarantined: "隔离保管",
  queued: "已排队",
  parsing: "解析中",
  parsed: "已解析",
  control: "控制包",
  envelope: "网关封装",
  retry_wait: "等待重试",
  registered: "已登记",
  deprecated: "停止新采用",
  unknown: "状态未知",
  observed: "状态可观测",
  acknowledged: "云端已确认",
  pending_ack: "待确认",
  running: "处理中",
  succeeded: "已完成",
  cancelled: "已取消",
  applied: "已应用",
  awaiting_redelivery: "等待重新投递",
};
export function State({ value }: { value?: string | null }) {
  return (
    <Badge
      variant={
        value === "failed" || value === "blocked" || value === "quarantined"
          ? "destructive"
          : "outline"
      }
    >
      {value ? (labels[value] ?? value) : "尚无记录"}
    </Badge>
  );
}
export function Time({ value }: { value?: string | null }) {
  return (
    <span className="tabular-nums">
      {value ? new Date(value).toLocaleString() : "尚无记录"}
    </span>
  );
}
export function Digest({ value }: { value?: string | null }) {
  return (
    <code className="break-all text-xs" title={value ?? ""}>
      {value || "—"}
    </code>
  );
}

export function Facts({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="grid gap-3 text-sm">
      {items.map(([name, value]) => (
        <div key={name} className="grid min-w-0 gap-1 sm:grid-cols-[8rem_1fr]">
          <dt className="text-muted-foreground">{name}</dt>
          <dd className="min-w-0 break-words">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
