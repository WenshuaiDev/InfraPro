import test from 'node:test';
import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { extname } from 'node:path';
import { decimalCoefficient, formatNS, MAX_NS, MIN_NS, parseNS } from '../../apps/console/src/features/monitoring/format.ts';

// The app uses bundler-style extensionless imports; resolve only its pure TS
// modules here so these regressions exercise production code in Node.
const source = new URL('../../apps/console/src/', import.meta.url).href;
const resolver = registerHooks({ resolve(specifier, context, nextResolve) {
  if (specifier.startsWith('.') && !extname(specifier) && context.parentURL?.startsWith(source)) return nextResolve(new URL(`${specifier}.ts`, context.parentURL).href, context);
  return nextResolve(specifier, context);
} });
const { latestRange, observedAge, selectedQualities } = await import('../../apps/console/src/features/monitoring/workspace-model.ts');
const { plotObservations, plotBaseline } = await import('../../apps/console/src/features/monitoring/trend-points.ts');
const { projectTreePage } = await import('../../apps/console/src/features/resources/project-tree-query.ts');
resolver.deregister();

test('live window includes the newest nanosecond, clamps bounds and reports stale/future observations honestly', () => {
  const at = '2026-11-12T05:00:00.000000001Z', range = latestRange(at, 60);
  assert.equal(parseNS(range.to) - parseNS(at), 1n);
  assert.equal(parseNS(at) - parseNS(range.from), 3600000000000n);
  assert.equal(latestRange(formatNS(MIN_NS), 60).from, formatNS(MIN_NS));
  assert.equal(latestRange(formatNS(MAX_NS), 60).to, null);
  assert.equal(observedAge(at, Date.parse('2026-11-12T06:30:00Z')), '1 小时前观测');
  assert.equal(observedAge(at, Date.parse('2026-11-12T04:00:00Z')), '观测时间晚于当前时间');
  assert.deepEqual(selectedQualities('all'), ['good', 'unmarked', 'suspect', 'bad']);
});

test('numeric curves sort actual observations, break at known gaps and never invent zero values', () => {
  const p = (second, value) => ({ observed_at: `2026-11-12T00:00:0${second}Z`, value });
  const a=p(0,'0'), b=p(1,'1'), c=p(3,'3'), d=p(4,'4');
  const raw={summary:false,buckets:[{points:[b,a],discontinuous:false},{points:[d,c],discontinuous:true}]};
  assert.deepEqual(plotObservations(raw), [a,b,null,null,c,null,d]);
  assert.deepEqual(plotObservations({...raw,summary:true}).filter(Boolean), [a,b,c,d]);
  assert.ok(plotObservations({...raw,summary:true}).every((point,i,items)=>!point||i===0||items[i-1]===null));
  assert.deepEqual(plotObservations({summary:false,buckets:[{points:[],discontinuous:false}]}), []);
  const adjacent = { summary: false, buckets: [
    { points: [a,b], discontinuous: false, range: { from: a.observed_at, to: c.observed_at } },
    { points: [c,d], discontinuous: false, range: { from: c.observed_at, to: '2026-11-12T00:00:05Z' } },
  ] };
  assert.deepEqual(plotObservations(adjacent), [a,b,c,d]);
  adjacent.buckets[1].range.from = d.observed_at;
  assert.deepEqual(plotObservations(adjacent), [a,b,null,c,d]);
});

test('ordinary values keep physical axes while huge values preserve a one-trillionth difference', () => {
  assert.equal(plotBaseline({value:'-1.860000000000'}),0n);
  const baseline=plotBaseline({value:'999999999999999999.000000000001'});
  assert.equal(Number(decimalCoefficient('999999999999999999.000000000002')-baseline)/1e12,1e-12);
  assert.equal(plotBaseline(),0n);
});

test('project tree retains component ownership and independently paginates only project-scoped metadata', async () => {
  const scope={kind:'project',tenantId:'t',projectId:'p'}, calls=[];
  const client={async resourceList(actual, target, filter) {
    calls.push({scope:actual,target,filter});
    assert.deepEqual(actual,scope);
    if(target.kind==='component')return {items:[{kind:'component',id:'c',facility_id:'f',parent_component_id:null,name:'主梁',code:'C',status:'active'}],next_cursor:''};
    return {items:[{kind:'point',id:filter.cursor?'p2':'p1',facility_id:'f',component_id:null,name:'测点',code:'P',status:'active'}],next_cursor:filter.cursor?'':'next-point'};
  }};
  const filter={parent_kind:'facility',parent_id:'f',facility_id:'f'}, signal=new AbortController().signal;
  const first=await projectTreePage({client,scope},filter,signal);
  assert.equal(first.items[0].parent_id,'f');assert.equal(first.items[1].parent_kind,'facility');
  assert.equal(calls[0].filter.parent_id,'root');assert.equal(calls[1].filter.owner,'root');
  const second=await projectTreePage({client,scope},{...filter,cursor:first.next_cursor},signal);
  assert.equal(calls.length,3);assert.equal(calls[2].filter.cursor,'next-point');assert.equal(second.next_cursor,'');
  assert.equal(second.items[0].id,'p2');
  calls.length=0;
  await projectTreePage({client,scope},{parent_kind:'component',parent_id:'c',facility_id:'f'},signal);
  assert.equal(calls[0].filter.parent_id,'c');assert.equal(calls[1].filter.owner,'c');
});
