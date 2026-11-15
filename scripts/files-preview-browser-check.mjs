import { chromium, expect as baseExpect } from '@playwright/test';
import { createHash, randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { startFilesLab } from './fixtures/files-lab.mjs';
import { seedFilesAcceptance, filesClient } from './fixtures/files-acceptance.mjs';

const out = resolve('.artifacts/files/document-preview'), samples = join(out, 'samples');
mkdirSync(out, { recursive: true });
if (process.env.INFRAPRO_PREVIEW_PYTHON) execFileSync(process.env.INFRAPRO_PREVIEW_PYTHON, ['scripts/fixtures/document-preview-samples.py', samples], { stdio: 'inherit' });
const expect = baseExpect.configure({ timeout: 20000 }), checks = [], errors = [], external = [];
let lab, api, browser, page;
const mark = text => { checks.push(text); console.log(text); };
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
try {
  lab = await startFilesLab({ build: process.env.INFRAPRO_FILES_SKIP_BUILD !== '1', consolePort: 15578, apiPort: 18578, directory: join(out, 'lab') });
  const state = await seedFilesAcceptance(lab); api = await filesClient(lab.origin);
  const tenant = state.tenants[0], base = `/tenants/${tenant}/files`, path = `/t/${tenant}/files`;
  async function upload(name, bytes = readFileSync(join(samples, name)), replace) {
    const caps = await api.call(base + '/capabilities'), headers = { ...api.headers, 'X-Files-Epoch': caps.restore_epoch }, ext = name.split('.').pop();
    const type = { pdf: 'application/pdf', docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', csv: 'text/csv', txt: 'text/plain' }[ext];
    const start = await api.client.post(api.api + base + '/uploads', { headers, data: { request_id: randomUUID(), reason: '文档预览自验', host: caps.host, intent: replace ? 'replace' : 'add', ...(replace ? { attachment_id: replace.id, expected_attachment_revision: replace.revision } : {}), filename: name, display_name: name, description: '合成预览自验资料', size_bytes: String(bytes.length), sha256: digest(bytes), declared_media_type: type } });
    if (!start.ok()) throw Error('Start upload: ' + await start.text());
    const upload = await start.json(), size = Number(caps.part_bytes);
    for (let offset = 0; offset < bytes.length; offset += size) {
      const data = bytes.subarray(offset, offset + size), response = await api.client.put(api.api + base + `/uploads/${upload.upload_id}/parts/${offset / size + 1}`, { headers: { ...headers, 'Content-Type': 'application/octet-stream', 'X-Content-SHA256': digest(data) }, data });
      if (!response.ok()) throw Error('Upload part: ' + await response.text());
    }
    const current = await api.call(base + '/uploads/' + upload.upload_id), response = await api.client.post(api.api + base + `/uploads/${upload.upload_id}/complete`, { headers, data: { request_id: randomUUID(), expected_revision: current.revision, reason: '开始格式检查' } });
    if (!response.ok()) throw Error('Complete: ' + await response.text());
    const result = await response.json(); let job;
    await expect.poll(async () => { job = await api.call(base + '/jobs/' + result.job_id); return job.state; }, { timeout: 45000 }).toBe('succeeded');
    return api.call(base + '/attachments/' + job.result_attachment_id);
  }
  const files = {};
  for (const name of ['preview.pdf', 'preview.docx', 'preview.xlsx', 'preview.csv', 'preview.txt', 'preview-gbk.txt', 'preview-utf16.txt', 'broken.pdf', 'preview-untrusted.docx']) files[name] = await upload(name);
  mark('All five formats plus encoding and invalid-PDF fixtures pass the real upload/check/bind path');
  browser = await chromium.launch();
  const context = await browser.newContext({ storageState: await api.client.storageState(), viewport: { width: 1440, height: 1000 }, acceptDownloads: true });
  page = await context.newPage(); page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (/^https?:/.test(request.url()) && new URL(request.url()).origin !== lab.origin) external.push(request.url().split('?')[0]); });
  const open = async (name, revision) => { await page.goto(lab.origin + path + '?attachment=' + files[name].id + (revision ? '&revision=' + revision : '')); await expect(page.locator('[data-document-preview]')).toBeVisible(); };
  const shot = name => page.screenshot({ path: join(out, name + '.png'), fullPage: true });
  const fullScreen = async () => { await page.getByRole('button', { name: '全屏查看', exact: true }).click(); await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(true); await page.getByRole('button', { name: '退出全屏', exact: true }).click(); await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(false); };
  const zoom = async () => { const label = page.getByRole('button', { name: '恢复原始比例' }), before = await label.innerText(); await page.getByRole('button', { name: '放大', exact: true }).click(); await expect(label).not.toHaveText(before); };

  await open('preview.pdf'); await expect(page.locator('[data-pdf-container] canvas').first()).toBeVisible();
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByLabel('跳转页码')).toHaveValue('2');
  await page.getByLabel('跳转页码').fill('3'); await page.getByLabel('跳转页码').press('Enter'); await expect(page.getByLabel('跳转页码')).toHaveValue('3');
  await zoom(); await page.getByRole('button', { name: '适合宽度', exact: true }).click();
  await page.getByLabel('搜索文档').fill('expansion'); await expect(page.locator('[data-document-preview]')).toContainText(/\d \/ 3 处/);
  await page.getByRole('button', { name: '下一个匹配', exact: true }).click();
  await expect(page.locator('.textLayer .highlight').first()).toBeVisible();
  await page.getByRole('button', { name: '顺时针旋转', exact: true }).click(); await expect.poll(() => page.locator('.page').first().evaluate(el => el.clientWidth > el.clientHeight)).toBe(true);
  await fullScreen(); await shot('pdf'); mark('PDF real pages, jump, zoom, width fit, rotate, text search/highlight and fullscreen');

  await open('preview.docx'); const word = page.frameLocator('iframe[title$="Word 阅读区"]');
  await expect(word.locator('section.docx')).toHaveCount(3); await expect(word.locator('table')).toContainText('P-01'); await expect(word.locator('img')).toBeVisible();
  await expect(page.locator('iframe[title$="Word 阅读区"]')).toHaveAttribute('sandbox', 'allow-scripts');
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByLabel('跳转页码')).toHaveValue('2');
  await page.getByLabel('搜索文档').fill('伸缩缝'); await expect(page.locator('[data-document-preview]')).toContainText('1 / 2 处'); await page.getByRole('button', { name: '下一个匹配', exact: true }).click(); await expect(page.locator('[data-document-preview]')).toContainText('2 / 2 处');
  await zoom(); await fullScreen(); await shot('word'); mark('Word pages, Chinese text, table, image, search, zoom and opaque sandbox fullscreen');
  await open('preview-untrusted.docx'); const hostile = page.frameLocator('iframe[title$="Word 阅读区"]');
  await expect(hostile.getByText('外部链接测试', { exact: true })).toBeVisible();
  await expect(hostile.locator('a[href]')).toHaveCount(0); await expect(hostile.getByText('UNSAFE_ALTCHUNK', { exact: true })).toHaveCount(0);
  await expect.poll(() => hostile.locator('body').evaluate(() => { try { return !!parent.document; } catch { return false; } })).toBe(false);
  await expect(page.locator('body')).not.toHaveAttribute('data-pwned'); mark('Word external hyperlink, HTML altChunk and parent document access remain disabled');

  await open('preview.xlsx'); await expect(page.getByRole('table', { name: '观测数据 数据' })).toBeVisible();
  await page.getByRole('button', { name: '单元格 B2', exact: true }).click(); await expect(page.getByLabel('单元格详情')).toContainText('42'); await expect(page.getByLabel('单元格详情')).toContainText('SUM(B3:B4)');
  await page.getByRole('button', { name: '单元格 B2', exact: true }).press('ArrowRight'); await expect(page.getByRole('button', { name: '单元格 C2', exact: true })).toBeFocused();
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByRole('button', { name: '单元格 A51', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '单元格 A51', exact: true })).toHaveAttribute('tabindex', '0'); await expect(page.getByRole('button', { name: '单元格 C51', exact: true })).toContainText('跨页合并内容');
  await page.getByLabel('搜索文档').fill('伸缩缝'); await expect(page.getByRole('button', { name: '单元格 A91', exact: true })).toBeVisible();
  await page.getByLabel('工作表').selectOption('1'); await expect(page.getByRole('table', { name: '汇总说明 数据' })).toBeVisible(); await expect(page.locator('td[colspan="3"]')).toBeVisible();
  await expect(page.getByRole('table')).toContainText('<script>alert("not executable")</script>'); await zoom(); await fullScreen(); await shot('excel'); mark('Excel multiple sheets, saved formula value, original row numbers, search, merged cells and inert text');

  await open('preview.csv'); await expect(page.getByRole('table')).toContainText('中文,逗号'); await expect(page.getByRole('table')).toContainText('00123'); await expect(page.getByRole('table')).toContainText('第一行\n第二行'); await expect(page.getByRole('table')).toContainText('双引号"测试'); await expect(page.getByRole('table')).toContainText('=1+1');
  await page.getByLabel('CSV 分隔符').selectOption(','); await expect(page.getByRole('table')).toContainText('00123'); await zoom(); await fullScreen(); await shot('csv'); mark('CSV preserves BOM, quoted commas/newlines, escaped quotes, leading zero and formula-like values');

  await open('preview.txt'); await expect(page.getByLabel('文本内容')).toContainText('第 1 行');
  await page.getByLabel('搜索文档').fill('末尾定位关键词'); await page.getByRole('button', { name: '下一个匹配', exact: true }).click(); await expect(page.getByLabel('跳转页码')).toHaveValue('3'); await expect(page.locator('[data-line="700"] mark')).toBeInViewport();
  await page.getByRole('button', { name: '自动换行', exact: true }).click(); await expect(page.getByRole('button', { name: '自动换行', exact: true })).toHaveAttribute('aria-pressed', 'false'); await zoom(); await fullScreen(); await shot('text');
  await open('preview-gbk.txt'); await expect(page.getByLabel('文本内容')).toContainText('中文编码验证');
  await open('preview-utf16.txt'); await expect(page.getByLabel('文本内容')).toContainText('UTF16 中文预览'); mark('TXT line numbers, cross-page match navigation, wrap toggle, zoom, fullscreen and GB18030/UTF16 decoding');

  const oldRevision = files['preview.txt'].current_revision.id;
  files['preview.txt'] = await upload('preview.txt', Buffer.from('新的修订正文'), files['preview.txt']);
  await open('preview.txt'); await expect(page.getByLabel('文本内容')).toContainText('新的修订正文');
  await expect(page.locator('[data-slot="drawer-popup"]').getByRole('tab')).toHaveCount(0);
  await page.getByRole('button', { name: '选择预览版本', exact: true }).click(); await page.getByRole('menuitemradio', { name: '版本 1', exact: true }).click();
  await expect(page.getByLabel('文本内容')).toContainText('第 1 行'); await expect(page.getByRole('button', { name: '还原此版本', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '返回最新版', exact: true }).click(); await expect(page.getByLabel('文本内容')).toContainText('新的修订正文');
  await open('preview.txt', oldRevision); await expect(page.getByLabel('文本内容')).toContainText('第 1 行');
  const download = page.waitForEvent('download'); await page.getByRole('link', { name: '下载原件', exact: true }).click(); const downloaded = await download; await downloaded.saveAs(join(out, 'original.txt')); if (digest(readFileSync(join(out, 'original.txt'))) !== digest(readFileSync(join(samples, 'preview.txt')))) throw Error('Historical original bytes changed');
  mark('Historical preview remains pinned after replacement and native download SHA-256 is unchanged');
  await open('broken.pdf'); await expect(page.getByRole('alert')).toContainText('PDF 无法预览'); await expect(page.getByRole('link', { name: '下载原件', exact: true })).toBeVisible(); mark('Corrupt PDF shows an explicit reading error with original download available');

  await page.setViewportSize({ width: 375, height: 812 });
  for (const name of ['preview.pdf', 'preview.docx', 'preview.xlsx', 'preview.csv', 'preview.txt']) {
    await open(name); await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expect(page.getByRole('button', { name: '全屏查看', exact: true })).toBeVisible();
    await page.getByRole('button', { name: '选择预览版本', exact: true }).click(); await expect(page.getByRole('menuitemradio').first()).toBeInViewport(); await page.keyboard.press('Escape'); await expect(page.locator('[data-document-preview]')).toBeVisible();
    await shot('mobile-' + name.split('.').pop());
  }
  await page.setViewportSize({ width: 720, height: 500 }); await open('preview.pdf'); await expect(page.getByRole('button', { name: '全屏查看', exact: true })).toBeVisible(); await shot('zoom-200');
  await page.setViewportSize({ width: 1440, height: 1000 }); await page.evaluate(() => document.documentElement.classList.add('dark')); await open('preview.xlsx'); await page.evaluate(() => document.documentElement.classList.add('dark')); await expect(page.getByRole('table')).toBeVisible(); await shot('dark-excel'); mark('375px, 200 percent equivalent layout and dark mode keep controls reachable');

  for (const role of ['reader', 'manager', 'downloader']) {
    const client = await filesClient(lab.origin, state.accounts[role].username), other = await browser.newContext({ storageState: await client.client.storageState() }), p = await other.newPage();
    let contentRequests = 0; p.on('request', r => { if (/\/revisions\/[^/]+\/content$/.test(r.url())) contentRequests++; });
    await p.goto(lab.origin + path + '?attachment=' + files['preview.txt'].id);
    if (role === 'downloader') { await expect(p.getByLabel('文本内容')).toContainText('新的修订正文'); if (!contentRequests) throw Error('Authorized reader did not read content'); }
    else { await expect(p.getByText('当前可查看文件信息', { exact: true })).toBeVisible(); await expect(p.locator('[data-document-preview]')).toHaveCount(0); if (contentRequests) throw Error('Reader without download permission requested content'); }
    await other.close(); await client.client.dispose();
  }
  mark('Actual reader/manager/download-only accounts enforce content authorization without unauthorized fetches');
  await open('preview.txt'); await expect(page.getByLabel('文本内容')).toContainText('新的修订正文');
  await page.evaluate(() => Object.defineProperty(document, 'fullscreenEnabled', { value: false, configurable: true }));
  await page.getByRole('button', { name: '全屏查看', exact: true }).click(); await expect(page.locator('dialog[open]')).toBeVisible();
  await expect.poll(() => page.locator('dialog[open]').evaluate(el => Math.abs(el.getBoundingClientRect().height - innerHeight) < 2)).toBe(true);
  await expect(page.getByLabel('文本内容')).toContainText('新的修订正文'); await page.keyboard.press('Escape'); await expect(page.locator('dialog[open]')).toHaveCount(0); await expect(page.locator('[data-document-preview]')).toBeVisible();
  mark('Native-fullscreen fallback fills the window, keeps the reader and closes with Escape without closing the attachment');
  const revokedClient = await filesClient(lab.origin, state.accounts.downloader.username), revokedContext = await browser.newContext({ storageState: await revokedClient.client.storageState() }), revokedPage = await revokedContext.newPage();
  await revokedPage.goto(lab.origin + path + '?attachment=' + files['preview.txt'].id); await expect(revokedPage.getByLabel('文本内容')).toContainText('新的修订正文');
  const member = state.accounts.downloader.memberID, authorization = await api.call(`/tenants/${tenant}/members/${member}/authorization`);
  const change = { expected_membership_version: authorization.membership_version, role_ids: [state.accounts.reader.role], reason: '预览读取中撤销正文下载资格' };
  const preview = await api.call(`/tenants/${tenant}/members/${member}/roles/preview`, change);
  await api.call(`/tenants/${tenant}/members/${member}/roles`, { ...change, role_versions: preview.role_versions, preview_hash: preview.preview_hash }, 'PUT');
  await revokedPage.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(revokedPage.getByText('当前可查看文件信息', { exact: true })).toBeVisible(); await expect(revokedPage.getByLabel('文本内容')).toHaveCount(0);
  const denied = await revokedClient.client.get(revokedClient.api + base + '/attachments/' + files['preview.txt'].id + '/revisions/' + files['preview.txt'].current_revision.id + '/content'); expect(denied.status()).toBe(403);
  await revokedContext.close(); await revokedClient.client.dispose();
  mark('Real role revocation clears an open preview and denies subsequent content reads');
  if (errors.length || external.length) throw Error(JSON.stringify({ errors, external }));
  writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: true, checks, errors, external, synthetic: true }, null, 2));
} catch (error) { await page?.screenshot({ path: join(out, 'failure.png'), fullPage: true }).catch(() => {}); writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: false, checks, errors, external, error: String(error) }, null, 2)); throw error; }
finally { await browser?.close(); await api?.client.dispose(); await lab?.close(); }
