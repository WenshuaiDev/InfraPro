import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';
import { root } from './architecture-runtime.mjs';

const { values: options } = parseArgs({
  options: {
    output: { type: 'string' },
    target: { type: 'string' },
    'parser-package': { type: 'string' },
    'host-release': { type: 'string' },
    bundle: { type: 'string' },
    'manifest-sha256': { type: 'string' },
  },
});
if (!options.output) throw Error('Provide the isolated acceptance run under --output');
const out = resolve(options.output);
const target = options.target ?? 'linux/amd64';
if (!/^(linux|darwin)\/(amd64|arm64)$/.test(target)) throw Error('Unsupported release target');
if (Boolean(options.bundle) !== Boolean(options['manifest-sha256'])) {
  throw Error('Specify --bundle and its trusted --manifest-sha256 together');
}
const [goos, goarch] = target.split('/');
const deploy = join(root, 'scripts/architecture-deploy.mjs');
const json = path => JSON.parse(readFileSync(path, 'utf8'));
const sha = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const run = (program, args, cwd = root, env = process.env) =>
  execFileSync(program, args, { cwd, env, stdio: 'inherit' });

function retainedPackage(scratch) {
  if (options['parser-package']) return { directory: resolve(options['parser-package']) };
  const baseline = json(join(out, 'baseline.json'));
  assert.equal(baseline.schema, 58, 'A frozen schema-58 source is required to reconstruct the old machine package');
  assert.match(baseline.source_archive_sha256, /^[a-f0-9]{64}$/);
  assert.equal(sha(baseline.sourceArchive), baseline.source_archive_sha256, 'The frozen S0 archive changed');
  const source = join(scratch, 'source');
  mkdirSync(source, { mode: 0o700 });
  run('tar', ['-xf', baseline.sourceArchive, '-C', source]);
  const runner = join(scratch, 'runner');
  const directory = join(scratch, 'package');
  const parser = join(source, 'services/parser');
  const host = { ...process.env, GOWORK: 'off', GOPROXY: 'off', GOTOOLCHAIN: 'local' };
  delete host.GOOS;
  delete host.GOARCH;
  run('go', ['build', '-trimpath', '-ldflags', '-s -w', '-o', runner, './cmd/parser-runner'],
    parser, { ...host, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' });
  run('go', ['run', './cmd/parser-package', runner, join(source, 'samples/parsing'), directory], parser, host);
  return { directory, commit: baseline.commit, archive: baseline.source_archive_sha256 };
}

function methods(directory) {
  const measurement = json(join(directory, 'catalog.json')).entries.map(method => ({
    reference: ['measurement', method.method_id, method.method_version].join('/'),
    artifact: method.artifact_digest,
    manifest: method.manifest_digest,
    outputs: method.output_contract_digest,
    samples: method.sample_suite_digest,
  }));
  const capabilities = json(join(directory, 'ingestion-capabilities.json'));
  return measurement.concat(capabilities.identification_methods.map(({ method }) => ({
    reference: ['identification', method.id, method.version].join('/'),
    artifact: method.artifact_digest,
    manifest: method.manifest_digest,
  })));
}

function buildOrLocate(directory) {
  if (options.bundle) {
    const expected = options['manifest-sha256'];
    if (!/^[a-f0-9]{64}$/.test(expected)) throw Error('Invalid trusted manifest SHA-256');
    return { directory: resolve(options.bundle), expected };
  }
  mkdirSync(out, { recursive: true, mode: 0o700 });
  const bundle = mkdtempSync(join(out, 'offline-' + goos + '-' + goarch + '-'));
  run(process.execPath, [deploy, 'build-release', '--target', target, '--directory', bundle, '--parser-package', directory]);
  return { directory: bundle, expected: sha(join(bundle, 'release.json')) };
}

const scratch = mkdtempSync(join(tmpdir(), 'infrapro-retained-release-'));
try {
  const prior = retainedPackage(scratch);
  const oldMethods = methods(prior.directory);
  const oldHashes = [...new Set(oldMethods.map(method => method.artifact))].sort();
  assert(oldHashes.length > 0);
  for (const hash of oldHashes) {
    assert.match(hash, /^[a-f0-9]{64}$/);
    assert.equal(sha(join(prior.directory, 'artifacts', hash)), hash);
  }

  const bundle = buildOrLocate(prior.directory);
  const verified = JSON.parse(execFileSync(process.execPath, [
    deploy, 'verify-release', '--directory', bundle.directory, '--manifest-sha256', bundle.expected,
  ], { cwd: root, encoding: 'utf8' }));
  assert.deepEqual([verified.schema, verified.goos, verified.goarch], [59, goos, goarch]);

  const manifestPath = join(bundle.directory, 'release.json');
  const manifest = json(manifestPath);
  const memberHashes = new Map(manifest.files.map(({ path, sha256 }) => [path, sha256]));
  const roleHashes = Object.fromEntries(postgresRoleSQLFiles.map(path => {
    const actual = sha(join(bundle.directory, path));
    assert.equal(actual, memberHashes.get(path), 'The packaged role SQL does not match its manifest: ' + path);
    return [path, actual];
  }));
  const installed = join(bundle.directory, 'parser-package');
  const current = new Map(methods(installed).map(method => [method.reference, method]));
  for (const method of oldMethods) assert.deepEqual(current.get(method.reference), method, method.reference);
  for (const hash of oldHashes) assert.equal(sha(join(installed, 'artifacts', hash)), hash);
  const entrypoints = Object.values(manifest.entrypoints);
  for (const path of ['bin/api', 'bin/compat-api', 'bin/nats-server', 'deploy/architecture/replica-proxy.py']) {
    assert(entrypoints.includes(path), path);
  }
  // Use the shipped renderer and its own imports. This catches missing offline
  // dependencies even if the checkout could have satisfied those imports.
  const layout=join(scratch,'offline-native');
  const deployed=join(bundle.directory,'scripts/architecture-deploy.mjs');
  run(process.execPath,[deployed,'prepare','--mode','native','--directory',layout],bundle.directory);
  run(process.execPath,[deployed,'resource-limits','--directory',layout],bundle.directory,{});
  const capacity=json(join(layout,'resource-limits.json'));
  assert.equal(capacity.totals.split_application.ch_connections,27);
  const profileLayout=join(scratch,'offline-display-profile');
  const published=join(bundle.directory,'deploy/architecture');
  run(process.execPath,[deployed,'prepare','--mode','native','--directory',profileLayout,
    '--process-budgets',join(published,'display-processes.example.json'),'--max-inflight',join(published,'display-inflight.example.json')],bundle.directory);
  const selectedCapacity=json(join(profileLayout,'resource-limits.json'));
  assert.equal(selectedCapacity.totals.split_application.ch_connections,18);
  for(const side of ['split','schema58']){
    const cap=selectedCapacity.selected_process_profiles[side];
    assert.deepEqual([cap.cpu_cores,cap.memory_mib,cap.go_threads,cap.go_memory_mib],[13,2560,13,2048]);
  }
  const operatingPath=join(out,'load/display/resource-profile.json');
  let operatingProfile;
  if (existsSync(operatingPath)) {
    const operating=json(operatingPath);
    const compare=(rows,settings,connections)=>{
      assert.deepEqual(Object.keys(settings).sort(),Object.keys(rows).sort());
      for(const [role,[,ch,goThreads,memory]] of Object.entries(rows)){
        assert.equal(settings[role].go_threads,goThreads,role+' configured Go schedulers');
        assert.equal(settings[role].go_memory_mib,memory,role+' configured Go runtime memory');
        assert.equal(connections[role],ch,role+' configured physical ClickHouse pool total');
      }
    };
    const cpu=selectedCapacity.selected_process_profiles;
    const current=selectedCapacity.slots.filter(row=>cpu.roles.includes(row.role));
    compare(operating.roles.b,Object.fromEntries(current.map(row=>[row.role,row.process_resources])),
      Object.fromEntries(current.map(row=>[row.role,row.clickhouse.max_open])));
    compare(operating.roles.a,cpu.schema58_roles,Object.fromEntries(selectedCapacity.original_schema58_storage_processes.map(row=>[row.role,row.clickhouse.max_open])));
    operatingProfile={source_sha256:sha(operatingPath),schema58_roles:Object.keys(operating.roles.a).length,split_roles:current.length,
      compared:['clickhouse_driver_max_open','go_threads','go_memory_mib']};
  }
  const resources = json(join(bundle.directory, 'deploy/architecture/offline-members.json')).directories;
  const declared = Object.entries(resources).flatMap(([directory, members]) =>
    members.map(member => directory + '/' + member));
  const additions = ['deploy/events/broker.json', 'deploy/events/SHA256SUMS'];
  const packaged = manifest.files.map(file => file.path).filter(path =>
    Object.keys(resources).some(directory => path.startsWith(directory + '/')));
  assert.deepEqual(packaged.sort(), [...declared, ...additions].sort());

  const selected = options['host-release'] || process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE;
  const hostManifest = join(selected ? resolve(selected) : out, 'candidate.json');
  let host;
  if (selected || existsSync(hostManifest)) {
    const release = json(hostManifest);
    assert.equal(manifest.commit, release.inputs.commit, 'The staged application and offline build use different revisions');
    assert.equal(manifest.application_source_sha256, release.inputs.sourceSHA256, 'The staged application and offline build use different software inputs');
    const web = Object.fromEntries(manifest.files.filter(f => f.path.startsWith('web/'))
      .map(({path, sha256}) => [path.slice(4),sha256]));
    assert.deepEqual(web, release.staticFiles, 'The staged console, display shell or diagnostics assets differ');
    for (const [name,expected] of Object.entries(release.binaries)) {
      assert.equal(sha(join(release.bin,name)),expected,'The selected host binary changed: '+name);
    }
    host = { manifest_sha256:sha(hostManifest), application_source_sha256:release.inputs.sourceSHA256,
      commit:release.inputs.commit, verified_host_binaries:release.binaries, matched_static_files:Object.keys(web).length };
  }

  const record = {
    version: 1,
    commit: manifest.commit,
    working_tree_modified: manifest.working_tree_modified,
    schema: verified.schema,
    goos, goarch,
    source_tree_sha256: verified.source_tree_sha256,
    application_source_sha256: manifest.application_source_sha256,
    role_sql_sha256: roleHashes,
    offline_native_capacity:{slots:capacity.slots.length,split_application:capacity.totals.split_application,
      renderer_sha256:sha(join(bundle.directory,'scripts/architecture-capacities.mjs')),display_profile:{
        split_clickhouse:selectedCapacity.totals.split_application.ch_connections,split:selectedCapacity.selected_process_profiles.split,
        schema58:selectedCapacity.selected_process_profiles.schema58,operating_profile:operatingProfile}},
    host_release: host,
    source_digest_version: manifest.source_digest_version,
    manifest_sha256: bundle.expected,
    file_count: verified.files,
    verified: true,
    verified_at: new Date().toISOString(),
    release: relative(out, manifestPath),
    role_entrypoints: Object.keys(manifest.entrypoints).length,
    bin_count: readdirSync(join(bundle.directory, 'bin')).length,
    declared_resource_count: declared.length,
    retained_source_commit: prior.commit,
    retained_source_archive_sha256: prior.archive,
    retained_parser_artifacts: oldHashes,
    retained_methods: oldMethods,
    parser_targets: manifest.parser_artifacts,
  };
  const evidence = join(out, 'deploy/offline-release.json');
  const pending = evidence + '.' + process.pid;
  mkdirSync(dirname(evidence), { recursive: true, mode: 0o700 });
  try {
    writeFileSync(pending, JSON.stringify(record, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
    renameSync(pending, evidence);
  } finally {
    rmSync(pending, { force: true });
  }
  console.log(evidence);
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
