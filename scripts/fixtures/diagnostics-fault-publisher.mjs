import { randomUUID } from 'node:crypto';
import { join } from 'node:path';
import { readJSON, privateJSON, sleep } from './diagnostics-lab.mjs';
const [file,name]=process.argv.slice(2),lab=readJSON(file);if(!lab.faults.includes(name)||!['clock-skew','stale-replay'].includes(name))throw Error('No owned fault authorization');
const dir=join(lab.directory,'native-c'),cfg=readJSON(join(dir,'observer.json')),key=readJSON(cfg.credential_file),manifest=readJSON(cfg.manifest_file),state=readJSON(join(cfg.state_dir,'epoch.json'));
state.epoch=String(BigInt(state.epoch)+1n);privateJSON(join(cfg.state_dir,'epoch.json'),state);
const identity={protocol_major:1,instance_id:cfg.instance_id,node_id:cfg.node_id,manifest_revision:manifest.revision,assignment_revision:cfg.assignment_revision,epoch:state.epoch,boot_id:randomUUID()};
async function post(path,body,token){const r=await fetch(lab.origin+path,{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+token},body:JSON.stringify(body),signal:AbortSignal.timeout(5000)});if(!r.ok)throw Error('Owned fault publication rejected: '+r.status);return r.json()}
const session=await post('/observer/v1/sessions',identity,key.key_id+'.'+key.secret);let sequence=BigInt(session.last_accepted_sequence),frame;
for(;;){if(!frame||name==='clock-skew'){const at=new Date(Date.now()+(name==='clock-skew'?60000:0)).toISOString();frame={...identity,sequence:String(++sequence),generated_at:at,observations:manifest.probes.filter(p=>p.node_id===cfg.node_id).map(p=>({probe_id:p.id,attempt_id:randomUUID(),observer_boot_id:identity.boot_id,observed_at:at,sample_age_ms:0,duration_ms:0,outcome:'ok',facets:{presence:'not_applicable',readiness:'ready',loop:'not_applicable'},reason_code:'ok',metrics:[],evidence:{source:'protocol',vantage_node_id:cfg.node_id,scope:'none',codes:[]}}))}}await post('/observer/v1/reports',frame,session.token);await sleep(5000)}
