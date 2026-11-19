// Short-lived backend-only pair cloned from the saved baseline. Its authorization
// revision and audit history cannot race other suites' logins or role changes.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { createInterface } from 'node:readline';
import { randomBytes, randomInt } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { launch, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { provision } from './architecture-postgres.mjs';
import { pairedPostgres } from './architecture-http-postgres.mjs';
import { availableRange } from './architecture-serve.mjs';
import { httpOwners, ingestionProfile, splitWorkers } from './architecture-runtime.mjs';
import { archiveHTTPStorage, httpStorage } from './architecture-http-storage.mjs';
import { stop, waitURL } from './fixtures/parsing-lab.mjs';

function run(cmd, args, opts = {}) {
  return execFileSync(cmd, args, {
    cwd: root,
    stdio: 'pipe',
    maxBuffer: 16 << 20,
    ...opts,
  });
}
export function pairedCandidate(seedDir) {
  const evaluated = join(seedDir, 'candidate.evaluated.json');
  const selected =
    process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE ||
    (existsSync(evaluated) && read(evaluated).directory);
  const dir = selected ? runDirectory(selected) : seedDir;
  const release = read(join(dir, 'candidate.json'));
  assert.equal(release.bin, join(dir, 'candidate-bin'));
  return release;
}
export function pairedRoleManifest(topology, role, endpoints) {
  const { replica_id, replica_slots, pg_connections, max_inflight } = read(
    join(topology, role, 'architecture.json'),
  );
  return {
    endpoints,
    cert_directory: join(topology, role, 'tls'),
    split_display: true,
    replica_id,
    replica_slots,
    pg_connections,
    max_inflight,
  };
}
export function pairedRoleConfigs(privateDir, cfg, container, restrictedRoles, sqlDirectory) {
  if (!restrictedRoles.length) return { roleEnv: () => ({}) };
  const roleFiles = new Map();
  const roleRoot = join(privateDir, 'credentials');
  for (const role of restrictedRoles)
    mkdirSync(join(roleRoot, 'roles', role), {
      recursive: true,
      mode: 0o700,
    });
  write(join(roleRoot, 'layout.json'), { mode: 'local' });
  const appliedRoleSQL = provision({
    url: cfg.database_url,
    container,
    directory: roleRoot,
    localLogins: true,
    sqlDirectory,
  });
  for (const role of restrictedRoles) {
    const runtime = join(roleRoot, 'roles', role, 'runtime.json');
    const login = read(join(roleRoot, 'roles', role, 'environment.json')).INFRAPRO_DATABASE_URL;
    assert.notEqual(new URL(login).username, new URL(cfg.database_url).username);
    const settings = { ...cfg, database_url: login };
    if (cfg.ingestion_file) {
      const profile = ingestionProfile(role, read(cfg.ingestion_file));
      if (profile) {
        settings.ingestion_file = join(roleRoot, 'roles', role, 'ingestion.json');
        write(settings.ingestion_file, profile);
      } else delete settings.ingestion_file;
      if (role === 'ingest-custody-worker' || role === 'transfer-api')
        settings.parsing = {
          ...cfg.parsing,
          enabled: 'false',
          url: '',
          token_file: '',
        };
    }
    write(runtime, settings);
    roleFiles.set(role, runtime);
  }
  const gateway = join(privateDir, 'gateway-runtime.json');
  write(
    gateway,
    Object.fromEntries(
      [
        'http_addr',
        'log_level',
        'public_console_url',
        'trusted_origins',
        'csrf_auth_key',
        'cookie_secure',
      ].map((k) => [k, cfg[k]]),
    ),
  );
  roleFiles.set('api', gateway);
  const roleEnv = (role) => {
    assert.ok(roleFiles.has(role), 'Missing isolated runtime for ' + role);
    return { INFRAPRO_CONFIG: roleFiles.get(role) };
  };
  return { roleEnv, appliedRoleSQL };
}
export async function startPairedWorkers({
  side,
  privateDir,
  bin,
  env,
  endpoints,
  topology,
  children,
  legacy,
  split,
  claimRoles = split,
  roleEnv = () => ({}),
  controls,
}) {
  const roles = side === 'a' ? legacy : split;
  const running = [];
  const workerEnv = { ...env };
  if (side === 'b' && claimRoles.length) {
    const events = join(privateDir, 'worker-events.json');
    write(events, {
      ...read(join(root, 'deploy/events/events.example.json')),
      mode: 'pg',
      environment: 'arc_http',
      nats_url: '',
      ca_file: '',
      cert_file: '',
      key_file: '',
      replicas: 1,
    });
    workerEnv.INFRAPRO_EVENTS_FILE = events;
    const admin = (...args) => run(join(bin, 'event-relay'), args, { env: workerEnv });
    admin('enable', '--reason', 'isolated HTTP pair');
    for (const role of claimRoles)
      admin(
        'switch',
        '--role',
        role,
        '--expected',
        '1',
        '--mode',
        'split',
        '--reason',
        'isolated HTTP pair',
      );
  }
  for (const role of roles) {
    const report = join(privateDir, role + '-report.json');
    const opts = {
      ...workerEnv,
      ...roleEnv(role),
      INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'arc-http',
      INFRAPRO_DIAGNOSTICS_NODE_ID: 'local',
      INFRAPRO_DIAGNOSTICS_REPLICA_ID: role,
      INFRAPRO_DIAGNOSTICS_REPORT_FILE: report,
    };
    if (side === 'b') {
      const manifest = join(privateDir, role + '.json');
      write(manifest, pairedRoleManifest(topology, role, endpoints));
      opts.INFRAPRO_ARCHITECTURE_FILE = manifest;
    }
    let child;
    const start = async () => {
      rmSync(report, { force: true });
      child = launch(join(bin, role), [], opts, join(privateDir, role + '.log'));
      children.push(child);
      for (let i = 0; ; i++) {
        if (existsSync(report) && read(report).phase === 'ready') return;
        if (i >= 200 || child.exitCode !== null || child.signalCode)
          throw Error(role + ' startup failed in ' + privateDir);
        await sleep(150);
      }
    };
    await start();
    running.push(child);
    controls?.push({ side, role, start, stop: () => stop(child) });
  }
  return running;
}
export async function withHTTPPair(
  seedDir,
  label,
  fn,
  {
    configure,
    monitoringWorkers = false,
    workerRoles = [],
    restrictedRoles = [...httpOwners, ...workerRoles, ...(monitoringWorkers ? splitWorkers : [])],
  } = {},
) {
  assert.match(label, /^[a-z-]+$/);
  const source = read(join(seedDir, 'private/lab.json')),
    baseline = read(join(seedDir, 'baseline.json'));
  const pg = await pairedPostgres(seedDir, source),
    pgPort = pg.port;
  const storage = await httpStorage(seedDir, source.id, { newBuckets: true });
  const id = randomBytes(6).toString('hex'),
    dir = join(seedDir, 'http', 'sandboxes', label + '-' + id);
  mkdirSync(join(dir, 'private'), { recursive: true, mode: 0o700 });
  for (const path of ['samples.json', 'baseline.json', 'private/accounts.json'])
    cpSync(join(seedDir, path), join(dir, path));
  const topology = join(seedDir, 'topology', 'roles');
  const release = pairedCandidate(seedDir),
    candidate = release.bin;
  const children = [],
    monitoring = [],
    lab = {
      directory: dir,
      id,
      container: pg.container,
      containerLabel: pg.containerLabel,
      chContainer: storage.chContainer,
    };
  let appliedRoleSQL;
  const privateRoles = [...httpOwners, 'monitoring-incremental-worker'];
  // The restored databases can take several seconds to start. Keep listener
  // addresses apart from the ephemeral connections opened during that interval.
  const firstPort = await availableRange(2 + privateRoles.length, () => randomInt(20000, 24500));
  const endpoints = Object.fromEntries(
    privateRoles.map((role, index) => [role, 'https://127.0.0.1:' + (firstPort + 2 + index)]),
  );
  const tested = read(join(seedDir, 'snapshot/manifest.json'));
  let scenarioError, result;
  try {
    for (const side of ['a', 'b']) {
      const db = 'arc_http_' + id + '_' + side,
        cfg = read(source[side].config),
        database = new URL(cfg.database_url);
      assert.equal(database.hostname, '127.0.0.1');
      run('docker', [
        'exec',
        pg.container,
        'createdb',
        '--username',
        'archtest',
        '--template',
        'arc_seed',
        db,
      ]);
      database.port = String(pgPort);
      database.pathname = '/' + db;
      cfg.database_url = database.href;
      cfg.redis_key_prefix = 'infrapro:arc-http-' + id + '-' + side + ':identity:v1:';
      const privateDir = join(dir, 'private', side),
        api = '127.0.0.1:' + (firstPort + (side === 'a' ? 0 : 1));
      mkdirSync(privateDir, { recursive: true, mode: 0o700 });
      const address = 'http://' + api;
      cfg.http_addr = api;
      cfg.public_console_url = address + '/';
      cfg.trusted_origins = address;
      const name = 'arc_http_' + id + '_' + side;
      cfg.monitoring_clickhouse_database = name;
      cfg.monitoring_clickhouse_replicas = storage.ch;
      cfg.monitoring_object_endpoint = storage.s3;
      for (const domain of ['files', 'audit']) cfg[domain].object_endpoint = storage.s3;
      const bucket = 'arc-http-' + id + '-' + side;
      cfg.monitoring_source_bucket = bucket;
      cfg.files.bucket = bucket;
      cfg.audit.bucket = bucket;
      for (const domain of ['files', 'audit']) {
        cfg[domain].temp_directory = join(privateDir, domain);
        mkdirSync(cfg[domain].temp_directory, { recursive: true, mode: 0o700 });
      }
      cfg.monitoring_temp_directory = join(privateDir, 'monitor');
      mkdirSync(cfg.monitoring_temp_directory, {
        recursive: true,
        mode: 0o700,
      });
      const bin = side === 'a' ? baseline.bin : candidate;
      const extra = await configure?.({ side, cfg, privateDir, dir, baseline, bin, release });
      const config = join(privateDir, 'service.json');
      write(config, cfg);
      lab[side] = {
        origin: address,
        api: address,
        config,
        database: db,
        ch: name,
        bucket,
      };
      const env = {
        ...Object.fromEntries(
          Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_')),
        ),
        INFRAPRO_CONFIG: config,
      };
      const cli = join(bin, 'infrapro');
      if (side === 'b') run(cli, ['database', 'migrate'], { env });
      run(cli, ['monitoring', 'storage', 'init'], { env });
      for (const [table, summary] of Object.entries(tested.physical)) {
        assert.match(table, /^[a-z0-9_]+$/);
        if (!summary.rows) continue;
        const chArgs = [
          'exec',
          '-i',
          storage.chContainer,
          'clickhouse-client',
          '--database',
          name,
          '--query',
        ];
        const count = () =>
          Number(
            run('docker', [...chArgs, 'SELECT count() FROM ' + table])
              .toString()
              .trim(),
          );
        if (!count())
          run('docker', [...chArgs, 'INSERT INTO ' + table + ' FORMAT Native'], {
            input: readFileSync(join(seedDir, 'snapshot', table + '.native')),
          });
        assert.equal(count(), summary.rows);
      }
      const actual = JSON.parse(
        run(join(seedDir, 'fixture'), ['restore', join(seedDir, 'snapshot')], {
          env,
        }),
      );
      assert.deepEqual(actual, tested.objects);
      if (extra?.beforeRoles) {
        await extra.beforeRoles(cfg);
        write(config, cfg);
      }
      const installed = pairedRoleConfigs(
        privateDir,
        cfg,
        pg.container,
        side === 'b' ? restrictedRoles : [],
      );
      const { roleEnv } = installed;
      if (installed.appliedRoleSQL) appliedRoleSQL = installed.appliedRoleSQL;
      for (const role of side === 'a' ? ['api'] : [...httpOwners, 'api']) {
        const opts = { ...env, ...roleEnv(role) };
        if (side === 'b') {
          const manifest = join(privateDir, role + '.json');
          write(manifest, pairedRoleManifest(topology, role, endpoints));
          opts.INFRAPRO_ARCHITECTURE_FILE = manifest;
        }
        assert.ok(existsSync(join(bin, role)));
        children.push(
          launch(join(bin, role), [], opts, join(dir, 'logs', side + '-' + role + '.log')),
        );
      }
      if (extra?.start)
        await extra.start({
          env,
          config,
          bin,
          endpoints,
          topology,
          children,
          roleEnv,
        });
      if (monitoringWorkers)
        await startPairedWorkers({
          side,
          privateDir,
          bin,
          env,
          endpoints,
          topology,
          children,
          roleEnv,
          controls: monitoring,
          legacy: ['monitoring-worker'],
          split: splitWorkers,
        });
    }
    write(join(dir, 'private/lab.json'), lab);
    await Promise.all([lab.a.api, lab.b.api].map((api) => waitURL(api + '/readyz')));
    write(join(dir, 'snapshot.json'), {
      baseline: baseline.commit,
      candidateSHA256: release.inputs.sourceSHA256,
      sourceDumpSHA256: tested.pg.seed,
      appliedRoleSQL,
      rows: Object.fromEntries(Object.entries(tested.physical).map(([k, v]) => [k, v.rows])),
      objects: tested.objects.length,
    });
    const fixtureID = process.env.ARC_HTTP_FIXTURE_ID;
    if (fixtureID) {
      assert.match(fixtureID, /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/);
      write(join(seedDir, 'commands', 'public-fixtures', fixtureID + '.json'), { directory: dir });
    }
    const withQueuesStopped = async (work) => {
      assert.ok(monitoring.length > 0, 'dedicated workers required');
      // A suspended process can own an advisory lock or an open transaction.
      // Gracefully drain both queues before exercising queued cancel/retry.
      await Promise.all(monitoring.map((worker) => worker.stop()));
      try {
        return await work();
      } finally {
        await Promise.all(monitoring.map((worker) => worker.start()));
      }
    };
    // Hold back ordinary foreground claims while both releases try their real
    // reserved inline path under the original deadline. In S0 the scanner is
    // the separate monitoring process. In B its foreground admission lock is
    // distinct from the live incremental owner's interactive slot.
    const trials = [];
    const withInteractiveSlot = async (work) => {
      const original = monitoring.find((w) => w.side === 'a' && w.role === 'monitoring-worker');
      assert(original, 'Paired trial requires live original and incremental workers');
      const database = 'arc_http_' + id + '_b';
      const session = spawn(
        'docker',
        [
          'exec',
          '-i',
          pg.container,
          'psql',
          '-XqAt',
          '-v',
          'ON_ERROR_STOP=1',
          '-U',
          'archtest',
          '-d',
          database,
        ],
        { stdio: ['pipe', 'pipe', 'inherit'] },
      );
      const ended = once(session, 'exit');
      const reader = createInterface({ input: session.stdout });
      try {
        session.stdin.write(
          "SET statement_timeout='10s'; SELECT 'reserved' FROM pg_advisory_lock(hashtextextended('monitoring/claim/monitoring-incremental-worker/foreground',0));\n",
        );
        const [line] = await Promise.race([
          once(reader, 'line'),
          ended.then(() => {
            throw Error('Candidate foreground admission session ended');
          }),
        ]);
        assert.equal(line, 'reserved');
        await original.stop();
        const reservation = {
          startedAt: new Date().toISOString(),
          originalScanner: 'stopped',
          candidateForeground: 'held',
          candidateInteractive: 'available',
        };
        try {
          return await work();
        } finally {
          trials.push({ ...reservation, finishedAt: new Date().toISOString() });
          write(join(dir, 'trial-schedule.json'), trials);
          await original.start();
        }
      } finally {
        session.stdin.end();
        reader.close();
        await ended;
      }
    };
    result = await fn(dir, { withQueuesStopped, withInteractiveSlot });
  } catch (error) {
    scenarioError = error;
  }
  const databases = [lab.a?.ch, lab.b?.ch].filter(Boolean);
  try {
    await Promise.all(children.map(stop));
    if (databases.length) archiveHTTPStorage(dir, storage, id, databases);
  } catch (error) {
    write(join(dir, 'cleanup-pending.json'), {
      recordedAt: new Date().toISOString(),
      databases,
      message: 'Inspect the sandbox logs and retry archival of these owned databases.',
    });
    if (!scenarioError) throw error;
  }
  if (scenarioError) throw scenarioError;
  return result;
}
