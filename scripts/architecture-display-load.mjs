import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';
import { copyFileSync,existsSync,mkdirSync,readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { read, root, runDirectory, write, sleep, digest, safeDiagnosticLine, fileLease } from './architecture-lab.mjs';
import { historyInputs, historyJobs } from './architecture-display-history.mjs';
import { prepareLargeSource, readLargeSource } from './architecture-display-stream.mjs';
import {hostPressure,databasePressure,databaseStatements,statementDelta} from './architecture-display-observe.mjs';
import { openDisplayCampaign } from './architecture-display-campaign.mjs';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';
import { displayHostResources, parkPastDisplayTrials } from './architecture-display-retention.mjs';
import { displayBudgetTotal, displayRoleBudget, displaySnapshot, displayStack, formalDisplayBudget, resumeDisplayStack, retainDisplayStorage } from './architecture-display-lab.mjs';
import { displayHardware, displayProcessLimits, measuredDisplayBudget, openDisplaySockets, publishedServiceLimits } from './architecture-display-processes.mjs';
import { displayInflightProfile, displayProcessProfile } from './architecture-display-topology.mjs';
const execute=promisify(execFile);
const now=()=>performance.now();
const p95=a=>a.toSorted((x,y)=>x-y)[Math.ceil(a.length*.95)-1];
const median=a=>a.toSorted((x,y)=>x-y)[Math.floor(a.length/2)];
const statistics={targets:[{view:'c'}],source_kinds:['direct'],group_by:'none',all_history:true};
const directory={target:{view:'r'},limit:50};

export function displayFailure(error) {
  const where = String(error?.stack || '').match(/\/(architecture-display-[a-z-]+\.mjs:\d+):\d+/)?.[1];
  const measurable = (value) => typeof value === 'number' || typeof value === 'boolean' ||
    (typeof value === 'string' && /^(?:[0-9.:-]+|ready|pending|done|succeeded|failed|received|raw|good)$/.test(value));
  const mismatch = error?.code === 'ERR_ASSERTION' && measurable(error.expected) && measurable(error.actual)
    ? ` (expected ${error.expected}, observed ${error.actual})` : '';
  return (safeDiagnosticLine(error?.message || error?.name) + mismatch + (where ? ' [' + where + ']' : '')).slice(0,350);
}

export async function personnel(stack,username=stack.s.username) {
  const c=await request.newContext({timeout:30000});
  const headers={Origin:stack.origin};
  const raw=async(path,data,method=data===undefined?'GET':'POST')=>{
    const r=await c.fetch(stack.api+'/api/v1'+path,{headers,data,method,timeout:120000});
    if(!r.ok()){
      const e=await r.json().catch(()=>({}));
      const status=r.status();await r.dispose();
      throw Error(method+' '+path.replace(/[\da-f-]{36}/g,'{id}')+' HTTP '+status+' '+(e.code||''));
    }
    return r;
  };
  const call=async(...args)=>{const r=await raw(...args);try{return r.status()===204?{}:await r.json();}finally{await r.dispose();}};
  const login=async()=>{const r=await call('/auth/login',{username,password:stack.s.password});assert.equal(r.status,'authenticated');};
  const authenticatedFetch=async(path,options={})=>{
    assert(path.startsWith('/tenants/'));
    const url=new URL('/api/v1'+path,stack.api);
    assert.equal(url.origin,stack.api);assert.equal(url.hostname,'127.0.0.1');
    const state=await c.storageState();
    const cookies=state.cookies.filter(item=>item.domain===url.hostname&&url.pathname.startsWith(item.path)&&!item.secure);
    assert(cookies.length>0);
    const cookie=cookies.map(item=>item.name+'='+item.value).join('; ');
    return fetch(url,{...options,headers:{...headers,...options.headers,Cookie:cookie},redirect:'error',signal:AbortSignal.timeout(120000)});
  };
  try{
    const csrf=await c.get(stack.api+'/api/v1/auth/csrf');
    try {assert(csrf.ok());headers['X-CSRF-Token']=(await csrf.json()).token;} finally {await csrf.dispose();}
    await login();
    return {raw,call,login,fetch:authenticatedFetch,close:()=>c.dispose()};
  }catch(error){await c.dispose();throw error;}
}

export async function openScreens(stack,numberOfSessions=10) {
  const screens=[];
  const owner=await personnel(stack,'SDKLoad');
  const scope=(id,domain,field_groups)=>({id,domain,selector:{kind:'project'},field_groups});
  const scopes=[scope('r','resources',['resource.metadata','statistics.counts']),scope('c','channels',['resource.metadata','statistics.counts','monitoring.values'])];
  const base='/tenants/'+stack.s.tenant+'/projects/'+stack.s.project+'/displays/instances/';
  try{
    const instances=stack.sql('SELECT id::text FROM display_instances ORDER BY id').split('\n');
    assert.equal(instances.length,10);
    for(const id of instances.slice(0,numberOfSessions)){
      let detail=await owner.call(base+id);
      await owner.call(base+id+'/grant',{request_id:randomUUID(),reason:'隔离大屏验收授权',expected_revision:detail.instance.revision,
        grant:{scope_format:2,read_scopes:scopes,capabilities:detail.grant.capabilities,valid_until:new Date(Date.now()+7200000).toISOString()}},'PUT');
      detail=await owner.call(base+id);
      const verifier=randomBytes(32).toString('hex');
      const launched=await owner.call(base+id+'/launch',{expected_revision:detail.instance.revision,challenge:digest(verifier),audience:stack.runtime});
      const response=await fetch(stack.runtime+'/runtime/exchange',{method:'POST',headers:{Origin:stack.runtime,'Content-Type':'application/json','X-Display-Protocol':'1'},body:JSON.stringify({code:launched.code,verifier,audience:stack.runtime})});
      assert.equal(response.status,200,'display exchange must succeed');
      const data=await response.json(),cookie=response.headers.getSetCookie().find(v=>v.startsWith('infrapro_display='));
      assert(data.session.id && cookie);
      screens.push({id:data.session.id,token:cookie.split(';')[0].slice('infrapro_display='.length)});
    }
  }finally{await owner.close();}
  stack.s.sessions=screens;
  let active=true;
  const keepAlive=(async()=>{let last=Date.now();while(active){
    await sleep(300);if(!active || Date.now()-last<30000)continue;last=Date.now();
    await Promise.all(screens.map(async h=>{
      const response=await fetch(stack.runtime+'/runtime/'+h.id+'/renew',{method:'POST',signal:AbortSignal.timeout(30000),headers:{Origin:stack.runtime,'X-Display-Protocol':'1',Authorization:'Bearer '+h.token}});
      if(!response.ok)throw Error('Runtime session renewal failed with '+response.status);
      await response.arrayBuffer();
    }));
  }})().then(()=>null,e=>e);
  return async()=>{active=false;const error=await keepAlive;if(error)throw error;};
}

export async function sdk(stack,session,method,params,generation,signal){
  const response=await fetch(stack.runtime+'/runtime/'+session.id+'/query',{
    method:'POST',headers:{'Content-Type':'application/json','X-Display-Protocol':'1',Origin:stack.runtime,Authorization:'Bearer '+session.token},
    body:JSON.stringify({method,params,generation}),signal});
  const value=await response.json();
  if(!response.ok)throw Error('SDK '+method+' HTTP '+response.status+' '+(value.code||''));
  return value;
}
export async function readyStatistics(stack,session,name,allowPending){
  const signal=AbortSignal.timeout(120000);
  let v=await sdk(stack,session,'statistics.monitoring',statistics,name,signal);
  if(!allowPending)assert.equal(v.state,'ready','A hot query returned a non-ready response');
  while(v.state==='pending'){
    await sleep(100);
    v=await sdk(stack,session,'queries.get',{query_id:v.query.id},name,signal);
  }
  assert.equal(v.state,'ready');
  const totals=v.data.totals,raw=totals.raw;
  assert.equal(totals.scope_channel_count,'600');assert.equal(totals.raw_channel_count,'600');assert.equal(totals.derived_channel_count,'0');
  for(const key of ['ever_published_count','active_count','sample_count'])assert.equal(raw[key],'100000');
  const start=Date.UTC(2026,10,1),last=start+166*60000;
  assert.equal(Date.parse(raw.first_time),start);assert.equal(Date.parse(raw.last_time),last);
  if(raw.quality_counts)assert.deepEqual(raw.quality_counts,{good:'100000',unmarked:'0',suspect:'0',bad:'0'});
  return {observations:100000,channels:600,first:raw.first_time,last:raw.last_time};
}

export function checkCurve(result,source) {
  assert.equal(result.state,'ready');assert.equal(result.trends.length,1);
  const trend=result.trends[0],ordinal=source.channels.indexOf(source.channel),count=Math.ceil((100000-ordinal)/600);
  assert(ordinal>=0);assert.equal(trend.channel_id,source.channel);assert.equal(trend.resolution,'raw');assert.equal(trend.sample_count,String(count));
  const rows=trend.buckets.flatMap(b=>b.points);
  assert.equal(rows.length,count);
  rows.forEach((row,i)=>{
    assert.equal(row.channel_id,source.channel);assert.equal(row.quality,'good');assert.equal(row.revoked,false);
    assert.match(row.value,/^\d+(\.0+)?$/);assert.equal(Number(row.value),ordinal+i*600);
    assert.equal(Date.parse(row.observed_at),Date.UTC(2026,10,1)+i*60000);
  });
  return {channels:1,count,valuesSHA256:digest(JSON.stringify(rows.map(row=>[row.observed_at,row.value,row.quality])))};
}
function resourceSampler(stack){
  const samples=[];let running=true;
  const pids=Object.entries(stack.processes).map(([role,p])=>[p.pid,role]);
  const tool=join(stack.dir,'private','display-process-io');
  execFileSync('cc',['-O2','-o',tool,join(root,'scripts/fixtures/architecture-process-io.c')]);
  const loop=(async()=>{
    while(running){
      const when=new Date().toISOString();
      const rolePIDs=Object.fromEntries(pids);
      const roles={};
      try{
        const mac=process.platform==='darwin';
        const {stdout}=await execute('ps',['-p',pids.map(x=>x[0]).join(','),'-o','pid=,%cpu=,rss=,time='+(mac?',inblk=,oublk=':'')]);
        for(const row of stdout.trim().split('\n')){
          const [pid,cpu,rss,cpuTime,inputBlocks,outputBlocks]=row.trim().split(/\s+/);
          roles[rolePIDs[pid]]={cpuPercent:Number(cpu),rssKiB:Number(rss),cpuTime,...mac?{inputBlocks:Number(inputBlocks),outputBlocks:Number(outputBlocks)}:{}};
        }
        const io=await execute(tool,pids.map(([pid])=>String(pid)));
        for(const line of io.stdout.trim().split('\n')){
          const [pid,input,output]=line.split(' ');
          Object.assign(roles[rolePIDs[pid]],{diskReadBytes:Number(input),diskWriteBytes:Number(output)});
        }
        const clickhouse=stack.storage?.clickhouse||'infrapro-monitoring-contract-ch1-1';
        const values=await execute('docker',['stats','--no-stream','--format','{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.BlockIO}}|{{.NetIO}}',
          stack.lab.container,clickhouse,clickhouse.replace(/-ch1-1$/,'-ch2-1'),clickhouse.replace(/-ch1-1$/,'-keeper-1'),stack.storage?.objects||'infrapro-monitoring-contract-s3-1','infrapro-account-tests-redis-1']);
        const pg=databasePressure(stack.sql);
        const [machine,connections]=samples.length%3===0?await Promise.all([
          hostPressure(new Set(pids.map(([pid])=>pid))),openDisplaySockets(stack)
        ]):[];
        if(connections)for(const [role,values] of Object.entries(connections.roles)){
          const [pgLimit,chLimit]=stack.budget.roles[role];
          assert(values.postgres<=pgLimit && values.clickhouse<=chLimit,role+' opened connections beyond the declared pool');
        }
        samples.push({at:when,roles,databases:values.stdout.trim().split('\n'),pgConnections:pg.connections,pg,machine,connections});
      }catch(e){samples.push({at:when,roles,error:e.code || e.name});}
      await sleep(1000);
    }
  })();
  return async()=>{running=false;await loop;return samples;};
}

export async function displayScene(stack,name,file,onMeasurementStart=()=>{},verifyBudget) {
  const samples={},history=[],jobs=[];
  const failures=[];
  const start=new Date().toISOString();
  const s=stack.s;
  const prefix='/tenants/'+s.tenant+'/monitoring';
  const measure=async(kind,action)=>{
    const begin=now(),at=new Date().toISOString();
    try{const value=await action();(samples[kind]??=[]).push({at,ms:now()-begin,ok:true,...value});}
    catch(e){const message=displayFailure(e);(samples[kind]??=[]).push({at,ms:now()-begin,ok:false,error:message});failures.push([kind,message]);}
  };
  const meta=()=>({request_id:randomUUID(),expected_revision:'0',reason:'隔离大屏混合负载'});
  let user,closeScreens,stopMetrics,resources=[],execution=[];
  let streams=[];
  let historySeed,historyQuery,historyChannels=[],measurementStart,exportRows;
  const overlaps=[];
  const bgStart=Date.now();
  function historyState(){return stack.sql("SELECT coalesce(json_agg(x),'[]') FROM (SELECT id,state,kind,updated_at,created_at FROM monitoring_jobs WHERE id IN ("+jobs.map(j=>"'"+j.job_id+"'").join(',')+") ORDER BY id)x")}
  async function waitHistory(){
    if(!jobs.length)return;
    const timeout=Date.now()+20*60000;
    for(;;){
      const state=JSON.parse(historyState());
      history.push({at:new Date().toISOString(),states:state});
      if(state.length===jobs.length && state.every(r=>r.state==='succeeded')){
        const exports=jobs.filter(j=>j.export_id);
        const expected=historyChannels.reduce((sum,id)=>sum+Math.ceil((100000-s.channels.indexOf(id))/600),0);
        exportRows=[];
        for(const job of exports){
          const manifest=await user.call(prefix+'/exports/'+job.export_id+'/files');
          assert.equal(Number(manifest.row_count),expected);assert(manifest.files.length>0);
          exportRows.push({format:manifest.format,rows:Number(manifest.row_count)});
        }
        assert.equal(exportRows.length,18);
        return;
      }
      if(state.some(r=>['failed','canceled','cancelled','needs_action'].includes(r.state)) || Date.now()>timeout)throw Error('History backlog did not finish every accepted job');
      await sleep(1500);
    }
  }
  async function submitExports(count){
    const accepted=await Promise.all(Array.from({length:count},(_,i)=>user.call(prefix+'/exports',{...meta(),query:historyQuery,format:i%2===0?'parquet':'csv',kind:'details'})));
    for(const job of accepted){assert.match(job.job_id,/^[a-f0-9-]{36}$/);const id=job.id||job.export_id;assert.match(id,/^[a-f0-9-]{36}$/);jobs.push({...job,export_id:id});}
  }
  function checkpoint(phase){
    if(!jobs.length)return;
    const states=JSON.parse(historyState()),at=new Date().toISOString();
    overlaps.push({phase,at,running:states.filter(v=>v.state==='running').length,queued:states.filter(v=>v.state==='queued').length,succeeded:states.filter(v=>v.state==='succeeded').length});
    history.push({at,states});
  }
  let originalPrivateRows,coldStatements;
  try{
    closeScreens=await openScreens(stack);
    user=await personnel(stack);
    stack.largeSource??=await prepareLargeSource(user,prefix);
    const select=await user.call(prefix+'/selections',{channel_ids:[s.channel]});
    const dataQuery={selection_id:select.id,all_history:true,qualities:['good'],timezone:'UTC',limit:200};
    const authorized=new Set(stack.sql("SELECT DISTINCT kind||'/'||id::text FROM project_resource_visibility('"+s.tenant+"','"+s.project+"',NULL) WHERE full_access").split('\n'));
    assert.equal(authorized.size,2600);
    stopMetrics=resourceSampler(stack);
    if(name!=='idle')historySeed=await historyInputs(stack,user);
    // Warm all sessions against the real endpoints first. Then clear only
    // disposable per-query results. Published measurements and indexes persist.
    await Promise.all(s.sessions.map((h,i)=>readyStatistics(stack,h,'warmup-'+i,true)));
    originalPrivateRows=Number(stack.sql('SELECT count(*) FROM display_read_queries'));
    stack.sql('DELETE FROM display_read_queries');
    assert.equal(stack.sql('SELECT count(*) FROM display_read_queries'),'0');
    if(name!=='idle'){
      historyChannels=s.channels.slice(0,36);
      const selection=await user.call(prefix+'/selections',{channel_ids:historyChannels});
      historyQuery={...dataQuery,selection_id:selection.id,limit:200};
      jobs.push(...await historyJobs(historySeed,user));await submitExports(6);
      checkpoint('cold-start');
    }
    measurementStart=new Date().toISOString();
    onMeasurementStart(measurementStart);
    if(name==='mixed'){
      streams=[measure('publish',async()=>{
        const queued=await user.call(prefix+'/imports/'+s.staged.id+'/publish',{...meta(),expected_revision:s.staged.revision});
        assert(queued.job_id);const deadline=Date.now()+120000;
        for(;;){const v=await user.call(prefix+'/imports/'+s.staged.id);
          if(v.state==='published'){assert.equal(v.counts.proposed,'1');return{adopted:1};}
          assert(Date.now()<deadline && !['failed','canceled'].includes(v.state),'Publication did not complete');
          await sleep(100);
        }
      })];
    }
    const beforeCold=databaseStatements(stack.sql);
    const beforeJobs=stack.sql("SELECT clock_timestamp()::text");
    await Promise.all(s.sessions.map((h,i)=>measure('cold',()=>readyStatistics(stack,h,'cold-'+i,true))));
    const afterJobs=stack.sql("SELECT clock_timestamp()::text");
    for(const timestamp of [beforeJobs,afterJobs])assert.match(timestamp,/^[\d:\s.+-]+$/);
    execution=JSON.parse(stack.sql("SELECT coalesce(json_agg(row_to_json(x)),'[]') FROM (SELECT state,attempts,extract(epoch FROM first_started_at-created_at)*1000 AS queue_ms,extract(epoch FROM updated_at-first_started_at)*1000 AS execution_ms FROM display_read_jobs WHERE created_at BETWEEN '"+beforeJobs+"' AND '"+afterJobs+"' ORDER BY created_at,query_id)x"));
    coldStatements=statementDelta(beforeCold,databaseStatements(stack.sql));
    for(let i=0;i<3;i++){
      if(name!=='idle'){await submitExports(4);checkpoint('hot-'+i+'-start');}
      await Promise.all(s.sessions.map((h,index)=>readyStatistics(stack,h,'preheat-'+i+'-'+index,true)));
      const additional=()=>Array.from({length:10},async(_,j)=>{
        // Tenant administration and the console curve use actual authenticated
        // HTTP, alongside the live display, streaming and history requests.
        await measure('ledger',async()=>{const v=await user.call('/tenants/'+s.tenant+'/project-governance');assert(v.items.some(p=>(p.id||p.project_id||p.project?.id)===s.project));return{projects:v.items.length};});
        await measure('curve',async()=>checkCurve(await user.call(prefix+'/queries/trends',dataQuery),s));
        if(j<3)await measure('login',async()=>{const f=await personnel(stack);await f.close();return{status:'authenticated'};});
        if(j<2)await measure('stream',()=>readLargeSource(user,stack.largeSource));
      });
      await Promise.all([...(name==='idle'?[]:additional()),...s.sessions.map(async(h,index)=>{
        await measure('hot',()=>readyStatistics(stack,h,'hot-'+i+'-'+index,false));
        await measure('directory',async()=>{
          const page=await sdk(stack,h,'resources.query',directory,'directory-'+i+'-'+index,AbortSignal.timeout(120000));
          assert.equal(page.data.total,'2600');assert.equal(page.data.items.length,50);
          const rows=page.data.items.map(x=>x.kind+'/'+x.id);assert.equal(new Set(rows).size,50);assert(rows.every(id=>authorized.has(id)));
          return{total:2600,delivered:50,orderedPageSHA256:digest(JSON.stringify(rows))};
        });
      })]);
      if(name==='idle')await Promise.all(additional());else checkpoint('hot-'+i+'-end');
    }
    await Promise.all(streams);
    await waitHistory();
    if(name!=='idle')assert(overlaps.filter(v=>v.phase.endsWith('start')).every(v=>v.running+v.queued>0),'History queue emptied before a measured phase');
  }catch(e){failures.push(['scene',displayFailure(e)]);}
  finally{
    await Promise.allSettled(streams);
    for(const [kind,cleanup] of [['personnel-session',()=>user?.close()],['runtime-renewal',closeScreens],['resource-sampler',async()=>{resources=await stopMetrics?.()||[];}]]) {
      try{await cleanup?.();}catch(e){failures.push([kind,safeDiagnosticLine(e.message||e.name).slice(0,300)]);}
    }
  }
  const metrics=Object.fromEntries(Object.entries(samples).map(([kind,values])=>[kind,{count:values.length,failed:values.filter(v=>!v.ok).length,p95Ms:p95(values.map(v=>v.ms))}]));
  const required={cold:10,hot:30,directory:30,ledger:30,curve:30,login:9,stream:6,...name==='mixed'?{publish:1}:{}};
  const ok=failures.length===0 && resources.length>0 && resources.every(v=>!v.error) && Object.entries(required).every(([key,n])=>metrics[key]?.count===n) &&
    metrics.hot.p95Ms<=2000 && metrics.directory.p95Ms<=2000 && samples.cold.every(v=>v.ms<=120000) &&
    execution.length===required.cold && execution.every(job=>job.state==='done'&&job.queue_ms>=0&&job.execution_ms>=0);
  const result={scene:name,side:stack.side,round:stack.round,at:start,finishedAt:new Date().toISOString(),elapsedMs:Date.now()-bgStart,
    measurementStart,historyOverlap:overlaps,submittedHistoryJobs:jobs.length,historyExportChannels:historyChannels.length,exportRows,dimensions:{instances:10,resources:2600,channels:600,observations:100000},planner:stack.planner,budget:stack.budget,originalPrivateRows,passed:ok,metrics,samples,failures,resources,execution,coldStatements,history};
  if(verifyBudget && !verifyBudget(result)){
    result.passed=false;
    result.failures.push(['operating-budget','Observed process identities, service limits, sockets or object concurrency do not match the signed profile']);
  }
  write(file,result);console.log(stack.side+' '+stack.round+' '+name+' '+(result.passed?'passed':'failed')+' '+JSON.stringify(Object.fromEntries(Object.entries(metrics).map(([k,v])=>[k,Math.round(v.p95Ms)]))));
  return result;
}

export async function displayLoad(dir,options={}) {
  const unlock=await fileLease(join(dir,'load/display/campaign.lock'),{wait:false});
  try{return await runDisplayLoad(dir,options);}finally{await unlock();}
}
async function runDisplayLoad(dir,{snapshot,rounds=3,onlySide,onlyScene,resume=false,first='b',startFile,stopOnFailure=false,releaseDirectory=dir,budgetProfile=formalDisplayBudget}={}) {
  assert(['a','b'].includes(first));
  assert(Number.isInteger(rounds)&&rounds>=1&&rounds<=3);
  assert(!onlySide||['a','b'].includes(onlySide));
  assert(!onlyScene||['idle','history','mixed'].includes(onlyScene));
  const where=join(dir,'load/display');mkdirSync(where,{recursive:true,mode:0o700});
  if(startFile)assert.equal(dirname(resolve(startFile)),where,'An explicit scheduler gate must be inside this benchmark directory');
  const pointer=join(dir,'private/display-snapshot.json');
  snapshot??=resume&&existsSync(pointer)?read(pointer).path:await displaySnapshot(dir);
  assert.equal(dirname(snapshot),join(dir,'private/display-load'));
  write(pointer,{path:snapshot});
  const manifest=read(join(snapshot,'manifest.json'));
  const candidate=read(join(releaseDirectory,'candidate.json'));
  const roleSQL=Object.fromEntries(postgresRoleSQLFiles.map(file=>[file,digest(readFileSync(join(root,file)))]));
  const baseline=read(join(dir,'baseline.json'));
  const roles=Object.fromEntries(['a','b'].map(side=>[side,displayRoleBudget(side,budgetProfile)]));
  const total=displayBudgetTotal(roles.a);assert.deepEqual(total,displayBudgetTotal(roles.b));
  const profile={budgetProfile,roles,total,objectRequests:16,hardware:displayHardware()};
  if(budgetProfile===formalDisplayBudget)profile.serviceLimits=publishedServiceLimits(read(join(root,displayProcessProfile)));
  const provenance={snapshot:digest(readFileSync(join(snapshot,'manifest.json'))),baseline:baseline.commit,candidate:candidate.inputs.sourceSHA256,roleSQL,resourceProfile:digest(JSON.stringify(profile))};
  const driverNames=['architecture-display-load','architecture-display-lab','architecture-display-roles','architecture-display-history','architecture-display-stream','architecture-display-campaign','architecture-display-retention','architecture-display-topology','architecture-display-postgres','architecture-display-observe','architecture-display-processes','architecture-analysis-parity','architecture-deploy','architecture-process-resources'];
  const files=[...driverNames.map(name=>'scripts/'+name+'.mjs'),displayInflightProfile,displayProcessProfile,'scripts/fixtures/architecture-process-io.c'];
  const drivers=Object.fromEntries(files.map(file=>[file,digest(readFileSync(join(root,file)))]));
  const signature=digest(JSON.stringify({
    ...provenance,binaries:candidate.binaries,driver:Object.values(drivers)
  }));
  const report=[],scope={rounds,onlySide:onlySide??null,onlyScene:onlyScene??null,first};
  const campaign=openDisplayCampaign(where,signature,scope,provenance,resume);
  write(join(campaign.folder,'resource-profile.json'),profile);write(join(where,'resource-profile.json'),profile);
  for(const [file,value] of Object.entries(drivers)){
    const dest=join(campaign.folder,'drivers',file);mkdirSync(dirname(dest),{recursive:true,mode:0o700});
    if(!existsSync(dest))copyFileSync(join(root,file),dest);
    assert.equal(digest(readFileSync(dest)),value);
  }
  write(join(campaign.folder,'drivers/manifest.json'),{signature,files:drivers});
  const roleSQLDirectory=join(campaign.folder,'role-sql');
  for(const [file,sha] of Object.entries(roleSQL)){
    const target=join(roleSQLDirectory,file);
    if(!existsSync(target)){mkdirSync(dirname(target),{recursive:true,mode:0o700});copyFileSync(join(root,file),target);}
    assert.equal(digest(readFileSync(target)),sha,'The SQL role snapshot must match the campaign signature');
  }
  const infrastructureFile=join(campaign.folder,'infrastructure.json');
  let reference=existsSync(infrastructureFile)?read(infrastructureFile).resources:undefined,retained=false;
  const order=first==='a'?['a','b']:['b','a'];
  const checkpoint=(stage,current)=>{
    const progress={at:new Date().toISOString(),stage,signature,campaign:campaign.campaign,...provenance,current,
      completed:report.map(({side,round,scene,passed})=>({side,round,scene,passed}))};
    write(join(campaign.folder,'progress.json'),progress);write(join(where,'progress.json'),progress);
  };
  checkpoint('started');
  series:for(let r=1;r<=rounds;r++)for(const side of onlySide?[onlySide]:r%2===1?order:order.toReversed()){
    for(const scene of onlyScene?[onlyScene]:['idle','history','mixed']){
      const trial={side,round:r,scene},label=side+'-'+r+'-'+scene,file=join(where,label+'.json');
      const sealed=campaign.previous(trial);
      if(sealed){
        assert.equal(sealed.campaign,campaign.campaign);assert.equal(sealed.signature,signature);
        report.push(sealed);write(file,sealed);checkpoint('resumed recorded trial',trial);
        if(stopOnFailure&&!sealed.passed)break series;
        continue;
      }
      const prior=existsSync(file)?read(file):null;
      if(prior&&!prior.campaign){
        const archive=join(where,'attempts',label+'-'+(prior.at||new Date().toISOString()).replaceAll(':','-'));
        mkdirSync(archive,{recursive:true,mode:0o700});copyFileSync(file,join(archive,'sample.json'));
      }
      const attempt=campaign.prepare(trial),began=new Date().toISOString();
      let current,stack,infrastructure;
      try{
        checkpoint('preparing',trial);
        const staged=process.env.ARC_DISPLAY_PREPARED&&read(resolve(process.env.ARC_DISPLAY_PREPARED));
        const reusable=staged?.side===side&&staged.round===r&&staged.snapshot===provenance.snapshot&&staged.budgetProfile===budgetProfile&&staged.version===(side==='b'?provenance.candidate:provenance.baseline)&&
          read(join(staged.workspace,'cohort.json')).phase==='prepared';
        stack=reusable?await resumeDisplayStack(dir,snapshot,staged.workspace,side,{releaseDirectory,round:r,usePrepared:true,roleSQLDirectory}):
          await displayStack(dir,snapshot,side,r,{releaseDirectory,roleSQLDirectory,budgetProfile});
        stack.budget.processLimits=displayProcessLimits(stack,side==='a'?baseline:candidate);
        assert.deepEqual(stack.budget.total,total);
        if(side==='b'){
          assert.deepEqual(stack.budget.roleLogins.appliedRoleSQL,roleSQL);
          assert.equal(stack.budget.roleLogins.total,48);
        }
        if (!retained) {
          const receipt=parkPastDisplayTrials(dir,stack.workspace);
          if (!reference) { reference=receipt.resources;write(infrastructureFile,{resources:reference,at:receipt.finished}); }
          retained=true;
        }
        infrastructure=displayHostResources(dir);
        assert.deepEqual(infrastructure,reference,'Every scene must use the same PostgreSQL, ClickHouse, Keeper and object store resource limits');
        if(startFile){
          write(join(where,'prepared.json'),{at:new Date().toISOString(),...trial,signature,campaign:campaign.campaign,waitingFor:resolve(startFile)});
          while(!existsSync(startFile))await sleep(500);
          startFile=undefined;
        }
        const verifyBudget=budgetProfile===formalDisplayBudget
          ? row=>measuredDisplayBudget(row,profile,(side==='a'?baseline:candidate).binaries)
          : undefined;
        current=await displayScene(stack,scene,attempt.file,at=>{
          attempt.begin(at,{workspace:stack.workspace,database:stack.dbName,clickhouse:stack.chName,bucket:stack.bucket,infrastructure});
          checkpoint('measuring',trial);
        },verifyBudget);
      }catch(error){
        const reason=safeDiagnosticLine(error.message||error.name).slice(0,350);
        current={...trial,at:began,finishedAt:new Date().toISOString(),passed:false,metrics:{},execution:[],samples:{},resources:[],history:[],historyOverlap:[],failures:[['cohort',reason]]};
        console.error(label+' failed '+reason);
      }finally{
        if(stack){
          current.workspace=stack.workspace;current.database=stack.dbName;current.clickhouse=stack.chName;current.bucket=stack.bucket;current.infrastructure=infrastructure;
          try{await stack.close();retainDisplayStorage(stack);}
          catch(error){current.passed=false;current.failures.push(['cohort-retention',safeDiagnosticLine(error.message).slice(0,350)]);}
        }
      }
      current=campaign.store(trial,current);write(file,current);report.push(current);
      checkpoint('recorded',trial);
      if((stopOnFailure||!current.measurementStart)&&!current.passed)break series;
    }
  }
  const relative=[];
  for(const scene of ['idle','history','mixed']){
    const baseline=report.filter(v=>v.side==='a'&&v.scene===scene),candidate=report.filter(v=>v.side==='b'&&v.scene===scene);
    if(baseline.length!==3 || candidate.length!==3)continue;
    for(const metric of ['cold','hot','directory','login','curve','ledger','stream',...(scene==='mixed'?['publish']:[])]){
      if(![...baseline,...candidate].every(row=>row.metrics[metric]?.count))continue;
      const a=median(baseline.map(r=>r.metrics[metric].p95Ms)),b=median(candidate.map(r=>r.metrics[metric].p95Ms));
      relative.push({scene,metric,baselineMedianP95Ms:a,candidateMedianP95Ms:b,ratio:b/a,passed:b<=1.10*a});
    }
  }
  const facts={};
  for(const [metric,field] of [['curve','valuesSHA256'],['directory','orderedPageSHA256'],['stream','sha256']]){
    const all=report.flatMap(r=>r.samples[metric]||[]);
    const observed=[...new Set(all.map(r=>r[field]))];
    facts[metric]={responses:all.length,digests:observed,identical:all.length>0&&observed.length===1&&/^[a-f0-9]{64}$/.test(observed[0])};
  }
  const passed=report.length===18 && report.every(r=>r.passed) && relative.length===22 && relative.every(r=>r.passed)&&Object.values(facts).every(v=>v.identical);
  const result={at:new Date().toISOString(),passed,signature,campaign:campaign.campaign,provenance,relative,facts,rounds:report.map(r=>({side:r.side,round:r.round,scene:r.scene,passed:r.passed,metrics:r.metrics,failures:r.failures,receipt:r.receipt})),snapshot:manifest};
  checkpoint('finished');
  write(join(campaign.folder,'comparison.json'),result);write(join(where,'comparison.json'),result);return result;
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const r=await displayLoad(runDirectory(process.argv[2]),{snapshot:process.env.ARC_DISPLAY_SNAPSHOT,rounds:Number(process.env.ARC_DISPLAY_ROUNDS || 3),onlySide:process.env.ARC_DISPLAY_SIDE,onlyScene:process.env.ARC_DISPLAY_SCENE,resume:process.env.ARC_DISPLAY_RESUME==='1',first:process.env.ARC_DISPLAY_FIRST||'b',startFile:process.env.ARC_DISPLAY_START_FILE,stopOnFailure:process.env.ARC_DISPLAY_STOP_ON_FAILURE==='1',releaseDirectory:process.env.ARC_DISPLAY_RELEASE?runDirectory(process.env.ARC_DISPLAY_RELEASE):undefined});
  process.exitCode=r.passed?0:1;
}
