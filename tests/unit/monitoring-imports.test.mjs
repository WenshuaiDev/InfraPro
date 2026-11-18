import test from 'node:test';
import assert from 'node:assert/strict';
import { importSearch, importFilter, batchCapabilities } from '../../apps/console/src/features/monitoring/import-search.ts';
import { monitoringMethods } from '../../packages/ts/api-client/src/monitoring.ts';

test('bookmark inputs normalize multi-select filters and keep task and drawer state out of list requests', () => {
 const job = '0eeaf420-46b3-4081-86e0-1afd640648cd';
 const input = {q:'  桥梁\n ',states:'ready,published,ready,unknown',formats:'xlsx,csv,pdf',limit:'20',sort:'oldest',tab:'issues',cursor:'valid-position'};
 const filter = {q:'桥梁',states:'published,ready',formats:'csv,xlsx',limit:20,sort:'oldest',cursor:'valid-position'};
 const search = importSearch({...input, job, execution:'2'});
 assert.deepEqual(search, {...filter, tab:'issues', job, execution:'2'});
 assert.deepEqual(importFilter(search), filter);
 assert.deepEqual(importFilter(importSearch(input)), filter);
 assert.equal(importSearch({limit:100000,sort:'id',tab:'arbitrary'}).limit,10);
 assert.equal(importSearch({q:'界'.repeat(201)}).q.length,200);
});
test('partial, archived and source-denied batches never inherit write/download affordances', () => {
 const batch={state:'ready',mapping_revision:'1',source_file_id:'source',validation_hash:'hash',counts:{errors:'0',duplicates:'2'}};
 for(const state of ['uploading','received','needs_resolution','ready','published','revoked']){
  const partial=batchCapabilities({...batch,state,scope_clipped:true},true,true,true,true);
  assert.ok(Object.values(partial).every(v=>!v));
  const unavailable=batchCapabilities({...batch,state},true,true,true,false);assert.ok(Object.values(unavailable).every(v=>!v));
  const archived=batchCapabilities({...batch,state},false,false,true,true);assert.equal(archived.source,true);assert.ok(Object.entries(archived).filter(([k])=>k!=='source').every(([,v])=>!v));
 }
 assert.equal(batchCapabilities(batch,true,true,false,true).source,false);
 assert.equal(batchCapabilities({...batch,state:'needs_resolution',counts:{errors:'1',duplicates:'2'}},true,true,true,true).resolve,false);
 assert.equal(batchCapabilities({...batch,readopt_batch_id:'original'},true,true,true,true).mapping,false);
});
test('tenant and project list requests carry exact encoded search, opaque cursor and page size', async()=>{
 const calls=[];const controller=new AbortController();
 const client=monitoringMethods(async(...args)=>{calls.push(args);return {items:[],next_cursor:''};});
 await client.monitoringImportList({kind:'tenant',tenantId:'tenant'}, {q:'桥_%&',states:'ready,failed',formats:'xlsx',limit:20,cursor:'opaque:cursor'},controller.signal);
 await client.monitoringImportList({kind:'project',tenantId:'tenant',projectId:'project'}, {sort:'oldest',limit:10});
 const first=new URL('http://local'+calls[0][0]);assert.equal(first.pathname,'/tenants/tenant/monitoring/imports');assert.equal(first.searchParams.get('q'),'桥_%&');assert.equal(first.searchParams.get('states'),'ready,failed');assert.equal(first.searchParams.get('limit'),'20');assert.equal(first.searchParams.get('cursor'),'opaque:cursor');assert.ok(calls[0].includes(controller.signal));
 assert.ok(calls[1][0].startsWith('/tenants/tenant/projects/project/monitoring/imports?'));
});
