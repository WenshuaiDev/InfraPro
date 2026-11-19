import assert from 'node:assert/strict';
import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import { dirname, extname, join, relative, resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { chromium } from '@playwright/test';
import { digest, read, safeDiagnosticLine, sleep, write } from './architecture-lab.mjs';
import { consoleDigest, sharedConsoleFor } from './architecture-shared-console.mjs';
import { consoleFiles, finishVisualFrontend, readServedConsole, visualFrontend } from './architecture-visual-frontend.mjs';
import { createVisualRecording } from './architecture-visual-recording.mjs';
import { pixelDiff, viewports, visualRoleGrants } from './architecture-visual.mjs';

const cases = [
  { template: '/security', endpoint: /\/account\/security-events$/ },
  { template: '/access/audit', endpoint: /\/audit\/events$/ },
  { template: '/platform/security-policy', endpoint: /\/platform\/security-policy$/ },
  { template: '/notifications', endpoint: /\/me\/notifications$/ },
  { template: '/t/$tenantId/projects', endpoint: /\/project-governance$/, governance: true },
];
const mime = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css',
  '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.woff': 'font/woff',
  '.png': 'image/png', '.webp': 'image/webp', '.ico': 'image/x-icon', '.jpg': 'image/jpeg',
};
const require = createRequire(import.meta.url);
const playwright = createRequire(require.resolve('@playwright/test'));
const { PNG } = playwright(join(dirname(playwright.resolve('playwright-core/package.json')), 'lib/utilsBundle.js'));
const selectors = 'main,main h1,main h2,main h3,nav,header,aside,footer,main form,label,input,select,textarea,button,a[href],table,th,tbody,[role="alert"],[role="dialog"],[role="tabpanel"],canvas';

// Only the verified immutable static files and the requested SPA documents can
// be served. This listener has no backend, cookies or private configuration.
async function signedStatic(directory, files, routes) {
  const server = createServer((request, response) => {
    const pathname = new URL(request.url, 'http://localhost').pathname;
    const name = decodeURIComponent(pathname.slice(1));
    const target = Object.hasOwn(files, name) ? name : routes.has(pathname) || pathname === '/' ? 'index.html' : null;
    if (!target || !['GET','HEAD'].includes(request.method)) {
      response.writeHead(404).end();
      return;
    }
    try {
      const buffer = readFileSync(join(directory, target));
      assert.equal(digest(buffer), files[target]);
      response.writeHead(200, { 'Content-Type': mime[extname(target)] || 'application/octet-stream' });
      response.end(request.method === 'HEAD' ? undefined : buffer);
    } catch {
      response.writeHead(500).end();
    }
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  return {
    origin: 'http://127.0.0.1:' + server.address().port,
    close: () => new Promise((resolve, reject) => {
      server.close(err => err ? reject(err) : resolve());
      server.closeAllConnections();
    }),
  };
}

function signature(request, response) {
  const url = new URL(request.url()), body = request.postDataBuffer(), headers = request.headers();
  // Cursors, CSRF tokens, cookies and session details never enter this ledger.
  return {
    method: request.method(), pathname: url.pathname, query: [...url.searchParams.keys()].sort(),
    input: digest(JSON.stringify([request.method(),url.pathname+url.search,body && digest(body),
      headers['content-type'] ?? null,headers.range ?? null,headers.accept ?? null])),
    status: response.status, bodySHA256: digest(response.body), bytes: response.body.byteLength,
  };
}

function cleaned(headers) {
  const values = { ...headers };
  for (const key of ['set-cookie','content-encoding','content-length','transfer-encoding']) delete values[key];
  return values;
}

// The original synthetic response bodies may contain session information.
// Retain them in an owner-only directory. Each renderer reopens the sealed
// bytes, and public reports contain only checksums and request identities.
export function retainedRecordings(directory, key, create = false) {
  if (create) mkdirSync(directory,{recursive:true,mode:0o700});
  assert.equal(statSync(directory).mode & 0o077,0);
  const records=[];
  function restore(record) {
    assert.match(record.file,/^\d{3}\.bin$/);
    const path=join(directory,record.file);
    assert.equal(statSync(path).mode & 0o077,0);
    const bytes=readFileSync(path);assert.equal(digest(bytes),record.ciphertextSHA256);
    const cipher=createDecipheriv('aes-256-gcm',key,bytes.subarray(0,12));
    cipher.setAAD(Buffer.from(record.input));cipher.setAuthTag(bytes.subarray(12,28));
    const payload=JSON.parse(Buffer.concat([cipher.update(bytes.subarray(28)),cipher.final()]));
    const body=Buffer.from(payload.body,'base64');
    assert.equal(digest(body),record.bodySHA256);assert.equal(body.length,record.bytes);
    assert.equal(payload.status,record.status);
    return {status:payload.status,headers:payload.headers,body};
  }
  return {records,restore,save(request,answer) {
    const record=signature(request,answer);
    const payload=Buffer.from(JSON.stringify({status:answer.status,headers:cleaned(answer.headers),body:answer.body.toString('base64')}));
    const iv=randomBytes(12),cipher=createCipheriv('aes-256-gcm',key,iv);
    cipher.setAAD(Buffer.from(record.input));
    const encrypted=Buffer.concat([cipher.update(payload),cipher.final()]);
    const stored=Buffer.concat([iv,cipher.getAuthTag(),encrypted]);
    record.file=String(records.length+1).padStart(3,'0')+'.bin';
    record.ciphertextSHA256=digest(stored);
    writeFileSync(join(directory,record.file),stored,{mode:0o600,flag:'wx'});
    records.push(record);
    return record;
  }};
}

async function login(browser, origin, account) {
  const context = await browser.newContext();
  try {
    const csrf = await context.request.get(origin + '/api/v1/auth/csrf');
    assert.equal(csrf.status(), 200);
    const { token } = await csrf.json();
    const result = await context.request.post(origin + '/api/v1/auth/login', {
      headers: { Origin: origin, 'X-CSRF-Token': token }, data: account,
    });
    assert.equal(result.status(), 200);
    assert.equal((await result.json()).status, 'authenticated');
    return await context.storageState();
  } finally {
    await context.close();
  }
}

async function settle(page, pending) {
  await page.locator('main').first().waitFor({ state: 'visible', timeout: 12000 });
  const limit = Date.now() + 20000;
  let text, at = Date.now();
  while (Date.now() < limit) {
    const next = await page.locator('body').innerText();
    if (next !== text) { text = next; at = Date.now(); }
    if (pending.size === 0 && Date.now() - at > 400) {
      await page.evaluate(async () => {
        await document.fonts.ready;
        await new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)));
      });
      if (pending.size === 0) return;
    }
    await sleep(60);
  }
  throw Error('The page did not settle after its genuine business reads');
}

async function capture({ browser, viewport, origin, state, clock, path, frozen, recording, reader, vault, where, name }) {
  const context = await browser.newContext({
    viewport, storageState: state, deviceScaleFactor: 1, hasTouch: viewport.width === 375,
    isMobile: viewport.width === 375, reducedMotion: 'reduce', timezoneId: 'Asia/Shanghai',
    locale: 'zh-CN', serviceWorkers: 'block',
  });
  const facts = [], errors = [], resources = new Set(), pending = new Set();
  try {
    const page = await context.newPage();
    page.on('pageerror', err => errors.push({ stage: 'browser', error: safeDiagnosticLine(err.message) }));
    await page.clock.setFixedTime(new Date(clock));
    await page.route('**/*', async route => {
      const req = route.request(), url = new URL(req.url());
      if (!['http:', 'https:'].includes(url.protocol)) return route.continue();
      try {
        assert.equal(url.origin, origin, 'Unexpected origin in the selected console');
        if (!url.pathname.startsWith('/api/')) {
          if (!frozen) return route.continue();
          assert(['GET','HEAD'].includes(req.method()));
          // Preserve the public origin for ordinary backend sessions. The
          // static listener receives neither the browser's cookies nor headers.
          const asset = await fetch(new URL(url.pathname + url.search, frozen), { method: req.method(), redirect: 'error' });
          const body = Buffer.from(await asset.arrayBuffer());
          assert.equal(asset.status, 200, 'Unexpected frozen asset ' + url.pathname);
          resources.add(url.pathname);
          return await route.fulfill({ status: asset.status, body, contentType: asset.headers.get('content-type') });
        }
        pending.add(req);
        if (reader) {
          const record = await reader.take(req);
          assert(record, 'Genuine response unavailable for ' + req.method() + ' ' + url.pathname);
          const answer = vault.restore(record);
          facts.push(signature(req, answer));
          return await route.fulfill({ ...answer, headers: cleaned(answer.headers) });
        }
        const save = recording?.reserve(req);
        const response = await route.fetch({ timeout: 22000, maxRedirects: 0 });
        try {
          const answer = { status: response.status(), headers: response.headers(), body: await response.body() };
          facts.push(signature(req, answer));
          if(save)save(vault.save(req,answer));
          await route.fulfill({ response, body: answer.body });
        } finally {
          await response.dispose();
        }
      } catch (error) {
        if (!page.isClosed()) errors.push({ stage: 'transport', pathname: url.pathname, error: safeDiagnosticLine(error.message) });
        await route.abort().catch(() => {});
      } finally {
        pending.delete(req);
      }
    });
    await page.goto(origin + path, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await settle(page, pending);
    const aria = await page.locator('body').ariaSnapshot({ timeout: 6000 });
    const dom = await page.evaluate(selector => {
      const rect = el => { const b=el.getBoundingClientRect(); return [b.x,b.y,b.width,b.height]; };
      const status = [...document.querySelectorAll('main p[role="status"]')].find(el => el.textContent.includes('按创建时间排序'));
      const footer = status?.parentElement;
      const after = footer?.nextElementSibling;
      const heading = document.querySelector('main h1')?.textContent;
      const main = document.querySelector('main');
      const walker = document.createTreeWalker(main,NodeFilter.SHOW_TEXT);
      const outside=[];
      while(walker.nextNode()) if(!footer?.contains(walker.currentNode))outside.push(walker.currentNode.textContent);

      const elements = [...document.querySelectorAll(selector)].filter(el=>el.getClientRects().length).map(el=>({
        tag:el.tagName, role:el.getAttribute('role'), box:rect(el),
        pager: footer?.contains(el) || after?.contains(el) || false,
      }));
      return {
        heading, path:location.pathname, scroll:[scrollX,scrollY], focus:document.activeElement?.tagName,
        elements, visibleText:main.innerText, outsideText:outside.join('').replaceAll(/\s+/g,' ').trim(),
        pager:footer?{rect:rect(footer),after:after?rect(after):null,summary:status.textContent,
          navigation:footer.querySelector('nav')?.getAttribute('aria-label'),
          children:[...footer.querySelectorAll('button,[role="combobox"],nav')].filter(el=>el.getClientRects().length).map(rect)}:null,
      };
    }, selectors);
    assert.equal(dom.path, path);
    const png = await page.screenshot({ path:join(where,name+'.png'), animations:'disabled', caret:'hide', scale:'css', timeout:12000 });
    let outsideAria=aria;
    if(dom.pager){
      // The screenshot and full accessible tree above include every control.
      // Compare the complement separately to attribute an approved pager-only difference.
      const footer=page.locator('main p[role="status"]').filter({hasText:'按创建时间排序'}).locator('..');
      assert.equal(await footer.count(),1);
      const prior=await footer.getAttribute('aria-hidden');
      await footer.evaluate(el=>el.setAttribute('aria-hidden','true'));
      try{outsideAria=await page.locator('body').ariaSnapshot({timeout:6000});}
      finally{await footer.evaluate((el,old)=>old===null?el.removeAttribute('aria-hidden'):el.setAttribute('aria-hidden',old),prior);}
    }
    const metadata={ facts,errors,aria,outsideAria,dom,staticRoutes:[...resources].sort(),screenshotSHA256:digest(png) };
    write(join(where,name+'.json'),metadata);
    return { ...metadata,png };
  } finally {
    await context.close();
  }
}

function changedArea(left,right,rectangles) {
  const old=PNG.sync.read(left), current=PNG.sync.read(right);
  assert.equal(old.width,current.width);assert.equal(old.height,current.height);
  let changed=0,outside=0;
  const bounds={left:old.width,top:old.height,right:-1,bottom:-1};
  for(let y=0;y<old.height;y++)for(let x=0;x<old.width;x++) {
    const i=4*(y*old.width+x);
    if(old.data.readUInt32BE(i)===current.data.readUInt32BE(i))continue;
    changed++;
    bounds.left=Math.min(x,bounds.left);bounds.top=Math.min(y,bounds.top);
    bounds.right=Math.max(x,bounds.right);bounds.bottom=Math.max(y,bounds.bottom);
    if(!rectangles.some(([a,b,w,h])=>x>=Math.floor(a)&&y>=Math.floor(b)&&x<Math.ceil(a+w)&&y<Math.ceil(b+h)))outside++;
  }
  return {changed,outside,bounds:changed?bounds:null};
}

function renderParity(before,after,governance,path) {
  const rects=[before,after].flatMap(view=>[view.dom.pager?.rect,view.dom.pager?.after,...(view.dom.pager?.children||[])]).filter(Boolean);
  const area=changedArea(before.png,after.png,rects);
  assert.equal(pixelDiff(before.png,after.png,path),area.changed);
  const fields=view=>({heading:view.dom.heading,path:view.dom.path,scroll:view.dom.scroll,focus:view.dom.focus,
    elements:view.dom.elements.filter(el=>!governance||!el.pager)});
  const layoutEqual=isDeepStrictEqual(fields(before),fields(after));
  const ariaEqual=before.aria===after.aria;
  const contentEqual=before.dom.visibleText===after.dom.visibleText;
  const outsideAccessibilityEqual=before.outsideAria===after.outsideAria, outsideContentEqual=before.dom.outsideText===after.dom.outsideText;
  const pagination=governance&&before.dom.pager&&after.dom.pager&&
    before.dom.pager.summary===after.dom.pager.summary&&before.dom.pager.navigation==='项目治理分页'&&
    after.dom.pager.navigation===before.dom.pager.navigation;
  return {area,ariaEqual,contentEqual,outsideAccessibilityEqual,outsideContentEqual,layoutEqual,pagination:Boolean(pagination),rectangles:rects,
    passed:layoutEqual&&(governance?pagination&&outsideAccessibilityEqual&&outsideContentEqual&&area.outside===0:ariaEqual&&contentEqual&&area.changed===0)};
}

const sortedInputs = facts => facts.map(f=>[f.input,f.status,f.bodySHA256]).sort((a,b)=>JSON.stringify(a).localeCompare(JSON.stringify(b)));
function authorized(view,endpoint) {
  const ok=pattern=>view.facts.some(f=>f.method==='GET'&&f.status===200&&pattern.test(f.pathname));
  return view.errors.length===0&&ok(/\/api\/v1\/auth\/me$/)&&ok(endpoint)&&view.facts.every(f=>f.status>=200&&f.status<400);
}

export async function crossConsole(dir,output=dir) {
  const current=sharedConsoleFor(output,{verifyFiles:true});assert(current);
  const baseline=read(join(dir,'baseline.json')),lab=read(join(dir,'private/lab.json')),samples=read(join(dir,'samples.json'));
  const account=read(join(dir,'private/accounts.json')).admin;
  const {output:profile,frontend}=await visualFrontend(dir,'b',output);
  const clock=samples.clock||'2026-11-18T06:00:00Z',tenant=samples.structure.tenant;
  const frozenFiles=consoleFiles(baseline.staticFiles), expectedFrozen=consoleDigest(frozenFiles);
  assert.notEqual(expectedFrozen,current.console.sha256);
  const routes=cases.map(row=>({...row,path:row.template.replace('$tenantId',tenant)}));
  const root=join(profile,'visual/cross-release/same-responses');
  const dirOut=join(root,new Date().toISOString().replaceAll(':','-'));
  mkdirSync(dirOut,{recursive:true,mode:0o700});
  const vaultRoot=join(resolve(output),'private','shared-console-responses');
  const restricted=join(vaultRoot,dirOut.slice(root.length+1));
  mkdirSync(restricted,{recursive:true,mode:0o700});
  const key=randomBytes(32);
  writeFileSync(join(restricted,'replay.key'),key,{mode:0o600,flag:'wx'});
  const grants=visualRoleGrants(dir,'b');
  const report={passed:false,expectedRows:15,startedAt:new Date().toISOString(),directory:dirOut,sourceDirectory:dir,
    snapshot:read(join(dir,'snapshot/manifest.json')).pg.seed,candidate:current.backendReleaseSHA256,clock,frontend,roleSQL:grants,
    oldConsoleSHA256:expectedFrozen,newConsoleSHA256:current.console.sha256,
    privateRecordingDirectory:restricted,
    driverSHA256:digest(readFileSync(new URL(import.meta.url))),approvedSources:current.approvedSources,rows:[]};
  const manifest=join(dirOut,'result.json');
  write(manifest,report);
  const publicRoutes=new Set(routes.map(row=>row.path));
  const frozen=await signedStatic(join(baseline.assets,'console'),frozenFiles,publicRoutes);
  const previous=current.beforeImportFocus?.console;
  let browser,intermediate;
  try {
    const original=await readServedConsole(frozen.origin,frozenFiles);
    assert.equal(original.sha256,expectedFrozen);assert.equal(original.failures.length,0);
    report.oldStatic={before:original};
    if(previous){
      intermediate=await signedStatic(previous.directory,previous.files,publicRoutes);
      report.previousStatic={issuerReleaseSHA256:current.beforeImportFocus.issuerReleaseSHA256,
        before:await readServedConsole(intermediate.origin,previous.files)};
      assert.equal(report.previousStatic.before.sha256,previous.sha256);
      assert.equal(report.previousStatic.before.failures.length,0);
    }
    browser=await chromium.launch({headless:true,args:['--disable-gpu','--disable-gpu-compositing','--disable-lcd-text',
      '--disable-skia-runtime-opts','--num-raster-threads=1','--disable-partial-raster','--run-all-compositor-stages-before-draw',
      '--force-device-scale-factor=1','--force-color-profile=srgb']});
    report.chromium=browser.version();
    const sessions={a:await login(browser,lab.a.origin,account),b:await login(browser,lab.b.origin,account)};
    for(const viewport of viewports)for(const item of routes) {
      const where=join(dirOut,viewport.width+'-'+item.template.replaceAll(/[^a-zA-Z0-9]+/g,'-'));
      mkdirSync(where,{recursive:true,mode:0o700});
      const vaultName=viewport.width+'-'+cases.findIndex(c=>c.template===item.template);
      const vault=retainedRecordings(join(restricted,vaultName),key,true);
      const row={template:item.template,path:item.path,viewport,where,vaultName,passed:false};
      report.rows.push(row);
      try {
        const recording=createVisualRecording();
        const snap=(side,name,publisher,options={})=>capture({browser,viewport,origin:lab[side].origin,state:sessions[side],
          clock,path:item.path,frozen:publisher?.origin,where,name,vault,...options});
        const oldLive=await snap('a','c0-s0-live',frozen,{recording});
        const newLive=await snap('b','shared-candidate-live');
        const first=recording.reader(),second=recording.reader();
        const oldFixed=await snap('a','c0-same-responses',frozen,{reader:first});
        const newFixed=await snap('b','shared-same-responses',null,{reader:second});
        const snapshots=[oldLive,newLive,oldFixed,newFixed];
        row.live={baseline:authorized(oldLive,item.endpoint),candidate:authorized(newLive,item.endpoint),
          changedPixels:pixelDiff(oldLive.png,newLive.png,join(where,'live-diff.png'))};
        row.recording=recording.summary();
        row.retained=vault.records;
        row.reads={original:sortedInputs(oldLive.facts),before:sortedInputs(oldFixed.facts),after:sortedInputs(newFixed.facts)};
        row.sameInputs=isDeepStrictEqual(row.reads.original,row.reads.before)&&isDeepStrictEqual(row.reads.before,row.reads.after);
        row.unused={before:first.unusedBusinessInputs(),after:second.unusedBusinessInputs()};
        row.fixed=renderParity(oldFixed,newFixed,item.governance,join(where,'fixed-diff.png'));
        if(intermediate){
          const third=recording.reader();
          const previousLive=await snap('a','c1-s0-live',intermediate);
          const previousFixed=await snap('a','c1-same-responses',intermediate,{reader:third});
          snapshots.push(previousLive,previousFixed);
          row.reads.intermediate=sortedInputs(previousFixed.facts);
          row.unused.intermediate=third.unusedBusinessInputs();
          row.sameInputs&&=isDeepStrictEqual(row.reads.original,row.reads.intermediate);
          // The approved focus change does not alter any of these five views.
          row.fromIntermediate=renderParity(previousFixed,newFixed,false,join(where,'previous-fixed-diff.png'));
        }
        row.rendered=snapshots.map(x=>({heading:x.dom.heading,screenshotSHA256:x.screenshotSHA256,errors:x.errors}));
        row.passed=row.live.baseline&&row.live.candidate&&snapshots.every(v=>authorized(v,item.endpoint))&&
          row.sameInputs&&Object.values(row.unused).every(v=>!v.length)&&row.fixed.passed&&
          (!intermediate||row.fromIntermediate.passed);
      } catch(error) {row.error=safeDiagnosticLine(error.message).slice(0,360);}
      write(manifest,report);
    }
    report.oldStatic.after=await readServedConsole(frozen.origin,frozenFiles);
    assert.equal(report.oldStatic.after.sha256,expectedFrozen);assert.equal(report.oldStatic.after.failures.length,0);
    if(intermediate){
      report.previousStatic.after=await readServedConsole(intermediate.origin,previous.files);
      assert.equal(report.previousStatic.after.sha256,previous.sha256);assert.equal(report.previousStatic.after.failures.length,0);
    }
    report.frontendAtEnd=await finishVisualFrontend(dir,'b',output,frontend);
    assert.deepEqual(visualRoleGrants(dir,'b'),grants);
  } finally {
    await browser?.close();await intermediate?.close();await frozen.close();report.finishedAt=new Date().toISOString();
    report.passed=Boolean(report.frontendAtEnd)&&report.rows.length===15&&report.rows.every(r=>r.passed);
    write(manifest,report);write(join(root,'latest.json'),{passed:report.passed,result:resolve(manifest),sha256:digest(readFileSync(manifest))});
  }
  return report;
}

// Recheck the selected release, each browser view and the exact successful
// fixed inputs when the ledger is regenerated without a running browser.
export function verifiedCrossConsole(pointer, directory, frozen, current, run) {
  const root = resolve(directory);
  if (!pointer?.passed || !pointer.result || !existsSync(pointer.result)) return false;
  const path = relative(root, resolve(pointer.result));
  if (!path || path.startsWith('..') || digest(readFileSync(pointer.result)) !== pointer.sha256) return false;
  const result = read(pointer.result);
  if (!result.passed || result.driverSHA256 !== digest(readFileSync(new URL(import.meta.url))) ||
      result.oldConsoleSHA256 !== consoleDigest(consoleFiles(frozen.staticFiles)) ||
      result.newConsoleSHA256 !== current.console.sha256 || result.rows?.length !== 15) return false;
  for (const observation of [result.oldStatic?.before, result.oldStatic?.after]) {
    if (!observation || observation.failures?.length !== 0 ||
        observation.sha256 !== result.oldConsoleSHA256 ||
        !isDeepStrictEqual(observation.files, consoleFiles(frozen.staticFiles))) return false;
  }
  const privateParent=resolve(run,'private','shared-console-responses');
  if (!result.privateRecordingDirectory || dirname(resolve(result.privateRecordingDirectory)) !== privateParent) return false;
  const keyPath=join(result.privateRecordingDirectory,'replay.key');
  if (!existsSync(keyPath) || (statSync(keyPath).mode & 0o077) !== 0) return false;
  const key=readFileSync(keyPath);
  const waits = new Set(viewports.flatMap(v => cases.map(r => [v.width,r.template].join('/'))));
  const names = ['c0-s0-live','shared-candidate-live','c0-same-responses','shared-same-responses'];
  const previous=current.beforeImportFocus?.console;
  if(previous){
    if(result.previousStatic?.issuerReleaseSHA256!==current.beforeImportFocus.issuerReleaseSHA256)return false;
    for(const sample of [result.previousStatic.before,result.previousStatic.after]){
      if(sample?.sha256!==previous.sha256 || sample.failures?.length!==0 || !isDeepStrictEqual(sample.files,previous.files))return false;
    }
    names.push('c1-s0-live','c1-same-responses');
  }
  return result.rows.every(row => {
    const route = cases.find(item => item.template === row.template);
    if (!route || !waits.delete([row.viewport.width,row.template].join('/')) ||
        !row.passed || !row.live?.baseline || !row.live?.candidate ||
        !row.sameInputs || row.unused?.before?.length !== 0 || row.unused?.after?.length !== 0 ||
        !row.fixed?.passed || !row.fixed.layoutEqual ||
        (route.governance ? !row.fixed.pagination || !row.fixed.outsideAccessibilityEqual || !row.fixed.outsideContentEqual || row.fixed.area.outside !== 0 :
          !row.fixed.ariaEqual || !row.fixed.contentEqual || row.fixed.area.changed !== 0) ||
        relative(root,resolve(row.where)).startsWith('..') || row.rendered?.length !== names.length) return false;
    if (!row.vaultName || !/^\d+-[0-4]$/.test(row.vaultName) ||
        !row.retained?.length || row.retained.length!==row.recording.requests) return false;
    try {
      const vault=retainedRecordings(join(result.privateRecordingDirectory,row.vaultName),key);
      for (const record of row.retained) vault.restore(record);
    } catch { return false; }
    const inputs = [];
    for (const [index,name] of names.entries()) {
      const data = join(row.where,name+'.json'), image = join(row.where,name+'.png');
      if (!existsSync(data) || !existsSync(image)) return false;
      const receipt = read(data);
      if (!authorized(receipt,route.endpoint) || receipt.dom.path !== row.path ||
          receipt.screenshotSHA256 !== row.rendered[index].screenshotSHA256 ||
          digest(readFileSync(image)) !== receipt.screenshotSHA256) return false;
      inputs.push(sortedInputs(receipt.facts));
    }
    if(previous && (!row.fromIntermediate?.passed || row.fromIntermediate.area.changed!==0 ||
        !row.fromIntermediate.ariaEqual || !row.fromIntermediate.contentEqual || !row.fromIntermediate.layoutEqual ||
        row.unused.intermediate?.length!==0 || !isDeepStrictEqual(row.reads.intermediate,inputs[5]) ||
        !isDeepStrictEqual(inputs[0],inputs[5])))return false;
    return isDeepStrictEqual(sortedInputs(row.retained),inputs[0]) && isDeepStrictEqual(inputs[0],inputs[2]) && isDeepStrictEqual(inputs[2],inputs[3]) &&
      isDeepStrictEqual(row.reads.original,inputs[0]) && isDeepStrictEqual(row.reads.before,inputs[2]) &&
      isDeepStrictEqual(row.reads.after,inputs[3]);
  }) && !waits.size;
}
