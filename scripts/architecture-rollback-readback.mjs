// Use one disposable database throughout the three rollback boundaries. Every
// publication enters through the real HTTP API and all reads use normal clients.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, expect } from '@playwright/test';
import {
  pairedCandidate,
  pairedRoleManifest,
  startPairedWorkers,
  withHTTPPair,
} from './architecture-http-sandbox.mjs';
import {
  client,
  digest,
  launch,
  read,
  root,
  runDirectory,
  sleep,
  write,
} from './architecture-lab.mjs';
import { httpOwners, splitWorkers } from './architecture-runtime.mjs';
import { verifySummaryPermissions } from './architecture-summary-permissions.mjs';
import { verifyRestrictedLookupPermissions } from './architecture-domain-permissions.mjs';
import {
  changeDisplayReader,
  configureChangeDisplay,
  startChangeDisplay,
} from './fixtures/architecture-change-display.mjs';
import { queueNotificationImport } from './fixtures/notifications-acceptance.mjs';
import { availablePort, stop, waitURL } from './fixtures/parsing-lab.mjs';

const execute = (bin, args, env = process.env) =>
  execFileSync(bin, args, { cwd: root, env, stdio: 'pipe', encoding: 'utf8' }).trim();
const reason = '在同一隔离数据库验证切流及原身份回读';
const overrides = (owners) => Object.fromEntries(owners.map((role) => [role, 'compat-api']));

async function setup(priv, cfg, ctl) {
  ctl.web = 'http://127.0.0.1:' + (await availablePort());
  ctl.runtime = await configureChangeDisplay(cfg);
  cfg.display.shell_directory = join(ctl.release.release, 'display-runtime');
  cfg.trusted_origins += ',' + ctl.web;
  cfg.public_console_url = ctl.web + '/';
  return {
    start: async (args) => {
      const { env, bin, endpoints, topology, children, roleEnv } = args;
      ctl.bin = bin;
      ctl.env = env;
      const gatewayManifest = join(priv, 'api.json');
      let gateway = children.findLast((child) => child.spawnfile === join(bin, 'api'));
      assert(gateway, 'isolated gateway process');
      endpoints['compat-api'] = 'https://127.0.0.1:' + (await availablePort());
      const compat = join(priv, 'compat-api.json');
      write(compat, pairedRoleManifest(topology, 'compat-api', endpoints));
      children.push(
        launch(
          join(bin, 'compat-api'),
          [],
          { ...env, ...roleEnv('compat-api'), INFRAPRO_ARCHITECTURE_FILE: compat },
          join(priv, 'compat-api.log'),
        ),
      );
      ctl.route = async (targets) => {
        await stop(gateway);
        write(gatewayManifest, {
          ...pairedRoleManifest(topology, 'api', endpoints),
          route_targets: targets,
        });
        gateway = launch(
          join(bin, 'api'),
          [],
          { ...env, ...roleEnv('api'), INFRAPRO_ARCHITECTURE_FILE: gatewayManifest },
          join(priv, 'gateway.log'),
        );
        children.push(gateway);
        await waitURL('http://' + cfg.http_addr + '/readyz');
      };
      await ctl.route(overrides(httpOwners));
      const profile = join(priv, 'pg-events.json');
      write(profile, { mode: 'pg', environment: 'arc_rollback' });
      const eventEnv = { ...env, INFRAPRO_EVENTS_FILE: profile };
      const events = (...cmd) => JSON.parse(execute(join(bin, 'event-relay'), cmd, eventEnv));
      events('enable', '--reason', reason);
      ctl.dispatch = (to) => {
        const result = [];
        for (const old of events('status').ownership) {
          if (old.mode !== to) {
            const change = (mode, expected) =>
              events(
                'switch',
                '--role',
                old.role,
                '--expected',
                String(expected),
                '--mode',
                mode,
                '--reason',
                reason,
              );
            change('draining', old.generation);
            change(to, old.generation + 1);
          }
          result.push({ role: old.role, previous: old.generation });
        }
        const actual = events('status').ownership;
        assert.equal(actual.length, result.length);
        assert(actual.every((x) => x.mode === to));
        return actual.map((x) => ({
          role: x.role,
          generation: x.generation,
          mode: x.mode,
          position: x.freeze_position,
        }));
      };
      await startPairedWorkers({
        ...args,
        env: eventEnv,
        side: 'a',
        privateDir: priv,
        legacy: ['monitoring-worker'],
        split: [],
        claimRoles: [],
      });
      ctl.startSplitWorkers = async () => {
        const workers = [];
        await startPairedWorkers({
          ...args,
          env: eventEnv,
          side: 'b',
          privateDir: priv,
          legacy: [],
          split: splitWorkers,
          claimRoles: [],
          controls: workers,
        });
        ctl.publisher = workers.find((w) => w.role === 'monitoring-publish-worker');
        ctl.publisherProcess = children.findLast(
          (child) => child.spawnfile === join(bin, 'monitoring-publish-worker'),
        );
      };
      await startChangeDisplay('b', priv, ctl.runtime, args);
      children.push(
        launch(
          process.execPath,
          [
            join(root, 'apps/console/node_modules/vite/bin/vite.js'),
            'preview',
            '--host',
            '127.0.0.1',
            '--outDir',
            join(ctl.release.release, 'console'),
          ],
          {
            ...process.env,
            API_PROXY_TARGET: 'http://' + cfg.http_addr,
            CONSOLE_PORT: new URL(ctl.web).port,
          },
          join(priv, 'console.log'),
          join(root, 'apps/console'),
        ),
      );
      await waitURL(ctl.web);
    },
  };
}

async function waitForBatch(user, path, wanted) {
  const end = Date.now() + 90000;
  while (Date.now() < end) {
    const row = await user.call(path);
    if (row.state === wanted) return row;
    if (['failed', 'invalid', 'cancelled', 'needs_action'].includes(row.state))
      throw Error('unexpected batch state ' + row.state);
    await sleep(200);
  }
  throw Error('batch did not reach ' + wanted);
}

async function publish(user, sample, phase, index, pausePublisher) {
  const name = 'rollback-' + phase + '.csv';
  const row = { at: '2026-11-08T00:0' + index + ':00Z', value: 31 + index };
  const queued = await queueNotificationImport(
    user,
    { tenants: [sample.tenant], channel: sample.channels.P1.id },
    name,
    'observed_at,value,quality\n' + row.at + ',' + row.value + ',good\n',
  );
  const path = '/tenants/' + sample.tenant + '/monitoring/imports/' + queued.batch;
  const ready = await waitForBatch(user, path, 'ready');
  await pausePublisher?.();
  const input = { request_id: randomUUID(), expected_revision: ready.revision, reason };
  const commandID = randomUUID();
  const submit = async () => {
    const reply = await user.client.post(user.api + path + '/publish', {
      headers: { ...user.headers, 'X-Command-ID': commandID },
      data: input,
    });
    try {
      assert(reply.ok(), 'publish HTTP ' + reply.status());
      return await reply.json();
    } finally {
      await reply.dispose();
    }
  };
  const first = await submit();
  assert.match(first.job_id, /^[a-f0-9-]{36}$/);
  if (!pausePublisher) await waitForBatch(user, path, 'published');
  return {
    name,
    phase,
    row,
    path,
    batch: queued.batch,
    publishJob: first.job_id,
    validateJob: queued.job,
    request: input.request_id,
    command: commandID,
    submit,
  };
}

// Exercise the installed RLS in the replay's database after its workers have
// drained. The entire simulation, including its temporary functions, rolls back.
export function verifyCompatibilityLeases(sourceRun, out) {
  const lab = read(join(sourceRun, 'private/lab.json'));
  const snapshot = read(join(sourceRun, 'snapshot.json'));
  assert.match(lab.id, /^[0-9a-f]{12}$/);
  assert.equal(lab.b.database, 'arc_http_' + lab.id + '_b');
  const fixturePath = 'scripts/fixtures/architecture-compat-leases.sql';
  const fixture = readFileSync(join(root, fixturePath), 'utf8');
  const file = join(out, 'deploy/compat-lease-check.json');
  const check = {
    sourceRun,
    candidateSHA256: snapshot.candidateSHA256,
    policySHA256: snapshot.appliedRoleSQL['deploy/events/postgres-grants.sql'],
    fixture: fixturePath,
    fixtureSHA256: digest(fixture),
    startedAt: new Date().toISOString(),
    passed: false,
    cases: [],
  };
  try {
    const output = execFileSync(
      'docker',
      ['exec', '-i', lab.container, 'psql', '-XAtq', '-v', 'ON_ERROR_STOP=1', '-U', 'archtest', '-d', lab.b.database],
      { input: 'BEGIN;\n' + fixture, stdio: 'pipe', encoding: 'utf8' },
    );
    check.cases = output.split('\n')
      .filter((line) => line.startsWith('{'))
      .map((line) => JSON.parse(line));
    const roles = [
      'infrapro_compat_monitoring_worker',
      'infrapro_compat_ingest_worker',
      'infrapro_compat_api',
    ];
    for (const role of roles) {
      const stages = check.cases.filter((row) => row.role === role).map((row) => row.stage).sort();
      assert.deepEqual(stages, ['compat-after', 'compat-before', 'frozen', 'invalid-ticket', 'split']);
    }
    check.passed = true;
  } catch (error) {
    const row = String(error.stderr || '').split('\n').find((s) => s.includes('ERROR:'));
    check.error = (row || error.message.split('\n')[0]).slice(0, 240);
    throw error;
  } finally {
    check.endedAt = new Date().toISOString();
    write(file, check);
  }
  return check;
}

export async function verifyRollbackReadback(seed, out = seed) {
  const ctl = { release: pairedCandidate(seed) };
  const file = join(out, 'deploy/rollback-readback.json');
  if (existsSync(file)) {
    const prior = read(file);
    const archive = join(out, 'deploy/rollback-history');
    mkdirSync(archive, {recursive:true,mode:0o700});
    copyFileSync(file, join(archive, (prior.startedAt || new Date().toISOString()).replaceAll(':','-')+'.json'));
  }
  const report = {
    arc: 'ARC-035',
    startedAt: new Date().toISOString(),
    status: 'starting',
    phases: [],
  };
  write(file, report);
  try {
    const verified = await withHTTPPair(
      seed,
      'rollback-readback',
      async (dir) => {
        const lab = read(join(dir, 'private/lab.json')),
          sample = read(join(dir, 'samples.json')).structure;
        assert.match(lab.id, /^[0-9a-f]{12}$/);
        const db = 'arc_http_' + lab.id + '_b';
        assert.equal(lab.b.database, db);
        const sql = (q) =>
          execute('docker', [
            'exec',
            lab.container,
            'psql',
            '-v',
            'ON_ERROR_STOP=1',
            '-XqAt',
            '-U',
            'archtest',
            '-d',
            db,
            '-c',
            q,
          ]);
        const account = read(join(dir, 'private/accounts.json')).admin;
        const user = await client(lab.b, account);
        const shots = join(out, 'deploy/rollback-gui', lab.id);
        mkdirSync(shots, { recursive: true, mode: 0o700 });
        const snapshot = read(join(dir, 'snapshot.json'));
        const shellSHA = digest(readFileSync(join(ctl.release.release, 'console/index.html')));
        assert.equal(shellSHA, ctl.release.staticFiles['console/index.html']);
        Object.assign(report, {
          sourceRun: dir,
          candidateSHA256: snapshot.candidateSHA256,
          role_grants: snapshot.appliedRoleSQL,
          console_index_sha256: shellSHA,
          status: 'running',
        });
        const save = () => {
          write(file, report);
          write(join(dir, 'rollback-readback.json'), report);
        };
        let browser, display;
        try {
          browser = await chromium.launch({ headless: true });
          const web = await browser.newContext({ storageState: await user.client.storageState() });
          const page = await web.newPage();
          display = await changeDisplayReader(lab.b, ctl.runtime, sample, account, []);
          const committed = [];
          const range = { from: '2026-11-08T00:00:00Z', to: '2026-11-08T00:04:00Z' };
          const prior = await user.call(
            '/tenants/' + sample.tenant + '/monitoring/imports/' + sample.batch,
          );
          const priorJob = sql(
            `SELECT id FROM monitoring_jobs WHERE tenant_id='${sample.tenant}' AND target_id='${sample.batch}' AND kind='publish' AND state='succeeded'`,
          );
          assert.match(priorJob, /^[a-f0-9-]{36}$/);
          const phases = ['before-cutover', 'partial-http', 'after-worker-switch'];
          for (const [i, phase] of phases.entries()) {
            const phaseStarted = new Date().toISOString();
            const session = await display.renew();
            const routes =
              i === 0
                ? overrides(httpOwners)
                : i === 1
                  ? overrides(['identity-api', 'transfer-api'])
                  : {};
            await ctl.route(routes);
            const ownership = ctl.dispatch(i === 2 ? 'split' : 'compat');
            if (i === 2) await ctl.startSplitWorkers();
            const item = await publish(
              user,
              sample,
              phase,
              i,
              i === 2 ? ctl.publisher.stop : undefined,
            );
            committed.push(item);
            const row = {
              phase,
              startedAt: phaseStarted,
              display_session: session,
              acceptedWith: { routes, ownership },
              accepted: {
                at: new Date().toISOString(),
                batch: item.batch,
                publish_job: item.publishJob,
                validate_job: item.validateJob,
                request: item.request,
                command: item.command,
              },
              readbacks: [],
            };
            row.accepted.state = sql(
              `SELECT state FROM monitoring_jobs WHERE id='${item.publishJob}'`,
            );
            assert.equal(row.accepted.state, i === 2 ? 'queued' : 'succeeded');
            if (i === 2) {
              assert(ctl.publisherProcess.exitCode !== null || ctl.publisherProcess.signalCode);
              row.splitPublisherAtHandoff = {
                pid: ctl.publisherProcess.pid,
                exit: ctl.publisherProcess.exitCode,
                signal: ctl.publisherProcess.signalCode,
              };
            }
            report.phases.push(row);
            save();
            row.rolledBackTo = ctl.dispatch('compat');
            await ctl.route(overrides(httpOwners));
            row.rolledBackAt = new Date().toISOString();
            await display.renew();
            await waitForBatch(user, item.path, 'published');
            await page.goto(ctl.web + '/t/' + sample.tenant + '/monitoring/imports');
            await expect(page.getByText(prior.name, { exact: true }).first()).toBeVisible({
              timeout: 20000,
            });
            const legacy = JSON.parse(
              execute(
                join(ctl.bin, 'infrapro'),
                ['monitoring', 'jobs', 'inspect', '--tenant', sample.tenant, '--id', priorJob],
                ctl.env,
              ),
            );
            assert.equal(legacy.id, priorJob);
            assert.equal(legacy.state, 'succeeded');
            const priorScreen = await display.read({
              from: '2026-11-01T00:00:00Z',
              to: '2026-11-01T00:11:00Z',
            });
            assert.deepEqual(
              priorScreen.rows.map((x) => x.value).sort((a, b) => a - b),
              Array.from({ length: 10 }, (_, n) => n),
            );
            row.frozen = {
              batch: sample.batch,
              gui: prior.name,
              cli: { job: legacy.id, state: legacy.state },
              sdk: priorScreen,
            };
            for (const accepted of committed) {
              const gui = page.getByText(accepted.name, { exact: true }).first();
              await expect(gui).toBeVisible({ timeout: 20000 });
              const replay = await accepted.submit();
              assert.equal(replay.job_id, accepted.publishJob);
              const jobs = [accepted.validateJob, accepted.publishJob].map((id) => {
                const job = JSON.parse(
                  execute(
                    join(ctl.bin, 'infrapro'),
                    ['monitoring', 'jobs', 'inspect', '--tenant', sample.tenant, '--id', id],
                    ctl.env,
                  ),
                );
                assert.equal(job.id, id);
                assert.equal(job.state, 'succeeded');
                return { id: job.id, kind: job.kind, state: job.state, generation: job.generation };
              });
              const effects = JSON.parse(
                sql(`SELECT jsonb_build_object(
            'receipt',(SELECT jsonb_agg(jsonb_build_object('request',request_id,'job',result->>'job_id','sha256',payload_hash)) FROM monitoring_receipts WHERE tenant_id='${sample.tenant}' AND request_id='${accepted.request}'),
            'publication',(SELECT jsonb_agg(jsonb_build_object('id',id,'position',sequence)) FROM monitoring_publications WHERE tenant_id='${sample.tenant}' AND batch_id='${accepted.batch}'),
            'changes',(SELECT jsonb_agg(jsonb_build_object('id',c.id,'position',c.source_position,'outbox',o.event_id)) FROM monitoring_changes c JOIN monitoring_publications p ON p.tenant_id=c.tenant_id AND p.id=c.source_id JOIN event_outbox o ON o.aggregate_id=c.id AND o.type='monitoring.changed' WHERE p.batch_id='${accepted.batch}'),
            'audit',(SELECT jsonb_agg(id::text) FROM account_security_events WHERE object_id::text='${accepted.batch}' AND event_type='monitoring.imports.published'),
            'notifications',(SELECT jsonb_agg(jsonb_build_object('id',id,'outcome',outcome,'job',task_reference->>'source_root_id') ORDER BY outcome) FROM notification_events WHERE target_id='${accepted.batch}' AND source_module='monitoring' AND event_type='task.result'),
            'jobs',(SELECT count(*) FROM monitoring_jobs WHERE tenant_id='${sample.tenant}' AND target_id='${accepted.batch}' AND kind='publish'))`),
              );
              assert.equal(effects.publication.length, 1);
              assert.equal(effects.jobs, 1);
              assert.equal(effects.changes.length, 1);
              assert.equal(effects.receipt.length, 1);
              assert.equal(effects.receipt[0].job, accepted.publishJob);
              assert.equal(effects.changes[0].position, effects.publication[0].position);
              assert.equal(effects.audit.length, 1);
              assert.deepEqual(
                effects.notifications.map((x) => x.job).sort(),
                [accepted.publishJob, accepted.validateJob].sort(),
              );
              if (accepted.effects) assert.deepEqual(effects, accepted.effects);
              accepted.effects = effects;
              row.readbacks.push({
                batch: accepted.batch,
                gui: await gui.textContent(),
                replayed_job: replay.job_id,
                cli: jobs,
                effects,
              });
            }
            const screen = await display.read(range),
              values = screen.rows.map((x) => x.value).sort((a, b) => a - b);
            assert.deepEqual(
              values,
              committed.map((x) => x.row.value),
            );
            row.sdk = { query: screen.query, rows: screen.rows };
            const screenshot = join(shots, phase + '.png');
            await page.screenshot({ path: screenshot, animations: 'disabled' });
            row.screenshot = 'deploy/rollback-gui/' + lab.id + '/' + basename(screenshot);
            row.verifiedAt = new Date().toISOString();
            save();
          }
          report.status = 'passed';
          report.endedAt = new Date().toISOString();
          save();
          return report;
        } catch (error) {
          report.status = 'failed';
          report.error = error.message.split('\n')[0].slice(0, 240);
          report.endedAt = new Date().toISOString();
          save();
          throw error;
        } finally {
          await display?.close();
          await browser?.close();
          await user.client.dispose();
        }
      },
      {
        restrictedRoles: [
          ...httpOwners,
          ...splitWorkers,
          'monitoring-worker',
          'compat-api',
          'display-runtime',
          'display-assets',
          'display-package-worker',
        ],
        configure: async ({ side, cfg, privateDir }) =>
          side === 'b' ? setup(privateDir, cfg, ctl) : undefined,
      },
    );
    const leases = verifyCompatibilityLeases(verified.sourceRun, out);
    verified.compatibilityLeases = {
      file: 'deploy/compat-lease-check.json',
      cases: leases.cases.length,
    };
    verified.summaryRepairGrants = verifySummaryPermissions(verified, out);
    const recoveredLab=read(join(verified.sourceRun,'private/lab.json'));
    const readDatabase=query=>execFileSync('docker',['exec','-i',recoveredLab.container,'psql',
      '-v','ON_ERROR_STOP=1','-XqAt','-U','archtest','-d',recoveredLab.b.database],
    {input:query,encoding:'utf8',stdio:'pipe'}).trim();
    // This paired snapshot has real facilities and device history, in addition
    // to the sparse database used to verify fresh role startup in ARC-032.
    verified.sourceMetadataGrants=verifyRestrictedLookupPermissions(readDatabase,readDatabase('SELECT id FROM accounts ORDER BY id LIMIT 1'));
    assert(verified.sourceMetadataGrants.auditEngineeringGraph.rows>0,'rollback snapshot must retain readable facility history');
    write(file, verified);
    write(join(verified.sourceRun, 'rollback-readback.json'), verified);
    return verified;
  } catch (error) {
    if (report.status !== 'failed') {
      report.status = 'failed';
      report.error = error.message
        .split('\n')[0]
        .replace(/https?:\/\/\S+/g, '[local endpoint]')
        .slice(0, 240);
      report.endedAt = new Date().toISOString();
      write(file, report);
    }
    throw error;
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const directory = runDirectory(process.argv[2]);
  await verifyRollbackReadback(
    directory,
    process.argv[3] ? runDirectory(process.argv[3]) : directory,
  );
}
