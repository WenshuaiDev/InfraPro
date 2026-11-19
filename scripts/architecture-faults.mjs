import assert from 'node:assert/strict';
import { randomUUID, createHash } from 'node:crypto';
import { basename,dirname,join,resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { client, launch, read, root, runDirectory, write, sleep } from './architecture-lab.mjs';
import { dependencyGate,diagnostics } from './architecture-serve.mjs';
import { withHTTPPair,startPairedWorkers } from './architecture-http-sandbox.mjs';
import { availablePort, stop, waitURL } from './fixtures/parsing-lab.mjs';

const cause=(r)=>({status:r.status(),code:r?.code});
// Each failure affects this stack's listeners or private TCP path only.
export async function isolatedFaults(stack) {
  assert.equal(stack.side,'b');assert(stack.chGate,'An owned B stack is required for controlled dependency faults');
  const {lab,cfg}=stack,dir=lab.directory,s=read(join(dir,'samples.json')).structure;
  const actors=read(join(dir,'private/accounts.json')),checks=[];
  const rpc=await client(cfg,actors.admin);
  const test=async(name,action)=>{
    const startedAt=new Date().toISOString();
    try{checks.push({name,startedAt,...await action(),passed:true});}
    catch(e){checks.push({name,startedAt,passed:false,error:e.message.slice(0,240)});}
    write(join(dir,'faults','isolation.json'),{arc:'ARC-027',name:'independent role failures',cases:checks,passed:checks.every(v=>v.passed)});
  };
  const unavailable='/api/v1/tenants/'+s.tenant+'/context';
  const checkLogin=async()=>{const c=await client(cfg,actors.admin);try{
    const x=await c.call('/tenants/'+s.tenant+'/context');assert(x.membership_id);
    const facilities=await c.call('/tenants/'+s.tenant+'/engineering/resources/facilities');assert.ok(facilities.items.length>0);
    return {membership:true,facilities:facilities.items.length};
  }finally{await c.client.dispose();}};
  const checkout=async(role,work)=>{
    await stack.stopRole(role);try{return await work();}finally{stack.startRole(role);}
  };
  try{
    await test('ClickHouse disconnected with a cold identity process',async()=>{
      stack.chGate.set(false);
      try{
        await stack.stopRole('identity-api');stack.startRole('identity-api');
        await waitURL(cfg.api+'/api/v1/public/account-help');
        return{disconnected:'ClickHouse',coldStarted:'identity-api',login:true,tenantLedger:await checkLogin()};
      }finally{stack.chGate.set(true);}
    });
    await test('identity owner unavailable',()=>checkout('identity-api',async()=>{
      const r=await rpc.client.get(cfg.api+unavailable);const v=await r.json();assert.equal(r.status(),503);assert(v.code);
      const site='http://127.0.0.1:'+cfg.ports.diagnostics,diagnostic=await request.newContext({baseURL:site});
      try{
        const account=read(join(dirname(cfg.config),'diagnostics/accounts.json'));
        const csrf=await(await diagnostic.get('/api/diagnostics/v1/session/bootstrap')).json();
        const login=await diagnostic.post('/api/diagnostics/v1/session/login',{headers:{Origin:site,'X-CSRF-Token':csrf.data.csrf_token},data:{username:account.username,password:account.password}});
        assert.equal(login.status(),200);
        const dashboard=await diagnostic.get('/api/diagnostics/v1/overview');assert.equal(dashboard.status(),200);assert.ok((await dashboard.json()).data);
        return{protected:{...cause(r),code:v.code},diagnosticLogin:200,diagnosticOverview:200};
      }finally{await diagnostic.dispose();}
    }));
    await waitURL(cfg.api+'/api/v1/public/account-help');
    await test('diagnostics unavailable',()=>checkout('diagnostics',async()=>{await checkLogin();return{login:true,tenantLedger:true};}));
    await test('read worker stopped while a publication commits',()=>checkout('monitoring-read-worker',async()=>{
      const prefix='/tenants/'+s.tenant+'/monitoring/imports',key=randomUUID(),source='observed_at,value,quality\n2030-01-01T00:00:00.000000001Z,2,good\n';
      const bytes=Buffer.from(source),hash=createHash('sha256').update(bytes).digest('hex');
      const meta=revision=>({request_id:randomUUID(),expected_revision:String(revision),reason:'架构隔离读取故障'});
      let v=await rpc.call(prefix,{...meta(0),name:'ARC-'+key+'.csv',size_bytes:String(bytes.length)});
      const item=prefix+'/'+v.id;
      const res=await rpc.client.put(rpc.api+item+'/upload/parts/1',{headers:{...rpc.headers,'Content-Type':'application/octet-stream','X-Expected-Revision':v.revision,'X-Content-SHA256':hash},data:bytes});assert(res.ok());
      v=await rpc.call(item+'/upload/complete',{...meta(v.revision),sha256:hash});
      v=await rpc.call(item+'/mappings',{...meta(v.revision),mapping:{format:'csv',mode:'single',encoding:'utf-8',delimiter:',',header_row:1,time_column:'observed_at',value_column:'value',quality_column:'quality',timezone:'UTC',empty_wide:'error',channels:[{alias:'C',channel_id:s.channels.P1.id,unit:'mm'}]}});
      const wait=async state=>{const end=Date.now()+120000;do{v=await rpc.call(item);if(v.state===state)return;await sleep(250);}while(Date.now()<end);throw Error('Publication failed to reach '+state+' (last '+v.state+')');};
      await rpc.call(item+'/validate',meta(v.revision));await wait('ready');await rpc.call(item+'/publish',meta(v.revision));await wait('published');
      assert.equal(v.counts.proposed,'1');await checkLogin();return{batch:v.id,state:v.state,inputSHA256:hash};
    }));
  }finally{await rpc.client.dispose();}
  const result={arc:'ARC-027',name:'independent service failures',passed:checks.every(v=>v.passed),checks};
  write(join(dir,'faults','http-isolation.json'),[result]);return result;
}

// Each fault owns its process, TCP gate and cloned business database. A run
// may be repeated while the original A/B pair remains available to operators.
export async function faultRehearsal(dir){
  let gate,stack;
  const configure=async({side,cfg,privateDir,dir:sandbox,baseline})=>{
    if(side!=='b')return;
    gate=await dependencyGate(cfg.monitoring_clickhouse_replicas);
    return {beforeRoles(runtime){runtime.monitoring_clickhouse_replicas=gate.address;},start:async({env,bin,endpoints,topology,children,roleEnv})=>{
      const port=await availablePort();
      const lab={id:basename(sandbox),directory:sandbox,assets:baseline.assets,b:{ports:{diagnostics:port}}};
      const diag=diagnostics(lab,'b',bin,root);
      await startPairedWorkers({side,privateDir,bin,env,endpoints,topology,children,roleEnv,legacy:[],split:['monitoring-publish-worker','monitoring-read-worker']});
      const processes=new Map(children.filter(p=>p.spawnfile.startsWith(bin+'/')).map(p=>[basename(p.spawnfile),p]));
      const startRole=role=>{
        const current=processes.get(role);
        if(current?.exitCode===null&&current.signalCode===null)return;
        const settings=role==='diagnostics'?env:{...env,...roleEnv(role),INFRAPRO_ARCHITECTURE_FILE:join(privateDir,role+'.json')};
        if(role.endsWith('-worker'))settings.INFRAPRO_EVENTS_FILE=join(privateDir,'worker-events.json');
        const child=role==='diagnostics'?launch(join(bin,'infrapro-diagnostics'),['server','--config',diag],env,join(privateDir,'diagnostics.log')):launch(join(bin,role),[],settings,join(privateDir,role+'.log'));
        children.push(child);processes.set(role,child);
      };
      const stopRole=async role=>{
        const p=processes.get(role);
        assert.ok(p && p.exitCode===null && p.signalCode===null,'fault requires active process '+role);
        await stop(p);
        const n=children.indexOf(p);assert(n>=0);children.splice(n,1);
      };
      startRole('diagnostics');await waitURL('http://127.0.0.1:'+port+'/readyz');
      stack={side,lab,chGate:gate,startRole,stopRole,port};
    }};
  };
  try{
    return await withHTTPPair(dir,'fault-isolation',async sandbox=>{
      const paired=read(join(sandbox,'private/lab.json'));
      stack.cfg={...paired.b,ports:{diagnostics:stack.port}};
      const result=await isolatedFaults(stack);
      result.source=sandbox;result.fixture=read(join(sandbox,'snapshot.json'));
      write(join(dir,'faults','http-isolation.json'),{cases:[result]});
      return result;
    },{configure,workerRoles:['monitoring-publish-worker','monitoring-read-worker']});
  }finally{if(gate)await gate.close();}
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const r=await faultRehearsal(runDirectory(process.argv[2]));
  console.log('Isolated dependency failures',r.passed);process.exitCode=r.passed?0:1;
}
