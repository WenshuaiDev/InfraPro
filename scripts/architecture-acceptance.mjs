import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, relative } from 'node:path';
import { arch, platform, release, totalmem, cpus } from 'node:os';
import { digest, read, root, write } from './architecture-lab.mjs';
import { writeHTTPCoverage } from './architecture-http-coverage.mjs';
import { visualTriage } from './architecture-visual-triage.mjs';
import { consoleDigest, sharedConsoleFor } from './architecture-shared-console.mjs';
import { verifySessionActivityResult } from './architecture-session-activity.mjs';
import { verifiedCrossConsole } from './architecture-cross-console.mjs';
import { verifiedVisualInvitations } from './architecture-visual-invitations.mjs';
import { verifiedVisualFrontend, visualOutputDirectory } from './architecture-visual-frontend.mjs';
import { visualRecordingDrivers } from './architecture-visual-recording.mjs';
import { measuredDisplayBudget } from './architecture-display-processes.mjs';
import {
  currentPublicEvidence,
  currentRoleSQL,
  observedPublicFixture,
} from './architecture-parity-evidence.mjs';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';
import { readChecks } from './architecture-regression.mjs';
import { renderVerification } from './architecture-verification-report.mjs';
import { performanceArcs, performanceStages, registerFunctionalScope, satisfied, scopePath, skipReason } from './architecture-acceptance-scope.mjs';

export const suites = ['routes', 'visual', 'parity', 'faults', 'load', 'restore'];
export function versionManifest(dir) {
  const run = (cmd, args) =>
    execFileSync(cmd, args, {
      cwd: root,
      encoding: 'utf8',
      maxBuffer: 16 << 20,
    }).trim();
  const local = createRequire(import.meta.url);
  const playwright = createRequire(local.resolve('@playwright/test')).resolve(
    'playwright/package.json',
  );
  const core = createRequire(playwright).resolve('playwright-core/package.json');
  const browsers = read(join(dirname(core), 'browsers.json')).browsers.filter((b) =>
    ['chromium', 'chromium-headless-shell'].includes(b.name),
  );
  assert.equal(browsers.length, 2, 'Playwright must declare both Chromium distributions');
  let fonts;
  try {
    fonts = run('fc-list', ['--format', '%{family}\t%{style}\t%{fontversion}\n'])
      .split('\n')
      .filter(Boolean);
  } catch (error) {
    if (platform() !== 'darwin') throw error;
    const system = JSON.parse(run('system_profiler', ['-json', 'SPFontsDataType'])).SPFontsDataType;
    fonts = system.flatMap((f) =>
      (f.typefaces || []).map((t) => [t.family, t.style, t.version].join('\t')),
    );
  }
  fonts = [...new Set(fonts)].sort();
  assert(fonts.length > 0, 'Browser font inventory is empty');
  const file = join(dir, 'versions.json'),
    prior = existsSync(file) ? read(file) : {};
  const now = new Date().toISOString();
  const visual = {
    playwright: read(playwright).version,
    browsers,
    fontsSHA256: digest(fonts.join('\n') + '\n'),
    fonts: fonts.length,
    timezone: 'Asia/Shanghai',
    locale: 'zh-CN',
    deviceScaleFactor: 1,
  };
  write(join(dir, 'visual', 'fonts.json'), {
    sha256: visual.fontsSHA256,
    fonts,
  });
  const facts = {
    node: process.version,
    pnpm: run('pnpm', ['--version']),
    go: run('go', ['version']),
    docker: run('docker', ['version', '--format', '{{.Server.Version}}']),
    os: [platform(), release(), arch()],
    cpu: cpus()[0].model,
    logicalCpus: cpus().length,
    memoryBytes: totalmem(),
    commit: run('git', ['rev-parse', 'HEAD']),
    visual,
    startedAt: prior.startedAt ?? now,
    lastCheckedAt: now,
  };
  write(file, facts);
  return facts;
}
function requirements() {
  return [
    ...readFileSync(
      join(root, 'docs/development/system-architecture-refactor-spec.md'),
      'utf8',
    ).matchAll(/^\| (ARC-\d{3}) \| ([^|]+) \| ([^|]+) \|$/gm),
  ].map(([, id, input, expected]) => ({ id, input, expected }));
}
const expected = requirements();
assert.equal(expected.length, 39);
export function acceptance(dir, selected, commands = []) {
  const scope = registerFunctionalScope(dir);
  visualTriage(dir);
  const load = (path) => (existsSync(join(dir, path)) ? read(join(dir, path)) : null);
  const paths = (folder) =>
    existsSync(join(dir, folder))
      ? readdirSync(join(dir, folder))
          .filter((name) => name.endsWith('.json'))
          .map((name) => folder + '/' + name)
      : [];
  const groups = new Map(expected.map(({ id }) => [id, []]));
  const add = (id, part, path, passed, actual, status) => {
    const failed =
      actual === 'failed' || (Array.isArray(actual) && actual.some((r) => r?.passed === false));
    groups.get(id).push({
      part,
      path,
      passed: passed === true,
      status: passed === true ? 'passed' : (status ?? (failed ? 'failed' : 'incomplete')),
      actual,
    });
  };
  function checked(id, part, path, verify) {
    if (performanceArcs.includes(id)) {
      groups.get(id).push({ part, path: scopePath, historicalEvidence: path, passed: false, status: 'skipped', skipReason });
      return;
    }
    const v = load(path);
    const ok = v && verify(v);
    add(id, part, path, !!ok, v ? (ok ? 'passed' : 'failed') : 'pending');
  }
  const evaluated =
    load('candidate.evaluated.json')?.inputs?.sourceSHA256 ||
    load('candidate.json')?.inputs?.sourceSHA256;
  const packagedRoleSQL = load('deploy/offline-release.json')?.role_sql_sha256;
  const checkoutRoleSQL = currentRoleSQL();
  const sameRoleSQL = (actual) =>
    Boolean(
      packagedRoleSQL &&
        actual &&
        Object.keys(packagedRoleSQL).length === postgresRoleSQLFiles.length &&
        Object.keys(actual).length === postgresRoleSQLFiles.length &&
        postgresRoleSQLFiles.every(
          (path) =>
            /^[a-f0-9]{64}$/.test(packagedRoleSQL[path] ?? '') &&
            actual[path] === packagedRoleSQL[path] &&
            packagedRoleSQL[path] === checkoutRoleSQL[path],
        ),
    );
  const sharedConsole = sharedConsoleFor(dir);
  const visualRoot = visualOutputDirectory(dir, sharedConsole);
  const visualFile = name => relative(dir, join(visualRoot, name));
  const servingMatches = (report, side) => !sharedConsole || (report?.frontendAtEnd &&
    report.frontend?.identity === report.frontendAtEnd.identity &&
    verifiedVisualFrontend(sharedConsole,dir,report,side) &&
    verifiedVisualFrontend(sharedConsole,dir,{...report,frontend:report.frontendAtEnd},side));
  const visualEnvironments = ['visual/environment.json', 'visual/fixture-environment.json'].map(visualFile)
    .map(load)
    .filter(Boolean);
  const currentVisual = (report, fixtures = true) =>
    Boolean(
      evaluated &&
        servingMatches(report,'b') &&
        report?.candidate === evaluated &&
        sameRoleSQL(report?.roleSQL) &&
        visualEnvironments.some(
          (env) =>
            env.releaseSHA256 === evaluated &&
            env.consoleSHA256 === sharedConsole?.console.sha256 &&
            Boolean(env.fixtures) === fixtures &&
            report?.sourceDirectory === env.directory &&
            report.snapshot === env.snapshot &&
            env.snapshot === load('snapshot/manifest.json')?.pg.seed,
        ),
    );
  const frozen = load('baseline.json');
  for (const side of ['baseline','candidate']) {
    const path = 'identity/session-activity/' + side + '/result.json';
    const verified = verifySessionActivityResult(dir,side,side === 'baseline' ? frozen?.source_archive_sha256 : evaluated);
    for (const id of ['ARC-003','ARC-039'])
      checked(id,side + ' accepted activity, cache loss, idle and absolute expiry, and revocation',path,() => verified);
  }
  const publicEvidence = currentPublicEvidence(dir, evaluated);
  const currentSelf = (report, fixtures = false) => {
    const reference = load(
      visualFile(fixtures ? 'visual/frozen-fixture-self.json' : 'visual/frozen-self.json'),
    );
    return Boolean(
      reference &&
        reference.consoleSHA256 === sharedConsole?.console.sha256 &&
        reference.deliveryReleaseSHA256 === (sharedConsole ? report?.frontend?.deliveryReleaseSHA256 : undefined) &&
        servingMatches(report,'self') &&
        Boolean(reference.fixtures) === fixtures &&
        frozen &&
        report &&
        dirname(reference.directory) === dirname(dir) &&
        report.sourceDirectory === reference.directory &&
        report.baseline === frozen.commit &&
        report.baseline === reference.inputs.baseline &&
        report.snapshot === load('snapshot/manifest.json')?.pg.seed &&
        report.snapshot === reference.inputs.snapshot &&
        frozen.source_archive_sha256 === reference.inputs.archive &&
        digest(JSON.stringify(frozen.binaries)) === reference.inputs.binaries &&
        digest(JSON.stringify(frozen.staticFiles)) === reference.inputs.assets &&
        existsSync(join(reference.directory, 'candidate.json')) &&
        report.candidate === read(join(reference.directory, 'candidate.json')).inputs.sourceSHA256,
    );
  };
  function hasworkflow(id) {
    const records = paths('http/workflows')
      .concat(paths('workers'), paths('faults'))
      .flatMap((path) => {
        const v = load(path),
          list = Array.isArray(v) ? v : v.cases || [v];
        return Array.isArray(list)
          ? list.filter((r) => {
              if (r.arc !== id) return false;
              if (id === 'ARC-017' && currentVisual(v)) return true;
              if (v.candidateSHA256) return v.candidateSHA256 === evaluated && sameRoleSQL(v.appliedRoleSQL);
              return observedPublicFixture(dir, path, r.fixture ? { fixture: r.fixture } : v, evaluated, publicEvidence);
            }).map((r) => ({ ...r, path }))
          : [];
      });
    add(
      id,
      'paired-business-assertions',
      'http/workflows',
      records.length > 0 && records.every((r) => r.passed === true),
      records.map((r) => ({ name: r.name, passed: r.passed, path: r.path })),
    );
    return records;
  }
  for (const r of expected.filter((r) => Number(r.id.slice(4)) >= 3 && Number(r.id.slice(4)) <= 15))
    hasworkflow(r.id);
  const operations = load('http/operations.json');
  const seenWorkflows = writeHTTPCoverage(dir, publicEvidence);
  const opCoverage = seenWorkflows.operations;
  const all = opCoverage.length === 1071;
  const missingPositive = opCoverage.filter((op) => !op.positive);
  const missingNegative = opCoverage.filter(
    (op) => !['public', 'probe'].includes(op.access) && !op.negative,
  );
  checked('ARC-001', 'method-template-owner-operation', 'routes/registered.json', (v) =>
    Object.entries({
      api: 2,
      'identity-api': 158,
      'control-api': 796,
      'monitoring-query-api': 42,
      'transfer-api': 73,
      'compat-api': 1071,
    }).every(([role, count]) => v[role]?.length === count),
  );
  checked(
    'ARC-001',
    'anonymous gateway and owner parity',
    'anonymous-http.json',
    (v) => v.length === 1071 && new Set(v.map((r) => r.method + ' ' + r.template)).size === 1071,
  );
  add(
    'ARC-001',
    'valid public operations and rejections',
    'http/coverage.json',
    all &&
      !missingPositive.length &&
      !missingNegative.length &&
      observedPublicFixture(dir, 'http/operations.json', operations, evaluated, publicEvidence) &&
      operations?.totals.differences === 0,
    {
      operations: opCoverage.length,
      missingSuccess: missingPositive.map((o) => o.method + ' ' + o.template),
      missingExpectedError: missingNegative.map((o) => o.method + ' ' + o.template),
      differences: operations?.totals?.differences ?? null,
    },
    operations?.totals?.differences > 0 ? 'failed' : 'incomplete',
  );
  const regressions = Object.fromEntries(
    ['baseline', 'candidate'].map((s) => [s, readChecks(dir, s)]),
  );
  for (const side of ['baseline', 'candidate'])
    for (const name of ['test:http', 'test:https']) {
      const row = regressions[side].find((v) => v.command[1] === name);
      add(
        'ARC-002',
        side + ' ' + name,
        'regression/' + side + '/' + name.replaceAll(':', '-') + '.json',
        row?.status === 'passed',
        row?.status || 'pending',
      );
    }
  hasworkflow('ARC-002');
  if (sharedConsole) checked('ARC-002', 'same published console through each original Nginx rule', 'http/workflows/edges.json', report => {
    const checks = report.cases?.[0]?.checks?.filter(row =>
      row.transport === 'nginx' && ['console root', 'account deep link'].includes(row.name));
    return publicEvidence.has('http/workflows/edges.json') &&
      report.consoleSHA256 === sharedConsole.console.sha256 &&
      report.indexSHA256 === sharedConsole.console.files['index.html'] &&
      checks?.length === 2 && new Set(checks.map(row => row.name)).size === 2 &&
      checks.every(row => row.passed && [row.a, row.b].every(response =>
        response.status === 200 && response.body.sha256 === report.indexSHA256));
  });
  // An unrelated passing assertion cannot prove an absent domain suite.
  const requiredWorkflows = {
    '003': ['governance', 'governance-security'],
    '004': ['governance', 'governance-roles'],
    '005': ['authority-races', 'monitoring'],
    '006': ['governance'],
    '007': ['engineering-all', 'resources', 'catalog-definitions', 'catalog-taxonomy'],
    '008': ['engineering-all', 'catalog-devices', 'catalog-products'],
    '009': ['monitoring', 'engineering-all'],
    '010': ['monitoring', 'engineering-all'],
    '011': ['engineering-all', 'analysis', 'derived', 'authority-races'],
    '012': ['spatial'],
    '013': ['spatial', 'monitoring', 'display-files-both'],
    '014': ['audit-task', 'task-notifications-audit'],
    '015': ['display-files-both'],
  };
  for (const [number, files] of Object.entries(requiredWorkflows))
    for (const name of files) {
      const id = 'ARC-' + number;
      const path = 'http/workflows/' + name + '.json',
        v = load(path),
        target = v?.cases?.filter((r) => r.arc === id) ?? [];
      const complete = name !== 'authority-races' || (v?.complete === true && v.expected === 22);
      add(
        id,
        name,
        path,
        complete &&
          target.length > 0 &&
          target.every((r) => r.passed) &&
          observedPublicFixture(dir, path, v, evaluated, publicEvidence),
        target.map((r) => ({ name: r.name, passed: r.passed })),
      );
    }
  const invitations = visualFile('visual/invitation-notifications/b/latest.json');
  checked(
    'ARC-014',
    'personal invitation inboxes, accepted targets and cross-account denials',
    invitations,
    (pointer) => verifiedVisualInvitations(pointer, dirname(join(dir, invitations)), {
      baseline: frozen?.source_archive_sha256,
      candidate: evaluated,
      consoleSHA256: sharedConsole?.console.sha256 ?? null,
      sourceDirectory: load(visualFile('visual/fixture-environment.json'))?.directory,
      snapshot: load('snapshot/manifest.json')?.pg.seed,
      roleSQL: sameRoleSQL,
    }),
  );
  checked(
    'ARC-015',
    'trusted terminal and asset database permissions',
    'deploy/display-scope-permissions.json',
    (v) =>
      v.complete === true &&
      v.sealed === true &&
      v.releaseSHA256 === evaluated &&
      v.applicationSourceSHA256 === evaluated &&
      v.sourceDumpSHA256 === load('snapshot/manifest.json')?.pg.seed &&
      sameRoleSQL(v.appliedRoleSQL) &&
      ['display-runtime', 'display-assets'].every(
        (role) =>
          v.accounts?.[role]?.group &&
          !v.accounts[role].superuser &&
          !v.accounts[role].bypassRLS &&
          !v.accounts[role].canCreateRole,
      ),
  );
  checked(
    'ARC-015',
    'fixed analysis binding, terminal renewal and source revocation',
    'deploy/display-artifact-permissions.json',
    (v) => {
      const proof = v.outcome,
        revoked = proof?.revoked;
      return (
        v.complete === true &&
        v.sealed === true &&
        v.applicationSourceSHA256 === evaluated &&
        v.selectedReleaseSHA256 === evaluated &&
        v.sourceReleaseSHA256 === evaluated &&
        v.sourceDumpSHA256 === load('snapshot/manifest.json')?.pg.seed &&
        sameRoleSQL(v.appliedRoleSQL) &&
        v.afterMigrate === true &&
        v.outputRows > 0 &&
        v.privateSQL?.privateFixedBaselineEvidence === '42501' &&
        v.physicalObservations?.ownerRows > 0 &&
        [497, 516].includes(v.physicalObservations.code) &&
        proof?.complete === true &&
        proof.runtimeBinding?.ids?.includes(proof.version) &&
        revoked?.ownerQualification === revoked?.runtimeQualification &&
        ['ownerQualification', 'ownerPreparation', 'runtimeBinding', 'pair', 'renew'].every((step) =>
          /^4\d\d\/.+/.test(revoked?.[step] ?? ''),
        )
      );
    },
  );
  const visualDrivers = visualRecordingDrivers();
  const matchesRecording = (matrix) => matrix?.requestOrder === 'natural' &&
    Object.entries(visualDrivers).every(([name, hash]) => matrix[name] === hash);
  for (const side of ['self', 'b']) {
    const path = visualFile('visual/paired/' + side + '/matrix.json'),
      v = load(path);
    const ok =
      (side === 'self' ? currentSelf(v) : currentVisual(v, false)) &&
      matchesRecording(v) &&
      !!v?.complete &&
      v.passed &&
      v.rows.length === v.expectedRows;
    const failed = v?.rows?.some((row) => !row.passed);
    add(
      'ARC-016',
      side + ' screenshot matrix',
      path,
      ok,
      ok
        ? 'passed'
        : failed
          ? 'failed'
          : {
              status: 'pending',
              captured: v?.rows?.length || 0,
              total: v?.expectedRows,
            },
    );
    const features = visualFile('visual/feature-states/' + side + '/matrix.json'),
      extra = load(features);
    add(
      'ARC-016',
      side + ' shared layouts and authorized detail pages',
      features,
      (side === 'self' ? currentSelf(extra, true) : currentVisual(extra)) &&
        matchesRecording(extra) &&
        extra?.passed === true &&
        extra.complete &&
        extra.rows.length === extra.expectedRows,
      {
        captured: extra?.rows.length ?? 0,
        expected: extra?.expectedRows ?? null,
        failed: extra?.rows.filter((r) => !r.passed).map((r) => r.key) ?? [],
      },
    );
    checked(
      'ARC-016',
      side + ' live person and terminal screens',
      visualFile('visual/published-screens/' + side + '/matrix.json'),
      (value) =>
        (side === 'self' ? currentSelf(value, true) : currentVisual(value)) &&
        value.passed &&
        value.expectedRows === 6 &&
        value.rows.length === 6 &&
        value.rows.every(
          (r) => r.passed && r.changedPixels === 0 && r.sameData && r.sameAccessibility,
        ),
    );
  }
  checked(
    'ARC-016',
    'published static files and diagnostics entry references',
    visualFile('visual/static-assets.json'),
    (v) => v.passed && v.baseline === frozen?.commit && v.candidate === evaluated &&
      (!sharedConsole || (v.frontend?.consoleSHA256 === sharedConsole.console.sha256 &&
        v.frontend.backendReleaseSHA256 === evaluated &&
        v.frontend.deliveryReleaseSHA256 === sharedConsole.deliveryReleaseSHA256 &&
        JSON.stringify(v.frontend.sourceDiff) === JSON.stringify(sharedConsole.sourceDiff))),
  );
  const style = load(visualFile('visual/static-assets.json'))?.diagnosticsStyle;
  if(sharedConsole && style?.permittedAddition){
    for(const side of ['self','b']){
      const path=visualFile('visual/paired/'+side+'/matrix.json'),matrix=load(path);
      const rows=matrix?.rows.filter(row=>row.app==='diagnostics') ?? [];
      const observed=rows.map(row=>({
        key:row.key,template:row.template,width:row.viewport.width,passed:row.passed,
        matched:Object.values(row.diagnosticsStyle ?? {}).reduce((n,view)=>n+view.matching.length,0),
        animations:Object.values(row.diagnosticsStyle ?? {}).reduce((n,view)=>n+view.affectedAnimations,0),
        selectedStylesheets:['baseline','peer','fixedBaseline','fixedPeer'].every(view=>{
          const isNew=side==='b' && (view==='peer'||view==='fixedPeer');
          const file=isNew?style.candidate.file:style.original.file;
          return row.diagnosticsStyle?.[view]?.styles.some(sheet=>sheet.path.endsWith('/'+file) && sheet.selector===isNew);
        }),
        fixedPixels:row.fixed?.changedPixels,
      }));
      const complete=matrix?.complete && rows.length>=27;
      const ok=complete && (side==='self'?currentSelf(matrix):currentVisual(matrix,false)) &&
        style.original.sha256===style.candidateWithoutAdditionSHA256 && style.fonts.allIdentical &&
        observed.every(row=>row.passed && row.selectedStylesheets && row.matched===0 && row.animations===0 && row.fixedPixels===0);
      add('ARC-016',side+' diagnostic stylesheets, DOM usage and animation on three viewports',path,
        ok,{captured:observed.length,expected:27,matchingDOM:observed.reduce((n,r)=>n+r.matched,0),
          affectedAnimations:observed.reduce((n,r)=>n+r.animations,0),rows:observed},complete&&!ok?'failed':'incomplete');
    }
  }
  const currentRoleDiagnostics = (v) =>
    v.passed === true && !!evaluated && v.release?.source_sha256 === evaluated;
  const catalogPath = 'contracts/diagnostics/runtime-catalog.json';
  const frozenCatalog = frozen?.source && join(frozen.source, catalogPath);
  const frozenRoles = new Set(frozenCatalog && existsSync(frozenCatalog) ? read(frozenCatalog).entries.map(r => r.id) : []);
  const addedRoles = read(join(root, catalogPath)).entries.map(r => r.id).filter(id => !frozenRoles.has(id)).sort();
  checked('ARC-016', 'new role diagnostics on real data', 'diag-role-faults.json', currentRoleDiagnostics);
  checked(
    'ARC-016',
    'live diagnostic role and process rows in all three viewports',
    visualFile('visual/live-topology/result.json'),
    (v) =>
      currentVisual(v) &&
      v.passed &&
      JSON.stringify(v.addedRoles) === JSON.stringify(addedRoles) &&
      (!sharedConsole || [
        [v.servedStyles?.a, style?.original],
        [v.servedStyles?.b, style?.candidate],
      ].every(([actual, sealed]) => actual && sealed && actual.file === sealed.file &&
        actual.sha256 === sealed.sha256 && actual.bytes === sealed.bytes)) &&
      v.rows.length === 3 &&
      v.rows.every(
        (row) =>
          row.passed &&
          JSON.stringify(row.listed?.map(role => role.id).sort()) === JSON.stringify(addedRoles) &&
          row.listed.every(role => role.expectation && role.state && role.visible?.cells?.length === 5 &&
            row.screenshots.includes(role.screenshot)) &&
          row.roles.length === 8 &&
          row.roles.every((role) => role.hasProcessIdentity),
      ),
  );
  if (!sharedConsole) hasworkflow('ARC-017');
  if (sharedConsole?.beforeImportFocus) {
    const focusPath = 'console-corrections/import-focus.json';
    const fileMatches = file => file?.file?.startsWith('console-corrections/import-focus/') &&
      existsSync(join(dir,file.file)) && digest(readFileSync(join(dir,file.file))) === file.sha256;
    const matchesFocus = report => {
      if (!report?.passed || report.consoleSHA256 !== sharedConsole.console.sha256 ||
          !sameRoleSQL(report.appliedRoleSQL)) return false;
      const names = ['keyboardEarly','keyboardLate','mouseEarly','mouseLate','userTab','userPointer',
        'rowMissing','filterChanged','scopeChanged','unmounted'];
      return ['a','b'].every(side => {
        const view = report.sides?.[side], assets = view?.assets;
        if (!view?.passed || view.backendSHA256 !== (side === 'a' ? frozen?.source_archive_sha256 : evaluated) ||
            !assets?.matched || !fileMatches(assets.artifact) || !assets.html?.includes('index.html') ||
            !assets.css?.length || !assets.js?.length) return false;
        const manifest = read(join(dir,assets.artifact.file));
        if (assets.checked !== Object.keys(manifest).length || consoleDigest(manifest) !== assets.sha256 ||
            !Object.entries(manifest).every(([file,sha]) => sharedConsole.console.files[file] === sha)) return false;
        return names.every(name => {
          const row = view.cases?.[name];
          if (!row?.passed || !row.expectedFocus || row.expectedFocus !== row.actualFocus || !row.responses?.length ||
              !row.responses.every(reply => reply.status === 200 && /^[a-f0-9]{64}$/.test(reply.sha256)) ||
              row.artifacts?.length < 2 || !row.artifacts?.every(fileMatches)) return false;
          if (/Early$|Late$/.test(name))
            return row.returnedRowVisible && /^import-[a-f\d-]{36}$/.test(row.actualFocus) &&
              row.listReturnedAfterClose === name.endsWith('Late');
          return name !== 'rowMissing' || row.returnedRowVisible === false;
        });
      });
    };
    const verified = matchesFocus(load(focusPath));
    for (const id of ['ARC-016','ARC-017','ARC-039'])
      checked(id,'monitoring import keyboard and pointer focus after refreshed history',focusPath,() => verified);
  }
  if (sharedConsole) {
    const crossDirectory = join(visualRoot, 'visual/cross-release/same-responses');
    checked('ARC-016', 'frozen and shared console with identical live inputs on fifteen screens',
      visualFile('visual/cross-release/same-responses/latest.json'), pointer =>
        verifiedCrossConsole(pointer, crossDirectory, frozen, sharedConsole, dir) &&
        currentVisual(read(pointer.result)));
    checked('ARC-016', 'governance pagination on both backends in four viewports and two themes',
      'console-corrections/pagination.json', (v) => {
        if (!v.passed || v.consoleSHA256 !== sharedConsole.console.sha256) return false;
        return ['a', 'b'].every(side => {
          const result = v.sides?.[side];
          const expected = side === 'a' ? frozen?.source_archive_sha256 : evaluated;
          if (!expected || result?.backendSHA256 !== expected || result.consoleSHA256 !== v.consoleSHA256 ||
              result.servedFiles !== Object.keys(sharedConsole.console.files).length || result.rows?.length !== 8 ||
              result.projectIds?.length !== 11 || new Set(result.projectIds).size !== 11) return false;
          const views = new Set([375, 768, 1024, 1440].flatMap(width => ['light', 'dark'].map(theme => [width, theme].join('/'))));
          return result.rows.every(row => {
            const screenshot = row.screenshot?.file && join(dir, 'console-corrections', result.screenshotDirectory, row.screenshot.file);
            const pageIds = [...(row.firstPageIds ?? []), row.nextPageId];
            return views.delete([row.width, row.theme].join('/')) && row.scrollWidth === row.width &&
              row.listStatus === 200 && row.geometry?.pager?.width > 0 && row.menuClick?.reached &&
              row.menuClick.horizontalOffset === 0 && row.geometry.buttons?.length > 0 &&
              row.geometry.buttons.every(b => b.x >= 0 && b.right <= row.width) &&
              row.mouse && row.keyboard && row.nextPage && row.previousPage && new Set(row.firstPage).size === 10 &&
              row.firstPage?.length === 10 && row.nextPageProject && !row.firstPage.includes(row.nextPageProject) &&
              pageIds.length === 11 && new Set(pageIds).size === 11 && pageIds.every(id => result.projectIds.includes(id)) && screenshot &&
              existsSync(screenshot) && digest(readFileSync(screenshot)) === row.screenshot.sha256;
          }) && !views.size;
        });
      });
    checked('ARC-017', 'member and invitation drafts survive revalidation and clear on real revocation',
      'console-corrections/drafts.json', (v) => {
        if (!v.passed || !v.identical_assets || v.consoleSHA256 !== sharedConsole.console.sha256) return false;
        return ['a', 'b'].every(side => {
          const result = v.sides?.[side];
          const expected = side === 'a' ? frozen?.source_archive_sha256 : evaluated;
          if (!expected || result?.backendSHA256 !== expected || result.consoleSHA256 !== v.consoleSHA256 ||
              !result.passed || !result.no_draft_persisted || result.cases?.length !== 2) return false;
          const actions = new Set(['member-suspend', 'invitation-regenerate']);
          return result.cases.every(row => actions.delete(row.name) && row.passed && row.sameScopeDraftRetained &&
            row.submitBlockedDuringCheck && row.revocationClearedDraft && row.serverDenied === 403 &&
            row.browserBusinessSubmissions === 0) && !actions.size &&
            ['http', 'browser'].every(name => {
              const evidence = result.evidence?.[name];
              return evidence?.file && existsSync(join(dir, evidence.file)) &&
                digest(readFileSync(join(dir, evidence.file))) === evidence.sha256;
            });
        });
      });
  }
  checked(
    'ARC-017',
    'three actual browser navigation pairs on the selected release',
    visualFile('http/workflows/navigation.json'),
    (v) =>
      currentVisual(v) &&
      v.cases?.some(
        (c) =>
          c.arc === 'ARC-017' &&
          c.passed &&
          c.checks.length === 3 &&
          c.checks.every(
            (pair) =>
              pair.passed && pair.a.steps.length > 0 && pair.a.steps.length === pair.b.steps.length,
          ),
      ),
  );
  for (const id of ['018', '019', '020', '021', '026', '028'])
    checked(
      'ARC-' + id,
      'frozen and candidate worker behavior',
      'worker-checks/summary.json',
      (v) =>
        !!evaluated && v.candidate_source_sha256 === evaluated && v.criteria?.['ARC-' + id]?.passed,
    );
  for (const id of ['022', '029', '030', '031']) hasworkflow('ARC-' + id);
  checked(
    'ARC-022',
    'four durable consumers and authorized screen values',
    'faults/change-recovery.json',
    (report) => {
      const row = report.cases?.find((v) => v.arc === 'ARC-022');
      if (!row?.passed || !evaluated || report.candidateSHA256 !== evaluated) return false;
      const v = row.checks,
        groups = ['derived', 'summary', 'query-cache', 'display-cache'];
      const screen = (result, values) =>
        result?.a &&
        result.b &&
        result.a !== result.b &&
        JSON.stringify(result.rows.map((r) => r.value)) === JSON.stringify(values);
      return (
        v.tail > v.start &&
        groups.every(
          (name) => v.reached?.[name] === v.tail && v.executions?.[name] === v.tail - v.start,
        ) &&
        v.derivedInputPosition === v.tail &&
        v.cacheFloors?.['query-cache'] > 0 &&
        v.cacheFloors?.['display-cache'] > 0 &&
        screen(v.display?.before, [40]) &&
        screen(v.display?.after, [13, 42]) &&
        v.display.failures.length === 0
      );
    },
  );
  checked(
    'ARC-029',
    'cold upgrade in-flight work from schema 58',
    'faults/legacy-work.json',
    (report) => {
      const cases = report.cases?.filter((r) => r.arc === 'ARC-029') || [];
      if (
        !evaluated ||
        report.candidateSHA256 !== evaluated ||
        cases.length !== 5 ||
        cases.some((r) => !r.passed)
      )
        return false;
      const rows = cases.map((r) => r.checks);
      return (
        rows.some(
          (v) =>
            v.schemaBefore === 58 &&
            v.schemaAfter >= 59 &&
            v.artifactVersion &&
            v.values?.length === 10,
        ) &&
        rows.some(
          (v) => v.run && v.generation && v.method?.artifact_digest && v.formallyAdopted === false,
        ) &&
        rows.some((v) => v.job && v.generation && v.sameUpload && v.sha256) &&
        rows.some((v) => v.batch && v.job && v.sourceFile && v.sha256 && Number(v.proposed) > 0) &&
        rows.some((v) => v.packageJob && v.package && v.archiveSHA256 && v.terminal && v.release)
      );
    },
  );
  checked(
    'ARC-031',
    'ClickHouse write before PostgreSQL commit',
    'faults/change-recovery.json',
    (report) => {
      const row = report.cases?.find((r) => r.arc === 'ARC-031');
      return (
        row?.passed &&
        ['a', 'b'].every((side) => {
          const facts = row.checks?.physical?.[side];
          return (
            facts?.storedRows > 0 &&
            facts.selectedPublications === 0 &&
            facts.selectedAfterRepair === 1
          );
        })
      );
    },
  );
  checked(
    'ARC-031',
    'stream interruption, S3 write and visible receipts',
    'http/workflows/spatial.json',
    (report) => {
      const spatial = load('http/spatial-parity.json');
      if (
        !evaluated ||
        spatial?.candidateSHA256 !== evaluated ||
        spatial.sourceRun !== report.sourceRun
      )
        return false;
      const rows = report.cases?.filter((r) => r.arc === 'ARC-031' && r.passed) || [];
      const facts = rows.flatMap((r) => r.checks || []);
      const partial = facts.find((v) => v.firstBodyBytes),
        commit = facts.find((v) => v.failedCommit)?.failedCommit;
      const final = facts.find((v) => v.sourceAndArtifactSetSHA256);
      const zeroInBoth = (sides) => sides?.length === 2 && sides.every((value) => value === 0);
      return (
        partial?.firstBodyBytes?.length === 2 &&
        partial.firstBodyBytes.every((size) => size > 0 && size < partial.wholeBytes) &&
        commit?.afterObjectWrite &&
        zeroInBoth(commit.visibleParts) &&
        zeroInBoth(commit.orphanTemporaryObjects) &&
        commit.retryAndReplay &&
        final?.effects?.openTransfers === 0 &&
        final.effects.invalidReferences === 0 &&
        final.sourceAndArtifactCount > 0 &&
        final.interrupted?.length === 2 &&
        final.interrupted.every(
          (item, i) =>
            item.facts === 2 &&
            item.sent >= partial.firstBodyBytes[i] &&
            item.sent <= partial.wholeBytes &&
            ['interrupted', 'error', 'complete'].includes(item.result),
        )
      );
    },
  );
  const eventTests = {
    '023': ['TestTransactionalOutboxFaults'],
    '024': [
      'TestAllStreamDiskWatermarks',
      'TestContinuousPositionsAndBackpressure',
      'TestMTLSRuntimeFallback',
    ],
    '025': ['TestContinuousPositionsAndBackpressure', 'TestFinalDeliverySupervision'],
    '026': [
      'TestEpochAndDispatchFences',
      'TestIdentifyHandoffResumesAfterLostExecutor',
      'TestParseHandoffResumesAfterLostExecutor',
    ],
    '030': ['TestRestrictedDatabaseRoles', 'TestMTLSRuntimeFallback'],
    '036': ['TestJetStreamQuorumAndCapacity'],
  };
  for (const [number, names] of Object.entries(eventTests))
    checked(
      'ARC-' + number,
      'real event delivery and PostgreSQL',
      'events/checks.json',
      (v) =>
        v.passed &&
        !!evaluated &&
        v.provenance?.sourcesUnchanged &&
        v.provenance.expectedSHA === evaluated &&
        v.provenance.after?.applicationSHA256 === evaluated &&
        names.every((n) => v.cases?.some((c) => c.name === n && c.state === 'pass')),
    );
  const faultRecords = hasworkflow('ARC-027');
  checked(
    'ARC-027',
    'file checker outage on the selected release',
    'faults/checker-outage.json',
    (v) =>
      !!evaluated &&
      v.candidateSHA256 === evaluated &&
      v.cases?.some((c) => c.arc === 'ARC-027' && c.passed),
  );
  checked(
    'ARC-027',
    'cold identity, independent diagnostics and published import',
    'faults/http-isolation.json',
    (v) => {
      const checks = v.cases?.[0]?.checks ?? [];
      return (
        !!evaluated &&
        v.cases?.[0]?.fixture?.candidateSHA256 === evaluated &&
        checks.length === 4 &&
        checks.every((r) => r.passed) &&
        checks.some((r) => r.tenantLedger?.facilities > 0) &&
        checks.some(
          (r) =>
            r.diagnosticLogin === 200 &&
            r.diagnosticOverview === 200 &&
            r.protected?.status === 503,
        ) &&
        checks.some((r) => r.state === 'published')
      );
    },
  );
  for (const [service, pattern] of [
    ['parser', /parser|解析服务/i],
    ['file-checker', /checker|文件检查器/i],
    ['event-relay', /event-relay|转发器/i],
  ]) {
    const records = faultRecords.filter((r) => pattern.test(r.name));
    add(
      'ARC-027',
      service + ' stopped with active unaffected requests',
      records[0]?.path ?? 'faults',
      records.length > 0 && records.every((r) => r.passed),
      records.map((r) => r.name),
    );
  }
  for (const id of ['ARC-033', 'ARC-034']) {
    checked(id, 'schema 58 and 59 clean-cluster restore permissions', 'deploy/cold-restore-grants.json', (v) => {
      const stages = v.stages;
      return (
        v.complete === true && v.candidate === evaluated &&
        v.seedSHA256 === load('snapshot/manifest.json')?.pg.seed && sameRoleSQL(v.appliedRoleSQL) &&
        stages?.emptyCluster?.schema === 58 && stages.emptyCluster.auditGroupInitiallyPresent === false &&
        stages.emptyCluster.controlGroupInitiallyPresent === false &&
        ['upgrade', 'migrate', 'provision'].every((step) => {
          const row = stages[step];
          return row?.schema === 59 && row.accountDigest === stages.emptyCluster.accountDigest &&
            row.epoch === stages.upgrade?.epoch && row.permissions?.infrapro_audit_worker === true &&
            row.permissions.infrapro_display_runtime === true && row.permissions.infrapro_display_assets === false &&
            row.permissions.arc_acl_reader === false && row.personalInvitationReader?.infrapro_control_api === true &&
            row.personalInvitationReader.infrapro_query_api === false && row.personalInvitationReader.infrapro_notification_worker === false &&
            row.personalInvitationReader.arc_acl_reader === false &&
            (step === 'upgrade' || (row.defaultExecuteBefore === true && row.personalInvitationDefaultExecuteBefore === true));
        })
      );
    });
  }
  const deploymentStages = {
    '032': [
      'scheduler-privileges',
      'domain-privileges',
      'native-template',
      'compose-template',
      'mixed-template',
      'live-native',
      'live-compose',
      'live-mixed',
      'replica-quotas',
      'object-privileges',
    ],
    '033': ['controlled-write-window', 'schema-upgrade', 'all-owners'],
    '034': ['empty-target-restore', 'joint-recovery'],
    '035': ['gateway-fallback', 'worker-fallback'],
  };
  for (const [number, required] of Object.entries(deploymentStages)) {
    const id = 'ARC-' + number,
      v = load('deploy/deployment.json');
    const stages = (v?.checks || v?.stages || []).filter((r) => r.arc === id);
    const witnessed = stages
      .filter((r) => r.status !== 'failed' && r.passed !== false)
      .map((r) => r.key);
    add(
      id,
      'release and restore rehearsal',
      'deploy/deployment.json',
      v?.status === 'passed' &&
        !!evaluated &&
        v.release?.source_sha256 === evaluated &&
        required.every((key) => witnessed.includes(key)),
      {
        verified: witnessed,
        missing: required.filter((key) => !witnessed.includes(key)),
      },
      v?.status === 'failed' ? 'failed' : 'incomplete',
    );
  }
  checked(
    'ARC-032',
    'native positive and failure probes',
    'diag-role-faults.json',
    currentRoleDiagnostics,
  );
  checked(
    'ARC-032',
    'verified offline Linux release and retained parser',
    'deploy/offline-release.json',
    (v) =>
      v.verified === true &&
      !!evaluated &&
      v.host_release?.application_source_sha256 === evaluated &&
      v.application_source_sha256 === evaluated &&
      sameRoleSQL(v.role_sql_sha256) &&
      v.schema === 59 &&
      v.goos === 'linux' &&
      v.role_entrypoints >= 31 &&
      v.file_count > v.role_entrypoints &&
      /^[a-f0-9]{64}$/.test(v.manifest_sha256),
  );
  checked(
    'ARC-034',
    'combined PG, relay, CH, objects and recovery epoch',
    'deploy/joint-recovery.json',
    (v) => {
      const roleGrants = v.steps?.role_grants?.result;
      const installation = roleGrants?.schema_installation;
      const guard = v.steps?.monitoring?.result;
      const projection = installation?.after?.baseline_function;
      const invitation = installation?.after?.notification_invitation_function;
      const upgradedSchema = load('deploy/deployment.json')?.checks?.find(
        (check) => check.key === 'schema-upgrade',
      );
      const upgraded = upgradedSchema?.baselineMetadata;
      const upgradedInvitation = upgradedSchema?.invitationMetadata;
      const cli = load('deploy/offline-release.json')?.host_release?.verified_host_binaries?.infrapro;
      const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/;
      const epochs = ['audit', 'event', 'files', 'tasks'];
      return (
        v.status === 'passed' &&
        !!evaluated &&
        v.release?.source_sha256 === evaluated &&
        sameRoleSQL(v.applied_role_sql) &&
        sameRoleSQL(v.cold_starts?.appliedRoleSQL) &&
        sameRoleSQL(roleGrants?.applied_role_sql) &&
        /^[0-9a-f]{64}$/.test(cli ?? '') &&
        installation?.cli_sha256 === cli &&
        uuid.test(guard?.backup_id ?? '') &&
        guard.mode === 'read_only' &&
        guard.access_revoked === true &&
        [installation.before, installation.after].every((s) =>
          s?.schema === 59 &&
          s.monitoring?.backup_id === guard.backup_id &&
          s.monitoring.mode === guard.mode &&
          s.monitoring.access_revoked === true &&
          Object.keys(s.epochs ?? {}).length === epochs.length &&
          epochs.every(
            (domain) =>
              uuid.test(s.epochs[domain] ?? '') &&
              s.epochs[domain] === installation.before.epochs[domain],
          ),
        ) &&
        projection?.name === 'architecture_fixed_baseline_metadata' &&
        projection.arguments === 'uuid, uuid, text, text' &&
        projection.result === 'TABLE(snapshot jsonb, input_ranges jsonb)' &&
        projection.security_definer === true &&
        /^[0-9a-f]{64}$/.test(upgraded?.sha256 ?? '') &&
        projection.definition_sha256 === upgraded.sha256 &&
        invitation?.name === 'architecture_notification_invitation_targets' &&
        invitation.arguments === 'uuid, uuid, uuid[]' &&
        invitation.result === 'TABLE(invitation_id uuid, is_recipient boolean, inviter_id text, inviter_membership_id text, effective_status text, membership_status text, accepted_tenant_name text)' &&
        invitation.security_definer && invitation.fixed_search_path && invitation.public_execute === false &&
        upgradedInvitation?.controlCanExecute && upgradedInvitation.queryCanExecute === false &&
        upgradedInvitation.workerCanExecute === false &&
        /^[0-9a-f]{64}$/.test(upgradedInvitation.sha256) &&
        invitation.definition_sha256 === upgradedInvitation.sha256 &&
        v.keys?.before &&
        v.keys?.after &&
        ['source', 'files', 'parsing', 'audit'].every((name) => v.object_counts?.[name] > 0) &&
        v.before_open === 'verified_read_only' &&
        v.after_open === 'ready'
      );
    },
  );
  checked(
    'ARC-035',
    'GUI, SDK, CLI and accepted writes after each cutback',
    'deploy/rollback-readback.json',
    (v) =>
      v.status === 'passed' &&
      !!evaluated &&
      v.candidateSHA256 === evaluated &&
      sameRoleSQL(v.role_grants) &&
      ['before-cutover', 'partial-http', 'after-worker-switch'].every((phase, index) =>
        v.phases?.some(
          (row) =>
            row.phase === phase &&
            row.accepted?.publish_job &&
            row.sdk?.rows?.length === index + 1 &&
            row.readbacks?.length === index + 1 &&
            row.screenshot &&
            row.readbacks.every((r) => r.batch && r.gui && r.replayed_job && r.effects?.jobs === 1),
        ),
      ),
  );
  checked(
    'ARC-037',
    'three modes, three verified end-to-end rounds',
    'ingestion/comparison.json',
    (v) =>
      v.passed === true &&
      !!evaluated &&
      v.candidate_release?.source_sha256 === evaluated &&
      /^[a-z][a-z0-9]{0,20}$/.test(v.campaign ?? '') &&
      sameRoleSQL(
        load('ingestion/campaigns/' + v.campaign + '/registration.json')?.grant_sql_sha256,
      ) &&
      ['a', 'pg', 'mq'].every(
        (mode) =>
          v.rounds?.filter((r) => r.mode === mode && r.passed && r.performance_eligible).length >=
          3,
      ) &&
      v.gates?.business_keys_and_values &&
      v.gates.isolated_worker_database_roles &&
      v.gates.independent_round_databases &&
      v.gates.observed_pg_connection_limits &&
      v.gates.mq_net_benefit &&
      v.gates.pg_end_to_end_and_catchup &&
      v.gates.mq_end_to_end_and_catchup,
  );
  checked(
    'ARC-038',
    '18 paired monitoring and screen scenarios',
    'load/display/comparison.json',
    (v) => {
      if (
        !v.passed ||
        !sameRoleSQL(v.provenance?.roleSQL) ||
        !/^[a-f0-9-]{36}$/.test(v.campaign ?? '')
      )
        return false;
      const archive = 'load/display/campaigns/' + v.campaign + '/';
      const campaign = load(archive + 'manifest.json');
      const infrastructure = load(archive + 'infrastructure.json')?.resources;
      const profile = load(archive + 'resource-profile.json');
      const release = load(load('candidate.evaluated.json')?.manifest ?? 'candidate.json');
      const binaries = {
        a: load('baseline.json')?.binaries,
        b: release?.binaries,
      };
      if (
        !profile ||
        digest(JSON.stringify(profile)) !== v.provenance?.resourceProfile ||
        !(profile.hardware?.host?.logicalCPUs > 0) ||
        !(profile.hardware?.docker?.cpus > 0) ||
        JSON.stringify(profile.total) !== JSON.stringify([48, 18, 13, 2048]) ||
        profile.objectRequests !== 16 ||
        release?.inputs?.sourceSHA256 !== evaluated ||
        JSON.stringify(campaign?.provenance) !== JSON.stringify(v.provenance)
      )
        return false;
      if (
        infrastructure?.length !== 6 ||
        !infrastructure.every((r) => /^sha256:[a-f0-9]{64}$/.test(r.image)) ||
        !infrastructure.some(
          (r) =>
            r.name.startsWith('arc-display-postgres-') &&
            r.cpuNano === 4000000000 &&
            r.memoryBytes === 1536 * 1024 * 1024,
        )
      )
        return false;
      if (
        campaign?.provenance?.candidate !== evaluated ||
        campaign.scope.rounds !== 3 ||
        campaign.scope.onlySide ||
        campaign.scope.onlyScene
      )
        return false;
      const expected = new Set(
        ['a', 'b'].flatMap((side) =>
          [1, 2, 3].flatMap((round) =>
            ['idle', 'history', 'mixed'].map((scene) => [side, round, scene].join('/')),
          ),
        ),
      );
      const databases = new Set(),
        buckets = new Set();
      for (const trial of v.rounds ?? []) {
        const key = [trial.side, trial.round, trial.scene].join('-');
        if (!/^[ab]-[1-3]-(idle|history|mixed)$/.test(key) || trial.receipt !== key + '.json')
          return false;
        const row = load(archive + trial.receipt);
        if (trial.side === 'b') {
          const logins = row?.budget?.roleLogins;
          if (
            !logins ||
            logins.total !== 48 ||
            logins.gatewayDatabase ||
            Object.keys(logins.accounts).length !== 10 ||
            !Object.values(logins.accounts).every(
              (item) => item.authenticated && item.nonprivileged && item.limit > 0,
            ) ||
            JSON.stringify(logins.appliedRoleSQL) !== JSON.stringify(v.provenance.roleSQL)
          )
            return false;
        }
        if (
          !row?.passed ||
          row.signature !== v.signature ||
          row.campaign !== v.campaign ||
          !measuredDisplayBudget(row, profile, binaries[trial.side]) ||
          JSON.stringify(row.infrastructure) !== JSON.stringify(infrastructure) ||
          !row.measurementStart ||
          row.failures?.length ||
          !row.samples.cold.every(
            (r) => r.ok && r.ms <= 120000 && r.observations === 100000 && r.channels === 600,
          ) ||
          !row.samples.stream.every(
            (r) => r.ok && r.bytes > 32 * 1024 * 1024 && r.chunks > 1 && r.firstByteMs >= 0,
          ) ||
          row.execution?.length !== 10 ||
          !row.execution.every(
            (job) => job.state === 'done' && job.queue_ms >= 0 && job.execution_ms >= 0,
          ) ||
          !row.resources.length ||
          row.resources.some((r) => r.error) ||
          !row.resources.some((r) => r.pg && r.machine)
        )
          return false;
        databases.add(row.database);
        buckets.add(row.bucket);
      }
      return (
        v.passed === true &&
        !!evaluated &&
        v.provenance?.candidate === evaluated &&
        v.provenance?.baseline === load('baseline.json')?.commit &&
        v.rounds?.length === 18 &&
        databases.size === 18 &&
        buckets.size === 18 &&
        v.rounds.every(
          (r) =>
            r.passed &&
            expected.delete([r.side, r.round, r.scene].join('/')) &&
            r.metrics.hot.count >= 30 &&
            r.metrics.directory.count >= 30 &&
            r.metrics.cold.count >= 10 &&
            r.metrics.hot.p95Ms <= 2000 &&
            r.metrics.directory.p95Ms <= 2000,
        ) &&
        expected.size === 0 &&
        ['curve', 'directory', 'stream'].every(
          (metric) =>
            v.facts?.[metric]?.identical &&
            v.facts[metric].responses === (metric === 'stream' ? 108 : 540),
        ) &&
        v.relative?.length === 22 &&
        v.relative.every((r) => r.passed && r.ratio <= 1.1)
      );
    },
  );
  for (const [side, rows] of Object.entries(regressions))
    for (const row of rows) {
      const name = row.command[1];
      for (const stage of performanceStages[name] || [])
        groups.get('ARC-039').push({ part: side + ' ' + name + ' / ' + stage, path: scopePath, passed: false, status: 'skipped', skipReason });
      if (row.status === 'skipped') {
        groups.get('ARC-039').push({ part: side + ' ' + name, path: scopePath, passed: false, status: 'skipped', skipReason, previousRecord: row.previousRecord });
        continue;
      }
      add(
        'ARC-039',
        side + ' ' + name,
        row.evidencePath || 'regression/' + side + '/' + name.replaceAll(':', '-') + '.json',
        row.status === 'passed',
        row.defaultOutcome ? {status: row.status, defaultOutcome: row.defaultOutcome, qualification: row.qualificationEvidence} : row.status,
      );
    }
  checked(
    'ARC-039',
    'exact response for an unscoped tenant administrator candidate',
    'http/scenarios/governance-roles.json',
    (v) => {
      if (!publicEvidence.has('http/scenarios/governance-roles.json')) return false;
      const row = v.records?.find(item => item.scenario === 'unscoped_tenant_admin_candidate');
      const effects = row?.effects;
      return row?.operationId === 'tenantCandidates' && row.method === 'GET' &&
        row.template === '/api/v1/platform/tenant-admin-candidates' && row.equal === true &&
        JSON.stringify(row.status) === '[200,200]' && effects?.matchesRequestedAccount === true &&
        effects.hasAvatar === false && effects.avatarRevision === '' &&
        JSON.stringify(effects.replacedRoles) === '[]' &&
        JSON.stringify(effects.fields) === JSON.stringify([
          'avatar_revision', 'display_name', 'has_avatar', 'id', 'replaced_roles', 'username',
        ]);
    },
  );
  checked(
    'ARC-039',
    'published audit metadata grants and private column boundaries on both releases',
    'regression/audit-role-assembly.json',
    (v) => {
      const frozen = load('baseline.json'), offline = load('deploy/offline-release.json');
      const before = v.sides?.baseline, after = v.sides?.candidate;
      const frozenFile = 'deploy/production/schema-55-audit-role-completion.sql';
      if (!v.passed || v.remaining?.length !== 0 || !before || !after || !offline ||
          v.seedSHA256 !== load('snapshot/manifest.json')?.pg?.seed ||
          before.schema !== 58 || before.releaseSHA256 !== frozen?.source_archive_sha256 ||
          before.applied?.[frozenFile] !== frozen?.sourceFiles?.[frozenFile] ||
          after.schema !== 59 || after.releaseSHA256 !== evaluated ||
          after.publication !== offline.release || after.publicationSHA256 !== offline.manifest_sha256 ||
          !postgresRoleSQLFiles.every(file => after.applied?.[file] === offline.role_sql_sha256?.[file])) return false;
      return [before, after].every(row => {
        const actor = row.after;
        return row.sourceAtCompletion?.matchesSelectedRelease &&
          row.sourceAtCompletion.sourceSHA256 === row.releaseSHA256 &&
          row.before?.engineeringReadSQLSTATE === '42501' &&
          actor?.membership === 'infrapro_audit_worker' && actor.sessionUser === actor.currentUser &&
          actor.superuser === false && actor.bypassRLS === false && actor.canCreateRole === false &&
          actor.entity?.kind === 'facility' && !!actor.entity.id &&
          ['passwordRead', 'accountWrite', 'ingestionBody'].every(name => actor.negative?.[name] === '42501');
      }) && ['id', 'tenant', 'facility', 'metadataMD5'].every(name =>
        before.after.entity[name] === after.after.entity[name]);
    },
  );
  checked(
    'ARC-039',
    'cancel a running database query and replay its command on both releases',
    'regression/derived-cancelled-query.json',
    (v) => {
      if (!v.passed || v.remainingContainers?.length !== 0) return false;
      const fixture = readFileSync(join(root, 'scripts/fixtures/architecture-derived-cancel.go.inc'));
      if (v.fixtureSHA256 !== digest(fixture)) return false;
      return ['baseline', 'candidate'].every((side) => {
        const row = v.sides?.[side];
        const source = side === 'baseline' ? load('baseline.json')?.source_archive_sha256 : evaluated;
        if (!source || row?.releaseSHA256 !== source || row.sourceAtCompletion?.sourceSHA256 !== source ||
            !row.sourceAtCompletion.matchesSelectedRelease || row.status !== 'passed' || row.exitCode !== 0 ||
            !Number.isInteger(row.activeBackendPID) || row.activeBackendPID < 1 || row.activeAfterCancel !== false ||
            row.terminalState !== 'cancelled' || !row.replayedSameCommand || row.matchingReceipts !== 1 ||
            ![row.runID, row.jobID, row.requestID].every(id => /^[a-f0-9-]{36}$/.test(id))) return false;
        const log = join(dir, row.log);
        if (!existsSync(log)) return false;
        const raw = readFileSync(log, 'utf8');
        const observation = 'ARCH_PG_CANCEL run=' + row.runID + ' job=' + row.jobID +
          ' request=' + row.requestID + ' active_pid=' + row.activeBackendPID +
          ' active_after=false state=cancelled replayed=true receipts=1';
        return digest(raw) === row.logSHA256 && raw.includes(observation) &&
          raw.includes('--- PASS: TestArchitectureCancelActiveDerivedSQL');
      });
    },
  );
  checked(
    'ARC-039',
    'original diagnostic command on both immutable releases',
    'diagnostics-comparison.json',
    (v) => {
      const verified = (release) =>
        release?.exit_code === 0 && release.suites === 13 && release.screenshots === 9;
      const cases = (group, count) =>
        group?.length === count &&
        new Set(group.map((row) => row.case)).size === count &&
        group.every((row) => row.result === 'passed');
      const exactPrograms = (side) => {
        const evidence = v.release_binaries?.[side],
          release = v[side];
        if (!evidence?.qualified || !evidence.phases?.length) return false;
        return evidence.phases.every((phase) => {
          if (
            !['parked', 'after_command'].includes(phase.phase) ||
            !phase.passed ||
            phase.expected !== (side === 'baseline' ? 15 : 14) ||
            phase.matched !== phase.expected ||
            phase.evidence !==
              'regression/' + side + '/diagnostics-binaries-' + phase.phase + '.json'
          )
            return false;
          const raw = load(phase.evidence);
          return (
            raw?.passed &&
            raw.run_id === release.run_id &&
            raw.source === release.source &&
            raw.aggregate_finished_at === release.finished_at &&
            raw.phase === phase.phase &&
            digest(readFileSync(join(dir, phase.evidence))) === phase.evidence_sha256 &&
            raw.programs.length === phase.expected &&
            new Set(raw.programs.map((item) => item.role)).size === phase.expected &&
            raw.programs.every(
              (item) =>
                item.confirmed &&
                item.matches &&
                Number.isInteger(item.pid) &&
                item.pid > 0 &&
                item.os_start_time_ms > 0 &&
                /^[a-f0-9]{64}$/.test(item.expected_sha256) &&
                item.actual_sha256 === item.expected_sha256,
            )
          );
        });
      };
      return (
        JSON.stringify(v.command) === JSON.stringify(['pnpm', 'test:diagnostics']) &&
        v.common_frame_clock?.passed === true &&
        v.common_compose_network?.passed === true &&
        JSON.stringify(v.common_frame_clock?.candidate_original?.command) === JSON.stringify(v.command) &&
        /^[a-f0-9]{64}$/.test(v.runner_sha256) &&
        verified(v.baseline) &&
        verified(v.candidate) &&
        exactPrograms('baseline') &&
        exactPrograms('candidate') &&
        v.baseline.source === load('baseline.json')?.source_archive_sha256 &&
        !!evaluated &&
        v.candidate.source === evaluated &&
        cases(v.common?.faults, 15) &&
        cases(v.common?.browser, 22)
      );
    },
  );
  checked(
    'ARC-039',
    'paired operator entry points and restricted project accounts',
    'manual/environments.json',
    (v) =>
      ['a', 'self', 'b'].every(
        (side) =>
          v.sides?.[side]?.limitedProjectViewer?.count === 10 &&
          [403, 404].includes(v.sides[side].limitedProjectViewer.crossProjectStatus) &&
          v.sides[side].artifact &&
          v.sides[side].pendingInvitation,
      ),
  );
  const operatorPagination = sample => {
    if (sample?.pageSize !== 10 || sample.total !== 11 || sample.firstPage?.length !== 10 ||
        sample.secondPage?.length !== 1 || sample.projects?.length !== 11 || !sample.returnToFirstPage) return false;
    const ids = new Set(sample.projects.map(project => project.id));
    return ids.size === 11 && sample.openPath === '/t/' + sample.tenant + '/projects' &&
      [...sample.firstPage, ...sample.secondPage].every(project => ids.delete(project.id)) &&
      sample.replayed?.verifiedTenant && sample.projects.some(p => p.id === sample.replayed.verifiedProject);
  };
  const operatorDrafts = walk => {
    const sample = walk.consoleCorrections?.drafts;
    const credentials = 'private/walkthrough-drafts/' + walk.side + '-' + walk.environment + '/operator-accounts.json';
    const path = join(dir, credentials);
    if (!sample?.tenant?.id || sample.credentialFile !== credentials || walk.credentials?.drafts !== credentials ||
        !existsSync(path) || (statSync(path).mode & 0o777) !== 0o600) return false;
    const prefix = '/t/' + sample.tenant.id;
    return sample.readback?.initiallyEditable && sample.readback.memberStatus === 'active' &&
      sample.readback.invitationStatus === 'pending' && !!sample.invitation?.id && !!sample.target?.membershipID &&
      !!sample.operator?.membershipID && sample.target.membershipID !== sample.operator.membershipID &&
      !!sample.editableRole?.id && !!sample.readonlyRole?.id && sample.editableRole.id !== sample.readonlyRole.id &&
      sample.pages?.members === prefix + '/members' && sample.pages.invitations === prefix + '/invitations' &&
      sample.pages.operatorAuthorization === prefix + '/members/' + sample.operator.membershipID;
  };
  const operatorFocus = (sample, tenant) => {
    if (!sample || sample.tenant !== tenant || sample.count !== 23 || sample.pageSize !== 10 ||
        sample.pageNumber !== 2 || sample.rows?.length !== 23 || !sample.target?.id || !sample.instructions) return false;
    const rows = new Map(sample.rows.map(row => [row.id,row.name]));
    if (rows.size !== 23 || rows.get(sample.target.id) !== sample.target.name ||
        sample.target.domID !== 'import-' + sample.target.id) return false;
    const base = '/t/' + tenant + '/monitoring/imports';
    const paths = [sample.listPath,sample.secondPagePath,sample.detailPath].map(path => new URL(path,'http://localhost'));
    const [first,second,detail] = paths;
    return paths.every(url => url.origin === 'http://localhost' && url.searchParams.get('q') === sample.prefix &&
      url.searchParams.get('limit') === '10') && first.pathname === base && !first.searchParams.has('cursor') &&
      second.pathname === base && !!second.searchParams.get('cursor') && detail.pathname === base + '/' + sample.target.id &&
      detail.searchParams.get('cursor') === second.searchParams.get('cursor') && detail.searchParams.get('tab') === 'history';
  };
  for (const side of ['a', 'b'])
    checked(
      'ARC-039',
      side + ' full operator walkthrough',
      'manual/walkthrough-' + side + '.json',
      (v) =>
        v.side === side &&
        !!v.execution?.launchID &&
        !!v.execution.serverBootID &&
        (!sharedConsole || (v.sharedConsoleSHA256 === sharedConsole.console.sha256 &&
          operatorPagination(v.consoleCorrections?.projectPagination) && operatorDrafts(v) &&
          (!sharedConsole.beforeImportFocus || operatorFocus(v.consoleCorrections.importFocus,v.scope?.tenant)))) &&
        v.releaseSHA256 ===
          (side === 'b' ? evaluated : load('baseline.json')?.source_archive_sha256) &&
        v.sampleReadback?.permittedRows === 10 &&
        [403, 404].includes(v.sampleReadback?.deniedCrossProject) &&
        v.sampleReadback?.history?.length === 2 &&
        v.sampleReadback.history.every(
          (item) => item.rows === 10 && /^[a-f0-9]{64}$/.test(item.valuesSHA256),
        ) &&
        v.restrictedProjectRows === 10 &&
        !!v.data?.receipt &&
        !!v.data?.result &&
        v.data.history?.length === 2 &&
        !!v.data.model &&
        !!v.data.terminal,
    );
  for (const side of ['a', 'b']) {
    const walk = load('manual/walkthrough-' + side + '.json');
    checked(
      'ARC-039',
      side + ' operator fault and recovery walkthrough',
      'manual/operator-' + side + '.json',
      (v) =>
        v.status === 'passed' &&
        v.side === side &&
        !!walk?.execution?.launchID &&
        (!sharedConsole || v.sharedConsoleSHA256 === sharedConsole.console.sha256) &&
        walk.execution.launchID === v.launchID &&
        !!walk.execution.serverBootID &&
        walk.execution.serverBootID === v.diagnosticBootID &&
        v.descriptorSHA256 ===
          digest(readFileSync(join(dir, 'manual', 'walkthrough-' + side + '.json'))) &&
        v.databaseSchema === walk.execution.schema?.version &&
        v.mismatchedReleaseRejected === true &&
        v.releaseSHA256 === walk.releaseSHA256 &&
        v.releaseSHA256 ===
          (side === 'b' ? evaluated : load('baseline.json')?.source_archive_sha256) &&
        ['parser', 'clickhouse', ...(side === 'b' ? ['broker'] : [])].every((name) =>
          v.records?.some(
            (r) =>
              r.name === name &&
              r.before?.outcome === 'ok' &&
              ['failed', 'timeout'].includes(r.stopped?.outcome) &&
              r.recovered?.outcome === 'ok' &&
              r.facilities > 0,
          ),
        ),
    );
  }
  const state = (checks) =>
    checks.some((p) => p.status === 'failed')
      ? 'failed'
      : checks.length > 0 && checks.every((p) => p.passed)
        ? 'passed'
        : 'incomplete';
  const arcs = expected.map((r) => ({
    ...r,
    checks: groups.get(r.id),
    status: performanceArcs.includes(r.id) ? 'skipped' : state(groups.get(r.id).filter(c => c.status !== 'skipped')),
    ...(performanceArcs.includes(r.id) && { skipReason }),
    manual: performanceArcs.includes(r.id) ? 'skipped' : 'pending',
  }));
  const last = arcs.at(-1);
  const prior = arcs.slice(0, -1).filter(row => !performanceArcs.includes(row.id)),
    priorStatus = state(prior.map((r) => ({ passed: r.status === 'passed', status: r.status })));
  last.checks.push({
    part: 'all prior functional architecture gates',
    passed: priorStatus === 'passed',
    status: priorStatus,
    unmet: prior.filter((r) => r.status !== 'passed').map((r) => r.id),
  });
  last.status = state(last.checks.filter(c => c.status !== 'skipped'));
  const durable = new Map(), historicalCommands = [];
  for (const path of paths('commands')) {
    const value = load(path);
    const label = value.label || path.slice(9, -5);
    const source = value.candidateSHA256 ?? value.provenance?.releaseSHA256;
    if (!source && !value.suite) {
      historicalCommands.push({ label, path, status: value.status, startedAt: value.startedAt });
      continue;
    }
    durable.set(label, value.suite === 'load'
      ? { label, suite: 'load', status: 'skipped', skipReason, previousRecord: path, priorStatus: value.status }
      : source && source !== evaluated
        ? { ...value, status: 'pending', priorStatus: value.status, requiredRelease: evaluated }
        : value);
  }
  for (const [name, results] of Object.entries(regressions))
    for (const value of results)
      durable.set(name + ':' + value.command[1], { ...value, side: name });
  for (const side of ['baseline', 'candidate']) {
    const value = load('http/domain/' + side + '/http-tests.json');
    if (!value) continue;
    const release = side === 'baseline' ? load('baseline.json')?.source_archive_sha256 : evaluated;
    const current = value.provenance?.releaseSHA256 === release && value.sourceAtCompletion?.matchesSelectedRelease;
    durable.set(side + ':domain-http', current ? { ...value, side } : {
      ...value, side, status: 'pending', priorStatus: value.status, requiredRelease: release,
    });
  }
  const unlabelled = commands.filter((c) => !c.label && !c.command);
  const allCommands = [...durable.values(), ...unlabelled];
  const passed =
    arcs.every(satisfied) && allCommands.every(satisfied) && commands.every(satisfied);
  const ledger = {
    at: new Date().toISOString(),
    selected,
    scope,
    baseline: load('baseline.json')?.commit,
    versions: load('versions.json'),
    selfVerification: passed ? 'passed' : 'in_progress',
    manualAcceptance: 'pending',
    productionRelease: 'pending',
    candidate: (load('candidate.evaluated.json') ?? load('candidate.json'))?.inputs,
    candidateManifest: load('candidate.evaluated.json')?.manifest ?? 'candidate.json',
    commands: allCommands,
    historicalCommands,
    arcs,
    passed,
  };
  write(join(dir, 'acceptance.json'), ledger);
  renderVerification(dir, ledger);
  return ledger;
}
