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
test('account switch and caller cancellation discard late inbox payloads',async t=>{
 const c=createApiClient(),d=deferred();t.mock.method(globalThis,'fetch',async()=>d.promise);
 const pending=c.notificationSummary(),rejected=assert.rejects(pending,{name:'AbortError'});c.invalidateIdentity();d.resolve(reply({unread:9}));await rejected;
 const abort=new AbortController(),d2=deferred();t.mock.method(globalThis,'fetch',async()=>d2.promise);const list=c.notifications({},abort.signal),cancelled=assert.rejects(list,{name:'AbortError'});abort.abort();d2.resolve(reply({items:[{body:'old account'}]}));await cancelled;
});
test('notification writes use original CSRF protocol and structured watermark',async t=>{
 const c=createApiClient(),requests=[];t.mock.method(globalThis,'fetch',async(url,options)=>{requests.push([url,options]);return reply(String(url).endsWith('/auth/csrf')?{token:'synthetic-csrf'}:{changed:2});});
 await c.readNotifications({category:'tasks',scope:'tenant'},'9223372036854775806');const [url,options]=requests.at(-1);assert.match(url,/\/me\/notifications\/read-all$/);assert.equal(options.method,'POST');assert.equal(options.cache,'no-store');assert.equal(options.headers['X-CSRF-Token'],'synthetic-csrf');assert.deepEqual(JSON.parse(options.body),{filter:{category:'tasks',scope:'tenant'},watermark:'9223372036854775806'});
});
test('failed summaries remain errors rather than becoming zero',async t=>{
 const c=createApiClient();t.mock.method(globalThis,'fetch',async()=>new Response(JSON.stringify({code:'unavailable',message:'synthetic outage'}),{status:503,headers:{'Content-Type':'application/json'}}));await assert.rejects(c.notificationSummary(),{code:'unavailable'});
});
