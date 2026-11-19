import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, root, runDirectory, write } from './architecture-lab.mjs';
import { operation, PairedHTTP } from './architecture-workflow-http.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
const schema = read(join(root, 'contracts/openapi/api.yaml'));
function fields(path, method, data) {
  const op = operation(method, '/api/v1' + path);
  assert.ok(op, method + ' ' + path);
  const name = schema.paths[op.template.replace(/:(\w+)/g, '{$1}')][
    method.toLowerCase()
  ].requestBody?.content['application/json']?.schema?.$ref
    ?.split('/')
    .pop();
  const allowed = schema.components.schemas[name]?.properties;
  return allowed
    ? Object.fromEntries(Object.entries(data).filter(([k]) => Object.hasOwn(allowed, k)))
    : data;
}
const meta = () => ({ request_id: randomUUID(), reason: '架构验收独立工程对象' });
export async function resourceParity(dir, output = dir) {
  return withHTTPPair(dir, 'resources', (fixture) => exercise(fixture, output));
}
async function exercise(dir, output) {
  const pair = new PairedHTTP(dir, 'resources', output),
    log = [],
    fixture = read(join(dir, 'samples.json')).structure;
  const { call } = await pair.login(read(join(dir, 'private/accounts.json')).admin);
  const tenant = '/tenants/' + fixture.tenant,
    suffix = randomUUID().slice(0, 8),
    member = (await call(tenant + '/context')).membership_id;
  const project = (
    await call(tenant + '/projects', {
      name: '资源生命周期验收 ' + suffix,
      code: 'L' + suffix,
      purpose: '隔离项目',
      client_request_id: randomUUID(),
      owner_tenant_membership_id: member,
      administrator_tenant_membership_id: member,
      reason: '工程验收',
    })
  ).project_id;
  const pb = tenant + '/projects/' + project;
  const policy = (await call(pb + '/resource-access')).policy;
  const enable = {
    ...meta(),
    action: 'policy',
    expected_access_revision: policy.revision,
    allow_facility_create: true,
  };
  const pv = await call(pb + '/resource-access/policy/preview', enable);
  await call(pb + '/resource-access/policy', { ...enable, preview_hash: pv.preview_hash });
  async function section(name, fn) {
    const first = pair.records.length;
    let error;
    try {
      await fn();
    } catch (e) {
      error = e.message;
      pair.problems.push({ name, error });
    } finally {
      const cases = pair.records.slice(first);
      log.push({
        arc: 'ARC-007',
        name,
        passed: !error && cases.every((r) => !r.differences.length),
        caseIds: cases.map((r) => r.command),
        checks: [
          '每次变更先读取权威版本和权限依据',
          '按相同命令提交、比较两端实体与版本',
          '按原作用域读回终态',
        ],
      });
      write(join(output, 'http/workflows/resources.json'), { cases: log });
      pair.save();
    }
  }
  await section('已发布设施类型的兼容采用与命令收据', async () => {
    const type = fixture.types.A,
      definition = '/platform/engineering/type-definitions/' + type.definition_id;
    const current = await call(definition);
    const draft = await call(definition + '/draft', {
      ...meta(),
      revision: current.revision,
      version_id: type.version_id,
    });
    const { content } = (await call(definition)).draft;
    const saved = await call(
      definition + '/draft',
      {
        ...meta(),
        revision: draft.revision,
        content: { ...content, release_notes: '结构保持兼容的版本升级' },
      },
      'PUT',
    );
    const next = await call(definition + '/publish', { ...meta(), revision: saved.revision });
    assert.notEqual(next.version_id, type.version_id);
    const path = tenant + '/engineering/resources/facilities/' + fixture.nodes.A.id,
      facility = await call(path);
    assert.equal(facility.type_version_id, type.version_id);
    const adopt = {
      ...meta(),
      expected_revision: facility.revision,
      expected_structure_revision: facility.structure_revision,
      type_version_id: next.version_id,
    };
    const preview = await call(path + '/adopt-type-preview', adopt);
    assert.equal(preview.preserves_identities, true);
    const changed = await call(path + '/adopt-type', {
      ...adopt,
      preview_hash: preview.preview_hash,
    });
    assert.equal(changed.id, facility.id);
    const receipt = await call(tenant + '/engineering/resources/requests/' + adopt.request_id);
    assert.equal(receipt.id, changed.id);
    assert.equal((await call(path)).type_version_id, next.version_id);
  });
  for (const scope of ['tenant', 'project']) {
    const scoped = scope === 'project',
      base = scoped ? pb : tenant,
      facilities = base + '/engineering/resources/facilities';
    let facility, point;
    async function authority() {
      if (!scoped) return {};
      const endpoint = pb + '/resource-access' + (facility ? '/facilities/' + facility : '');
      const s = await call(endpoint);
      return {
        expected_access_revision: s.policy.revision,
        ...(s.link && { expected_link_revision: s.link.revision }),
      };
    }
    async function create(path, data) {
      let versions = await authority();
      if (facility)
        versions.expected_structure_revision = (
          await call(facilities + '/' + facility)
        ).structure_revision;
      return call(path, fields(path, 'POST', { ...meta(), ...versions, ...data }));
    }
    async function update(path, action, extras = {}) {
      const state = await call(path),
        method = action === 'update' ? 'PATCH' : 'POST',
        target = action === 'update' ? path : path + '/' + action;
      const body = fields(target, method, {
        ...meta(),
        ...(await authority()),
        expected_revision: state.revision,
        expected_structure_revision: (await call(facilities + '/' + facility)).structure_revision,
        ...extras,
      });
      const preview = await call(path + '/mutation-preview', { action, ...body });
      const out = await call(target, { ...body, preview_hash: preview.preview_hash }, method);
      assert.ok(out.id || out.replayed);
      return out;
    }
    async function related(path, relative, input) {
      const st = await call(path),
        rootState = await call(facilities + '/' + facility);
      const body = {
        ...meta(),
        ...(await authority()),
        expected_revision: st.revision,
        expected_structure_revision: rootState.structure_revision,
        ...input,
      };
      const v = await call(path + '/' + relative + '-preview', body);
      assert.ok(v.preview_hash);
      return call(path + '/' + relative, { ...body, preview_hash: v.preview_hash });
    }
    function details(node) {
      return (
        facilities +
        '/' +
        facility +
        (node.kind === 'facility' ? '' : '/' + node.group + '/' + node.id)
      );
    }
    async function allowChildren(kinds) {
      const path = pb + '/resource-access/facilities/' + facility,
        state = await call(path);
      const update = state.grants
        .filter((g) => kinds.includes(g.kind))
        .map((g) => ({
          grant_id: g.id,
          expected_revision: g.revision,
          capabilities: [
            ...new Set([...g.capabilities, 'create_monitoring_section', 'create_borehole']),
          ],
        }));
      assert.equal(update.length, kinds.length);
      const change = {
        ...meta(),
        action: 'change_grants',
        expected_access_revision: state.policy.revision,
        expected_link_revision: state.link.revision,
        update,
      };
      const preview = await call(path + '/preview', change);
      await call(path + '/changes', { ...change, preview_hash: preview.preview_hash });
    }
    const nodes = [];
    try {
      const createID = randomUUID();
      const made = await create(facilities, {
        request_id: createID,
        code: 'F' + suffix + scope[0],
        name: '生命周期设施',
        type_version_id: fixture.types.A.version_id,
        values: {},
      });
      facility = made.id;
      nodes.push({ kind: 'facility', id: facility });
      await section(scope + ' 创建后查询原命令收据', async () => {
        const receipt = await call(base + '/engineering/resources/requests/' + createID);
        assert.equal(receipt.id, made.id);
        assert.equal(receipt.structure_revision, made.structure_revision);
      });
      const facilityPath = facilities + '/' + facility;
      if (scoped) await allowChildren(['facility']);
      for (const [kind, group, extra] of [
        ['component', 'components', { category_key: 'span', values: {} }],
        ['monitoring_section', 'monitoring-sections', {}],
        ['borehole', 'boreholes', { borehole: { measured_depth_m: 6 } }],
        ['point', 'points', {}],
      ]) {
        await section(scope + ' 创建 ' + kind, async () => {
          const item = await create(facilityPath + '/' + group, {
            ...extra,
            code: 'N' + kind,
            name: '工程验收 ' + kind,
            parent_ref: { kind: 'facility', id: facility },
          });
          nodes.push({ kind, group, id: item.id });
          if (kind === 'point') point = item.id;
        });
      }
      if (point) {
        await section(scope + ' 创建通道', async () => {
          const item = await create(facilityPath + '/points/' + point + '/channels', {
            code: 'CH1',
            name: '测试位移',
            metric_version_id: fixture.metric,
            unit_version_id: fixture.unit,
            timestamp_meaning: 'observation_time',
          });
          nodes.push({ kind: 'channel', group: 'points/' + point + '/channels', id: item.id });
        });
      }
      for (const node of nodes) {
        const path = details(node);
        await section(scope + ' 修改 ' + node.kind, () =>
          update(path, 'update', { name: '已修改 ' + node.kind }),
        );
        if (node.kind === 'channel') continue;
        const other = nodes.find((v) => v.kind !== node.kind && v.kind !== 'channel');
        if (other) {
          await section(scope + ' 实体参照 ' + node.kind, async () => {
            await related(path, 'structure-records', {
              action: 'reference',
              target: { kind: other.kind, id: other.id },
              purpose: '验收关联对象',
            });
            const details = await call(path + '/structure-details?kind=references');
            const record =
              details.items.find((r) => r.target_id === other.id || r.target?.id === other.id) ||
              details.items[0];
            assert.ok(record?.id);
            await related(path, 'structure-records', {
              action: 'end_reference',
              record_id: record.id,
            });
          });
        }
      }
      if (scoped) await allowChildren(['monitoring_section', 'borehole']);
      const sectionNode = nodes.find((n) => n.kind === 'monitoring_section'),
        holeNode = nodes.find((n) => n.kind === 'borehole');
      for (const node of nodes.filter((n) => !['facility', 'channel'].includes(n.kind))) {
        const parent = node.kind === 'monitoring_section' ? holeNode : sectionNode;
        if (!parent) continue;
        await section(scope + ' 移动 ' + node.kind, () =>
          related(details(node), 'move', {
            parent_ref: { kind: parent.kind, id: parent.id },
            position_disposition: 'history_pending',
          }),
        );
        await section(scope + ' 复位 ' + node.kind, () =>
          related(details(node), 'move', {
            parent_ref: { kind: 'facility', id: facility },
            position_disposition: 'history_pending',
          }),
        );
      }
      for (const node of nodes.toReversed()) {
        const path = details(node);
        for (const action of ['retire', 'restore'])
          if (operation('POST', '/api/v1' + path + '/' + action))
            await section(scope + ' ' + node.kind + ' ' + action, () => update(path, action));
        if (node.kind === 'channel') {
          const alias = facilityPath + '/channels/' + node.id;
          await section(scope + ' 通道别名', async () => {
            const a = await call(alias);
            assert.equal(a.id, node.id);
            await update(alias, 'update', { name: '按设施寻址' });
            await update(alias, 'retire');
            await update(alias, 'restore');
          });
        }
        if (!scoped)
          await section(scope + ' 删除保护 ' + node.kind, async () => {
            if (node.kind === 'channel') return update(path, 'delete');
            // Even an ended engineering reference must retain its source and target.
            // The facility also owns these retained children.
            await assert.rejects(
              update(path, 'delete'),
              node.kind === 'facility' ? /409 resource_referenced/ : /422 resource_invalid/,
            );
            assert.equal((await call(path)).id, node.id);
          });
      }
      if (!scoped)
        await section('空白工程对象删除', async () => {
          const disposable = await call(
            facilities,
            fields(facilities, 'POST', {
              ...meta(),
              name: '可删除设施',
              code: 'D' + suffix,
              type_version_id: fixture.types.A.version_id,
              values: {},
            }),
          );
          facility = disposable.id;
          const base = facilities + '/' + facility;
          for (const [kind, group, extra] of [
            ['component', 'components', { category_key: 'span', values: {} }],
            ['monitoring_section', 'monitoring-sections', {}],
            ['borehole', 'boreholes', { borehole: { measured_depth_m: 2 } }],
            ['point', 'points', {}],
          ]) {
            const row = await create(base + '/' + group, {
              ...extra,
              code: 'D' + kind,
              name: '可删除工程对象',
              parent_ref: { kind: 'facility', id: facility },
            });
            const path = base + '/' + group + '/' + row.id;
            if (kind === 'point') {
              const channel = await create(path + '/channels', {
                code: 'DCH',
                name: '可删除通道',
                metric_version_id: fixture.metric,
                unit_version_id: fixture.unit,
                timestamp_meaning: 'observation_time',
              });
              await update(base + '/channels/' + channel.id, 'delete');
            }
            await update(path, 'delete');
          }
          await update(base, 'delete');
        });
    } catch (e) {
      pair.problems.push({ name: scope, error: e.message });
    }
  }
  await pair.close();
  return {
    passed: !pair.problems.length && log.every((c) => c.passed),
    problems: pair.problems,
    records: pair.records.length,
  };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]);
  const r = await resourceParity(dir, runDirectory(process.argv[3] || dir));
  console.log(r);
  process.exitCode = r.passed ? 0 : 1;
}
