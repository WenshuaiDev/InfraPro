import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { loadLab, command, readJSON, processAlive, stopProcess, resumeProcess, sleep, waitHTTP, ownedContainers, checkContainerOwnership } from './fixtures/diagnostics-lab.mjs';
import { diagnosticClient, eventually, evidence } from './fixtures/diagnostics-client.mjs';
import { faultCases, injectFault, recoverLab, rebind } from './fixtures/diagnostics-faults.mjs';

const lab=loadLab(); if(!lab.business||lab.faults.length)throw Error('Recover the full owned lab first');
const resume=process.argv.includes('--resume'),selected=process.argv.slice(2).filter(v=>v!=='--resume');for(const name of selected)assert(faultCases.includes(name));
const records=resume?readJSON(lab.directory+'/evidence/faults.json').cases.filter(c=>!selected.includes(c.case)):[];
const auth={logins:0,retry_after_ms:[]};
async function login() {
 const session=await diagnosticClient(lab,{loginTimeoutMs:75000,onRetry:ms=>auth.retry_after_ms.push(ms)});
 auth.logins++;return session;
}
let client=await login();
async function usableSession() {
 try{await client.get('/overview');return 'reused';}
 catch(error){if(error.status!==401)throw error;client=await login();return 'renewed_after_401';}
}
// An independently labelled control proves that stopping/recovering this lab
// leaves unrelated workloads running, including their original start times.
const pg=ownedContainers(lab).find(id=>command('docker',['inspect','--format','{{index .Config.Labels "io.infrapro.replica"}}',id]).trim()==='postgres-1');
checkContainerOwnership(lab,pg);
const controlTag=randomUUID(),controlImage=command('docker',['inspect','--format','{{.Image}}',pg]).trim();
const control=command('docker',['run','--detach','--rm','--pull','never','--network','none','--read-only','--cap-drop','ALL','--label','io.infrapro.diagnostic.control='+controlTag,'--entrypoint','sleep',controlImage,'3600']).trim();
assert.match(control,/^[0-9a-f]{64}$/);
process.once('exit',()=>{
 try{if(command('docker',['inspect','--format','{{index .Config.Labels "io.infrapro.diagnostic.control"}}',control]).trim()===controlTag)command('docker',['rm','-f',control])}catch{/* Already removed by --rm. */}
});
const controlState=()=>command('docker',['inspect','--format','{{.State.Running}} {{.State.StartedAt}}',control]).trim();
const baseline=controlState();assert.match(baseline,/^true /);
assert.throws(()=>checkContainerOwnership(lab,control),/ownership/);
const get=async path=>(await client.get(path)).data;
const check=async id=>(await get('/checks/'+id)).current_observation;
const record=(name,value)=>{const at=records.findIndex(r=>r.case===name),row={case:name,result:'passed',...value};if(at<0)records.push(row);else records[at]=row;evidence(lab,'faults',{command:'node scripts/diagnostics-fault-check.mjs',evidence_type:'owned real services + labelled fault injection',result:'in_progress',cases:records});console.log(`Fault ${name}: passed and recovered`)};
for(const name of (selected.length?selected:faultCases).filter(n=>!resume||!records.some(r=>r.case===n))){
 const before=await get('/overview'),logins=auth.logins,retries=auth.retry_after_ms.length;let observed,recoverySession;
 try{
  await injectFault(lab,name);
  if(['disk-pressure','store-unwritable'].includes(name))await usableSession();
  switch(name){
   case'business-offline':{const old=client;await old.get('/overview');client=await login();await old.logout();observed=await eventually(async()=>{const api=await check('api-presence'),pg=await check('postgres-protocol'),web=await check('web-container');return api.facets.presence==='missing'&&pg.outcome==='failed'&&web.facets.presence==='stopped'&&{api,pg,web}});break;}
   case'postgres-offline':observed=await eventually(async()=>{const v=await check('postgres-protocol');return v.outcome==='failed'&&v});break;
   case'worker-loop-failure':observed=await eventually(async()=>{const values=await Promise.all(['notification-worker','task-index-worker'].map(id=>check(id+'-report')));return values.every(v=>v.facets.presence==='running'&&v.facets.loop==='blocked')&&!processAlive(lab,'files-worker')&&{reports:values,files_worker_exited:true}});assert(processAlive(lab,'notification-worker')&&processAlive(lab,'task-index-worker'));break;
   case'worker-stop':observed=await eventually(async()=>{const v=await check('notification-worker-presence');return v.facets.presence==='missing'&&v});break;
   case'observer-disconnect':case'stale-replay':observed=await eventually(async()=>{const n=await get('/nodes/native-c'),v=await check('native-c-diagnostics');return !n.observer.online&&v.freshness==='stale'&&{observer:n.observer,observation:v}});assert((await get('/nodes/native-b')).observer.online);break;
   case'container-restart':observed=await eventually(async()=>{const v=await get('/components/clickhouse');return v.component.expected_replicas===2&&v.component.ready_replicas===2&&v});break;
   case'permission-denied':case'report-corrupt':observed=await eventually(async()=>{const v=await check('notification-worker-report');return v.outcome===(name==='permission-denied'?'permission_denied':'invalid_report')&&v});assert(processAlive(lab,'notification-worker'));break;
   case'disk-pressure':observed=await eventually(async()=>{const v=await get('/overview');return v.resource_warnings.some(x=>x.reason_code==='disk_pressure')&&v.resource_warnings});break;
   case'store-unwritable':observed=await eventually(async()=>{const self=await get('/self'),v=await check('local-resources');return self.store_state.status==='state_unavailable'&&v.persistence==='volatile'&&{store:self.store_state,observation:v}});break;
   case'credential-revoke':await eventually(async()=>{try{await get('/overview');return false}catch(e){return /401/.test(e.message)}});observed={old_session_rejected:true};break;
   case'clock-skew':observed=await eventually(async()=>{const v=await check('native-c-diagnostics');return v.freshness==='clock_uncertain'&&v});break;
  }
 }finally{await recoverLab(lab);recoverySession=await usableSession()}
 await eventually(async()=>{const v=await get('/overview');return v.coverage.fresh===v.coverage.expected&&v.abnormal_components.length===0},70000);
 assert.equal(controlState(),baseline,'Unrelated control container changed');
 record(name,{before:{summary:before.summary,coverage:before.coverage},observed,session:{recovery:recoverySession,fresh_logins:auth.logins-logins,retry_after_ms:auth.retry_after_ms.slice(retries)},recovery_result:'all expected probes fresh; no abnormal components; separate control container retained its start time'});
}
// The same actual display PID owns two HTTP endpoints. Fail only one fixed
// endpoint URL while preserving its peer and native process identity.
const originalBindings=readJSON(lab.directory+'/native-a/bindings.json');
let endpointEvidence;
try{const broken=structuredClone(originalBindings);broken.bindings.find(b=>b.id==='display-runtime-assets-check').url+='/missing-fixture';await rebind(lab,'native-a',broken);endpointEvidence=await eventually(async()=>{const v=await get('/replicas/display-runtime-1');const a=v.checks.find(c=>c.probe_summary.id==='display-runtime-assets-check').current_observation;const b=v.checks.find(c=>c.probe_summary.id==='display-runtime-runtime-check').current_observation;const p=v.checks.find(c=>c.probe_summary.id==='display-runtime-presence').current_observation;return a.reason_code==='response_mismatch'&&b.outcome==='ok'&&p.facets.presence==='running'&&{process:v.process,failed_endpoint:a,healthy_endpoint:b}})}finally{await rebind(lab,'native-a',originalBindings)}
await eventually(async()=>{const v=await check('display-runtime-assets-check');return v.outcome==='ok'});
record('display-one-endpoint-failure',{observed:endpointEvidence,session:{recovery:await usableSession()},recovery_result:'same PID; original endpoint binding restored with an observed successful response'});
// Losing diagnostics cannot stop business endpoints or worker task loops.
const business=readJSON(lab.business_config),prior=readJSON(lab.directory+'/reports/notification-worker.json');
await stopProcess(lab,'server');await stopProcess(lab,'native-a');
try{assert.equal((await fetch('http://'+business.http_addr+'/readyz')).status,200);await sleep(2500);const after=readJSON(lab.directory+'/reports/notification-worker.json');assert.notEqual(after.lanes[0].last_attempt_at,prior.lanes[0].last_attempt_at)}finally{await resumeProcess(lab,'server');await resumeProcess(lab,'native-a')}
await waitHTTP(lab.origin);const restartedSession=await usableSession();await eventually(async()=>{return(await get('/overview')).coverage.complete});
record('diagnostics-stopped-business-continues',{session:{recovery:restartedSession},recovery_result:'server and observer restarted; independent business readiness and loop progressed'});
assert.equal(controlState(),baseline);
await client.logout();
evidence(lab,'faults',{command:'node scripts/diagnostics-fault-check.mjs',evidence_type:'owned real services + labelled fault injection',result:'passed',cases:records,session:auth,recovery_result:'lab recovered; independently labelled control container stayed running and retained its start time'});
