import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type FileScope = { tenant: string; project?: string };
export type FileCapabilities = S<'FileCapabilities'>;
export type FileAttachment = S<'FileAttachmentDetail'>;
export type FileRevision = S<'FileAttachmentRevision'>;
export type FileUpload = S<'FileUploadDetail'>;
export type FileJob = S<'FileJobDetail'>;
export type FileCommandResult = S<'FileCommandResult'>;
export type FileStartInput = S<'FileStartUploadInput'>;
export type FileReconfirmInput = S<'FileReconfirmInput'>;
export type FileRevisionCommand = S<'FileRevisionCommand'>;
export type FileAttachmentCommand = S<'FileAttachmentCommand'>;
type Request = <T>(path:string, method?:string, body?:unknown, notify?:boolean, signal?:AbortSignal, options?:MonitoringRequestOptions)=>Promise<T>;
export const filePrefix = (s:FileScope) => `/tenants/${encodeURIComponent(s.tenant)}${s.project?`/projects/${encodeURIComponent(s.project)}`:''}/files`;
export function fileMethods(request:Request, base:string) {
 const id=encodeURIComponent, query=(v:object)=>new URLSearchParams(Object.entries(v).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)])).toString();
 const change=(s:FileScope,path:string,input:unknown,epoch:string,signal?:AbortSignal,method='POST')=>request<FileCommandResult>(`${filePrefix(s)}${path}`,method,input,true,signal,{headers:{'X-Files-Epoch':epoch}});
 return {
 fileCapabilities:(s:FileScope,signal?:AbortSignal)=>request<FileCapabilities>(`${filePrefix(s)}/capabilities`,'GET',undefined,true,signal),
 fileAttachments:(s:FileScope,filter:{state?:'active'|'trashed'|'purged';q?:string;cursor?:string;limit?:number;created_from?:string;created_to?:string}={},signal?:AbortSignal)=>request<S<'FileAttachmentPage'>>(`${filePrefix(s)}/attachments?${query(filter)}`,'GET',undefined,true,signal),
 fileAttachment:(s:FileScope,value:string,signal?:AbortSignal)=>request<FileAttachment>(`${filePrefix(s)}/attachments/${id(value)}`,'GET',undefined,true,signal),
 fileRevisions:(s:FileScope,value:string,cursor='',signal?:AbortSignal)=>request<S<'FileRevisionPage'>>(`${filePrefix(s)}/attachments/${id(value)}/revisions?${query({cursor})}`,'GET',undefined,true,signal),
 fileUploads:(s:FileScope,filter:{view?:'mine'|'managed';state?:string;cursor?:string}={},signal?:AbortSignal)=>request<S<'FileUploadPage'>>(`${filePrefix(s)}/uploads?${query(filter)}`,'GET',undefined,true,signal),
 fileUpload:(s:FileScope,value:string,signal?:AbortSignal)=>request<FileUpload>(`${filePrefix(s)}/uploads/${id(value)}`,'GET',undefined,true,signal),
 fileStartUpload:(s:FileScope,input:FileStartInput,epoch:string,signal?:AbortSignal)=>change(s,'/uploads',input,epoch,signal),
 fileUploadPart:(s:FileScope,value:string,part:number,sha256:string,blob:Blob,epoch:string,signal?:AbortSignal)=>request<S<'FilePartReceipt'>>(`${filePrefix(s)}/uploads/${id(value)}/parts/${part}`,'PUT',undefined,true,signal,{binary:blob,headers:{'X-Files-Epoch':epoch,'X-Content-SHA256':sha256},timeoutMs:120000}),
 fileCompleteUpload:(s:FileScope,value:string,input:FileRevisionCommand,epoch:string,signal?:AbortSignal)=>change(s,`/uploads/${id(value)}/complete`,input,epoch,signal),
 fileCancelUpload:(s:FileScope,value:string,input:FileRevisionCommand,epoch:string,signal?:AbortSignal)=>change(s,`/uploads/${id(value)}/cancel`,input,epoch,signal),
 fileReconfirm:(s:FileScope,value:string,input:FileReconfirmInput,epoch:string,signal?:AbortSignal)=>change(s,`/uploads/${id(value)}/reconfirm`,input,epoch,signal),
 fileChangeAttachment:(s:FileScope,value:string,action:'update'|'trash'|'restore'|'revert',input:FileAttachmentCommand,epoch:string,signal?:AbortSignal)=>change(s,`/attachments/${id(value)}${action==='update'?'':`/${action}`}`,input,epoch,signal,action==='update'?'PATCH':'POST'),
 fileJob:(s:FileScope,value:string,generation?:string,signal?:AbortSignal)=>request<FileJob>(`${filePrefix(s)}/jobs/${id(value)}?${query({generation})}`,'GET',undefined,true,signal),
 fileReceipt:(s:FileScope,value:string,signal?:AbortSignal)=>request<FileCommandResult>(`${filePrefix(s)}/requests/${id(value)}`,'GET',undefined,true,signal),
 fileDownloadURL:(s:FileScope,attachment:string,revision:string)=>`${base}${filePrefix(s)}/attachments/${id(attachment)}/revisions/${id(revision)}/content`,
 fileContent:(s:FileScope,attachment:string,revision:string,signal?:AbortSignal)=>request<Blob>(`${filePrefix(s)}/attachments/${id(attachment)}/revisions/${id(revision)}/content`,'GET',undefined,true,signal,{responseType:'blob',timeoutMs:120000}),
 filePreview:(s:FileScope,attachment:string,revision:string,signal?:AbortSignal)=>request<Blob>(`${filePrefix(s)}/attachments/${id(attachment)}/revisions/${id(revision)}/preview`,'GET',undefined,true,signal,{responseType:'blob'}),
 };
}
