import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';
import {
  draftReads as reads,
  draftWrites as writes,
  newDraftFixture,
  openDraftHTTP,
  prepareDraftFixture,
  retainedDraftFiles,
} from './architecture-console-draft-fixture.mjs';
import {
  digest as sha,
  fileLease,
  launch,
  read,
  root,
  runDirectory,
  safeDiagnosticLine,
  sleep,
  write,
} from './architecture-lab.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { liveWalkthroughStatus } from './architecture-walkthrough-control.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { availablePort } from './fixtures/parsing-lab.mjs';

async function exercise(run, sandbox, endpoints, retained, observeFixture) {
  const shared = sharedConsoleFor(run, { verifyFiles: true });
  assert(shared, 'Select the corrected console first');
  const lab = read(join(sandbox, 'private/lab.json')),
    accounts = read(join(sandbox, 'private/accounts.json'));
  const frozen = read(join(run, 'baseline.json')),
    selected = join(run, 'candidate.evaluated.json');
  const candidate = read(join(existsSync(selected) ? read(selected).directory : run, 'candidate.json')),
    helper = join(candidate.bin, 'diagnostics-labtool');
  const ledger = join(run, retained ? 'manual' : 'console-corrections'),
    attempt = join(ledger, 'drafts-' + new Date().toISOString().replaceAll(':', '-'));
  mkdirSync(attempt, { recursive: true, mode: 0o700 });
  const previous = join(ledger, retained ? 'live-drafts.json' : 'drafts.json');
  if (existsSync(previous)) write(join(attempt, 'previous.json'), read(previous));
  const assets = Object.fromEntries(
      Object.entries(shared.console.files).map(([name, hash]) => ['console/' + name, hash]),
    ),
    release = shared.console.directory,
    allRequests = new Set();
  function backend(side, env) {
    const manifest = side === 'a' ? frozen : candidate;
    const expected = side === 'a' ? frozen.source_archive_sha256 : candidate.inputs.sourceSHA256;
    assert.equal(expected, side === 'a' ? shared.baselineReleaseSHA256 : shared.backendReleaseSHA256);
    // Observe the actual public listener, then verify the bytes of its running executable.
    const lines = execFileSync(
      'lsof',
      ['-nP', '-a', '-iTCP:' + new URL(env.api).port, '-sTCP:LISTEN', '-Fp'],
      { encoding: 'utf8' },
    )
      .trim()
      .split('\n');
    const pids = [...new Set(lines.filter((l) => /^p[0-9]+$/.test(l)).map((l) => Number(l.slice(1))))];
    assert.equal(pids.length, 1);
    const identity = JSON.parse(
      execFileSync(helper, ['identity', String(pids[0])], {
        encoding: 'utf8',
        timeout: 5000,
      }),
    );
    assert.equal(identity.pid, pids[0]);
    assert(identity.os_start_time_ms > 0);
    const actual = sha(readFileSync(identity.executable));
    assert.equal(actual, manifest.binaries.api, 'Running public API differs from its selected sealed binary');
    return {
      backendSHA256: expected,
      pid: identity.pid,
      osStart: identity.os_start_time_ms,
      executableRole: 'api',
      fileSHA256: actual,
    };
  }
  const browser = await chromium.launch({
    headless: true,
    args: ['--disable-gpu'],
  });
  const tag = randomBytes(5).toString('hex'),
    reason = '租户操作草稿与撤权功能验真';
  const report = {
    passed: false,
    started_at: new Date().toISOString(),
    browser: browser.version(),
    consoleSHA256: shared.console.sha256,
    common_release_directory: release,
    static_file_count: Object.keys(assets).length,
    static_sha256: shared.console.sha256,
    sides: {},
  };
  const reasonText = (label) => '本地未提交 ' + label;
  const qualification = (context) => ({
    membershipFingerprint: sha(tag + '\0' + context.membership_id),
    permissionsSHA256: sha(JSON.stringify(context.permissions)),
    relevantPermissions: Object.fromEntries(
      [...reads, ...writes].map((code) => [code, context.permissions.includes(code)]),
    ),
  });
  function checkLoopback(v) {
    assert(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(v).hostname));
  }
  const privateActors = [];
  const pendingEvidence = new Map();
  try {
    for (const side of ['a', 'b']) {
      const env = { ...lab[side], ...endpoints[side] };
      checkLoopback(env.api);
      checkLoopback(env.origin);
      const seen = {},
        trace = [],
        typed = {},
        states = {},
        browserTraffic = [],
        screenshotFiles = [];
      const running = backend(side, env);
      const rawPath = join(attempt, side);
      mkdirSync(rawPath, { recursive: true, mode: 0o700 });
      const result = {
        side,
        backendSHA256: running.backendSHA256,
        consoleSHA256: shared.console.sha256,
        listener: running,
        actual_api: env.api,
        states,
        assets: seen,
      };
      report.sides[side] = result;
      pendingEvidence.set(side, { rawPath, trace, browserTraffic, states, seen });
      const http = openDraftHTTP(env, trace);
      allRequests.add(http);
      const local = retained?.[side];
      const prepared = await prepareDraftFixture({
        http,
        adminLogin: accounts.admin,
        state: local?.state ?? newDraftFixture(tag),
        save: local?.save,
      });
      if (observeFixture) await observeFixture({ side, env, prepared });
      if (local) {
        const expected = local.descriptor.consoleCorrections.drafts;
        assert.equal(prepared.tenantID, expected.tenant.id);
        assert.equal(prepared.actorMembership.membership_id, expected.operator.membershipID);
        assert.equal(prepared.targetMembership.membership_id, expected.target.membershipID);
        assert.equal(prepared.pendingInvite.id, expected.invitation.id);
        assert.equal(prepared.edit.id, expected.editableRole.id);
        assert.equal(prepared.readOnly.id, expected.readonlyRole.id);
        result.launchID = local.status.launchID;
        result.retainedSampleVerified = true;
      }
      const {
        people: creds,
        tenantID,
        space,
        owner,
        operator,
        targetMembership,
        pendingInvite,
        edit,
        readOnly,
        allowed,
        assign,
      } = prepared;
      const { call } = http;
      const editable = edit.id,
        readonly = readOnly.id;
      result.qualification = { before: qualification(allowed) };
      const permissionHash = sha(JSON.stringify(allowed.permissions));
      const storage = await operator.c.storageState(),
        bc = await browser.newContext({
          storageState: storage,
          serviceWorkers: 'block',
          viewport: { width: 1440, height: 900 },
          timezoneId: 'Asia/Shanghai',
        });
      privateActors.push(bc);
      const activeGates = new Set();
      let restoreEditable = false;
      try {
        const setup = async (leaf) => {
          const full = env.origin + '/t/' + tenantID + '/' + leaf,
            page = await bc.newPage();
          const subject = leaf === 'members' ? creds.target : creds.pending,
            title = leaf === 'members' ? '暂停成员' : '重新生成';
          let held;
          await page.route('**/*', async (route) => {
            const req = route.request(),
              url = new URL(req.url());
            assert.equal(url.origin, env.origin);
            if (url.pathname.startsWith('/api/v1/')) {
              // Forward real application traffic on the same browser origin and cookie jar.
              const response = await route.fetch({ maxRedirects: 0 });
              browserTraffic.push({
                tab: leaf,
                method: req.method(),
                path: url.pathname.replaceAll(/[0-9a-f]{8}-[0-9a-f-]{27}/gu, ':id'),
                status: response.status(),
              });
              const gate = held;
              if (gate && url.pathname === '/api/v1' + space + '/context') {
                gate.enter.resolve({
                  status: response.status(),
                  body: await response.json(),
                });
                await gate.release.promise;
              }
              return route.fulfill({ response });
            }
            const asset =
              'console/' + (req.resourceType() === 'document' ? 'index.html' : url.pathname.slice(1));
            const upstream = await route.fetch({ maxRedirects: 0 });
            if (!assets[asset]) {
              assert(
                !['document', 'script', 'stylesheet'].includes(req.resourceType()),
                'Unexpected console asset ' + asset,
              );
              return route.fulfill({ response: upstream });
            }
            assert.equal(upstream.status(), 200);
            const body = await upstream.body();
            assert.equal(
              sha(body),
              assets[asset],
              'HTTP response differs from the selected console asset ' + asset,
            );
            seen[asset] = sha(body);
            return route.fulfill({ response: upstream, body });
          });
          const row = page.getByRole('row').filter({ hasText: subject.username }),
            actions = row.getByRole('button', { name: /的操作$/ });
          const reasonField = page.getByLabel('操作原因'),
            value = reasonText(title);
          const open = async () => {
            await actions.click();
            await page.getByRole('menuitem', { name: title, exact: true }).click();
          };
          await page.goto(full);
          await open();
          await reasonField.fill(value);
          const pause = async (checkSubmitKey = false) => {
            held = {
              enter: Promise.withResolvers(),
              release: Promise.withResolvers(),
            };
            activeGates.add(held);
            await page.evaluate(() => window.dispatchEvent(new Event('focus')));
            const res = await Promise.race([
              held.enter.promise,
              new Promise((_, reject) =>
                setTimeout(() => reject(Error('timed out waiting for actual qualification')), 10000),
              ),
            ]);
            assert.equal(res.status, 200);
            await page.waitForFunction(() => !document.querySelector('[role=alertdialog]'));
            assert.equal(await reasonField.count(), 0);
            assert.equal(
              await page.getByRole('button', { name: '确认操作', includeHidden: true }).count(),
              0,
            );
            if (checkSubmitKey) {
              await page.keyboard.press('Enter');
              assert.equal(await reasonField.count(), 0);
            }
            const wait = page.waitForResponse(
              (r) => new URL(r.url()).pathname === '/api/v1' + space + '/context' && r.status() === 200,
            );
            held.release.resolve();
            activeGates.delete(held);
            held = undefined;
            await wait;
            return res.body;
          };
          const same = await pause(true);
          assert.equal(same.membership_id, allowed.membership_id);
          assert.equal(sha(JSON.stringify(same.permissions)), permissionHash);
          await reasonField.waitFor();
          assert.equal(await reasonField.inputValue(), value);
          await page.getByRole('button', { name: '取消', exact: true }).click();
          await page.waitForFunction(
            (label) => document.activeElement?.getAttribute('aria-label') === label,
            subject.display_name + '的操作',
          );
          await open();
          await reasonField.fill(value);
          const shot = join(rawPath, leaf + '-retained.png');
          await page.screenshot({ path: shot });
          screenshotFiles.push({
            stage: 'retained',
            view: leaf,
            file: shot.slice(run.length + 1),
            sha256: sha(readFileSync(shot)),
          });
          typed[leaf] = {
            page,
            actions,
            reasonField,
            value,
            open,
            title,
            pause,
          };
          states[leaf] = {
            stable_scope_preserved: true,
            root_portal_absent_during_check: true,
            attempted_submit_key_during_check: true,
            focus_returned_to_action: true,
          };
        };
        await setup('members');
        await setup('invitations');
        restoreEditable = true;
        const denied = await assign(readonly);
        result.qualification.withdrawn = qualification(denied);
        assert.equal(denied.membership_id, allowed.membership_id);
        assert(writes.every((p) => !denied.permissions.includes(p)));
        assert(reads.every((p) => denied.permissions.includes(p)));
        for (const [leaf, t] of Object.entries(typed)) {
          const observed = await t.pause();
          assert(writes.every((p) => !observed.permissions.includes(p)));
          await t.actions.waitFor();
          assert.equal(await t.reasonField.count(), 0);
          await t.actions.click();
          assert.equal(await t.page.getByRole('menuitem', { name: t.title, exact: true }).count(), 0);
          await t.page.keyboard.press('Escape');
          states[leaf].withdrawn_action_absent = true;
          const shot = join(rawPath, leaf + '-revoked.png');
          await t.page.screenshot({ path: shot });
          screenshotFiles.push({
            stage: 'revoked',
            view: leaf,
            file: shot.slice(run.length + 1),
            sha256: sha(readFileSync(shot)),
          });
        }
        const canChange = await call(
          operator,
          'POST',
          space + '/members/' + targetMembership.membership_id + '/suspend',
          { expected_version: targetMembership.membership_version, reason },
          [403],
        );
        const canReplace = await call(
          operator,
          'POST',
          space + '/member-invitations/' + pendingInvite.id + '/regenerate',
          {
            expected_version: pendingInvite.version,
            client_request_id: randomUUID(),
            reason,
          },
          [403],
        );
        states.members.server_denied = canChange.status;
        states.invitations.server_denied = canReplace.status;
        const memberAfterDenial = (
          await call(owner, 'GET', space + '/members/' + targetMembership.membership_id)
        ).body;
        const inviteAfterDenial = (
          await call(owner, 'GET', space + '/member-invitations/' + pendingInvite.id)
        ).body;
        assert.equal(memberAfterDenial.version, targetMembership.membership_version);
        assert.equal(memberAfterDenial.membership_status, 'active');
        assert.equal(inviteAfterDenial.version, pendingInvite.version);
        assert.equal(inviteAfterDenial.status, 'pending');
        states.members.target_after_denial = {
          status: memberAfterDenial.membership_status,
          version: memberAfterDenial.version,
        };
        states.invitations.target_after_denial = {
          status: inviteAfterDenial.status,
          version: inviteAfterDenial.version,
        };
        const regain = await assign(editable);
        restoreEditable = false;
        result.qualification.restored = qualification(regain);
        assert.equal(regain.membership_id, allowed.membership_id);
        assert.equal(sha(JSON.stringify(regain.permissions)), permissionHash);
        for (const [leaf, t] of Object.entries(typed)) {
          const ok = await t.pause();
          assert.equal(sha(JSON.stringify(ok.permissions)), permissionHash);
          await t.actions.waitFor();
          assert.equal(await t.reasonField.count(), 0);
          await t.open();
          assert.equal(await t.reasonField.inputValue(), '');
          states[leaf].blank_after_regain = true;
        }
        const end = backend(side, env);
        assert.deepEqual(end, running);
        if (local) {
          const after = await liveWalkthroughStatus(local.descriptor);
          assert.equal(after.launchID, local.status.launchID);
          assert.equal(after.sharedConsoleSHA256, shared.console.sha256);
        }
        const browserMutations = browserTraffic.filter(
          (r) => r.method !== 'GET' && /\/(members|member-invitations)(\/|$)/u.test(r.path),
        );
        assert.deepEqual(browserMutations, []);
        for (const p of Object.keys(seen))
          assert.equal(seen[p], shared.console.files[p.slice('console/'.length)]);
        const save = (name, content) => {
          const path = join(rawPath, name + '.json');
          write(path, content);
          return {
            file: path.slice(run.length + 1),
            sha256: sha(readFileSync(path)),
          };
        };
        result.evidence = {
          http: save('http', {
            listener_before: running,
            listener_after: end,
            operations: trace,
          }),
          browser: save('browser', {
            version: browser.version(),
            requests: browserTraffic,
            assetReadback: seen,
            businessMutations: browserMutations,
            observations: states,
            screenshots: screenshotFiles,
          }),
        };
        result.cases = Object.entries(states).map(([leaf, v]) => ({
          name: leaf === 'members' ? 'member-suspend' : 'invitation-regenerate',
          passed: true,
          sameScopeDraftRetained: v.stable_scope_preserved,
          submitBlockedDuringCheck: v.root_portal_absent_during_check,
          revocationClearedDraft: v.withdrawn_action_absent && v.blank_after_regain,
          serverDenied: v.server_denied,
          browserBusinessSubmissions: browserMutations.length,
        }));
        result.same_membership_for_each_step = true;
        result.no_draft_persisted = true;
        result.passed = true;
        pendingEvidence.delete(side);
      } finally {
        for (const gate of activeGates) gate.release.resolve();
        await Promise.all(bc.pages().map((page) => page.unrouteAll({ behavior: 'ignoreErrors' })));
        await bc.close();
        if (restoreEditable) {
          const recovered = await assign(editable);
          assert(writes.every((permission) => recovered.permissions.includes(permission)));
        }
        await http.close();
        allRequests.delete(http);
      }
    }
    assert.deepEqual(report.sides.a.assets, report.sides.b.assets);
    report.identical_assets = true;
    report.finished_at = new Date().toISOString();
    report.passed = true;
    return report;
  } catch (error) {
    report.error = safeDiagnosticLine(error.message);
    throw error;
  } finally {
    report.finished_at ??= new Date().toISOString();
    for (const [side, { rawPath, trace, browserTraffic, states, seen }] of pendingEvidence) {
      const http = join(rawPath, 'partial-http.json');
      const browserPath = join(rawPath, 'partial-browser.json');
      write(http, trace);
      write(browserPath, { requests: browserTraffic, observations: states, assets: seen });
      report.sides[side].partialEvidence = {
        http: { file: http.slice(run.length + 1), sha256: sha(readFileSync(http)) },
        browser: { file: browserPath.slice(run.length + 1), sha256: sha(readFileSync(browserPath)) },
      };
    }
    for (const c of privateActors) await c.close().catch(() => {});
    for (const c of allRequests) await c.close().catch(() => {});
    await browser.close();
    write(join(attempt, 'result.json'), report);
    write(previous, report);
  }
}

export async function verifyLiveWalkthroughDrafts(directory) {
  const endpoints = {},
    retained = {},
    unlock = [];
  try {
    for (const side of ['a', 'b']) {
      const descriptor = read(join(directory, 'manual', 'walkthrough-' + side + '.json'));
      const status = await liveWalkthroughStatus(descriptor);
      const files = retainedDraftFiles(directory, descriptor.environment, side);
      assert.equal(join(directory, descriptor.consoleCorrections.drafts.credentialFile), files.peopleFile);
      unlock.push(await fileLease(files.lockFile));
      const lab = read(join(directory, descriptor.laboratory, 'private/lab.json'));
      assert.equal(lab.id, descriptor.environment);
      endpoints[side] = { origin: descriptor.console, api: lab[side].api };
      const state = read(files.stateFile);
      retained[side] = { state, save: () => write(files.stateFile, state), status, descriptor };
    }
    return await exercise(directory, directory, endpoints, retained);
  } finally {
    for (const release of unlock.reverse()) await release();
  }
}

export async function verifyConsoleDrafts(directory, pairedDirectory, observeFixture) {
  if (pairedDirectory) return exercise(directory, pairedDirectory, {}, undefined, observeFixture);
  const shared = sharedConsoleFor(directory, { verifyFiles: true });
  assert(shared, 'Select the corrected console first');
  const endpoints = {};
  return withHTTPPair(
    directory,
    'console-drafts',
    async (sandbox) => {
      const previews = [];
      try {
        for (const side of ['a', 'b']) {
          const { origin, api } = endpoints[side],
            log = join(sandbox, 'logs', 'drafts-' + side + '-preview.log');
          const child = launch(
            process.execPath,
            [
              join(root, 'apps/console/node_modules/vite/bin/vite.js'),
              'preview',
              '--outDir',
              shared.console.directory,
            ],
            {
              ...process.env,
              CONSOLE_PORT: new URL(origin).port,
              API_PROXY_TARGET: api,
            },
            log,
            join(root, 'apps/console'),
          );
          const exit = once(child, 'exit');
          previews.push({ child, exit });
          let ready = false;
          for (let n = 0; n < 150; n++) {
            assert.equal(child.exitCode, null, 'Console preview exited before startup');
            if (
              await fetch(origin)
                .then((r) => r.ok)
                .catch(() => false)
            ) {
              ready = true;
              break;
            }
            await sleep(200);
          }
          assert(ready, 'Console preview failed to listen');
        }
        return await exercise(directory, sandbox, endpoints);
      } finally {
        await Promise.all(
          previews.map(async ({ child, exit }) => {
            if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
            await exit;
          }),
        );
      }
    },
    {
      async configure({ side, cfg }) {
        const origin = 'http://127.0.0.1:' + (await availablePort()),
          api = 'http://' + cfg.http_addr;
        cfg.trusted_origins = [cfg.trusted_origins, origin].filter(Boolean).join(',');
        endpoints[side] = { origin, api };
      },
    },
  );
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [directory, paired] = process.argv.slice(2);
  assert(directory, 'Supply an architecture run');
  const result =
    paired === '--walkthrough'
      ? await verifyLiveWalkthroughDrafts(runDirectory(directory))
      : await verifyConsoleDrafts(runDirectory(directory), paired ? runDirectory(paired) : undefined);
  console.log(
    JSON.stringify({
      passed: result.passed,
      consoleSHA256: result.consoleSHA256,
      sides: Object.keys(result.sides),
    }),
  );
}
