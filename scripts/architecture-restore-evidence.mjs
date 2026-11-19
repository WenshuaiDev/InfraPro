import { constants, cpSync, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { digest, read } from './architecture-lab.mjs';

const expectedTests = {
  monitoring: 'TestMonitoringIndependentRestore',
  display: 'TestMonitoringIndependentRestore',
  files: 'TestFilesFrozenBackupIndependentDatabaseAndObjectsRestore',
  notifications: 'TestUpgradeFrom35AndIndependentNotificationRestore',
  audit: 'TestAuditFrozenBackupIndependentRestore',
};

export function validRestoreEvidence(directory, name, result) {
  const [, domain, action] = name.split(':');
  const evidence = result.restoreEvidence;
  if (action !== 'restore' || !Object.hasOwn(expectedTests, domain) || !evidence?.complete ||
      evidence.test !== expectedTests[domain] || !evidence.files?.length) return false;
  const fingerprint = (path, expected) => existsSync(path) && statSync(path).isFile() && digest(readFileSync(path)) === expected;
  const target = resolve(directory, 'restore', domain, result.startedAt.replaceAll(':', '-'));
  if (!fingerprint(join(directory, result.log), evidence.runnerLogSHA256)) return false;
  if (!evidence.files.every(({ file, sha256 }) => {
    const path = resolve(directory, file);
    return dirname(path) === target && fingerprint(path, sha256);
  })) return false;
  if (domain !== 'monitoring' && domain !== 'display') return true;
  const archive = evidence.archive;
  const path = join(target, 'archive');
  return Boolean(archive && resolve(directory, archive.directory) === path &&
    fingerprint(join(path, 'manifest.json'), archive.manifest_sha256));
}

export function archiveRestoreEvidence(directory, cwd, name, result) {
  const [, domain, action] = name.split(':');
  if (action !== 'restore' || !Object.hasOwn(expectedTests, domain)) return;

  const original = readFileSync(join(directory, result.log));
  const output = original.toString();
  const parent = join('restore', domain, result.startedAt.replaceAll(':', '-'));
  const target = join(directory, parent);
  mkdirSync(target, { recursive: true, mode: 0o700 });
  const files = [];
  function retain(source) {
    if (!existsSync(source) || !statSync(source).isFile()) return false;
    const bytes = readFileSync(source);
    const name = basename(source);
    writeFileSync(join(target, name), bytes, { mode: 0o600 });
    files.push({ file: join(parent, name), source: relative(cwd, source), sha256: digest(bytes), bytes: bytes.length });
    return true;
  }
  const evidence = { test: expectedTests[domain], files, runnerLogSHA256: digest(original) };
  const passed = new Set([...output.matchAll(/^--- PASS: (\S+)/gm)].map(match => match[1]));
  let complete = passed.has(expectedTests[domain]);
  if (domain === 'monitoring' || domain === 'display') {
    const path = output.match(/^Independent restore evidence: (.+)$/m)?.[1]?.trim();
    const root = resolve(cwd, '.artifacts', 'monitoring-restore');
    if (path && resolve(path) === join(root, basename(path))) {
      const archive = join(path, 'archive');
      const manifest = join(archive, 'manifest.json');
      if (existsSync(manifest)) {
        cpSync(archive, join(target, 'archive'), { recursive: true, mode: constants.COPYFILE_FICLONE });
        evidence.archive = {
          directory: join(parent, 'archive'),
          source: relative(cwd, archive),
          manifest_sha256: digest(readFileSync(manifest)),
        };
        const assets = join(path, 'display-objects');
        if (existsSync(assets)) {
          cpSync(assets, join(target, 'display-objects'), { recursive: true, mode: constants.COPYFILE_FICLONE });
        }
      } else {
        complete = false;
      }
      for (const name of ['result.json', 'restore-proof.json', 'restore.log', 'source.log', 'target.log', 'target-services.log', 'cleanup.log']) {
        const found = retain(join(path, name));
        if (['result.json', 'restore-proof.json', 'restore.log'].includes(name) && !found) complete = false;
      }
      if (!existsSync(join(path, 'result.json')) || read(join(path, 'result.json')).status !== 'passed') complete = false;
    } else {
      complete = false;
    }
  } else if (domain === 'files' || domain === 'audit') {
    const path = join(cwd, '.artifacts', domain, 'restore.log');
    if (!existsSync(path) || statSync(path).mtimeMs < Date.parse(result.startedAt) - 2 || !retain(path)) complete = false;
  } else {
    // The original notification restore prints its assertion transcript to stdout.
    retain(join(directory, result.log));
  }
  evidence.complete = complete;
  result.restoreEvidence = evidence;
  if (result.status === 'passed' && !complete) {
    result.status = 'failed';
    result.failure = 'The original restore command did not produce the expected test and its verification evidence';
  }
}
