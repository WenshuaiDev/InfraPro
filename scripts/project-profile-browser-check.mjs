import { chromium, expect as baseExpect } from '@playwright/test';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { startFilesLab } from './fixtures/files-lab.mjs';
import { seedFilesAcceptance, filesClient, filesLogin } from './fixtures/files-acceptance.mjs';

const expect = baseExpect.configure({ timeout: 20000 });
const out = resolve('.artifacts/project-profile/browser');
mkdirSync(out, { recursive: true });
const checks = [], errors = [];
let lab, browser, page, api;
const mark = note => { checks.push(note); console.log(note); };
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
async function signIn(target, credentials) {
  await target.goto(lab.origin + '/login');
  await target.getByLabel('用户名', { exact: true }).fill(credentials.username);
  await target.getByLabel('密码', { exact: true }).fill(credentials.password);
  await target.getByRole('button', { name: '登录', exact: true }).click();
  await target.waitForURL(url => url.pathname !== '/login');
}
async function confirmAction(label, reason) {
  const dialog = page.getByRole('dialog', { name: label, exact: true });
  await expect(dialog.getByRole('button', { name: `确认${label}`, exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '预览变更影响', exact: true }).click();
  await expect(dialog.getByRole('region', { name: '项目变更影响', exact: true })).toBeVisible();
  await dialog.getByLabel('变更原因', { exact: true }).fill(reason);
  await dialog.getByRole('button', { name: `确认${label}`, exact: true }).click();
  await expect(dialog).toHaveCount(0);
}
try {
  lab = await startFilesLab({ build: process.env.INFRAPRO_FILES_SKIP_BUILD !== '1', consolePort: 15696, apiPort: 18696, directory: join(out, 'lab'), consoleDist: process.env.INFRAPRO_PROFILE_CONSOLE_DIST });
  const state = await seedFilesAcceptance(lab);
  api = await filesClient(lab.origin);
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce', acceptDownloads: true });
  page = await context.newPage(); page.on('pageerror', error => errors.push(error.message));
  await signIn(page, filesLogin);
  const tenant = state.tenants[0], project = state.projects[0];
  const profilePath = `/t/${tenant}/p/${project}/profile`, base = `/tenants/${tenant}/projects/${project}`, go = () => page.goto(lab.origin + profilePath);
  await go();
  await expect(page.getByRole('heading', { name: '责任分工', exact: false })).toBeVisible();
  await expect(page.getByText('未设置', { exact: true })).toHaveCount(2);
  await expect(page.getByText('尚未填写项目说明。', { exact: true })).toBeVisible();
  await expect(page.getByText('尚未添加普通附件。', { exact: true })).toBeVisible();
  await expect(page.getByText('当前项目授权', { exact: true })).toHaveCount(0);
  mark('A brief renders real empty dates, description and attachments, without raw authorization details');

  const edit = page.getByRole('button', { name: '编辑项目资料', exact: true });
  await edit.click();
  const editor = page.getByRole('dialog', { name: '编辑项目资料', exact: true });
  await editor.getByLabel('项目名称', { exact: true }).fill('青澜大桥健康监测 · A 方案验收');
  await editor.getByLabel('业务目的', { exact: true }).fill('持续掌握桥梁结构状态，为养护决策提供依据。');
  await editor.getByLabel('项目说明', { exact: true }).fill('项目简报隔离验收，保留真实业务约束。');
  await editor.getByLabel('计划开始日期', { exact: true }).fill('2026-11-01');
  await editor.getByLabel('计划结束日期', { exact: true }).fill('2026-10-31');
  await editor.getByLabel('变更原因', { exact: true }).fill('A 方案真实编辑与日期约束验收');
  await editor.getByRole('button', { name: '保存项目资料', exact: true }).click();
  await expect(editor.getByText('结束日期不得早于开始日期。', { exact: true })).toBeVisible();
  await editor.getByLabel('计划结束日期', { exact: true }).fill('2027-10-31');
  await editor.getByRole('button', { name: '保存项目资料', exact: true }).click();
  await expect(editor).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('heading', { name: '青澜大桥健康监测 · A 方案验收', exact: true })).toBeVisible();
  const saved = await api.call(base + '/profile');
  expect(saved.planned_start).toBe('2026-11-01'); expect(saved.planned_end).toBe('2027-10-31');
  expect(saved.status).toBe('active');
  await expect(page.getByText('2026.11.01', { exact: true })).toBeVisible();
  mark('Real edit persists name/purpose/description/dates after reload; reversed dates are rejected and status remains independent');

  await page.getByRole('button', { name: '变更业务负责人', exact: true }).click();
  await page.getByLabel('选择业务负责人', { exact: true }).fill(state.accounts.reader.username);
  await page.getByRole('button', { name: `${state.accounts.reader.display_name}（${state.accounts.reader.username}）`, exact: true }).click();
  await confirmAction('变更业务负责人', '验证业务责任不自动加入项目');
  expect((await api.call(base + '/profile')).owner.account_id).toBe(state.accounts.reader.accountID);
  expect((await api.call(base + '/members')).items.some(member => member.account_id === state.accounts.reader.accountID)).toBe(false);
  await expect(page.getByText(state.accounts.reader.display_name, { exact: true })).toBeVisible();
  const more = page.getByRole('button', { name: '更多项目操作', exact: true });
  await more.click(); await page.getByRole('menuitem', { name: '归档项目', exact: true }).click();
  const archiveDialog = page.getByRole('dialog', { name: '归档项目', exact: true });
  await expect.poll(() => archiveDialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
  // A refresh failure must disable submission without making the return-focus target unfocusable.
  const transientContext = '**' + base + '/context';
  await page.route(transientContext, route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '短暂刷新故障验收' }) }));
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(archiveDialog.getByRole('button', { name: '预览变更影响', exact: true })).toBeDisabled();
  await page.keyboard.press('Escape'); await expect(archiveDialog).toHaveCount(0); await expect(more).toBeFocused();
  await page.unroute(transientContext); await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByRole('button', { name: '变更业务负责人', exact: true })).toBeEnabled();
  mark('Owner changes use actual candidate/impact/confirmation APIs without granting membership; menu drawers preserve keyboard focus');

  await page.getByRole('button', { name: '添加附件', exact: true }).click();
  await page.getByLabel('选择普通附件', { exact: true }).setInputFiles(join(state.samples, '合成平面图.png'));
  await page.getByLabel('显示名称', { exact: true }).fill('A 方案布设平面图.png');
  await page.getByLabel('操作原因', { exact: true }).fill('项目资料原位附件闭环');
  await page.getByRole('button', { name: '上传并提交检查', exact: true }).click();
  await expect(page.getByRole('button', { name: '查看本代附件修订', exact: true })).toBeVisible({ timeout: 45000 });
  await page.getByRole('button', { name: '查看本代附件修订', exact: true }).click();
  const preview = page.getByRole('img', { name: 'A 方案布设平面图.png 修订 1 的安全预览', exact: true });
  await expect(preview).toBeVisible(); expect(new URL(page.url()).pathname).toBe(profilePath);
  const downloadEvent = page.waitForEvent('download');
  await page.getByRole('link', { name: '下载原件', exact: true }).click();
  const download = await downloadEvent; await download.saveAs(join(out, 'download.png'));
  expect(hash(readFileSync(join(out, 'download.png')))).toBe(hash(readFileSync(join(state.samples, '合成平面图.png'))));
  await page.reload(); await expect(preview).toBeVisible();
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  const file = page.getByRole('button', { name: '查看附件 A 方案布设平面图.png', exact: true });
  await file.focus(); await page.keyboard.press('Enter'); await expect(preview).toBeVisible();
  await page.keyboard.press('Escape'); await expect(preview).toHaveCount(0); await expect(file).toBeFocused();
  mark('Actual upload/checker/PNG preview/download hash and reloadable deep link stay on the project profile; close restores file focus');

  // Assign independent project roles; tenant roles deliberately do not grant project rights.
  const combinations = { profile: ['project.profile.read'], reader: ['project.profile.read', 'project.attachments.read'], downloader: ['project.profile.read', 'project.attachments.read', 'project.attachments.download'], manager: ['project.profile.read', 'project.profile.update', 'project.attachments.read', 'project.attachments.manage'] };
  for (const [key, permission_codes] of Object.entries(combinations)) {
    const person = state.accounts[key];
    const membership = await api.call(base + '/members', { tenant_membership_id: person.memberID, expected_project_version: (await api.call(base + '/profile')).version, client_request_id: randomUUID(), reason: '项目资料权限矩阵' });
    person.projectMembership = membership.object_id;
    const role = await api.call(base + '/roles', { name: `项目简报 ${key}`, permission_codes, expected_project_version: (await api.call(base + '/profile')).version, client_request_id: randomUUID(), reason: '最小项目权限验收' });
    const body = { role_ids: [role.object_id] }, rolePath = base + `/members/${membership.object_id}/roles`, impact = await api.call(rolePath + '/preview', body);
    await api.call(rolePath, { ...body, expected_version: impact.expected_version, expected_project_version: impact.expected_project_version, preview_hash: impact.preview_hash, role_versions: impact.role_versions, client_request_id: randomUUID(), reason: '替换默认角色以验证独立资格' }, 'PUT');
  }
  for (const key of Object.keys(combinations)) {
    const viewer = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
    try {
      const target = await viewer.newPage(); target.on('pageerror', error => errors.push(error.message));
      await signIn(target, state.accounts[key]); await target.goto(lab.origin + profilePath);
      await expect(target.getByRole('heading', { name: saved.name, exact: true })).toBeVisible();
      await expect(target.getByRole('button', { name: '编辑项目资料', exact: true })).toHaveCount(key === 'manager' ? 1 : 0);
      await expect(target.getByRole('button', { name: '变更业务负责人', exact: true })).toHaveCount(0);
      await expect(target.getByRole('button', { name: '交接项目管理员', exact: true })).toHaveCount(0);
      await expect(target.getByRole('link', { name: '变更记录', exact: true })).toHaveCount(0);
      await expect(target.getByRole('main').getByRole('link', { name: '项目成员', exact: true })).toHaveCount(0);
      if (key === 'profile') {
        await expect(target.getByText('当前没有本空间普通附件查看权限；资料查看权与附件权限分别授予。', { exact: true })).toBeVisible();
        await expect(target.locator('[data-file-row]')).toHaveCount(0);
      } else {
        await expect(target.getByRole('button', { name: '添加附件', exact: true })).toHaveCount(key === 'manager' ? 1 : 0);
        await target.getByRole('button', { name: '查看附件 A 方案布设平面图.png', exact: true }).click();
        await expect(target.getByRole('heading', { name: 'A 方案布设平面图.png', exact: true })).toBeVisible();
        await expect(target.getByRole('link', { name: '下载原件', exact: true })).toHaveCount(key === 'downloader' ? 1 : 0);
        await expect(target.getByRole('img', { name: 'A 方案布设平面图.png 修订 1 的安全预览', exact: true })).toHaveCount(key === 'downloader' ? 1 : 0);
      }
    } finally { await viewer.close(); }
  }
  mark('Four real project roles verify independent profile/read/download/manage rights and hide unauthorized governance/member/audit controls');

  await go(); await more.click(); await page.getByRole('menuitem', { name: '归档项目', exact: true }).click();
  await confirmAction('归档项目', 'A 方案生命周期验收');
  await expect(page.getByRole('region', { name: '项目归档信息', exact: true })).toBeVisible();
  await expect(page.getByText('A 方案生命周期验收', { exact: true })).toBeVisible();
  await expect(edit).toHaveCount(0);
  await expect(page.getByRole('button', { name: '变更业务负责人', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '添加附件', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '交接项目管理员', exact: true })).toBeVisible();
  await file.click(); await expect(preview).toBeVisible(); await page.keyboard.press('Escape');
  await page.screenshot({ path: join(out, 'archived.png'), fullPage: true, animations: 'disabled' });
  await page.getByRole('button', { name: '恢复项目', exact: true }).click();
  await confirmAction('恢复项目', '验证恢复沿用原项目与授权');
  await expect(edit).toBeVisible(); await expect(page.getByRole('button', { name: '添加附件', exact: true })).toBeVisible();
  expect((await api.call(base + '/profile')).status).toBe('active');
  mark('Actual archive makes profile/owner/upload read-only but preserves attachment preview and admin transfer; actual restore re-enables eligible operations');

  for (const width of [1440, 1024, 720, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: join(out, `profile-${width}.png`), fullPage: true, animations: 'disabled' });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: join(out, 'profile-dark.png'), fullPage: true, animations: 'disabled' });
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  mark('Light/dark and 1440/1024/720/375/320px layouts avoid horizontal overflow');

  const profilePattern = '**' + base + '/profile', contextPattern = '**' + base + '/context';
  await page.route(profilePattern, async route => {
    const response = await route.fetch(), value = await response.json();
    await route.fulfill({ response, json: { ...value, name: '交通基础设施健康监测项目'.repeat(8), purpose: '持续监测与协同养护。'.repeat(75), description: '', planned_start: '', planned_end: '', management_gap: true, administrator: { ...value.administrator, available: false }, owner: { ...value.owner, available: false } } });
  });
  await go(); await expect(page.getByText('项目管理员空缺', { exact: true })).toBeVisible();
  await expect(page.getByText('业务负责人暂不可用', { exact: true })).toBeVisible();
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  await page.screenshot({ path: join(out, 'long-content-management-gap.png'), fullPage: true, animations: 'disabled' });
  await page.unroute(profilePattern); await page.setViewportSize({ width: 1440, height: 1000 }); await go();
  mark('Injected long text, missing dates and unavailable responsible people show contextual guidance and wrap on narrow screens');

  await file.click(); await expect(preview).toBeVisible();
  await page.route(contextPattern, async route => {
    const response = await route.fetch(), value = await response.json();
    await route.fulfill({ response, json: { ...value, authorization: { ...value.authorization, permissions: [], allowed_operations: [] } } });
  });
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByText('当前未获项目资料查看权限，请联系项目管理员。', { exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: saved.name, exact: true })).toHaveCount(0);
  await expect(page.getByText('当前项目授权', { exact: true })).toHaveCount(0);
  await page.unroute(contextPattern); await go(); await expect(edit).toBeVisible();
  await page.route(profilePattern, route => route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'permission_denied', message: '项目资料读取资格变化' }) }));
  await page.waitForResponse(response => response.url().endsWith(base + '/profile') && response.status() === 403, { timeout: 45000 });
  await expect(page.getByRole('heading', { name: '业务目的', exact: true })).toHaveCount(0);
  await expect(page.locator('[data-file-row]')).toHaveCount(0);
  await page.unroute(profilePattern); await go(); await expect(edit).toBeVisible();
  mark('Injected context permission loss and a denied profile refresh clear protected content/open previews; a fresh allowed read recovers');

  // Last mutation intentionally ends the old administrator's project membership.
  const oldProfile = await api.call(base + '/profile');
  let deniedContextReads = 0;
  const countDenials = response => { if (response.url().endsWith(base + '/context') && response.status() === 404) deniedContextReads++; };
  page.on('response', countDenials);
  await page.getByRole('button', { name: '交接项目管理员', exact: true }).click();
  await page.getByLabel('选择接任管理员', { exact: true }).fill(state.accounts.manager.username);
  await page.getByRole('button', { name: `${state.accounts.manager.display_name}（${state.accounts.manager.username}）`, exact: true }).click();
  await confirmAction('交接项目管理员', 'A 方案管理员交接闭环');
  await expect(page.getByRole('heading', { name: '当前项目不可访问', exact: true })).toBeVisible();
  expect(deniedContextReads).toBeLessThanOrEqual(4);
  page.off('response', countDenials);
  const successor = await filesClient(lab.origin, state.accounts.manager.username);
  try {
    const next = await successor.call(base + '/profile');
    expect(next.administrator.account_id).toBe(state.accounts.manager.accountID);
    expect(next.owner.account_id).toBe(oldProfile.owner.account_id);
    // The member list defaults to active members; inspect the stable historical
    // membership directly instead of treating absence from that list as proof.
    const previousMember = await successor.call(base + '/members/' + oldProfile.administrator_membership_id);
    expect(previousMember.status).toBe('ended');
    const tenantMembers = await api.call(`/tenants/${tenant}/members`);
    expect(tenantMembers.items.find(member => member.account.id === oldProfile.administrator.account_id).membership_status).toBe('active');
  } finally { await successor.client.dispose(); }
  mark('Real administrator transfer activates the successor, ends old project access, preserves owner and tenant membership');

  expect(errors).toEqual([]);
  writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: true, synthetic: true, checks, errors, note: 'Writes use an isolated real backend. Long content, management gaps, permission loss and denied profile reads explicitly inject responses; role matrix/archive/restore/transfer use real APIs.' }, null, 2));
  console.log(`Passed ${checks.length} project-profile browser journeys.`);
} catch (error) {
  if (page) await page.screenshot({ path: join(out, 'failure.png'), fullPage: true }).catch(() => {});
  writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: false, checks, errors, error: String(error) }, null, 2));
  throw error;
} finally { await api?.client.dispose(); await browser?.close(); await lab?.close(); }
