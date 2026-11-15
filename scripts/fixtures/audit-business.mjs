import { createHash, randomUUID } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { auditClient } from './audit-acceptance.mjs';
import { seedNotificationAcceptance } from './notifications-acceptance.mjs';
import { seedDerivedAcceptance } from './derived-acceptance.mjs';
import { seedSpatialAcceptance } from './spatial-acceptance.mjs';

// Reuse real public-API fixtures, not generated audit rows. Their source workers
// and object stores remain available throughout manual acceptance.
export async function seedAuditBusiness(lab, auditState) {
 const state={synthetic:true,files:[],startedAt:new Date().toISOString()};
 const save=()=>writeFileSync(join(lab.directory,'business-acceptance.json'),JSON.stringify(state,null,2),{mode:0o600});
 state.monitoringDisplay=await seedNotificationAcceptance(lab);save();
 await seedDerivedAcceptance(lab.directory,lab.origin);
 state.derived=JSON.parse(readFileSync(join(lab.directory,'acceptance.json'),'utf8'));
 state.derivedLogin=JSON.parse(readFileSync(join(lab.directory,'login.json'),'utf8'));save();
 await seedSpatialAcceptance(lab.directory,lab.origin);
 state.spatial=JSON.parse(readFileSync(join(lab.directory,'spatial-acceptance.json'),'utf8'));save();
 const a=await auditClient(lab.origin);
 const wait=async fn=>{for(let i=0;i<180;i++){const job=await fn();if(job.state==='succeeded')return job;if(['failed','cancelled','needs_action'].includes(job.state))throw Error('Synthetic attachment failed: '+job.error_code);await new Promise(r=>setTimeout(r,300));}throw Error('Synthetic attachment timed out');};
 try {
  for(const base of [`/tenants/${auditState.tenants[0]}/files`,`/tenants/${auditState.tenants[0]}/projects/${auditState.projects[0][0]}/files`]) {
   const caps=await a.call(base+'/capabilities'),headers={...a.headers,'X-Files-Epoch':caps.restore_epoch};
   const call=async(path,data)=>{const r=await a.client.post(a.api+base+path,{headers,data});if(!r.ok())throw Error(await r.text());return r.json();};
   let attachment;
   for(let n=1;n<=2;n++) {
    const body=Buffer.from(`审计与操作追踪合成附件，第 ${n} 修订。\n无真实业务或个人资料。\n`),sha256=createHash('sha256').update(body).digest('hex');
    const upload=await call('/uploads',{request_id:randomUUID(),reason:'审计来源修订验收',host:caps.host,intent:attachment?'replace':'add',attachment_id:attachment?.id??null,expected_attachment_revision:attachment?.revision??null,filename:'审计合成说明.txt',display_name:'审计修订样本',description:'真实附件检查、绑定与不可变修订',size_bytes:String(body.length),sha256,declared_media_type:'text/plain'});
    const part=await a.client.put(a.api+base+`/uploads/${upload.upload_id}/parts/1`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha256},data:body});if(!part.ok())throw Error(await part.text());
    const current=await a.call(base+'/uploads/'+upload.upload_id),submitted=await call(`/uploads/${upload.upload_id}/complete`,{request_id:randomUUID(),reason:'提交真实附件检查',expected_revision:current.revision});
    const job=await wait(()=>a.call(base+'/jobs/'+submitted.job_id));
    attachment=await a.call(base+'/attachments/'+job.result_attachment_id);
    state.files.push({base,attachment:attachment.id,revision:job.result_revision_id,job:job.id,generation:job.generation});save();
   }
  }
 } finally {await a.client.dispose();}
 state.complete=true;state.completedAt=new Date().toISOString();save();return state;
}
