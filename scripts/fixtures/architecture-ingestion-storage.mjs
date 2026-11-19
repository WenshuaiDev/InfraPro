// Keep different acceptance rounds' physical objects recoverable without allocating
// writable bucket volumes from any other round's fixed quota.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const cmd = (program, args) => execFileSync(program, args, { stdio: ['ignore', 'pipe', 'pipe'] });
const json = (file, value) => writeFileSync(file, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
const read = (file) => JSON.parse(readFileSync(file, 'utf8'));
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
export async function ingestionStorage(directory, maxVolumes = 64) {
  const label = createHash('sha256').update(directory).digest('hex').slice(0, 16);
  const container = 'infrapro-arc-ingestion-s3-' + label;
  const config = join(directory, 'storage.json');
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  if (existsSync(config)) {
    const previous = read(config);
    assert.equal(previous.container, container);
    assert.equal(
      cmd('docker', [
        'inspect',
        '-f',
        '{{index .Config.Labels "infrapro.architecture.ingestion"}}',
        container,
      ])
        .toString()
        .trim(),
      label,
    );
    if (cmd('docker', ['inspect', '-f', '{{.State.Running}}', container]).toString().trim() !== 'true')
      cmd('docker', ['start', container]);
    const port = Number(cmd('docker', ['port', container, '8333/tcp']).toString().trim().split(':').at(-1));
    previous.endpoint = 'http://127.0.0.1:' + port;
    json(config, previous);
    return previous;
  }
  const image =
    'chrislusf/seaweedfs:4.53@sha256:ccad23e18639c8b98cee17e7100b95830a06077b56d8e5b006d6dcf8ec5e94a0';
  const spec = {
    container,
    image,
    cpus: 2,
    memory: '768m',
    goMemLimit: '448MiB',
    maxVolumes,
    volumeMiB: 128,
  };
  cmd('docker', [
    'run',
    '-d',
    '--name',
    container,
    '--hostname',
    'arc-ingest-s3',
    '--label',
    'infrapro.architecture.ingestion=' + label,
    '--cpus',
    String(spec.cpus),
    '--memory',
    spec.memory,
    '--memory-swap',
    spec.memory,
    '-e',
    'GOMEMLIMIT=' + spec.goMemLimit,
    '-e',
    'GOGC=40',
    '-p',
    '127.0.0.1::8333',
    '--mount',
    'type=bind,src=' +
      join(root, 'deploy/monitoring-contract/s3.json') +
      ',dst=/etc/contract/s3.json,readonly',
    image,
    'server',
    '-ip=arc-ingest-s3',
    '-dir=/data',
    '-s3',
    '-s3.config=/etc/contract/s3.json',
    '-volume.max=' + spec.maxVolumes,
    '-master.volumeSizeLimitMB=128',
    '-s3.readerCacheSizeMB=24',
    '-s3.concurrentUploadLimitMB=32',
    '-s3.concurrentFileUploadLimit=8',
    '-volume.concurrentDownloadLimitMB=32',
  ]);
  const port = Number(cmd('docker', ['port', container, '8333/tcp']).toString().trim().split(':').at(-1));
  spec.endpoint = 'http://127.0.0.1:' + port;
  for (let i = 0; i < 80; i++) {
    try {
      const reply = await fetch(spec.endpoint, { signal: AbortSignal.timeout(800) });
      if ([200, 403].includes(reply.status)) {
        json(config, spec);
        return spec;
      }
    } catch {
      /* gateway booting */
    }
    await delay(250);
  }
  throw Error('Dedicated SeaweedFS did not become ready');
}
export function stopIngestionStorage(spec) {
  cmd('docker', ['stop', '--time=8', spec.container]);
}
