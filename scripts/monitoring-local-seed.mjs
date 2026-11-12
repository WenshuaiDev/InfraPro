import { createHash, randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { request } from '@playwright/test';

// Only this explicit local fixture command creates example business resources.
export async function seed(directory) {
  const stateFile = join(directory, 'acceptance.json');
  const state = existsSync(stateFile) ? JSON.parse(readFileSync(stateFile, 'utf8')) : { origin: 'synthetic; no real tenant data', steps: {}, batches: {} };
  if (state.complete) { console.log(`合成验收资料已存在：${stateFile}`); return; }
  const save = () => writeFileSync(stateFile, JSON.stringify(state, null, 2) + '\n', { mode: 0o600 });
  const once = async (key, work) => { if (!(key in state.steps)) { state.steps[key] = await work(); save(); } return state.steps[key]; };
  const user = JSON.parse(readFileSync(join(directory, 'login.json'), 'utf8'));
  const origin = 'http://127.0.0.1:15273', api = `${origin}/api/v1`;
  const client = await request.newContext();
  const wm = (revision = '0') => ({ request_id: randomUUID(), expected_revision: revision, reason: '本机合成样本初始化（非真实业务数据）' });
  const em = () => ({ request_id: randomUUID(), reason: '本机合成样本初始化' });
  const csrf = await (await client.get(`${api}/auth/csrf`)).json();
  const headers = { Origin: origin, 'X-CSRF-Token': csrf.token };
  const post = async (path, data, status = 200) => {
    const response = await client.post(api + path, { headers, data });
    if (response.status() !== status) throw new Error(`${path}: ${response.status()} ${await response.text()}`);
    return response.json();
  };
  const get = async path => { const r = await client.get(api + path); if (!r.ok()) throw new Error(`${path}: ${r.status()}`); return r.json(); };
  const publish = async (kind, code, content) => {
    const draft = await once(code + ':draft', () => post(`/platform/engineering/${kind}`, { ...em(), code, content: { metamodel_version: '1', description: '仅供本机人工验收', release_notes: '合成样本', ...content } }));
    return once(code + ':version', () => post(`/platform/engineering/${kind}/${draft.definition_id}/publish`, { ...em(), revision: draft.revision }));
  };
  try {
    await post('/auth/login', { username: user.username, password: user.password });
    await once('category', () => post('/platform/engineering/unit-categories', { ...em(), key: 'local_temperature', name: '合成温度', description: '本机验收' }));
    const unit = await publish('units', 'local_degc', { name: '摄氏度', unit: { symbol: 'degC', category: 'local_temperature' } });
    const thermal = { shape: 'decimal', category: 'local_temperature', standard_unit: unit.version_id, allowed_units: [unit.version_id], options: [] };
    const metric = await publish('metrics', 'local_temperature', { name: '温度', metric: { ...thermal, minimum: -100, maximum: 100 } });
    const exact = await publish('metrics', 'local_exact', { name: '精度边界值', metric: thermal });
    const enumeration = await publish('metrics', 'local_state', { name: '状态', metric: { shape: 'enum', category: '', standard_unit: '', allowed_units: [], options: [{ key: 'yes', name: '正常' }, { key: 'no', name: '异常' }] } });
    const type = await publish('type-definitions', 'local_observation_facility', { name: '合成观测设施', fields: [], components: [], relations: [] });
    const account = (await get(`/access/accounts?q=${user.username}`)).items.find(a => a.username === user.username);
    const tenant = await once('tenant', () => post('/platform/tenants', { name: '监测数据 · 本机合成验收', code: 'monitoring-local', administrator_account_id: account.id, client_request_id: randomUUID(), reason: '独立合成验收空间' }, 201));
    state.tenant = tenant.tenant_id; const base = `/tenants/${state.tenant}`, monitoring = base + '/monitoring';
    const facility = await once('facility', () => post(`${base}/engineering/resources/facilities`, { request_id: randomUUID(), code: 'LOCAL', name: '合成观测站 A', type_version_id: type.version_id, values: {} }, 201));
    const point = async depth => once(`point${depth}`, () => post(`${base}/engineering/resources/facilities/${facility.id}/points`, { request_id: randomUUID(), code: `P${depth}`, name: `孔 A 深度 ${depth} m` }, 201));
    const p10 = await point(10), p20 = await point(20);
    const channel = async (key, p, m, name, symbol = 'degC') => once(key, () => post(`${base}/engineering/resources/facilities/${facility.id}/points/${p.id}/channels`, { request_id: randomUUID(), code: key, name, metric_version_id: m.version_id, ...(symbol ? { unit_version_id: unit.version_id } : {}), timestamp_meaning: 'observation_time' }, 201));
    const c10 = await channel('T10', p10, metric, '温度 10 m'), c20 = await channel('T20', p20, metric, '温度 20 m');
    const cExact = await channel('EXACT', p10, exact, '精确小数'), cState = await channel('STATE', p20, enumeration, '状态样本', '');
    state.channels = { temperature10: c10.id, temperature20: c20.id, exact: cExact.id, state: cState.id };
    state.url = `${origin}/t/${state.tenant}/monitoring?point=${p10.id}&channel=${c10.id}`;
    mkdirSync(join(directory, 'samples'), { recursive: true });
    const wait = async (id, expected) => {
      for (let i = 0; i < 240; i++) {
        const batch = await get(`${monitoring}/imports/${id}`);
        if (batch.state === expected) return batch;
        if (batch.state === 'failed') throw new Error(`Batch ${id}: ${batch.error_code}`);
        await new Promise(resolve => setTimeout(resolve, 500));
      }
      throw new Error(`Batch ${id} did not reach ${expected}`);
    };
    const upload = async (name, csv, target, { expected = 'ready', publishNow = true, mapping: change = {}, unit: symbol = 'degC' } = {}) => {
      writeFileSync(join(directory, 'samples', name), csv, { mode: 0o600 });
      const bytes = Buffer.from(csv), hash = createHash('sha256').update(bytes).digest('hex');
      const created = await once(name + ':create', () => post(`${monitoring}/imports`, { ...wm(), name, size_bytes: String(bytes.length) }, 201));
      await once(name + ':sealed', async () => {
        const r = await client.put(`${api}${monitoring}/imports/${created.id}/upload/parts/1`, { headers: { ...headers, 'Content-Type': 'application/octet-stream', 'X-Expected-Revision': created.revision, 'X-Content-SHA256': hash }, data: bytes });
        if (!r.ok()) throw new Error(`Upload ${name}: ${r.status()}`);
        return post(`${monitoring}/imports/${created.id}/upload/complete`, { ...wm(created.revision), sha256: hash });
      });
      await once(name + ':mapped', async () => {
        const b = await get(`${monitoring}/imports/${created.id}`);
        return post(`${monitoring}/imports/${created.id}/mappings`, { ...wm(b.revision), mapping: { format: 'csv', mode: 'single', encoding: 'utf-8', delimiter: ',', header_row: 1, time_column: 'observed_at', value_column: 'value', quality_column: 'quality', timezone: 'UTC', empty_wide: 'error', channels: [{ alias: 'C', channel_id: target, unit: symbol }], ...change } });
      });
      await once(name + ':validated', async () => { const b = await get(`${monitoring}/imports/${created.id}`); if (b.state === 'received') await post(`${monitoring}/imports/${b.id}/validate`, wm(b.revision), 202); return wait(b.id, expected); });
      if (publishNow) await once(name + ':published', async () => { const b = await get(`${monitoring}/imports/${created.id}`); if (b.state === 'ready') await post(`${monitoring}/imports/${b.id}/publish`, wm(b.revision), 202); return wait(b.id, 'published'); });
      const b = await get(`${monitoring}/imports/${created.id}`); state.batches[name] = { id: b.id, state: b.state, counts: b.counts, sha256: hash }; save(); return b;
    };
    const csv = rows => 'observed_at,value,quality\n' + rows.join('\n') + '\n';
    const series = (year, start, length, value) => Array.from({ length }, (_, i) => `${new Date(Date.UTC(year, 0, 1) + (start + i) * 1000).toISOString()},${value(i, start + i)},good`);
    await upload('A-100.csv', csv(series(2020, 0, 100, (_, n) => n % 10 - 5)), c10.id);
    await upload('B-80-duplicates-20-new.csv', csv(series(2020, 20, 100, (_, n) => n % 10 - 5)), c10.id, { expected: 'needs_resolution', publishNow: false });
    await upload('temperature20-605.csv', csv(series(2010, 0, 605, (_, n) => n % 10)), c20.id);
    await upload('two-outside-bounds-100.csv', csv(series(2023, 0, 100, i => i === 98 ? 1000 : i === 99 ? -1000 : i % 10)), c10.id);
    const invalid = series(2022, 0, 100, i => i % 10); invalid[99] = 'invalid-date,0,good';
    await upload('one-invalid-100.csv', csv(invalid), c10.id, { expected: 'needs_resolution', publishNow: false });
    await upload('exact-decimal.csv', csv(['2018-01-01T00:00:00.000000001Z,10000000000000000000000000.000000000001,good', '2018-01-01T00:00:00.000000002Z,10000000000000000000000000.000000000003,unmarked']), cExact.id);
    await upload('states.csv', csv(['2024-01-01T00:00:00Z,yes,good', '2024-01-02T00:00:00Z,no,good', '2024-01-03T00:00:00Z,yes,unmarked', '2024-01-04T00:00:00Z,no,bad']), cState.id, { unit: '' });
    await upload('history-1900.csv', csv(['1900-01-01T00:00:00.000000001Z,-2,good', '1900-01-02T00:00:00Z,-1,unmarked']), c10.id);
    await upload('mixed-wide.csv', 'observed_at,t10,t20,quality\n2019-01-01T00:00:00Z,-2,-3,good\n2019-01-02T00:00:00Z,0,1,good\n', c10.id, { mapping: { mode: 'wide', value_column: '', channels: [{ alias: 'T10', value_column: 't10', channel_id: c10.id, unit: 'degC' }, { alias: 'T20', value_column: 't20', channel_id: c20.id, unit: 'degC' }] } });
    writeFileSync(join(directory, 'samples', 'conflict.csv'), csv(['2020-01-01T00:00:00Z,99,good', '2020-01-01T00:03:00Z,2,good']), { mode: 0o600 });
    const context = await get(base + '/context');
    const project = await once('project', () => post(base + '/projects', { client_request_id: randomUUID(), reason: '局部历史授权验收', name: '合成项目 · 仅温度 10 m 局部历史', code: 'local-partial', purpose: '验证项目时间范围边界', owner_tenant_membership_id: context.membership_id, administrator_tenant_membership_id: context.membership_id }));
    const pb = base + '/projects/' + project.project_id, resource = pb + '/resource-access/facilities/' + facility.id;
    await once('resourceGrant', async () => {
      const current = await get(resource), change = { request_id: randomUUID(), reason: '限定单通道', action: 'create_link', expected_access_revision: current.policy.revision, add: [{ kind: 'facility', capabilities: ['read'] }, { kind: 'channel', channel_id: c10.id, capabilities: ['read'] }] };
      const preview = await post(resource + '/preview', change); return post(resource + '/changes', { ...change, preview_hash: preview.preview_hash });
    });
    await once('dataGrant', async () => {
      const resourceState = await get(resource), data = await get(pb + '/monitoring-access');
      const grant = resourceState.grants.find(g => g.resource_id === c10.id && !g.ended_at);
      return post(pb + '/monitoring-access', { ...wm(data.access_revision), channel_id: c10.id, resource_grant_id: grant.id, range: { from: '2020-01-01T00:00:20Z', to: '2020-01-01T00:00:30Z' }, capabilities: ['read', 'export', 'history', 'source'] });
    });
    state.project = project.project_id; state.project_url = `${origin}/t/${state.tenant}/p/${state.project}/monitoring?point=${p10.id}&channel=${c10.id}`;
    state.expected = { active: '817', ever_published: '817', default_samples: '814', temperature10_active: '204', project_rows: '10', B_pending_duplicates: '80', B_pending_new: '20', exact_mean: '10000000000000000000000000.000000000002', exact_population_stddev: '0.000000000001' };
    state.complete = true; save(); console.log(`合成验收空间已准备：${state.url}\n数据清单：${stateFile}`);
  } finally { await client.dispose(); }
}
