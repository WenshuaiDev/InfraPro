// Each measured round gets new isolated business/relay PG schemas, tenant
// principals, private object bucket and event epoch. Run from the repo root.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile, execFileSync, spawn } from 'node:child_process';
import {
  existsSync,
  mkdirSync,
  openSync,
  renameSync,
  closeSync,
  readFileSync,
  writeFileSync,
  appendFileSync,
} from 'node:fs';
import { dirname, resolve, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import os from 'node:os';
import { promisify } from 'node:util';
import { startParser, stop, waitURL } from './fixtures/parsing-lab.mjs';
import {
  checkReplicaPools,
  createBusinessDatabase,
  evidenceDatabase,
  grantFiles,
  prepareWorkerRoles,
  workerPG,
} from './fixtures/architecture-ingestion-database.mjs';
import { ingestionStorage, stopIngestionStorage } from './fixtures/architecture-ingestion-storage.mjs';
import {
  campaignRounds,
  readIngestionRelease,
  verifyCompletedIngestionRound,
  verifyIngestionPause,
  verifyIngestionRelease,
} from './architecture-ingestion-report.mjs';
import { startRelay } from './fixtures/ingestion-relay.mjs';
import { ingestionTestEnv } from './fixtures/ingestion-environment.mjs';
import { testEnv } from './test-environment.mjs';
const exec = promisify(execFile),
  root = resolve(fileURLToPath(new URL('../', import.meta.url)));
const out = resolve(process.argv[2] || '.artifacts/architecture-refactor/run-20261118');
const baseline = JSON.parse(readFileSync(join(out, 'baseline.json'), 'utf8'));
const dir = join(out, 'ingestion');
const roles = Object.keys(workerPG);
const [command, ...arguments_] = process.argv.slice(3);
const campaign =
  command === undefined
    ? 'run' + Date.now().toString(36)
    : ['warm', 'freeze', 'probe-campaign', 'reuse-baseline', 'resume', 'start', 'pause-after-current', 'report'].includes(
          command,
        )
      ? arguments_[0]
      : undefined;
if (campaign) assert.match(campaign, /^[a-z][a-z0-9]{0,20}$/);
const bin = campaign ? join(dir, 'campaigns', campaign, 'bin') : join(dir, 'bin');
mkdirSync(bin, { recursive: true, mode: 0o700 });
const release = campaign ? join(dir, 'campaigns', campaign + '.release') : undefined;
const json = (path, v) => writeFileSync(path, JSON.stringify(v, null, 2) + '\n', { mode: 0o600 });
const read = (path) => JSON.parse(readFileSync(path, 'utf8'));
const cmd = (program, args, opts = {}) =>
  execFileSync(program, args, { cwd: root, stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 8 << 20, ...opts });
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
const clean = Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_')));
function launch(file, args, env, log, cwd = root) {
  const fd = openSync(log, 'a', 0o600);
  try {
    return spawn(file, args, { cwd, env, stdio: ['ignore', fd, fd] });
  } finally {
    closeSync(fd);
  }
}
function deployment(mode, rebuild = false) {
  if (mode === 'a' || (!rebuild && campaign && roles.every((role) => existsSync(join(bin, role))))) return;
  cmd('go', ['build', '-trimpath', '-o', bin + '/', ...roles.map((r) => './services/api/cmd/' + r)]);
}
function driver(mode, rebuild = false) {
  const frozen = mode === 'a',
    source = frozen ? baseline.source : root;
  const output = join(bin, 'fixture-' + (frozen ? 'a' : 'b'));
  if (!rebuild && campaign && existsSync(output)) return { output, source };
  const pkg = 'packages/go/platform/parsing';
  const fixture = 'architecture_load_test.go';
  const measured = join(bin, 'monotonic_' + fixture);
  // The archived S0 and the selected release keep their original application
  // digests. Both test binaries use this identical timing-only fixture overlay.
  cmd('patch', [
    '-s',
    '-f',
    '-F',
    '0',
    '-o',
    measured,
    join(root, pkg, fixture),
    join(root, 'scripts/fixtures/architecture-ingestion-monotonic.patch'),
  ]);
  const replacements = { [join(source, pkg, fixture)]: measured };
  if (frozen)
    for (const file of ['parsing_test.go', 'ingestion_test.go'])
      replacements[join(source, pkg, file)] = join(root, pkg, file);
  const overlay = join(bin, 'test-overlay-' + (frozen ? 'a' : 'b') + '.json');
  json(overlay, { Replace: replacements });
  cmd('go', ['test', '-c', '-overlay', overlay, '-o', output, './' + pkg], { cwd: source });
  return { output, source };
}
const sha = (file) => createHash('sha256').update(readFileSync(file)).digest('hex');
const selectedRelease =
  process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE ||
  (process.env.INFRAPRO_ARCHITECTURE_RUN_DIR ? out : undefined);
const bindingFile = campaign && join(dir, 'campaigns', campaign, 'release-binding.json');
let binding;
function bindWorkers(actual) {
  const saved = bindingFile && existsSync(bindingFile) ? read(bindingFile) : null;
  if (!selectedRelease && !saved) return;
  if (!binding) {
    const manifest = selectedRelease ? resolve(selectedRelease, 'candidate.json') : saved.manifest;
    binding = readIngestionRelease(manifest, roles);
    if (saved) assert.deepEqual(binding, saved, 'Campaign was pinned to another candidate release');
  }
  verifyIngestionRelease(binding, actual);
  if (bindingFile && !saved) json(bindingFile, binding);
  return binding.manifest_sha256;
}
function executableRecord(mode, driverSHA, workers) {
  return {
    mode,
    source: mode === 'a' ? baseline.commit : 'working-tree',
    frozenParser: baseline.binaries.parser,
    releaseRunner: baseline.binaries['parser-runner'],
    externalRelay: baseline.binaries['ingest-relay'],
    driver: driverSHA,
    workers,
  };
}
async function sample(directory, pids, fixture, mode, objectContainer) {
  const line = { at: new Date().toISOString() };
  const activity = `datname=current_database() AND application_name='${fixture.tag}'`;
  const observations = await Promise.allSettled([
    exec('python3', [join(root, 'scripts/fixtures/architecture-process-usage.py'), ...pids.map(String)], {
      maxBuffer: 2 << 20,
    }).then((r) => JSON.parse(r.stdout)),
    exec(
      'docker',
      [
        'stats',
        '--no-stream',
        '--format',
        '{{json .}}',
        'infrapro-ingestion-lab-business-1',
        'infrapro-ingestion-lab-postgres-1',
        objectContainer,
      ],
      { maxBuffer: 2 << 20 },
    ).then((r) =>
      r.stdout
        .trim()
        .split('\n')
        .filter(Boolean)
        .map((x) => JSON.parse(x)),
    ),
    exec('docker', [
      'exec',
      'infrapro-ingestion-lab-business-1',
      'psql',
      '-qAt',
      '-U',
      'ingestion_test',
      '-d',
      evidenceDatabase(directory),
      '-c',
      `SELECT json_build_object('connections',count(*),'locks_waiting',count(*) FILTER(WHERE wait_event_type='Lock'),
        'active',count(*) FILTER(WHERE state='active'),
        'by_login',(SELECT json_object_agg(login,n) FROM (SELECT usename login,count(*) n FROM pg_stat_activity WHERE ${activity} GROUP BY usename) named),
        'lock_wait_ms',COALESCE((SELECT sum(extract(epoch FROM clock_timestamp()-l.waitstart)*1000) FROM pg_locks l WHERE NOT l.granted AND l.pid=ANY(array_agg(a.pid))),0))
        FROM pg_stat_activity a WHERE ${activity}`,
    ]).then((r) => JSON.parse(r.stdout)),
  ]);
  for (const [i, key] of ['processes', 'containers', 'postgres'].entries()) {
    const r = observations[i];
    if (r.status === 'fulfilled') line[key] = r.value;
    else line[key] = { error: r.reason.message.split('\n')[0] };
  }
  if (mode !== 'a')
    for (const role of roles) {
      const path = join(directory, role + '.report.json');
      if (existsSync(path)) {
        const v = read(path);
        (line.reports ??= {})[role] = v.metrics || [];
      }
    }
  if (existsSync(join(directory, 'progress.json')))
    line.domain = read(join(directory, 'progress.json')).counts;
  appendFileSync(join(directory, 'resources.jsonl'), JSON.stringify(line) + '\n', { mode: 0o600 });
}
async function runRound(name, warmOnly = false) {
  assert.match(name, /^(a|pg|mq)-[a-z0-9]+$/);
  const mode = name.split('-')[0],
    directory = join(dir, name);
  const { output, source } = driver(mode);
  const fixtureSources = campaign && join(bin, '..', 'fixture-sources');
  if (fixtureSources && existsSync(fixtureSources)) {
    for (const sample of ['samples/parsing/measurement.bin', 'samples/ingestion/vendor-batch.json'])
      assert.equal(sha(join(source, sample)), sha(join(fixtureSources, sample)), 'Input differs from the campaign snapshot');
  }
  deployment(mode);
  const productionBin = mode === 'a' ? baseline.bin : bin;
  for (const [role, digest] of Object.entries(baseline.binaries))
    if (
      ['parser', 'parser-runner', 'ingest-relay', ...(mode === 'a' ? ['ingest-worker'] : [])].includes(role)
    )
      assert.equal(sha(join(baseline.bin, role)), digest);
  const workerHashes = Object.fromEntries(
    (mode === 'a' ? ['ingest-worker'] : roles).map((r) => [r, sha(join(productionBin, r))]),
  );
  const manifestDigest = mode === 'a' ? undefined : bindWorkers(workerHashes);
  const binaries = executableRecord(mode, sha(output), workerHashes);
  if (existsSync(join(directory, 'result.json'))) {
    verifyCompletedIngestionRound(directory, { name, mode }, binaries, manifestDigest);
    return;
  }
  assert.ok(!existsSync(directory), 'Fresh trials require an unused round name: ' + name);
  mkdirSync(join(directory, 'private'), { recursive: true, mode: 0o700 });
  json(join(directory, 'binaries.json'), binaries);
  const databaseURL = createBusinessDatabase(directory);
  const objects = await ingestionStorage(directory);
  let parser,
    relay,
    test,
    done,
    sampling = false,
    sampler;
  const children = [];
  try {
    parser = await startParser(join(directory, 'private', 'parser'), {
      bin: baseline.bin,
      source: baseline.source,
    });
    relay = await startRelay(join(directory, 'private', 'relay'), {
      binary: join(baseline.bin, 'ingest-relay'),
    });
    const testenv = {
      ...clean,
      ...testEnv,
      ...ingestionTestEnv,
      INFRAPRO_TEST_DATABASE_URL: databaseURL,
      ...relay.env,
      INFRAPRO_PARSER_TEST_PACKAGE: parser.pkg,
      INFRAPRO_PARSING_URL: parser.url,
      INFRAPRO_PARSING_TOKEN_FILE: parser.token,
      INFRAPRO_INGESTION_LOAD: '1',
      INFRAPRO_INGESTION_KEEP_LOAD: '1',
      INFRAPRO_INGESTION_EVIDENCE: directory,
      INFRAPRO_ARCH_INGESTION_ROUND: directory,
      INFRAPRO_ARCH_TRANSPORT: mode,
      INFRAPRO_INGESTION_TEST_S3: objects.endpoint,
      ...(warmOnly ? { INFRAPRO_ARCH_WARM_CHECK: '1' } : {}),
      ...(release && !warmOnly ? { INFRAPRO_ARCH_RELEASE: release } : {}),
    };
    test = launch(
      output,
      ['-test.v', '-test.run', '^TestArchitectureIngestionLoad$', '-test.timeout', '3h'],
      testenv,
      join(directory, 'test.log'),
      join(source, 'packages/go/platform/parsing'),
    );
    done = new Promise((ok) => test.once('exit', (code, signal) => ok({ code, signal })));
    for (let n = 0; !existsSync(join(directory, 'fixture.json')); n++) {
      if (test.exitCode !== null || test.signalCode || n > 1800)
        throw Error('Fixture initialization failed; inspect ' + name + '/test.log');
      await delay(100);
    }
    const fixture = read(join(directory, 'fixture.json'));
    assert.match(fixture.tag, /^arc_identity_test_[a-z0-9]+$/);
    const baseEnv = { ...clean, INFRAPRO_CONFIG: join(directory, 'private', 'service.json') };
    const topology = join(directory, 'private', 'architecture.json'),
      endpoints = Object.fromEntries(
        ['identity-api', 'control-api', 'transfer-api', 'monitoring-query-api'].map((k) => [
          k,
          'https://127.0.0.1:1',
        ]),
      );
    json(topology, {
      endpoints,
      cert_directory: join(directory, 'private'),
      pg_connections: workerPG,
    });
    const eventDir = join(directory, 'private', 'events'),
      pgFile = join(eventDir, 'pg', 'events.json');
    const rolefile = (role) => (mode === 'pg' ? pgFile : join(eventDir, role, 'events.json'));
    const diag = (role) => ({
      INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'arc-ingestion',
      INFRAPRO_DIAGNOSTICS_NODE_ID: 'local',
      INFRAPRO_DIAGNOSTICS_REPLICA_ID: role,
      INFRAPRO_DIAGNOSTICS_REPORT_FILE: join(directory, role + '.report.json'),
    });
    if (mode !== 'a') {
      const roleEnv = prepareWorkerRoles(directory, fixture);
      const registrationFile = campaign && join(bin, '..', 'registration.json');
      if (registrationFile && existsSync(registrationFile)) {
        const registration = read(registrationFile);
        if (registration.grant_sql_sha256)
          assert.deepEqual(
            read(join(directory, 'worker-database-roles.json')).grant_sql_sha256,
            registration.grant_sql_sha256,
            'The installed role grants differ from the frozen campaign',
          );
      }
      if (mode === 'mq') {
        cmd(process.execPath, [
          'scripts/architecture-event-lab.mjs',
          eventDir,
          'arc_ingestion',
          fixture.epoch,
        ]);
        const lab = read(join(eventDir, 'lab.json'));
        const target = join(
          root,
          'build',
          'events',
          process.platform + '-' + (process.arch === 'x64' ? 'amd64' : process.arch),
          'nats-server',
        );
        if (!existsSync(target)) cmd(process.execPath, ['scripts/architecture-broker-build.mjs']);
        children.push(launch(target, ['-c', lab.config], clean, join(directory, 'broker.log')));
        await waitURL('http://127.0.0.1:' + lab.monitorPort + '/healthz?js-enabled-only=true');
      } else {
        mkdirSync(join(eventDir, 'pg'), { recursive: true, mode: 0o700 });
        json(pgFile, {
          ...read(join(root, 'deploy/events/events.example.json')),
          mode: 'pg',
          environment: 'arc_ingestion',
          replicas: 1,
          nats_url: '',
          ca_file: '',
          cert_file: '',
          key_file: '',
        });
      }
      const admin = { ...baseEnv, INFRAPRO_EVENTS_FILE: rolefile('event-admin') },
        adminCmd = (...args) => cmd(join(bin, 'event-relay'), args, { env: admin });
      if (mode === 'mq') adminCmd('provision');
      adminCmd('enable', '--reason', 'ARC-037 isolated load');
      for (const role of roles.slice(0, 2))
        adminCmd(
          'switch',
          '--role',
          role,
          '--expected',
          '1',
          '--mode',
          'split',
          '--reason',
          'ARC-037 isolated load',
        );
      for (const role of roles) {
        const env = {
          ...baseEnv,
          ...roleEnv(role),
          ...diag(role),
          INFRAPRO_ARCHITECTURE_FILE: topology,
          INFRAPRO_EVENTS_FILE: role === 'ingest-custody-worker' ? pgFile : rolefile(role),
        };
        const file = join(bin, role);
        if (manifestDigest)
          assert.equal(sha(file), workerHashes[role], 'Launch file changed after release validation for ' + role);
        children.push(launch(file, [], env, join(directory, role + '.log')));
      }
      if (manifestDigest)
        json(join(directory, 'release-binding.json'), { manifest_sha256: manifestDigest, workers: workerHashes });
    } else
      children.push(
        launch(
          join(baseline.bin, 'ingest-worker'),
          [],
          { ...baseEnv, ...diag('ingest-worker') },
          join(directory, 'ingest-worker.log'),
        ),
      );
    const expected = mode === 'a' ? ['ingest-worker'] : roles;
    for (let n = 0; ; n++) {
      assert.ok(
        children.every((c) => c.exitCode === null && !c.signalCode),
        'Background role exited: ' + name,
      );
      if (
        expected.every((role) => {
          const report = join(directory, role + '.report.json');
          return existsSync(report) && read(report).phase === 'ready';
        })
      )
        break;
      if (n >= 300) throw Error('Worker readiness timed out: ' + name);
      await delay(100);
    }
    if (mode !== 'a') checkReplicaPools(directory, fixture);
    const workers = children.slice(-expected.length);
    const monitored = expected.map((role, i) => ({ role, process: workers[i] }));
    json(join(directory, 'role-pids.json'), Object.fromEntries(monitored.map((r) => [r.role, r.process.pid])));
    let pending = Promise.resolve();
    const observe = () => {
      if (sampling) return;
      sampling = true;
      pending = sample(
        directory,
        [parser.pid, relay.pid, ...children.map((c) => c.pid)],
        fixture,
        mode,
        objects.container,
      ).finally(() => {
        sampling = false;
      });
    };
    const timer = setInterval(observe, 5000);
    sampler = async () => {
      clearInterval(timer);
      await pending;
    };
    observe();
    writeFileSync(join(directory, 'workers.ready'), 'ready\n');
    const listeners = [];
    const roleExit = new Promise((resolve) => {
      for (const { role, process } of monitored) {
        const stopped = (code, signal) => resolve({ role, code, signal });
        process.once('exit', stopped);
        listeners.push(() => process.off('exit', stopped));
        if (process.exitCode !== null || process.signalCode) stopped(process.exitCode, process.signalCode);
      }
    });
    const end = await Promise.race([done, roleExit]);
    for (const remove of listeners) remove();
    if (end.role) {
      json(join(directory, 'role-exit.json'), { ...end, at: new Date().toISOString() });
      throw Error('Background role exited during ' + name + ': ' + end.role);
    }
    await sampler();
    sampler = undefined;
    if (end.code !== 0)
      throw Error(name + ' failed field assertions; inspect ' + join(directory, 'test.log'));
    if (mode !== 'a') checkReplicaPools(directory, fixture, 'replica-pools-at-end.json');
    if (warmOnly) {
      assert.ok(existsSync(join(directory, 'warm-check.json')));
      console.log(name + ' warm probes passed');
      return;
    }
    assert.ok(existsSync(join(directory, 'verified.json')));
    json(join(directory, 'result.json'), {
      passed: true,
      mode,
      name,
      configuration: {
        pgRoleLimits: mode === 'a' ? [12] : Object.values(workerPG),
        sameParser: baseline.binaries.parser,
        singleNodeBroker: mode === 'mq',
      },
      finishedAt: new Date().toISOString(),
    });
    console.log(name + ' verified ' + JSON.stringify(read(join(directory, 'verified.json'))));
  } finally {
    if (sampler) await sampler();
    await Promise.allSettled(children.map(stop));
    if (test && test.exitCode === null && !test.signalCode) await stop(test);
    if (relay) await relay.close();
    if (parser) await parser.close();
    stopIngestionStorage(objects);
  }
}
function environment(campaignName) {
  assert.match(campaignName, /^[a-z][a-z0-9]{0,20}$/);
  const directory = join(dir, 'campaigns', campaignName);
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const dest = join(directory, 'environment.json');
  const dbs = ['infrapro-ingestion-lab-business-1', 'infrapro-ingestion-lab-postgres-1'];
  const containers = JSON.parse(cmd('docker', ['inspect', ...dbs]).toString()).map((v) => ({
    name: v.Name.slice(1),
    image: v.Image,
    configured_image: v.Config.Image,
    memory_limit_bytes: v.HostConfig.Memory,
    nano_cpus: v.HostConfig.NanoCpus,
    cpuset: v.HostConfig.CpusetCpus,
    swap_limit_bytes: v.HostConfig.MemorySwap,
  }));
  const data = {
    os: process.platform,
    release: os.release(),
    arch: process.arch,
    cpus: os.cpus().length,
    cpu_model: os.cpus()[0].model,
    host_memory_bytes: os.totalmem(),
    docker: JSON.parse(
      cmd('docker', [
        'info',
        '--format',
        '{"version":"{{.ServerVersion}}","cpus":{{.NCPU}},"memory_bytes":{{.MemTotal}}}',
      ]).toString(),
    ),
    containers,
    frozen_commit: baseline.commit,
    worker_pg_connections: { a: [12], pg: Object.values(workerPG), mq: Object.values(workerPG) },
    core_inputs: Object.fromEntries(
      ['samples/parsing/measurement.bin', 'samples/ingestion/vendor-batch.json'].map((path) => [
        path,
        sha(join(root, path)),
      ]),
    ),
    sample_hz: 100,
    s3: 'Per-round storage.json and docker resource samples record the isolated, retained object container. All modes use the same image and resource limits.',
    visibility_probe_ms: 250,
    resource_sample_ms: 5000,
  };
  if (existsSync(dest)) assert.deepEqual(read(dest), data, 'Campaign hardware or data configuration changed');
  else json(dest, data);
  return data;
}
function productionSources() {
  const files = [];
  const template =
    '{{if .Module}}{{.Dir}}|{{join .GoFiles " "}}|{{join .CgoFiles " "}}|{{join .EmbedFiles " "}}{{end}}';
  const dependencies = cmd('go', [
    'list',
    '-deps',
    '-f',
    template,
    ...roles.map((role) => './services/api/cmd/' + role),
    './packages/go/platform/migrations',
  ]).toString();
  for (const row of dependencies.split('\n').filter(Boolean)) {
    const [packageDir, ...groups] = row.split('|');
    const path = relative(root, packageDir);
    if (!path || path.startsWith('..')) continue;
    for (const file of groups.flatMap((group) => group.split(' ').filter(Boolean)))
      files.push(join(path, file));
  }
  files.push(
    ...[
      'go.work',
      'go.work.sum',
      'packages/go/platform/go.mod',
      'packages/go/platform/go.sum',
      'services/api/go.mod',
      'services/api/go.sum',
      'scripts/architecture-ingestion-load.mjs',
      'scripts/architecture-ingestion-report.mjs',
      'scripts/architecture-event-lab.mjs',
      'scripts/architecture-broker-build.mjs',
      'scripts/architecture-lab.mjs',
      'scripts/architecture-runtime.mjs',
      'scripts/fixtures/architecture-ingestion-storage.mjs',
      'scripts/fixtures/architecture-ingestion-database.mjs',
      'scripts/fixtures/architecture-process-usage.py',
      'scripts/fixtures/architecture-benchmark-window.mjs',
      'scripts/fixtures/ingestion-environment.mjs',
      'scripts/fixtures/ingestion-relay.mjs',
      'scripts/fixtures/parsing-lab.mjs',
      'scripts/test-environment.mjs',
      'scripts/architecture-postgres.mjs',
      'scripts/architecture-http-sandbox.mjs',
      ...grantFiles,
      'deploy/architecture/roles.json',
      'deploy/events/events.example.json',
      'deploy/monitoring-contract/s3.json',
      'samples/parsing/measurement.bin',
      'samples/ingestion/vendor-batch.json',
      'scripts/fixtures/architecture-ingestion-monotonic.patch',
      ...['architecture_load_test.go', 'ingestion_test.go', 'parsing_test.go'].map((name) =>
        join('packages/go/platform/parsing', name),
      ),
    ].filter((name) => existsSync(join(root, name))),
  );
  const values = files.sort().map((file) => ({ file, sha256: sha(join(root, file)) }));
  return { tree_sha256: createHash('sha256').update(JSON.stringify(values)).digest('hex'), files: values };
}
function freezeCampaign() {
  const location = join(dir, 'campaigns', campaign);
  const builds = join(location, 'builds.json'),
    sources = join(location, 'source-digests.json');
  const names = ['fixture-a', 'fixture-b', ...roles];
  const hashes = () => Object.fromEntries(names.map((name) => [name, sha(join(bin, name))]));
  if (existsSync(builds) && existsSync(sources)) {
    const current = hashes();
    assert.deepEqual(read(builds), current, 'A pinned executable changed');
    if (arguments_.includes('--fresh'))
      assert.equal(read(join(location, 'registration.json')).fresh_rounds, true, 'Existing campaign allows reused rounds');
    bindWorkers(current);
    return;
  }
  const input = productionSources();
  driver('a', true);
  driver('pg', true);
  deployment('pg', true);
  const compiled = productionSources();
  assert.equal(
    compiled.tree_sha256,
    input.tree_sha256,
    'Source changed during freeze: choose a fresh campaign',
  );
  json(sources, {
    ...compiled,
    captured_at: new Date().toISOString(),
    head: cmd('git', ['rev-parse', 'HEAD']).toString().trim(),
  });
  // Keep the executable fixtures, samples and SQL beside the binary digests.
  // The application source is separately archived with its release manifest.
  for (const { file, sha256 } of compiled.files.filter(({ file }) =>
    /^(scripts|deploy|samples)\//.test(file) || /^packages\/go\/platform\/parsing\/.*_test\.go$/.test(file))) {
    const snapshot = join(location, 'fixture-sources', file);
    const bytes = readFileSync(join(root, file));
    assert.equal(createHash('sha256').update(bytes).digest('hex'), sha256);
    mkdirSync(dirname(snapshot), { recursive: true, mode: 0o700 });
    writeFileSync(snapshot, bytes, { mode: 0o600 });
  }
  const current = hashes();
  bindWorkers(current);
  const fresh = command === undefined || arguments_.includes('--fresh');
  const order = roundsFor(campaign);
  if (fresh)
    assert.ok(
      order.every((r) => !r.reused && !existsSync(join(dir, r.name))),
      'Fresh campaign requires nine unused round directories',
    );
  json(join(location, 'registration.json'), {
    at: new Date().toISOString(),
    fresh_rounds: fresh,
    single_role_database_logins: true,
    baseline_commit: baseline.commit,
    baseline_binaries: Object.fromEntries(
      ['ingest-worker', 'parser', 'parser-runner', 'ingest-relay'].map((role) => [role, baseline.binaries[role]]),
    ),
    candidate_manifest_sha256: binding?.manifest_sha256,
    grant_sql_sha256: Object.fromEntries(grantFiles.map((file) => [file, sha(join(root, file))])),
    rounds: order.map((r) => r.name),
    builds: current,
  });
  json(builds, current);
}
function roundsFor(name) {
  return campaignRounds(dir, name);
}
async function initialGate(name, round) {
  const { trial } = await import('./architecture-ingestion-report.mjs');
  const data = roundsFor(name).filter((r) => r.ordinal === 1);
  const row = trial(dir, round.name, round.mode);
  const a = trial(dir, data[0].name, 'a');
  const performance = {
    end_to_end_p95: row.time_to_owner_readable_ms.p95 / a.time_to_owner_readable_ms.p95,
    catchup: row.catchup_seconds / a.catchup_seconds,
  };
  let passed = Math.max(performance.end_to_end_p95, performance.catchup) <= 1.1;
  if (round.mode === 'mq') {
    const pg = trial(dir, data[1].name, 'pg');
    performance.claim_vs_pg =
      row.ready_to_first_effective_claim_ms.p95 / pg.ready_to_first_effective_claim_ms.p95;
    performance.scans_vs_pg = row.events.scans_per_1000_jobs / pg.events.scans_per_1000_jobs;
    passed &&=
      Math.min(performance.claim_vs_pg, performance.scans_vs_pg) <= 0.8 &&
      Math.max(performance.claim_vs_pg, performance.scans_vs_pg) <= 1.1;
  }
  json(join(dir, 'campaigns', name, round.mode + '-initial-gate.json'), {
    passed,
    round: round.name,
    baseline: a.name,
    ratios: performance,
  });
  return passed;
}
async function summarize() {
  const { report } = await import('./architecture-ingestion-report.mjs');
  if (selectedRelease || (bindingFile && existsSync(bindingFile)))
    for (const round of roundsFor(campaign).filter((r) => r.mode !== 'a'))
      bindWorkers(read(join(dir, round.name, 'binaries.json')).workers);
  const summary = report(dir, campaign);
  if (!summary.passed) process.exitCode = 1;
}
if (command === 'round') {
  const name = arguments_[0];
  assert.match(name, /^(a|pg|mq)-[a-z0-9]+$/);
  const { trial } = await import('./architecture-ingestion-report.mjs');
  const summary = trial(dir, name, name.split('-')[0]);
  json(join(dir, name, 'metrics.json'), summary);
  console.log(
    JSON.stringify({
      name,
      catchup: summary.catchup_seconds,
      e2e_p95: summary.time_to_owner_readable_ms.p95,
      claim_p95: summary.ready_to_first_effective_claim_ms.p95,
      scans: summary.events?.scans_per_1000_jobs,
    }),
  );
} else if (command === 'stages') {
  const name = arguments_[0];
  assert.match(name, /^(a|pg|mq)-[a-z0-9]+$/);
  const { stageSamples } = await import('./architecture-ingestion-report.mjs');
  console.log(stageSamples(join(dir, name)).length + ' stage samples');
} else if (command === 'watch') {
  const name = arguments_[0];
  assert.match(name, /^[a-z][a-z0-9]{0,20}$/);
  const target = join(dir, 'campaigns', name, 'timeline.jsonl');
  const previous = existsSync(target)
    ? readFileSync(target, 'utf8')
        .trim()
        .split('\n')
        .filter(Boolean)
        .map((line) => JSON.parse(line))
    : [];
  const known = new Set(previous.map((row) => row.round + '/' + row.phase));
  const rounds = roundsFor(name).map((round) => round.name);
  const observe = (round, phase, source, f) => {
    const key = round + '/' + phase;
    if (known.has(key)) return;
    const path = join(dir, round, source + '.json');
    if (!existsSync(path)) return;
    const info = f(read(path));
    const row = { round, phase, ...info };
    appendFileSync(target, JSON.stringify(row) + '\n', { mode: 0o600 });
    known.add(key);
    console.log(JSON.stringify(row));
  };
  while (
    !known.has(rounds.at(-1) + '/verified') &&
    !existsSync(join(dir, 'campaigns', name, 'stopped.json'))
  ) {
    for (const round of rounds) {
      observe(round, 'warmed', 'warm', (v) => ({ at: v.at }));
      observe(round, 'measuring', 'start', (v) => ({ at: v.started }));
      observe(round, 'injection_finished', 'outage', (v) => ({
        at: new Date(Date.parse(v.down) - 10000 + v.production_seconds * 1000).toISOString(),
        offered_fps: v.offered_fps,
        disconnect_seconds: v.duration_seconds,
      }));
      observe(round, 'owner_readback', 'drained', (v) => ({ at: v.at }));
      observe(round, 'verified', 'result', (v) => ({ at: v.finishedAt, passed: v.passed }));
    }
    await delay(750);
  }
} else if (command === 'environment') {
  environment(arguments_[0]);
} else if (command === 'probe' || command === 'probe-campaign') {
  const selected = command === 'probe-campaign' ? arguments_.slice(1) : arguments_;
  if (command === 'probe-campaign') {
    assert.ok(campaign && existsSync(join(bin, '../builds.json')), 'Freeze the selected campaign first');
    freezeCampaign();
  }
  for (const mode of selected.length ? selected : ['pg', 'mq']) {
    assert.ok(['pg', 'mq'].includes(mode));
    await runRound(mode + '-probe' + (campaign || '') + Date.now().toString(36), true);
  }
} else if (command === 'prepare') {
  driver('a');
  driver('pg');
  deployment('pg');
  console.log('Prepared frozen A and candidate B. Each round creates an isolated S3.');
} else if (command === 'reuse-baseline') {
  assert.ok(campaign, 'Campaign is required');
  assert.ok(!existsSync(release), 'Baseline selection must precede release');
  const registration = join(dir, 'campaigns', campaign, 'registration.json');
  assert.ok(!existsSync(registration) || !read(registration).fresh_rounds, 'This campaign requires nine fresh rounds');
  const names = arguments_.slice(1);
  assert.ok(names.length >= 1 && names.length <= 3, 'Supply the original baseline rounds in ordinal order');
  const current = environment(campaign);
  const frozenDriver = sha(driver('a').output);
  const anchors = names.map((name, i) => {
    assert.match(name, /^a-[a-z][a-z0-9]*[1-3]$/);
    assert.equal(Number(name.at(-1)), i + 1, 'Expected the same ordinal from the original campaign');
    const source = join(dir, name);
    const bins = read(join(source, 'binaries.json'));
    assert.equal(bins.driver, frozenDriver, 'Frozen A test harness differs');
    assert.equal(bins.source, baseline.commit);
    assert.equal(bins.workers['ingest-worker'], baseline.binaries['ingest-worker']);
    assert.equal(bins.frozenParser, baseline.binaries.parser);
    assert.equal(bins.releaseRunner, baseline.binaries['parser-runner']);
    assert.equal(bins.externalRelay, baseline.binaries['ingest-relay']);
    assert.equal(read(join(source, 'result.json')).passed, true);
    assert.ok(!existsSync(join(source, 'interference.json')) && existsSync(join(source, 'storage.json')));
    assert.deepEqual(current, read(join(dir, 'campaigns', name.slice(2, -1), 'environment.json')));
    return {
      round: name,
      result_sha256: sha(join(source, 'result.json')),
      inputs_sha256: sha(join(source, 'inputs.json')),
    };
  });
  json(join(dir, 'campaigns', campaign, 'reused-baseline.json'), {
    round: names[0],
    round_names: names,
    anchors,
    binaries: read(join(dir, names[0], 'binaries.json')),
    reason: 'Identical frozen baseline, driver, samples and resource configuration',
    accepted_at: new Date().toISOString(),
  });
} else if (command === 'freeze') {
  assert.ok(campaign, 'Campaign is required');
  freezeCampaign();
} else if (command === 'pause-after-current') {
  assert.ok(campaign, 'Campaign is required');
  json(join(dir, 'campaigns', campaign, 'pause-after-current.json'), {
    requested_at: new Date().toISOString(),
  });
} else if (command === 'start') {
  assert.ok(campaign, 'Campaign is required');
  const first = roundsFor(campaign).find((r) => !r.reused);
  const directory = join(dir, first.name);
  assert.ok(existsSync(join(directory, 'warm.json')), 'First round has not completed independent warmup');
  assert.ok(!existsSync(join(directory, 'role-exit.json')), 'A warmed role has exited');
  const pids = read(join(directory, 'role-pids.json'));
  for (const pid of Object.values(pids)) process.kill(pid, 0);
  if (first.mode !== 'a')
    checkReplicaPools(directory, read(join(directory, 'fixture.json')), 'replica-pools-at-release.json');
  assert.ok(
    ['fixture-a', 'fixture-b', ...roles].every((name) => existsSync(join(bin, name))),
    'Campaign has not frozen both drivers and the candidate roles',
  );
  writeFileSync(release, new Date().toISOString() + '\n', { mode: 0o600, flag: 'wx' });
  console.log('Released ' + campaign);
} else if (command === 'warm' || command === 'resume' || command === undefined) {
  assert.ok(campaign, 'Campaign is required');
  mkdirSync(join(dir, 'campaigns'), { recursive: true, mode: 0o700 });
  if (command === 'warm') assert.ok(!existsSync(release), 'An already released campaign can only resume from its saved pause');
  if (command === undefined)
    writeFileSync(release, new Date().toISOString() + '\n', { mode: 0o600, flag: 'wx' });
  environment(campaign);
  if (command === undefined || !arguments_.includes('--defer-candidate')) {
    freezeCampaign();
  } else driver('a');
  const stateDirectory = join(dir, 'campaigns', campaign);
  const initialGateFile = join(stateDirectory, 'check-first-round.json');
  if (arguments_.includes('--gate-first') && !existsSync(initialGateFile))
    json(initialGateFile, { requested_at: new Date().toISOString() });
  const pause = join(stateDirectory, 'pause-after-current.json'),
    stopped = join(stateDirectory, 'stopped.json');
  if (command === 'resume') {
    assert.ok(
      existsSync(release) && existsSync(stopped) && existsSync(pause),
      'An explicitly paused campaign is required',
    );
    const builds = read(join(stateDirectory, 'builds.json'));
    verifyIngestionPause(dir, roundsFor(campaign), read(stopped), (folder, round) => {
      const frozen = round.mode === 'a';
      const workers = frozen
        ? { 'ingest-worker': baseline.binaries['ingest-worker'] }
        : Object.fromEntries(roles.map((role) => [role, builds[role]]));
      const binaries = executableRecord(round.mode, builds[frozen ? 'fixture-a' : 'fixture-b'], workers);
      verifyCompletedIngestionRound(folder, round, binaries, frozen ? undefined : bindWorkers(workers));
    });
    const at = Date.now();
    renameSync(pause, pause + '.' + at);
    renameSync(stopped, stopped + '.' + at);
  }
  for (const round of roundsFor(campaign)) {
    if (round.reused) continue;
    if (existsSync(pause)) {
      json(stopped, { before: round.name, at: new Date().toISOString() });
      console.log('Stopped before ' + round.name);
      break;
    }
    console.log('Starting ' + round.name);
    try {
      await runRound(round.name);
    } catch (error) {
      json(stopped, {
        during: round.name,
        measuring: existsSync(join(dir, round.name, 'start.json')),
        at: new Date().toISOString(),
        reason: 'role, fixture or business assertion failed',
      });
      throw error;
    }
    if (
      existsSync(initialGateFile) &&
      round.ordinal === 1 &&
      round.mode !== 'a' &&
      !(await initialGate(campaign, round))
    ) {
      json(stopped, {
        after: round.name,
        at: new Date().toISOString(),
        reason: 'initial performance limits',
      });
      console.log('Stopped after first comparison: ' + round.name);
      process.exitCode = 1;
      break;
    }
  }
  if (!existsSync(join(dir, 'campaigns', campaign, 'stopped.json'))) await summarize();
} else if (command === 'report') {
  assert.ok(campaign, 'Campaign is required');
  await summarize();
} else {
  for (const name of [command, ...arguments_]) {
    console.log('Starting ' + name);
    await runRound(name);
  }
}
