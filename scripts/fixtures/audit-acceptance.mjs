import {randomUUID,randomBytes} from 'node:crypto';
import {writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {request} from '@playwright/test';
export const auditOwner={username:'e2e_owner',password:'Real browser 河流 secret 592'};
export async function auditClient(origin,login=auditOwner){
 const client=await request.newContext(),api=origin+'/api/v1';const csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:origin,'X-CSRF-Token':csrf.token};
 const call=async(path,data,method=data===undefined?'GET':'POST',command=randomUUID())=>{const r=await client.fetch(api+path,{method,headers:{...headers,'X-Command-ID':command},data});if(!r.ok())throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?{}:r.json();};
 await call('/auth/login',{username:login.username,password:login.password});return {client,api,headers,call};
}
const reason='审计与操作追踪隔离合成验收';
export async function seedAuditAcceptance(lab){
 if(lab.origin!=='http://127.0.0.1:15576')throw Error('Audit fixture requires its own isolated loopback laboratory');
 const a=await auditClient(lab.origin),{call}=a,state={synthetic:true,origin:lab.origin,accounts:{owner:auditOwner},tenants:[],projects:[],roles:[],members:{},startedAt:new Date().toISOString()};
 const save=()=>{writeFileSync(join(lab.directory,'audit-acceptance.json'),JSON.stringify(state,null,2),{mode:0o600});writeFileSync(join(lab.directory,'accounts.json'),JSON.stringify(state.accounts,null,2),{mode:0o600});};
 try{
 const owner=(await call('/access/accounts?q=e2e_owner')).items[0];state.owner=owner.id;
 for(const n of [1,2]){
  const tenant=(await call('/platform/tenants',{name:`审计合成租户 ${n}`,code:`audit-${n}`,administrator_account_id:owner.id,client_request_id:randomUUID(),reason})).tenant_id;state.tenants.push(tenant);
  const member=(await call(`/tenants/${tenant}/members`)).items.find(m=>m.account.id===owner.id);
  const projects=[];for(const k of [1,2])projects.push((await call(`/tenants/${tenant}/projects`,{name:`审计项目 ${k}`,code:`audit-${n}-${k}`,purpose:'独立合成验收',client_request_id:randomUUID(),reason,owner_tenant_membership_id:member.id,administrator_tenant_membership_id:member.id})).project_id);state.projects.push(projects);
 }
 const profilePath=`/tenants/${state.tenants[0]}/profile`,profile=await call(profilePath);
 await call(profilePath,{expected_version:profile.version,description:'租户成员在当前空间维护的合成资料历史',reason},'PATCH');
 for(const key of ['platform','tenant','project','exporter','manager','business','rejoined']){
  const login={username:'audit_'+key,password:randomBytes(24).toString('base64url')};state.accounts[key]=login;
  const inv=await call('/invitations',{note:reason}),token=new URLSearchParams(new URL(inv.url).hash.slice(1)).get('token'),guest=await request.newContext(),csrf=await(await guest.get(a.api+'/auth/csrf')).json();
  const res=await guest.post(a.api+'/public/invitations/accept',{headers:{Origin:lab.origin,'X-CSRF-Token':csrf.token},data:{token,...login,display_name:`审计验收 ${key}`}});if(res.status()!==204)throw Error('Synthetic account '+key+': '+await res.text());await guest.dispose();
  const account=(await call('/access/accounts?q='+login.username)).items.find(v=>v.username===login.username);login.id=account.id;
  if(key==='platform'){
   let role=await call('/access/roles',{name:'纯平台审计员',description:'无租户成员资格',reason});role=await call('/access/roles/'+role.id+'/permissions',{expected_version:role.version,permission_codes:['platform.audit.read','platform.audit.export','platform.audit.technical.read'],reason},'PUT');
   await call('/access/accounts/'+account.id+'/roles',{expected_version:account.management_version,role_ids:[role.id],reason},'PUT');state.platformRole=role.id;continue;
  }
  const tenant=state.tenants[0],invitation=await call(`/tenants/${tenant}/member-invitations`,{target_account_id:account.id,client_request_id:randomUUID(),reason});
  const c=await auditClient(lab.origin,login);try{await c.call('/me/tenant-invitations/'+invitation.invitation.id+'/accept',{expected_version:invitation.invitation.version});}finally{await c.client.dispose();}
  const member=(await call(`/tenants/${tenant}/members`)).items.find(m=>m.account.id===account.id);state.members[key]=member.id;
  const permissions=key==='business'?['tenant.profile.read']:key==='manager'?['tenant.audit.read','tenant.audit.exports.manage']:key==='project'?['tenant.profile.read']:['tenant.audit.read',...(key==='exporter'?['tenant.audit.export']:[])];
  const role=(await call(`/tenants/${tenant}/roles`,{name:'审计验收 '+key,permission_codes:permissions,client_request_id:randomUUID(),reason})).role;
  const auth=await call(`/tenants/${tenant}/members/${member.id}/authorization`),change={expected_membership_version:auth.membership_version,role_ids:[role.id],reason},p=await call(`/tenants/${tenant}/members/${member.id}/roles/preview`,change);
  await call(`/tenants/${tenant}/members/${member.id}/roles`,{...change,role_versions:p.role_versions,preview_hash:p.preview_hash},'PUT');
  if(key==='rejoined'){
   await call(`/tenants/${tenant}/members/${member.id}/remove`,{expected_version:(await call(`/tenants/${tenant}/members/${member.id}`)).version,reason});
   const again=await call(`/tenants/${tenant}/member-invitations`,{target_account_id:account.id,client_request_id:randomUUID(),reason});const returning=await auditClient(lab.origin,login);try{await returning.call('/me/tenant-invitations/'+again.invitation.id+'/accept',{expected_version:again.invitation.version});}finally{await returning.client.dispose();}
   const fresh=(await call(`/tenants/${tenant}/members`)).items.find(m=>m.account.id===account.id&&m.membership_status==='active');if(!fresh||fresh.id===member.id)throw Error('Rejoin must create a fresh membership');state.rejoinedFrom=member.id;state.members[key]=fresh.id;
   const nextAuth=await call(`/tenants/${tenant}/members/${fresh.id}/authorization`),next={expected_membership_version:nextAuth.membership_version,role_ids:[role.id],reason},nextPreview=await call(`/tenants/${tenant}/members/${fresh.id}/roles/preview`,next);await call(`/tenants/${tenant}/members/${fresh.id}/roles`,{...next,role_versions:nextPreview.role_versions,preview_hash:nextPreview.preview_hash},'PUT');
  }
  if(key==='project'){
   const base=`/tenants/${tenant}/projects/${state.projects[0][0]}`,profile=await call(base+'/profile');
   const added=await call(base+'/members',{client_request_id:randomUUID(),reason,expected_project_version:profile.version,tenant_membership_id:member.id});state.projectMember=added.member_id??added.membership_id;
   const current=await call(base+'/profile');const created=await call(base+'/roles',{client_request_id:randomUUID(),reason,name:'项目审计员',description:'独立项目审计资格',permission_codes:['project.audit.read','project.audit.export'],expected_project_version:current.version});state.projectRole=created.object_id;
   const members=await call(base+'/members'),pm=members.items.find(m=>m.account_id===account.id||m.account?.id===account.id||m.person?.account_id===account.id);if(!pm)throw Error('Project membership fixture missing');state.projectMember=pm.id;
   const detail=await call(base+'/members/'+pm.id+'/authorization'),input={client_request_id:randomUUID(),reason,expected_version:detail.membership_version??pm.version,expected_project_version:(await call(base+'/profile')).version,role_ids:[created.object_id]};const preview=await call(base+'/members/'+pm.id+'/roles/preview',input);
   await call(base+'/members/'+pm.id+'/roles',{...input,role_versions:preview.role_versions,preview_hash:preview.preview_hash},'PUT');
  }
 }
 for(let i=0;i<16;i++)state.roles.push(await call('/access/roles',{name:i<2?'同名审计样本 '+i:i===2?'=危险公式、中文与引号"换行':`审计分页 ${i}`,description:'纯合成资料',reason:i===3?'合'.repeat(500):reason}));
 const archived=`/tenants/${state.tenants[0]}/projects/${state.projects[0][1]}`,preview=await call(archived+'/archive/preview',{});await call(archived+'/archive',{client_request_id:randomUUID(),reason,expected_version:preview.expected_version,expected_project_version:preview.expected_project_version,preview_hash:preview.preview_hash});state.archivedProject=state.projects[0][1];
 const disablePath=`/platform/tenants/${state.tenants[1]}/disable`,disable=await call(disablePath+'/preview',{});await call(disablePath,{expected_version:disable.expected_version,preview_hash:disable.preview_hash,reason:'内部合成维护依据',tenant_notice:'公开合成维护说明'});state.disabledTenant=state.tenants[1];
 state.complete=true;state.completedAt=new Date().toISOString();save();return state;
 }catch(e){save();throw e;}finally{await a.client.dispose();}
}
