import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type DeviceScope = { kind: 'tenant'; tenantId: string } | { kind: 'project'; tenantId: string; projectId: string };
export type DeviceTenantScope = Extract<DeviceScope, { kind: 'tenant' }>;
export type DeviceCatalogScope = { kind: 'platform' } | { kind: 'tenant'; tenantId: string; source: 'public' | 'private' };
export type DeviceCatalogWriteScope = { kind: 'platform' } | { kind: 'tenant'; tenantId: string; source: 'private' };
export type DeviceFilter = {q?: string; cursor?: string; limit?: number; manufacturer_id?: string; product_id?: string; product_version_id?: string; device_id?: string; status?: string; installation?: 'recorded'|'unrecorded'; source?: 'all'|'public'|'private'; state?: string; level?: 'manufacturer'|'product'|'device'|'output'; parent_id?: string; kind?: string; slot_key?: string; metric_version_id?: string; unit_version_id?: string; compatible_only?: boolean};
export type DeviceSourceTarget = { facility_id: string; point_id: string; channel_id: string };
export type Device = Schema<'Device'>;
export type DeviceActions = Schema<'DeviceActions'>;
export type DeviceCatalogActions = Schema<'DeviceCatalogActions'>;
export type DeviceCatalogChange = Schema<'DeviceCatalogChange'>;
export type DeviceCatalogChanges = Schema<'DeviceCatalogChanges'>;
export type DeviceChannelCandidate = Schema<'DeviceChannelCandidate'>;
export type DeviceChannelCandidatePage = Schema<'DeviceChannelCandidatePage'>;
export type DeviceContext = Schema<'DeviceContext'>;
export type DeviceCreate = Schema<'DeviceCreate'>;
export type DeviceError = Schema<'DeviceError'>;
export type DeviceFieldIssue = Schema<'DeviceFieldIssue'>;
export type DeviceGrant = Schema<'DeviceGrant'>;
export type DeviceGrantCandidate = Schema<'DeviceGrantCandidate'>;
export type DeviceGrantCandidatePage = Schema<'DeviceGrantCandidatePage'>;
export type DeviceGrantChangeSet = Schema<'DeviceGrantChangeSet'>;
export type DeviceGrantChanges = Schema<'DeviceGrantChanges'>;
export type DeviceGrantHistory = Schema<'DeviceGrantHistory'>;
export type DeviceGrantPage = Schema<'DeviceGrantPage'>;
export type DeviceInstallation = Schema<'DeviceInstallation'>;
export type DeviceInstallationCandidate = Schema<'DeviceInstallationCandidate'>;
export type DeviceInstallationCandidatePage = Schema<'DeviceInstallationCandidatePage'>;
export type DeviceInstallationCorrect = Schema<'DeviceInstallationCorrect'>;
export type DeviceInstallationCreate = Schema<'DeviceInstallationCreate'>;
export type DeviceInstallationEnd = Schema<'DeviceInstallationEnd'>;
export type DeviceInstallationPage = Schema<'DeviceInstallationPage'>;
export type DeviceInstallationRelocate = Schema<'DeviceInstallationRelocate'>;
export type DeviceInstallationTarget = Schema<'DeviceInstallationTarget'>;
export type DeviceManufacturer = Schema<'DeviceManufacturer'>;
export type DeviceManufacturerCreate = Schema<'DeviceManufacturerCreate'>;
export type DeviceManufacturerPage = Schema<'DeviceManufacturerPage'>;
export type DeviceManufacturerState = Schema<'DeviceManufacturerState'>;
export type DeviceManufacturerUpdate = Schema<'DeviceManufacturerUpdate'>;
export type DeviceMutationPreview = Schema<'DeviceMutationPreview'>;
export type DeviceOutput = Schema<'DeviceOutput'>;
export type DeviceOutputConfigure = Schema<'DeviceOutputConfigure'>;
export type DeviceOutputEnd = Schema<'DeviceOutputEnd'>;
export type DeviceOutputPage = Schema<'DeviceOutputPage'>;
export type DeviceOutputReplace = Schema<'DeviceOutputReplace'>;
export type DeviceOutputSlot = Schema<'DeviceOutputSlot'>;
export type DeviceOutputSlotPage = Schema<'DeviceOutputSlotPage'>;
export type DeviceOutputSource = Schema<'DeviceOutputSource'>;
export type DeviceOutputSourcePage = Schema<'DeviceOutputSourcePage'>;
export type DeviceOutputUpdate = Schema<'DeviceOutputUpdate'>;
export type DevicePage = Schema<'DevicePage'>;
export type DeviceProductContent = Schema<'DeviceProductContent'>;
export type DeviceProductCopy = Schema<'DeviceProductCopy'>;
export type DeviceProductCreate = Schema<'DeviceProductCreate'>;
export type DeviceProductDraft = Schema<'DeviceProductDraft'>;
export type DeviceProductDraftCreate = Schema<'DeviceProductDraftCreate'>;
export type DeviceProductDraftRevision = Schema<'DeviceProductDraftRevision'>;
export type DeviceProductDraftSave = Schema<'DeviceProductDraftSave'>;
export type DeviceProductIdentity = Schema<'DeviceProductIdentity'>;
export type DeviceProductPage = Schema<'DeviceProductPage'>;
export type DeviceProductSlot = Schema<'DeviceProductSlot'>;
export type DeviceProductSummary = Schema<'DeviceProductSummary'>;
export type DeviceProductValidation = Schema<'DeviceProductValidation'>;
export type DeviceProductVersion = Schema<'DeviceProductVersion'>;
export type DeviceProductVersionPage = Schema<'DeviceProductVersionPage'>;
export type DeviceProductVersionState = Schema<'DeviceProductVersionState'>;
export type DeviceResourceChange = Schema<'DeviceResourceChange'>;
export type DeviceResourceChanges = Schema<'DeviceResourceChanges'>;
export type DeviceRevisionInput = Schema<'DeviceRevisionInput'>;
export type DeviceSemantics = Schema<'DeviceSemantics'>;
export type DeviceSourceActions = Schema<'DeviceSourceActions'>;
export type DeviceSourceBind = Schema<'DeviceSourceBind'>;
export type DeviceSourceBinding = Schema<'DeviceSourceBinding'>;
export type DeviceSourceCandidate = Schema<'DeviceSourceCandidate'>;
export type DeviceSourceCandidatePage = Schema<'DeviceSourceCandidatePage'>;
export type DeviceSourceCurrent = Schema<'DeviceSourceCurrent'>;
export type DeviceSourceHistory = Schema<'DeviceSourceHistory'>;
export type DeviceSourceSwitch = Schema<'DeviceSourceSwitch'>;
export type DeviceSourceUnbind = Schema<'DeviceSourceUnbind'>;
export type DeviceState = Schema<'DeviceState'>;
export type DeviceTreeNode = Schema<'DeviceTreeNode'>;
export type DeviceTreePage = Schema<'DeviceTreePage'>;
export type DeviceUpdate = Schema<'DeviceUpdate'>;
export type DeviceWriteResult = Schema<'DeviceWriteResult'>;
export type DeviceFixedSlot = Schema<'DeviceFixedSlot'>;
export type DeviceConfigurableSlot = Schema<'DeviceConfigurableSlot'>;
export type DeviceInstallationResourceTarget = Schema<'DeviceInstallationResourceTarget'>;
export type DeviceInstallationLocationTarget = Schema<'DeviceInstallationLocationTarget'>;
export type DeviceTenantSourceBind = Schema<'DeviceTenantSourceBind'>;
export type DeviceProjectSourceBind = Schema<'DeviceProjectSourceBind'>;
export type DeviceTenantSourceBindPreview = Schema<'DeviceTenantSourceBindPreview'>;
export type DeviceProjectSourceBindPreview = Schema<'DeviceProjectSourceBindPreview'>;
export type DeviceTenantSourceSwitch = Schema<'DeviceTenantSourceSwitch'>;
export type DeviceProjectSourceSwitch = Schema<'DeviceProjectSourceSwitch'>;
export type DeviceTenantSourceSwitchPreview = Schema<'DeviceTenantSourceSwitchPreview'>;
export type DeviceProjectSourceSwitchPreview = Schema<'DeviceProjectSourceSwitchPreview'>;
export type DeviceTenantSourceUnbind = Schema<'DeviceTenantSourceUnbind'>;
export type DeviceProjectSourceUnbind = Schema<'DeviceProjectSourceUnbind'>;
export type DeviceTenantSourceUnbindPreview = Schema<'DeviceTenantSourceUnbindPreview'>;
export type DeviceProjectSourceUnbindPreview = Schema<'DeviceProjectSourceUnbindPreview'>;
export type DeviceTenantSourcePreviewInput = Schema<'DeviceTenantSourcePreviewInput'>;
export type DeviceProjectSourcePreviewInput = Schema<'DeviceProjectSourcePreviewInput'>;
export type DevicePreviewUpdate = Schema<'DevicePreviewUpdate'>;
export type DevicePreviewState = Schema<'DevicePreviewState'>;
export type DevicePreviewDelete = Schema<'DevicePreviewDelete'>;
export type DevicePreviewOutputReplace = Schema<'DevicePreviewOutputReplace'>;
export type DevicePreviewOutputEnd = Schema<'DevicePreviewOutputEnd'>;
export type DevicePreviewInstallationCreate = Schema<'DevicePreviewInstallationCreate'>;
export type DevicePreviewInstallationEnd = Schema<'DevicePreviewInstallationEnd'>;
export type DevicePreviewInstallationRelocate = Schema<'DevicePreviewInstallationRelocate'>;
export type DevicePreviewInstallationCorrect = Schema<'DevicePreviewInstallationCorrect'>;
export type DeviceMutationPreviewInput = Schema<'DeviceMutationPreviewInput'>;
export type DeviceManufacturerUpdatePreview = Schema<'DeviceManufacturerUpdatePreview'>;
export type DeviceManufacturerStatePreview = Schema<'DeviceManufacturerStatePreview'>;
export type DeviceManufacturerDeletePreview = Schema<'DeviceManufacturerDeletePreview'>;
export type DeviceManufacturerPreviewInput = Schema<'DeviceManufacturerPreviewInput'>;
export type DeviceProductStatePreview = Schema<'DeviceProductStatePreview'>;
export type DeviceProductDeletePreview = Schema<'DeviceProductDeletePreview'>;
export type DeviceProductPreviewInput = Schema<'DeviceProductPreviewInput'>;
export type DeviceSourceVisibleBinding = Schema<'DeviceSourceVisibleBinding'>;
export type DeviceSourceRestrictedBinding = Schema<'DeviceSourceRestrictedBinding'>;
export type DeviceSourceEmptyBinding = Schema<'DeviceSourceEmptyBinding'>;
export type DeviceProject = Schema<'DeviceProject'>;
export type DeviceProjectOutput = Schema<'DeviceProjectOutput'>;
export type DeviceProjectPage = Schema<'DeviceProjectPage'>;
export type DeviceProjectOutputPage = Schema<'DeviceProjectOutputPage'>;
export type DeviceEmptyInput = Schema<'DeviceEmptyInput'>;

type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal) => Promise<T>;
const id = encodeURIComponent;
const root = (s: DeviceScope | {kind:'platform'}) => s.kind === 'platform' ? '/platform' : `/tenants/${id(s.tenantId)}${s.kind === 'project' ? `/projects/${id(s.projectId)}` : ''}`;
const catalog = (s: DeviceCatalogScope) => s.kind === 'platform' ? '/platform/device-definitions' : `/tenants/${id(s.tenantId)}/${s.source === 'public' ? 'device-catalog' : 'device-definitions'}`;
const query = (f: DeviceFilter = {}) => new URLSearchParams(Object.entries(f).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
const source = (s: DeviceScope, t: DeviceSourceTarget) => `${root(s)}/engineering/resources/facilities/${id(t.facility_id)}/points/${id(t.point_id)}/channels/${id(t.channel_id)}/source`;
const grants = (tenant: string, project: string) => `/tenants/${id(tenant)}/projects/${id(project)}/device-access`;
export function deviceMethods(request: Request) {
 const get = <T>(path:string,f:DeviceFilter={},signal?:AbortSignal) => request<T>(`${path}?${query(f)}`,'GET',undefined,true,signal);
 return {
  deviceManufacturers: (scope:DeviceCatalogScope,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceManufacturerPage>(`${catalog(scope)}/manufacturers`,f,signal),
  deviceManufacturer: (scope:DeviceCatalogWriteScope,manufacturer:string,signal?:AbortSignal) => get<DeviceManufacturer>(`${catalog(scope)}/manufacturers/${id(manufacturer)}`,{},signal),
  deviceProducts: (scope:DeviceCatalogScope,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceProductPage>(`${catalog(scope)}/products`,f,signal),
  deviceProduct: (scope:DeviceCatalogWriteScope,product:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceProductIdentity>(`${catalog(scope)}/products/${id(product)}`,f,signal),
  devicePublicVersions: (tenant:string,product:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceProductVersionPage>(`/tenants/${id(tenant)}/device-catalog/products/${id(product)}/versions`,f,signal),
  deviceProductVersion: (scope:DeviceCatalogScope,product:string,version:string,signal?:AbortSignal) => get<DeviceProductVersion>(`${catalog(scope)}/products/${id(product)}/versions/${id(version)}`,{},signal),
  deviceCatalogChanges: (scope:DeviceCatalogWriteScope,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceCatalogChanges>(`${catalog(scope)}/changes`,f,signal),
  deviceTree: (scope:DeviceScope,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceTreePage>(`${root(scope)}/device-tree`,f,signal),
  devices: (scope:DeviceScope,f:DeviceFilter={},signal?:AbortSignal) => get<DevicePage>(`${root(scope)}/devices`,f,signal),
  device: (scope:DeviceScope,device:string,signal?:AbortSignal) => get<Device>(`${root(scope)}/devices/${id(device)}`,{},signal),
  deviceProductCandidates: (scope:DeviceTenantScope,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceProductVersionPage>(`${root(scope)}/devices/product-candidates`,f,signal),
  deviceSlots: (scope:DeviceTenantScope,device:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceOutputSlotPage>(`${root(scope)}/devices/${id(device)}/outputs`,f,signal),
  deviceGenerations: (scope:DeviceTenantScope,device:string,f:DeviceFilter & {slot_key:string},signal?:AbortSignal) => get<DeviceOutputPage>(`${root(scope)}/devices/${id(device)}/output-generations`,f,signal),
  projectDeviceOutputs: (scope:Extract<DeviceScope,{kind:'project'}>,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceProjectOutputPage>(`${root(scope)}/device-outputs`,f,signal),
  deviceOutput: (scope:DeviceScope,output:string,device?:string,signal?:AbortSignal) => get<DeviceOutput>(scope.kind==='project'?`${root(scope)}/device-outputs/${id(output)}`:`${root(scope)}/devices/${id(device!)}/outputs/${id(output)}`,{},signal),
  deviceInstallations: (scope:DeviceScope,device:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceInstallationPage>(`${root(scope)}/devices/${id(device)}/installations`,f,signal),
  deviceInstallationTargets: (scope:DeviceTenantScope,device:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceInstallationCandidatePage>(`${root(scope)}/devices/${id(device)}/installation-targets`,f,signal),
  deviceChanges: (scope:DeviceScope,device:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceResourceChanges>(`${root(scope)}/devices/${id(device)}/changes`,f,signal),
  deviceOutputChanges: (scope:Extract<DeviceScope,{kind:'project'}>,output:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceResourceChanges>(`${root(scope)}/device-outputs/${id(output)}/changes`,f,signal),
  deviceSource: (scope:DeviceScope,target:DeviceSourceTarget,signal?:AbortSignal) => get<DeviceSourceCurrent>(source(scope,target),{},signal),
  deviceSourceHistory: (scope:DeviceScope,target:DeviceSourceTarget,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceSourceHistory>(`${source(scope,target)}/history`,f,signal),
  deviceSourceCandidates: (scope:DeviceScope,target:DeviceSourceTarget,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceSourceCandidatePage>(`${source(scope,target)}/candidates`,f,signal),
  deviceChannelCandidates: (scope:DeviceScope,output:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceChannelCandidatePage>(`${root(scope)}/device-outputs/${id(output)}/channel-candidates`,f,signal),
  deviceOutputSources: (scope:DeviceScope,output:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceOutputSourcePage>(`${root(scope)}/device-outputs/${id(output)}/source-history`,f,signal),
  deviceGrants: (tenant:string,project:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceGrantPage>(grants(tenant,project),f,signal),
  deviceGrantCandidates: (tenant:string,project:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceGrantCandidatePage>(`${grants(tenant,project)}/candidates`,f,signal),
  deviceGrantChanges: (tenant:string,project:string,f:DeviceFilter={},signal?:AbortSignal) => get<DeviceGrantChanges>(`${grants(tenant,project)}/changes`,f,signal),
  deviceReceipt: (scope:DeviceScope|{kind:'platform'},requestId:string,signal?:AbortSignal) => get<DeviceWriteResult>(`${root(scope)}/device-requests/${id(requestId)}`,{},signal),
  createDeviceManufacturer: (scope:DeviceCatalogWriteScope,input:DeviceManufacturerCreate) => request<DeviceWriteResult>(`${catalog(scope)}/manufacturers`,'POST',input),
  updateDeviceManufacturer: (scope:DeviceCatalogWriteScope,manufacturer:string,input:DeviceManufacturerUpdate) => request<DeviceWriteResult>(`${catalog(scope)}/manufacturers/${id(manufacturer)}`,'PATCH',input),
  previewDeviceManufacturer: (scope:DeviceCatalogWriteScope,manufacturer:string,input:DeviceManufacturerPreviewInput) => request<DeviceMutationPreview>(`${catalog(scope)}/manufacturers/${id(manufacturer)}/mutation-preview`,'POST',input),
  stateDeviceManufacturer: (scope:DeviceCatalogWriteScope,manufacturer:string,input:DeviceManufacturerState) => request<DeviceWriteResult>(`${catalog(scope)}/manufacturers/${id(manufacturer)}/state`,'POST',input),
  deleteDeviceManufacturer: (scope:DeviceCatalogWriteScope,manufacturer:string,input:DeviceRevisionInput) => request<DeviceWriteResult>(`${catalog(scope)}/manufacturers/${id(manufacturer)}/delete`,'POST',input),
  createDeviceProduct: (scope:DeviceCatalogWriteScope,input:DeviceProductCreate) => request<DeviceWriteResult>(`${catalog(scope)}/products`,'POST',input),
  copyDeviceProduct: (scope:DeviceCatalogWriteScope,input:DeviceProductCopy) => request<DeviceWriteResult>(`${catalog(scope)}/products/copy`,'POST',input),
  saveDeviceProductDraft: (scope:DeviceCatalogWriteScope,product:string,input:DeviceProductDraftSave) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/draft`,'PUT',input),
  newDeviceProductDraft: (scope:DeviceCatalogWriteScope,product:string,input:DeviceProductDraftCreate) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/drafts`,'POST',input),
  deleteDeviceProductDraft: (scope:DeviceCatalogWriteScope,product:string,input:DeviceProductDraftRevision) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/draft/delete`,'POST',input),
  publishDeviceProduct: (scope:DeviceCatalogWriteScope,product:string,input:DeviceProductDraftRevision) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/publish`,'POST',input),
  stateDeviceProductVersion: (scope:DeviceCatalogWriteScope,product:string,version:string,input:DeviceProductVersionState) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/versions/${id(version)}/state`,'POST',input),
  previewDeviceProduct: (scope:DeviceCatalogWriteScope,product:string,input:DeviceProductPreviewInput) => request<DeviceMutationPreview>(`${catalog(scope)}/products/${id(product)}/mutation-preview`,'POST',input),
  deleteDeviceProduct: (scope:DeviceCatalogWriteScope,product:string,input:DeviceRevisionInput) => request<DeviceWriteResult>(`${catalog(scope)}/products/${id(product)}/delete`,'POST',input),
  createDevice: (scope:DeviceTenantScope,input:DeviceCreate) => request<DeviceWriteResult>(`${root(scope)}/devices`,'POST',input),
  updateDevice: (scope:DeviceTenantScope,device:string,input:DeviceUpdate) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}`,'PATCH',input),
  stateDevice: (scope:DeviceTenantScope,device:string,input:DeviceState) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/state`,'POST',input),
  deleteDevice: (scope:DeviceTenantScope,device:string,input:DeviceRevisionInput) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/delete`,'POST',input),
  previewDevice: (scope:DeviceTenantScope,device:string,input:DeviceMutationPreviewInput) => request<DeviceMutationPreview>(`${root(scope)}/devices/${id(device)}/mutation-preview`,'POST',input),
  configureDeviceOutput: (scope:DeviceTenantScope,device:string,input:DeviceOutputConfigure) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/outputs`,'POST',input),
  updateDeviceOutput: (scope:DeviceTenantScope,device:string,output:string,input:DeviceOutputUpdate) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/outputs/${id(output)}`,'PATCH',input),
  replaceDeviceOutput: (scope:DeviceTenantScope,device:string,output:string,input:DeviceOutputReplace) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/outputs/${id(output)}/replace`,'POST',input),
  endDeviceOutput: (scope:DeviceTenantScope,device:string,output:string,input:DeviceOutputEnd) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/outputs/${id(output)}/end`,'POST',input),
  createDeviceInstallation: (scope:DeviceTenantScope,device:string,input:DeviceInstallationCreate) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/installations`,'POST',input),
  endDeviceInstallation: (scope:DeviceTenantScope,device:string,installation:string,input:DeviceInstallationEnd) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/installations/${id(installation)}/end`,'POST',input),
  relocateDeviceInstallation: (scope:DeviceTenantScope,device:string,installation:string,input:DeviceInstallationRelocate) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/installations/${id(installation)}/relocate`,'POST',input),
  correctDeviceInstallation: (scope:DeviceTenantScope,device:string,installation:string,input:DeviceInstallationCorrect) => request<DeviceWriteResult>(`${root(scope)}/devices/${id(device)}/installations/${id(installation)}`,'PATCH',input),
  previewDeviceSource: (scope:DeviceScope,target:DeviceSourceTarget,input:DeviceTenantSourcePreviewInput|DeviceProjectSourcePreviewInput) => request<DeviceMutationPreview>(`${source(scope,target)}/preview`,'POST',input),
  previewDeviceGrants: (tenant:string,project:string,input:DeviceGrantChangeSet) => request<DeviceMutationPreview>(`${grants(tenant,project)}/preview`,'POST',input),
  changeDeviceGrants: (tenant:string,project:string,input:DeviceGrantChangeSet) => request<DeviceWriteResult>(`${grants(tenant,project)}/changes`,'POST',input),
  validateDeviceProduct: (scope:DeviceCatalogWriteScope,product:string) => request<DeviceProductValidation>(`${catalog(scope)}/products/${id(product)}/validate`,'POST',{}),
  changeDeviceSource: (scope:DeviceScope,target:DeviceSourceTarget,input:DeviceTenantSourcePreviewInput|DeviceProjectSourcePreviewInput) => { const {action,...body}=input;return request<DeviceWriteResult>(`${source(scope,target)}/${action}`,'POST',body); },
 };
}
