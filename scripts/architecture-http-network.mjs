import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { BlockList } from 'node:net';
import { networkInterfaces } from 'node:os';
import { join } from 'node:path';
import { digest, write } from './architecture-lab.mjs';

// A /24 must also exclude smaller existing Docker subnets within its range.
export function unusedDockerSubnet(docker, prefix = '10.253', offset = 0) {
  const networks = execFileSync(docker, ['network', 'ls', '-q'], { encoding: 'utf8' })
    .trim()
    .split(/\s+/);
  const definitions = execFileSync(
    docker,
    ['network', 'inspect', '--format', '{{json .IPAM.Config}}', ...networks],
    { encoding: 'utf8' },
  );
  const occupied = new BlockList(),
    addresses = [];
  for (const line of definitions.trim().split('\n')) {
    for (const { Subnet } of JSON.parse(line) ?? []) {
      if (!Subnet || Subnet.includes(':')) continue;
      const [ip, prefix] = Subnet.split('/');
      occupied.addSubnet(ip, Number(prefix));
      addresses.push(ip);
    }
  }
  for (let i = 0; i < 256; i++) {
    const network = prefix + '.' + ((offset + i) % 256) + '.';
    if (!occupied.check(network + '0') && !addresses.some((ip) => ip.startsWith(network))) {
      return network + '0/24';
    }
  }
  throw Error('No unused /24 in the requested Docker address range');
}

export function ensureOwnedDockerNetwork({ name, owner, label, prefix }) {
  const inspectOwner = () => spawnSync('docker', ['network', 'inspect', '--format',
    '{{index .Labels "' + label + '"}}', name], { encoding: 'utf8' });
  const offset = Number.parseInt(digest(owner).slice(0, 2), 16);
  let existing = inspectOwner();
  for (let attempt = 0; existing.status !== 0 && attempt < 4; attempt++) {
    const subnet = unusedDockerSubnet('docker', prefix, offset);
    const created = spawnSync('docker', ['network', 'create', '--label', label + '=' + owner,
      '--subnet', subnet, name], { encoding: 'utf8' });
    if (created.error) throw created.error;
    existing = inspectOwner();
    if (created.status !== 0 && existing.status !== 0 && !/overlap/i.test(created.stderr))
      throw Error('Could not create isolated network ' + name + ': ' + created.stderr.trim());
  }
  if (existing.error) throw existing.error;
  assert.equal(existing.stdout.trim(), owner, 'Unexpected owner for network ' + name);
  return name;
}

// Compose may define several initializers with one image and build definition.
// Warm each image once so the original up --build can reuse all three caches.
export function httpImageBuilds(services) {
  const images = new Map();
  for (const [service, { build, image }] of Object.entries(services)) {
    if (!build) continue;
    assert(image, 'The LAN HTTP build must name its image');
    if (images.has(image)) assert.deepEqual(images.get(image).build, build);
    else images.set(image, { service, build });
  }
  return Array.from(images.values(), entry => entry.service);
}

// The original LAN test trusts only its proxy's /24.
export function isolatedHTTPNetwork(directory, source, role) {
  const project = 'arc-http-' + digest(directory).slice(0, 12) + '-' + role;
  const docker = execFileSync('which', ['docker'], { encoding: 'utf8' }).trim();
  const subnet = unusedDockerSubnet(docker);
  const interfaces = networkInterfaces();
  const host = [...(interfaces.en0 ?? []), ...(interfaces.eth0 ?? [])].find(
    (address) => address.family === 'IPv4' && !address.internal,
  )?.address;
  assert(host, 'The original HTTP browser test requires a LAN IPv4 address');
  const original = join(source, 'deploy/compose/http-test.yaml');
  const definition = JSON.parse(
    execFileSync(
      docker,
      ['compose', '-p', project, '-f', original, '--profile', '*', 'config', '--format', 'json'],
      {
        env: { ...process.env, INFRAPRO_HTTP_TEST_HOST: host },
        encoding: 'utf8',
      },
    ),
  );
  assert(
    definition.services.ops?.profiles?.includes('ops'),
    'The maintenance CLI profile is required',
  );
  const originalSubnet = definition.networks.default.ipam.config[0].subnet;
  assert.equal(originalSubnet, '172.30.43.0/24');
  definition.networks.default.ipam.config[0].subnet = subnet;
  for (const service of Object.values(definition.services)) {
    if (service.environment?.INFRAPRO_TRUSTED_PROXIES === originalSubnet)
      service.environment.INFRAPRO_TRUSTED_PROXIES = subnet;
  }
  const folder = join(directory, 'private', 'lan-http', role);
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const compose = join(folder, 'compose.json');
  write(compose, definition);
  write(join(directory, 'regression', role, 'http-network.json'), {
    project,
    originalSubnet,
    subnet,
    composeSHA256: digest(JSON.stringify(definition)),
  });
  writeFileSync(
    join(folder, 'docker'),
    [
      '#!/bin/sh',
      'if [ "$1" = compose ] && [ "$2" = -f ] && [ "$3" = deploy/compose/http-test.yaml ]; then',
      '  shift 3',
      '  exec "$ARC_LAN_DOCKER" compose -p "$COMPOSE_PROJECT_NAME" -f "$ARC_LAN_COMPOSE" "$@"',
      'fi',
      'exec "$ARC_LAN_DOCKER" "$@"',
      '',
    ].join('\n'),
    { mode: 0o700 },
  );
  return {
    COMPOSE_PROJECT_NAME: project,
    INFRAPRO_HTTP_TEST_HOST: host,
    ARC_LAN_DOCKER: docker,
    ARC_LAN_COMPOSE: compose,
    PATH: folder + ':' + process.env.PATH,
  };
}
