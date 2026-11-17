import { useQuery } from "@tanstack/react-query";
import { Permissions as P } from "@infrapro/api-client";
import { DirectoryTable } from "@infrapro/ui/patterns/directory-table";
import { useParsing } from "../parsing/context";
import { State, Time } from "./common";
export function DeviceIngestion({ device }: { device: string }) {
  const c = useParsing();
  const query = useQuery({
    queryKey: [...c.prefix, "device-ingestion-summary", device],
    queryFn: ({ signal }) => c.client.ingestionDevice(c.scope, device, signal),
    enabled: c.scope.kind !== "platform",
  });
  return (
    <section className="space-y-3">
      <h3 className="font-medium">设备接入状态</h3>
      <DirectoryTable
        label="设备获准接入摘要"
        items={query.data ?? []}
        rowId={(v) => v.source_id ?? v.source_project_id}
        loading={query.isPending}
        error={query.error?.message}
        empty="此设备尚未配置接入关联，现有采用和试解析功能可独立使用。"
        columns={[
          {
            id: "state",
            label: "接收状态",
            cell: (v) => <State value={v.state} />,
          },
          {
            id: "receive",
            label: "最近接收",
            cell: (v) => <Time value={v.last_received_at} />,
          },
          {
            id: "parse",
            label: "最近解析成功",
            cell: (v) => <Time value={v.last_parsed_at} />,
          },
        ]}
      />
      {c.scope.kind === "tenant" &&
        [
          P.TenantIngestionMessageRead,
          P.TenantIngestionTerminalRead,
          P.TenantDeviceRead,
        ].every(c.can) && (
          <a
            className="text-sm underline"
            href={`/t/${c.scope.tenantId}/ingestion?device=${encodeURIComponent(device)}`}
          >
            查看设备接入报文与处理证据
          </a>
        )}
    </section>
  );
}
