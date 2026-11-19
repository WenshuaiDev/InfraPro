import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, expect } from '@playwright/test';
import { digest, launch, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { withHTTPPair } from './architecture-http-sandbox.mjs';
import { seedPaginationProjects } from './architecture-pagination-sample.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { readServedConsole } from './architecture-visual-frontend.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';
import { availablePort } from './fixtures/parsing-lab.mjs';

async function waitForPreview(origin, child) {
  for (let n = 0; n < 150; n++) {
    assert.equal(child.exitCode, null, 'The console preview exited during startup');
    if (await fetch(origin).then(response => response.ok).catch(() => false)) return;
    await sleep(200);
  }
  throw Error('The shared console preview did not start');
}

async function seedProjects(pair, account) {
  const { call } = await pair.login(account);
  const scope = randomUUID();
  const accepted = [];
  const checkpoint = new Error('The first three public project commands committed');
  const stopAfterThirdProject = async (path, data, method) => {
    const result = await call(path, data, method);
    if (data && path.endsWith('/projects') && result.project_id) {
      accepted.push(result.project_id);
      if (accepted.length === 3) throw checkpoint;
    }
    return result;
  };
  await assert.rejects(seedPaginationProjects(stopAfterThirdProject, account.username, scope), err => err === checkpoint);
  const sample = await seedPaginationProjects(call, account.username, scope);
  const { tenant } = sample;
  const projectIds = sample.projects.map(project => project.id);
  assert.deepEqual(projectIds.slice(0, 3), accepted);
  assert.equal(sample.replayed.tenant, true);
  assert.equal(sample.replayed.projects, 3);
  const original = await call('/tenants/' + tenant + '/project-governance?status=all');
  assert.equal(original.overview.total, 11);
  assert.deepEqual(pair.problems, []);
  return {
    a: { tenant, projectIds, recoveredProjectIds: accepted },
    b: { tenant: pair.translate(tenant), projectIds: projectIds.map(id => pair.translate(id)), recoveredProjectIds: accepted.map(id => pair.translate(id)) },
  };
}

async function physicalClick(page, control) {
  await control.scrollIntoViewIfNeeded();
  const point = await control.evaluate(el => {
    const b = el.getBoundingClientRect();
    const x = b.left + b.width / 2;
    const y = b.top + b.height / 2;
    return { x, y, reached: el.contains(document.elementFromPoint(x, y)), horizontalOffset: scrollX };
  });
  assert(point.reached && point.horizontalOffset === 0, 'The control center is not reachable in the viewport');
  await page.mouse.click(point.x, point.y);
  return point;
}

async function reachByKeyboard(page, control) {
  const focused = await control.evaluate(target => {
    const items = [...document.querySelectorAll('a[href],button,input,select,textarea,[tabindex]')]
      .filter(el => el.tabIndex >= 0 && !el.matches(':disabled') && el.getClientRects().length);
    const previous = items[items.indexOf(target) - 1];
    if (!previous) return false;
    previous.focus({ preventScroll: true });
    return document.activeElement === previous;
  });
  assert(focused);
  await page.keyboard.press('Tab');
  await expect(control).toBeFocused();
  await page.keyboard.press('Space');
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.keyboard.press('Escape');
}

async function geometry(nav) {
  return nav.evaluate(el => {
    const box = node => {
      const rect = node.getBoundingClientRect();
      return {
        x: rect.x, right: rect.right, y: rect.y, width: rect.width, height: rect.height,
        clientWidth: node.clientWidth, scrollWidth: node.scrollWidth,
      };
    };
    const pager = el.parentElement.parentElement;
    return {
      viewportWidth: innerWidth,
      root: box(document.documentElement),
      parent: box(pager.parentElement),
      pager: box(pager),
      navigation: box(el),
      buttons: [...pager.querySelectorAll('button')].map(button => ({
        label: button.getAttribute('aria-label') || button.textContent.trim(),
        disabled: button.disabled,
        ...box(button),
      })),
    };
  });
}

async function rowsForSide(browser, side, origin, account, fixture, output) {
  const rows = [];
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
  try {
    const csrf = await context.request.get(origin + '/api/v1/auth/csrf');
    assert.equal(csrf.status(), 200);
    const login = await context.request.post(origin + '/api/v1/auth/login', {
      headers: { Origin: origin, 'X-CSRF-Token': (await csrf.json()).token },
      data: account,
    });
    assert.equal(login.status(), 200);
    assert.equal((await login.json()).status, 'authenticated');
    const page = await context.newPage();
    const nav = page.getByRole('navigation', { name: '项目治理分页' });
    const size = page.getByRole('combobox', { name: '每页条数', exact: true });
    const cards = page.locator('[data-project-card]');
    const titles = cards.getByRole('heading');
    for (const width of [1440, 1024, 768, 375]) {
      for (const theme of ['light', 'dark']) {
        const started = Date.now();
        await page.setViewportSize({ width, height: 1000 });
        const listing = '/api/v1/tenants/' + fixture.tenant + '/project-governance';
        const [response] = await Promise.all([
          page.waitForResponse(res => res.request().method() === 'GET' && new URL(res.url()).pathname === listing),
          page.goto(origin + '/t/' + fixture.tenant + '/projects'),
        ]);
        assert.equal(response.status(), 200);
        assert.equal((await response.json()).overview.total, 11);
        await expect(nav).toBeVisible();
        await expect(cards.first()).toBeVisible();
        await page.evaluate(async value => {
          document.documentElement.classList.toggle('dark', value === 'dark');
          await document.fonts.ready;
          await new Promise(requestAnimationFrame);
          await new Promise(requestAnimationFrame);
        }, theme);
        assert.equal(await page.locator('html').evaluate(el => el.classList.contains('dark')), theme === 'dark');
        const sizes = await geometry(nav);
        assert.equal(sizes.viewportWidth, width);
        assert.equal(sizes.root.scrollWidth, width);
        assert(sizes.pager.width > 0);
        assert(sizes.buttons.every(b => b.x >= 0 && b.right <= width));
        await reachByKeyboard(page, size);
        const mouse = await physicalClick(page, size);
        await expect(page.getByRole('listbox')).toBeVisible();
        const [sized] = await Promise.all([
          page.waitForResponse(res => {
            const url = new URL(res.url());
            return res.request().method() === 'GET' && url.pathname === listing &&
              url.searchParams.get('limit') === '10' && !url.searchParams.get('cursor');
          }),
          page.getByRole('option', { name: '10', exact: true }).click(),
        ]);
        assert.equal(sized.status(), 200);
        const firstPayload = await sized.json();
        assert.equal(firstPayload.items.length, 10);
        assert.equal(firstPayload.overview.total, 11);
        await expect(cards).toHaveCount(10);
        const first = await titles.allInnerTexts();
        const cardIds = () => cards.evaluateAll(nodes => nodes.map(node => node.getAttribute('data-project-card')));
        const firstIds = await cardIds();
        assert.equal(new Set(first).size, 10);
        const next = nav.getByRole('button', { name: '下一页', exact: true });
        await expect(next).toBeEnabled();
        await physicalClick(page, next);
        await expect(cards).toHaveCount(1);
        const last = await titles.first().innerText();
        const [lastId] = await cardIds();
        assert(!first.includes(last));
        assert.deepEqual([...firstIds, lastId].sort(), [...fixture.projectIds].sort());
        const previous = nav.getByRole('button', { name: '上一页', exact: true });
        await expect(previous).toBeEnabled();
        await physicalClick(page, previous);
        await expect(cards).toHaveCount(10);
        await expect.poll(() => titles.allInnerTexts()).toEqual(first);
        assert.deepEqual(await cardIds(), firstIds);
        const file = join(output, side + '-' + width + '-' + theme + '.png');
        await nav.scrollIntoViewIfNeeded();
        await page.screenshot({ path: file });
        const row = {
          width, theme, elapsedMs: Date.now() - started, scrollWidth: sizes.root.scrollWidth,
          listStatus: response.status(), sizeRequest: { limit: 10, status: sized.status(), items: firstPayload.items.length },
          mouse: true, keyboard: true, nextPage: true, previousPage: true,
          menuClick: mouse, geometry: sizes, firstPage: first, nextPageProject: last,
          firstPageIds: firstIds, nextPageId: lastId,
          screenshot: { file: relative(output, file), sha256: digest(readFileSync(file)) },
        };
        write(join(output, side + '-' + width + '-' + theme + '.json'), row);
        rows.push(row);
      }
    }
    return rows;
  } finally {
    await context.close();
  }
}

export async function verifyConsolePagination(directory) {
  const selection = sharedConsoleFor(directory, { verifyFiles: true });
  assert(selection, 'The signed shared console is required');
  const out = join(directory, 'console-corrections');
  const images = join(out, 'pagination-' + new Date().toISOString().replaceAll(':', '-'));
  mkdirSync(images, { recursive: true, mode: 0o700 });
  const published = { passed: false, consoleSHA256: selection.console.sha256, sides: {} };
  const endpoints = {};
  return withHTTPPair(directory, 'console-pagination', async sandbox => {
    const snapshot = read(join(sandbox, 'snapshot.json'));
    const baseline = read(join(sandbox, 'baseline.json'));
    assert.equal(snapshot.candidateSHA256, selection.backendReleaseSHA256);
    assert.equal(baseline.source_archive_sha256, selection.baselineReleaseSHA256);
    const account = read(join(sandbox, 'private/accounts.json')).admin;
    const pair = new PairedHTTP(sandbox, 'console-pagination', images);
    const browser = await chromium.launch();
    try {
      const fixtures = await seedProjects(pair, account);
      for (const side of ['a', 'b']) {
        const { origin, api } = endpoints[side];
        const log = join(images, side + '-preview.log');
        const child = launch(process.execPath, [
          join(root, 'apps/console/node_modules/vite/bin/vite.js'), 'preview', '--outDir', selection.console.directory,
        ], {
          ...process.env, CONSOLE_PORT: new URL(origin).port, API_PROXY_TARGET: api,
        }, log, join(root, 'apps/console'));
        const stopped = once(child, 'exit');
        try {
          await waitForPreview(origin, child);
          const served = await readServedConsole(origin, selection.console.files);
          assert.deepEqual(served.failures, []);
          assert.equal(served.sha256, selection.console.sha256);
          const rows = await rowsForSide(browser, side, origin, account, fixtures[side], images);
          published.sides[side] = {
            backendSHA256: side === 'a' ? baseline.source_archive_sha256 : snapshot.candidateSHA256,
            consoleSHA256: served.sha256, servedFiles: served.count, rows,
            tenant: fixtures[side].tenant, projectIds: fixtures[side].projectIds,
            recoveredProjectIds: fixtures[side].recoveredProjectIds,
            screenshotDirectory: relative(out, images),
          };
        } finally {
          child.kill('SIGTERM');
          await stopped;
          if (published.sides[side])
            published.sides[side].log = { file: relative(out, log), sha256: digest(readFileSync(log)) };
        }
      }
      const a = published.sides.a.rows;
      const b = published.sides.b.rows;
      assert.deepEqual(a.map(r => [r.firstPage, r.nextPageProject]), b.map(r => [r.firstPage, r.nextPageProject]));
      published.passed = true;
      published.sourceSnapshot = relative(directory, join(sandbox, 'snapshot.json'));
      published.appliedRoleSQL = snapshot.appliedRoleSQL;
      const previous = join(out, 'pagination.json');
      if (existsSync(previous)) write(join(images, 'previous.json'), read(previous));
      write(previous, published);
      return published;
    } finally {
      await browser.close();
      await pair.close();
      write(join(images, 'attempt.json'), published);
    }
  }, {
    async configure({ side, cfg }) {
      const origin = 'http://127.0.0.1:' + await availablePort();
      const api = 'http://' + cfg.http_addr;
      cfg.trusted_origins = [cfg.trusted_origins, origin].filter(Boolean).join(',');
      endpoints[side] = { origin, api };
    },
  });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = await verifyConsolePagination(runDirectory(process.argv[2]));
  console.log('Governance pagination passed on ' + Object.keys(result.sides).length + ' backends');
}
