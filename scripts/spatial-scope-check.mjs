import { requireAuthenticatedLogin } from './login-result.mjs';
import assert from 'node:assert/strict';
import { request } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { readFileSync,readdirSync,writeFileSync } from 'node:fs';
import { join } from 'node:path';
const root=process.cwd(),sessions=join(root,'.artifacts/spatial-tests'),directory=process.env.SPATIAL_SESSION??join(sessions,readdirSync(sessions).sort().at(-1));
const state=JSON.parse(readFileSync(join(directory,'spatial-acceptance.json'))),login=JSON.parse(readFileSync(join(directory,'login.json'))),origin='http://127.0.0.1:15473',api=origin+'/api/v1',client=await request.newContext(),csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:origin,'X-CSRF-Token':csrf.token},results=[];
const base='/tenants/'+state.tenant,tenant=base+'/spatial',pa=base+'/projects/'+state.projects.A.id,pb=base+'/projects/'+state.projects.B.id;
const wm=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'工程空间范围自验，完成后恢复样本授权'});
async function call(path,data,method=data===undefined?'GET':'POST',expected=200){const r=await client.fetch(api+path,{method,headers,data,timeout:120000});assert.equal(r.status(),expected,`${method} ${path}: ${r.status()} ${expected===200?await r.text():''}`);return expected===200||expected===201?r.json():undefined;}
const summary=(scope,kind,id)=>call(scope+'/summaries',{objects:[{kind,id}]}),related=(scope,kind,id,descendants=false)=>call(scope+'/related-artifacts',{owner:{kind,id},descendants,limit:20});
const channel=(s,id)=>s.items.flatMap(o=>o.channels).find(c=>c.channel.id===id);
const canonical=v=>String(v).replace(/\.0+$/,'');
const observed=(s,id)=>{const c=channel(s,id);return c&&(c.latest.channels[0].latest_output??c.latest.channels[0].last)?.value;};
async function check(name,fn){const started=performance.now();await fn();results.push({name,passed:true,duration_ms:Math.round(performance.now()-started)});console.log('PASS '+name);}
async function spatialChange(project,add,end=[]){const current=await call(project+'/spatial-access'),input={...wm(current.revision),add,end},preview=await call(project+'/spatial-access/preview',input);return call(project+'/spatial-access',{...input,preview_hash:preview.preview_hash},'PUT');}
const cleanups=[];
try{
 requireAuthenticatedLogin(await call('/auth/login',{username:login.username,password:login.password}));
 const pdf=state.assets['BRIDGE-PDF'],v1=pdf.versions[0].id,v2=pdf.versions[1].id,ps=pa+'/spatial',bs=pb+'/spatial';
 await check('SC-01 same identity across PDF, GLB, IFC and geographic position',async()=>{
  for(const key of ['BRIDGE-PDF','BRIDGE-GLB','BRIDGE-IFC4']){const p=await call(tenant+'/versions/'+state.assets[key].versions[0].id+'/placements?owner_id='+state.point);assert(p.items.some(p=>p.point_id===state.point));}
  const point=await call(tenant+'/objects/'+state.point+'?kind=point');assert.equal(point.longitude,116.3802);assert.equal(point.latitude,39.9001);
  assert.equal(canonical(observed(await summary(tenant,'point',state.point),state.channels.A.id)),'20');
 });
 await check('SC-04 replacement candidates remain separate from fixed V1',async()=>{const draft=await call(tenant+'/versions/'+v2+'/draft');assert(draft.version.candidates.length);assert((await call(tenant+'/versions/'+v1+'/placements')).items.length>0);});
 await check('SC-05 V2 requires an explicit project grant',async()=>{
  await call(ps+'/versions/'+v2+'/manifest',undefined,'GET',404);
  await spatialChange(pa,[{version_id:v2,source_download:false,is_default:false}]);
  const grants=await call(pa+'/spatial-access'),g=grants.grants.find(g=>g.version_id===v2&&!g.ended_at);cleanups.push(()=>spatialChange(pa,[],[g.id]));
  assert.equal((await call(ps+'/versions/'+v2+'/manifest')).id,v2);
 });
 await check('SC-06 channel ancestor context has no point placements or exact object',async()=>{
  assert.equal((await call(bs+'/versions/'+v1+'/placements')).items.length,0);
  await call(bs+'/objects/'+state.point+'?kind=point',undefined,'GET',404);
  assert.equal((await summary(bs,'facility',state.facility)).items[0].channels.length,0);
 });
 await check('SC-07 source download is independent of view',async()=>{
  await call(ps+'/versions/'+v1+'/source',undefined,'GET',404);
  const current=await call(pa+'/spatial-access'),old=current.grants.find(g=>g.version_id===v1&&!g.ended_at);
  await spatialChange(pa,[{version_id:v1,source_download:true,is_default:true}],[old.id]);
  cleanups.push(async()=>{const g=(await call(pa+'/spatial-access')).grants.find(g=>g.version_id===v1&&!g.ended_at);await spatialChange(pa,[{version_id:v1,source_download:false,is_default:true}],[g.id]);});
  const response=await client.get(api+ps+'/versions/'+v1+'/source');assert.equal(response.status(),200);assert((await response.body()).subarray(0,4).equals(Buffer.from('%PDF')));assert.match(response.headers()['cache-control'],/no-store/);
 });
 await check('SC-08 output-only source context and independent placement',async()=>{
  const path=pa+`/engineering/resources/facilities/${state.facility}/points/${state.point}/channels/${state.channels.A.id}/source`;
  const current=await call(path),history=await call(path+'/history');assert(current.source.configured);const text=JSON.stringify({current,history});assert(!text.includes('PRIVATE-SYNTHETIC'));assert(!text.includes('serial_number'));assert(text.includes(state.devices[1].output));assert(text.includes(state.devices[0].output));
  const placements=await call(ps+'/versions/'+v1+'/placements?owner_id='+state.point);assert(placements.items.length>0);
 });
 await check('SC-09 point, facility and component own their real calculated channels',async()=>{
  const facility=await summary(ps,'facility',state.facility),component=await summary(ps,'component',state.component),point=await summary(ps,'point',state.point);
  assert.equal(canonical(observed(facility,state.channels.A_MINUS_B.id)),'9');assert.equal(canonical(observed(component,state.channels.COMPONENT_DIFF.id)),'9');assert(!channel(point,state.channels.A_MINUS_B.id));assert(point.items[0].channels.some(c=>c.channel.source_kind==='derived'));
 });
 await check('SC-11 related artifacts use actual inputs and explicit descendants',async()=>{
  assert.equal((await related(ps,'facility',state.facility)).items.length,0);
  assert((await related(ps,'facility',state.facility,true)).items.length>0);
  const own=await related(ps,'point',state.point);assert(own.items.length>0);assert.equal((await related(bs,'facility',state.facility,true)).items.length,0);
 });
 await check('SC-11 shared inputs stay separate from project artifact ownership',async()=>{
  const path=pb+'/resource-access/facilities/'+state.facility,current=await call(path),add=[{kind:'point',point_id:state.point,capabilities:['read']},{kind:'point',point_id:state.second_point,capabilities:['read']},...['B','A_MINUS_B','C_MEAN'].map(key=>({kind:'channel',channel_id:state.channels[key].id,capabilities:['read']}))],input={request_id:randomUUID(),reason:'共享输入隔离验证',action:'change_grants',expected_access_revision:current.policy.revision,expected_link_revision:current.link.revision,add},preview=await call(path+'/preview',input);
  await call(path+'/changes',{...input,preview_hash:preview.preview_hash});
  const after=await call(path),oldIDs=new Set(current.grants.map(g=>g.id)),added=after.grants.filter(g=>!oldIDs.has(g.id));
  cleanups.push(async()=>{const c=await call(path),end=added.map(g=>{const now=c.grants.find(v=>v.id===g.id);return {grant_id:g.id,expected_revision:now.revision};}),change={request_id:randomUUID(),reason:'恢复项目B祖先上下文样本',action:'change_grants',expected_access_revision:c.policy.revision,expected_link_revision:c.link.revision,end},p=await call(path+'/preview',change);await call(path+'/changes',{...change,preview_hash:p.preview_hash});});
  for(const g of added.filter(g=>g.kind==='channel'||g.resource_kind==='channel')){const mp=pb+'/monitoring-access',current=await call(mp),granted=await call(mp,{...wm(current.access_revision),channel_id:g.resource_id,resource_grant_id:g.id,range:{from:'2023-12-31T00:00:00Z',to:'2024-01-03T00:00:00Z'},capabilities:['read','history']});cleanups.push(async()=>{const list=await call(mp),g=list.items.find(v=>v.id===granted.id),p=await call(mp+'/'+g.id+'/revoke-preview',{expected_revision:g.revision});await call(mp+'/'+g.id+'/revoke',{...wm(g.revision),preview_hash:p.preview_hash});});}
  assert.equal(canonical(observed(await summary(bs,'facility',state.facility),state.channels.A_MINUS_B.id)),'9');assert.equal((await related(bs,'point',state.point)).items.length,0);
  const fixed=state.expected.projectA_artifact;await call(pb+'/monitoring/derived/analysis/artifacts/'+fixed.artifact_id+'?version_id='+fixed.artifact_version_id,undefined,'GET',404);
 });
 await check('SC-10 recursive input revocation hides derived summaries and artifacts',async()=>{
  const path=pa+'/monitoring-access',grants=await call(path),grant=grants.items.find(g=>g.channel_id===state.channels.B.id&&g.effective);assert(grant);
  const preview=await call(path+'/'+grant.id+'/revoke-preview',{expected_revision:grant.revision});await call(path+'/'+grant.id+'/revoke',{...wm(grant.revision),preview_hash:preview.preview_hash});
  const restore=async()=>{const latest=await call(path);await call(path,{...wm(latest.access_revision),channel_id:grant.channel_id,resource_grant_id:grant.resource_grant_id,range:grant.range,capabilities:grant.capabilities});};cleanups.push(restore);
  assert.equal(channel(await summary(ps,'facility',state.facility),state.channels.A_MINUS_B.id),undefined);
  assert.equal(channel(await summary(ps,'component',state.component),state.channels.COMPONENT_DIFF.id),undefined);
  assert.equal((await related(ps,'point',state.point)).items.length,0);
  assert.equal(canonical(observed(await summary(bs,'facility',state.facility),state.channels.A_MINUS_B.id)),'9');
  const selected=await call(pa+'/monitoring/selections',{channel_ids:[state.channels.C_MEAN.id]},'POST',201);const denied=await client.post(api+pa+'/monitoring/queries/latest',{headers,data:{selection_id:selected.id,all_history:true,timezone:'UTC',status:'active',limit:1}});assert([403,404].includes(denied.status()),'recursive latest remained accessible: '+denied.status());
  assert.equal(canonical(observed(await summary(tenant,'facility',state.facility),state.channels.A_MINUS_B.id)),'9');
  await restore();cleanups.pop();assert.equal(canonical(observed(await summary(ps,'facility',state.facility),state.channels.A_MINUS_B.id)),'9');
 });
 await check('SC-10 losing only the original baseline range hides the derived value',async()=>{
  const path=pa+'/monitoring-access',grant=(await call(path)).items.find(g=>g.channel_id===state.channels.A.id&&g.effective);assert(grant);const p=await call(path+'/'+grant.id+'/revoke-preview',{expected_revision:grant.revision});await call(path+'/'+grant.id+'/revoke',{...wm(grant.revision),preview_hash:p.preview_hash});let narrowed;
  const restore=async()=>{if(narrowed){const g=(await call(path)).items.find(g=>g.id===narrowed.id),p=await call(path+'/'+g.id+'/revoke-preview',{expected_revision:g.revision});await call(path+'/'+g.id+'/revoke',{...wm(g.revision),preview_hash:p.preview_hash});}const current=await call(path);await call(path,{...wm(current.access_revision),channel_id:grant.channel_id,resource_grant_id:grant.resource_grant_id,range:grant.range,capabilities:grant.capabilities});};cleanups.push(restore);
  const current=await call(path);narrowed=await call(path,{...wm(current.access_revision),channel_id:grant.channel_id,resource_grant_id:grant.resource_grant_id,range:{...grant.range,from:'2024-01-01T00:15:00Z'},capabilities:grant.capabilities});
  const selected=await call(pa+'/monitoring/selections',{channel_ids:[state.channels.A.id]},'POST',201),latest=await call(pa+'/monitoring/queries/latest',{selection_id:selected.id,range:{from:'2024-01-01T00:30:00Z',to:'2024-01-01T00:31:00Z'},timezone:'UTC',status:'active',limit:1});assert.equal(canonical(latest.channels[0].last.value),'20');
  const read=()=>call(ps+'/summaries',{objects:[{kind:'point',id:state.point}],main_channels:{['point:'+state.point]:state.channels.BASELINE.id}});assert.equal(channel(await read(),state.channels.BASELINE.id),undefined);await restore();cleanups.pop();assert.equal(canonical(observed(await read(),state.channels.BASELINE.id)),'10');
 });
}catch(error){console.error(error);throw error;}finally{const cleanupErrors=[];for(const cleanup of cleanups.reverse()){try{await cleanup();}catch(error){cleanupErrors.push(String(error));}}writeFileSync(join(root,'.artifacts/spatial-scope-check.json'),JSON.stringify({results,restored:cleanupErrors.length===0,cleanupErrors,completedAt:new Date().toISOString()},null,2));await client.dispose();if(cleanupErrors.length){console.error(cleanupErrors.join('\n'));process.exitCode=1;}}
