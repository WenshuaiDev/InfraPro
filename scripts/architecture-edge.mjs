import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, symlinkSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { digest, launch, read, root, runDirectory, safeDiagnosticLine, write } from './architecture-lab.mjs';
import { pairedCandidate } from './architecture-http-sandbox.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { availablePort, stop, waitURL } from './fixtures/parsing-lab.mjs';

const extraHeaders = [
  'cache-control',
  'content-security-policy',
  'content-type',
  'location',
  'referrer-policy',
  'allow',
  'vary',
  'www-authenticate',
  'x-content-type-options',
  'x-frame-options',
];
const cases = [
  ['root', 'GET', '/'],
  ['console root', 'GET', '/console/'],
  ['old screen login', 'GET', '/screen/login'],
  ['account deep link', 'GET', '/console/access/accounts?q=arc_'],
  ['unknown web path', 'GET', '/arc-unknown-page'],
  ['public account help', 'GET', '/api/v1/public/account-help'],
  ['health', 'GET', '/healthz'],
  ['readiness', 'GET', '/readyz'],
  ['unknown API', 'GET', '/api/v1/arc-unknown-operation'],
  ['wrong method', 'GET', '/api/v1/auth/login'],
  ['anonymous authority', 'GET', '/api/v1/auth/me'],
  ['bootstrap cookie', 'GET', '/api/v1/auth/csrf'],
  ['missing CSRF', 'POST', '/api/v1/auth/login'],
  ['wrong origin', 'POST', '/api/v1/auth/login'],
  ['existing session', 'GET', '/api/v1/auth/me'],
  ['same-origin personnel scope', 'GET', '/api/v1/tenants/{tenant}/context'],
];
async function edge(dir, side, workspace, sharedConsole) {
  const lab = read(join(dir, 'private/lab.json')),
    baseline = read(join(dir, 'baseline.json')),
    cfg = lab[side],
    port = await availablePort();
  const web = join(workspace, 'web'),
    repo = side === 'a' ? baseline.source : root;
  mkdirSync(web, { recursive: true, mode: 0o700 });
  symlinkSync(
    sharedConsole?.console.directory ??
      join(side === 'a' ? baseline.assets : pairedCandidate(dir).release, 'console'),
    join(web, 'console'),
  );
  const template = readFileSync(join(repo, 'deploy/nginx/default.conf'), 'utf8'),
    conf = join(workspace, 'nginx.conf');
  const server = template
    .replace('listen 80;', 'listen 127.0.0.1:' + port + ';')
    .replaceAll('/usr/share/nginx/html', web)
    .replaceAll('http://api:8080', cfg.api);
  assert.notEqual(template, server);
  writeFileSync(
    conf,
    'worker_processes 1;\npid "' +
      join(workspace, 'nginx.pid') +
      '";\nerror_log "' +
      join(workspace, 'error.log') +
      '";\nevents { worker_connections 256; }\nhttp { types { text/html html; text/css css; application/javascript js; }\n' +
      server +
      '\n}\n',
    { mode: 0o600 },
  );
  const executable = process.env.INFRAPRO_TEST_NGINX || 'nginx';
  execFileSync(executable, ['-t', '-p', workspace + '/', '-c', conf], { stdio: 'pipe' });
  const child = launch(
    executable,
    ['-p', workspace + '/', '-c', conf, '-g', 'daemon off;'],
    process.env,
    join(workspace, 'nginx.log'),
  );
  const origin = 'http://127.0.0.1:' + port;
  try {
    await waitURL(origin + '/healthz');
    return { origin, close: () => stop(child), template: digest(template) };
  } catch (e) {
    await stop(child);
    throw e;
  }
}
function checkedCookie(raw) {
  const [pair, ...attributes] = raw.split(';').map((s) => s.trim()),
    p = pair.indexOf('=');
  assert(p > 0);
  const key = pair.slice(0, p),
    value = pair.slice(p + 1);
  assert(value.length >= 16 || value === '', 'invalid test cookie');
  return {
    key,
    attributes: attributes.map((a) => (a.startsWith('Expires=') ? 'Expires={date}' : a)).sort(),
  };
}
function canonical(value, origin, requestID, path = '') {
  if (Array.isArray(value))
    return value.map((v, i) => canonical(v, origin, requestID, path + '.' + i));
  if (value && typeof value === 'object')
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((k) => [k, canonical(value[k], origin, requestID, path + '.' + k)]),
    );
  if (typeof value !== 'string') return value;
  if (path === '.request_id') {
    assert.equal(value, requestID);
    return '{request_id}';
  }
  if (path === '.token') {
    assert(value.length >= 16);
    return '{csrf}';
  }
  if (path === '.session.id') {
    assert.match(value, /^[a-f0-9-]{36}$/);
    return '{session}';
  }
  if (
    /\.session\.(created_at|last_activity_at|expires_at|reauthenticated_at)$/.test(path) &&
    value
  ) {
    assert(Number.isFinite(Date.parse(value)));
    return '{session timestamp}';
  }
  return value.replaceAll(origin, '{configured origin}');
}
async function inspect(cfg, origin, accounts, tenant, transport) {
  const session = await request.newContext({ baseURL: origin, timeout: 15000, maxRedirects: 0 }),
    records = [];
  try {
    let token;
    const requestIDs = new Set();
    for (const [name, method, template] of cases) {
      const path = template.replace('{tenant}', tenant);
      if (name === 'existing session') {
        const login = await session.post('/api/v1/auth/login', {
          headers: { Origin: cfg.origin, 'X-CSRF-Token': token },
          data: accounts.admin,
        });
        assert.equal(login.status(), 200);
        assert.equal((await login.json()).status, 'authenticated');
        await login.dispose();
      }
      const headers = {
        Origin: cfg.origin,
        'X-Request-ID': '17aa889e-4d6c-40f9-a4e0-45fb457fc919',
      };
      if (name === 'wrong origin')
        Object.assign(headers, { Origin: 'http://outside.invalid', 'X-CSRF-Token': token });
      const r = await session.fetch(path, {
        method,
        headers,
        data: method === 'POST' ? {} : undefined,
      });
      try {
        const h = r.headers(),
          raw = await r.body();
        if (h['x-request-id']) {
          const id = h['x-request-id'];
          assert.match(id, /^[A-Z2-7]{26}$/);
          assert(!requestIDs.has(id));
          requestIDs.add(id);
        }
        const cookieFacts = r
          .headersArray()
          .filter((x) => x.name.toLowerCase() === 'set-cookie')
          .map((x) => checkedCookie(x.value));
        let body;
        if ((h['content-type'] || '').includes('json')) {
          const value = JSON.parse(raw);
          if (name === 'bootstrap cookie') token = value.token;
          body = {
            jsonSHA256: digest(JSON.stringify(canonical(value, cfg.origin, h['x-request-id']))),
            code: value.code ?? null,
          };
        } else body = { sha256: digest(raw), bytes: raw.length };
        if (
          [
            'existing session',
            'same-origin personnel scope',
            'public account help',
            'bootstrap cookie',
          ].includes(name)
        )
          assert.equal(r.status(), 200, name);
        if (['wrong origin', 'missing CSRF', 'anonymous authority'].includes(name))
          assert([400, 401, 403].includes(r.status()), name);
        if (name === 'root' && transport === 'nginx') assert.equal(h.location, '/console/');
        if (name === 'account deep link') assert.equal(r.status(), 200);
        const visible = Object.fromEntries(
          extraHeaders.filter((k) => h[k]).map((k) => [k, h[k].replaceAll(origin, '{origin}')]),
        );
        records.push({
          name,
          method,
          template,
          status: r.status(),
          headers: visible,
          hasRequestID: !!h['x-request-id'],
          cookies: cookieFacts,
          body,
        });
      } finally {
        await r.dispose();
      }
    }
    const curl = execFileSync(
      'curl',
      [
        '--silent',
        '--show-error',
        '--output',
        '/dev/null',
        '--write-out',
        '%{http_code}',
        origin + '/api/v1/public/account-help',
      ],
      { encoding: 'utf8' },
    );
    assert.equal(curl, '200');
    return records;
  } finally {
    await session.dispose();
  }
}
export async function edgeParity(dir, output = dir) {
  const lab = read(join(dir, 'private/lab.json')),
    accounts = read(join(dir, 'private/accounts.json')),
    tenant = read(join(dir, 'samples.json')).structure.tenant;
  const parent = join(dir, 'private', 'edge');
  mkdirSync(parent, { recursive: true, mode: 0o700 });
  const directory = mkdtempSync(join(parent, 'run-'));
  const selected = sharedConsoleFor(output, { verifyFiles: true });
  if (selected) {
    assert.equal(selected.backendReleaseSHA256, pairedCandidate(dir).inputs.sourceSHA256);
    assert.equal(selected.baselineReleaseSHA256, read(join(dir, 'baseline.json')).source_archive_sha256);
  }
  const paired = {};
  const records = [];
  for (const side of ['a', 'b']) {
    const location = join(directory, side);
    mkdirSync(location, { mode: 0o700 });
    const proxy = await edge(dir, side, location, selected);
    try {
      paired[side] = {};
      for (const [transport, origin] of [
        ['nginx', proxy.origin],
        ['development proxy', lab[side].origin],
      ]) {
        paired[side][transport] = await inspect(lab[side], origin, accounts, tenant, transport);
        if (selected && transport === 'nginx') {
          for (const name of ['console root', 'account deep link']) {
            const document = paired[side][transport].find(item => item.name === name);
            assert.equal(document.body.sha256, selected.console.files['index.html'], name);
          }
        }
      }
    } finally {
      await proxy.close();
    }
  }
  for (const [transport, left] of Object.entries(paired.a)) {
    const right = paired.b[transport];
    for (const [i, a] of left.entries()) {
      const b = right[i];
      records.push({
        transport,
        name: a.name,
        passed: JSON.stringify(a) === JSON.stringify(b),
        a,
        b,
      });
    }
  }
  const result = {
    arc: 'ARC-002',
    name: 'Nginx, original console development proxy, curl and authority contract',
    passed: records.length === 2 * cases.length && records.every((v) => v.passed),
    checks: records,
  };
  write(join(output, 'http/workflows/edges.json'), {
    sourceRun: dir,
    ...(selected && { consoleSHA256: selected.console.sha256, indexSHA256: selected.console.files['index.html'] }),
    cases: [result],
  });
  return result;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]);
  try {
    const r = await edgeParity(dir, process.argv[3] ? runDirectory(process.argv[3]) : dir);
    console.log('Edge checks', r.checks.length, r.passed);
    process.exitCode = r.passed ? 0 : 1;
  } catch (error) {
    console.error('Error: ' + safeDiagnosticLine(error.message));
    process.exitCode = 1;
  }
}
