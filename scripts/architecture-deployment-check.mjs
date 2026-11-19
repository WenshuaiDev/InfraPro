// Upgrade/fallback using unmodified schema-58 binaries and the same saved
// browser session. All writable services and databases belong to this run.
import assert from 'node:assert/strict';
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { closeSync, existsSync, mkdirSync, openSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import { dirname, join, relative, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { setTimeout as sleep } from 'node:timers/promises';
import { prepare } from './architecture-deploy.mjs';
import { ensureOwnedDockerNetwork } from './architecture-http-network.mjs';
import { provision } from './architecture-postgres.mjs';
import { managed, roleEnvironment, root } from './architecture-runtime.mjs';
import { checkProfiles } from './architecture-deployment-profile-check.mjs';
import { checkReplicas } from './architecture-replica-check.mjs';
import { createMFAKeyring } from './mfa-keyring.mjs';
import { verifyAuditDownloadPermissions, verifyRestrictedLookupPermissions } from './architecture-domain-permissions.mjs';

const {values:o}=parseArgs({options:{output:{type:'string'},'frozen-bin':{type:'string'},'frozen-commit':{type:'string'},'current-bin':{type:'string'},keep:{type:'boolean'}}});
const runId='deployment-'+new Date().toISOString().replaceAll(':','-'),out=resolve(o.output??join(root,'.artifacts/architecture-refactor',runId),'deploy'),secret=join(out,'private',runId+'-'+randomBytes(3).toString('hex'));
mkdirSync(secret,{mode:0o700,recursive:true});
const previous=join(out,'deployment.json');if(existsSync(previous))writeFileSync(join(secret,'previous-deployment.json'),readFileSync(previous),{mode:0o600});
const tag='arc-deploy-'+randomBytes(5).toString('hex'),pg=tag+'-pg',cache=tag+'-redis',network=tag+'-data';
let ownedNetwork;
const reports=[],processes=new Map(),containers=new Set(),ledger={startedAt:new Date().toISOString(),checks:reports},now=()=>new Date().toISOString();
const hostEnv={...process.env}; for(const key of Object.keys(hostEnv))if(key.startsWith('INFRAPRO_'))delete hostEnv[key];
const dbpass=randomBytes(16).toString('hex'),userpass=randomBytes(16).toString('hex'),redisPrefix='infrapro:'+tag+':identity:v1:';
const dumpPath=join(secret,'frozen.dump'),bin=resolve(o['current-bin']??join(secret,'bin'));
const keyring=join(secret,'mfa.json'),layout=join(secret,'split');
const envBase={...hostEnv,INFRAPRO_CSRF_AUTH_KEY:randomBytes(32).toString('hex'),INFRAPRO_COOKIE_SECURE:'false',INFRAPRO_SUPPORT_CONTACT:'架构验收',INFRAPRO_MFA_KEYRING_FILE:keyring};
const log=(key,value)=>{reports.push({key,at:now(),...value});writeFileSync(join(out,'deployment.json'),JSON.stringify(ledger,null,2)+'\n',{mode:0o600});};
const run=(name,args,env=hostEnv,input,options={})=>execFileSync(name,args,{cwd:root,env,input,encoding:'utf8',maxBuffer:64<<20,timeout:10*60*1000,stdio:['pipe','pipe','pipe'],...options});
const docker=(args,input)=>run('docker',args,hostEnv,input);
const sql=(query,db='arc')=>docker(['exec','-i',pg,'psql','-XqAt','-v','ON_ERROR_STOP=1','-U','arc','-d',db],query).trim();
const pgdump=db=>run('docker',['exec',pg,'pg_dump','-Fc','--no-owner','--no-acl','-U','arc',db],hostEnv,undefined,{encoding:'buffer'});
const pgrestore=(db,data)=>docker(['exec','-i',pg,'pg_restore','--no-owner','--no-acl','--single-transaction','--exit-on-error','-U','arc','-d',db],data);
function child(name,file,args,env) {
  if(processes.has(name))throw Error('Duplicate role '+name);
  const fd=openSync(join(secret,name+'.log'),'a',0o600);
  const proc=spawn(file,args,{env,stdio:['ignore',fd,fd]});closeSync(fd);
  const closed=new Promise(done=>{proc.once('close',(code,signal)=>done({code,signal}));proc.once('error',()=>done({code:1,signal:null}))});
  processes.set(name,{proc,closed});return proc;
}
async function stop(name,successful=true){
  const state=processes.get(name);if(!state)return;
  state.proc.kill('SIGTERM');const result=await Promise.race([state.closed,sleep(120000,null,{ref:false})]);
  if(!result){state.proc.kill('SIGKILL');await state.closed;throw Error('Shutdown timed out: '+name)}
  processes.delete(name);
  if(successful) assert.equal(result.code,0,name+' graceful stop');
}
async function port(){
  const s=net.createServer();await new Promise((res,rej)=>{s.once('error',rej);s.listen(0,'127.0.0.1',res)});const p=s.address().port;await new Promise(res=>s.close(res));return p;
}
async function ports(){
  // Reserve a contiguous block long enough to verify none is already occupied.
  for(let i=0;i<60;i++){const start=19500+Math.floor(Math.random()*18000),sockets=[];try {
    for(let n=0;n<7;n++){const s=net.createServer();await new Promise((res,rej)=>{s.once('error',rej);s.listen(start+n,'127.0.0.1',res)});sockets.push(s)}
    return start;
  } catch { /* choose another block */ } finally {for(const s of sockets)await new Promise(res=>s.close(res));}}
  throw Error('No contiguous private ports');
}
async function ready(url){
  for(let n=0;n<90;n++){try{const r=await fetch(url+'/readyz',{signal:AbortSignal.timeout(2300)});if(r.ok)return}catch{ /* dependency may still be starting */ } await sleep(200)}
  throw Error('Readiness failed for '+new URL(url).port);
}
function profile(dir){return JSON.parse(readFileSync(join(dir,'layout.json'),'utf8'));}
function frozenBinaries(){
  if(o['frozen-bin'])return resolve(o['frozen-bin']);
  const base=join(root,'.artifacts/architecture-refactor');
  for(const f of readdirSync(base).sort())if(f.startsWith('freeze-')&&existsSync(join(base,f,'baseline.json'))&&existsSync(join(base,f,'bin/api'))&&JSON.parse(readFileSync(join(base,f,'baseline.json'))).schema===58)return join(base,f,'bin');
  const dir=join(secret,'frozen-src'),archive=join(secret,'frozen.tar'),old=join(secret,'frozen-bin');
  mkdirSync(dir,{recursive:true});mkdirSync(old,{recursive:true});
  run('git',['archive','--format=tar','--output='+archive,o['frozen-commit']??'220aee11790d900af823b05b70796da1a4733c79']);
  run('tar',['-xf',archive,'-C',dir]);
  run('go',['build','-trimpath','-o',old+'/','./services/api/cmd/api','./tools/infrapro/cmd/infrapro'],hostEnv,undefined,{cwd:dir});
  return old;
}
const session=new Map();let token='';
async function request(url,method,path,body,want,cmd){
  const headers={Origin:envBase.INFRAPRO_TRUSTED_ORIGINS,'Content-Type':'application/json','Cookie':[...session].map(k=>k.join('=')).join('; ')};
  if(token)headers['X-CSRF-Token']=token;if(cmd)headers['X-Command-ID']=cmd;
  const response=await fetch(url+path,{method,headers,body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(10000)});
  for(const c of response.headers.getSetCookie()){const pair=c.split(';',1)[0],i=pair.indexOf('=');session.set(pair.slice(0,i),pair.slice(i+1));}
  const text=await response.text(),data=text?JSON.parse(text):null;
  if(want!==undefined)assert.equal(response.status,want,method+' '+path+' '+(data?.code??''));
  return data;
}
const pwdBootstrap=(cli,env)=>run('expect',['-c',`
log_user 0
set timeout 15
spawn -noecho $env(ARC_CLI) account bootstrap
expect "用户名："; send "arcowner\\r"
expect "显示名称"; send "Arc Owner\\r"
expect "不回显"; send "$env(ARC_PASSWORD)\\r"
expect "再次输入"; send "$env(ARC_PASSWORD)\\r"
expect eof
set status [wait]; exit [lindex $status 3]
`],{...env,ARC_CLI:cli,ARC_PASSWORD:userpass});
try {
  const old=frozenBinaries();
  if(!o['current-bin']){
    mkdirSync(bin,{recursive:true});run('go',['build','-trimpath','-o',bin+'/','./services/api/cmd/...','./tools/infrapro/cmd/infrapro']);
  }
  for(const name of managed)assert(statSync(join(bin,name)).size>0,name);
  const selectedManifest=join(dirname(bin),'candidate.json');
  if(existsSync(selectedManifest)){
    const selected=JSON.parse(readFileSync(selectedManifest));
    assert.equal(resolve(selected.bin),bin);
    for(const [role,sha] of Object.entries(selected.binaries))
      assert.equal(createHash('sha256').update(readFileSync(join(bin,role))).digest('hex'),sha,role);
    ledger.release={source_sha256:selected.inputs.sourceSHA256,commit:selected.inputs.commit,
      binary_count:Object.keys(selected.binaries).length,manifest_sha256:createHash('sha256').update(readFileSync(selectedManifest)).digest('hex')};
  }
  ledger.private_evidence_directory=relative(out,secret);
  const publicPort=await port(),origin='http://127.0.0.1:'+publicPort,privatePort=await ports();
  Object.assign(envBase,{INFRAPRO_PUBLIC_CONSOLE_URL:origin+'/',INFRAPRO_TRUSTED_ORIGINS:origin,INFRAPRO_HTTP_ADDR:'127.0.0.1:'+publicPort});
  createMFAKeyring(keyring);prepare({mode:'local',directory:layout,'port-base':String(privatePort)});
  ownedNetwork = ensureOwnedDockerNetwork({ name: network, owner: tag, label: 'infrapro.architecture.deployment-check', prefix: '10.246' });
  run('docker',['run','-d','--name',pg,'--network',network,'--network-alias','pg','-p','127.0.0.1::5432','--memory','512m','-e','POSTGRES_PASSWORD','-e','POSTGRES_USER=arc','-e','POSTGRES_DB=arc','postgres:17-alpine'],{...hostEnv,POSTGRES_PASSWORD:dbpass});containers.add(pg);
  docker(['run','-d','--name',cache,'--network',network,'--network-alias','redis','-p','127.0.0.1::6379','--memory','96m','redis:7.4-alpine','redis-server','--save','','--appendonly','no']);containers.add(cache);
  for(let i=0;i<100;i++){try{docker(['exec',pg,'pg_isready','-h','127.0.0.1','-U','arc']);break}catch{await sleep(150)}}
  const hostPort=(id,p)=>docker(['port',id,p+'/tcp']).trim().split(':').pop();
  const uri=db=>'postgres://arc:'+dbpass+'@127.0.0.1:'+hostPort(pg,5432)+'/'+db+'?sslmode=disable';
  const envA={...envBase,INFRAPRO_DATABASE_URL:uri('arc'),INFRAPRO_REDIS_URL:'redis://127.0.0.1:'+hostPort(cache,6379)+'/0',INFRAPRO_REDIS_KEY_PREFIX:redisPrefix};
  const oldCli=join(old,'infrapro');
  run(oldCli,['database','migrate'],envA);run(oldCli,['access','sync'],envA);pwdBootstrap(oldCli,envA);
  child('baseline',join(old,'api'),[],envA);await ready(origin);
  token=(await request(origin,'GET','/api/v1/auth/csrf',undefined,200)).token;
  await request(origin,'POST','/api/v1/auth/login',{username:'arcowner',password:userpass},200);
  const account=sql("SELECT id::text FROM accounts WHERE username='arcowner'");
  async function tenant(code,name,status=201){
    const id=randomUUID(); const input={name,code,description:'ARCH fixture',administrator_account_id:account,client_request_id:id,reason:'架构切换验收'};
    const data=await request(origin,'POST','/api/v1/platform/tenants',input,status,id);
    return {input,id,tenant:data.tenant_id};
  }
  const before=await tenant('arc-before','切换前资产');
  const membership=(await request(origin,'GET','/api/v1/tenants/'+before.tenant+'/context',undefined,200)).membership_id;
  const projectCommand=randomUUID();
  const seededProject=await request(origin,'POST','/api/v1/tenants/'+before.tenant+'/projects',{name:'联合恢复项目',code:'ARCJOINT',purpose:'独立联合恢复',owner_tenant_membership_id:membership,administrator_tenant_membership_id:membership,client_request_id:projectCommand,reason:'冻结项目与大屏授权样本'},200,projectCommand);
  assert.match(seededProject.project_id,/^[0-9a-f-]{36}$/);
  const readPaths=['/api/v1/me/notifications','/api/v1/account/avatar','/api/v1/tenants/'+before.tenant+'/monitoring/channels','/api/v1/tenants/'+before.tenant+'/files/attachments'];
  async function contract(path){
    const headers={Origin:origin,Cookie:[...session].map(k=>k.join('=')).join('; ')};
    const r=await fetch(origin+path,{headers});
    const value=await r.json();
    return {status:r.status,code:value.code??null,shape:Object.keys(value).sort()};
  }
  const oldResponses={};
  for(const path of readPaths)oldResponses[path]=await contract(path);
  const pendingID=randomUUID(),writePath='/api/v1/platform/tenants';
  const pending={name:'切换窗口重试资产',code:'arc-after',description:'ARCH fixture',administrator_account_id:account,client_request_id:pendingID,reason:'原客户端切换后重试'};
  const job=sql("INSERT INTO monitoring_jobs(kind,tenant_id,target_id,target_revision,input,input_hash) VALUES('summary','"+before.tenant+"',gen_random_uuid(),1,'{\"mode\":\"rebuild\"}','"+"0".repeat(64)+"') RETURNING id");
  const freezeAt=Date.now(),offline=[];
  await stop('baseline');
  for(const path of [...readPaths,writePath]){
    const attempt=path===writePath?request(origin,'POST',path,pending,undefined,pendingID):contract(path);
    await assert.rejects(attempt,e=>['ECONNREFUSED','UND_ERR_SOCKET','ECONNRESET'].includes(e.cause?.code));
    offline.push({method:path===writePath?'POST':'GET',path,refusedAt:Date.now()});
  }
  writeFileSync(dumpPath,pgdump('arc'),{mode:0o600});
  sql('CREATE DATABASE arc_b');pgrestore('arc_b',readFileSync(dumpPath));
  const envB={...envA,INFRAPRO_DATABASE_URL:uri('arc_b'),INFRAPRO_REDIS_KEY_PREFIX:'infrapro:'+tag+'-b:identity:v1:'};
  const cli=join(bin,'infrapro');
  const version=()=>Number(sql('SELECT max(version_id) FROM goose_db_version WHERE is_applied','arc_b'));
  assert.equal(version(),58);assert.throws(()=>run(cli,['access','check'],envB));
  const start=performance.now();run(cli,['database','migrate'],envB);const elapsed=performance.now()-start;
  assert.equal(version(),59);assert.throws(()=>run(oldCli,['access','check'],envB));
  ledger.applied_role_sql=provision({url:envB.INFRAPRO_DATABASE_URL,container:pg,directory:layout,localLogins:true});
  const originalTopology=roleEnvironment('compat-api',envB,layout);
  delete originalTopology.INFRAPRO_ARCHITECTURE_FILE;
  delete originalTopology.INFRAPRO_REPLICA_ID;
  child('compat-public',join(bin,'api'),[],originalTopology);await ready(origin);
  for(const item of offline){
    if(item.method==='GET'){
      const retry=await contract(item.path);assert.deepEqual(retry,oldResponses[item.path]);item.retryStatus=retry.status;
    }else{
      const accepted=await request(origin,'POST',writePath,pending,201,pendingID);
      item.retryStatus=201;item.acceptedID=accepted.tenant_id;
      assert.equal((await request(origin,'POST',writePath,pending,200,pendingID)).tenant_id,accepted.tenant_id);
    }
    item.restoredAt=Date.now();item.interruptionMs=item.restoredAt-item.refusedAt;
  }
  await request(origin,'GET','/api/v1/me/workspaces',undefined,200);
  const introduced={id:pendingID,input:pending,tenant:offline.find(item=>item.method==='POST').acceptedID};
  log('controlled-write-window',{arc:'ARC-033',schema:59,firstRuntime:'api compatibility topology',fromUnixMs:freezeAt,reopenedUnixMs:Date.now(),endpoints:offline,originalSessionAccepted:true,repeatDidNotDuplicate:true});
  const schedulerOwners={
    infrapro_identity_api:['identity/session-activity','identity/identity-cleanup','identity/identity-retention'],
    infrapro_monitoring_publish:['storage-verification'],infrapro_monitoring_incremental:['derived-schedule'],
    infrapro_monitoring_read:['display-read-cleanup'],infrapro_parsing_dispatch:['manual-parsing-cleanup'],
    infrapro_notification_worker:['notification-maintenance'],infrapro_files_worker:['file-object-cleanup','file-sweep/test-node'],
  };
  for(const [owner,lanes] of Object.entries(schedulerOwners)){
    const claim=lane=>sql(`BEGIN; SET LOCAL ROLE ${owner}; INSERT INTO monitoring_scheduler_leases(lane,next_run) VALUES('${lane}',now()) ON CONFLICT(lane) DO UPDATE SET next_run=EXCLUDED.next_run RETURNING lane; ROLLBACK;`,'arc_b');
    for(const lane of lanes)assert.equal(claim(lane),lane);
    assert.throws(()=>claim('outside-owned-lane'),error=>error.stderr?.toString().includes('row-level security'));
  }
  const clockProof={status:'passed',owned:schedulerOwners,denied:'outside-owned-lane'};
  writeFileSync(join(out,'scheduler-privileges.json'),JSON.stringify(clockProof,null,2)+'\n',{mode:0o600});
  log('scheduler-privileges',{arc:'ARC-032',roles:Object.keys(schedulerOwners).length});
  // These privileges are exercised by cross-domain reads and by upload
  // completion triggers, so a successful cold start alone cannot verify them.
  const domainPrivileges=[
    ['infrapro_identity_api','project_resource_policies','revision','SELECT',true],
    ['infrapro_identity_api','project_resource_policies','tenant_id','INSERT',true],
    ['infrapro_identity_api','project_resource_policies','project_id','INSERT',true],
    ['infrapro_identity_api','project_resource_policies','allow_facility_create','INSERT',false],
    ['infrapro_identity_api','project_resource_policies','revision','UPDATE',false],
    ['infrapro_identity_api','project_resource_policies','allow_facility_create','UPDATE',false],
    ['infrapro_control_api','account_invitations','id','SELECT',true],
    ['infrapro_control_api','account_invitations','inviter_id','SELECT',true],
    ['infrapro_control_api','account_invitations','token_hash','SELECT',false],
    ['infrapro_control_api','account_invitations','note','SELECT',false],
    ['infrapro_control_api','tenant_invitations','target_account_id','SELECT',false],
    ['infrapro_control_api','tenant_invitations','token_hash','SELECT',false],
    ['infrapro_control_api','tenant_invitations','accepted_context_snapshot','SELECT',false],
    ['infrapro_control_api','tenant_invitations','close_reason','SELECT',false],
    ['infrapro_control_api','tenant_invitations','status','UPDATE',false],
    ['infrapro_control_api','tenant_invitations','accepted_context_snapshot','UPDATE',false],
    ['infrapro_audit_worker','device_category_releases','content','SELECT',true],
    ['infrapro_audit_worker','device_category_releases','created_by','SELECT',false],
    ['infrapro_audit_worker','device_category_releases','created_txid','SELECT',false],
    ['infrapro_audit_worker','device_category_releases','content','UPDATE',false],
    ['infrapro_audit_worker','engineering_resource_records','data','SELECT',true],
    ['infrapro_audit_worker','engineering_resource_records','id','UPDATE',false],
    ['infrapro_audit_worker','device_classification_state','current_release_id','SELECT',true],
    ['infrapro_audit_worker','device_classification_state','draft','SELECT',false],
    ['infrapro_audit_worker','product_classification_records','selected_release_id','SELECT',true],
    ['infrapro_audit_worker','product_classification_records','reason','SELECT',false],
    ['infrapro_notification_worker','display_events','details','SELECT',true],
    ['infrapro_notification_worker','display_task_handoffs','fingerprint','SELECT',true],
    ['infrapro_notification_worker','display_events','details','UPDATE',false],
    ['infrapro_query_api','monitoring_selections','channel_ids','INSERT',true],
    ['infrapro_query_api','monitoring_selections','read_windows','UPDATE',true],
    ['infrapro_query_api','monitoring_jobs','input','INSERT',true],
    ['infrapro_query_api','monitoring_jobs','lease_owner','UPDATE',false],
    ['infrapro_query_api','engineering_facilities','name','UPDATE',false],
    ['infrapro_transfer_api','file_protocol','id','UPDATE',true],
    ['infrapro_transfer_api','file_protocol','mode','UPDATE',false],
    ['infrapro_transfer_api','audit_exports','id','UPDATE',true],
    ['infrapro_transfer_api','audit_exports','current_generation','UPDATE',false],
    ['infrapro_transfer_api','audit_export_generations','export_id','UPDATE',true],
    ['infrapro_transfer_api','audit_export_generations','state','UPDATE',false],
    ['infrapro_transfer_api','audit_export_objects','state','UPDATE',true],
    ['infrapro_transfer_api','audit_export_objects','object_key','UPDATE',false],
    ['infrapro_transfer_api','audit_temp_reservations','reserved_bytes','INSERT',true],
    ['infrapro_transfer_api','audit_temp_reservations','reserved_bytes','UPDATE',false],
    ['infrapro_transfer_api','audit_temp_reservations','released_at','UPDATE',true],
    ['infrapro_transfer_api','audit_temp_reservations','created_at','SELECT',false],
    ['infrapro_transfer_api','display_settings','tenant_id','INSERT',true],
    ['infrapro_transfer_api','display_settings','enabled','UPDATE',false],
    ['infrapro_transfer_api','display_task_handoffs','snapshot','INSERT',true],
    ['infrapro_display_runtime','display_instances','architecture_lock','UPDATE',true],
    ['infrapro_display_runtime','display_instances','current_release_id','UPDATE',false],
    ['infrapro_display_runtime','display_package_jobs','upload_id','INSERT',false],
    ['infrapro_display_runtime','account_sessions','last_activity_at','UPDATE',true],
    ['infrapro_display_assets','account_sessions','revoked_at','UPDATE',false],
    ['infrapro_display_assets','engineering_resource_nodes','id','SELECT',true],
    ['infrapro_display_assets','spatial_asset_versions','state','UPDATE',false],
    ['infrapro_notification_worker','file_uploads','display_name','SELECT',true],
    ['infrapro_query_api','derived_graph_states','revision','UPDATE',false],
    ['infrapro_control_api','display_package_jobs','state','UPDATE',true],
    ['infrapro_control_api','display_package_files','object_key','INSERT',false],
    ['infrapro_monitoring_incremental','monitoring_tenant_states','commit_sequence','UPDATE',true],
    ['infrapro_monitoring_history','monitoring_tenant_states','route_revision','UPDATE',false],
    ['infrapro_monitoring_incremental','monitoring_changes','impact_hash','UPDATE',false],
    ['infrapro_monitoring_history','monitoring_change_ranges','ordinal','INSERT',true],
    ['infrapro_monitoring_history','notification_conditions','active','UPDATE',false],
    ['infrapro_monitoring_incremental','notification_conditions','source_module','UPDATE',false],
    ['infrapro_monitoring_incremental','notification_conditions','recovered_at','SELECT',false],
    ['infrapro_monitoring_history','display_read_monitoring_refs','snapshot_id','SELECT',true],
    ['infrapro_monitoring_history','display_read_queries','expires_at','SELECT',true],
    ['infrapro_monitoring_history','display_read_queries','request','SELECT',false],
    ['infrapro_monitoring_incremental','analysis_trial_receipts','result','INSERT',true],
    ['infrapro_monitoring_incremental','analysis_trial_receipts','created_at','INSERT',false],
    ['infrapro_monitoring_incremental','analysis_trial_receipts','result','UPDATE',false],
    ['infrapro_monitoring_read','monitoring_summary_heads','watermark','UPDATE',true],
    ['infrapro_monitoring_read','monitoring_summary_heads','channel_id','UPDATE',false],
    ['infrapro_monitoring_history','monitoring_contribution_replacements','object_key','INSERT',true],
    ['infrapro_monitoring_history','monitoring_contribution_replacements','object_key','UPDATE',false],
    ['infrapro_monitoring_history','monitoring_channel_versions','quarantined','UPDATE',true],
    ['infrapro_monitoring_history','monitoring_channel_versions','integrity_revision','UPDATE',false],
  ];
  const domainChecks=domainPrivileges.map(([group,table,column,op,allowed])=>{
    const actual=sql(`SELECT has_column_privilege('${group}','${table}','${column}','${op}')`,'arc_b')==='t';
    assert.equal(actual,allowed,[group,table,column,op].join(' '));
    return {group,table,column,op,allowed};
  });
  const createOwnedJob=(role,kind,mode)=>sql(`\\set VERBOSITY verbose
    BEGIN; SET LOCAL ROLE ${role};
    INSERT INTO monitoring_jobs(tenant_id,actor_id,kind,target_id,target_revision,input_hash,input)
    VALUES('${before.tenant}','${account}','${kind}',gen_random_uuid(),1,repeat('f',64),jsonb_build_object('mode','${mode}'::text))
    RETURNING kind; ROLLBACK;`,'arc_b');
  assert.equal(createOwnedJob('infrapro_query_api','summary','query'),'summary');
  for(const [kind,mode] of [['summary','rebuild'],['publish','query']])
    assert.throws(()=>createOwnedJob('infrapro_query_api',kind,mode),error=>/ERROR:\s+42501:/.test(error.stderr?.toString()));
  assert.equal(createOwnedJob('infrapro_compat_api','summary','rebuild'),'summary');
  const jobOwnership={queryAccepted:['summary/query'],queryRefused:['summary/rebuild','publish/query'],compatAccepted:['summary/rebuild']};
  const ownerStatement=(group,statement)=>sql(`\\set VERBOSITY verbose
    BEGIN; SET LOCAL ROLE ${group}; ${statement}; ROLLBACK;`,'arc_b');
  const refused=error=>/ERROR:\s+42501:/.test(error.stderr?.toString());
  const projectPolicyKey=`tenant_id='${before.tenant}' AND project_id='${seededProject.project_id}'`;
  const projectPolicyID=`'${before.tenant}','${seededProject.project_id}'`;
  // A transaction on the empty fixture project reproduces the insertion from
  // projects.Create, including the actual default revision and deny flag.
  assert.equal(sql(`BEGIN; DELETE FROM project_resource_policies WHERE ${projectPolicyKey};
      SET LOCAL ROLE infrapro_identity_api;
      INSERT INTO project_resource_policies(tenant_id,project_id) VALUES(${projectPolicyID})
      RETURNING revision::text||'/'||allow_facility_create::text; ROLLBACK;`,'arc_b'),'1/false');
  for(const statement of [
    `INSERT INTO project_resource_policies(tenant_id,project_id,allow_facility_create) VALUES(${projectPolicyID},true)`,
    `UPDATE project_resource_policies SET allow_facility_create=true WHERE ${projectPolicyKey}`,
    `UPDATE project_resource_policies SET revision=revision+1 WHERE ${projectPolicyKey}`,
  ]) assert.throws(()=>ownerStatement('infrapro_identity_api',statement),refused);
  for(const group of ['infrapro_query_api','infrapro_transfer_api','infrapro_monitoring_read']){
    assert.match(ownerStatement(group,`SELECT revision FROM derived_graph_states WHERE tenant_id='${before.tenant}' FOR SHARE`),/^\d+$/);
    assert.throws(()=>ownerStatement(group,`UPDATE derived_graph_states SET tenant_id=tenant_id WHERE tenant_id='${before.tenant}'`),refused);
  }
  assert.match(ownerStatement('infrapro_compat_api',`UPDATE derived_graph_states SET revision=revision+1 WHERE tenant_id='${before.tenant}' RETURNING revision`),/^\d+$/);
  for(const group of ['infrapro_display_runtime','infrapro_display_assets','infrapro_query_api','infrapro_monitoring_read']){
    assert.match(ownerStatement(group,`SELECT count(*) FROM project_resource_visibility('${before.tenant}',NULL,NULL)`),/^\d+$/);
    const sessions=ownerStatement(group,`UPDATE account_sessions SET last_activity_at=clock_timestamp() WHERE account_id='${account}' RETURNING account_id`).split('\n');
    assert(sessions.length&&sessions.every(value=>value===account));
    assert.throws(()=>ownerStatement(group,`UPDATE account_sessions SET revoked_at=clock_timestamp() WHERE account_id='${account}'`),refused);
  }
  ownerStatement('infrapro_notification_worker','SELECT j.space_id,j.host_id,j.task_snapshot,u.display_name FROM file_jobs j JOIN file_uploads u ON u.id=j.upload_id LIMIT 1');
  assert.throws(()=>ownerStatement('infrapro_notification_worker','SELECT object_key FROM file_objects LIMIT 1'),refused);
  for(const group of ['infrapro_monitoring_incremental','infrapro_monitoring_history']) {
    const position=sql(`BEGIN; INSERT INTO monitoring_tenant_states(tenant_id) VALUES('${before.tenant}') ON CONFLICT DO NOTHING;
      SET LOCAL ROLE ${group}; UPDATE monitoring_tenant_states SET commit_sequence=commit_sequence+1
      WHERE tenant_id='${before.tenant}' RETURNING commit_sequence; ROLLBACK;`,'arc_b');
    assert.match(position,/^\d+$/);
    assert.throws(()=>ownerStatement(group,`UPDATE monitoring_tenant_states SET route_revision=route_revision+1 WHERE tenant_id='${before.tenant}'`),refused);
    for(const [kind,source] of [['raw_publish','direct'],['derived_publish','derived']])
      assert.throws(()=>ownerStatement(group,`INSERT INTO monitoring_changes(tenant_id,source_position,change_type,source_kind,source_id,impact_hash)
        VALUES('${before.tenant}',2000001,'${kind}','${source}',gen_random_uuid(),repeat('a',64))`),refused);
  }
  const conditionKey=randomUUID();
  const conditionResult=sql(`BEGIN;
    INSERT INTO notification_conditions(source_module,condition_key) VALUES('display','${conditionKey}');
    SET LOCAL ROLE infrapro_monitoring_incremental;
    SELECT count(condition_key) FROM notification_conditions WHERE source_module='display' AND condition_key='${conditionKey}';
    INSERT INTO notification_conditions(source_module,condition_key) VALUES('derivedmonitoring','${conditionKey}') ON CONFLICT DO NOTHING;
    SELECT active,cycle FROM notification_conditions WHERE source_module='derivedmonitoring' AND condition_key='${conditionKey}' FOR UPDATE;
    UPDATE notification_conditions SET occurrences=occurrences+1,last_at=clock_timestamp()
      WHERE source_module='derivedmonitoring' AND condition_key='${conditionKey}' RETURNING occurrences;
    UPDATE notification_conditions SET active=false,cycle=cycle,occurrences=occurrences,last_at=clock_timestamp(),
      first_at=first_at,recovered_at=clock_timestamp() WHERE source_module='derivedmonitoring' AND condition_key='${conditionKey}' RETURNING active;
    ROLLBACK;`,'arc_b');
  assert.deepEqual(conditionResult.split('\n'),['0','t|1','2','f']);
  assert.throws(()=>ownerStatement('infrapro_monitoring_incremental',`INSERT INTO notification_conditions(source_module,condition_key) VALUES('display','${conditionKey}')`),refused);
  ownerStatement('infrapro_monitoring_history',`SELECT s.id FROM monitoring_snapshots s WHERE NOT s.display_managed OR EXISTS (
    SELECT 1 FROM display_read_monitoring_refs r JOIN display_read_queries q ON q.id=r.query_id WHERE r.snapshot_id=s.id
    AND q.state NOT IN ('canceled','expired','failed') AND q.expires_at>clock_timestamp()) LIMIT 1`);
  assert.throws(()=>ownerStatement('infrapro_monitoring_history','SELECT request FROM display_read_queries'),refused);
  const unknownRun=randomUUID();
  assert.throws(()=>ownerStatement('infrapro_monitoring_incremental',`INSERT INTO analysis_trial_receipts(tenant_id,run_id,result)
    VALUES('${before.tenant}','${unknownRun}',jsonb_build_object('trial_receipt_id','${unknownRun}'::text))`),refused);
  const auditDownload=verifyAuditDownloadPermissions({ownerQuery:query=>sql(query,'arc_b'),container:pg,
    transferURL:roleEnvironment('transfer-api',envB,layout).INFRAPRO_DATABASE_URL});
  const references=verifyRestrictedLookupPermissions(query=>sql(query,'arc_b'),account);
  const domainPaths={auditDownload,...references,
    identityProjectPolicy:{seededRevision:1,seededAllowFacilityCreate:false,refusedSQLState:'42501',refusedStatements:3},graphReaders:3,compatGraphWrite:true,personSessionAndBindingReaders:4,notificationDisplayNameOnly:true,
    derivedPositionWriters:2,unsealedOrRawPublicationsDenied:4,automaticConditionCycle:{locked:true,occurrences:2,recovered:true,otherProducerVisibleRows:0},
    exportQueryLifetimeOnly:true,trialWithoutMatchingRunDenied:true};
  writeFileSync(join(out,'domain-privileges.json'),JSON.stringify({status:'passed',checks:domainChecks,jobOwnership,domainPaths},null,2)+'\n',{mode:0o600});
  log('domain-privileges',{arc:'ARC-032',checks:domainChecks.length,jobOwnership,domainPaths});
  assert.equal(sql("SELECT state FROM monitoring_jobs WHERE id='"+job+"'","arc_b"),'queued');
  assert.equal(sql("SELECT count(*) FROM tenants WHERE id='"+before.tenant+"'",'arc_b'),'1');
  const tenantPressureIndex=JSON.parse(sql("SELECT json_build_object('definition',pg_get_indexdef(indexrelid),'valid',indisvalid,'predicate',pg_get_expr(indpred,indrelid)) FROM pg_index WHERE indexrelid='public.event_outbox_tenant_pressure'::regclass",'arc_b'));
  assert.ok(tenantPressureIndex.valid);
  assert.match(tenantPressureIndex.definition,/USING btree \(restore_epoch, scope\)/);
  for(const term of ['pending','leased','platform','<>'])assert.ok(tenantPressureIndex.predicate.includes(term));
  const baselineMetadata=JSON.parse(sql(`SELECT json_build_object(
    'result',pg_get_function_result(p.oid),'securityDefiner',p.prosecdef,
    'runtimeCanExecute',has_function_privilege('infrapro_display_runtime',p.oid,'EXECUTE'),
    'assetsCanExecute',has_function_privilege('infrapro_display_assets',p.oid,'EXECUTE'),
    'runtimeReadsEvidence',has_column_privilege('infrapro_display_runtime','derived_fixed_baselines','evidence','SELECT'),
    'sha256',encode(sha256(convert_to(pg_get_functiondef(p.oid),'UTF8')),'hex'))
    FROM pg_proc p WHERE p.oid='architecture_fixed_baseline_metadata(uuid,uuid,text,text)'::regprocedure`,'arc_b'));
  assert.equal(baselineMetadata.result,'TABLE(snapshot jsonb, input_ranges jsonb)');
  assert.ok(baselineMetadata.securityDefiner && baselineMetadata.runtimeCanExecute);
  assert.ok(!baselineMetadata.assetsCanExecute && !baselineMetadata.runtimeReadsEvidence);
  const invitationMetadata=JSON.parse(sql(`SELECT json_build_object(
    'result',pg_get_function_result(p.oid),'controlCanExecute',has_function_privilege('infrapro_control_api',p.oid,'EXECUTE'),
    'queryCanExecute',has_function_privilege('infrapro_query_api',p.oid,'EXECUTE'),
    'workerCanExecute',has_function_privilege('infrapro_notification_worker',p.oid,'EXECUTE'),
    'sha256',encode(sha256(convert_to(pg_get_functiondef(p.oid),'UTF8')),'hex'))
    FROM pg_proc p WHERE p.oid='architecture_notification_invitation_targets(uuid,uuid,uuid[])'::regprocedure`,'arc_b'));
  assert.ok(invitationMetadata.controlCanExecute && !invitationMetadata.queryCanExecute && !invitationMetadata.workerCanExecute);
  assert.equal(invitationMetadata.result,'TABLE(invitation_id uuid, is_recipient boolean, inviter_id text, inviter_membership_id text, effective_status text, membership_status text, accepted_tenant_name text)');
  log('schema-upgrade',{arc:'ARC-033',versions:[58,59],elapsedMs:elapsed,oldJob:job,oldTenant:before.tenant,oldBinaryRejected:true,tenantPressureIndex,baselineMetadata,invitationMetadata});
  for(const name of ['identity-api','control-api','monitoring-query-api','transfer-api','compat-api']){
    const env=roleEnvironment(name,envB,layout);
    child(name,join(bin,name),[],env);
    if(name!=='identity-api'&&name!=='compat-api'){
      assert(!env.INFRAPRO_MFA_KEYRING_FILE);
      const login=decodeURIComponent(new URL(env.INFRAPRO_DATABASE_URL).username);
      assert.equal(sql("SELECT has_column_privilege('"+login+"','account_mfa','secret_ciphertext','select') OR has_table_privilege('"+login+"','account_passwords','select')",'arc_b'),'f');
      if(name==='monitoring-query-api')assert.equal(sql("SELECT has_table_privilege('"+login+"','monitoring_publications','insert')",'arc_b'),'f');
      if(name==='control-api')assert.equal(sql("SELECT has_column_privilege('"+login+"','monitoring_publications','revoked_by','update') AND NOT (has_column_privilege('"+login+"','monitoring_publications','committed_by','update') OR has_table_privilege('"+login+"','monitoring_publications','insert') OR has_table_privilege('"+login+"','monitoring_publication_chunks','insert'))",'arc_b'),'t');
    }
  }
  const gatewayManifest=join(layout,'roles/api/architecture.json'),owners=['identity-api','control-api','monitoring-query-api','transfer-api'];
  const gateway=()=>child('gateway',join(bin,'api'),[],roleEnvironment('api',envB,layout));
  async function switchRoutes(route_targets){
    const began=performance.now();await stop('gateway');const cfg=JSON.parse(readFileSync(gatewayManifest));cfg.route_targets=route_targets;
    writeFileSync(gatewayManifest,JSON.stringify(cfg)+'\n');gateway();await ready(origin);
    return Math.round(performance.now()-began);
  }
  await stop('compat-public');
  const phased=Object.fromEntries(owners.map(owner=>[owner,'compat-api'])),cutovers=[];
  await switchRoutes(phased);
  for(const owner of owners){
    delete phased[owner];const durationMs=await switchRoutes(phased);
    for(const path of readPaths)assert.deepEqual(await contract(path),oldResponses[path]);
    assert.equal((await request(origin,'POST',writePath,pending,200,pendingID)).tenant_id,introduced.tenant);
    cutovers.push({owner,durationMs,oldSession:true,idempotentRetry:true});
  }
  const health=await fetch(origin+'/healthz');assert.equal(await health.text(),'{"status":"ok"}');
  await request(origin,'GET','/api/v1/me/workspaces',undefined,200);
  for(const path of readPaths)assert.deepEqual(await contract(path),oldResponses[path],path+' role-specific credentials');
  log('all-owners',{arc:'ARC-033',reads:oldResponses,cutovers});
  await switchRoutes({'identity-api':'compat-api'});
  assert.equal((await request(origin,'POST','/api/v1/platform/tenants',introduced.input,200,introduced.id)).tenant_id,introduced.tenant);
  const after=await tenant('arc-rollback','回退期资产');
  await switchRoutes({});await request(origin,'GET','/api/v1/me/workspaces',undefined,200);
  assert.equal((await request(origin,'POST','/api/v1/platform/tenants',after.input,200,after.id)).tenant_id,after.tenant);
  const expected=sql('SELECT id::text FROM tenants ORDER BY id','arc_b');
  assert.equal(expected.split('\n').length,3);
  log('gateway-fallback',{arc:'ARC-035',retainedTenantIDs:expected.split('\n'),originalSessionAccepted:true,restoredJob:job,replays:2});
  const eventsFile=join(secret,'events.json');writeFileSync(eventsFile,JSON.stringify({mode:'pg',environment:'acceptance'}),{mode:0o600});
  const events=(args,env=envB)=>JSON.parse(run(join(bin,'event-relay'),args,{...env,INFRAPRO_EVENTS_FILE:eventsFile}));
  for(const item of events(['status']).ownership){
    const args=['--role',item.role,'--reason','架构验收调度切换'];
    events(['switch',...args,'--mode','draining','--expected',String(item.generation)]);
    events(['switch',...args,'--mode','split','--expected',String(item.generation+1)]);
    events(['switch',...args,'--mode','draining','--expected',String(item.generation+2)]);
    events(['switch',...args,'--mode','compat','--expected',String(item.generation+3)]);
  }
  log('worker-fallback',{arc:'ARC-035',ownership:events(['status']).ownership,retainedTenantIDs:expected.split('\n')});
  // Pause the writer set and restore into a third, empty database. The full
  // CH/object and device/relay recovery checks have separate domain evidence.
  for(const name of [...processes.keys()].reverse())await stop(name);
  const dump=pgdump('arc_b');sql('CREATE DATABASE arc_restore');pgrestore('arc_restore',dump);
  const restoreEnv={...envB,INFRAPRO_DATABASE_URL:uri('arc_restore')};
  const epoch=events(['status'],restoreEnv).protocol.restore_epoch;
  events(['freeze','--epoch',epoch,'--reason','架构恢复核验'],restoreEnv);
  const next=events(['rotate-epoch','--epoch',epoch,'--reason','架构恢复核验'],restoreEnv);
  assert.notEqual(next.restore_epoch,epoch);assert(next.frozen);assert.equal(sql('SELECT id::text FROM tenants ORDER BY id','arc_restore'),expected);
  assert.equal(sql('SELECT state FROM monitoring_jobs WHERE id=\''+job+'\'','arc_restore'),'queued');
  log('empty-target-restore',{arc:'ARC-034',scope:'business PG and worker dispatch',frozen:true,oldEpoch:epoch,newEpoch:next.restore_epoch,dumpSHA256:createHash('sha256').update(dump).digest('hex'),retainedTenantIDs:expected.split('\n')});
  for(const mode of ['native','compose','mixed']){
    const dir=join(secret,mode);
    prepare({mode,directory:dir,'port-base':String(privatePort)});
    if(mode!=='native')docker(['compose','-f',join(dir,'compose.json'),'config','--quiet']);
    log(mode+'-template',{arc:'ARC-032',roleCount:profile(dir).roles.length});
  }
  await checkProfiles({sql, directory:secret,network,layout,keyring,envB,baseEnv:hostEnv,port:privatePort,origin,bin,child,stop,request,log});
  sql('CREATE DATABASE arc_slots');pgrestore('arc_slots',dump);
  const replicas=await checkReplicas({directory:secret,url:uri('arc_slots'),pg,envB,bin,child,stop,sql,port:privatePort,freePort:port});
  log('replica-quotas',{arc:'ARC-032',...replicas});
  const objectReport=join(out,'object-privileges.json');
  run('python3',[join(root,'scripts/architecture-object-profile-check.py'),'--layout',join(secret,'native'),'--output',objectReport]);
  const objects=JSON.parse(readFileSync(objectReport));
  assert.equal(objects.status,'passed');log('object-privileges',{arc:'ARC-032',roles:[...new Set(objects.checks.map(row=>row.role))],checks:objects.checks.length});
  const joint=join(secret,'joint-context.json');
  writeFileSync(joint,JSON.stringify({pg,application_bins:bin,database:envB.INFRAPRO_DATABASE_URL,redis:envB.INFRAPRO_REDIS_URL,redis_prefix:envB.INFRAPRO_REDIS_KEY_PREFIX,mfa:keyring,csrf:envB.INFRAPRO_CSRF_AUTH_KEY,origin}),{mode:0o600});
  run('python3',[join(root,'scripts/architecture-joint-check.py'),'--context',joint,'--output',secret],hostEnv,undefined,{timeout:20*60*1000});
  const jointReport=JSON.parse(readFileSync(join(secret,'joint-recovery.json')));
  assert.equal(jointReport.cold_starts.status,'passed');
  for(const role of jointReport.cold_starts.roles){
    assert(role.capacity?.report.boot_id,'quota sample at cold startup for '+role.role);
    if(role.capacity.configured.clickhouse.max_open)
      assert.equal(role.capacity.report.observed['quota.ch_connections'],role.capacity.configured.clickhouse.max_open);
  }
  const capacities=JSON.parse(readFileSync(join(secret,'native/resource-limits.json')));
  writeFileSync(join(out,'resource-limits.json'),JSON.stringify({...capacities,cold_start_reports:
    jointReport.cold_starts.roles.map(({role,capacity})=>({role,...capacity}))},null,2)+'\n',{mode:0o600});
  assert.deepEqual(jointReport.cold_starts.appliedRoleSQL,ledger.applied_role_sql);
  assert.deepEqual(jointReport.steps.role_grants.result.applied_role_sql,ledger.applied_role_sql);
  const installation = jointReport.steps.role_grants.result.schema_installation;
  const guard = jointReport.steps.monitoring.result;
  assert.equal(installation.cli_sha256, createHash('sha256').update(readFileSync(join(bin,'infrapro'))).digest('hex'));
  assert.equal(guard.mode, 'read_only');
  assert.equal(guard.access_revoked, true);
  assert.match(guard.backup_id, /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/);
  for (const snapshot of [installation.before, installation.after]) {
    assert.equal(snapshot.schema, 59);
    assert.deepEqual(snapshot.monitoring, guard);
  }
  assert.deepEqual(Object.keys(installation.before.epochs).sort(), ['audit','event','files','tasks']);
  for (const epoch of Object.values(installation.before.epochs))
    assert.match(epoch, /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/);
  assert.deepEqual(installation.after.epochs, installation.before.epochs);
  assert.deepEqual(installation.after.baseline_function, {
    name: 'architecture_fixed_baseline_metadata', arguments: 'uuid, uuid, text, text',
    result: baselineMetadata.result, security_definer: true, definition_sha256: baselineMetadata.sha256,
  });
  assert.deepEqual(installation.after.notification_invitation_function, {
    name: 'architecture_notification_invitation_targets', arguments: 'uuid, uuid, uuid[]',
    result: invitationMetadata.result, security_definer: true, definition_sha256: invitationMetadata.sha256,
    fixed_search_path: true, public_execute: false,
  });
  jointReport.applied_role_sql=ledger.applied_role_sql;
  jointReport.release=ledger.release;
  jointReport.private_evidence_directory=relative(out,join(secret,'joint-private'));
  writeFileSync(join(out,'role-cold-start.json'),JSON.stringify(jointReport.cold_starts,null,2)+'\n',{mode:0o600});
  writeFileSync(join(out,'joint-recovery.json'),JSON.stringify(jointReport,null,2)+'\n',{mode:0o600});
  log('joint-recovery',jointReport);
  ledger.status='passed';
} catch(err) {
  ledger.status='failed';
  if(containers.has(pg)){const dbErrors=spawnSync('docker',['logs',pg],{encoding:'utf8'}).stderr.split("\n").filter(line=>line.includes('permission denied'));ledger.databaseErrors=dbErrors.map(line=>line.match(/permission denied for (?:table|column|schema|function|sequence) [A-Za-z_0-9]+/)?.[0]).filter(Boolean); }
  const detail=err.stderr?.toString().split('\n').find(s=>s.includes('ERROR:')||s.includes('操作失败'))??'';
  ledger.error=err.code==='ERR_ASSERTION'?err.message:err.status!==undefined?'subprocess failed ('+err.status+') '+detail:err.message;
  console.error(ledger.error);process.exitCode=1;
} finally {
  for(const name of [...processes.keys()].reverse())await stop(name,false).catch(()=>{});
  if(!o.keep){
    for(const id of containers)try{docker(['rm','-f',id])}catch{ /* container or network was already removed */ }
    if(ownedNetwork)try{docker(['network','rm',ownedNetwork])}catch{ /* the disposable network was already removed */ }
  }
  ledger.endedAt=now();writeFileSync(join(out,'deployment.json'),JSON.stringify(ledger,null,2)+'\n',{mode:0o600});
  console.log(join(out,'deployment.json'));
}
