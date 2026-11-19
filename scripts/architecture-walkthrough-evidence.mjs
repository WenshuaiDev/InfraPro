import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, constants, copyFileSync, existsSync, mkdirSync, readFileSync, realpathSync, renameSync, rmSync, statSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { client, digest, queryPG, read, sleep, write } from './architecture-lab.mjs';
import { diagnosticClient, eventually } from './fixtures/diagnostics-client.mjs';

export function archiveWalkthroughRecord(dir, name, record, environment = record.environment, release = record.releaseSHA256 ?? 'unversioned') {
  assert.match(environment, /^[a-f0-9]{12}$/);
  assert(release === 'unversioned' || /^[a-f0-9]{64}$/.test(release));
  const signature = digest(JSON.stringify(record)).slice(0, 16);
  const file = join(dir, 'manual', 'previous', environment, release, signature, name + '.json');
  if (!existsSync(file)) write(file, record);
}

// Early S0 archives include the original sources and UI but omit a binary
// checksum for the independent server. Retain the exact executable observed
// in the original diagnostics suite when that evidence is available.
export function walkthroughDiagnosticServer(parent, lab, side, release) {
  const name = 'infrapro-diagnostics', binary = join(release.bin, name);
  if (release.binaries[name]) return { binary, sha256: release.binaries[name], source: 'release manifest' };
  assert.equal(side, 'a');
  const evidence = join(parent, 'diagnostics-baseline/summary.json');
  if (!existsSync(evidence)) return { binary, sha256: digest(readFileSync(binary)), source: 'frozen source and UI build' };
  const summary = read(evidence), sha256 = summary.fixture.native_server_sha256;
  assert.equal(summary.aggregate_status, 'passed');
  assert.equal(summary.fixture.source, release.source_archive_sha256);
  assert.match(sha256, /^[a-f0-9]{64}$/);
  const privateBin = join(lab.directory, 'private', side, 'programs', sha256, name);
  if (!existsSync(privateBin) || digest(readFileSync(privateBin)) !== sha256) {
    const original = join(release.source, 'build/bin', name);
    assert.equal(digest(readFileSync(original)), sha256, 'S0 original diagnostic executable changed');
    mkdirSync(dirname(privateBin), { recursive: true, mode: 0o700 });
    const temp = privateBin + '.copy-' + process.pid;
    try {
      copyFileSync(original, temp, constants.COPYFILE_FICLONE);
      chmodSync(temp, 0o700);
      assert.equal(digest(readFileSync(temp)), sha256);
      renameSync(temp, privateBin);
    } finally {
      rmSync(temp, { force: true });
    }
  }
  return { binary: privateBin, sha256, source: 'original diagnostics suite', suiteRunID: summary.run_id, suiteFinishedAt: summary.finished_at };
}

export async function observeWalkthrough(stack, sharedConsole) {
  const { lab, side, bin, children, manual } = stack;
  const frozen = read(join(lab.directory, 'baseline.json'));
  const current = side === 'b' ? read(join(dirname(bin), 'candidate.json')) : frozen;
  assert.equal(stack.releaseSHA256, current.inputs?.sourceSHA256 ?? current.source_archive_sha256);
  assert.equal(stack.sharedConsoleSHA256, sharedConsole?.console.sha256);
  const snapshotPath = join(lab.directory, 'snapshot/manifest.json');
  const snapshot = read(snapshotPath);
  assert.equal(snapshot.baseline, frozen.commit);
  assert(snapshot.restored.includes(side));
  const schema = JSON.parse(queryPG(lab, side, `SELECT json_build_object(
    'database', current_database(), 'version', MAX(version_id), 'publication', to_regprocedure('event_publication_permit(boolean)') IS NOT NULL,
    'tenantIndex', to_regclass('event_outbox_tenant_pressure') IS NOT NULL) FROM goose_db_version WHERE is_applied`));
  assert.equal(schema.version, side === 'b' ? current.schema.version : frozen.schema);
  assert.equal(schema.database, 'arc_walk_' + lab.id + '_' + side);
  assert.equal(schema.publication, side === 'b');
  assert.equal(schema.tenantIndex, side === 'b');
  if (side === 'b') {
    assert.match(current.schema.migrationSHA256, /^[a-f0-9]{64}$/);
    assert.equal(basename(lab.directory), 'b-' + current.schema.migrationSHA256.slice(0, 16));
    schema.migrationSHA256 = current.schema.migrationSHA256;
  }
  const processes = {};
  const helper = join(bin, 'diagnostics-labtool');
  const identify = (name, pid, executable, expectedSHA) => {
    const observed = execFileSync(helper, ['identity', String(pid)], { encoding: 'utf8', timeout: 5000 });
    const os = JSON.parse(observed);
    assert.equal(os.pid, pid);
    assert.equal(realpathSync(os.executable), realpathSync(executable));
    const sha256 = digest(readFileSync(os.executable));
    if (expectedSHA) assert.equal(sha256, expectedSHA, name);
    processes[name] = { pid, started: os.os_start_time_ms, sha256 };
  };
  for (const [name, child] of Object.entries(children)) {
    assert.equal(child.exitCode, null, name + ' exited');
    assert.equal(child.signalCode, null, name + ' stopped');
    if (name === 'web') {
      assert(child.spawnargs.includes(sharedConsole?.console.directory || join(lab[side].release || frozen.assets, 'console')));
      identify(name, child.pid, process.execPath);
    } else if (name === 'diagnostics') {
      const selected = stack.diagnosticServer;
      assert.equal(realpathSync(child.spawnfile), realpathSync(selected.binary));
      identify(name, child.pid, selected.binary, selected.sha256);
      Object.assign(processes[name], { source: selected.source, ...(selected.suiteRunID && { suiteRunID: selected.suiteRunID, suiteFinishedAt: selected.suiteFinishedAt }) });
    } else {
      assert.equal(realpathSync(child.spawnfile), realpathSync(join(bin, name)));
      assert.match(current.binaries[name], /^[a-f0-9]{64}$/);
      identify(name, child.pid, child.spawnfile, current.binaries[name]);
    }
  }
  for (const name of ['parser', 'ingest-relay']) {
    const pid = name === 'parser' ? manual.parserPID : manual.relay.pid;
    identify(name, pid, join(bin, name), current.binaries[name]);
    const file = join(lab.directory, 'private', side, 'reports', name, 'process.json');
    let report;
    const until = Date.now() + 20000;
    while (Date.now() < until) {
      if (existsSync(file)) report = read(file);
      if (report?.process.pid === pid && String(processes[name].started) === report.process.os_start_time_ms) {
        break;
      }
      await sleep(150);
    }
    assert.equal(report?.process.pid, pid);
    assert.equal(report.replica_id, name + '-1');
    assert.equal(String(processes[name].started), report.process.os_start_time_ms);
  }
  if (side === 'b') {
    const broker = manual.brokerProbe;
    identify('nats-server', Number(readFileSync(broker.pidFile, 'utf8').trim()), broker.binary);
  }
  const servedAssets = {};
  const origins = {
    console: stack.cfg.origin,
    'display-runtime': stack.cfg.runtime,
    diagnostics: 'http://127.0.0.1:' + lab[side].ports.diagnostics,
  };
  for (const [app, origin] of Object.entries(origins)) {
    const fetchAsset = async (path, key) => {
      const response = await fetch(origin + path, { redirect: 'error', signal: AbortSignal.timeout(10000) });
      assert.equal(response.status, 200, app + path);
      const bytes = Buffer.from(await response.arrayBuffer());
      const actual = digest(bytes);
      const expected = app === 'console' && sharedConsole ? sharedConsole.console.files[key.slice('console/'.length)] : current.staticFiles[key];
      assert.equal(actual, expected, 'Rendered release bytes for ' + key);
      servedAssets[key] = actual;
      return bytes;
    };
    const html = await fetchAsset('/', app + '/index.html');
    const module = /<script\b[^>]*\bsrc="(\/assets\/[A-Za-z0-9_.-]+\.js)"/.exec(html.toString('utf8'));
    assert(module, 'Missing release entry module for ' + app);
    await fetchAsset(module[1], app + module[1]);
  }
  const account = join(lab.directory, 'private', side, 'diagnostics/accounts.json');
  assert.equal(statSync(account).mode & 0o777, 0o600);
  const diag = await diagnosticClient(
    { origin: 'http://127.0.0.1:' + lab[side].ports.diagnostics, accounts_file: account },
    { loginTimeoutMs: 75000 },
  );
  try {
    const overview = (await diag.get('/overview')).data;
    assert.equal(overview.instance.id, 'arc-' + lab.id);
    const self = (await diag.get('/self')).data;
    assert(self.server_boot_id && self.build_id);
    const presence = await eventually(async () => {
      const { current_observation: observed } = (await diag.get('/checks/diagnostics-process')).data;
      const running = processes.diagnostics;
      return observed.freshness === 'fresh' && observed.outcome === 'ok' &&
        observed.process?.pid === running.pid && observed.process.os_start_time_ms === String(running.started) && observed;
    }, 30000);
    processes.diagnostics.observedBootID = presence.process.boot_id;
    processes.diagnostics.observedAt = presence.observed_at;
    return {
      launchID: manual.launchID,
      ...(sharedConsole && { sharedConsoleSHA256: sharedConsole.console.sha256 }),
      schema,
      ...(manual.appliedRoleSQL && { appliedRoleSQL: manual.appliedRoleSQL }),
      snapshotSHA256: digest(readFileSync(snapshotPath)),
      processes,
      servedAssets,
      diagnosticInstance: overview.instance.id,
      serverBootID: self.server_boot_id,
      diagnosticBuild: self.build_id,
    };
  } finally {
    await diag.logout();
  }
}

export async function verifyWalkthroughHistory(cfg, accounts, samples, governance) {
  const { tenant } = governance.scope;
  const admin = await client(cfg, accounts.admin);
  const history = [];
  try {
    const project = '/tenants/' + tenant + '/projects/' + governance.scope.project;
    const facility = await admin.call(project + '/engineering/resources/facilities/' + governance.sample.facility);
    assert.equal(facility.id, governance.sample.facility);
    assert.equal(facility.tenant_id, tenant);
    const artifacts = '/tenants/' + tenant + '/monitoring/derived/analysis/artifacts/' + governance.artifact;
    const versions = [governance.version1, governance.version2];
    assert.equal(new Set(versions).size, 2);
    for (const [i, version] of versions.entries()) {
      const fact = await admin.call(artifacts + '?version_id=' + version);
      assert.equal(fact.version.version_no, String(i + 1));
      assert.equal(fact.version.output_rows, '10');
      const values = await admin.call(artifacts + '/query', { version_id: version, limit: 20 });
      const numbers = values.items.map(row => Number(row.value));
      assert.deepEqual(numbers, [0, 2, 4, 6, 8, 10, 12, 14, 16, 18]);
      history.push({ version, rows: numbers.length, valuesSHA256: digest(JSON.stringify(numbers)) });
    }
    const notice = await admin.call('/me/notifications/' + governance.notification + '/target');
    assert.equal(notice.status, 'available');
    assert.equal(notice.kind, 'task');
    assert.equal(notice.id, governance.noticeTask);
  } finally {
    await admin.client.dispose();
  }
  const other = await client(cfg, accounts.other);
  try {
    const project = await other.call('/tenants/' + samples.secondTenant + '/projects/' + samples.secondProject + '/context');
    assert(project.can_enter);
    assert.equal(project.authorization.account_id, samples.actors.other);
  } finally {
    await other.client.dispose();
  }
  return history;
}
