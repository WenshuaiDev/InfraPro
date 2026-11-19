import assert from 'node:assert/strict';
import { readFileSync, existsSync, mkdirSync, renameSync, writeFileSync } from 'node:fs';
import { join, dirname, resolve } from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { chromium } from '@playwright/test';
import { digest, read, write, runDirectory, sleep, queryPG, safeDiagnosticLine } from './architecture-lab.mjs';
import { currentRoleSQL } from './architecture-parity-evidence.mjs';
import { finishVisualFrontend, visualFrontend } from './architecture-visual-frontend.mjs';
import { createVisualRecording, scheduleVisualRequests, visualRecordingDrivers } from './architecture-visual-recording.mjs';

const loadedDrivers=visualRecordingDrivers();

export function visualRoleGrants(dir,side) {
  if(side==='self')return null;
  const grants=read(join(dir,'role-grants.json'));
  assert.deepEqual(grants,currentRoleSQL(),'Visual replay requires the installed role grants for this checkout');
  return grants;
}

const require=createRequire(import.meta.url);
const playwrightRequire=createRequire(require.resolve('@playwright/test'));
const {PNG}=playwrightRequire(join(dirname(playwrightRequire.resolve('playwright-core/package.json')),'lib/utilsBundle.js'));
export const viewports=[{width:1440,height:1000},{width:1024,height:768},{width:375,height:812}];
function pathList(base,lab,samples){
  const source=readFileSync(join(base.source,'apps/console/src/routeTree.gen.ts'),'utf8');
  const section=source.split('export interface FileRoutesByFullPath {')[1]?.split('\n}')[0];
  assert(section,'No TanStack FileRoutesByFullPath in frozen console');
  const templates=[...new Set([...section.matchAll(/^ {2}'([^']+)':/gm)].map(m=>m[1].replace(/\/$/,'')||'/'))];
  const structure=samples.structure;
  const sentinel='00000000-0000-4000-8000-000000000001';
  const one=(query)=>queryPG(lab,'seed',query)||sentinel;
  const ids={tenantId:structure.tenant,projectId:structure.project,facilityId:structure.nodes.A.id,channelId:structure.channels.P1.id,batchId:structure.batch,
    deviceId:structure.devices[0].id,productId:one("SELECT product_id::text FROM devices ORDER BY id LIMIT 1"), membershipId:one("SELECT id::text FROM tenant_memberships WHERE tenant_id='"+structure.tenant+"' ORDER BY id LIMIT 1"), outputId:one("SELECT id::text FROM device_outputs ORDER BY id LIMIT 1"),
    artifactId:sentinel,taskId:one("SELECT id::text FROM task_records ORDER BY id LIMIT 1"),notificationId:sentinel,invitationId:sentinel,roleId:one("SELECT id::text FROM roles ORDER BY id LIMIT 1"),kind:'units'};
  const fromTemplate=template=>template.replace(/\$(\w+)/g,(_,k)=>{assert(Object.hasOwn(ids,k),'Unbound route parameter '+k);return ids[k];});
  const tenantRole=one("SELECT id FROM roles WHERE binding_scope='tenant' AND (owner_tenant_id IS NULL OR owner_tenant_id='"+structure.tenant+"') ORDER BY owner_tenant_id NULLS LAST,id LIMIT 1");
  const projectRole=one("SELECT id FROM roles WHERE binding_scope='project' AND (owner_project_id IS NULL OR owner_project_id='"+structure.project+"') ORDER BY owner_project_id NULLS LAST,id LIMIT 1");
  const projectMember=one("SELECT id FROM project_memberships WHERE project_id='"+structure.project+"' ORDER BY joined_at,id LIMIT 1");
  const consoleRoutes=templates.map(template=>{
    const guest=/^\/(?:login|forgot-password|invite|mfa-recovery|reset-password|tenant-invite)$/.test(template);
    let path=fromTemplate(template);
    if(template.includes('/p/$projectId/roles/'))path=path.replace(ids.roleId,projectRole);
    else if(template.includes('/t/$tenantId/roles/'))path=path.replace(ids.roleId,tenantRole);
    if(template.includes('/p/$projectId/members/'))path=path.replace(ids.membershipId,projectMember);
    if(template.includes('/device-products/$productId'))path+='?source=public';
    const restricted=template.includes('$artifactId') || template.includes('/monitoring/derived/$channelId') || template.includes('$invitationId') || template.includes('$notificationId');
    return {app:'console',template,path,persona:guest?'guest':restricted?'member':'admin',state:restricted?'outside-scope':'default',
      expectation:guest?'public':restricted?'scope-guard':'authenticated'};
  });
  // The route is shared by units, metrics, facility definitions and template catalogs.
  for(const kind of ['metrics','type-definitions'])for(const prefix of ['/platform/engineering/','/t/$tenantId/engineering/catalog/']){
    const template=prefix+'$kind';if(templates.includes(template))consoleRoutes.push({app:'console',template,path:fromTemplate(prefix)+kind,persona:'admin',state:kind});
  }
  for(const [template,state] of [['/access/accounts','search-empty'],['/access/accounts','search-data'],['/platform/tenants','search-empty'],['/profile','profile-edit'],['/platform/security-policy','member-denied'],['/tasks','pending'],['/tasks','error']]){
    if(templates.includes(template))consoleRoutes.push({app:'console',template,path:fromTemplate(template),persona:state==='member-denied'?'member':'admin',state});
  }
  const diag=readFileSync(join(base.source,'apps/diagnostics/src/router.ts'),'utf8');
  const diagRoutes=[...diag.matchAll(/createRoute\(\{[^\n]*?path: '([^']+)'/g)].map(([,template])=>({app:'diagnostics',template,path:template.replace('$componentId','api').replace('$replicaId','api-1').replace('$nodeId','local'),persona:template==='/login'?'guest':'operator',state:'default'}));
  assert(diagRoutes.length>=9);
  const displayRoutes=['/','/terminal','/display/$id','/exchange'].map(template=>({app:'display-runtime',template,path:template.replace('$id',sentinel),persona:'guest',state:'default'}));
  return [...consoleRoutes,...diagRoutes,...displayRoutes];
}
function origin(cfg,app){return app==='console'?cfg.origin:app==='display-runtime'?cfg.runtime:'http://127.0.0.1:'+cfg.ports.diagnostics;}
async function session(ctx,app,actor,cfg,lab){
  if(actor==='guest')return;
  const site=origin(cfg,app);
  if(app==='console'){
    const u=actor==='projectViewer'?read(join(lab.directory,'private/manual-project-account.json')):read(join(lab.directory,'private/accounts.json'))[actor];
    const result=await ctx.request.get(site+'/api/v1/auth/csrf');assert(result.ok());
    const {token}=await result.json();
    const login=await ctx.request.post(site+'/api/v1/auth/login',{headers:{Origin:site,'X-CSRF-Token':token},data:u});assert.equal(login.status(),200);
    assert.equal((await login.json()).status,'authenticated');
  } else if(app==='diagnostics'){
    const account=read(join(dirname(cfg.config),'diagnostics','accounts.json'));
    const csrf=await(await ctx.request.get(site+'/api/diagnostics/v1/session/bootstrap')).json();
    const resp=await ctx.request.post(site+'/api/diagnostics/v1/session/login',{headers:{Origin:site,'X-CSRF-Token':csrf.data.csrf_token},data:{username:account.username,password:account.password}});
    assert.equal(resp.status(),200);
  }
}
const selector='main,main h1,main h2,main h3,nav,header,aside,footer,main form,label,input,select,textarea,button,a[href],table,th,tbody,[role="alert"],[role="dialog"],[role="tabpanel"],canvas';
async function geometry(page) {
  return page.evaluate(selector=>{
    const nodes=[...document.querySelectorAll(selector)].filter(el=>el.getClientRects().length>0);
    const box=el=>{const r=el.getBoundingClientRect();return [r.x,r.y,r.width,r.height];};
    const name=el=>el.tagName+(el.getAttribute('role')?'['+el.getAttribute('role')+']':'');
    return {scroll:[scrollX,scrollY],url:location.pathname+location.search.replace(/([?&](?:challenge|audience)=)[^&]*/g,'$1<bound>'),focus:document.activeElement?.tagName,
      elements:nodes.map(el=>({tag:name(el),box:box(el)}))};
  },selector);
}
function apiRequest(url){return /^\/(?:api\/|runtime(?:\/|-config$)|local\/)/.test(new URL(url).pathname);}
function responseKey(req){
  const u=new URL(req.url());
  return req.method()+' '+u.pathname+u.search;
}
function trackTraffic(page,row){
  const pending=new Set();let activity=Date.now();
  const relevant=r=>apiRequest(r.url()) && !(row.state==='pending' && injectState(r,row)) && !r.url().includes('/events/stream');
  page.on('request',r=>{if(relevant(r)){pending.add(r);activity=Date.now();}});
  for(const ev of ['requestfailed','requestfinished'])page.on(ev,r=>{if(pending.delete(r))activity=Date.now();});
  return ()=>pending.size===0 && Date.now()-activity>350;
}
async function settle(page,row,idle) {
  await page.locator('body').waitFor({state:'visible'});
  if(row.app==='console' && row.persona!=='guest')await page.locator('header,[role="banner"],main form,main [role="alert"]').first().waitFor({timeout:6000}).catch(()=>{});
  if(row.state==='pending' && row.heading!==null)await page.getByRole('heading',{name:row.heading||'任务中心',exact:true}).first().waitFor({timeout:7000});
  await page.evaluate(()=>document.fonts.ready.then(()=>true));
  const limit=Date.now()+20000;let signature='',steady=Date.now();
  while(Date.now()<limit){
    const value=await page.locator('body').innerText();
    if(value!==signature){steady=Date.now();signature=value;}
    if(idle() && Date.now()-steady>350)return;
    await sleep(65);
  }
  throw Error('Page did not settle at the expected network and DOM state');
}
async function interact(page,row) {
  if(row.interact)await row.interact(page);
  if(row.state.startsWith('search-')){
    const q=page.locator('main input[type="search"], main input:not([type])').first();
    await q.fill(row.state==='search-empty'?'ARC-zero-result-982':'arc_');
    await q.press('Enter');
  }
  if(row.state==='profile-edit'){
    const field=page.getByRole('textbox',{name:'显示名称'});
    await field.fill('架构验收资料');
    await page.getByRole('button',{name:'保存资料'}).waitFor();
    assert(await page.getByRole('button',{name:'保存资料'}).isEnabled());
  }
}
function captureFile(dir,key){return join(dir,key+'.png');}
async function capture(page,row,site,dir,key) {
  const idle=trackTraffic(page,row);
  await page.bringToFront();
  await page.goto(new URL(row.path,site).href,{waitUntil:'domcontentloaded'});
  await settle(page,row,idle);
  await interact(page,row);
  if(row.state.startsWith('search-') || row.state==='profile-edit' || row.interact){await sleep(450);await settle(page,row,idle);}
  if(row.verify)await row.verify(page);
  // Native form controls are painted by the browser compositor. Give each
  // foreground page the same completed font/layout and two paint frames.
  await page.evaluate(async()=>{
    await document.fonts.ready;
    await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  });
  const aria=await page.locator('body').ariaSnapshot({timeout:5000});
  const box=await geometry(page);
  const diagnosticsStyle=row.app==='diagnostics'?await page.evaluate(()=>{
    function containsPaused(rules) {
      return [...rules].some(rule=>rule.selectorText==='.paused' || (rule.cssRules && containsPaused(rule.cssRules)));
    }
    const styles=[...document.styleSheets].map(sheet=>({path:sheet.href?new URL(sheet.href).pathname:'embedded',selector:containsPaused(sheet.cssRules)}));
    const matching=[...document.querySelectorAll('.paused')].map(node=>{
      const css=getComputedStyle(node);return {tag:node.tagName,animationName:css.animationName,playState:css.animationPlayState};
    });
    const affectedAnimations=document.getAnimations().filter(animation=>animation.effect?.target?.matches?.('.paused')).length;
    return {styles,matching,affectedAnimations};
  }):undefined;
  const png=await page.screenshot({path:captureFile(dir,key),animations:'disabled',caret:'hide',scale:'css',timeout:12000});
  write(join(dir,key+'.json'),{box,aria,diagnosticsStyle});
  return {box,aria,png,diagnosticsStyle};
}
export function pixelDiff(a,b,path){
  if(Buffer.compare(a,b)===0){return 0;}
  const left=PNG.sync.read(a),right=PNG.sync.read(b);
  assert.equal(left.width,right.width);assert.equal(left.height,right.height);
  const diff=new PNG({width:left.width,height:left.height});
  let pixels=0;
  for(let i=0;i<left.data.length;i+=4){
    const different=!left.data.subarray(i,i+4).equals(right.data.subarray(i,i+4));
    if(different){pixels++;diff.data.set([230,0,90,255],i);}
    else{const gray=Math.round((left.data[i]+left.data[i+1]+left.data[i+2])/3);diff.data.set([gray,gray,gray,96],i);}
  }
  if(pixels!==0)writeFileSync(path,PNG.sync.write(diff),{mode:0o600});
  return pixels;
}
function compare(before,after,dir,key){
  return {changedPixels:pixelDiff(before.png,after.png,join(dir,key+'-diff.png')),accessibilityEqual:before.aria===after.aria,layoutEqual:JSON.stringify(before.box)===JSON.stringify(after.box)};
}
function injectState(req,row) {
  return ['error','pending'].includes(row.state) && req.method()==='GET' && (row.endpoint?new URL(req.url()).pathname===row.endpoint:req.url().includes('/api/v1/tasks'));
}
export async function visual(dir,side='b',{apps,limit,filter,resume=false,extraRows,extraSource,requestOrder='natural',outputDirectory=dir}={}) {
  assert(requestOrder==='natural'||(requestOrder==='serial'&&extraRows?.length),'Ordered reading requires an explicit supplemental case');
  assert.deepEqual(visualRecordingDrivers(),loadedDrivers,'Visual driver changed after process startup; resume with the recorded inputs');
  const {output:profile,frontend}=await visualFrontend(dir,side,outputDirectory);
  outputDirectory=profile;
  const roleSQL=visualRoleGrants(dir,side);
  const lab=read(join(dir,'private/lab.json')),base=read(join(dir,'baseline.json'));
  const rows=extraRows||pathList(base,lab,read(join(dir,'samples.json')));
  let selected=apps?rows.filter(v=>apps.includes(v.app)):rows;
  if(filter)selected=selected.filter(row=>row.template===filter);
  if(limit)selected=selected.slice(0,limit);
  const category=requestOrder==='serial'?'ordered-read-states':extraRows?'feature-states':'paired';
  const output=join(outputDirectory,'visual',category,side,...(apps||limit||filter?['selection-'+digest(JSON.stringify({apps,limit,filter})).slice(0,12)]:[]));
  mkdirSync(output,{recursive:true,mode:0o700});
  const browser=await chromium.launch({headless:true,args:['--disable-gpu','--disable-gpu-compositing','--disable-lcd-text','--disable-skia-runtime-opts','--num-raster-threads=1','--disable-partial-raster','--run-all-compositor-stages-before-draw','--force-device-scale-factor=1','--force-color-profile=srgb']});
  const matrix=[],browserVersion=browser.version();
  const assets=side==='self'?base.staticFiles:(existsSync(join(dir,'static-files.json'))?read(join(dir,'static-files.json')).candidate:base.staticFiles);
  const snapshot=read(join(dir,'snapshot/manifest.json')).pg.seed;
  const candidate=read(join(dir,'candidate.json')).inputs.sourceSHA256;
  const {driver,recordingDriver}=visualRecordingDrivers();
  const identity=digest(JSON.stringify([driver,recordingDriver,requestOrder,snapshot,browserVersion,base.commit,candidate,roleSQL,assets,frontend.identity,extraSource,extraRows?.map(r=>[r.template,r.state,r.path,r.bounds])]));
  let previous=existsSync(join(output,'matrix.json'))?read(join(output,'matrix.json')):null;
  if(previous && (!resume || previous.identity!==identity)){
    const archive=join(outputDirectory,'visual','attempts',category,side,previous.identity+'-'+Date.now());
    mkdirSync(dirname(archive),{recursive:true,mode:0o700});
    renameSync(output,archive);mkdirSync(output,{recursive:true,mode:0o700});previous=null;
  }
  const prior=new Map((previous?.identity===identity?previous.rows:[]).map(r=>[r.key,r]));
  const finished=new Map(prior);
  const time=read(join(dir,'samples.json')).clock || read(join(dir,'baseline.json')).capturedAt || '2026-11-18T06:00:00.000Z';
  async function save(){
    for(const record of matrix)finished.set(record.key,record);
    const observations=[...finished.values()].sort((a,b)=>a.key.localeCompare(b.key));
    write(join(output,'matrix.json'),{identity,frontend,driver,recordingDriver,requestOrder,browserVersion,time,timezone:'Asia/Shanghai',language:'zh-CN',deviceScaleFactor:1,masks:[],pixelTolerance:0,viewportTolerance:0,
      baseline:base.commit,candidate,roleSQL,sourceDirectory:dir,snapshot,expectedRows:rows.length*viewports.length,rows:observations,
      complete:observations.length===rows.length*viewports.length,passed:observations.length===rows.length*viewports.length && observations.every(r=>r.passed)});
  }
  try{
    outer:for(const viewport of viewports) {
      const contexts=new Map(),sessions=new Map();
      const make=async(side,app,actor,name)=>{
        const key=[side,app,actor,name].join('/');
        if(!contexts.has(key)){
          const subject=[side,app,actor].join('/');
          const ctx=await browser.newContext({viewport,storageState:sessions.get(subject),deviceScaleFactor:1,isMobile:viewport.width===375,hasTouch:viewport.width===375,reducedMotion:'reduce',timezoneId:'Asia/Shanghai',locale:'zh-CN',serviceWorkers:'block'});
          contexts.set(key,ctx);
          if(!sessions.has(subject)){
            try{await session(ctx,app,actor,lab[side],lab);sessions.set(subject,await ctx.storageState());}
            catch(error){contexts.delete(key);await ctx.close();throw error;}
          }
        }
        return contexts.get(key);
      };
      try{
        for(const row of selected){
          if(process.env.ARC_VISUAL_CHECKPOINT_FILE&&existsSync(process.env.ARC_VISUAL_CHECKPOINT_FILE))break outer;
          const idx=rows.indexOf(row);
          const key=row.app+'-'+viewport.width+'-'+String(idx).padStart(3,'0'), where=join(output,key);
          if(prior.has(key)){matrix.push(prior.get(key));continue;}
          mkdirSync(where,{recursive:true,mode:0o700});
          const record={...row,viewport,key,passed:false};matrix.push(record);
          const peer={...row,...row.bounds?.[side]};
          const active={...row,...row.bounds?.a};
          const bound=(value,which)=>{
            let result=value;for(const [k,id] of Object.entries(row.bounds?.[which]?.ids||{}))result=result.replaceAll(id,'{'+k+'}');return result;
          };
          const pages=[],transportErrors=[],browserErrors=[],statuses=[],liveStatuses=[];
          const guard=fn=>async route=>{try{await fn(route);}catch(e){if(!route.request().frame().page().isClosed())transportErrors.push({method:route.request().method(),path:new URL(route.request().url()).pathname,error:e.name});await route.abort('failed').catch(()=>{});}};
          const open=async(side,name)=>{
            const ctx=await make(side,row.app,row.persona,name),page=await ctx.newPage();
            pages.push(page);
            page.on('pageerror',err=>browserErrors.push({side,view:name,kind:err.name,message:safeDiagnosticLine(err.message).slice(0,300)}));
            await page.clock.setFixedTime(new Date(time));return page;
          };
          try{
            const a=await open('a','reference');
            // Public response fixtures exist in memory only. Cookies, one-use
            // challenges and response bodies never enter screenshot metadata.
            const fixtures=createVisualRecording();
            const observe=async(page,statuses,fixtures,current)=>{
              const schedule=scheduleVisualRequests(requestOrder);
              const call=async route=>{
                const req=route.request();
                if(!apiRequest(req.url()))return route.continue();
                if(injectState(req,current)){
                  if(row.state==='pending'){statuses.push({request:responseKey(req),status:'pending'});return;}
                  statuses.push({request:responseKey(req),status:503});
                  return route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:'unavailable',message:'服务暂时不可用，请稍后重试。',request_id:'arc-frozen-request'})});
                }
                const saveResponse=fixtures?.reserve(req);
                const res=await route.fetch({timeout:22000,maxRedirects:0});
                try{
                  const body=await res.body(),id=responseKey(req);
                  saveResponse?.({status:res.status(),body,headers:res.headers()});
                  const observed={request:id,status:res.status()};
                  if(row.observeRead)observed.read=row.observeRead(req,res.status(),body,current.ids);
                  if(current.requireEmpty&&req.method()==='GET'&&new URL(req.url()).pathname===current.endpoint&&res.ok()){
                    const json=JSON.parse(body.toString()),value=json.data??json;
                    const items=Array.isArray(value)?value:['items','nodes','roots','jobs','attachments','rows'].map(key=>value?.[key]).find(Array.isArray);
                    if(items)observed.itemCount=items.length;
                  }
                  statuses.push(observed);
                  await route.fulfill({response:res,body});
                }finally{await res.dispose();}
              };
              await page.route('**/*',guard(route=>apiRequest(route.request().url())?schedule(()=>call(route)):route.continue()));
            };
            const target=await open(side,'paired');const unseen=[];
            const live=await open(side,'live');
            // Both live browsers use the same network observation method. A
            // router redirect can abort an in-flight browser request while its
            // route.fetch still completes, so a direct response listener on
            // only one side would compare different transport lifecycles.
            await Promise.all([observe(a,statuses,fixtures,active),observe(live,liveStatuses,undefined,peer)]);
            const oldLive=await capture(a,active,origin(lab.a,row.app),where,'old-live');
            // Each real browser observes only the requests leading to its own
            // screenshot. Leaving a page open would count timer-driven refreshes
            // while the other three captures are running.
            await a.close();
            const liveResult=await capture(live,peer,origin(lab[side],row.app),where,'live');
            await live.close();
            const liveDiff=compare(oldLive,liveResult,where,'live');
            const replay=reader=>async route=>{
              const req=route.request();if(!apiRequest(req.url()))return route.continue();
              if(injectState(req,active)){
                if(row.state==='pending')return;
                return route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:'unavailable',message:'服务暂时不可用，请稍后重试。',request_id:'arc-frozen-request'})});
              }
              const id=responseKey(req),fixture=await reader.take(req);
              if(!fixture){unseen.push(id);return route.abort('failed');}
              const headers={...fixture.headers};delete headers['set-cookie'];delete headers['content-encoding'];delete headers['content-length'];
              await route.fulfill({status:fixture.status,body:fixture.body,headers});
            };
            const staticA=await open('a','paired-baseline');
            const originalReader=fixtures.reader(),peerReader=fixtures.reader();
            await Promise.all([staticA.route('**/*',guard(replay(originalReader))),target.route('**/*',guard(replay(peerReader)))]);
            const reference=await capture(staticA,active,origin(lab.a,row.app),where,'old');
            await staticA.close();
            const paired=await capture(target,active,origin(lab[side],row.app),where,'paired');
            await target.close();
            const fixed=compare(reference,paired,where,'paired');
            const unusedInputs={baseline:originalReader.unusedBusinessInputs(),peer:peerReader.unusedBusinessInputs()};
            const playedInputsEqual=isDeepStrictEqual(unusedInputs.baseline,unusedInputs.peer);
            const stableRequests=(values,which)=>{
              const normalized=values.filter(item=>item.request!=='POST /api/v1/auth/activity').map(item=>{
                const [method,path]=item.request.split(' '),u=new URL(path,'http://localhost');
                for(const parameter of ['cursor','page_cursor','challenge','audience','snapshot_id','request_id','token','code','permit']){
                  if(u.searchParams.has(parameter))u.searchParams.set(parameter,'bound-'+parameter);
                }
                return bound(method+' '+u.pathname+u.search+' '+item.status,which);
              });
              const uniqueReads=new Set(normalized.filter(value=>/^(GET|HEAD) /.test(value)));
              return [...uniqueReads,...normalized.filter(value=>!/^(GET|HEAD) /.test(value))].sort();
            };
            const requestsMatch=JSON.stringify(stableRequests(statuses,'a'))===JSON.stringify(stableRequests(liveStatuses,side));
            const eligible=(result,statuses,current)=>{
              if(current.requireEmpty&&!statuses.some(v=>v.itemCount===0))return false;
              if(row.state==='pending'&&!statuses.some(v=>v.status==='pending'))return false;
              if(row.state==='error'&&!statuses.some(v=>v.status===503&&v.request.startsWith('GET '+(current.endpoint||'/api/v1/tasks'))))return false;
              if(row.app!=='console' || row.persona==='guest')return true;
              const signedIn=statuses.some(v=>v.request==='GET /api/v1/auth/me' && v.status===200);
              if(!signedIn || result.box.url.startsWith('/login') || result.aria.includes('正在确认登录状态'))return false;
              if(row.expectation==='scope-guard'){
                return statuses.some(v=>[403,404].includes(v.status)) || /没有权限|无权|不可用|无访问权限|没有可用的工作空间/.test(result.aria);
              }
              if(row.state==='error')return statuses.some(v=>v.request.startsWith('GET '+(current.endpoint||'/api/v1/tasks')) && v.status===503);
              return !statuses.some(v=>v.status>=500 || v.status===401 || v.status===403 || v.status===404);
            };
            const liveAuthorized=eligible(oldLive,statuses,active)&&eligible(liveResult,liveStatuses,peer);
            const comparedURL=(result,actor,observed)=>{
              const url=new URL(result.box.url,'http://visual.invalid');
              const cursor=url.searchParams.get('cursor');
              if(row.state==='second-page'&&cursor){
                assert(observed.some(fact=>{
                  const [method,path]=fact.request.split(' ');
                  const endpoint=new URL(path,'http://visual.invalid');
                  return method==='GET'&&fact.status===200&&endpoint.pathname.endsWith('/audit/events')&&endpoint.searchParams.get('cursor')===cursor;
                }),'The browser must page with the cursor accepted by the live audit endpoint');
                url.searchParams.set('cursor','accepted-existing-page');
              }
              return bound(url.pathname+url.search+url.hash,actor);
            };
            // Frozen replay carries the same cursor as the baseline response.
            // Authenticate the peer cursor against its own live request first.
            const urlMatch=comparedURL(reference,'a',statuses)===comparedURL(liveResult,side,liveStatuses);
            const reads=row.observeRead?{baseline:statuses.flatMap(v=>v.read?[v.read]:[]),peer:liveStatuses.flatMap(v=>v.read?[v.read]:[])}:undefined;
            const readsMatch=!row.verifyReads||row.verifyReads(reads);
            if(row.app==='diagnostics'){
              record.diagnosticsStyle=Object.fromEntries([['baseline',oldLive],['peer',liveResult],['fixedBaseline',reference],['fixedPeer',paired]]
                .map(([name,capture])=>[name,capture.diagnosticsStyle]));
              record.sharedUtilityUnused=Object.values(record.diagnosticsStyle).every(view=>view.matching.length===0 && view.affectedAnimations===0);
            }
            Object.assign(record,{liveAuthorized,baselinePath:reference.box.url,livePath:liveResult.box.url,fixed,live:liveDiff,requestsMatch,baselineRequests:stableRequests(statuses,'a'),liveRequests:stableRequests(liveStatuses,side),unseen,recording:fixtures.summary(),unusedInputs,playedInputsEqual,...(reads?{reads,readsMatch}:{}),
              ...(active.requireEmpty?{itemCounts:{baseline:statuses.filter(v=>v.itemCount!==undefined).map(v=>v.itemCount),live:liveStatuses.filter(v=>v.itemCount!==undefined).map(v=>v.itemCount)}}:{}),
              transportErrors,passed:liveAuthorized && transportErrors.length===0 && fixed.changedPixels===0 && fixed.layoutEqual && fixed.accessibilityEqual && !unseen.length && playedInputsEqual && requestsMatch && urlMatch && readsMatch && record.sharedUtilityUnused!==false});
          }catch(e){record.error=(e.message || String(e)).replaceAll(/https?:\/\/[^\s]+/g,'[lab URL]').slice(0,700);}
          finally{
            for(const p of pages.reverse())await p.close().catch(()=>{});
            record.browserErrors=browserErrors;
            if(browserErrors.length)record.passed=false;
            if(!record.baselineRequests){
              const publicStatuses=items=>items.map(v=>{
                const [method,url]=v.request.split(' ');
                return method+' '+new URL(url,'http://localhost').pathname+' '+v.status;
              });
              record.baselineRequests=publicStatuses(statuses);record.liveRequests=publicStatuses(liveStatuses);
            }
            if(row.observeRead&&!record.reads){
              record.reads={baseline:statuses.flatMap(v=>v.read?[v.read]:[]),peer:liveStatuses.flatMap(v=>v.read?[v.read]:[])};
              record.readsMatch=row.verifyReads(record.reads);
            }
          }
          if(!record.passed)console.error('Visual difference '+key+' '+row.path+' '+(record.error||JSON.stringify(record.fixed)));
          await save();
        }
      }finally{await Promise.all([...contexts.values()].map(c=>c.close()));await save();}
    }
  }finally{await save();await browser.close();}
  const result=read(join(output,'matrix.json'));
  if(result.complete){
    assert.deepEqual(visualRecordingDrivers(),loadedDrivers,'Visual driver changed during capture');
    result.frontendAtEnd=await finishVisualFrontend(dir,side,outputDirectory,frontend);
    write(join(output,'matrix.json'),result);
  }
  return result;
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const dir=runDirectory(process.argv[2]);
  const r=await visual(dir,process.argv[3]||'b',{resume:process.env.ARC_VISUAL_RESUME==='1',filter:process.env.ARC_VISUAL_TEMPLATE,apps:process.env.ARC_VISUAL_APPS?.split(','),limit:Number(process.env.ARC_VISUAL_LIMIT || 0)});
  console.log(r.rows.filter(v=>v.passed).length+'/'+r.rows.length+' visual cases');
  process.exitCode=r.passed?0:1;
}
