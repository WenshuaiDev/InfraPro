import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type ResourceAccessInput, type ResourceAccessPreview, type ResourceGrant, type ResourceKind } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from '../tenancy/context';
import { useOperation } from '../account/use-operation';
import { requestUUID } from '../tenancy/membership-utils';
import { TextControl } from '../engineering-fields/controls';
import { kinds, capabilities, capNames, type Capability } from './model';
// eslint-disable-next-line react-refresh/only-export-components
export function useResourceAccess(project?:string) {
 const tenant=useTenant();const identity=useIdentity();
 return {...identity,tenant,canRead:[P.TenantProjectRead,P.TenantEngineeringResourceRead,P.TenantEngineeringResourceAuthorizationRead].every(p=>tenant.permissions.includes(p)),canWrite:tenant.available&&[P.TenantEngineeringResourceManage,P.TenantEngineeringResourceAuthorizationManage].every(p=>tenant.permissions.includes(p)),prefix:['tenant',identity.account?.id,tenant.tenant.id,tenant.membership_id,'resource-access',project] as const};
}
export function CapabilityPicker({kind,value,onChange,disabled,original,archived=false}:{kind:ResourceKind;value:Capability[];onChange:(v:Capability[])=>void;disabled?:boolean;original?:Capability[];archived?:boolean}) {
 return <fieldset disabled={disabled} className="flex flex-wrap gap-x-4 gap-y-1"><legend className="sr-only">{kinds[kind]}授权能力</legend>{capabilities[kind].map(c=><label className="flex min-h-11 items-center gap-2 text-sm" key={c}><input type="checkbox" checked={value.includes(c)} disabled={c==='read'||archived&&!original?.includes(c)} onChange={e=>onChange(e.target.checked?[...value,c]:value.filter(v=>v!==c))}/>{capNames[c]}</label>)}</fieldset>;
}
export function GrantSummary({items,label}:{items:ResourceGrant[];label:string}) {return <section className="space-y-2"><h3 className="font-medium">{label}（{items.length}）</h3><ul className="max-h-64 space-y-2 overflow-y-auto">{items.map((v,i)=><li className="break-words text-sm" key={v.id||`${v.kind}:${v.resource_id}:${i}`}>{kinds[v.kind]} · {v.name}（{v.code}）— {v.capabilities.map(c=>capNames[c]).join('、')}</li>)}</ul></section>;}
export function AccessCommit({project,facility,build,disabled,onSaved,draftKey=''}:{project:string;facility?:string;build:(reason:string,request:string)=>ResourceAccessInput;disabled?:boolean;onSaved:()=>void;draftKey?:string}) {
 const {client,tenant,prefix,canWrite}=useResourceAccess(project);const cache=useQueryClient();const [reason,setReason]=useState('');const [staged,setStaged]=useState<{input:ResourceAccessInput;preview:ResourceAccessPreview;key:string}>();
 const saved=()=>{setStaged(undefined);void cache.invalidateQueries({queryKey:prefix});void cache.invalidateQueries({queryKey:['project-context']});onSaved();};
 const preview=useOperation({mutationFn:async()=>{const input=build(reason,requestUUID());return {input,key:draftKey,preview:await client.previewResourceAccess(tenant.tenant.id,project,facility,input)};},onSuccess:setStaged});
 const commit=useOperation({mutationFn:()=>{if(!staged)throw new Error('请重新预览。');return client.changeResourceAccess(tenant.tenant.id,project,facility,{...staged.input,preview_hash:staged.preview.preview_hash});},onSuccess:saved});
 const receipt=useOperation({mutationFn:()=>{if(!staged)throw new Error('尚无提交请求。');return client.resourceAccessReceipt(tenant.tenant.id,project,staged.input.request_id);},onSuccess:saved});
 const current=staged?.key===draftKey?staged:undefined;
 const busy=preview.isPending||commit.isPending||receipt.isPending;
 return <div className="space-y-4"><TextControl label="授权变更原因" value={reason} disabled={busy||!!current} onChange={setReason} multiline/>{(preview.error||commit.error||receipt.error)&&<FormNotice error>{(preview.error||commit.error||receipt.error)?.message} 输入已保留；如修订冲突，请重新载入并复核选择。</FormNotice>}{current?<section aria-label="授权影响确认" className="space-y-4 rounded-lg border p-4"><h2 className="font-semibold">确认授权影响</h2><p className="text-sm">项目：{current.preview.policy.project_name} · {current.preview.policy.project_status==='archived'?'已归档，只可缩减':'运行中'}</p>{current.input.action==='policy'?<p>{current.input.allow_facility_create?'允许新建设施并维护新建资源':'关闭新建设施；保留已有授权'}</p>:<><p>{current.input.action==='create_link'?'新建设施关联并明确授予根设施读取':current.input.action==='revoke_link'?'结束整条设施关联，撤销以下全部授权':'调整当前设施授权'}</p><GrantSummary label="新增" items={current.preview.add}/><GrantSummary label="能力调整" items={current.preview.update}/><GrantSummary label="撤销" items={current.preview.end}/>{current.preview.retained_context.length>0&&<p className="break-words text-sm">仅保留定位摘要：{current.preview.retained_context.map(v=>v.name).join('、')}</p>}<p className="text-sm text-muted-foreground">授权编辑影响同一份公共资源。新增能力包含对应新对象在本项目内的标准维护能力，不会自动授予其他项目。</p></>}<div className="flex flex-wrap gap-3"><Button type="button" disabled={busy||!canWrite} onClick={()=>commit.mutate()}>{commit.isPending?'保存中…':'确认授权变更'}</Button><Button type="button" variant="outline" disabled={busy} onClick={()=>{setStaged(undefined);commit.reset();}}>取消确认，返回修改</Button>{commit.error&&<Button type="button" variant="outline" disabled={busy} onClick={()=>receipt.mutate()}>核对原请求结果</Button>}</div></section>:<Button type="button" disabled={busy||disabled||!canWrite||!reason.trim()} onClick={()=>preview.mutate()}>{preview.isPending?'正在计算影响…':'预览授权变更'}</Button>}</div>;
}
