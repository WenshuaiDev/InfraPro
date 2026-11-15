import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type AuditScope=S<'AuditScope'>;
export type AuditFilter=S<'AuditFilter'>;
export type AuditEvent=S<'AuditEvent'>;
export type AuditEventDetail=S<'AuditEventDetail'>;
export type AuditCapabilities=S<'AuditCapabilities'>;
export type AuditExport=S<'AuditExport'>;
export type AuditExportInput=S<'AuditExportInput'>;
export type AuditTrace=S<'AuditTracePage'>;
export type AuditTarget=S<'AuditTarget'>;
export type AuditLink=S<'AuditResolvedLink'>;
export type AuditChange=S<'AuditChange'>;
type Request=<T>(path:string,method?:string,body?:unknown,notify?:boolean,signal?:AbortSignal,options?:MonitoringRequestOptions)=>Promise<T>;
const id=encodeURIComponent;
export function auditScopePath(scope:AuditScope){return scope.kind==='platform'?'/audit':`/tenants/${id(scope.tenant_id)}${scope.kind==='project'?`/projects/${id(scope.project_id)}`:''}/audit`;}
const query=(value:object)=>new URLSearchParams(Object.entries(value).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
export function auditMethods(request:Request){return {
 auditCapabilities:(scope:AuditScope,signal?:AbortSignal)=>request<AuditCapabilities>(`${auditScopePath(scope)}/capabilities`,'GET',undefined,true,signal),
 auditEvents:(scope:AuditScope,filter:AuditFilter={},signal?:AbortSignal)=>request<S<'AuditEventPage'>>(`${auditScopePath(scope)}/events?${query(filter)}`,'GET',undefined,true,signal,{timeoutMs:15000}),
 auditEvent:(scope:AuditScope,event:string,signal?:AbortSignal)=>request<AuditEventDetail>(`${auditScopePath(scope)}/events/${id(event)}`,'GET',undefined,true,signal),
 auditTrace:(scope:AuditScope,event:string,cursor?:string,signal?:AbortSignal)=>request<AuditTrace>(`${auditScopePath(scope)}/events/${id(event)}/trace?${query({cursor})}`,'GET',undefined,true,signal),
 auditTarget:(scope:AuditScope,event:string,link:string,signal?:AbortSignal)=>request<AuditTarget>(`${auditScopePath(scope)}/events/${id(event)}/targets/${id(link)}`,'GET',undefined,true,signal),
 auditExports:(scope:AuditScope,filter:S<'AuditExportFilter'> & {mode?:'mine'|'managed'}={},signal?:AbortSignal)=>request<S<'AuditExportPage'>>(`${auditScopePath(scope)}/exports?${query(filter)}`,'GET',undefined,true,signal),
 auditExport:(scope:AuditScope,exportId:string,generation?:string,signal?:AbortSignal)=>request<AuditExport>(`${auditScopePath(scope)}/exports/${id(exportId)}?${query({generation})}`,'GET',undefined,true,signal),
 createAuditExport:(scope:AuditScope,input:AuditExportInput,commandId:string,signal?:AbortSignal)=>request<AuditExport>(`${auditScopePath(scope)}/exports`,'POST',input,true,signal,{headers:{'X-Command-ID':commandId},timeoutMs:20000}),
 downloadAuditExport:(scope:AuditScope,exportId:string,generation:string,signal?:AbortSignal)=>request<Blob>(`${auditScopePath(scope)}/exports/${id(exportId)}/generations/${id(generation)}/download`,'GET',undefined,true,signal,{responseType:'blob',timeoutMs:15*60*1000}),
};}
