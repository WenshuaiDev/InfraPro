import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { createRequire } from 'node:module';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, runDirectory, write } from './architecture-lab.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';

const xlsx = createRequire(new URL('../apps/console/package.json', import.meta.url))('xlsx');
const meta = (expected_revision = '0') => ({
  request_id: randomUUID(),
  expected_revision,
  reason: '以原数据核对监测发布、撤销及读取',
});
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const measureWindow = { from: '2026-11-01T00:00:00Z', to: '2026-11-01T00:12:00Z' };
function observations(format, time) {
  const rows = [
    ['observed_at', 'reading', 'quality'],
    [time, '12.75', 'good'],
  ];
  if (format === 'csv') return Buffer.from(rows.map((r) => r.join(',')).join('\n') + '\n');
  const book = xlsx.utils.book_new();
  xlsx.utils.book_append_sheet(book, xlsx.utils.aoa_to_sheet(rows), 'Observations');
  return xlsx.write(book, { type: 'buffer', bookType: 'xlsx' });
}
export function monitoringParity(dir) {
  return withHTTPPair(
    dir,
    'monitoring',
    async (fixture, { withQueuesStopped }) => {
      const pair = new PairedHTTP(fixture, 'monitoring', dir),
        sample = read(join(fixture, 'samples.json')).structure;
      const { client, api, call, headers } = await pair.login(
        read(join(fixture, 'private/accounts.json')).admin,
      );
      const tenant = '/tenants/' + sample.tenant,
        project = tenant + '/projects/' + sample.project,
        channel = sample.channels.P1.id;
      const workflows = [];
      async function exercise(name, arcs, fn) {
        const from = pair.records.length;
        let issue;
        try {
          await fn();
        } catch (e) {
          issue = e.message.split('\n')[0].slice(0, 260);
          pair.problems.push({ name, error: issue });
        }
        const rows = pair.records.slice(from);
        for (const arc of arcs)
          workflows.push({
            arc,
            name,
            passed: !issue && rows.every((x) => !x.differences.length),
            caseIds: rows.map((x) => x.command),
          });
        pair.save();
        write(join(dir, 'http/workflows/monitoring.json'), { cases: workflows });
      }
      const wait = (path, state) => client.wait(api + path, headers, (v) => v.state, state);
      const accessPath = project + '/monitoring-access';
      let grantID;
      async function removeGrant(grant) {
        const path = accessPath + '/' + grant.id;
        const preview = await call(path + '/revoke-preview', { expected_revision: grant.revision });
        return call(path + '/revoke', {
          ...meta(grant.revision),
          preview_hash: preview.preview_hash,
        });
      }
      async function download(path, want) {
        const r = await client.get(api + path, { headers });
        assert.equal(r.status(), 200);
        const bytes = await r.body();
        assert.equal(hash(bytes), want);
        assert.equal(Number(r.headers()['content-length']), bytes.length);
        await r.dispose();
      }
      try {
        await exercise('项目数据授权与工程授权分别复核', ['ARC-005', 'ARC-010'], async () => {
          const old = (await call(accessPath)).items.find(
            (g) => g.channel_id === channel && g.effective,
          );
          assert.ok(old);
          const revoked = await removeGrant(old);
          const next = await call(accessPath, {
            ...meta(revoked.access_revision),
            channel_id: channel,
            resource_grant_id: old.resource_grant_id,
            range: { from: null, to: null },
            capabilities: ['read', 'source', 'history', 'export', 'import', 'revoke'],
          });
          grantID = next.id;
        });
        for (const [scope, base, format, time] of [
          ['tenant', tenant, 'csv', '2026-11-02T00:00:00Z'],
          ['project', project, 'xlsx', '2026-11-03T00:00:00Z'],
        ]) {
          const m = base + '/monitoring',
            imports = m + '/imports';
          async function upload(bytes) {
            let b = await call(imports, {
              ...meta(),
              name: 'check.' + format,
              size_bytes: String(bytes.length),
            });
            const path = imports + '/' + b.id;
            const part = await client.put(api + path + '/upload/parts/1', {
              headers: {
                ...headers,
                'Content-Type': 'application/octet-stream',
                'X-Expected-Revision': b.revision,
                'X-Content-SHA256': hash(bytes),
              },
              data: bytes,
            });
            assert.equal(part.status(), 200);
            await part.dispose();
            b = await call(path + '/upload/complete', { ...meta(b.revision), sha256: hash(bytes) });
            const mapping = {
              format,
              mode: 'wide',
              ...(format === 'csv'
                ? { encoding: 'utf-8', delimiter: ',' }
                : { sheet: 'Observations' }),
              header_row: 1,
              time_column: 'observed_at',
              quality_column: 'quality',
              timezone: 'UTC',
              empty_wide: 'error',
              channels: [{ alias: 'x', value_column: 'reading', channel_id: channel, unit: 'mm' }],
            };
            b = await call(path + '/mappings', { ...meta(b.revision), mapping });
            return { b, path };
          }
          await exercise(
            scope + ' 冻结观测、游标、热统计及报表',
            ['ARC-010', 'ARC-013'],
            async () => {
              const selection = await call(m + '/selections', { channel_ids: [channel] });
              assert.equal(selection.channel_count, '1');
              const query = {
                selection_id: selection.id,
                range: measureWindow,
                limit: 4,
                include_total: true,
              };
              const details = await call(m + '/queries/details', query);
              assert.equal(details.items.length, 4);
              assert.equal(details.total, 10);
              assert.deepEqual(
                details.items.map((x) => Number(x.value)),
                [9, 8, 7, 6],
              );
              const renewed = await call(m + '/snapshots/' + details.snapshot.id + '/renew', query);
              assert.equal(renewed.id, details.snapshot.id);
              for (const kind of ['latest', 'overview', 'statistics', 'trends']) {
                const result = await call(m + '/queries/' + kind, {
                  ...query,
                  ...(kind === 'statistics' && { period: 'day' }),
                  ...(kind === 'trends' && { plot_point_budget: 32 }),
                });
                assert.equal(result.state, 'ready');
              }
              const exp = await call(m + '/exports', {
                ...meta(),
                kind: 'details',
                format: 'csv',
                query: { selection_id: selection.id, range: measureWindow },
              });
              await wait(m + '/jobs/' + exp.job_id, 'succeeded');
              const manifest = await call(m + '/exports/' + exp.id + '/files');
              assert.equal(manifest.row_count, '10');
              assert.ok(manifest.files.length > 0);
              for (const file of manifest.files)
                await download(
                  m + '/exports/' + exp.id + '/files/' + file.part + '/download',
                  file.sha256,
                );
            },
          );
          await exercise(
            scope + ' ' + format + ' 上传、重复、撤销及重采',
            ['ARC-009', 'ARC-013'],
            async () => {
              const bytes = observations(format, time);
              const { b, path } = await upload(bytes);
              const input = meta(b.revision);
              const queued = await call(path + '/validate', input);
              assert.equal((await call(path + '/validate', input)).job_id, queued.job_id);
              let batch = await wait(path, 'ready');
              await call(path + '/publish', meta(batch.revision));
              batch = await wait(path, 'published');
              await download(
                m + '/source-files/' + batch.source_file_id + '/download',
                hash(bytes),
              );
              const repeated = await upload(bytes);
              await call(repeated.path + '/retry', meta(repeated.b.revision));
              const conflict = await wait(repeated.path, 'needs_resolution'),
                issues = await call(repeated.path + '/issues');
              assert.equal(conflict.counts.duplicates, '1');
              await call(repeated.path + '/resolutions', {
                ...meta(conflict.revision),
                preview_hash: issues.preview_hash,
                confirm_same_observation: true,
              });
              const revoke = m + '/batches/' + batch.id;
              const preview = await call(revoke + '/revoke-preview', {
                expected_revision: batch.revision,
              });
              await call(revoke + '/revoke', {
                ...meta(batch.revision),
                preview_hash: preview.preview_hash,
              });
              const previous = await call(path);
              assert.equal(previous.state, 'revoked');
              const adopted = await call(path + '/readopt', {
                ...meta(previous.revision),
                all_originally_adopted: true,
              });
              assert.notEqual(adopted.id, batch.id);
            },
          );
          await exercise(scope + ' 队列取消与导入取消', ['ARC-009', 'ARC-014'], async () => {
            await withQueuesStopped(async () => {
              const { b, path } = await upload(observations(format, time));
              const queued = await call(path + '/validate', meta(b.revision));
              const job = m + '/jobs/' + queued.job_id,
                current = await call(job);
              assert.equal(current.state, 'queued');
              assert.equal(
                (await call(job + '/cancel', meta(current.revision))).state,
                'cancelled',
              );
            });
            const empty = await call(imports, {
              ...meta(),
              name: 'unfinished.csv',
              size_bytes: '4',
            });
            assert.equal(
              (await call(imports + '/' + empty.id + '/cancel', meta(empty.revision))).state,
              'cancelled',
            );
          });
        }
        if (grantID)
          await exercise('撤销数据后项目读取受限', ['ARC-005', 'ARC-010'], async () => {
            const m = project + '/monitoring',
              selection = await call(m + '/selections', { channel_ids: [channel] });
            const query = { selection_id: selection.id, range: measureWindow, limit: 2 },
              first = await call(m + '/queries/details', query);
            const grant = (await call(accessPath)).items.find((g) => g.id === grantID);
            await removeGrant(grant);
            const r = await client.post(api + m + '/queries/details', {
              headers,
              data: {
                ...query,
                snapshot_id: first.snapshot.id,
                cursor: first.next_cursor,
                direction: 'forward',
              },
            });
            assert.ok([403, 404, 409].includes(r.status()));
            await r.dispose();
          });
      } finally {
        await pair.close();
      }
      return {
        passed: workflows.every((w) => w.passed),
        cases: pair.records.length,
        failures: pair.problems,
      };
    },
    { monitoringWorkers: true },
  );
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = await monitoringParity(runDirectory(process.argv[2]));
  console.log(result);
  process.exitCode = result.passed ? 0 : 1;
}
