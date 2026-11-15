import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
const root=new URL('../../',import.meta.url).href;
registerHooks({resolve(s,c,next){if(s.startsWith('.')&&!/\.(ts|js|mjs)$/.test(s)&&c.parentURL?.startsWith(root)&&c.parentURL.endsWith('.ts'))return next(new URL(s+'.ts',c.parentURL).href,c);return next(s,c)},load(url,c,next){if(url.startsWith(root)&&url.endsWith('.ts'))return{format:'module',shortCircuit:true,source:ts.transpileModule(readFileSync(new URL(url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText};return next(url,c)}});
const {createApiClient}=await import('../../packages/ts/api-client/src/index.ts');
const {auditMethods,auditScopePath}=await import('../../packages/ts/api-client/src/audit.ts');
const {auditSearch,auditFilter}=await import('../../apps/console/src/features/audit/search.ts');
const {auditDestination}=await import('../../apps/console/src/features/audit/targets.ts');
const tenant='11111111-1111-4111-8111-111111111111',project='22222222-2222-4222-8222-222222222222',id='33333333-3333-4333-8333-333333333333';
const json=(value,status=200)=>new Response(JSON.stringify(value),{status,headers:{'Content-Type':'application/json'}});
function intercept(t,handler){const prior=globalThis.fetch;globalThis.fetch=(url,options)=>String(url).endsWith('/auth/csrf')?Promise.resolve(json({token:'synthetic-csrf'})):handler(String(url),options);t.after(()=>{globalThis.fetch=prior;});}

test('lost transport and malformed success keep the same native command; definitive conflict releases it',async t=>{
 const keys=[];let mode='lost';intercept(t,async(_url,o)=>{keys.push(o.headers['X-Command-ID']);if(mode==='lost')throw Error('reply lost');if(mode==='malformed')return new Response('{');if(mode==='conflict')return json({code:'version_conflict',message:'changed'},409);return json({id});});
 const c=createApiClient(),input={name:'角色',description:'',reason:'真实意图'};
 await assert.rejects(c.createRole(input));mode='malformed';await assert.rejects(c.createRole(input));mode='conflict';await assert.rejects(c.createRole(input));assert.equal(new Set(keys).size,1);
 mode='ok';await c.createRole(input);assert.notEqual(keys[2],keys[3]);
});
test('128 unresolved commands have a finite cap and identity changes release no old response',async t=>{
 let calls=0;intercept(t,async()=>{calls++;throw Error('unknown');});const c=createApiClient();
 for(let i=0;i<128;i++)await assert.rejects(c.createRole({name:String(i),reason:'pending'}));
 await assert.rejects(c.createRole({name:'129',reason:'pending'}),{code:'command_reconciliation_required'});assert.equal(calls,128);
 c.invalidateIdentity();await assert.rejects(c.createRole({name:'new account',reason:'pending'}));assert.equal(calls,129);
});
test('audit metadata and blobs are no-store; identity changes reject late data',async t=>{
 let release,observed;intercept(t,async(_url,o)=>{observed=o;return new Promise(r=>{release=r;});});const c=createApiClient(),pending=c.auditEvents({kind:'platform'});
 c.invalidateIdentity();release(json({items:[{id:'1'}]}));await assert.rejects(pending,{name:'AbortError'});assert.equal(observed.cache,'no-store');
 const controller=new AbortController(),blob=c.downloadAuditExport({kind:'platform'},id,'9223372036854775806',controller.signal);controller.abort();release(new Response('private csv'));await assert.rejects(blob,{name:'AbortError'});
});
test('scope paths, immutable generations and explicit export command headers are preserved',async()=>{
 const calls=[],methods=auditMethods(async(...args)=>{calls.push(args);return{};}),scope={kind:'project',tenant_id:tenant,project_id:project},signal=new AbortController().signal;
 assert.equal(auditScopePath({kind:'platform'}),'/audit');assert.equal(auditScopePath(scope),`/tenants/${tenant}/projects/${project}/audit`);
 await methods.createAuditExport(scope,{reason:'测试'},id,signal);assert.equal(calls[0][5].headers['X-Command-ID'],id);assert.equal(calls[0][4],signal);
 await methods.downloadAuditExport(scope,id,'9223372036854775806',signal);assert.match(calls[1][0],/generations\/9223372036854775806\/download$/);assert.equal(calls[1][5].responseType,'blob');
});
test('search admits finite fields and targets reject arbitrary routes, malformed ownership and missing generations',()=>{
 const parsed=auditSearch({execution:'9223372036854775806',object_id:id,event:'17',url:'https://outside.invalid',export_id:project,tab:'exports'});
 assert.equal(parsed.execution,'9223372036854775806');assert.equal(auditSearch({execution:'9223372036854775808'}).execution,undefined);assert.equal(parsed.url,undefined);assert.deepEqual(auditFilter(parsed),{object_id:id});
 const s={kind:'project',tenant_id:tenant,project_id:project};assert.equal(auditDestination({available:true,kind:'https://outside.invalid',scope:s,id}),undefined);assert.equal(auditDestination({available:true,kind:'task',scope:s,id}),undefined);
 assert.deepEqual(auditDestination({available:true,kind:'task',scope:s,id,generation:'9223372036854775806'}),{to:`/tasks/${id}`,search:{execution:'9223372036854775806'}});
 assert.equal(auditDestination({available:true,kind:'device',scope:{...s,tenant_id:'../../outside'},id}),undefined);
 assert.deepEqual(auditDestination({available:true,kind:'file_attachment',scope:s,id,revision:project}),{to:`/t/${tenant}/p/${project}/files`,search:{attachment:id,revision:project}});
 assert.deepEqual(auditDestination({available:true,kind:'display_package',scope:s,id,parameters:{instance:tenant}}),{to:`/t/${tenant}/p/${project}/displays`,search:{instance:tenant,package:id}});
});
test('registered audit source destinations resolve to existing console routes',()=>{
 const routes=[...readFileSync(new URL('../../apps/console/src/routeTree.gen.ts',import.meta.url),'utf8').matchAll(/fullPath: '([^']+)'/g)].map(([,path])=>new RegExp('^'+path.replace(/\/$/,'').split('/').map(part=>part.startsWith('$')?'[^/]+':part).join('/')+'/?$'));
 const groups=[
  [{kind:'platform'},['account','role','tenant','engineering_definition','device_product','device_manufacturer','task','audit_export']],
  [{kind:'tenant',tenant_id:tenant},['tenant','tenant_role','tenant_member','tenant_invitation','project','engineering_definition','engineering_facility','engineering_component','engineering_point','engineering_channel','device_product','device_manufacturer','device','device_output','device_installation','project_device_grant_set','project_resource_policy','monitoring_batch','monitoring_job','monitoring_export','derived_channel','derived_run','analysis_artifact','analysis_export','derived_evidence_export','spatial_asset','spatial_version','spatial_access','file_attachment','file_upload']],
  [{kind:'project',tenant_id:tenant,project_id:project},['project','project_role','project_member','monitoring_batch','monitoring_job','derived_channel','derived_run','analysis_artifact','spatial_asset','spatial_version','display_instance','display_package','file_attachment','file_upload']],
 ];
 for(const[scope,kinds]of groups)for(const kind of kinds){const target=auditDestination({available:true,scope,kind,id,generation:'1',parameters:{kind:'units',facility:tenant,point:project,channel:id,device:id,project,instance:tenant}});assert.ok(target,kind);assert.ok(routes.some(route=>route.test(target.to)),`${scope.kind}/${kind}: ${target.to} is not a registered route`);}
});

test('audit filters keep Shanghai wall times independent of browser timezone and preserve exact bounds',async()=>{
 const {shanghaiInput,shanghaiISO,recentFilter,validateFilter}=await import('../../apps/console/src/features/audit/filter-state.ts');
 assert.equal(shanghaiInput('2026-11-15T10:49:00Z'),'2026-11-15T18:49:00');
 assert.equal(shanghaiISO('2026-11-15T18:49:00'),'2026-11-15T10:49:00.000Z');
 assert.equal(shanghaiInput('2026-11-15T02:49:00-08:00'),'2026-11-15T18:49:00');
 assert.equal(shanghaiISO(''),undefined);
 assert.equal(shanghaiInput('malformed'),'');
 const now=Date.parse('2026-11-15T10:49:00Z');
 const next=recentFilter({object_id:id,actor_id:tenant,cursor:'old',limit:25},7,now);
 assert.equal(next.from,'2026-11-08T10:49:00.000Z');assert.equal(next.to,'2026-11-15T10:49:00.000Z');assert.equal(next.cursor,undefined);assert.equal(next.object_id,id);assert.equal(next.actor_id,tenant);assert.equal(next.limit,25);
 assert.equal(validateFilter(next,now),undefined);
 assert.match(validateFilter({from:next.to,to:next.from},now),/结束时间/);
 assert.match(validateFilter({from:'2025-01-01T00:00:00Z',to:next.to},now),/366/);
 assert.match(validateFilter({actor_id:'bad'},now),/UUID/);
 assert.match(validateFilter({task_id:id},now),/模块、类型和代次/);
 assert.equal(validateFilter({task_id:id,task_module:'audit',task_kind:'export',generation:'1'},now),undefined);
 assert.match(validateFilter({generation:'9223372036854775808'},now),/正整数/);
});
