import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  activeDatabasePort,
  digest,
  fileLease,
  pgManifest,
  read,
  restorePostgres,
  sleep,
  write,
} from './architecture-lab.mjs';
import { availablePort } from './fixtures/parsing-lab.mjs';

// The copied application fixture and the running operator database have
// different PostgreSQL servers. Restore the S0 dump once into an immutable
// template; each HTTP side starts with PostgreSQL's consistent physical copy.
export async function pairedPostgres(directory, source) {
  assert.match(source.id, /^[0-9a-f]{12}$/);
  const container = 'arc-http-pg-' + source.id;
  const lab = { container, containerLabel: source.id };
  const lock = await fileLease(join(directory, 'private', 'http-pg.lock'));
  try {
    const docker = (args, opts = {}) =>
      execFileSync('docker', args, {
        encoding: 'utf8',
        stdio: 'pipe',
        ...opts,
      }).trim();
    const prior = spawnSync('docker', ['container', 'inspect', container], {
      stdio: 'ignore',
    });
    if (prior.status !== 0) {
      const address = new URL(read(source.a.config).database_url);
      assert.equal(address.hostname, '127.0.0.1');
      assert.equal(address.username, 'archtest');
      const image = docker(['inspect', '-f', '{{.Config.Image}}', source.container]);
      assert.match(image, /^postgres:17-alpine@sha256:[a-f0-9]{64}$/);
      docker(
        [
          'run',
          '-d',
          '--name',
          container,
          '--label',
          'infrapro.architecture=' + source.id,
          '-p',
          '127.0.0.1:' + (await availablePort()) + ':5432',
          '-e',
          'POSTGRES_USER=archtest',
          '-e',
          'POSTGRES_DB=arc_seed',
          '-e',
          'POSTGRES_PASSWORD',
          image,
          '-c',
          'max_locks_per_transaction=512',
          '-c',
          'max_connections=256',
        ],
        {
          env: {
            ...process.env,
            POSTGRES_PASSWORD: decodeURIComponent(address.password),
          },
        },
      );
    } else {
      assert.equal(
        docker(['inspect', '-f', '{{index .Config.Labels "infrapro.architecture"}}', container]),
        source.id,
      );
      if (docker(['inspect', '-f', '{{.State.Running}}', container]) !== 'true')
        docker(['start', container]);
    }
    const snapshot = join(directory, 'snapshot'),
      facts = read(join(snapshot, 'manifest.json'));
    const marker = join(directory, 'private/http-pg-template.json');
    const cluster = docker(['inspect', '-f', '{{.Id}}', container]);
    const record = existsSync(marker) ? read(marker) : null;
    const until = Date.now() + 60000;
    while (
      spawnSync('docker', ['exec', container, 'pg_isready', '-d', 'postgres', '-U', 'archtest'], {
        stdio: 'ignore',
      }).status !== 0
    ) {
      if (Date.now() > until) throw Error('HTTP fixture PostgreSQL did not start');
      await sleep(250);
    }
    if (record?.cluster === cluster)
      assert.equal(record.archive, facts.pg.seed, 'HTTP template source changed');
    if (record?.cluster !== cluster) {
      assert.equal(digest(readFileSync(join(snapshot, 'seed.pgdump'))), facts.pg.seed);
      const logical = facts.pgLogical?.seed ?? digest(pgManifest(source, 'arc_seed'));
      restorePostgres(lab, snapshot, 'seed', 'arc_seed', logical);
      write(marker, {
        container,
        cluster,
        archive: facts.pg.seed,
        logical,
        restoredAt: new Date().toISOString(),
      });
    }
    docker([
      'exec',
      container,
      'psql',
      '-Xq',
      '-v',
      'ON_ERROR_STOP=1',
      '-U',
      'archtest',
      '-d',
      'postgres',
      '-c',
      'ALTER DATABASE arc_seed WITH ALLOW_CONNECTIONS false IS_TEMPLATE true',
    ]);
    return { ...lab, port: activeDatabasePort(lab) };
  } finally {
    await lock();
  }
}
