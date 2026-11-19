// Exercise pg_dump's cluster-role and function-ACL boundaries on an empty PG17.
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  architecturePostgresImage,
  digest,
  read,
  runDirectory,
  sleep,
  write,
} from './architecture-lab.mjs';
import { provision } from './architecture-postgres.mjs';

const dir = runDirectory(process.argv[2]);
const selected = read(join(dir, 'candidate.evaluated.json'));
const release = read(join(selected.directory, 'candidate.json'));
const cli = join(release.bin, 'infrapro');
const archive = readFileSync(join(dir, 'snapshot/seed.pgdump'));
const seed = read(join(dir, 'snapshot/manifest.json')).pg.seed;
assert.equal(digest(archive), seed);
assert.equal(digest(readFileSync(cli)), release.binaries.infrapro);
const password = randomBytes(24).toString('hex');
const tag = randomBytes(8).toString('hex');
const cluster = 'arc-cold-grants-' + tag;
const fn = 'architecture_fixed_baseline_metadata(uuid,uuid,text,text)';
const invitations = 'architecture_notification_invitation_targets(uuid,uuid,uuid[])';
const clean = Object.fromEntries(
  Object.entries(process.env).filter(([k]) => !/^(INFRAPRO_|PG)/.test(k)),
);
const report = {
  baseline: read(join(dir, 'baseline.json')).commit,
  candidate: release.inputs.sourceSHA256,
  cliSHA256: release.binaries.infrapro,
  seedSHA256: seed,
  image: architecturePostgresImage,
  startedAt: new Date().toISOString(),
  stages: {},
  complete: false,
};
function docker(args, input, encoding = 'utf8') {
  return execFileSync('docker', args, {
    env: { ...clean, POSTGRES_PASSWORD: password },
    input,
    encoding,
    maxBuffer: 32 << 20,
    stdio: 'pipe',
    timeout: 120_000,
  });
}
function sql(db, statement) {
  return docker(
    [
      'exec',
      '-i',
      cluster,
      'psql',
      '-XqAt',
      '-v',
      'ON_ERROR_STOP=1',
      '-v',
      'VERBOSITY=sqlstate',
      '-U',
      'arc',
      '-d',
      db,
    ],
    statement,
  ).trim();
}
function restore(db, dump) {
  sql('postgres', 'CREATE DATABASE ' + db);
  docker(
    [
      'exec',
      '-i',
      cluster,
      'pg_restore',
      '--no-owner',
      '--no-acl',
      '--single-transaction',
      '--exit-on-error',
      '-U',
      'arc',
      '-d',
      db,
    ],
    dump,
  );
}
const version = (db) =>
  Number(sql(db, 'SELECT max(version_id) FROM goose_db_version WHERE is_applied'));
const canExecute = (db, role, signature = fn) =>
  sql(db, `SELECT has_function_privilege('${role}','${signature}','EXECUTE')`) === 't';
const identity = (db) =>
  sql(
    db,
    "SELECT encode(sha256(convert_to(string_agg(id::text,',' ORDER BY id),'UTF8')),'hex') FROM accounts",
  );
const eventEpoch = (db) => sql(db, 'SELECT restore_epoch::text FROM event_protocol WHERE id=1');
function verifyRoles(db) {
  const permissions = Object.fromEntries(
    [
      'arc_acl_reader',
      'infrapro_audit_worker',
      'infrapro_display_runtime',
      'infrapro_display_assets',
    ].map((name) => [name, canExecute(db, name)]),
  );
  assert.deepEqual(permissions, {
    arc_acl_reader: false,
    infrapro_audit_worker: true,
    infrapro_display_runtime: true,
    infrapro_display_assets: false,
  });
  const zero = '00000000-0000-0000-0000-000000000000';
  for (const [role, allowed] of Object.entries(permissions)) {
    const call = () =>
      sql(
        db,
        `SET ROLE ${role}; SELECT count(*) FROM architecture_fixed_baseline_metadata('${zero}','${zero}','missing',NULL)`,
      );
    if (allowed) assert.equal(call(), '0');
    else assert.throws(call, (error) => /42501/.test(error.stderr?.toString()));
  }
  return permissions;
}
function verifyInvitationReader(db) {
  const allowed = {};
  for (const name of ['arc_acl_reader','infrapro_control_api','infrapro_query_api','infrapro_notification_worker']) {
    allowed[name] = canExecute(db, name, invitations);
    const call = () => sql(db, `SET ROLE ${name}; SELECT count(*) FROM architecture_notification_invitation_targets(NULL,NULL,'{}')`);
    if (name === 'infrapro_control_api') assert.equal(call(), '0');
    else assert.throws(call, (error) => /42501/.test(error.stderr?.toString()));
    assert.equal(allowed[name], name === 'infrapro_control_api');
  }
  return allowed;
}
let container;
let phase = 'start empty PostgreSQL';
try {
  container = docker([
    'run',
    '-d',
    '--rm',
    '--name',
    cluster,
    '--label',
    'infrapro.architecture.acl=' + tag,
    '--tmpfs',
    '/var/lib/postgresql/data:rw,size=1g',
    '-e',
    'POSTGRES_USER=arc',
    '-e',
    'POSTGRES_PASSWORD',
    '-p',
    '127.0.0.1::5432',
    architecturePostgresImage,
    'postgres',
    '-c',
    'log_error_verbosity=verbose',
  ]).trim();
  const limit = Date.now() + 30_000;
  for (;;) {
    try {
      docker(['exec', cluster, 'pg_isready', '-h', '127.0.0.1', '-U', 'arc']);
      break;
    } catch (error) {
      if (Date.now() >= limit) throw error;
      await sleep(150);
    }
  }
  const port = docker(['port', cluster, '5432/tcp'])
    .trim()
    .match(/^127\.0\.0\.1:(\d+)$/)?.[1];
  assert(port);
  const url = (db) =>
    'postgres://arc:' + password + '@127.0.0.1:' + port + '/' + db + '?sslmode=disable';
  const migrate = (db) =>
    execFileSync(cli, ['database', 'migrate'], {
      env: { ...clean, INFRAPRO_DATABASE_URL: url(db) },
      stdio: 'pipe',
      timeout: 60_000,
    });
  const old = 'arc_upgrade';
  phase = 'restore frozen schema 58';
  restore(old, archive);
  sql('postgres', 'CREATE ROLE arc_acl_reader NOLOGIN');
  assert.equal(sql(old, "SELECT to_regrole('infrapro_audit_worker') IS NULL"), 't');
  assert.equal(sql(old, "SELECT to_regrole('infrapro_control_api') IS NULL"), 't');
  assert.equal(version(old), 58);
  const accounts = identity(old);
  assert.match(accounts, /^[a-f0-9]{64}$/);
  report.stages.emptyCluster = {
    schema: 58,
    auditGroupInitiallyPresent: false,
    controlGroupInitiallyPresent: false,
    accountDigest: accounts,
  };
  phase = 'upgrade without a cluster audit role';
  migrate(old);
  assert.equal(version(old), 59);
  assert.equal(identity(old), accounts);
  assert.equal(canExecute(old, 'arc_acl_reader'), false);
  assert.equal(canExecute(old, 'arc_acl_reader', invitations), false);
  const epoch = eventEpoch(old);
  phase = 'issue migrated role grants';
  report.appliedRoleSQL = provision({ url: url(old), container: cluster });
  report.stages.upgrade = {
    schema: 59,
    accountDigest: accounts,
    epoch,
    permissions: verifyRoles(old),
    personalInvitationReader: verifyInvitationReader(old),
  };
  const upgraded = docker(
    ['exec', cluster, 'pg_dump', '-Fc', '--no-owner', '--no-acl', '-U', 'arc', old],
    undefined,
    'buffer',
  );
  report.schema59DumpSHA256 = digest(upgraded);
  for (const path of ['migrate', 'provision']) {
    phase = 'restore schema 59 and ' + path;
    const db = 'arc_check_' + path;
    restore(db, upgraded);
    assert.equal(version(db), 59);
    assert.equal(canExecute(db, 'arc_acl_reader', invitations), true);
    assert.equal(
      canExecute(db, 'arc_acl_reader'),
      true,
      'The dump must actually reproduce the default PUBLIC EXECUTE',
    );
    if (path === 'migrate') {
      migrate(db);
      assert.equal(canExecute(db, 'arc_acl_reader'), false);
      assert.equal(canExecute(db, 'infrapro_audit_worker'), true);
      assert.equal(canExecute(db, 'arc_acl_reader', invitations), false);
      assert.equal(canExecute(db, 'infrapro_control_api', invitations), true);
    }
    const applied = provision({ url: url(db), container: cluster });
    assert.deepEqual(applied, report.appliedRoleSQL);
    assert.equal(identity(db), accounts);
    assert.equal(eventEpoch(db), epoch);
    report.stages[path] = {
      schema: version(db),
      defaultExecuteBefore: true,
      personalInvitationDefaultExecuteBefore: true,
      accountDigest: accounts,
      epoch,
      permissions: verifyRoles(db),
      personalInvitationReader: verifyInvitationReader(db),
    };
  }
  report.complete = true;
} catch (error) {
  // Docker arguments carrying the database URL and unfiltered tool output are private.
  report.failedPhase = phase;
  report.failure =
    error.code === 'ERR_ASSERTION' ? error.message.slice(0, 250) : 'Cold restore command failed';
  if (container) {
    const logs = spawnSync('docker', ['logs', container], { encoding: 'utf8', maxBuffer: 8 << 20 });
    const message = (logs.stderr ?? '') + (logs.stdout ?? '');
    report.sqlStates = [
      ...new Set([...message.matchAll(/ERROR:\s+([0-9A-Z]{5}):/g)].map((m) => m[1])),
    ];
    report.missingAuditGroup = message.includes('role "infrapro_audit_worker" does not exist');
  }
  process.exitCode = 1;
} finally {
  if (container) {
    const label = docker([
      'inspect',
      '--format',
      '{{index .Config.Labels "infrapro.architecture.acl"}}',
      container,
    ]).trim();
    assert.equal(label, tag);
    docker(['stop', '-t', '5', container]);
  }
  report.endedAt = new Date().toISOString();
  write(join(dir, 'deploy/cold-grant-attempts', tag + '.json'), report);
  if (report.complete) write(join(dir, 'deploy/cold-restore-grants.json'), report);
  console.log(
    'Cold PostgreSQL restore ' + (report.complete ? 'passed' : 'failed') + ' (' + tag + ')',
  );
}
