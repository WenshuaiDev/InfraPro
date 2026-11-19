import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { analysisBody } from './architecture-analysis-parity.mjs';
import { read, runDirectory, write } from './architecture-lab.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';

const meta = (expected_revision = '0') => ({
  request_id: randomUUID(),
  expected_revision,
  reason: '固定窗口派生配置、回算与依据验收',
});
const toNS = (date) => (BigInt(Date.parse(date)) * 1000000n).toString();
const window = { from_ns: toNS('2026-11-01T00:00:00Z'), to_ns: toNS('2026-11-01T00:10:00Z') };
const expected = Array.from({ length: 10 }, (_, i) => i * 2);
const caps = ['read', 'history', 'export'];

export function derivedParity(dir) {
  return withHTTPPair(
    dir,
    'derived',
    async (fixture, { withQueuesStopped, withInteractiveSlot }) => {
      const p = new PairedHTTP(fixture, 'derived', dir),
        s = read(join(fixture, 'samples.json')).structure;
      const { call, client, api, headers } = await p.login(
        read(join(fixture, 'private/accounts.json')).admin,
      );
      const wait = (path, pick, want) => client.wait(api + path, headers, pick, want);
      async function shortTrial(path, revision, config) {
        return withInteractiveSlot(async () => {
          const response = await client.post(api + path + '/trials', {
            headers,
            data: {
              ...meta(revision),
              config_id: config.id,
              config_hash: config.config_hash,
              output_range: window,
            },
          });
          try {
            assert.equal(response.status(), 200, 'Ten-row trial with a free interactive slot');
            const run = await response.json();
            assert.equal(run.job.state, 'succeeded');
            return run;
          } finally {
            await response.dispose();
          }
        });
      }
      const tenant = '/tenants/' + s.tenant,
        project = tenant + '/projects/' + s.project,
        grants = project + '/monitoring-access';
      const structural = project + '/resource-access/facilities/' + s.nodes.A.id;
      const dm = tenant + '/monitoring/derived',
        metricCatalog = dm + '/metric-definitions';
      const { rule, input_bindings } = analysisBody(s),
        workflow = [];
      let def, version;
      async function scenario(name, action) {
        const start = p.records.length;
        let error;
        try {
          await action();
        } catch (e) {
          error = e.message.split('\n')[0].slice(0, 300);
          p.problems.push({ name, error });
        }
        const cases = p.records.slice(start);
        workflow.push({
          arc: 'ARC-011',
          name,
          passed: !error && cases.length > 0 && cases.every((r) => !r.differences.length),
          caseIds: cases.map((r) => r.command),
        });
        write(join(dir, 'http/workflows/derived.json'), { cases: workflow });
        p.save();
        return !error;
      }
      async function authorize(channel, resourceGrant) {
        const existing = await call(grants);
        const old = existing.items.find((g) => g.effective && g.channel_id === channel);
        let revision = existing.access_revision;
        if (old) {
          const revoke = grants + '/' + old.id,
            preview = await call(revoke + '/revoke-preview', { expected_revision: old.revision });
          revision = (
            await call(revoke + '/revoke', {
              ...meta(old.revision),
              preview_hash: preview.preview_hash,
            })
          ).access_revision;
        }
        return call(grants, {
          ...meta(revision),
          channel_id: channel,
          resource_grant_id: resourceGrant,
          range: { from: null, to: null },
          capabilities: caps,
        });
      }
      try {
        await scenario('租户派生指标草稿及版本生命周期', async () => {
          def = (
            await call(metricCatalog, { ...meta(), code: 'PARITY_DOUBLE', name: '位移双倍', rule })
          ).definition;
          const path = metricCatalog + '/' + def.id;
          def = (
            await call(
              path + '/draft',
              { ...meta(def.revision), name: '位移双倍核验', rule },
              'PATCH',
            )
          ).definition;
          const v = await call(path + '/validate', { draft_revision: def.draft.revision });
          version = (
            await call(path + '/publish', {
              ...meta(def.revision),
              draft_revision: def.draft.revision,
              validation_hash: v.validation_hash,
            })
          ).version;
          const vp = dm + '/metric-versions/' + version.id;
          const off = await call(vp + '/state', {
            ...meta(version.state_revision),
            enabled: false,
          });
          assert.equal(off.version.usable, false);
          version = (
            await call(vp + '/state', { ...meta(off.version.state_revision), enabled: true })
          ).version;
          assert.equal(version.usable, true);
          const copy = (
            await call(path + '/copy', {
              ...meta(),
              source_version_id: version.id,
              code: 'PARITY_COPY',
              name: '可删除副本',
            })
          ).definition;
          assert.equal(
            (await call(metricCatalog + '/' + copy.id + '/draft', meta(copy.revision), 'DELETE'))
              .definition.status,
            'deleted',
          );
        });
        await scenario('项目点位计算委托与直接通道授权', async () => {
          const detail = await call(structural),
            point = detail.grants.find((g) => g.resource_id === s.nodes.P1.id),
            raw = detail.grants.find((g) => g.resource_id === s.channels.P1.id);
          assert.ok(point && raw);
          const change = {
            request_id: randomUUID(),
            reason: '按点位明确授予派生通道创建资格',
            action: 'change_grants',
            expected_access_revision: detail.policy.revision,
            expected_link_revision: detail.link.revision,
            update: [
              {
                grant_id: point.id,
                expected_revision: point.revision,
                capabilities: [...new Set([...point.capabilities, 'create_derived_channel'])],
              },
            ],
          };
          const preview = await call(structural + '/preview', change);
          await call(structural + '/changes', { ...change, preview_hash: preview.preview_hash });
          await authorize(s.channels.P1.id, raw.id);
        });
        for (const [scope, base] of [
          ['tenant', tenant],
          ['project', project],
        ]) {
          const d = base + '/monitoring/derived';
          if (!version) break;
          await scenario(scope + ' 指标版本和方法模板', async () => {
            const detail = await call(d + '/metric-definitions/' + def.id);
            assert.equal(detail.id, def.id);
            const versions = await call(d + '/metric-definitions/' + def.id + '/versions');
            assert.ok(versions.items.some((v) => v.id === version.id));
            assert.equal((await call(d + '/metric-versions/' + version.id)).id, version.id);
            const expanded = await call(d + '/templates/difference/expand', {
              inputs: rule.inputs,
            });
            assert.ok(expanded.nodes.length);
          });
          let path, channel, cfg, generation;
          const configured = await scenario(
            scope + ' 派生通道创建、试算、启用及正式代次',
            async () => {
              channel = (
                await call(d + '/channels', {
                  ...meta(),
                  code: 'PARITY_' + scope.toUpperCase(),
                  name: '位移双倍 ' + scope,
                  owner: { kind: 'point', id: s.nodes.P1.id, facility_id: s.nodes.A.id },
                  definition_version_id: version.id,
                  input_bindings,
                  parameters: {},
                })
              ).channel;
              path = d + '/channels/' + channel.resource.id;
              assert.equal((await call(path)).resource.id, channel.resource.id);
              await call(path + '/dependencies');
              cfg = (await call(path + '/configs')).items[0];
              assert.ok(cfg.id);
              if (scope === 'project') {
                const structuralGrant = (await call(structural)).grants.find(
                  (g) => g.resource_id === channel.resource.id,
                );
                assert.ok(structuralGrant);
                await authorize(channel.resource.id, structuralGrant.id);
              }
              const trial = await shortTrial(path, channel.config_revision, cfg);
              const checked = await wait(d + '/runs/' + trial.id, (v) => v.job.state, 'succeeded');
              assert.equal(checked.trial.output_count, '10');
              const preview = await call(path + '/control-preview', {
                action: 'enable',
                expected_revision: channel.control_revision,
                auto_start_ns: window.from_ns,
                stop_ns: window.to_ns,
                trial_receipt_id: checked.trial.trial_receipt_id,
              });
              channel = (
                await call(path + '/control', {
                  ...meta(channel.control_revision),
                  preview_id: preview.id,
                  preview_hash: preview.hash,
                })
              ).channel;
              const list =
                path + '/generations?from_ns=' + window.from_ns + '&to_ns=' + window.to_ns;
              generation = (await wait(list, (v) => v.items.length > 0, true)).items[0].id;
              const rows = await call(d + '/generations/' + generation + '/query', {
                output_range: window,
                limit: 20,
              });
              assert.deepEqual(
                rows.items.map((v) => Number(v.value)),
                expected,
              );
            },
          );
          if (!configured) continue;
          const gen = d + '/generations/' + generation;
          await scenario(scope + ' 谱系、历史重放及证据导出', async () => {
            const lineage = await call(gen + '/lineage', { output_range: window });
            assert.equal(lineage.config.id, cfg.id);
            const replay = await call(gen + '/replay', { ...meta(), output_range: window });
            const result = await wait(d + '/runs/' + replay.id, (v) => v.job.state, 'succeeded');
            assert.equal(result.historical_replay.difference_count, '0');
            let ex;
            await withQueuesStopped(async () => {
              const created = await call(gen + '/exports', {
                ...meta(),
                format: 'csv',
                output_range: window,
              });
              ex = d + '/evidence/exports/' + created.id;
              const queued = await call(ex);
              assert.equal(queued.job.state, 'queued');
              const cancelled = await call(ex + '/cancel', meta(queued.job.revision));
              assert.equal(cancelled.job.state, 'cancelled');
              await call(ex + '/retry', meta(cancelled.job.revision));
            });
            await wait(ex, (v) => v.job.state, 'succeeded');
            const manifest = await call(ex + '/files');
            assert.ok(manifest.files.length);
            assert.ok(Number(manifest.row_count) >= 10);
          });
          await scenario(scope + ' 配置更正、分配记录及回算', async () => {
            const saved = await call(path + '/configs', {
              ...meta(channel.config_revision),
              base_config_id: cfg.id,
              definition_version_id: version.id,
              input_bindings,
              parameters: {},
              numeric_contract: { scale: 8, rounding: 'half_even' },
              change_kind: 'ordinary',
            });
            const next = saved.config;
            const t = await shortTrial(path, channel.config_revision, next);
            const trial = await wait(d + '/runs/' + t.id, (v) => v.job.state, 'succeeded');
            const pv = await call(path + '/config-preview', {
              expected_revision: channel.config_revision,
              config_id: next.id,
              mode: 'correction',
              output_range: window,
              trial_receipt_id: trial.trial.trial_receipt_id,
            });
            channel = (
              await call(path + '/apply-config', {
                ...meta(channel.config_revision),
                preview_id: pv.id,
                preview_hash: pv.hash,
              })
            ).channel;
            const assignments = await call(path + '/assignments');
            assert.ok(assignments.items.some((v) => v.config_id === next.id));
            const backfill = await call(path + '/backfill-preview', {
              expected_revision: channel.config_revision,
              output_range: window,
            });
            const job = await call(path + '/backfill', {
              ...meta(channel.config_revision),
              preview_id: backfill.id,
              preview_hash: backfill.hash,
            });
            assert.ok(job.runs.length);
            for (const run of job.runs)
              await wait(d + '/runs/' + run.id, (v) => v.job.state, 'succeeded');
            const stop = await call(path + '/control-preview', {
              expected_revision: channel.control_revision,
              action: 'pause',
            });
            await call(path + '/control', {
              ...meta(channel.control_revision),
              preview_id: stop.id,
              preview_hash: stop.hash,
            });
          });
        }
      } finally {
        await p.close();
      }
      return {
        passed: workflow.every((v) => v.passed),
        records: p.records.length,
        failures: p.problems,
      };
    },
    { monitoringWorkers: true },
  );
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = await derivedParity(runDirectory(process.argv[2]));
  console.log(result);
  process.exitCode = result.passed ? 0 : 1;
}
