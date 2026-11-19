import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { basename, dirname, join, resolve } from 'node:path';
import { digest, fileLease, launch, read, root } from './architecture-lab.mjs';
import { pairedCandidate } from './architecture-http-sandbox.mjs';
import { archiveHTTPStorage } from './architecture-http-storage.mjs';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';
import { httpOwners } from './architecture-runtime.mjs';
import { stop, waitURL } from './fixtures/parsing-lab.mjs';

// A host with a nearly full SeaweedFS store can reopen this exact completed
// fixture. Restore only its detached tables and bind the same signed binaries,
// actual restricted database logins, trusted origins, certificates and ports.
export async function resumeFocusPair(directory, saved, endpoints, work) {
  const path = resolve(saved);
  assert.equal(dirname(path), join(directory, 'http/sandboxes'));
  assert.match(basename(path), /^import-focus-[a-f0-9]{12}$/);
  const lab = read(join(path, 'private/lab.json')), snapshot = read(join(path, 'snapshot.json'));
  assert.equal(basename(path), 'import-focus-' + lab.id);
  assert.equal(path, lab.directory);
  const release = pairedCandidate(directory), baseline = read(join(path, 'baseline.json'));
  assert.equal(snapshot.candidateSHA256, release.inputs.sourceSHA256);
  assert.equal(baseline.source_archive_sha256, read(join(directory, 'baseline.json')).source_archive_sha256);
  assert.deepEqual(snapshot.appliedRoleSQL, Object.fromEntries(postgresRoleSQLFiles.map(file =>
    [file, digest(readFileSync(join(root, file)))])));
  const archive = read(join(path, 'storage-archive.json'));
  assert.equal(archive.status, 'detached');
  assert.equal(archive.container, lab.chContainer);
  assert.match(archive.container, /^arc-http-storage-[a-f0-9]{12}-ch1-1$/);
  const tables = archive.tables;
  assert(tables.length && tables.every(t => new RegExp('^arc_http_' + lab.id + '_[ab]\\.[a-z][a-z0-9_]*$').test(t)));
  const unlock = await fileLease(join(path, 'focus-resume.lock'), { wait: false });
  const children = [], claimed = new Set();
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_')));
  let attached = false;
  try {
    for (const side of ['a', 'b']) {
      const cfg = read(lab[side].config);
      const origin = cfg.trusted_origins.split(',').find(url => url !== lab[side].api);
      assert.match(origin, /^http:\/\/127\.0\.0\.1:\d+$/);
      endpoints[side] = { origin, api: lab[side].api };
      for (const url of [origin, lab[side].api]) claimed.add(Number(new URL(url).port));
    }
    for (const url of Object.values(read(join(path, 'private/b/api.json')).endpoints)) claimed.add(Number(new URL(url).port));
    for (const port of claimed) {
      const server = createServer();
      await new Promise((ok, fail) => server.once('error', fail).listen(port, '127.0.0.1', ok));
      await new Promise(ok => server.close(ok));
    }
    attached = true;
    execFileSync('docker', ['exec','-i',archive.container,'clickhouse-client','--multiquery'], {
      input: tables.map(table => 'ATTACH TABLE IF NOT EXISTS ' + table + ';').join('\n'), stdio: ['pipe','pipe','pipe'],
    });
    for (const side of ['a', 'b']) {
      const privateDir = join(path, 'private', side);
      const bin = side === 'a' ? baseline.bin : release.bin;
      for (const role of side === 'a' ? ['api'] : [...httpOwners, 'api']) {
        const variables = { ...env, INFRAPRO_CONFIG: lab[side].config };
        if (side === 'b') {
          variables.INFRAPRO_CONFIG = role === 'api' ? join(privateDir, 'gateway-runtime.json') :
            join(privateDir, 'credentials/roles', role, 'runtime.json');
          variables.INFRAPRO_ARCHITECTURE_FILE = join(privateDir, role + '.json');
        }
        children.push(launch(join(bin, role), [], variables, join(path, 'logs', 'focus-resume-' + side + '-' + role + '.log')));
      }
    }
    await Promise.all(['a','b'].map(side => waitURL(lab[side].api + '/readyz')));
    assert(children.every(child => child.exitCode === null && !child.signalCode));
    return await work(path);
  } finally {
    try {
      await Promise.all(children.map(stop));
      if (attached) archiveHTTPStorage(path, { chContainer: archive.container }, lab.id, [lab.a.ch, lab.b.ch]);
    } finally { await unlock(); }
  }
}
