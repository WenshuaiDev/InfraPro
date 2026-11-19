import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { basename, dirname, join, normalize } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { digest, read, root, write } from './architecture-lab.mjs';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';

const scenario = (name) => 'http/scenarios/' + name + '.json';
const workflow = (name) => 'http/workflows/' + name + '.json';
const pair = (name) => [scenario(name), workflow(name)];
export const outputs = {
  'architecture-edge': [workflow('edges')],
  'architecture-http-parity': ['http/operations.json'],
  'architecture-governance-parity': pair('governance'),
  'governance-roles': pair('governance-roles'),
  'governance-security': pair('governance-security'),
  ...Object.fromEntries(
    ['definitions', 'taxonomy', 'products', 'devices'].map((name) => [
      'catalog-' + name,
      pair('catalog-' + name),
    ]),
  ),
  'architecture-engineering-parity': pair('engineering-all'),
  'architecture-resource-parity': pair('resources'),
  'architecture-monitoring-parity': pair('monitoring'),
  'architecture-analysis-parity': pair('analysis'),
  'architecture-derived-parity': pair('derived'),
  'architecture-spatial-parity': pair('spatial'),
  'architecture-ingestion-http': [scenario('ingestion-parsing')],
  'architecture-task-parity': [
    scenario('task-notifications'),
    workflow('task-notifications-audit'),
  ],
  'architecture-audit-parity': pair('audit-task'),
  'architecture-display-files-parity': pair('display-files-both'),
  'architecture-authority': [workflow('authority-races')],
};
const commandOutputs = new Set(Object.values(outputs).flat());

export function archivePublicAttempt(dir, label, command) {
  if (!outputs[label] || !command.startedAt || !command.finishedAt) return;
  const attempt = join('http/attempts', label, command.startedAt.replaceAll(':', '-'));
  const files = {};
  for (const file of outputs[label]) {
    const source = join(dir, file);
    if (!existsSync(source)) continue;
    const at = statSync(source).mtimeMs;
    if (at < Date.parse(command.startedAt) - 2 || at > Date.parse(command.finishedAt) + 2) continue;
    const bytes = readFileSync(source);
    const target = join(dir, attempt, file);
    mkdirSync(dirname(target), { recursive: true, mode: 0o700 });
    if (!existsSync(target)) writeFileSync(target, bytes, { flag: 'wx', mode: 0o600 });
    files[file] = digest(bytes);
    if (digest(readFileSync(target)) !== files[file]) throw Error('Archived replay output changed: ' + file);
  }
  if (!Object.keys(files).length) return;
  const manifest = join(attempt, 'manifest.json');
  write(join(dir, manifest), {
    label, startedAt: command.startedAt, finishedAt: command.finishedAt,
    releaseSHA256: command.provenance?.releaseSHA256, status: command.status,
    exitCode: command.exitCode, failure: command.failure, files,
  });
  return manifest;
}

function runningGrants(dir, provenance) {
  const cohort = provenance?.httpEnvironment;
  if (cohort && dirname(cohort) === dirname(dir) && /^run-[a-z0-9._-]+$/i.test(basename(cohort))) {
    const build = join(cohort, 'candidate.json'),
      installed = join(cohort, 'role-grants.json');
    if (
      existsSync(build) &&
      existsSync(installed) &&
      read(build).inputs.sourceSHA256 === provenance.releaseSHA256
    )
      return read(installed);
  }
  const id = provenance?.pairedFixture;
  if (!/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(id ?? '')) return null;
  const pointer = join(dir, 'commands', 'public-fixtures', id + '.json');
  if (!existsSync(pointer)) return null;
  const source = read(pointer).directory;
  if (typeof source !== 'string' || dirname(source) !== join(dir, 'http', 'sandboxes')) return null;
  const snapshot = join(source, 'snapshot.json');
  if (!existsSync(snapshot)) return null;
  const actual = read(snapshot);
  return actual.candidateSHA256 === provenance.releaseSHA256 ? actual.appliedRoleSQL : null;
}

function transactionTests(label, command) {
  return (
    label === 'architecture-authority' &&
    command.provenance?.evidenceScope === 'domain-transaction-tests'
  );
}

export function bindPublicEvidence(dir, label, command) {
  if (command.status !== 'passed' || !outputs[label]) return;
  const installed = runningGrants(dir, command.provenance);
  if (!transactionTests(label, command) && !isDeepStrictEqual(installed, currentRoleSQL())) {
    command.processStatus = command.status;
    command.status = 'failed';
    command.failure = 'The running HTTP owners did not install the current role grants';
    return;
  }
  if (installed) command.roleSQL = installed;
  command.evidenceSHA256 = {};
  for (const file of outputs[label]) {
    const path = join(dir, file);
    const modified = existsSync(path) && statSync(path).mtimeMs;
    if (
      !modified ||
      modified < Date.parse(command.startedAt) - 2 ||
      modified > Date.parse(command.finishedAt) + 2
    ) {
      command.processStatus = 'passed';
      command.status = 'failed';
      command.failure = 'The command did not produce its expected receipt during this run: ' + file;
      return;
    }
    command.evidenceSHA256[file] = digest(readFileSync(path));
  }
}

export function currentRoleSQL() {
  return Object.fromEntries(
    postgresRoleSQLFiles.map((file) => [file, digest(readFileSync(join(root, file)))]),
  );
}

// The nine journeys on one writable pair share a signature. Every other
// command restores a fresh fixture and can be replayed independently.
function replayGroup(label) {
  if (!outputs[label]) throw Error('Unknown public replay: ' + label);
  return label.startsWith('catalog-') || label.startsWith('governance-') ||
    ['architecture-edge','architecture-governance-parity','architecture-display-files-parity'].includes(label)
    ? 'cohort' : label;
}

function replayFiles(paths, group) {
  const known = new Set(paths);
  // Fixtures build some asset paths from the format or the deployment role.
  // The original domain/browser regression drivers run as separate commands.
  const selected = new Set(paths.filter(name =>
    /^(?:samples|examples|contracts|scripts\/fixtures|tests\/http)\//.test(name) ||
    (name.startsWith('deploy/') && !name.endsWith('.md'))));
  const pending = ['scripts/architecture.mjs','scripts/architecture-parity-evidence.mjs',
    'scripts/architecture-lab.mjs','scripts/architecture-postgres.mjs',
    'scripts/architecture-fixture.go', ...(group === 'cohort'
      ? ['scripts/architecture-public-cohort.mjs','scripts/architecture-edge.mjs','scripts/architecture-governance-parity.mjs',
        'scripts/architecture-catalog-parity.mjs','scripts/architecture-display-files-parity.mjs']
      : ['scripts/' + group + '.mjs'])];
  const visited = new Set();
  while (pending.length) {
    const name = pending.pop();
    if (visited.has(name) || !known.has(name)) continue;
    visited.add(name);
    selected.add(name);
    // The top-level dispatcher also names every independent acceptance suite.
    // Its bytes and the command's actual child entry points are covered above.
    if (['scripts/architecture.mjs','scripts/architecture-parity-evidence.mjs'].includes(name) ||
        !existsSync(join(root, name))) continue;
    const source = readFileSync(join(root, name), 'utf8');
    for (const match of source.matchAll(/['"`]([\w./-]+\.(?:mjs|js|ts|py|go|sh|inc|txt|json|ya?ml|sql))['"`]/g)) {
      const ref = match[1];
      const child = known.has(ref) ? ref : normalize(join(dirname(name), ref));
      if (known.has(child) && !visited.has(child)) pending.push(child);
    }
  }
  return [...selected].sort();
}

export function publicReplayInputs(dir, label) {
  const group = replayGroup(label);
  const hash = createHash('sha256');
  const fixture = join(dir, 'snapshot', 'manifest.json');
  hash.update(group).update('\0').update(digest(readFileSync(fixture)));
  const files = execFileSync(
    'git',
    [
      'ls-files',
      '-z',
      '--cached',
      '--others',
      '--exclude-standard',
      '--',
      'scripts',
      'tests',
      'contracts',
      'deploy',
      'samples',
      'examples',
    ],
    { cwd: root, encoding: 'utf8' },
  );
  for (const name of replayFiles(files.split('\0').filter(Boolean), group)) {
    const path = join(root, name);
    hash
      .update(name)
      .update('\0')
      .update(existsSync(path) ? digest(readFileSync(path)) : 'deleted')
      .update('\0');
  }
  return hash.digest('hex');
}

export function currentPublicEvidence(dir, releaseSHA256) {
  const current = new Set();
  if (!existsSync(join(dir, 'snapshot/manifest.json'))) return current;
  const signatures = new Map();
  const grants = currentRoleSQL();
  for (const [label, paths] of Object.entries(outputs)) {
    const file = join(dir, 'commands', label + '.json');
    if (!existsSync(file)) continue;
    const cmd = read(file);
    if (cmd.status !== 'passed' || cmd.provenance?.releaseSHA256 !== releaseSHA256) continue;
    const group = replayGroup(label);
    if (!signatures.has(group)) signatures.set(group,publicReplayInputs(dir,label));
    const inputs = signatures.get(group);
    if (
      cmd.provenance.publicInputs !== inputs || cmd.publicInputsAtCompletion !== inputs ||
      (!transactionTests(label, cmd) &&
        (!isDeepStrictEqual(cmd.roleSQL, grants) ||
          !isDeepStrictEqual(runningGrants(dir, cmd.provenance), grants)))
    )
      continue;
    for (const path of paths) {
      if (
        cmd.evidenceSHA256?.[path] &&
        existsSync(join(dir, path)) &&
        digest(readFileSync(join(dir, path))) === cmd.evidenceSHA256[path]
      )
        current.add(path);
    }
  }
  return current;
}

export function observedPublicFixture(dir, file, report, release, recorded) {
  if (!release || !report) return false;
  if (commandOutputs.has(file) && !recorded.has(file)) return false;
  const current = (fixture) =>
    fixture?.candidateSHA256 === release &&
    isDeepStrictEqual(fixture.appliedRoleSQL, currentRoleSQL());
  if (report.fixture?.candidateSHA256) return current(report.fixture);
  const source = report.sourceRun ?? report.sandbox;
  if (typeof source !== 'string' || dirname(source) !== join(dir, 'http', 'sandboxes'))
    return recorded.has(file);
  const snapshot = join(source, 'snapshot.json');
  return existsSync(snapshot) && current(read(snapshot));
}
