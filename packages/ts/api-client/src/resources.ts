import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type Resource = Schema<'Resource'>;
export type ResourceDetail = Schema<'ResourceDetail'>;
export type ResourceContext = Schema<'ResourceContext'>;
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
export type ResourceScope = { kind: 'tenant'; tenantId: string } | { kind: 'project'; tenantId: string; projectId: string };
export type ResourceMutationPreview = Schema<'ResourceMutationPreview'>;
export type ResourceAccessDetail = Schema<'ProjectResourceAccessDetail'>;
export type ResourceAccessInput = Schema<'ProjectResourceAccessInput'>;
export type ResourceAccessPreview = Schema<'ProjectResourceAccessPreview'>;
export type ResourceAccessResult = Schema<'ProjectResourceAccessResult'>;
export type ResourceGrant = Schema<'ProjectResourceGrant'>;
export type ResourceGrantInput = Schema<'ProjectResourceGrantInput'>;
export type ResourceAccessFilter = { cursor?: string; limit?: number; grant_cursor?: string; candidate_kind?: ResourceKind; candidate_cursor?: string; candidate_limit?: number; candidate_q?: string; candidate_status?: string; candidate_parent_id?: string; candidate_owner?: string };
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal) => Promise<T>;
const workspace = (scope: ResourceScope | string) => typeof scope === 'string' ? `/tenants/${encodeURIComponent(scope)}` : `/tenants/${encodeURIComponent(scope.tenantId)}${scope.kind === 'project' ? `/projects/${encodeURIComponent(scope.projectId)}` : ''}`;
const base = (scope: ResourceScope | string) => `${workspace(scope)}/engineering/resources`;
const governance = (tenant: string, project: string, facility?: string) => `/tenants/${encodeURIComponent(tenant)}/projects/${encodeURIComponent(project)}/resource-access${facility ? `/facilities/${encodeURIComponent(facility)}` : ''}`;
function path(tenant: ResourceScope | string, t: ResourceTarget) { const root = `${base(tenant)}/facilities`; const list = t.kind === 'facility' ? root : `${root}/${encodeURIComponent(t.facility!)}/${t.kind === 'component' ? 'components' : t.kind === 'point' ? 'points' : `points/${encodeURIComponent(t.point!)}/channels`}`; return t.id ? `${list}/${encodeURIComponent(t.id)}` : list; }
const query = (f: object) => new URLSearchParams(Object.entries(f).filter(([,v]) => v !== undefined && v !== '').map(([k,v]) => [k,String(v)])).toString();
export function resourceMethods(request: Request) {
 return {
  resourcePreview: (scope: ResourceScope | string, t: ResourceTarget, action: 'update' | 'retire' | 'restore' | 'delete', body: object) => request<ResourceMutationPreview>(`${path(scope,t)}/mutation-preview`, 'POST', { ...body, action }),
  resourceList: (tenant: ResourceScope | string, t: ResourceTarget, f: ResourceFilter = {}, signal?: AbortSignal) => request<Schema<'ResourcePage'>>(`${path(tenant,t)}?${query(f)}`, 'GET', undefined, true, signal),
  resourceDetail: (tenant: ResourceScope | string, t: ResourceTarget, signal?: AbortSignal) => request<ResourceDetail>(path(tenant,t), 'GET', undefined, true, signal),
  resourceCreate: (tenant: ResourceScope | string, t: ResourceTarget, body: ResourceCreate) => request<ResourceResult>(path(tenant,t), 'POST', body),
  resourceUpdate: (tenant: ResourceScope | string, t: ResourceTarget, body: ResourceUpdate) => request<ResourceResult>(path(tenant,t), 'PATCH', body),
  resourceState: (tenant: ResourceScope | string, t: ResourceTarget, action: 'retire' | 'restore' | 'delete', body: ResourceState) => request<ResourceResult>(`${path(tenant,t)}/${action}`, 'POST', body),
  resourceChanges: (tenant: ResourceScope | string, facility: string, f: { kind?: ResourceKind; id?: string; cursor?: string; limit?: number } = {}, signal?: AbortSignal) => request<Schema<'ResourceChanges'>>(`${base(tenant)}/facilities/${encodeURIComponent(facility)}/changes?${query(f)}`, 'GET', undefined, true, signal),
  resourceReceipt: (tenant: ResourceScope | string, id: string, signal?: AbortSignal) => request<ResourceResult>(`${base(tenant)}/requests/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
  resourceUsage: (tenant: string, facility: string, filter: {cursor?: string;limit?: number} = {}, signal?: AbortSignal) => request<Schema<'ResourceUsagePage'>>(`${base(tenant)}/facilities/${encodeURIComponent(facility)}/project-usage?${query(filter)}`, 'GET', undefined, true, signal),
  resourceAccess: (tenant: string, project: string, facility?: string, filter: ResourceAccessFilter = {}, signal?: AbortSignal) => request<ResourceAccessDetail>(`${governance(tenant,project,facility)}?${query(filter)}`, 'GET', undefined, true, signal),
  previewResourceAccess: (tenant: string, project: string, facility: string | undefined, input: ResourceAccessInput) => request<ResourceAccessPreview>(`${governance(tenant,project,facility)}/${facility ? 'preview' : 'policy/preview'}`, 'POST', input),
  changeResourceAccess: (tenant: string, project: string, facility: string | undefined, input: ResourceAccessInput) => request<ResourceAccessResult>(`${governance(tenant,project,facility)}/${facility ? 'changes' : 'policy'}`, 'POST', input),
  resourceAccessChanges: (tenant: string, project: string, filter: {cursor?: string;limit?: number} = {}, signal?: AbortSignal) => request<Schema<'ProjectResourceAccessChanges'>>(`${governance(tenant,project)}/changes?${query(filter)}`, 'GET', undefined, true, signal),
  resourceAccessReceipt: (tenant: string, project: string, id: string) => request<ResourceAccessResult>(`${governance(tenant,project)}/requests/${encodeURIComponent(id)}`),
  projectResourceCatalog: (scope: Extract<ResourceScope, {kind:'project'}>, kind: 'type-definitions' | 'metrics' | 'units', filter: {q?: string;offset?: number;limit?: number}, signal?: AbortSignal) => request<Schema<'ProjectResourceCatalogPage'>>(`${workspace(scope)}/engineering/catalog/${kind}?${query(filter)}`, 'GET', undefined, true, signal),
  projectResourceCatalogVersion: (scope: Extract<ResourceScope, {kind:'project'}>, kind: 'type-definitions' | 'metrics' | 'units', id: string, signal?: AbortSignal) => request<Schema<'ProjectResourceCatalogVersion'>>(`${workspace(scope)}/engineering/catalog/${kind}/versions/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
 };
}
