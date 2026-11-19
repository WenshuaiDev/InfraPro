// Start each process against its own role login, a real CH storage layout,
// private buckets and its own report file. The selected DB is a disposable clone.
import assert from 'node:assert/strict';
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { closeSync, existsSync, mkdirSync, mkdtempSync, openSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { prepare } from './architecture-deploy.mjs';
import { compareQuotaReport, resourceLimits } from './architecture-capacities.mjs';
import { provision } from './architecture-postgres.mjs';
import { checkAnalytics } from './architecture-clickhouse-check.mjs';
import { roleEnvironment, root } from './architecture-runtime.mjs';

const [config, output, pg, bin, cert, parserPackage, relayID,ch]=process.argv.slice(2);
const cfg=JSON.parse(readFileSync(config)),db=new URL(cfg.database_url);
assert.equal(db.hostname,'127.0.0.1');
assert.match(db.pathname,/^\/arc_roles_[a-f0-9]+$/);
const dir=resolve(output),layout=join(dir,'layout');
mkdirSync(dir,{recursive:true,mode:0o700});
const temp=realpathSync(mkdtempSync('/tmp/ip-role-')),controller=new AbortController();
const records=[];
const parent=Object.fromEntries(Object.entries(process.env).filter(([k])=>!k.startsWith('INFRAPRO_')));
const env={...parent,SSL_CERT_FILE:cert};
async function port(){
  const s=createServer();await new Promise((ok,no)=>s.once('error',no).listen(0,'127.0.0.1',ok));const p=s.address().port;await new Promise(ok=>s.close(ok));return p;
}
for(const [k,v] of Object.entries(cfg)){
  if(typeof v==='string')env['INFRAPRO_'+k.toUpperCase()]=v;
  else for(const [field,value] of Object.entries(v))env['INFRAPRO_'+k.toUpperCase()+'_'+field.toUpperCase()]=value;
}
const privatePort=await port();
prepare({mode:'local',directory:layout,'port-base':String(Math.min(privatePort,65527)),'clickhouse-database':cfg.monitoring_clickhouse_database});
const limits=resourceLimits(layout,{'runtime-config':config});
const slots=new Map(limits.slots.map(row=>[row.role,row]));
const appliedRoleSQL=provision({url:cfg.database_url,container:pg,directory:layout,localLogins:true});
const analytics=ch?checkAnalytics(layout,ch,cfg.monitoring_clickhouse_database):undefined;
const events=join(dir,'events.json');
writeFileSync(events,'{"mode":"pg","environment":"acceptance"}',{mode:0o600});
const ops=(...args)=>JSON.parse(execFileSync(join(bin,'event-relay'),args,{env:{...env,INFRAPRO_EVENTS_FILE:events},encoding:'utf8',stdio:['ignore','pipe','pipe']}));
const roles=['ingest-custody-worker','parsing-dispatch-worker','monitoring-publish-worker','monitoring-incremental-worker','monitoring-history-worker','monitoring-read-worker','notification-worker','task-index-worker','audit-worker','files-checker','files-worker','display-package-worker','spatial-worker','display-runtime','display-assets','ingest-worker','monitoring-worker'];
const parserToken=join(dir,'parser.token');
writeFileSync(parserToken,randomBytes(40).toString('hex'),{mode:0o600});
const parserPort=await port();
const parserEnv={...parent,INFRAPRO_PARSER_ADDR:'127.0.0.1:'+parserPort,INFRAPRO_PARSER_PACKAGE:parserPackage,INFRAPRO_PARSER_TOKEN_FILE:parserToken};
Object.assign(env,{INFRAPRO_PARSING_ENABLED:'true',INFRAPRO_PARSING_URL:'http://127.0.0.1:'+parserPort,INFRAPRO_PARSING_TOKEN_FILE:parserToken,
  INFRAPRO_FILES_TEMP_DIRECTORY:temp,INFRAPRO_FILES_CHECKER_SOCKET:join(temp,'checker.sock'),INFRAPRO_DISPLAY_RUNTIME_ADDR:'127.0.0.1:'+await port(),INFRAPRO_DISPLAY_ASSET_ADDR:'127.0.0.1:'+await port(),INFRAPRO_DISPLAY_SHELL_DIRECTORY:join(root,'apps/display-runtime/dist'),
  INFRAPRO_SPATIAL_PYTHON:join(root,'.artifacts/spatial-venv/bin/python'),INFRAPRO_SPATIAL_CONVERTER_SCRIPT:join(root,'tools/spatial-converter/convert.py'),INFRAPRO_SPATIAL_TEMP_DIRECTORY:temp});
const ingestion=JSON.parse(readFileSync(join(root,'deploy/ingestion/worker.example.json')));
const client={certificate_file:cert,key_file:join(resolve(cert,'..'),'tls.key'),ca_file:cert};
Object.assign(ingestion.relays[0],{id:relayID,url:'https://127.0.0.1:'+await port(),consumer:client,control:client,configuration_ref:'joint-test'});
env.INFRAPRO_INGESTION_FILE=join(dir,'ingestion.json');
writeFileSync(env.INFRAPRO_INGESTION_FILE,JSON.stringify(ingestion),{mode:0o600});
const owned=new Map();
const prerequisiteChecks={};
async function checkWorkerStorage(role, settings){
  const attempts=[];
  prerequisiteChecks[role]=attempts;
  const login=new URL(settings.INFRAPRO_DATABASE_URL);
  const db=spawnSync('docker',['exec','-i','-e','PGPASSWORD',pg,'psql','-XqAt','-v','ON_ERROR_STOP=1',
    '-h','127.0.0.1','-U',decodeURIComponent(login.username),'-d',login.pathname.slice(1)],{
    input:"BEGIN; SELECT revision FROM authorization_state WHERE id=1 FOR SHARE; SELECT to_regclass('monitoring_changes') IS NOT NULL AND to_regclass('monitoring_change_consumers') IS NOT NULL AND to_regclass('monitoring_queue_fairness') IS NOT NULL AND NOT EXISTS(SELECT 1 FROM derived_metric_versions WHERE engine_version<>'derived-engine/1' OR compiler_version<>'derived-compiler/1' OR schema_version<>'derived-rule/1') AND NOT EXISTS(SELECT 1 FROM analysis_config_versions WHERE engine_version<>'derived-engine/1' OR compiler_version<>'derived-compiler/1'); ROLLBACK;",
    env:{...parent,PGPASSWORD:decodeURIComponent(login.password)},encoding:'utf8'});
  const catalog={at:new Date().toISOString(),database_exit:db.status,
    compatible:db.status===0&&db.stdout.trim().split('\n').at(-1)==='t'};
  if(db.status!==0)catalog.error=db.stderr.trim().split('\n')[0];
  attempts.push(catalog);
  const save=()=>writeFileSync(join(dir,'worker-prerequisites.json'),JSON.stringify(prerequisiteChecks,null,2)+'\n',{mode:0o600});
  save();assert(catalog.compatible,role+' calculation catalog or lock unavailable');
  for(let attempt=0;attempt<12;attempt++){
    const result=spawnSync(join(bin,'infrapro'),['monitoring','storage','check'],{env:settings,encoding:'utf8',timeout:30000});
    attempts.push({at:new Date().toISOString(),storage_exit:result.status,detail:result.status===0?'ready':
      (result.stderr||result.error?.code||'failed').trim().split('\n').at(-1).replace(/(?:https?|postgres(?:ql)?):\/\/\S+/g,'[endpoint]')});
    save();
    if(result.status===0)return;
    await sleep(500);
  }
  throw Error(role+' physical table/replica readiness failed');
}
function start(role, executable, args, settings){
  const path=join(dir,role);mkdirSync(path,{mode:0o700});
  const report=join(path,'process.json'),fd=openSync(join(path,'process.log'),'a',0o600);
  const proc=spawn(executable,args,{cwd:root,env:{...settings,INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'arc-roles',INFRAPRO_DIAGNOSTICS_NODE_ID:'native',INFRAPRO_DIAGNOSTICS_REPLICA_ID:role+'-1',INFRAPRO_DIAGNOSTICS_REPORT_FILE:report},stdio:['ignore',fd,fd],signal:controller.signal});
  closeSync(fd);
  const closed=new Promise(done=>{proc.once('close',(code,signal)=>done({code,signal}));proc.once('error',()=>done({code:1}));});
  const entry={proc,closed,report};owned.set(role,entry);return entry;
}
async function verify(role,entry){
  for(let i=0;i<160;i++){
    if(existsSync(entry.report)){
      const doc=JSON.parse(readFileSync(entry.report));
      if(doc.phase==='ready'){
        // Readiness and metrics can be emitted in adjacent report updates at startup.
        let quota;
        try{quota=compareQuotaReport(slots.get(role),doc);}
        catch(error){if(!error.message.includes(' missing quota.')||i===159)throw error;await sleep(150);continue;}
        const [rssKiB,cpuPercent]=execFileSync('ps',['-p',String(entry.proc.pid),'-o','rss=','-o','%cpu='],{encoding:'utf8'}).trim().split(/\s+/).map(Number);
        return {role,ready:true,rssKiB,cpuPercent,capacity:{configured:slots.get(role),report:quota},lanes:doc.lanes.map(l=>l.id),metrics:doc.metrics??[]};
      }
    }
    if(entry.proc.exitCode!==null||entry.proc.signalCode)throw Error(role+' exited before readiness');
    await sleep(150);
  }
  throw Error(role+' did not reach readiness');
}
async function stop(role){
  const entry=owned.get(role);if(!entry)return;
  entry.proc.kill('SIGTERM');const closed=await Promise.race([entry.closed,sleep(90000,null,{ref:false})]);
  if(!closed){entry.proc.kill('SIGKILL');await entry.closed;throw Error(role+' exceeded its grace period');}
  owned.delete(role);assert.equal(closed.code,0,role+' graceful shutdown');
}
function switchRoles(mode){for(const owner of ops('status').ownership)if(owner.mode!==mode)ops('switch','--role',owner.role,'--expected',String(owner.generation),'--mode',mode,'--reason','隔离进程冷启动');}
try{
  const parserBin=join(resolve(parserPackage,'..'),'bin/parser');
  const parser=start('parser',parserBin,[],parserEnv);records.push(await verify('parser',parser));
  ops('resume','--reason','隔离进程冷启动');switchRoles('split');
  for(const role of roles){
    if(role==='ingest-worker')switchRoles('compat');
    const settings=roleEnvironment(role,env,layout);
    settings.INFRAPRO_EVENTS_FILE=events;
    const args=role==='files-checker'?['--directory',temp,'--socket',join(temp,'checker.sock')]:[];
    try{
      if(role==='monitoring-publish-worker')await checkWorkerStorage(role,settings);
      const proc=start(role,join(bin,role),args,settings);
      records.push(await verify(role,proc));
      if(role!=='files-checker')await stop(role);
    }catch(error){
      records.push({role,ready:false,error:error.message});
      await stop(role).catch(()=>{ /* preserve this startup error and test the next role */ });
    }
    if(role==='files-worker')await stop('files-checker');
    writeFileSync(join(dir,'cold-start.json'),JSON.stringify({status:'running',roles:records},null,2)+'\n',{mode:0o600});
  }
  await stop('parser');
  const failures=records.filter(r=>!r.ready);if(failures.length)throw Error('Cold startup failed for '+failures.map(r=>r.role).join(', '));
  writeFileSync(join(dir,'cold-start.json'),JSON.stringify({status:'passed',roles:records,analytics,prerequisiteChecks,appliedRoleSQL},null,2)+'\n',{mode:0o600});
} catch(error){
  writeFileSync(join(dir,'cold-start.json'),JSON.stringify({status:'failed',error:error.message,roles:records},null,2)+'\n',{mode:0o600});throw error;
} finally {
  for(const role of [...owned.keys()].reverse())await stop(role).catch(()=>{ /* preserve initial failed startup */ });
  rmSync(temp,{recursive:true,force:true});
}
