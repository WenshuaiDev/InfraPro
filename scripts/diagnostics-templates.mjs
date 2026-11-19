import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
const check=process.argv.includes('--check'),temp=mkdtempSync(join(tmpdir(),'diagnostics-templates-'));
try{
 for(const profile of ['native','docker','mixed']){
  const dir=join(temp,profile);execFileSync('go',['run','./tools/infrapro/cmd/infrapro','diagnostics','config','init','--directory',dir,'--instance-id','example','--profile',profile],{stdio:'pipe'});
  const cfg='/etc/infrapro/diagnostics',state='/var/lib/infrapro-diagnostics';
  const read=name=>JSON.parse(readFileSync(join(dir,name),'utf8'));
  const server=read('server.json'),manifest=read('manifest.json'),bindings=read('bindings.json');
  Object.assign(server,{manifest_file:cfg+'/manifest.json',auth_file:cfg+'/auth.json',node_keys_file:cfg+'/node-keys.json',state_dir:state});server.local_observer.bindings_file=cfg+'/bindings.json';
  Object.assign(bindings.bindings[0],{pid_file:state+'/process.json',allowed_root:state,expected_executable:'/usr/local/bin/infrapro-diagnostics',expected_uid:10001});bindings.bindings[2].mount_paths[0].path=state;
  if(profile==='docker'){
   Object.assign(manifest.nodes[0],{kind:'container',resource_scope:'container',display_name:'诊断 server 容器'});bindings.bindings[2].scope='container';
   Object.assign(server,{listen:'0.0.0.0:8096',public_origin:'https://diagnostics.example.invalid:8096',allowed_hosts:['diagnostics.example.invalid:8096','127.0.0.1:8096'],tls:{cert_file:cfg+'/tls/server.pem',key_file:cfg+'/tls/server-key.pem'}});
   Object.assign(bindings.bindings[1],{url:'https://127.0.0.1:8096/readyz',tls_ca_file:cfg+'/tls/ca.pem'});
  }
  if(profile!=='native'){
   const n=manifest.nodes.find(n=>n.id==='docker-observer');Object.assign(n,{enabled:true,display_name:'独立网络观察器',kind:'container',resource_scope:'container'});
   manifest.probes.push({id:'observer-resources',node_id:n.id,target_kind:'node',target_id:n.id,adapter:'host_resources',binding_id:'observer-resources',facet:'resource',required:false,interval_ms:10000,timeout_ms:1000,stale_after_ms:30000});
  }
  const files={'server.json':server,'manifest.json':manifest,'bindings.json':bindings,'auth.json':read('auth.json'),'node-keys.json':read('node-keys.json')};
  if(profile!=='native'){
   files['observer.json']={schema_version:1,instance_id:'example',node_id:'docker-observer',assignment_revision:'1',server_url:'https://diagnostics.example.invalid:8096',manifest_file:'/etc/infrapro/observer/manifest.json',bindings_file:'/etc/infrapro/observer/bindings.json',credential_file:'/etc/infrapro/observer/key.json',state_dir:'/var/lib/infrapro-observer',tls_ca_file:'/etc/infrapro/observer/ca.pem'};
   files['observer-bindings.json']={schema_version:1,instance_id:'example',node_id:'docker-observer',assignment_revision:'1',bindings:[{id:'observer-resources',adapter:'host_resources',scope:'container',mount_paths:[{id:'observer-state',path:'/var/lib/infrapro-observer'}],network_interfaces:[]}]};
  }
  const splitDir=join(temp,profile+'-split');
  const args=['run','./tools/infrapro/cmd/infrapro','diagnostics','config','init','--directory',splitDir,'--instance-id','example','--profile',profile,'--topology','split'];
  if(profile!=='native')args.push('--container-report-directory','/var/lib/infrapro-architecture/reports');
  execFileSync('go',args,{stdio:'pipe'});
  const split=name=>JSON.parse(readFileSync(join(splitDir,name),'utf8'));
  const splitManifest=split('manifest.json'),splitLocal=split('bindings.json');
  const additional=splitManifest.nodes.filter(n=>!manifest.nodes.some(existing=>existing.id===n.id));
  splitManifest.nodes=[...manifest.nodes,...additional];
  for(const node of additional)files['split-observer-'+node.id+'-bindings.json']=split('observer-'+node.id+'-bindings.json');
  splitLocal.bindings.splice(0,3,...bindings.bindings.slice(0,3));
  if(profile!=='native'){
    splitManifest.probes.push(manifest.probes.find(p=>p.id==='observer-resources'));
    const reader=split('observer-bindings.json');
    reader.bindings.push(...files['observer-bindings.json'].bindings);
    files['split-observer-bindings.json']=reader;
  }
  files['split-manifest.json']=splitManifest;
  files['split-bindings.json']=splitLocal;
  for(const [name,value]of Object.entries(files)){const path=join('deploy/diagnostics',profile,name),text=JSON.stringify(value,null,2)+'\n';if(check){if(!existsSync(path)||readFileSync(path,'utf8')!==text)throw Error('Deployment template drift: '+path)}else{mkdirSync(join('deploy/diagnostics',profile),{recursive:true});writeFileSync(path,text)}}
 }
 console.log('Diagnostics native/docker/mixed templates '+(check?'checked':'generated'));
}finally{rmSync(temp,{recursive:true,force:true})}
