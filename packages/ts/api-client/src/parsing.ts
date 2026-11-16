import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type ParsingScope = {kind:'platform'} | {kind:'tenant';tenantId:string} | {kind:'project';tenantId:string;projectId:string};
export type ParsingMethod=S<'ParsingMethod'>;
export type ParsingConfig=S<'ParsingConfigRevision'>;
export type ParsingConfigInput=S<'ParsingConfigInput'>;
export type ParsingCommand=S<'ParsingCommand'>;
export type ParsingRunInput=S<'ParsingRunInput'>;
export type ParsingRun=S<'ParsingRun'>;
export type ParsingSample=S<'ParsingSample'>;
export type ParsingDeviceState=S<'ParsingDeviceState'>;
export type ParsingResponse=S<'ParsingWireResponse'>;
export type ParsingMapping=S<'ParsingMapping'>;
export type ParsingResult=S<'ParsingResult'>;
export type ParsingDefault=S<'ParsingDefault'>;
type Request = <T>(path:string, method?:string, body?:unknown, notify?:boolean, signal?:AbortSignal, options?:MonitoringRequestOptions)=>Promise<T>;
export const parsingPrefix=(s:ParsingScope)=>s.kind==='platform'?'/platform':`/tenants/${encodeURIComponent(s.tenantId)}${s.kind==='project'?`/projects/${encodeURIComponent(s.projectId)}`:''}`;
export function parsingMethods(request:Request){
 const id=encodeURIComponent, base=(s:ParsingScope)=>`${parsingPrefix(s)}/parsing`, product=(s:ParsingScope,p:string,v:string,publicCatalog=false)=>`${parsingPrefix(s)}/${publicCatalog?'device-catalog':'device-definitions'}/products/${id(p)}/versions/${id(v)}`;
 const q=(v:object)=>new URLSearchParams(Object.entries(v).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
 const change=<T=S<'ParsingResult'>>(path:string,input:unknown,method='POST')=>request<T>(path,method,input);
 return {
 parsingCatalogStatus:(signal?:AbortSignal)=>request<S<'ParsingCatalogStatus'>>('/platform/parsing/catalog-status','GET',undefined,true,signal),
 parsingExecution:(s:ParsingScope,run:string,generation:string,signal?:AbortSignal)=>request<S<'ParsingExecution'>>(`${base(s)}/runs/${id(run)}/executions/${id(generation)}`,'GET',undefined,true,signal),
 parsingMethods:(s:ParsingScope,cursor='',signal?:AbortSignal,filters:{q?:string;state?:string}={})=>request<S<'ParsingMethodPage'>>(`${base(s)}/methods?${q({cursor,limit:100,...filters})}`,'GET',undefined,true,signal),
 parsingSync:(input:S<'ParsingMeta'>)=>change('/platform/parsing/catalog-syncs',input),
 parsingMethodState:(method:string,version:string,input:S<'ParsingStateInput'>)=>change(`/platform/parsing/methods/${id(method)}/versions/${id(version)}/state`,input),
 parsingConfigs:(s:ParsingScope,p:string,v:string,publicCatalog=false,cursor='',signal?:AbortSignal,query='')=>request<S<'ParsingConfigRevisionPage'>>(`${product(s,p,v,publicCatalog)}/parsing-configs?${q({cursor,q:query})}`,'GET',undefined,true,signal),
 parsingSaveConfig:(s:ParsingScope,p:string,v:string,input:ParsingConfigInput,config?:string,device?:string)=>change(device?`${parsingPrefix(s)}/devices/${id(device)}/parsing/configs${config?`/${id(config)}/revisions`:''}`:`${product(s,p,v)}/parsing-configs${config?`/${id(config)}/revisions`:''}`,input),
 parsingConfigState:(s:ParsingScope,p:string,v:string,config:string,input:S<'ParsingStateInput'>)=>change(`${product(s,p,v)}/parsing-configs/${id(config)}/state`,input),
 parsingDefault:(s:ParsingScope,p:string,v:string,publicCatalog=false,signal?:AbortSignal)=>request<ParsingDefault>(`${product(s,p,v,publicCatalog)}/parsing-default`,'GET',undefined,true,signal),
 parsingSetDefault:(s:ParsingScope,p:string,v:string,input:S<'ParsingDefaultInput'>)=>change(`${product(s,p,v)}/parsing-default`,input,'PUT'),
 parsingDevice:(s:ParsingScope,device:string,signal?:AbortSignal)=>request<ParsingDeviceState>(`${parsingPrefix(s)}/devices/${id(device)}/parsing`,'GET',undefined,true,signal),
 parsingDeviceConfigs:(s:ParsingScope,device:string,cursor='',signal?:AbortSignal)=>request<S<'ParsingConfigRevisionPage'>>(`${parsingPrefix(s)}/devices/${id(device)}/parsing/configs?${q({cursor})}`,'GET',undefined,true,signal),
 parsingHistory:(s:ParsingScope,device:string,cursor='',signal?:AbortSignal)=>request<S<'ParsingAdoptionPage'>>(`${parsingPrefix(s)}/devices/${id(device)}/parsing/history?${q({cursor})}`,'GET',undefined,true,signal),
 parsingPreview:(s:ParsingScope,device:string,input:ParsingCommand)=>change<S<'ParsingPreview'>>(`${parsingPrefix(s)}/devices/${id(device)}/parsing/preview`,input),
 parsingAdopt:(s:ParsingScope,device:string,input:ParsingCommand)=>change(`${parsingPrefix(s)}/devices/${id(device)}/parsing/commands`,input),
 parsingBatch:(s:ParsingScope,input:S<'ParsingBatchCommand'>,preview:boolean)=>change<S<'ParsingPreview'>[]|ParsingResult>(`${base(s)}/device-adoptions/${preview?'preview':'commands'}`,input),
 parsingSampleStart:(s:ParsingScope,input:S<'ParsingSampleInput'>)=>change(`${base(s)}/samples`,input),
 parsingSampleUpload:(s:ParsingScope,sample:string,body:Blob,requestID:string)=>request<ParsingResult>(`${base(s)}/samples/${id(sample)}/content`,'PUT',undefined,true,undefined,{binary:body,headers:{'X-Command-ID':requestID}}),
 parsingSampleConfirm:(s:ParsingScope,sample:string,input:S<'ParsingMeta'>)=>change(`${base(s)}/samples/${id(sample)}/confirm`,input),
 parsingSamples:(s:ParsingScope,cursor='',signal?:AbortSignal)=>request<S<'ParsingSamplePage'>>(`${base(s)}/samples?${q({cursor})}`,'GET',undefined,true,signal),
 parsingSampleContent:(s:ParsingScope,sample:string,representation=false,signal?:AbortSignal)=>request<Blob>(`${base(s)}/samples/${id(sample)}/content?${q({representation:representation?'original':undefined})}`,'GET',undefined,true,signal,{responseType:'blob'}),
 parsingRunCreate:(s:ParsingScope,input:ParsingRunInput)=>change(`${base(s)}/runs`,input),
 parsingCompare:(s:ParsingScope,input:S<'ParsingCompareInput'>)=>change<S<'ParsingCompareResult'>>(`${base(s)}/runs/compare`,input),
 parsingRuns:(s:ParsingScope,cursor='',signal?:AbortSignal,filters:{q?:string;state?:string;device_id?:string}={})=>request<S<'ParsingRunPage'>>(`${base(s)}/runs?${q({cursor,...filters})}`,'GET',undefined,true,signal),
 parsingRun:(s:ParsingScope,run:string,signal?:AbortSignal)=>request<S<'ParsingRunDetail'>>(`${base(s)}/runs/${id(run)}`,'GET',undefined,true,signal),
 parsingResult:(s:ParsingScope,run:string,generation:string,offset=0,signal?:AbortSignal)=>request<S<'ParsingResultView'>>(`${base(s)}/runs/${id(run)}/executions/${id(generation)}/result?${q({offset,limit:100})}`,'GET',undefined,true,signal),
 parsingRawResponse:(s:ParsingScope,run:string,generation:string,signal?:AbortSignal)=>request<Blob>(`${base(s)}/runs/${id(run)}/executions/${id(generation)}/response`,'GET',undefined,true,signal,{responseType:'blob'}),
 };
}
