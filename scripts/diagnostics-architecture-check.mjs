// Observe each actual command through the deployed UID, executable and process
// report checks. A named pipe holds startup in configuration I/O long enough
// to observe it, then releases invalid input. No business listeners are opened.
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { constants, closeSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, openSync,
  readFileSync, realpathSync, rmSync, writeFileSync, writeSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { randomBytes, createHash } from 'node:crypto';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { setTimeout as delay } from 'node:timers/promises';
import { checkCoverage } from './diagnostics-coverage.mjs';
import { checkContainer } from './diagnostics-architecture-container.mjs';
import { checkDeploymentSlots, checkRunningReplicaProxy } from './diagnostics-deployment-slots.mjs';
import { checkoutApplicationDigest } from './architecture-runtime.mjs';

const coverage = checkCoverage();
const repo = resolve('.');
const output = resolve(process.argv[2] || '.artifacts/architecture-refactor/diagnostics-probes.json');
const scratch = mkdtempSync(join(realpathSync(tmpdir()), 'infrapro-arch-probes-'));
const bin = join(scratch,'bin'), reports = join(scratch,'reports'), config = join(scratch,'diagnostics');
const instance = 'arc-diag-' + randomBytes(6).toString('hex');
const children = new Map();
const configPipes = [];
const baseline = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_')));
const read = (path) => JSON.parse(readFileSync(path,'utf8'));
const fileDigest = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const run = (name,args,options={}) => execFileSync(name,args,{cwd:repo,env:baseline,stdio:['ignore','pipe','pipe'],timeout:600000,...options});
const freePort = () => new Promise((ok,no) => {
  const s=createServer();s.once('error',no).listen(0,'127.0.0.1',()=>{const port=s.address().port;s.close(()=>ok(port));});
});

function launch(name,binary,args,env) {
  const child=spawn(binary,args,{env:{...baseline,...env},stdio:'ignore'});
  const stopped=new Promise((ok,no)=>{child.once('error',no);child.once('exit',(code,signal)=>ok({code,signal}));});
  children.set(name,{child,stopped});
}
async function waitFor(check,message) {
  for(let i=0;i<150;i++) { if(await check()) return; await delay(50); }
  throw Error(message);
}
function collect() {
  const p=spawnSync(join(bin,'infrapro'),['diagnostics','config','validate','--server',join(config,'server.json'),'--probe'],{env:baseline,encoding:'utf8',timeout:45000});
  if(![0,3].includes(p.status)) throw Error('Independent probe command failed: '+p.status);
  return Object.fromEntries(JSON.parse(p.stdout).map(({probe_id,outcome,reason_code})=>[probe_id,{outcome,reason_code}]));
}
function fifo(role) {
  const dir=join(reports,role);mkdirSync(dir,{mode:0o700});
  const name=join(dir,'startup.pipe');run('mkfifo',[name]);
  configPipes.push(openSync(name,constants.O_RDWR | constants.O_NONBLOCK));
  return name;
}
const results={started_at:new Date().toISOString(),coverage,processes:[],broker:[],passed:false};
try {
  mkdirSync(bin,{mode:0o700});mkdirSync(reports,{mode:0o700});
  const {roles}=read(join(repo,'deploy/architecture/roles.json'));
  const names=Object.keys(roles).filter(id=>roles[id].module==='services/api'&&['split','standby'].includes(roles[id].mode));
  const cli=join(bin,'infrapro');
  const selected = join(dirname(output), 'candidate.evaluated.json');
  const manifest = join(existsSync(selected) ? read(selected).directory : dirname(output), 'candidate.json');
  let release;
  if (existsSync(manifest)) {
    release = read(manifest);
    if (checkoutApplicationDigest(repo) !== release.inputs.sourceSHA256) throw Error('Diagnostic probe sources differ from the declared release');
    for (const name of ['infrapro', ...names]) {
      const binary = join(bin, name);
      copyFileSync(join(release.bin, name), binary, constants.COPYFILE_FICLONE);
      if (fileDigest(binary) !== release.binaries[name]) throw Error('Diagnostic probe binary differs from the declared release: ' + name);
    }
    results.release = {source_sha256: release.inputs.sourceSHA256, cli_sha256: release.binaries.infrapro};
  } else {
    run('go',['build','-trimpath','-o',bin+'/', './tools/infrapro/cmd/infrapro',...names.map(id=>'./services/api/cmd/'+id)]);
  }
  const port=await freePort(),clientPort=await freePort();
  const target=run('go',['env','GOOS','GOARCH'],{encoding:'utf8'}).trim().split('\n');
  const broker=join(repo,'build/events',target.join('-'),'nats-server');
  if(!existsSync(broker))run(process.execPath,['scripts/architecture-broker-build.mjs',...target]);
  const brokerConfig=join(scratch,'broker.conf'),brokerPID=join(scratch,'broker.pid');
  writeFileSync(brokerPID,'',{mode:0o600,flag:'wx'});
  run(cli,['diagnostics','config','init','--directory',config,'--instance-id',instance,'--profile','native','--topology','split','--compat-api','--runtime-uid',String(process.getuid()),'--bin-directory',bin,'--report-directory',reports,
    '--broker-pid-file',brokerPID,'--broker-executable',broker,'--broker-monitor-port',String(port)]);
  writeFileSync(brokerConfig,`listen: 127.0.0.1:${clientPort}\nhttp: 127.0.0.1:${port}\njetstream { store_dir: ${JSON.stringify(join(scratch,'js'))} }\n`,{mode:0o600});
  launch('nats-server',broker,['-c',brokerConfig,'-P',brokerPID],{});
  for(const role of names) {
    const input=fifo(role);
    launch(role,join(bin,role),[],{INFRAPRO_CONFIG:input,INFRAPRO_ARCHITECTURE_FILE:input,INFRAPRO_EVENTS_FILE:input,
      INFRAPRO_DIAGNOSTICS_INSTANCE_ID:instance,INFRAPRO_DIAGNOSTICS_NODE_ID:'local',
      INFRAPRO_DIAGNOSTICS_REPLICA_ID:role+'-1',INFRAPRO_DIAGNOSTICS_REPORT_FILE:join(reports,role,'process.json')});
  }
  await waitFor(()=>names.every(id=>existsSync(join(reports,id,'process.json'))),'A real binary did not produce its starting report');
  await waitFor(async()=>{try{return(await fetch(`http://127.0.0.1:${port}/healthz?js-enabled-only=true`,{signal:AbortSignal.timeout(300)})).ok;}catch{return false;}},'Broker did not become ready');
  const live=collect();
  for(const role of names) {
    const report=read(join(reports,role,'process.json'));
    const presence=live[role+'-presence'],self=live[role+'-report'];
    if(report.phase!=='starting'||!report.process.boot_id||presence?.outcome!=='ok'||self?.outcome!=='ok')throw Error('Live probe mismatch: '+role+' '+JSON.stringify({presence,self}));
    results.processes.push({role,executable_sha256:fileDigest(join(bin,role)),phase:report.phase,presence,report:self});
  }
  if(live['nats-server-ready'].outcome!=='ok'||live['nats-server-presence'].outcome!=='ok')throw Error('Broker process or JetStream readiness probe failed');
  results.broker.push({state:'jetstream_running',...live['nats-server-ready'],presence:live['nats-server-presence']});
  while(configPipes.length) {const fd=configPipes.pop();writeSync(fd,Buffer.from('malformed-input\n'));closeSync(fd);}
  children.get('nats-server').child.kill('SIGTERM');
  await waitFor(()=>[...children.values()].every(({child})=>child.exitCode!==null||child.signalCode!==null),'Owned binaries did not exit');
  const stopped=collect();
  for(const item of results.processes) {
    const name=item.role, {code}=await children.get(name).stopped;
    const report=read(join(reports,name,'process.json')),presence=stopped[name+'-presence'],self=stopped[name+'-report'];
    if(!code||report.phase!=='failed'||!report.lanes.some(l=>l.id==='startup'&&l.state==='blocked')||presence?.reason_code!=='process_not_found'||self?.reason_code!=='process_not_found')throw Error('Stopped probe mismatch: '+name+' '+JSON.stringify({code,phase:report.phase,presence,self}));
    Object.assign(item,{exit_code:code,terminal_phase:report.phase,terminal_reason:report.lanes.find(l=>l.id==='startup').reason_code,after_exit:{presence,report:self}});
  }
  if(stopped['nats-server-ready'].outcome!=='failed'||stopped['nats-server-presence'].outcome==='ok')throw Error('Stopped broker reported healthy');
  results.broker.push({state:'jetstream_stopped',...stopped['nats-server-ready'],presence:stopped['nats-server-presence']});
  results.container=await checkContainer({scratch,cli,instance,uid:process.getuid(),env:baseline});
  results.deployment_slots=checkDeploymentSlots(cli,scratch);
  results.replica_proxy=await checkRunningReplicaProxy(cli,scratch);
  if (release && checkoutApplicationDigest(repo) !== release.inputs.sourceSHA256) throw Error('Diagnostic probe sources changed during this run');
  results.passed=true;
} finally {
  for(const fd of configPipes) closeSync(fd);
  for(const {child,stopped} of children.values()) {if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL');await stopped;}
  rmSync(scratch,{recursive:true,force:true});
  results.finished_at=new Date().toISOString();mkdirSync(dirname(output),{recursive:true});
  writeFileSync(output,JSON.stringify(results,null,2)+'\n',{mode:0o600});
}
console.log(`Observed ${results.processes.length} real split/standby commands and JetStream before and after failure. Evidence: ${output}`);
