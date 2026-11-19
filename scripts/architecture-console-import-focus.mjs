import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { mkdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { chromium, expect } from '@playwright/test';
import { client, digest, launch, read, root, runDirectory, safeDiagnosticLine, sleep, write } from './architecture-lab.mjs';
import { pairedCandidate, withHTTPPair } from './architecture-http-sandbox.mjs';
import { resumeFocusPair } from './architecture-import-focus-resume.mjs';
import { importFocusReason, seedImportFocus } from './architecture-import-focus-sample.mjs';
import { consoleDigest, sharedConsoleFor } from './architecture-shared-console.mjs';
import { liveWalkthroughStatus } from './architecture-walkthrough-control.mjs';
import { availablePort } from './fixtures/parsing-lab.mjs';

const cases = [
  'keyboardEarly', 'keyboardLate', 'mouseEarly', 'mouseLate', 'userTab', 'userPointer',
  'rowMissing', 'filterChanged', 'scopeChanged', 'unmounted',
];
const viewport = { width: 375, height: 812 };

function observed() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

async function bounded(promise, description) {
  let timer;
  try {
    return await Promise.race([promise, new Promise((_, reject) => {
      timer = setTimeout(() => reject(Error('Timed out waiting for ' + description)), 15000);
    })]);
  } finally { clearTimeout(timer); }
}

function attachment(directory, file) {
  return { file: relative(directory, file), sha256: digest(readFileSync(file)) };
}

// Hold only the receipt of a real, already authorized 200. Preserve its status,
// headers and complete body. A disposed request is expected during navigation.
async function holdList(page, apiPath, query, target, responses) {
  const first = observed(), hold = observed(), jobs = new Set();
  const predicate = url => url.pathname === apiPath &&
    ['q', 'states', 'cursor'].every(key => url.searchParams.get(key) === query.get(key));
  const handler = async route => {
    if (route.request().method() !== 'GET') return route.continue();
    const item = { method: 'GET', template: '/api/v1/tenants/:tenant_id/monitoring/imports', at: Date.now() };
    responses.push(item);
    const work = (async () => {
      const upstream = await route.fetch();
      try {
        const bytes = await upstream.body();
        item.status = upstream.status();
        assert.equal(item.status, 200);
        item.sha256 = digest(bytes);
        const body = JSON.parse(bytes);
        item.rowCount = body.items.length;
        item.containsOriginalRow = body.items.some(batch => batch.id === target);
        item.serverReturnedAt = Date.now();
        first.resolve();
        await hold.promise;
        try {
          await route.fulfill({ response: upstream });
          item.deliveredAt = Date.now();
        } catch (error) {
          if (!/closed|already handled|Invalid InterceptionId|canceled|cancelled/i.test(error.message)) throw error;
          item.clientCancelled = true;
        }
      } finally { await upstream.dispose(); }
    })();
    jobs.add(work);
    try { await work; } finally { jobs.delete(work); }
  };
  await page.route(predicate, handler);
  return {
    seen: () => bounded(first.promise, 'the original list HTTP response'), release: hold.resolve,
    async close() { hold.resolve(); await page.unroute(predicate, handler); await Promise.all([...jobs]); },
  };
}

async function sampleFocus(page, row) {
  return page.evaluate(id => {
    const el = document.activeElement;
    const focus = el?.id || el?.getAttribute('aria-label') ||
      el?.textContent?.trim().slice(0, 42) || el?.tagName || 'none';
    return {
      at: Date.now(), documentEpoch: performance.timeOrigin,
      focused: el === document.body ? 'BODY' : focus,
      drawer: !!document.querySelector('[role="dialog"]'),
      rowVisible: !!document.getElementById(id)?.getClientRects().length,
    };
  }, row);
}

async function teamSwitch(page, tenant) {
  const trigger = page.getByRole('button', { name: /^切换工作空间：/ });
  const sidebar = page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true });
  await expect(trigger.or(sidebar).first()).toBeVisible();
  if (!await trigger.isVisible()) await sidebar.click();
  await trigger.click();
  const picker = page.getByRole('dialog', { name: '切换工作空间', exact: true });
  await picker.getByLabel('选择租户', { exact: true }).selectOption({ label: tenant.name });
  await picker.getByRole('button', { name: new RegExp(tenant.name + '.*进入租户') }).click();
  await expect(page).toHaveURL(new RegExp('/t/' + tenant.id + '(?:/|$)'));
  await expect(picker).toHaveCount(0);
  await expect(page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true })).toBeVisible();
}

function observeAssets(page, origin, expected, manifest, errors) {
  const jobs = [];
  page.on('pageerror', error => errors.push(error.name));
  page.on('response', response => {
    const url = new URL(response.url());
    if (url.origin !== origin || response.status() !== 200 || url.pathname.startsWith('/api/')) return;
    const file = url.pathname.startsWith('/assets/') ? url.pathname.slice(1) :
      response.request().resourceType() === 'document' ? 'index.html' : null;
    if (!file) return;
    assert(expected[file], 'Loaded an undeclared static asset: ' + file);
    jobs.push(response.body().then(bytes => {
      const sha = digest(bytes);
      assert.equal(sha, expected[file], file);
      manifest.set(file, sha);
    }).catch(error => {
      if (!/No resource with given identifier|No data found for resource|Target.*closed/.test(error.message)) errors.push(error.message);
    }));
  });
  return async () => { await Promise.all(jobs); assert.deepEqual(errors, []); };
}

async function exercise({ name, page, sample, origin, call, otherTenant, directory, folder, evidence }) {
  const missing = name === 'rowMissing', mouse = name.startsWith('mouse');
  const early = name.endsWith('Early');
  const api = '/tenants/' + sample.tenant + '/monitoring/imports';
  let target = sample.target;
  if (missing) {
    target = await call(api + '/' + sample.rows.at(-1).id);
    if (target.state !== 'uploading') target = await call(api, {
      name: '消失原行-' + randomUUID().slice(0, 8) + '.csv', size_bytes: '4',
      request_id: randomUUID(), expected_revision: '0', reason: importFocusReason,
    });
  }
  const rowID = 'import-' + target.id;
  const start = new URL(sample.listPath, origin);
  if (missing) start.search = new URLSearchParams({ q: target.name, states: 'uploading', limit: '10' }).toString();
  const row = page.getByRole('button', { name: target.name, exact: true });
  const drawer = page.getByRole('dialog');
  const actions = evidence.actions;
  await page.goto(start.href);
  await expect(page.getByRole('table', { name: '监测导入批次' })).toBeVisible({ timeout: 30000 });
  if (!missing) {
    const next = page.getByRole('button', { name: '下一页批次', exact: true });
    await expect(next).toBeEnabled();
    await next.click();
  }
  await expect(row).toBeVisible();
  const originalList = new URL(page.url());
  assert(missing || originalList.searchParams.has('cursor'));
  if (mouse) await row.click();
  else { await row.focus(); await page.keyboard.press('Enter'); }
  actions.push(mouse ? 'click original row' : 'Enter on original row');
  await expect(drawer.getByRole('heading', { name: target.name, exact: true })).toBeVisible({ timeout: 30000 });
  await drawer.getByRole('tab', { name: '处理记录', exact: true }).click();
  await expect(drawer).toContainText(importFocusReason, { timeout: 30000 });
  if (missing) {
    const data = await call(api + '/' + target.id);
    assert.equal(data.state, 'uploading');
    const retired = await call(api + '/' + target.id + '/cancel', {
      request_id: randomUUID(), expected_revision: data.revision, reason: '原行不再满足进行中筛选',
    });
    assert.equal(retired.state, 'cancelled');
    actions.push('POST cancel; GET uploading excludes the original row');
    evidence.changedBatch = { before: data.state, after: retired.state, revision: retired.revision };
  }
  const gate = await holdList(page, '/api/v1' + api, originalList.searchParams, target.id, evidence.responses);
  try {
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(drawer.getByRole('tab', { name: '处理记录', exact: true })).toHaveAttribute('data-state', 'active', { timeout: 30000 });
    await gate.seen();
    actions.push('reload actual history tab');
    if (early) {
      gate.release();
      await page.waitForFunction(id => !!document.getElementById(id), rowID);
    }
    evidence.originalRow = rowID;
    evidence.beforeClose = await sampleFocus(page, rowID);
    if (mouse) await drawer.getByRole('button', { name: '关闭批次详情', exact: true }).click();
    else await page.keyboard.press('Escape');
    actions.push(mouse ? 'click drawer close button' : 'press Escape');
    await expect(drawer).toHaveCount(0);
    await expect(page).toHaveURL(originalList.href);
    const afterClose = await sampleFocus(page, rowID);
    evidence.drawerClosed = !afterClose.drawer;
    evidence.focusedBefore = afterClose.focused;
    assert.equal(afterClose.rowVisible, early);
    const search = page.locator('#import-search');
    const delta = name === 'filterChanged' ? sample.rows[1] : null;
    let expected = rowID;
    if (!early) {
      await expect(search).toBeFocused();
      if (name === 'userTab') {
        await page.keyboard.press('Tab');
        expected = (await sampleFocus(page, rowID)).focused;
        assert(![rowID, 'BODY', 'import-search'].includes(expected));
        actions.push('move focus by Tab');
      } else if (name === 'userPointer') {
        await search.click();
        await page.keyboard.type('pending');
        evidence.typedValue = await search.inputValue();
        assert(evidence.typedValue.includes('pending'));
        expected = 'import-search';
        actions.push('click search and type');
      } else if (missing) {
        assert(evidence.responses.some(response => response.status === 200 && response.containsOriginalRow === false));
        expected = 'import-search';
      } else if (delta) {
        await search.fill(delta.name);
        await page.getByRole('button', { name: '搜索', exact: true }).click();
        await expect(page.getByRole('button', { name: delta.name, exact: true })).toBeVisible();
        assert.equal(new URL(page.url()).searchParams.get('q'), delta.name);
        expected = (await sampleFocus(page, rowID)).focused;
        actions.push('submit a different real list filter');
      } else if (name === 'scopeChanged') {
        await teamSwitch(page, otherTenant);
        const anchor = page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true });
        await anchor.focus();
        await expect(anchor).toBeFocused();
        expected = (await sampleFocus(page, rowID)).focused;
        actions.push('switch to a different authorized tenant');
      } else if (name === 'unmounted') {
        await page.reload({ waitUntil: 'domcontentloaded' });
        await expect(search).toBeVisible();
        assert.notEqual((await sampleFocus(page, rowID)).documentEpoch, afterClose.documentEpoch);
        expected = (await sampleFocus(page, rowID)).focused;
        actions.push('unload and reopen the same list document');
      }
      await sleep(110);
      evidence.listReleasedAt = Date.now();
      gate.release();
    }
    if (missing || delta || name === 'scopeChanged') {
      await expect(row).toHaveCount(0);
    } else await expect(row).toBeVisible();
    if (expected === rowID) await expect(row).toBeFocused();
    if (name === 'userPointer') await expect(search).toHaveValue(evidence.typedValue);
    await gate.close();
    await page.evaluate(() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done))));
    const end = await sampleFocus(page, rowID);
    evidence.expectedFocus = expected;
    evidence.actualFocus = end.focused;
    evidence.focusedAfter = end.focused;
    evidence.returnedRowVisible = end.rowVisible;
    assert.equal(end.focused, expected);
    assert.equal(end.rowVisible, !missing && !delta && name !== 'scopeChanged');
    assert(evidence.responses.length && evidence.responses.every(response => response.status === 200 && response.sha256));
    if (early || /Late$/.test(name)) {
      const firstDelivered = Math.min(...evidence.responses.filter(r => r.deliveredAt).map(r => r.deliveredAt));
      assert(Number.isFinite(firstDelivered));
      evidence.listReturnedAfterClose = firstDelivered > afterClose.at;
      assert.equal(evidence.listReturnedAfterClose, !early);
      evidence.listRelativeToCloseMs = firstDelivered - afterClose.at;
    }
    const photo = join(folder, name + '.png');
    await page.screenshot({ path: photo, animations: 'disabled' });
    const dom = join(folder, name + '-dom.json');
    write(dom, { expected, beforeClose: evidence.beforeClose, afterClose, end, responses: evidence.responses, actions });
    evidence.artifacts = [attachment(directory, photo), attachment(directory, dom)];
    evidence.passed = true;
  } finally { await gate.close(); }
}

function loadedAssets(directory, folder, files, shared) {
  const manifest = Object.fromEntries([...files].sort(([a], [b]) => a.localeCompare(b)));
  const paths = Object.keys(manifest);
  assert(manifest['index.html'] && paths.some(file => file.endsWith('.js')) && paths.some(file => file.endsWith('.css')));
  const file = join(folder, 'loaded-assets.json');
  write(file, manifest);
  return {
    matched: paths.every(path => manifest[path] === shared.console.files[path]),
    checked: paths.length, html: ['index.html'], js: paths.filter(path => path.endsWith('.js')),
    css: paths.filter(path => path.endsWith('.css')), sha256: consoleDigest(manifest), artifact: attachment(directory, file),
  };
}

async function sideTests({ directory, folder, browser, side, origin, seed, account, shared, record, save }) {
  const ctx = await browser.newContext({ viewport, isMobile: true, hasTouch: true });
  const session = await client({ origin, api: origin }, account);
  try {
    await ctx.addCookies((await session.client.storageState()).cookies);
    const sample = await seedImportFocus(session.call, { tenant: seed.tenant, scope: 'focus-suite-' + origin, keepLastUploading: true });
    const code = 'focus-' + randomUUID().slice(0, 8), otherName = '回焦范围 ' + code;
    const other = await session.call('/platform/tenants', {
      name: otherName, code, administrator_account_id: seed.admin, reason: '验证切换范围后的键盘焦点', client_request_id: randomUUID(),
    });
    assert(other.tenant_id);
    record.sample = { count: sample.count, pageSize: sample.pageSize, pageNumber: sample.pageNumber, target: sample.target };
    const files = new Map();
    const thisFolder = join(folder, side);
    mkdirSync(thisFolder, { recursive: true });
    for (const name of cases) {
      const page = await ctx.newPage();
      const finishAssets = observeAssets(page, origin, shared.console.files, files, []);
      const result = { passed: false, actions: [], responses: [], browserHTTP: [] };
      page.on('response', response => {
        const url = new URL(response.url());
        if (url.origin === origin && url.pathname.startsWith('/api/v1/')) result.browserHTTP.push({
          method: response.request().method(), status: response.status(), at: Date.now(),
          template: url.pathname.replace(/[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}/gi, ':id'),
        });
      });
      record.cases[name] = result;
      save();
      try {
        await exercise({ name, page, sample, origin, call: session.call, otherTenant: { id: other.tenant_id, name: otherName },
          directory, folder: thisFolder, evidence: result });
        await finishAssets();
      } finally { await page.close(); save(); }
    }
    record.assets = loadedAssets(directory, thisFolder, files, shared);
    record.passed = record.assets.matched && cases.every(name => record.cases[name]?.passed);
    save();
  } finally { await ctx.close(); await session.client.dispose(); }
}

async function preview({ origin, api, shared, folder, work }) {
  const command = join(root, 'apps/console/node_modules/vite/bin/vite.js');
  const child = launch(process.execPath, [command, 'preview', '--outDir', shared.console.directory],
    { ...process.env, CONSOLE_PORT: new URL(origin).port, API_PROXY_TARGET: api }, join(folder, 'preview.log'), join(root, 'apps/console'));
  const exit = once(child, 'exit');
  try {
    for (let i = 0; i < 150; i++) {
      assert.equal(child.exitCode, null);
      if (await fetch(origin).then(response => response.ok).catch(() => false)) break;
      await sleep(200);
    }
    assert.equal((await fetch(origin)).status, 200);
    await work();
  } finally { child.kill('SIGTERM'); await exit; }
}

async function retainedCases(directory, folder, shared, result, save) {
  const browser = await chromium.launch();
  try {
    for (const side of ['a', 'b']) {
      const fixture = read(join(directory, 'manual/walkthrough-' + side + '.json'));
      const before = await liveWalkthroughStatus(fixture);
      assert.equal(before.sharedConsoleSHA256, shared.console.sha256);
      const lab = read(join(directory, fixture.laboratory, 'private/lab.json'));
      assert.equal(lab.id, fixture.environment);
      const sample = fixture.consoleCorrections.importFocus;
      assert.equal(sample.count, 23);
      assert.equal(sample.pageNumber, 2);
      const account = read(join(directory, 'private/accounts.json')).admin;
      const origin = fixture.console, session = await client({ origin, api: lab[side].api }, account);
      const ctx = await browser.newContext({ viewport, isMobile: true, hasTouch: true });
      const files = new Map(), current = join(folder, side);
      mkdirSync(current, { recursive: true });
      const record = { backendSHA256: fixture.releaseSHA256, launchID: before.launchID,
        sample: { tenant: sample.tenant, target: sample.target, count: sample.count, pageSize: sample.pageSize }, cases: {} };
      result.sides[side] = record;
      try {
        await ctx.addCookies((await session.client.storageState()).cookies);
        // These are read-only visits to the live operator's retained second-page row.
        for (const name of ['keyboardEarly', 'keyboardLate', 'userTab']) {
          const page = await ctx.newPage(), errors = [];
          const finish = observeAssets(page, origin, shared.console.files, files, errors);
          const item = { passed: false, actions: [], responses: [] };
          record.cases[name] = item;
          try {
            await exercise({ name, page, sample, origin, directory, folder: current, evidence: item });
            await finish();
          } finally { await page.close(); save(); }
        }
        const after = await liveWalkthroughStatus(fixture);
        assert.equal(after.launchID, before.launchID);
        assert.equal(after.sharedConsoleSHA256, before.sharedConsoleSHA256);
        record.assets = loadedAssets(directory, current, files, shared);
        record.passed = record.assets.matched && Object.values(record.cases).every(item => item.passed);
      } finally { await ctx.close(); await session.client.dispose(); }
    }
  } finally { await browser.close(); }
}

assert(process.argv[2], 'Supply the architecture run directory');
assert([undefined, '--resume', '--walkthrough'].includes(process.argv[3]), 'Unknown import-focus mode');
assert.equal(process.argv.length, process.argv[3] === '--resume' ? 5 : process.argv[3] ? 4 : 3);
const directory = runDirectory(process.argv[2]);
const retained = process.argv[3] === '--walkthrough';
const base = join(directory, retained ? 'manual' : 'console-corrections');
const shared = sharedConsoleFor(directory, { verifyFiles: true });
assert(shared);
const run = new Date().toISOString().replaceAll(':', '-');
const folder = join(base, 'import-focus', run);
mkdirSync(folder, { recursive: true, mode: 0o700 });
const result = { passed: false, startedAt: new Date().toISOString(), consoleSHA256: shared.console.sha256, viewport, sides: {} };
const save = () => write(join(folder, 'attempt.json'), result);
const endpoints = {};
try {
  const runCases = async sandbox => {
    const snap = read(join(sandbox, 'snapshot.json')), sample = read(join(sandbox, 'samples.json'));
    const ids = { a: read(join(sandbox, 'baseline.json')).source_archive_sha256, b: snap.candidateSHA256 };
    assert.equal(ids.a, read(join(directory, 'baseline.json')).source_archive_sha256);
    assert.equal(ids.b, pairedCandidate(directory).inputs.sourceSHA256);
    result.snapshot = attachment(directory, join(sandbox, 'snapshot.json'));
    result.appliedRoleSQL = snap.appliedRoleSQL;
    const account = read(join(sandbox, 'private/accounts.json')).admin;
    const browser = await chromium.launch();
    try {
      for (const side of ['a', 'b']) {
        const record = { passed: false, backendSHA256: ids[side], cases: {} };
        result.sides[side] = record;
        mkdirSync(join(folder, side), { recursive: true });
        await preview({ ...endpoints[side], folder: join(folder, side), shared, work: () => sideTests({
          directory, folder, browser, side, origin: endpoints[side].origin, seed: { tenant: sample.structure.tenant, admin: sample.actors.admin },
          account, shared, record, save,
        }) });
      }
    } finally { await browser.close(); }
  };
  if (retained) await retainedCases(directory, folder, shared, result, save);
  else if (process.argv[3] === '--resume') {
    assert(process.argv[4], 'Supply the previously archived import-focus sandbox');
    result.resumed = true;
    await resumeFocusPair(directory, process.argv[4], endpoints, runCases);
  } else await withHTTPPair(directory, 'import-focus', runCases, { async configure({ side, cfg }) {
    const origin = 'http://127.0.0.1:' + await availablePort();
    endpoints[side] = { origin, api: 'http://' + cfg.http_addr };
    cfg.trusted_origins = [cfg.trusted_origins, origin].filter(Boolean).join(',');
  } });
  result.passed = ['a', 'b'].every(side => result.sides[side]?.passed);
  assert(result.passed);
} catch (error) {
  result.error = safeDiagnosticLine(error.message);
  throw error;
} finally {
  result.finishedAt = new Date().toISOString();
  save();
  write(join(base, retained ? 'live-import-focus.json' : 'import-focus.json'), result);
}
