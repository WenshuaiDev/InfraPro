import { requireAuthenticatedLogin } from '../login-result.mjs';
import { randomBytes, randomUUID, createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { request } from '@playwright/test';
import { seedNotificationAcceptance, notificationClient, notificationLogin, queueNotificationImport } from './notifications-acceptance.mjs';
import { seedDerivedAcceptance } from './derived-acceptance.mjs';
import { seedSpatialAcceptance } from './spatial-acceptance.mjs';

export async function taskClient(origin, login) {
 const client=await request.newContext(),api=origin+'/api/v1';
 const csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:origin,'X-CSRF-Token':csrf.token};
 const call=async(path,data,method=data===undefined?'GET':'POST')=>{const r=await client.fetch(api+path,{method,headers:{...headers,'X-Command-ID':randomUUID()},data});if(!r.ok())throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?{}:r.json();};
 requireAuthenticatedLogin(await call('/auth/login',{username:login.username,password:login.password}));return {client,call,headers,api};
}
const wm=(expected_revision='0')=>({request_id:randomUUID(),expected_revision,reason:'统一任务中心独立合成验收'});
async function wait(fn,predicate,label='task') {const end=Date.now()+120000;while(Date.now()<end){const value=await fn();if(predicate(value))return value;await new Promise(r=>setTimeout(r,100));}throw Error(`Synthetic ${label} timed out`);}
// Refreshes the real short-lived personal query after role/catalog fixture
// changes; never edits a snapshot's authorization vector or lifetime.
export async function refreshTaskQuery(lab,state) {
 const a=await notificationClient(lab.origin);try {
  const base=`/tenants/${state.owner.tenants[0]}/monitoring`;
  const selection=await a.call(base+'/selections',{channel_ids:[state.owner.channel]}),query={selection_id:selection.id,range:{from:'2024-01-01T00:00:00Z',to:'2024-01-05T00:00:00Z'}};
  let queued;
  for(let n=0;n<4&&!queued?.job_id;n++) {
   const fraction=String(parseInt(randomBytes(3).toString('hex'),16)).padStart(9,'0');
   const imported=await queueNotificationImport(a,state.owner,'固定查询合成验收.csv',`observed_at,value,quality\n2024-01-04T00:00:00.${fraction}Z,17,good\n`);
   const ready=await wait(()=>a.call(base+'/imports/'+imported.batch),v=>v.state==='ready','fresh query input');
   await a.call(base+`/imports/${ready.id}/publish`,wm(ready.revision));await wait(()=>a.call(base+'/imports/'+ready.id),v=>['published','completed_noop'].includes(v.state),'fresh publication');
   queued=await a.call(base+'/queries/overview',query);
  }
  if(!queued?.job_id)throw Error('Fresh exact-query fixture did not enqueue');
  await wait(()=>a.call(base+'/jobs/'+queued.job_id),v=>v.state==='succeeded','fresh exact query');
  const exp=await a.call(base+'/exports',{...wm(),kind:'details',format:'csv',csv_mode:'exact',query});await wait(()=>a.call(base+'/jobs/'+exp.job_id),v=>v.state==='succeeded','fresh export');
  state.backgroundQuery=queued.job_id;state.export=exp;
  return queued.job_id;
 }finally{await a.client.dispose();}
}
export async function seedTaskAcceptance(lab) {
 if(!['http://127.0.0.1:15573','http://127.0.0.1:15773'].includes(lab.origin))throw Error('Task fixture requires its dedicated isolated loopback laboratory');
 const file=name=>join(lab.directory,name),save=(name,value)=>writeFileSync(file(name),JSON.stringify(value,null,2),{mode:0o600});
 const state={accounts:{A:notificationLogin},synthetic:true,startedAt:new Date().toISOString(),origin:lab.origin,tasks:[],checks:[]};
 const notices=await seedNotificationAcceptance(lab);save('notification-acceptance.json',notices);state.owner=notices;
 const owner=await notificationClient(lab.origin);
 try {
  const {call}=owner,base=`/tenants/${notices.tenants[0]}/monitoring`;
  const selection=await call(base+'/selections',{channel_ids:[notices.channel]});
  const query={selection_id:selection.id,range:{from:'2024-01-01T00:00:00Z',to:'2024-01-05T00:00:00Z'}};
  // Request the existing exact-query path immediately after real publication,
  // before the background summary pass has built this content revision.
  let queued;
  for(let n=0;n<4&&!queued?.job_id;n++) {
   const imported=n===0?notices.success:await queueNotificationImport(owner,notices,`后台查询验收-${n}.csv`,`observed_at,value,quality\n2024-01-0${n+1}T00:00:00Z,${12+n},good\n`);
   const ready=await wait(()=>call(base+'/imports/'+imported.batch),v=>v.state==='ready','validation');
   await call(base+`/imports/${ready.id}/publish`,wm(ready.revision));await wait(()=>call(base+'/imports/'+ready.id),v=>['published','completed_noop'].includes(v.state),'adoption');
   queued=await call(base+'/queries/overview',query);
  }
  if(!queued?.job_id)throw Error('Exact-query fixture did not produce a real background task');
  await wait(()=>call(base+'/jobs/'+queued.job_id),v=>v.state==='succeeded','exact query');state.backgroundQuery=queued.job_id;
  const exp=await call(base+'/exports',{...wm(),kind:'details',format:'csv',csv_mode:'exact',query});await wait(()=>call(base+'/jobs/'+exp.job_id),v=>v.state==='succeeded','ordinary export');state.export=exp;
  // Same account owns real tasks in a second tenant/project.
  const display=`/tenants/${notices.tenants[1]}/projects/${notices.projects[1]}/displays`;
  await call(display+'/settings',{...wm(),enabled:true},'PUT');const instance=await call(display+'/instances',{...wm(),name:'跨空间任务合成大屏'});
  const bytes=readFileSync('.artifacts/display/example.zip'),sha256=createHash('sha256').update(bytes).digest('hex');
  const root=display+'/instances/'+instance.id,up=await call(root+'/uploads',{...wm(instance.revision),size:bytes.length,sha256});
  const part=await owner.client.put(owner.api+root+`/uploads/${up.upload_id}/parts/0`,{headers:{...owner.headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha256},data:bytes});if(!part.ok())throw Error('Cross-scope package part failed');
  await call(root+`/uploads/${up.upload_id}/complete`,wm(up.revision));await wait(()=>call(root),v=>v.packages.length>0,'second-tenant package');
  state.checks.push('same account: two tenants and project task scopes, exact query, validate/adopt, export');
 }finally{await owner.client.dispose();}
 // M receives only a task summary/control role in A's first tenant.
 const a=await notificationClient(lab.origin);let manager;
 try {
  const invite=await a.call('/invitations',{note:'任务中心仅部分管理权限账号'}),token=new URLSearchParams(new URL(invite.url).hash.slice(1)).get('token');
  manager={username:'task_manager',display_name:'任务管理员 M',password:randomBytes(24).toString('base64url')};
  const guest=await request.newContext(),csrf=await(await guest.get(a.api+'/auth/csrf')).json();
  const accepted=await guest.post(a.api+'/public/invitations/accept',{headers:{Origin:lab.origin,'X-CSRF-Token':csrf.token},data:{token,...manager}});if(accepted.status()!==204)throw Error('Manager account creation failed');await guest.dispose();
  const account=(await a.call('/access/accounts?q='+manager.username)).items.find(v=>v.username===manager.username);
  const tenant=notices.tenants[0],inv=await a.call(`/tenants/${tenant}/member-invitations`,{target_account_id:account.id,client_request_id:randomUUID(),reason:'任务中心部分管理权限验收'});
  const m=await taskClient(lab.origin,manager);let membership;try{membership=await m.call('/me/tenant-invitations/'+inv.invitation.id+'/accept',{expected_version:inv.invitation.version});}finally{await m.client.dispose();}
  const memberID=membership.membership_id??(await a.call(`/tenants/${tenant}/members`)).items.find(v=>v.account.id===account.id).id;
  const role=await a.call(`/tenants/${tenant}/roles`,{name:'仅监测任务管理',permission_codes:['tenant.monitoring.jobs.manage'],client_request_id:randomUUID(),reason:'摘要管理与结果读取分离'});
  const auth=await a.call(`/tenants/${tenant}/members/${memberID}/authorization`),change={expected_membership_version:auth.membership_version,role_ids:[role.role.id],reason:'赋予最小任务管理资格'};
  const p=await a.call(`/tenants/${tenant}/members/${memberID}/roles/preview`,change);
  await a.call(`/tenants/${tenant}/members/${memberID}/roles`,{...change,role_versions:p.role_versions,preview_hash:p.preview_hash},'PUT');
  state.manager={tenant,memberID,accountID:account.id,role:role.role.id};state.accounts.M=manager;
 }finally{await a.client.dispose();}
 await seedDerivedAcceptance(lab.directory,lab.origin);
 const derived=JSON.parse(readFileSync(file('acceptance.json'),'utf8')),login=JSON.parse(readFileSync(file('login.json'),'utf8'));
 save('derived-acceptance.json',derived);state.derived=derived;state.accounts.B=login;
 await seedSpatialAcceptance(lab.directory,lab.origin);state.spatial=JSON.parse(readFileSync(file('spatial-acceptance.json'),'utf8'));
 const b=await taskClient(lab.origin,login);
 try {
  const {call}=b,base=`/tenants/${derived.tenant}/monitoring/derived`,selected=derived.channels.A_MINUS_B;
  const channel=await call(base+'/channels/'+selected.id);
  const preview=await call(base+`/channels/${selected.id}/backfill-preview`,{expected_revision:channel.config_revision,output_range:derived.range});
  const backfill=await call(base+`/channels/${selected.id}/backfill`,{...wm(channel.config_revision),preview_id:preview.id,preview_hash:preview.hash});
  for(const run of backfill.runs)await wait(()=>call(base+'/runs/'+run.id),v=>v.job.state==='succeeded','backfill');
  const replay=await call(base+`/generations/${selected.generation}/replay`,wm());await wait(()=>call(base+'/runs/'+replay.id),v=>v.job.state==='succeeded','replay');
  const cfg=(await call(base+`/channels/${selected.id}/configs`)).items[0];
  const created=await call(base+'/analysis/configs',{...wm(),code:'TASK_ANALYSIS',name:'任务中心固定分析',body:{rule:cfg.rule,input_bindings:cfg.input_bindings,parameters:cfg.parameters}});
  const validation=await call(base+`/analysis/configs/${created.config.id}/validate`,{draft_revision:created.config.draft.revision});
  const published=await call(base+`/analysis/configs/${created.config.id}/versions`,{...wm(created.config.revision),draft_revision:created.config.draft.revision,validation_hash:validation.validation_hash});
  const input={config_version_id:published.version.id,config_hash:published.version.config_hash,output_range:derived.range};
  const trial=await call(base+'/analysis/trials',{...wm(),...input});await wait(()=>call(base+'/runs/'+trial.id),v=>v.job.state==='succeeded','analysis trial');
  const first=await call(base+'/analysis/runs',{...wm(),...input});await wait(()=>call(base+'/runs/'+first.id),v=>v.job.state==='succeeded','analysis run');
  const second=await call(base+'/analysis/runs',{...wm(),...input,artifact_id:first.analysis.artifact_id});await wait(()=>call(base+'/runs/'+second.id),v=>v.job.state==='succeeded','analysis next version');
  const comparison=await call(base+'/analysis/comparisons',{...wm(),left_version_id:first.analysis.artifact_version_id,right_version_id:second.analysis.artifact_version_id});await wait(()=>call(base+'/runs/'+comparison.id),v=>v.job.state==='succeeded','comparison');
  const artifactExport=await call(base+`/analysis/artifacts/${first.analysis.artifact_id}/exports`,{...wm(),version_id:first.analysis.artifact_version_id,format:'csv',csv_mode:'exact',history:false});
  const evidenceExport=await call(base+`/generations/${selected.generation}/exports`,{...wm(),format:'csv',csv_mode:'exact'});
  for(const out of [artifactExport,evidenceExport])await wait(()=>call(`/tenants/${derived.tenant}/monitoring/jobs/${out.job.id}`),v=>v.state==='succeeded','derived export');
  state.analysis={trial,first,second,comparison,artifactExport,evidenceExport};state.checks.push('all derived and analysis real source types');
 }finally{await b.client.dispose();}
 await refreshTaskQuery(lab,state);
 save('task-acceptance.partial.json',state);
 // Verify every registered executable type through current public authorization.
 const catalog=new Set();
 for(const key of ['A','B']){const c=await taskClient(lab.origin,state.accounts[key]);try{let cursor;do{const page=await wait(()=>c.call('/tasks?limit=100'+(cursor?'&cursor='+encodeURIComponent(cursor):'')),v=>v.coverage==='complete','index coverage');for(const item of page.items){catalog.add(item.type);state.tasks.push({account:key,...item});}cursor=page.next_cursor;}while(cursor);}finally{await c.client.dispose();}}
 const expected=['monitoring.import.validate','monitoring.import.adopt','monitoring.export','monitoring.query','derived.trial','derived.backfill','derived.replay','analysis.trial','analysis.run','analysis.compare','analysis.export','derived.evidence.export','spatial.process','display.package'];
 const missing=expected.filter(type=>!catalog.has(type));if(missing.length)throw Error('Real acceptance sources missing: '+missing.join(', '));
 state.completedAt=new Date().toISOString();state.complete=true;state.sourceTypes=[...catalog].sort();save('task-acceptance.json',state);save('accounts.json',state.accounts);
 console.log(`Task acceptance: ${lab.origin}/tasks; ${state.tasks.length} real tasks; ${expected.length} executable types. Synthetic accounts are stored locally in ${file('accounts.json')}`);
 return state;
}
