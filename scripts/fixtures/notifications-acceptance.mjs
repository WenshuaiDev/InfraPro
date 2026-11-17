import { requireAuthenticatedLogin } from '../login-result.mjs';
import {randomUUID,createHash} from 'node:crypto';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {request} from '@playwright/test';
export const notificationLogin={username:'e2e_owner',password:'Real browser 河流 secret 592'};
export async function notificationClient(origin,username=notificationLogin.username){
 const client=await request.newContext();const api=origin+'/api/v1',csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:origin,'X-CSRF-Token':csrf.token};
 const call=async(path,data,method=data===undefined?'GET':'POST')=>{const r=await client.fetch(api+path,{method,headers:{...headers,'X-Command-ID':randomUUID()},data});if(!r.ok())throw Error(`${path}: ${r.status()} ${await r.text()}`);return r.status()===204?{}:r.json();};
 requireAuthenticatedLogin(await call('/auth/login',{username,password:notificationLogin.password}));return {client,call,headers,api};
}
export async function queueNotificationImport(connection,state,name,csv){
 const {client,call,headers,api}=connection,base=`/tenants/${state.tenants[0]}/monitoring`,wm=revision=>({request_id:randomUUID(),expected_revision:revision,reason:'站内通知合成验收'}),bytes=Buffer.from(csv),sha256=createHash('sha256').update(bytes).digest('hex');
 let batch=await call(base+'/imports',{...wm('0'),name,size_bytes:String(bytes.length)});
 const part=await client.put(api+base+`/imports/${batch.id}/upload/parts/1`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Expected-Revision':batch.revision,'X-Content-SHA256':sha256},data:bytes});if(!part.ok())throw Error('Synthetic upload failed');
 batch=await call(base+`/imports/${batch.id}/upload/complete`,{...wm(batch.revision),sha256});
 batch=await call(base+`/imports/${batch.id}/mappings`,{...wm(batch.revision),mapping:{format:'csv',mode:'single',encoding:'utf-8',delimiter:',',header_row:1,time_column:'observed_at',value_column:'value',quality_column:'quality',timezone:'UTC',empty_wide:'error',channels:[{alias:'C',channel_id:state.channel,unit:'mm'}]}});
 const task=await call(base+`/imports/${batch.id}/validate`,wm(batch.revision));return {batch:batch.id,job:task.job_id};
}
export async function seedNotificationAcceptance(lab){
 const connection=await notificationClient(lab.origin),{client,call,headers,api}=connection,state={synthetic:true,tenants:[],projects:[],accounts:['e2e_owner','notification_member']};
 const wait=async(fn,predicate)=>{for(let i=0;i<120;i++){const v=await fn();if(predicate(v))return v;await new Promise(r=>setTimeout(r,500));}throw Error('Synthetic result timed out');};
 try{
 const owner=(await call('/access/accounts?q=e2e_owner')).items.find(v=>v.username==='e2e_owner');
 const signup=await call('/invitations',{note:'通知验收第二账号'}),token=new URLSearchParams(new URL(signup.url).hash.slice(1)).get('token');
 const guest=await request.newContext(),guestCSRF=await(await guest.get(api+'/auth/csrf')).json();const accepted=await guest.post(api+'/public/invitations/accept',{headers:{Origin:lab.origin,'X-CSRF-Token':guestCSRF.token},data:{token,username:'notification_member',display_name:'通知验收成员',password:notificationLogin.password}});if(accepted.status()!==204)throw Error('Synthetic second account failed');await guest.dispose();
 const member=(await call('/access/accounts?q=notification_member')).items.find(v=>v.username==='notification_member');
 for(const n of [1,2]){const tenant=(await call('/platform/tenants',{name:`站内通知 · 合成租户 ${n}`,code:`notification-${n}`,administrator_account_id:owner.id,client_request_id:randomUUID(),reason:'通知独立合成验收'})).tenant_id;state.tenants.push(tenant);const tm=(await call(`/tenants/${tenant}/members`)).items.find(v=>v.account.id===owner.id);state.projects.push((await call(`/tenants/${tenant}/projects`,{name:`通知验收项目 ${n}`,code:`notification-${n}`,purpose:'合成验证',client_request_id:randomUUID(),reason:'通知验收',owner_tenant_membership_id:tm.id,administrator_tenant_membership_id:tm.id})).project_id);}
 const invite=await call(`/tenants/${state.tenants[0]}/member-invitations`,{target_account_id:member.id,client_request_id:randomUUID(),reason:'明确接受个人邀请验收'});state.invitation=invite.invitation.id;state.member=member.id;
 const publish=async(kind,code,content)=>{const draft=await call(`/platform/engineering/${kind}`,{request_id:randomUUID(),reason:'通知任务合成定义',code,content:{metamodel_version:'1',description:'合成数据',release_notes:'',...content}});return call(`/platform/engineering/${kind}/${draft.definition_id}/publish`,{request_id:randomUUID(),reason:'通知验收',revision:draft.revision});};
 await call('/platform/engineering/unit-categories',{request_id:randomUUID(),reason:'通知验收',key:'notification_length',name:'通知验收长度',description:'合成'});
 const unit=await publish('units','notify_mm',{name:'通知验收毫米',unit:{symbol:'mm',category:'notification_length'}}),metric=await publish('metrics','notify_value',{name:'通知验收位移',metric:{shape:'decimal',category:'notification_length',standard_unit:unit.version_id,allowed_units:[unit.version_id],options:[]}}),type=await publish('type-definitions','notify_bridge',{name:'通知验收设施',fields:[],components:[],relations:[]});
 const resource=`/tenants/${state.tenants[0]}/engineering/resources/facilities`,facility=await call(resource,{request_id:randomUUID(),code:'NOTIFY',name:'通知验收桥梁',type_version_id:type.version_id,values:{}}),point=await call(resource+`/${facility.id}/points`,{request_id:randomUUID(),code:'POINT',name:'通知验收测点'});state.channel=(await call(resource+`/${facility.id}/points/${point.id}/channels`,{request_id:randomUUID(),code:'CHANNEL',name:'通知验收位移',metric_version_id:metric.version_id,unit_version_id:unit.version_id,timestamp_meaning:'observation_time'})).id;
 state.success=await queueNotificationImport(connection,state,'通知验收成功.csv','observed_at,value,quality\n2024-01-01T00:00:00Z,12,good\n');
 await wait(()=>call(`/tenants/${state.tenants[0]}/monitoring/jobs/${state.success.job}`),v=>v.state==='succeeded');
 const display=`/tenants/${state.tenants[0]}/projects/${state.projects[0]}/displays`,wm=expected_revision=>({request_id:randomUUID(),expected_revision,reason:'通知大屏合成验收'});
 await call(display+'/settings',{...wm('0'),enabled:true},'PUT');const instance=await call(display+'/instances',{...wm('0'),name:'通知验收大屏'});state.instance=instance.id;const root=display+'/instances/'+instance.id;
 if(!existsSync('.artifacts/display/example.zip'))execFileSync('node',['scripts/display-external-check.mjs'],{stdio:'inherit'});
 const upload=async(bytes)=>{const detail=await call(root),sha256=createHash('sha256').update(bytes).digest('hex'),up=await call(root+'/uploads',{...wm(detail.instance.revision),size:bytes.length,sha256});const part=await client.put(api+root+`/uploads/${up.upload_id}/parts/0`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha256},data:bytes});if(!part.ok())throw Error('Display sample upload failed');await call(root+`/uploads/${up.upload_id}/complete`,wm(up.revision));return up.upload_id;};
 state.readyUpload=await upload(readFileSync('.artifacts/display/example.zip'));await wait(()=>call(root),v=>v.packages.length>0);
 state.failedUpload=await upload(Buffer.from('synthetic invalid ZIP; no executable content'));await wait(()=>call(root),v=>v.uploads.some(u=>u.id===state.failedUpload&&u.job.state==='failed'));
 await wait(()=>call('/me/notifications/summary'),v=>v.unread>=6);
 writeFileSync(join(lab.directory,'acceptance.json'),JSON.stringify(state,null,2),{mode:0o600});return state;
 }finally{await client.dispose();}
}
