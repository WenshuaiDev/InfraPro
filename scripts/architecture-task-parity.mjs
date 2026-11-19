// One published monitoring batch has a durable task, inbox, audit and authorized
// deep link on both releases. Compare those facts, including per-recipient IDs.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { client,digest,queryPG,read,runDirectory,safeDiagnosticLine,sleep,write } from './architecture-lab.mjs';
import { operation } from './architecture-workflow-http.mjs';
import { startPairedWorkers,withHTTPPair } from './architecture-http-sandbox.mjs';

async function inspect(dir,lab,side,expected,command,calls) {
  const cfg=lab[side],accounts=read(join(dir,'private/accounts.json'));
  const admin=await client(cfg,accounts.admin),outsider=await client(cfg,accounts.other);
  async function request(principal,method,path,input){
    const op=principal==='owner'?admin:outsider, started=Date.now();
    const r=await op.client.fetch(op.api+path,{method,headers:{...op.headers,'X-Command-ID':command},data:input});
    try {
      const v=await r.json().catch(()=>null),status=r.status();
      calls.push({
        principal,method,path:path.split('?')[0],status,error:v?.code||null,
        startedAt:new Date(started).toISOString(),durationMs:Date.now()-started,
        bodySHA256:digest(await r.body()),requestID:r.headers()['x-request-id'],
      });
      return {status,v};
    } finally { await r.dispose(); }
  }
  async function allow(method,path,input){const result=await request('owner',method,path,input);assert(result.status>=200 && result.status<300,path+' returned '+result.status);return result.v;}
  const taskID=expected.task,job=expected.job,batch=expected.batch;
  const db=new URL(read(cfg.config).database_url);assert.equal(db.hostname,'127.0.0.1');
  assert.match(db.pathname,/^\/arc_http_[0-9a-f]{12}_[ab]$/);
  const sql=q=>execFileSync('docker',['exec','-i',lab.container,'psql','-XqAt','-v','ON_ERROR_STOP=1','-U','archtest','-d',db.pathname.slice(1)],{encoding:'utf8',input:q}).trim();
  try{
    const task=await allow('GET','/tasks/'+taskID);
    assert.equal(task.state,'succeeded');assert.equal(task.generation,'1');
    assert.equal(task.source_ref.source_root_id,job);
    const executable=await allow('GET','/tasks/'+taskID+'/executions');
    assert.equal(executable.items.length,1);
    const events=await allow('GET','/tasks/'+taskID+'/events?generation=1');
    const target=await allow('GET','/tasks/'+taskID+'/target?generation=1');
    assert.equal(target.available,true);
    const batchResource='/tenants/'+expected.tenant+'/monitoring/imports/'+batch;
    assert.equal((await allow('GET',batchResource)).state,'published');
    const projection=JSON.parse(sql("SELECT to_jsonb(x) FROM (SELECT id,source_root_id,current_generation,source_revision,state,originator_account_id FROM task_records WHERE id='"+taskID+"')x"));
    assert.equal(projection.originator_account_id,expected.actor);
    const notificationQuery="SELECT n.id FROM account_notifications n JOIN notification_events e ON e.id=n.event_id WHERE n.account_id='"+expected.actor+"' AND e.source_module='monitoring' AND e.event_type='task.result' AND e.outcome='adopted' AND e.target_id='"+batch+"'";
    let id='';const end=Date.now()+90000;
    do{id=sql(notificationQuery);if(id)break;await sleep(300);}while(Date.now()<end);
    assert.match(id,/^[0-9a-f-]{36}$/,'An adopted source must deliver an inbox item to the originator');
    let notice=await allow('GET','/me/notifications/'+id);
    const destination=await allow('GET','/me/notifications/'+id+'/target');
    assert.equal(destination.status,'available');assert(destination.id);
    const pref=await allow('GET','/me/notification-preferences');
    const toggled=await allow('PUT','/me/notification-preferences',{tasks:!pref.tasks,updates:pref.updates,revision:pref.revision});
    assert.equal(toggled.tasks,!pref.tasks);
    const restored=await allow('PUT','/me/notification-preferences',{tasks:pref.tasks,updates:pref.updates,revision:toggled.revision});
    assert.equal(restored.tasks,pref.tasks);
    if(notice.read_at)notice=await allow('PATCH','/me/notifications/'+id,{expected_revision:notice.revision,read:false});
    const before=await allow('GET','/me/notifications/summary');
    const changed=await allow('PATCH','/me/notifications/'+id,{expected_revision:notice.revision,read:true});
    assert(changed.read_at);const after=await allow('GET','/me/notifications/summary');
    assert.equal(after.unread,before.unread-1);
    const again=await request('owner','PATCH','/me/notifications/'+id,{expected_revision:notice.revision,read:true});
    const inboxFilter={category:'tasks',tenant_id:expected.tenant};
    const current=await allow('GET','/me/notifications/'+id);
    await allow('PATCH','/me/notifications/'+id,{expected_revision:current.revision,read:false});
    const otherInbox=()=>sql("SELECT md5(coalesce(string_agg(id::text||coalesce(read_at::text,'unread'),',' ORDER BY id),'')) FROM account_notifications WHERE account_id='"+expected.other+"'");
    const retained=otherInbox(),list=await allow('GET','/me/notifications?status=unread&category=tasks&tenant_id='+expected.tenant+'&limit=100');
    assert(list.items.some(item=>item.id===id));assert(!list.next_cursor);
    const bulkInput={filter:inboxFilter,watermark:list.watermark};
    const bulk=await allow('POST','/me/notifications/read-all',bulkInput);
    assert.equal(bulk.changed,list.items.length);assert(bulk.changed>0);
    assert.equal((await allow('POST','/me/notifications/read-all',bulkInput)).changed,0);
    assert((await allow('GET','/me/notifications/'+id)).read_at);assert.equal(otherInbox(),retained);
    assert([200,409].includes(again.status));
    const results={};
    for(const [name,path] of Object.entries({task:'/tasks/'+taskID,inbox:'/me/notifications/'+id,noticeTarget:'/me/notifications/'+id+'/target',batch:batchResource})){
      const r=await request('other-tenant','GET',path);assert([403,404].includes(r.status),name+' returned '+r.status+' '+(r.v?.code||''));results[name]={status:r.status,code:r.v?.code};
    }
    const audit='/tenants/'+expected.tenant+'/audit/events',filter='?task_module=monitoring&task_kind=job&task_id='+job+'&generation=1&limit=100';
    const related=await allow('GET',audit+filter);
    assert(related.items.length>0,'No audit item links the task execution');
    const detail=await allow('GET',audit+'/'+related.items[0].id);assert(detail);
    const trace=await allow('GET',audit+'/'+related.items[0].id+'/trace');assert(trace);
    const deniedAudit=await request('other-tenant','GET',audit+'/'+related.items[0].id);assert([403,404].includes(deniedAudit.status));
    const forbidden=await request('owner','POST','/tasks/'+taskID+'/commands',{request_id:command,action:'retry',expected_generation:task.generation,expected_control_revision:task.control_revision,reason:'验证终态任务不会重试'});
    assert([400,403,409,422].includes(forbidden.status),'Completed job accepted retry');
    assert.equal(sql(notificationQuery).split('\n').length,1);
    return {calls,projection,task:{type:task.type,state:task.state,generation:task.generation,actions:task.actions,target,events:events.items?.length,executions:executable.items.map(v=>v.state)},
      notification:{eventType:notice.type,category:notice.category,target:destination.kind,read:changed.read_at!==null,readDelta:after.unread-before.unread,recipient:expected.actor,retryStatus:again.status,bulkChanged:bulk.changed,bulkRetryChanged:0,otherRecipientUnchanged:true},
      preferences:{initial:pref.tasks,changed:toggled.tasks,restored:restored.tasks},
      audit:related.items.map(v=>({eventType:v.event_type,outcome:v.outcome,actor:v.actor,object:v.object,changes:v.changes,scope:v.scope})),
      rejectedRetry:{status:forbidden.status,code:forbidden.v?.code},crossTenant:results,auditDenial:deniedAudit.status};
  }finally{await admin.client.dispose();await outsider.client.dispose();}
}
export function recordTaskOperations(proof,output) {
  const rows=[],b=proof.paired?.b?.calls||[];
  for(const [index,left] of (proof.paired?.a?.calls||[]).entries()){
    const right=b[index],op=operation(left.method,'/api/v1'+left.path);
    if(!op||!right)continue;
    const other=operation(right.method,'/api/v1'+right.path);
    rows.push({scenario:proof.name,principal:left.principal,method:op.method,template:op.template,owner:op.owner,operationId:op.operationId,
      status:[left.status,right.status],errors:[left.error,right.error],body_sha256:[left.bodySHA256,right.bodySHA256],
      differences:proof.passed&&op.template===other?.template&&left.method===right.method&&left.principal===right.principal&&left.error===right.error&&left.status===right.status?[]:['paired assertion failed']});
  }
  write(join(output,'http/scenarios/task-notifications.json'),{scenario:'durable task, notification and audit',records:rows});
}
export async function taskParity(dir,output=dir) {
  const lab=read(join(dir,'private/lab.json')),s=read(join(dir,'samples.json'));
  const [task,job]=queryPG(lab,'seed',"SELECT id::text||'|'||source_root_id::text FROM task_records WHERE task_type='monitoring.import.adopt' ORDER BY created_at LIMIT 1").split('|');
  const expected={task,job,batch:s.structure.batch,tenant:s.structure.tenant,actor:s.actors.admin,other:s.actors.other};
  const command=randomUUID(),paired={};
  const proof={arc:'ARC-014',name:'published import and task, notification, audit, recipient and target',passed:false,source:expected,command,checks:[]};
  const configure=async({side,privateDir})=>({start:options=>startPairedWorkers({...options,side,privateDir,claimRoles:[],legacy:['notification-worker','task-index-worker'],split:['notification-worker','task-index-worker']})});
  return withHTTPPair(dir,'task-notification',async sandbox=>{
   try{
    const fixture=read(join(sandbox,'private/lab.json'));proof.sandbox=sandbox;
    for (const side of ['a','b']) {
      const calls=[];
      try {
        paired[side]=await inspect(sandbox,fixture,side,expected,command,calls);
      } catch (error) {
        paired[side]={calls,failure:safeDiagnosticLine(error.message).slice(0,400)};
      }
    }
    for (const [side,result] of Object.entries(paired))
      assert(!result.failure,side+' '+result.failure);
    for(const key of Object.keys(paired.a).filter(k=>k!=='calls')){assert.deepEqual(paired.a[key],paired.b[key],'A/B '+key+' differs');proof.checks.push(key);}
    proof.passed=true;
   }catch(e){proof.error=e.message.slice(0,400);}
   proof.paired=paired;write(join(sandbox,'task.json'),proof);recordTaskOperations(proof,output);
   write(join(output,'http/workflows/task-notifications-audit.json'),{cases:[proof]});return proof;
  },{configure,workerRoles:['notification-worker','task-index-worker']});
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const r=await taskParity(runDirectory(process.argv[2]),runDirectory(process.argv[3]||process.argv[2]));console.log(r.passed?'ARC-014 passed':r.error);process.exitCode=r.passed?0:1;
}
