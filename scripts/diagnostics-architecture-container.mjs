// Start the production files-checker binary as PID 1. The independent Docker
// observer uses exactly the released read-only adapters and split bindings.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

export async function checkContainer({ scratch, cli, instance, uid, env }) {
  const run=(name,args,options={})=>execFileSync(name,args,{env,stdio:['ignore','pipe','pipe'],encoding:'utf8',timeout:180000,...options});
  const docker=(...args)=>run('docker',args).trim();
  const socket=new URL(docker('context','inspect','--format','{{.Endpoints.docker.Host}}'));
  if(socket.protocol!=='unix:')throw Error('Local Docker Unix socket required');
  const image=docker('image','inspect','--format','{{.Id}}','alpine:3.22');
  const dir=join(scratch,'container-diag'), base=join(scratch,'container-reports');
  const work=join(base,'files-checker'),report=join(work,'process.json'),checker=join(scratch,'files-checker-linux');
  const observer=join(scratch,'labtool');
  mkdirSync(work,{recursive:true,mode:0o700});
  run('go',['build','-trimpath','-o',checker,'./services/api/cmd/files-checker'],{env:{...env,CGO_ENABLED:'0',GOOS:'linux',GOARCH:docker('version','--format','{{.Server.Arch}}')}});
  run('go',['build','-trimpath','-o',observer,'./services/diagnostics/internal/labtool']);
  run(cli,['diagnostics','config','init','--directory',dir,'--instance-id',instance,'--profile','docker','--topology','split','--runtime-uid',String(uid),'--report-directory',base]);
  const file=join(dir,'observer-bindings.json'), manifest=join(dir,'manifest.json');
  const bindings=JSON.parse(readFileSync(file,'utf8'));
  for(const b of bindings.bindings)if(b.adapter==='docker_container')b.socket_path=socket.pathname;
  const save=()=>writeFileSync(file,JSON.stringify(bindings),{mode:0o600});
  save();
  const observe=(id)=>JSON.parse(run(observer,['probe',manifest,file,id]));
  const probe=()=>observe('files-checker-report');
  const reporter=bindings.bindings.find(b=>b.id==='files-checker-report');
  const name=instance+'-checker',role='/usr/local/bin/files-checker';
  let id;
  try {
    id=docker('run','-d','--name',name,'--label','io.infrapro.instance='+instance,'--label','io.infrapro.replica=files-checker-1',
      '--network','none','--user',uid+':'+process.getgid(),'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','32','--memory','96m',
      '--tmpfs','/tmp:rw,nosuid,noexec,mode=1777','--mount','type=bind,src='+work+',dst=/run/infrapro-report','--mount','type=bind,src='+checker+',dst='+role+',readonly',
      '-e','INFRAPRO_DIAGNOSTICS_INSTANCE_ID='+instance,'-e','INFRAPRO_DIAGNOSTICS_NODE_ID=docker-observer','-e','INFRAPRO_DIAGNOSTICS_REPLICA_ID=files-checker-1',
      '-e','INFRAPRO_DIAGNOSTICS_REPORT_FILE=/run/infrapro-report/process.json','-e','INFRAPRO_DIAGNOSTICS_PID_NAMESPACE=container',
      '--entrypoint',role,image,'--directory','/tmp/files','--socket','/tmp/files/test.sock');
    assert.match(id,/^[a-f0-9]{64}$/);
    let ready;
    for(let i=0;i<45;i++) {
      if(existsSync(report) && JSON.parse(readFileSync(report)).phase==='ready' && (ready=probe()).outcome==='ok')break;
      await delay(300);
    }
    assert.equal(ready?.outcome,'ok','Live container observation: '+(ready?.reason_code??'report_missing'));
    assert.equal(ready.process.pid,1);assert.equal(ready.process.uid,uid);assert.equal(ready.process.pid_namespace,'container');
    const presence=observe('files-checker-presence'); assert.equal(presence.facets.presence,'running');
    reporter.expected_executable='/usr/local/bin/unmatched';save();
    const mismatch=probe();assert.equal(mismatch.reason_code,'vantage_mismatch');
    reporter.expected_executable=role;save();
    docker('stop','--timeout','4',id);
    const stopped=probe();assert.notEqual(stopped.outcome,'ok');
    assert.equal(observe('files-checker-presence').facets.presence,'stopped');
    return {profile:'docker',role:'files-checker',uid,pid:1,namespace:'container',phase:'ready',live:ready.outcome,
      wrong_executable:mismatch.reason_code,stopped:stopped.reason_code,stopped_presence:'stopped'};
  } finally {
    if(id&&/^[a-f0-9]{64}$/.test(id)&&docker('inspect','--format','{{index .Config.Labels "io.infrapro.instance"}}',id)===instance)docker('rm','-f',id);
    if(existsSync(report))chmodSync(report,0o600);
  }
}
