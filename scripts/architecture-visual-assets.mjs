import assert from 'node:assert/strict';
import { finishVisualFrontend, visualFrontend } from './architecture-visual-frontend.mjs';
import { randomBytes, randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, renameSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, expect, request } from '@playwright/test';
import { client, digest, fileLease, read, write } from './architecture-lab.mjs';
import { pixelDiff, viewports, visualRoleGrants } from './architecture-visual.mjs';
import { seedWalkthroughFilesDisplay, walkthroughDisplayGrant } from './architecture-walkthrough-assets.mjs';

export async function visualAssets(directory,side){
  const file=join(directory,'private',side,'visual-assets.json');
  const known=existsSync(file)?read(file):null;
  if(known?.display.grant)return known;
  const unlock=await fileLease(file+'.lock');
  try{
    const lab=read(join(directory,'private/lab.json'))[side];
    const account=read(join(directory,'private/accounts.json')).admin;
    const {structure}=read(join(directory,'samples.json'));
    if(existsSync(file)){
      const saved=read(file);
      if(!saved.display.grant){
        const person=await client(lab,account);
        try{
          const path='/tenants/'+saved.tenant+'/projects/'+saved.project+'/displays/instances/'+saved.display.instance;
          saved.display.grant=await walkthroughDisplayGrant(person.call,path,structure.nodes.A.id);
          write(file,saved);
        }finally{await person.client.dispose();}
      }
      return saved;
    }
    const result=await seedWalkthroughFilesDisplay({directory,side,lab,structure,account});
    write(file,result);return result;
  }finally{await unlock();}
}

async function pairDisplayTerminal(dir, side, assets) {
  const app = read(join(dir,'private/lab.json'))[side];
  const admin = await client(app,read(join(dir,'private/accounts.json')).admin);
  const session = await request.newContext();
  try {
    const path='/tenants/'+assets.tenant+'/projects/'+assets.project+'/displays/instances/'+assets.display.instance;
    const detail=await admin.call(path);
    const pair=await admin.call(path+'/terminals/'+assets.display.terminal,{
      request_id:randomUUID(), reason:'在隔离浏览器中配对用于三屏对照的合成终端',
      expected_revision:detail.instance.revision, state:'reset',
    },'PATCH');
    const res=await session.post(app.runtime+'/runtime/terminal/pair',{
      headers:{Origin:app.runtime,'X-Display-Protocol':'1'},data:{code:pair.pair_code},
    });
    assert.equal(res.status(),200,'Visual terminal pairing must reach the deployed runtime');
    await res.dispose();
    write(assets.display.terminalSessionFile,await session.storageState());
  } finally {await session.dispose();await admin.client.dispose();}
}

async function publishedScreen(browser,dir,side,subject,viewport,assets,pngPath,recorded){
  const app=read(join(dir,'private/lab.json'))[side],storage=subject==='terminal'?assets.display.terminalSessionFile:undefined;
  const context=await browser.newContext({storageState:storage,viewport,hasTouch:viewport.width===375,isMobile:viewport.width===375,deviceScaleFactor:1,
    reducedMotion:'reduce',locale:'zh-CN',timezoneId:'Asia/Shanghai',serviceWorkers:'block'});
  let owner;
  try{
    const page=await context.newPage(),observed=[],assetRequests=[];
    await page.clock.setFixedTime(new Date('2026-11-18T08:00:00Z'));
    page.on('response',response=>{
      const url=new URL(response.url());
      if(url.origin===app.runtime&&url.pathname.startsWith('/runtime/')){
        observed.push({method:response.request().method(),action:url.pathname.split('/').at(-1),status:response.status()});
      }
    });
    page.on('request',request=>{if(new URL(request.url()).origin===app.assets)assetRequests.push(request.allHeaders());});
    if(subject==='terminal')await page.goto(app.runtime+'/terminal');
    else{
      owner=await client(app,read(join(dir,'private/accounts.json')).admin);
      const path='/tenants/'+assets.tenant+'/projects/'+assets.project+'/displays/instances/'+assets.display.instance;
      const item=await owner.call(path),verifier=randomBytes(32).toString('hex');
      assert.equal(item.instance.current_release_id,assets.display.release);
      const ticket=await owner.call(path+'/launch',{expected_revision:item.instance.revision,challenge:digest(verifier),audience:app.runtime});
      await page.addInitScript(({origin,verifier})=>{if(location.origin===origin&&location.pathname==='/exchange')sessionStorage.setItem('display_verifier',verifier);},{origin:app.runtime,verifier});
      await page.goto(app.runtime+'/exchange#code='+encodeURIComponent(ticket.code));
    }
    await expect(page.locator('#status')).toHaveText('已连接 · 应用已就绪',{timeout:30000});
    // The real terminal start rotates its credential; keep the new, private
    // cookie exactly as the continuously running paired browser would.
    if(subject==='terminal')write(storage,await context.storageState());
    const shell=page.locator('iframe:not(.preparing)'),frame=page.frameLocator('iframe:not(.preparing)');
    await expect(shell).toHaveAttribute('sandbox','allow-scripts');
    await expect(frame.getByRole('heading',{name:'桥梁工程概览',exact:true})).toBeVisible();
    await expect(frame.locator('#mode')).toHaveText('正式数据');
    await expect(frame.locator('#output')).toContainText('设施',{timeout:15000});
    const headers=await Promise.all(assetRequests);
    assert(headers.length&&headers.every(h=>!h.authorization&&!h.cookie));
    assert(observed.some(r=>r.action==='query'&&r.status===200));
    assert(observed.some(r=>r.action===(subject==='terminal'?'start':'exchange')&&r.status===200));
    const path=new URL(page.url());assert.equal(path.hash,'');
    await page.evaluate(()=>document.fonts.ready);
    await frame.locator('body').evaluate(async el=>{await el.ownerDocument.fonts.ready;});
    const actual=JSON.parse(await frame.locator('#output').innerText());
    assert.equal(actual.state,'ready');assert.ok(Number.isFinite(Date.parse(actual.read_at)));
    assert.match(actual.authorization_revision,/^[a-f0-9]{64}$/);assert.equal(actual.source_vector.config,assets.display.config);
    assert.ok(JSON.stringify(actual.data).includes(read(join(dir,'samples.json')).structure.nodes.A.id));
    const capture=path=>page.screenshot({path,animations:'disabled',caret:'hide',scale:'css'});
    const live=await capture(pngPath.replace(/\.png$/,'-live.png'));
    const fixed=recorded||actual;
    // The installed sample prints the entire live SDK envelope, including its
    // server clock, configuration ID and authority hash. Keep that live view.
    // Then issue its original refresh against the identical frozen response.
    // The actual resource records and their IDs are compared independently.
    await page.route('**/runtime/*/query',async route=>{
      const request=route.request();
      assert.equal(request.method(),'POST');assert.equal(request.postDataJSON().method,'resources.list');
      await route.fulfill({status:200,json:fixed});
    });
    await Promise.all([page.waitForResponse(response=>new URL(response.url()).pathname.endsWith('/query')&&response.ok()),
      frame.getByRole('button',{name:'刷新资源',exact:true}).click()]);
    await expect(frame.locator('#output')).toHaveText(JSON.stringify(fixed,null,2),{useInnerText:true});
    await expect(frame.getByRole('button',{name:'刷新资源',exact:true})).toBeEnabled();
    return {png:await capture(pngPath),live,actual,layout:await shell.boundingBox(),shell:await page.locator('body').ariaSnapshot(),content:await frame.locator('body').ariaSnapshot(),
      resource:digest(JSON.stringify(actual.data)),url:path.pathname,statuses:observed,credentialFreeAssets:headers.length};
  }finally{await context.close();await owner?.client.dispose();}
}

export async function visualScreens(dir,side,output=dir){
  const {output:profile,frontend}=await visualFrontend(dir,side,output);
  output=profile;
  const roleSQL=visualRoleGrants(dir,side);
  const assets={a:await visualAssets(dir,'a'),[side]:await visualAssets(dir,side)};
  const folder=join(output,'visual','published-screens',side);mkdirSync(folder,{recursive:true,mode:0o700});
  const browser=await chromium.launch({headless:true,args:['--disable-lcd-text','--force-color-profile=srgb','--force-device-scale-factor=1']});
  const path=join(folder,'matrix.json'),candidate=read(join(dir,'candidate.json')).inputs.sourceSHA256;
  const identity=digest(JSON.stringify([digest(readFileSync(fileURLToPath(import.meta.url))),dir,candidate,roleSQL,frontend.identity,read(join(dir,'snapshot/manifest.json')).pg.seed,browser.version(),
    Object.values(assets).map(r=>[r.display.release,r.display.config,r.display.terminal,r.display.grant.revision])]));
  const old=existsSync(path)?read(path):null;
  if(old&&old.identity!==identity){
    const previous=join(output,'visual','attempts','published-screens',side,(old.identity||'unversioned')+'-'+Date.now());
    mkdirSync(dirname(previous),{recursive:true,mode:0o700});renameSync(folder,previous);mkdirSync(folder,{recursive:true,mode:0o700});
  }
  const cached=new Map((old?.identity===identity?old.rows:[]).map(r=>[r.key,r]));
  const report={identity,frontend,sourceDirectory:dir,baseline:read(join(dir,'baseline.json')).commit,candidate,roleSQL,snapshot:read(join(dir,'snapshot/manifest.json')).pg.seed,side,browser:browser.version(),expectedRows:6,rows:[],passed:false};
  try{
    if(viewports.some(v=>!cached.get('terminal-'+v.width)?.passed))
      for(const [actor,asset] of Object.entries(assets))await pairDisplayTerminal(dir,actor,asset);
    scenes:for(const viewport of viewports)for(const subject of ['person','terminal']){
      const key=subject+'-'+viewport.width;
      if(cached.get(key)?.passed){report.rows.push(cached.get(key));continue;}
      if(process.env.ARC_VISUAL_CHECKPOINT_FILE&&existsSync(process.env.ARC_VISUAL_CHECKPOINT_FILE))break scenes;
      const row={subject,viewport,key,passed:false};report.rows.push(row);
      try{
        const left=await publishedScreen(browser,dir,'a',subject,viewport,assets.a,join(folder,key+'-a.png'));
        const right=await publishedScreen(browser,dir,side,subject,viewport,assets[side],join(folder,key+'-'+side+'.png'),left.actual);
        row.changedPixels=pixelDiff(left.png,right.png,join(folder,key+'-diff.png'));
        row.liveChangedPixels=pixelDiff(left.live,right.live,join(folder,key+'-live-diff.png'));
        row.sameAccessibility=left.shell===right.shell&&left.content===right.content;
        row.sameLayout=JSON.stringify(left.layout)===JSON.stringify(right.layout);
        row.sameData=left.resource===right.resource;row.sameURL=left.url===right.url;
        row.liveEvidence=[left,right].map(r=>({resourceSHA256:r.resource,state:r.actual.state,sourceFields:Object.keys(r.actual.source_vector).sort(),statuses:r.statuses,credentialFreeAssets:r.credentialFreeAssets}));
        row.fixedInputSHA256=digest(JSON.stringify(left.actual));row.liveImages=['a',side].map(s=>basename(join(folder,key+'-'+s+'-live.png')));
        row.passed=row.changedPixels===0&&row.sameAccessibility&&row.sameLayout&&row.sameData&&row.sameURL;
      }catch(e){row.error=e.message.replace(/https?:\/\/[^\s]+/g,'[isolated URL]').slice(0,400);}
      write(path,report);
    }
    report.passed=report.rows.length===6&&report.rows.every(r=>r.passed);
    if(report.rows.length===6)report.frontendAtEnd=await finishVisualFrontend(dir,side,output,frontend);
    write(path,report);return report;
  }finally{await browser.close();}
}
