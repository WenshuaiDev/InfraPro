import assert from 'node:assert/strict';
import { join } from 'node:path';
import { loadLab, startDiagnosticsLab, stopLab, readJSON, privateJSON, stopProcess, resumeProcess, waitHTTP, command } from './fixtures/diagnostics-lab.mjs';
import { diagnosticClient, eventually, evidence } from './fixtures/diagnostics-client.mjs';
import { rebind } from './fixtures/diagnostics-faults.mjs';
const parent=loadLab(),lab=await startDiagnosticsLab({build:false,business:false,ephemeral:true}),cases=[];
const record=(name,value={})=>{cases.push({case:name,result:'passed',...value});console.log('L1 '+name+': passed')};let c;
try{
 c=await diagnosticClient(lab);let overview=await eventually(async()=>{const v=(await c.get('/overview')).data;return v.coverage.complete&&v});
 const m=readJSON(lab.manifest_file);assert(m.probes.every(p=>p.adapter!=='docker_container'));assert(!lab.compose_project);assert.equal((await fetch(lab.origin)).status,200);record('independent-native-no-business-or-docker',{coverage:overview.coverage});
 const components=(await c.get('/components')).data;assert(components.items.some(v=>v.expectation==='disabled'));assert(components.items.some(v=>v.expectation==='on_demand'));record('disabled-and-on-demand-retained');
 await stopProcess(lab,'native-c');const page=(await c.get('/components?limit=1')).data;assert(page.next_cursor);
 m.probes.push({id:'isolated-target',node_id:'native-c',target_kind:'node',target_id:'native-c',adapter:'tcp',binding_id:'isolated-target',facet:'readiness',required:false,interval_ms:5000,timeout_ms:1000,stale_after_ms:20000});privateJSON(lab.manifest_file,m);
 const bindings=readJSON(join(lab.directory,'native-c/bindings.json'));bindings.bindings.push({id:'isolated-target',adapter:'tcp',host:'127.0.0.1',port:1,tls:false});await rebind(lab,'native-c',bindings);
 await assert.rejects(()=>c.get('/components?limit=1&cursor='+encodeURIComponent(page.next_cursor)),/409.*snapshot_expired/);const added=(await c.get('/checks/isolated-target')).data;assert.equal(added.current_observation.freshness,'never');record('hot-add-invalidates-cursor-and-new-target-never');
 await resumeProcess(lab,'native-c');const failed=await eventually(async()=>{const v=(await c.get('/checks/isolated-target')).data.current_observation;return v.reason_code==='connection_refused'&&v.freshness==='fresh'&&v});record('target-failure-is-query-data',{observation:failed});
 const good=readJSON(lab.manifest_file);privateJSON(lab.manifest_file,{...good,unknown_key:true});await eventually(async()=>(await c.get('/self')).data.config_state==='reload_error');assert.equal((await c.get('/self')).data.manifest_revision,good.revision);privateJSON(lab.manifest_file,good);await eventually(async()=>(await c.get('/self')).data.config_state==='ok');record('invalid-reload-keeps-effective-topology-and-recovers');
 // Hold one actual observer down so the restart cannot turn its cache fresh.
 await stopProcess(lab,'native-c');await stopProcess(lab,'server');await resumeProcess(lab,'server');await waitHTTP(lab.origin);c=await diagnosticClient(lab);const old=(await c.get('/checks/isolated-target')).data.current_observation;assert.equal(old.freshness,'historical');record('restart-cache-historical',{observation:old});await resumeProcess(lab,'native-c');await eventually(async()=>(await c.get('/checks/isolated-target')).data.current_observation.freshness==='fresh');
 // Delete/rebind using new revisions. Configuration, not observation absence,
 // owns the list of slots and assigned probes.
 const next=readJSON(lab.manifest_file);next.probes=next.probes.filter(p=>p.id!=='isolated-target');privateJSON(lab.manifest_file,next);const nextBindings=readJSON(join(lab.directory,'native-c/bindings.json'));nextBindings.bindings=nextBindings.bindings.filter(b=>b.id!=='isolated-target');await rebind(lab,'native-c',nextBindings);await assert.rejects(()=>c.get('/checks/isolated-target'),/404/);record('hot-delete-and-assignment-rebind');
 await stopLab(lab);const state=JSON.parse(command(lab.cli,['diagnostics','state','inspect','--server',lab.server_file]));command(lab.cli,['diagnostics','state','compact','--server',lab.server_file]);const after=JSON.parse(command(lab.cli,['diagnostics','state','inspect','--server',lab.server_file]));assert.deepEqual(after.fences,state.fences);record('offline-inspect-compact-fences');
 evidence(parent,'native-contract',{command:'node scripts/diagnostics-native-check.mjs',native_run_id:lab.run_id,evidence_type:'real_local; no Docker adapter/socket configured; no business services',result:'passed',cases,recovery_result:'all owned native children stopped; main demo unchanged'});
}finally{await stopLab(lab)}
