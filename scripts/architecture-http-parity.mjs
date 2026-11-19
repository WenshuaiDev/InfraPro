// Exercises the frozen public program and the candidate's public gateway. The
// snapshots are independent, so operations which touch activity, even GETs,
// are issued exactly once to each environment. Only digests enter the ledger.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { digest, read, root, runDirectory, write } from './architecture-lab.mjs';

const unknown = '00000000-0000-4000-8000-000000000001';
const readMethod = method => method === 'GET' || method === 'HEAD';
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const timestamp = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/;
const generatedTimes = new Set(['computed_at','generated_at','server_time','as_of','created_at','updated_at','expires_at','last_activity_at','reauthenticated_at','recorded_at','started_at','completed_at','finished_at','occurred_at','last_seen_at','verified_at','as_of_time','snapshot_at']);
const digestValue = value => digest(Buffer.from(typeof value === 'string'?value:JSON.stringify(value)));
const group = values => Object.fromEntries([...new Set(values.map(x=>x.name.toLowerCase()))].sort().map(name=>[name,values.filter(h=>h.name.toLowerCase()===name).map(h=>h.value)]));

function catalog(){
  return readFileSync(join(root,'services/api/internal/routes/ownership.tsv'),'utf8').trimEnd().split('\n').map(line=>{
    const [method,template,owner,operationId,permission,access]=line.split('\t');
    return {method,template,owner,operationId,permission,access,cases:[]};
  });
}
async function actor(side,credentials){
  assert.equal(new URL(side.api).hostname,'127.0.0.1');
  const http=await request.newContext({baseURL:side.api,timeout:45000,maxRedirects:0});
  const csrf=await (await http.get('/api/v1/auth/csrf')).json();
  const headers={Origin:side.origin,'X-CSRF-Token':csrf.token};
  let sessionProof;
  if(credentials){
    const login=await http.post('/api/v1/auth/login',{headers,data:credentials});
    assert.equal(login.status(),200,'paired login failed');
    const result=await login.json();assert.equal(result.status,'authenticated');
    const sessions=await http.get('/api/v1/account/sessions',{headers});assert.equal(sessions.status(),200);
    const current=(await sessions.json()).items.filter(s=>s.current);
    assert.equal(current.length,1);assert.match(current[0].id,uuid);
    sessionProof={account:result.account.id,session:current[0].id,request:login.headers()['x-request-id']};
    assert.ok(sessionProof.request);await sessions.dispose();await login.dispose();
  }
  return {http,headers,side,sessionProof};
}
async function sample(actor, method, path, input, command, extra){
  const start=Date.now();
  let response;try{response=await actor.http.fetch(path,{method,data:input,headers:{...actor.headers,'X-Command-ID':command,...extra}});}catch(e){return {status:0,headers:{},bytes:Buffer.alloc(0),failure:e.name==='TimeoutError'?'timeout':'transport_unavailable',start,end:Date.now()};}
  try {
    const bytes=await response.body(), headers=group(response.headersArray()), status=response.status();
    const json=headers['content-type']?.[0].includes('json') && bytes.length?JSON.parse(bytes):undefined;
    return {status,headers,bytes,json,start,end:Date.now()};
  } finally {await response.dispose();}
}
function stableHeaders(r) {
  const h={...r.headers};
  assert.equal(h['x-request-id']?.length,1,'missing unique request ID');
  assert.ok(h['x-request-id'][0].length>=16);
  delete h['x-request-id'];
  if(h.date){assert.ok(Number.isFinite(Date.parse(h.date[0])));delete h.date;}
  if(h['x-monitoring-trace-id']){assert.match(h['x-monitoring-trace-id'][0],/^[0-9a-f-]{36}$/i);h['x-monitoring-trace-id']=['[trace]'];}
  if(h['set-cookie'])h['set-cookie']=h['set-cookie'].map(s=>s.replace(/^([^=]+=)([^;]*)/,(_,key,value)=>key+(value?'[credential]':'')));
  return h;
}
// Map only ephemeral values produced in the two paired environments. Business
// timestamps such as observation time and effective ranges are always exact.
function compare(op,a,b,labStart,sides,proofs,cursor){
  const differences=[], normalized=[];
  const mismatch=(path,x,y)=>differences.push({path,a:digestValue(x===undefined?'(missing)':x),b:digestValue(y===undefined?'(missing)':y)});
  function walk(x,y,path,key){
    if(Object.is(x,y))return;
    if(typeof x==='string' && typeof y==='string'){
      if(key==='console_url' && x===sides[0].origin+'/' && y===sides[1].origin+'/'){normalized.push(path);return;}
      if(key==='runtime_origin' && x===sides[0].runtime && y===sides[1].runtime){normalized.push(path);return;}
      if(key==='next_cursor' && cursor?.verified && x===cursor.left && y===cursor.right){normalized.push(path);return;}
      if(['id','object_id','request_id'].includes(key) && proofs.some(([l,r])=>l.account===r.account &&
        (key==='request_id'?l.request===x&&r.request===y:l.session===x&&r.session===y))){normalized.push(path);return;}
      if(key==='observed_at' && (op.template.includes('/audit/') || op.template.startsWith('/api/v1/tasks')) && timestamp.test(x) && timestamp.test(y) && Math.abs(Date.parse(x)-a.end)<10000 && Math.abs(Date.parse(y)-b.end)<10000){normalized.push(path);return;}
      if(key==='request_id' && x===a.headers['x-request-id'][0] && y===b.headers['x-request-id'][0]){normalized.push(path);return;}
      if(key==='request_id'&&proofs.requests.get(x)===y){normalized.push(path);return;}
      if(key==='token' && op.operationId==='getCSRF' && x.length>=32 && x.length===y.length){normalized.push(path);return;}
      if(generatedTimes.has(key) && timestamp.test(x) && timestamp.test(y)){
        const left=Date.parse(x),right=Date.parse(y);
        // Both stamps must be independently produced with the same offset
        // relative to this request, and belong to this test run. Old facts
        // restored from the snapshot can never satisfy the lower bound.
        if(Math.min(left,right)>=labStart-1000 && Math.abs((left-a.start)-(right-b.start))<=1500){normalized.push(path);return;}
      }
    }
    if(x&&y&&typeof x==='object'&&typeof y==='object'&&Array.isArray(x)===Array.isArray(y)){
      if(x.event_type?.startsWith('audit.command.')&&x.event_type===y.event_type&&x.object?.type==='operation'&&y.object?.type==='operation'&&x.object.id===y.object.id){
        const l=x.technical,r=y.technical;
        const permission=l&&proofs.permissions.get(l.request_id);
        if(permission&&r&&proofs.requests.get(l.request_id)===r.request_id&&l.source_module==='audit'&&r.source_module==='audit'&&l.source_key===l.request_id+':'+permission+':command'&&r.source_key===r.request_id+':'+permission+':command'&&uuid.test(x.operation_id)&&uuid.test(y.operation_id)&&x.operation_id===l.operation_id&&y.operation_id===r.operation_id){
          normalized.push(path+'/operation_id',path+'/technical/operation_id',path+'/technical/source_key');
          x={...x,operation_id:'[attempt]',technical:{...l,operation_id:'[attempt]',source_key:'[attempt]'}};
          y={...y,operation_id:'[attempt]',technical:{...r,operation_id:'[attempt]',source_key:'[attempt]'}};
        }
      }
      if(x.event_type==='login' && y.event_type==='login' && x.object?.type==='session' && y.object?.type==='session'){
        const proof=proofs.find(([l,r])=>l.session===x.object.id&&r.session===y.object.id&&l.account===x.actor?.id&&r.account===y.actor?.id&&l.request===x.technical?.request_id&&r.request===y.technical?.request_id);
        if(proof && uuid.test(x.operation_id) && uuid.test(y.operation_id) && x.operation_id===x.technical.operation_id && y.operation_id===y.technical.operation_id){
          const left=x.technical.source_key,right=y.technical.source_key;
          if(/^\d+$/.test(left.split(':')[0]) && /^\d+$/.test(right.split(':')[0]) && left.endsWith(':login:'+x.object.id) && right.endsWith(':login:'+y.object.id)){
            normalized.push(path+'/operation_id',path+'/technical/operation_id',path+'/technical/source_key');
            x={...x,operation_id:'[login]',technical:{...x.technical,operation_id:'[login]',source_key:'[login]'}};
            y={...y,operation_id:'[login]',technical:{...y.technical,operation_id:'[login]',source_key:'[login]'}};
          }
        }
      }
      for(const k of new Set([...Object.keys(x),...Object.keys(y)]))walk(x[k],y[k],path+'/'+k,k);
      return;
    }
    mismatch(path,x,y);
  }
  if(a.status!==b.status)mismatch('status',a.status,b.status);
  const h1=stableHeaders(a),h2=stableHeaders(b);
  if(a.json!==undefined && b.json!==undefined){
    walk(a.json,b.json,'body','');
    if(h1['content-length'] && h2['content-length'] && a.bytes.length!==b.bytes.length && !differences.length && Number(h1['content-length'][0])===a.bytes.length && Number(h2['content-length'][0])===b.bytes.length){
      normalized.push('headers/content-length');delete h1['content-length'];delete h2['content-length'];
    }
  }else if(!a.bytes.equals(b.bytes))mismatch('body',a.bytes.toString('hex'),b.bytes.toString('hex'));
  walk(h1,h2,'headers','');
  return {equal:differences.length===0,differences,normalized};
}
function auditCursor(result,first){
  const value=result.json.next_cursor;
  const [encoded,signature,...more]=value.split('.');
  assert.equal(more.length,0);assert.equal(Buffer.from(signature,'base64url').length,32);
  const claim=JSON.parse(Buffer.from(encoded,'base64url'));
  assert.deepEqual(Object.keys(claim).sort(),['a','f','i','p','s','t','u','v']);
  const last=result.json.items.at(-1);
  assert.equal(claim.i,String(last.id));assert.equal(Date.parse(claim.t),Date.parse(last.recorded_at||last.created_at));
  assert.equal(claim.p,'events');assert.ok(Date.parse(claim.u)>=first.start-1000 && Date.parse(claim.u)<=first.end+1000);
  return claim;
}
async function followAudit(op,people,paths,results,started,sides,proofs){
  if(!/\/audit(?:\/events|-events)$/.test(op.template)||!results.every(v=>v.status===200&&v.json?.next_cursor))return;
  let page=results;
  const seen=[new Set(),new Set()],pages=[];
  while(page.every(v=>v.json.next_cursor)){
    assert.ok(pages.length<100,'unbounded audit pagination');
    const cursors=page.map((r,i)=>auditCursor(r,results[i]));
    const {u:leftUpper,...left}=cursors[0],{u:rightUpper,...right}=cursors[1];
    assert.ok(leftUpper&&rightUpper);assert.deepEqual(left,right);
    for(const [i,r] of page.entries())for(const row of r.json.items){assert.ok(!seen[i].has(row.id),'overlapping audit pages');seen[i].add(row.id);}
    const tokens=page.map(r=>r.json.next_cursor),command=randomUUID();
    const next=await Promise.all(people.map((p,i)=>{const url=new URL(paths[i],sides[i].api);url.searchParams.set('cursor',tokens[i]);return sample(p,'GET',url.pathname+url.search,undefined,command,{});}));
    assert.ok(next.every(r=>r.status===200&&Array.isArray(r.json?.items)),'signed continuation rejected');
    const rowIDs=next.map(r=>r.json.items.map(row=>row.id));assert.deepEqual(...rowIDs);
    for(const [i,ids] of rowIDs.entries())for(const id of ids)assert.ok(!seen[i].has(id));
    // The next cursor, if present, is checked against its own page on the following iteration.
    const fixed=next.map(r=>({...r,json:{...r.json,next_cursor:Boolean(r.json.next_cursor)}}));
    const checked=compare(op,...fixed,started,sides,proofs);
    assert.deepEqual(checked.differences,[],'different audit continuation');
    pages.push({command,count:rowIDs[0].length,idsDigest:digestValue(rowIDs[0])});page=next;
  }
  assert.equal(Boolean(page[0].json.next_cursor),Boolean(page[1].json.next_cursor));
  return {verified:true,left:results[0].json.next_cursor,right:results[1].json.next_cursor,pages};
}
const sourceRows = obj => Array.isArray(obj)?obj:(['items','rows','definitions','members','versions','accounts','roles','events','data','results','grants'].map(k=>obj?.[k]).find(Array.isArray)||[]);
function fixtureID(op, name, prefix, cache, fixture){
  const s=fixture.structure;
  const found=cache.get(prefix);
  const first=sourceRows(found)[0];
  const known={tenant_id:s.tenant,project_id:s.project,facility_id:s.nodes.A.id,component_id:s.nodes.C1.id,monitoring_section_id:s.nodes.S1.id,borehole_id:s.nodes.H1.id,point_id:s.nodes.P1.id,channel_id:s.channels.P1.id,device_id:s.devices[0].id,kind:'device',part:'1',generation:'1',version:'1'};
  if(known[name])return known[name];
  const def=['type-definitions','monitoring-templates','units','metrics'].find(k=>prefix.includes('/'+k));
  if(name==='definition_id' && def==='type-definitions')return s.types.A.definition_id;
  if(name==='version_id' && def){const v={'type-definitions':s.types.A.version_id,'monitoring-templates':s.publicTemplate.version_id,units:s.unit,metrics:s.metric};return v[def];}
  if(name==='definition_id' && def==='monitoring-templates')return prefix.includes('/tenants/')&&!prefix.includes('/catalog/')?s.template.definition_id:s.publicTemplate.definition_id;
  if(name==='id' && prefix.endsWith('/accounts'))return fixture.actors.admin;
  if(name==='id' && (prefix.endsWith('/imports') || prefix.endsWith('/monitoring/batches')))return s.batch;
  if(name==='id' && prefix.endsWith('/monitoring/points'))return s.nodes.P1.id;
  if(name==='id' && prefix.endsWith('/device-classifications/:kind'))return s.devices[0].id;
  if(first&&typeof first==='object'){
    const item=first.current??first;
    for(const key of [name,...(name==='definition_id'?['definition_id']:[]),'id','version_id','account_id','membership_id','request_id','tenant_id','project_id'])if(typeof item[key]==='string' && uuid.test(item[key]))return item[key];
  }
  return unknown;
}
function makePath(op,cache,f,absent=false){
  const parts=op.template.split('/');const filled=[];
  for(let i=0;i<parts.length;i++){
    const s=parts[i];if(!s.startsWith(':')){filled.push(s);continue;}
    const key=s.slice(1);
    const id=fixtureID(op,key,parts.slice(0,i).join('/'),cache,f);
    filled.push(encodeURIComponent(absent && !['tenant_id','project_id','kind','part','generation','version'].includes(key)?unknown:id));
  }
  let path=filled.join('/');
  if(op.method==='GET' && op.template.endsWith('/read-scope-options'))path+='?purpose=config';
  return path;
}
export async function httpParity(directory,{output=directory}={}){
  return withHTTPPair(directory,'operations',fixture=>exerciseOperations(fixture,output));
}
async function exerciseOperations(directory,output){
  const lab=read(join(directory,'private/lab.json')),f=read(join(directory,'samples.json')),accounts=read(join(directory,'private/accounts.json'));
  const matrix=catalog(),cache=[new Map(),new Map()], actors={};
  const file=join(output,'http','operations.json'),started=Date.now(),fixture=read(join(directory,'snapshot.json'));
  const sides=[lab.a,lab.b].map(v=>({...v,runtime:read(v.config).display.runtime_origin})),proofs=[];
  proofs.requests=new Map();proofs.permissions=new Map();
  async function run(op,who,paths,data,mode,extra={}){
    const command=randomUUID();
    const result=await Promise.all(actors[who].map((person,i)=>sample(person,op.method,paths[i],data,command,extra)));
    const requests=result.map(r=>r.headers['x-request-id']?.[0]);
    if(requests.every(Boolean)){proofs.requests.set(...requests);proofs.permissions.set(requests[0],op.permission);}
    const [a,b]=result;let cursor,cursorError;
    if(mode==='restored-resource')try{cursor=await followAudit(op,actors[who],paths,result,started,sides,proofs);}catch(e){cursorError=e.name;}
    const checked=a.failure||b.failure?{equal:false,differences:[{path:'transport',a:a.failure||'received',b:b.failure||'received'}],normalized:[]}:compare(op,a,b,started,sides,proofs,cursor);
    if(cursorError){checked.equal=false;checked.differences.push({path:'audit/continuation',error:cursorError});}
    const evidence={principal:who,scenario:mode,command,paths,input_sha256:data===undefined?null:digestValue(data),status:[a.status,b.status],
      errors:result.map(r=>r.json?.code||null),body_sha256:result.map(r=>digest(r.bytes)),...checked,...cursor&&{continuations:cursor.pages}};
    op.cases.push(evidence);
    return result;
  }
  function save(){write(file,{startedAt:new Date(started).toISOString(),baselineCommit:read(join(directory,'baseline.json')).commit,fixture,operations:matrix,
    totals:{operations:matrix.length,cases:matrix.reduce((n,o)=>n+o.cases.length,0),success:matrix.filter(o=>o.cases.some(c=>c.equal&&c.status[0]>=200&&c.status[0]<300)).length,
      differences:matrix.filter(o=>o.cases.some(c=>!c.equal)).length}});}
  try{
    for(const [name,account] of Object.entries({anonymous:null,admin:accounts.admin,outsider:accounts.other})){
      actors[name]=await Promise.all(sides.map(side=>actor(side,account)));
      if(account)proofs.push(actors[name].map(v=>v.sessionProof));
    }
    const ordered=matrix.toSorted((a,b)=>a.template.split('/').length-b.template.split('/').length||a.template.localeCompare(b.template));
    // Lookup results supply valid nested identities before resource detail
    // reads. Erroneous references separately exercise the hiding policy.
    for(const [index,op] of ordered.filter(o=>readMethod(o.method)).entries()){
      const paths=cache.map(c=>makePath(op,c,f));
      const values=await run(op,'admin',paths,undefined,'restored-resource');
      values.forEach((v,i)=>{if(v.status>=200&&v.status<300 && v.json)cache[i].set(op.template,v.json);});
      if(index%50===0)save();
    }
    for(const [index,op] of ordered.entries()){
      const paths=cache.map(c=>makePath(op,c,f,true));
      const read=readMethod(op.method);
      const data=read?undefined:'{';
      const headers=read?{}:{'Content-Type':'application/json'};
      await run(op,'anonymous',paths,data,'missing-session',headers);
      // The paired outsider owns only the other tenant. Never invalidate its
      // own browser before exercising the remaining protected routes.
      if(!op.template.startsWith('/api/v1/auth/') && !op.template.startsWith('/api/v1/account/') && (op.template.includes(':tenant_id') || op.access==='permission' || op.access==='permission_any'))await run(op,'outsider',paths,data,'outside-scope',headers);
      if(!read && !op.template.startsWith('/api/v1/auth/') && !op.template.startsWith('/api/v1/account/'))await run(op,'admin',paths,data,'malformed-command',headers);
      if(index%60===0)save();
    }
    save();return read(file);
  }finally {save();await Promise.all(Object.values(actors).flatMap(p=>p.map(a=>a.http.dispose())));}
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const result=await httpParity(runDirectory(process.argv[2]),{output:runDirectory(process.argv[3]||process.argv[2])});
  console.log(result.totals);
  // Legitimate writes acquire their linked inputs and authoritative revisions
  // in the domain workflows. The final combined ledger gates all 1,071.
  process.exitCode=result.totals.differences||result.totals.operations!==1071?1:0;
}
