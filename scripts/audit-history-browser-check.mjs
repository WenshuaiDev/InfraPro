import {chromium,expect as baseExpect} from '@playwright/test';
import {readFileSync,mkdirSync,writeFileSync,rmSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {auditClient} from './fixtures/audit-acceptance.mjs';

// Uses the same public-API business samples kept alive for manual acceptance.
const directory=resolve('.artifacts/audit/lab'),out=resolve('.artifacts/audit/history-browser');
const read=name=>JSON.parse(readFileSync(join(directory,name+'.json'),'utf8'));
const state=read('audit-acceptance'),business=read('business-acceptance'),session=read('session');
if(!business.complete||session.state!=='running'||!session.business)throw Error('Start pnpm audit:demo and wait for the complete business fixture.');
const origin=session.origin,expect=baseExpect.configure({timeout:20000}),checks=[],errors=[],focused=new Set(process.argv.slice(2));
mkdirSync(out,{recursive:true});if(!focused.size)rmSync(join(out,'result.json'),{force:true});
const source=(kind,path,id,reference=false,label='操作历史',extra={})=>({kind,path,id,reference,label,...extra});
const param=(url,key)=>{const value=new URL(url,origin).searchParams.get(key);try{return JSON.parse(value);}catch{return value;}};
let browser,page,owner,derived;
try{
 owner=await auditClient(origin,state.accounts.owner);derived=await auditClient(origin,business.derivedLogin);
 const t=state.tenants[0],p=state.projects[0][0],base='/t/'+t,pbase=base+'/p/'+p;
 const roles=await owner.call(`/tenants/${t}/roles`),invitations=await owner.call(`/tenants/${t}/member-invitations`),definitions=await owner.call('/platform/engineering/units');
 const role=roles.items.find(r=>r.name==='审计验收 tenant'),invitation=invitations.items.find(i=>i.target.id===state.accounts.tenant.id)??invitations.items[0],definition=definitions.items[0];
 if(!role||!invitation||!definition)throw Error('Governance sample selection failed');
 const n=business.monitoringDisplay,nt='/t/'+n.tenants[0],np=nt+'/p/'+n.projects[0],display=await owner.call(`/tenants/${n.tenants[0]}/projects/${n.projects[0]}/displays/instances/${n.instance}`);
 const ownerCases=[
  source('account','/access/accounts?account='+state.accounts.tenant.id,state.accounts.tenant.id),
  source('role','/access/roles?role='+state.roles[4].id,state.roles[4].id),
  source('platform_tenant','/platform/tenants/'+t,t),source('tenant',base+'/profile',t),
  source('tenant_member',base+'/members/'+state.members.tenant,state.members.tenant),
  source('tenant_role',base+'/roles/'+role.id,role.id,false,'操作历史',{tab:'操作记录'}),source('tenant_invitation',base+'/invitations/'+invitation.id,invitation.id),
  source('project',pbase+'/profile',p),source('project_member',pbase+'/members/'+state.projectMember,state.projectMember),
  source('project_role',pbase+'/roles/'+state.projectRole,state.projectRole),
  source('engineering_definition','/platform/engineering/units?definition='+definition.id,definition.id),
  source('monitoring_batch',nt+'/monitoring/imports/'+n.success.batch,n.success.batch),source('monitoring_job',nt+'/monitoring/jobs?job='+n.success.job,n.success.job,true),
  source('display_instance',np+'/displays?instance='+n.instance,n.instance),
  source('display_package',np+'/displays?instance='+n.instance+'&package='+display.packages[0].id,n.instance,false,'包管理历史',{ref:display.packages[0].id}),
 ];
 for(const file of business.files){const project=file.base.includes('/projects/'),path=(project?pbase:base)+'/files?attachment='+file.attachment+'&revision='+file.revision;
  if(!ownerCases.some(c=>c.kind==='file_attachment_'+project))ownerCases.push(source('file_attachment_'+project,path,file.attachment));
  ownerCases.push(source('file_revision_'+project+'_'+file.generation,path,file.attachment,false,'此修订操作历史',{ref:file.revision}));
 }
 const s=business.spatial,d=business.derived,db='/t/'+d.tenant,dp='/tenants/'+d.tenant,facility=db+'/resources/facilities/'+d.facility;
 const products=await derived.call(dp+'/device-definitions/products'),manufacturers=await derived.call(dp+'/device-definitions/manufacturers'),runs=await derived.call(dp+'/monitoring/derived/runs');
 const product=products.items.find(v=>v.code==='SPATIAL-SENSOR'),maker=manufacturers.items.find(v=>v.code==='SPATIAL-MAKER'),run=runs.items.find(v=>v.mode!=='automatic')??runs.items[0],asset=s.assets['BRIDGE-PDF'],version=asset.versions[0],artifact=s.expected.tenant_artifact;
 if(!product||!maker||!run||!artifact)throw Error('Business sample selection failed');
 const derivedCases=[
  source('facility',facility,d.facility),source('component',facility+'?component='+s.component,s.component),
  source('point',facility+'?point='+d.point,d.point),source('channel',facility+'?point='+d.point+'&channel='+d.channels.A.id,d.channels.A.id,true),
  source('manufacturer',db+'/device-products?manufacturer='+maker.id,maker.id),source('product',db+'/device-products/'+product.id,product.id),
  source('device',db+'/devices/'+s.devices[1].id,s.devices[1].id,true),
  source('device_access',db+'/projects/'+s.projects.A.id+'/device-access',s.projects.A.id),
  source('resource_access',db+'/resource-access/'+s.projects.A.id,s.projects.A.id,true),
  source('resource_access_facility',db+'/resource-access/'+s.projects.A.id+'/facilities/'+d.facility,s.projects.A.id,true,'操作历史',{targetFacility:d.facility}),
  source('derived_channel',db+'/monitoring/derived/'+d.channels.A_MINUS_B.id,d.channels.A_MINUS_B.id),
  source('derived_run',db+'/monitoring/derived-runs?id='+run.id,run.id),
  source('analysis_artifact',db+'/monitoring/analysis/'+artifact.artifact_id,artifact.artifact_id),
  source('spatial_asset',db+'/spatial?mode=manage&asset='+asset.id,asset.id,true),
  source('spatial_version',db+'/spatial?mode=manage&asset='+asset.id+'&version='+version.id,version.id,true,'此版本操作历史'),
 ];
 browser=await chromium.launch({headless:true});
 for(const [login,cases]of [[state.accounts.owner,ownerCases],[business.derivedLogin,derivedCases]]){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  if(focused.size)page.on('framenavigated',frame=>{if(frame===page.mainFrame())console.log('Navigation: '+frame.url());});
  await page.goto(origin+'/login');await page.getByLabel('用户名',{exact:true}).fill(login.username);await page.getByLabel('密码',{exact:true}).fill(login.password);await page.getByRole('button',{name:'登录',exact:true}).click();await page.waitForURL(u=>!u.pathname.includes('/login'));
  for(const entry of cases){
   if(focused.size&&!focused.has(entry.kind))continue;
   try{
   console.log('History entry: '+entry.kind);await page.goto(origin+entry.path);
   if(entry.tab)await page.getByRole('tab',{name:entry.tab,exact:true}).click();
   const links=page.getByRole('link',{name:entry.label,exact:true});await expect(links.first()).toBeVisible();
   const expectedKey=entry.reference?'reference_id':'object_id',hrefs=await links.evaluateAll(nodes=>nodes.map(n=>n.getAttribute('href'))),index=hrefs.findIndex(href=>param(href,expectedKey)===entry.id&&(!entry.ref||param(href,'reference_id')===entry.ref));
   if(index<0)throw Error(entry.kind+': no history link has its exact immutable object filter');
   await links.nth(index).click();await page.waitForURL(u=>u.pathname.endsWith('/audit'));
   if(param(page.url(),expectedKey)!==entry.id||entry.ref&&param(page.url(),'reference_id')!==entry.ref)throw Error(entry.kind+': history navigation changed identity');
   const records=page.locator('[data-audit-record]');await expect(records.first()).toBeVisible();await page.reload();await expect(records.first()).toBeVisible();
   const auditBase=new URL(page.url()).pathname.replace('/access/audit','/audit').replace('/t/','/tenants/').replace('/p/','/projects/'),connection=login===state.accounts.owner?owner:derived;
   let event,available,targetIndex=-1,rowIndex=0;
   for(;rowIndex<Math.min(await records.count(),20);rowIndex++){
    event=await records.nth(rowIndex).getAttribute('data-audit-record');const detail=await connection.call(auditBase+'/events/'+event);available=detail.links.filter(l=>l.available&&l.target?.available);
    targetIndex=available.findIndex(l=>(!entry.targetFacility||l.target.parameters?.facility===entry.targetFacility)&&(entry.ref?(l.target.id===entry.ref||l.target.id===entry.id&&l.target.revision===entry.ref):(l.target.id===entry.id||Object.values(l.target.parameters??{}).includes(entry.id)||entry.kind==='monitoring_job'&&l.target.kind==='task')));
    if(targetIndex>=0)break;
   }
   if(targetIndex<0)throw Error(entry.kind+': no visible fact has its currently authorized fixed source target');
   await records.nth(rowIndex).click();await expect(page.getByRole('dialog').getByText('名称来源',{exact:true})).toBeVisible();
   const sourceSection=page.getByRole('dialog').locator('section').filter({has:page.getByRole('heading',{name:'关联来源',exact:true})});
   await sourceSection.getByRole('button',{name:'打开关联来源',exact:true}).nth(targetIndex).click();await page.waitForURL(u=>!u.pathname.endsWith('/audit'));
   if(entry.tab)await page.getByRole('tab',{name:entry.tab,exact:true}).click();
   const returned=page.getByRole('link',{name:entry.label,exact:true});await expect(returned.first()).toBeVisible();
   const returnedHrefs=await returned.evaluateAll(nodes=>nodes.map(n=>n.getAttribute('href')));
   const opened=available[targetIndex].target,returnKey=opened.kind==='task'?'task_id':expectedKey;
   if(!returnedHrefs.some(href=>param(href,returnKey)===entry.id&&(!entry.ref||param(href,'reference_id')===entry.ref)))throw Error(entry.kind+': source target changed the fixed object or revision');
   if(opened.kind==='task'&&String(param(page.url(),'execution'))!==opened.generation)throw Error(entry.kind+': task target changed its fixed generation');
   if(['monitoring_job','resource_access','spatial_version'].includes(entry.kind))await page.screenshot({path:join(out,entry.kind+'.png'),fullPage:true});
   checks.push({kind:entry.kind,source:entry.path,filter:{[expectedKey]:entry.id,...(entry.ref?{reference_id:entry.ref}:{})},event,target:available[targetIndex].target.kind,passed:true});
   }catch(error){const message=entry.kind+': '+String(error);errors.push(message);checks.push({kind:entry.kind,source:entry.path,passed:false,error:String(error)});console.error(message);await page.screenshot({path:join(out,'failure-'+entry.kind+'.png'),fullPage:true}).catch(()=>{});writeFileSync(join(out,'failure-'+entry.kind+'.html'),await page.content().catch(()=>''));}
   writeFileSync(join(out,focused.size?'focused-progress.json':'progress.json'),JSON.stringify({passed:false,checks,updatedAt:new Date().toISOString()},null,2));
  }
  await context.close();page=undefined;
 }
 if(errors.length)throw Error(errors.join('\n'));
 writeFileSync(join(out,focused.size?'focused-result.json':'result.json'),JSON.stringify({passed:true,synthetic:true,focused:!!focused.size,checks,completedAt:new Date().toISOString()},null,2));console.log('All selected source-history journeys passed: '+checks.length);
}catch(e){if(page){await page.screenshot({path:join(out,'failure.png'),fullPage:true}).catch(()=>{});writeFileSync(join(out,'failure.html'),await page.content().catch(()=>''));}writeFileSync(join(out,'failure.txt'),String(e));console.error(e);process.exitCode=1;}
finally{await owner?.client.dispose();await derived?.client.dispose();await browser?.close();}
