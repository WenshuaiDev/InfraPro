import assert from 'node:assert/strict';
import { execFileSync,spawn } from 'node:child_process';
import { once } from 'node:events';
import { existsSync,mkdirSync,readFileSync,writeFileSync } from 'node:fs';
import https from 'node:https';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { prepare } from './architecture-deploy.mjs';
import { provision } from './architecture-postgres.mjs';
import { roleEnvironment,root } from './architecture-runtime.mjs';

export async function checkReplicas({directory,url,pg,envB,bin,child,stop,sql,port,freePort}){
  const layout=join(directory,'replica-layout'),list=join(directory,'replicas.json'),role='identity-api';
  const ids=['identity-north','identity-south'];
  writeFileSync(list,JSON.stringify({[role]:ids,parser:['parse-1','parse-2','parse-3']}),{mode:0o600});
  prepare({mode:'local',directory:layout,replicas:list,'port-base':String(port)});
  provision({url,container:pg,directory:layout,localLogins:true,roles:[role]});
  const manifest=JSON.parse(readFileSync(join(layout,'layout.json'))),instances=manifest.instances[role];
  const envs=instances.map(i=>roleEnvironment(role,{...envB,INFRAPRO_DATABASE_URL:url},layout,i));
  const auth=envs.map(e=>new URL(e.INFRAPRO_DATABASE_URL));
  assert.equal(new Set(auth.map(u=>u.username)).size,2);
  assert.deepEqual(instances.map(i=>i.pgConnections),[4,4]);
  assert.deepEqual(manifest.instances.parser.map(i=>i.maxInflight),[2,1,1]);
  const database=new URL(url).pathname.slice(1),held=[],started=[],reserves=[];
  const checkpoint=(phase)=>writeFileSync(join(layout,'quota-observations.json'),JSON.stringify({phase,reserves},null,2)+'\n',{mode:0o600});
  const pgCommand=u=>['exec','-i','-e','PGPASSWORD',pg,'psql','-XqAt','-v','ON_ERROR_STOP=1','-h','127.0.0.1','-U',decodeURIComponent(u.username),'-d',database];
  const shellenv=u=>({...process.env,PGPASSWORD:decodeURIComponent(u.password)});
  const certificate=join(layout,'roles/api/tls'),tls={ca:readFileSync(join(certificate,'ca.pem')),cert:readFileSync(join(certificate,'api.pem')),key:readFileSync(join(certificate,'api.key')),servername:role+'.infrapro.internal',agent:false};
  const read=addr=>new Promise((ok,no)=>{
    const req=https.request('https://'+addr+'/internal/v1/ready',{...tls,method:'POST',headers:{'Content-Type':'application/json'},timeout:1400},res=>{
      let body='';res.setEncoding('utf8');res.on('data',v=>body+=v);
      res.once('end',()=>ok(res.statusCode===200&&body.trim()==='true'));
    });
    req.on('timeout',()=>req.destroy());req.on('error',no);req.end('{}');
  });
  const awaitReady=async addr=>{for(let i=0;i<90;i++){try{if(await read(addr))return;}catch{/* startup */}await sleep(90);}throw Error('Replica failed its authenticated private readiness probe');};
  const reportFile=name=>join(layout,'process-reports',name,'process.json');
  const startRole=(name,spec)=>{
    mkdirSync(join(layout,'process-reports',name),{recursive:true,mode:0o700});
    started.push(name);return child(name,join(bin,role),[],{...spec,INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'arc-slots',
      INFRAPRO_DIAGNOSTICS_NODE_ID:'local',INFRAPRO_DIAGNOSTICS_REPLICA_ID:spec.INFRAPRO_REPLICA_ID,INFRAPRO_DIAGNOSTICS_REPORT_FILE:reportFile(name)});
  };
  try{
    for(const [index,u] of auth.entries()){
      assert.equal(sql("SELECT rolconnlimit FROM pg_roles WHERE rolname='"+u.username+"'",database),'4');
      for(let n=0;n<4;n++){
        const proc=spawn('docker',pgCommand(u),{env:shellenv(u),stdio:'pipe'});
        const exit=once(proc,'close');held.push({proc,exit});
        const ready=once(proc.stdout,'data');proc.stdin.write("SELECT 'ready';\n");
        const output=await Promise.race([ready,exit.then(()=>[]) ,sleep(5000,null,{ref:false})]);
        assert.equal(output?.[0]?.toString().trim(),'ready','physical role reservation');
      }
      assert.throws(()=>execFileSync('docker',pgCommand(u),{input:'SELECT 1;',env:shellenv(u),stdio:'pipe'}),e=>e.stderr?.toString().includes('too many connections for role'));
      reserves.push({id:ids[index],actual:4,additionalRejected:true});
    }
    checkpoint('connections-exhausted');
    for(const {proc,exit} of held.splice(0)){proc.stdin.end('\\q\n');await exit;}
    for(const [index,i] of instances.entries()){
      startRole(i.id,envs[index]);await awaitReady(i.listen_address);
      let report;
      for(let tick=0;tick<60;tick++){
        if(existsSync(reportFile(i.id)))report=JSON.parse(readFileSync(reportFile(i.id)));
        if(report?.phase==='ready')break;
        await sleep(100);
      }
      assert.equal(report?.phase,'ready');
      const limits=Object.fromEntries((report.metrics??[]).filter(m=>m.key.startsWith('quota.')).map(m=>[m.key,m.value]));
      assert.equal(limits['quota.pg_connections'],i.pgConnections);assert.equal(limits['quota.http_inflight'],i.maxInflight);
      reserves[index].reportedLimits=limits;
    }
    checkpoint('both-active');
    const extra='127.0.0.1:'+await freePort(),duplicate=startRole('identity-duplicate',{...envs[0],INFRAPRO_PRIVATE_LISTEN_ADDR:extra});
    const rejected=await Promise.race([once(duplicate,'exit'),sleep(10000,null,{ref:false})]);assert.ok(rejected&&rejected[0]!==0,'a copied active slot was rejected');
    await stop('identity-duplicate',false);
    const proxy=child('replica-proxy','python3',[join(root,'deploy/architecture/replica-proxy.py'),join(layout,'replica-proxy.json')],{PATH:process.env.PATH});
    started.push('replica-proxy');assert.ok(proxy.pid);
    const ingress=new URL(JSON.parse(readFileSync(join(layout,instances[0].directory,'architecture.json'))).endpoints[role]).host;
    await awaitReady(ingress);
    await stop(ids[0]);await awaitReady(ingress);
    startRole('identity-replacement',{...envs[0],INFRAPRO_PRIVATE_LISTEN_ADDR:extra});await awaitReady(extra);
    const replacement=join(directory,'single-slot');
    prepare({mode:'native',directory:replacement});
    const issue={url,container:pg,directory:replacement,issueLogins:true,roles:[role],retireSlots:true};
    assert.throws(()=>provision(issue),/Superseded slot still available/);
    await stop('identity-replacement');await stop(ids[1]);provision(issue);
    const old=auth.map(u=>"'"+u.username+"'").join(',');
    assert.equal(sql('SELECT count(*) FROM pg_roles WHERE rolname IN ('+old+') AND (rolcanlogin OR rolconnlimit<>0)',database),'0');
    checkpoint('retired');
    return {status:'passed',role,instances:reserves,totalActualPG:8,duplicateRejected:true,survivingReplicaRouted:true,vacatedSlotReused:true,
      activeRetirementDenied:true,obsoleteLoginsDisabled:true,configuredHTTP:instances.map(i=>i.maxInflight),parserConcurrentSlots:manifest.instances.parser.map(i=>i.maxInflight)};
  }finally{
    for(const {proc,exit} of held){proc.stdin.end();await exit;}
    for(const name of started.reverse())await stop(name,false);
  }
}
