import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, renameSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';
import {
  read,
  root,
  runDirectory,
  prepareLab,
  safeDiagnosticLine,
  write,
} from './architecture-lab.mjs';
import { prepareCandidate, prepareTopology, selectedCandidate } from './architecture-serve.mjs';
import { visual } from './architecture-visual.mjs';
import { compareStaticBundles } from './architecture-static.mjs';
import {
  archivePublicAttempt,
  bindPublicEvidence,
  currentPublicEvidence,
  outputs,
  publicReplayInputs,
} from './architecture-parity-evidence.mjs';
import { navigation } from './architecture-navigation.mjs';
import { manualEnvironment } from './architecture-manual.mjs';
import { visualStates } from './architecture-visual-states.mjs';
import { orderedVisualReads } from './architecture-visual-read-order.mjs';
import { visualScreens } from './architecture-visual-assets.mjs';
import { visualTopology } from './architecture-visual-topology.mjs';
import { crossConsole } from './architecture-cross-console.mjs';
import { membershipResult, qualifyMembership } from './architecture-membership-result.mjs';
import { frozenVisualLab, visualLab } from './architecture-visual-lab.mjs';
import { acceptance, suites, versionManifest } from './architecture-acceptance.mjs';
import {
  checks,
  readChecks,
  recordCommand,
  regression,
} from './architecture-regression.mjs';
import { regressionTriage } from './architecture-regression-triage.mjs';
import { sessionActivity } from './architecture-session-activity.mjs';
import { regressionQueue } from './architecture-regression-queue.mjs';
import { renderVerification } from './architecture-verification-report.mjs';
import { domainHTTP } from './architecture-domain-http.mjs';
import { cohortCommands, openPublicCohort } from './architecture-public-cohort.mjs';
import { selectEvaluatedRelease } from './architecture-source-archive.mjs';
import { selectSharedConsole } from './architecture-shared-console.mjs';
import { testEnv } from './test-environment.mjs';
import { performanceCommands, registerFunctionalScope, satisfied, skipReason } from './architecture-acceptance-scope.mjs';

async function run(argv) {
  const args = parseArgs({
    args: argv,
    options: {
      'run-dir': { type: 'string' },
      'release-dir': { type: 'string' },
      'derived-soak': { type: 'string' },
      help: { type: 'boolean' },
      resume: { type: 'boolean' },
    },
    allowPositionals: true,
  });
  if (args.values.help) {
    console.log(
      'pnpm test:architecture [routes parity visual faults load restore] [--run-dir .artifacts/architecture-refactor/run-ID] [--release-dir .artifacts/architecture-refactor/run-candidate-ID] [--resume]  （功能验收。load 仅记录按用户要求跳过。）',
    );
    return true;
  }
  const functionalSuites = suites.filter(name => name !== 'load');
  const selected = args.positionals.length ? [...new Set(args.positionals)] : functionalSuites;
  for (const s of selected) assert(suites.includes(s), 'Unknown architecture suite ' + s);
  const complete = functionalSuites.every((s) => selected.includes(s)),
    resume = args.values.resume;
  const dir = runDirectory(args.values['run-dir'] || process.env.INFRAPRO_ARCHITECTURE_RUN_DIR),
    commands = [];
  const releasePath = args.values['release-dir'] || process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE;
  registerFunctionalScope(dir);
  if (args.values['derived-soak']) console.log('派生稳定性测试' + skipReason);
  if (selected.length === 1 && selected[0] === 'load') {
    console.log('ARC-037、ARC-038 ' + skipReason);
    return true;
  }
  const releaseDirectory = releasePath ? runDirectory(releasePath) : dir;
  const environment = { ...process.env, INFRAPRO_ARCHITECTURE_RUN_DIR: dir };
  const metadata = join(dir, 'commands'),
    art = (name) => join(dir, name);
  let suite = '',
    release,
    publicLab,
    publicInputs;
  console.log('Architecture evidence ' + dir);
  const oldVersion = existsSync(art('versions.json')) ? read(art('versions.json')) : null;
  const version = versionManifest(dir);
  const resumeVisual =
    resume &&
    oldVersion?.visual &&
    JSON.stringify([oldVersion.os, oldVersion.visual]) ===
      JSON.stringify([version.os, version.visual]);
  const slow = async (label, action) => {
    const before = Date.now();
    const destination = join(metadata, label + '.json');
    if (existsSync(destination)) {
      const previous = read(destination);
      const attempt = join(
        metadata,
        'attempts',
        label + '-' + (previous.startedAt ?? String(before)).replaceAll(':', '-'),
      );
      mkdirSync(attempt, { recursive: true, mode: 0o700 });
      renameSync(destination, join(attempt, label + '.json'));
    }
    const invocation = {
      suite,
      label,
      command: ['pnpm', 'test:architecture', suite],
      startedAt: new Date(before).toISOString(),
      candidateSHA256: release.inputs.sourceSHA256,
      node: process.version,
    };
    write(destination, {
      ...invocation,
      status: 'running',
    });
    let result = { status: 'failed' };
    try {
      const outcome = await action();
      if (outcome === false || outcome?.passed === false)
        throw Error('Assertions failed; see suite evidence');
      result = { status: 'passed' };
    } catch (e) {
      result.error = safeDiagnosticLine(e.message || e.name).slice(0, 450);
    }
    const record = {
      ...invocation,
      durationMs: Date.now() - before,
      exitCode: result.status === 'passed' ? 0 : 1,
      ...result,
    };
    commands.push(record);
    write(destination, record);
    console.log(
      suite + ' ' + label + ' ' + result.status + (result.error ? ' ' + result.error : ''),
    );
    return record;
  };
  const call = async (label, cmd, args, options = {}) => {
    const pairedFixture =
      outputs[label] && !publicLab && label !== 'architecture-authority' ? randomUUID() : undefined;
    const result = await recordCommand(metadata, label, cmd, args, {
      cwd: root,
      ...options,
      env: {
        ...(options.env ?? environment),
        ...(pairedFixture && { ARC_HTTP_FIXTURE_ID: pairedFixture }),
      },
      provenance: {
        commit: release.inputs.commit,
        releaseSHA256: release.inputs.sourceSHA256,
        node: process.version,
        ...(publicLab && { httpEnvironment: publicLab.directory }),
        ...(pairedFixture && { pairedFixture }),
        ...(outputs[label] && publicInputs && { publicInputs }),
        ...(label === 'architecture-authority' && { evidenceScope: 'domain-transaction-tests' }),
        ...options.provenance,
      },
    });
    const record = {
      ...result,
      suite,
      label,
      candidateSHA256: release.inputs.sourceSHA256,
    };
    if (record.provenance.publicInputs) {
      record.publicInputsAtCompletion = publicReplayInputs(dir,label);
      if (record.publicInputsAtCompletion !== record.provenance.publicInputs) {
        record.processStatus = result.status;
        record.status = 'failed';
        record.failure = [result.failure, 'Public replay inputs changed during execution. Repeat with the recorded release.'].filter(Boolean).join(' ');
      }
    }
    bindPublicEvidence(dir, label, record);
    if (outputs[label]) record.archivedOutput = archivePublicAttempt(dir, label, record);
    write(join(metadata, label + '.json'), record);
    commands.push(record);
    console.log(suite + ' ' + label + ' ' + record.status);
    return record;
  };
  const node = (label, file, args = [], options) =>
    call(label, process.execPath, [file, ...args], options);
  function regressionSuite(command) {
    if (performanceCommands.has(command)) return 'load';
    return command.endsWith(':restore') ? 'restore' : 'parity';
  }
  const regress = async (group) => {
    const first = commands.length;
    const defaults = group === 'parity' && resume ? membershipResult(dir).defaults : null;
    for (const name of ['baseline', 'candidate']) {
      const finished = new Set(
        resume
          ? readChecks(dir, name)
              .filter((r) => r.status === 'passed')
              .map((r) => r.command[1])
          : [],
      );
      if (defaults?.[name]?.state === 'original-default-timeout') finished.add('test:display:load');
      const selectedChecks = checks.filter((c) => regressionSuite(c) === group && !finished.has(c));
      if (selectedChecks.length) {
        const results = await regression(dir, name, selectedChecks, { stopStorage: true, resume });
        commands.push(...results.map((r) => ({ ...r, suite })));
      }
    }
    if (group === 'parity') {
      let qualified;
      await slow('display-member-boundary-qualification', async () => {
        qualified = await qualifyMembership(dir,regression);
        return qualified;
      });
      if (qualified?.passed && qualified.mode === 'both-matched-after-frozen-timeout') {
        const original = commands.findIndex((item,index) => index >= first &&
          item.command?.[1] === 'test:display:load' && item.provenance?.role === 'baseline');
        if (original >= 0)
          commands[original] = {...readChecks(dir,'baseline').find(item => item.command[1] === 'test:display:load'),suite};
      }
    }
  };
  const closeSides = async () => {
    const cohort = publicLab;
    publicLab = undefined;
    await cohort?.close();
  };
  let exercisedPublicOperations = false;
  const publicOperations = async () => {
    if (exercisedPublicOperations) return;
    publicInputs = publicReplayInputs(dir,'architecture-edge');
    const observed = resume ? currentPublicEvidence(dir, release.inputs.sourceSHA256) : new Set();
    const cached = (label) => {
      const path = join(metadata, label + '.json');
      if (!outputs[label].every((file) => observed.has(file)) || !existsSync(path)) return null;
      const receipt = read(path);
      return receipt.provenance?.publicInputs === publicInputs ? receipt : null;
    };
    const cohort = cohortCommands('', dir).map(([label]) => cached(label));
    if (
      cohort.every(Boolean) &&
      new Set(cohort.map((r) => r.provenance.httpEnvironment)).size === 1
    ) {
      commands.push(...cohort);
    } else {
      publicLab = await openPublicCohort(dir, releaseDirectory);
      for (const [label, file, args] of cohortCommands(publicLab.directory, dir)) {
        await node(label, file, args);
      }
      // These journeys share writes to one restored pair. Resume them as a group.
      await closeSides();
    }
    for (const file of [
      'architecture-http-parity',
      'architecture-engineering-parity',
      'architecture-resource-parity',
      'architecture-monitoring-parity',
      'architecture-analysis-parity',
      'architecture-derived-parity',
      'architecture-spatial-parity',
      'architecture-ingestion-http',
      'architecture-task-parity',
      'architecture-audit-parity',
      'architecture-authority',
    ]) {
      // Each remaining command restores its own fixture. Bind it to the inputs
      // present at its start, even if an earlier suite observed a change.
      publicInputs = publicReplayInputs(dir,file);
      const previous = cached(file);
      if (previous) commands.push(previous);
      else await node(file, 'scripts/' + file + '.mjs', [dir]);
    }
    await node('display-scope-permissions', 'scripts/architecture-display-scope-permissions.mjs', [
      dir,
    ]);
    for (const name of ['baseline', 'candidate'])
      commands.push({ ...(await domainHTTP(dir, name, { stopStorage: true })), suite });
    exercisedPublicOperations = true;
  };
  try {
    await prepareLab(dir);
    release = releasePath ? selectedCandidate(releaseDirectory) : prepareCandidate(dir);
    const sharedConsole = selectSharedConsole(dir, releaseDirectory, release);
    if (!sharedConsole) {
      delete process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
      delete environment.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
    }
    const releaseEnvironment = {
      INFRAPRO_ARCHITECTURE_HTTP_RELEASE: releaseDirectory,
      INFRAPRO_ARCHITECTURE_REGRESSION_RELEASE: releaseDirectory,
      ...(sharedConsole && { INFRAPRO_ARCHITECTURE_SHARED_CONSOLE: art('shared-console.json') }),
    };
    Object.assign(process.env, releaseEnvironment);
    Object.assign(environment, releaseEnvironment);
    selectEvaluatedRelease(dir, releaseDirectory, release);
    await prepareTopology(dir);
    for (const s of selected) {
      suite = s;
      try {
        switch (s) {
          case 'routes': {
            mkdirSync(art('routes'), { recursive: true, mode: 0o700 });
            await node('route-catalog', 'scripts/architecture-routes.mjs', ['--check']);
            await slow('owner-inventory', async () => {
              const raw = JSON.parse(
                execFileSync('go', ['run', './services/api/cmd/api', 'routes'], {
                  encoding: 'utf8',
                  maxBuffer: 8 << 20,
                }),
              );
              assert.equal(raw['compat-api']?.length, 1071);
              write(art('routes/registered.json'), raw);
            });
            await call(
              'gateway',
              'go',
              [
                'test',
                '-v',
                '-count=1',
                '-timeout',
                '30m',
                './services/api/internal/bootstrap',
                '-run',
                '^TestArchitecture(HTTP|StreamingProxy|ColdStartOutages|ReplicaMTLS)',
              ],
              {
                env: {
                  ...environment,
                  ...testEnv,
                  INFRAPRO_ARCHITECTURE_ROUTE_EVIDENCE: art('anonymous-http.json'),
                },
              },
            );
            await call(
              'bound-display-read',
              'go',
              [
                'test',
                '-v',
                '-count=1',
                './packages/go/platform/resources',
                '-run',
                '^Test(ArchitectureBoundDisplayRead(Delivery)?|PublishedAnalysisInputChannelQualification|ConcurrentPersonnelRawInputReadAndLogout)$',
              ],
              { env: { ...environment, ...testEnv } },
            );
            // A routes-only run also proves legitimate authenticated calls.
            // In the complete suite these replays follow the frozen visual run.
            if (!selected.includes('parity')) await publicOperations();
            break;
          }
          case 'parity': {
            await publicOperations();
            for (const side of ['baseline','candidate'])
              await slow('session-activity-' + side, () => sessionActivity(dir,side,{resume}));
            // Enterprise browser, HTTP, CLI and domain regressions are part of the
            // default gate. Core checks include both the actual frozen and candidate release.
            await regress('parity');
            break;
          }
          case 'visual': {
            const visualRelease =
              !releasePath && process.env.INFRAPRO_ARCHITECTURE_VISUAL_RELEASE
                ? runDirectory(process.env.INFRAPRO_ARCHITECTURE_VISUAL_RELEASE)
                : releaseDirectory;
            await slow('static-assets', () =>
              compareStaticBundles(dir, selectedCandidate(visualRelease)),
            );
            for (const role of ['self', 'b']) {
              const routes =
                role === 'self'
                  ? await frozenVisualLab(dir, visualRelease)
                  : await visualLab(dir, visualRelease);
              const clean = role === 'self' ? await routes.start() : await routes.start(['a', 'b']);
              try {
                await slow('visual-' + role, () =>
                  visual(routes.dir, role, {
                    resume: resumeVisual,
                    outputDirectory: dir,
                  }),
                );
              } finally {
                await clean.close();
              }
            }
            const manualSamples = { sides: {}, sources: {} };
            for (const role of ['self', 'b']) {
              const sides = ['a', role];
              const fixtures =
                role === 'self'
                  ? await frozenVisualLab(dir, visualRelease, {
                      fixtures: true,
                    })
                  : await visualLab(dir, visualRelease, { fixtures: true });
              const running =
                  role === 'self' ? await fixtures.start() : await fixtures.start(sides),
                source = fixtures.dir;
              try {
                await slow('manual-project-personas-and-history-' + role, async () => {
                  const outcome = await manualEnvironment(source, sides);
                  for (const side of sides) {
                    manualSamples.sides[side] = outcome.sides[side];
                    manualSamples.sources[side] = source;
                  }
                  write(art('manual/environments.json'), manualSamples);
                  return outcome;
                });
                const { visualInvitationReceipts } = await import('./architecture-visual-invitations.mjs');
                await slow('personal-invitation-notifications-' + role, () => visualInvitationReceipts(source,role,dir));
                if (role === 'b') {
                  await slow('navigation', () => navigation(source, dir));
                  await slow('live-diagnostics-layout', () => visualTopology(source, dir));
                }
                await slow('shared-states-' + role, () =>
                  visualStates(source, role, {
                    resume: resumeVisual,
                    outputDirectory: dir,
                  }),
                );
                await slow('ordered-multichannel-pages-' + role, () =>
                  orderedVisualReads(source, role, dir, { resume: resumeVisual }),
                );
                await slow('published-screens-' + role, () => visualScreens(source, role, dir));
                if (sharedConsole && role === 'b') {
                  await node('approved-console-drafts', 'scripts/architecture-console-drafts.mjs', [dir, source]);
                  await slow('same-response-console-releases', () => crossConsole(source, dir));
                }
              } finally {
                await running.close();
              }
            }
            if (sharedConsole)
              await node('approved-console-pagination', 'scripts/architecture-console-pagination.mjs', [dir]);
            if (sharedConsole?.beforeImportFocus)
              await node('approved-import-return-focus','scripts/architecture-console-import-focus.mjs',[dir]);
            break;
          }
          case 'faults': {
            await node('jetstream-pg-faults', 'scripts/test-architecture-events.mjs');
            await node('role-workflows', 'scripts/architecture-worker-checks.mjs', [dir, 'all']);
            await node('running-derived-query-cancel', 'scripts/architecture-derived-cancel-check.mjs', [dir]);
            await node('independent-process-probes', 'scripts/diagnostics-architecture-check.mjs', [
              art('diag-role-faults.json'),
            ]);
            await node('isolated-dependency-faults', 'scripts/architecture-faults.mjs', [dir]);
            await node(
              'ordered-change-and-relay-faults',
              'scripts/architecture-change-faults.mjs',
              [dir],
            );
            await call(
              'two-live-replica-budgets',
              'go',
              [
                'test',
                '-v',
                '-count=1',
                './packages/go/platform/config',
                '-run',
                '^TestReplicaBudget(RejectsDuplicateAndRecovers|IdleStallPreservesSession|BrowserCancellationPreservesOwnership|IdleCheckoutAndShrink|DelayedCancelPreservesBackend)$',
              ],
              { env: { ...environment, ...testEnv } },
            );
            await call(
              'fixed-baseline-metadata-authority',
              'go',
              ['test', '-v', '-count=1', './packages/go/platform/derivedmonitoring', '-run', '^TestFixedBaselineMetadataProjection$'],
              { env: { ...environment, ...testEnv } },
            );
            await node('legacy-in-progress-cold-upgrade', 'scripts/architecture-legacy-work.mjs', [
              dir,
            ]);
            await node('file-checker-outage', 'scripts/architecture-display-files-parity.mjs', [
              dir,
              'checker',
            ]);
            if (!selected.includes('parity') && !selected.includes('routes')) {
              await node('parser-custody-outage', 'scripts/architecture-ingestion-http.mjs', [dir]);
              await node('spatial-transfer-recovery', 'scripts/architecture-spatial-parity.mjs', [
                dir,
              ]);
            }
            break;
          }
          case 'load':
            console.log('ARC-037、ARC-038 ' + skipReason);
            break;
          case 'restore': {
            await node('cold-restore-grants', 'scripts/architecture-cold-grants.mjs', [dir]);
            const frozen = read(art('baseline.json'));
            await node('deployment-and-recovery', 'scripts/architecture-deployment-check.mjs', [
              '--output',
              dir,
              '--frozen-bin',
              frozen.bin,
              '--frozen-commit',
              frozen.commit,
              '--current-bin',
              release.bin,
            ]);
            await node('verified-offline-release', 'scripts/architecture-offline-check.mjs', [
              '--output',
              dir,
              '--host-release',
              releaseDirectory,
            ]);
            await node('published-audit-role', 'scripts/architecture-audit-role-check.mjs', [dir]);
            await node('rollback-gui-sdk-cli', 'scripts/architecture-rollback-readback.mjs', [dir]);
            await regress('restore');
            break;
          }
        }
      } catch (e) {
        const error = safeDiagnosticLine(e.message || e.name).slice(0, 450);
        commands.push({ suite, status: 'failed', error });
        console.error(suite + ' failed: ' + error);
      } finally {
        await closeSides();
      }
      regressionTriage(dir);
      regressionQueue(dir);
      acceptance(dir, selected, commands);
    }
  } finally {
    await closeSides();
  }
  if (complete) {
    suite = 'restore';
    const { startWalkthroughRehearsal } = await import('./architecture-walkthrough-rehearsal.mjs');
    for (const name of ['a', 'b'])
      await slow('walkthrough-' + name, () =>
        startWalkthroughRehearsal(dir, name, releaseDirectory),
      );
  }
  const ledger = acceptance(dir, selected, commands);
  if (complete) {
    if (ledger.passed)
      renderVerification(
        dir,
        ledger,
        join(root, 'docs/verification/system-architecture-refactor.md'),
      );
    return ledger.passed;
  }
  const relevant = {
    routes: ['001'],
    parity: [
      '002',
      '003',
      '004',
      '005',
      '006',
      '007',
      '008',
      '009',
      '010',
      '011',
      '012',
      '013',
      '014',
      '015',
    ],
    visual: ['016', '017'],
    faults: [
      '018',
      '019',
      '020',
      '021',
      '022',
      '023',
      '024',
      '025',
      '026',
      '027',
      '028',
      '029',
      '030',
      '031',
      '036',
    ],
    load: ['037', '038'],
    restore: ['032', '033', '034', '035'],
  };
  const ids = new Set(selected.flatMap((s) => relevant[s].map((n) => 'ARC-' + n)));
  return (
    commands.every(satisfied) &&
    ledger.arcs.filter((a) => ids.has(a.id)).every(satisfied)
  );
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = (await run(process.argv.slice(2))) ? 0 : 1;
  } catch (e) {
    // Release credentials can appear in underlying HTTP traces. Only expose
    // the exception class and a safe first line at this public entry point.
    console.error(safeDiagnosticLine(e.message || e.name).slice(0, 500));
    process.exitCode = 1;
  }
}
