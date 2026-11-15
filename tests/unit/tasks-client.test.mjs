import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
const source=new URL('../../packages/ts/api-client/src/',import.meta.url).href;
registerHooks({resolve(specifier,context,next){if(specifier.startsWith('.')&&!/\.(ts|js|mjs)$/.test(specifier)&&context.parentURL?.startsWith(source))return next(new URL(specifier+'.ts',context.parentURL).href,context);return next(specifier,context);},load(url,context,next){if(url.startsWith(source)&&url.endsWith('.ts'))return {format:'module',shortCircuit:true,source:ts.transpileModule(readFileSync(new URL(url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText};return next(url,context);}});
const {createApiClient}=await import('../../packages/ts/api-client/src/index.ts');
const reply=value=>new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}});
const deferred=()=>{let resolve;return {promise:new Promise(r=>resolve=r),resolve:v=>resolve(v)};};
test('task responses are discarded on identity change and cancellation',async t=>{
 const c=createApiClient(),d=deferred();t.mock.method(globalThis,'fetch',async()=>d.promise);
 const pending=c.tasks(),rejected=assert.rejects(pending,{name:'AbortError'});c.invalidateIdentity();d.resolve(reply({items:[{title:'old person'}]}));await rejected;
 const abort=new AbortController(),d2=deferred();t.mock.method(globalThis,'fetch',async()=>d2.promise);const request=c.task('id','1',abort.signal),cancelled=assert.rejects(request,{name:'AbortError'});abort.abort();d2.resolve(reply({state:'failed'}));await cancelled;
});
test('commands keep request identity, decimal revisions, and CSRF',async t=>{
 const c=createApiClient(),requests=[];t.mock.method(globalThis,'fetch',async(url,options)=>{requests.push([url,options]);return reply(String(url).endsWith('/auth/csrf')?{token:'synthetic-csrf'}:{command_state:'requested'});});
 const body={request_id:'11111111-1111-4111-8111-111111111111',action:'retry',expected_generation:'9223372036854775806',expected_control_revision:'9223372036854775805',reason:'固定输入继续执行'};
 await c.taskCommand('task',body);await c.taskCommand('task',body);const [url,options]=requests.at(-1);assert.match(url,/\/tasks\/task\/commands$/);assert.equal(options.method,'POST');assert.equal(options.cache,'no-store');assert.equal(options.headers['X-CSRF-Token'],'synthetic-csrf');assert.deepEqual(JSON.parse(options.body),body);
});
test('source outages stay explicit and filters are encoded',async t=>{
 const c=createApiClient();t.mock.method(globalThis,'fetch',async url=>{assert.match(String(url),/mode=managed/);assert.match(String(url),/q=/);const params=new URL(url,'http://local').searchParams;assert.equal(params.get('sort'),'oldest');assert.equal(params.get('type'),'monitoring.query,monitoring.export');return new Response(JSON.stringify({code:'task_source_unavailable',message:'来源暂不可用'}),{status:503,headers:{'Content-Type':'application/json'}});});await assert.rejects(c.tasks({mode:'managed',q:'任务 & 结果',sort:'oldest',type:'monitoring.query,monitoring.export'}),{code:'task_source_unavailable'});
});
