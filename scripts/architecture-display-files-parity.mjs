// Exercise newly created file and display resources in two isolated copies of
// the frozen business snapshot. The ledger contains templates, status, digest
// and checked facts. Session cookies, permit URLs and pairing codes stay in RAM.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, randomUUID, randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs';
import { basename,join,resolve } from 'node:path';
import { chromium, expect, request } from '@playwright/test';
import { client, launch, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { stop } from './fixtures/parsing-lab.mjs';
import { pairedCandidate, withHTTPPair } from './architecture-http-sandbox.mjs';
import { roleEnvironment } from './architecture-runtime.mjs';
import { configEnvironment } from './architecture-walkthrough-runtime.mjs';

const selection=process.argv[3]??'both', output=runDirectory(process.argv[4]??process.argv[2]);
function sourceDirectory(){
  if(!process.argv.includes('--sandboxed'))return runDirectory(process.argv[2]);
  assert.equal(selection,'checker');
  const source=resolve(process.argv[2]),parent=runDirectory(resolve(source,'../../..'));
  const label=basename(source);assert.match(label,/^file-checker-[a-f0-9]{12}$/);
  assert.equal(source,join(parent,'http','sandboxes',label));
  assert.equal(label,'file-checker-'+read(join(source,'private/lab.json')).id);
  return source;
}
const dir=sourceDirectory();
const seed=process.argv.includes('--sandboxed')?resolve(dir,'../../..'):dir;
const release=()=>pairedCandidate(existsSync(join(seed,'candidate.json'))?seed:output);
assert.ok(['both','display','files','checker'].includes(selection));
const lab=read(join(dir,'private/lab.json')), fixture=read(join(dir,'samples.json')), accounts=read(join(dir,'private/accounts.json'));
const run=randomUUID().slice(0,8), privateDir=join(dir,'private','display-files-'+run);
if(!process.argv.some(v=>v==='--archive'||v==='--merge'))mkdirSync(privateDir,{recursive:true,mode:0o700});
const started=new Date(), sides={};
const catalog=readFileSync(join(root,'services/api/internal/routes/ownership.tsv'),'utf8').trimEnd().split('\n').map(line=>{
  const [method,template,owner,operationId]=line.split('\t');return {method,template,owner,operationId,pattern:new RegExp('^'+template.split('/').map(p=>p.startsWith(':')?'[^/]+':p).join('/')+'$')};
});
const keys=new Map();
const key=name=>{if(!keys.has(name))keys.set(name,randomUUID());return keys.get(name);};
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const commonExpiry=new Date(Date.now()+3600_000).toISOString();
const meta=(label,expected_revision)=>({request_id:key(label),reason:'架构配对验收合成记录',expected_revision});
const template=(method,path)=>catalog.find(r=>r.method===method&&r.pattern.test(path))?.template;
const beforeBuild=Object.fromEntries(Object.entries(process.env).filter(([k])=>!k.startsWith('INFRAPRO_')));

function record(side,label,method,route,status,checked={},details={}){
  assert(!sides[side].some(r=>r.label===label),'duplicate scenario '+label);
  sides[side].push({label,method,template:route,status,checked,command:key(label),...details});
  if(selection==='checker')write(join(privateDir,side+'-checks.json'),sides[side]);
}
async function checkedRequest(state,label,method,path,{data,headers={},status=200,verify,client:other,surface,route}={}){
  const url=(surface??state.side.api)+path;
  const endpoint=route??template(method,path.split('?')[0]);
  assert.ok(endpoint,'unmapped public route '+method+' '+path.split('?')[0]);
  const allowed=Array.isArray(status)?status:[status],readOnly=['GET','HEAD'].includes(method);
  const transient=[];
  for(;;){
    let response;
    try{
      response=await (other??state.admin.client).fetch(url,{method,headers:{...(!surface?state.admin.headers:{}),'X-Command-ID':key(label),...headers},data,timeout:30000,maxRedirects:0});
    }catch(e){
      if(e.code==='ECONNRESET'&&readOnly&&transient.length<5){
        transient.push({type:'connection_reset',time:new Date().toISOString()});continue;
      }
      throw Error([state.name,label,method,endpoint,'transport',e.code??e.name].join(' '),{cause:e});
    }
    try{
      const body=await response.body(),h=response.headers(),actual=response.status();
      let doc;if(h['content-type']?.includes('json')&&body.length)doc=JSON.parse(body);
      if(!allowed.includes(actual)){
        // An integration run can share a recovering object store. Preserve its
        // first response and retry only an idempotent read that named the
        // existing storage-unavailable error, within a bounded interval.
        const temporary=['file_storage_unavailable','avatar_storage_unavailable'].includes(doc?.code)||(method==='HEAD'&&!body.length);
        if(actual===503&&readOnly&&temporary&&transient.length<5){
          transient.push({type:'object_store',status:actual,code:doc?.code??null,time:new Date().toISOString()});
          await sleep(Math.min(8000,500*2**transient.length));continue;
        }
        const paths=(doc?.issues??[]).map(i=>i.path).filter(Boolean).join(',');
        throw Error([state.name,label,method,endpoint,'got',actual,doc?.code,paths].filter(Boolean).join(' '));
      }
      const accepted=verify?await verify(doc,body,h):{};
      record(state.name,label,method,endpoint,actual,accepted??{},{errorCode:actual>=400?doc?.code??null:null,sha256:sha(body),transient});
      return {doc,body,h,status:actual};
    }finally{await response.dispose();}
  }
}
const json=async(s,l,m,path,options)=>(await checkedRequest(s,l,m,path,options)).doc;
const publicPath=s=>'/api/v1/tenants/'+s.tenant+'/projects/'+s.project;
async function workers(state) {
  const names=['display-package-worker','files-worker','files-checker'].filter(n=>selection==='both'||(selection==='display'?n.startsWith('display'):!n.startsWith('display')));
  const bin=state.name==='a'?read(join(dir,'baseline.json')).bin:release().bin;
  for(const name of names)assert.ok(existsSync(join(bin,name)),name+' missing from the selected release');
  const restricted=join(dir,'private',state.name,'credentials','roles','files-worker','runtime.json');
  const cfg=read(selection==='checker'&&existsSync(restricted)?restricted:state.side.config),tmp=mkdtempSync('/tmp/ip-arc-check-'),socket=join(tmp,'check.sock');
  assert.match(new URL(cfg.database_url).hostname,/^(127\.0\.0\.1|localhost)$/);
  cfg.files={...cfg.files,temp_directory:tmp,checker_socket:socket};
  const path=join(privateDir,state.name+'-worker.json');write(path,cfg);
  for(const name of names.toReversed()){
    let env={...beforeBuild,INFRAPRO_CONFIG:path,GOMAXPROCS:'2'};
    if(state.name==='b'&&name==='files-checker')env={...beforeBuild,GOMAXPROCS:'2'};
    else if(state.name==='b'&&selection!=='checker'){
      const topology=join(seed,'topology');
      assert.ok(existsSync(join(topology,'roles',name,'environment.json')),name+' role login required');
      env={...roleEnvironment(name,configEnvironment(cfg),topology),GOMAXPROCS:'2'};
      assert.notEqual(new URL(env.INFRAPRO_DATABASE_URL).username,new URL(cfg.database_url).username);
    }else if(state.name==='b'){
      assert.ok(existsSync(restricted),'File-checker fault requires the files-worker login');
      env.INFRAPRO_ARCHITECTURE_FILE=join(seed,'topology','roles',name,'architecture.json');
    }
    const start=()=>{
      const process=launch(join(bin,name),name==='files-checker'?['--directory',tmp,'--socket',socket]:[],env,join(privateDir,state.name+'-'+name+'.log'));
      state.procs.push(process);return process;
    };
    const child=start();if(name==='files-checker')state.checker={child,start};
  }
}
async function newProject(s) {
  s.tenant=fixture.structure.tenant;
  const base='/api/v1/tenants/'+s.tenant;
  const membership=(await s.admin.call('/tenants/'+s.tenant+'/context')).membership_id;
  const project=await json(s,'new-project','POST',base+'/projects',{status:200,data:{name:'消息切分配对项目 '+run,code:'arc-'+run,purpose:'普通文件和大屏验收',owner_tenant_membership_id:membership,administrator_tenant_membership_id:membership,client_request_id:key('new-project'),reason:'隔离验收范围'}});
  s.project=project.project_id;
  assert.match(s.project,/^[\da-f-]{36}$/);
}
async function files(s,scope) {
  let tenant=s.tenant;
  if(scope==='tenant'){
    const created=await json(s,'file-private-tenant','POST','/api/v1/platform/tenants',{data:{name:'配对附件 '+run,code:'fa-'+run,administrator_account_id:fixture.actors.admin,client_request_id:key('file-private-tenant'),reason:'独立附件作用域'},status:201});
    tenant=created.tenant_id;s.fileTenant=tenant;
  }
  const prefix='file-'+scope+'-', base=(scope==='tenant'?'/api/v1/tenants/'+tenant:publicPath(s))+'/files';
  const get=(l,m,p,opt)=>json(s,prefix+l,m,base+p,opt);
  const caps=await get('capabilities','GET','/capabilities');
  const headers={'X-Files-Epoch':caps.restore_epoch};
  const data=Buffer.alloc(selection==='checker'?256:8*1024*1024+817,0x61);Buffer.from('InfraPro paired file\n').copy(data);
  const input={request_id:key(prefix+'create'),reason:'真实文件校验',host:caps.host,intent:'add',filename:'paired.txt',display_name:'配对文件 '+run,description:'合成 UTF-8 文本',size_bytes:String(data.length),sha256:sha(data),declared_media_type:'text/plain'};
  const upload=await get('create','POST','/uploads',{status:201,data:input,headers});
  const uid=upload.upload_id,path='/uploads/'+uid;
  await get('create-replay','POST','/uploads',{status:201,data:input,headers,verify:d=>{assert.equal(d.upload_id,uid);assert.equal(d.replayed,true);return{replayed:true}}});
  const part=async(i,label,bytes)=>get(label,'PUT',path+'/parts/'+i,{data:bytes,headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha(bytes)}});
  await part(1,'part-one',data.subarray(0,8388608));await get('resume','GET',path);
  await part(1,'part-replay',data.subarray(0,8388608));if(data.length>8388608)await part(2,'part-two',data.subarray(8388608));
  const complete=await get('complete','POST',path+'/complete',{status:202,headers,data:meta(prefix+'complete',upload.revision)});
  let job;
  for(let n=0;n<240;n++){
    job=await s.admin.call(base.slice(7)+'/jobs/'+complete.job_id);
    if(['succeeded','failed','cancelled','needs_action'].includes(job.state))break;
    await sleep(250);
  }
  assert.equal(job.state,'succeeded',prefix+'check job: '+job.state+'/'+(job.reason_code??''));
  const aid=job.result_attachment_id,rev=job.result_revision_id;
  await get('job','GET','/jobs/'+complete.job_id,{verify:d=>({state:d.state})});
  await get('request','GET','/requests/'+input.request_id);
  await get('uploads','GET','/uploads');
  await get('attachments','GET','/attachments');
  await get('attachment','GET','/attachments/'+aid);
  await get('revisions','GET','/attachments/'+aid+'/revisions');
  const content=base+'/attachments/'+aid+'/revisions/'+rev+'/content';
  let tag;
  if(selection!=='checker'){
  const whole=await checkedRequest(s,prefix+'full','GET',content,{verify:(_,bytes,h)=>{assert.ok(bytes.equals(data));assert.match(h['cache-control'],/no-store/);assert.match(h['content-disposition'],/^attachment/);return{bytes:bytes.length,sha256:sha(bytes),nosniff:h['x-content-type-options']}}});
  tag=whole.h.etag;
  for(const [label,method,headers,status,expected] of [
    ['head','HEAD',{},200,0],['range','GET',{Range:'bytes=7-31'},206,25],['suffix','GET',{Range:'bytes=-33'},206,33],
    ['cached','GET',{'If-None-Match':tag},304,0],['stale','GET',{'If-Match':'"previous"'},412,0],['multi-range','GET',{Range:'bytes=0-1,4-5'},416,0]]){
    await checkedRequest(s,prefix+label,method,content,{headers,status,verify:(_,b,h)=>{assert.equal(b.length,expected);if(label==='range')assert.ok(data.subarray(7,32).equals(b));else if(label==='suffix')assert.ok(data.subarray(-33).equals(b));return {bytes:b.length,sha256:sha(b),contentRange:h['content-range']??null}}});
  }
  const cookie=(await s.admin.client.storageState()).cookies.map(c=>c.name+'='+c.value).join('; ');
  const ac=new AbortController(),response=await fetch(s.side.api+content,{headers:{cookie},signal:ac.signal});
  const reader=response.body.getReader(),first=await reader.read();assert.equal(response.status,200);assert.ok(first.value.length>0&&first.value.length<data.length);
  ac.abort();await reader.cancel().catch(()=>{});
  record(s.name,prefix+'interrupt','GET',template('GET',content),200,{interruptedBeforeEnd:true});
  await checkedRequest(s,prefix+'resume-download','GET',content,{headers:{Range:'bytes=33-'},status:206,verify:(_,b)=>{assert.ok(data.subarray(33).equals(b));return{bytes:b.length,sha256:sha(b)}}});
  }
  if(scope==='tenant'){
    const test=Buffer.from('checker outage evidence\n');
    const started=await get('checker-upload','POST','/uploads',{status:201,headers,data:{...input,request_id:key(prefix+'checker-upload'),filename:'checker.txt',size_bytes:String(test.length),sha256:sha(test)}});
    const uploadPath='/uploads/'+started.upload_id;
    await get('checker-part','PUT',uploadPath+'/parts/1',{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha(test)},data:test});
    await stop(s.checker.child);assert(s.checker.child.exitCode!==null||s.checker.child.signalCode);
    async function waitJob(id,states){
      for(let i=0;i<220;i++){
        const job=await s.admin.call(base.slice(7)+'/jobs/'+id);
        if(states.includes(job.state))return job;
        if(['failed','succeeded','cancelled','needs_action'].includes(job.state))throw Error('unexpected file job state '+job.state+' '+job.error_code);
        await sleep(200);
      }
      throw Error('checker test did not reach '+states.join('/'));
    }
    const verifyAttempt=(id,generation)=>{
      assert.match(id,/^[0-9a-f-]{36}$/);assert.match(generation,/^\d+$/);
      const db=new URL(read(s.side.config).database_url);
      assert.equal(db.hostname,'127.0.0.1');assert.match(db.pathname,/^\/arc_(?:[ab]|http_[a-f0-9]{12}_[ab])$/);
      const sql="SELECT count(*) FROM file_job_attempts WHERE job_id='"+id+"' AND generation="+generation+" AND outcome='retry_wait' AND error_code='file_worker_unavailable'";
      const count=Number(execFileSync('docker',['exec',lab.container,'psql','-XAtq','-U','archtest','-d',db.pathname.slice(1),'-c',sql],{encoding:'utf8'}));
      assert(count>=1,'no durable failed checker attempt');return true;
    };
    let parked;
    try{
      const accepted=await get('checker-accept','POST',uploadPath+'/complete',{headers,data:meta(prefix+'checker-accept',started.revision),status:202});
      parked=await waitJob(accepted.job_id,['retry_wait']);
      assert(!parked.result_attachment_id);
      await get('checker-isolated-job','GET','/jobs/'+accepted.job_id,{verify:v=>{
        assert(!v.result_attachment_id);return{accepted:true,waitingForChecker:verifyAttempt(parked.id,parked.generation),attached:false};
      }});
      await checkedRequest(s,prefix+'checker-existing-bytes','GET',content,{headers:{Range:'bytes=0-31'},status:206,verify:(_,b)=>{assert.deepEqual(b,data.subarray(0,32));return{oldBytes:sha(b)}}});
    }finally{s.checker.child=s.checker.start();}
    const resolved=await json(s,prefix+'checker-task','GET','/api/v1/tasks/resolve?module=files&kind=job&source_id='+parked.id);
    assert(resolved.task_id);
    const recovered=await waitJob(parked.id,['succeeded']);
    assert.equal(recovered.generation,parked.generation);assert(recovered.attempt>parked.attempt);
    await get('checker-recovered','GET','/jobs/'+parked.id,{verify:v=>({state:v.state,generation:v.generation,attached:!!v.result_attachment_id})});
    const restored=base+'/attachments/'+recovered.result_attachment_id+'/revisions/'+recovered.result_revision_id+'/content';
    await checkedRequest(s,prefix+'checker-recovered-body','GET',restored,{verify:(_,body)=>{assert.deepEqual(body,test);return{sha256:sha(body)}}});
  }
  if(selection==='checker')return;
  const cross=content.replace(tenant,fixture.secondTenant);
  await checkedRequest(s,prefix+'cross-tenant','GET',cross,{status:[403,404],verify:(_,b,h)=>{assert(!h['content-range']&&!h.etag);return{exposesFileHeaders:false}}});
  const target='/attachments/'+aid;
  const original=await get('before-metadata','GET',target);
  const replacement=Buffer.from('deterministic replacement for '+scope+'\n');
  const replace=await get('start-replace','POST','/uploads',{status:201,headers,data:{...input,request_id:key(prefix+'start-replace'),intent:'replace',attachment_id:aid,expected_attachment_revision:original.revision,size_bytes:String(replacement.length),sha256:sha(replacement)}});
  await get('replace-part','PUT','/uploads/'+replace.upload_id+'/parts/1',{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha(replacement)},data:replacement});
  await get('edit','PATCH',target,{headers,data:{...meta(prefix+'edit',original.revision),display_name:'改名后的配对资料',description:'并发元数据修订'}});
  const submitted=await get('replace-complete','POST','/uploads/'+replace.upload_id+'/complete',{status:202,headers,data:meta(prefix+'replace-complete',replace.revision)});
  const waitJob=async(id,goal)=>{
    let j;
    for(let n=0;n<180;n++){j=await s.admin.call(base.slice(7)+'/jobs/'+id);if(j.state===goal||['failed','cancelled'].includes(j.state))break;await sleep(250)}
    assert.equal(j.state,goal);return j;
  };
  const stalled=await waitJob(submitted.job_id,'needs_action'), updated=await get('updated','GET',target);
  await get('reconfirm','POST','/uploads/'+replace.upload_id+'/reconfirm',{status:[200,202],headers,data:{request_id:key(prefix+'reconfirm'),reason:'按当前版本确认覆盖',expected_generation:stalled.generation,expected_control_revision:stalled.control_revision,expected_attachment_revision:updated.revision}});
  await waitJob(submitted.job_id,'succeeded');
  const changed=await get('replaced','GET',target);
  assert.notEqual(changed.current_revision.id,rev);
  await get('revert','POST',target+'/revert',{headers,data:{...meta(prefix+'revert',changed.revision),revision_id:rev}});
  const reverted=await get('reverted','GET',target);
  const restoredBody=await checkedRequest(s,prefix+'reverted-content','GET',base+target+'/revisions/'+reverted.current_revision.id+'/content',{verify:(_,b)=>{assert(data.equals(b));return{sha256:sha(b)}}});assert(restoredBody.body.length===data.length);
  const rootAttachment='/attachments/'+aid;
  let detail=await get('before-trash','GET',rootAttachment);
  await get('trash','POST',rootAttachment+'/trash',{headers,data:meta(prefix+'trash',detail.revision)});
  await checkedRequest(s,prefix+'trashed-content','GET',content,{status:[403,404,409,410],headers:{'If-None-Match':tag}});
  detail=await get('trashed','GET',rootAttachment);
  await get('restore','POST',rootAttachment+'/restore',{headers,data:meta(prefix+'restore',detail.revision)});
  const partial=await get('start-cancel','POST','/uploads',{data:{...input,request_id:key(prefix+'start-cancel')},headers,status:201});
  await get('cancel','POST','/uploads/'+partial.upload_id+'/cancel',{headers,data:meta(prefix+'cancel',partial.revision)});
  const png=readFileSync(join(root,'samples/spatial/bridge-plan.png'));
  const image=await get('image-upload','POST','/uploads',{status:201,headers,data:{...input,request_id:key(prefix+'image-upload'),filename:'bridge.png',display_name:'配对工程图',declared_media_type:'image/png',sha256:sha(png),size_bytes:String(png.length)}});
  const imgPath='/uploads/'+image.upload_id;
  await get('image-part','PUT',imgPath+'/parts/1',{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha(png)},data:png});
  const accepted=await get('image-complete','POST',imgPath+'/complete',{status:202,headers,data:meta(prefix+'image-complete',image.revision)});
  let img;
  for(let n=0;n<180;n++){img=await s.admin.call(base.slice(7)+'/jobs/'+accepted.job_id);if(img.state==='succeeded')break;await sleep(250)}
  assert.equal(img.state,'succeeded');
  const previewPath=base+'/attachments/'+img.result_attachment_id+'/revisions/'+img.result_revision_id+'/preview';
  const preview=await checkedRequest(s,prefix+'image-preview','GET',previewPath,{verify:(_,b,h)=>{assert.match(h['content-type'],/^image\//);return {sha256:sha(b),type:h['content-type'],bytes:b.length}}});
  await checkedRequest(s,prefix+'image-preview-head','HEAD',previewPath,{verify:(_,b,h)=>{assert.equal(b.length,0);return{length:Number(h['content-length'])}}});
  await checkedRequest(s,prefix+'image-preview-range','GET',previewPath,{headers:{Range:'bytes=0-19'},status:206,verify:(_,b)=>{assert(preview.body.subarray(0,20).equals(b));return{sha256:sha(b)}}});

  await engineeringAttachments(s,scope,tenant,img.result_attachment_id,img.result_revision_id,png,preview.body);

}

async function avatars(s){
  const base='/api/v1', tenant=base+'/tenants/'+s.fileTenant;
  const username='av_'+run, password='P6!'+randomBytes(20).toString('base64url');
  const reason='配对成员头像验收';
  await checkedRequest(s,'file-avatar-admin-reauthenticate','POST',base+'/auth/reauthenticate',{status:204,data:{password:accounts.admin.password}});
  const invite=await json(s,'file-avatar-registration','POST',base+'/invitations',{data:{note:reason}});
  const token=new URLSearchParams(new URL(invite.url).hash.slice(1)).get('token');
  const guest=await client(s.side);
  try {
    await checkedRequest(s,'file-avatar-register','POST',base+'/public/invitations/accept',{status:204,client:guest.client,headers:guest.headers,data:{username,password,display_name:'头像成员 '+run,token}});
  }finally{await guest.client.dispose();}
  const peer=await client(s.side,{username,password}), own={client:peer.client,headers:peer.headers};
  try {
    const current=await peer.call('/auth/me'), accountID=current.account.id;
    const invitation=await json(s,'file-avatar-invite-member','POST',tenant+'/member-invitations',{status:201,data:{target_account_id:accountID,client_request_id:key('file-avatar-invite-member'),reason}});
    const joined=await json(s,'file-avatar-accept-membership','POST',base+'/me/tenant-invitations/'+invitation.invitation.id+'/accept',{...own,data:{expected_version:invitation.invitation.version}});
    const manager=(await s.admin.call(tenant.slice(7)+'/context')).membership_id;
    const project=await json(s,'file-avatar-create-project','POST',tenant+'/projects',{data:{name:'头像授权 '+run,code:'av-'+run,purpose:reason,owner_tenant_membership_id:manager,administrator_tenant_membership_id:manager,client_request_id:key('file-avatar-create-project'),reason}});
    const proj=tenant+'/projects/'+project.project_id;
    const pm=await json(s,'file-avatar-project-member','POST',proj+'/members',{data:{expected_project_version:project.project_version,client_request_id:key('file-avatar-project-member'),tenant_membership_id:joined.membership_id,reason}});
    const file='/api/v1/account/avatar', png=readFileSync(join(root,'samples/spatial/bridge-plan.png'));
    const updated=await json(s,'file-avatar-upload','PUT',file+'?expected_revision=0',{client:peer.client,headers:{...peer.headers,'Content-Type':'image/png'},data:png,verify:d=>{assert(d.has_avatar);assert.equal(d.avatar_revision,'1');return{has_avatar:true,revision:d.avatar_revision};}});
    const version='?version='+updated.avatar_revision;
    let digest;
    for(const [label,path,actor] of [
      ['self',file,own],['platform',base+'/access/accounts/'+accountID+'/avatar',{}],
      ['tenant',tenant+'/members/'+joined.membership_id+'/avatar',{}],
      ['project',proj+'/members/'+pm.object_id+'/avatar',{}],
    ]){
      await checkedRequest(s,'file-avatar-'+label,'GET',path+version,{...actor,verify:(_,bytes,h)=>{
        assert.match(h['content-type'],/^image\/png/);assert.deepEqual([...bytes.subarray(0,8)],[137,80,78,71,13,10,26,10]);
        assert.match(h['cache-control'],/no-store/);assert.equal(h['x-content-type-options'],'nosniff');
        if(digest)assert.equal(sha(bytes),digest);else digest=sha(bytes);
        return{bytes:bytes.length,sha256:digest,revision:updated.avatar_revision};
      }});
    }
    await checkedRequest(s,'file-avatar-wrong-tenant','GET',base+'/tenants/'+s.tenant+'/members/'+joined.membership_id+'/avatar'+version,{status:[403,404]});
    await checkedRequest(s,'file-avatar-stale','DELETE',file+'?expected_revision=0',{...own,status:409});
    await json(s,'file-avatar-remove','DELETE',file+'?expected_revision='+updated.avatar_revision,{...own,verify:d=>{assert(!d.has_avatar);return{has_avatar:false,revision:d.avatar_revision}}});
    await checkedRequest(s,'file-avatar-old-revision','GET',file+version,{...own,status:404});
  }finally{await peer.call('/auth/logout',{}).catch(()=>{});await peer.client.dispose();}
}

async function engineeringAttachments(s,scope,tenant,attachment,revision,original,preview){
  const project=scope==='project',prefix='engineering-file-'+scope+'-';
  const root='/api/v1/tenants/'+tenant,facilities='/engineering/resources/facilities';
  const tenantRoot=root+facilities,base=(project?publicPath(s):root)+facilities;
  const input=(label,extra)=>({request_id:key(prefix+label),reason:'新建工程附件引用并核对工程授权',...extra});
  const node=await json(s,prefix+'create-facility','POST',tenantRoot,{status:201,data:{request_id:key(prefix+'facility'),code:'ARC'+run+'F'+scope[0],name:'配对附件设施',type_version_id:fixture.structure.types.A.version_id,values:{}}});
  const id=node.id,nodes=[{kind:'facility',path:'',id}];
  for(const [kind,group,fields] of [['component','components',{category_key:'span'}],['monitoring_section','monitoring-sections',{}],['borehole','boreholes',{borehole:{measured_depth_m:12}}],['point','points',{}]]){
    const f=await s.admin.call(tenantRoot.slice(7)+'/'+id);
    const added=await json(s,prefix+'create-'+kind,'POST',tenantRoot+'/'+id+'/'+group,{status:201,data:{request_id:key(prefix+'create-'+kind),code:'AR'+run+kind[0],name:'附件工程对象 '+kind,parent_ref:{kind:'facility',id},expected_structure_revision:f.structure_revision,...fields}});
    nodes.push({kind,path:'/'+group+'/'+added.id,id:added.id});
  }
  let revisions={};
  if(project){
    const access=publicPath(s)+'/resource-access/facilities/'+id;
    const current=await s.admin.call(access.slice(7));
    const g=input('grant',{action:'create_link',expected_access_revision:current.policy.revision,add:nodes.map(n=>({kind:n.kind,...(n.kind==='facility'?{}:{[n.kind+'_id']:n.id}),capabilities:['read','update']}))});
    const p=await json(s,prefix+'grant-preview','POST',access+'/preview',{data:g});await json(s,prefix+'grant','POST',access+'/changes',{data:{...g,preview_hash:p.preview_hash}});
    s.attachmentFacility=id;
    await json(s,prefix+'grant-receipt','GET',publicPath(s)+'/resource-access/requests/'+g.request_id);
    const result=await s.admin.call(access.slice(7));revisions={expected_access_revision:result.policy.revision,expected_link_revision:result.link.revision};
  }
  for(const n of nodes){
    const path=base+'/'+id+n.path, ref=await s.admin.call(path.slice(7)), owner=await s.admin.call(base.slice(7)+'/'+id);
    const command=input('attach-'+n.kind,{action:'attachment',expected_revision:ref.revision,expected_structure_revision:owner.structure_revision,...revisions,attachment:{attachment_id:attachment,attachment_revision_id:revision},purpose:'固定工程证据'});
    const pending=await json(s,prefix+n.kind+'-preview-ref','POST',path+'/structure-records-preview',{data:command});
    await json(s,prefix+n.kind+'-add-ref','POST',path+'/structure-records',{data:{...command,preview_hash:pending.preview_hash}});
    const details=await json(s,prefix+n.kind+'-records','GET',path+'/structure-details?kind=attachments');
    const bound=details.items.find(v=>v.attachment_revision_id===revision);assert(bound);
    for(const [variant,expected] of [['content',original],['preview',preview]]){
      const endpoint=path+'/attachments/'+bound.id+'/attachment-'+variant;
      await checkedRequest(s,prefix+n.kind+'-'+variant,'GET',endpoint,{verify:(_,b)=>{assert(b.equals(expected));return {bytes:b.length,sha256:sha(b),same_file_revision:true}}});
      await checkedRequest(s,prefix+n.kind+'-'+variant+'-range','GET',endpoint,{headers:{Range:'bytes=2-11'},status:206,verify:(_,b)=>{assert(expected.subarray(2,12).equals(b));return{bytes:b.length,sha256:sha(b)}}});
      // A valid evidence reference used with a different resource still has
      // to pass both the resource scope and the original file's host guard.
      const peer=nodes.find(v=>v.kind===n.kind?false:v.kind==='facility'||n.kind==='facility');
      const other=base+'/'+id+peer.path+'/attachments/'+bound.id+'/attachment-'+variant;
      await checkedRequest(s,prefix+n.kind+'-'+variant+'-wrong-resource','GET',other,{status:[403,404]});
    }
  }
}

async function side(name) {
  sides[name]=[];const target=lab[name],state={name,side:target,procs:[]};
  for(const key of selection==='checker'?['api','origin']:['api','origin','runtime','assets'])assert.equal(new URL(target[key]).hostname,'127.0.0.1','isolated loopback fixture required');
  state.admin=await client(target,accounts.admin);
  try {
    await workers(state);
    if(selection==='checker'){await files(state,'tenant');return;}
    await newProject(state);
    if(selection!=='display'){await files(state,'tenant');await files(state,'project');await avatars(state);}
    if(selection!=='files')await displays(state);
  }finally{
    await state.admin.call('/auth/logout',{}).catch(()=>{});await state.admin.client.dispose();
    await Promise.all(state.procs.map(stop));
  }
}
function save(complete=false){
  const entries=(sides.a??[]).map(a=>{
    const b=sides.b?.find(v=>v.label===a.label);
    const equal=!!b&&a.method===b.method&&a.template===b.template&&a.status===b.status&&a.errorCode===b.errorCode&&JSON.stringify(a.checked)===JSON.stringify(b.checked);
    return {scenario:a.label,method:a.method,template:a.template,command:a.command,status:[a.status,b?.status??null],errors:[a.errorCode??null,b?.errorCode??null],transient:[a.transient??[],b?.transient??[]],sha256:[a.sha256??null,b?.sha256??null],facts:[a.checked,b?.checked??null],equal};
  });
  const path=join(dir,'http','display-files-'+selection+'-parity.json');mkdirSync(join(dir,'http'),{mode:0o700,recursive:true});
  const result={run,started:started.toISOString(),baseline:read(join(dir,'baseline.json')).commit,candidate:release().inputs.sourceSHA256,selection,complete,cases:entries,passed:complete&&entries.length>0&&entries.every(v=>v.equal)};
  write(join(dir,'http','sandboxes','display-files-'+run+'-'+selection+'.json'),result);
  if(result.passed){write(path,result);archive(result);}
  return result;
}
function archive(result){
  assert(result.complete&&result.passed,"Only completed paired trials can enter the acceptance ledger");
  const source=join(dir,'http','paired','display-files-'+result.run+'-'+selection+'.json');
  if(!existsSync(source))write(source,result);
  if(output!==dir)write(join(output,'http','display-files-'+selection+'-parity.json'),{...result,sourceRun:dir,source});
  const records=result.cases.flatMap(row=>{
    const op=catalog.find(v=>v.method===row.method&&v.template===row.template);
    if(!op)return [];
    return [{method:row.method,template:row.template,operationId:op.operationId,owner:op.owner,command:row.command??row.scenario,scenario:row.scenario,
      status:row.status,errors:row.errors??row.facts.map(v=>v?.reason??null),sha256:row.sha256??[],differences:row.equal?[]:['status, error or checked fact'],effects:row.facts}];
  });
  const label='display-files-'+selection;
  write(join(output,'http/scenarios',label+'.json'),{scenario:label,baselineCommit:result.baseline,sourceRun:dir,source,inputs:'两个隔离数据库的同一冻结样本及同一调用参数',records,failures:[]});
  const proof=[['ARC-013','普通附件与工程引用',row=>row.scenario.startsWith('file-')||row.scenario.startsWith('engineering-file-')],
    ['ARC-027','文件检查器停机时旧字节仍可读，恢复后原任务重试',row=>row.scenario.startsWith('file-tenant-checker-')],
    ['ARC-015','程序包、授权、终端与大屏运行期',row=>row.scenario.startsWith('display-')],
    ['ARC-030','工程作用域、已撤回运行授权与错误来源',row=>/cross-tenant|wrong-resource|wrong-origin|revoked-session|old-permit|config-outside/.test(row.scenario)]];
  const cases=proof.flatMap(([arc,name,contains])=>{
    const matched=result.cases.filter(contains);return matched.length?[{arc,name,passed:matched.every(r=>r.equal),source,caseIds:matched.map(r=>r.command??r.scenario),checks:matched.map(r=>r.scenario)}]:[];
  });
  write(join(output,'http/workflows',label+'.json'),{cases});
  if(selection==='checker')write(join(output,'faults/checker-outage.json'),{sourceRun:dir,candidateSHA256:result.candidate,appliedRoleSQL:read(join(dir,'snapshot.json')).appliedRoleSQL,cases:cases.filter(c=>c.arc==='ARC-027')});
}
if(selection==='checker'&&!process.argv.includes('--sandboxed')){
  await withHTTPPair(dir,'file-checker',async sandbox=>{
    execFileSync(process.execPath,[join(root,'scripts/architecture-display-files-parity.mjs'),sandbox,'checker',output,'--sandboxed'],{
      stdio:'inherit',
    });
  },{restrictedRoles:['identity-api','control-api','monitoring-query-api','transfer-api','files-worker']});
}else if(process.argv.includes('--merge')){
  assert.equal(selection,'both');
  const parts=['files','display'].map(mode=>{
    const part=read(join(dir,'http','display-files-'+mode+'-parity.json'));
    assert(part.complete&&part.passed&&part.selection===mode);
    return part;
  });
  assert.equal(parts[0].baseline,parts[1].baseline);
  assert.equal(parts[0].candidate,parts[1].candidate);
  const cases=parts.flatMap(part=>part.cases.map(row=>row.scenario==='new-project'?{...row,scenario:part.selection+'-setup-project',originalScenario:row.scenario}:row));
  assert.equal(new Set(cases.map(c=>c.scenario)).size,cases.length);
  const combined={run:parts.map(p=>p.run).join('-'),started:parts.map(p=>p.started).sort()[0],baseline:parts[0].baseline,candidate:parts[0].candidate,selection,complete:true,passed:cases.every(v=>v.equal),cases,parts:parts.map(p=>join(dir,'http/paired','display-files-'+p.run+'-'+p.selection+'.json'))};
  write(join(dir,'http','display-files-both-parity.json'),combined);archive(combined);
  console.log('Merged independent verified paired workflows',cases.length);
}else if(process.argv.includes('--archive')){
  archive(read(join(dir,'http','display-files-'+selection+'-parity.json')));
}else try{
  const tried=await Promise.allSettled(['a','b'].map(async name=>{
    try{await side(name);}catch(error){
      const location=error.stack?.match(/architecture-display-files-parity\.mjs:(\d+):\d+/)?.[1];
      const last=sides[name]?.at(-1)?.label??'start';
      throw Error([name,'after',last,location&&'line '+location,String(error.message).split('\n')[0]].filter(Boolean).join(' '),{cause:error});
    }
  }));
  const failed=tried.find(v=>v.status==='rejected');
  if(failed)throw failed.reason;
  const result=save(true);assert(result.passed,'A/B response or asserted business fact differs');console.log(selection+' paired assertions '+result.cases.length);
}catch(e){
  save();
  // Browser transport errors may include request headers and one-time URLs.
  const detail=String(e.message??e.name).split('\n')[0].replace(/https?:\/\/\S+/g,'[loopback endpoint]');
  console.error('Paired acceptance failed',detail.slice(0,260));process.exitCode=1;
}

// Use the real runtime's one-time code exchange and paired verifier, then let
// the installed package establish its MessageChannel and perform SDK reads.
async function browserSDK(s, format, prefix, authorize, instance){
  const browser=await chromium.launch({headless:true});
  try{
    const context=await browser.newContext();
    const page=await context.newPage(),assetHeaders=[];
    page.on('request',req=>{
      if(new URL(req.url()).origin===s.side.assets)assetHeaders.push(req.allHeaders());
    });
    const verifier=randomBytes(32).toString('hex'),issued=await authorize('browser-launch','/launch',{challenge:sha(verifier),audience:s.side.runtime});
    await page.addInitScript(({origin,nonce})=>{if(location.origin===origin&&location.pathname==='/exchange')sessionStorage.setItem('display_verifier',nonce);},{origin:s.side.runtime,nonce:verifier});
    await page.goto(s.side.runtime+'/exchange#code='+encodeURIComponent(issued.code));
    await expect(page.locator('#status')).toHaveText('已连接 · 应用已就绪',{timeout:30000});
    const iframe=page.locator('iframe:not(.preparing)');
    await expect(iframe).toHaveAttribute('sandbox','allow-scripts');
    const app=page.frameLocator('iframe:not(.preparing)');
    if(format===1){
      await expect(app.locator('#output')).toContainText(fixture.structure.nodes.A.name??'设施',{timeout:15000});
    }else{
      await expect(app.getByRole('heading',{name:'配对兼容',exact:true})).toBeVisible({timeout:15000});
    }
    const blocked=await app.locator('body').evaluate(()=>{try{void document.cookie;return false}catch{return true}});
    assert(blocked,'sandbox package can read the parent cookie jar');
    const session=await page.evaluate(()=>sessionStorage.getItem('display_session'));
    assert.match(session,/^[\da-f-]{36}$/);
    assert(!page.url().includes('#'),'pairing code remained in browser address');
    const headers=await Promise.all(assetHeaders);assert(headers.length>0);
    assert(headers.every(h=>!h.authorization&&!h.cookie),'browser asset request carried credentials');
    record(s.name,prefix+'sdk-frame','GET','/display/:instance',200,{manifest_format:format,message_channel_ready:true,sandbox:'allow-scripts',opaque_cookie_blocked:true,asset_credentials_sent:false});
    // Re-enter the canonical route in the same tab. The shell bootstraps the
    // same person via its scoped runtime cookie and the previous session ID.
    await page.goto(s.side.runtime+'/display/'+instance);
    await expect(page.locator('#status')).toHaveText('已连接 · 应用已就绪',{timeout:30000});
    await context.close();
    return session;
  }finally{await browser.close();}
}

async function displays(s){
  const ledger=publicPath(s),facility=fixture.structure.nodes.A.id, extra=fixture.structure.nodes.C1.id;
  const access=ledger+'/resource-access/facilities/'+facility;
  const previous=await json(s,'display-resource-policy','GET',access);
  const proposal={request_id:key('link'),reason:'仅向本次新项目公开两个合成工程对象',action:'create_link',expected_access_revision:previous.policy.revision,
    add:[{kind:'facility',capabilities:['read']},{kind:'component',component_id:extra,capabilities:['read']}]};
  const preview=await json(s,'display-link-preview','POST',access+'/preview',{data:proposal});
  await json(s,'display-link-apply','POST',access+'/changes',{data:{...proposal,preview_hash:preview.preview_hash}});
  const otherFacility=fixture.structure.nodes.B.id;
  const cross=ledger+'/resource-access/facilities/'+otherFacility;
  const prior=await json(s,'display-second-policy','GET',cross);
  const expand={...proposal,request_id:key('second-link'),expected_access_revision:prior.policy.revision,add:[{kind:'facility',capabilities:['read']}]};
  const extraPreview=await json(s,'display-other-preview','POST',cross+'/preview',{data:expand});
  await json(s,'display-other-apply','POST',cross+'/changes',{data:{...expand,preview_hash:extraPreview.preview_hash}});
  const area=ledger+'/displays';
  await json(s,'display-list','GET',area);
  await json(s,'display-enable','PUT',area+'/settings',{data:{...meta('display-enable','0'),enabled:true}});
  for(const format of [1,2]){
    const prefix='display-v'+format+'-',label=l=>prefix+l;
    const created=await json(s,label('create'),'POST',area+'/instances',{data:{...meta(label('create'),'0'),name:'配对生命周期 '+format+' '+run}});
    const base=area+'/instances/'+created.id;
    const detail=()=>s.admin.call(base.slice(7));
    const command=async(l,suffix,data,method='POST',opts={})=>{
      const d=await detail();
      return json(s,label(l),method,base+suffix,{data:{...meta(label(l),d.instance.revision),...data},...opts});
    };
    await json(s,label('detail'),'GET',base);
    if(format===1){
      await command('rename','',{name:'已命名的配对大屏 '+run,state:'enabled',viewers:[]},'PATCH');
      const verifier=randomBytes(32).toString('hex'),audience='http://127.0.0.1:39008';
      const authorized=await command('tool-authorize','/tool-authorize',{challenge:sha(verifier),audience});
      const machine=await request.newContext();
      try{
        await checkedRequest(s,label('tool-exchange'),'POST','/tool/exchange',{data:{verifier,audience,code:authorized.code},surface:s.side.runtime,client:machine,route:'/tool/exchange',headers:{Origin:s.side.runtime,'X-Display-Protocol':'1'},
          verify:d=>{assert(d.tool_token||d.token);return {exchanged:true}}});
      }finally{await machine.dispose();}
      const canceled=await command('cancel-start','/uploads',{size:17,sha256:sha(Buffer.alloc(17,1))});
      await command('cancel-upload','/uploads/'+canceled.upload_id+'/cancel',{});
    }
    await json(s,label('location'),'GET','/api/v1/displays/'+created.id+'/location',{verify:d=>{assert.equal(d.project_id,s.project);return{project_matched:true}}});
    const filename=format===1?'example.zip':'monitoring-read-example.zip', zipPath=join(root,'.artifacts','display',filename);
    if(!existsSync(zipPath))execFileSync(process.execPath,['scripts/display-external-check.mjs'],{cwd:root,stdio:'inherit'});
    const zip=readFileSync(zipPath),hash=sha(zip), manifest=JSON.parse(execFileSync('unzip',['-p',zipPath,'manifest.json'],{encoding:'utf8'}));
    const expectedEntry=manifest.files.find(f=>f.path===manifest.entry).sha256;
    const upload=await command('upload','/uploads',{size:zip.length,sha256:hash});
    const url=base+'/uploads/'+upload.upload_id;
    await json(s,label('upload-state'),'GET',url);
    await json(s,label('part'),'PUT',url+'/parts/0',{data:zip,headers:{'Content-Type':'application/octet-stream','X-Content-SHA256':hash},verify:d=>{assert(d.received);return{zip_sha256:hash}}});
    await command('complete','/uploads/'+upload.upload_id+'/complete',{});
    let d, pkg;
    for(let n=0;n<180;n++){
      d=await detail();pkg=d.packages.find(p=>p.state==='ready');if(pkg)break;await sleep(250);
    }
    assert(pkg,prefix+'package verification did not publish a ready manifest');
    assert.equal(pkg.manifest.manifest_version,format);
    if(format===1){
      const queue=async(l,bytes)=>{
        const hash=sha(bytes),v=await command(l+'-start','/uploads',{size:bytes.length,sha256:hash}),p='/uploads/'+v.upload_id;
        await json(s,label(l+'-part'),'PUT',base+p+'/parts/0',{data:bytes,headers:{'Content-Type':'application/octet-stream','X-Content-SHA256':hash}});
        await command(l+'-complete',p+'/complete',{});return p;
      };
      const bad=await queue('bad-zip',Buffer.from('synthetic package with no ZIP structure')),waitFor=async(path,state,min=1)=>{
        let result;
        for(let n=0;n<180;n++){result=await s.admin.call((base+path).slice(7));if(result.jobs?.[0]?.state===state&&result.jobs[0].attempt>=min)return result.jobs[0];await sleep(250)}
        throw Error('package job did not become '+state);
      };
      const failed=await waitFor(bad,'failed');
      await command('retry-failed',bad+'/retry',{});
      const again=await waitFor(bad,'failed',failed.attempt+1);
      assert.equal(again.error_code,failed.error_code);
      await json(s,label('retry-proof'),'GET',base+bad,{verify:d=>({attempt:d.jobs[0].attempt,error_code:d.jobs[0].error_code,state:d.jobs[0].state})});
      const unused=readFileSync(join(root,'.artifacts/display/monitoring-read-example.zip'));
      const spare=await queue('spare-package',unused);
      await waitFor(spare,'ready');
      const other=(await detail()).packages.find(p=>p.id!==pkg.id&&p.state==='ready');assert(other);
      await command('delete-unreferenced','/packages/'+other.id,{},'DELETE');
    }
    const grant={capabilities:format===1?['resources.read']:['views.read','queries.read','resources.directory','resources.detail','statistics.resources','map.read'],
      valid_until:commonExpiry,scope:{resources:[facility]}};
    const definition={id:'paired',domain:'resources',selector:{kind:'ids',ids:[facility,extra]},field_groups:['resource.metadata','statistics.counts']};
    let views={};
    if(format===2){
      const grantScope={...definition,selector:{kind:'project'}};
      const visibleFacilities=[facility,otherFacility,...(s.attachmentFacility?[s.attachmentFacility]:[])];
      const p=await json(s,label('grant-options'),'GET',base+'/read-scope-options?purpose=grant&domain=resources&kind=facility&limit=1',
        {verify:d=>({total:d.total,item:d.items[0]?.id===facility})});
      assert(p.items.every(o=>visibleFacilities.includes(o.id)));assert.equal(Number(p.total),visibleFacilities.length);
      await json(s,label('grant-visible-facilities'),'GET',base+'/read-scope-options?purpose=grant&domain=resources&kind=facility&limit=200',
        {verify:d=>{assert.deepEqual(d.items.map(o=>o.id).sort(),[...visibleFacilities].sort());return{facilities:d.items.length}}});
      const all=await json(s,label('grant-options-page'),'GET',base+'/read-scope-options?purpose=grant&domain=resources&limit=1');
      assert(all.has_more&&all.next_cursor,'grant options did not paginate');
      await json(s,label('grant-options-next'),'GET',base+'/read-scope-options?purpose=grant&domain=resources&limit=1&cursor='+encodeURIComponent(all.next_cursor),{verify:d=>{assert.notEqual(d.items[0].id,all.items[0].id);return {advanced:true}}});
      await json(s,label('grant-preview'),'POST',base+'/read-scope-preview',{data:{purpose:'grant',definition:grantScope},verify:d=>{assert(Number(d.member_count)>=2);return{members:d.member_count}}});
      grant.scope_format=2;grant.read_scopes=[definition];
      views={project_resources:{scope_id:'paired',domain:'resources',selector:{kind:'ids',ids:[facility,extra]},field_groups:['resource.metadata','statistics.counts']}};
    }
    await command('grant','/grant',{grant},'PUT');
    if(format===2){
      const p=await json(s,label('config-options'),'GET',base+'/read-scope-options?purpose=config&domain=resources&scope_id=paired&kind=facility&limit=1',
        {verify:d=>({total:d.total,item:d.items[0]?.id===facility})});
      assert(p.items.every(o=>o.id===facility));assert.equal(Number(p.total),1);
      const q=await json(s,label('config-preview'),'POST',base+'/read-scope-preview',{data:{purpose:'config',definition:views.project_resources},verify:d=>({members:d.member_count})});
      assert.equal(q.member_count,'2');
      const all=await json(s,label('config-options-page'),'GET',base+'/read-scope-options?purpose=config&scope_id=paired&domain=resources&limit=1');
      assert(all.has_more&&all.next_cursor);
      await json(s,label('config-options-next'),'GET',base+'/read-scope-options?purpose=config&scope_id=paired&domain=resources&limit=1&cursor='+encodeURIComponent(all.next_cursor),
        {verify:d=>{assert.notEqual(d.items[0].id,all.items[0].id);return {advanced:true}}});
      const hidden=await json(s,label('config-outside'),'GET',base+'/read-scope-options?purpose=config&scope_id=absent&domain=resources',{status:404});assert.ok(hidden.code);
    }
    const cfg=await command('config','/configs',{package_id:pkg.id,config_format:format,parameters:{title:'配对兼容'},bindings:format===1?{site:{kind:'resources',ids:[facility]}}:{},read_views:views,mode:'follow'});
    const prep=await command('prepare','/prepare',{config_id:cfg.config_id});
    await command('publish','/publish',{config_id:cfg.config_id,preparation:prep.hash});
    const initial=(await detail()).instance.current_release_id;
    const browserSession=await browserSDK(s,format,prefix,command,created.id);
    if(format===1)await json(s,label('sdk-delivery-state'),'GET',base,{verify:d=>{
      const session=d.sessions.find(row=>row.id===browserSession);
      assert(session&&Number.isFinite(Date.parse(session.last_data_at)),'runtime did not record the delivered SDK read');
      assert.equal(session.last_query_state,'ready');
      return{recorded:true,state:session.last_query_state};
    }});
    const runtime=s.side.runtime;
    const rheaders={Origin:runtime,'X-Display-Protocol':'1'};
    const runtimeContext=await request.newContext(),terminalContext=await request.newContext(),assets=await request.newContext();
    const runtimeCmd=(l,path,data,http=runtimeContext,opts={})=>json(s,label(l),'POST',path,{data,client:http,surface:runtime,headers:rheaders,route:'/runtime/'+(path.includes('/terminal/')?'terminal/'+path.split('/').at(-1):path==='/runtime/exchange'?'exchange':':session/'+path.split('/').at(-1)),...opts});
    try{
      const start=async(l,action)=>{
        const verifier=randomBytes(32).toString('hex'),challenge=sha(verifier);
        const launch=await command(l,'/'+action,{challenge,audience:runtime,config_id:cfg.config_id});
        return runtimeCmd(l+'-exchange','/runtime/exchange',{code:launch.code,verifier,audience:runtime},runtimeContext,{verify:d=>{assert.equal(d.session.preview,action==='preview');assert.equal(d.manifest.manifest_version,format);return{preview:d.session.preview,manifest_format:d.manifest.manifest_version}}});
      };
      await start('preview','preview');
      const boot=await start('launch','launch'),session=boot.session.id;
      const sessionPath='/runtime/'+session;
      await runtimeCmd('ready',sessionPath+'/control',{action:'ready'});
      const entry=new URL(boot.entry_url);
      assert.equal(entry.origin,s.side.assets);assert.equal((await assets.storageState()).cookies.length,0);
      const assetOptions={surface:s.side.assets,client:assets,route:'/assets/:permit/*path'};
      await checkedRequest(s,label('public-asset'),'GET',entry.pathname,{...assetOptions,verify:(_,bytes,h)=>{assert.equal(sha(bytes),expectedEntry);assert.match(h['content-security-policy'],/sandbox allow-scripts/);assert.equal(h['access-control-allow-origin'],'*');return{sha256:sha(bytes),personnel_credentials_sent:false,sandbox:true}}});
      const current=await runtimeCmd('get-permit',sessionPath+'/bootstrap',{});
      await checkedRequest(s,label('old-permit'),'GET',entry.pathname,{...assetOptions,status:[401,403,404,410],verify:d=>({reason:d.code})});
      if(format===2){
        await runtimeCmd('named-views',sessionPath+'/query',{method:'views.list',params:{}},runtimeContext,{verify:d=>{assert(JSON.stringify(d).includes('project_resources'));return{named_view:'project_resources'}}});
      }
      const runtimeSession=(await runtimeContext.storageState()).cookies.find(c=>c.name==='infrapro_display'&&c.path.includes(session));assert.ok(runtimeSession);
      const readSSE=async(name)=>{
        const ac=new AbortController(),timeout=setTimeout(()=>ac.abort(),9000);
        try{
          const response=await fetch(runtime+sessionPath+'/events',{headers:{Accept:'text/event-stream',Cookie:runtimeSession.name+'='+runtimeSession.value},signal:ac.signal});
          assert.equal(response.status,200);assert.match(response.headers.get('content-type'),/text\/event-stream/);
          let text='';const reader=response.body.getReader();while(!text.includes('event: control\ndata:')||!text.endsWith('\n\n')){const v=await reader.read();if(v.done)break;text+=new TextDecoder().decode(v.value);assert(text.length<128000);}
          const events=text.split('\n\n').filter(x=>x.startsWith('event: ')).map(x=>x.split('\n')[0].slice(7));assert.equal(events[0],'control');
          assert(text.includes('"actual_release_id":"'+initial+'"'));
          record(s.name,label(name),'GET','/runtime/:session/events',200,{first:'control',current_release:true});
        }finally{ac.abort();clearTimeout(timeout);}
      };
      await readSSE('sse');await readSSE('sse-reconnect');
      await runtimeCmd('wrong-origin',sessionPath+'/renew',{},runtimeContext,{headers:{...rheaders,Origin:s.side.origin},status:403});
      const term=await command('terminal','/terminals',{name:'配对合成终端',valid_until:commonExpiry});
      await runtimeCmd('pair','/runtime/terminal/pair',{code:term.pair_code},terminalContext);
      const wall=await runtimeCmd('machine-session','/runtime/terminal/start',{},terminalContext);
      assert.equal(wall.session.release_id,initial);
      if(format===1){
        await command('reset-terminal','/terminals/'+term.terminal_id,{state:'reset'},'PATCH');
        await runtimeCmd('retired-terminal','/runtime/terminal/start',{},terminalContext,{status:[401,403,404,409,410]});
        await command('delete-referenced','/packages/'+pkg.id,{},'DELETE',{status:409});
      }
      const second=await command('second-release','/publish',{config_id:cfg.config_id,preparation:(await command('second-prepare','/prepare',{config_id:cfg.config_id})).hash});
      assert(second);const target=(await detail()).instance.current_release_id;assert.notEqual(target,initial);
      const rolled=await command('rollback-prepare','/prepare',{release_id:initial});await command('rollback','/rollback',{release_id:initial,preparation:rolled.hash});
      await command('withdraw','/withdraw',{release_id:(await detail()).instance.current_release_id});
      await command('revoke','/grant',{grant:{revoked:true}},'PUT');
      await runtimeCmd('revoked-session',sessionPath+'/renew',{},runtimeContext,{status:[401,403,404,409,410],verify:d=>({reason:d.code})});
      await checkedRequest(s,label('revoked-asset'),'GET',new URL(current.entry_url).pathname,{...assetOptions,status:[401,403,404,410]});
    }finally{await Promise.all([runtimeContext.dispose(),terminalContext.dispose(),assets.dispose()]);}
  }
}
