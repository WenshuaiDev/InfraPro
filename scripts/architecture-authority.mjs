// Run the frozen transactional race tests against each release. The overlay
// changes only localhost storage addresses in the pre-existing test fixture.
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { httpStorage } from './architecture-http-storage.mjs';
import { read, root, runDirectory, write } from './architecture-lab.mjs';
import {
  recordCommand,
  regressionRelease,
  verifyRegressionSource,
} from './architecture-regression.mjs';
import { testEnv } from './test-environment.mjs';

const suites = {
  devicecatalog: [
    'TestSourceInvalidationBothCommitOrders',
    'TestProjectOutputOnlyGrantsAndTwoEndedSourceAuthority',
  ],
  resources: [
    'TestRevocationAndProjectWriteBothCommitOrders',
    'TestMonitoringExplicitTimeGrantsAndRevocation',
    'TestMonitoringPartialProjectMetadataAndDownloadRevocation',
    'TestDerivedCurrentExportStopsAfterRevokedDownloadBlock',
    'TestDisplayReadDirectorySnapshotAndIsolation',
    'TestDisplayReadWorkerInstanceConcurrencyAndRevocation',
  ],
  identity: ['TestIntegrationSessionRevocationAndRecovery'],
  tenancy: ['TestTenantLifecycleIndependentPermissionsAndTransactionGuards'],
};
const sides = ['baseline', 'candidate'];
const additional = [
  'TestArchitectureBoundDisplayRead',
  'TestArchitectureTrialFinishesWithExistingClaim',
];

function testNames(side, pkg) {
  return side === 'candidate' && pkg === 'resources'
    ? [...suites[pkg], ...additional]
    : suites[pkg];
}

function casesFor(dir, side, pkg) {
  const names = testNames(side, pkg);
  const folder = join(dir, 'http/authority', side),
    receipt = join(folder, pkg + '.json');
  if (!existsSync(receipt)) return [];
  const record = read(receipt);
  if (record.status === 'running') return [];
  const log = pkg + '.log';
  const states = new Map();
  for (const line of readFileSync(join(folder, log), 'utf8').split('\n')) {
    if (!line.startsWith('{')) continue;
    const e = JSON.parse(line);
    if (e.Test && ['skip', 'pass', 'fail'].includes(e.Action)) states.set(e.Test, e.Action);
  }
  return names.map((name) => {
    const subcases = [...states]
      .filter(([id]) => id.startsWith(name + '/'))
      .map(([name, state]) => ({ name, state }));
    return {
      arc: name === 'TestArchitectureTrialFinishesWithExistingClaim' ? 'ARC-011' : 'ARC-005',
      name: side + ' ' + name,
      passed:
        record.status === 'passed' &&
        record.exitCode === 0 &&
        record.sourceAtCompletion?.matchesSelectedRelease === true &&
        states.get(name) === 'pass' &&
        subcases.every((r) => r.state === 'pass'),
      source: 'http/authority/' + side + '/' + log,
      startedAt: record.startedAt,
      subcases,
    };
  });
}

function saveEvidence(dir) {
  const cases = sides.flatMap((side) =>
    Object.keys(suites).flatMap((pkg) => casesFor(dir, side, pkg)),
  );
  const expected = sides.flatMap((side) =>
    Object.keys(suites).flatMap((pkg) => testNames(side, pkg)),
  ).length;
  const complete = cases.length === expected;
  write(join(dir, 'http/workflows/authority-races.json'), { cases, complete, expected });
  return complete && cases.every((r) => r.passed);
}

export async function authorityRaces(dir, selected = 'both') {
  assert(['both', ...sides].includes(selected));
  const storage = await httpStorage(dir, read(join(dir, 'private/lab.json')).id);
  for (const side of sides.filter((s) => selected === 'both' || selected === s)) {
    const release = regressionRelease(dir, side);
    const cwd = side === 'baseline' ? read(join(dir, 'baseline.json')).source : root;
    const folder = join(dir, 'http/authority', side);
    mkdirSync(folder, { mode: 0o700, recursive: true });
    const source = join(cwd, 'packages/go/platform/resources/monitoring_import_test.go');
    const original = readFileSync(source, 'utf8');
    const config = original
      .replaceAll('127.0.0.1:59000', storage.ch)
      .replaceAll('127.0.0.1:59001', storage.ch2)
      .replaceAll('127.0.0.1:58333', new URL(storage.s3).host);
    const local = join(folder, 'storage-fixture.go');
    assert.notEqual(config, original);
    writeFileSync(local, config, { mode: 0o600 });
    const overlay = join(folder, 'overlay.json');
    write(overlay, { Replace: { [source]: local } });
    const env = { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1' };
    for (const pkg of Object.keys(suites)) {
      const names = testNames(side, pkg);
      verifyRegressionSource(release);
      const result = await recordCommand(
        folder,
        pkg,
        'go',
        [
          'test',
          '-overlay',
          overlay,
          '-p',
          '1',
          '-parallel',
          '1',
          '-count=1',
          '-tags',
          'monitoringintegration',
          '-json',
          '-timeout',
          '12m',
          './packages/go/platform/' + pkg,
          '-run',
          '^(' + names.join('|') + ')$',
        ],
        {
          cwd,
          env,
          provenance: {
            role: side,
            releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
          },
        },
      );
      verifyRegressionSource(release, result);
      write(join(folder, pkg + '.json'), result);
      saveEvidence(dir);
    }
  }
  return saveEvidence(dir);
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = (await authorityRaces(runDirectory(process.argv[2]), process.argv[3])) ? 0 : 1;
}
