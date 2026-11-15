/** Protocol and business capability versions are independent of the npm version. */
export { PROTOCOL_MAJOR } from "./protocol.generated.js";
export type { EventKind, ProtocolMethod } from "./protocol.generated.js";
import type { EventKind } from "./protocol.generated.js";
export const SDK_VERSION = "1.0.0";
export type ExactNumber = string;
export type Timestamp = string;
export type JSONValue =
  | null
  | boolean
  | number
  | string
  | JSONValue[]
  | { [key: string]: JSONValue };
export interface Target {
  binding?: string;
  ids?: string[];
}
export interface TimeRange {
  from?: Timestamp | null;
  to?: Timestamp | null;
}
export interface Query {
  target?: Target;
  id?: string;
  range?: TimeRange;
  view_range?: TimeRange;
  all_history?: boolean;
  qualities?: Quality[];
  timezone?: string;
  selection_id?: string;
  snapshot_id?: string;
  cursor?: string;
  limit?: number;
  search?: string;
  kind?: string;
  plot_width_px?: number;
  plot_point_budget?: number;
  period?: "day" | "month" | "year";
  output?: string;
  follow_seconds?: number;
}
export interface FileQuery extends Query {
  file_id: string;
  offset?: number;
  length?: number;
}
export type Quality = "good" | "unmarked" | "suspect" | "bad";
export type ResultState = "ready" | "empty" | "pending" | "partial";
export interface Result<T> {
  state: ResultState;
  data: T;
  read_at: Timestamp;
  authorization_revision: string;
  generation: string;
  source_vector: Record<string, string>;
}
export interface Context {
  tenant_id: string;
  project_id: string;
  instance_id: string;
  release_id: string;
  preview: boolean;
  environment: string;
  generation: string;
  mode: "fixed" | "follow";
  timezone: string;
  server_time: Timestamp;
  simulated?: boolean;
}
export interface Capability {
  name: string;
  version: number;
  availability: "available" | "unavailable";
  reason?: string;
  required_permissions: string[];
  subscribe: boolean;
  max_targets: number;
}
export interface Binding {
  kind: "channels" | "resources" | "spatial" | "artifacts";
  ids: string[];
  revision: string;
  state: ResultState | "unavailable";
  code?: string;
  from?: Timestamp;
  to?: Timestamp;
}
export interface Resource {
  id: string;
  kind: "facility" | "component" | "point" | "channel";
  facility_id: string;
  parent_id: string;
  parent_kind: string;
  code: string;
  name: string;
  status: string;
  revision: string;
}
export interface Page<T> {
  items: T[];
  next_cursor: string;
  count?: ExactNumber;
}
export interface ResourcePage extends Page<Resource> {
  count: ExactNumber;
  counts: Record<string, ExactNumber>;
}
export interface Channel {
  id: string;
  code: string;
  name: string;
  source_kind: "direct" | "derived";
  owner_kind: string;
  owner_id: string;
  owner_name: string;
  facility_id: string;
  point_id: string | null;
  metric_version_id: string | null;
  metric_name: string;
  unit: string;
  unit_version_id: string | null;
  value_kind: "integer" | "decimal" | "enum" | "boolean";
  observation_direction: string;
  reference_basis: string;
  timestamp_meaning: string;
  read_ranges: TimeRange[];
  quantity_spec?: JSONValue;
  output_semantics?: JSONValue;
}
export interface DirectObservation {
  observation_id: string;
  channel_id: string;
  source_kind: "direct";
  observed_at: Timestamp;
  interval_start: Timestamp | null;
  value_kind: string;
  value: ExactNumber;
  quality: Quality;
  quality_reason: string;
  revoked: boolean;
  [key: string]: unknown;
}
export interface Snapshot {
  id: string;
  watermark: string;
  route_revision: string;
  expires_at: Timestamp;
  query_hash: string;
}
export interface CalculatedObservation {
  source_kind: "derived"; channel_id:string; result_id:string; generation_id:string;
  config_id:string;run_id:string;output_at:Timestamp;interval_start:Timestamp|null;interval_end:Timestamp|null;
  value:ExactNumber|null;scale:number;rounding:string;rounded:boolean;data_quality:Quality;
  calculation_status:string;reason_flags:string[];sample_count:ExactNumber;excluded_count:ExactNumber;
  input_first:Timestamp|null;input_last:Timestamp|null;freshness:string;lineage_id:string;
}
export type Observation = DirectObservation | CalculatedObservation;
export interface CalculationStatus {
  last_valid_output?:CalculatedObservation|null;computed_through?:Timestamp|null;input_through?:Timestamp|null;
  pending_ranges?:TimeRange[];control_state?:string;health?:string;
}
export interface Coverage { channel_id:string;range:TimeRange;state:string;generation_id?:string; }
export interface EnumStatistic {value:string;count:ExactNumber;sample_proportion:ExactNumber;first_at:Timestamp;last_at:Timestamp;}
export interface TrendSeries {channel_id:string;range:TimeRange;resolution:string;summary:boolean;sample_count:ExactNumber;
 buckets:{range:TimeRange;sample_count:ExactNumber;points:Observation[];states:EnumStatistic[];discontinuous:boolean}[];
 coverage:string;connection:string;gap_explanation:string;
}
export interface Overview {
  ever_published_count: ExactNumber;
  active_count: ExactNumber;
  sample_count: ExactNumber;
  quality_counts: Record<Quality, ExactNumber>;
  earliest: Timestamp | null;
  latest: Timestamp | null;
  derived?: JSONValue;
}
export interface ChannelStatistic extends CalculationStatus, Overview {
  channel_id: string;
  source_kind: "direct" | "derived";
  n: ExactNumber;
  first: Observation | null;
  last: Observation | null;
  minimum: Observation | null;
  maximum: Observation | null;
  mean: ExactNumber | null;
  population_stddev: ExactNumber | null;
  latest_output?: CalculatedObservation;
  states: EnumStatistic[];
}
export interface AnalyticResult {
  state: ResultState;
  selection_id: string;
  snapshot: Snapshot;
  job_id?: string;
  overview?: Overview;
  channels?: ChannelStatistic[];
  groups?: (ChannelStatistic & {
    period: string;
    timezone: string;
    calendar_start: Timestamp;
    calendar_end: Timestamp;
    partial: boolean;
    included_ranges: TimeRange[];
  })[];
  trends?: TrendSeries[];
  coverage?: Coverage[];
  computation: string;
  next_cursor?: string;
  has_more: boolean;
  filters: {
    range?: TimeRange;
    all_history: boolean;
    qualities: Quality[];
    timezone: string;
  };
}
export interface DetailResult extends Page<Observation> {
  selection_id: string;
  snapshot: Snapshot;
  has_more: boolean;
  prev_cursor: string;
  coverage?: Coverage[];
}
export interface SourceProjection {
  configured?: boolean;
  visibility?: "full" | "restricted";
  output?: JSONValue;
  started_at?: Timestamp;
  ended_at?: Timestamp;
  [key: string]: unknown;
}
export interface Artifact {
  artifact: { id: string; name: string; [key: string]: unknown };
  version: {
    id: string;
    version_no: ExactNumber;
    output_rows: ExactNumber;
    [key: string]: unknown;
  };
  outputs: JSONValue[];
  [key: string]: unknown;
}
export interface ArtifactRow {
  result_id: string;
  output_key: string;
  at_ns: string;
  interval: {from_ns: string; to_ns: string | null} | null;
  value: ExactNumber | null;
  scale: number;
  rounded: boolean;
  calculation_status: string;
  quality: Quality;
  reason_flags: string[];
  count: ExactNumber;
  excluded_count: ExactNumber;
  lineage_ref: string;
  [key: string]: unknown;
}
export interface SpatialVersion {
  id: string;
  asset_id: string;
  number: ExactNumber;
  format: string;
  state: string;
  revision: string;
  manifest_hash: string;
}
export interface SpatialFile {
  id: string;
  name: string;
  sha256: string;
  size: ExactNumber;
  media_type: string;
}
export interface SpatialManifest {
  version_id: string;
  revision: string;
  manifest_hash: string;
  manifest: {
    files: SpatialFile[];
    pages: JSONValue[];
    elements: JSONValue[];
    coordinates: JSONValue;
    [key: string]: unknown;
  };
}
export interface FileChunk {
  file_id: string;
  media_type: string;
  size: ExactNumber;
  offset: ExactNumber;
  length: ExactNumber;
  sha256: string;
  bytes: ArrayBuffer;
}
/** Values and dictionaries are owned by the future alert provider, not the SDK. */
export interface Alert {
  id: string;
  object_id: string;
  level: string;
  status: string;
  occurred_at: Timestamp;
  updated_at: Timestamp;
  event_version: string;
}
export interface AlertQuery {
  target?: Target;
  from?: Timestamp;
  to?: Timestamp;
  levels?: string[];
  statuses?: string[];
  cursor?: string;
  limit?: number;
}
export interface AlertSummary {
  total: ExactNumber;
  by_level: Record<string, ExactNumber>;
  by_status: Record<string, ExactNumber>;
  dictionaries: {
    levels: Record<string, string>;
    statuses: Record<string, string>;
  };
}
export interface CallOptions {
  signal?: AbortSignal;
  generation?: string;
}
export interface SubscriptionEvent<T = AnalyticResult> {
  subscription_id: string;
  generation: string;
  sequence: string;
  kind: EventKind;
  result?: Result<T>;
  code?: string;
}
export interface SubscriptionInput {
  kind: "latest" | "trends" | "overview" | "statistics";
  query: Query;
  generation?: string;
}
export type Consumer = (event: SubscriptionEvent) => void | Promise<void>;
export type AlertConsumer = (event: SubscriptionEvent<Page<Alert>>) => void | Promise<void>;
export interface Subscription {
  unsubscribe(): void;
}
export type RuntimeEvent = {
  type:
    | "connected"
    | "disconnected"
    | "reconnecting"
    | "resynchronized"
    | "release_changed"
    | "revoked"
    | "paused"
    | "resumed"
    | "disposed"
    | "error";
  code?: string;
  target_release_id?: string;
};
export interface DisplayTransport {
  request<T>(
    method: string,
    params: unknown,
    options?: CallOptions,
  ): Promise<T>;
  onEvent(
    listener: (event: SubscriptionEvent | RuntimeEvent) => void,
  ): () => void;
  dispose(): void;
}
