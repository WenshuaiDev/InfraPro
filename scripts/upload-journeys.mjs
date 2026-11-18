import { requireAuthenticatedLogin } from './login-result.mjs';
// Isolated production-handler journeys through the shipped Nginx HTTP template.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, openSync, closeSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { randomUUID, createHash } from 'node:crypto';
import net from 'node:net';
import http from 'node:http';
import assert from 'node:assert/strict';
import { request } from '@playwright/test';
import { testEnv } from './test-environment.mjs';
import { pack } from '../packages/ts/display-devtools/src/package.mjs';

const root=resolve('.'),out=join(root,'.artifacts/nginx-uploads/journeys');mkdirSync(out,{recursive:true});rmSync(join(out,'result.json'),{force:true});
const children=[];let client;
const hash=b=>createHash('sha256').update(b).digest('hex');
const meta=()=>({request_id:randomUUID(),reason:'上传部署合成自检'});
async function freePort(){const s=net.createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const p=s.address().port;await new Promise(r=>s.close(r));return p;}
function start(cmd,args,env={},cwd=root){const fd=openSync(join(out,`process-${children.length}.log`),'w');const p=spawn(cmd,args,{cwd,env:{...process.env,...env},stdio:['ignore',fd,fd]});closeSync(fd);children.push(p);return p;}
const apiPort=await freePort(),port=await freePort(),runPort=await freePort(),assetPort=await freePort(),origin=`http://127.0.0.1:${port}`;
try{
 execFileSync('go',['test','-c','-o',join(out,'api-test'),'./services/api/internal/transport/http'],{stdio:'inherit'});
 start(join(out,'api-test'),['-test.run','^TestE2EServer$','-test.v','-test.timeout','20m'],{...testEnv,INFRAPRO_E2E_SERVER:'1',INFRAPRO_E2E_UPLOAD_LIMITS:'1',INFRAPRO_DISPLAY_E2E:'1',INFRAPRO_FILES_E2E:'1',INFRAPRO_MONITORING_E2E:'1',INFRAPRO_SPATIAL_E2E:'1',INFRAPRO_HTTP_ADDR:`127.0.0.1:${apiPort}`,INFRAPRO_E2E_CONSOLE_URL:origin,INFRAPRO_E2E_ORIGINS:origin,INFRAPRO_E2E_RUNTIME_ADDR:`127.0.0.1:${runPort}`,INFRAPRO_E2E_ASSET_ADDR:`127.0.0.1:${assetPort}`,INFRAPRO_E2E_RUNTIME_ORIGIN:`http://127.0.0.1:${runPort}`,INFRAPRO_E2E_ASSET_ORIGIN:`http://127.0.0.1:${assetPort}`,INFRAPRO_SPATIAL_PYTHON:join(root,'.artifacts/spatial-venv/bin/python'),INFRAPRO_SPATIAL_CONVERTER_SCRIPT:join(root,'tools/spatial-converter/convert.py'),INFRAPRO_FILES_CHECKER_EXECUTABLE:join(root,'.artifacts/nginx-uploads/files-checker'),INFRAPRO_UPLOADS_SPATIAL_MAX_FILE_BYTES:String(270<<20),INFRAPRO_UPLOADS_MONITORING_CSV_MAX_BYTES:String(64*1024**3),INFRAPRO_UPLOADS_MONITORING_XLSX_MAX_BYTES:String(1024**3),INFRAPRO_UPLOADS_DISPLAY_MAX_PACKAGE_BYTES:String(160<<20)},join(root,'services/api/internal/transport/http'));
 let ready=false;for(let n=0;n<180;n++){try{if((await fetch(`http://127.0.0.1:${apiPort}/healthz`)).ok){ready=true;break;}}catch{/* starting */}await new Promise(r=>setTimeout(r,500));}assert.ok(ready,'fixture startup');
 const template=readFileSync('deploy/nginx/default.conf','utf8').replace('listen 80;',`listen 127.0.0.1:${port};`).replaceAll('api:8080',`127.0.0.1:${apiPort}`);
 const conf=join(out,'nginx.conf');writeFileSync(conf,`pid ${out}/nginx.pid;\nerror_log ${out}/nginx.log;\nevents {}\nhttp { ${template} }\n`);
 execFileSync('nginx',['-t','-p',out+'/','-c',conf],{stdio:'inherit'});start('nginx',['-p',out+'/','-c',conf,'-g','daemon off;']);
 client=await request.newContext({baseURL:origin,timeout:240000});
 let csrf='';
 async function call(path,data,method=data===undefined?'GET':'POST',extra={}){
  const r=await client.fetch('/api/v1'+path,{method,headers:{Origin:origin,'X-CSRF-Token':csrf,'X-Command-ID':randomUUID(),...extra},...(data===undefined?{}:{data})});
  const raw=await r.body(),status=r.status();await r.dispose();assert.ok(status>=200&&status<300,`${method} ${path}: ${status} ${raw.toString().slice(0,800)}`);return raw.length?JSON.parse(raw):{};
 }
 csrf=(await call('/auth/csrf')).token;requireAuthenticatedLogin(await call('/auth/login',{username:'e2e_owner',password:'Real browser 河流 secret 592'}));
 const limits=await call('/auth/upload-limits');assert.equal(limits.spatial_max_file_bytes,String(270<<20));assert.equal(limits.display_max_package_bytes,String(160<<20));
 const owner=(await call('/access/accounts?q=e2e_owner')).items.find(a=>a.username==='e2e_owner');
 const tenant=(await call('/platform/tenants',{name:'上传部署验收',code:'upload-deploy',administrator_account_id:owner.id,client_request_id:randomUUID(),reason:'合成验收'})).tenant_id;
 const member=(await call(`/tenants/${tenant}/members`)).items.find(m=>m.account.id===owner.id);
 const project=(await call(`/tenants/${tenant}/projects`,{name:'上传验收',code:'upload',purpose:'合成验收',client_request_id:randomUUID(),reason:'合成验收',owner_tenant_membership_id:member.id,administrator_tenant_membership_id:member.id})).project_id;
 const base=`/tenants/${tenant}`,results=[];
 async function poll(path,select){for(let n=0;n<600;n++){const v=await call(path);if(select(v))return v;await new Promise(r=>setTimeout(r,250));}throw Error('worker timeout '+path);}
 async function download(path,expected){const r=await client.get('/api/v1'+path);const b=await r.body();assert.equal(r.status(),200);assert.equal(hash(b),hash(expected));await r.dispose();}
 async function part(path,b,headers={}){await call(path,b,'PUT',{'Content-Type':'application/octet-stream','X-Content-SHA256':hash(b),...headers});}
 async function slowPart(path,b,headers){
  const cookies=(await client.storageState()).cookies.map(c=>`${c.name}=${c.value}`).join(';');
  await new Promise((resolve,reject)=>{
   const req=http.request(origin+'/api/v1'+path,{method:'PUT',headers:{Origin:origin,Cookie:cookies,'X-CSRF-Token':csrf,'Content-Type':'application/octet-stream','Content-Length':b.length,'X-Content-SHA256':hash(b),...headers}},res=>{const chunks=[];res.on('data',c=>chunks.push(c));res.on('end',()=>{if(res.statusCode===200)resolve();else reject(Error(`slow upload: ${res.statusCode} ${Buffer.concat(chunks)}`));});});
   const timer=setTimeout(()=>req.end(b.subarray(1)),16000);req.on('error',e=>{clearTimeout(timer);reject(e);});req.write(b.subarray(0,1));
  });
 }
 // Ordinary file exceeds the previous 100 MiB database/checker/backup boundary.
 {
  const bytes=Buffer.alloc(101<<20,120),caps=await call(base+'/files/capabilities'),headers={'X-Files-Epoch':caps.restore_epoch};
  assert.equal(caps.max_file_bytes,String(110<<20));
  const created=await call(base+'/files/uploads',{...meta(),host:caps.host,intent:'add',filename:'large.txt',display_name:'大文件验收',description:'',size_bytes:String(bytes.length),sha256:hash(bytes),declared_media_type:'text/plain'},'POST',headers);
  const path=base+`/files/uploads/${created.upload_id}`;
  for(let off=0;off<bytes.length;off+=8<<20){const b=bytes.subarray(off,off+(8<<20)),i=off/(8<<20)+1;if(i===1)await slowPart(path+`/parts/${i}`,b,headers);else await part(path+`/parts/${i}`,b,headers);if(i===1){const resumed=await call(path);assert.equal(resumed.parts.length,1);await part(path+'/parts/1',b,headers);}}
  const done=await call(path+'/complete',{...meta(),expected_revision:created.revision},'POST',headers);
  const job=await poll(base+`/files/jobs/${done.job_id}`,v=>{assert.notEqual(v.state,'failed',JSON.stringify(v));return v.state==='succeeded';});
  await download(base+`/files/attachments/${job.result_attachment_id}/revisions/${job.result_revision_id}/content`,bytes);
  console.log("Completed upload module");results.push({module:'files',bytes:bytes.length,resume:true,worker:true,download:true});
 }
 // Monitoring two-part source: source download validates the seal without an unrelated data import.
 {
  const bytes=Buffer.alloc((32<<20)+1,32);bytes.write('observed_at,value\n');
  let batch=await call(base+'/monitoring/imports',{...meta(),expected_revision:'0',name:'large.csv',size_bytes:String(bytes.length)});
  const path=base+`/monitoring/imports/${batch.id}`,headers={'X-Expected-Revision':batch.revision};
  await part(path+'/upload/parts/1',bytes.subarray(0,32<<20),headers);assert.equal((await call(path)).parts.length,1);await part(path+'/upload/parts/1',bytes.subarray(0,32<<20),headers);await part(path+'/upload/parts/2',bytes.subarray(32<<20),headers);
  batch=await call(path+'/upload/complete',{...meta(),expected_revision:batch.revision,sha256:hash(bytes)});
  await download(base+`/monitoring/source-files/${batch.source_file_id}/download`,bytes);
  console.log("Completed upload module");results.push({module:'monitoring',bytes:bytes.length,resume:true,download:true});
 }
 // Spatial: more than 32 chunks and the previous 256 MiB input/DB boundary.
 {
  const type=await call('/platform/engineering/type-definitions',{...meta(),code:'upload_type',content:{metamodel_version:'1',name:'上传验收设施',description:'',release_notes:'',fields:[],components:[],relations:[]}});
  const published=await call(`/platform/engineering/type-definitions/${type.definition_id}/publish`,{...meta(),revision:type.revision});
  const facility=await call(base+'/engineering/resources/facilities',{request_id:randomUUID(),code:'UPLOAD',name:'上传验收设施',type_version_id:published.version_id,values:{}});
  const asset=await call(base+'/spatial/assets',{...meta(),facility_id:facility.id,kind:'geolayer',code:'UPLOAD',name:'上传验收图件',category:'general',source_note:''});
  const version=await call(base+`/spatial/assets/${asset.id}/versions`,{...meta(),expected_revision:asset.revision,base_version_id:'',new_source:false});
  const source=readFileSync('samples/spatial/engineering/bridge.geojson'),bytes=Buffer.alloc(257<<20,32);source.copy(bytes);
  const created=await call(base+'/spatial/uploads',{...meta(),expected_revision:version.revision,version_id:version.id,filename:'large.geojson',size:bytes.length,sha256:hash(bytes)}),path=base+`/spatial/uploads/${created.id}`;
  for(let off=0;off<bytes.length;off+=8<<20){const b=bytes.subarray(off,off+(8<<20)),i=off/(8<<20)+1;await part(path+`/parts/${i}`,b);if(i===1){assert.equal((await call(path)).state,'uploading');await part(path+'/parts/1',b);}}
  const current=await call(path);await call(path+'/complete',{...meta(),expected_revision:current.revision});
  const v=await call(base+`/spatial/versions/${version.id}/manifest`),job=await call(base+`/spatial/versions/${version.id}/process`,{...meta(),expected_revision:v.revision});
  await poll(base+`/spatial/jobs/${job.id}`,v=>{assert.notEqual(v.state,'failed',JSON.stringify(v));return v.state==='succeeded';});
  await download(base+`/spatial/versions/${version.id}/source`,bytes);
  console.log("Completed upload module");results.push({module:'spatial',bytes:bytes.length,resume:true,worker:true,download:true});
 }
 // Display: ZIP is stored, so total exceeds the old 128 MiB/32-part bound.
 {
  const dir=join(out,'display-source');mkdirSync(dir,{recursive:true});writeFileSync(join(dir,'index.html'),'<!doctype html><title>Upload acceptance</title>');
  for(let i=0;i<5;i++)writeFileSync(join(dir,`data-${i}.bin`),Buffer.alloc(27<<20,65+i));
  const manifest=JSON.parse(readFileSync('samples/display-sdk/display.manifest.json'));manifest.required_capabilities={};manifest.optional_capabilities={};manifest.binding_schema={};manifest.config_schema={type:'object',properties:{}};
  const manifestPath=join(out,'display.manifest.json');writeFileSync(manifestPath,JSON.stringify(manifest));const zip=join(out,'large.zip');await pack(dir,manifestPath,zip);const bytes=readFileSync(zip);
  const path=base+`/projects/${project}/displays`;let result=await call(path+'/settings',{...meta(),expected_revision:'0',enabled:true},'PUT');
  result=await call(path+'/instances',{...meta(),expected_revision:'0',name:'上传验收大屏'});
  const instance=result.id,created=await call(path+`/instances/${instance}/uploads`,{...meta(),expected_revision:result.revision,size:bytes.length,sha256:hash(bytes)}),up=path+`/instances/${instance}/uploads/${created.upload_id}`;
  for(let off=0;off<bytes.length;off+=4<<20){const b=bytes.subarray(off,off+(4<<20)),i=off/(4<<20);await part(up+`/parts/${i}`,b);if(i===0){assert.equal((await call(up)).parts.length,1);await part(up+'/parts/0',b);}}
  await call(up+'/complete',{...meta(),expected_revision:created.revision});
  await poll(up,v=>{assert.ok(!v.jobs.some(j=>j.state==='failed'),JSON.stringify(v.jobs));return v.jobs.some(j=>j.state==='ready');});
  console.log("Completed upload module");results.push({module:'display',bytes:bytes.length,resume:true,worker:true});
 }
 writeFileSync(join(out,'result.json'),JSON.stringify({passed:true,productionDeployment:false,limits,results},null,2)+'\n');console.log(JSON.stringify(results));
}finally{await client?.dispose();for(const p of children.reverse()){p.kill('SIGTERM');await Promise.race([new Promise(r=>p.once('exit',r)),new Promise(r=>setTimeout(r,15000))]);}}
