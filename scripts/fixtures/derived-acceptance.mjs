import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { request } from '@playwright/test';

export async function seedDerivedAcceptance(directory, origin) {
  if (origin !== 'http://127.0.0.1:15373') throw new Error('Synthetic derived fixture requires its dedicated loopback session.');
  const client = await request.newContext(), api = `${origin}/api/v1`;
  const state = { synthetic: true, fixture: 'development-plan-25.1', channels: {}, expected: {}, batches: {} };
  const save = () => writeFileSync(join(directory, 'acceptance.json'), JSON.stringify(state, null, 2), { mode: 0o600 });
  const wm = (revision = '0') => ({ request_id: randomUUID(), expected_revision: revision, reason: '方案25.1独立合成验收样本' });
  const csrf = await (await client.get(`${api}/auth/csrf`)).json(), headers = { Origin: origin, 'X-CSRF-Token': csrf.token };
  const call = async (path, data, statuses = [200, 201, 202, 204]) => {
    const response = data === undefined ? await client.get(api + path) : await client.post(api + path, { headers, data });
    if (!statuses.includes(response.status())) throw new Error(`${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? null : response.json();
  };
  const wait = async (path, pick, desired) => {
    const deadline = Date.now() + 120000;
    while (Date.now() < deadline) {
      const v = await call(path), status = pick(v);
      if (status === desired) return v;
      if (['failed', 'needs_action', 'cancelled'].includes(status)) throw new Error(`${path}: ${status} ${JSON.stringify(v)}`);
      await new Promise(resolve => setTimeout(resolve, 500));
    }
    throw new Error(`${path} did not reach ${desired}`);
  };
  try {
    await call('/auth/login', { username: 'e2e_owner', password: 'Real browser 河流 secret 592' });
    const invitation = await call('/invitations', { note: '方案25.1合成验收账号' });
    const token = new URLSearchParams(new URL(invitation.url).hash.slice(1)).get('token');
    await call('/auth/logout', {});
    const user = { username: `derived_${randomBytes(5).toString('hex')}`, display_name: '派生监测验收', password: randomBytes(24).toString('base64url') };
    await call('/public/invitations/accept', { token, ...user });
    writeFileSync(join(directory, 'login.json'), JSON.stringify({ ...user, url: origin, synthetic: true }, null, 2), { mode: 0o600 });
    await call('/auth/login', { username: 'e2e_owner', password: 'Real browser 河流 secret 592' });
    const publish = async (kind, code, content) => {
      const draft = await call(`/platform/engineering/${kind}`, { request_id: randomUUID(), reason: '合成验收量纲', code, content: { metamodel_version: '1', description: '方案25.1独立合成样本', release_notes: '', ...content } });
      return call(`/platform/engineering/${kind}/${draft.definition_id}/publish`, { request_id: randomUUID(), reason: '固定验收版本', revision: draft.revision });
    };
    await call('/platform/engineering/unit-categories', { request_id: randomUUID(), reason: '合成验收量纲', key: 'acceptance_length', name: '验收长度', description: '合成位移' });
    const unit = await publish('units', 'acceptance_mm', { name: '毫米', unit: { symbol: 'mm', category: 'acceptance_length' } });
    const metric = await publish('metrics', 'acceptance_displacement', { name: '验收位移', metric: { shape: 'decimal', category: 'acceptance_length', standard_unit: unit.version_id, allowed_units: [unit.version_id], options: [] } });
    const type = await publish('type-definitions', 'acceptance_bridge', { name: '验收桥梁', fields: [], components: [{ key: 'beam', name: '主梁', description: '', fields: [] }], relations: [{ parent: 'root', child: 'beam', min_count: 0, max_count: 5, order: 0 }] });
    const account = (await call(`/access/accounts?q=${user.username}`)).items.find(v => v.username === user.username);
    const tenant = await call('/platform/tenants', { name: '派生监测 · 方案25.1合成验收', code: 'derived-acceptance', administrator_account_id: account.id, client_request_id: randomUUID(), reason: '独立临时验收空间' });
    state.tenant = tenant.tenant_id;
    await call('/auth/logout', {}); await call('/auth/login', { username: user.username, password: user.password });
    const base = `/tenants/${state.tenant}`, monitoring = `${base}/monitoring`, derived = `${monitoring}/derived`;
    const facility = await call(`${base}/engineering/resources/facilities`, { request_id: randomUUID(), code: 'BRIDGE', name: 'A/B位移验收桥', type_version_id: type.version_id, values: {} });
    const point = await call(`${base}/engineering/resources/facilities/${facility.id}/points`, { request_id: randomUUID(), code: 'P1', name: '位移验收测点' });
    const secondPoint = await call(`${base}/engineering/resources/facilities/${facility.id}/points`, { request_id: randomUUID(), code: 'P2', name: '位移B验收测点' });
    state.facility = facility.id; state.point = point.id; state.second_point = secondPoint.id;
    for (const key of ['A', 'B']) state.channels[key] = await call(`${base}/engineering/resources/facilities/${facility.id}/points/${key === 'A' ? point.id : secondPoint.id}/channels`, { request_id: randomUUID(), code: key, name: `位移${key}`, metric_version_id: metric.version_id, unit_version_id: unit.version_id, timestamp_meaning: 'observation_time' });
    mkdirSync(join(directory, 'samples'), { mode: 0o700 });
    const csv = 'observed_at,A,B,quality\n2024-01-01T00:00:00Z,10,6,good\n2024-01-01T00:10:00Z,14,9,good\n2024-01-01T00:30:00Z,20,11,good\n';
    writeFileSync(join(directory, 'samples', 'A-B.csv'), csv, { mode: 0o600 });
    writeFileSync(join(directory, 'samples', 'append-00-40.csv'), 'observed_at,A,B,quality\n2024-01-01T00:40:00Z,26,13,good\n', { mode: 0o600 });
    writeFileSync(join(directory, 'samples', 'late-00-20.csv'), 'observed_at,A,B,quality\n2024-01-01T00:20:00Z,17,10,good\n', { mode: 0o600 });
    const sha256 = createHash('sha256').update(csv).digest('hex');
    let batch = await call(`${monitoring}/imports`, { ...wm(), name: 'A-B.csv', size_bytes: String(Buffer.byteLength(csv)) });
    const upload = await client.put(`${api}${monitoring}/imports/${batch.id}/upload/parts/1`, { headers: { ...headers, 'Content-Type': 'application/octet-stream', 'X-Expected-Revision': batch.revision, 'X-Content-SHA256': sha256 }, data: Buffer.from(csv) });
    if (upload.status() !== 200) throw new Error(`A/B upload failed: ${upload.status()}`);
    batch = await call(`${monitoring}/imports/${batch.id}/upload/complete`, { ...wm(batch.revision), sha256 });
    batch = await call(`${monitoring}/imports/${batch.id}/mappings`, { ...wm(batch.revision), mapping: { format: 'csv', mode: 'wide', encoding: 'utf-8', delimiter: ',', header_row: 1, time_column: 'observed_at', quality_column: 'quality', timezone: 'UTC', empty_wide: 'error', channels: ['A', 'B'].map(key => ({ alias: key, value_column: key, channel_id: state.channels[key].id, unit: 'mm' })) } });
    await call(`${monitoring}/imports/${batch.id}/validate`, wm(batch.revision)); batch = await wait(`${monitoring}/imports/${batch.id}`, v => v.state, 'ready');
    await call(`${monitoring}/imports/${batch.id}/publish`, wm(batch.revision)); batch = await wait(`${monitoring}/imports/${batch.id}`, v => v.state, 'published');
    state.batches.original = { id: batch.id, sha256 };
    const selection = await call(`${monitoring}/selections`, { channel_ids: [state.channels.A.id] });
    const source = await call(`${monitoring}/queries/details`, { selection_id: selection.id, range: { from: '2024-01-01T00:00:00Z', to: '2024-01-01T00:01:00Z' }, limit: 10 });
    if (source.items.length !== 1) throw new Error('Original fixed baseline was not uniquely selected.');
    const baseline = source.items[0];
    const quantity = { base_unit_version_id: unit.version_id, base_power: 1, time_power: 0, quantity_mode: 'level', level_scale: 'ratio' };
    const input = (key, sourceKind = 'direct', sourceQuantity = quantity) => ({ key, shape: 'numeric', source_kinds: [sourceKind], ...(sourceKind === 'direct' ? { metric_version_id: metric.version_id } : {}), quantity_spec: sourceQuantity, direction: 'not_applicable', reference: 'not_applicable', time_semantics: { kind: 'instant' } });
    const range = { from_ns: '1704067200000000000', to_ns: '1704069060000000000' };
    const rolling = (op, id = 'result', ref = 'input:A') => ({ id, op: `window_${op}`, inputs: [ref], params: { window: { kind: 'trailing_duration', duration_ns: '1800000000000' }, min_samples: op === 'count' ? '0' : '2', max_gap_ns: '1800000000000', max_age_ns: '1800000000000', ...(op === 'quantile' ? { percentile: '95' } : {}) } });
    const delta = (op, id = 'result', ref = 'input:A') => ({ id, op, inputs: [ref], params: { difference_mode: 'adjacent', invalid_policy: 'break', max_gap_ns: '1800000000000', ...(op === 'rate' ? { rate_time_unit: 'minute' } : {}) } });
    const add = async (code, name, nodes, expected, options = {}) => {
      const keys = options.inputs || ['A'];
      const rule = { schema_version: 'derived-rule/1', inputs: keys.map(key => input(key, options.sourceKind, options.quantity)), parameters: [], clock: { kind: 'primary_input', input: keys[0] }, nodes, outputs: [{ key: 'value', node: nodes.at(-1).id, semantic_key: code.toLowerCase(), name, numeric_contract: { scale: code === 'COUNT' ? 0 : 18, rounding: 'half_even' } }], policies: { quality: ['good', 'unmarked'], config_boundary: 'break', allowed_lateness_ns: '0' } };
      const definition = await call(`${derived}/metric-definitions`, { ...wm(), code, name, description: '方案25.1固定验收样本', rule });
      const valid = await call(`${derived}/metric-definitions/${definition.definition.id}/validate`, { draft_revision: definition.definition.draft.revision });
      const published = await call(`${derived}/metric-definitions/${definition.definition.id}/publish`, { ...wm(definition.definition.revision), draft_revision: definition.definition.draft.revision, validation_hash: valid.validation_hash });
      let channel = (await call(`${derived}/channels`, { ...wm(), code, name, owner: options.owner || { kind: 'point', id: point.id, facility_id: facility.id }, definition_version_id: published.version.id, input_bindings: Object.fromEntries(keys.map(key => [key, { channel_id: options.channel || state.channels[key].id }])), parameters: {}, ...(options.baseline ? { baseline_bindings: { origin: { input: 'A', observation_id: baseline.observation_id, adoption_id: baseline.adoption_id, at_ns: range.from_ns } } } : {}) })).channel;
      const path = `${derived}/channels/${channel.resource.id}`, config = (await call(`${path}/configs`)).items[0];
      let trial = await call(`${path}/trials`, { ...wm(channel.config_revision), config_id: config.id, config_hash: config.config_hash, output_range: range });
      trial = await wait(`${derived}/runs/${trial.id}`, v => v.job.state, 'succeeded');
      const preview = await call(`${path}/control-preview`, { expected_revision: channel.control_revision, action: 'enable', auto_start_ns: options.autoStart || range.from_ns, stop_ns: '1704153600000000000', trial_receipt_id: trial.trial.trial_receipt_id });
      channel = (await call(`${path}/control`, { ...wm(channel.control_revision), preview_id: preview.id, preview_hash: preview.hash })).channel;
      const end = Date.now() + 120000;
      let generations;
      do {
        generations = await call(`${path}/generations?from_ns=${range.from_ns}&to_ns=${range.to_ns}`);
        if (generations.items.length) break;
        await new Promise(resolve => setTimeout(resolve, 500));
      } while (Date.now() < end);
      if (!generations.items.length) throw new Error(`${code}: automatic publication did not finish`);
      const rows = await call(`${derived}/generations/${generations.items[0].id}/query`, {});
      const canonical = value => value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value;
      const values = rows.items.map(row => row.value === null ? null : canonical(row.value));
      if (JSON.stringify(values) !== JSON.stringify(expected)) throw new Error(`${code}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(values)}`);
      state.channels[code] = { id: channel.resource.id, name, definition: definition.definition.id, version: published.version.id, config: config.id, quantity: channel.resource.quantity_spec, generation: generations.items[0].id, url: `${origin}/t/${state.tenant}/monitoring/derived/${channel.resource.id}` };
      state.expected[code] = expected; save();
      console.log(`合成样本已核对：${code} ${JSON.stringify(expected)}`);
      return rule;
    };
    await add('BASELINE', '相对A首个观测的变化', [{ id: 'result', op: 'baseline_delta', inputs: ['input:A'], params: { baseline: { kind: 'observation', reference: 'origin' } } }], ['0', '4', '10'], { baseline: true });
    await add('DIFFERENCE', 'A相邻变化', [delta('difference')], [null, '4', '6']);
    await add('RATE', 'A变化速率 mm/min', [delta('rate')], [null, '0.4', '0.3']);
    await add('A_MINUS_B', '设施直属 A−B', [{ id: 'result', op: 'subtract', inputs: ['input:A', 'input:B'], params: { alignment: 'exact' } }], ['4', '5', '9'], { inputs: ['A', 'B'], owner: { kind: 'facility', id: facility.id, facility_id: facility.id }, autoStart: '1704065400000000000' });
    for (const [op, name, second, third] of [['count', '样本数量', '2', '2'], ['sum', '和', '24', '34'], ['min', '最小值', '10', '14'], ['max', '最大值', '14', '20'], ['range', '极差', '4', '6'], ['mean', '均值', '12', '17'], ['variance', '总体方差', '4', '9'], ['stddev', '总体标准差', '2', '3'], ['median', '中位数', '12', '17'], ['quantile', 'R7 P95', '13.8', '19.7']]) await add(op.toUpperCase(), `A 30分钟${name}`, [rolling(op)], [op === 'count' ? '1' : null, second, third]);
    await add('MEAN_RATE', '30分钟均值的每分钟速率', [rolling('mean', 'mean'), delta('rate', 'result', 'node:mean')], [null, null, '0.25']);
    await add('RATE_MEAN', '每分钟速率的30分钟均值', [delta('rate', 'rate'), rolling('mean', 'result', 'node:rate')], [null, null, '0.35']);
    await add('C_MEAN', '派生A−B的30分钟均值', [rolling('mean', 'result', 'input:C')], [null, '4.5', '7'], { inputs: ['C'], sourceKind: 'derived', channel: state.channels.A_MINUS_B.id, quantity: state.channels.A_MINUS_B.quantity });
    state.range = range; state.url = `${origin}/t/${state.tenant}/monitoring/derived`; state.raw_url = `${origin}/t/${state.tenant}/monitoring`; state.complete = true; save();
    return { url: state.url, tenant: state.tenant, channels: Object.keys(state.channels).length, directory };
  } finally { await client.dispose(); }
}
