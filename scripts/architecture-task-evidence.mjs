import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { digest, write } from './architecture-lab.mjs';

const stages = [
  ['business-sources', ['packages/go/platform/resources']],
  ['task-lease-and-cancellation', ['packages/go/platform/derivedmonitoring']],
  [
    'roles-and-scope',
    ['packages/go/platform/access', 'packages/go/platform/projects', 'packages/go/platform/tenancy'],
  ],
  [
    'task-index-and-http',
    [
      'packages/go/platform/taskrecord',
      'packages/go/platform/tasks',
      'packages/go/platform/notificationevent',
      'services/api/internal/transport/http',
    ],
  ],
  ['notification-restore', ['packages/go/platform/notifications']],
];

function observeTaskPhases(relative, command, source, fresh) {
  const commandText = command.toString('utf8');
  const packages = new Map();
  for (const line of commandText.split('\n')) {
    const entry = line.match(
      /^(ok|FAIL)\s+github\.com\/WenshuaiDev\/InfraPro\/(\S+)\s+(\d+(?:\.\d+)?)s(?:\s|$)/,
    );
    if (entry)
      packages.set(entry[2], {
        package: entry[2],
        seconds: Number(entry[3]),
        status: entry[1] === 'ok' ? 'passed' : 'failed',
      });
  }
  const cases = [
    ...source.toString('utf8').matchAll(/^--- (PASS|FAIL|SKIP): (\S+) \(([.\d]+)s\)/gm),
  ].map((m) => ({ name: m[2], status: m[1].toLowerCase(), seconds: Number(m[3]) }));
  const phaseEvidence = stages.map(([stage, expected]) => {
    const observed = expected.map((name) => packages.get(name)).filter(Boolean);
    let status =
      observed.length === 0
        ? 'unreached'
        : observed.length === expected.length && observed.every((p) => p.status === 'passed')
          ? 'passed'
          : 'failed';
    if (stage === 'business-sources' && (fresh || observed.length)) {
      if (cases.length === 0 || cases.some((v) => v.status !== 'pass') || observed.length === 0)
        status = 'failed';
      return {
        stage,
        status,
        packages: observed,
        cases,
        log: join(relative, 'sources.log'),
        sha256: digest(source),
      };
    }
    return { stage, status, packages: observed };
  });
  const clientCases = commandText
    .split('\n')
    .filter((line) => /^(?:[✔✖]\s|(?:not )?ok \d+ - )/.test(line));
  const tests = commandText.match(/^(?:ℹ|#) tests (\d+)$/m);
  const passed = commandText.match(/^(?:ℹ|#) pass (\d+)$/m);
  const failed = commandText.match(/^(?:ℹ|#) fail (\d+)$/m);
  const clientOK = !!(
    tests &&
    passed &&
    failed &&
    +tests[1] > 0 &&
    +tests[1] === +passed[1] &&
    +failed[1] === 0
  );
  phaseEvidence.push({
    stage: 'task-client',
    status: clientOK ? 'passed' : tests ? 'failed' : 'unreached',
    tests: tests ? Number(tests[1]) : null,
    passed: passed ? Number(passed[1]) : null,
    failed: failed ? Number(failed[1]) : null,
    cases: clientCases,
  });
  return phaseEvidence;
}

export function archiveTaskEvidence(directory, cwd, result) {
  const relative = join('tasks-functional', result.startedAt.replaceAll(':', '-'));
  const output = join(directory, relative);
  mkdirSync(output, { mode: 0o700, recursive: true });
  const command = readFileSync(join(directory, result.log));
  writeFileSync(join(output, 'command.log'), command, { mode: 0o600 });
  const sourceLog = join(cwd, '.artifacts/task-center/sources.log');
  const fresh =
    existsSync(sourceLog) && statSync(sourceLog).mtimeMs >= Date.parse(result.startedAt) - 1000;
  const source = fresh ? readFileSync(sourceLog) : Buffer.alloc(0);
  if (fresh) writeFileSync(join(output, 'sources.log'), source, { mode: 0o600 });
  const phaseEvidence = observeTaskPhases(relative, command, source, fresh);
  if (result.status === 'passed')
    assert(
      phaseEvidence.every((s) => s.status === 'passed'),
      'Original task regression lacks one of its six stage results',
    );
  const file = join(relative, 'phases.json');
  write(join(directory, file), {
    startedAt: result.startedAt,
    finishedAt: result.finishedAt,
    exitCode: result.exitCode,
    status: result.status,
    releaseSHA256: result.provenance?.releaseSHA256,
    sourceAtCompletion: result.sourceAtCompletion,
    originalCommand: result.command,
    commandLog: join(relative, 'command.log'),
    commandLogSHA256: digest(command),
    sourceSHA256: fresh ? digest(source) : null,
    originalDriverSHA256: digest(readFileSync(join(cwd, 'scripts/test-tasks.mjs'))),
    phases: phaseEvidence,
  });
  result.taskPhaseEvidence = { file, sha256: digest(readFileSync(join(directory, file))) };
  write(join(directory, 'test-tasks.json'), result);
  return phaseEvidence;
}

export function validTaskEvidence(directory, row) {
  try {
    const release = row.provenance?.releaseSHA256;
    if (
      !/^[a-f0-9]{64}$/.test(release) ||
      row.status !== 'passed' ||
      row.exitCode !== 0 ||
      !row.sourceAtCompletion?.matchesSelectedRelease ||
      row.sourceAtCompletion.sourceSHA256 !== release ||
      new Date(row.startedAt).toISOString() !== row.startedAt
    )
      return false;
    const relative = join('tasks-functional', row.startedAt.replaceAll(':', '-'));
    const evidence = row.taskPhaseEvidence;
    if (evidence?.file !== join(relative, 'phases.json') || row.log !== 'test-tasks.log') return false;
    const json = readFileSync(join(directory, evidence.file));
    if (digest(json) !== evidence.sha256) return false;
    const phases = JSON.parse(json);
    const command = readFileSync(join(directory, relative, 'command.log'));
    const source = readFileSync(join(directory, relative, 'sources.log'));
    const observed = observeTaskPhases(relative, command, source, true);
    return (
      phases.startedAt === row.startedAt &&
      phases.finishedAt === row.finishedAt &&
      phases.releaseSHA256 === release &&
      phases.exitCode === 0 &&
      phases.status === 'passed' &&
      isDeepStrictEqual(phases.sourceAtCompletion, row.sourceAtCompletion) &&
      isDeepStrictEqual(phases.originalCommand, row.command) &&
      phases.commandLog === join(relative, 'command.log') &&
      digest(command) === phases.commandLogSHA256 &&
      command.equals(readFileSync(join(directory, row.log))) &&
      command.includes(source) &&
      digest(source) === phases.sourceSHA256 &&
      observed.every((s) => s.status === 'passed') &&
      isDeepStrictEqual(observed, phases.phases)
    );
  } catch {
    return false;
  }
}
