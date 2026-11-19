// Isolated synthetic A/B laboratory. No service reads a live business database.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import {
  closeSync,
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readdirSync,
  realpathSync,
  renameSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { ensureObjectCapacity } from './architecture-object-capacity.mjs';
import { createMFAKeyring } from './mfa-keyring.mjs';
import { seedSensorStructure } from './fixtures/sensor-structure-acceptance.mjs';
import { availablePort, stop, waitURL } from './fixtures/parsing-lab.mjs';

export const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const artifactRoot = join(root, '.artifacts/architecture-refactor');
const developmentBaseline = '220aee11790d900af823b05b70796da1a4733c79';
export function read(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
}
export function write(path, value) {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = path + '.' + randomUUID();
  writeFileSync(temporary, JSON.stringify(value, null, 2) + '\n', {
    mode: 0o600,
  });
  renameSync(temporary, path);
}
// Kernel ownership is released automatically if the parent or its pipe exits.
export async function fileLease(lock, { wait = true } = {}) {
  mkdirSync(dirname(lock), { recursive: true, mode: 0o700 });
  const program = [
    'import fcntl, os, sys',
    'with os.fdopen(os.open(sys.argv[1], os.O_CREAT | os.O_RDWR, 0o600), "r+") as lock:',
    ' try: fcntl.flock(lock, fcntl.LOCK_EX | (0 if sys.argv[2] == "wait" else fcntl.LOCK_NB))',
    ' except BlockingIOError: sys.exit(2)',
    ' print("ready", flush=True)',
    ' sys.stdin.read()',
  ].join('\n');
  const holder = spawn('python3', ['-c', program, lock, wait ? 'wait' : 'try'], {
    stdio: ['pipe', 'pipe', 'inherit'],
  });
  const ended = once(holder, 'exit');
  await Promise.race([
    once(holder.stdout, 'data'),
    ended.then(([code]) => {
      throw Error(
        code === 2 ? 'Environment already running: ' + lock : 'Environment lock unavailable',
      );
    }),
  ]);
  return async () => {
    holder.stdin.end();
    await ended;
  };
}

export function runDirectory(path) {
  mkdirSync(artifactRoot, { recursive: true, mode: 0o700 });
  if (!path) return mkdtempSync(join(artifactRoot, 'run-'));
  const dir = resolve(path);
  assert.equal(
    dirname(dir),
    artifactRoot,
    'architecture runs belong directly under .artifacts/architecture-refactor',
  );
  assert.match(dir.slice(artifactRoot.length + 1), /^run-[a-z0-9._-]+$/i);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  assert.equal(realpathSync(dir), dir);
  return dir;
}
export const digest = (data) => createHash('sha256').update(data).digest('hex');
export function safeDiagnosticLine(value) {
  return String(value)
    .split(/\r?\n/, 1)[0]
    .replace(
      /\b(?:set-cookie|cookie|authorization|proxy-authorization|x-csrf-token)\s*:\s*.*$/i,
      '[redacted header]',
    )
    .replace(/\bBearer\s+\S+/gi, '[redacted credential]')
    .replace(/(?:https?|postgres(?:ql)?|redis):\/\/[^\s)]+/gi, '[isolated endpoint]')
    .replace(
      /(["']?(?:password|token|secret|credential|key)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)/gi,
      '$1[redacted]',
    );
}
const binaryFile = (path, args, options = {}) =>
  execFileSync(path, args, { cwd: root, maxBuffer: 32 << 20, ...options });
const bufferText = (cmd, args, options) => binaryFile(cmd, args, options).toString().trim();
const cleanEnv = () =>
  Object.fromEntries(
    Object.entries(process.env).filter(
      ([k]) => !k.startsWith('INFRAPRO_') && !k.startsWith('VITE_') && !k.startsWith('PG'),
    ),
  );
const image =
  'postgres:17-alpine@sha256:3540c0362c809df084ffac0ae69fe794515f9de92692f02178bcdce0ef0302ac';
export { image as architecturePostgresImage };
const pgUser = 'archtest';
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function eventually(fn, valid, seconds = 120) {
  const until = Date.now() + seconds * 1000;
  for (;;) {
    const v = await fn();
    if (valid(v)) return v;
    if (Date.now() > until) throw Error('Timed out waiting for a fixture state');
    await sleep(300);
  }
}

export function freeze(dir, supplied) {
  let frozen = supplied || process.env.INFRAPRO_ARCHITECTURE_BASELINE;
  if (!frozen)
    frozen = readdirSync(artifactRoot)
      .filter((v) => v.startsWith('freeze-') && existsSync(join(artifactRoot, v, 'baseline.json')))
      .map((v) => join(artifactRoot, v))
      .find((v) => read(join(v, 'baseline.json')).commit === developmentBaseline);
  if (!frozen) {
    frozen = join(dir, 'baseline-input');
    mkdirSync(frozen, { recursive: true, mode: 0o700 });
    const archive = join(frozen, 'frozen-source.tar');
    binaryFile('git', ['archive', '--format=tar', '--output=' + archive, developmentBaseline]);
    write(join(frozen, 'baseline.json'), {
      commit: developmentBaseline,
      source_archive_sha256: digest(readFileSync(archive)),
    });
  }
  const manifest = read(join(frozen, 'baseline.json'));
  assert.equal(manifest.commit, developmentBaseline);
  const archive = join(frozen, 'frozen-source.tar');
  assert.equal(digest(readFileSync(archive)), manifest.source_archive_sha256);
  const source = join(dir, 'baseline-source');
  if (!existsSync(join(source, 'go.work'))) {
    mkdirSync(source, { recursive: true, mode: 0o700 });
    binaryFile('tar', ['-xf', archive, '-C', source]);
  }
  // The archive is the authority for build bytes. Keep the original checkout
  // fingerprints separately, since Git can store a text file with LF even
  // when the captured working copy uses CRLF.
  const sourceFiles = JSON.parse(
    binaryFile('python3', [
      '-c',
      'import hashlib,json,sys,tarfile\nwith tarfile.open(sys.argv[1]) as source:\n print(json.dumps({m.name:hashlib.sha256(source.extractfile(m).read()).hexdigest() for m in source if m.isfile()}))',
      archive,
    ]),
  );
  for (const [path, hash] of Object.entries(sourceFiles)) {
    assert.equal(
      digest(readFileSync(join(source, path))),
      hash,
      'Frozen build input changed: ' + path,
    );
  }
  const migration = readFileSync(
    join(source, 'packages/go/platform/migrations/migrations.go'),
    'utf8',
  );
  const schema = Number(/^const Version int64 = (\d+)$/m.exec(migration)?.[1]);
  assert.equal(schema, 58, 'Unexpected frozen database version');
  const bin = join(dir, 'baseline-bin');
  mkdirSync(bin, { recursive: true, mode: 0o700 });
  const roles = [
    'api',
    'display-runtime',
    'monitoring-worker',
    'ingest-worker',
    'notification-worker',
    'audit-worker',
    'task-index-worker',
    'display-package-worker',
    'files-worker',
    'files-checker',
    'spatial-worker',
    'parser',
    'parser-runner',
    'ingest-relay',
    'infrapro',
  ];
  const missing = [];
  for (const role of roles) {
    if (existsSync(join(bin, role))) continue;
    const cached = join(frozen, 'bin', role);
    if (existsSync(cached)) cpSync(cached, join(bin, role), { mode: 1 });
    else missing.push(role);
  }
  const command = (role) =>
    role === 'infrapro'
      ? './tools/infrapro/cmd/infrapro'
      : role === 'ingest-relay'
        ? './services/ingest-relay/cmd/ingest-relay'
        : role.startsWith('parser')
          ? './services/parser/cmd/' + role
          : './services/api/cmd/' + role;
  if (missing.length)
    binaryFile('go', ['build', '-trimpath', '-o', bin + '/', ...missing.map(command)], {
      cwd: source,
    });
  let assets = join(frozen, 'release');
  if (!existsSync(assets)) {
    binaryFile('pnpm', ['install', '--frozen-lockfile'], { cwd: source });
    binaryFile('pnpm', ['build:apps'], { cwd: source });
    assets = join(frozen, 'release');
    for (const app of ['console', 'display-runtime', 'diagnostics'])
      cpSync(join(source, 'apps', app, 'dist'), join(assets, app), {
        recursive: true,
      });
  }
  const staticFiles = {};
  for (const app of ['console', 'display-runtime', 'diagnostics']) {
    for (const file of readdirSync(join(assets, app), {
      recursive: true,
      withFileTypes: true,
    })) {
      if (file.isFile()) {
        const full = join(file.parentPath, file.name);
        staticFiles[full.slice(assets.length + 1)] = digest(readFileSync(full));
      }
    }
  }
  const binaries = Object.fromEntries(
    roles.map((role) => [role, digest(readFileSync(join(bin, role)))]),
  );
  const manifestPath = join(dir, 'baseline.json');
  if (existsSync(manifestPath)) {
    const original = read(manifestPath);
    assert.equal(
      original.source_archive_sha256,
      manifest.source_archive_sha256,
      'Frozen source archive changed',
    );
    assert.deepEqual(binaries, original.binaries, 'Frozen executables changed');
    assert.deepEqual(staticFiles, original.staticFiles, 'Frozen frontend files changed');
  }
  const baseline = {
    ...manifest,
    checkoutFiles: manifest.source ?? {},
    sourceFiles,
    sourceArchive: archive,
    source,
    bin,
    assets,
    staticFiles,
    binaries,
    schema,
  };
  write(manifestPath, baseline);
  return baseline;
}
function docker(args, options) {
  return binaryFile('docker', args, options);
}
// Docker Desktop can reassign an ephemeral published port after restarting
// its VM. Resolve the labelled copy itself when opening a new fixture.
export function activeDatabasePort(lab) {
  const entries = JSON.parse(
    bufferText('docker', ['inspect', '--format', '{{json .NetworkSettings.Ports}}', lab.container]),
  )['5432/tcp'];
  assert.equal(
    bufferText('docker', [
      'inspect',
      '-f',
      '{{index .Config.Labels "infrapro.architecture"}}',
      lab.container,
    ]),
    lab.containerLabel ?? lab.id,
  );
  const port = Number(entries?.find((e) => e.HostIp === '127.0.0.1')?.HostPort);
  assert(
    Number.isInteger(port) && port > 0 && port < 65536,
    'Isolated PostgreSQL must have a published loopback port',
  );
  return port;
}
function pg(lab, db, sql) {
  assert.match(
    db,
    /^arc_(seed|a|b|relay_seed|relay_a|relay_b|self|(?:(?:relay_)?walk|http)_[a-f0-9]{12}_[ab])$/,
  );
  return bufferText(
    'docker',
    [
      'exec',
      '-i',
      lab.container,
      'psql',
      '-X',
      '--set',
      'ON_ERROR_STOP=1',
      '-qAt',
      '-U',
      pgUser,
      '-d',
      db,
    ],
    { input: sql },
  );
}
export function processEnv(lab, side, role, split = false) {
  const cfg = lab[side];
  const env = { ...cleanEnv(), INFRAPRO_CONFIG: cfg.config };
  if (split)
    env.INFRAPRO_ARCHITECTURE_FILE = join(
      lab.directory,
      'topology',
      'roles',
      role,
      'architecture.json',
    );
  return env;
}
export function launch(file, args, env, log, cwd = root, detached = false) {
  mkdirSync(dirname(log), { recursive: true, mode: 0o700 });
  const fd = openSync(log, 'a', 0o600);
  try {
    const child = spawn(file, args, {
      cwd,
      env,
      detached,
      stdio: ['ignore', fd, fd],
    });
    if (detached) child.unref();
    return child;
  } finally {
    closeSync(fd);
  }
}
function makeConfig(dir, side, ports, lab, secret) {
  const base = join(dir, 'private', side),
    monitor = join(base, 'monitor');
  mkdirSync(monitor, { recursive: true, mode: 0o700 });
  const api = '127.0.0.1:' + ports.api;
  const origin = 'http://127.0.0.1:' + ports.web;
  const runtime = 'http://127.0.0.1:' + ports.runtime;
  const assets = 'http://127.0.0.1:' + ports.assets;
  const config = join(base, 'service.json');
  const ch = 'arc_' + lab.id + '_' + side,
    bucket = 'arc-' + lab.id + '-' + side;
  const cfg = {
    database_url: 'postgres://' + pgUser + ':' + secret + '@127.0.0.1:' + ports.db + '/arc_' + side,
    redis_url: 'redis://:test-only-local-password@127.0.0.1:56379/0',
    redis_key_prefix: 'infrapro:arc-' + lab.id + '-' + side + ':identity:v1:',
    http_addr: api,
    log_level: 'warn',
    mfa_keyring_file: join(dir, 'private', 'mfa.json'),
    csrf_auth_key: lab.csrf,
    public_console_url: origin + '/',
    trusted_origins: origin,
    cookie_secure: 'false',
    support_contact: '架构隔离验收',
    ip_request_limit: '10000',
    credential_request_limit: '1000',
    invitation_rate: '1000',
    monitoring_clickhouse_replicas: '127.0.0.1:59000',
    monitoring_clickhouse_database: ch,
    monitoring_clickhouse_user: 'default',
    monitoring_clickhouse_policy: 'monitoring_tiered',
    monitoring_object_endpoint: 'http://127.0.0.1:58333',
    monitoring_object_access_key: 'contract-admin',
    monitoring_object_secret_key: 'contract-admin-test-only',
    monitoring_source_bucket: bucket,
    monitoring_temp_directory: monitor,
    display: {
      runtime_origin: runtime,
      asset_origin: assets,
      runtime_addr: new URL(runtime).host,
      asset_addr: new URL(assets).host,
      shell_directory: join(lab.assets, 'display-runtime'),
    },
    files: {
      object_endpoint: 'http://127.0.0.1:58333',
      access_key: 'contract-admin',
      secret_key: 'contract-admin-test-only',
      bucket,
      temp_directory: join(base, 'files'),
      node_id: 'arc-' + side,
    },
    audit: {
      object_endpoint: 'http://127.0.0.1:58333',
      access_key: 'contract-admin',
      secret_key: 'contract-admin-test-only',
      bucket,
      temp_directory: join(base, 'audit'),
      node_id: 'arc-' + side,
    },
  };
  for (const typ of ['audit', 'files'])
    mkdirSync(join(base, typ), { recursive: true, mode: 0o700 });
  write(config, cfg);
  return {
    origin,
    api: 'http://' + api,
    runtime,
    assets,
    config,
    ch,
    bucket,
    ports,
  };
}
async function client(side, account, { timeout = 30000 } = {}) {
  const browser = await request.newContext({ timeout }),
    rootURL = side.api + '/api/v1';
  const headers = { Origin: side.origin };
  async function call(path, data, method = data === undefined ? 'GET' : 'POST') {
    let response;
    try {
      response = await browser.fetch(rootURL + path, { method, headers: { ...headers, 'X-Command-ID': randomUUID() }, data });
    } catch (error) {
      throw new Error(method + ' ' + path.split('?')[0] + ' ' + safeDiagnosticLine(error.message), { cause: error });
    }
    try {
      if (!response.ok()) {
        const err = await response.json().catch(() => ({}));
        throw Error(method + ' ' + path.split('?')[0] + ' returned ' + response.status() + ' ' + (err.code || ''));
      }
      return response.status() === 204 ? {} : await response.json();
    } finally {
      await response.dispose();
    }
  }
  try {
    const csrf = await call('/auth/csrf');
    assert(typeof csrf.token === 'string' && csrf.token.length > 0, 'GET /auth/csrf did not issue a session token');
    headers['X-CSRF-Token'] = csrf.token;
    if (account) assert.equal((await call('/auth/login', account)).status, 'authenticated');
    return { call, client: browser, api: rootURL, headers };
  } catch (error) {
    await browser.dispose();
    throw error;
  }
}
export { client };
async function seed(lab, base) {
  const dir = lab.directory;
  const accounts = Object.fromEntries(
    ['admin', 'member', 'other'].map((w) => [
      w,
      {
        username: 'arc_' + w,
        password: 'A1!' + randomBytes(20).toString('base64url'),
      },
    ]),
  );
  write(join(dir, 'private/accounts.json'), accounts);
  const env = processEnv(lab, 'seed');
  for (const args of [
    ['database', 'migrate'],
    ['access', 'sync'],
    ['monitoring', 'storage', 'init'],
  ])
    binaryFile(join(base.bin, 'infrapro'), args, { env });
  binaryFile(join(dir, 'fixture'), ['seed', join(dir, 'private')], { env });
  const children = ['api', 'monitoring-worker'].map((role) =>
    launch(join(base.bin, role), [], env, join(dir, 'logs', 'seed-' + role + '.log')),
  );
  try {
    await waitURL(lab.seed.api + '/readyz');
    const admin = await client(lab.seed, accounts.admin);
    try {
      const accountID = (await admin.call('/access/accounts?q=arc_admin')).items[0].id;
      const structure = await seedSensorStructure({
        ...admin,
        accountID,
        wait: (path, field, state) =>
          eventually(
            () => admin.call(path),
            (value) => field(value) === state,
          ),
      });
      const actors = { admin: accountID };
      for (const who of ['member', 'other']) {
        const invite = await admin.call('/invitations', {
          note: '架构隔离验收',
        });
        const token = new URLSearchParams(new URL(invite.url).hash.slice(1)).get('token');
        const guest = await client(lab.seed);
        try {
          await guest.call('/public/invitations/accept', {
            ...accounts[who],
            display_name: who === 'member' ? '受限项目成员' : '第二租户管理员',
            token,
          });
        } finally {
          await guest.client.dispose();
        }
        actors[who] = (
          await admin.call('/access/accounts?q=' + accounts[who].username)
        ).items[0].id;
      }
      const other = await admin.call('/platform/tenants', {
        name: '架构验收第二租户',
        code: 'ARC-2',
        administrator_account_id: actors.other,
        client_request_id: randomUUID(),
        reason: '合成范围对照',
      });
      const otherOwner = await client(lab.seed, accounts.other);
      try {
        const member = (await otherOwner.call('/tenants/' + other.tenant_id + '/context'))
          .membership_id;
        const proj = await otherOwner.call('/tenants/' + other.tenant_id + '/projects', {
          name: '架构隔离二项目',
          code: 'SECOND',
          purpose: '隔离对照',
          owner_tenant_membership_id: member,
          administrator_tenant_membership_id: member,
          client_request_id: randomUUID(),
          reason: '合成范围对照',
        });
        write(join(dir, 'samples.json'), {
          actors,
          structure,
          secondTenant: other.tenant_id,
          secondProject: proj.project_id,
        });
      } finally {
        await otherOwner.client.dispose();
      }
    } finally {
      await admin.client.dispose();
    }
  } finally {
    for (const p of children.reverse()) await stop(p);
  }
}
function ch(database, sql, container = 'infrapro-monitoring-contract-ch1-1') {
  assert.match(database, /^arc_[a-z0-9_]+$/);
  return bufferText('docker', [
    'exec',
    container,
    'clickhouse-client',
    '--database',
    database,
    '--query',
    sql,
  ]);
}
function chRows(db, table, container = 'infrapro-monitoring-contract-ch1-1') {
  return docker([
    'exec',
    container,
    'clickhouse-client',
    '--database',
    db,
    '--query',
    'SELECT * FROM ' + table + ' ORDER BY ALL FORMAT RowBinary',
  ]);
}
export function pgManifest(lab, db) {
  const statements = pg(
    lab,
    db,
    "SELECT format($row$SELECT %L AS relation,count(*),md5(coalesce(string_agg(h,'' ORDER BY h),'')) FROM (SELECT md5(to_jsonb(r)::text) h FROM %I.%I r) t;$row$,tablename,schemaname,tablename) FROM pg_tables WHERE schemaname='public' ORDER BY tablename",
  );
  return pg(lab, db, statements);
}
export function restorePostgres(lab, dir, from, db, logical) {
  if (digest(pgManifest(lab, db)) === logical) return;
  assert.equal(
    pg(lab, db, "SELECT count(*) FROM pg_tables WHERE schemaname='public'"),
    '0',
    'target schema already has different data',
  );
  const fd = openSync(join(dir, from + '.pgdump'), 'r');
  try {
    docker(
      [
        'exec',
        '-i',
        lab.container,
        'pg_restore',
        '-U',
        pgUser,
        '-d',
        db,
        '--exit-on-error',
        '--single-transaction',
        '--no-owner',
        '--no-privileges',
      ],
      { stdio: [fd, 'pipe', 'pipe'] },
    );
  } finally {
    closeSync(fd);
  }
  assert.equal(digest(pgManifest(lab, db)), logical, 'restored business rows differ');
}
function restorePayloads(lab, side, base, dir, physical) {
  const env = processEnv(lab, side),
    container = lab.chContainer ?? 'infrapro-monitoring-contract-ch1-1';
  binaryFile(join(base.bin, 'infrapro'), ['monitoring', 'storage', 'init'], {
    env,
  });
  for (const [table, fact] of Object.entries(physical)) {
    assert.match(table, /^[a-z_0-9]+$/);
    const where = lab[side].ch;
    const data = readFileSync(join(dir, table + '.native'));
    assert.equal(digest(data), fact.sha256, 'frozen ClickHouse input differs');
    if (Number(ch(where, 'SELECT count() FROM ' + table, container)) === 0 && fact.rows > 0) {
      docker(
        [
          'exec',
          '-i',
          container,
          'clickhouse-client',
          '--database',
          where,
          '--query',
          'INSERT INTO ' + table + ' FORMAT Native',
        ],
        { input: data },
      );
    }
    assert.equal(
      digest(chRows(where, table, container)),
      fact.logical,
      'restored ClickHouse table ' + table,
    );
  }
  const back = JSON.parse(
    binaryFile(lab.fixture ?? join(lab.directory, 'fixture'), ['restore', dir], { env }),
  );
  assert.deepEqual(back, read(join(dir, 'objects.json')), 'object bytes differ after restore');
}
function snapshot(lab, base) {
  const dir = join(lab.directory, 'snapshot');
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  for (const src of ['seed', 'relay_seed']) {
    const fd = openSync(join(dir, src + '.pgdump'), 'w', 0o600);
    try {
      docker(
        ['exec', lab.container, 'pg_dump', '-U', pgUser, '--no-owner', '-F', 'c', 'arc_' + src],
        { stdio: ['ignore', fd, 'pipe'] },
      );
    } finally {
      closeSync(fd);
    }
  }
  const tables = ch(
    lab.seed.ch,
    "SELECT name FROM system.tables WHERE database=currentDatabase() AND endsWith(engine,'MergeTree') ORDER BY name",
  )
    .split('\n')
    .filter(Boolean);
  const physical = {};
  for (const table of tables) {
    assert.match(table, /^[a-z_0-9]+$/);
    const data = docker([
      'exec',
      'infrapro-monitoring-contract-ch1-1',
      'clickhouse-client',
      '--database',
      lab.seed.ch,
      '--query',
      'SELECT * FROM ' + table + ' ORDER BY ALL FORMAT Native',
    ]);
    writeFileSync(join(dir, table + '.native'), data, { mode: 0o600 });
    physical[table] = {
      sha256: digest(data),
      logical: digest(chRows(lab.seed.ch, table)),
      rows: Number(ch(lab.seed.ch, 'SELECT count() FROM ' + table)),
    };
  }
  binaryFile(join(lab.directory, 'fixture'), ['snapshot', dir], {
    env: processEnv(lab, 'seed'),
  });
  const pgLogical = Object.fromEntries(
    ['seed', 'relay_seed'].map((k) => [k, digest(pgManifest(lab, 'arc_' + k))]),
  );
  for (const side of ['a', 'b', 'self']) {
    for (const [from, db] of [
      ['seed', 'arc_' + side],
      ...(side === 'self' ? [] : [['relay_seed', 'arc_relay_' + side]]),
    ]) {
      restorePostgres(lab, dir, from, db, pgLogical[from]);
    }
    restorePayloads(lab, side, base, dir, physical);
  }
  write(join(dir, 'manifest.json'), {
    physical,
    objects: read(join(dir, 'objects.json')),
    pg: Object.fromEntries(
      ['seed', 'relay_seed'].map((k) => [k, digest(readFileSync(join(dir, k + '.pgdump')))]),
    ),
    pgLogical,
    restored: ['a', 'self', 'b'],
  });
}

export async function prepareLab(dir, frozenSnapshot) {
  const unlock = await fileLease(join(dir, 'private', 'prepare-lab.lock'));
  try {
    return await openLab(dir, frozenSnapshot);
  } finally {
    await unlock();
  }
}

async function openLab(dir, frozenSnapshot) {
  const configPath = join(dir, 'private/lab.json');
  if (existsSync(configPath) && existsSync(join(dir, 'snapshot/manifest.json'))) {
    const lab = read(configPath);
    const state = bufferText('docker', [
      'inspect',
      '-f',
      '{{index .Config.Labels "infrapro.architecture"}} {{.State.Status}}',
      lab.container,
    ]).split(' ');
    assert.equal(state[0], lab.containerLabel ?? lab.id);
    if (state[1] !== 'running') docker(['start', lab.container]);
    await eventually(() => {
      try {
        docker(['exec', lab.container, 'pg_isready', '-U', pgUser, '-d', 'postgres']);
        return true;
      } catch {
        return false;
      }
    }, Boolean);
    const port = activeDatabasePort(lab);
    let changed = false;
    for (const side of ['seed', 'a', 'self', 'b']) {
      const target = lab[side];
      if (!target) continue;
      const config = read(target.config),
        url = new URL(config.database_url);
      assert.equal(url.hostname, '127.0.0.1');
      if (Number(url.port) !== port) {
        url.port = String(port);
        write(target.config, { ...config, database_url: url.href });
      }
      if (target.ports.db !== port) {
        target.ports.db = port;
        changed = true;
      }
    }
    if (changed) write(configPath, lab);
    return lab;
  }
  const base = freeze(dir);
  let lab;
  if (existsSync(configPath)) {
    lab = read(configPath);
    assert.equal(
      bufferText('docker', [
        'inspect',
        '-f',
        '{{index .Config.Labels "infrapro.architecture"}}',
        lab.container,
      ]),
      lab.id,
    );
  } else {
    const id = randomBytes(6).toString('hex'),
      secret = randomBytes(28).toString('hex');
    lab = {
      id,
      container: 'infrapro-arch-' + id,
      directory: dir,
      csrf: randomBytes(32).toString('hex'),
      assets: base.assets,
    };
    mkdirSync(join(dir, 'private'), { recursive: true, mode: 0o700 });
    if (frozenSnapshot) {
      for (const path of ['fixture', 'samples.json', 'private/accounts.json', 'private/mfa.json']) {
        cpSync(join(frozenSnapshot.directory, path), join(dir, path));
      }
      lab.csrf = read(join(frozenSnapshot.directory, 'private/lab.json')).csrf;
    } else {
      createMFAKeyring(join(dir, 'private', 'mfa.json'));
      binaryFile(
        'go',
        ['build', '-o', join(dir, 'fixture'), join(root, 'scripts/architecture-fixture.go')],
        { cwd: base.source },
      );
    }
    docker(['compose', '-f', 'deploy/compose/test.yaml', 'up', '-d', '--wait']);
    if (!frozenSnapshot) {
      docker(['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait']);
      ensureObjectCapacity(id);
    }
    // A private durable Postgres volume avoids tmpfs limits and never addresses a user DB.
    const port = await availablePort();
    docker(
      [
        'run',
        '-d',
        '--label',
        'infrapro.architecture=' + id,
        '--name',
        lab.container,
        '-p',
        '127.0.0.1:' + port + ':5432',
        '-e',
        'POSTGRES_USER=' + pgUser,
        '-e',
        'POSTGRES_PASSWORD',
        '-e',
        'POSTGRES_DB=arc_seed',
        image,
        '-c',
        'max_locks_per_transaction=512',
        '-c',
        'max_connections=256',
      ],
      { env: { ...cleanEnv(), POSTGRES_PASSWORD: secret } },
    );
    await eventually(() => {
      try {
        docker([
          'exec',
          lab.container,
          'pg_isready',
          '-h',
          '127.0.0.1',
          '-U',
          pgUser,
          '-d',
          'arc_seed',
        ]);
        return true;
      } catch {
        return false;
      }
    }, Boolean);
    for (const db of ['arc_relay_seed', 'arc_a', 'arc_b', 'arc_self', 'arc_relay_a', 'arc_relay_b'])
      docker(['exec', lab.container, 'createdb', '-U', pgUser, db]);
    for (const side of ['seed', 'a', 'b', 'self']) {
      const ports = { db: port };
      for (const name of ['web', 'api', 'runtime', 'assets', 'diagnostics'])
        ports[name] = await availablePort();
      lab[side] = makeConfig(dir, side, ports, lab, secret);
    }
    write(configPath, lab);
  }
  if (frozenSnapshot) {
    const source = join(frozenSnapshot.directory, 'snapshot'),
      snapshotDir = join(dir, 'snapshot'),
      facts = read(join(source, 'manifest.json'));
    const original = read(join(frozenSnapshot.directory, 'private/lab.json'));
    assert.equal(base.commit, read(join(frozenSnapshot.directory, 'baseline.json')).commit);
    mkdirSync(snapshotDir, { recursive: true, mode: 0o700 });
    for (const file of readdirSync(source))
      if (file !== 'manifest.json')
        cpSync(join(source, file), join(snapshotDir, file), {
          recursive: true,
        });
    for (const [from, targets] of [
      ['seed', ['seed', 'a', 'self', 'b']],
      ['relay_seed', ['relay_seed', 'relay_a', 'relay_b']],
    ]) {
      assert.equal(digest(readFileSync(join(snapshotDir, from + '.pgdump'))), facts.pg[from]);
      const logical = facts.pgLogical?.[from] ?? digest(pgManifest(original, 'arc_' + from));
      for (const to of targets) restorePostgres(lab, snapshotDir, from, 'arc_' + to, logical);
    }
    const storage = frozenSnapshot.storage;
    lab.chContainer = storage.chContainer;
    for (const side of ['seed', 'a', 'self', 'b']) {
      const settings = read(lab[side].config);
      settings.monitoring_clickhouse_replicas = storage.ch;
      settings.monitoring_object_endpoint = storage.s3;
      for (const domain of ['audit', 'files', 'display'])
        if (settings[domain]?.object_endpoint) settings[domain].object_endpoint = storage.s3;
      write(lab[side].config, settings);
      restorePayloads(lab, side, base, snapshotDir, facts.physical);
    }
    const relay = read(join(frozenSnapshot.directory, 'private/relay-seed.json'));
    relay.database_url = read(lab.seed.config).database_url.replace('/arc_seed', '/arc_relay_seed');
    write(join(dir, 'private/relay-seed.json'), relay);
    write(configPath, lab);
    write(join(dir, 'private/visual-storage.json'), {
      clickhouse: storage.chContainer,
      source: facts.pg.seed,
      verifiedSides: 3,
    });
    write(join(snapshotDir, 'manifest.json'), {
      ...facts,
      sourceDirectory: frozenSnapshot.directory,
      restored: ['a', 'self', 'b'],
    });
    return lab;
  }
  // Relay migrates against its isolated spool before cloning its exact epoch.
  const relayFile = join(dir, 'private/relay-seed.json');
  if (!existsSync(relayFile))
    write(relayFile, {
      relay_id: randomUUID(),
      database_url: read(lab.seed.config).database_url.replace('/arc_seed', '/arc_relay_seed'),
      control_address: '127.0.0.1:' + (await availablePort()),
      certificate_file: 'seed.pem',
      key_file: 'seed.key',
      client_ca_file: 'ca.pem',
      principals_file: 'roles.json',
      budget_bytes: '67108864',
      listener_port_min: 1024,
      listener_port_max: 65535,
      listen_addresses: ['127.0.0.1'],
      max_connections: 16,
    });
  binaryFile(join(base.bin, 'ingest-relay'), ['migrate', relayFile]);
  if (!existsSync(join(dir, 'samples.json'))) await seed(lab, base);
  snapshot(lab, base);
  return lab;
}

// Human operators keep their edits even when parity and fault suites replay.
// Restore only into brand new names inside the labelled laboratory server.
export async function prepareWalkthroughSide(parent, side, schema) {
  assert(['a', 'b'].includes(side));
  const primary = await prepareLab(parent),
    base = read(join(parent, 'baseline.json'));
  // Keep an earlier rehearsal with an intermediate embedded migration intact.
  // Each schema variant gets its own fresh S0 database, bucket and receipts.
  if (side === 'b') assert.match(schema, /^[a-f0-9]{64}$/);
  const directory = join(
    parent,
    'private',
    'walkthrough-lab',
    side === 'b' ? side + '-' + schema.slice(0, 16) : side,
  );
  const plan = join(directory, 'private/lab.json'),
    receipt = join(directory, 'snapshot/manifest.json');
  mkdirSync(join(directory, 'private'), { recursive: true, mode: 0o700 });
  const { httpStorage } = await import('./architecture-http-storage.mjs');
  const existing = existsSync(receipt) && existsSync(plan);
  const storage = await httpStorage(parent, primary.id, { newBuckets: !existing });
  if (existing) {
    const saved = read(plan);
    assert.equal(saved.chContainer, storage.chContainer);
    const cfg = read(saved[side].config),
      database = new URL(cfg.database_url);
    assert.equal(database.hostname, '127.0.0.1');
    database.port = String(activeDatabasePort(saved));
    cfg.database_url = database.href;
    write(saved[side].config, cfg);
    return saved;
  }
  let lab;
  if (existsSync(plan)) lab = read(plan);
  else {
    const id = randomBytes(6).toString('hex'),
      port = activeDatabasePort(primary);
    const url = new URL(read(primary[side].config).database_url);
    assert.equal(url.hostname, '127.0.0.1');
    assert.equal(url.username, pgUser);
    lab = {
      id,
      container: primary.container,
      containerLabel: primary.id,
      directory,
      csrf: primary.csrf,
      assets: base.assets,
      fixture: join(parent, 'fixture'),
      chContainer: storage.chContainer,
    };
    const ports = { db: port };
    for (const key of ['web', 'api', 'runtime', 'assets', 'diagnostics'])
      ports[key] = await availablePort();
    const cfg = makeConfig(directory, side, ports, lab, decodeURIComponent(url.password));
    cfg.database = 'arc_walk_' + id + '_' + side;
    cfg.relayDatabase = 'arc_relay_walk_' + id + '_' + side;
    const settings = read(cfg.config),
      db = new URL(settings.database_url);
    db.pathname = '/' + cfg.database;
    settings.database_url = db.href;
    settings.monitoring_clickhouse_replicas = storage.ch;
    settings.monitoring_object_endpoint = storage.s3;
    settings.files.object_endpoint = storage.s3;
    settings.audit.object_endpoint = storage.s3;
    write(cfg.config, settings);
    lab[side] = cfg;
    write(plan, lab);
  }
  for (const file of [
    'baseline.json',
    'samples.json',
    'private/accounts.json',
    'private/mfa.json',
    'private/relay-seed.json',
  ]) {
    cpSync(join(parent, file), join(directory, file));
  }
  const snapshot = join(parent, 'snapshot'),
    facts = read(join(snapshot, 'manifest.json'));
  const verified = {};
  for (const [from, db] of [
    ['seed', lab[side].database],
    ['relay_seed', lab[side].relayDatabase],
  ]) {
    assert.match(db, /^arc_(?:relay_)?walk_[a-f0-9]{12}_[ab]$/);
    assert.equal(digest(readFileSync(join(snapshot, from + '.pgdump'))), facts.pg[from]);
    if (pg(primary, 'arc_seed', `SELECT count(*) FROM pg_database WHERE datname='${db}'`) === '0') {
      docker(['exec', primary.container, 'createdb', '-U', pgUser, db]);
    }
    const logical = facts.pgLogical?.[from] ?? digest(pgManifest(primary, 'arc_' + from));
    restorePostgres(lab, snapshot, from, db, logical);
    verified[from] = { database: db, archive: facts.pg[from], logical };
  }
  restorePayloads(lab, side, base, snapshot, facts.physical);
  write(receipt, {
    baseline: base.commit,
    restored: [side],
    postgres: verified,
    physical: facts.physical,
    objects: facts.objects,
  });
  return lab;
}

export function queryPG(lab, side, sql) {
  return pg(lab, lab[side].database ?? 'arc_' + side, sql);
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]);
  const lab = await prepareLab(dir);
  console.log('Prepared paired snapshots in ' + lab.directory);
}
