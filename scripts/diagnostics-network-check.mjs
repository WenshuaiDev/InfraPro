import assert from 'node:assert/strict';
import https from 'node:https';
import { randomBytes, randomUUID } from 'node:crypto';
import { readFileSync, mkdirSync, mkdtempSync, chmodSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { root, loadLab, readJSON, privateJSON, command, freePort, startProcess, stopProcess, saveLab, sleep } from './fixtures/diagnostics-lab.mjs';
import { eventually, evidence } from './fixtures/diagnostics-client.mjs';

// The TLS relay is only an acceptance fault injector. Production has no relay,
// traffic recording, fault-control endpoint, or credential logging.
const parent=loadLab(),directory=mkdtempSync('/private/tmp/infrapro-diag-network-');chmodSync(directory,0o700);
const lab={...parent,run_id:'diag-'+randomBytes(8).toString('hex'),directory,ephemeral:true,processes:{},compose_project:undefined,compose_file:undefined};
for(const name of ['logs','processes','evidence','tls'])mkdirSync(join(directory,name),{mode:0o700});
const internalPort=await freePort(),publicPort=await freePort();lab.origin=`https://127.0.0.1:${publicPort}`;
const tlsDir=join(directory,'tls');command(lab.helper,['cert',tlsDir]);
const ca=readFileSync(join(tlsDir,'ca.pem')),tls={key:readFileSync(join(tlsDir,'server-key.pem')),cert:readFileSync(join(tlsDir,'server.pem'))};
const configDir=join(directory,'server');command(lab.cli,['diagnostics','config','init','--directory',configDir,'--instance-id',lab.run_id,'--profile','native']);
lab.server_file=join(configDir,'server.json');const cfg=readJSON(lab.server_file);
Object.assign(cfg,{listen:`0.0.0.0:${internalPort}`,public_origin:lab.origin,allowed_hosts:[`127.0.0.1:${publicPort}`,`host.docker.internal:${publicPort}`,`127.0.0.1:${internalPort}`],tls:{cert_file:join(tlsDir,'server.pem'),key_file:join(tlsDir,'server-key.pem')}});
privateJSON(lab.server_file,cfg);lab.manifest_file=cfg.manifest_file;lab.accounts_file=join(directory,'accounts.json');command(lab.helper,['auth',lab.server_file,lab.accounts_file]);
const local=readJSON(cfg.local_observer.bindings_file);const ready=local.bindings.find(b=>b.id==='diagnostics-ready');Object.assign(ready,{url:`https://127.0.0.1:${internalPort}/readyz`,tls_ca_file:join(tlsDir,'ca.pem')});privateJSON(cfg.local_observer.bindings_file,local);
const manifest=readJSON(cfg.manifest_file),ids=['native-a','native-b','container-c'];
for(const id of ids){
 const dir=join(directory,id);mkdirSync(dir,{mode:0o700});mkdirSync(join(dir,'state'),{mode:0o700});
 manifest.nodes.push({id,display_name:`本机逻辑测试 ${id}`,kind:'logical_test',resource_scope:'none',assignment_revision:'1',enabled:true});
 const probe=id+'-health';manifest.probes.push({id:probe,node_id:id,target_kind:'node',target_id:id,adapter:'http_live',binding_id:probe,facet:'readiness',required:false,interval_ms:5000,timeout_ms:1000,stale_after_ms:20000});
 const host=id==='container-c'?'host.docker.internal':'127.0.0.1';
 privateJSON(join(dir,'bindings.json'),{schema_version:1,instance_id:lab.run_id,node_id:id,assignment_revision:'1',allowed_target_hosts:['host.docker.internal'],bindings:[{id:probe,adapter:'http_live',url:`https://${host}:${publicPort}/healthz`,tls_ca_file:join(tlsDir,'ca.pem'),expected_status:200}]});
 privateJSON(join(dir,'observer.json'),{schema_version:1,instance_id:lab.run_id,node_id:id,assignment_revision:'1',server_url:`https://${host}:${publicPort}`,manifest_file:cfg.manifest_file,bindings_file:join(dir,'bindings.json'),credential_file:join(dir,'key.json'),state_dir:join(dir,'state'),tls_ca_file:join(tlsDir,'ca.pem')});
}
// Same dependency, two actual network namespaces. 127.0.0.1 reaches the native
// server from the host and is intentionally unreachable from the container.
manifest.components.push({id:'network-source',display_name:'隔离网络视角样本',kind:'application',expectation:'external',min_ready_replicas:0,expectation_reason:'固定依赖视角 fixture'});
for(const id of ['native-a','container-c']){
 const replica=id+'-source',probe=id+'-dependency';manifest.replicas.push({id:replica,component_id:'network-source',node_id:id,expectation:'external',required_probe_ids:[]});
 manifest.dependencies.push({id:probe,from_replica_id:replica,to_component_id:'diagnostics-server',probe_id:probe,required:false,evidence_mode:'shared_network'});
 manifest.probes.push({id:probe,node_id:id,target_kind:'dependency',target_id:probe,adapter:'http_live',binding_id:probe,facet:'readiness',required:false,interval_ms:5000,timeout_ms:1000,stale_after_ms:20000});
 const path=join(directory,id,'bindings.json'),b=readJSON(path);b.bindings.push({id:probe,adapter:'http_live',url:`https://127.0.0.1:${internalPort}/healthz`,tls_ca_file:join(tlsDir,'ca.pem'),expected_status:200});privateJSON(path,b);
}
manifest.revision='2';privateJSON(cfg.manifest_file,manifest);for(const id of ids)command(lab.cli,['diagnostics','observer-key','issue','--server',lab.server_file,'--node-id',id,'--output-file',join(directory,id,'key.json')]);
const modes=new Map(),frames=new Map(),acks=[],events=[];let container;
function request(origin,path,{body,token,cookies,csrf}={}){return new Promise((resolve,reject)=>{
 const req=https.request(origin+path,{ca,method:body===undefined?'GET':'POST',headers:{Origin:lab.origin,...(cookies?{Cookie:cookies}:{}),...(csrf?{'X-CSRF-Token':csrf}:{}),...(token?{Authorization:'Bearer '+token}:{}),...(body===undefined?{}:{'Content-Type':'application/json'})},timeout:8000},res=>{const chunks=[];res.on('data',x=>chunks.push(x));res.on('end',()=>{let value;try{value=JSON.parse(Buffer.concat(chunks))}catch{value=null}resolve({status:res.statusCode,value,headers:res.headers})});});req.on('error',()=>reject(Error('TLS/network request failed')));req.on('timeout',()=>req.destroy());if(body!==undefined)req.write(typeof body==='string'?body:JSON.stringify(body));req.end();
});}
const relay=https.createServer(tls,async(req,res)=>{
 const chunks=[];let size=0;for await(const chunk of req){size+=chunk.length;if(size>1<<20){res.writeHead(413).end();return}chunks.push(chunk)}
 const body=Buffer.concat(chunks).toString();let value;try{value=JSON.parse(body)}catch{ /* Only observer JSON carries a node identity. */ }const node=value?.node_id,mode=modes.get(node);
 if(req.url==='/observer/v1/reports'){
  frames.set(node,{body,token:req.headers.authorization?.slice(7)});
  if(mode==='disconnect'){res.writeHead(503).end();return}
  if(mode==='delay')await sleep(6000);
 }
 try{
  const upstream=await request(`https://127.0.0.1:${internalPort}`,req.url,{body:body||undefined,token:req.headers.authorization?.slice(7)});
  if(req.url==='/observer/v1/reports'&&upstream.status===200){acks.push({node,sequence:upstream.value.accepted_sequence,duplicate:upstream.value.duplicate,received_at:upstream.value.received_at});if(acks.length>128)acks.shift();if(mode==='lose-ack'){modes.delete(node);res.destroy();return}}
  res.writeHead(upstream.status,{'Content-Type':'application/json'}).end(JSON.stringify(upstream.value));
 }catch{if(!res.destroyed)res.writeHead(503).end();}
});
await new Promise(resolve=>relay.listen(publicPort,'0.0.0.0',resolve));
let cookies='',csrf='';
async function login(){const jar=new Map(),call=async(path,body)=>{const r=await request(`https://127.0.0.1:${internalPort}`,'/api/diagnostics/v1'+path,{body,cookies:[...jar].map(([k,v])=>`${k}=${v}`).join('; '),csrf});for(const x of r.headers['set-cookie']??[]){const part=x.split(';')[0],eq=part.indexOf('=');jar.set(part.slice(0,eq),part.slice(eq+1))}assert.equal(r.status,200);csrf=r.value.data.csrf_token;return r};await call('/session/bootstrap');const a=readJSON(lab.accounts_file);await call('/session/login',{username:a.username,password:a.password});cookies=[...jar].map(([k,v])=>`${k}=${v}`).join('; ')}
async function get(path){const r=await request(`https://127.0.0.1:${internalPort}`,'/api/diagnostics/v1'+path,{cookies});assert.equal(r.status,200);return r.value.data}
async function fresh(id){const v=await get('/checks/'+id+'-health');return v.current_observation.freshness==='fresh'&&v.current_observation.outcome==='ok'&&v}
async function start(id){return startProcess(lab,id,lab.binary,['observer','--config',join(directory,id,'observer.json')])}
async function injected(raw,token){return request(`https://127.0.0.1:${internalPort}`,'/observer/v1/reports',{body:raw,token})}
const record=(id,details={})=>{events.push({case:id,result:'passed',...details});console.log(`L3 ${id}: passed`)};
try{
 await startProcess(lab,'server',lab.binary,['server','--config',lab.server_file]);await eventually(async()=>{try{return(await request(`https://127.0.0.1:${internalPort}`,'/healthz')).status===200}catch{return false}});await login();
 for(const id of ids.slice(0,2))await start(id);
 const name='infrapro-'+lab.run_id+'-observer';
 container=command('docker',['run','-d','--name',name,'--user',`${process.getuid()}:${process.getgid()}`,'--memory','128m','--cpus','0.5','--pids-limit','32','--cap-drop','ALL','--security-opt','no-new-privileges:true','--label',`io.infrapro.lab.run_id=${lab.run_id}`,'--mount',`type=bind,source=${directory},target=${directory}`,'--mount',`type=bind,source=${join(root,'build/bin/infrapro-diagnostics-linux')},target=/diagnostics,readonly`,'alpine:3.22','/diagnostics','observer','--config',join(directory,'container-c/observer.json')]).trim();
 for(const id of ids)await eventually(()=>fresh(id));record('three-independent-observers',{native:2,container:1,tls:'short-lived private CA; hostname verified'});
 const host=await get('/checks/native-a-dependency'),isolated=await get('/checks/container-c-dependency');assert.equal(host.current_observation.outcome,'ok');assert.equal(isolated.current_observation.reason_code,'connection_refused');record('dependency-vantage-separation');
 modes.set('native-b','lose-ack');await eventually(()=>acks.some(a=>a.node==='native-b'&&a.duplicate));const dup=acks.findLast(a=>a.node==='native-b'&&a.duplicate),original=acks.find(a=>a.node===dup.node&&a.sequence===dup.sequence&&!a.duplicate);assert.equal(dup.received_at,original.received_at);record('lost-ack-exact-retry');
 await stopProcess(lab,'native-a');await sleep(1100);const saved=frames.get('native-a');const f=JSON.parse(saved.body);const before=await get('/checks/native-a-health');
 for(const [name,change,status] of [
  ['node-impersonation',x=>x.node_id='native-b',403],['unassigned-probe',x=>x.observations[0].probe_id='foreign',403],['partial-frame',x=>x.observations.pop(),403],
  ['same-sequence-different-body',x=>x.observations[0].reason_code='connection_refused',409],['old-sequence',x=>x.sequence=String(BigInt(x.sequence)-1n),409]
 ]){await sleep(1100);const x=structuredClone(f);if(name!=='same-sequence-different-body'&&name!=='old-sequence')x.sequence=String(BigInt(x.sequence)+1n);change(x);const r=await injected(x,saved.token);assert.equal(r.status,status,name);record(name)}
 const oldKey=readJSON(join(directory,'native-a/key.json'));
 const identity={...f};delete identity.observations;delete identity.sequence;delete identity.generated_at;
 let wrong=await request(`https://127.0.0.1:${internalPort}`,'/observer/v1/sessions',{body:{...identity,node_id:'native-b'},token:oldKey.key_id+'.'+oldKey.secret});assert.equal(wrong.status,403);record('key-node-impersonation');
 await sleep(21000);const replay=await injected(saved.body,saved.token);assert.equal(replay.status,200);assert.equal(replay.value.duplicate,true);const after=await get('/checks/native-a-health');assert.equal(after.current_observation.received_at,before.current_observation.received_at);assert.equal(after.current_observation.freshness,'stale');assert.equal((await get('/nodes/native-a')).observer.online,false);assert(await fresh('container-c'));record('duplicate-does-not-renew-age-or-online');
 const handshake=body=>request(`https://127.0.0.1:${internalPort}`,'/observer/v1/sessions',{body,token:oldKey.key_id+'.'+oldKey.secret});
 assert.equal((await handshake({...identity,boot_id:randomUUID()})).status,409);record('same-epoch-different-boot-rejected');
 const advanced={...identity,epoch:String(BigInt(identity.epoch)+1n),boot_id:randomUUID()};assert.equal((await handshake(advanced)).status,200);assert.equal((await handshake(identity)).status,409);assert.notEqual((await injected(saved.body,saved.token)).status,200);record('old-epoch-old-token-old-handshake-rejected');
 const epochPath=join(directory,'native-a/state/epoch.json');unlinkSync(epochPath);await start('native-a');await sleep(7000);assert.equal((await get('/nodes/native-a')).observer.online,false);await stopProcess(lab,'native-a');record('lost-observer-state-fenced');
 const keyPath=join(directory,'native-a/key.json');unlinkSync(keyPath);command(lab.cli,['diagnostics','observer-key','rotate','--server',lab.server_file,'--node-id','native-a','--output-file',keyPath]);await sleep(5500);await start('native-a');await eventually(()=>fresh('native-a'));wrong=await request(`https://127.0.0.1:${internalPort}`,'/observer/v1/sessions',{body:identity,token:oldKey.key_id+'.'+oldKey.secret});assert.equal(wrong.status,403);assert.notEqual((await injected(saved.body,saved.token)).status,200);record('generation-rotation-recovers-rejects-old-key-token');
 const current=frames.get('native-a');await stopProcess(lab,'native-a');await sleep(21000);const cached=JSON.parse(current.body);cached.sequence=String(BigInt(cached.sequence)+1n);cached.generated_at=new Date().toISOString();for(const o of cached.observations)o.sample_age_ms=Math.max(o.sample_age_ms,Date.now()-Date.parse(o.observed_at));assert.equal((await injected(cached,current.token)).status,200);assert.equal((await get('/checks/native-a-health')).current_observation.freshness,'stale');record('same-attempt-new-frame-remains-stale');
 const rollback=structuredClone(cached);rollback.sequence=String(BigInt(cached.sequence)+1n);for(const o of rollback.observations)o.sample_age_ms=0;await sleep(1100);assert.equal((await injected(rollback,current.token)).status,400);record('sample-age-rollback-rejected');
 const future=structuredClone(cached);future.sequence=String(BigInt(cached.sequence)+1n);future.generated_at=new Date(Date.now()+60000).toISOString();for(const o of future.observations){o.attempt_id=randomUUID();o.observed_at=future.generated_at;o.sample_age_ms=0}await sleep(1100);assert.equal((await injected(future,current.token)).status,200);assert.equal((await get('/checks/native-a-health')).current_observation.freshness,'clock_uncertain');record('future-clock-never-fresh');
 const past=structuredClone(future);past.sequence=String(BigInt(future.sequence)+1n);past.generated_at=new Date(Date.now()-60000).toISOString();for(const o of past.observations){o.attempt_id=randomUUID();o.observed_at=past.generated_at}await sleep(1100);assert.equal((await injected(past,current.token)).status,400);assert.equal((await get('/checks/native-a-health')).current_observation.freshness,'clock_uncertain');record('clock-backward-rejected');await start('native-a');await eventually(()=>fresh('native-a'));
 modes.set('native-b','disconnect');await sleep(22000);assert.equal((await get('/nodes/native-b')).observer.online,false);assert(await fresh('container-c'));record('partition-isolated-and-stale');modes.delete('native-b');await eventually(()=>fresh('native-b'),60000);record('partition-recovery-current-snapshot');
 modes.set('native-b','delay');await sleep(7000);assert(await fresh('container-c'));modes.delete('native-b');record('slow-network-isolated');
 const priorBoot=(await get('/self')).server_boot_id;await stopProcess(lab,'native-a');await stopProcess(lab,'server');await startProcess(lab,'server',lab.binary,['server','--config',lab.server_file]);await sleep(600);await login();assert.notEqual((await get('/self')).server_boot_id,priorBoot);assert.equal((await get('/checks/native-a-health')).current_observation.freshness,'historical');record('server-restart-cache-historical');await start('native-a');for(const id of ids)await eventually(()=>fresh(id),60000);record('server-restart-and-durable-fences');
 // Each TLS negative case uses a real observer and a real TLS listener. No
 // credentials reach its HTTP handler when certificate validation rejects it.
 await stopProcess(lab,'native-a');const observerPath=join(directory,'native-a/observer.json'),originalConfig=readJSON(observerPath);
 for(const variant of ['hostname','expired','wrong-ca']){
  const d=join(directory,'tls-'+variant);mkdirSync(d,{mode:0o700});command(lab.helper,['cert',d,variant==='wrong-ca'?'valid':variant]);let requests=0;const bad=https.createServer({key:readFileSync(join(d,'server-key.pem')),cert:readFileSync(join(d,'server.pem'))},(_,res)=>{requests++;res.end('{}')});await new Promise(resolve=>bad.listen(0,'127.0.0.1',resolve));
  privateJSON(observerPath,{...originalConfig,server_url:`https://127.0.0.1:${bad.address().port}`,tls_ca_file:join(variant==='wrong-ca'?tlsDir:d,'ca.pem')});await start('native-a');await sleep(3500);await stopProcess(lab,'native-a');assert.equal(requests,0,variant);await new Promise(resolve=>bad.close(resolve));record('tls-reject-'+variant);
 }
 privateJSON(observerPath,originalConfig);await start('native-a');await eventually(()=>fresh('native-a'));record('final-recovery');
 evidence(parent,'network-contract',{evidence_type:'real_local/real-container/fault_injection',command:'node scripts/diagnostics-network-check.mjs',network_run_id:lab.run_id,environment:{architecture:process.arch,transport:'verified TLS',container_image:'alpine:3.22'},result:'passed',cases:events,recovery_result:'all three observers fresh'});
}finally{
 for(const name of Object.keys(lab.processes).reverse())await stopProcess(lab,name);
 if(container){const labels=JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',container]));assert.equal(labels['io.infrapro.lab.run_id'],lab.run_id);command('docker',['rm','-f',container]);}
 relay.closeAllConnections();await new Promise(resolve=>relay.close(resolve));lab.state='stopped';saveLab(lab);
}
