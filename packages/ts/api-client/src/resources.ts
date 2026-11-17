import type { MonitoringRequestOptions } from './monitoring';
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
export type ResourceTreeRef = Schema<'ResourceTreeRef'>;
export type ResourceTreeNode = Schema<'ResourceTreeNode'>;
export type ResourceTreePage = Schema<'ResourceTreePage'>;
export type ResourceTreeFilter = Pick<ResourceFilter, 'q' | 'status' | 'type_definition_id' | 'deleted' | 'limit' | 'cursor'> & { facility_id?: string; parent_kind?: Exclude<ResourceKind, 'channel'>; parent_id?: string; node_id?: string };
export type ResourceTarget = { kind: ResourceKind; facility?: string; point?: string; id?: string };
export type ResourceFilter = { q?: string; status?: 'normal' | 'retired' | 'all'; type_definition_id?: string; deleted?: 'only'; parent_id?: string; owner?: string; owner_kind?: Exclude<ResourceKind, 'channel'>; owner_id?: string; limit?: number; cursor?: string };
export type ResourceStructureRef = Schema<'ResourceStructureRef'>;
export type ResourceBorehole = Schema<'ResourceBorehole'>;
export type ResourceBoreholePosition = Schema<'ResourceBoreholePosition'>;
export type ResourceMoveInput = Schema<'ResourceMoveInput'>;
export type ResourceMovePreview = Schema<'ResourceMovePreview'>;
export type ResourceTypeAdoptionInput = Schema<'ResourceTypeAdoptionInput'>;
export type ResourceTypeAdoptionPreview = Schema<'ResourceTypeAdoptionPreview'>;
export type ResourceAttachmentRef = Schema<'ResourceAttachmentRef'>;
export type ResourceStructureRecordInput = Schema<'ResourceStructureRecordInput'>;
export type ResourceStructureRecordPage = Schema<'ResourceStructureRecordPage'>;
export type ResourceStructureRecordPreview = Schema<'ResourceStructureRecordPreview'>;
export type ResourceCreate = Schema<'ResourceMonitoringSectionCreate'> | Schema<'ResourceBoreholeCreate'> | Schema<'ResourceFacilityCreate'> | Schema<'ResourceComponentCreate'> | Schema<'ResourcePointCreate'> | Schema<'ResourceChannelCreate'>;
export type ResourceUpdate = Schema<'ResourceMonitoringSectionUpdate'> | Schema<'ResourceBoreholeUpdate'> | Schema<'ResourceFacilityUpdate'> | Schema<'ResourceComponentUpdate'> | Schema<'ResourcePointUpdate'> | Schema<'ResourceChannelUpdate'>;
export type ResourceState = Schema<'ResourceFacilityState'> | Schema<'ResourceComponentState'>;
export type ResourceScope = { kind: 'tenant'; tenantId: string } | { kind: 'project'; tenantId: string; projectId: string };
export type ResourceMutationPreview = Schema<'ResourceMutationPreview'>;
export type ResourceAccessDetail = Schema<'ProjectResourceAccessDetail'>;
export type ResourceAccessInput = Schema<'ProjectResourceAccessInput'>;
export type ResourceAccessPreview = Schema<'ProjectResourceAccessPreview'>;
export type ResourceAccessResult = Schema<'ProjectResourceAccessResult'>;
export type ResourceGrant = Schema<'ProjectResourceGrant'>;
export type ResourceGrantInput = Schema<'ProjectResourceGrantInput'>;
export type ResourceAccessFilter = { q?: string; candidate_structure_parent_kind?: Exclude<ResourceKind, 'point' | 'channel'>; candidate_structure_parent_id?: string; candidate_point_id?: string; cursor?: string; limit?: number; grant_cursor?: string; candidate_kind?: ResourceKind; candidate_cursor?: string; candidate_limit?: number; candidate_q?: string; candidate_status?: string; candidate_parent_id?: string; candidate_owner?: string; candidate_owner_kind?: Exclude<ResourceKind, 'channel'>; candidate_owner_id?: string };
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal, options?:MonitoringRequestOptions) => Promise<T>;
const workspace = (scope: ResourceScope | string) => typeof scope === 'string' ? `/tenants/${encodeURIComponent(scope)}` : `/tenants/${encodeURIComponent(scope.tenantId)}${scope.kind === 'project' ? `/projects/${encodeURIComponent(scope.projectId)}` : ''}`;
const base = (scope: ResourceScope | string) => `${workspace(scope)}/engineering/resources`;
const governance = (tenant: string, project: string, facility?: string) => `/tenants/${encodeURIComponent(tenant)}/projects/${encodeURIComponent(project)}/resource-access${facility ? `/facilities/${encodeURIComponent(facility)}` : ''}`;
function path(tenant: ResourceScope | string, t: ResourceTarget) { const root = `${base(tenant)}/facilities`; const list = t.kind === 'facility' ? root : `${root}/${encodeURIComponent(t.facility!)}/${t.kind === 'component' ? 'components' : t.kind === 'point' ? 'points' : t.kind === 'monitoring_section' ? 'monitoring-sections' : t.kind === 'borehole' ? 'boreholes' : t.point ? `points/${encodeURIComponent(t.point)}/channels` : 'channels'}`; return t.id ? `${list}/${encodeURIComponent(t.id)}` : list; }
const query = (f: object) => new URLSearchParams(Object.entries(f).filter(([,v]) => v !== undefined && v !== '').map(([k,v]) => [k,String(v)])).toString();
export function resourceMethods(request: Request) {
 return {
  resourceAttachmentContent: (scope:ResourceScope,t:ResourceTarget,record:string,variant:'content'|'preview'='content',signal?:AbortSignal)=>request<Blob>(`${path(scope,t)}/attachments/${encodeURIComponent(record)}/attachment-${variant}`,'GET',undefined,true,signal,{responseType:'blob',timeoutMs:120000}),
  resourceMovePreview: (scope: ResourceScope, t:ResourceTarget, body:ResourceMoveInput) => request<ResourceMovePreview>(`${path(scope,t)}/move-preview`,'POST',body),
  resourceMove: (scope: ResourceScope, t:ResourceTarget, body:ResourceMoveInput) => request<ResourceResult>(`${path(scope,t)}/move`,'POST',body),
  resourceAdoptTypePreview: (scope: ResourceScope, t:ResourceTarget, body:ResourceTypeAdoptionInput) => request<ResourceTypeAdoptionPreview>(`${path(scope,t)}/adopt-type-preview`,'POST',body),
  resourceAdoptType: (scope: ResourceScope, t:ResourceTarget, body:ResourceTypeAdoptionInput) => request<ResourceResult>(`${path(scope,t)}/adopt-type`,'POST',body),
  resourceStructureRecords: (scope: ResourceScope,t:ResourceTarget,filter:{kind:'positions'|'references'|'legacy'|'changes'|'attachments';cursor?:string;limit?:number},signal?:AbortSignal) => request<ResourceStructureRecordPage>(`${path(scope,t)}/structure-details?${query(filter)}`,'GET',undefined,true,signal),
  resourceStructureRecordPreview: (scope: ResourceScope,t:ResourceTarget,body:ResourceStructureRecordInput) => request<ResourceStructureRecordPreview>(`${path(scope,t)}/structure-records-preview`,'POST',body),
  resourceStructureRecord: (scope: ResourceScope,t:ResourceTarget,body:ResourceStructureRecordInput) => request<ResourceResult>(`${path(scope,t)}/structure-records`,'POST',body),
  resourceTree: (scope: ResourceScope | string, filter: ResourceTreeFilter = {}, signal?: AbortSignal) => request<ResourceTreePage>(`${base(scope)}/tree?${query(filter)}`, 'GET', undefined, true, signal),
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
  resourceCatalogVersion: (scope: ResourceScope, kind: 'type-definitions' | 'metrics' | 'units', id: string, signal?: AbortSignal) => request<Schema<'ProjectResourceCatalogVersion'>>(`${workspace(scope)}/engineering/catalog/${kind}/versions/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
  projectResourceCatalogVersion: (scope: Extract<ResourceScope, {kind:'project'}>, kind: 'type-definitions' | 'metrics' | 'units', id: string, signal?: AbortSignal) => request<Schema<'ProjectResourceCatalogVersion'>>(`${workspace(scope)}/engineering/catalog/${kind}/versions/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
 };
}
