import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { digest, read, write } from '../../scripts/architecture-lab.mjs';
import { selectEvaluatedRelease } from '../../scripts/architecture-source-archive.mjs';

test('a release change verifies each source archive in its own directory', t => {
  const run = mkdtempSync(join(tmpdir(), 'infrapro-release-'));
  t.after(() => rmSync(run, { recursive: true, force: true }));
  const versions = ['old', 'new'].map(version => {
    const directory = join(run, version);
    const release = {
      inputs: { sourceSHA256: digest(version) },
      schema: { version: 59, migrationSHA256: digest('migration') },
      binaries: { api: digest('same executable'), worker: digest(version) },
      staticFiles: {},
    };
    const archive = 'application-source.tar.gz';
    const bytes = Buffer.from('archived bytes ' + version);
    write(join(directory, 'candidate.json'), release);
    write(join(directory, 'application-source.json'), {
      application_source_sha256: release.inputs.sourceSHA256,
      archive, archive_sha256: digest(bytes), files: { 'worker.go': digest(version) },
    });
    writeFileSync(join(directory, archive), bytes);
    return { directory, release, archive, bytes };
  });
  const [old, next] = versions;
  selectEvaluatedRelease(run, old.directory, old.release);
  writeFileSync(join(next.directory, next.archive), 'changed');
  assert.throws(() => selectEvaluatedRelease(run, next.directory, next.release));
  assert.equal(read(join(run, 'candidate.evaluated.json')).directory, old.directory);
  writeFileSync(join(next.directory, next.archive), next.bytes);
  const selected = selectEvaluatedRelease(run, next.directory, next.release);
  const comparison = read(join(run, selected.comparison));
  assert.deepEqual(comparison.sourceComparison.archives, versions.map(v => digest(v.bytes)));
  assert.equal(comparison.sourceComparison.identicalNativeBinaries, 1);
  assert.deepEqual(comparison.changedBinaries, ['worker']);
});
