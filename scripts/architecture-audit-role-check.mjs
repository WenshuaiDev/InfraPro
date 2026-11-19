import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import {
  digest,
  read,
  root,
  runDirectory,
  safeDiagnosticLine,
  sleep,
  write,
} from './architecture-lab.mjs';
import { auditRoleInputs } from './architecture-audit-role-setup.mjs';
import { pairedRoleConfigs } from './architecture-http-sandbox.mjs';
import { regressionRelease, verifyRegressionSource } from './architecture-regression.mjs';
import { availablePort } from './fixtures/parsing-lab.mjs';

const run = runDirectory(process.argv[2]),
  startedAt = new Date().toISOString();
const out = join(run, 'regression', 'audit-role-assembly', startedAt.replaceAll(':', '-'));
const privateDir = join(out, 'private');
mkdirSync(privateDir, { recursive: true, mode: 0o700 });
const pg = 'arc-audit-grants-' + randomUUID().slice(0, 12),
  admin = 'infrapro_test';
const password = randomBytes(24).toString('hex'),
  port = await availablePort();
const docker = (args, options = {}) =>
  execFileSync('docker', args, { encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'], ...options });
const ownerURL = (db) => 'postgres://' + admin + ':' + password + '@127.0.0.1:' + port + '/' + db;
const dbname = (side) => 'audit_' + side + '_test';
let host;
function psql(db, sql, login = admin, pass = password, denied = false) {
  const cmd = [
    'exec',
    '-i',
    '-e',
    'PGPASSWORD',
    pg,
    'psql',
    '-X',
    '-qAt',
    '-h',
    host,
    '-U',
    login,
    '-d',
    db,
    '-v',
    'ON_ERROR_STOP=1',
  ];
  const res = spawnSync('docker', cmd, {
    input: '\\set VERBOSITY verbose\n' + sql + '\n',
    env: { ...process.env, PGPASSWORD: pass },
    encoding: 'utf8',
    maxBuffer: 12 << 20,
  });
  if (denied) {
    assert.notEqual(res.status, 0, 'A forbidden SQL operation succeeded');
    assert.match(res.stderr, /\b42501\b/, 'Expected insufficient_privilege');
    return '42501';
  }
  if (res.status !== 0)
    throw Error(
      'Isolated SQL failed: ' +
        (res.stderr || '')
          .split('\n')
          .find((x) => x.includes('ERROR:'))
          ?.replaceAll(pass, '[private]'),
    );
  return res.stdout.trim();
}
function migrate(side) {
  const release = regressionRelease(run, side);
  verifyRegressionSource(release);
  const cfg = join(privateDir, side + '.json');
  write(cfg, { database_url: ownerURL(dbname(side)) });
  execFileSync(join(release.bin, 'infrapro'), ['database', 'migrate'], {
    env: {
      ...Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_'))),
      INFRAPRO_CONFIG: cfg,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  return release;
}
const role = 'infrapro_audit_worker',
  checks = { startedAt, container: pg, sides: {} };
const seed = join(run, 'snapshot', 'seed.pgdump'),
  snapshot = read(join(run, 'snapshot/manifest.json'));
checks.seedSHA256 = digest(readFileSync(seed));
assert.equal(checks.seedSHA256, snapshot.pg.seed);
let created = false;
try {
  docker(
    [
      'run',
      '-d',
      '--name',
      pg,
      '--network',
      'bridge',
      '--label',
      'infrapro.architecture.fixture=' + pg,
      '-p',
      '127.0.0.1:' + port + ':5432',
      '--tmpfs',
      '/var/lib/postgresql/data',
      '-e',
      'POSTGRES_USER',
      '-e',
      'POSTGRES_PASSWORD',
      '-e',
      'POSTGRES_DB=postgres',
      'postgres:17-alpine',
      'postgres',
      '-c',
      'max_locks_per_transaction=512',
    ],
    {
      env: { ...process.env, POSTGRES_USER: admin, POSTGRES_PASSWORD: password },
    },
  );
  created = true;
  host = docker([
    'inspect',
    '--format',
    '{{(index .NetworkSettings.Networks "bridge").IPAddress}}',
    pg,
  ]).trim();
  for (let i = 0; i < 70; i++) {
    try {
      if (psql('postgres', 'SELECT 1') === '1') break;
    } catch (error) {
      if (i === 69) throw error;
    }
    await sleep(200);
  }
  psql('postgres', 'CREATE DATABASE ' + dbname('baseline'));
  const frozen = migrate('baseline'),
    a = dbname('baseline');
  // Retain Goose's original grants while loading real S0 data into this new,
  // dedicated database. Leave the fresh migration ledger and its sequence intact.
  psql(
    a,
    "DO $$ DECLARE tables text; BEGIN SELECT string_agg(format('%I.%I',schemaname,tablename),',') INTO tables FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('goose_db_version'); IF tables IS NULL THEN RAISE EXCEPTION 'empty fixture schema'; END IF; EXECUTE 'TRUNCATE TABLE '||tables||' CASCADE'; END $$;",
  );
  docker(['cp', seed, pg + ':/tmp/seed.pgdump']);
  const toc = docker(['exec', pg, 'pg_restore', '-l', '/tmp/seed.pgdump']);
  const entries = toc
    .split('\n')
    .filter((l) => !/\b(?:TABLE DATA|SEQUENCE SET) public goose_db_version(?:_id_seq)?\b/.test(l))
    .join('\n');
  assert(entries.length < toc.length);
  docker(['exec', '-i', pg, 'sh', '-c', 'cat > /tmp/content.list'], { input: entries + '\n' });
  docker(
    [
      'exec',
      '-e',
      'PGPASSWORD',
      pg,
      'pg_restore',
      '-h',
      host,
      '-U',
      admin,
      '-d',
      a,
      '--single-transaction',
      '--data-only',
      '--disable-triggers',
      '--no-owner',
      '--no-acl',
      '-L',
      '/tmp/content.list',
      '/tmp/seed.pgdump',
    ],
    { env: { ...process.env, PGPASSWORD: password } },
  );
  psql('postgres', 'CREATE DATABASE ' + dbname('candidate') + ' TEMPLATE ' + a);
  const candidate = migrate('candidate');
  const row = JSON.parse(
    psql(
      a,
      "SELECT json_build_object('tenant',tenant_id,'facility',facility_id,'kind',kind,'id',id,'hash',md5(data::text)) FROM engineering_resource_records WHERE kind='facility' ORDER BY id LIMIT 1",
    ),
  );
  for (const key of ['tenant', 'facility', 'id']) assert.match(row[key], /^[a-f0-9-]{36}$/);
  const readEntity =
    "SELECT md5(data::text) FROM engineering_resource_records WHERE tenant_id='" +
    row.tenant +
    "' AND facility_id='" +
    row.facility +
    "' AND kind='facility' AND id='" +
    row.id +
    "'";
  for (const side of ['baseline', 'candidate']) {
    const db = dbname(side),
      rel = side === 'baseline' ? frozen : candidate,
      cwd = side === 'baseline' ? read(join(run, 'baseline.json')).source : root;
    const inputs = auditRoleInputs(run, side, cwd),
      files = {};
    const ownerHash = psql(db, readEntity);
    assert.equal(ownerHash, row.hash);
    const bareIdentity = psql(db, 'SET ROLE ' + role + '; SELECT session_user,current_user;');
    assert.equal(bareIdentity, admin + '|' + role);
    const before = psql(db, 'SET ROLE ' + role + '; ' + readEntity, admin, password, true);
    let login, secret;
    if (side === 'baseline') {
      for (const [name, sha] of Object.entries(inputs.files)) {
        const body = readFileSync(join(inputs.directory, name));
        assert.equal(digest(body), sha);
        psql(db, body.toString());
        files[name] = sha;
      }
      login = 'audit_s0_' + randomBytes(5).toString('hex');
      secret = randomBytes(25).toString('hex');
      psql(
        db,
        'CREATE ROLE ' +
          login +
          " LOGIN INHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '" +
          secret +
          "'; GRANT " +
          role +
          ' TO ' +
          login,
      );
    } else {
      const provisioned = pairedRoleConfigs(
        join(privateDir, 'b9'),
        { database_url: ownerURL(db) },
        pg,
        ['audit-worker'],
        inputs.directory,
      );
      Object.assign(files, provisioned.appliedRoleSQL);
      assert.deepEqual(files, inputs.files);
      const runtime = read(provisioned.roleEnv('audit-worker').INFRAPRO_CONFIG),
        url = new URL(runtime.database_url);
      login = decodeURIComponent(url.username);
      secret = decodeURIComponent(url.password);
    }
    const identity = psql(
      db,
      "SELECT session_user,current_user,pg_has_role(current_user,'" +
        role +
        "','MEMBER'),rolsuper,rolbypassrls,rolcreaterole FROM pg_roles WHERE rolname=current_user",
      login,
      secret,
    );
    assert.equal(identity, login + '|' + login + '|t|f|f|f');
    const actual = psql(db, readEntity, login, secret);
    assert.equal(actual, ownerHash);
    const negative = {
      passwordRead: psql(db, 'SELECT password_hash FROM account_passwords LIMIT 0', login, secret, true),
      accountWrite: psql(db, 'UPDATE accounts SET status=status WHERE false', login, secret, true),
      ingestionBody: psql(db, 'SELECT envelope FROM ingestion_receipts LIMIT 0', login, secret, true),
    };
    const result = {
      releaseSHA256: inputs.releaseSHA256,
      schema: Number(psql(db, 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')),
      before: { sessionUser: admin, currentUser: role, engineeringReadSQLSTATE: before },
      publication: inputs.publication,
      ...(inputs.manifestSHA256 && { publicationSHA256: inputs.manifestSHA256 }),
      applied: files,
      after: {
        sessionUser: login,
        currentUser: login,
        membership: role,
        superuser: false,
        bypassRLS: false,
        canCreateRole: false,
        entity: {
          tenant: row.tenant,
          facility: row.facility,
          kind: row.kind,
          id: row.id,
          metadataMD5: actual,
        },
        negative,
      },
    };
    verifyRegressionSource(rel, result);
    assert(result.sourceAtCompletion.matchesSelectedRelease);
    checks.sides[side] = result;
  }
  checks.passed = true;
} catch (error) {
  checks.passed = false;
  checks.failure = safeDiagnosticLine(error.message).slice(0, 280);
  throw error;
} finally {
  if (created) docker(['rm', '-f', '-v', pg]);
  checks.finishedAt = new Date().toISOString();
  checks.remaining = docker(['ps', '-aq', '--filter', 'label=infrapro.architecture.fixture=' + pg])
    .trim()
    .split('\n')
    .filter(Boolean);
  assert.equal(checks.remaining.length, 0);
  write(join(out, 'summary.json'), checks);
  write(join(run, 'regression/audit-role-assembly.json'), checks);
}
assert(checks.passed);
console.log(relative(run, join(out, 'summary.json')), 'S0 and candidate account grants verified');
