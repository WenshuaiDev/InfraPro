import assert from 'node:assert/strict';
import { finishVisualFrontend, visualFrontend } from './architecture-visual-frontend.mjs';
import { createHash } from 'node:crypto';
import { existsSync,mkdirSync,readFileSync,renameSync } from 'node:fs';
import { dirname,join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, expect as browserExpect } from '@playwright/test';
import { client,read,runDirectory,sleep,write,queryPG } from './architecture-lab.mjs';
import { analysisBody } from './architecture-analysis-parity.mjs';
import { visualRoleGrants } from './architecture-visual.mjs';
const expect=browserExpect.configure({timeout:18000});

export async function navigationInputs(dir,side,scope='tenant'){
  assert(['tenant','project'].includes(scope));
  const lab=read(join(dir,'private/lab.json')),samples=read(join(dir,'samples.json')),s=samples.structure;
  const user=await client(lab[side],read(join(dir,'private/accounts.json')).admin);
  const scopedBase='/tenants/'+s.tenant+(scope==='project'?'/projects/'+s.project:'');
  const tenant=scopedBase+'/monitoring/derived',analysis=tenant+'/analysis';
  const file=join(dir,'private',side,scope==='tenant'?'navigation.json':'navigation-project.json');
  const command=(action,revision='0')=>{
    const v=createHash('sha256').update(lab.id+':navigation:'+(scope==='project'?'project:':'')+action).digest('hex');
    return {request_id:[v.slice(0,8),v.slice(8,12),'4'+v.slice(13,16),'8'+v.slice(17,20),v.slice(20,32)].join('-'),expected_revision:revision,reason:'既有入口历史成果回放'};
  };
  try{
    const previous=existsSync(file)?read(file):undefined;
    if(previous?.noticeTask){
      const current=await user.call(analysis+'/artifacts/'+previous.artifact+'/versions');
      if([previous.version1,previous.version2].every(id=>current.items.some(v=>v.id===id)))return previous;
    }
    const scopeClause=scope==='tenant'?'project_id IS NULL':"project_id='"+s.project+"'";
    const key=queryPG(lab,side,"SELECT id FROM analysis_configs WHERE tenant_id='"+s.tenant+"' AND "+scopeClause+" AND code='ARC_NAV_VERSIONS'");
    const config=key?await user.call(analysis+'/configs/'+key):(await user.call(analysis+'/configs',{...command('config'),code:'ARC_NAV_VERSIONS',name:'历史成果走查规则',body:analysisBody(s)})).config;
    const path=analysis+'/configs/'+config.id;
    let fixed=(await user.call(path+'/versions')).items[0];
    if(!fixed){
      const v=await user.call(path+'/validate',{draft_revision:config.draft.revision});assert.deepEqual(v.issues,[]);
      fixed=(await user.call(path+'/versions',{...command('fixed-config',config.revision),draft_revision:config.draft.revision,validation_hash:v.validation_hash})).version;
    }
    assert.match(fixed.config_hash,/^[a-f0-9]{64}$/);
    const from=BigInt(Date.parse('2026-11-01T00:00:00Z'))*1000000n;
    const output_range={from_ns:String(from),to_ns:String(from+600000000000n)};
    const create=async(key,options)=>{
      const run=await user.call(analysis+'/runs',{...command(key),config_version_id:fixed.id,config_hash:fixed.config_hash,output_range,...options});
      const deadline=Date.now()+180000;
      while(Date.now()<deadline){
        const result=await user.call(tenant+'/runs/'+run.id);
        if(result.job.state==='succeeded')return result.analysis;
        assert(!['failed','needs_action','cancelled'].includes(result.job.state),'Historical navigation input '+result.job.state);
        await sleep(250);
      }
      throw Error('Historical navigation fixture did not finish');
    };
    const first=await create('first-version',{artifact_name:'既有历史成果'}),second=await create('second-version',{artifact_id:first.artifact_id});
    assert.equal(first.artifact_id,second.artifact_id);assert.notEqual(first.artifact_version_id,second.artifact_version_id);
    const result={artifact:first.artifact_id,version1:first.artifact_version_id,version2:second.artifact_version_id};
    for(const [key,num] of [['version1','1'],['version2','2']]){
      const detail=await user.call(analysis+'/artifacts/'+result.artifact+'?version_id='+result[key]);
      assert.equal(detail.version.version_no,num);assert.equal(detail.version.output_rows,'10');
      const rows=await user.call(analysis+'/artifacts/'+result.artifact+'/query',{version_id:result[key],limit:20});
      assert.deepEqual(rows.items.map(v=>Number(v.value)),[0,2,4,6,8,10,12,14,16,18]);
    }
    const sql="SELECT n.id FROM account_notifications n JOIN notification_events e ON n.event_id=e.id WHERE n.account_id='"+samples.actors.admin+"' AND e.event_type='task.result' AND e.outcome='adopted' AND e.target_id='"+s.batch+"'";
    const deadline=Date.now()+30000;
    do{result.notification=queryPG(lab,side,sql);if(result.notification)break;await sleep(250);}while(Date.now()<deadline);
    assert.match(result.notification,/^[0-9a-f-]{36}$/);
    const target=await user.call('/me/notifications/'+result.notification+'/target');
    assert.equal(target.status,'available');assert.equal(target.kind,'task');assert.equal(target.generation,'1');
    result.noticeTask=target.id;assert.match(result.noticeTask,/^[0-9a-f-]{36}$/);
    write(file,result);return result;
  }finally{await user.client.dispose();}
}
async function walk(browser,dir,side,viewport,inputs,outputDirectory,storageState) {
  const lab=read(join(dir,'private/lab.json')),cfg=lab[side],s=read(join(dir,'samples.json')).structure;
  const ctx=await browser.newContext({viewport,storageState,locale:'zh-CN',timezoneId:'Asia/Shanghai',reducedMotion:'reduce',deviceScaleFactor:1,hasTouch:viewport.width===375});
  const page=await ctx.newPage(),steps=[],responses=[];
  const pending=new Set();let lastActivity=Date.now();
  const watch=r=>new URL(r.url()).pathname.startsWith('/api/v1/')&&!r.url().includes('/events/stream');
  page.on('request',r=>{if(watch(r)){pending.add(r);lastActivity=Date.now();}});
  for(const event of ['requestfailed','requestfinished'])page.on(event,r=>{if(pending.delete(r))lastActivity=Date.now();});
  page.on('response',res=>{
    const url=new URL(res.url());
    if(url.pathname.startsWith('/api/v1/'))responses.push({method:res.request().method(),template:url.pathname.replaceAll(/[a-f0-9-]{36}/g,'{id}'),status:res.status()});
  });
  const output=join(outputDirectory,'visual/navigation',side,viewport.width.toString());mkdirSync(output,{recursive:true,mode:0o700});
  const capture=async(name,heading)=>{
    if(name==='business-target')await expect(page.getByRole('tab',{name:'概览',exact:true})).toHaveAttribute('aria-selected','true',{timeout:15000});
    if(name==='project-selected')await expect(page.getByRole('heading',{name:'项目资料',exact:true})).toBeVisible();
    if(name==='source-task-back')await expect(page.getByRole('button',{name:/打开来源|打开原业务|查看结果|查看来源/}).first()).toBeVisible();
    if(['notice-detail','notice-browser-back'].includes(name))await expect(page.getByRole('button',{name:'打开业务页面'})).toBeVisible();
    if(name==='notice-business-target')await expect(page.getByRole('button',{name:/打开来源|打开原业务|查看结果|查看来源/}).first()).toBeVisible();
    if(heading)await expect(page.getByRole('heading',{name:heading,exact:true}).first()).toBeVisible({timeout:15000});
    if (['tenant-monitoring','tenant-return','project-monitoring','project-return'].includes(name)) {
      const lookup=page.getByRole('textbox',{name:/^(搜索设施、构件、测点或通道|搜索项目设施)$/});
      if (await lookup.isVisible()) {
        await expect(page.getByRole('treeitem').first()).toBeVisible({timeout:20000});
        await expect(page.getByRole('button',{name:'第一页',exact:true}).first()).toBeVisible({timeout:10000});
      }
    }
    // SPA navigation can complete after the document's first networkidle event.
    // Observe each step's own requests and stabilized accessible state.
    const settleAt=Date.now(),deadline=settleAt+16000;
    let previous,stableSince=settleAt;
    for(;;){
      const current=await page.evaluate(()=>JSON.stringify([
        document.activeElement?.tagName, document.activeElement?.id, scrollX, scrollY,
        ...[...document.querySelectorAll('main h1,[aria-selected="true"],[aria-current="page"]')]
          .filter(v=>v.getClientRects().length).map(v=>v.textContent),
      ]));
      if(current!==previous){stableSince=Date.now();previous=current;}
      if(!pending.size && Date.now()-Math.max(stableSince,lastActivity,settleAt)>=420)break;
      assert(Date.now()<deadline,'The '+name+' page did not settle after its live business requests');
      await sleep(65);
    }
    await page.evaluate(()=>document.fonts.ready);
    const state=await page.evaluate(()=>{
      const focus=document.activeElement;
      return {url:location.pathname+location.search+location.hash,heading:[...document.querySelectorAll('main h1')].map(n=>n.textContent),
        scroll:[scrollX,scrollY],focus:{tag:focus?.tagName,role:focus?.getAttribute('role'),label:focus?.getAttribute('aria-label'),id:focus?.id||''},
        selected:[...document.querySelectorAll('[aria-selected="true"], [aria-current="page"]')].filter(v=>v.getClientRects().length).map(v=>v.textContent||v.getAttribute('aria-label'))};
    });
    for(const [meaning,id] of Object.entries(inputs))state.url=state.url.replaceAll(id,'{'+meaning+'}');
    steps.push({name,...state});await page.screenshot({path:join(output,steps.length+'-'+name+'.png'),animations:'disabled',caret:'hide'});
  };
  try{
    const source='/access/accounts?q=arc_&page=1#accounts';
    await page.goto(cfg.origin+source);await capture('filtered-accounts','账号管理');
    await page.getByRole('link',{name:/^任务中心/}).first().click();await capture('account-tasks','任务中心');
    const back=page.getByRole('link',{name:/^返回工作台/});await expect(back).toHaveAttribute('href',source);
    await back.focus();await page.keyboard.press('Enter');await capture('account-return','账号管理');
    assert.equal(new URL(page.url()).pathname+new URL(page.url()).search+new URL(page.url()).hash,source);
    const switcher=page.getByRole('button',{name:/^切换工作空间：/});
    if(viewport.width===375 && !await switcher.isVisible())await page.getByRole('banner').getByRole('button',{name:'切换侧栏',exact:true}).click();
    await switcher.click();
    const picker=page.getByRole('dialog',{name:'切换工作空间',exact:true});
    await expect(picker).toBeVisible();
    const projectName=queryPG(lab,'seed',"SELECT name FROM projects WHERE id='"+s.project+"'");
    await picker.getByLabel('搜索名称或编号').fill(projectName);
    const result=picker.getByRole('button').filter({hasText:projectName}).first();
    await expect(result).toBeVisible();await capture('workspace-picker');
    if(viewport.width===375)await result.tap();else{await result.focus();await page.keyboard.press('Enter');}
    await expect(picker).toBeHidden();await page.waitForURL(u=>u.pathname.includes('/p/'+s.project));
    await capture('project-selected');
    const facilities='/t/'+s.tenant+'/p/'+s.project+'/resources/facilities';
    await page.goto(cfg.origin+facilities);
    const tree=page.getByRole('tree',{name:'设施、构件、测点与通道'});
    const selectedName=queryPG(lab,'seed',"SELECT name FROM engineering_channels WHERE id='"+s.channels.P1.id+"'");
    const searchBox=page.getByLabel('搜索设施、构件、断面、孔位、测点或通道',{exact:true});
    const [filteredPage]=await Promise.all([
      page.waitForResponse(res=>{const url=new URL(res.url());return url.pathname.endsWith('/tree')&&url.searchParams.get('q')===selectedName;},{timeout:20000}),
      searchBox.fill(selectedName),
    ]);
    assert.equal(filteredPage.status(),200,'The project resource filter must reach the published tree endpoint');
    const filtered=await filteredPage.json();
    assert(filtered.items.some(item=>item.id===s.channels.P1.id),'The channel in the frozen fixture must appear in the authorized search response');
    const channelItem=tree.getByRole('treeitem',{name:selectedName+'，业务通道',exact:true});
    await expect(channelItem).toBeVisible({timeout:15000});await capture('filtered-engineering-tree');
    await channelItem.focus();await page.keyboard.press('Enter');
    await expect(page.getByRole('heading',{name:selectedName,exact:true}).first()).toBeVisible();
    await page.waitForURL(u=>u.searchParams.get('channel')===s.channels.P1.id,{timeout:10000});await capture('channel-detail');
    await page.reload();await expect(page.getByRole('navigation',{name:'当前资源路径'})).toBeVisible();await capture('channel-reload');
    // Tenant and project context selection use their actual navigation entries.
    for(const [scope,base] of [['tenant','/t/'+s.tenant],['project','/t/'+s.tenant+'/p/'+s.project]]){
      const path=base+'/monitoring?view=history&from=2026-11-01T00%3A00%3A00Z#observations';
      await page.goto(cfg.origin+path);await capture(scope+'-monitoring','监测浏览');const from=new URL(page.url()),originalSelection=steps.at(-1).selected;
      await page.getByRole('button',{name:/^通知，/}).click();
      await page.getByRole('link',{name:'查看全部通知',exact:true}).click();await capture(scope+'-notifications','通知中心');
      await expect(back).toHaveAttribute('href',from.pathname+from.search+from.hash);await back.click();
      await expect(page).toHaveURL(from.href);await capture(scope+'-return','监测浏览');
      assert.deepEqual(steps.at(-1).selected,originalSelection,'The selected monitoring view or original page changed on return');
    }
    const task=queryPG(lab,'seed',"SELECT id FROM task_records WHERE task_type='monitoring.import.adopt' LIMIT 1");
    await page.goto(cfg.origin+'/tasks/'+task);await expect(page.getByRole('button',{name:'打开来源',exact:true}).or(page.getByRole('button',{name:'打开原业务',exact:true})).first()).toBeVisible({timeout:8000}).catch(()=>{});
    const action=page.getByRole('button',{name:/打开来源|打开原业务|查看结果|查看来源/}).first();
    await expect(action).toBeVisible();await capture('source-task');const detail=page.url();
    await action.click();await page.waitForURL(url=>url.pathname!==new URL(detail).pathname,{timeout:10000});await capture('business-target');
    await page.goBack();await page.waitForURL(detail);await capture('source-task-back');
    const notice=cfg.origin+'/notifications/'+inputs.notification;
    await page.goto(notice);
    const openNotice=page.getByRole('button',{name:'打开业务页面'});
    await expect(openNotice).toBeVisible();
    await openNotice.evaluate(element=>element.scrollIntoView({block:'center',behavior:'instant'}));
    await capture('notice-detail');
    await openNotice.click();
    await page.waitForURL(url=>url.pathname!==new URL(notice).pathname);await capture('notice-business-target');
    assert.equal(new URL(page.url()).pathname,'/tasks/'+inputs.noticeTask);const execution=new URL(page.url()).searchParams.get('execution');
    assert.equal(execution?.startsWith('"')?JSON.parse(execution):execution,'1');
    await page.goBack();await page.waitForURL(notice);await capture('notice-browser-back');
    const historical=cfg.origin+'/t/'+s.tenant+'/monitoring/analysis?'+new URLSearchParams({view:'artifacts',id:inputs.artifact,version:inputs.version2});
    await page.goto(historical);
    const artifact=page.getByRole('article',{name:'固定成果'});
    await expect(artifact.getByText('固定成果 V2',{exact:true})).toBeVisible();await capture('historical-current','既有历史成果');
    await artifact.getByRole('button',{name:'版本与比较'}).click();
    const select=artifact.getByLabel('选择固定成果版本');
    await expect(select).toHaveValue(inputs.version2);await expect(select.locator('option[value="'+inputs.version1+'"]')).toHaveCount(1);
    await select.selectOption(inputs.version1);
    await page.waitForURL(url=>url.searchParams.get('version')===inputs.version1);
    await expect(artifact.getByText('固定成果 V1',{exact:true})).toBeVisible();await capture('historical-previous','既有历史成果');
    await page.reload();await expect(artifact.getByText('固定成果 V1',{exact:true})).toBeVisible();await capture('historical-reload','既有历史成果');
    await page.goBack();await page.waitForURL(url=>url.searchParams.get('version')===inputs.version2);
    await expect(artifact.getByText('固定成果 V2',{exact:true})).toBeVisible();await capture('historical-browser-back','既有历史成果');
    return {passed:true,steps};
  }catch(e){
    await page.screenshot({path:join(output,'failure.png'),animations:'disabled',caret:'hide'}).catch(()=>{});
    return {passed:false,steps,responses:responses.slice(-40),error:e.message.replaceAll(/https?:\/\/[^\s]+/g,'[isolated URL]').replaceAll(/(?:cookie|authorization|token)[=:][^\n]*/gi,'[redacted]').slice(0,600)};
  }
  finally{await ctx.close();}
}
export async function navigation(dir,outputDirectory=dir) {
  const {output:profile,frontend}=await visualFrontend(dir,'b',outputDirectory);
  outputDirectory=profile;
  const roleSQL=visualRoleGrants(dir,'b');
  const candidate=read(join(dir,'candidate.json')).inputs.sourceSHA256;
  const snapshot=read(join(dir,'snapshot/manifest.json')).pg.seed;
  const browser=await chromium.launch(),comparisons=[];
  try{
    const inputs={a:await navigationInputs(dir,'a'),b:await navigationInputs(dir,'b')};
    const lab=read(join(dir,'private/lab.json')),account=read(join(dir,'private/accounts.json')).admin;
    const sessions={};
    for(const side of ['a','b']){
      const identity=await client(lab[side],account);
      try{sessions[side]=await identity.client.storageState();}finally{await identity.client.dispose();}
    }
    const signature=createHash('sha256').update(readFileSync(fileURLToPath(import.meta.url))).update(
      JSON.stringify([dir,candidate,roleSQL,snapshot,inputs,frontend.identity,browser.version()])).digest('hex');
    const folder=join(outputDirectory,'visual/navigation'),previousFile=join(outputDirectory,'http/workflows/navigation.json');
    if(existsSync(previousFile)&&existsSync(folder)){
      const previous=read(previousFile),oldSignature=previous.cases?.[0]?.checks?.[0]?.signature;
      if(oldSignature!==signature){
        const archive=join(outputDirectory,'visual/attempts/navigation',(oldSignature||'unversioned')+'-'+Date.now());
        mkdirSync(dirname(archive),{recursive:true,mode:0o700});renameSync(folder,archive);write(join(archive,'workflow.json'),previous);
      }
    }
    for(const viewport of [{width:1440,height:1000},{width:1024,height:768},{width:375,height:812}]){
      const saved=join(outputDirectory,'visual/navigation','pair-'+viewport.width+'.json');
      if(existsSync(saved)){
        const previous=read(saved);if(previous.signature===signature&&previous.passed){comparisons.push(previous);continue;}
        const folder=dirname(saved),archive=join(outputDirectory,'visual/attempts/navigation','failed-'+viewport.width+'-'+Date.now());
        mkdirSync(archive,{recursive:true,mode:0o700});write(join(archive,'pair.json'),previous);
        for(const side of ['a','b']){const images=join(folder,side,String(viewport.width));if(existsSync(images))renameSync(images,join(archive,side));}
      }
      const a=await walk(browser,dir,'a',viewport,inputs.a,outputDirectory,sessions.a);
      const b=await walk(browser,dir,'b',viewport,inputs.b,outputDirectory,sessions.b);
      const same=JSON.stringify(a.steps)===JSON.stringify(b.steps);
      const pair={signature,viewport,passed:same&&a.passed&&b.passed,a,b};
      comparisons.push(pair);write(saved,pair);
      if(process.env.ARC_NAVIGATION_CHECKPOINT_FILE&&existsSync(process.env.ARC_NAVIGATION_CHECKPOINT_FILE))break;
    }
  }finally{await browser.close();}
  const result={arc:'ARC-017',name:'workspace switch, tree selection, filtered account, tenant, project and source navigation',passed:comparisons.length===3&&comparisons.every(v=>v.passed),checks:comparisons};
  const frontendAtEnd=await finishVisualFrontend(dir,'b',outputDirectory,frontend);
  write(join(outputDirectory,'http/workflows/navigation.json'),{sourceDirectory:dir,candidate,snapshot,roleSQL,frontend,frontendAtEnd,cases:[result]});return result;
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const r=await navigation(runDirectory(process.argv[2]));console.log('navigation',r.passed);process.exitCode=r.passed?0:1;
}
