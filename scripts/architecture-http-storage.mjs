import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { availablePort } from './fixtures/parsing-lab.mjs';
import { fileLease, read, root, sleep, write } from './architecture-lab.mjs';

const docker = (args, options = {}) =>
  execFileSync('docker', args, {
    cwd: root,
    encoding: 'utf8',
    stdio: 'pipe',
    ...options,
  }).trim();

// A closed replay retains its rows on disk for inspection without keeping
// hundreds of inactive MergeTrees attached to the shared ClickHouse server.
export function archiveHTTPStorage(dir, storage, id, databases) {
  assert.match(id, /^[a-f0-9]{12}$/);
  assert.match(storage.chContainer, /^arc-http-storage-[a-f0-9]{12}-ch1-1$/);
  assert(databases.every((db) => db === 'arc_http_' + id + '_a' || db === 'arc_http_' + id + '_b'));
  const query = (sql) => docker(['exec', storage.chContainer, 'clickhouse-client', '--query', sql]);
  const archived = [];
  const file = join(dir, 'storage-archive.json');
  const previous = existsSync(file) ? read(file) : null;
  for (const db of databases) {
    const tables = query("SELECT name FROM system.tables WHERE database='" + db + "' ORDER BY name")
      .split('\n')
      .filter(Boolean);
    for (const table of tables) {
      assert.match(table, /^[a-z][a-z0-9_]*$/);
      archived.push(db + '.' + table);
    }
  }
  if (!archived.length) {
    if (previous?.status === 'detaching') write(file, { ...previous, status: 'detached' });
    return;
  }
  const tables = [...new Set([...(previous?.tables ?? []), ...archived])];
  const receipt = {
    at: new Date().toISOString(),
    container: storage.chContainer,
    tables,
    inspection: tables.map((table) => 'ATTACH TABLE ' + table),
  };
  write(file, { ...receipt, status: 'detaching' });
  docker(['exec', '-i', storage.chContainer, 'clickhouse-client', '--multiquery'], {
    input:
      archived.map((table) => 'DETACH TABLE IF EXISTS ' + table + ' PERMANENTLY SYNC;').join('\n') +
      '\n',
  });
  write(file, { ...receipt, status: 'detached' });
}
// Frozen and candidate regression scripts may restart their original Compose
// stacks. A separate named network and fixed ports keep paired HTTP transfers,
// long running imports and analytic snapshot readers isolated from those tests.
async function prepareBucketCapacity(project, compose, config, folder, newBuckets) {
  const prefix = 's3-volume-';
  const services = Object.keys(config.services).filter((role) => role.startsWith(prefix));
  if (!newBuckets && !services.length) return;
  const start = (roles) =>
    docker(['compose', '-p', project, '-f', compose, 'up', '-d', '--no-deps', ...roles]);
  const topology = () => {
    const summary = docker(['exec', '-i', project + '-s3-1', 'weed', 'shell', '-master=s3:9333'], {
      input: 'volume.list -v=0\n',
    });
    const line = summary.split('\n').find((line) => line.startsWith('Topology '));
    const slots = line?.match(/\bvolume:\d+\/(\d+)\b.*?\bfree:(\d+)\b/);
    assert.ok(slots, 'SeaweedFS master did not report total and free volume slots');
    return { total: Number(slots[1]), free: Number(slots[2]) };
  };
  const capacity = (role, flag) =>
    Number(config.services[role].command.find((arg) => arg.startsWith(flag))?.slice(flag.length));
  const expected =
    capacity('s3', '-volume.max=') + services.reduce((n, role) => n + capacity(role, '-max='), 0);
  assert.ok(Number.isInteger(expected) && expected > 0);
  let observed = topology();
  for (let n = 0; observed.total < expected && n < 60; n++) {
    await sleep(500);
    observed = topology();
  }
  assert.ok(observed.total >= expected, 'Existing isolated volume servers did not reregister');
  // SeaweedFS allocates up to eight volumes on a collection's first write.
  // A full lab restores seed, A, A-self and B, while a domain replay restores two.
  const collections = newBuckets === true ? 2 : Number(newBuckets);
  assert.ok(Number.isSafeInteger(collections) && collections >= 0 && collections <= 4);
  const required = collections * 8;
  const before = observed.free;
  if (!required || before >= required) return;
  const role = prefix + (services.length + 1);
  config.volumes[role] = {};
  config.services[role] = {
    image: config.services.s3.image,
    command: ['volume', '-ip=' + role, '-dir=/data', '-master=s3:9333', '-max=512'],
    environment: { GOMEMLIMIT: '192MiB' },
    mem_limit: '256m',
    volumes: [{ type: 'volume', source: role, target: '/data' }],
  };
  write(compose, config);
  start([role]);
  for (let n = 0; n < 60; n++) {
    const { free: after, total } = topology();
    if (total >= expected + 512 && after >= required) {
      write(join(folder, role + '.json'), {
        at: new Date().toISOString(),
        project,
        role,
        collections,
        freeBefore: before,
        freeAfter: after,
      });
      return;
    }
    await sleep(500);
  }
  throw Error('Supplemental test volume server did not register with its isolated master');
}

export async function httpStorage(dir, id, { newBuckets = false } = {}) {
  assert.match(id, /^[0-9a-f]{12}$/);
  const project = 'arc-http-storage-' + id,
    folder = join(dir, 'private', 'http-storage');
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const descriptor = join(folder, 'endpoints.json'),
    compose = join(folder, 'compose.json');
  const running = () => {
    const roles = [
      's3',
      'keeper',
      'ch1',
      'ch2',
      ...Object.keys(read(compose).services).filter((role) => /^s3-volume-\d+$/.test(role)),
    ];
    const states = docker([
      'inspect',
      '-f',
      '{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{end}}',
      ...roles.map((role) => project + '-' + role + '-1'),
    ]).split('\n');
    return (
      states.length === roles.length &&
      states.every(
        (state, i) =>
          state.trim() === 'true healthy' ||
          (!['ch1', 'ch2'].includes(roles[i]) && state.trim() === 'true'),
      )
    );
  };
  const ready = () => {
    try {
      return existsSync(descriptor) && read(descriptor).ch2 && running();
    } catch {
      return false;
    }
  };
  const unlock = await fileLease(join(folder, 'startup.lock'));
  try {
    if (!existsSync(compose) || !read(compose).services.ch2) {
      const source = JSON.parse(
        docker([
          'compose',
          '-f',
          'deploy/monitoring-contract/compose.yaml',
          'config',
          '--format',
          'json',
        ]),
      );
      const existing = existsSync(compose) ? read(compose) : null;
      const services = Object.fromEntries(
        ['s3', 'keeper', 'ch1', 'ch2'].map((name) => [name, source.services[name]]),
      );
      services.s3.command = services.s3.command.map((v) =>
        v.startsWith('-volume.max=') ? '-volume.max=1024' : v,
      );
      const volumes = {};
      for (const [role, dest] of [
        ['s3', '/data'],
        ['ch1', '/var/lib/clickhouse'],
        ['ch2', '/var/lib/clickhouse'],
        ['keeper', '/var/lib/clickhouse-keeper'],
      ]) {
        volumes[role] = {};
        services[role].volumes.push({
          type: 'volume',
          source: role,
          target: dest,
        });
      }
      const ports = new Set();
      for (const role of ['s3', 'ch1', 'ch2'])
        for (const mapping of services[role].ports) {
          let port = existing?.services[role]?.ports?.find(
            (p) => p.target === mapping.target,
          )?.published;
          if (!port) {
            do {
              port = await availablePort();
            } while (ports.has(Number(port)));
          }
          ports.add(Number(port));
          mapping.published = String(port);
        }
      write(compose, {
        name: project,
        services,
        volumes,
        networks: { default: { name: project + '_default' } },
      });
    }
    if (!ready())
      docker([
        'compose',
        '-p',
        project,
        '-f',
        compose,
        'up',
        '-d',
        '--wait',
        '--wait-timeout',
        '90',
      ]);
    const config = read(compose),
      port = (role, target) =>
        Number(config.services[role].ports.find((p) => p.target === target).published);
    const result = {
      chContainer: project + '-ch1-1',
      ch: '127.0.0.1:' + port('ch1', 9000),
      ch2: '127.0.0.1:' + port('ch2', 9000),
      s3: 'http://127.0.0.1:' + port('s3', 8333),
    };
    {
      let writable = false;
      for (let n = 0; n < 45 && !writable; n++) {
        try {
          const shell = ['exec', '-i', project + '-s3-1', 'weed', 'shell', '-master=s3:9333'];
          const hasBucket = (input) =>
            docker(shell, { input })
              .split('\n')
              .some((row) => /^contract-ch(?:\s|$)/.test(row.trim()));
          writable =
            hasBucket('s3.bucket.list\n') ||
            hasBucket('s3.bucket.create -name=contract-ch\ns3.bucket.list\n');
        } catch {
          /* Wait for this master's initial election. */
        }
        if (!writable) await sleep(500);
      }
      assert.ok(writable, 'isolated HTTP object store has not registered its first volume');
    }
    await prepareBucketCapacity(project, compose, config, folder, newBuckets);
    write(descriptor, result);
    return result;
  } finally {
    await unlock();
  }
}
