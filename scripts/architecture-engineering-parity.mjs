import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, renameSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, root, runDirectory, write } from './architecture-lab.mjs';
import { withHTTPPair, startPairedWorkers } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';
import { seedSensorStructure } from './fixtures/sensor-structure-acceptance.mjs';
import { seedDerivedAcceptance } from './fixtures/derived-acceptance.mjs';
import { seedSpatialAcceptance } from './fixtures/spatial-acceptance.mjs';

function namespace(value, suffix) {
  if (typeof value === 'string') {
    if (
      value === 'SBM' ||
      ['SBM-S', 'SBM-U'].includes(value) ||
      value.startsWith('sbm_') ||
      value.startsWith('sbm-') ||
      value.startsWith('acceptance_') ||
      value === 'derived-acceptance'
    )
      return value + '_' + suffix;
    return value;
  }
  if (Array.isArray(value)) return value.map((v) => namespace(v, suffix));
  if (value === null || Buffer.isBuffer(value) || typeof value !== 'object') return value;
  return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, namespace(v, suffix)]));
}
export async function engineeringParity(dir, output = dir, section = 'all') {
  const spatial = section === 'spatial';
  return withHTTPPair(
    dir,
    'engineering',
    (fixture, controls) => exercise(fixture, output, section, controls),
    {
      monitoringWorkers: true,
      workerRoles: spatial ? ['spatial-worker'] : [],
      configure: ({ side, cfg, privateDir, baseline }) => {
        if (!spatial) return;
        const folder = join(privateDir, 'spatial');
        mkdirSync(folder, { recursive: true, mode: 0o700 });
        cfg.spatial_python = join(root, '.artifacts/spatial-venv/bin/python');
        cfg.spatial_converter_script = join(
          side === 'a' ? baseline.source : root,
          'tools/spatial-converter/convert.py',
        );
        cfg.spatial_temp_directory = folder;
        assert.ok(
          existsSync(cfg.spatial_python) && existsSync(cfg.spatial_converter_script),
          'spatial converter required',
        );
        return {
          start: (opts) =>
            startPairedWorkers({
              ...opts,
              side,
              privateDir,
              legacy: ['spatial-worker'],
              split: ['spatial-worker'],
              claimRoles: [],
            }),
        };
      },
    },
  );
}
async function exercise(dir, output, section, { withInteractiveSlot }) {
  const p = new PairedHTTP(dir, 'engineering-' + section, output),
    accounts = read(join(dir, 'private/accounts.json'));
  const suffix = randomUUID().slice(0, 7),
    fixture = join(dir, 'private', 'engineering-parity-' + suffix);
  mkdirSync(fixture, { recursive: true, mode: 0o700 });
  const workflows = [];
  const paired = {
    origin: p.origin,
    client: async () => {
      const c = await p.client();
      function scoped(options) {
        return { ...options, data: namespace(options?.data, suffix) };
      }
      return {
        ...c,
        async post(url, options) {
          const send = () => c.post(url, scoped(options));
          if (!new URL(url).pathname.endsWith('/trials')) return send();
          return withInteractiveSlot(async () => {
            const response = await send();
            assert.equal(
              response.status(),
              200,
              'Three-second synchronous trial with a free interactive slot',
            );
            return response;
          });
        },
        fetch: (u, o) => c.fetch(u, scoped(o)),
      };
    },
  };
  const context = async () => {
    const c = await paired.client(),
      api = p.origin + '/api/v1';
    const token = (await (await c.get(api + '/auth/csrf')).json()).token,
      headers = { Origin: p.origin, 'X-CSRF-Token': token };
    const call = async (path, data, method = data === undefined ? 'GET' : 'POST') => {
      const r = await c.fetch(api + path, { method, headers, data });
      if (!r.ok())
        throw Error(
          `${method} ${path.replace(/[a-f0-9-]{36}/g, '<id>')} ${r.status()} ${(await r.json().catch(() => ({}))).code || ''}`,
        );
      return r.status() === 204 ? {} : r.json();
    };
    await call('/auth/login', accounts.admin);
    const wait = (path, pick, want) => c.wait(api + path, headers, pick, want);
    return {
      call,
      client: c,
      api,
      headers,
      accountID: read(join(dir, 'samples.json')).actors.admin,
      categoryIDs: read(join(dir, 'samples.json')).structure.categories,
      wait,
    };
  };
  async function scenario(name, arcs, fn, checks) {
    const start = p.records.length;
    let ok = false;
    try {
      await fn();
      ok = true;
    } catch (e) {
      p.problems.push({ name, error: e.message.slice(0, 320) });
    } finally {
      const cases = p.records.slice(start);
      for (const arc of arcs)
        workflows.push({
          arc,
          name,
          passed: ok && cases.every((v) => !v.differences.length),
          caseIds: cases.map((v) => v.command),
          checks,
        });
      p.save();
      write(join(output, 'http', 'workflows', 'engineering-' + section + '.json'), {
        cases: workflows,
      });
    }
    return ok;
  }
  try {
    if (['all', 'structure'].includes(section))
      await scenario(
        '工程结构、产品、来源与数据权限',
        ['ARC-007', 'ARC-008', 'ARC-009'],
        async () => {
          const actor = await context();
          const state = await seedSensorStructure(actor);
          const { batch, nodes, channels } = state;
          assert.ok(batch && nodes.A.id && channels.P1.id);
          write(join(fixture, 'structure-a.json'), state);
          write(join(fixture, 'structure-b.json'), p.translate(state));
        },
        [
          '发布类型、量纲和模板，逐节点建立设施、断面、孔位、测点',
          '6 台设备的产品分类、输出与通道绑定、两个设施的成员授权',
          '同一 CSV 分片、验证、正式发布后读回 70 条观测的导入状态',
        ],
      );
    if (['all', 'derived', 'spatial'].includes(section)) {
      const built = await scenario(
        '原始与派生监测及短试算',
        ['ARC-009', 'ARC-010', 'ARC-011'],
        async () => {
          await seedDerivedAcceptance(fixture, p.origin, {
            paired,
            adminLogin: accounts.admin,
            minimal: section === 'spatial',
          });
        },
        [
          '上传、映射、验证及正式发布三组同时间的双通道观测',
          '同一依据的逐项数值、空值、单位、同步/后台路径及稳定谱系',
        ],
      );
      if (built && section === 'spatial')
        await scenario(
          '空间模型、图纸、地图及版本',
          ['ARC-012'],
          async () => {
            await seedSpatialAcceptance(fixture, p.origin, { paired });
          },
          [
            '真实转换器发布 PDF、图片、模型和 GIS，并通过租户和项目空间读取版本与坐标',
            '下载权限、挂接关系、成果撤回及稳定引用',
          ],
        );
    }
  } finally {
    await p.close();
  }
  // A complete replay runs the same full formula sequence as the standalone
  // derived diagnostic. Preserve its older attempt, then link the newer raw
  // request IDs instead of keeping an obsolete failure in the current ledger.
  if (section === 'all' && workflows.length === 6 && workflows.every((item) => item.passed)) {
    const derived = workflows.filter((item) => item.name === '原始与派生监测及短试算');
    assert.equal(derived.length, 3);
    const path = join(output, 'http', 'workflows', 'engineering-derived.json');
    if (existsSync(path)) {
      const archive = join(output, 'http', 'attempts', 'engineering-derived');
      mkdirSync(archive, { recursive: true, mode: 0o700 });
      renameSync(path, join(archive, Date.now() + '-workflows.json'));
    }
    write(path, { source: 'http/scenarios/engineering-all.json', cases: derived });
  }
  return {
    records: p.records.length,
    problems: p.problems,
    passed: workflows.length > 0 && workflows.every((v) => v.passed),
  };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]);
  const result = await engineeringParity(
    dir,
    runDirectory(process.argv[3] || dir),
    process.argv[4],
  );
  console.log(result);
  process.exitCode = result.passed ? 0 : 1;
}
