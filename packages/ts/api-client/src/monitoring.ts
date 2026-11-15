import type { components } from './schema';
type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];
export type MonitoringRange = Schema<'MonitoringRange'>;
export type MonitoringCapability = Schema<'MonitoringCapability'>;
export type MonitoringGrantInput = Schema<'MonitoringGrantInput'>;
export type MonitoringDataGrant = Schema<'MonitoringDataGrant'>;
export type MonitoringGrantResult = Schema<'MonitoringGrantResult'>;
export type MonitoringGrantPage = Schema<'MonitoringGrantPage'>;
export type MonitoringGrantPreview = Schema<'MonitoringGrantPreview'>;
export type MonitoringRevisionInput = Schema<'MonitoringRevisionInput'>;
export type MonitoringRevokeInput = Schema<'MonitoringRevokeInput'>;
export type MonitoringIssue = Schema<'MonitoringIssue'>;
export type MonitoringError = Schema<'MonitoringError'>;
export type MonitoringWriteMeta = Schema<'MonitoringWriteMeta'>;
export type MonitoringImportInput = Schema<'MonitoringImportInput'>;
export type MonitoringUploadComplete = Schema<'MonitoringUploadComplete'>;
export type MonitoringUploadPart = Schema<'MonitoringUploadPart'>;
export type MonitoringChannelMapping = Schema<'MonitoringChannelMapping'>;
export type MonitoringMapping = Schema<'MonitoringMapping'>;
export type MonitoringMappingInput = Schema<'MonitoringMappingInput'>;
export type MonitoringResolveInput = Schema<'MonitoringResolveInput'>;
export type MonitoringImportCounts = Schema<'MonitoringImportCounts'>;
export type MonitoringImportBatch = Schema<'MonitoringImportBatch'>;
export type MonitoringImportPage = Schema<'MonitoringImportPage'>;
export type MonitoringImportFilter = Partial<Schema<'MonitoringImportFilter'>>;
export type MonitoringImportIssue = Schema<'MonitoringImportIssue'>;
export type MonitoringIssuePage = Schema<'MonitoringIssuePage'>;
export type MonitoringJob = Schema<'MonitoringJob'>;
export type MonitoringJobPage = Schema<'MonitoringJobPage'>;
export type MonitoringSelectionInput = Schema<'MonitoringSelectionInput'>;
export type MonitoringSelection = Schema<'MonitoringSelection'>;
export type MonitoringQuery = Omit<Schema<'MonitoringQuery'>, 'limit' | 'digits'> & Partial<Pick<Schema<'MonitoringQuery'>, 'limit' | 'digits'>>;
export type MonitoringSnapshot = Schema<'MonitoringSnapshot'>;
export type MonitoringObservation = Schema<'MonitoringObservation'>;
export type MonitoringDetailPage = Schema<'MonitoringDetailPage'>;
export type MonitoringBatchCoverage = Schema<'MonitoringBatchCoverage'>;
export type MonitoringBatchRevokePreview = Schema<'MonitoringBatchRevokePreview'>;
export type MonitoringBatchRevokeResult = Schema<'MonitoringBatchRevokeResult'>;
export type MonitoringReadoptInput = Schema<'MonitoringReadoptInput'>;
export type MonitoringPreviewSheet = Schema<'MonitoringPreviewSheet'>;
export type MonitoringSourcePreview = Schema<'MonitoringSourcePreview'>;
export type MonitoringOverview = Schema<'MonitoringOverview'>;
export type MonitoringEnumStatistic = Schema<'MonitoringEnumStatistic'>;
export type MonitoringChannelStatistic = Schema<'MonitoringChannelStatistic'>;
export type MonitoringCalendarStatistic = Schema<'MonitoringCalendarStatistic'>;
export type MonitoringTrendBucket = Schema<'MonitoringTrendBucket'>;
export type MonitoringTrendSeries = Schema<'MonitoringTrendSeries'>;
export type MonitoringAnalyticResult = Schema<'MonitoringAnalyticResult'>;
export type MonitoringPoint = Schema<'MonitoringPoint'>;
export type MonitoringChannel = Schema<'MonitoringChannel'>;
export type MonitoringPointPage = Schema<'MonitoringPointPage'>;
export type MonitoringChannelPage = Schema<'MonitoringChannelPage'>;
export type MonitoringExportInput = Omit<Schema<'MonitoringExportInput'>, 'query' | 'csv_mode'> & {query: MonitoringQuery; csv_mode?: 'exact' | 'spreadsheet_safe'};
export type MonitoringExportRecord = Schema<'MonitoringExportRecord'>;
export type MonitoringExportFile = Schema<'MonitoringExportFile'>;
export type MonitoringExportField = Schema<'MonitoringExportField'>;
export type MonitoringExportManifest = Schema<'MonitoringExportManifest'>;
export type MonitoringScope = {kind:'tenant';tenantId:string}|{kind:'project';tenantId:string;projectId:string};
export type MonitoringRequestOptions = { responseType?: 'blob'; binary?: Blob; headers?: Record<string,string>; timeoutMs?: number };
type Request = <T>(path:string,method?:string,body?:unknown,notify?:boolean,signal?:AbortSignal,options?:MonitoringRequestOptions)=>Promise<T>;
const id=encodeURIComponent;
export const monitoringRoot=(scope:MonitoringScope)=>`/tenants/${id(scope.tenantId)}${scope.kind==='project'?`/projects/${id(scope.projectId)}`:''}/monitoring`;
const params=(values:Record<string,string|undefined>)=>new URLSearchParams(Object.entries(values).filter((entry):entry is [string,string]=>!!entry[1])).toString();
export function monitoringMethods(request:Request,base:string){
const get=<T>(path:string,query:Record<string,string|undefined>={},signal?:AbortSignal)=>request<T>(`${path}?${params(query)}`,'GET',undefined,true,signal,{timeoutMs:30000});
const post=<T>(path:string,input:unknown,signal?:AbortSignal)=>request<T>(path,'POST',input,true,signal,{timeoutMs:30000});
return {
monitoringPoints:(scope:MonitoringScope,query:{cursor?:string;search?:string;facility_id?:string;point_id?:string}={},signal?:AbortSignal)=>get<MonitoringPointPage>(`${monitoringRoot(scope)}/points`,query,signal),
monitoringDirectory:(scope:MonitoringScope,query:{cursor?:string;search?:string;facility_id?:string;owner_kind?:'facility'|'component'|'point';owner_id?:string;source_kind?:'direct'|'derived';channel_id?:string}={},signal?:AbortSignal)=>get<MonitoringChannelPage>(`${monitoringRoot(scope)}/channels`,query,signal),
monitoringChannels:(scope:MonitoringScope,point:string,cursor?:string,signal?:AbortSignal)=>get<MonitoringChannelPage>(`${monitoringRoot(scope)}/points/${id(point)}/channels`,{cursor},signal),
monitoringChannelByID:(scope:MonitoringScope,point:string,channel:string,signal?:AbortSignal)=>get<MonitoringChannelPage>(`${monitoringRoot(scope)}/points/${id(point)}/channels`,{channel_id:channel},signal),
monitoringSelect:(scope:MonitoringScope,input:MonitoringSelectionInput,signal?:AbortSignal)=>post<MonitoringSelection>(`${monitoringRoot(scope)}/selections`,input,signal),
monitoringQuery:(scope:MonitoringScope,kind:'overview'|'latest'|'statistics'|'trends',input:MonitoringQuery,signal?:AbortSignal)=>post<MonitoringAnalyticResult>(`${monitoringRoot(scope)}/queries/${kind}`,input,signal),
monitoringDetails:(scope:MonitoringScope,input:MonitoringQuery,signal?:AbortSignal)=>post<MonitoringDetailPage>(`${monitoringRoot(scope)}/queries/details`,input,signal),
monitoringRenew:(scope:MonitoringScope,snapshot:string,input:MonitoringQuery,signal?:AbortSignal)=>post<MonitoringSnapshot>(`${monitoringRoot(scope)}/snapshots/${id(snapshot)}/renew`,input,signal),
monitoringImports:(scope:MonitoringScope,cursor?:string,signal?:AbortSignal)=>get<MonitoringImportPage>(`${monitoringRoot(scope)}/imports`,{cursor},signal),
monitoringImportList:(scope:MonitoringScope,filter:MonitoringImportFilter={},signal?:AbortSignal)=>get<MonitoringImportPage>(`${monitoringRoot(scope)}/imports`,{...filter,limit:filter.limit === undefined ? undefined : String(filter.limit)},signal),
monitoringImport:(scope:MonitoringScope,batch:string,signal?:AbortSignal)=>get<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}`,{},signal),
monitoringCreateImport:(scope:MonitoringScope,input:MonitoringImportInput,signal?:AbortSignal)=>post<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports`,input,signal),
monitoringUploadPart:(scope:MonitoringScope,batch:string,revision:string,part:number,sha256:string,body:Blob,signal?:AbortSignal)=>request<MonitoringUploadPart>(`${monitoringRoot(scope)}/imports/${id(batch)}/upload/parts/${part}`,'PUT',undefined,true,signal,{binary:body,headers:{'X-Expected-Revision':revision,'X-Content-SHA256':sha256},timeoutMs:180000}),
monitoringCompleteUpload:(scope:MonitoringScope,batch:string,input:MonitoringUploadComplete,signal?:AbortSignal)=>request<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}/upload/complete`,'POST',input,true,signal,{timeoutMs:1800000}),
monitoringMapping:(scope:MonitoringScope,batch:string,input:MonitoringMappingInput,signal?:AbortSignal)=>post<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}/mappings`,input,signal),
monitoringImportAction:(scope:MonitoringScope,batch:string,action:'validate'|'publish'|'cancel'|'retry',input:MonitoringWriteMeta,signal?:AbortSignal)=>post<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}/${action}`,input,signal),
monitoringIssues:(scope:MonitoringScope,batch:string,cursor?:string,signal?:AbortSignal)=>get<MonitoringIssuePage>(`${monitoringRoot(scope)}/imports/${id(batch)}/issues`,{cursor},signal),
monitoringResolve:(scope:MonitoringScope,batch:string,input:MonitoringResolveInput,signal?:AbortSignal)=>post<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}/resolutions`,input,signal),
monitoringReadopt:(scope:MonitoringScope,batch:string,input:MonitoringReadoptInput,signal?:AbortSignal)=>post<MonitoringImportBatch>(`${monitoringRoot(scope)}/imports/${id(batch)}/readopt`,input,signal),
monitoringRevokePreview:(scope:MonitoringScope,batch:string,input:MonitoringRevisionInput,signal?:AbortSignal)=>post<MonitoringBatchRevokePreview>(`${monitoringRoot(scope)}/batches/${id(batch)}/revoke-preview`,input,signal),
monitoringRevoke:(scope:MonitoringScope,batch:string,input:MonitoringRevokeInput,signal?:AbortSignal)=>post<MonitoringBatchRevokeResult>(`${monitoringRoot(scope)}/batches/${id(batch)}/revoke`,input,signal),
monitoringJobs:(scope:MonitoringScope,cursor?:string,signal?:AbortSignal)=>get<MonitoringJobPage>(`${monitoringRoot(scope)}/jobs`,{cursor},signal),
monitoringJob:(scope:MonitoringScope,job:string,signal?:AbortSignal)=>get<MonitoringJob>(`${monitoringRoot(scope)}/jobs/${id(job)}`,{},signal),
monitoringCancelJob:(scope:MonitoringScope,job:string,input:MonitoringWriteMeta,signal?:AbortSignal)=>post<MonitoringJob>(`${monitoringRoot(scope)}/jobs/${id(job)}/cancel`,input,signal),
monitoringExport:(scope:MonitoringScope,input:MonitoringExportInput,signal?:AbortSignal)=>post<MonitoringExportRecord>(`${monitoringRoot(scope)}/exports`,input,signal),
monitoringExportFiles:(scope:MonitoringScope,exportId:string,signal?:AbortSignal)=>get<MonitoringExportManifest>(`${monitoringRoot(scope)}/exports/${id(exportId)}/files`,{},signal),
monitoringDownloadURL:(scope:MonitoringScope,exportId:string,part:string)=>`${base}${monitoringRoot(scope)}/exports/${id(exportId)}/files/${id(part)}/download`,
monitoringSourceURL:(scope:MonitoringScope,file:string)=>`${base}${monitoringRoot(scope)}/source-files/${id(file)}/download`,
monitoringGrants:(tenant:string,project:string,cursor?:string,signal?:AbortSignal)=>get<MonitoringGrantPage>(`/tenants/${id(tenant)}/projects/${id(project)}/monitoring-access`,{cursor},signal),
monitoringGrant:(tenant:string,project:string,input:MonitoringGrantInput,signal?:AbortSignal)=>post<MonitoringGrantResult>(`/tenants/${id(tenant)}/projects/${id(project)}/monitoring-access`,input,signal),
monitoringGrantPreview:(tenant:string,project:string,grant:string,input:MonitoringRevisionInput,signal?:AbortSignal)=>post<MonitoringGrantPreview>(`/tenants/${id(tenant)}/projects/${id(project)}/monitoring-access/${id(grant)}/revoke-preview`,input,signal),
monitoringGrantRevoke:(tenant:string,project:string,grant:string,input:MonitoringRevokeInput,signal?:AbortSignal)=>post<MonitoringGrantResult>(`/tenants/${id(tenant)}/projects/${id(project)}/monitoring-access/${id(grant)}/revoke`,input,signal),
};}
