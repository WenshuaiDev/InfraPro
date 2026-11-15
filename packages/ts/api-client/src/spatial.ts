import type { components } from './schema';
import type { MonitoringScope, MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type SpatialScope=MonitoringScope;
export type SpatialAsset=S<'SpatialAsset'>;
export type SpatialVersion=S<'SpatialVersion'>;
export type SpatialPage=S<'SpatialPage'>;
export type SpatialElement=S<'SpatialElement'>;
export type SpatialPlacement=S<'SpatialPlacement'>;
export type SpatialComponentLink=S<'SpatialComponentLink'>;
export type SpatialFeature=S<'SpatialFeatureView'>;
export type SpatialObject=S<'SpatialObject'>;
export type SpatialObjectRef=S<'SpatialObjectRef'>;
export type SpatialDraft=S<'SpatialDraft'>;
export type SpatialSettings=S<'SpatialSettings'>;
export type SpatialDraftInput=S<'SpatialDraftInput'>;
export type SpatialControlInput=S<'SpatialControlInput'>;
export type SpatialAccessInput=S<'SpatialAccessInput'>;
export type SpatialJob=S<'SpatialJob'>;
export type SpatialPreview=S<'SpatialPreview'>;
export type SpatialGrant=S<'SpatialGrant'>;
export type SpatialSummaries=S<'SpatialSummaries'>;
export type SpatialLocation=S<'SpatialLocation'>;
export type SpatialFilter={owner_id?:string;q?:string;kind?:string;facility_id?:string;state?:string;page_id?:string;element_key?:string;cursor?:string;limit?:number;bbox?:string};
type Request=<T>(path:string,method?:string,body?:unknown,notify?:boolean,signal?:AbortSignal,options?:MonitoringRequestOptions)=>Promise<T>;
const id=encodeURIComponent;
export const spatialRoot=(s:SpatialScope)=>`/tenants/${id(s.tenantId)}${s.kind==='project'?`/projects/${id(s.projectId)}`:''}/spatial`;
const query=(v:SpatialFilter={})=>new URLSearchParams(Object.entries(v).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
export function spatialMethods(request:Request,base=''){
 const get=<T>(s:SpatialScope,path:string,f:SpatialFilter={},signal?:AbortSignal)=>request<T>(`${spatialRoot(s)}${path}?${query(f)}`,'GET',undefined,true,signal,{timeoutMs:30000});
 const write=<T>(s:SpatialScope,path:string,input:unknown,method='POST',signal?:AbortSignal)=>request<T>(`${spatialRoot(s)}${path}`,method,input,true,signal,{timeoutMs:60000});
 return {
 spatialAssets:(s:SpatialScope,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialAssetList'>>(s,'/assets',f,signal),
 spatialAsset:(s:SpatialScope,asset:string,signal?:AbortSignal)=>get<SpatialAsset>(s,`/assets/${id(asset)}`,{},signal),
 spatialSaveAsset:(s:SpatialScope,asset:string|undefined,input:S<'SpatialAssetInput'>)=>write<S<'SpatialResult'>>(s,`/assets${asset?`/${id(asset)}`:''}`,input,asset?'PATCH':'POST'),
 spatialVersions:(s:SpatialScope,asset:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialVersionList'>>(s,`/assets/${id(asset)}/versions`,f,signal),
 spatialCreateVersion:(s:SpatialScope,asset:string,input:S<'SpatialVersionInput'>)=>write<S<'SpatialResult'>>(s,`/assets/${id(asset)}/versions`,input),
 spatialDraft:(s:SpatialScope,v:string,signal?:AbortSignal)=>get<SpatialDraft>(s,`/versions/${id(v)}/draft`,{},signal),
 spatialEditDraft:(s:SpatialScope,v:string,input:SpatialDraftInput)=>write<S<'SpatialResult'>>(s,`/versions/${id(v)}/draft`,input,'PATCH'),
 spatialDeleteDraft:(s:SpatialScope,v:string,input:S<'SpatialRevisionInput'>)=>write<S<'SpatialResult'>>(s,`/versions/${id(v)}/draft`,input,'DELETE'),
 spatialValidate:(s:SpatialScope,v:string,input:S<'SpatialRevisionInput'>)=>write<SpatialDraft>(s,`/versions/${id(v)}/validate`,input),
 spatialPreview:(s:SpatialScope,kind:'assets'|'versions',target:string,action:'publish'|'withdraw'|'current'|'retire',input:SpatialControlInput)=>write<SpatialPreview>(s,`/${kind}/${id(target)}/${action}-preview`,input),
 spatialControl:(s:SpatialScope,kind:'assets'|'versions',target:string,action:'publish'|'withdraw'|'current'|'retire'|'restore',input:SpatialControlInput)=>write<S<'SpatialResult'>>(s,`/${kind}/${id(target)}/${action}`,input),
 spatialHistory:(s:SpatialScope,asset:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialHistoryList'>>(s,`/assets/${id(asset)}/history`,f,signal),
 spatialStartUpload:(s:SpatialScope,input:S<'SpatialUploadInput'>,signal?:AbortSignal)=>write<S<'SpatialResult'>>(s,'/uploads',input,'POST',signal),
 spatialUpload:(s:SpatialScope,upload:string,signal?:AbortSignal)=>get<S<'SpatialUpload'>>(s,`/uploads/${id(upload)}`,{},signal),
 spatialUploadPart:(s:SpatialScope,upload:string,part:number,data:Blob,sha256:string,signal?:AbortSignal)=>request<S<'SpatialResult'>>(`${spatialRoot(s)}/uploads/${id(upload)}/parts/${part}`,'PUT',undefined,true,signal,{binary:data,headers:{'X-Content-SHA256':sha256},timeoutMs:120000}),
 spatialCompleteUpload:(s:SpatialScope,upload:string,input:S<'SpatialRevisionInput'>,signal?:AbortSignal)=>write<S<'SpatialResult'>>(s,`/uploads/${id(upload)}/complete`,input,'POST',signal),
 spatialCancelUpload:(s:SpatialScope,upload:string,input:S<'SpatialRevisionInput'>)=>write<S<'SpatialResult'>>(s,`/uploads/${id(upload)}/cancel`,input),
 spatialProcess:(s:SpatialScope,v:string,input:S<'SpatialRevisionInput'>)=>write<S<'SpatialResult'>>(s,`/versions/${id(v)}/process`,input),
 spatialJob:(s:SpatialScope,job:string,signal?:AbortSignal)=>get<SpatialJob>(s,`/jobs/${id(job)}`,{},signal),
 spatialControlJob:(s:SpatialScope,job:string,action:'cancel'|'retry',input:S<'SpatialRevisionInput'>)=>write<S<'SpatialResult'>>(s,`/jobs/${id(job)}/${action}`,input),
 spatialManifest:(s:SpatialScope,v:string,signal?:AbortSignal)=>get<SpatialVersion>(s,`/versions/${id(v)}/manifest`,{},signal),
 spatialPages:(s:SpatialScope,v:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialPageList'>>(s,`/versions/${id(v)}/pages`,f,signal),
 spatialElements:(s:SpatialScope,v:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialElementList'>>(s,`/versions/${id(v)}/elements`,f,signal),
 spatialPlacements:(s:SpatialScope,v:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialPlacementList'>>(s,`/versions/${id(v)}/placements`,f,signal),
 spatialComponentLinks:(s:SpatialScope,v:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialComponentLinkList'>>(s,`/versions/${id(v)}/component-links`,f,signal),
 spatialFeatures:(s:SpatialScope,v:string,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialFeatureViewList'>>(s,`/versions/${id(v)}/features`,f,signal),
 spatialObject:(s:SpatialScope,o:SpatialObjectRef,signal?:AbortSignal)=>get<SpatialObject>(s,`/objects/${id(o.id)}`,{kind:o.kind},signal),
 spatialObjects:(s:SpatialScope,f?:SpatialFilter,signal?:AbortSignal)=>get<S<'SpatialObjectList'>>(s,'/objects',f,signal),
 spatialBasemaps:(s:SpatialScope,signal?:AbortSignal)=>get<S<'SpatialBasemapList'>>(s,'/basemaps',{},signal),
 spatialSummaries:(s:SpatialScope,objects:SpatialObjectRef[],signal?:AbortSignal,mainChannels?:Record<string,string>)=>write<SpatialSummaries>(s,'/summaries',{objects,main_channels:mainChannels},'POST',signal),
 spatialLocate:(s:SpatialScope,objects:SpatialObjectRef[],signal?:AbortSignal)=>write<S<'SpatialLocationList'>>(s,'/locate',{objects},'POST',signal),
 spatialRelated:(s:SpatialScope,input:S<'SpatialRelatedInput'>,signal?:AbortSignal)=>write<S<'AnalysisArtifactPage'>>(s,'/related-artifacts',input,'POST',signal),
 spatialFile:(s:SpatialScope,v:string,file:string,signal?:AbortSignal)=>request<Blob>(`${spatialRoot(s)}/versions/${id(v)}/${file==='source'?'source':`files/${id(file)}`}`,'GET',undefined,true,signal,{responseType:'blob',timeoutMs:120000}),
 spatialFileURL:(s:SpatialScope,v:string,file:string)=>`${base}${spatialRoot(s)}/versions/${id(v)}/${file==='source'?'source':`files/${id(file)}`}`,
 spatialView:(s:SpatialScope,view:string,signal?:AbortSignal)=>get<S<'SpatialView'>>(s,`/views/${id(view)}`,{},signal),
 spatialSaveView:(s:SpatialScope,view:string,input:S<'SpatialViewInput'>)=>write<S<'SpatialResult'>>(s,`/views/${id(view)}`,input,'PUT'),
 spatialAccess:(tenant:string,project:string,signal?:AbortSignal)=>request<S<'SpatialAccessDetail'>>(`/tenants/${id(tenant)}/projects/${id(project)}/spatial-access`,'GET',undefined,true,signal),
 spatialAccessPreview:(tenant:string,project:string,input:SpatialAccessInput)=>request<SpatialPreview>(`/tenants/${id(tenant)}/projects/${id(project)}/spatial-access/preview`,'POST',input),
 spatialAccessSave:(tenant:string,project:string,input:SpatialAccessInput)=>request<S<'SpatialResult'>>(`/tenants/${id(tenant)}/projects/${id(project)}/spatial-access`,'PUT',input),
 };
}
