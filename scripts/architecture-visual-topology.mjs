import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readdirSync, renameSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { chromium, expect } from '@playwright/test';
import { read, safeDiagnosticLine, sleep, write } from './architecture-lab.mjs';
import { viewports, visualRoleGrants } from './architecture-visual.mjs';
import { finishVisualFrontend, visualFrontend } from './architecture-visual-frontend.mjs';
import { servedDiagnosticsStyles } from './architecture-static.mjs';

const groups = {
  application: ['identity-api', 'control-api', 'monitoring-query-api', 'transfer-api'],
  worker: ['monitoring-publish-worker', 'monitoring-incremental-worker', 'monitoring-history-worker', 'monitoring-read-worker'],
};
const labels = {
  application:'应用', worker:'后台进程', gateway:'网关', tool:'工具',
  continuous:'常驻', on_demand:'按需', disabled:'未启用', idle:'空闲',
  healthy:'正常', degraded:'部分异常', unavailable:'不可用', unknown:'未知',
};

export async function visualTopology(dir, output = dir) {
  const { output: profile, frontend } = await visualFrontend(dir, 'b', output);
  output = profile;
  const roleSQL = visualRoleGrants(dir,'b');
  const cfg = read(join(dir, 'private/lab.json')).b, diagnostics = join(dirname(cfg.config), 'diagnostics');
  const account = read(join(diagnostics, 'accounts.json')), manifest = read(join(diagnostics, 'manifest.json'));
  const frozen = read(join(dir, 'baseline.json'));
  const existing = new Set(read(join(frozen.source, 'contracts/diagnostics/runtime-catalog.json')).entries.map(role => role.id));
  // config init exports the installed catalog as a JSON array. The tracked
  // repository contract wraps its entries with a schema version.
  const installed = read(join(diagnostics, 'runtime-catalog.json'));
  assert(Array.isArray(installed) && new Set(installed.map(role=>role.id)).size === installed.length);
  const added = installed.filter(role => !existing.has(role.id));
  assert(added.length > 0 && added.every(role => manifest.components.some(c => c.id === role.id && c.kind === role.kind)));
  const kinds = [...new Set(added.map(role => role.kind))];
  const folder = join(output, 'visual', 'live-topology'), origin = 'http://127.0.0.1:' + cfg.ports.diagnostics;
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  if (existsSync(join(folder,'result.json'))) {
    const older = join(folder,'attempts',String(Date.now()));
    mkdirSync(older,{recursive:true,mode:0o700});
    for (const name of readdirSync(folder).filter(v => /\.png$|^result\.json$/.test(v))) renameSync(join(folder,name),join(older,name));
  }
  const servedStyles = await servedDiagnosticsStyles(dir);
  const browser = await chromium.launch(), report = { frontend, servedStyles, addedRoles:added.map(role => role.id).sort(), sourceDirectory: dir, candidate: read(join(dir, 'candidate.json')).inputs.sourceSHA256, snapshot: read(join(dir, 'snapshot/manifest.json')).pg.seed, roleSQL, rows: [], passed: false };
  try {
    for (const viewport of viewports) {
      const ctx = await browser.newContext({ viewport, locale: 'zh-CN', timezoneId: 'Asia/Shanghai', hasTouch: viewport.width === 375 });
      const row = { viewport, roles: [], listed: [], screenshots: [], passed: false };
      report.rows.push(row);
      let page;
      try {
        page = await ctx.newPage();
        const capture = async (label) => {
          const file = label + '-' + viewport.width + '.png';
          await page.screenshot({ path: join(folder, file), animations: 'disabled', caret: 'hide' }); row.screenshots.push(file);
        };
        await page.goto(origin + '/components');
        await page.getByLabel('诊断账号', { exact: true }).fill(account.username);
        await page.getByLabel('密码', { exact: true }).fill(account.password);
        await page.getByRole('button', { name: '登录诊断', exact: true }).click();
        await expect(page.getByRole('heading', { name: '运行组件', exact: true })).toBeVisible({ timeout: 15000 });
        for (const kind of kinds) {
          const roles = groups[kind] ?? [], required = added.filter(role => role.kind === kind);
          row.checking = { kind, step: 'list' };
          await page.goto(origin + '/components');
          const listResponse = cursor => page.waitForResponse(response => {
            const url = new URL(response.url());
            return url.pathname === '/api/diagnostics/v1/components' && url.searchParams.get('kind') === kind &&
              (url.searchParams.get('cursor') || '') === cursor;
          });
          let [response] = await Promise.all([listResponse(''), page.getByLabel('类型', { exact: true }).selectOption(kind)]);
          const count = manifest.components.filter(c => c.kind === kind).length;
          await expect(page.getByText('共 ' + count + ' 条', { exact: true })).toBeVisible();
          const found = new Set(), rows = page.locator('a[href^="/components/"]');
          for (let index = 1; index <= count; index++) {
            assert.equal(response.status(), 200);
            const result = (await response.json()).data;
            assert.equal(result.total, count);
            await expect(rows.first()).toHaveAttribute('href', '/components/' + result.items[0].id);
            const rendered = await rows.evaluateAll(links => links.map(link => ({
              id: new URL(link.href).pathname.split('/').at(-1), title:link.innerText,
              cells: Array.from(link.closest('tr').cells, cell => cell.innerText.trim()),
            })));
            assert.deepEqual(rendered.map(item => item.id), result.items.map(item => item.id));
            const screenshot = kind + '-list-' + index;
            await capture(screenshot);
            for (const [position, item] of result.items.entries()) {
              if (!required.some(expected => expected.id === item.id)) continue;
              const definition = manifest.components.find(c => c.id === item.id);
              const visible = rendered[position];
              assert.equal(item.expectation, definition.expectation);
              assert.equal(visible.title, definition.display_name);
              assert.equal(visible.cells.length, 5);
              assert(visible.cells[1].includes(labels[kind]) && visible.cells[1].includes(labels[item.expectation]));
              assert(visible.cells[2].includes(labels[item.display_state]));
              assert(visible.cells[3].includes(item.ready_replicas + '/' + item.expected_replicas + ' 可用'));
              found.add(item.id);
              row.listed.push({ id:item.id, kind, expectation:item.expectation, state:item.display_state, visible,
                ready:item.ready_replicas, expected:item.expected_replicas, screenshot:screenshot + '-' + viewport.width + '.png' });
            }
            if (required.every(role => found.has(role.id))) break;
            assert(result.next_cursor);
            const next = page.getByRole('button', { name: '下一页', exact: true });
            await expect(next).toBeEnabled();
            [response] = await Promise.all([listResponse(result.next_cursor), next.click()]);
          }
          assert(required.every(role => found.has(role.id)));
          for (const id of roles) {
            // The independent operator account admits two reads per second.
            // A component and replica page each load several real endpoints.
            await sleep(4600);
            row.checking = { kind, id, step: 'component' };
            const role = manifest.components.find(c => c.id === id);
            await page.goto(origin + '/components/' + id);
            await expect(page.getByRole('heading', { name: role.display_name, exact: true })).toBeVisible();
            const replica = id + '-1';
            row.checking.step = 'replica identity';
            const path = '/api/diagnostics/v1/replicas/' + replica;
            const nextReport = () => page.waitForResponse(r => new URL(r.url()).pathname === path,{timeout:15000});
            const [arrived] = await Promise.all([nextReport(),page.getByRole('link', { name: replica, exact: true }).click()]);
            assert.equal(arrived.status(),200);
            let live = (await arrived.json()).data;
            await expect(page.getByRole('heading', { name: replica, exact: true })).toBeVisible();
            const until = Date.now() + 30000;
            const reportFile = join(dir, 'private', 'b', 'reports', id, 'process.json');
            const native = read(reportFile);
            assert.equal(native.replica_id, replica);
            assert(native.process.boot_id && native.process.pid > 0);
            while ((live.process?.boot_id !== native.process.boot_id || live.freshness !== 'fresh') && Date.now() < until) {
              const update = await nextReport();
              assert.equal(update.status(),200);
              live = (await update.json()).data;
            }
            assert.equal(live.replica.component_id, id);
            row.checking.freshness = live.freshness;
            row.checking.sameBoot = live.process?.boot_id === native.process.boot_id;
            assert.equal(live.process?.boot_id, native.process.boot_id);
            assert.equal(live.freshness, 'fresh');
            process.kill(native.process.pid, 0);
            await expect(page.getByText(live.process.boot_id, { exact: true })).toBeVisible({ timeout: 12000 });
            if (id === 'identity-api' || id === 'monitoring-history-worker') await capture(id);
            row.roles.push({ id, replica, title: role.display_name, state: live.display_state, facets: live.facets, freshness: live.freshness, hasProcessIdentity: true });
          }
        }
        row.passed = row.roles.length === Object.values(groups).flat().length &&
          row.listed.map(role => role.id).sort().join(',') === report.addedRoles.join(',') &&
          row.listed.every(role => row.screenshots.includes(role.screenshot));
        delete row.checking;
      } catch (error) {
        row.error = safeDiagnosticLine(error.message).slice(0,350);
        if (page && !page.isClosed()) {
          row.failureScreenshot = 'failure-' + viewport.width + '.png';
          await page.screenshot({ path: join(folder,row.failureScreenshot),animations:'disabled',caret:'hide' }).catch(() => {});
        }
      }
      finally { await ctx.close(); write(join(folder, 'result.json'), report); }
    }
    report.passed = report.rows.length === 3 && report.rows.every(r => r.passed);
    report.frontendAtEnd = await finishVisualFrontend(dir, 'b', output, frontend);
    write(join(folder, 'result.json'), report); return report;
  } finally { await browser.close(); }
}
