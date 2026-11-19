import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, symlinkSync } from 'node:fs';
import { join } from 'node:path';
import { httpStorage } from './architecture-http-storage.mjs';
import { prepareLab, read, runDirectory, write } from './architecture-lab.mjs';
import { currentRoleSQL } from './architecture-parity-evidence.mjs';
import { startSide } from './architecture-serve.mjs';

export function cohortCommands(cohort, output) {
  const governance = 'scripts/architecture-governance-parity.mjs';
  return [
    ['architecture-edge', 'scripts/architecture-edge.mjs', [cohort, output]],
    ['architecture-governance-parity', governance, [cohort, output]],
    ...['definitions', 'taxonomy', 'products', 'devices'].map((section) => [
      'catalog-' + section,
      'scripts/architecture-catalog-parity.mjs',
      [cohort, output, section],
    ]),
    ...['governance-roles', 'governance-security'].map((group) => [
      group,
      governance,
      [cohort, output, group],
    ]),
    [
      'architecture-display-files-parity',
      'scripts/architecture-display-files-parity.mjs',
      [cohort, 'both', output],
    ],
  ];
}

// These journeys change memberships, catalogues, sessions and files. Start
// both applications from the original snapshot even after a migration edit.
export async function openPublicCohort(dir, releaseDirectory) {
  const directory = runDirectory(),
    frozen = read(join(dir, 'baseline.json'));
  const manifest = read(join(releaseDirectory, 'candidate.json'));
  const stacks = [],
    descriptor = join(directory, 'private', 'lab.json');
  symlinkSync(frozen.source, join(directory, 'baseline-source'), 'dir');
  symlinkSync(frozen.bin, join(directory, 'baseline-bin'), 'dir');
  for (const name of ['candidate.json', 'static-files.json'])
    cpSync(join(releaseDirectory, name), join(directory, name));
  async function close() {
    try {
      while (stacks.length) await stacks.pop().close();
    } finally {
      if (existsSync(descriptor)) {
        const lab = read(descriptor);
        assert.equal(lab.directory, directory);
        const owner = execFileSync(
          'docker',
          [
            'inspect',
            '--format',
            '{{index .Config.Labels "infrapro.architecture"}}',
            lab.container,
          ],
          { encoding: 'utf8' },
        ).trim();
        assert.equal(owner, lab.id);
        execFileSync('docker', ['stop', lab.container], { stdio: 'pipe' });
      }
    }
  }
  try {
    const primary = read(join(dir, 'private', 'lab.json'));
    const lab = await prepareLab(directory, {
      directory: dir,
      storage: await httpStorage(dir, primary.id, { newBuckets: 4 }),
    });
    const snapshot = read(join(directory, 'snapshot', 'manifest.json'));
    assert.deepEqual(snapshot.pg, read(join(dir, 'snapshot', 'manifest.json')).pg);
    for (const side of ['a', 'b'])
      stacks.push(await startSide(directory, side, { releaseDirectory }));
    const appliedRoleSQL = read(join(directory, 'role-grants.json'));
    assert.deepEqual(appliedRoleSQL, currentRoleSQL());
    write(join(dir, 'http', 'environments', lab.id + '.json'), {
      directory,
      baseline: frozen.commit,
      candidate: manifest.inputs.sourceSHA256,
      snapshot: snapshot.pg,
      appliedRoleSQL,
      at: new Date().toISOString(),
    });
    return { directory, close };
  } catch (error) {
    await close();
    throw error;
  }
}
