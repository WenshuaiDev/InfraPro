import { spawn, spawnSync } from 'node:child_process';
import assert from 'node:assert/strict';
import {
  closeSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { ingestionStorage, stopIngestionStorage } from './fixtures/architecture-ingestion-storage.mjs';
import { startParser } from './fixtures/parsing-lab.mjs';
import { startRelay } from './fixtures/ingestion-relay.mjs';
import { testEnv } from './test-environment.mjs';
import { ingestionTestEnv } from './fixtures/ingestion-environment.mjs';
import { checkoutApplicationDigest } from './architecture-runtime.mjs';

const out = resolve(process.argv[2] || '.artifacts/architecture-refactor/run-20261118');
const baseline = JSON.parse(readFileSync(join(out, 'baseline.json')));
const scope = process.argv[3] || 'b';
const root = join(out, 'worker-checks');
mkdirSync(root, { recursive: true, mode: 0o700 });
const evaluated = join(out, 'candidate.evaluated.json');
const selected = process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE ||
  (existsSync(evaluated) ? JSON.parse(readFileSync(evaluated)).directory : out);
const manifest = join(resolve(selected), 'candidate.json');
const releaseSHA = existsSync(manifest) ? JSON.parse(readFileSync(manifest)).inputs.sourceSHA256 : null;

function sourceEvidence(side, requireMatch = true) {
  if (side === 'a') return { baseline_commit: baseline.commit, archive_sha256: baseline.source_archive_sha256 };
  const actual = checkoutApplicationDigest(process.cwd());
  if (releaseSHA && requireMatch) assert.equal(actual, releaseSHA, 'Worker checks must use the selected release source');
  return { source_sha256: actual, manifest, matches_release: !releaseSHA || actual === releaseSHA };
}
const common = [
  'TestArchitectureCustodyCrashBoundaries',
  'TestRealServiceAdoptionExecutionAndReplay',
  'TestIngestionMigrationPreservesV44ParsingEvidence',
  ...[
    'EndToEnd',
    'StorageIsolationRecoveryAndGovernance',
    'GatewayExactViewsAndAdoption',
    'MachineGrantAndLateResponseFence',
    'RecoveryTaskGenerationsRevocationAndAuditRollback',
    'AssociationWatermarkAndRestoredEpochIsolation',
    'SeparatedRawResultPermissionsAndIdentityResponse',
  ].map((s) => 'TestIngestion' + s),
];
const candidate = [
  'TestArchitectureMultipleDispatcherParserBudget',
  'TestArchitectureRecordedDispatchRace',
  'TestArchitectureAmbiguousParserReplyKeepsIdentity',
  'TestArchitectureBrokerAndPGRace',
  'TestArchitectureLocalParserReservation',
  'TestArchitectureResponseEvidenceRetry',
  'TestIngestionUnchangedRecoveryKeepsRevisionAndTaskEvents',
];

async function check(side) {
  const before = sourceEvidence(side);
  const wanted = side === 'a' ? common : [...common, ...candidate];
  const filter = process.env.ARC_WORKER_TESTS || '^(' + wanted.join('|') + ')$';
  const pattern = new RegExp(filter);
  const expected = wanted.filter((name) => pattern.test(name));
  assert(expected.length > 0, 'The worker test filter must select a registered functional test');
  const dir = mkdtempSync(join(root, side + '-'));
  const objects = await ingestionStorage(dir, 256);
  const source = side === 'a' ? baseline.source : process.cwd();
  let parser, relay;
  try {
    parser = await startParser(join(dir, 'parser'), { bin: baseline.bin, source: baseline.source });
    relay = await startRelay(join(dir, 'relay'), { binary: join(baseline.bin, 'ingest-relay') });
    const env = {
      ...process.env,
      ...testEnv,
      ...ingestionTestEnv,
      ...relay.env,
      INFRAPRO_INGESTION_TEST_S3: objects.endpoint,
      INFRAPRO_PARSER_TEST_PACKAGE: parser.pkg,
      INFRAPRO_PARSING_URL: parser.url,
      INFRAPRO_PARSING_TOKEN_FILE: parser.token,
      INFRAPRO_INGESTION_EVIDENCE: dir,
      INFRAPRO_TEST_REPO_ROOT: process.cwd(),
      INFRAPRO_ARCH_WORKER_CHECKS: '1',
    };
    const args = ['test', '-count=1', '-timeout=8m', '-json'];
    if (side === 'a') {
      // Only the common test harness and its test-only S3 address are overlaid.
      // Domain sources, original migrations and frozen machine binaries are retained.
      const pkg = 'packages/go/platform/parsing';
      const replacements = [
        'parsing_test.go',
        'ingestion_test.go',
        'ingestion_boundaries_test.go',
        'architecture_custody_test.go',
        'architecture_load_test.go',
      ];
      const overlay = join(dir, 'common-test-overlay.json');
      writeFileSync(
        overlay,
        JSON.stringify({
          Replace: Object.fromEntries(
            replacements.map((file) => [join(source, pkg, file), resolve(pkg, file)]),
          ),
        }),
      );
      args.push('-overlay', overlay);
    }
    args.push('./packages/go/platform/parsing', '-run', filter);
    const log = openSync(join(dir, 'go-test.jsonl'), 'w', 0o600);
    const start = Date.now();
    let exit;
    try {
      const child = spawn('go', args, { cwd: source, env, stdio: ['ignore', log, log] });
      exit = await new Promise((r) => child.on('exit', (code) => r(code ?? 1)));
    } finally {
      closeSync(log);
    }
    const lines = readFileSync(join(dir, 'go-test.jsonl'), 'utf8')
      .trim()
      .split('\n')
      .filter(Boolean)
      .map((line) => {
        try {
          return JSON.parse(line);
        } catch {
          return { Output: line + '\n' };
        }
      });
    writeFileSync(join(dir, 'domain.log'), lines.map((row) => row.Output || row.BuildOutput || '').join(''), {
      mode: 0o600,
    });
    const cases = lines
      .filter((row) => row.Test && ['pass', 'skip', 'fail'].includes(row.Action))
      .map((row) => ({ name: row.Test, state: row.Action, seconds: row.Elapsed }));
    const after = sourceEvidence(side, false);
    const passed =
      (side === 'a' || (after.matches_release && before.source_sha256 === after.source_sha256)) &&
      exit === 0 &&
      expected.length > 0 &&
      expected.every((name) => cases.some((c) => c.name === name && c.state === 'pass')) &&
      cases.every((c) => c.state === 'pass');
    const result = {
      passed,
      side,
      source_before: before,
      source_after: after,
      started_at: new Date(start),
      duration_ms: Date.now() - start,
      command: ['go', ...args],
      objects: objects.container,
      relative_directory: relative(out, dir),
      cases,
      exit_code: exit,
    };
    writeFileSync(join(dir, 'result.json'), JSON.stringify(result, null, 2) + '\n', { mode: 0o600 });
    console.log(result.relative_directory + ' ' + (passed ? 'passed' : 'failed'));
    if (!passed) process.exitCode = 1;
  } finally {
    try {
      await Promise.all([relay?.close(), parser?.close()]);
    } finally {
      stopIngestionStorage(objects);
    }
  }
}
async function domain(side) {
  const before = sourceEvidence(side);
  const source = side === 'a' ? baseline.source : process.cwd();
  const dir = mkdtempSync(join(root, side + '-domain-'));
  const wanted = [
    'TestCalculationReservedQueuesAndTenantFairness',
    'TestCalculationSharedTaskLeaseFenceAndCancellation',
    'TestAvatarLifecycleAuthorizationAndRecovery',
    'TestExpiredClaimFencingPoisonIsolationAndReplay',
    'TestExpiryMaintenancePersistsOnceWithoutOpeningInvitation',
    'TestFilesRegisteredCleanupRespectsHistoryReadAndBackupHold',
    'TestFilesCleanupRechecksIndependentReadAndLateConsumerProtection',
    'TestMonitoringDailyVerificationAndUnrepairableSourceQuarantine',
    'TestDerivedClockDueRestartAndEmptyCount',
    'TestDerivedTrialUsesRealSnapshotAndSharedTask',
    'TestDerivedBackgroundTrialPartitionsAndResumesCompleteSnapshot',
  ];
  if (side === 'b')
    wanted.push(
      'TestArchitectureEligibleTaskOwnersAndIdleSchedulers',
      'TestArchitectureTransactionLimitsAndReuse',
    );
  const args = [
    'test',
    '-tags',
    'monitoringintegration',
    '-p=1',
    '-count=1',
    '-timeout=12m',
    '-json',
    ...[
      'monitoring',
      'derivedmonitoring',
      'identity',
      'notifications',
      'resources',
      ...(side === 'b' ? ['access'] : []),
    ].map((pkg) => './packages/go/platform/' + pkg),
    '-run',
    '^(' + wanted.join('|') + ')$',
  ];
  const log = openSync(join(dir, 'go-test.jsonl'), 'w', 0o600),
    start = Date.now();
  let exit;
  try {
    const child = spawn('go', args, {
      cwd: source,
      env: { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1' },
      stdio: ['ignore', log, log],
    });
    exit = await new Promise((r) => child.on('exit', (code) => r(code ?? 1)));
  } finally {
    closeSync(log);
  }
  const lines = readFileSync(join(dir, 'go-test.jsonl'), 'utf8')
    .trim()
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
  const cases = lines
    .filter((row) => row.Test && ['pass', 'skip', 'fail'].includes(row.Action))
    .map((row) => ({ name: row.Test, state: row.Action, seconds: row.Elapsed }));
  const after = sourceEvidence(side, false);
  const passed =
    (side === 'a' || (after.matches_release && before.source_sha256 === after.source_sha256)) &&
    exit === 0 &&
    wanted.every((name) => cases.some((c) => c.name === name && c.state === 'pass')) &&
    cases.every((c) => c.state === 'pass');
  writeFileSync(join(dir, 'domain.log'), lines.map((row) => row.Output || row.BuildOutput || '').join(''), {
    mode: 0o600,
  });
  writeFileSync(
    join(dir, 'result.json'),
    JSON.stringify(
      {
        side,
        passed,
        source_before: before,
        source_after: after,
        cases,
        command: ['go', ...args],
        started_at: new Date(start),
        duration_ms: Date.now() - start,
        exit_code: exit,
      },
      null,
      2,
    ) + '\n',
    { mode: 0o600 },
  );
  console.log(relative(out, dir) + ' ' + (passed ? 'passed' : 'failed'));
  if (!passed) process.exitCode = 1;
}
function summary() {
  const records = [];
  for (const item of readdirSync(root, { withFileTypes: true })) {
    if (!item.isDirectory()) continue;
    const dir = join(root, item.name);
    try {
      const file = join(dir, 'result.json');
      const result = JSON.parse(readFileSync(file));
      if (!result.side || !Array.isArray(result.cases)) continue;
      if (result.side === 'b' && releaseSHA &&
          (result.source_before?.source_sha256 !== releaseSHA || result.source_after?.source_sha256 !== releaseSHA)) continue;
      if (result.side === 'a' && ![result.source_before, result.source_after].every(
        (source) => source?.baseline_commit === baseline.commit && source?.archive_sha256 === baseline.source_archive_sha256,
      )) continue;
      const evidence = relative(out, join(dir, 'go-test.jsonl'));
      const finished = Date.parse(result.started_at) + result.duration_ms;
      const at = Number.isFinite(finished) ? finished : statSync(file).mtimeMs;
      for (const c of result.cases) records.push({ side: result.side, name: c.name, state: c.state, at, evidence });
    } catch {
      /* A previous diagnostics run may not have a structured result. */
    }
  }
  for (const name of releaseSHA ? [] : ['domain', 'idle-maintenance']) {
    const file = join(root, name + '.log');
    let log;
    try {
      log = readFileSync(file, 'utf8');
    } catch {
      continue;
    }
    const at = statSync(file).mtimeMs;
    for (const m of log.matchAll(/^\s*--- (PASS|FAIL|SKIP): ([^ ]+) /gm))
      records.push({ side: 'b', name: m[2], state: m[1].toLowerCase(), at, evidence: relative(out, file) });
  }
  records.sort((a, b) => a.at - b.at);
  const criteria = {};
  const check = (id, groups) => {
    const checks = groups.flatMap(([sides, tests]) =>
      sides.flatMap((side) =>
        tests.map((name) => {
          const matched = records.findLast((row) => row.side === side && row.name === name);
          return { side, name, passed: matched?.state === 'pass', evidence: matched?.evidence || null };
        }),
      ),
    );
    criteria[id] = {
      passed: checks.every((c) => c.passed),
      checks,
      evidence: [...new Set(checks.map((c) => c.evidence).filter(Boolean))],
    };
  };
  const paired = ['a', 'b'],
    split = ['b'];
  check('ARC-018', [
    [
      paired,
      [
        'TestIngestionEndToEnd',
        'TestIngestionAssociationWatermarkAndRestoredEpochIsolation',
        'TestIngestionStorageIsolationRecoveryAndGovernance',
      ],
    ],
  ]);
  check('ARC-019', [
    [
      paired,
      ['pre-write', 'failed-read', 'pre-commit', 'committed-before-confirmation'].map(
        (s) => 'TestArchitectureCustodyCrashBoundaries/' + s,
      ),
    ],
  ]);
  check('ARC-020', [
    [
      paired,
      [
        'TestRealServiceAdoptionExecutionAndReplay',
        'TestIngestionMigrationPreservesV44ParsingEvidence',
        'TestIngestionGatewayExactViewsAndAdoption',
        'TestIngestionRecoveryTaskGenerationsRevocationAndAuditRollback',
        'TestIngestionSeparatedRawResultPermissionsAndIdentityResponse',
      ],
    ],
    [
      split,
      [
        'TestArchitectureMultipleDispatcherParserBudget',
        'TestArchitectureAmbiguousParserReplyKeepsIdentity',
        'TestArchitectureLocalParserReservation',
        'TestArchitectureResponseEvidenceRetry',
      ],
    ],
  ]);
  check('ARC-021', [
    [split, ['TestArchitectureEligibleTaskOwnersAndIdleSchedulers']],
    [
      paired,
      [
        'TestCalculationReservedQueuesAndTenantFairness',
        'TestCalculationSharedTaskLeaseFenceAndCancellation',
        'TestDerivedTrialUsesRealSnapshotAndSharedTask',
        'TestDerivedBackgroundTrialPartitionsAndResumesCompleteSnapshot',
      ],
    ],
  ]);
  check('ARC-026', [
    [
      paired,
      [
        'TestIngestionMachineGrantAndLateResponseFence',
        'TestIngestionRecoveryTaskGenerationsRevocationAndAuditRollback',
      ],
    ],
    [
      split,
      [
        'TestArchitectureRecordedDispatchRace',
        'TestArchitectureAmbiguousParserReplyKeepsIdentity',
        'TestArchitectureBrokerAndPGRace',
        'TestArchitectureResponseEvidenceRetry',
        'TestIngestionUnchangedRecoveryKeepsRevisionAndTaskEvents',
      ],
    ],
  ]);
  check('ARC-028', [
    [split, ['TestArchitectureEligibleTaskOwnersAndIdleSchedulers']],
    [
      paired,
      [
        'TestDerivedClockDueRestartAndEmptyCount',
        'TestAvatarLifecycleAuthorizationAndRecovery',
        'TestExpiredClaimFencingPoisonIsolationAndReplay',
        'TestExpiryMaintenancePersistsOnceWithoutOpeningInvitation',
        'TestFilesRegisteredCleanupRespectsHistoryReadAndBackupHold',
        'TestFilesCleanupRechecksIndependentReadAndLateConsumerProtection',
        'TestMonitoringDailyVerificationAndUnrepairableSourceQuarantine',
      ],
    ],
  ]);
  const result = { passed: Object.values(criteria).every((c) => c.passed), baseline_commit: baseline.commit,
    candidate_source_sha256: releaseSHA, criteria };
  const destination = join(root, 'summary.json');
  if (existsSync(destination)) {
    const previous = JSON.parse(readFileSync(destination));
    if (previous.candidate_source_sha256 !== releaseSHA) {
      const historical = join(root, 'summary-' + (previous.candidate_source_sha256 || 'unversioned') + '.json');
      if (!existsSync(historical)) writeFileSync(historical, JSON.stringify(previous, null, 2) + '\n', { mode: 0o600 });
    }
  }
  writeFileSync(destination, JSON.stringify(result, null, 2) + '\n', { mode: 0o600 });
  console.log(Object.fromEntries(Object.entries(criteria).map(([k, v]) => [k, v.passed])));
  return result;
}
if (scope === 'authority') {
  const trial = process.argv[4];
  if (!/^(pg|mq)-[a-z0-9]+$/.test(trial || '')) throw Error('A retained candidate round is required');
  const cfg = JSON.parse(readFileSync(join(out, 'ingestion', trial, 'private/service.json')));
  const run = spawnSync(
    'go',
    [
      'test',
      '-p=1',
      '-count=1',
      '-json',
      '-run',
      '^TestArchitectureRetainedWorkerAuthority$',
      './packages/go/platform/ingestion',
    ],
    {
      env: { ...process.env, GOMAXPROCS: '2', INFRAPRO_ARCH_AUTHORITY_FIXTURE: cfg.database_url },
      encoding: 'utf8',
    },
  );
  writeFileSync(join(root, 'retained-authority-' + trial + '.jsonl'), run.stdout + run.stderr, {
    mode: 0o600,
  });
  if (
    run.status !== 0 ||
    !run.stdout
      .split('\n')
      .filter(Boolean)
      .some((line) => {
        const row = JSON.parse(line);
        return row.Test === 'TestArchitectureRetainedWorkerAuthority' && row.Action === 'pass';
      })
  )
    process.exitCode = 1;
} else if (scope === 'summary') {
  if (!summary().passed) process.exitCode = 1;
} else if (scope === 'all') {
  for (const side of ['a', 'b']) {
    await check(side);
    await domain(side);
  }
  if (!summary().passed) process.exitCode = 1;
} else if (['domain', 'domain-a', 'domain-b'].includes(scope)) {
  const sides = scope === 'domain' ? ['a', 'b'] : [scope.slice(-1)];
  for (const side of sides) await domain(side);
  summary();
} else if (['a', 'b'].includes(scope)) {
  await check(scope);
  summary();
} else throw Error('Choose a, b, domain, domain-a, domain-b, all, authority or summary');
