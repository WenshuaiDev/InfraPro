import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type DeviceCategoryNode = Schema<'DeviceCategoryNode'>;
export type DeviceCategoryContent = Schema<'DeviceCategoryContent'>;
export type DeviceCategoryDictionary = Schema<'DeviceCategoryDictionary'>;
export type DeviceCategoryRelease = Schema<'DeviceCategoryRelease'>;
export type DeviceCategoryPage = Schema<'DeviceCategoryPage'>;
export type DeviceCategorySave = Schema<'DeviceCategorySave'>;
export type DeviceCategoryPublish = Schema<'DeviceCategoryPublish'>;
export type DeviceCategoryPreview = Schema<'DeviceCategoryPreview'>;
export type DeviceClassificationSelection = Schema<'DeviceClassificationSelection'>;
export type DeviceClassificationEffective = Schema<'DeviceClassificationEffective'>;
export type DeviceClassificationTarget = Schema<'DeviceClassificationTarget'>;
export type DeviceClassificationTargetDetail = Schema<'DeviceClassificationTargetDetail'>;
export type DeviceClassificationBatch = Schema<'DeviceClassificationBatch'>;
export type DeviceClassificationPreview = Schema<'DeviceClassificationPreview'>;
export type DeviceClassificationResult = Schema<'DeviceClassificationResult'>;
export type DeviceClassificationHistoryRecord = Schema<'DeviceClassificationHistoryRecord'>;
export type ClassificationScope = {kind:'platform'} | {kind:'tenant';tenantId:string} | {kind:'project';tenantId:string;projectId:string};
type Request=<T>(path:string,method?:string,body?:unknown,notify?:boolean,signal?:AbortSignal)=>Promise<T>;
const base=(s:ClassificationScope)=>s.kind==='platform'?'/platform':`/tenants/${encodeURIComponent(s.tenantId)}${s.kind==='project'?`/projects/${encodeURIComponent(s.projectId)}`:''}`;
const dictionary='/business-dictionaries/device-category';
const query=(f:object)=>new URLSearchParams(Object.entries(f).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
export function classificationMethods(request:Request){return {
 deviceCategoryDictionary: (signal?:AbortSignal)=>request<DeviceCategoryDictionary>(`/platform${dictionary}`,'GET',undefined,true,signal),
 saveDeviceCategoryDraft:(body:DeviceCategorySave)=>request<DeviceClassificationResult>(`/platform${dictionary}/draft`,'PUT',body),
 previewDeviceCategoryPublish:(body:DeviceCategoryPublish)=>request<DeviceCategoryPreview>(`/platform${dictionary}/publish-preview`,'POST',body),
 publishDeviceCategories:(body:DeviceCategoryPublish)=>request<DeviceClassificationResult>(`/platform${dictionary}/publish`,'POST',body),
 deviceCategories:(scope:ClassificationScope,filter:{q?:string;cursor?:string;limit?:number;release_id?:string}={},signal?:AbortSignal)=>request<DeviceCategoryPage>(`${base(scope)}${dictionary}/categories?${query(filter)}`,'GET',undefined,true,signal),
 deviceCategoryReleases:(scope:ClassificationScope,filter:{cursor?:string;limit?:number}={},signal?:AbortSignal)=>request<Schema<'DeviceCategoryReleasePage'>>(`${base(scope)}${dictionary}/releases?${query(filter)}`,'GET',undefined,true,signal),
 deviceClassificationTarget:(scope:ClassificationScope,kind:'device'|'product_version',id:string,signal?:AbortSignal)=>request<DeviceClassificationTargetDetail>(`${base(scope)}/device-classifications/${kind}/${encodeURIComponent(id)}`,'GET',undefined,true,signal),
 deviceClassificationTargets:(scope:ClassificationScope,filter:{kind:'device'|'product_version';q?:string;cursor?:string;limit?:number},signal?:AbortSignal)=>request<Schema<'DeviceClassificationTargetPage'>>(`${base(scope)}/device-classifications?${query(filter)}`,'GET',undefined,true,signal),
 deviceClassificationHistory:(scope:ClassificationScope,kind:'device'|'product_version',id:string,filter:{cursor?:string;limit?:number}={},signal?:AbortSignal)=>request<Schema<'DeviceClassificationHistory'>>(`${base(scope)}/device-classifications/${kind}/${encodeURIComponent(id)}/history?${query(filter)}`,'GET',undefined,true,signal),
 previewDeviceClassification:(scope:ClassificationScope,body:DeviceClassificationBatch)=>request<DeviceClassificationPreview>(`${base(scope)}/device-classifications/preview`,'POST',body),
 applyDeviceClassification:(scope:ClassificationScope,body:DeviceClassificationBatch)=>request<DeviceClassificationResult>(`${base(scope)}/device-classifications/apply`,'POST',body),
};}
