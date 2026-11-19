import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { join } from 'node:path';
import { digest, sleep, write } from './architecture-lab.mjs';

const argumentsPG = ['shared_preload_libraries=pg_stat_statements','pg_stat_statements.track=all','shared_buffers=256MB','max_connections=100','track_io_timing=on','track_wal_io_timing=on','log_lock_waits=on','deadlock_timeout=250ms'];
const docker=(args,options={})=>execFileSync('docker',args,{encoding:'utf8',stdio:['ignore','pipe','pipe'],...options}).trim();

// The source used to build the frozen fixture may also serve independently
// retained walkthrough databases. No walkthrough or original regression uses
// the PostgreSQL instance that runs the measured trials.
export async function displayPostgres(dir,source,fixture){
  const image=docker(['inspect','--format','{{.Image}}',source.container]);
  assert.match(image,/^sha256:[0-9a-f]{64}$/);
  const limits={cpu:4,memoryMiB:1536,sharedMemoryMiB:256};
  const id=digest(JSON.stringify([source.id,image,limits,argumentsPG])).slice(0,12);
  const name='arc-display-postgres-'+id,label=id+'-display';
  const existing=spawnSync('docker',['inspect','--format','{{.State.Status}}',name],{encoding:'utf8'});
  if(existing.status!==0){
    const endpoint=new URL(fixture.database);
    assert.equal(endpoint.username,'archtest');
    const run=spawnSync('docker',['run','-d','--name',name,'--label','infrapro.architecture='+label,
      '--cpus',String(limits.cpu),'--memory',limits.memoryMiB+'m','--shm-size',limits.sharedMemoryMiB+'m',
      '-p','127.0.0.1::5432','-e','POSTGRES_USER','-e','POSTGRES_PASSWORD','-e','POSTGRES_DB',
      image,'postgres',...argumentsPG.flatMap(setting=>['-c',setting])],
    {encoding:'utf8',env:{...process.env,POSTGRES_USER:'archtest',POSTGRES_PASSWORD:decodeURIComponent(endpoint.password),POSTGRES_DB:'postgres'}});
    if(run.status!==0&&!run.stderr.includes('already in use'))throw Error('Could not create isolated monitoring-load PostgreSQL: '+run.stderr.slice(0,250));
  }
  const state=JSON.parse(docker(['inspect','--format','{{json .}}',name]));
  assert.equal(state.Image,image);assert.equal(state.Config.Labels['infrapro.architecture'],label);
  assert.equal(state.HostConfig.Memory,limits.memoryMiB*2**20);assert.equal(state.HostConfig.NanoCpus,limits.cpu*1e9);
  assert.equal(state.HostConfig.ShmSize,limits.sharedMemoryMiB*2**20);
  if(!state.State.Running)docker(['start',name]);
  let ready=false;
  for(let i=0;i<120&&!ready;i++){
    ready=spawnSync('docker',['exec',name,'pg_isready','-h','127.0.0.1','-U','archtest','-d','postgres'],{stdio:'ignore'}).status===0;
    if(!ready)await sleep(350);
  }
  assert(ready,'Isolated monitoring-load PostgreSQL did not become ready');
  const settings=JSON.parse(docker(['exec',name,'psql','-X','-qAt','-U','archtest','-d','postgres','-c',
    "SELECT jsonb_object_agg(name,setting) FROM pg_settings WHERE name IN ('shared_buffers','max_connections','track_io_timing','track_wal_io_timing','log_lock_waits','deadlock_timeout','server_version','shared_preload_libraries','pg_stat_statements.track')"]));
  const database={container:name,containerLabel:label,image,limits,settings};
  write(join(dir,'load/display/postgres.json'),database);
  return {...source,...database};
}
