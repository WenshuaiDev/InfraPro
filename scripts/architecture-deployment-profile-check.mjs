// Runs the rendered Compose application with release binaries and an unchanged
// nginx proxy, then a mixed deployment against the same already-upgraded DB.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { prepare } from './architecture-deploy.mjs';
import { compareQuotaReport, resourceLimits } from './architecture-capacities.mjs';
import { processEnvironment } from './architecture-process-resources.mjs';
import { applicationSourceDigest, checkoutApplicationDigest, httpOwners, roleEnvironment, root } from './architecture-runtime.mjs';

const digest = bytes => createHash('sha256').update(bytes).digest('hex');

export function selectedProfileInputs(directory, bin) {
  const releaseDir=dirname(bin),manifest=join(releaseDir,'candidate.json');
  if(!existsSync(manifest))return {source:root,console:join(root,'apps/console/dist'),proof:{origin:'checkout'}};
  const selected=JSON.parse(readFileSync(manifest)),expected=selected.inputs.sourceSHA256;
  const consoleDir=join(selected.release,'console'),index=selected.staticFiles['console/index.html'];
  assert.equal(digest(readFileSync(join(consoleDir,'index.html'))),index);
  const archived=join(releaseDir,'application-source.json');
  if(!existsSync(archived)){
    assert.equal(checkoutApplicationDigest(root),expected);
    return {source:root,console:consoleDir,proof:{origin:'verified-checkout',source_sha256:expected,console_index_sha256:index}};
  }
  const record=JSON.parse(readFileSync(archived));
  assert.equal(record.manifest_sha256,digest(readFileSync(manifest)));
  assert.equal(record.application_source_sha256,expected);
  assert.equal(record.archive,'application-source.tar.gz');
  const archive=join(releaseDir,record.archive);
  assert.equal(digest(readFileSync(archive)),record.archive_sha256);
  const source=join(directory,'linux-selected-source');
  mkdirSync(source,{mode:0o700});
  execFileSync('python3',['-c',[
    'import hashlib, json, pathlib, sys, tarfile',
    'target = pathlib.Path(sys.argv[2])',
    'with open(sys.argv[3]) as manifest: pending = json.load(manifest)["files"]',
    'with tarfile.open(sys.argv[1]) as archive:',
    ' for member in archive:',
    '  path = pathlib.PurePosixPath(member.name)',
    '  if not member.isfile() or path.is_absolute() or ".." in path.parts: raise ValueError("invalid archived source member")',
    '  data = archive.extractfile(member).read()',
    '  if pending.pop(member.name, None) != hashlib.sha256(data).hexdigest(): raise ValueError("unexpected or changed source member")',
    '  out = target / path',
    '  out.parent.mkdir(parents=True, exist_ok=True)',
    '  with out.open("xb") as file: file.write(data)',
    'if pending: raise ValueError("incomplete archived source")',
  ].join('\n'),archive,source,archived],{stdio:'pipe'});
  const files=Object.keys(record.files),names=[...files,...record.deleted].sort();
  assert.equal(files.length,record.files_count);
  assert.equal(applicationSourceDigest(source,names),expected);
  return {source,console:consoleDir,proof:{origin:'selected-archive',source_sha256:expected,archive_sha256:record.archive_sha256,
    files:files.length,console_index_sha256:index}};
}

export async function checkProfiles(ctx) {
  const {directory,network,layout,keyring,envB,baseEnv,port,origin,bin,child,stop,request,log}=ctx;
  const linuxDir=join(directory,'linux-bin');
  mkdirSync(linuxDir,{recursive:true});
  const run=(tool,args,env=baseEnv,cwd=root)=>execFileSync(tool,args,{cwd,env,encoding:'utf8',maxBuffer:8<<20,timeout:10*60*1000,stdio:['ignore','pipe','pipe']}).trim();
  const docker=args=>run('docker',args);
  const arch={aarch64:'arm64','x86_64':'amd64',arm64:'arm64',amd64:'amd64'}[docker(['info','--format','{{.Architecture}}'])];
  assert(arch,'supported Linux target required');
  const roles=['api',...httpOwners,'event-relay'];
  const inputs=selectedProfileInputs(directory,bin);
  run('go',['build','-trimpath','-buildvcs=false','-ldflags=-s -w','-o',linuxDir+'/',...roles.map(r=>'./services/api/cmd/'+r)],
    {...baseEnv,GOOS:'linux',GOARCH:arch,CGO_ENABLED:'0',GOWORK:join(inputs.source,'go.work')},inputs.source);
  if(inputs.proof.origin==='verified-checkout')assert.equal(checkoutApplicationDigest(root),inputs.proof.source_sha256);
  const linuxRelease={...inputs.proof,platform:'linux/'+arch,
    binaries:Object.fromEntries(roles.map(role=>[role,digest(readFileSync(join(linuxDir,role)))]))};
  function roleEnv(role,mode,profile){
    const env=roleEnvironment(role,envB,layout),mount=join(profile,'roles',role);
    const container=mode==='compose'||(mode==='mixed'&&role==='event-relay');
    if(env.INFRAPRO_DATABASE_URL&&container){
      const url=new URL(env.INFRAPRO_DATABASE_URL);
      if(mode==='compose'){url.hostname='pg';url.port='5432'}else url.hostname='host.docker.internal';
      env.INFRAPRO_DATABASE_URL=url.toString();
    }
    if(env.INFRAPRO_REDIS_URL&&mode==='compose')env.INFRAPRO_REDIS_URL='redis://redis:6379/0';
    if(env.INFRAPRO_MFA_KEYRING_FILE&&container){copyFileSync(keyring,join(mount,'mfa.json'));env.INFRAPRO_MFA_KEYRING_FILE='/run/infrapro/mfa.json';}
    delete env.INFRAPRO_ARCHITECTURE_FILE;
    delete env.INFRAPRO_CONFIG;
    if(container)delete env.INFRAPRO_HTTP_ADDR;
    return Object.fromEntries(Object.entries(env).filter(([k])=>k.startsWith('INFRAPRO_')));
  }
  async function waitReport(path){
    for(let i=0;i<90;i++){if(existsSync(path)){const report=JSON.parse(readFileSync(path));if(report.phase==='ready')return report;}await sleep(250);}
    throw Error('Process did not report ready for '+path.split('/').slice(-2,-1)[0]);
  }
  function actualGoSettings(role,row,pid,container){
    if(!row.process_resources.go_threads)return null;
    const proc='/proc/'+pid+'/environ';
    const raw=container?docker(['exec',container,'cat','/proc/1/environ']):
      existsSync(proc)?readFileSync(proc,'utf8'):run('ps',['eww','-p',String(pid),'-o','command=']);
    const keys=Object.fromEntries([...raw.replaceAll('\0',' ').matchAll(/(?:^|\s)(GOMAXPROCS|GOMEMLIMIT)=(\S+)/g)].map(m=>[m[1],m[2]]));
    const expected={GOMAXPROCS:String(row.process_resources.go_threads),GOMEMLIMIT:row.process_resources.go_memory_mib+'MiB'};
    assert.deepEqual(keys,expected,'Running Go scheduler/memory environment for '+role);
    return keys;
  }
  for(const mode of ['native','compose','mixed']){
    const profile=join(directory,'live-'+mode);
    prepare({mode,directory:profile,'local-ca':true,'port-base':String(port),network:mode==='compose'?network:undefined,'container-roles':'event-relay',
      'process-budgets':join(root,'deploy/architecture/display-processes.example.json')});
    const physical=resourceLimits(profile,{environment:{INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS:'',...envB}});
    const byRole=new Map(physical.slots.map(row=>[row.role,row]));
    const instancesByRole=JSON.parse(readFileSync(join(profile,'layout.json'))).instances;
    const reportFile=role=>join(profile,'reports',role,'process.json');
    const deployed=mode==='compose'?['api',...httpOwners]:[];
    if(mode==='mixed')deployed.push('event-relay');
    const mounted=[];const nativePids={};
    let cmd;
    const project=network+'-'+mode;
    const dockerEnv={...baseEnv,HTTP_PORT:new URL(origin).port,INFRAPRO_SERVICE_UID:String(process.getuid()),INFRAPRO_SERVICE_GID:String(process.getgid()),INFRAPRO_INSTANCE:project,INFRAPRO_API_IMAGE:'alpine:3.22',INFRAPRO_WEB_IMAGE:'nginx:1.28-alpine'};
    if(mode!=='native'){
      const manifest=JSON.parse(readFileSync(join(profile,'compose.json')));
      for(const role of deployed){
        const vars=roleEnv(role,mode,profile);
        if(role==='event-relay'){
          writeFileSync(join(profile,'roles',role,'events.json'),JSON.stringify({mode:'pg',environment:'acceptance'}),{mode:0o600});
          vars.INFRAPRO_EVENTS_FILE='/run/infrapro/events.json';
        }
        const mount=join(profile,'roles',role);
        writeFileSync(join(mount,'runtime.env'),Object.entries(vars).map(([k,v])=>k+'='+v).join('\n')+'\n',{mode:0o600});
        manifest.services[role].volumes.push({type:'bind',source:linuxDir,target:'/usr/local/bin',read_only:true});
      }
      if(mode==='compose'){
        manifest.services.web.volumes.push({type:'bind',source:inputs.console,target:'/usr/share/nginx/html/console',read_only:true});
      }
      writeFileSync(join(profile,'runtime-compose.json'),JSON.stringify(manifest,null,2)+'\n',{mode:0o600});
      cmd=(...args)=>run('docker',['compose','-p',project,'-f',join(profile,'runtime-compose.json'),...args],dockerEnv);
      cmd('config','--quiet');
    }
    try{
      if(cmd)cmd('up','--detach',...deployed,...(mode==='compose'?['web']:[]));
      if(mode!=='compose'){
        for(const role of [...httpOwners,'api']){
          const env={...baseEnv,...roleEnv(role,mode,profile),INFRAPRO_CONFIG:join(profile,'roles',role,'runtime.yaml'),INFRAPRO_ARCHITECTURE_FILE:join(profile,'roles',role,'architecture.json')};
          mkdirSync(join(profile,'reports',role),{recursive:true,mode:0o700});
          Object.assign(env,{INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'arc-profiles',INFRAPRO_DIAGNOSTICS_NODE_ID:'native',
            INFRAPRO_DIAGNOSTICS_REPLICA_ID:role+'-1',INFRAPRO_DIAGNOSTICS_REPORT_FILE:reportFile(role)});
          nativePids[role]=child(mode+'-'+role,join(bin,role),[],processEnvironment(instancesByRole[role][0],env)).pid;mounted.push(mode+'-'+role);
        }
      }
      let served=false;
      for(let i=0;i<90;i++){
        try{const res=await fetch(origin+'/api/v1/public/account-help',{signal:AbortSignal.timeout(1800)});if(res.ok){served=true;break;}}catch{ /* retry startup or retain independent evidence */ }
        await sleep(220);
      }
      assert(served,mode+' public console proxy');
      await request(origin,'GET','/api/v1/me/workspaces',undefined,200);
      const internal=await fetch(origin+'/internal/v1/upstreams');assert([401,403,404].includes(internal.status),'private probe cannot be called on the public listener');
      const nativeUsage=await Promise.all(Object.entries(nativePids).map(async([role,pid])=>{
        const [rssKiB,cpuPercent]=run('ps',['-p',String(pid),'-o','rss=','-o','%cpu=']).split(/\s+/).filter(Boolean).map(Number);
        return {role,rssKiB,cpuPercent,go_environment:actualGoSettings(role,byRole.get(role),pid),quota:compareQuotaReport(byRole.get(role),await waitReport(reportFile(role)))};
      }));
      const pgCounts=ctx.sql("SELECT jsonb_object_agg(usename,n) FROM (SELECT usename,count(*) n FROM pg_stat_activity WHERE datname=current_database() GROUP BY usename) a",'arc_b');
      const instances=[];
      if(cmd){
        for(const role of deployed){
          const id=cmd('ps','-q',role);
          const item=JSON.parse(docker(['inspect',id]))[0];
          assert.equal(item.State.Running,true);
          assert.deepEqual(Object.keys(item.NetworkSettings.Ports??{}).filter(k=>item.NetworkSettings.Ports[k]?.length),[]);
          const report=await waitReport(reportFile(role));
          const row=byRole.get(role),identity=actualGoSettings(role,row,item.State.Pid,id);
          assert.equal(item.HostConfig.NanoCpus,Math.round(row.process_resources.cpu_cores*1e9));
          assert.equal(item.HostConfig.Memory,row.process_resources.memory_mib*1048576);
          instances.push({role,pid:item.State.Pid,go_environment:identity,quota:compareQuotaReport(byRole.get(role),report),usage:JSON.parse(docker(['stats','--no-stream','--format','{{json .}}',id])),memoryBytes:item.HostConfig.Memory,cpuNanos:item.HostConfig.NanoCpus,reported:report.schema_version});
        }
      }
      if(mode==='compose'){
        const home=await fetch(origin+'/console/');assert.equal(home.status,200);
        const html=Buffer.from(await home.arrayBuffer());assert(html.toString('utf8').includes('id="root"'));
        if(linuxRelease.console_index_sha256)assert.equal(digest(html),linuxRelease.console_index_sha256);
      }
      for(const name of mounted.reverse())await stop(name);
      mounted.length=0;
      if(cmd){
        cmd('stop',...deployed);
        for(const role of deployed){const id=cmd('ps','-a','-q',role);assert.equal(docker(['inspect','--format','{{.State.ExitCode}}',id]),'0',role+' graceful shutdown');}
      }
      log('live-'+mode,{arc:'ARC-032',publicRoute:true,continuedSession:true,internalPortPublished:false,graceful:true,
        ...(mode!=='native'?{linux_release:linuxRelease}:{}),nativeRoles:mode==='compose'?[]:[...httpOwners,'api'],containers:instances,nativeUsage,pgConnections:JSON.parse(pgCounts)});
    } finally {
      if(cmd){
        // Protected diagnostics preserve startup failures without sharing private runtime files.
        try{writeFileSync(join(profile,'services.log'),cmd('logs','--no-color','--tail','32'),{mode:0o600});}catch{ /* retry startup or retain independent evidence */ }
        // Explicitly selected workers start even when their Compose profile is inactive.
        cmd('--profile','*','down','--remove-orphans');
        assert.equal(docker(['ps','-a','-q','--filter','label=com.docker.compose.project='+project]),'',project+' cleanup');
      }
      for(const name of mounted.reverse())await stop(name,false);
    }
  }
}
