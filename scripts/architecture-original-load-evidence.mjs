import { createHash } from 'node:crypto';
import {
  createReadStream,
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { isDeepStrictEqual } from 'node:util';

const suites = {
  'test:derived': ['derived-monitoring', 'Derived monitoring evidence: ', 'rules.log'],
  'test:derived:load': [
    'derived-monitoring',
    'Derived monitoring evidence: ',
    'calculator-load.log',
  ],
  'test:derived:pipeline': ['derived-load', 'DP-0 real pipeline evidence: ', 'pipeline.log'],
  'test:derived:soak': ['derived-soak', 'DP-0 real stability evidence: ', 'soak.log'],
  'test:display:load': ['display/monitoring-read', '', 'load-and-boundary.log'],
};
const operators = ['mean', 'quantile', 'rate', 'variance'];
const expectedPackages = [
  'derivedmonitoring',
  'derivedmonitoring/internal/compute',
  'derivedmonitoring/internal/exactsort',
  'derivedmonitoring/internal/planning',
  'monitoringvalue',
  'monitoring/internal/observation',
  'monitoring',
];
export const hasOriginalLoadEvidence = (command) => Object.hasOwn(suites, command);

function requireEvidence(ok, message) {
  if (!ok) throw new Error(message);
}

async function lines(file, visit) {
  const input = createReadStream(file);
  const reader = createInterface({ input, crlfDelay: Infinity });
  try {
    for await (const line of reader) if (visit(line) === false) break;
  } finally {
    reader.close();
    input.destroy();
  }
}

async function announcedDirectory(parent, label, commandLog) {
  if (!label) return realpathSync(parent);
  let announced;
  await lines(commandLog, (line) => {
    if (!line.startsWith(label)) return;
    announced = realpathSync(line.slice(label.length).trim());
    return false;
  });
  requireEvidence(
    announced && dirname(announced) === realpathSync(parent),
    'Original runner did not announce its own evidence directory',
  );
  return announced;
}

function goDurationSeconds(value) {
  if (typeof value !== 'string') return NaN;
  const parts = value.match(/(?:\d+(?:\.\d+)?)(?:ns|µs|us|ms|s|m|h)/g) || [];
  if (parts.join('') !== value) return NaN;
  const scales = {
    h: 3600,
    m: 60,
    s: 1,
    ms: 1e-3,
    us: 1e-6,
    µs: 1e-6,
    ns: 1e-9,
  };
  return parts.reduce(
    (seconds, part) => seconds + parseFloat(part) * scales[part.replace(/^[\d.]+/, '')],
    0,
  );
}

async function readGoLog(path) {
  const packages = [],
    tests = [],
    memberships = [],
    markers = {};
  await lines(path, (line) => {
    const pkg = line.match(/^ok\s+[^\s]*\/packages\/go\/platform\/(\S+)\s+/);
    if (pkg) packages.push(pkg[1]);
    const test = line.match(/^\s*--- PASS: ([^\s]+) \(([\d.]+)s\)/);
    if (test) tests.push({ name: test[1], seconds: Number(test[2]) });
    const fixture = line.match(/constraint-checked membership fixture total=(\d+) roots=(\d+) points=(\d+) committed_batches=(\d+)/);
    if(fixture)memberships.push(fixture.slice(1).map(Number));
    const metric = line.match(
      /\b(CALCULATOR_BASELINE|DP0_PIPELINE|DP0_INTERACTIONS|DP0_SOAK|DISPLAY_READ_LOAD) (.*)$/,
    );
    if (!metric) return;
    const data =
      metric[1] === 'CALCULATOR_BASELINE'
        ? Object.fromEntries(
            metric[2]
              .trim()
              .split(/\s+/)
              .map((pair) => pair.split('=')),
          )
        : JSON.parse(metric[2]);
    (markers[metric[1]] ||= []).push(data);
  });
  return { packages, tests, markers, memberships };
}

function assertTests(tests, names) {
  const ran = new Set(tests.map((t) => t.name));
  requireEvidence(
    names.every((n) => ran.has(n)),
    'Required Go tests did not report PASS',
  );
}

function verifyRules(log) {
  requireEvidence(
    expectedPackages.every((p) => log.packages.includes(p)),
    'Derived rule package receipts incomplete',
  );
}

function verifyCalculator(log) {
  assertTests(log.tests, [
    'TestMillionInputCalculator',
    ...operators.map((op) => 'TestMillionInputCalculator/' + op),
  ]);
  const metrics = log.markers.CALCULATOR_BASELINE || [];
  requireEvidence(
    isDeepStrictEqual(metrics.map((v) => v.op).sort(), operators),
    'Missing million-input calculator operators',
  );
  requireEvidence(
    metrics.every(
      (v) =>
        Number(v.input) === 1_000_000 &&
        Number(v.outputs) === 1_000_000 &&
        Number(v.partitions) === 10 &&
        Number(v.rows_per_second) >= Number(v.min_rows_per_second) &&
        goDurationSeconds(v.duration) > 0 &&
        goDurationSeconds(v.max_partition) <= 60 &&
        Number(v.peak_heap_bytes) > 0 &&
        Number(v.peak_heap_bytes) <= 2 ** 30 &&
        Number(v.GOMAXPROCS) === 2 &&
        v.GOMEMLIMIT === '1GiB',
    ),
    'Calculator size, duration, or resource gate failed',
  );
  return metrics;
}

function verifyPipeline(original, log) {
  assertTests(log.tests, [
    'TestMillionInputPipeline',
    'TestDerivedInteractionTargets',
    ...operators.map((op) => 'TestMillionInputPipeline/' + op),
  ]);
  const { metrics, interactions } = original;
  requireEvidence(
    original.fullQualification === true &&
      original.operator === 'all' &&
      isDeepStrictEqual(metrics, log.markers.DP0_PIPELINE) &&
      isDeepStrictEqual(interactions, log.markers.DP0_INTERACTIONS) &&
      isDeepStrictEqual(metrics.map((v) => v.operator).sort(), operators) &&
      interactions.length === 1,
    'Pipeline qualification or raw metric match failed',
  );
  requireEvidence(
    metrics.every(
      (m) =>
        m.inputs === 1_000_000 &&
        m.outputs === 1_000_000 &&
        m.formal_rows_per_second >= m.minimum_rows_per_second &&
        m.recovery_seconds <= m.recovery_max_seconds &&
        m.peak_heap_bytes <= 2 ** 30 &&
        m.peak_temporary_bytes <= 20 * 2 ** 30,
    ),
    'Million-input pipeline gates failed',
  );
  const i = interactions[0];
  requireEvidence(
    i.channels === 10 &&
      i.input_rows === 120 &&
      i.logical_outputs === 1200 &&
      i.trial_samples > 0 &&
      i.trends_samples > 0 &&
      i.submit_samples > 0 &&
      i.trial_p95_seconds <= 3 &&
      i.trends_p95_seconds <= 2 &&
      i.overview_p95_seconds <= 2 &&
      i.remote_first_read_seconds <= 10 &&
      i.submit_max_seconds <= 1 &&
      i.cancel_database_query_seconds <= 2,
    'Interaction samples or latency gates failed',
  );
  return { metrics, interactions };
}

function verifySoak(original, log, smoke) {
  assertTests(log.tests, ['TestDerivedContinuousMixedLoadAndObservation']);
  const [m] = original.metrics || [];
  requireEvidence(
    original.smoke === smoke &&
      original.fullQualification === !smoke &&
      original.metrics.length === 1 &&
      isDeepStrictEqual(original.metrics, log.markers.DP0_SOAK) &&
      m.smoke === smoke &&
      m.full_qualification === !smoke,
    'Soak mode, original qualification, or raw metric mismatch',
  );
  const mixed = smoke ? 120 : 7200,
    observation = smoke ? 120 : 86400;
  requireEvidence(
    m.mixed_target_seconds === mixed &&
      m.observation_target_seconds === observation &&
      m.mixed_seconds >= mixed &&
      m.observation_seconds >= observation &&
      m.elapsed_seconds >= mixed + observation &&
      m.input_rows > 0 &&
      m.current_rows > 0 &&
      m.queries > 0 &&
      m.verification_exports > 0 &&
      m.query_p95_seconds <= 2 &&
      m.automatic_p95_seconds <= 10 &&
      m.peak_heap_bytes <= 2 ** 30 &&
      m.peak_temporary_bytes <= 20 * 2 ** 30,
    'Soak duration, readback, stability, or resource gate failed',
  );
  return m;
}

function verifyDisplay(original, load, log) {
  assertTests(log.tests, [
    'TestDisplayReadFiftyThousandMemberBoundary',
    'TestDisplayReadLoadTenInstances',
  ]);
  requireEvidence(
    original.passed &&
      original.command.includes(
        '^TestDisplayRead(FiftyThousandMemberBoundary|LoadTenInstances)$',
      ) &&
      isDeepStrictEqual(log.markers.DISPLAY_READ_LOAD, [load]),
    'Display result does not match the raw Go metric',
  );
  requireEvidence(
    load.instances === 10 &&
      load.objects === 2600 &&
      load.channels === 600 &&
      load.observations === 100_000 &&
      ['cold', 'hot', 'directory'].every(
        (k) => load[k].samples_ms.length > 0 && Number.isFinite(load[k].p95_ms),
      ) &&
      load.cold.samples_ms.every((n) => n <= 120_000) &&
      load.hot.p95_ms <= 2000 &&
      load.directory.p95_ms <= 2000,
    'Display dimensions or latency gates failed',
  );
  return load;
}

// Archive small original result files. Published records reference logs and resource samples in place.
// The per-command log and the time interval identify the correct run even when a native script reuses a directory.
export async function archiveOriginalLoadEvidence(directory, cwd, name, result) {
  if (!hasOriginalLoadEvidence(name)) return null;
  const [group, announcement, goLog] = suites[name];
  const boundary = name === 'test:display:load' && result.command.includes('--boundary');
  const displayRun = boundary ? 'member-boundary' : 'load-and-boundary';
  const label = name.replaceAll(':', '-');
  const archive = join('evidence', label, result.startedAt.replaceAll(':', '-'));
  const target = join(directory, archive);
  mkdirSync(target, { recursive: true, mode: 0o700 });
  const smoke = result.command.includes('--smoke');
  const report = {
    command: result.command,
    releaseSHA256: result.provenance?.releaseSHA256,
    startedAt: result.startedAt,
    finishedAt: result.finishedAt,
    durationMs: result.durationMs,
    exitCode: result.exitCode,
    commandStatus: result.status,
    passed: false,
    mode: boundary ? 'member-boundary' : name === 'test:derived:soak' ? (smoke ? 'official-smoke' : 'full-qualification') : name,
    fullQualification: false,
    files: {},
    artifact: join(archive, 'summary.json'),
  };
  const start = Date.parse(result.startedAt),
    end = Date.parse(result.finishedAt);
  const during = (ms) => Number.isFinite(ms) && ms >= start && ms <= end + 1;
  try {
    const options = result.command.slice(2);
    const permitted =
      name === 'test:derived' ? ['--keep'] : name === 'test:derived:soak' ? ['--smoke'] : boundary ? ['--boundary'] : [];
    requireEvidence(
      result.command[0] === 'pnpm' &&
        result.command[1] === name &&
        resolve(result.cwd) === resolve(cwd) &&
        options.length <= 1 &&
        options.every((option) => permitted.includes(option)),
      'Unexpected command or working directory',
    );
    const suiteCommand = JSON.parse(readFileSync(join(cwd, 'package.json'), 'utf8')).scripts[name];
    const script = /^node (scripts\/test-[\w-]+\.mjs)(?: --[\w-]+)*$/.exec(suiteCommand)?.[1];
    requireEvidence(
      script &&
        (!result.provenance?.suiteCommand || result.provenance.suiteCommand === suiteCommand),
      'Original package script differs from its recorded command',
    );
    report.driver = {
      command: suiteCommand,
      path: script,
      sha256: createHash('sha256')
        .update(readFileSync(join(cwd, script)))
        .digest('hex'),
    };
    const folder = await announcedDirectory(
      join(cwd, '.artifacts', group),
      announcement,
      join(directory, result.log),
    );
    report.originalDirectory = relative(cwd, folder);
    function source(file) {
      const path = join(folder, file),
        { size, mtimeMs } = statSync(path);
      requireEvidence(size > 0 && during(mtimeMs), 'Stale or missing original ' + file);
      report.files[file] = { source: relative(cwd, path), bytes: size };
      return path;
    }
    function json(file, copy = false) {
      const path = source(file);
      requireEvidence(
        report.files[file].bytes <= 16 * 1024 * 1024,
        'Unexpected original JSON size for ' + file,
      );
      const bytes = readFileSync(path);
      report.files[file].sha256 = createHash('sha256').update(bytes).digest('hex');
      if (copy && bytes.length <= 128 * 1024) {
        writeFileSync(join(target, file), bytes, { mode: 0o600 });
        report.files[file].archived = join(archive, file);
      }
      return JSON.parse(bytes);
    }
    const display = name === 'test:display:load';
    const original = json(display ? displayRun + '-result.json' : 'result.json', true);
    requireEvidence(
      during(Date.parse(original.completedAt)),
      'Original completion timestamp belongs to another run',
    );
    if (display) {
      const environment = json(displayRun + '-environment.json');
      requireEvidence(
        during(Date.parse(environment.startedAt)),
        'Display environment belongs to another run',
      );
      report.resources = {
        logicalCpus: environment.logicalCpus,
        memoryBytes: environment.memoryBytes,
        dockerEngine: environment.dockerEngine,
        quotas: environment.containers,
      };
    } else {
      const manifest = json('source.json');
      requireEvidence(
        /^[a-f\d]{64}$/.test(manifest.sha256) &&
          /^[a-f\d]{40,64}$/.test(manifest.head) &&
          manifest.files?.length > 0,
        'Native source manifest incomplete',
      );
      report.source = {
        head: manifest.head,
        sha256: manifest.sha256,
        fileCount: manifest.files.length,
      };
      if (original.startedAt) {
        requireEvidence(
          during(Date.parse(original.startedAt)) && original.elapsedSeconds > 0,
          'Original elapsed time missing or stale',
        );
        report.elapsedSeconds = original.elapsedSeconds;
        const env = json('environment.json');
        report.resources = {
          host: env.host,
          docker: env.docker,
          processBudget: env.processBudget,
        };
        source('docker-samples.jsonl');
        source('host-process-samples.jsonl');
      } else {
        requireEvidence(
          original.runId === basename(folder),
          'Original run ID differs from announced evidence',
        );
      }
    }
    const nativeLog = source(boundary ? displayRun + '.log' : goLog);
    const log = await readGoLog(nativeLog);
    report.go = { packages: log.packages, tests: log.tests };
    if (name === 'test:derived' || name === 'test:derived:load') {
      requireEvidence(
        original.calculatorLoad === (name === 'test:derived:load') &&
          !original.database &&
          !original.contract,
        'Derived rule or calculator command mode differs',
      );
      const rules = name === 'test:derived' ? log : await readGoLog(source('rules.log'));
      verifyRules(rules);
      report.go.packages = rules.packages;
      if (name === 'test:derived:load') report.measurement = verifyCalculator(log);
    } else if (name === 'test:derived:pipeline') {
      report.measurement = verifyPipeline(original, log);
    } else if (name === 'test:derived:soak') {
      report.measurement = verifySoak(original, log, smoke);
    } else if (boundary) {
      report.memberBoundary = {};
      const logBytes = readFileSync(nativeLog);
      if (logBytes.length <= 1024 * 1024) {
        writeFileSync(join(target,displayRun + '.log'),logBytes,{mode:0o600});
        Object.assign(report.files[displayRun + '.log'],{ archived:join(archive, displayRun + '.log'),
          sha256:createHash('sha256').update(logBytes).digest('hex') });
      }
      const setup = result.provenance?.membershipFixture;
      if (setup) {
        const plans = [], settings = [];
        report.memberBoundary.fixture={...setup,memberships:log.memberships,plans,settings};
        for (const points of [49999,50000]) for (const stage of ['before-analyze','after-analyze']) {
          const file = join(setup.traces,points+'-settings-'+stage+'.json');
          if (existsSync(file)) {
            const observed = JSON.parse(readFileSync(file,'utf8'));
            requireEvidence(during(statSync(file).mtimeMs) && isDeepStrictEqual(observed,{
              enable_nestloop:setup.planner.enableNestloop,jit:setup.planner.jit,statement_timeout:'10s',lock_timeout:'5s',
            }), 'PostgreSQL transaction settings differ from their selected profile');
            settings.push({points,stage,observed,path:relative(directory,file)});
          }
          for (const [query,prefix,sqlSHA256] of [['members','',setup.previewSQLSHA256],['preview-directory','directory-',setup.directorySQLSHA256]]) {
            const path = join(setup.traces,points+'-'+prefix+stage+'.json');
            if (!existsSync(path)) continue;
            const bytes = readFileSync(path), plan = JSON.parse(bytes)[0]?.Plan;
            requireEvidence(during(statSync(path).mtimeMs) && plan?.['Node Type'] && Number.isFinite(plan['Plan Rows']),
              'Invalid in-window authorization query plan for '+points+' '+stage);
            plans.push({points,stage,query,sqlSHA256,path:relative(directory,path),sha256:createHash('sha256').update(bytes).digest('hex'),
              bytes:bytes.length, root:plan['Node Type'], estimatedRows:plan['Plan Rows']});
          }
        }
      }
      assertTests(log.tests, ['TestDisplayReadFiftyThousandMemberBoundary']);
      requireEvidence(original.passed === true && original.command.at(-1) === '^TestDisplayReadFiftyThousandMemberBoundary$' &&
        log.tests.length === 1 && !log.markers.DISPLAY_READ_LOAD, 'Display member-boundary command or result differs');
      if (setup) requireEvidence(isDeepStrictEqual(log.memberships,[[50000,1,49999,100],[50001,1,50000,1]]) &&
        report.memberBoundary.fixture.plans.length === 8 && report.memberBoundary.fixture.settings.length === 4,
        'Original boundary test did not verify both fully constrained fixture sets and both in-window business query plans');
      Object.assign(report.memberBoundary,{acceptedMembers:50000,rejectedMembers:50001});
    } else {
      const load = json('load.json', true);
      requireEvidence(during(Date.parse(load.at)), 'Display samples belong to another run');
      report.measurement = verifyDisplay(original, load, log);
    }
    requireEvidence(
      result.exitCode === 0 &&
        result.status === 'passed' &&
        (display || original.status === 'passed'),
      'Original command failed',
    );
    report.passed = true;
    report.fullQualification = original.fullQualification === true;
  } catch (error) {
    report.failure = ['ENOENT', 'EACCES'].includes(error.code)
      ? error.code + ' ' + basename(error.path || '')
      : error.message;
  }
  writeFileSync(join(directory, report.artifact), JSON.stringify(report, null, 2) + '\n', {
    mode: 0o600,
  });
  return report;
}
