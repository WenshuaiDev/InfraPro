import { execFileSync, spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import {
  openSync,
  closeSync,
  mkdirSync,
  existsSync,
  renameSync,
  fstatSync,
  readSync,
  cpSync,
  readFileSync,
  readdirSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { regressionInfra, stopRegressionStorage } from './architecture-regression-infra.mjs';
import { httpImageBuilds, isolatedHTTPNetwork } from './architecture-http-network.mjs';
import { archiveHTTPSEvidence, validHTTPSEvidence } from './architecture-https-evidence.mjs';
import { checkoutApplicationDigest } from './architecture-runtime.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { archiveRestoreEvidence, validRestoreEvidence } from './architecture-restore-evidence.mjs';
import { archiveTaskEvidence, validTaskEvidence } from './architecture-task-evidence.mjs';
import { archiveIdentityRegressionEvidence, validIdentityRegressionEvidence } from './architecture-identity-regression-evidence.mjs';
import { monitoringRecoveryEvidence } from './architecture-monitoring-restart.mjs';
import { membershipPlanner } from './architecture-display-boundary-setup.mjs';
import { membershipRow } from './architecture-membership-result.mjs';
import { performanceCommands, performanceStages, registerFunctionalScope, skippedOriginal, skipReason } from './architecture-acceptance-scope.mjs';
import { prepareFrozenBuild } from './architecture-frozen-build.mjs';
import { archiveOriginalIngestionLoad } from './architecture-original-ingestion-evidence.mjs';
import {
  archiveOriginalLoadEvidence,
  hasOriginalLoadEvidence,
} from './architecture-original-load-evidence.mjs';
import {
  archiveDiagnosticsEvidence,
  archiveFrozenDiagnosticsBrowser,
  archiveOriginalDiagnostics,
  diagnosticsRegressionEnvironment,
  parkedDiagnosticsLab,
  diagnosticFrameClock,
  recordDiagnosticsFixture,
} from './architecture-diagnostics-harness.mjs';
import {
  digest,
  fileLease,
  root,
  runDirectory,
  read,
  safeDiagnosticLine,
  write,
} from './architecture-lab.mjs';

export const checks = [
  'check',
  'build',
  'contracts:check',
  'permissions:check',
  'test:e2e:preview',
  'test:http',
  'test:https',
  'test:identity',
  'test:mfa',
  'test:parsing',
  'test:parsing:regression',
  'test:ingestion',
  'test:ingestion:load',
  'test:monitoring:contract',
  'test:monitoring',
  'test:monitoring:client',
  'test:derived',
  'test:derived:contract',
  'test:derived:database',
  'test:derived:load',
  'test:derived:pipeline',
  'test:derived:soak',
  'test:spatial',
  'test:display',
  'test:display:plans',
  'test:display:load',
  'test:display:compatibility',
  'test:uploads',
  'test:files',
  'test:files:http',
  'test:files:client',
  'test:files:sandbox',
  'test:tasks',
  'test:notifications',
  'test:notifications:client',
  'test:notifications:domains',
  'test:audit',
  'test:audit:regression',
  'test:diagnostics',
  'test:monitoring:restore',
  'test:files:restore',
  'test:display:restore',
  'test:notifications:restore',
  'test:audit:restore',
];

const originalStorageGroups = {
  'test:https': 'https',
  'test:e2e:preview': 'https',
  'test:monitoring': 'monitoring',
  'test:monitoring:contract': 'monitoring',
  'test:derived:contract': 'monitoring',
  'test:spatial': 'monitoring',
  'test:display': 'monitoring',
  'test:display:plans': 'monitoring',
  'test:display:load': 'monitoring',
  'test:derived:database': 'derived-database',
  'test:files': 'files',
  'test:files:http': 'files',
  'test:uploads': 'files',
};

function originalStorageGroup(name, role, soak) {
  if (soak === 'full') return 'soak';
  if (name.endsWith(':restore')) return 'restore';
  if (name === 'test:identity' && role === 'candidate') return 'identity';
  return originalStorageGroups[name] ?? 'general';
}

export function derivedSoakMode(dir, requested) {
  const file = join(dir, 'regression', 'derived-soak-profile.json');
  const previous = existsSync(file) ? read(file).mode : undefined;
  const mode = requested ?? previous ?? 'full';
  if (!['full', 'smoke'].includes(mode)) throw Error('Expected derived soak mode full or smoke');
  if (requested && requested !== previous)
    write(file, { mode, selectedAt: new Date().toISOString() });
  return mode;
}
function failureSummary(path) {
  const fd = openSync(path, 'r');
  try {
    const size = fstatSync(fd).size,
      length = Math.min(size, 512 * 1024),
      chunk = Buffer.alloc(length);
    readSync(fd, chunk, 0, length, size - length);
    const lines = chunk.toString().split('\n');
    const failedGo = lines.findIndex((line) => /^\s*--- FAIL: Test\S+/.test(line));
    if (failedGo >= 0) {
      const earlier = lines.slice(0, failedGo);
      const lastTest = earlier.findLastIndex((line) => /^=== (?:RUN|CONT)\s/.test(line));
      const location = earlier
        .slice(lastTest < 0 ? failedGo : lastTest + 1)
        .findLast((line) => /^\s+[^\n]+\.go:\d+:/.test(line));
      const timeout = lines.find((line) => /^panic: test timed out after /.test(line));
      return safeDiagnosticLine(
        [lines[failedGo].trim(), location?.trim(), timeout].filter(Boolean).join(' / '),
      ).slice(0, 300);
    }
    const timeout = lines.find((line) => /^panic: test timed out after /.test(line));
    if (timeout) return safeDiagnosticLine(timeout).slice(0, 300);
    const match = lines.find((line) =>
      /^--- FAIL: |^ {2}\d+\) |Error: |protocol error|failed to solve:|^not ok |: error /.test(
        line,
      ),
    );
    if (!match) return 'Process exited unsuccessfully; inspect the private command log';
    return safeDiagnosticLine(match).slice(0, 300);
  } finally {
    closeSync(fd);
  }
}
export async function recordCommand(directory, label, command, args, options = {}) {
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const log = join(directory, label + '.log');
  const destination = join(directory, label + '.json');
  const start = Date.now();
  if (existsSync(destination)) {
    const previous = read(destination),
      attempt = join(
        directory,
        'attempts',
        label + '-' + (previous.startedAt || String(start)).replaceAll(':', '-'),
      );
    mkdirSync(attempt, { recursive: true, mode: 0o700 });
    if (existsSync(log)) renameSync(log, join(attempt, label + '.log'));
    renameSync(destination, join(attempt, label + '.json'));
  }
  const result = {
    command: [command, ...args],
    cwd: options.cwd || root,
    ...(options.provenance && { provenance: options.provenance }),
    startedAt: new Date(start).toISOString(),
    log: label + '.log',
    status: 'running',
  };
  write(destination, result);
  const fd = openSync(log, 'w', 0o600);
  try {
    const child = spawn(command, args, {
      cwd: result.cwd,
      env: options.env || process.env,
      stdio: ['ignore', fd, fd],
    });
    const code = await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', (code, signal) => {
        result.signal = signal;
        resolve(code);
      });
    });
    Object.assign(result, {
      status: code === 0 ? 'passed' : 'failed',
      exitCode: code,
    });
  } catch (error) {
    Object.assign(result, {
      status: 'failed',
      exitCode: null,
      error: error.code || error.name,
    });
  } finally {
    closeSync(fd);
    if (result.status === 'failed') result.failure = failureSummary(log);
    result.finishedAt = new Date().toISOString();
    result.durationMs = Date.now() - start;
    write(destination, result);
  }
  return result;
}
function baselineHTTPHarness(dir, cwd) {
  const target = join(dir, 'private', 'baseline-http-browser'),
    test = 'tests/http/engineering.spec.ts';
  mkdirSync(target, { recursive: true, mode: 0o700 });
  cpSync(join(cwd, 'tests'), join(target, 'tests'), { recursive: true });
  const before = readFileSync(join(cwd, test), 'utf8'),
    current = readFileSync(join(root, test), 'utf8');
  const heading = "getByRole('heading', { name: 'CSP 动态新类型', exact: true })";
  const patched = before
    .replace(heading, "getByRole('heading', { level: 1, name: 'CSP 动态新类型', exact: true })")
    .replace(
      "await page.getByRole('tab', { name: '样例预览' }).click();",
      "await page.getByRole('button', { name: '版本与管理' }).click(); await page.getByRole('button', { name: '更多操作' }).click(); await page.getByRole('button', { name: '试验建档样例' }).click();",
    );
  if (before === patched || patched !== current)
    throw Error('Frozen HTTP test has unexpected changes');
  writeFileSync(join(target, test), patched);
  for (const entry of ['packages', 'node_modules'])
    if (!existsSync(join(target, entry))) symlinkSync(join(cwd, entry), join(target, entry), 'dir');
  cpSync(join(cwd, 'playwright.http.config.ts'), join(target, 'playwright.http.config.ts'));
  const helper = join(target, 'redirect.mjs');
  writeFileSync(
    helper,
    "import {registerHooks} from 'node:module';import {fileURLToPath} from 'node:url';" +
      "registerHooks({load(url,context,next){const loaded=next(url,context);if(url.startsWith('file:')&&fileURLToPath(url)===" +
      JSON.stringify(join(cwd, 'scripts/test-http.mjs')) +
      '){return {...loaded,source:Buffer.from(loaded.source).toString().replace("\'playwright.http.config.ts\'",' +
      JSON.stringify(JSON.stringify(join(target, 'playwright.http.config.ts'))) +
      ')};}return loaded;}});\n',
  );
  write(join(target, 'harness.json'), {
    test,
    frozenSHA256: digest(before),
    sharedSHA256: digest(patched),
    reason:
      'Open the published page heading and the existing Versions / More / Sample controls. Both versions receive the same deep link, invalid submission, valid sample, CSP and browser assertions.',
  });
  return '--import=' + helper;
}
const sharedOriginalBrowserFixtures = [
  {
    file: 'tests/e2e/project-governance.spec.ts',
    original: '0dbaa3002f0e26fa39167b5f4c371c3adf2953e356495cb8ecec5875c4b15d27',
    shared: '69d9921dd4930a43e57cc771a63a6bf3dfc412ea4eef193525186824894eb239',
    reason: 'Exercise numbered pages, all 21 real project IDs and retry on the published governance screen.',
  },
  {
    file: 'tests/e2e/binding-workspace.spec.ts',
    original: 'aba16b6c76a7c6a8855bd60401cfbf8327cda222524ecb73c5bf52d0d9d34691',
    shared: '508b3ac90f5a3530fc7db062f7e0926ec9b431d9a80df03523db2cde92ababb7',
    reason: 'Match the selected channel source by its origin, path and empty query before checking its full label.',
  },
  {
    file: 'tests/e2e/monitoring-data.spec.ts',
    original: 'fd925c77d8e9d4ac5f5df3b713d65d5cbbac6cbe8cf1a1dcb9cc2b633da28698',
    shared: 'ca2b543f8ad07113b6e7964f658d6d7c0129089b9b9614354c8d8e0315c12073',
    reason: 'Follow the published task to its full export page and wait for the next cursor page.',
  },
  {
    file: 'tests/e2e/workspace-return.spec.ts',
    original: '92b637ac60aeacb45b3025f373d9289cec8ccb4c49d9bc284ffbbb3731f27845',
    shared: 'cd0ad5311f248bb06757ba85a514a0d6ffb375e2b38d99b70f4256740293c2bc',
    reason: 'Open notification preferences inside the notification dialog opened by the preceding action.',
  },
  {
    file: 'tests/e2e/tenancy.spec.ts',
    original: 'f4677481840a63cf52f2297b4ab9a6882b7551c800a69031545f2ff786a71d5f',
    shared: '9c4e593029d8bb7422aad4ce73eeb74440a30023edee8754a253500756a1cec1',
    reason: 'Verify the frozen six-field candidate with its two fixed empty avatar values.',
  },
  {
    file: 'tests/e2e/account.spec.ts',
    original: '9d34a0807ed7e4ea227290339a4eb3f393b8063522ad1029b6a845efa664df4d',
    shared: '2fb245fc612168d26ea024c261a9d5d54456ac797775e67ba10ea3a7cdcc7fa8',
    reason: 'Wait for the invitation inspection before asserting the recipient form.',
  },
];

function baselineBrowserConfig(dir, cwd) {
  // Test discovery and the preview command must import the same installation
  // of Playwright, even on a fresh extraction of the frozen release.
  const target = join(cwd, '.artifacts', 'architecture-browser');
  mkdirSync(target, { recursive: true, mode: 0o700 });
  cpSync(join(cwd, 'tests'), join(target, 'tests'), { recursive: true });
  for (const name of ['packages', 'apps', 'contracts', 'node_modules'])
    if (!existsSync(join(target, name))) symlinkSync(join(cwd, name), join(target, name), 'dir');
  const file = 'tests/e2e/accounts-page.spec.ts',
    before = readFileSync(join(cwd, file)),
    after = readFileSync(join(root, file));
  // One original helper sent "^基础成员" to a literal text search. Keep the
  // unchanged old-version assertion and send its actual display text.
  const patched = before
    .toString()
    .replace(
      'const option = async (name: RegExp)',
      'const option = async (search: string, name: RegExp)',
    )
    .replace('.fill(name.source);', '.fill(search);')
    .replaceAll('option(new RegExp(role.name))', 'option(role.name, new RegExp(role.name))')
    .replace('option(/^基础成员/)', "option('基础成员', /^基础成员/)");
  if (patched !== after.toString())
    throw Error(
      'Frozen and candidate account acceptance assertions differ beyond the literal search correction',
    );
  writeFileSync(join(target, file), patched, { mode: 0o600 });
  const derived = 'tests/e2e/derived-monitoring.spec.ts';
  const frozenDerived = readFileSync(join(cwd, derived), 'utf8');
  const link =
    " await expect(page.getByRole('link',{name:'查看派生计算',exact:true})).toBeVisible();";
  const correctedDerived = frozenDerived
    .replace(
      link,
      " await expect(page.getByRole('article',{name:'当前资源详情'})).toContainText(longName,{timeout:30000});\n" +
        link,
    )
    .replace(
      '输出范围：2020-01-01T00:01:00.000000000Z 至 2020-01-01T00:02:00.000000000Z（终点不含）',
      '输出范围：2020/01/01 08:01:00 至 2020/01/01 08:02:00（终点不含）',
    );
  if (
    correctedDerived === frozenDerived ||
    correctedDerived !== readFileSync(join(root, derived), 'utf8')
  )
    throw Error('Unexpected changes to the frozen derived browser journeys');
  writeFileSync(join(target, derived), correctedDerived, { mode: 0o600 });
  for (const fixture of sharedOriginalBrowserFixtures) {
    const frozen = readFileSync(join(cwd, fixture.file));
    const shared = readFileSync(join(root, fixture.file));
    if (digest(frozen) !== fixture.original || digest(shared) !== fixture.shared)
      throw Error('Unexpected browser fixture changes in ' + fixture.file);
    writeFileSync(join(target, fixture.file), shared, { mode: 0o600 });
  }
  const config = join(target, 'playwright.config.ts');
  const serialize = JSON.stringify;
  writeFileSync(
    config,
    'import release from ' +
      serialize(join(cwd, 'playwright.config.ts')) +
      ';\n' +
      'export default {...release,testDir:' +
      serialize(join(target, 'tests/e2e')) +
      ',outputDir:' +
      serialize(join(dir, 'regression/baseline/browser-output')) +
      ',webServer:(Array.isArray(release.webServer)?release.webServer:[release.webServer]).map(server=>({...server,cwd:' +
      serialize(cwd) +
      '}))};\n',
    { mode: 0o600 },
  );
  write(join(dir, 'regression/baseline/browser-harness.json'), {
    source: read(join(dir, 'baseline.json')).commit,
    changed: [
      ...sharedOriginalBrowserFixtures,
      {
        file,
        original: digest(before),
        shared: digest(after),
        reason: 'Use literal search text before checking the same matching role checkbox',
      },
      {
        file: derived,
        original: digest(frozenDerived),
        shared: digest(correctedDerived),
        reason:
          'Wait for the named channel to load; verify the same exclusive UTC interval in its published Asia/Shanghai display format',
      },
    ],
  });
  return config;
}
function browserConfigFor(directory, role, cwd) {
  const base = role === 'baseline' ? baselineBrowserConfig(directory, cwd) : join(cwd, 'playwright.config.ts');
  const shared = sharedConsoleFor(directory, { verifyFiles: true });
  if (!shared) return role === 'baseline' ? base : null;
  // The original preview command still starts its own real backend.
  // Both of its console servers read the same sealed Vite output directory.
  const out = join(cwd, '.artifacts', 'architecture-shared-browser', role);
  mkdirSync(out, { recursive: true, mode: 0o700 });
  const file = join(out, 'playwright.config.ts');
  const testDirectory = role === 'baseline' ? join(dirname(base), 'tests/e2e') : join(cwd, 'tests/e2e');
  writeFileSync(file, [
    'import original from ' + JSON.stringify(base) + ';',
    'const servers = Array.isArray(original.webServer) ? original.webServer : [original.webServer];',
    'const consoleCommand = "node scripts/e2e-console.mjs";',
    'if (servers.filter(server => server?.command === consoleCommand).length !== 1) throw Error("Expected one console server");',
    'export default { ...original, testDir: ' + JSON.stringify(testDirectory) + ',',
    '  outputDir: ' + JSON.stringify(join(directory, 'regression', role, 'browser-output')) + ',',
    '  webServer: servers.map(server => ({ ...server, stdout: "pipe", stderr: "pipe", cwd: ' + JSON.stringify(cwd) + ',',
    '    env: { ...server.env, ...(server.command === consoleCommand && { INFRAPRO_E2E_PREVIEW_DIR: ' +
      JSON.stringify(shared.console.directory) + ' }) } })),',
    '};',
  ].join('\n'), { mode: 0o600 });
  return file;
}
function browserSignature(cwd, role, releaseSHA256, sharedConsoleSHA256) {
  const paths = [
    'playwright.config.ts',
    'scripts/e2e.mjs',
    'scripts/e2e-server.mjs',
    'scripts/e2e-console.mjs',
    'scripts/test-environment.mjs',
  ];
  for (const folder of ['tests/e2e', 'tests/helpers']) {
    for (const entry of readdirSync(join(cwd, folder), {
      withFileTypes: true,
      recursive: true,
    })) {
      if (entry.isFile()) paths.push(relative(cwd, join(entry.parentPath, entry.name)));
    }
  }
  // Each verified fixture is identical in the two original browser replays.
  const corrected = new Set([
    ...sharedOriginalBrowserFixtures.map((fixture) => fixture.file),
    'tests/e2e/accounts-page.spec.ts',
    'tests/e2e/derived-monitoring.spec.ts',
  ]);
  const inputs = paths
    .sort()
    .map((file) => [
      file,
      digest(readFileSync(join(role === 'baseline' && corrected.has(file) ? root : cwd, file))),
    ]);
  for (const file of [
    'scripts/architecture-browser-reporter.mjs',
    'scripts/architecture-domain-http.mjs',
    'scripts/fixtures/architecture-http-recorder.go.txt',
    'scripts/fixtures/architecture-test-owners.go.txt',
  ]) {
    inputs.push([file, digest(readFileSync(join(root, file)))]);
  }
  return digest(JSON.stringify(sharedConsoleSHA256 ? [releaseSHA256, inputs, sharedConsoleSHA256] : [releaseSHA256, inputs]));
}
// A shard is complete only after the original Playwright process exits and
// its declared tests agree with the observed outcomes. This permits real
// benchmark windows between independent browser files without losing work.
async function browserRegression(directory, cwd, options, reportArgs, signature, release) {
  const folder = join(directory, 'browser-shards'),
    shards = 24;
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const operatorDirectory = join(dirname(dirname(directory)), 'private', 'browser-operators', options.provenance.role);
  mkdirSync(operatorDirectory, { recursive: true, mode: 0o700 });
  const reporter = join(root, 'scripts/architecture-browser-reporter.mjs');
  const configIndex = reportArgs.indexOf('--config');
  const config = configIndex < 0 ? [] : ['--config', reportArgs[configIndex + 1]];
  const inventoryFile = join(folder, 'all-' + signature + '.json');
  if (!existsSync(inventoryFile) || !read(inventoryFile).length) {
    const listing = await recordCommand(folder, 'inventory-' + signature, 'pnpm',
      ['exec', 'playwright', 'test', ...config, '--list', '--reporter', 'list,' + reporter],
      { cwd, env: { ...options.env, ARC_BROWSER_INVENTORY_FILE: inventoryFile }, provenance: options.provenance });
    if (listing.status !== 'passed') throw Error('Playwright discovery failed; see ' + join(folder, listing.log));
  }
  const expected = read(inventoryFile).map((test) => test.id);
  assertUnique(expected);
  const first = Number(process.env.ARC_BROWSER_ONLY_SHARD || 1),
    last = process.env.ARC_BROWSER_ONLY_SHARD ? first : shards;
  if (!Number.isInteger(first) || first < 1 || first > shards)
    throw Error('Invalid original-browser shard');
  const started = new Date().toISOString(),
    command = ['pnpm', 'test:e2e:preview'],
    allFile = join(directory, 'test-e2e-preview.json');
  const priorSummary = existsSync(allFile) ? read(allFile) : null;
  if (priorSummary?.shards && priorSummary.signature && priorSummary.signature !== signature) {
    const archive = join(directory, 'browser-history', priorSummary.signature,
      (priorSummary.finishedAt || priorSummary.startedAt).replaceAll(':', '-'));
    const originalShards = join(archive, 'browser-shards');
    mkdirSync(originalShards, { recursive: true, mode: 0o700 });
    for (let n = 1; n <= priorSummary.shards; n++) {
      const label = 'shard-' + n;
      const result = join(folder, label + '.json');
      if (!existsSync(result) || read(result).signature !== priorSummary.signature) continue;
      for (const suffix of ['.json', '.log', '-cases.json', '-outcomes.jsonl', '-html']) {
        const file = join(folder, label + suffix);
        if (existsSync(file)) renameSync(file, join(originalShards, label + suffix));
      }
    }
    const priorInventory = 'all-' + priorSummary.signature + '.json';
    if (existsSync(join(folder, priorInventory)))
      cpSync(join(folder, priorInventory), join(originalShards, priorInventory));
    renameSync(allFile, join(archive, 'test-e2e-preview.json'));
  }
  if (existsSync(allFile) && !read(allFile).shards) {
    const previous = read(allFile),
      archive = join(
        directory,
        'attempts',
        'test-e2e-preview-' + previous.startedAt.replaceAll(':', '-'),
      );
    mkdirSync(archive, { recursive: true, mode: 0o700 });
    renameSync(allFile, join(archive, 'test-e2e-preview.json'));
    const oldLog = join(directory, 'test-e2e-preview.log');
    if (existsSync(oldLog)) renameSync(oldLog, join(archive, 'test-e2e-preview.log'));
  }
  let summary;
  const collect = () => {
    const outcomes = [],
      observed = [],
      duplicates = [];
    for (let n = 1; n <= shards; n++) {
      const file = join(folder, 'shard-' + n + '.json');
      if (!existsSync(file)) continue;
      const r = read(file);
      if (r.signature !== signature) continue;
      outcomes.push({
        shard: n,
        status: r.status,
        startedAt: r.startedAt,
        log: join('browser-shards', r.log),
        tests: r.tests?.length ?? 0,
        durationMs: r.durationMs,
      });
      for (const test of r.tests || []) {
        if (observed.includes(test.id)) duplicates.push(test.id);
        observed.push(test.id);
      }
    }
    const missing = expected.filter((id) => !observed.includes(id));
    const passed =
      outcomes.length === shards &&
      !duplicates.length &&
      !missing.length &&
      outcomes.every((r) => r.status === 'passed');
    summary = {
      command,
      cwd,
      startedAt: outcomes.map((r) => r.startedAt).sort()[0] || started,
      finishedAt: new Date().toISOString(),
      durationMs: outcomes.reduce((sum, r) => sum + r.durationMs, 0),
      status: passed
        ? 'passed'
        : outcomes.some((r) => r.status === 'failed')
          ? 'failed'
          : 'pending',
      provenance: options.provenance,
      exitCode: passed ? 0 : outcomes.some((r) => r.status === 'failed') ? 1 : null,
      signature,
      shards,
      completed: outcomes.length,
      cases: observed.length,
      expectedCases: expected.length,
      missing,
      duplicates,
      outcomes,
    };
    write(allFile, summary);
  };
  for (let n = first; n <= last; n++) {
    const label = 'shard-' + n,
      result = join(folder, label + '.json'),
      inventory = join(folder, label + '-cases.json'),
      events = join(folder, label + '-outcomes.jsonl');
    const old = existsSync(result) ? read(result) : null;
    if (old?.signature === signature && old.status === 'passed') continue;
    verifyRegressionSource(release);
    if (old) {
      // Keep the discovered cases, all attempts and the HTML report next to
      // the old command receipt when an actual browser shard is run again.
      const attempt = join(folder, 'attempts', label + '-' + old.startedAt.replaceAll(':', '-'));
      mkdirSync(attempt, { recursive: true, mode: 0o700 });
      for (const name of [label + '-cases.json', label + '-outcomes.jsonl', label + '-html']) {
        const prior = join(folder, name);
        if (existsSync(prior)) renameSync(prior, join(attempt, name));
      }
    }
    writeFileSync(events, '', { mode: 0o600 });
    const r = await recordCommand(
      folder,
      label,
      'pnpm',
      ['test:e2e:preview', ...reportArgs, '--shard', n + '/' + shards],
      {
        cwd,
        provenance: options.provenance,
        env: {
          ...options.env,
          // The fixture creates this file exclusively and shares it with its CLI child.
          // A previous server may have been killed before its deferred cleanup ran.
          INFRAPRO_E2E_OPS_CONFIG: join(operatorDirectory, label + '-' + randomUUID() + '.json'),
          PLAYWRIGHT_HTML_OUTPUT_DIR: join(folder, label + '-html'),
          ARC_BROWSER_INVENTORY_FILE: inventory,
          ARC_BROWSER_OUTCOME_FILE: events,
        },
      },
    );
    const parsed = readFileSync(events, 'utf8')
      .trim()
      .split('\n')
      .filter(Boolean)
      .map((v) => JSON.parse(v));
    const declared = existsSync(inventory) ? read(inventory) : [],
      lastAttempt = new Map(parsed.map((v) => [v.id, v]));
    const observations = declared.map((v) => lastAttempt.get(v.id)).filter(Boolean);
    if (
      r.status === 'passed' &&
      (!declared.length ||
        declared.length !== observations.length ||
        observations.some((v) => v.expected !== v.status))
    )
      r.status = 'failed';
    Object.assign(r, { signature, tests: observations });
    verifyRegressionSource(release, r);
    const observedSignature = browserSignature(
      cwd,
      options.provenance.role,
      options.provenance.releaseSHA256,
      options.provenance.sharedConsoleSHA256,
    );
    if (observedSignature !== signature) {
      r.status = 'failed';
      r.failure = 'Browser specifications or their harness changed during the shard';
      r.observedSignature = observedSignature;
    }
    write(result, r);
    collect();
    console.log(
      'Original browser shard ' +
        n +
        '/' +
        shards +
        ' ' +
        r.status +
        ' (' +
        observations.length +
        '/' +
        declared.length +
        ' cases)',
    );
    if (
      r.observedSignature ||
      r.sourceAtCompletion?.matchesSelectedRelease === false ||
      (process.env.ARC_BROWSER_CHECKPOINT_FILE &&
        existsSync(process.env.ARC_BROWSER_CHECKPOINT_FILE))
    )
      break;
  }
  collect();
  return summary;
}
function assertUnique(values) {
  if (!values.length || new Set(values).size !== values.length)
    throw Error('Original browser inventory must contain unique tests');
}

const auditPhases = ['governance', 'resources', 'metadata', 'compatibility', 'files-restore'];
function archiveAuditPhases(directory, cwd, result) {
  const original = join(cwd, '.artifacts/audit');
  const evidence = join('audit-regression', result.startedAt.replaceAll(':', '-'));
  const dest = join(directory, evidence);
  mkdirSync(dest, { recursive: true, mode: 0o700 });
  const phases = [];
  for (const phase of auditPhases) {
    const marker = join(original, 'regression-' + phase + '.json');
    if (!existsSync(marker)) continue;
    const markerBytes = readFileSync(marker);
    const record = JSON.parse(markerBytes);
    if (record.startedAt < result.startedAt || record.completedAt > result.finishedAt) continue;
    const log = readFileSync(join(original, 'regression-' + phase + '.log'));
    writeFileSync(join(dest, phase + '.log'), log, { mode: 0o600 });
    writeFileSync(join(dest, phase + '.json'), markerBytes, { mode: 0o600 });
    phases.push({
      phase,
      command: record.command,
      passed: record.passed,
      startedAt: record.startedAt,
      finishedAt: record.completedAt,
      log: join(evidence, phase + '.log'),
      sha256: digest(log),
      manifest: join(evidence, phase + '.json'),
      manifestSHA256: digest(markerBytes),
      cases: [...log.toString('utf8').matchAll(/^--- (PASS|FAIL|SKIP): (\S+)/gm)].map(
        ([, status, name]) => ({ name, status }),
      ),
    });
  }
  result.phaseEvidence = phases;
  if (
    result.status === 'passed' &&
    (phases.length !== auditPhases.length || phases.some((p) => !p.passed))
  ) {
    result.status = 'failed';
    result.failure =
      'The command exited successfully but its original audit phase reports are incomplete';
  }
  write(join(directory, 'test-audit-regression.json'), result);
}

function validAuditPhases(directory, row) {
  const grants = row.provenance?.auditRoleSetup;
  if (!grants?.files || grants.releaseSHA256 !== row.provenance.releaseSHA256 ||
      row.phaseEvidence?.length !== auditPhases.length) return false;
  return row.phaseEvidence.every((phase, index) => {
    if (!phase.passed || phase.phase !== auditPhases[index] || !phase.manifest || !phase.log) return false;
    const manifest = join(directory, phase.manifest), log = join(directory, phase.log);
    if (!existsSync(manifest) || !existsSync(log)) return false;
    const marker = readFileSync(manifest), bytes = readFileSync(log);
    if (digest(marker) !== phase.manifestSHA256 || digest(bytes) !== phase.sha256) return false;
    const original = JSON.parse(marker);
    if (!original.passed || original.startedAt !== phase.startedAt || original.completedAt !== phase.finishedAt) return false;
    if (phase.phase !== 'metadata') return true;
    const output = bytes.toString('utf8');
    return output.includes('current_user=infrapro_audit_worker metadata_read=true credential_read=false account_write=false') &&
      Object.entries(grants.files).every(([name, sha]) => output.includes('audit source fixture installed ' + name + ' ' + sha));
  });
}

export function regressionRelease(dir, role) {
  if (role === 'baseline') return read(join(dir, 'baseline.json'));
  const chosen =
    process.env.INFRAPRO_ARCHITECTURE_REGRESSION_RELEASE ||
    process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE;
  const evaluated = join(dir, 'candidate.evaluated.json');
  const location = chosen || (existsSync(evaluated) && read(evaluated).directory) || dir;
  const manifest = read(join(runDirectory(location), 'candidate.json'));
  return manifest;
}

export function verifyRegressionSource(release, result) {
  const frozen = !release.inputs;
  const expected = frozen ? release.source_archive_sha256 : release.inputs.sourceSHA256;
  let actual;
  if (frozen) {
    const changed = Object.entries(release.sourceFiles).find(([name, sha]) => {
      const file = join(release.source, name);
      return !existsSync(file) || digest(readFileSync(file)) !== sha;
    });
    if (changed && !result) throw Error('Frozen build input changed: ' + changed[0]);
    actual = changed ? null : expected;
  } else {
    actual = checkoutApplicationDigest(root);
  }
  const verification = {
    checkedAt: new Date().toISOString(),
    sourceSHA256: actual,
    expectedSHA256: expected,
    matchesSelectedRelease: actual === expected,
  };
  if (!result) {
    if (actual !== expected) throw Error('Regression source differs from the selected release');
    return verification;
  }
  result.sourceAtCompletion = verification;
  if (actual !== expected) {
    result.processStatus = result.exitCode === 0 ? 'passed' : 'failed';
    result.status = 'failed';
    result.failure = [
      result.failure,
      'Application source changed during this run. Repeat against the selected release.',
    ]
      .filter(Boolean)
      .join(' ');
  }
}

export async function regression(dir, role, selected = checks, { stopStorage = false, resume = false, membershipPlan = 'default' } = {}) {
  const planner = membershipPlanner(membershipPlan);
  if (membershipPlan !== 'default' && (selected.length !== 1 || selected[0] !== 'test:display:load' || resume))
    throw Error('A matched planner replay requires one fresh member boundary run');
  registerFunctionalScope(dir);
  if (selected.every(name => performanceCommands.has(name)))
    return selected.map(name => skippedOriginal(dir, role, name));
  const source = role === 'baseline' ? join(dir, 'baseline-source') : root;
  const standard = join(dir, 'regression', role);
  const directory = membershipPlan === 'default' ? standard : join(standard,'membership-planners',membershipPlan);
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const release = regressionRelease(dir, role);
  const env = {
    ...Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_'))),
    CI: '1',
    INFRAPRO_FILES_CHECKER_EXECUTABLE: join(release.bin, 'files-checker'),
  };
  if (role === 'baseline') {
    // The frozen source is a git archive. Commands which inspect tracked
    // sources need its own index, not the enclosing candidate checkout.
    const privateIndex = join(dir, 'private', 'baseline-index');
    Object.assign(env, {
      GIT_DIR: execFileSync('git', ['rev-parse', '--absolute-git-dir'], {
        cwd: root,
        encoding: 'utf8',
      }).trim(),
      GIT_WORK_TREE: source,
      GIT_INDEX_FILE: privateIndex,
      GIT_OPTIONAL_LOCKS: '0',
    });
    if (!existsSync(privateIndex))
      execFileSync('git', ['read-tree', read(join(dir, 'baseline.json')).commit], {
        cwd: source,
        env,
      });
    // The all-domain/task suites also call the Python converter. Its locked
    // interpreter is a generated test dependency for the whole frozen tree.
    const requirement = 'tools/spatial-converter/requirements.lock';
    const installed = join(root, '.artifacts/spatial-venv'),
      local = join(source, '.artifacts/spatial-venv');
    if (
      existsSync(join(installed, 'bin/python')) &&
      !existsSync(local) &&
      digest(readFileSync(join(root, requirement))) ===
        digest(readFileSync(join(source, requirement)))
    ) {
      mkdirSync(join(source, '.artifacts'), { recursive: true });
      try {
        symlinkSync(installed, local, 'dir');
      } catch (error) {
        if (error.code !== 'EEXIST') throw error;
      }
    }
  }
  const entries = [];
  for (const name of selected) {
    if (performanceCommands.has(name)) {
      entries.push(skippedOriginal(dir, role, name));
      continue;
    }
    const label = name.replaceAll(':', '-');
    const monitoringOutage = ['test:monitoring', 'test:monitoring:contract'].includes(name);
    const soak = name === 'test:derived:soak' ? derivedSoakMode(dir) : undefined;
    // S-01 stops the complete object store, then proves an existing payload is
    // readable within its original 20-second recovery limit. Retain a dedicated
    // per-side contract project whose volumes belong only to this scenario.
    const storageGroup = originalStorageGroup(name, role, soak);
    // Domain commands control their own Compose stack and Go test overlay.
    // The diagnostics pair, the HTTP fixture and the SDK each share a host
    // resource even across different releases.
    const usesContract = ![
      'check',
      'build',
      'contracts:check',
      'permissions:check',
      'test:http',
      'test:diagnostics',
      'test:monitoring:client',
      // Without --contract or --database the published derived command runs
      // the seven rule packages directly and starts no shared dependencies.
      'test:derived',
      'test:derived:load',
      'test:files:client',
      'test:files:sandbox',
      'test:notifications:client',
      'test:display:compatibility',
    ].includes(name);
    const shared = usesContract
      ? (storageGroup === 'general' ? 'contract-' : storageGroup + '-') + role
      : {
          'test:http': 'nginx-18088',
          'test:display:compatibility': 'sdk-listeners',
          'test:diagnostics': 'independent-diagnostics',
        }[name];
    const unlock = shared
      ? await fileLease(join(dir, 'regression', 'locks', shared + '.flock'))
      : null;
    let unlockSharedTarget, frameClock, reused = false;
    try {
      // Another domain runner may have finished this exact release while
      // the aggregate runner was waiting for its isolated storage.
      if (resume) {
        const selectedSource = release.inputs?.sourceSHA256 ?? release.source_archive_sha256;
        const completed = readChecks(dir, role, selectedSource).find(row => row.command[1] === name && row.status === 'passed');
        if (completed) {
          reused = true;
          entries.push(completed);
          continue;
        }
      }
      // Both public commands create a fresh restore target on the same host ports.
      if (['test:monitoring:restore', 'test:display:restore'].includes(name)) {
        unlockSharedTarget = await fileLease(join(dir, 'regression', 'locks', 'restore-target.flock'));
      } else if (['test:https', 'test:e2e:preview'].includes(name)) {
        // Both original browser commands share this release's browser fixture.
        // Their application databases are independent of domain jobs and faults.
        unlockSharedTarget = await fileLease(join(dir, 'regression', 'locks', 'https-browser.flock'));
      } else if (['test:http', 'test:diagnostics'].includes(name)) {
        // These original commands both compile Linux images in the same Docker VM.
        // Starting both builds together exhausted its memory during a real replay.
        unlockSharedTarget = await fileLease(join(dir, 'regression', 'locks', 'docker-build.flock'));
      }
      const sourceAtStart = verifyRegressionSource(release);
      const frozenBuild =
        role === 'baseline' && ['check', 'build', 'test:e2e:preview', 'test:https'].includes(name)
          ? prepareFrozenBuild(dir, release, name, env)
          : null;
      const cwd = frozenBuild?.cwd ?? source;
      const extra =
        name === 'test:e2e:preview'
          ? {
              INFRAPRO_MONITORING_E2E: '1',
              INFRAPRO_GOVERNANCE_E2E: '1',
              INFRAPRO_E2E_OPS_CONFIG: join(directory, 'e2e-ops.json'),
              INFRAPRO_E2E_API_URL:
                role === 'baseline' ? 'http://127.0.0.1:18973' : 'http://127.0.0.1:18974',
              INFRAPRO_E2E_CONSOLE_URL:
                role === 'baseline' ? 'http://127.0.0.1:15973' : 'http://127.0.0.1:15974',
            }
          : {};
      if (name === 'test:https') {
        extra.INFRAPRO_HTTPS_OUTPUT = join(dir, 'private', 'https-regression', role, new Date().toISOString().replaceAll(':', '-'));
        mkdirSync(extra.INFRAPRO_HTTPS_OUTPUT, { recursive: true, mode: 0o700 });
      }
      if (name === 'test:notifications')
        extra.GOFLAGS = '-v';
      if (name === 'test:display:load') {
        extra.ARC_DISPLAY_BOUNDARY_PLANNER = planner.profile;
        extra.PGOPTIONS = planner.pgOptions;
      }
      // The original all-module checks use the same parser, cache and test store.
      // Run their batches sequentially on both releases and retain every test's deadline.
      if (name === 'test:identity') extra.INFRAPRO_GO_TEST_JOBS = '1';
      // The original runner disconnects its worker at 120 s and restores it at 165 s.
      // Its fresh compatibility dataset is separate from the nine timed ARC-037 rounds.
      if (name === 'test:ingestion:load') extra.INFRAPRO_INGESTION_LOAD_SECONDS = '180';
      if (name === 'test:http') {
        Object.assign(extra, isolatedHTTPNetwork(dir, cwd, role));
        if (role === 'baseline') extra.NODE_OPTIONS = baselineHTTPHarness(dir, cwd);
        const services = read(extra.ARC_LAN_COMPOSE).services;
        for (const service of httpImageBuilds(services)) {
          const built = await recordCommand(directory, 'test-http-build-' + service, extra.ARC_LAN_DOCKER,
            ['compose', '-f', extra.ARC_LAN_COMPOSE, '-p', extra.COMPOSE_PROJECT_NAME, 'build', service], {
              cwd, env: { ...env, ...extra }, provenance: { role, releaseSHA256: sourceAtStart.sourceSHA256 },
            });
          if (built.status !== 'passed') throw Error('Original HTTP image build failed: ' + service);
        }
      }
      if (name === 'test:diagnostics') {
        Object.assign(extra, diagnosticsRegressionEnvironment(dir, role, cwd));
        extra.INFRAPRO_DIAGNOSTICS_SEALED_BIN = release.bin;
        if (parkedDiagnosticsLab(dir, role, cwd)) {
          const resumed = await recordCommand(
            directory,
            'test-diagnostics-resume',
            'node',
            [join(root, 'scripts/architecture-diagnostics-harness.mjs'), '--resume', dir, role],
            { cwd, env: { ...env, ...extra } },
          );
          if (resumed.status !== 'passed')
            throw Error(
              'Could not resume the recorded diagnostics lab; see ' + join(directory, resumed.log),
            );
        }
        const setup = await recordCommand(
          directory,
          'test-diagnostics-environment',
          'pnpm',
          ['test:diagnostics:environment'],
          { cwd, env: { ...env, ...extra } },
        );
        if (setup.status !== 'passed')
          throw Error('Independent diagnostics setup failed; see ' + join(directory, setup.log));
        const bind = await recordCommand(
          directory,
          'test-diagnostics-bind-release',
          'node',
          [
            join(root, 'scripts/architecture-diagnostics-harness.mjs'),
            '--bind',
            dir,
            role,
            join(dirname(release.bin), role + '.json'),
          ],
          { cwd, env: { ...env, ...extra } },
        );
        if (bind.status !== 'passed')
          throw Error(
            'Independent diagnostics release processes failed; see ' + join(directory, bind.log),
          );
        recordDiagnosticsFixture(dir, role, cwd, release);
        // Preserve the original S0 browser driver's actual outcome separately.
        // This invocation deliberately uses the frozen driver's own assertions.
        if (role === 'baseline') {
          const originalAggregate = await recordCommand(
            directory,
            'test-diagnostics-unmodified',
            'pnpm',
            ['test:diagnostics'],
            {
              cwd,
              env: { ...env, GOWORK: extra.GOWORK },
              provenance: { role, commit: release.commit, fixtureOverlay: false },
            },
          );
          archiveOriginalDiagnostics(dir, role, cwd, originalAggregate);
          await recordCommand(
            directory,
            'test-diagnostics-frozen-templates',
            'node',
            ['scripts/diagnostics-templates.mjs', '--check'],
            {
              cwd,
              env: { ...env, GOWORK: extra.GOWORK },
              provenance: { role, commit: release.commit, fixtureOverlay: false },
            },
          );
          const originalBrowser = await recordCommand(
            directory,
            'test-diagnostics-frozen-browser',
            'node',
            ['scripts/diagnostics-browser-check.mjs'],
            {
              cwd,
              env: { ...env, GOWORK: extra.GOWORK },
              provenance: {
                role,
                commit: release.commit,
                fixtureOverlay: false,
              },
            },
          );
          await archiveFrozenDiagnosticsBrowser(dir, cwd, originalBrowser);
        } else {
          const prior = join(directory, 'test-diagnostics-unmodified.json');
          const raw = existsSync(prior) ? read(prior) : null;
          if (raw?.provenance?.releaseSHA256 !== release.inputs.sourceSHA256) {
            const originalAggregate = await recordCommand(directory, 'test-diagnostics-unmodified', 'pnpm', ['test:diagnostics'], {
              cwd, env: { ...env, GOWORK: extra.GOWORK },
              provenance: {role,releaseSHA256:release.inputs.sourceSHA256,fixtureOverlay:false},
            });
            archiveOriginalDiagnostics(dir, role, cwd, originalAggregate);
          }
        }
        frameClock = diagnosticFrameClock(dir, role, cwd);
        const clockFlag = '-overlay=' + frameClock.overlay;
        extra.GOFLAGS = [extra.GOFLAGS, /\s/.test(clockFlag) ? JSON.stringify(clockFlag) : clockFlag].filter(Boolean).join(' ');
      }
      const browserEnvironment = async (storage) => {
        const { domainTestEnvironment } = await import('./architecture-domain-http.mjs');
        const observed = (await domainTestEnvironment(dir, role, 'browser', storage, cwd)).env;
        for (const [key, value] of Object.entries(observed))
          if (key === 'GOFLAGS' || key.startsWith('INFRAPRO_ARCH_TEST_')) extra[key] = value;
        extra.PLAYWRIGHT_HTML_OPEN = 'never';
        extra.PLAYWRIGHT_HTML_OUTPUT_DIR = join(directory, 'browser-report');
        const pointer = join(directory, 'browser-records.json');
        const signature = browserSignature(
          cwd,
          role,
          release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
          sharedConsoleFor(dir)?.console.sha256,
        );
        const prior = existsSync(pointer) ? read(pointer) : null;
        if (prior?.signature === signature) extra.INFRAPRO_ARCH_TEST_RECORD = prior.directory;
        if (prior?.signature && prior.signature !== signature)
          write(join(directory, 'browser-history', prior.signature, 'recording.json'), prior);
        write(pointer, {
          directory: extra.INFRAPRO_ARCH_TEST_RECORD,
          signature,
          releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
        });
        return extra;
      };
      const keepsSharedDependencies = [
        'test:monitoring',
        'test:monitoring:contract',
        'test:derived',
        'test:derived:contract',
        'test:derived:database',
      ].includes(name);
      const browserConfig = name === 'test:e2e:preview' ? browserConfigFor(dir, role, cwd) : null;
      const browserReport =
        name === 'test:e2e:preview'
          ? [
              ...(browserConfig ? ['--config', browserConfig] : []),
              '--reporter',
              'list,html,' + join(root, 'scripts/architecture-browser-reporter.mjs'),
            ]
          : [];
      const inheritedFlags = env.GOFLAGS || '';
      extra.GOFLAGS = [inheritedFlags, extra.GOFLAGS].filter(Boolean).join(' ');
      if (monitoringOutage)
        extra.GOFLAGS = (extra.GOFLAGS + ' -skip=^TestInitialCapacity$').trim();
      const overrides = usesContract
        ? await regressionInfra(
            dir,
            role,
            cwd,
            extra,
            name === 'test:e2e:preview' ? browserEnvironment : undefined,
            storageGroup,
            ['test:derived:pipeline', 'test:derived:soak'].includes(name),
            name,
          )
        : extra;
      const options = {
        cwd,
        env: { ...env, ...overrides, ...(frozenBuild && { GIT_WORK_TREE: cwd }) },
        provenance: {
          ...(performanceStages[name] && { functionalScope: true, skippedStages: performanceStages[name], skipReason }),
          ...(overrides.ARC_TRIAL_SETUP_EVIDENCE && {
            trialSetup: JSON.parse(overrides.ARC_TRIAL_SETUP_EVIDENCE),
          }),
          ...(overrides.ARC_AUDIT_ROLE_SETUP && {
            auditRoleSetup: JSON.parse(overrides.ARC_AUDIT_ROLE_SETUP),
          }),
          ...(overrides.ARC_DISPLAY_BOUNDARY_SETUP && {
            membershipFixture: JSON.parse(overrides.ARC_DISPLAY_BOUNDARY_SETUP),
          }),
          ...(storageGroup === 'monitoring' && {
            monitoringStorage: read(join(dirname(overrides.ARC_REGRESSION_INFRA), 'monitoring-store.json')),
          }),
          role,
          commit: release.inputs?.commit ?? release.commit,
          releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
          suiteCommand: read(join(cwd, 'package.json')).scripts[name],
          ...(frameClock && { diagnosticFrameClock: frameClock.receipt }),
          ...(['derived-database','monitoring','identity','files'].includes(storageGroup) && {
            storageGroup,
            storageProject: read(overrides.ARC_REGRESSION_INFRA).project,
          }),
          ...(soak && { derivedSoakMode: soak }),
          ...(name === 'test:ingestion:load' && {
            ingestionSeconds: Number(extra.INFRAPRO_INGESTION_LOAD_SECONDS),
          }),
          ...(name === 'test:identity' && {
            parallelGoBatches: Number(extra.INFRAPRO_GO_TEST_JOBS),
          }),
          node: process.version,
          browserOverlay: role === 'baseline' && name === 'test:e2e:preview',
          ...(name === 'test:e2e:preview' && { sharedConsoleSHA256: sharedConsoleFor(dir)?.console.sha256 }),
        },
      };
      const result =
        name === 'test:e2e:preview'
          ? await browserRegression(
              directory,
              cwd,
              options,
              browserReport,
              read(join(directory, 'browser-records.json')).signature,
              release,
            )
          : await recordCommand(
              directory,
              label,
              'pnpm',
              [
                name,
                ...(name === 'test:display:load' ? ['--boundary'] : []),
                ...(keepsSharedDependencies ? ['--keep'] : []),
                ...(soak === 'smoke' ? ['--smoke'] : []),
              ],
              options,
            );
      result.sourceAtStart = sourceAtStart;
      verifyRegressionSource(release, result);
      if (name === 'test:identity') {
        try {
          result.identityBatchEvidence = archiveIdentityRegressionEvidence(directory, cwd, result);
          if (result.status === 'passed' && !validIdentityRegressionEvidence(directory, result))
            throw Error('The native Go runner did not account for every discovered batch and test.');
        } catch (error) {
          result.processStatus ??= result.exitCode === 0 ? 'passed' : 'failed';
          result.status = 'failed';
          result.failure = [result.failure, 'Identity batch evidence: ' + safeDiagnosticLine(error.message)].filter(Boolean).join(' ');
        }
      }
      if (monitoringOutage && result.status === 'passed') {
        try {
          result.monitoringRecovery = monitoringRecoveryEvidence(
            directory, result, read(overrides.ARC_REGRESSION_INFRA).monitoringRestart,
          );
        } catch (error) {
          result.status = 'failed';
          result.failure = 'Monitoring outage verification failed: ' + safeDiagnosticLine(error.message);
        }
      }
      frozenBuild?.inspect(result);
      if (name === 'test:diagnostics' && result.status === 'passed') {
        try {
          archiveDiagnosticsEvidence(dir, role, cwd, result);
        } catch (error) {
          result.status = 'failed';
          result.failure =
            'Diagnostics evidence validation failed: ' + safeDiagnosticLine(error.message);
        }
        write(join(directory, label + '.json'), result);
      }
      if (name === 'test:audit:regression') archiveAuditPhases(directory, cwd, result);
      if (name === 'test:tasks') {
        try {
          archiveTaskEvidence(directory, cwd, result);
        } catch (error) {
          result.status = 'failed';
          result.failure = 'Task regression evidence validation failed: ' + safeDiagnosticLine(error.message);
        }
      }
      archiveOriginalIngestionLoad(directory, cwd, name, result);
      const loadEvidence = await archiveOriginalLoadEvidence(directory, cwd, name, result);
      if (loadEvidence) {
        const { artifact, passed, fullQualification, mode } = loadEvidence;
        result.originalLoadEvidence = {
          artifact,
          passed,
          fullQualification,
          mode,
        };
        if (result.status === 'passed' && !passed) {
          result.status = 'failed';
          result.failure = safeDiagnosticLine(loadEvidence.failure);
        }
      }
      archiveRestoreEvidence(directory, cwd, name, result);
      write(join(directory, label + '.json'), result);
      if (name === 'test:https' && result.status === 'passed') {
        try {
          result.httpsEvidence = archiveHTTPSEvidence(directory, extra.INFRAPRO_HTTPS_OUTPUT, result);
        } catch (error) {
          result.status = 'failed';
          result.failure = 'HTTPS evidence verification: ' + safeDiagnosticLine(error.message);
        }
        write(join(directory, label + '.json'), result);
      }
      entries.push(result);
      console.log(
        role +
          ' ' +
          name +
          ' ' +
          result.status +
          ' (' +
          Math.round(result.durationMs / 1000) +
          's)',
      );
    } finally {
      try {
        if (!reused && usesContract && (stopStorage || ['https', 'derived-database', 'identity', 'files'].includes(storageGroup)))
          stopRegressionStorage(dir, role, storageGroup);
      } finally {
        await unlockSharedTarget?.();
        await unlock?.();
      }
    }
    if (
      process.env.ARC_REGRESSION_CHECKPOINT_FILE &&
      existsSync(process.env.ARC_REGRESSION_CHECKPOINT_FILE)
    )
      break;
  }
  return entries;
}
export function readChecks(dir, role, selectedSource) {
  const manifest = join(dir, role === 'candidate' ? 'candidate.evaluated.json' : 'baseline.json');
  const release = existsSync(manifest) ? read(manifest) : null;
  const current = selectedSource ?? release?.inputs?.sourceSHA256 ?? release?.source_archive_sha256;
  return checks.map((name) => {
    if (performanceCommands.has(name)) return skippedOriginal(dir, role, name);
    const file = join(dir, 'regression', role, name.replaceAll(':', '-') + '.json');
    if (!existsSync(file)) return { command: ['pnpm', name], status: 'pending' };
    const row = read(file);
    if (name === 'test:display:load' && row.status === 'passed' && !row.command.includes('--boundary'))
      return { ...row, status: 'pending', priorStatus: row.status, reason: 'Run the original member boundary independently of the load scenario' };
    if (current && row.provenance?.releaseSHA256 !== current) {
      return {
        ...row,
        status: 'pending',
        priorStatus: row.status,
        requiredRelease: current,
        reason: 'Re-run against the selected release with its recorded source digest',
      };
    }
    if (name === 'test:display:load') return membershipRow(dir,role,row);
    if (name === 'test:identity' && row.status === 'passed' && !validIdentityRegressionEvidence(dirname(file), row, current))
      return { ...row, status: 'pending', priorStatus: 'passed', reason: 'Original identity batch logs or their test plan are incomplete or have changed.' };
    if (name === 'test:diagnostics' && row.status === 'passed') {
      const clock = row.provenance?.diagnosticFrameClock;
      const summary = row.diagnosticsEvidence?.summary && join(dir, row.diagnosticsEvidence.summary);
      const archived = summary && existsSync(summary) && read(summary).frame_clock;
      if (!clock || !archived || archived.original_sha256 !== clock.original_sha256 || archived.effective_sha256 !== clock.effective_sha256)
        return {...row, status:'pending', priorStatus:'passed', reason:'The shared report-clock fixture has not been verified for this diagnostic command.'};
    }
    if (name === 'test:https' && row.status === 'passed' && !validHTTPSEvidence(dirname(file), row)) {
      return { ...row, status: 'pending', reason: 'Repeat the original HTTPS command and archive its proxy, browser and public certificate evidence' };
    }
    if (name === 'test:audit:regression' && row.status === 'passed' && !validAuditPhases(dirname(file), row)) {
      return { ...row, status: 'pending', priorStatus: 'passed', reason: 'Repeat the five original audit phases with their published role grants and retain each phase manifest and log.' };
    }
    if (name === 'test:tasks' && row.status === 'passed' && !validTaskEvidence(dirname(file), row)) {
      return { ...row, status: 'pending', priorStatus: 'passed', reason: 'Original task regression phase results are missing or differ from the retained command logs.' };
    }
    if (name.endsWith(':restore') && row.status === 'passed' && !validRestoreEvidence(dirname(file), name, row)) {
      return { ...row, status: 'pending', priorStatus: row.status, reason: 'The original restore transcript or its archived proof is missing or changed' };
    }
    if (row.status === 'failed' && row.exitCode && row.log === name.replaceAll(':', '-') + '.log') {
      const originalLog = join(dir, 'regression', role, row.log);
      if (existsSync(originalLog)) {
        const reason = failureSummary(originalLog);
        if (reason && reason !== row.failure) {
          row.recordedFailure = row.failure;
          row.failure = reason;
        }
      }
    }
    if (row.status === 'passed' && hasOriginalLoadEvidence(name)) {
      const artifact = row.originalLoadEvidence?.artifact;
      const archive = artifact && join(dir, 'regression', role, artifact);
      const evidence = archive && existsSync(archive) && read(archive);
      if (
        !evidence?.passed ||
        evidence.startedAt !== row.startedAt ||
        evidence.releaseSHA256 !== row.provenance?.releaseSHA256
      ) {
        return {
          ...row,
          status: 'pending',
          priorStatus: 'passed',
          reason: 'Original command measurements are missing for this run',
        };
      }
    }
    if (
      row.status === 'passed' &&
      name === 'test:ingestion:load' &&
      row.ingestionLoadEvidence?.complete !== true
    ) {
      return {
        ...row,
        status: 'pending',
        priorStatus: 'passed',
        reason: 'Original ingestion and recovery receipts are missing',
      };
    }
    if (
      name === 'test:derived:soak' &&
      row.status === 'passed' &&
      row.provenance?.derivedSoakMode !== derivedSoakMode(dir)
    ) {
      return {
        ...row,
        status: 'pending',
        priorStatus: 'passed',
        reason: 'A different derived soak profile was selected',
      };
    }
    if (row.status === 'passed' && name === 'test:e2e:preview') {
      const cwd = role === 'baseline' ? join(dir, 'baseline-source') : root;
      const release =
        role === 'baseline'
          ? read(join(dir, 'baseline.json')).source_archive_sha256
          : (current ?? regressionRelease(dir, role).inputs.sourceSHA256);
      if (row.signature !== browserSignature(cwd, role, release, sharedConsoleFor(dir)?.console.sha256)) {
        return {
          ...row,
          status: 'pending',
          priorStatus: 'passed',
          reason: 'Browser test inputs have changed',
        };
      }
    }
    return row;
  });
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [, , target, role, ...arguments_] = process.argv;
  const requestMatched = arguments_.includes('--matched-membership-planner');
  const names = arguments_.filter(name => name !== '--matched-membership-planner');
  if (!['baseline', 'candidate'].includes(role)) throw Error('Expected baseline or candidate');
  // Complete module evaluation before loading the browser recorder, which also uses recordCommand.
  regression(runDirectory(target), role, names.length ? names : checks,
    requestMatched ? { membershipPlan:'matched', stopStorage:true } : {})
    .then((results) => {
      process.exitCode = results.some((r) => !['passed','skipped'].includes(r.status)) ? 1 : 0;
    })
    .catch((error) => {
      console.error(safeDiagnosticLine(error.message || error.name).slice(0, 450));
      process.exitCode = 1;
    });
}
