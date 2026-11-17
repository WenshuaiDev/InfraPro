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
const { facilityWorkspaceSearch, selectedTarget, resourceSelection } = await import('../../apps/console/src/features/resources/workspace-search.ts');
const { plotCoefficient } = await import('../../apps/console/src/features/monitoring/detail-format.ts');
const { resourceMethods } = await import('../../packages/ts/api-client/src/resources.ts');
const { buildTrendModel, channelAppearance } = await import('../../apps/console/src/features/monitoring/trend-model.ts');
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

test('project tree preserves server ownership and opaque pagination in the explicit project scope', async () => {
  const scope={kind:'project',tenantId:'t',projectId:'p'}, calls=[];
  const signal=new AbortController().signal;
  const items=[{kind:'channel',id:'derived',facility_id:'f',parent_kind:'component',parent_id:'c',name:'派生通道',code:'D',status:'normal'}];
  const client=resourceMethods(async(path,method,body,notify,actualSignal)=>{
    calls.push(new URL(path,'https://test.invalid'));
    assert.equal(method,'GET');assert.equal(body,undefined);assert.equal(actualSignal,signal);
    return {items,next_cursor:calls.length===1?'opaque/next+position':''};
  });
  const filter={facility_id:'f',parent_kind:'component',parent_id:'c',status:'all',limit:25};
  const first=await client.resourceTree(scope,filter,signal);
  assert.equal(first.items,items);assert.equal(first.items[0].parent_kind,'component');
  const second=await client.resourceTree(scope,{...filter,cursor:first.next_cursor},signal);
  assert.equal(second.next_cursor,'');assert.equal(calls.length,2);
  for(const url of calls){assert.equal(url.pathname,'/tenants/t/projects/p/engineering/resources/tree');assert.equal(url.searchParams.get('parent_id'),'c');assert.equal(url.searchParams.get('facility_id'),'f');}
  assert.equal(calls[1].searchParams.get('cursor'),'opaque/next+position');
});

test('derived curves retain null output gaps and differences beyond direct observation scale', () => {
 const row=(s,value)=>({source_kind:'derived',output_at:`2020-01-01T00:00:0${s}Z`,value});
 const a=row(0,'123456789012345678.000000000000000001'), gap=row(1,null), b=row(2,'123456789012345678.000000000000000002');
 assert.deepEqual(plotObservations({summary:false,buckets:[{points:[b,a,gap],discontinuous:false}]}),[a,gap,b]);
 const baseline=plotBaseline(a,18);
 assert.equal(Number(plotCoefficient(b.value,18)-baseline)/1e18,1e-18);
 assert.equal(plotBaseline(gap,18),0n);
});

test('facility and component channel deep links retain their identity without inventing a point', () => {
 const facility='aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa', channel='bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb';
 for(const parent_kind of ['facility','component']) {
  const selection=resourceSelection({kind:'channel',id:channel,parent_kind,parent_id:facility,deleted_at:null});
  assert.equal(selection.point,undefined);
  const search=facilityWorkspaceSearch(selection);
  assert.equal(search.channel,channel);
  assert.deepEqual(selectedTarget(facility,search),{kind:'channel',facility,point:undefined,id:channel});
 }
});


const comparisonRange = { from: '2020-01-01T00:00:00Z', to: '2020-01-02T00:00:00Z' };
const comparisonChannel = (id, patch = {}) => ({ id, name: `通道 ${id}`, source_kind: 'direct', unit: 'mm', value_kind: 'decimal', owner_name: '测点', facility_name: '设施', observation_direction: '向下为正', reference_basis: '初始观测', timestamp_meaning: 'observation_time', ...patch });
const comparisonSeries = (channel, values = ['1', '3'], patch = {}) => ({ channel_id: channel.id, range: comparisonRange, summary: false, buckets: [{ range: comparisonRange, discontinuous: false, states: [], points: values.map((value, i) => ({ source_kind: channel.source_kind, value, value_kind: channel.value_kind, quality: 'good', data_quality: 'good', calculation_status: value === null ? 'no_result' : 'valid', observed_at: `2020-01-01T00:00:0${i}Z`, output_at: `2020-01-01T00:00:0${i}Z` })) }], ...patch });

test('every selected channel is represented in one plot up to fifty, regardless of metadata or missing samples', () => {
  const channels = Array.from({ length: 50 }, (_, i) => comparisonChannel(`channel-${i}`, { metric_version_id: `metric-${i}`, source_kind: i % 2 ? 'derived' : 'direct', reference_basis: `${i}`, timestamp_meaning: i % 3 ? 'observation_time' : 'interval_end' }));
  const model = buildTrendModel(channels, channels.slice(0, 49).map(channel => comparisonSeries(channel)), comparisonRange);
  assert.deepEqual(model.lines.map(line => line.id), channels.map(channel => channel.id));
  assert.equal(model.axes.length, 1);
  assert.ok(model.lines.slice(0, 49).every(line => line.data.length === 2));
  assert.deepEqual(model.lines[49].data, []); // no fabricated zero for the empty channel
  assert.equal(model.lines[1].data[0].name.includes('区间结束时刻'), false);
  assert.ok(model.lines[3].data[0].name.includes('区间结束时刻'));
  const before = channels.map(channelAppearance);
  assert.deepEqual(channels.slice(2).map(channelAppearance), before.slice(2));
  assert.equal(channelAppearance(channels[1]).lineType, 'dashed');
});

test('same-unit series share an axis while other units and states remain on the same time plot', () => {
  const channels = [comparisonChannel('direct'), comparisonChannel('derived', { source_kind: 'derived' }), comparisonChannel('temperature', { unit: '°C' }), comparisonChannel('state', { unit: '', value_kind: 'enum' })];
  const series = channels.map(channel => comparisonSeries(channel, channel.value_kind === 'enum' ? ['yes', 'no'] : ['2', '4']));
  const model = buildTrendModel(channels, series, comparisonRange);
  assert.deepEqual(model.lines.map(line => line.axisIndex), [0, 0, 1, 2]);
  assert.deepEqual(model.axes.map(axis => axis.name), ['mm', '°C', '状态']);
  assert.deepEqual(model.lines[3].data.map(point => point.value[1]), ['yes', 'no']);
  assert.deepEqual(model.axes[2].states, ['no', 'yes']);
  const relative = buildTrendModel(channels, series, comparisonRange, 'relative');
  assert.deepEqual(relative.lines.map(line => line.axisIndex), [0, 0, 0, 1]);
  assert.deepEqual(relative.lines[0].data.map(point => point.value[1]), [0, 100]);
  assert.match(relative.lines[0].data[0].name, /原值 2 mm/);
});

test('mixed-unit precision, derived null outputs and summary discontinuities survive comparison', () => {
  const channels = [comparisonChannel('large', { source_kind: 'derived' }), comparisonChannel('ordinary', { unit: '°C' })];
  const series = [comparisonSeries(channels[0], ['123456789012345678.000000000000000001', null, '123456789012345678.000000000000000002']), comparisonSeries(channels[1], ['20', '21'])];
  const model = buildTrendModel(channels, series, comparisonRange);
  assert.deepEqual(model.lines[0].data.map(point => point.value[1]), [0, null, 1e-18]);
  assert.deepEqual(model.lines[1].data.map(point => point.value[1]), [20, 21]);
  assert.match(model.lines[0].data[2].name, /123456789012345678.000000000000000002/);
  const summary = buildTrendModel([channels[1]], [comparisonSeries(channels[1], ['20', '21'], { summary: true })], comparisonRange);
  assert.ok(summary.lines[0].data.every((point, i, points) => point.value[1] === null || i === 0 || points[i - 1].value[1] === null));
});

test('relative amplitude uses only the current window, keeps constant signals and never fills missing data', () => {
  const channel = comparisonChannel('relative');
  const series = comparisonSeries(channel, ['-999', '5', null, '15', '999']);
  const range = { from: '2020-01-01T00:00:01Z', to: '2020-01-01T00:00:04Z' };
  const model = buildTrendModel([channel], [series], range, 'relative');
  assert.equal(model.lines[0].data[1].value[1], 0);
  assert.equal(model.lines[0].data[2].value[1], null);
  assert.equal(model.lines[0].data[3].value[1], 100);
  const constant = buildTrendModel([channel], [comparisonSeries(channel, ['5', '5'])], comparisonRange, 'relative');
  assert.deepEqual(constant.lines[0].data.map(point => point.value[1]), [50, 50]);
});
