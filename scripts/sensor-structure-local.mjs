import { requireAuthenticatedLogin } from './login-result.mjs';
import {readFileSync,writeFileSync,existsSync,mkdirSync} from 'node:fs';
import {join} from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {request,expect as baseExpect} from '@playwright/test';
import {readJSON,directory,origin} from './local-runtime.mjs';
import {seedSensorStructure} from './fixtures/sensor-structure-acceptance.mjs';

// Add a separate synthetic acceptance tenant to the existing local runtime.
// Preserve the existing account and business data. Never replace a dictionary.
const expect=baseExpect.configure({timeout:30000}),api=origin+'/api/v1',file=join(directory,'engineering/sensor-structure-acceptance.json'),client=await request.newContext();
try{
 const csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:origin,'X-CSRF-Token':csrf.token},account=readJSON('account.json');
 async function call(path,data,method=data===undefined?'GET':'POST'){const r=await client.fetch(api+path,{method,headers,data,timeout:120000});if(!r.ok())throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?{}:r.json();}
 async function wait(path,pick,want){let data;await expect.poll(async()=>{data=await call(path);return pick(data)},{timeout:120000}).toBe(want);return data;}
 const save=state=>{mkdirSync(join(directory,'engineering'),{recursive:true,mode:0o700});writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});};
 requireAuthenticatedLogin(await call('/auth/login',{username:account.username,password:account.password}));let state;
 if(existsSync(file))state=JSON.parse(readFileSync(file,'utf8'));
 else{
  const dictionary=await call('/platform/business-dictionaries/device-category');if(dictionary.current_release_id||dictionary.draft.nodes.length)throw Error('Existing classification dictionary requires explicit fixture mapping; nothing was overwritten.');
  const owner=(await call('/access/accounts?q='+encodeURIComponent(account.username))).items.find(a=>a.username===account.username);
  state=await seedSensorStructure({call,client,api,headers,accountID:owner.id,wait});save(state);
 }
 const wm=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'完整方案本地合成验收'}),displays=`/tenants/${state.tenant}/projects/${state.project}/displays`;
 if(!state.display){
  const settings=(await call(displays)).settings;await call(displays+'/settings',{...wm(settings.revision),enabled:true},'PUT');
  const created=await call(displays+'/instances',{...wm(),name:'工程结构与设备分类 · SBM 验收'});state.display=created.id;save(state);
 }
 const path=displays+'/instances/'+state.display,command=async(action,body,method='POST')=>{const d=await call(path);return call(path+action,{...wm(d.instance.revision),...body},method);};
 if(!state.display_published){
  let d=await call(path);
  if(!d.packages.some(p=>p.state==='ready')){const zip=readFileSync('.artifacts/display/engineering-structure-example.zip'),sha256=createHash('sha256').update(zip).digest('hex'),upload=await command('/uploads',{size:zip.length,sha256});const response=await client.put(api+path+`/uploads/${upload.upload_id}/parts/0`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':sha256},data:zip});expect(response.ok()).toBe(true);await command(`/uploads/${upload.upload_id}/complete`,{});d=await wait(path,v=>v.packages[0]?.state,'ready');}
  const definitions=[['project_resources','resources',{kind:'project'},['resource.metadata','resource.attributes','resource.position','statistics.counts']],['project_raw','channels',{kind:'ids',ids:Object.values(state.channels).map(v=>v.id)},['resource.metadata','source.binding','monitoring.values','monitoring.quality','statistics.counts']],['project_devices','devices',{kind:'project'},['device.metadata','device.outputs','device.installations','device.classification','statistics.counts','statistics.classification']]],scopes=definitions.map(([id,domain,selector,field_groups])=>({id,structure_model:2,domain,selector,field_groups,include_future:false})),views=Object.fromEntries(scopes.map(({id,...s})=>[id,{...s,scope_id:id}])),manifest=JSON.parse(readFileSync('examples/display-engineering-structure/display.manifest.json','utf8'));
  const capabilities=[...Object.keys(manifest.required_capabilities),...Object.keys(manifest.optional_capabilities)].filter(c=>!c.startsWith('spatial.'));state.display_valid_until=new Date(Date.now()+30*86400000).toISOString();
  await command('/grant',{grant:{scope_format:2,read_scopes:scopes,capabilities,valid_until:state.display_valid_until}},'PUT');
  const config=await command('/configs',{package_id:d.packages.find(p=>p.state==='ready').id,config_format:2,read_views:views,parameters:{title:'设备分类与工程结构 · 合成验收'},bindings:{},mode:'follow'}),prepared=await command('/prepare',{config_id:config.config_id});await command('/publish',{config_id:config.config_id,preparation:prepared.hash});state.display_published=true;save(state);
 }
 console.log(JSON.stringify({synthetic:true,tenant:state.tenant,project:state.project,facility:origin+`/t/${state.tenant}/resources/facilities/${state.nodes.A.id}`,devices:origin+`/t/${state.tenant}/devices`,display:'http://localhost:25180/display/'+state.display,display_valid_until:state.display_valid_until},null,2));
}finally{await client.dispose();}
