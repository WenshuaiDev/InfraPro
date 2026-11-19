import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, runDirectory, write } from './architecture-lab.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';

const meta = (expected_revision = '0') => ({
  request_id: randomUUID(),
  expected_revision,
  reason: '从冻结观测复算并核对分析成果',
});
const from = Date.parse('2026-11-01T00:00:00Z'),
  to = from + 600000;
const output_range = {
  from_ns: (BigInt(from) * 1000000n).toString(),
  to_ns: (BigInt(to) * 1000000n).toString(),
};
export function analysisBody(s) {
  const numeric_contract = { scale: 6, rounding: 'half_even' };
  const input = {
    key: 'x',
    shape: 'numeric',
    source_kinds: ['direct'],
    metric_version_id: s.metric,
    quantity_spec: {
      base_unit_version_id: s.unit,
      base_power: 1,
      time_power: 0,
      quantity_mode: 'level',
      level_scale: 'ratio',
    },
    direction: 'not_applicable',
    reference: 'not_applicable',
    time_semantics: { kind: 'instant' },
  };
  const rule = {
    schema_version: 'derived-rule/1',
    inputs: [input],
    parameters: [],
    clock: { kind: 'primary_input', input: 'x' },
    nodes: [
      { id: 'double', op: 'formula', params: { expression: 'x * 2', variables: { x: 'input:x' } } },
    ],
    outputs: [
      {
        key: 'double',
        node: 'double',
        semantic_key: 'double_value',
        name: '位移复核',
        numeric_contract,
      },
    ],
    policies: { quality: ['good', 'unmarked'], config_boundary: 'break', allowed_lateness_ns: '0' },
  };
  return { rule, input_bindings: { x: { channel_id: s.channels.P1.id } }, parameters: {} };
}

export function analysisParity(dir) {
  return withHTTPPair(
    dir,
    'analysis',
    async (fixture, { withQueuesStopped }) => {
      const pair = new PairedHTTP(fixture, 'analysis', dir),
        s = read(join(fixture, 'samples.json')).structure;
      const { client, api, call, headers } = await pair.login(
        read(join(fixture, 'private/accounts.json')).admin,
      );
      const tenant = '/tenants/' + s.tenant,
        project = tenant + '/projects/' + s.project,
        workflow = [];
      const wait = (path, pick, state) => client.wait(api + path, headers, pick, state);
      async function track(name, fn) {
        const start = pair.records.length;
        let err;
        try {
          await fn();
        } catch (e) {
          err = e.message.split('\n')[0].slice(0, 260);
          pair.problems.push({ name, error: err });
        }
        const cases = pair.records.slice(start);
        workflow.push({
          arc: 'ARC-011',
          name,
          passed: !err && cases.every((v) => !v.differences.length),
          caseIds: cases.map((v) => v.command),
        });
        write(join(dir, 'http/workflows/analysis.json'), { cases: workflow });
        pair.save();
        return !err;
      }
      try {
        await track('项目取得已关联通道的分析与导出权限', async () => {
          const path = project + '/monitoring-access',
            g = (await call(path)).items.find(
              (g) => g.effective && g.channel_id === s.channels.P1.id,
            );
          assert.ok(g);
          const preview = await call(path + '/' + g.id + '/revoke-preview', {
            expected_revision: g.revision,
          });
          const rev = await call(path + '/' + g.id + '/revoke', {
            ...meta(g.revision),
            preview_hash: preview.preview_hash,
          });
          await call(path, {
            ...meta(rev.access_revision),
            channel_id: g.channel_id,
            resource_grant_id: g.resource_grant_id,
            range: g.range,
            capabilities: ['read', 'history', 'export'],
          });
        });
        for (const [scope, base] of [
          ['tenant', tenant],
          ['project', project],
        ]) {
          const d = base + '/monitoring/derived',
            a = d + '/analysis',
            configs = a + '/configs';
          let cfgPath, version, artifact, versionID, detail, request;
          const config = await track(scope + ' 分析草稿、版本、复制及归档', async () => {
            let cfg = (
              await call(configs, {
                ...meta(),
                code: 'CHECK',
                name: '位移二倍复核',
                body: analysisBody(s),
              })
            ).config;
            cfgPath = configs + '/' + cfg.id;
            assert.equal((await call(cfgPath)).code, 'CHECK');
            cfg = (
              await call(
                cfgPath + '/draft',
                {
                  ...meta(cfg.revision),
                  name: '冻结位移复核',
                  description: '逐行核对原值两倍',
                  body: analysisBody(s),
                },
                'PATCH',
              )
            ).config;
            const validation = await call(cfgPath + '/validate', {
              draft_revision: cfg.draft.revision,
            });
            assert.equal(validation.issues.length, 0);
            const published = await call(cfgPath + '/versions', {
              ...meta(cfg.revision),
              draft_revision: cfg.draft.revision,
              validation_hash: validation.validation_hash,
            });
            version = published.version;
            const versions = await call(cfgPath + '/versions');
            assert.equal(versions.items[0].id, version.id);
            const copy = (
              await call(cfgPath + '/copy', {
                ...meta(),
                code: 'CHECK_COPY',
                name: '分析副本',
                source_version_id: version.id,
              })
            ).config;
            await call(configs + '/' + copy.id + '/archive', meta(copy.revision));
          });
          if (!config) continue;
          const built = await track(scope + ' 试算、正式结果、逐行数值及谱系', async () => {
            request = {
              config_version_id: version.id,
              config_hash: version.config_hash,
              output_range,
            };
            const trial = await call(a + '/trials', { ...meta(), ...request });
            const checked = await wait(d + '/runs/' + trial.id, (v) => v.job.state, 'succeeded');
            assert.equal(checked.trial.output_count, '10');
            const cmd = { ...meta(), ...request };
            let run;
            // Observe the original receipt twice before either worker takes the job.
            await withQueuesStopped(async () => {
              run = await call(a + '/runs', cmd);
              assert.equal((await call(a + '/runs', cmd)).id, run.id);
            });
            const output = await wait(d + '/runs/' + run.id, (v) => v.job.state, 'succeeded');
            artifact = a + '/artifacts/' + output.analysis.artifact_id;
            versionID = output.analysis.artifact_version_id;
            detail = await call(artifact + '?version_id=' + versionID);
            assert.equal(detail.version.output_rows, '10');
            const versions = await call(artifact + '/versions');
            assert.ok(versions.items.some((v) => v.id === versionID));
            const rows = await call(artifact + '/query', { version_id: versionID, limit: 20 });
            assert.deepEqual(
              rows.items.map((r) => Number(r.value)),
              [0, 2, 4, 6, 8, 10, 12, 14, 16, 18],
            );
            await call(artifact + '/lineage', {
              version_id: versionID,
              outputs: ['double'],
              output_range,
              include_samples: true,
            });
          });
          if (!built) continue;
          await track(scope + ' 同一版本成果比较与明细', async () => {
            const run = await call(a + '/comparisons', {
              ...meta(),
              left_version_id: versionID,
              right_version_id: versionID,
            });
            const completed = await wait(d + '/runs/' + run.id, (v) => v.job.state, 'succeeded');
            assert.equal(completed.comparison.difference_count, '0');
            const diff = await call(a + '/comparisons/' + run.id + '/query', { limit: 20 });
            assert.deepEqual(diff.items, []);
          });
          await track(scope + ' 取消待办后重试固定版本导出', async () => {
            let ex;
            await withQueuesStopped(async () => {
              ex = await call(artifact + '/exports', {
                ...meta(),
                version_id: versionID,
                format: 'csv',
              });
              const path = a + '/exports/' + ex.id,
                queued = await call(path);
              assert.equal(queued.job.state, 'queued');
              const stopped = await call(path + '/cancel', meta(queued.job.revision));
              assert.equal(stopped.job.state, 'cancelled');
              await call(path + '/retry', meta(stopped.job.revision));
            });
            const path = a + '/exports/' + ex.id;
            await wait(path, (v) => v.job.state, 'succeeded');
            const manifest = await call(path + '/files');
            assert.ok(manifest.files.length);
            assert.equal(manifest.row_count, '10');
          });
          await track(scope + ' 运行取消与重试', async () => {
            let job;
            await withQueuesStopped(async () => {
              const run = await call(a + '/runs', { ...meta(), ...request });
              job = d + '/runs/' + run.id;
              const queued = await call(job);
              const canceled = await call(job + '/cancel', meta(queued.job.revision));
              assert.equal(canceled.job.state, 'cancelled');
              await call(job + '/retry', meta(canceled.job.revision));
            });
            await wait(job, (v) => v.job.state, 'succeeded');
          });
          await track(scope + ' 成果归档、历史读取和删除收据', async () => {
            const r = await call(artifact + '/archive', meta(detail.artifact.revision));
            assert.equal(r.artifact.status, 'archived');
            const cmd = meta(r.artifact.revision),
              gone = await call(artifact + '/delete', cmd);
            assert.equal(gone.replayed, false);
            assert.equal((await call(artifact + '/delete', cmd)).replayed, true);
            await assert.rejects(call(artifact), /404/);
          });
        }
      } finally {
        await pair.close();
      }
      return {
        passed: workflow.every((w) => w.passed),
        cases: pair.records.length,
        failures: pair.problems,
      };
    },
    { monitoringWorkers: true },
  );
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = await analysisParity(runDirectory(process.argv[2]));
  console.log(result);
  process.exitCode = result.passed ? 0 : 1;
}
