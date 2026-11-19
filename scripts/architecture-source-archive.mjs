import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, renameSync, rmSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { randomUUID } from 'node:crypto';
import { applicationFiles, applicationSourceDigest, root } from './architecture-runtime.mjs';
import { digest, read, write } from './architecture-lab.mjs';

export function selectEvaluatedRelease(directory, releaseDirectory, release) {
  const selected = join(directory, 'candidate.evaluated.json');
  const previous = existsSync(selected) ? read(selected) : null;
  const same =
    previous?.inputs?.sourceSHA256 === release.inputs.sourceSHA256 &&
    previous.directory === releaseDirectory;
  let history;
  if (previous && !same && /^[a-f0-9]{64}$/.test(previous.inputs?.sourceSHA256)) {
    history = 'candidate-history/' + previous.inputs.sourceSHA256 + '.json';
    write(join(directory, history), previous);
  }
  const selection = {
    ...(same ? previous : {}),
    directory: releaseDirectory,
    manifest: relative(directory, join(releaseDirectory, 'candidate.json')),
    inputs: release.inputs,
    schema: release.schema,
    builtAt: release.builtAt,
  };
  if (history && previous.directory && existsSync(join(previous.directory, 'candidate.json'))) {
    const prior = read(join(previous.directory, 'candidate.json'));
    if (prior.inputs.sourceSHA256 === previous.inputs.sourceSHA256) {
      const changed = (before, after) =>
        [...new Set([...Object.keys(before), ...Object.keys(after)])]
          .filter((key) => before[key] !== after[key])
          .sort();
      const file = join(releaseDirectory, 'release-comparison.json');
      const archives = [previous.directory, releaseDirectory].map(dir => join(dir, 'application-source.json'));
      let sourceComparison;
      if (archives.every(path => existsSync(path))) {
        const records = archives.map(path => read(path));
        assert.deepEqual(records.map(record => record.application_source_sha256),
          [prior.inputs.sourceSHA256, release.inputs.sourceSHA256]);
        for (const [index, record] of records.entries()) {
          const archive = join(dirname(archives[index]), record.archive);
          assert.equal(digest(readFileSync(archive)), record.archive_sha256);
        }
        const names = changed(records[0].files, records[1].files);
        sourceComparison = {
          archives: records.map(record => record.archive_sha256),
          changedFiles: names.map(path => ({
            path, before: records[0].files[path] ?? null, after: records[1].files[path] ?? null,
          })),
          identicalNativeBinaries: Object.keys(release.binaries).filter(name =>
            prior.binaries[name] === release.binaries[name]).length,
        };
      }
      write(file, {
        at: new Date().toISOString(),
        previous: prior.inputs.sourceSHA256,
        next: release.inputs.sourceSHA256,
        previousDirectory: previous.directory,
        changedBinaries: changed(prior.binaries, release.binaries),
        changedStatic: changed(prior.staticFiles, release.staticFiles),
        schema: [prior.schema.version, release.schema.version],
        sameMigration: prior.schema.migrationSHA256 === release.schema.migrationSHA256,
        sourceComparison,
      });
      selection.previousManifest = history;
      selection.comparison = relative(directory, file);
    }
  }
  write(selected, selection);
  return selection;
}

export function archiveCandidateSource(directory, manifest) {
  const names = applicationFiles(root);
  assert.equal(applicationSourceDigest(root, names), manifest.inputs.sourceSHA256);
  const files = {},
    included = [];
  let bytes = 0;
  for (const name of names) {
    const path = join(root, name);
    if (!existsSync(path)) continue;
    const data = readFileSync(path);
    files[name] = digest(data);
    included.push(name);
    bytes += data.length;
  }
  const archive = 'application-source.tar.gz',
    temp = join(directory, randomUUID() + '.tar.gz');
  try {
    execFileSync('tar', ['-czf', temp, '--null', '-T', '-'], {
      cwd: root,
      input: included.join('\0') + '\0',
      env: { ...process.env, COPYFILE_DISABLE: '1' },
    });
    const measured = JSON.parse(
      execFileSync(
        'python3',
        [
          '-c',
          'import hashlib,json,sys,tarfile\nwith tarfile.open(sys.argv[1]) as archive:\n print(json.dumps({m.name:hashlib.sha256(archive.extractfile(m).read()).hexdigest() for m in archive if m.isfile()}))',
          temp,
        ],
        { encoding: 'utf8', maxBuffer: 8 << 20 },
      ),
    );
    assert.deepEqual(measured, files, 'Archive bytes differ from the build inputs');
    assert.equal(applicationSourceDigest(root, names), manifest.inputs.sourceSHA256);
    const record = {
      captured: new Date().toISOString(),
      application_source_sha256: manifest.inputs.sourceSHA256,
      manifest_sha256: digest(readFileSync(join(directory, 'candidate.json'))),
      archive,
      archive_sha256: digest(readFileSync(temp)),
      files_count: included.length,
      bytes,
      compressed_bytes: statSync(temp).size,
      archive_readback: true,
      files,
      deleted: names.filter((name) => !(name in files)),
    };
    const target = join(directory, archive);
    renameSync(temp, target);
    write(join(directory, 'application-source.json'), record);
    return record;
  } finally {
    rmSync(temp, { force: true });
  }
}
