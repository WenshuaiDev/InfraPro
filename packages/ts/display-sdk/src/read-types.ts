import type { CallOptions, SubscriptionEvent } from './types.js';
import type { ReadRequests, ReadData, ReadQueryInfo, ReadMonitoringRequest, ReadMonitoringStatisticsRequest, ReadResourcesRequest } from './read.generated.js';
export * from './read.generated.js';
export type ViewTarget = { view: string; ids?: string[]; selection?: never };
export type SelectionTarget = { selection: string; ids?: string[]; view?: never };
export type ReadMethod = keyof ReadData;
export type ReadTime = { timezone?: string; qualities?: Array<'good'|'unmarked'|'suspect'|'bad'> } & (
  | { range: { from: string; to: string }; all_history?: never; follow_seconds?: never }
  | { all_history: true; range?: never; follow_seconds?: never }
  | { follow_seconds: number; range?: never; all_history?: never }
);
type TimingKeys = 'range'|'all_history'|'follow_seconds'|'timezone'|'qualities';
export type MonitoringReadRequest = Omit<ReadMonitoringRequest, TimingKeys|'target'> & (
  | ({ target: ViewTarget } & ReadTime)
  | { target: SelectionTarget; range?: never; all_history?: never; follow_seconds?: never; qualities?: never; timezone?: never; history?: false }
);
export type MonitoringStatisticsRequest = Omit<ReadMonitoringStatisticsRequest,TimingKeys|'targets'> & ReadTime & { targets: ViewTarget[] };
type Targets<T> = { [K in keyof T]: K extends 'target'|'resource_target'|'channel_target'|'device_target'|'spatial_target' ? ViewTarget : T[K] };
export type ReadRequest<M extends keyof ReadRequests> =
  M extends 'devices.list'|'resources.query' ? Omit<Targets<ReadRequests[M]>,'target'> & {target:ViewTarget|SelectionTarget} :
  M extends 'monitoring.latest' ? MonitoringReadRequest | (Omit<ReadMonitoringRequest,TimingKeys|'target'> & { target: ViewTarget }) :
  M extends `monitoring.${string}` ? MonitoringReadRequest :
  M extends 'statistics.monitoring' ? MonitoringStatisticsRequest :
  M extends 'resources.tree'|'resources.hierarchy' ? Omit<Targets<ReadRequests[M]>,'mode'|'parent_ref'|'node_ref'> & ({mode:'root';parent_ref?:never;node_ref?:never}|{mode:'children';parent_ref:NonNullable<ReadRequests[M]['parent_ref']>;node_ref?:never}|{mode:'path';node_ref:NonNullable<ReadRequests[M]['node_ref']>;parent_ref?:never}) :
  M extends 'sources.bindings' ? Omit<Targets<ReadRequests[M]>,'point_ids'|'channel_ids'> & ({point_ids:string[];channel_ids?:never}|{channel_ids:string[];point_ids?:never}) : Targets<ReadRequests[M]>;
export type ResourceReadRequest = Targets<ReadResourcesRequest>;
interface ReadEnvelope {
  read_at: string;
  authorization_revision: string;
  generation: string;
  source_vector: Record<string,string>;
  included_field_groups: string[];
  query?: ReadQueryInfo;
}
export type ReadResult<T> = ReadEnvelope & (
  | { state: 'pending'; data: null; query: ReadQueryInfo; retry_after_ms: number }
  | { state: 'ready'|'empty'|'partial'; data: T; retry_after_ms?: never }
);
export type ReadSubscriptionEvent<T> = Omit<SubscriptionEvent<T>,'result'> & { result?: ReadResult<T> };
export type ReadConsumer<T> = (event: ReadSubscriptionEvent<T>)=>void|Promise<void>;
export interface QueryConsumer<T> {
  result: Promise<ReadResult<T>>;
  /** Idempotent; only the last consumer cancels/releases the shared query. */
  release(): void;
}
export type ReadCallOptions = CallOptions;
export type ReadErrorCode = 'query_invalid'|'field_not_granted'|'scope_unavailable'|'reset_required'|'query_expired'|'query_timeout'|'query_failed'|'invalid_cursor'|'budget_exceeded'|'capability_unavailable'|'version_incompatible'|'structure_model_upgrade_required'|'dependency_unavailable'|'not_found'|'aborted'|'disposed';
