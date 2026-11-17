import { useState } from "react";
import { Permissions as P } from "@infrapro/api-client";
import { useIdentity } from "@infrapro/api-client/react";
import { PageHeader } from "@infrapro/layouts";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@infrapro/ui/components/tabs";
import { ErrorState } from "@infrapro/ui/patterns/feedback";
import { ParsingBoundary, useParsing } from "../parsing/context";
import { useOptionalTenant } from "../tenancy/context";
import { Sources } from "./sources";
import { Terminals } from "./terminals";
import { Messages, MessageEvidence } from "./messages";
import { Recovery, Unmatched, Operations } from "./recovery";
export type IngestionSearch = {
  source?: string;
  terminal?: string;
  message?: string;
  device?: string;
  recovery?: string;
  generation?:string;
  tab?: string;
};
export function IngestionPage(props: IngestionSearch) {
  const identity = useIdentity(),
    tenant = useOptionalTenant(),
    can = tenant
      ? (p: Parameters<typeof identity.can>[0]) =>
          tenant.permissions.includes(p)
      : identity.can;
  return (
    <ParsingBoundary
      scope={
        tenant
          ? { kind: "tenant", tenantId: tenant.tenant.id }
          : { kind: "platform" }
      }
      can={can}
      available={tenant?.available}
    >
      <Body {...props} />
    </ParsingBoundary>
  );
}
function Body(props: IngestionSearch) {
  const c = useParsing(),
    platform = c.scope.kind === "platform";
  const tabs = [
    ...(c.can(platform ? P.IngestionSourceRead : P.TenantIngestionSourceRead)
      ? [{ id: "sources", name: "接入入口" }]
      : []),
    ...(!platform &&
    c.can(P.TenantIngestionTerminalRead) &&
    c.can(P.TenantDeviceRead)
      ? [{ id: "terminals", name: "终端与关联" }]
      : []),
    ...(!platform &&
    [
      P.TenantIngestionMessageRead,
      P.TenantIngestionTerminalRead,
      P.TenantDeviceRead,
    ].every(c.can)
      ? [{ id: "messages", name: "原始报文" }]
      : []),
    ...(c.can(
      platform ? P.IngestionQuarantineRead : P.TenantIngestionUnmatchedRead,
    )
      ? [{ id: "unmatched", name: platform ? "隔离元数据" : "未匹配报文" }]
      : []),
    ...(c.can(
      platform ? P.IngestionQuarantineRead : P.TenantIngestionSourceRead,
    )
      ? [{ id: "recovery", name: "恢复作业" }]
      : []),
    ...(!platform && c.can(P.TenantIngestionSourceRead)
      ? [{ id: "operations", name: "运行概况" }]
      : []),
  ];
  const [tab, setTab] = useState(
    props.recovery
      ? "recovery"
      : props.terminal
        ? "terminals"
        : props.device
          ? "messages"
          : (props.tab ?? tabs[0]?.id ?? ""),
  );
  if (!tabs.length) return <ErrorState message="没有接入工作区访问权限。" />;
  return (
    <section className="min-w-0 space-y-5">
      <PageHeader
        title={platform ? "接入入口管理" : "设备接入与原始报文"}
        description={
          platform
            ? "维护可信来源和监听修订，查看隔离元数据及传输对账。"
            : "按已确认入口登记终端、关联设备，查看接收事实与固定版本解析证据。"
        }
      />
      {props.message ? (
        <MessageEvidence id={props.message} />
      ) : (
        <Tabs value={tab} onValueChange={(v) => setTab(String(v))}>
          <div className="overflow-x-auto">
            <TabsList aria-label="接入工作区">
              {tabs.map((v) => (
                <TabsTrigger key={v.id} value={v.id}>
                  {v.name}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          <TabsContent value="sources">
            <Sources initial={props.source} />
          </TabsContent>
          <TabsContent value="terminals">
            <Terminals source={props.source} initial={props.terminal} />
          </TabsContent>
          <TabsContent value="messages">
            <Messages source={props.source} device={props.device} />
          </TabsContent>
          <TabsContent value="unmatched">
            <Unmatched />
          </TabsContent>
          <TabsContent value="recovery">
            <Recovery initial={props.recovery} generation={props.generation} />
          </TabsContent>
          <TabsContent value="operations">
            <Operations />
          </TabsContent>
        </Tabs>
      )}
    </section>
  );
}
