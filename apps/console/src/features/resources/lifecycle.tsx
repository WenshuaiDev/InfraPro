import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, type ResourceDetail, type ResourceMutationPreview, type ResourceTarget } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { TextControl } from '../engineering-fields/controls';
import { useOperation } from '../account/use-operation';
import { requestUUID } from '../tenancy/membership-utils';
import { refreshResources, useResources } from './context';
import { ImpactPreview } from './impact-preview';
export function LifecycleForm({target,resource,structure,action,done}:{target:ResourceTarget;resource:ResourceDetail;structure?:number;action:'retire'|'restore'|'delete';done:()=>void}){
 const {client,scope,prefix,available,writeMeta}=useResources();const cache=useQueryClient();const [reason,setReason]=useState('');const [current,setCurrent]=useState(resource);const [currentStructure,setCurrentStructure]=useState(structure);const [offline,setOffline]=useState(false);const attempt=useRef<{body:string;id:string}>({body:'',id:''});
 const [pending,setPending]=useState<{body:{reason:string;expected_revision:number;request_id:string;expected_structure_revision?:number};preview:ResourceMutationPreview}>();
 const label={retire:'退役',restore:'恢复',delete:'删除'}[action];
 const mutation=useOperation({mutationFn:()=>client.resourceState(scope,target,action,{...pending!.body,preview_hash:pending!.preview.preview_hash}),onSuccess:()=>{void refreshResources(cache,prefix,target.facility??target.id);done();},onError:(e)=>{if(!(e instanceof ApiError)||!e.status||e.status>=500)setOffline(true);if(e instanceof ApiError&&['preview_changed','resource_access_changed'].includes(e.code??''))setPending(undefined);}});
 const preview=useOperation({mutationFn:async()=>{const fields={reason:reason.trim(),expected_revision:current.revision,...writeMeta(current),...(target.kind==='component'?{expected_structure_revision:currentStructure}:{})};const serialized=JSON.stringify(fields);if(attempt.current.body!==serialized)attempt.current={body:serialized,id:requestUUID()};const body={...fields,request_id:attempt.current.id};return {body,preview:await client.resourcePreview(scope,target,action,body)};},onSuccess:setPending});
 const receipt=useOperation({mutationFn:()=>client.resourceReceipt(scope,attempt.current.id),onSuccess:()=>{void refreshResources(cache,prefix,target.facility??target.id);done();}});
 const verify=useOperation({mutationFn:async()=>({resource:await client.resourceDetail(scope,target),facility:target.facility?await client.resourceDetail(scope,{kind:'facility',id:target.facility}):undefined}),onSuccess:(v)=>{setCurrent(v.resource);setCurrentStructure(v.facility?.structure_revision);setOffline(false);setPending(undefined);}});
 return <form className="space-y-4" onSubmit={e=>{e.preventDefault();if(reason.trim()&&reason.length<=2000&&!offline&&available)preview.mutate();}}><p className="break-words">{label}“{current.name}”（{current.code}）</p><p className="text-sm text-muted-foreground">{action==='delete'?'未删除子项和有效项目引用会阻止删除；身份、编号和历史保留。':action==='retire'?'后代资料保留并只读；不会批量改变后代自身状态。':'只恢复当前对象；不会恢复已撤销的项目授权。'}</p><TextControl label="原因 *" value={reason} onChange={v=>{setReason(v);setPending(undefined);}} multiline/><FormNotice error>{reason.length>2000?'原因最多 2000 字符。':preview.error?.message??mutation.error?.message??receipt.error?.message??verify.error?.message}</FormNotice>
 {Object.entries((preview.error instanceof ApiError?preview.error:mutation.error instanceof ApiError?mutation.error:undefined)?.dependencies??{}).map(([kind,count])=><p key={kind} className="text-sm text-destructive">仍有 {count} 个{{components:'构件',points:'测点',channels:'通道'}[kind]??kind}</p>)}
 {pending?<ImpactPreview value={pending.preview} busy={mutation.isPending} confirm={()=>{if(available&&!offline)mutation.mutate();}} cancel={()=>setPending(undefined)}/>:<Button type="submit" disabled={!available||!reason.trim()||reason.length>2000||offline||preview.isPending}>{preview.isPending?'正在预览…':`预览${label}影响`}</Button>}
 {(mutation.error||preview.error)&&<div className="flex flex-wrap gap-3"><Button type="button" variant="outline" disabled={receipt.isPending} onClick={()=>receipt.mutate()}>核对本次提交结果</Button><Button type="button" variant="ghost" disabled={verify.isPending} onClick={()=>verify.mutate()}>重新读取并复核</Button></div>}
 </form>;
}
