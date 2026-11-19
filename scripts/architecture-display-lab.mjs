import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes, randomInt } from 'node:crypto';
import { createServer, request as forward } from 'node:http';
import { closeSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, openSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { stop, waitURL } from './fixtures/parsing-lab.mjs';
import { activeDatabasePort, digest, launch, read, root, write } from './architecture-lab.mjs';
import { availableRange } from './architecture-serve.mjs';
import { campaignDisplayTopology, displayInflightProfile, displayProcessProfile } from './architecture-display-topology.mjs';
import { processEnvironment } from './architecture-process-resources.mjs';
import { httpStorage } from './architecture-http-storage.mjs';
import { displayFixture } from './architecture-display-fixture.mjs';
import { displayPostgres } from './architecture-display-postgres.mjs';
import { displayRoleConfigs } from './architecture-display-roles.mjs';

const exec=(cmd,args,opts={})=>execFileSync(cmd,args,{cwd:root,maxBuffer:96<<20,...opts});
const databaseUser='archtest';
const chContainer='infrapro-monitoring-contract-ch1-1';
const testEnv=(path)=>({...Object.fromEntries(Object.entries(process.env).filter(([k])=>!k.startsWith('INFRAPRO_'))),INFRAPRO_CONFIG:path,GOMAXPROCS:'4'});
// Bind all local consumers in a single checked block below the ephemeral
// range. An intervening object-proxy connection must never take an owner's port.
async function displayPorts(){
  const start=await availableRange(11,()=>randomInt(20000,28000));
  return {http:'127.0.0.1:'+start,shell:'127.0.0.1:'+(start+1),assets:'127.0.0.1:'+(start+2),objectProxy:start+3,privateBase:start+4};
}

// The budget is for the complete cohort. A has 12 PG connections in its
// API, 24 in the screen service, and 12 in its combined background worker.
const budgets={
  a:{
    api:[12,6,4,512],'display-runtime':[24,6,4,768],'monitoring-worker':[12,6,4,768]
  },
  b:{
    api:[0,0,1,64],'identity-api':[6,0,1,128],'control-api':[8,2,1,192],
    'monitoring-query-api':[10,6,2,384],'transfer-api':[3,1,1,128],
    'display-runtime':[6,0,1,128],'display-assets':[2,0,1,64],
    'monitoring-publish-worker':[3,2,1,192],'monitoring-incremental-worker':[3,2,1,192],
    'monitoring-history-worker':[4,3,1,320],'monitoring-read-worker':[3,2,1,256]
  }
};
for(const entries of Object.values(budgets)){
  const totals=[0,0,0,0];
  for(const values of Object.values(entries))values.forEach((v,i)=>totals[i]+=v);
  assert.deepEqual(totals,[48,18,12,2048]);
}
export const formalDisplayBudget = 'thirteen-application-threads';
export function displayBudgetTotal(roles) {
  const totals=[0,0,0,0];
  Object.values(roles).forEach(values=>values.forEach((value,i)=>{totals[i]+=value;}));
  return totals;
}
export function displayRoleBudget(side,profile='original') {
  const roles=structuredClone(budgets[side]);
  assert(['original','two-transfers','two-transfers-two-login-threads',formalDisplayBudget].includes(profile));
  if(profile===formalDisplayBudget){
    const profiles=read(join(root,displayProcessProfile)),inflight=read(join(root,displayInflightProfile));
    const published=side==='a'?profiles.schema58:profiles.roles;
    assert.deepEqual(Object.keys(published).sort(),Object.keys(roles).sort());
    for(const [role,quota] of Object.entries(roles)){
      if(side==='b' && quota[1] && inflight[role])quota[1]=Math.min(6,inflight[role]);
      quota[2]=published[role].go_threads;quota[3]=published[role].go_memory_mib;
    }
  } else if(side==='b' && profile!=='original'){
    // Two protected streams must be able to start together. Reserve the
    // original three Argon2 work areas within the unchanged cohort heap.
    roles['transfer-api'][1]=2;
    roles['monitoring-incremental-worker'][1]=1;
    roles['identity-api'][3]=256;
    roles['control-api'][3]=128;
    roles['monitoring-incremental-worker'][3]=128;
    if(profile==='two-transfers-two-login-threads'){
      roles['identity-api'][2]=2;
      roles['monitoring-query-api'][2]=1;
    }
  }
  assert.deepEqual(displayBudgetTotal(roles),[48,18,profile===formalDisplayBudget?13:12,2048]);
  return roles;
}
function ch(db,query,{input,output,container=chContainer}={}) {
  assert.match(db,/^(?:arc_|monitoring_it_)[a-z0-9_]+$/);
  const fd=output && openSync(output,'w',0o600);
  try{return exec('docker',['exec',...(input?['-i']:[]),container,'clickhouse-client','--database',db,'--query',query],
    input?{input,encoding:'utf8'}:output?{stdio:['ignore',fd,'pipe']}:{encoding:'utf8'})?.trim();}
  finally{if(output)closeSync(fd);}
}
function pg(lab,db,sql){
  assert.match(db,/^arc_[a-z0-9_]+$/);
  return exec('docker',['exec','-i',lab.container,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U',databaseUser,'-d',db],{input:sql,encoding:'utf8'}).trim();
}
const hashRows=table=>'SELECT count(),groupBitXor(cityHash64(toString(tuple(*)))) FROM '+table;
function fileCommand(cmd,args,path,direction){
  const fd=openSync(path,direction==='out'?'w':'r',0o600);
  try{exec(cmd,args,{stdio:direction==='out'?['ignore',fd,'pipe']:[fd,'pipe','pipe']});}finally{closeSync(fd);}
}

// Only facts from the disposable SDK fixture enter this snapshot. Each trial
// clones the same IDs, file bytes and formal ClickHouse indexes independently.
export async function displaySnapshot(dir, existing, resumeDirectory) {
  const lab=read(join(dir,'private/lab.json')),base=read(join(dir,'baseline.json'));
  const parent=join(dir,'private','display-load');mkdirSync(parent,{recursive:true,mode:0o700});
  const destination=resumeDirectory || mkdtempSync(join(parent,'snapshot-'));
  const seed=existing?{fixture:read(existing),close:async()=>{}}:await displayFixture(dir,'a');
  try {
    const s=seed.fixture;
    const db=new URL(s.database),dbName=db.pathname.slice(1),schema=db.searchParams.get('search_path');
    assert.equal(dbName,'arc_display_test');assert.match(schema,/^identity_test_[a-z0-9]+$/);
    assert.match(s.tenant,/^[0-9a-f-]{36}$/);
    const cfg={...read(lab.a.config),database_url:s.database,monitoring_source_bucket:'contract-source',
      monitoring_clickhouse_database:s.clickhouse,mfa_keyring_file:s.keyring};
    const file=join(destination,'runtime.json');write(file,cfg);
    exec('go',['build','-o',join(dir,'fixture'),join(root,'scripts/architecture-fixture.go')],{cwd:base.source});
    if(!existsSync(join(destination,'objects.json')))exec(join(dir,'fixture'),['snapshot',destination],{env:{...testEnv(file),ARC_DISPLAY_SNAPSHOT_TENANT:s.tenant}});
    const dump=join(destination,'business.pgdump');
    if(!existsSync(dump))fileCommand('docker',['exec',lab.container,'pg_dump','-F','c','--no-owner','-U',databaseUser,'-d',dbName],dump,'out');
    const names=ch(s.clickhouse,"SELECT name FROM system.tables WHERE database=currentDatabase() AND endsWith(engine,'MergeTree') ORDER BY name").split('\n').filter(Boolean);
    const tables=[];
    for(const table of names){
      assert.match(table,/^[a-z0-9_]+$/);
      const native=join(destination,table+'.native'),fingerprint=ch(s.clickhouse,hashRows(table));
      ch(s.clickhouse,'SELECT * FROM '+table+' ORDER BY ALL FORMAT Native',{output:native});
      tables.push({name:table,fingerprint,sha256:digest(readFileSync(native))});
    }
    const keyring=join(destination,'mfa.json');copyFileSync(s.keyring,keyring);
    write(join(destination,'private.json'),{...s,keyring,schema,createdAt:new Date().toISOString()});
    write(join(destination,'manifest.json'),{tables,pgSHA256:digest(readFileSync(dump)),objects:read(join(destination,'objects.json')),sourceBytes:s.sourceBytes,sourceSHA256:s.sourceSHA256,instances:s.sessions.length,channels:s.channels.length,observations:100000,resources:2600});
    return destination;
  }finally{await seed.close();}
}

// The frozen broad-directory statement chooses a quadratic nested loop with
// PostgreSQL 17's stock planner and reaches its original 10 s timeout. Apply
// the same database-local planner settings to both measured, disposable DBs.
// A default-planner calibration is retained independently.
export function matchedDisplayPlanner(lab,db) {
  assert.match(db,/^arc_load_[a-f0-9]{12}_test$/);
  for(const setting of ['enable_nestloop','jit'])pg(lab,db,'ALTER DATABASE '+db+' SET '+setting+' TO off');
  const settings=pg(lab,db,"SELECT jsonb_build_object('enable_nestloop',current_setting('enable_nestloop'),'jit',current_setting('jit'))");
  const effective=JSON.parse(settings);
  assert.equal(effective.enable_nestloop,'off');assert.equal(effective.jit,'off');return effective;
}
export async function displayStack(dir,snapshot,side,round,{planner='matched',releaseDirectory=dir,prepareOnly=false,roleSQLDirectory,budgetProfile='original'}={}) {
  const split=side==='b',sourceLab=read(join(dir,'private/lab.json')),base=read(join(dir,'baseline.json'));
  const s=read(join(snapshot,'private.json'));
  const lab=await displayPostgres(dir,sourceLab,s);
  const services=await httpStorage(join(dir,'private/display-load/infra'),digest(lab.id+'-monitoring-scale').slice(0,12));
  const id=randomBytes(6).toString('hex'),dbName='arc_load_'+id+'_test',chName='arc_load_'+id,bucket='arc-load-'+id;
  const workspace=join(dir,'private/display-load',id);mkdirSync(workspace,{mode:0o700});
  write(join(workspace,'postgres.json'),{container:lab.container,containerLabel:lab.containerLabel});
  const {http,shell,assets,objectProxy,privateBase}=await displayPorts();
  const runtime='http://'+shell,api='http://'+http,origin=api;
  const sql=q=>pg(lab,dbName,'SET search_path TO '+s.schema+';\n'+q);
  const bin=split?read(join(releaseDirectory,'candidate.json')).bin:base.bin;
  const cfg=read(lab[side].config),endpoint=new URL(s.database);
  assert.equal(endpoint.hostname,'127.0.0.1');
  endpoint.port=String(activeDatabasePort(lab));
  endpoint.pathname='/'+dbName;
  Object.assign(cfg,{database_url:endpoint.href,redis_url:s.redis,redis_key_prefix:'infrapro:arc-display-'+id+':identity:v1:',
    csrf_auth_key:s.csrf,mfa_keyring_file:s.keyring,public_console_url:api+'/',trusted_origins:api,http_addr:http,
    monitoring_object_endpoint:services.s3,monitoring_clickhouse_database:chName,monitoring_source_bucket:bucket,monitoring_clickhouse_replicas:services.ch,
    monitoring_temp_directory:workspace,display:{...cfg.display,runtime_origin:runtime,runtime_addr:shell,asset_origin:'http://'+assets,asset_addr:assets,lease:'120s',pool_size:'24'}});
  for(const domain of ['display','files','audit'])if(cfg[domain]?.object_endpoint)cfg[domain].object_endpoint=services.s3;
  const config=join(workspace,'runtime.json');write(config,cfg);
  const env=testEnv(config),topology=join(workspace,'topology');
  exec('docker',['exec',lab.container,'createdb','-U',databaseUser,dbName]);
  const settings=planner==='matched'?matchedDisplayPlanner(lab,dbName):JSON.parse(pg(lab,dbName,"SELECT jsonb_build_object('enable_nestloop',current_setting('enable_nestloop'),'jit',current_setting('jit'))"));
  const manifests=read(join(snapshot,'manifest.json'));
  fileCommand('docker',['exec','-i',lab.container,'pg_restore','--exit-on-error','--no-owner','--no-privileges','-U',databaseUser,'-d',dbName],join(snapshot,'business.pgdump'),'in');
  pg(lab,dbName,'CREATE EXTENSION IF NOT EXISTS pg_stat_statements WITH SCHEMA public');
  exec(join(bin,'infrapro'),['database','migrate'],{env});
  exec(join(bin,'infrapro'),['monitoring','storage','init'],{env});
  for(const t of manifests.tables){
    const existing=ch(chName,hashRows(t.name),{container:services.chContainer});
    if(existing!==t.fingerprint){
      assert(existing.startsWith('0\t'),'Target ClickHouse table already contains other facts');
      const data=readFileSync(join(snapshot,t.name+'.native'));assert.equal(digest(data),t.sha256);
      ch(chName,'INSERT INTO '+t.name+' FORMAT Native',{input:data,container:services.chContainer});
    }
    assert.equal(ch(chName,hashRows(t.name),{container:services.chContainer}),t.fingerprint);
  }
  const objects=JSON.parse(exec(join(dir,'fixture'),['restore',snapshot],{env}));assert.deepEqual(objects,manifests.objects);
  const roles=split?['identity-api','control-api','monitoring-query-api','transfer-api','api','display-runtime','display-assets','monitoring-publish-worker','monitoring-incremental-worker','monitoring-history-worker','monitoring-read-worker']:['api','display-runtime','monitoring-worker'];
  if(split||budgetProfile===formalDisplayBudget)campaignDisplayTopology(releaseDirectory,snapshot,topology,privateBase,budgetProfile===formalDisplayBudget);
  if(split){
    const events=args=>JSON.parse(exec(join(bin,'event-relay'),args,{env}));
    for(const role of events(['status']).ownership)events(['switch','--expected',String(role.generation),'--mode','split','--role',role.role,'--reason','隔离大屏负载']);
  }
  const storage={clickhouse:services.chContainer,objects:services.chContainer.replace(/-ch1-1$/,'-s3-1')};
  const metadata={side,round,snapshot:digest(readFileSync(join(snapshot,'manifest.json'))),version:split?read(join(releaseDirectory,'candidate.json')).inputs.sourceSHA256:base.commit,
    workspace,planner:settings,storage,database:lab.container,objectProxy,budgetProfile,phase:prepareOnly?'prepared':'started'};
  write(join(workspace,'cohort.json'),metadata);
  if(prepareOnly)return metadata;
  return startDisplayProcesses({dir,s,sql,side,round,snapshot,workspace,api,origin,runtime,bin,cfg,lab,dbName,chName,bucket,planner:settings,storage,roleSQLDirectory,objectProxy,budgetProfile},roles,env,topology);
}

async function limitedObjects(endpoint,limit,listenPort) {
  const origin=new URL(endpoint);
  assert.equal(origin.hostname,'127.0.0.1');
  const waiting=new Set(),sockets=new Set(),metrics={limit,active:0,peak:0,waitingPeak:0,completed:0};
  const server=createServer((req,res)=>{
    let upstream,active=false;
    const serve=()=>{
      if(res.destroyed)return;
      active=true;metrics.active++;metrics.peak=Math.max(metrics.peak,metrics.active);
      upstream=forward({hostname:origin.hostname,port:origin.port,path:req.url,method:req.method,headers:req.headers},reply=>{
        res.writeHead(reply.statusCode,reply.headers);reply.on('error',()=>res.destroy());reply.pipe(res);
      });
      upstream.on('error',()=>{if(!res.headersSent)res.writeHead(503);res.end();});
      req.pipe(upstream);
    };
    res.once('close',()=>{
      waiting.delete(serve);upstream?.destroy();
      if(active){metrics.active--;metrics.completed++;}
      if(waiting.size&&metrics.active<limit){const next=waiting.values().next().value;waiting.delete(next);next();}
    });
    if(metrics.active<limit)serve();else {waiting.add(serve);metrics.waitingPeak=Math.max(metrics.waitingPeak,waiting.size);}
  });
  server.on('connection',socket=>{sockets.add(socket);socket.on('close',()=>sockets.delete(socket));});
  await new Promise((accept,reject)=>server.once('error',reject).listen(listenPort,'127.0.0.1',accept));
  return {origin:'http://127.0.0.1:'+server.address().port,metrics,close:()=>{waiting.clear();for(const socket of sockets)socket.destroy();return new Promise(done=>server.close(done));}};
}

async function startDisplayProcesses(stack,roles,env,topology) {
  const {side,bin,workspace,api,runtime}=stack,split=side==='b',processes={};
  const cohortFile=join(workspace,'cohort.json');
  const budgetProfile=stack.budgetProfile??(existsSync(cohortFile)?read(cohortFile).budgetProfile:undefined)??'original';
  const roleBudget=displayRoleBudget(side,budgetProfile);
  const deployed=budgetProfile===formalDisplayBudget?read(join(topology,'process-budgets.json'))[side]:null;
  const objects=await limitedObjects(stack.cfg.monitoring_object_endpoint,16,stack.objectProxy);
  const processConfig={...stack.cfg,monitoring_object_endpoint:objects.origin};
  for(const domain of ['display','files','audit'])if(processConfig[domain]?.object_endpoint===stack.cfg.monitoring_object_endpoint)processConfig[domain]={...processConfig[domain],object_endpoint:objects.origin};
  const local=join(workspace,'limited-runtime.json');write(local,processConfig);
  let closing;
  const signalRoles=()=>{for(const child of Object.values(processes))if(child.exitCode===null&&child.signalCode===null)child.kill('SIGTERM');};
  const terminate=()=>{signalRoles();void close().finally(()=>process.exit(143));};
  const interrupt=()=>{signalRoles();void close().finally(()=>process.exit(130));};
  process.once('exit',signalRoles);process.once('SIGTERM',terminate);process.once('SIGINT',interrupt);
  function close(){
    closing??=(async()=>{
      try{
        await Promise.all(Object.values(processes).filter(p=>p.exitCode===null&&p.signalCode===null).map(stop));
        await objects.close();
      }finally{
        process.removeListener('exit',signalRoles);process.removeListener('SIGTERM',terminate);process.removeListener('SIGINT',interrupt);
      }
    })();
    return closing;
  }
  try{
    const installed=split?displayRoleConfigs(stack,processConfig,roles,roleBudget):null;
    for(const role of roles){
      const [pg,,cores,heapMiB]=roleBudget[role];
      const processBudget=deployed?processEnvironment(deployed[role]):{GOMAXPROCS:String(cores),GOMEMLIMIT:String(heapMiB)+'MiB'};
      assert.equal(processBudget.GOMAXPROCS,String(cores));assert.equal(processBudget.GOMEMLIMIT,heapMiB+'MiB');
      const roleEnv={...env,INFRAPRO_CONFIG:local,...installed?.roleEnv(role),...processBudget};
      if(split){
        const manifest=join(topology,'roles',role,'architecture.json'),runtime=read(manifest);
        if(pg)(runtime.pg_connections??={})[role]=pg;
        // The normal deployment projects the same total into the outbound
        // gateway and the receiving role. A single endpoint serves two replicas.
        if(deployed){
          for(const [owner,[,ch]] of Object.entries(roleBudget))if(ch)assert.equal(Math.min(6,runtime.max_inflight[owner]),ch);
        }else{
          const [,ch]=roleBudget[role];
          if(ch)(runtime.max_inflight??={})[role]=ch===6?Math.max(ch,runtime.max_inflight?.[role]||12):ch;
        }
        write(manifest,runtime);roleEnv.INFRAPRO_ARCHITECTURE_FILE=manifest;
      }
      const path=join(workspace,role+'.log');
      processes[role]=launch(join(bin,role),[],roleEnv,path);
    }
    await Promise.all([api+'/readyz',runtime+'/readyz'].map(waitURL));
    for(const [name,p] of Object.entries(processes))if(p.exitCode!==null)throw Error('Load role failed '+name);
    write(join(workspace,'pids.json'),Object.fromEntries(Object.entries(processes).map(([n,p])=>[n,p.pid])));
    return {...stack,processes,close,budget:{profile:budgetProfile,deployed,objectRequests:objects.metrics,roles:roleBudget,roleLogins:installed?.receipt,fields:['pgConnections','clickhouseConnections','goSchedulerThreads','goHeapMiB'],total:displayBudgetTotal(roleBudget)}};
  }catch(e){await close();throw e;}
}

// Replay a retained test run after inspecting an interrupted calibration.
export async function resumeDisplayStack(dir,snapshot,workspace,side,{workers=true,releaseDirectory=dir,round=0,usePrepared=false,roleSQLDirectory}={}) {
  assert.match(workspace.slice(workspace.lastIndexOf('/')+1),/^[a-f0-9]{12}$/);
  const lab={...read(join(dir,'private/lab.json')),...(existsSync(join(workspace,'postgres.json'))?read(join(workspace,'postgres.json')):{})},s=read(join(snapshot,'private.json')),cfg=read(join(workspace,'runtime.json'));
  const url=new URL(cfg.database_url),dbName=url.pathname.slice(1);
  assert.match(dbName,/^arc_load_[a-f0-9]{12}_test$/);assert.equal(url.hostname,'127.0.0.1');
  const split=side==='b',bin=split?read(join(releaseDirectory,'candidate.json')).bin:read(join(dir,'baseline.json')).bin;
  const file=join(workspace,'runtime.json'),meta=join(workspace,'cohort.json');
  const cohort=existsSync(meta)?read(meta):{};
  const retained=join(workspace,'retained-clickhouse.json');
  if(existsSync(retained)){
    const receipt=read(retained);
    assert.equal(receipt.database,cfg.monitoring_clickhouse_database);
    assert.equal(receipt.container,cohort.storage.clickhouse);
    for(const name of receipt.tables){
      assert.match(name,/^[a-z][a-z0-9_]*$/);
      if(ch(receipt.database,'EXISTS TABLE '+name,{container:receipt.container})==='0')
        ch(receipt.database,'ATTACH TABLE '+name,{container:receipt.container});
    }
  }
  const env=testEnv(file),topology=join(workspace,'topology');
  let objectProxy=cohort.objectProxy??await availableRange(1,()=>randomInt(28600,30000));
  if(usePrepared){
    assert.equal(cohort.phase,'prepared');assert.equal(cohort.round,round);assert.equal(cohort.side,side);
    assert.equal(cohort.snapshot,digest(readFileSync(join(snapshot,'manifest.json'))));
    assert.equal(cohort.version,split?read(join(releaseDirectory,'candidate.json')).inputs.sourceSHA256:read(join(dir,'baseline.json')).commit);
    assert.equal(cohort.database,lab.container);
    const active=await displayPostgres(dir,read(join(dir,'private/lab.json')),s);
    assert.equal(active.container,cohort.database);
    url.port=String(activeDatabasePort(active));cfg.database_url=url.href;
    const ports=await displayPorts();
    objectProxy=ports.objectProxy;
    cfg.http_addr=ports.http;cfg.public_console_url='http://'+cfg.http_addr+'/';cfg.trusted_origins='http://'+cfg.http_addr;
    cfg.display.runtime_addr=ports.shell;cfg.display.runtime_origin='http://'+cfg.display.runtime_addr;
    cfg.display.asset_addr=ports.assets;cfg.display.asset_origin='http://'+cfg.display.asset_addr;
    if(split||cohort.budgetProfile===formalDisplayBudget)
      campaignDisplayTopology(releaseDirectory,snapshot,topology,ports.privateBase,cohort.budgetProfile===formalDisplayBudget);
    write(file,cfg);write(meta,{...cohort,objectProxy,phase:'started',startedAt:new Date().toISOString()});
  }
  const roles=split?['identity-api','control-api','monitoring-query-api','transfer-api','api','display-runtime','display-assets','monitoring-publish-worker','monitoring-incremental-worker','monitoring-history-worker','monitoring-read-worker']:['api','display-runtime','monitoring-worker'];
  const sql=q=>pg(lab,dbName,'SET search_path TO '+s.schema+';\n'+q);
  return startDisplayProcesses({dir,s,sql,side,round,planner:cohort.planner,storage:cohort.storage,snapshot,workspace,api:'http://'+cfg.http_addr,origin:'http://'+cfg.http_addr,runtime:cfg.display.runtime_origin,bin,cfg,lab,dbName,chName:cfg.monitoring_clickhouse_database,bucket:cfg.monitoring_source_bucket,roleSQLDirectory,objectProxy},roles.filter(role=>workers || !role.endsWith('worker')),env,topology);
}

// Preserve the exact formal parts and their catalog entries on disk. Only the
// unique namespace of a stopped, sealed display trial may be parked.
export function retainDisplayStorage(stack) {
  assert.match(stack.chName,/^arc_load_[a-f0-9]{12}$/);
  assert.equal(stack.workspace.split('/').at(-1),stack.chName.slice('arc_load_'.length));
  const container=stack.storage.clickhouse;
  assert.match(container,/^arc-http-storage-[a-f0-9]{12}-ch1-1$/);
  const readQuery=query=>ch(stack.chName,query,{container});
  const names=readQuery("SELECT name FROM system.tables WHERE database=currentDatabase() ORDER BY name").split('\n').filter(Boolean);
  const file=join(stack.workspace,'retained-clickhouse.json');
  const previous=existsSync(file)?read(file):{tables:[]};
  const tables=[...new Set([...previous.tables,...names])].sort();
  for(const name of tables)assert.match(name,/^[a-z][a-z0-9_]*$/);
  const receipt={database:stack.chName,container,tables,at:new Date().toISOString()};
  write(file,{...receipt,state:'parking'});
  if(names.length)exec('docker',['exec','-i',container,'clickhouse-client','--database',stack.chName,'--multiquery'],{
    input:names.map(name=>'DETACH TABLE IF EXISTS '+name+' PERMANENTLY SYNC;').join('\n')+'\n'
  });
  write(file,{...receipt,state:'retained'});
  return receipt;
}
