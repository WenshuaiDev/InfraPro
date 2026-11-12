import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { ApiError, type DeviceFieldIssue, type DeviceMutationPreview, type DeviceWriteResult } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';
import { requestUUID } from '../tenancy/membership-utils';

type Input = { request_id:string; preview_hash?:string; reason?:string };
// eslint-disable-next-line react-refresh/only-export-components
export function useDeviceCommand<T extends Input>({available,write,preview,receipt,done,refresh}:{available:boolean;write:(input:T)=>Promise<DeviceWriteResult>;preview?:(input:T)=>Promise<DeviceMutationPreview>;receipt:(id:string)=>Promise<DeviceWriteResult>;done:(result:DeviceWriteResult)=>void;refresh?:()=>Promise<void>}){
 const cache=useQueryClient();const [refreshed,setRefreshed]=useState(false);
 const canSubmit=useRef(available);
 useLayoutEffect(()=>{canSubmit.current=available;},[available]);
 const [stage,setStage]=useState<{input:T;preview:DeviceMutationPreview}>();const [uncertain,setUncertain]=useState(false);const [receiptMessage,setReceiptMessage]=useState('');const attempt=useRef<{body:string;id:string}|undefined>(undefined);
 const summary=useRef<HTMLDivElement>(null);const [issues,setIssues]=useState<DeviceFieldIssue[]>([]);
 useEffect(()=>{if(issues.length)summary.current?.focus();},[issues]);
 const failure=(e:Error)=>{setIssues(e instanceof ApiError?e.issues:[]);queueMicrotask(()=>summary.current?.focus());};
 const writeOp=useOperation({mutationFn:(input:T)=>{if(!canSubmit.current)throw new ApiError('当前服务不可用，输入已保留。',503,'submission_unavailable');return write(input);},onSuccess:done,onError:(e)=>{failure(e);if(e instanceof ApiError&&e.code==='submission_unavailable')return;if(!(e instanceof ApiError)||!e.status||e.status>=500)setUncertain(true);if(e instanceof ApiError&&e.status===409)setStage(undefined);}});
 const previewOp=useOperation({mutationFn:async(input:T)=>({input,preview:await preview!(input)}),onSuccess:v=>{if(canSubmit.current&&!v.preview.required&&v.preview.can_submit)writeOp.mutate({...v.input,preview_hash:v.preview.preview_hash});else setStage(v);},onError:failure});
 const receiptOp=useOperation({mutationFn:()=>receipt(attempt.current!.id),onSuccess:done,onError:e=>setReceiptMessage(e instanceof ApiError&&e.status===404?'尚未查到可读取的提交结果。输入和请求编号已保留；请核对当前记录后手动重试。':e.message)});
 const refreshOp=useOperation({mutationFn:async()=>{if(refresh)await refresh();else await cache.invalidateQueries({type:'active'});},onSuccess:()=>{setStage(undefined);writeOp.reset();previewOp.reset();setRefreshed(true);},onError:failure});
 const submit=(input:T)=>{if(!canSubmit.current||uncertain||writeOp.isPending||previewOp.isPending)return;setRefreshed(false);writeOp.reset();previewOp.reset();setIssues([]);setStage(undefined);setReceiptMessage('');const body=JSON.stringify({...input,request_id:'',preview_hash:undefined});if(attempt.current?.body!==body)attempt.current={body,id:requestUUID()};const payload={...input,request_id:attempt.current.id};if(preview)previewOp.mutate(payload);else writeOp.mutate(payload);};
 return {available,submit,stage,refreshed,refresh:()=>refreshOp.mutate(),clear:()=>setStage(undefined),pending:writeOp.isPending||previewOp.isPending||receiptOp.isPending||refreshOp.isPending,uncertain,error:refreshOp.error??writeOp.error??previewOp.error,issues,setIssues,attachIssueSummary:(node:HTMLDivElement|null)=>{summary.current=node;},receiptMessage,check:()=>receiptOp.mutate(),retry:()=>{setUncertain(false);setStage(undefined);writeOp.reset();},confirm:()=>{if(canSubmit.current&&stage&&stage.preview.can_submit&&!uncertain&&!writeOp.isPending)writeOp.mutate({...stage.input,preview_hash:stage.preview.preview_hash});}};
}
export function DeviceIssues({issues}:{issues:DeviceFieldIssue[]}){return issues.length>0&&<ul className="space-y-2 text-sm">{issues.map((v,i)=><li key={`${v.path}:${i}`}><a href={`#${v.path}`} className="underline underline-offset-4" onClick={e=>{e.preventDefault();let path=v.path.replace(/^\/content(?=\/)/,'');while(path&&!document.getElementById(path))path=path.slice(0,path.lastIndexOf('/'));const target=document.getElementById(path);if(target){e.preventDefault();let parent=target.parentElement;while(parent){if(parent instanceof HTMLDetailsElement)parent.open=true;parent=parent.parentElement;}target.scrollIntoView({block:'center'});target.focus();}}}>{v.message}</a></li>)}</ul>;}
export function CommandFeedback<T extends Input>({command,children,disabled=false}:{command:ReturnType<typeof useDeviceCommand<T>>;children?:ReactNode;disabled?:boolean}){
 const {stage,attachIssueSummary}=command;
 disabled=disabled||!command.available;
 return <div className="space-y-4">
 {(command.error||command.issues.length>0)&&<div role="alert" tabIndex={-1} ref={attachIssueSummary} className="space-y-2 rounded-lg border border-destructive/40 bg-destructive/5 p-4"><p className="font-medium">{command.error?.message??'请修正以下问题'}</p><DeviceIssues issues={command.issues}/>{command.error instanceof ApiError&&command.error.status===409&&<div className="space-y-2"><p className="text-sm">当前输入已保留。请重新读取并核对对象，再按最新修订继续。</p><Button type="button" variant="outline" disabled={command.pending||disabled} onClick={command.refresh}>读取最新记录并保留输入</Button></div>}</div>}
 {command.refreshed&&<FormNotice>已读取最新记录，输入已保留。请核对变更并重新预览；授权范围变化时需重新选择当前可用对象。</FormNotice>}
 {stage&&<section className="space-y-3 rounded-lg border bg-muted/30 p-4" aria-label="变更预览"><h3 className="font-semibold">核对本次变更</h3>{children}{stage.preview.shared&&<FormNotice>此变更涉及共享设备或来源。提交后其他获准项目会看到更新。</FormNotice>}<DeviceIssues issues={stage.preview.blockers}/><div className="flex flex-wrap gap-2"><Button type="button" disabled={disabled||command.pending||!stage.preview.can_submit} onClick={command.confirm}>确认提交</Button><Button type="button" variant="outline" disabled={command.pending} onClick={command.clear}>返回修改</Button></div></section>}
 {command.uncertain&&<section role="status" className="space-y-3 rounded-lg border p-4"><p>提交结果尚未确认，输入已保留。请先查验本次结果。</p><div className="flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={command.pending} onClick={command.check}>查验本次提交结果</Button><Button type="button" variant="ghost" disabled={command.pending} onClick={command.retry}>已核对当前记录，允许手动重试</Button></div><p className="text-sm">{command.receiptMessage}</p></section>}
 </div>;
}
