import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, constants, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { pathToFileURL } from 'node:url';
import { digest, read, root, runDirectory, write } from './architecture-lab.mjs';
import { unusedDockerSubnet } from './architecture-http-network.mjs';

const fixtures = {
  'diagnostics-deploy-check.mjs': [{
    scenario: 'isolated production Compose deployment and cleanup',
    original: 'The private Compose project asks Docker to allocate a network from its default address pools.',
    shared: 'Use an available /24 from the diagnostic fixture range and verify the project network ownership, HTTPS application, and removal of its network and containers.',
  }],
  'diagnostics-browser-check.mjs': [
    {
      scenario: 'component-filter-counts',
      original: 'After selecting workers, require the exact fixed text 共 8 项 · 当前快照 and check that the API is absent.',
      shared: 'Require the exact count from this release’s manifest, rendered with its installed ListPagination text 共 N 条, and check that the API is absent.',
    }, {
      scenario: 'self-permission-gap-opens-evidence',
      original: 'Require the report button within 20 seconds of fault injection, then open the permission-denied evidence.',
      shared: 'After injection, require the live /self API to observe permission_denied within 70 seconds. Require the already-open page to display the report within another 15 seconds and open the same evidence.',
    },
  ],
  'diagnostics-fault-check.mjs': [{
    scenario: 'unrelated containers during fault recovery',
    original: 'Compare every running external container ID on the entire Docker host with the initial list.',
    shared: 'Start a separately labelled control, verify that ownership checks reject it, and require its running state and original start time throughout the suite.',
  }, {
    scenario: 'operator sessions during each fault and recovery',
    original: 'Start another operator login after each recovery, including faults that leave the session valid.',
    shared: 'Read the live overview using the existing session after recovery and reauthenticate only on an observed 401. During full business outage, require both an existing and a fresh login. For a genuine 429, honor the server’s Retry-After within a bounded deadline. Record per-fault session use and recovered observations.',
  }],
  'diagnostics-business-check.mjs': [{
    scenario: 'owning-process binaries remain sealed while compiling the original commands',
    original: 'Build every catalogued API command over the same lab/bin files used by the resident processes and their later restarts.',
    shared: 'Build all original commands into a private temporary directory. The real owning-process tests and restarted roles use the selected release. Compare the compiled and served digests and retain the build receipts.',
  }],
};

const suiteNames = [
  'core-tests', 'contracts', 'native-contract', 'owning-processes', 'business-regression',
  'checker-sandbox', 'fixed-protocols', 'docker-contract', 'linux-contract', 'network-contract',
  'faults', 'deployment', 'browser',
];
const dependencyReplicas = ['clickhouse-1', 'clickhouse-2', 'keeper-1', 'postgres-1', 'redis-1', 's3-1', 'web-1'];

function diagnosticsContainers(lab) {
  const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', stdio: 'pipe', timeout: 30000 }).trim();
  const ids = docker('ps', '-a', '-q', '--filter', 'label=io.infrapro.lab.run_id=' + lab.run_id,
    '--filter', 'label=com.docker.compose.project=' + lab.compose_project).split('\n').filter(Boolean);
  assert.equal(ids.length, dependencyReplicas.length);
  const format = '{"id":{{json .Id}},"replica":{{json (index .Config.Labels "io.infrapro.replica")}},"run":{{json (index .Config.Labels "io.infrapro.lab.run_id")}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"running":{{json .State.Running}},"mounts":{{json .Mounts}}}';
  const containers = docker('inspect', '--format', format, ...ids).split('\n').map(line => {
    const { id, replica, run, project, running, mounts } = JSON.parse(line);
    assert.match(id, /^[a-f0-9]{64}$/);
    assert.equal(run, lab.run_id);
    assert.equal(project, lab.compose_project);
    return { id, replica, running, volumes: mounts.filter(mount => mount.Type === 'volume').map(mount => mount.Name).sort() };
  }).sort((a, b) => a.replica.localeCompare(b.replica));
  assert.deepEqual(containers.map(c => c.replica), dependencyReplicas);
  return containers;
}

function recordFixtureState(folder, name, observation) {
  const file = join(folder, name + '.json');
  if (existsSync(file)) {
    const prior = readFileSync(file);
    const previous = join(folder, 'attempts', name + '-' + digest(prior).slice(0, 16));
    mkdirSync(previous, { recursive: true, mode: 0o700 });
    const archived = join(previous, name + '.json');
    if (!existsSync(archived)) writeFileSync(archived, prior, { flag: 'wx', mode: 0o600 });
  }
  write(file, observation);
}

function selectedLab(cwd) {
  const pointer = read(join(cwd, '.artifacts/diagnostics/active.json'));
  const lab = read(join(pointer.directory, 'lab.json'));
  assert.match(lab.run_id, /^diag-[a-f0-9]{16}$/);
  assert.equal(lab.run_id, pointer.run_id);
  assert.equal(lab.directory, pointer.directory);
  assert(lab.business && lab.state === 'running');
  assert.equal(lab.compose_project, 'infrapro-' + lab.run_id);
  assert.equal(lab.binary, join(cwd, 'build/bin/infrapro-diagnostics'));
  return lab;
}

export function parkedDiagnosticsLab(dir, side, cwd) {
  const active = join(cwd, '.artifacts/diagnostics/active.json');
  if (!existsSync(active)) return false;
  const pointer = read(active), lab = read(join(pointer.directory, 'lab.json'));
  if (lab.state !== 'parked') return false;
  assert.equal(pointer.run_id, lab.run_id);
  assert.equal(lab.run_id, read(join(dir, 'regression', side, 'diagnostics-fixture.json')).run_id);
  return true;
}

function baselineTemplateReferences(cwd, target) {
  const catalog = read(join(cwd, 'contracts/diagnostics/runtime-catalog.json')).entries;
  const expected = ['ingest-worker', 'ingest-relay', 'ingest-relay-postgres', 'parser'];
  const references = {}, comparisons = [];
  for (const profile of ['native', 'docker', 'mixed']) {
    const name = join('deploy/diagnostics', profile, 'manifest.json');
    const frozen = readFileSync(join(cwd, name)), manifest = JSON.parse(frozen);
    const missing = catalog.filter(row => !manifest.components.some(component => component.id === row.id));
    const monitoring = manifest.components.find(component => component.id === 'monitoring-worker');
    const currentName = catalog.find(row => row.id === monitoring.id).display_name;
    if (!missing.length && monitoring.display_name === currentName) continue;
    assert.deepEqual(missing.map(row => row.id), expected);
    assert(missing.every(row => row.default_expectation === 'disabled'));
    assert.equal(monitoring.display_name, '监测与派生计算');
    assert.equal(currentName, '监测、派生计算与试解析');
    const oldName = monitoring.display_name;
    const pattern = manifest.components.find(component => component.id === 'api');
    const replica = manifest.replicas.find(slot => slot.component_id === 'api');
    assert.equal(pattern.expectation, 'disabled');
    assert.equal(pattern.min_ready_replicas, 0);
    assert.deepEqual(replica.required_probe_ids, []);
    // Derive expectations from the frozen catalogue and the prior disabled
    // template. The independent CLI under test never provides these bytes.
    manifest.components.unshift(...missing.map(({ id, display_name, kind }) => ({
      ...pattern, id, display_name, kind,
    })));
    manifest.replicas.unshift(...missing.map(({ id }) => ({
      ...replica, id: id + '-1', component_id: id,
    })));
    monitoring.display_name = currentName;
    const reference = join(target, 'expected', name);
    write(reference, manifest);
    references[name] = reference;
    comparisons.push({ profile, file: name, frozen_sha256: digest(frozen), expected_sha256: digest(readFileSync(reference)),
      frozen_components: manifest.components.length - missing.length, expected_components: manifest.components.length,
      frozen_replicas: manifest.replicas.length - missing.length, expected_replicas: manifest.replicas.length,
      added_disabled: expected, monitoring_label: { frozen: oldName, expected: currentName } });
  }
  return { references, comparisons };
}

function diagnosticsSQL(postgres, query) {
  return execFileSync('docker', ['exec', postgres.id, 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1',
    '-U', 'diagnostics_lab', '-d', 'diagnostics_lab', '-c', query],
  { encoding: 'utf8', stdio: 'pipe', timeout: 30000 }).trim();
}

function diagnosticsSchema(postgres) {
  return JSON.parse(diagnosticsSQL(postgres, `WITH expected(kind,signature) AS (VALUES
    ('baseline','public.architecture_fixed_baseline_metadata(uuid,uuid,text,text)'),
    ('invitation','public.architecture_notification_invitation_targets(uuid,uuid,uuid[])')
  ), projection AS (
    SELECT e.kind,p.oid,p.proacl,p.proowner FROM expected e
    JOIN pg_catalog.pg_proc p ON p.oid=to_regprocedure(e.signature)
  ), grants AS (
    SELECT p.kind,a.grantee FROM projection p
    CROSS JOIN LATERAL pg_catalog.aclexplode(coalesce(p.proacl,pg_catalog.acldefault('f',p.proowner))) a
    WHERE a.privilege_type='EXECUTE'
  ) SELECT json_build_object(
    'version', COALESCE(MAX(version_id),0),
    'publication_permit', to_regprocedure('event_publication_permit(boolean)') IS NOT NULL,
    'tenant_pressure_index', to_regclass('event_outbox_tenant_pressure') IS NOT NULL,
    'fixed_baseline_metadata', EXISTS(SELECT FROM projection WHERE kind='baseline'),
    'metadata_public_execute', EXISTS(SELECT FROM grants WHERE kind='baseline' AND grantee=0),
    'legacy_audit_role', to_regrole('infrapro_audit_worker') IS NOT NULL,
    'metadata_audit_execute', EXISTS(SELECT FROM grants WHERE kind='baseline' AND grantee=to_regrole('infrapro_audit_worker')::oid),
    'notification_invitation_targets', EXISTS(SELECT FROM projection WHERE kind='invitation'),
    'invitation_public_execute', EXISTS(SELECT FROM grants WHERE kind='invitation' AND grantee=0),
    'control_api_role', to_regrole('infrapro_control_api') IS NOT NULL,
    'invitation_control_execute', EXISTS(SELECT FROM grants WHERE kind='invitation' AND grantee=to_regrole('infrapro_control_api')::oid)
  ) FROM goose_db_version WHERE is_applied`));
}

function alignDiagnosticsProjection(dir, side, cwd, lab, release, cli) {
  if (side !== 'candidate') return;
  const migration = readFileSync(join(cwd, 'packages/go/platform/migrations/00059_architecture_events.sql'));
  assert.equal(digest(migration), release.schema.migrationSHA256, 'Migration differs from selected release');
  if (!migration.includes('-- architecture:fixed-baseline-metadata:begin')) return;
  const containers = diagnosticsContainers(lab), pg = containers.find(c => c.replica === 'postgres-1');
  assert(pg.running);
  assert.equal(realpathSync(lab.business_config), join(realpathSync(lab.directory), 'business.json'));
  const value = read(lab.business_config).database_url;
  assert(typeof value === 'string' && URL.canParse(value), 'A private diagnostics database URL is required');
  const url = new URL(value);
  const published = execFileSync('docker', ['port', pg.id, '5432/tcp'], { encoding: 'utf8' }).trim();
  assert(url.protocol === 'postgres:' && url.hostname === '127.0.0.1' && url.username === 'diagnostics_lab'
    && url.pathname === '/diagnostics_lab' && url.search === '' && url.hash === ''
    && url.port === String(lab.ports.postgres) && published === '127.0.0.1:' + url.port,
  'Database target differs from the labelled diagnostics container and its loopback port');
  const before = diagnosticsSchema(pg);
  assert.equal(before.version, release.schema.version);
  const epoch = () => diagnosticsSQL(pg, 'SELECT restore_epoch::text FROM public.event_protocol WHERE id=1');
  const originalEpoch = epoch();
  const requiresInvitations = migration.includes('-- architecture:notification-invitation-targets:begin');
  const restricted = db => db.fixed_baseline_metadata && !db.metadata_public_execute &&
    (!db.legacy_audit_role || db.metadata_audit_execute) && (!requiresInvitations ||
      (db.notification_invitation_targets && !db.invitation_public_execute &&
        (!db.control_api_role || db.invitation_control_execute)));
  if (!restricted(before)) {
    // Only the validated private DSN reaches the selected CLI. In particular,
    // no ambient libpq host, service file or schema overrides are inherited.
    execFileSync(cli, ['database', 'migrate'], { env: { INFRAPRO_DATABASE_URL: value },
      stdio: 'pipe', timeout: 120000 });
  }
  const after = diagnosticsSchema(pg);
  assert.equal(after.version, before.version);
  assert(restricted(after));
  assert.equal(epoch(), originalEpoch);
  const role = after.legacy_audit_role ? 'infrapro_audit_worker' : 'diagnostics_lab';
  const result = diagnosticsSQL(pg, `BEGIN READ ONLY; SET LOCAL ROLE ${role};
    SELECT count(*) FROM public.architecture_fixed_baseline_metadata(NULL,NULL,NULL,NULL); ROLLBACK`);
  assert.equal(result, '0', 'The migrated legacy audit role could not call its read-only projection');
  let invitationRead;
  if (requiresInvitations) {
    const actor = after.control_api_role ? 'infrapro_control_api' : 'diagnostics_lab';
    const count = diagnosticsSQL(pg, `BEGIN READ ONLY; SET LOCAL ROLE ${actor};
      SELECT count(*) FROM public.architecture_notification_invitation_targets(NULL,NULL,ARRAY[]::uuid[]); ROLLBACK`);
    assert.equal(count, '0', 'The selected role could not call the invitation projection');
    invitationRead = { role: actor, transaction: 'read_only', observed_rows: 0 };
  }
  recordFixtureState(join(dir, 'regression', side), 'diagnostics-schema', {
    run_id: lab.run_id, checked_at: new Date().toISOString(), container_id: pg.id,
    source: release.inputs.sourceSHA256, cli_sha256: release.binaries.infrapro,
    migration_sha256: release.schema.migrationSHA256, migrated: !restricted(before), before, after,
    restore_epoch: originalEpoch, transaction: 'read_only', checked_role: role, observed_rows: 0, invitation_read: invitationRead,
  });
}

// A running fault lab can outlive a candidate build. Start its registered
// resident processes from the selected release before observing their faults.
export async function bindDiagnosticsResidents(dir, side, cwd, release) {
  const lab = selectedLab(cwd);
  const { processAlive, resumeProcess, saveLab, stopProcess, waitHTTP } = await import(pathToFileURL(join(cwd, 'scripts/fixtures/diagnostics-lab.mjs')));
  const owners = Object.entries(lab.processes).filter(([name, proc]) => Object.hasOwn(release.binaries, name) && proc.executable === join(lab.directory, 'bin', name));
  const native = ['server', 'native-a', 'native-b', 'native-c'];
  assert(owners.length && native.every(name => lab.processes[name].executable === lab.binary));
  assert.deepEqual(lab.faults, []);
  let ingressFixture;
  if (side === 'baseline' && owners.some(([name]) => name === 'ingest-worker')) {
    const business = read(lab.business_config), file = join(lab.directory, 'diagnostics-ingestion.json');
    assert.equal(new URL(business.monitoring_object_endpoint).hostname, '127.0.0.1');
    assert.equal(new URL(business.monitoring_object_endpoint).port, String(lab.ports.s3));
    if (!business.ingestion_file) {
      const prior = join(lab.directory, 'reports/ingest-worker.json');
      const original = existsSync(prior) ? read(prior) : null;
      write(file, { enabled: true, storage_budget_bytes: '4294967296', queue_budget: 500000, relays: [] });
      write(lab.business_config, { ...business, ingestion_file: file });
      ingressFixture = {
        source: 'The frozen lab registered its default-disabled ingest-worker as required but supplied no ingestion_file.',
        prior_phase: original?.phase, prior_lanes: original?.lanes.map(({ id, state, reason_code }) => ({ id, state, reason_code })),
        retained_database_and_existing_object_store: true, configured_remote_relays: 0,
      };
      recordFixtureState(join(dir, 'regression', side), 'diagnostics-ingestion-fixture', ingressFixture);
    }
  }
  const serverSHA = digest(readFileSync(lab.binary));
  if (release.binaries['infrapro-diagnostics']) assert.equal(serverSHA, release.binaries['infrapro-diagnostics']);
  const cli = join(release.bin, 'infrapro');
  assert.equal(digest(readFileSync(cli)), release.binaries.infrapro);
  for (const [name] of owners) assert.equal(digest(readFileSync(join(release.bin, name))), release.binaries[name]);
  const names = [...owners.map(([name]) => name), ...native];
  const prepared = [], errors = [];
  try {
    for (const [name, proc] of owners) {
      if (digest(readFileSync(proc.executable)) === release.binaries[name]) continue;
      const temp = proc.executable + '.release-' + process.pid;
      prepared.push([temp, proc.executable]);
      copyFileSync(join(release.bin, name), temp, constants.COPYFILE_FICLONE);
      chmodSync(temp, 0o755);
      assert.equal(digest(readFileSync(temp)), release.binaries[name]);
    }
    try {
      for (const name of [...names].reverse()) await stopProcess(lab, name);
      alignDiagnosticsProjection(dir, side, cwd, lab, release, cli);
      for (const [temp, destination] of prepared) renameSync(temp, destination);
      lab.cli = cli;
      saveLab(lab);
    } catch (error) {
      errors.push(error);
    }
    for (const name of names) {
      try { await resumeProcess(lab, name); }
      catch (error) { errors.push(error); }
    }
    if (errors.length) throw new AggregateError(errors, 'Could not refresh all isolated diagnostics processes');
  } finally {
    for (const [temp] of prepared) rmSync(temp, { force: true });
  }
  await waitHTTP(lab.origin);
  const { diagnosticClient, eventually } = await import('./fixtures/diagnostics-client.mjs');
  const operator = await diagnosticClient(lab, { loginTimeoutMs: 75000 });
  let coverage;
  try {
    coverage = await eventually(async () => {
      const { data } = await operator.get('/overview');
      return data.coverage.complete && !data.abnormal_components.length && data.coverage;
    }, 70000);
  } finally {
    await operator.logout();
  }
  const processes = names.map(name => {
    const proc = lab.processes[name];
    assert(processAlive(lab, name), 'Release process stopped during setup: ' + name);
    const sha = digest(readFileSync(proc.executable));
    assert.equal(sha, native.includes(name) ? serverSHA : release.binaries[name]);
    return { name, pid: proc.identity.pid, sha256: sha };
  });
  recordFixtureState(join(dir, 'regression', side), 'diagnostics-residents', {
    run_id: lab.run_id, recorded_at: new Date().toISOString(), source: release.inputs?.sourceSHA256 || release.source_archive_sha256,
    cli_sha256: release.binaries.infrapro, processes, observed_coverage: coverage,
    native_source: release.binaries['infrapro-diagnostics'] ? 'matched selected release binary' : 'built by original diagnostics environment command from verified frozen product sources',
  });
}

export function diagnosticFrameClock(dir, side, cwd) {
  const name = 'services/diagnostics/internal/transport/http/server_test.go';
  const source = join(cwd, name), original = readFileSync(source, 'utf8');
  const old = 'model.New(p, identity.BootID, time.Now().UTC())';
  const correction = 'model.New(p, identity.BootID, f.GeneratedAt)';
  assert.equal(original.split(old).length, 2, 'Review the frozen report fixture before applying the common clock');
  const selected = original.replace(old, correction);
  for (const assertion of ['if status != 503 {', 'if len(l.app.Engine.Read().Latest) != 0 {'])
    assert.equal(selected.split(assertion).length, original.split(assertion).length);
  const folder = join(dir, 'private', 'diagnostics-clock', side);
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const file = join(folder, 'server_test.go'), overlay = join(folder, 'overlay.json');
  writeFileSync(file, selected, { mode: 0o600 });
  write(overlay, { Replace: { [source]: file } });
  const receipt = { file: name, test: 'TestRealHTTPIngestionCommitFailureReturns503WithoutPartialState',
    original_sha256: digest(original), effective_sha256: digest(selected), overlay_sha256: digest(readFileSync(overlay)),
    original: old, shared: correction };
  recordFixtureState(join(dir, 'regression', side), 'diagnostics-frame-clock', receipt);
  return { overlay, receipt };
}

export function diagnosticsRegressionEnvironment(dir, side, cwd) {
  const target = join(dir, 'private', 'baseline-diagnostics-harness');
  // Both unmodified original runners build this tag. Run A and B sequentially,
  // then retain a separate tag for the exact image observed in each real deployment.
  const env = { GOWORK: join(cwd, 'go.work'), DIAGNOSTICS_IMAGE: 'infrapro-diagnostics:local' };
  const offset = Number.parseInt(digest(dir+'-'+side).slice(0,2),16);
  env.DIAGNOSTICS_STANDALONE_SUBNET = unusedDockerSubnet('docker', '10.243', offset);
  if (side !== 'baseline') return env;
  mkdirSync(target, { recursive: true, mode: 0o700 });
  const frozen = read(join(dir, 'baseline.json'));
  const relevant = /^(?:apps\/|services\/|packages\/|tools\/|contracts\/|deploy\/diagnostics\/|scripts\/(?:fixtures\/)?(?:test-)?diagnostics[-.]|go\.work|package\.json|pnpm-lock\.yaml)/;
  const inputs = Object.keys(frozen.sourceFiles).filter(file => relevant.test(file)).sort().map(file => [file, frozen.sourceFiles[file]]);
  assert(inputs.length > 0);
  for (const [file, sha] of inputs) assert.equal(digest(readFileSync(join(cwd, file))), sha, 'Frozen source changed: ' + file);
  const replacements = {}, changes = [];
  for (const [file, assertions] of Object.entries(fixtures)) {
    const path = join(cwd, 'scripts', file), before = readFileSync(path, 'utf8'), shared = readFileSync(join(root, 'scripts', file), 'utf8');
    const cases = source => [...source.matchAll(/\b(?:record\(|case\s*)'([^']+)'/g)].map(match => match[1]).sort();
    assert.deepEqual(cases(before), cases(shared), 'Different diagnostic cases in ' + file);
    const copy = join(target, file);
    writeFileSync(copy, shared, { mode: 0o600 });
    replacements[pathToFileURL(path).href] = copy;
    changes.push({ file, frozen_sha256: digest(before), effective_sha256: digest(shared), cases: cases(before), assertions });
  }
  // S0 already enforces this keyring when its personnel API starts. Its old
  // diagnostics fixture predates that configuration requirement.
  const setupFile = 'scripts/fixtures/diagnostics-business.mjs';
  const setupPath = join(cwd, setupFile), setupBefore = readFileSync(setupPath, 'utf8');
  const marker = '  const business = {\n';
  const withKeyring = [
    "  const keyring = join(lab.directory, 'mfa-keyring.json');",
    "  privateJSON(keyring, { format_version: 1, active_key_id: 'lab', keys: [{id: 'lab', key_base64: randomBytes(32).toString('base64')}] });",
    '  const business = {', '    mfa_keyring_file: keyring,', '',
  ].join('\n');
  assert.equal(setupBefore.split(marker).length, 2);
  assert(!setupBefore.includes('mfa_keyring_file'));
  assert(readFileSync(join(root, setupFile), 'utf8').includes(withKeyring), 'Candidate keyring fixture differs');
  const setupEffective = setupBefore.replace(marker, withKeyring);
  const setupCopy = join(target, 'diagnostics-business.mjs');
  writeFileSync(setupCopy, setupEffective, { mode: 0o600 });
  replacements[pathToFileURL(setupPath).href] = setupCopy;
  const clientFile = 'scripts/fixtures/diagnostics-client.mjs';
  const clientPath = join(cwd, clientFile), clientBefore = readFileSync(clientPath), clientShared = readFileSync(join(root, clientFile));
  const clientCopy = join(target, 'diagnostics-client.mjs');
  writeFileSync(clientCopy, clientShared, { mode: 0o600 });
  replacements[pathToFileURL(clientPath).href] = clientCopy;
  const templatesFile = 'scripts/diagnostics-templates.mjs';
  const templatesPath = join(cwd, templatesFile), originalTemplates = readFileSync(templatesPath, 'utf8');
  const template = baselineTemplateReferences(cwd, target);
  let comparedTemplates = originalTemplates;
  if (template.comparisons.length) {
    const originalRead = "readFileSync(path,'utf8')!==text";
    assert.equal(originalTemplates.split(originalRead).length, 2);
    comparedTemplates = originalTemplates.replace(originalRead,
      "readFileSync((" + JSON.stringify(template.references) + ")[path] ?? path,'utf8')!==text");
    const staged = join(target, 'diagnostics-templates.mjs');
    writeFileSync(staged, comparedTemplates, { mode: 0o600 });
    replacements[pathToFileURL(templatesPath).href] = staged;
  }
  const runner = 'scripts/test-diagnostics.mjs';
  const runnerSHA = digest(readFileSync(join(cwd, runner)));
  assert.equal(runnerSHA, digest(readFileSync(join(root, runner))), 'Different original diagnostics entry points');
  write(join(dir, 'regression', side, 'diagnostics-harness.json'), {
    release: frozen.commit, runner, runner_sha256: runnerSHA,
    matched_archive_files: inputs.length, matched_inventory_sha256: digest(JSON.stringify(inputs)),
    original_browser_receipt: 'test-diagnostics-frozen-browser.json', changes,
    templates: { file: templatesFile, original_receipt: 'test-diagnostics-frozen-templates.json',
      frozen_sha256: digest(originalTemplates), effective_sha256: digest(comparedTemplates), comparisons: template.comparisons,
      difference: 'Retain the unmodified byte check and its exit separately. Three stale release manifests are compared against independently reconstructed expectations from the frozen role catalogue. All other original template bytes and assertions are unchanged.' },
    client: {
      file: clientFile, frozen_sha256: digest(clientBefore), effective_sha256: digest(clientShared),
      difference: 'Expose the real HTTP status and retry delay. Only an explicitly requested fixture login retry honors a 429, with a fixed deadline.',
    },
    setup: {
      file: setupFile, frozen_sha256: digest(setupBefore), effective_sha256: digest(setupEffective),
      difference: 'Create this isolated lab’s 0600 random keyring for its frozen personnel API. Where the S0 lab declares ingest-worker continuous, supply its original accepted ingestion_file with zero remote relays and its owned live PG/S3. Preserve every S0 role and probe.',
    },
  });
  write(join(target, 'files.json'), replacements);
  const hook = join(target, 'loader.mjs');
  writeFileSync(hook, [
    "import { registerHooks } from 'node:module';",
    "import { readFileSync } from 'node:fs';",
    "const files = JSON.parse(readFileSync(new URL('./files.json', import.meta.url)));",
    'registerHooks({ load(url, context, next) {',
    '  if (!Object.hasOwn(files, url)) return next(url, context);',
    "  return { format: 'module', source: readFileSync(files[url], 'utf8'), shortCircuit: true };",
    '} });', '',
  ].join('\n'), { mode: 0o600 });
  env.NODE_OPTIONS = [process.env.NODE_OPTIONS, '--import=' + pathToFileURL(hook).href].filter(Boolean).join(' ');
  return env;
}

export function recordDiagnosticsFixture(dir, side, cwd, release) {
  const lab = selectedLab(cwd);
  const containers = diagnosticsContainers(lab);
  assert(containers.every(container => container.running));
  const postgres = containers.find(container => container.replica === 'postgres-1');
  const database = diagnosticsSchema(postgres);
  const expected = Math.max(...readdirSync(join(cwd, 'packages/go/platform/migrations')).map(file => Number(file.match(/^(\d+)_.*\.sql$/)?.[1] || 0)));
  assert.equal(database.version, expected);
  assert.equal(database.publication_permit, expected >= 59);
  assert.equal(database.tenant_pressure_index, expected >= 59);
  const residentFile = join(dir, 'regression', side, 'diagnostics-residents.json');
  const residents = read(residentFile), cliSHA = digest(readFileSync(lab.cli)), serverSHA = digest(readFileSync(lab.binary));
  assert.equal(residents.run_id, lab.run_id);
  assert.equal(residents.cli_sha256, cliSHA);
  assert.equal(residents.processes.find(proc => proc.name === 'server')?.sha256, serverSHA);
  recordFixtureState(join(dir, 'regression', side), 'diagnostics-fixture', {
    run_id: lab.run_id, source: release.inputs?.sourceSHA256 || release.source_archive_sha256, database_schema: database.version,
    publication_permit: database.publication_permit, tenant_pressure_index: database.tenant_pressure_index,
    fixed_baseline_metadata: database.fixed_baseline_metadata,
    runner_sha256: digest(readFileSync(join(cwd, 'scripts/test-diagnostics.mjs'))),
    native_server_sha256: serverSHA, cli_sha256: cliSHA,
    verified_residents: residents.processes.length, resident_evidence_sha256: digest(readFileSync(residentFile)),
    compose_project: lab.compose_project, containers,
    manifest_workers: read(lab.manifest_file).components.filter(component => component.kind === 'worker').length,
    ...(side === 'baseline' && { test_harness_sha256: digest(readFileSync(join(dir, 'regression', side, 'diagnostics-harness.json'))) }),
    catalog_sha256: digest(readFileSync(join(cwd, 'contracts/diagnostics/runtime-catalog.json'))),
  });
}

function collectSecrets(value, secrets) {
  if (!value || typeof value !== 'object') return;
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry === 'object') collectSecrets(entry, secrets);
    if (typeof entry !== 'string') continue;
    if (entry.length >= 8 && /(?:password|secret|secret_(?:access_)?key|csrf_auth_key|key_base64)$/i.test(key)) secrets.add(entry);
    if (key.endsWith('_url') && URL.canParse(entry)) {
      const password = new URL(entry).password;
      if (password) secrets.add(decodeURIComponent(password));
    }
  }
}

// Both invocations write into the same private evidence directory.
export function archiveOriginalDiagnostics(dir, side, cwd, result) {
  if (result.status === 'passed') {
    archiveDiagnosticsEvidence(dir, side, cwd, result, { original: true });
    return;
  }
  archiveOriginalDiagnosticCore(dir, side, cwd, result);
}

export function archiveOriginalDiagnosticCore(dir, side, cwd, result) {
  if (result.exitCode !== 1) return;
  const folder = join(dir, 'regression', side);
  const lab = selectedLab(cwd), core = join(lab.directory, 'evidence', 'core-go.jsonl');
  if (!readFileSync(join(folder, result.log), 'utf8').includes('Error: Diagnostics core checks failed: ' + core)) return;
  const modifiedAt = statSync(core).mtime.toISOString();
  assert(modifiedAt >= result.startedAt && modifiedAt <= result.finishedAt);
  const contents = readFileSync(core), sha256 = digest(contents);
  const testEvents = contents.toString('utf8').split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line));
  const failing = testEvents.filter(event => event.Action === 'fail' && event.Test)
    .map(event => ({package: event.Package, test: event.Test}));
  const failure = 'TestRealHTTPIngestionCommitFailureReturns503WithoutPartialState';
  const frameClockOnly = failing.length === 1 && failing[0].test === failure &&
    testEvents.filter(event => event.Action === 'fail' && !event.Test).every(event => event.Package === failing[0].package) &&
    !testEvents.some(event => event.Action === 'build-fail') &&
    testEvents.some(event => event.Test === failure && event.Output?.includes('uncommitted HTTP acknowledgement 400'));
  const stored = join(folder, 'attempts', 'original-diagnostic-core-' + sha256.slice(0,16), 'core-go.jsonl');
  mkdirSync(dirname(stored), {recursive: true, mode: 0o700});
  if (!existsSync(stored)) writeFileSync(stored, contents, {flag: 'wx', mode: 0o600});
  recordFixtureState(folder, 'diagnostics-unmodified-core', {
    run_id: lab.run_id, finished_at: result.finishedAt, sha256, file: relative(dir,stored), failing, frame_clock_only: frameClockOnly,
  });
}

export function checkDiagnosticsBinaries(dir, side, cwd) {
  const folder = join(dir, 'regression', side);
  const fixture = read(join(folder, 'diagnostics-fixture.json')), initial = read(join(folder, 'diagnostics-residents.json'));
  const aggregate = read(join(folder, 'test-diagnostics.json'));
  assert.equal(initial.run_id, fixture.run_id);
  assert(Number.isInteger(aggregate.exitCode) && aggregate.finishedAt);
  const pointer = read(join(cwd, '.artifacts/diagnostics/active.json'));
  const lab = read(join(pointer.directory, 'lab.json'));
  assert.equal(lab.run_id, fixture.run_id);
  assert.equal(lab.run_id, pointer.run_id);
  assert.equal(lab.directory, pointer.directory);
  assert(['running', 'parked'].includes(lab.state));
  const parked = lab.state === 'parked' ? read(join(folder, 'diagnostics-parked.json')) : null;
  if (parked) {
    assert.equal(parked.run_id, lab.run_id);
    assert(parked.started_at >= aggregate.finishedAt);
    assert(!Object.values(parked.after.processes).some(Boolean));
    assert(parked.after.containers.every(service => !service.running));
  }
  const programs = initial.processes.map(item => {
    const proc = lab.processes[item.name];
    const modified = statSync(proc.executable).mtime.toISOString();
    const sha256 = digest(readFileSync(proc.executable));
    let identity = proc.identity, confirmed;
    if (parked) {
      const atStop = parked.release_binaries?.find(row => row.name === item.name);
      confirmed = parked.before.processes[item.name] && modified <= parked.started_at &&
        (!atStop || (atStop.actual_sha256 === sha256 && atStop.pid === identity.pid));
    } else {
      identity = JSON.parse(execFileSync(lab.helper, ['identity', String(proc.identity.pid)], { encoding: 'utf8', timeout: 5000 }));
      confirmed = identity.pid === proc.identity.pid && identity.os_start_time_ms === proc.identity.os_start_time_ms &&
        realpathSync(identity.executable) === realpathSync(proc.executable);
    }
    return { role: item.name, expected_sha256: item.sha256, actual_sha256: sha256, pid: identity.pid,
      os_start_time_ms: identity.os_start_time_ms, modified_at: modified, confirmed, matches: confirmed && sha256 === item.sha256 };
  });
  const matched = programs.filter(program => program.matches).length;
  const record = { run_id: lab.run_id, source: fixture.source, checked_at: new Date().toISOString(), aggregate_finished_at: aggregate.finishedAt,
    phase: parked ? 'parked' : 'after_command', expected: programs.length, matched, passed: matched === programs.length, programs };
  if (parked) {
    record.parked_at = parked.finished_at;
    record.shutdown_evidence_sha256 = digest(readFileSync(join(folder, 'diagnostics-parked.json')));
  }
  const name = 'diagnostics-binaries-' + record.phase;
  recordFixtureState(folder, name, record);
  return record;
}

export function compareDiagnosticsEvidence(dir) {
  const baseline = join(dir, 'diagnostics-baseline'), candidate = join(dir, 'diagnostics');
  if (![baseline, candidate].every(folder => existsSync(join(folder, 'summary.json')))) return;
  const a = read(join(baseline, 'summary.json')), b = read(join(candidate, 'summary.json'));
  if (a.aggregate_status !== 'passed' || b.aggregate_status !== 'passed') return;
  const selected = join(dir, 'candidate.evaluated.json');
  const release = read(existsSync(selected) ? selected : join(dir, 'candidate.json'));
  if (b.fixture.source !== release.inputs.sourceSHA256) return;
  assert.equal(a.fixture.source, read(join(dir, 'baseline.json')).source_archive_sha256);
  assert.notEqual(a.run_id, b.run_id, 'Both releases refer to the same diagnostics lab');
  assert.equal(a.fixture.runner_sha256, b.fixture.runner_sha256);
  assert.deepEqual(a.command, b.command);
  assert.deepEqual(a.suites.map(suite => suite.name), b.suites.map(suite => suite.name));
  const common = {};
  for (const name of ['faults', 'browser']) {
    const collect = folder => read(join(folder, name + '.json')).cases
      .map(row => ({ case: row.case, result: row.result })).sort((x, y) => x.case.localeCompare(y.case));
    const cases = collect(baseline);
    assert.deepEqual(cases, collect(candidate), 'Different paired diagnostic outcomes for ' + name);
    assert.equal(new Set(cases.map(row => row.case)).size, cases.length);
    assert(cases.length && cases.every(row => row.result === 'passed'));
    common[name] = cases;
  }
  const observed = row => ({
    run_id: row.run_id, schema: row.fixture.database_schema, source: row.fixture.source,
    server_sha256: row.fixture.native_server_sha256, cli_sha256: row.fixture.cli_sha256,
    image: row.image.digest, suites: row.suites.length, screenshots: row.screenshots.length,
    exit_code: row.exit_code, finished_at: row.finished_at,
  });
  const folder = join(dir, 'regression/baseline');
  const original = Object.fromEntries([
    ['aggregate', 'test-diagnostics-unmodified'],
    ['templates', 'test-diagnostics-frozen-templates'],
    ['browser', 'test-diagnostics-frozen-browser'],
  ].map(([check, name]) => {
    const result = read(join(folder, name + '.json'));
    assert.equal(result.provenance.fixtureOverlay, false);
    assert.equal(result.provenance.commit, read(join(dir, 'baseline.json')).commit);
    assert(Number.isInteger(result.exitCode));
    return [check, { command: result.command, exit_code: result.exitCode, reason: result.failure,
      finished_at: result.finishedAt, log: 'regression/baseline/' + result.log,
      log_sha256: digest(readFileSync(join(folder, result.log))) }];
  }));
  const archivedHarness = join(baseline, 'fixture-harness.json');
  const harnessFile = existsSync(archivedHarness) ? archivedHarness : join(folder, 'diagnostics-harness.json');
  const fixture = read(harnessFile);
  original.template_differences = fixture.templates.comparisons;
  if (original.aggregate.exit_code !== 0 || original.templates.exit_code !== 0) {
    assert.equal(original.aggregate.exit_code, 1);
    assert.equal(original.templates.exit_code, 1);
    assert.equal(original.template_differences.length, 3);
    const templateDrift = 'Error: Deployment template drift: deploy/diagnostics/native/manifest.json';
    assert(readFileSync(join(dir, original.templates.log), 'utf8').includes(templateDrift));
    if (!readFileSync(join(dir, original.aggregate.log), 'utf8').includes(templateDrift)) {
      const log = read(join(folder, 'diagnostics-unmodified-core.json'));
      assert(log.run_id === a.run_id && log.finished_at === original.aggregate.finished_at && log.frame_clock_only &&
        digest(readFileSync(join(dir,log.file))) === log.sha256, 'Unclassified failure in a frozen diagnostics command');
      original.aggregate.core_failure = log;
    }
  }
  const pagination = join(folder, 'frozen-diagnostics-browser/worker-pagination.json');
  if (existsSync(pagination)) {
    const actual = read(pagination);
    assert.equal(actual.run_id, a.run_id);
    assert.equal(actual.source, a.fixture.source);
    original.pagination = { worker_count: actual.worker_ids.length, actual: actual.actualPageText,
      expected_frozen: actual.expectedFrozenText, retained_api_links: actual.visibleAPILinks,
      evidence: relative(dir, pagination), screenshot_sha256: actual.screenshotSHA256 };
  }
  if (original.browser.exit_code !== 0) {
    assert.equal(original.browser.exit_code, 1);
    assert(original.pagination);
    assert(readFileSync(join(dir, original.browser.log), 'utf8').includes(original.pagination.expected_frozen));
  }
  const release_binaries = {};
  for (const [side, summary] of [['baseline', a], ['candidate', b]]) {
    const phases = [];
    for (const phase of ['after_command', 'parked']) {
      const file = join('regression', side, 'diagnostics-binaries-' + phase + '.json');
      if (!existsSync(join(dir, file))) continue;
      const bytes = readFileSync(join(dir, file)), row = JSON.parse(bytes);
      if (row.run_id !== summary.run_id || row.source !== summary.fixture.source || row.aggregate_finished_at !== summary.finished_at) continue;
      phases.push({ phase, passed: row.passed, expected: row.expected, matched: row.matched, evidence: file, evidence_sha256: digest(bytes) });
    }
    release_binaries[side] = { qualified: phases.length > 0 && phases.every(phase => phase.passed), phases };
  }
  const rawCandidateFile = join(dir, 'regression/candidate/test-diagnostics-unmodified.json');
  const rawCandidate = existsSync(rawCandidateFile) ? read(rawCandidateFile) : null;
  const sharedClock = { baseline:a.frame_clock ?? null, candidate:b.frame_clock ?? null,
    passed:Boolean(a.frame_clock && b.frame_clock && a.frame_clock.original_sha256===b.frame_clock.original_sha256 &&
      a.frame_clock.effective_sha256===b.frame_clock.effective_sha256 && a.frame_clock.file===b.frame_clock.file &&
      a.frame_clock.test===b.frame_clock.test),
    candidate_original:rawCandidate?.provenance?.releaseSHA256===b.fixture.source ? {
      exit_code:rawCandidate.exitCode, command:rawCandidate.command, file:relative(dir,rawCandidateFile),
      sha256:digest(readFileSync(rawCandidateFile)), log_sha256:digest(readFileSync(join(dirname(rawCandidateFile),rawCandidate.log))),
    } : null };
  const networks = [baseline, candidate].map(folder => read(join(folder,'deployment.json')).network ?? null);
  const sharedNetwork = {baseline:networks[0],candidate:networks[1],passed:networks.every((network,index)=>
    network?.removed && network.project==='infrapro-'+[a,b][index].run_id+'-standalone' &&
    /^10\.243\.\d+\.0\/24$/.test(network.subnet) && network.actual_address.split('.').slice(0,3).join('.')===network.subnet.split('.').slice(0,3).join('.'))};
  write(join(dir, 'diagnostics-comparison.json'), {
    command: a.command, runner_sha256: a.fixture.runner_sha256, baseline: observed(a), candidate: observed(b),
    baseline_original_checks: original,
    common_fixture: { reference: relative(dir, harnessFile), sha256: digest(readFileSync(harnessFile)), changes: fixture.changes,
      baseline_exit_code: a.exit_code, candidate_exit_code: b.exit_code },
    release_binaries, common, common_frame_clock:sharedClock, common_compose_network:sharedNetwork,
  });
}

// Keep the frozen driver's actual page and assertions even after the common
// diagnostic browser driver writes new evidence into the same isolated lab.
export async function archiveFrozenDiagnosticsBrowser(dir, cwd, result) {
  const lab = selectedLab(cwd), evidence = join(lab.directory, 'evidence');
  const folder = join(dir, 'regression', 'baseline', 'frozen-diagnostics-browser');
  const original = read(join(evidence, 'browser.json'));
  assert.equal(original.run_id, lab.run_id);
  assert.equal(original.result, result.status);
  const files = ['browser.json', ...(result.status === 'failed' ? ['browser-failure.png'] : original.screenshots)];
  const records = [];
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  for (const file of files) {
    assert.match(file, /^(?:browser\.json|browser-[a-z-]+\.png)$/);
    const path = join(evidence, file), written = statSync(path).mtime.toISOString();
    assert(written >= result.startedAt && written <= result.finishedAt, 'Stale original diagnostic browser file: ' + file);
    const data = readFileSync(path);
    writeFileSync(join(folder, file), data, { mode: 0o600 });
    records.push({ file, sha256: digest(data) });
  }
  write(join(folder, 'summary.json'), {
    run_id: lab.run_id, command: result.command, exit_code: result.exitCode, result: original.result,
    started_at: result.startedAt, finished_at: result.finishedAt, cases: original.cases, files: records,
  });
  if (result.exitCode === 0 || !readFileSync(join(dir, 'regression/baseline', result.log), 'utf8').includes('共 8 项 · 当前快照')) return;
  const { chromium, expect } = await import('@playwright/test');
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ locale: 'zh-CN', viewport: { width: 1440, height: 1000 } });
    const account = read(lab.accounts_file);
    await page.goto(lab.origin + '/login');
    await page.getByLabel('诊断账号', { exact: true }).fill(account.username);
    await page.getByLabel('密码', { exact: true }).fill(account.password);
    await page.getByRole('button', { name: '登录诊断', exact: true }).click();
    await expect(page.locator('#diagnostics-content')).toBeVisible({ timeout: 15000 });
    await page.goto(lab.origin + '/components');
    await page.getByLabel('类型', { exact: true }).selectOption('worker');
    const worker_ids = read(lab.manifest_file).components.filter(row => row.kind === 'worker').map(row => row.id);
    const actualPageText = await page.getByText(/^共\s*\d+\s*条$/).textContent({ timeout: 10000 });
    assert.equal(actualPageText, '共 ' + worker_ids.length + ' 条');
    const expectedFrozenText = '共 8 项 · 当前快照';
    const visibleFrozenText = await page.getByText(expectedFrozenText, { exact: true }).count();
    const visibleAPILinks = await page.getByRole('link', { name: 'API', exact: true }).count();
    assert.equal(visibleFrozenText, 0);
    assert.equal(visibleAPILinks, 0);
    const screenshot = 'worker-pagination.png';
    const png = await page.screenshot({ path: join(folder, screenshot) });
    write(join(folder, 'worker-pagination.json'), {
      at: new Date().toISOString(), run_id: lab.run_id, source: read(join(dir, 'baseline.json')).source_archive_sha256,
      manifestSHA256: digest(readFileSync(lab.manifest_file)), worker_ids, expectedFrozenText, visibleFrozenText,
      actualPageText, visibleAPILinks, screenshot, screenshotSHA256: digest(png),
    });
  } finally {
    await browser.close();
  }
}

export function archiveDiagnosticsEvidence(dir, side, cwd, result, { original = false } = {}) {
  if (result.status !== 'passed') return;
  const lab = selectedLab(cwd);
  const fixture = read(join(dir, 'regression', side, 'diagnostics-fixture.json'));
  assert.equal(fixture.run_id, lab.run_id);
  assert.deepEqual(lab.faults, []);
  const source = join(lab.directory, 'evidence');
  const folder = original
    ? join('regression', side, 'attempts', 'original-diagnostics-' + result.startedAt.replaceAll(':', '-'))
    : side === 'candidate' ? 'diagnostics' : 'diagnostics-baseline';
  const target = join(dir, folder);
  const secrets = new Set();
  const keyring = read(lab.business_config).mfa_keyring_file;
  for (const file of [lab.accounts_file, lab.business_config, keyring, ...['native-a', 'native-b', 'native-c'].map(node => join(lab.directory, node, 'key.json'))]) {
    collectSecrets(read(file), secrets);
  }
  const summary = {
    run_id: lab.run_id, release: side, recorded_at: new Date().toISOString(), command: result.command,
    aggregate_status: result.status, exit_code: result.exitCode, started_at: result.startedAt,
    finished_at: result.finishedAt, duration_ms: result.durationMs, fixture, suites: [], screenshots: [],
  };
  const files = new Map();
  if (result.provenance?.diagnosticFrameClock) {
    const clock = readFileSync(join(dir, 'regression', side, 'diagnostics-frame-clock.json'));
    assert.deepEqual(JSON.parse(clock), result.provenance.diagnosticFrameClock);
    summary.frame_clock = result.provenance.diagnosticFrameClock;
    files.set('frame-clock.json', clock);
  }
  if (side === 'baseline' && !original) {
    const bytes = readFileSync(join(dir, 'regression', side, 'diagnostics-harness.json'));
    const sha = digest(bytes);
    assert.equal(fixture.test_harness_sha256, sha);
    summary.common_fixture_sha256 = sha;
    files.set('fixture-harness.json', bytes);
  }
  for (const name of suiteNames) {
    const file = name + '.json', buffer = readFileSync(join(source, file)), report = JSON.parse(buffer);
    assert.equal(report.run_id, lab.run_id);
    assert.equal(report.result, 'passed', name);
    // Four original Go receipts have no internal timestamp. Their source file
    // must also have been written by this invocation in the private lab.
    const sourceWrite = statSync(join(source, file)).mtime.toISOString();
    assert(sourceWrite >= result.startedAt && sourceWrite <= result.finishedAt, 'Stale diagnostic file for ' + name);
    if (report.recorded_at) assert(report.recorded_at >= result.startedAt && report.recorded_at <= result.finishedAt, 'Stale diagnostic evidence in ' + name);
    for (const secret of secrets) assert(!buffer.includes(secret), 'Sensitive value in ' + name);
    files.set(file, buffer);
    summary.suites.push({ name, cases: report.cases?.length ?? null, source_written_at: sourceWrite, sha256: digest(buffer) });
    if (name === 'deployment') {
      assert.match(report.image, /^sha256:[a-f0-9]{64}$/);
      assert.equal(report.build_id, read(join(cwd, 'build/bin/infrapro-diagnostics-linux.build.json')).build_id);
      const tag = 'infrapro-diagnostics:arc-' + side + '-' + digest(dir).slice(0, 12) + '-' + report.image.slice(7, 19);
      summary.image = { digest: report.image, tag, build_id: report.build_id };
    }
    if (name === 'browser') {
      assert.equal(report.build_id, read(join(cwd, 'build/bin/infrapro-diagnostics.build.json')).build_id);
      assert.equal(new Set(report.screenshots).size, 9);
      assert.equal(report.screenshots.length, 9);
      for (const screenshot of report.screenshots) {
        assert.match(screenshot, /^browser-[a-z-]+\.png$/);
        const path = join(source, screenshot), written = statSync(path).mtime.toISOString();
        assert(written >= result.startedAt && written <= result.finishedAt, 'Stale diagnostic screenshot: ' + screenshot);
        const png = readFileSync(path);
        assert(png.length > 1000 && png.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex')));
        files.set(screenshot, png);
        summary.screenshots.push({ file: screenshot, source_written_at: written, sha256: digest(png) });
      }
    }
  }
  if (original) {
    assert.equal(result.provenance.fixtureOverlay, false);
    if (side === 'baseline') {
      const baseline = read(join(dir, 'baseline.json'));
      assert.equal(baseline.commit, result.provenance.commit);
      assert.equal(fixture.source, baseline.source_archive_sha256);
    } else assert.equal(fixture.source, result.provenance.releaseSHA256);
    summary.fixture = { source: fixture.source, fixtureOverlay: false };
    delete summary.image.tag;
    mkdirSync(dirname(target), { recursive: true, mode: 0o700 });
    const staging = mkdtempSync(target + '.new-');
    try {
      for (const [file, data] of files) writeFileSync(join(staging, file), data, { mode: 0o600 });
      write(join(staging, 'command.json'), result);
      write(join(staging, 'summary.json'), summary);
      renameSync(staging, target);
    } finally { rmSync(staging, { recursive: true, force: true }); }
    return;
  }
  const buildFile = join(source, 'owning-process-builds.json');
  const builds = readFileSync(buildFile), buildReceipt = JSON.parse(builds);
  assert.equal(buildReceipt.run_id, lab.run_id);
  assert(buildReceipt.recorded_at >= result.startedAt && buildReceipt.recorded_at <= result.finishedAt);
  const initial = read(join(dir, 'regression', side, 'diagnostics-residents.json'));
  const owners = initial.processes.filter(proc => !['server', 'native-a', 'native-b', 'native-c'].includes(proc.name));
  assert.deepEqual(buildReceipt.owners.map(item => item.role).sort(), owners.map(proc => proc.name).sort());
  for (const owner of buildReceipt.owners) {
    assert.equal(owner.served_sha256, owner.selected_sha256);
    assert.equal(owner.selected_sha256, owners.find(proc => proc.name === owner.role).sha256);
    assert.match(owner.independent_compile_sha256, /^[a-f0-9]{64}$/);
  }
  files.set('owning-process-builds.json', builds);
  summary.independent_owner_builds = { file: 'owning-process-builds.json', sha256: digest(builds), roles: buildReceipt.built_roles.length };
  execFileSync('docker', ['image', 'tag', summary.image.digest, summary.image.tag], { stdio: 'pipe', timeout: 30000 });
  const staged = mkdtempSync(target + '.new-');
  let prior;
  try {
    for (const [file, buffer] of files) writeFileSync(join(staged, file), buffer, { mode: 0o600 });
    write(join(staged, 'summary.json'), summary);
    if (existsSync(target)) {
      const attempts = join(dir, 'regression', side, 'attempts');
      mkdirSync(attempts, { recursive: true, mode: 0o700 });
      prior = join(mkdtempSync(join(attempts, 'diagnostics-evidence-')), folder);
      renameSync(target, prior);
    }
    renameSync(staged, target);
  } catch (error) {
    if (prior && existsSync(prior) && !existsSync(target)) renameSync(prior, target);
    throw error;
  } finally {
    rmSync(staged, { recursive: true, force: true });
  }
  result.diagnosticsEvidence = { summary: join(folder, 'summary.json'), suites: summary.suites.length, screenshots: summary.screenshots.length };
  if (prior && existsSync(join(prior, 'summary.json'))) result.diagnosticsEvidence.previousSummary = relative(dir, join(prior, 'summary.json'));
  const verified = checkDiagnosticsBinaries(dir, side, cwd);
  compareDiagnosticsEvidence(dir);
  assert(verified.passed, 'A registered binary differs from the selected diagnostics release at command completion');
}

async function changeLabState(dir, side, action, expectedID) {
  const cwd = side === 'baseline' ? join(dir, 'baseline-source') : root;
  process.chdir(cwd);
  const { loadLab, saveLab, compose, processAlive, stopProcess } =
    await import(pathToFileURL(join(cwd, 'scripts/fixtures/diagnostics-lab.mjs')));
  const { diagnosticClient, eventually } = await import('./fixtures/diagnostics-client.mjs');
  const lab = loadLab(), folder = join(dir, 'regression', side);
  const fixture = read(join(folder, 'diagnostics-fixture.json'));
  assert.equal(lab.run_id, fixture.run_id);
  if (expectedID) assert.equal(lab.run_id, expectedID);
  assert.equal(lab.compose_project, 'infrapro-' + lab.run_id);
  assert.equal(new URL(lab.origin).hostname, '127.0.0.1');
  assert.deepEqual(lab.faults, []);
  const snapshot = () => {
    const containers = diagnosticsContainers(lab);
    const processes = Object.fromEntries(Object.keys(lab.processes).map(name => [name, processAlive(lab, name)]));
    return { containers, processes };
  };
  const clientRead = async () => {
    const client = await diagnosticClient(lab, { loginTimeoutMs: 75000 });
    try {
      const data = await eventually(async () => {
        const { data } = await client.get('/overview');
        return data.coverage.complete && !data.abnormal_components.length && data;
      }, 70000);
      return { fresh: data.coverage.fresh, expected: data.coverage.expected, ready: data.summary.ready_replicas };
    } finally { await client.logout(); }
  };
  const before = snapshot(), parking = action === '--park';
  const identities = containers => containers.map(({ id, replica, volumes }) => ({ id, replica, volumes }));
  if (fixture.containers) {
    assert.equal(fixture.compose_project, lab.compose_project);
    assert.deepEqual(identities(before.containers), identities(fixture.containers));
  }
  const record = { run_id: lab.run_id, action, started_at: new Date().toISOString(), before };
  if (parking) {
    if (lab.state === 'parked' && !Object.values(before.processes).some(Boolean) && before.containers.every(c => !c.running)) {
      const saved = join(folder, 'diagnostics-parked.json');
      if (existsSync(saved) && read(saved).run_id === lab.run_id) {
        console.log(lab.run_id + ' already parked');
        return;
      }
    }
    if (Object.values(before.processes).some(Boolean)) {
      try { record.client_before_stop = await clientRead(); }
      catch (error) { record.client_before_stop = { error: error.code ?? error.name, status: error.status ?? null }; }
    }
    const original = read(join(folder, 'diagnostics-residents.json'));
    assert.equal(original.run_id, lab.run_id);
    record.release_binaries = original.processes.map(({ name, sha256 }) => {
      const proc = lab.processes[name];
      const actual = existsSync(proc.executable) ? digest(readFileSync(proc.executable)) : null;
      return { name, running: before.processes[name], pid: proc.identity?.pid, expected_sha256: sha256, actual_sha256: actual };
    });
    for (const name of Object.keys(lab.processes).reverse()) await stopProcess(lab, name);
    compose(lab, ['stop', '--timeout', '5']);
    lab.state = 'parked';
  } else {
    const original = read(join(folder, 'diagnostics-residents.json'));
    assert.equal(original.run_id, lab.run_id);
    record.restored_release_binaries = [];
    if (lab.state === 'parked') {
      assert(!Object.values(before.processes).some(Boolean));
      for (const { name, sha256 } of original.processes) {
        const current = lab.processes[name].executable;
        if (current !== join(lab.directory, 'bin', name)) continue;
        const existingSHA = digest(readFileSync(current));
        if (existingSHA === sha256) continue;
        const sealed = join(dirname(lab.cli), name);
        assert.equal(digest(readFileSync(sealed)), sha256, 'Original release cannot be restored: ' + name);
        const prior = join(lab.directory, 'previous-builds', existingSHA, name);
        mkdirSync(dirname(prior), { recursive: true, mode: 0o700 });
        if (!existsSync(prior)) copyFileSync(current, prior, constants.COPYFILE_FICLONE);
        const temp = current + '.restored-' + process.pid;
        try {
          copyFileSync(sealed, temp, constants.COPYFILE_FICLONE);
          chmodSync(temp, 0o755);
          assert.equal(digest(readFileSync(temp)), sha256);
          renameSync(temp, current);
        } finally { rmSync(temp, { force: true }); }
        record.restored_release_binaries.push({ name, previous_sha256: existingSHA, selected_sha256: sha256 });
      }
    }
    const { recoverLab } = await import(pathToFileURL(join(cwd, 'scripts/fixtures/diagnostics-faults.mjs')));
    await recoverLab(lab);
    lab.state = 'running';
    record.client_after_resume = await clientRead();
  }
  saveLab(lab);
  record.after = snapshot();
  assert.deepEqual(identities(before.containers), identities(record.after.containers));
  assert(record.after.containers.every(c => c.running === !parking));
  const activeRoles = Object.entries(record.after.processes).filter(([name]) => name !== 'fault-publisher');
  assert(activeRoles.every(([, up]) => up === !parking));
  if (parking) {
    await assert.rejects(fetch(lab.origin + '/healthz', { signal: AbortSignal.timeout(1500) }));
    assert(!Object.values(record.after.processes).some(Boolean));
  }
  record.finished_at = new Date().toISOString();
  recordFixtureState(folder, parking ? 'diagnostics-parked' : 'diagnostics-resumed', record);
  if (parking) {
    const verified = checkDiagnosticsBinaries(dir, side, cwd);
    compareDiagnosticsEvidence(dir);
    assert(verified.passed, 'A diagnostics executable changed during the original suite; all owned processes and containers are parked');
  }
  console.log(lab.run_id + ' ' + lab.state + ' (' + activeRoles.length + ' processes, ' + record.after.containers.length + ' containers)');
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  const [, , action, path, side, input] = process.argv;
  assert(path, 'An architecture run directory is required');
  const dir = runDirectory(path);
  if (action === '--compare') compareDiagnosticsEvidence(dir);
  else if (action === '--verify-binaries') {
    assert(['candidate', 'baseline'].includes(side));
    const cwd = side === 'baseline' ? join(dir, 'baseline-source') : root;
    const verified = checkDiagnosticsBinaries(dir, side, cwd);
    compareDiagnosticsEvidence(dir);
    console.log(side + ' release binaries: ' + verified.matched + '/' + verified.expected + ' (' + verified.phase + ')');
    if (!verified.passed) process.exitCode = 1;
  }
  else if (action === '--bind') {
    assert(['candidate', 'baseline'].includes(side));
    await bindDiagnosticsResidents(dir, side, process.cwd(), read(input));
  }
  else {
    assert(['candidate', 'baseline'].includes(side));
    assert(['--park', '--resume'].includes(action));
    await changeLabState(dir, side, action, input);
  }
}
