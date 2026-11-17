import test from 'node:test';
import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
const source = new URL('../../packages/ts/api-client/src/', import.meta.url).href;
registerHooks({ resolve(specifier, context, next) { if (specifier.startsWith('.') && !/\.(ts|js|mjs)$/.test(specifier) && context.parentURL?.startsWith(source)) return next(new URL(specifier + '.ts', context.parentURL).href, context); return next(specifier, context); }, load(url, context, next) { if (url.startsWith(source) && url.endsWith('.ts')) return { format: 'module', shortCircuit: true, source: ts.transpileModule(readFileSync(new URL(url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText }; return next(url, context); } });
const { createApiClient } = await import('../../packages/ts/api-client/src/index.ts');
const modelCode = ts.transpileModule(readFileSync(new URL('../../apps/console/src/features/bindings/model.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { bindingCommand, bindingSearch } = await import('data:text/javascript;base64,' + Buffer.from(modelCode).toString('base64'));
const treeCode = ts.transpileModule(readFileSync(new URL('../../apps/console/src/features/bindings/tree-model.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { bindingTree, branchContains } = await import('data:text/javascript;base64,' + Buffer.from(treeCode).toString('base64'));
const response = value => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } });
const tenant = '11111111-1111-4111-8111-111111111111';

test('directory encodes filters and discards responses when device scope changes', async t => {
  const c = createApiClient();
  let release;
  t.mock.method(globalThis, 'fetch', async url => {
    const parsed = new URL(url, 'http://local');
    assert.equal(parsed.pathname, `/api/v1/tenants/${tenant}/monitoring-source-bindings`);
    assert.equal(parsed.searchParams.get('q'), '桥 & 位移');
    assert.equal(parsed.searchParams.get('state'), 'unbound');
    assert.equal(parsed.searchParams.get('channel_id'), tenant);
    return new Promise(resolve => { release = resolve; });
  });
  const pending = c.deviceBindingDirectory({ kind: 'tenant', tenantId: tenant }, { q: '桥 & 位移', state: 'unbound', channel_id: tenant });
  const rejected = assert.rejects(pending, { name: 'AbortError' });
  c.invalidateDeviceScope(tenant);
  release(response({ items: [{ name: 'stale restricted row' }], next_cursor: '' }));
  await rejected;
});

test('directory permission loss reaches the device boundary', async t => {
  const c = createApiClient(); let notice;
  c.onDeviceRestriction((tenantId, project, code) => { notice = { tenantId, project, code }; });
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({ code: 'permission_denied', message: '无权查看' }), { status: 403 }));
  await assert.rejects(c.deviceBindingDirectory({ kind: 'tenant', tenantId: tenant }), { code: 'permission_denied' });
  assert.deepEqual(notice, { tenantId: tenant, project: undefined, code: 'permission_denied' });
});

test('binding commands preserve both endpoint revisions and never bypass restricted sources', () => {
  const current = { actions: { bind: false, switch: true, unbind: true }, source_revision: '9223372036854775806', channel_revision: '3', source: { configured: true, visibility: 'full', id: 'binding', revision: '7', output: { revision: '4', device_revision: '9' } } };
  const next = { id: 'next', revision: '12', device_revision: '13' };
  const command = bindingCommand({ kind: 'tenant', tenantId: tenant }, 'switch', current, '  校正接线  ', next);
  assert.equal(command.expected_source_revision, current.source_revision);
  assert.equal(command.expected_old_output_revision, '4');
  assert.equal(command.expected_new_output_revision, '12');
  assert.equal(command.expected_binding_id, 'binding');
  assert.equal(command.reason, '校正接线');
  assert.equal(bindingCommand({ kind: 'tenant', tenantId: tenant }, 'bind', current, '无越权', next), undefined);
  assert.equal(bindingCommand({ kind: 'tenant', tenantId: tenant }, 'switch', { ...current, source: { configured: true, visibility: 'restricted' } }, '无越权', next), undefined);
  assert.equal(bindingCommand({ kind: 'tenant', tenantId: tenant }, 'switch', current, '必须选设备'), undefined);
  const unbind = bindingCommand({ kind: 'tenant', tenantId: tenant }, 'unbind', current, '解除');
  assert.equal(unbind.output_id, undefined);
  assert.equal(unbind.expected_old_device_revision, '9');
});

test('binding links reject invalid selections and prototype state', () => {
  const search = bindingSearch({ facility: tenant, channel: 'not-an-id', state: 'constructor', variant: 'C', q: 'a'.repeat(300) });
  assert.equal(search.facility, tenant); assert.equal(search.channel, undefined); assert.equal(search.state, undefined); assert.equal(search.variant, undefined); assert.equal(search.q.length, 200);
});

test('binding tree merges cursor pages by identity and retains nested and facility-owned points', () => {
  const first = { facility_id: 'bridge', facility_name: '同名设施', facility_code: 'B', point_id: 'point1', point_name: '同名测点', point_code: 'P1', channel_id: 'ch1', name: '位移', component_path: [{ id: 'span', name: '主跨', code: 'S' }, { id: 'beam', name: '主梁', code: 'B' }] };
  const second = { ...first, channel_id: 'ch2', name: '温度' };
  const direct = { ...first, point_id: 'point2', point_code: 'P2', channel_id: 'ch3', component_path: [] };
  const other = { ...direct, facility_id: 'tunnel', facility_code: 'T', point_id: 'point3', channel_id: 'ch4' };
  const tree = bindingTree([first, second, first, direct, other]);
  assert.equal(tree.length, 2);
  assert.equal(tree[0].count, 3);
  const point = tree[0].children[0].children[0].children[0];
  assert.equal(point.name, '同名测点');
  assert.deepEqual(point.channels.map(row => row.channel_id), ['ch1', 'ch2']);
  assert.equal(tree[0].children[1].kind, 'direct');
  assert.equal(tree[0].children[1].children[0].channels[0].channel_id, 'ch3');
  assert.equal(tree[1].count, 1);
  assert.equal(branchContains(tree[0], 'ch2'), true);
  assert.equal(branchContains(tree[0], 'ch4'), false);
});
