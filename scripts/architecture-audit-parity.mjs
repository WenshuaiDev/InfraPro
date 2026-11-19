// Exercise the same frozen audit history through personnel sessions on both
// releases, including the CSV transfer service and the task command adapter.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { client, digest, queryPG, read, runDirectory, sleep, write } from './architecture-lab.mjs';
import { startPairedWorkers, withHTTPPair } from './architecture-http-sandbox.mjs';
import { stop } from './fixtures/parsing-lab.mjs';
import { operation } from './architecture-workflow-http.mjs';

export async function auditParity(dir) {
  const seed=read(join(dir,'private/lab.json')),samples=read(join(dir,'samples.json')).structure;
  const range=queryPG(seed,'seed',"SELECT (min(created_at)-interval '1 second')::text||'|'||(max(created_at)+interval '1 millisecond')::text FROM account_security_events").split('|').map(v=>new Date(v).toISOString());
  const bounds={from:range[0],to:range[1]};
  const parameters=new URLSearchParams({...bounds,limit:'100'});
  const scopes={platform:'/audit',tenant:'/tenants/'+samples.tenant+'/audit',project:'/tenants/'+samples.tenant+'/projects/'+samples.project+'/audit'};
  const commands=new Map(),start=new Map(),records=[],failures=[];
  function command(label){if(!commands.has(label))commands.set(label,randomUUID());return commands.get(label);}
  async function exercise(folder,role){
    const lab=read(join(folder,'private/lab.json')),accounts=read(join(folder,'private/accounts.json'));
    const owner=await client(lab[role],accounts.admin),other=await client(lab[role],accounts.other);
    const receipts=new Map(),jobs=[];
    async function call(label,method,path,{data,headers={},principal='owner',allowed=[200],bytes=false,record=true,verify}={}){
      const session=principal==='owner'?owner:other;
      const response=await session.client.fetch(session.api+path,{method,headers:{...session.headers,...headers},data});
      try {
        const status=response.status(),body=await response.body();
        let value=bytes?body:null;if(!bytes && body.length)value=JSON.parse(body);
        const {template,owner,operationId}=operation(method,new URL(session.api+path).pathname);
        assert(allowed.includes(status),label+' '+method+' '+template+' returned '+status+' '+(value?.code||''));
        const facts=verify?verify(value,response.headers()):{};
        if(record)receipts.set(label,{scenario:label,principal,method,template,owner,operationId,status,facts,error:value?.code??null});
        return value;
      }finally{await response.dispose();}
    }
    const denied=(prefix,path)=>call(prefix+'-foreign','GET',path,{principal:'outside tenant',allowed:[403,404],verify:v=>({code:v.code})});
    try{
      for(const [name,prefix] of Object.entries(scopes)){
        if(jobs.length){
          const previous=jobs.at(-1),deadline=Date.now()+120000;
          while(Date.now()<deadline){
            const v=await call(previous.name+'-wait-capacity','GET',previous.prefix+'/exports/'+previous.job.id,{record:false});
            if(v.state==='succeeded')break;
            assert(!['failed','canceled','cancelled'].includes(v.state),previous.name+' export '+v.state+' '+v.error_code);
            await sleep(180);
          }
          await start.get(role).pause();
        }
        const caps=await call(name+'-caps','GET',prefix+'/capabilities',{verify:v=>{assert(v.read&&v.export&&v.storage_available);return{read:v.read,export:v.export,technical:v.technical};}});
        const absent=prefix+'/exports/'+command('absent-export')+'/generations/1/download';
        await call(name+'-missing-download','GET',absent,{allowed:[404],verify:v=>{assert.equal(v.code,'audit_not_found');return{code:v.code};}});
        await call(name+'-missing-head','HEAD',absent,{allowed:[404],bytes:true,verify:body=>{assert.equal(body.length,0);return{bytes:0};}});
        const page=await call(name+'-events','GET',prefix+'/events?'+parameters,{verify:v=>{assert(v.items.length>0);return{items:v.items.map(e=>[e.id,e.event_type,e.outcome]),coverage:v.coverage};}});
        if(name!=='project'){
          const legacy=name==='platform'?'/access/audit-events':'/tenants/'+samples.tenant+'/audit-events';
          await call(name+'-legacy','GET',legacy+'?limit=1',{verify:v=>{assert.equal(v.items.length,1);return{items:1};}});
        }
        let selected=null;
        for(const event of page.items){
          const path=prefix+'/events/'+event.id;
          const v=await call(name+'-detail-'+event.id,'GET',path,{verify:v=>{assert.equal(v.id,event.id);return{id:v.id,type:v.event_type,changes:v.changes,links:v.links.map(l=>[l.id,l.kind,l.state])};}});
          const link=v.links.find(l=>l.id && l.target);
          if(link){selected={path,event,link};break;}
        }
        assert(selected,name+' must have a reachable link from original facts');
        await call(name+'-trace','GET',selected.path+'/trace',{verify:v=>{assert(Array.isArray(v.items));return {events:v.items.map(e=>e.id),edges:v.edges?.length};}});
        await call(name+'-target','GET',selected.path+'/targets/'+selected.link.id,{verify:v=>{assert.deepEqual(v,selected.link.target);return{kind:v.kind,scope:v.scope};}});
        await denied(name+'-detail',selected.path);
        const input={filter:bounds,field_profile:'standard',reason:'隔离架构回归审计导出',epoch:caps.epoch,authorization_revision:caps.authorization_revision,projection_version:caps.projection_version};
        const headers={'X-Command-ID':command(name+'-export')};
        const submit=async(label,auth=headers)=>call(name+'-'+label,'POST',prefix+'/exports',{headers:auth,data:input,allowed:[200,201,202],verify:v=>{assert(v.id&&v.task_id);return{state:v.state,generation:v.generation,profile:v.field_profile};}});
        const job=await submit('export'),again=await submit('export-retry');assert.equal(again.id,job.id);
        if(name==='platform'){
          const canceled=await submit('export-for-cancel',{'X-Command-ID':command('cancel-source')});
          const task=await call('task-before-cancel','GET','/tasks/'+canceled.task_id,{verify:v=>({state:v.state,generation:v.generation})});
          const ref='/tasks/resolve?'+new URLSearchParams({module:task.source_ref.source_module,kind:task.source_ref.source_kind,source_id:canceled.id});
          await call('task-resolve','GET',ref,{verify:v=>{assert.equal(v.task_id||v.id,canceled.task_id);return {resolved:true};}});
          const data={request_id:command('task-cancel'),action:'cancel',expected_generation:task.generation,expected_control_revision:task.control_revision,reason:'验证原任务命令与审计投影'};
          for(const suffix of ['once','again'])await call('task-cancel-'+suffix,'POST','/tasks/'+task.id+'/commands',{data,allowed:[200,201,202],verify:v=>{assert.equal(v.task_id,task.id);return{state:v.state,commandState:v.command_state,replayed:v.replayed};}});
          await call('task-cancel-events','GET','/tasks/'+task.id+'/events?generation=1',{verify:v=>{assert(v.items.length);return{count:v.items.length};}});
        }
        jobs.push({name,prefix,job});
        await start.get(role).resume();
      }
      for(const {name,prefix,job} of jobs){
        let detail;const path=prefix+'/exports/'+job.id,deadline=Date.now()+120000;
        do{detail=await call(name+'-poll','GET',path,{record:false});if(detail.state==='succeeded')break;
          assert(!['failed','cancelled','canceled'].includes(detail.state),name+' '+detail.state+' '+detail.error_code);
          assert(Date.now()<deadline,name+' export timed out at '+detail.state);await sleep(100);
        }while(detail.state!=='succeeded');
        await call(name+'-export-detail','GET',path,{verify:v=>{assert(Number(v.row_count)>0);assert(v.file&&v.sha256);return{state:v.state,rows:v.row_count,bytes:v.size_bytes,sha:v.sha256,generation:v.generation};}});
        const file=path+'/generations/'+detail.generation+'/download';
        const verify=(body,headers)=>{assert.equal(headers.etag,'"'+detail.sha256+'"');assert.equal(headers['cache-control'],'private, no-store');return{bytes:body.length,type:headers['content-type'],etag:headers.etag,range:headers['content-range']||null};};
        await call(name+'-head','HEAD',file,{bytes:true,verify:(b,h)=>{assert.equal(b.length,0);return verify(b,h);}});
        await call(name+'-download','GET',file,{bytes:true,verify:(b,h)=>{assert.equal(digest(b),detail.sha256);return verify(b,h);}});
        await call(name+'-range','GET',file,{bytes:true,headers:{Range:'bytes=0-9'},allowed:[206],verify:(b,h)=>{assert.equal(b.length,10);return verify(b,h);}});
        await denied(name+'-export',path);
      }
    }catch(e){failures.push({side:role,error:e.message.split('\n')[0]});}
    finally{await owner.client.dispose();await other.client.dispose();}
    return receipts;
  }
  await withHTTPPair(dir,'audit-workflow',async folder=>{
    const a=await exercise(folder,'a'),b=await exercise(folder,'b');
    for(const label of new Set([...a.keys(),...b.keys()])){
      const left=a.get(label),right=b.get(label),x=left||right;
      records.push({...x,status:[left?.status??null,right?.status??null],errors:[left?.error,right?.error],facts:[left?.facts,right?.facts],
        differences:JSON.stringify(left)===JSON.stringify(right)?[]:['paired business result']});
    }
  },{workerRoles:['audit-worker'],configure:({side,privateDir})=>({start:options=>{
    let worker;
    start.set(side,{
      async resume(){await startPairedWorkers({...options,side,privateDir,legacy:['audit-worker'],split:['audit-worker'],claimRoles:[]});worker=options.children.at(-1);},
      async pause(){if(worker?.exitCode===null&&worker?.signalCode===null)await stop(worker);const i=options.children.indexOf(worker);if(i>=0)options.children.splice(i,1);}
    });
  }})});
  const passed=!failures.length&&records.length>0&&records.every(r=>r.differences.length===0);
  write(join(dir,'http/scenarios/audit-task.json'),{scenario:'audit task export',records,failures});
  write(join(dir,'http/workflows/audit-task.json'),{cases:[{arc:'ARC-014',name:'audit scope, task cancellation, CSV and guarded downloads',passed,assertions:records.length,failures}]});
  return {passed,assertions:records.length,failures,differences:records.filter(r=>r.differences.length).map(r=>r.scenario)};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const v=await auditParity(runDirectory(process.argv[2]));console.log(v);process.exitCode=v.passed?0:1;
}
