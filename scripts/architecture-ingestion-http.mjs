// Compare public commands and evidence on separate frozen and split backends.
// Each side has its own durable relay, parser service, source, objects and worker.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createServer,request as proxyRequest } from 'node:http';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { digest, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { startPairedWorkers, withHTTPPair } from './architecture-http-sandbox.mjs';
import { operation } from './architecture-workflow-http.mjs';
import { checkIngestionBodies, compareParserAttestation } from './architecture-ingestion-bodies.mjs';
import { availablePort, startParser } from './fixtures/parsing-lab.mjs';
import { startRelay } from './fixtures/ingestion-relay.mjs';
import { simulate } from './ingestion-simulate.mjs';

const measurement = readFileSync(join(root, 'samples/parsing/measurement.bin'));
const inputDigest = digest(measurement);
const method = 'example.hongdian.ddp.tcp.monitoring-json';
const version = '1.0.0';
const reason = '架构对照合成原文及固定方法试解析';
const commandKeys = ['status', 'state', 'changed', 'replayed', 'revision', 'generation', 'error_code', 'added', 'can_submit'];
const stableHeaders = ['content-type','content-disposition','cache-control','accept-ranges','content-range','allow','vary'];
const pick = (record, keys) => Object.fromEntries(keys.filter(key => Object.hasOwn(record, key)).map(key => [key, record[key]]));
const byName = items => [...items].sort((a,b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
async function parserGate(target){
  let hold,unlock;
  const server=createServer(async(req,res)=>{
    if(req.url==='/v1/identify'&&hold)await hold;
    if(req.destroyed)return;
    const upstream=proxyRequest(new URL(req.url,target),{method:req.method,headers:req.headers},response=>{
      res.writeHead(response.statusCode,response.headers);response.pipe(res);
    });
    upstream.on('error',()=>{if(!res.headersSent)res.writeHead(503);res.end();});
    req.pipe(upstream);
  });
  await new Promise((ok,no)=>server.once('error',no).listen(0,'127.0.0.1',ok));
  return {url:'http://127.0.0.1:'+server.address().port,hold(){assert.ok(!hold);hold=new Promise(r=>{unlock=r;});},
    release(){unlock?.();hold=unlock=undefined;},async close(){this.release();server.closeAllConnections();await new Promise(ok=>server.close(ok));}};
}
function shape(value) {
  if (Buffer.isBuffer(value)) return 'bytes';
  if (Array.isArray(value)) return [...new Set(value.map(row => JSON.stringify(shape(row))))].sort().map(s=>JSON.parse(s));
  if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map(key=>[key,shape(value[key])]));
  return value === null ? 'null' : typeof value;
}
function semantics(value) {
  if (Buffer.isBuffer(value)) return {sha256:digest(value), bytes:value.length};
  if (Array.isArray(value)) return {count:value.length};
  if (value?.items) return {count:value.items.length};
  return pick(value||{},commandKeys);
}
function checkResponse(response) {
  assert.equal(response.input_sha256,inputDigest);
  assert.equal(response.method.id,method);
  assert.equal(response.method.version,version);
  assert.match(response.execution_id,/^[0-9a-f-]{36}$/);
  assert.equal(response.records.length,1);
  // The reader also verifies the digest of the original serialized response.
  // Only its independently minted execution identity needs correspondence.
  const {execution_id: issued,...data}=response;
  assert.ok(issued);
  return data;
}

class Session {
  constructor(origin,shared,ledger,bodies) {
    this.origin=origin;this.shared=shared;this.ledger=ledger;this.bodies=bodies;this.occurrences=new Map();
  }
  async open(account) {
    this.client=await request.newContext({timeout:35000,maxRedirects:0});
    this.actor=account.username;
    const csrf=await this.call('/auth/csrf',undefined,{project:csrf=>({present:csrf.token.length>=32})});
    this.headers={Origin:this.origin,'X-CSRF-Token':csrf.token};
    const v=await this.call('/auth/login',account);assert.equal(v.status,'authenticated');
  }
  async call(path,data,options={}) {
    const method=options.method||(data===undefined?'GET':'POST'), url=new URL('/api/v1'+path,this.origin);
    const op=operation(method,url.pathname);assert.ok(op,method+' '+url.pathname);
    const stage=this.actor+' '+op.operationId+' '+(this.occurrences.get(op.operationId)??0);
    this.occurrences.set(op.operationId,(this.occurrences.get(op.operationId)??0)+1);
    if(!this.shared.has(stage))this.shared.set(stage,randomUUID());
    const command=options.replayCommand??this.shared.get(stage), start=Date.now(), observations=[];
    const input=data!==null&&typeof data==='object'&&!Buffer.isBuffer(data)&&data.request_id===true?{...data,request_id:command}:data;
    let status,bytes,value,headers;
    for(;;){
      let reply;
      try{
        reply=await this.client.fetch(url.href,{method,maxRetries:2,headers:{...this.headers,'X-Command-ID':command,...options.headers},data:input});
      }catch(error){
        throw Error(op.operationId+' '+error.message.split('\n')[0],{cause:error});
      }
      status=reply.status();bytes=await reply.body();headers=reply.headers();
      value=headers['content-type']?.includes('json')&&bytes.length?JSON.parse(bytes):bytes;
      await reply.dispose();
      if(status<200||status>=300||!options.wait)break;
      const state=options.wait(value);observations.push(state);
      if(state===true)break;
      if(Date.now()-start>=75000)throw Error(op.operationId+' did not settle: '+JSON.stringify(observations.slice(-2)));
      await sleep(175);
    }
    const mime=headers['content-type']?.split(';')[0];
    const row={...pick(op,['method','template','owner','operationId']),command,status,sha256:digest(bytes),mime,headers:pick(headers,stableHeaders),
      schema:shape(value),semantics:semantics(value),elapsedMs:Date.now()-start,...options.wait&&{samples:observations.length}};
    this.ledger.push(row);
    // Retain complete synthetic DTOs in the protected sandbox for auditing the
    // comparisons. Never persist session cookies or a usable anti-CSRF token.
    this.bodies.push({operationId:op.operationId,command,status,value:Buffer.isBuffer(value)?{base64:value.toString('base64')}:
      op.operationId==='getCSRF'?{tokenLength:value.token.length}:value});
    if(options.denied){
      assert.ok(options.denied.includes(status),op.operationId+' expected denial, got '+status);row.semantics={code:value.code};return value;
    }
    if(status<200||status>=300){row.error=pick(value,['code','message','issues']);throw Error(op.operationId+' '+status+' '+JSON.stringify(row.error));}
    if(options.schema)row.schema=options.schema(value);
    if(options.project)row.semantics=options.project(value,headers);
    return value;
  }
}
const cmd=(extra={})=>({request_id:true,reason,...extra});
function formalCount(container,database){
  assert.match(database,/^arc_http_[a-f0-9]+_[ab]$/);
  assert.match(container,/^arc-http-storage-[a-f0-9]+-ch1-1$/);
  return Number(execFileSync('docker',['exec',container,'clickhouse-client','--database',database,'--query','SELECT count() FROM observations'],{encoding:'utf8'}).trim());
}
async function acceptWithoutParser(lab,side,stack,source,deliver){
  assert.match(source,/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/);
  const database='arc_http_'+lab.id+'_'+side;assert.match(database,/^arc_http_[0-9a-f]+_[ab]$/);
  assert.equal(new URL(read(lab[side].config).database_url).pathname,'/'+database);
  const receipts=()=>JSON.parse(execFileSync('docker',['exec',lab.container,'psql','-XqAt','-U','archtest','-d',database,'-c',
    `SELECT coalesce(jsonb_agg(jsonb_build_object('key',o.object_key,'sha256',o.sha256,'size',o.byte_length) ORDER BY r.source_sequence),'[]'::jsonb)
     FROM ingestion_receipts r JOIN ingestion_objects o ON r.object_id=o.id WHERE r.source_id='${source}'`],{encoding:'utf8'}));
  await stack.parser.pause();
  try{
    assert.equal(stack.parser.pid,undefined);
    const delivered=await deliver();assert.equal(delivered.receipts,3);
    const end=Date.now()+60000;let accepted;
    do{accepted=receipts();if(accepted.length===delivered.receipts)break;await sleep(150);}while(Date.now()<end);
    assert.equal(accepted.length,delivered.receipts);
    const env={...Object.fromEntries(Object.entries(process.env).filter(([k])=>!k.startsWith('INFRAPRO_'))),INFRAPRO_CONFIG:lab[side].config};
    const inventory=JSON.parse(execFileSync(resolve(lab.directory,'../../..','fixture'),['list',lab.directory],{env,encoding:'utf8'}));
    const byKey=new Map(inventory.map(o=>[o.key,o]));
    for(const fact of accepted)assert.deepEqual(byKey.get(fact.key),fact);
    return {...delivered,parserOffline:{receipts:accepted.length,objects:accepted.map(({sha256,size})=>({sha256,size}))}};
  }finally{await stack.parser.resume();}
}
async function manual(session,base,device){
  const input={received_at:'2026-11-16T00:00:00Z',representation:'binary',source:'synthetic'};
  const sample=await session.call(base+'/samples',cmd(input));
  const replay=await session.call(base+'/samples',cmd(input),{replayCommand:session.ledger.at(-1).command});
  assert.equal(replay.id,sample.id);assert.equal(replay.replayed,true);
  const item=base+'/samples/'+sample.id;
  await session.call(item+'/content',measurement,{method:'PUT',headers:{'Content-Type':'application/octet-stream'}});
  await session.call(item+'/confirm',cmd());
  const verified=await session.call(item,undefined,{project:r=>pick(r,['source','state','sha256','byte_length','representation','received_at'])});
  assert.equal(verified.sha256,inputDigest);assert.equal(verified.byte_length,measurement.length);
  assert.ok((await session.call(base+'/samples')).items.some(r=>r.id===sample.id));
  assert.deepEqual(await session.call(item+'/content'),measurement);
  const job=await session.call(base+'/runs',cmd({kind:device?'device_test':'method_test',sample_id:sample.id,transport:'tcp',...device?{device_id:device}:{method_id:method,method_version:version}}));
  const run=base+'/runs/'+job.id;
  const detail=await session.call(run,undefined,{wait:v=>v.run.state==='succeeded',project:v=>({state:v.run.state,kind:v.run.kind,version:v.run.method_version,generation:v.run.current_generation,can_read_sample:v.can_read_sample})});
  assert.equal(detail.run.sample_id,sample.id);assert.equal(detail.run.method_version,version);
  const execution=run+'/executions/'+detail.run.current_generation;
  await session.call(execution,undefined,{project:v=>pick(v,['state','generation','error_code','response_status'])});
  const result=await session.call(execution+'/result',undefined,{project:v=>({state:v.state,quality:v.business_quality,adopted:v.formally_adopted,response:checkResponse(v.response)})});
  assert.equal(result.formally_adopted,false);assert.equal(result.business_quality,'not_evaluated');
  const raw=await session.call(execution+'/response',undefined,{project:b=>checkResponse(JSON.parse(b))});
  assert.equal(digest(raw),result.result_digest);assert.deepEqual(JSON.parse(raw),result.response);
  assert.ok((await session.call(base+'/runs')).items.some(r=>r.id===job.id));
  const left={kind:'method_test',sample_id:sample.id,transport:'tcp',method_id:method,method_version:version};
  const comparison=await session.call(base+'/runs/compare',cmd({left,right:{...left,method_version:'1.1.0'}}));
  assert.notEqual(comparison.left.id,comparison.right.id);
  for(const [job,expectedVersion] of [[comparison.left,version],[comparison.right,'1.1.0']]){
    const completed=await session.call(base+'/runs/'+job.id,undefined,{
      wait:v=>v.run.state==='succeeded',project:v=>pick(v.run,['kind','state','method_version','current_generation','error_code'])});
    assert.equal(completed.run.sample_id,sample.id);assert.equal(completed.run.method_version,expectedVersion);
  }
}

const configInput=(revision,name)=>cmd({expected_revision:revision,name,method_id:method,method_version:version,
  output_bindings:[{output_key:'displacement',slot_key:'displacement',confirmed:true}],ignored_outputs:[]});

export async function seedWalkthroughIngestion({origin,admin,tenant,project,unit,metric,relay,port=relay.ddpPort,privateDir}){
  mkdirSync(privateDir,{recursive:true,mode:0o700});
  const marker=join(privateDir,'walkthrough-ingestion-steps.json');
  const identity={origin,tenant,project,unit,metric,relay:relay.id};
  const state=existsSync(marker)?read(marker):{identity,steps:{}};
  assert.deepEqual(state.identity,identity,'walkthrough source belongs to another environment');
  async function once(key,create){
    if(!Object.hasOwn(state.steps,key)){state.steps[key]=await create();write(marker,state);}
    return state.steps[key];
  }
  const session=new Session(origin,new Map(),[],[]),call=(...params)=>session.call(...params);
  const parse='/platform/parsing',ingest='/platform/ingestion',tenantRoot='/tenants/'+tenant;
  const area=tenantRoot+'/ingestion',devices=tenantRoot+'/devices',products='/platform/device-definitions/products';
  try{
    await session.open(admin);
    await once('catalogue',async()=>{
      await call(parse+'/catalog-syncs',cmd());await call(parse+'/ingestion-catalog-syncs',cmd());
      for(const kind of ['methods','identification-methods','ingress-protocols','ingress-compatibilities']){
        for(const item of (await call(parse+'/'+kind+'?limit=100')).items){
          if(item.state==='enabled')continue;
          const part=kind==='methods'?item.entry.method_id+'/versions/'+item.entry.method_version:item.id;
          await call(parse+'/'+kind+'/'+part+'/state',cmd({state:'enabled',expected_revision:item.revision}));
        }
      }
      return true;
    });
    const maker=await once('maker',()=>call('/platform/device-definitions/manufacturers',cmd({code:'walk_ddp',name:'验收用 DDP 厂家'})));
    const product=await once('product',()=>call(products,cmd({manufacturer_id:maker.id,code:'walk_ddp',name:'验收用 DDP 终端',model:'DDP',
      output_slots:[{key:'displacement',name:'位移',mode:'fixed',metric_version_id:metric,unit_version_id:unit,timestamp_meaning:'observation_time'}]})));
    const released=await once('published',()=>call(products+'/'+product.id+'/publish',cmd({expected_draft_revision:product.revision,classification:{mode:'unclassified'}})));
    const v=products+'/'+product.id+'/versions/'+released.version_id;
    const cfg=await once('configuration',()=>call(v+'/parsing-configs',configInput('0','验收固定位移解析')));
    const def=await once('default',()=>call(v+'/parsing-default',cmd({expected_revision:'0',config_revision_id:cfg.config_revision_id}),{method:'PUT'}));
    const device=await once('device',()=>call(devices,cmd({code:'WALK-DDP-1',name:'验收用合成位移设备',product_version_id:released.version_id,parsing_adoption:{mode:'inherit_default',expected_default_revision:def.revision}})));
    await once('device-access',async()=>{
      const path=tenantRoot+'/projects/'+project+'/device-access',old=await call(path);
      const body=cmd({expected_device_access_revision:old.device_access_revision,expected_project_revision:old.project_revision,add:[{kind:'device',target_id:device.id,use_source:false}],update:[],end:[]});
      const preview=await call(path+'/preview',body);
      return call(path+'/changes',{...body,preview_hash:preview.preview_hash},{replayCommand:session.ledger.at(-1).command});
    });
    await once('relay',async()=>{
      const known=await call(ingest+'/relays');if((known.items??known).some(r=>r.id===relay.id))return true;
      await call(ingest+'/relays',cmd({id:relay.id,name:'验收合成中继',configuration_ref:'synthetic-lab'}));return true;
    });
    const simulator=join(privateDir,'walkthrough-simulator.json');
    if(!existsSync(simulator)){
      const credentials=relay.producerFor('walkddp');
      write(simulator,{synthetic:true,...credentials,sources:[{...credentials,name:'验收位移',source_code:'walkddp',host:'127.0.0.1',port,protocol:'hongdian.ddp.tcp'}]});
    }
    const source=await once('source',()=>call(ingest+'/sources',cmd({expected_revision:'0',name:'验收位移接入',tenant_id:tenant,source_project_id:project,
      manufacturer_id:maker.id,manufacturer_scope:'public',relay_id:relay.id,ingress_protocol_id:'hongdian.ddp.tcp',ingress_protocol_version:version,
      listener_address:'127.0.0.1',listener_port:read(simulator).sources[0].port,source_code:'walkddp',producer_principal_ids:[read(simulator).principal],
      upstream_segregated:true,synthetic:true,exclusive:false,idle_seconds:180,budget_bytes:'1073741824'})));
    const sp='/sources/'+source.id,detail=await call(area+sp);
    await once('confirmed',()=>call(area+sp+'/confirm',cmd({expected_revision:'1',action:'confirm',digest:detail.revisions[0].digest})));
    await once('published-source',()=>call(ingest+sp+'/commands',cmd({expected_revision:'1',action:'publish'})));
    await call(ingest+sp,undefined,{wait:v=>v.source.state==='active'&&!!v.source.application});
    await once('identify',()=>call(area+'/execution-grants',cmd({expected_revision:'0',source_id:source.id,identify:true,measure:false,enabled:true,budget:10000})));
    const terminal=await once('terminal',()=>call(area+'/terminals',cmd({source_id:source.id,terminal_key:'00000000001',name:'验收位移终端',mode:'single'})));
    const tb=area+'/terminals/'+terminal.id;
    await once('association',async()=>{
      const current=await call(tb);
      return call(tb+'/associations',cmd({expected_revision:current.revision,child_key:'',device_id:device.id,input_view:'complete_frame'}));
    });
    await once('measure',()=>call(area+'/execution-grants',cmd({expected_revision:'0',source_id:source.id,terminal_id:terminal.id,identify:false,measure:true,enabled:true,budget:10000})));
    const messages=area+'/messages',list=messages+'?device_id='+device.id;
    // An accepted bind command still waits for the relay's effective sequence.
    // Frames received earlier correctly retain their old association.
    const links=await call(tb+'/associations',undefined,{wait:rows=>rows.some(r=>r.device_id===device.id&&r.enabled&&r.epoch&&BigInt(r.start_sequence)>0n)});
    const effective=links.find(r=>r.device_id===device.id&&r.enabled);
    await once('delivered-'+effective.epoch+'-'+effective.start_sequence,async()=>{
      const existing=await call(messages+'?source_id='+source.id);
      for(const row of existing.items.filter(r=>r.message_kind==='measurement'&&r.sha256===inputDigest)){
        const seen=await call(messages+'/'+row.id);
        if(seen.receipt.epoch===effective.epoch&&BigInt(seen.receipt.source_sequence)>=BigInt(effective.start_sequence))return true;
      }
      await simulate(simulator,{source:'验收位移',split:true});
      return true;
    });
    const found=await call(list,undefined,{wait:v=>v.items.some(row=>row.message_kind==='measurement'&&row.sha256===inputDigest&&row.acknowledgement==='acknowledged'&&row.state==='parsed')});
    const msg=found.items.find(row=>row.message_kind==='measurement'&&row.sha256===inputDigest&&row.state==='parsed');
    const target=messages+'/'+msg.id,entry=await call(target),results=await call(target+'/results');
    assert.ok(entry.can_raw&&entry.can_results);assert.deepEqual(await call(target+'/content'),measurement);
    assert.equal(results.length,1);checkResponse(results[0].response);assert.equal(results[0].formally_adopted,false);
    const result={tenant_id:tenant,project_id:project,source_id:source.id,terminal_id:terminal.id,device_id:device.id,
      receipt_id:entry.receipt.id,unit_id:msg.id,result_id:results[0].id,method_id:method,method_version:version,raw_bytes:measurement.length,raw_sha256:inputDigest,synthetic:true};
    write(join(privateDir,'walkthrough-ingestion.json'),result);return result;
  }finally{await session.client?.dispose();}
}

async function productConfig(call,products,maker,state){
  const product=await call(products,cmd({manufacturer_id:maker,code:'arc_ddp',name:'ARC 合成终端',model:'ARC-DDP',
    output_slots:[{key:'displacement',name:'位移',mode:'fixed',metric_version_id:state.metric,unit_version_id:state.unit,timestamp_meaning:'observation_time'}]}));
  const published=await call(products+'/'+product.id+'/publish',cmd({expected_draft_revision:product.revision,classification:{mode:'unclassified'}}));
  const path=products+'/'+published.id+'/versions/'+published.version_id,configs=path+'/parsing-configs';
  const config=await call(configs,configInput('0','固定 DDP 1.0.0'));
  const revised=await call(configs+'/'+config.id+'/revisions',configInput(config.revision,'固定 DDP 修订'));
  assert.notEqual(revised.config_revision_id,config.config_revision_id);
  for(const [i,state] of ['deprecated','enabled'].entries()){
    const updated=await call(configs+'/'+config.id+'/state',cmd({expected_revision:String(Number(revised.revision)+i),state}));
    assert.equal(Number(updated.revision),Number(revised.revision)+i+1);
  }
  const defaultConfig=await call(path+'/parsing-default',cmd({expected_revision:'0',config_revision_id:config.config_revision_id}),{method:'PUT'});
  await call(configs);await call(path+'/parsing-default');
  return {published,config,defaultConfig};
}
async function oneSide({lab,side,seed,accounts,stack,keys,records,bodies}){
  const state=seed.structure,tenant=state.tenant,project=state.project;
  const session=new Session(lab[side].api,keys,records,bodies), outsider=new Session(lab[side].api,keys,records,bodies);
  const call=(...args)=>session.call(...args), ordinary=cmd();
  const parse='/platform/parsing',ingest='/platform/ingestion';
  const t='/tenants/'+tenant,tParse=t+'/parsing',tIngest=t+'/ingestion';
  try{
    await session.open(accounts.admin);
    const initial=formalCount(lab.chContainer,lab[side].ch);
    const sync=await call(parse+'/catalog-syncs',ordinary);
    await call(parse+'/catalog-syncs/'+sync.id);
    assert.equal((await call(parse+'/catalog-status')).state,'succeeded');
    const catalogue=await call(parse+'/methods?limit=100',undefined,{project:p=>byName(p.items.map(v=>({entry:v.entry,state:v.state})))});
    assert.ok(catalogue.items.some(v=>v.entry.method_id===method&&v.entry.method_version===version));
    for(const m of byName(catalogue.items.map(v=>({id:v.entry.method_id,version:v.entry.method_version,revision:v.revision})))){
      await call(parse+'/methods/'+m.id+'/versions/'+m.version+'/state',cmd({expected_revision:m.revision,state:'enabled'}));
    }
    await call(parse+'/ingestion-catalog-syncs',ordinary);
    for(const kind of ['identification-methods','ingress-protocols','ingress-compatibilities']){
      const selection=p=>byName(p.items.map(v=>({entry:v.entry,state:v.state})));
      const items=(await call(parse+'/'+kind+'?limit=100',undefined,{project:selection})).items;
      assert.ok(items.length);
      for(const v of [...items].sort((a,b)=>JSON.stringify(a.entry).localeCompare(JSON.stringify(b.entry)))){
        await call(parse+'/'+kind+'/'+v.id+'/state',cmd({expected_revision:v.revision,state:'enabled'}));
      }
      const enabled=(await call(tParse+'/'+kind+'?limit=100',undefined,{project:selection})).items;
      assert.equal(enabled.length,items.length);assert.ok(enabled.every(v=>v.state==='enabled'));
    }
    await call(tParse+'/methods?limit=100',undefined,{project:p=>byName(p.items.map(v=>v.entry))});
    const maker=await call('/platform/device-definitions/manufacturers',cmd({code:'arc_ingest',name:'ARC 接入协议对照厂家'}));
    const {published,config,defaultConfig}=await productConfig(call,'/platform/device-definitions/products',maker.id,state);
    await productConfig(call,t+'/device-definitions/products',maker.id,state);
    const publicVersion=t+'/device-catalog/products/'+published.id+'/versions/'+published.version_id;
    await call(publicVersion+'/parsing-configs');await call(publicVersion+'/parsing-default');
    const device=await call(t+'/devices',cmd({code:'ARC-DDP-1',name:'ARC 合成设备',product_version_id:published.version_id,parsing_adoption:{mode:'inherit_default',expected_default_revision:defaultConfig.revision}}));
    const dev=t+'/devices/'+device.id;
    const adoption=await call(dev+'/parsing');assert.equal(adoption.current.snapshot.method.version,version);
    await call(dev+'/parsing/configs');await call(dev+'/parsing/history');
    const own=await call(dev+'/parsing/configs',configInput('0','单台设备固定 DDP'));
    const fixedDevice=await call(dev+'/parsing/configs/'+own.id+'/revisions',configInput(own.revision,'单台设备独立修订'));
    const override=cmd({expected_revision:adoption.revision,action:'override',config_revision_id:fixedDevice.config_revision_id});
    assert.equal((await call(dev+'/parsing/preview',override)).can_submit,true);
    await call(dev+'/parsing/commands',override);
    const changed=await call(dev+'/parsing'),batchPath=tParse+'/device-adoptions';
    assert.equal(changed.current.config_revision_id,fixedDevice.config_revision_id);
    const inheritance=cmd({action:'restore_inheritance',expected_default_revision:defaultConfig.revision,devices:[{device_id:device.id,expected_revision:changed.revision}]});
    const approved=await call(batchPath+'/preview',inheritance);assert.ok(approved.every(v=>v.can_submit));
    await call(batchPath+'/commands',inheritance);assert.equal((await call(dev+'/parsing')).current.config_revision_id,config.config_revision_id);
    const grants=t+'/projects/'+project+'/device-access', permissions=await call(grants);
    const input=cmd({expected_device_access_revision:permissions.device_access_revision,expected_project_revision:permissions.project_revision,add:[{kind:'device',target_id:device.id,use_source:false}],update:[],end:[]});
    const preview=await call(grants+'/preview',input);
    await call(grants+'/changes',{...input,preview_hash:preview.preview_hash},{replayCommand:session.ledger.at(-1).command});
    const scoped=t+'/projects/'+project+'/devices/'+device.id+'/parsing';await call(scoped);await call(scoped+'/history');
    await manual(session,parse);await manual(session,tParse,device.id);
    await call(ingest+'/relays',cmd({id:stack.relay.id,name:'合成云中继',configuration_ref:'synthetic-lab'}));
    // Relay telemetry is polled independently of registering and publishing a
    // source. Observe the actual empty relay before creating its first listener.
    const relays=await call(ingest+'/relays',undefined,{wait:v=>v.items.some(r=>r.id===stack.relay.id&&r.fresh&&r.observed_at&&r.status?.relay_id===r.id)});
    const observedRelay=relays.items.find(r=>r.id===stack.relay.id);
    assert.deepEqual(observedRelay.status.sources,{});
    await call(ingest+'/source-options');
    const definition=cmd({expected_revision:'0',name:'ARC DDP',tenant_id:tenant,source_project_id:project,manufacturer_id:maker.id,manufacturer_scope:'public',relay_id:stack.relay.id,ingress_protocol_id:'hongdian.ddp.tcp',ingress_protocol_version:'1.0.0',listener_address:'127.0.0.1',listener_port:stack.port,source_code:'arcddp',producer_principal_ids:[stack.producer.principal],upstream_segregated:true,synthetic:true,exclusive:false,idle_seconds:180,budget_bytes:'1073741824'});
    const source=await call(ingest+'/sources',definition);
    const sp='/sources/'+source.id,detail=await call(tIngest+sp,undefined,{wait:v=>v.source.state==='awaiting_tenant_confirmation'&&v.fresh&&!!v.observed_at});
    assert.equal(detail.relay_status,null);
    const rev=detail.revisions[0];assert.equal(rev.snapshot.listener_port,stack.port);assert.equal(rev.snapshot.source_revision_digest,rev.digest);assert.equal(rev.snapshot.relay_id,stack.relay.id);
    assert.equal(digest(JSON.stringify({...rev.snapshot,source_revision_digest:''})),rev.digest);
    await call(tIngest+sp+'/confirm',cmd({expected_revision:'1',action:'confirm',digest:rev.digest}));
    await call(ingest+sp+'/commands',cmd({expected_revision:'1',action:'publish'}));
    const active=await call(ingest+sp,undefined,{
      wait:v=>v.source.state==='active'&&!!v.source.application&&v.fresh&&v.relay_status?.sources?.[source.id]?.state==='active',
      schema:v=>{
        // Each isolated database issued its own source ID. Verify the actual
        // key, then compare all of its fields using the same dictionary slot.
        assert.deepEqual(Object.keys(v.relay_status.sources),[v.source.id]);
        return shape({...v,relay_status:{...v.relay_status,sources:{'[source.id]':v.relay_status.sources[v.source.id]}}});
      },project:v=>pick(v.source,['state','applied_generation','applied_revision','synthetic'])});
    assert.equal(active.source.application.source_revision_digest,rev.digest);
    assert.equal(active.relay_status.sources[source.id].revision,rev.revision);
    assert.equal(active.relay_status.sources[source.id].digest,rev.digest);
    await call(ingest+'/sources');await call(tIngest+'/sources');
    await call(tIngest+'/execution-grants',cmd({expected_revision:'0',source_id:source.id,identify:true,measure:false,enabled:true,budget:10000}));
    const terminal=await call(tIngest+'/terminals',cmd({source_id:source.id,terminal_key:'00000000001',name:'合成设备端',mode:'single'}));
    const tb=tIngest+'/terminals/'+terminal.id, td=await call(tb);
    const assoc=cmd({expected_revision:td.revision,child_key:'',device_id:device.id,input_view:'complete_frame'});
    await call(tb+'/associations/preview',assoc);await call(tb+'/associations',assoc);
    const links=await call(tb+'/associations',undefined,{wait:v=>v.length===1,project:v=>v.map(r=>pick(r,['terminal_revision','start_sequence','end_sequence','enabled','input_view']))});
    assert.equal(links[0].device_id,device.id);assert.equal(links[0].terminal_id,terminal.id);
    await call(tIngest+'/terminals');await call(tb+'/revisions');
    await call(tIngest+'/execution-grants',cmd({expected_revision:'0',source_id:source.id,terminal_id:terminal.id,identify:false,measure:true,enabled:true,budget:10000}));
    const gl=await call(tIngest+'/execution-grants');assert.ok((gl.items??gl).length>=2);
    const sim=join(stack.privateDir,'simulator.json');
    write(sim,{synthetic:true,...stack.producer,sources:[{...stack.producer,name:'ARC',source_code:'arcddp',host:'127.0.0.1',port:stack.port,protocol:'hongdian.ddp.tcp'}]});
    const delivered=await acceptWithoutParser(lab,side,stack,source.id,()=>simulate(sim,{source:'ARC',split:true}));
    const messages=tIngest+'/messages',batch=await call(messages+'?device_id='+device.id,undefined,{
      wait:v=>v.items.length===3&&v.items.every(u=>u.acknowledgement==='acknowledged'&&
        (u.message_kind==='measurement'?u.method_version===version&&u.state==='parsed':u.state==='control')),
      project:v=>v.items.filter(u=>u.message_kind==='measurement').map(u=>pick(u,['method_version','sha256','byte_length','message_kind','acknowledgement','synthetic']))});
    const msg=batch.items.find(u=>u.message_kind==='measurement');assert.equal(msg.sha256,inputDigest);assert.equal(msg.source_project_id,project);
    const unit=messages+'/'+msg.id;
    const evidence=await call(unit,undefined,{wait:v=>v.plans.some(p=>p.state==='parsed'),project:v=>({can_raw:v.can_raw,can_results:v.can_results,can_reprocess:v.can_reprocess,kind:v.unit.message_kind,source_sequence:v.receipt.source_sequence,plans:v.plans.map(p=>pick(p,['state','error_code','attempts']))})});
    assert.ok(evidence.can_raw&&evidence.can_results&&evidence.can_reprocess);assert.equal(evidence.receipt.epoch,links[0].epoch);assert.equal(evidence.unit.device_id,device.id);
    await call(unit+'/decisions');assert.deepEqual(await call(unit+'/content'),measurement);
    const part=await call(unit+'/content',undefined,{headers:{Range:'bytes=0-7'},project:(b,h)=>({digest:digest(b),range:h['content-range']})});
    assert.deepEqual(part,measurement.subarray(0,8));
    const results=await call(unit+'/results',undefined,{project:items=>items.map(v=>({quality:v.business_quality,adopted:v.formally_adopted,response:checkResponse(v.response)}))});
    assert.equal(results.length,1);assert.equal(results[0].formally_adopted,false);assert.equal(results[0].business_quality,'not_evaluated');
    const resultFile=unit+'/results/'+results[0].id+'/content';
    const raw=await call(resultFile,undefined,{project:b=>checkResponse(JSON.parse(b))});assert.equal(digest(raw),results[0].sha256);assert.deepEqual(JSON.parse(raw),results[0].response);
    await call(dev+'/ingestion');await call(t+'/projects/'+project+'/devices/'+device.id+'/ingestion');
    const recovery=tIngest+'/recovery-operations',fixed=cmd({kind:'reparse',unit_ids:[msg.id],config_revision_id:config.config_revision_id});
    const eligible=await call(recovery+'/preview',fixed);assert.equal(eligible.state,'ready');
    // Hold the real parser response while both topologies expose the same
    // public intermediate checkpoint, then let the fixed method complete.
    stack.gate.hold();let job;
    try{
      job=await call(recovery,fixed);
      const active=await call(recovery,undefined,{wait:v=>v.some(r=>r.id===job.id&&r.state==='running'&&r.items.every(i=>i.state==='running'))});
      assert.equal(active.find(r=>r.id===job.id).revision,'3');
    }finally{stack.gate.release();}
    const jobs=await call(recovery,undefined,{wait:v=>v.some(r=>r.id===job.id&&r.state==='succeeded'),project:v=>v.map(r=>pick(r,['kind','state','generation']))});
    const generation=jobs.find(r=>r.id===job.id).generation;
    const exact=await call(recovery+'/'+job.id+'/generations/'+generation);
    assert.equal(exact.id,job.id);assert.deepEqual(exact.input.unit_ids,[msg.id]);assert.equal(exact.input.config_revision_id,config.config_revision_id);
    const adminRecovery=ingest+'/recovery-operations',reconcile=cmd({kind:'reconcile',receipt_ids:[evidence.receipt.id]});
    assert.equal((await call(adminRecovery+'/preview',reconcile)).state,'ready');
    const repair=await call(adminRecovery,reconcile);
    const repaired=await call(adminRecovery,undefined,{wait:v=>v.some(r=>r.id===repair.id&&r.state==='succeeded'),project:v=>v.map(r=>pick(r,['kind','state','generation']))});
    const fixedGeneration=repaired.find(r=>r.id===repair.id).generation;
    const historical=await call(adminRecovery+'/'+repair.id+'/generations/'+fixedGeneration);
    assert.deepEqual(historical.input.receipt_ids,[evidence.receipt.id]);assert.equal(historical.items[0].state,'succeeded');
    for(const action of ['pause','resume']){
      const current=(await call(tIngest+sp)).source;
      await call(tIngest+sp+'/planning',cmd({action,expected_revision:current.revision}));
      const changed=(await call(tIngest+sp)).source;assert.equal(changed.state,'active');assert.equal(changed.planning_paused,action==='pause');
    }
    assert.equal((await simulate(sim,{source:'ARC',unknown:true})).receipts,3);
    const unmatched=await call(tIngest+'/unmatched?source_id='+source.id,undefined,{
      wait:v=>v.items.filter(r=>r.unmatched_units!=='0').length>=3,
      project:v=>byName(v.items.map(r=>pick(r,['evidence_kind','byte_length','state','error_code','unmatched_units'])))});
    const missing=unmatched.items.find(r=>Number(r.byte_length)===measurement.length);
    assert.ok(missing);assert.equal((await call(tIngest+'/unmatched/'+missing.id+'/close',cmd())).state,'disposition_recorded');
    const currentTerminal=await call(tb);
    await call(tb+'/revisions',cmd({expected_revision:currentTerminal.revision,source_id:source.id,terminal_key:currentTerminal.terminal_key,name:'核验后的终端名称',mode:'single'}));
    const terminalRevision=(await call(tb)).revision;
    for(const action of ['disable','enable']){
      await call(tIngest+'/associations/'+links[0].id+'/state',cmd({expected_revision:terminalRevision,action}));
      assert.equal((await call(tb+'/associations'))[0].enabled,action==='enable');
    }
    // A pending watermark command is cancelable while that side's relay is offline.
    await stack.relay.pause();
    try{
      const pending=await call(tb+'/associations',{...assoc,expected_revision:terminalRevision});
      assert.equal(pending.state,'pending_watermark');
      const canceled=await call(tIngest+'/changes/'+pending.id+'/cancel',cmd());assert.equal(canceled.id,pending.id);
    }finally{await stack.relay.resume();}
    const updated=await call(ingest+sp+'/revisions',{...definition,expected_revision:active.source.revision,name:'ARC DDP 修订二'});
    assert.equal(updated.revision,'2');
    for(const path of [tIngest+'/operations',ingest+'/quarantine',ingest+'/integrity-events'])await call(path);
    await outsider.open(accounts.other);
    for(const endpoint of [unit,unit+'/content',resultFile])await outsider.call(endpoint,undefined,{denied:[403,404]});
    const end=formalCount(lab.chContainer,lab[side].ch);assert.equal(end,initial);
    return {formalObservationsBefore:initial,formalObservationsAfter:end,acceptedFrames:delivered.receipts,parserOffline:delivered.parserOffline,measurementBytes:measurement.length,measurementSHA256:inputDigest,method,version,associationStart:links[0].start_sequence,sourceGeneration:active.source.applied_generation,measurements:results.length,reprocessedGeneration:generation,crossTenantDenied:true};
  }finally{await Promise.all([session.client?.dispose(),outsider.client?.dispose()]);}
}

export async function ingestionHTTP(out){
  const stacks={},cleanups=[],records={a:[],b:[]},bodies={a:[],b:[]},proof={},keys=new Map(),name='ingestion-parsing',dest=join(out,'http','scenarios',name+'.json');
  const programs={};
  let sandbox;
  function save(status,info={}){
    const entries=[];
    for(let i=0;i<Math.max(records.a.length,records.b.length);i++){
      const l=records.a[i], r=records.b[i];if(!l||!r)continue;
      const differences=[];
      for(const key of ['method','template','command','status','mime','headers','schema'])if(JSON.stringify(l[key])!==JSON.stringify(r[key]))differences.push(key);
      const compared = [l,r].map((row,index) => row.status >= 200 && row.status < 300
        ? compareParserAttestation(row.operationId,row.semantics,programs[index ? 'b' : 'a'].binaries.parser) : row.semantics);
      if(JSON.stringify(compared[0])!==JSON.stringify(compared[1]))differences.push('semantics');
      entries.push({...pick(l,['method','template','operationId','owner','command']),status:[l.status,r.status],sha256:[l.sha256,r.sha256],bodyIndex:i,comparison:'typed response and domain assertions',differences,errors:[null,null]});
    }
    write(dest,{scenario:name,status,inputs:'separate snapshot clones, sealed parser services, identical method package and real mTLS DDP',parserPrograms:programs,
      bodyEvidence:['private/a/public-bodies.json','private/b/public-bodies.json'],records:entries,assertions:proof,...info});return entries;
  }
  try{
    return await withHTTPPair(out,'ingestion-parsing',async dir=>{
      const lab=read(join(dir,'private/lab.json')),seed=read(join(dir,'samples.json')),accounts=read(join(dir,'private/accounts.json'));
      sandbox=dir;
      write(join(dir,'parser-programs.json'),programs);
      const runs=await Promise.allSettled(['a','b'].map(async side=>{
        try{proof[side]=await oneSide({lab,side,seed,accounts,stack:stacks[side],keys,records:records[side],bodies:bodies[side]});}
        finally{
          write(join(dir,side+'-public-evidence.json'),records[side]);
          write(join(dir,'private',side,'public-bodies.json'),bodies[side]);
          save('running',{sandbox:dir});
        }
      }));
      const errors=runs.flatMap((r,i)=>r.status==='rejected'?[(i===0?'A':'B')+' '+r.reason.message.split('\n')[0]]:[]);
      if(errors.length)throw Error(errors.join(' | '));
      assert.deepEqual(proof.a,proof.b);
      const entries=save('checking',{sandbox:dir,assertions:proof});assert.equal(records.a.length,records.b.length);
      const failures=entries.filter(r=>r.differences.length);assert.deepEqual(failures.map(r=>[r.operationId,r.differences]),[],'public ingestion contract differences');
      const comparison=checkIngestionBodies(dir,entries);write(join(dir,'body-comparison.json'),comparison);
      assert.deepEqual(comparison.rows.filter(r=>r.differences.length),[],'complete ingestion DTO differences');
      save('passed',{sandbox:dir,assertions:proof});
      write(join(out,'faults','parser-outage.json'),{sourceRun:dir,cases:[{arc:'ARC-027',name:'停止 parser 后仍有可读原文与收据，恢复后完成同代解析',passed:true,checks:{baseline:proof.a.parserOffline,candidate:proof.b.parserOffline,measurements:proof.b.measurements,formalBefore:proof.b.formalObservationsBefore,formalAfter:proof.b.formalObservationsAfter}}]});
      return {calls:entries.length,operations:new Set(entries.map(r=>r.operationId)).size};
    },{restrictedRoles:['identity-api','control-api','monitoring-query-api','transfer-api','ingest-custody-worker','parsing-dispatch-worker'],configure:async({side,cfg,privateDir,baseline,bin,dir,release})=>{
      const manifest = side === 'a' ? baseline : release;
      const workers = side === 'a' ? ['ingest-worker','monitoring-worker'] : ['ingest-custody-worker','parsing-dispatch-worker'];
      const binaries = Object.fromEntries(['parser','ingest-relay',...workers].map(name => {
        const actual = digest(readFileSync(join(bin,name)));
        assert.equal(actual,manifest.binaries[name],side+' '+name);
        return [name,actual];
      }));
      programs[side]={releaseManifest:side==='a'?join(dir,'baseline.json'):join(dirname(bin),'candidate.json'),binaries};
      // Both parser services execute the same frozen method package and runner.
      const parser=await startParser(join(privateDir,'parser'),{bin:baseline.bin,source:baseline.source,serviceBinary:join(bin,'parser')});cleanups.push(()=>parser.close());
      const relay=await startRelay(join(privateDir,'relay'),{binary:join(bin,'ingest-relay')});cleanups.push(()=>relay.close());
      const gate=await parserGate(parser.url);cleanups.push(()=>gate.close());
      stacks[side]={privateDir,relay,parser,gate,producer:relay.producerFor('arcddp'),port:await availablePort()};
      cfg.parsing={enabled:'true',url:gate.url,token_file:parser.token,object_endpoint:cfg.monitoring_object_endpoint,object_access_key:cfg.monitoring_object_access_key,object_secret_key:cfg.monitoring_object_secret_key,bucket:cfg.monitoring_source_bucket};
      cfg.ingestion_file=relay.workerConfig;
      return {start:args=>startPairedWorkers({...args,side,privateDir,legacy:['ingest-worker','monitoring-worker'],split:['ingest-custody-worker','parsing-dispatch-worker']})};
    }});
  }catch(error){save('failed',{sandbox,runError:error.message.split('\n')[0]});throw error;}
  finally{await Promise.allSettled(cleanups.reverse().map(close=>close()));}
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url))console.log(await ingestionHTTP(runDirectory(process.argv[2])));
