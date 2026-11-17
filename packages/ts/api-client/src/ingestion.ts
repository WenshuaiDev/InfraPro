import type { components } from "./schema";
import type { ParsingScope } from "./parsing";
import { parsingPrefix } from "./parsing";
import type { MonitoringRequestOptions } from "./monitoring";
type S<N extends keyof components["schemas"]> = components["schemas"][N];
export type IngestionSource = S<"IngestionSource">;
export type IngestionSourceDetail = S<"IngestionSourceDetail">;
export type IngestionSourceInput = S<"IngestionSourceInput">;
export type IngestionTerminal = S<"IngestionTerminal">;
export type IngestionAssociation = S<"IngestionAssociation">;
export type IngestionMessage = S<"IngestionMessage">;
export type IngestionRecovery = S<"IngestionRecovery">;
export type IngestionCapabilityKind =
  | "ingress-protocols"
  | "identification-methods"
  | "ingress-compatibilities";
export type IngestionCapabilityEntry =
  | S<"IngestionProtocol">
  | S<"IngestionIdentification">
  | S<"IngestionCompatibility">;
export type IngestionCapability = {
  id: string;
  revision: string;
  state: string;
  available: boolean;
  entry: IngestionCapabilityEntry;
};
export type IngestionMetadata = {
  id: string;
  receipt_id: string;
  relay_receipt_id?: string;
  source_id: string;
  source_project_id?: string;
  terminal_id?: string;
  terminal_key?: string;
  child_key?: string;
  device_id?: string;
  state: string;
  error_code: string;
  message_kind?: string;
  received_at: string;
  stored_at?: string;
  byte_length?: string;
  sha256?: string;
  synthetic?: boolean;
  ingress_protocol?: string;
  ingress_version?: string;
  method_version?: string;
  acknowledgement?: string;
  spool_epoch?: string;
  relay_id?: string;
};
export type IngestionFilters = {
  q?: string;
  cursor?: string;
  limit?: number;
  state?: string;
  source_id?: string;
  source_project_id?: string;
  terminal_id?: string;
  device_id?: string;
  receipt_id?: string;
  method_version?: string;
  error_code?: string;
  from?: string;
  until?: string;
};
export type IngestionOperation = {
  source_id: string;
  source_project_id: string;
  source_state: string;
  planning_paused: boolean;
  connection_state: string;
  observed_at: string | null;
  last_received_at: string | null;
  last_measurement_at: string | null;
  last_parsed_at: string | null;
  unmatched: string;
  queued: string;
};
export type IngestionDeviceSummary = {
  source_project_id: string;
  connection_state: string;
  last_received_at: string | null;
  last_parsed_at: string | null;
  state: string;
  source_id?: string;
  terminal_id?: string;
};
type Request = <T>(
  path: string,
  method?: string,
  body?: unknown,
  notify?: boolean,
  signal?: AbortSignal,
  options?: MonitoringRequestOptions,
) => Promise<T>;
export function ingestionMethods(request: Request) {
  const base = (s: ParsingScope) => `${parsingPrefix(s)}/ingestion`,
    id = encodeURIComponent,
    q = (v: object) =>
      new URLSearchParams(
        Object.entries(v)
          .filter(([, v]) => v !== undefined && v !== "")
          .map(([k, v]) => [k, String(v)]),
      ).toString();
  const get = <T>(path: string, signal?: AbortSignal) =>
      request<T>(path, "GET", undefined, true, signal),
    post = <T = S<"IngestionResult">>(path: string, body: unknown) =>
      request<T>(path, "POST", body);
  return {
    ingestionCapabilities: (
      s: ParsingScope,
      kind: IngestionCapabilityKind,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<{ items: IngestionCapability[]; next_cursor: string }>(
        `${parsingPrefix(s)}/parsing/${kind}?${q(f)}`,
        signal,
      ),
    ingestionSync: (input: S<"IngestionMeta">) =>
      post("/platform/parsing/ingestion-catalog-syncs", input),
    ingestionCapabilityState: (
      kind: IngestionCapabilityKind,
      key: string,
      input: S<"ParsingStateInput">,
    ) => post(`/platform/parsing/${kind}/${id(key)}/state`, input),
    ingestionSourceOptions: (signal?: AbortSignal) =>
      get<{
        tenants: { id: string; name: string }[];
        projects: { id: string; tenant_id: string; name: string }[];
        manufacturers: {
          id: string;
          tenant_id: string | null;
          name: string;
          scope: string;
        }[];
      }>("/platform/ingestion/source-options", signal),
    ingestionRelays: (signal?: AbortSignal) =>
      get<{
        items: {
          id: string;
          name: string;
          configuration_ref: string;
          revision: string;
        }[];
        configured: { id: string; configuration_ref: string }[];
        enabled: boolean;
      }>("/platform/ingestion/relays", signal),
    ingestionCreateRelay: (input: S<"IngestionRelayInput">) =>
      post("/platform/ingestion/relays", input),
    ingestionSources: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) => get<S<"IngestionSourcePage">>(`${base(s)}/sources?${q(f)}`, signal),
    ingestionSource: (s: ParsingScope, source: string, signal?: AbortSignal) =>
      get<IngestionSourceDetail>(`${base(s)}/sources/${id(source)}`, signal),
    ingestionSaveSource: (input: IngestionSourceInput, source?: string) =>
      post(
        `/platform/ingestion/sources${source ? `/${id(source)}/revisions` : ""}`,
        input,
      ),
    ingestionSourceCommand: (
      s: ParsingScope,
      source: string,
      input: S<"IngestionCommand">,
    ) =>
      post(
        `${base(s)}/sources/${id(source)}/${s.kind === "platform" ? "commands" : "confirm"}`,
        input,
      ),
    ingestionPlanning: (
      s: ParsingScope,
      source: string,
      input: S<"IngestionCommand">,
    ) => post(`${base(s)}/sources/${id(source)}/planning`, input),
    ingestionReviseTerminal: (
      s: ParsingScope,
      terminal: string,
      input: S<"IngestionTerminalInput">,
    ) => post(`${base(s)}/terminals/${id(terminal)}/revisions`, input),
    ingestionTerminalHistory: (
      s: ParsingScope,
      terminal: string,
      signal?: AbortSignal,
    ) =>
      get<IngestionTerminal[]>(
        `${base(s)}/terminals/${id(terminal)}/revisions`,
        signal,
      ),
    ingestionTerminals: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<S<"IngestionTerminalPage">>(`${base(s)}/terminals?${q(f)}`, signal),
    ingestionTerminal: (
      s: ParsingScope,
      terminal: string,
      signal?: AbortSignal,
    ) => get<IngestionTerminal>(`${base(s)}/terminals/${id(terminal)}`, signal),
    ingestionCreateTerminal: (
      s: ParsingScope,
      input: S<"IngestionTerminalInput">,
    ) => post(`${base(s)}/terminals`, input),
    ingestionAssociations: (
      s: ParsingScope,
      terminal: string,
      signal?: AbortSignal,
    ) =>
      get<IngestionAssociation[]>(
        `${base(s)}/terminals/${id(terminal)}/associations`,
        signal,
      ),
    ingestionAssociate: (
      s: ParsingScope,
      terminal: string,
      input: S<"IngestionAssociationInput">,
      preview = false,
    ) =>
      post(
        `${base(s)}/terminals/${id(terminal)}/associations${preview ? "/preview" : ""}`,
        input,
      ),
    ingestionAssociationState: (
      s: ParsingScope,
      association: string,
      input: S<"IngestionCommand">,
    ) => post(`${base(s)}/associations/${id(association)}/state`, input),
    ingestionCancelChange: (
      s: ParsingScope,
      change: string,
      input: S<"IngestionMeta">,
    ) => post(`${base(s)}/changes/${id(change)}/cancel`, input),
    ingestionGrants: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<S<"IngestionGrant">[]>(`${base(s)}/execution-grants?${q(f)}`, signal),
    ingestionSaveGrant: (s: ParsingScope, input: S<"IngestionGrantInput">) =>
      post(`${base(s)}/execution-grants`, input),
    ingestionMessages: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<{ items: IngestionMetadata[]; next_cursor: string }>(
        `${base(s)}/messages?${q(f)}`,
        signal,
      ),
    ingestionMessage: (
      s: ParsingScope,
      message: string,
      signal?: AbortSignal,
    ) => get<IngestionMessage>(`${base(s)}/messages/${id(message)}`, signal),
    ingestionDecisions: (
      s: ParsingScope,
      message: string,
      signal?: AbortSignal,
    ) =>
      get<
        {
          unit_id: string;
          unit_key: string;
          state: string;
          generation: string;
          method: S<"IngestionIdentification">;
          input_view: S<"IngestionView">;
          decision_state: string;
          decision_error: string;
        }[]
      >(`${base(s)}/messages/${id(message)}/decisions`, signal),
    ingestionResults: (
      s: ParsingScope,
      message: string,
      signal?: AbortSignal,
    ) =>
      get<S<"IngestionResultView">[]>(
        `${base(s)}/messages/${id(message)}/results`,
        signal,
      ),
    ingestionContent: (
      s: ParsingScope,
      message: string,
      result?: string,
      signal?: AbortSignal,
    ) =>
      request<Blob>(
        `${base(s)}/messages/${id(message)}/${result ? `results/${id(result)}/content` : "content"}`,
        "GET",
        undefined,
        true,
        signal,
        { responseType: "blob" },
      ),
    ingestionIntegrity: (signal?: AbortSignal) =>
      get<
        {
          id: string;
          relay_id: string;
          receipt_id: string;
          source_id: string;
          error_code: string;
          occurrences: string;
          last_seen_at: string;
        }[]
      >("/platform/ingestion/integrity-events", signal),
    ingestionUnmatched: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<{ items: IngestionMetadata[]; next_cursor: string }>(
        `${base(s)}/${s.kind === "platform" ? "quarantine" : "unmatched"}?${q(f)}`,
        signal,
      ),
    ingestionCloseUnmatched: (
      s: ParsingScope,
      receipt: string,
      input: S<"IngestionMeta">,
    ) => post(`${base(s)}/unmatched/${id(receipt)}/close`, input),
    ingestionRecoveryGeneration:(s:ParsingScope,job:string,generation:string,signal?:AbortSignal)=>get<IngestionRecovery>(`${base(s)}/recovery-operations/${id(job)}/generations/${id(generation)}`,signal),
    ingestionRecoveries: (s: ParsingScope, signal?: AbortSignal) =>
      get<IngestionRecovery[]>(`${base(s)}/recovery-operations`, signal),
    ingestionRecover: (
      s: ParsingScope,
      input: S<"IngestionRecoveryInput">,
      preview = false,
    ) =>
      post(`${base(s)}/recovery-operations${preview ? "/preview" : ""}`, input),
    ingestionOperations: (
      s: ParsingScope,
      f: IngestionFilters = {},
      signal?: AbortSignal,
    ) =>
      get<{
        items: IngestionOperation[];
        changes: {
          id: string;
          state: string;
          error_code: string;
          source_id: string;
        }[];
      }>(`${base(s)}/operations?${q(f)}`, signal),
    ingestionDevice: (s: ParsingScope, device: string, signal?: AbortSignal) =>
      get<IngestionDeviceSummary[]>(
        `${parsingPrefix(s)}/devices/${id(device)}/ingestion`,
        signal,
      ),
  };
}
