import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import {
  cpSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
} from 'node:fs';
import { dirname, join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { digest, read, root, write } from './architecture-lab.mjs';

// Every measured round uses the same participant identities and the same
// finite limits. Only loopback listener addresses change between DB clones.
export function copyDisplayTopology(source, destination, firstPort) {
  assert(Number.isInteger(firstPort) && firstPort > 1023 && firstPort < 65530);
  const incoming = join(source, 'roles'),
    roles = readdirSync(incoming, { withFileTypes: true })
      .filter((e) => e.isDirectory() && existsSync(join(incoming, e.name, 'architecture.json')))
      .map((e) => e.name)
      .sort();
  assert(roles.includes('api') && roles.includes('monitoring-query-api'));
  mkdirSync(destination, { recursive: true, mode: 0o700 });
  const identities = {};
  for (const role of roles) {
    const old = join(incoming, role),
      target = join(destination, 'roles', role);
    const cfg = read(join(old, 'architecture.json')),
      urls = Object.entries(cfg.endpoints);
    assert(urls.every(([, address]) => new URL(address).hostname === '127.0.0.1'));
    const start = Math.min(...urls.map(([, address]) => Number(new URL(address).port)));
    for (const [name, address] of urls) {
      const u = new URL(address);
      u.port = String(firstPort + Number(u.port) - start);
      cfg.endpoints[name] = u.origin;
    }
    if (cfg.listen_address) {
      const listener = new URL('https://' + cfg.listen_address);
      assert.equal(listener.hostname, '127.0.0.1');
      const index = Number(listener.port) - start;
      assert(Number.isInteger(index) && index >= 0 && index < 7);
      listener.port = String(firstPort + index);
      cfg.listen_address = listener.host;
    }
    assert.equal(cfg.cert_directory, 'tls');
    if (existsSync(join(old, 'tls'))) {
      cpSync(join(old, 'tls'), join(target, 'tls'), { recursive: true });
      identities[role] = digest(readFileSync(join(target, 'tls', role + '.pem')));
    }
    write(join(target, 'architecture.json'), cfg);
  }
  write(join(destination, 'identities.json'), { roles: identities, ports: 7 });
  const layout = read(join(source, 'layout.json'));
  if (layout.process_resources?.roles?.length) {
    const shape = (slot) => ({
      cpu: slot.cpu,
      memoryMiB: slot.memoryMiB,
      goThreads: slot.goThreads,
      goMemoryMiB: slot.goMemoryMiB,
    });
    const a = Object.fromEntries(
      Object.entries(layout.process_resources.schema58).map(([role, slot]) => [role, shape(slot)]),
    );
    const b = Object.fromEntries(
      layout.process_resources.roles.map((role) => {
        assert.equal(layout.instances[role].length, 1);
        return [role, shape(layout.instances[role][0])];
      }),
    );
    write(join(destination, 'process-budgets.json'), { a, b });
  }
}

export const displayProcessProfile = 'deploy/architecture/display-processes.example.json';
export const displayInflightProfile = 'deploy/architecture/display-inflight.example.json';

export function campaignDisplayTopology(
  dir,
  snapshot,
  destination,
  port,
  withProcessProfile = false,
) {
  const binaries = read(join(dir, 'candidate.json')).binaries;
  const options = withProcessProfile
    ? [
        '--process-budgets',
        join(root, displayProcessProfile),
        '--max-inflight',
        join(root, displayInflightProfile),
      ]
    : [];
  const published = withProcessProfile
    ? [
        displayProcessProfile,
        displayInflightProfile,
        'scripts/architecture-deploy.mjs',
        'scripts/architecture-process-resources.mjs',
      ].map((file) => digest(readFileSync(join(root, file))))
    : [];
  const release = digest(JSON.stringify(withProcessProfile ? [binaries, ...published] : binaries));
  const profile = join(snapshot, 'cohorts', release);
  if (!existsSync(join(profile, 'roles'))) {
    mkdirSync(dirname(profile), { recursive: true, mode: 0o700 });
    const next = profile + '.' + randomUUID();
    execFileSync(
      process.execPath,
      [
        join(root, 'scripts/architecture-deploy.mjs'),
        'prepare',
        '--mode',
        'local',
        '--directory',
        next,
        '--port-base',
        String(port),
        ...options,
      ],
      { cwd: root, stdio: ['ignore', 'ignore', 'pipe'] },
    );
    try {
      renameSync(next, profile);
    } catch (error) {
      if (!['ENOTEMPTY', 'EEXIST'].includes(error.code) || !existsSync(join(profile, 'roles')))
        throw error;
      rmSync(next, { recursive: true, force: true });
    }
  }
  copyDisplayTopology(profile, destination, port);
}
