import assert from 'node:assert/strict';
import { loadLab, command, readJSON, processAlive, stopProcess, resumeProcess, sleep, waitHTTP } from './fixtures/diagnostics-lab.mjs';
import { diagnosticClient, eventually, evidence } from './fixtures/diagnostics-client.mjs';
import { faultCases, injectFault, recoverLab, rebind } from './fixtures/diagnostics-faults.mjs';

const lab=loadLab(); if(!lab.business||lab.faults.length)throw Error('Recover the full owned lab first');
const resume=process.argv.includes('--resume'),selected=process.argv.slice(2).filter(v=>v!=='--resume');for(const name of selected)assert(faultCases.includes(name));
const outside=()=>command('docker',['ps','-q','--filter','status=running']).trim().split('\n').filter(Boolean).filter(id=>{const labels=JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',id]));return labels['io.infrapro.lab.run_id']!==lab.run_id}).sort();
const baseline=outside(),records=resume?readJSON(lab.directory+'/evidence/faults.json').cases.filter(c=>!selected.includes(c.case)):[];let client=await diagnosticClient(lab);
const get=async path=>(await client.get(path)).data;
const check=async id=>(await get('/checks/'+id)).current_observation;
const record=(name,value)=>{const at=records.findIndex(r=>r.case===name),row={case:name,result:'passed',...value};if(at<0)records.push(row);else records[at]=row;evidence(lab,'faults',{command:'node scripts/diagnostics-fault-check.mjs',evidence_type:'owned real services + labelled fault injection',result:'in_progress',cases:records});console.log(`Fault ${name}: passed and recovered`)};
for(const name of (selected.length?selected:faultCases).filter(n=>!resume||!records.some(r=>r.case===n))){
 const before=await get('/overview');let observed;
 try{
  await injectFault(lab,name);
  if(['disk-pressure','store-unwritable'].includes(name))client=await diagnosticClient(lab);
  switch(name){
   case'business-offline':client=await diagnosticClient(lab);observed=await eventually(async()=>{const api=await check('api-presence'),pg=await check('postgres-protocol'),web=await check('web-container');return api.facets.presence==='missing'&&pg.outcome==='failed'&&web.facets.presence==='stopped'&&{api,pg,web}});break;
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
 }finally{await recoverLab(lab);client=await diagnosticClient(lab)}
 await eventually(async()=>{const v=await get('/overview');return v.coverage.fresh===v.coverage.expected&&v.abnormal_components.length===0},70000);
 assert.deepEqual(outside(),baseline,'Unrelated containers changed');
 record(name,{before:{summary:before.summary,coverage:before.coverage},observed,recovery_result:'all expected probes fresh; no abnormal components; unrelated containers unchanged'});
}
// The same actual display PID owns two HTTP endpoints. Fail only one fixed
// endpoint URL while preserving its peer and native process identity.
const originalBindings=readJSON(lab.directory+'/native-a/bindings.json');
try{const broken=structuredClone(originalBindings);broken.bindings.find(b=>b.id==='display-runtime-assets-check').url+='/missing-fixture';await rebind(lab,'native-a',broken);const result=await eventually(async()=>{const v=await get('/replicas/display-runtime-1');const a=v.checks.find(c=>c.probe_summary.id==='display-runtime-assets-check').current_observation;const b=v.checks.find(c=>c.probe_summary.id==='display-runtime-runtime-check').current_observation;const p=v.checks.find(c=>c.probe_summary.id==='display-runtime-presence').current_observation;return a.reason_code==='response_mismatch'&&b.outcome==='ok'&&p.facets.presence==='running'&&{process:v.process,failed_endpoint:a,healthy_endpoint:b}});record('display-one-endpoint-failure',{observed:result,recovery_result:'same PID; failed endpoint binding restored'})}finally{await rebind(lab,'native-a',originalBindings)}
// Losing diagnostics cannot stop business endpoints or worker task loops.
const business=readJSON(lab.business_config),prior=readJSON(lab.directory+'/reports/notification-worker.json');
await stopProcess(lab,'server');await stopProcess(lab,'native-a');
try{assert.equal((await fetch('http://'+business.http_addr+'/readyz')).status,200);await sleep(2500);const after=readJSON(lab.directory+'/reports/notification-worker.json');assert.notEqual(after.lanes[0].last_attempt_at,prior.lanes[0].last_attempt_at)}finally{await resumeProcess(lab,'server');await resumeProcess(lab,'native-a')}
await waitHTTP(lab.origin);client=await diagnosticClient(lab);await eventually(async()=>{return(await get('/overview')).coverage.complete});
record('diagnostics-stopped-business-continues',{recovery_result:'server and observer restarted; independent business readiness and loop progressed'});
evidence(lab,'faults',{command:'node scripts/diagnostics-fault-check.mjs',evidence_type:'owned real services + labelled fault injection',result:'passed',cases:records,recovery_result:'lab recovered; unrelated containers unchanged'});
