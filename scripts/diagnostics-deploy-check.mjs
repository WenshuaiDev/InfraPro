import assert from 'node:assert/strict';
import https from 'node:https';
import { mkdirSync, mkdtempSync, chmodSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadLab, readJSON, privateJSON, command, freePort } from './fixtures/diagnostics-lab.mjs';
import { eventually, evidence } from './fixtures/diagnostics-client.mjs';
const lab=loadLab(),dir=mkdtempSync('/private/tmp/diagnostics-compose-');chmodSync(dir,0o700);const config=join(dir,'config'),state=join(dir,'state');for(const d of[config,state,join(config,'tls')])mkdirSync(d,{recursive:true,mode:0o700});
const port=await freePort(),origin=`https://127.0.0.1:${port}`,project='infrapro-'+lab.run_id+'-standalone';
const server=readJSON('deploy/diagnostics/docker/server.json');Object.assign(server,{public_origin:origin,allowed_hosts:[`127.0.0.1:${port}`,'127.0.0.1:8096']});
for(const name of['manifest.json','bindings.json','auth.json','node-keys.json'])privateJSON(join(config,name),readJSON(join('deploy/diagnostics/docker',name)));
const b=readJSON(join(config,'bindings.json'));b.bindings[0].expected_uid=process.getuid();privateJSON(join(config,'bindings.json'),b);privateJSON(join(config,'server.json'),server);command(lab.helper,['cert',join(config,'tls')]);
// The admin helper writes only the owned fixture's local auth file. Runtime
// still uses the template's container paths and has no host credentials.
const native=structuredClone(server);Object.assign(native,{listen:`127.0.0.1:${port}`,auth_file:join(config,'auth.json'),manifest_file:join(config,'manifest.json'),node_keys_file:join(config,'node-keys.json'),state_dir:state,local_observer:null});privateJSON(join(dir,'admin.json'),native);command(lab.helper,['auth',join(dir,'admin.json'),join(dir,'accounts.json')]);
const subnet = process.env.DIAGNOSTICS_STANDALONE_SUBNET ??
  (await import('./architecture-http-network.mjs')).unusedDockerSubnet('docker', '10.243', Number.parseInt(lab.run_id.slice(-2), 16));
assert(/^10\.243\.(?:0|[1-9]\d{0,2})\.0\/24$/.test(subnet) && Number(subnet.split('.')[2]) < 256);
privateJSON(join(dir,'override.json'),{
  services:{server:{labels:{'io.infrapro.lab.run_id':lab.run_id}}},
  networks:{default:{ipam:{config:[{subnet}]},labels:{'io.infrapro.lab.run_id':lab.run_id}}},
});
const env={...process.env,DIAGNOSTICS_CONFIG_DIR:config,DIAGNOSTICS_STATE_DIR:state,DIAGNOSTICS_UID:String(process.getuid()),DIAGNOSTICS_GID:String(process.getgid()),DIAGNOSTICS_PORT:String(port)};
const compose=args=>command('docker',['compose','-p',project,'-f','deploy/diagnostics/compose.yaml','-f',join(dir,'override.json'),...args],{env});
const ca=readFileSync(join(config,'tls/ca.pem'));let csrf='';const jar=new Map();
function request(path,body){return new Promise((resolve,reject)=>{const req=https.request(origin+path,{ca,method:body===undefined?'GET':'POST',headers:{Origin:origin,Cookie:[...jar].map(([k,v])=>k+'='+v).join('; '),...(body===undefined?{}:{'Content-Type':'application/json','X-CSRF-Token':csrf})},timeout:5000},res=>{const chunks=[];for(const cookie of res.headers['set-cookie']??[]){const p=cookie.split(';')[0],at=p.indexOf('=');jar.set(p.slice(0,at),p.slice(at+1))}res.on('data',c=>chunks.push(c));res.on('end',()=>{const text=Buffer.concat(chunks).toString();let value;try{value=JSON.parse(text)}catch{ /* HTML is checked separately. */ }if(value?.data?.csrf_token)csrf=value.data.csrf_token;resolve({status:res.statusCode,value,text})})});req.on('error',reject);req.on('timeout',()=>req.destroy());if(body)req.write(JSON.stringify(body));req.end()})}
let result;
try{
 compose(['up','-d','server']);await eventually(async()=>{try{return(await request('/healthz')).status===200}catch{return false}});
 const id=compose(['ps','-q','server']).trim(),inspect=JSON.parse(command('docker',['inspect','--format','{{json .}}',id]));assert.equal(inspect.Config.Labels['io.infrapro.lab.run_id'],lab.run_id);assert(!inspect.Mounts.some(m=>m.Destination.includes('docker.sock')));assert.notEqual(inspect.Config.User,'0');assert.equal(inspect.HostConfig.Memory,268435456);
 assert.equal((await request('/api/diagnostics/v1/session/bootstrap')).status,200);const a=readJSON(join(dir,'accounts.json'));assert.equal((await request('/api/diagnostics/v1/session/login',{username:a.username,password:a.password})).status,200);const html=await request('/');assert.equal(html.status,200);assert(!/ui-not-built|@vite\/client|https?:\/\//.test(html.text));
 const node=await eventually(async()=>{const v=(await request('/api/diagnostics/v1/nodes/local')).value?.data;return v?.resource_series_summary.length&&v});assert.equal(node.node.resource_scope,'container');assert.equal(node.node.kind,'container');
 assert.equal((await request('/assets/absent.js')).status,404);assert.equal((await request('/api/diagnostics/v1/absent')).status,404);
 const version=JSON.parse(command('docker',['exec',id,'infrapro-diagnostics','version','--verify-assets']));assert(version.ui_ready);
 const [networkID] = Object.values(inspect.NetworkSettings.Networks).map(network => network.NetworkID);
 const [network] = JSON.parse(command('docker', ['network','inspect',networkID]));
 assert.equal(network.Labels['com.docker.compose.project'],project);
 assert.equal(network.Labels['io.infrapro.lab.run_id'],lab.run_id);
 assert(network.IPAM.Config.some(entry=>entry.Subnet===subnet));
 result={command:'node scripts/diagnostics-deploy-check.mjs',result:'passed',evidence_type:'real-container using delivered Compose template',image:inspect.Image,build_id:version.build_id,user:inspect.Config.User,server_socket_mount:false,dependency_services:[],resource_scope:node.node.resource_scope,network:{id:networkID,project,subnet,actual_address:inspect.NetworkSettings.Networks[network.Name].IPAddress}};
}finally{compose(['down','--volumes','--remove-orphans'])}
assert.equal(command('docker',['network','ls','-q','--filter','id='+result.network.id]).trim(),'');
assert.equal(compose(['ps','-aq']).trim(),'');
result.network.removed=true;result.recovery_result='owned standalone Compose project and network removed';
evidence(lab,'deployment',result);console.log('Standalone production Compose template passed');
