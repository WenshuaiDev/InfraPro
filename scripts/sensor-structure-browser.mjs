import {spawn,execFileSync} from 'node:child_process';
import {mkdirSync,readFileSync,writeFileSync,openSync,closeSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {randomUUID,createHash} from 'node:crypto';
import {chromium,request,expect as baseExpect} from '@playwright/test';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
import {seedSensorStructure} from './fixtures/sensor-structure-acceptance.mjs';

const expect=baseExpect.configure({timeout:30000}),root=resolve('.'),out=join(root,'.artifacts/sensor-structure/browser');
mkdirSync(out,{recursive:true,mode:0o700});
const consoleOrigin='http://display-console.127.0.0.1.nip.io:15573',runtimeOrigin='http://display-runtime.127.0.0.1.nip.io:15880',api='http://127.0.0.1:18580/api/v1';
const children=[],checks=[],errors=[];let browser,client,page,screen,state,passed=false;
function start(command,args,cwd,env={}) {const fd=openSync(join(out,'server.log'),'a'),child=spawn(command,args,{cwd,detached:true,env:{...process.env,...env},stdio:['ignore',fd,fd]});closeSync(fd);children.push(child);child.unref();}
const wm=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'SBM 真实浏览器自验'});
try {
 if(process.env.INFRAPRO_SBM_REUSE!=='1') {
  writeFileSync(join(out,'server.log'),'');ensureTestDependencies();
  if(process.env.INFRAPRO_SBM_SKIP_BUILD!=='1')execFileSync('pnpm',['--filter','@infrapro/console','build'],{stdio:'inherit'});
  execFileSync('go',['test','-c','-o',join(out,'api-test'),'./services/api/internal/transport/http'],{stdio:'inherit'});
  start(join(out,'api-test'),['-test.run','^TestE2EServer$','-test.v','-test.timeout','60m'],join(root,'services/api/internal/transport/http'),{...testEnv,INFRAPRO_E2E_SERVER:'1',INFRAPRO_DISPLAY_E2E:'1',INFRAPRO_MONITORING_E2E:'1',INFRAPRO_SPATIAL_E2E:'1',INFRAPRO_SPATIAL_PYTHON:join(root,'.artifacts/spatial-venv/bin/python'),INFRAPRO_SPATIAL_CONVERTER_SCRIPT:join(root,'tools/spatial-converter/convert.py'),INFRAPRO_HTTP_ADDR:'127.0.0.1:18580',INFRAPRO_E2E_CONSOLE_URL:consoleOrigin,INFRAPRO_E2E_ORIGINS:consoleOrigin+',http://127.0.0.1:15573'});
  start(process.execPath,[join(root,'apps/console/node_modules/vite/bin/vite.js'),'preview'],join(root,'apps/console'),{API_PROXY_TARGET:'http://127.0.0.1:18580',CONSOLE_PORT:'15573',__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS:'display-console.127.0.0.1.nip.io'});
  writeFileSync(join(out,'processes.json'),JSON.stringify(children.map(c=>c.pid)));
 }
 await expect.poll(async()=>{try{return (await fetch('http://127.0.0.1:18580/healthz')).ok}catch{return false}},{timeout:90000}).toBe(true);
 client=await request.newContext();const csrf=await(await client.get(api+'/auth/csrf')).json(),headers={Origin:consoleOrigin,'X-CSRF-Token':csrf.token};
 const call=async(path,data,method=data===undefined?'GET':'POST')=>{const r=await client.fetch(api+path,{method,headers,data});if(!r.ok())throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);return r.status()===204?{}:r.json();};
 const wait=async(path,pick,want)=>{let data;await expect.poll(async()=>{data=await call(path);return pick(data)},{timeout:120000}).toBe(want);return data;};
 await call('/auth/login',{username:'e2e_owner',password:'Real browser 河流 secret 592'});
 if(process.env.INFRAPRO_SBM_REUSE==='1')state=JSON.parse(readFileSync(join(out,'fixture.json'),'utf8'));
 else {
  const owner=(await call('/access/accounts?q=e2e_owner')).items.find(a=>a.username==='e2e_owner');
  state=await seedSensorStructure({call,client,api,headers,accountID:owner.id,wait});
  writeFileSync(join(out,'fixture.json'),JSON.stringify(state,null,2),{mode:0o600});
 }
 checks.push('public v2 facility types, public and tenant templates, 9.5 mixed structure, classification, six devices, independent grants, six source bindings, seventy formal observations via HTTP');
 const pb=`/tenants/${state.tenant}/projects/${state.project}`,displays=pb+'/displays';
 let path;
 if(!state.display) {
  const settings=(await call(displays)).settings;await call(displays+'/settings',{...wm(settings.revision),enabled:true},'PUT');
  const created=await call(displays+'/instances',{...wm(),name:'SBM 工程结构与设备分类验收'});state.display=created.id;path=displays+'/instances/'+created.id;
  const command=async(action,body,method='POST')=>{const d=await call(path);return call(path+action,{...wm(d.instance.revision),...body},method);};
  const zip=readFileSync(join(root,'.artifacts/display/engineering-structure-example.zip')),hash=createHash('sha256').update(zip).digest('hex'),upload=await command('/uploads',{size:zip.length,sha256:hash});
  const part=await client.put(api+path+`/uploads/${upload.upload_id}/parts/0`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Content-SHA256':hash},data:zip});expect(part.ok()).toBe(true);await command(`/uploads/${upload.upload_id}/complete`,{});await wait(path,v=>v.packages[0]?.state,'ready');
  const definitions=[['project_resources','resources',{kind:'project'},['resource.metadata','resource.attributes','resource.position','statistics.counts']],['project_raw','channels',{kind:'ids',ids:Object.values(state.channels).map(v=>v.id)},['resource.metadata','source.binding','monitoring.values','monitoring.quality','statistics.counts']],['project_devices','devices',{kind:'project'},['device.metadata','device.outputs','device.installations','device.classification','statistics.counts','statistics.classification']]],scopes=definitions.map(([id,domain,selector,field_groups])=>({id,structure_model:2,domain,selector,field_groups,include_future:false})),views=Object.fromEntries(scopes.map(({id,...s})=>[id,{...s,scope_id:id}])),manifest=JSON.parse(readFileSync(join(root,'examples/display-engineering-structure/display.manifest.json'),'utf8'));
  const capabilities=[...Object.keys(manifest.required_capabilities),...Object.keys(manifest.optional_capabilities)].filter(c=>!c.startsWith('spatial.'));
  await command('/grant',{grant:{scope_format:2,read_scopes:scopes,capabilities,valid_until:new Date(Date.now()+3600000).toISOString()}},'PUT');
  const d=await call(path),config=await command('/configs',{package_id:d.packages[0].id,config_format:2,read_views:views,parameters:{title:'SBM 完整方案验收'},bindings:{},mode:'follow'}),prepared=await command('/prepare',{config_id:config.config_id});await command('/publish',{config_id:config.config_id,preparation:prepared.hash});
  writeFileSync(join(out,'fixture.json'),JSON.stringify(state,null,2),{mode:0o600});
 }else path=displays+'/instances/'+state.display;
 browser=await chromium.launch({headless:true,args:['--use-angle=swiftshader','--enable-webgl','--host-resolver-rules=MAP display-console.127.0.0.1.nip.io 127.0.0.1, MAP display-runtime.127.0.0.1.nip.io 127.0.0.1, MAP display-assets.127.0.0.1.nip.io 127.0.0.1','--no-proxy-server']});
 const context=await browser.newContext({viewport:{width:1440,height:1000}});page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
 await page.goto(consoleOrigin+'/login');await page.getByLabel('用户名',{exact:true}).fill('e2e_owner');await page.getByLabel('密码',{exact:true}).fill('Real browser 河流 secret 592');await page.getByRole('button',{name:'登录',exact:true}).click();await page.waitForURL(u=>!u.pathname.includes('login'));
 screen=await context.newPage();screen.on('pageerror',e=>errors.push(e.message));await screen.goto(runtimeOrigin+'/display/'+state.display);await screen.waitForURL(u=>u.pathname==='/display-launch');await screen.getByRole('button',{name:'继续',exact:true}).click();await expect(screen.locator('#status')).toHaveText('已连接 · 应用已就绪');
 const app=screen.frameLocator('iframe:not(.preparing)'),card=(label)=>app.locator('.card').filter({has:app.getByText(label,{exact:true})}).locator('strong');
 for(const [label,count]of [['设施','2'],['断面','5'],['孔位','4'],['测点','7']])await expect(card(label)).toHaveText(count);
 await expect(app.locator('.error')).toHaveCount(0);await screen.screenshot({path:join(out,'sdk-structure.png'),fullPage:true});
 await app.getByLabel('归属口径').selectOption('subtree');await app.getByRole('button',{name:'读取最新统计',exact:true}).click();await expect(app.getByText('子树包含有重叠，各组不可相加作为唯一总数。',{exact:true})).toBeVisible();
 await app.locator('[data-page="categories"]').click();await expect(card('唯一设备')).toHaveText('6');await expect(card('传感器')).toHaveText('4');await expect(card('使用传感器类别')).toHaveText('3');await screen.screenshot({path:join(out,'sdk-categories.png'),fullPage:true});
 await app.getByLabel('设备范围').selectOption('monitoring_bound');await app.getByRole('button',{name:'读取最新分类统计'}).click();await expect(card('唯一设备')).toHaveText('4');await expect(card('传感器')).toHaveText('3');
 await app.locator('[data-page="monitoring"]').click();await expect(card('在用原始观测')).toHaveText('70',{timeout:120000});await expect(app.locator('.error')).toHaveCount(0);await screen.screenshot({path:join(out,'sdk-observations.png'),fullPage:true});checks.push('real SDK/Bridge: 5 sections, 4 holes, 7 points; 6/4 devices; 4/3 sensors; 70 observations');
 const workspace=consoleOrigin+`/t/${state.tenant}/resources/facilities/${state.nodes.A.id}`;
 await page.goto(workspace+'?borehole='+state.nodes.H1.id);await expect(page.getByRole('heading',{name:'H1',exact:true})).toBeVisible();await page.screenshot({path:join(out,'borehole-workspace.png'),fullPage:true});
 await page.goto(workspace+'?point='+state.nodes.P1.id);await expect(page.getByRole('heading',{name:'P1',exact:true})).toBeVisible();await page.getByRole('button',{name:'维护孔内定位',exact:true}).click();const positionPanel=page.getByRole('dialog');await positionPanel.getByLabel('沿孔距离（m，未知留空）').fill('0');await positionPanel.getByLabel('沿孔起点',{exact:true}).fill('孔口');await positionPanel.getByLabel('沿孔方向',{exact:true}).fill('沿孔向下');await positionPanel.getByLabel('测量来源',{exact:true}).fill('真实浏览器合成验收');await positionPanel.getByLabel('操作原因 *',{exact:true}).fill('保留有效零距离及未知垂直深度');await positionPanel.getByRole('button',{name:'预览变更',exact:true}).click();await positionPanel.getByRole('button',{name:'确认提交',exact:true}).click();await expect(positionPanel).toHaveCount(0);await expect(page.getByText('真实浏览器合成验收 · 当前定位',{exact:true})).toBeVisible();await page.screenshot({path:join(out,'point-position.png'),fullPage:true});
 await page.goto(consoleOrigin+`/t/${state.tenant}/resources/facilities/${state.nodes.N.id}`);await expect(page.getByRole('heading',{name:'无孔设施 N',exact:true})).toBeVisible();await page.getByRole('button',{name:'添加下级',exact:true}).click();await expect(page.getByRole('menuitem',{name:'新增孔位',exact:true})).toHaveCount(0);await page.keyboard.press('Escape');checks.push('borehole details, zero vs unknown position and no-hole menu');
 const beforeDictionary=await call('/platform/business-dictionaries/device-category'),environmentName=beforeDictionary.draft.nodes.find(n=>n.id===state.categories.environment).name,newEnvironmentName=environmentName==='环境类'?'环境传感器':'环境类';await page.goto(consoleOrigin+'/platform/device-categories');await expect(page.getByRole('heading',{name:'设备分类字典',exact:true})).toBeVisible();await page.getByLabel('搜索分类名称或编号').fill(environmentName);await page.getByRole('button',{name:new RegExp('传感器 / '+environmentName)}).click();await page.getByRole('dialog').getByLabel('分类名称 *',{exact:true}).fill(newEnvironmentName);await page.getByRole('button',{name:'完成草稿编辑'}).click();await page.getByLabel('维护或发布原因 *',{exact:true}).fill('真实浏览器改名验收，身份保持');await page.getByRole('button',{name:'校验并保存草稿'}).click();await expect(page.getByText(new RegExp('草稿修订 '+(Number(beforeDictionary.draft_revision)+1)+' ·'))).toBeVisible();await page.getByLabel('维护或发布原因 *',{exact:true}).fill('真实浏览器发布核对');await page.getByRole('button',{name:'预览发布',exact:true}).click();await page.getByRole('button',{name:'确认发布分类版本'}).click();await expect.poll(async()=>(await call('/platform/business-dictionaries/device-category')).current_release_id).not.toBe(beforeDictionary.current_release_id);await page.screenshot({path:join(out,'dictionary-published.png'),fullPage:true});checks.push('classification rename, draft save, fixed publish preview and publication in UI');
 await page.setViewportSize({width:390,height:844});await page.goto(workspace+'?monitoring_section='+state.nodes.S3.id);await expect(page.getByRole('heading',{name:'S3',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:join(out,'mobile-section.png'),fullPage:true});checks.push('mobile section layout');
 const d=await call(path),terminal=await call(path+'/terminals',{...wm(d.instance.revision),name:'SBM 真实终端',valid_until:new Date(Date.now()+3600000).toISOString()}),wallContext=await browser.newContext(),wall=await wallContext.newPage();await wall.goto(runtimeOrigin+'/terminal');await wall.getByLabel('一次性配对码').fill(terminal.pair_code);await wall.getByRole('button',{name:'配对并启动'}).click();await expect(wall.locator('#status')).toHaveText('已连接 · 应用已就绪');await expect(wall.frameLocator('iframe:not(.preparing)').locator('.card').filter({has:wall.frameLocator('iframe:not(.preparing)').getByText('断面',{exact:true})}).locator('strong')).toHaveText('5');await wall.screenshot({path:join(out,'paired-terminal.png'),fullPage:true});await wallContext.close();checks.push('fresh terminal pairing reads actual scoped mixed structure');
 expect(errors).toEqual([]);passed=true;
}catch(e){if(page){writeFileSync(join(out,'failure.txt'),await page.locator('body').innerText().catch(()=>''));await page.screenshot({path:join(out,'failure.png'),fullPage:true}).catch(()=>{});}if(screen)writeFileSync(join(out,'sdk-failure.txt'),await screen.frameLocator('iframe:not(.preparing)').locator('body').innerText().catch(()=>''));throw e;
}finally {
 writeFileSync(join(out,'result.json'),JSON.stringify({passed,checks,errors,completedAt:new Date().toISOString()},null,2));await browser?.close();await client?.dispose();
 if(process.env.INFRAPRO_SBM_KEEP!=='1')for(const child of children.reverse())try{process.kill(-child.pid,'SIGTERM')}catch{/* Already stopped. */}
}
