import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
const source=new URL('../../packages/ts/api-client/src/',import.meta.url).href;
registerHooks({resolve(specifier,context,next){if(specifier.startsWith('.')&&!/\.(ts|js|mjs)$/.test(specifier)&&context.parentURL?.startsWith(source))return next(new URL(specifier+'.ts',context.parentURL).href,context);return next(specifier,context)},load(url,context,next){if(url.startsWith(source)&&url.endsWith('.ts'))return {format:'module',shortCircuit:true,source:ts.transpileModule(readFileSync(new URL(url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText};return next(url,context)}});
const {createApiClient}=await import('../../packages/ts/api-client/src/index.ts');
const tid='11111111-1111-4111-8111-111111111111',pid='22222222-2222-4222-8222-222222222222',vid='33333333-3333-4333-8333-333333333333',scope={kind:'project',tenantId:tid,projectId:pid};
const response=(value,revision='1:1:1:1:1',status=200)=>new Response(JSON.stringify(value),{status,headers:{'Content-Type':'application/json','X-Spatial-Authorization-Revision':revision}});
const deferred=()=>{let resolve;return {promise:new Promise(r=>resolve=r),resolve:v=>resolve(v)}};

test('revocation invalidates in-flight manifest, geometry, and decoded file responses',async t=>{
 const client=createApiClient(),pending=[];
 t.mock.method(globalThis,'fetch',async()=>{const d=deferred();pending.push(d);return d.promise});
 const manifest=client.spatialManifest(scope,vid),file=client.spatialFile(scope,vid,'model.glb');
 const m=assert.rejects(manifest,{name:'AbortError'}),f=assert.rejects(file,{name:'AbortError'});
 client.invalidateSpatialScope(tid,pid);pending[0].resolve(response({id:vid}));pending[1].resolve(new Response('private geometry'));
 await Promise.all([m,f]);
});
test('a newer qualification cancels older geometry and does not invalidate another project',async t=>{
 const client=createApiClient();let pending;
 t.mock.method(globalThis,'fetch',async()=>response({items:[]}));await client.spatialAssets(scope);
 const d=deferred();t.mock.method(globalThis,'fetch',async()=>d.promise);pending=client.spatialManifest(scope,vid);const stale=assert.rejects(pending,{name:'AbortError'});
 t.mock.method(globalThis,'fetch',async()=>response({items:[]},'2:1:1:1:1'));await assert.rejects(client.spatialAssets(scope),{name:'AbortError'});
 d.resolve(response({id:vid},'1:1:1:1:1'));await stale;
 const other={...scope,projectId:'44444444-4444-4444-8444-444444444444'};assert.deepEqual(await client.spatialAssets(other),{items:[]});assert.match(client.spatialRevision(tid,pid),/^2:1:1:1:1:/);
});
test('forbidden deep links do not start an invalidation/remount loop',async t=>{
 const client=createApiClient(),notices=[];client.onSpatialRestriction((...v)=>notices.push(v));
 t.mock.method(globalThis,'fetch',async()=>response({code:'spatial_not_found',message:'unavailable'},'1:1:1:1:1',404));
 for(let i=0;i<3;i++)await assert.rejects(client.spatialManifest(scope,vid),{code:'spatial_not_found'});assert.equal(notices.length,0);
});
test('permission loss drops cached qualification and propagates the real error',async t=>{
 const client=createApiClient(),notices=[];client.onSpatialRestriction((...v)=>notices.push(v));
 t.mock.method(globalThis,'fetch',async()=>response({code:'permission_denied',message:'denied'},'2:1:1:1:1',403));
 await assert.rejects(client.spatialManifest(scope,vid),{code:'permission_denied'});assert.equal(notices.length,1);assert.equal(notices[0][2],'permission_denied');
});
test('download reads a Blob and cancellation survives response decoding',async t=>{
 const client=createApiClient(),abort=new AbortController();
 t.mock.method(globalThis,'fetch',async(_url,options)=>{assert.equal(options.cache,'no-store');return new Response(new Uint8Array([0x67,0x6c,0x54,0x46]))});
 const file=await client.spatialFile(scope,vid,'model.glb');assert.ok(file instanceof Blob);assert.equal(await file.text(),'glTF');
 const d=deferred();t.mock.method(globalThis,'fetch',async()=>d.promise);const pending=client.spatialFile(scope,vid,'model.glb',abort.signal),rejected=assert.rejects(pending,{name:'AbortError'});abort.abort();d.resolve(new Response('late'));await rejected;
});
