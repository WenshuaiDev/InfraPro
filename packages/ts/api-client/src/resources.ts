import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type Resource = Schema<'Resource'>;
export type ResourceDetail = Schema<'ResourceDetail'>;
export type ResourceItem = Schema<'ResourceItem'>;
export type ResourceChange = Schema<'ResourceChange'>;
export type ResourcePosition = Schema<'ResourcePosition'>;
export type ResourceSampling = Schema<'ResourceSampling'>;
export type ResourceResult = Schema<'ResourceResult'>;
export type ResourceKind = Resource['kind'];
export type ResourceTarget = { kind: ResourceKind; facility?: string; point?: string; id?: string };
export type ResourceFilter = { q?: string; status?: 'normal' | 'retired' | 'all'; type_definition_id?: string; deleted?: 'only'; parent_id?: string; owner?: string; limit?: number; cursor?: string };
export type ResourceCreate = Schema<'ResourceFacilityCreate'> | Schema<'ResourceComponentCreate'> | Schema<'ResourcePointCreate'> | Schema<'ResourceChannelCreate'>;
export type ResourceUpdate = Schema<'ResourceFacilityUpdate'> | Schema<'ResourceComponentUpdate'> | Schema<'ResourcePointUpdate'> | Schema<'ResourceChannelUpdate'>;
export type ResourceState = Schema<'ResourceFacilityState'> | Schema<'ResourceComponentState'>;
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal) => Promise<T>;
const base = (tenant: string) => `/tenants/${encodeURIComponent(tenant)}/engineering/resources`;
function path(tenant: string, t: ResourceTarget) { const root = `${base(tenant)}/facilities`; const list = t.kind === 'facility' ? root : `${root}/${encodeURIComponent(t.facility!)}/${t.kind === 'component' ? 'components' : t.kind === 'point' ? 'points' : `points/${encodeURIComponent(t.point!)}/channels`}`; return t.id ? `${list}/${encodeURIComponent(t.id)}` : list; }
const query = (f: object) => new URLSearchParams(Object.entries(f).filter(([,v]) => v !== undefined && v !== '').map(([k,v]) => [k,String(v)])).toString();
export function resourceMethods(request: Request) {
 return {
  resourceList: (tenant: string, t: ResourceTarget, f: ResourceFilter = {}, signal?: AbortSignal) => request<Schema<'ResourcePage'>>(`${path(tenant,t)}?${query(f)}`, 'GET', undefined, true, signal),
  resourceDetail: (tenant: string, t: ResourceTarget, signal?: AbortSignal) => request<ResourceDetail>(path(tenant,t), 'GET', undefined, true, signal),
  resourceCreate: (tenant: string, t: ResourceTarget, body: ResourceCreate) => request<ResourceResult>(path(tenant,t), 'POST', body),
  resourceUpdate: (tenant: string, t: ResourceTarget, body: ResourceUpdate) => request<ResourceResult>(path(tenant,t), 'PATCH', body),
  resourceState: (tenant: string, t: ResourceTarget, action: 'retire' | 'restore' | 'delete', body: ResourceState) => request<ResourceResult>(`${path(tenant,t)}/${action}`, 'POST', body),
  resourceChanges: (tenant: string, facility: string, f: { kind?: ResourceKind; id?: string; cursor?: string; limit?: number } = {}, signal?: AbortSignal) => request<Schema<'ResourceChanges'>>(`${base(tenant)}/facilities/${encodeURIComponent(facility)}/changes?${query(f)}`, 'GET', undefined, true, signal),
  resourceReceipt: (tenant: string, id: string, signal?: AbortSignal) => request<ResourceResult>(`${base(tenant)}/requests/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
 };
}
