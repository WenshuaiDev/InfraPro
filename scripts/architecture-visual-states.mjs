import assert from 'node:assert/strict';
import { visualOutputDirectory } from './architecture-visual-frontend.mjs';
import { randomUUID } from 'node:crypto';
import { existsSync,readFileSync,readdirSync } from 'node:fs';
import { join,resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { client,digest,queryPG,read,runDirectory,sleep,write } from './architecture-lab.mjs';
import { analysisBody } from './architecture-analysis-parity.mjs';
import { navigationInputs } from './architecture-navigation.mjs';
import { viewports,visual,visualRoleGrants } from './architecture-visual.mjs';
import { visualRecordingDrivers } from './architecture-visual-recording.mjs';
import { visualAssets } from './architecture-visual-assets.mjs';
import { archiveNaturalVisualAttempt } from './architecture-visual-read-order.mjs';

const command=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'固定派生通道与历史成果视觉走查'});
async function visibleGeneration(api,base,channel,config,fixtureDirectory){
  const path=base+'/channels/'+channel.resource.id;
  const ns=value=>(BigInt(Date.parse(value))*1000000n).toString();
  const range={from_ns:ns('2026-11-01T00:00:00Z'),to_ns:ns('2026-11-01T00:10:00Z')};
  const list=()=>api(path+'/generations?'+new URLSearchParams(range));
  let current=await list();
  if(!current.items.length && channel.control_state!=='enabled'){
    const trial=await api(path+'/trials',{...command(channel.config_revision),config_id:config.id,config_hash:config.config_hash,output_range:range});
    const wait=Date.now()+180000;let done;
    do{
      const value=await api(base+'/runs/'+trial.id);
      assert(!['failed','needs_action','cancelled'].includes(value.job.state),'Visual fixture computation failed');
      if(value.job.state==='succeeded'){done=value.trial;break;}
      await sleep(200);
    }while(Date.now()<wait);
    assert.equal(done?.output_count,'10','All frozen trial inputs must be available before opening the detail');
    const preview=await api(path+'/control-preview',{expected_revision:channel.control_revision,action:'enable',auto_start_ns:range.from_ns,stop_ns:range.to_ns,trial_receipt_id:done.trial_receipt_id});
    await api(path+'/control',{...command(channel.control_revision),preview_id:preview.id,preview_hash:preview.hash});
  }
  const deadline=Date.now()+180000;
  while(!current.items.length&&Date.now()<deadline){await sleep(250);current=await list();}
  const id=current.items[0]?.id;assert(id,'The data view needs an actual immutable generation');
  // This POST is a read of a named immutable generation. Only a connection
  // reset before a response permits a second fixture read, with the same ID
  // and input. Browser captures and mutations retain their first outcome.
  const readValues=()=>api(base+'/generations/'+id+'/query',{output_range:range,limit:20});
  let values;
  try { values=await readValues(); }
  catch(error){
    if(!/ECONNRESET|socket hang up/.test(error.message))throw error;
    const evidence=join(fixtureDirectory,'visual-fixture-reads',randomUUID()+'.json');
    const receipt={operation:'fixed-generation-query',at:new Date().toISOString(),generation:id,first:'connection-reset',second:'pending'};
    write(evidence,receipt);
    try { values=await readValues();receipt.second='response-received'; }
    finally {receipt.finished=new Date().toISOString();if(!values)receipt.second='failed';write(evidence,receipt);}
  }
  assert.deepEqual(values.items.map(item=>Number(item.value)),Array.from({length:10},(_,i)=>2*i));
  return id;
}
async function protectedInputs(dir,side) {
  const lab=read(join(dir,'private/lab.json')),s=read(join(dir,'samples.json')).structure;
  const walk=read(join(dir,'manual',side+'.json'));
  const evidence=join(dir,'private',side,'visual-protected.json'),stored=existsSync(evidence)?read(evidence):{};
  const tenant='/tenants/'+s.tenant,project=tenant+'/projects/'+s.project,derived=tenant+'/monitoring/derived';
  const credentials=read(join(dir,'private/accounts.json')),admin=await client(lab[side],credentials.admin);
  const api=admin.call,{rule,input_bindings}=analysisBody(s);
  try{
    const noticePath='/me/notifications/'+walk.notification;
    const notice=await api(noticePath);
    if(!notice.read_at)await api(noticePath,{read:true,expected_revision:notice.revision},'PATCH');
    const access=project+'/resource-access/facilities/'+s.nodes.A.id;
    const structure=await api(access),point=structure.grants.find(v=>v.resource_id===s.nodes.P1.id);
    assert(point);
    if(!point.capabilities.includes('create_derived_channel')){
      const change={request_id:randomUUID(),reason:'固定派生通道与历史成果视觉走查',action:'change_grants',expected_access_revision:structure.policy.revision,expected_link_revision:structure.link.revision,update:[{grant_id:point.id,expected_revision:point.revision,capabilities:[...point.capabilities,'create_derived_channel']}]};
      const review=await api(access+'/preview',change);
      await api(access+'/changes',{...change,preview_hash:review.preview_hash});
    }
    const enableChannel=async(channel)=>{
      const path=project+'/monitoring-access',grants=await api(path);
      const old=grants.items.find(entry=>entry.effective&&entry.channel_id===channel);
      const caps=['read','history','export'];
      if(old&&caps.every(c=>old.capabilities.includes(c))&&!old.range?.from&&!old.range?.to)return;
      let revision=grants.access_revision;
      if(old){
        const revoke=path+'/'+old.id,preview=await api(revoke+'/revoke-preview',{expected_revision:old.revision});
        revision=(await api(revoke+'/revoke',{...command(old.revision),preview_hash:preview.preview_hash})).access_revision;
      }
      const resource=(await api(access)).grants.find(entry=>entry.resource_id===channel);
      assert(resource,'The selected project channel needs an authorized structural grant');
      await api(path,{...command(revision),channel_id:channel,resource_grant_id:resource.id,range:{from:null,to:null},capabilities:caps});
    };
    await enableChannel(s.channels.P1.id);
    const catalog=derived+'/metric-definitions';
    const metricID=queryPG(lab,side,"SELECT id FROM derived_metric_definitions WHERE tenant_id='"+s.tenant+"' AND code='ARC_VISUAL_DOUBLE'");
    const definition=metricID?await api(catalog+'/'+metricID):(await api(catalog,{...command(),code:'ARC_VISUAL_DOUBLE',name:'视觉核验位移双倍',rule})).definition;
    const def=catalog+'/'+definition.id;
    let version=(await api(def+'/versions')).items[0];
    if(!version){
      const checked=await api(def+'/validate',{draft_revision:definition.draft.revision});
      version=(await api(def+'/publish',{...command(definition.revision),draft_revision:definition.draft.revision,validation_hash:checked.validation_hash})).version;
    }
    stored.metric=definition.id;stored.metricVersion=version.id;
    for(const [scope,base] of [['tenant',tenant],['project',project]]){
      const list=base+'/monitoring/derived/channels',code='ARC_VISUAL_'+scope.toUpperCase();
      const existing=queryPG(lab,side,"SELECT id FROM engineering_channels WHERE tenant_id='"+s.tenant+"' AND code='"+code+"'");
      const channel=existing?await api(list+'/'+existing):(await api(list,{...command(),code,name:scope==='project'?'项目位移双倍':'租户位移双倍',owner:{kind:'point',id:s.nodes.P1.id,facility_id:s.nodes.A.id},definition_version_id:version.id,input_bindings,parameters:{}})).channel;
      const key=channel.resource.id,detail=await api(list+'/'+key);
      assert.equal(detail.resource.code,code);
      if(scope==='project')await enableChannel(key);
      const config=(await api(list+'/'+key+'/configs')).items[0];
      stored[scope+'Channel']=key;stored[scope+'Config']=config.id;
      stored[scope+'Generation']=await visibleGeneration(api,base+'/monitoring/derived',detail,config,join(dir,'private',side));
    }
    const projectVersions=await navigationInputs(dir,side,'project');
    const tenantVersions=await navigationInputs(dir,side);
    Object.assign(stored,{notification:walk.notification,noticeTask:walk.noticeTask,invitation:walk.pendingInvitation});
    for(const [scope,h] of [['tenant',tenantVersions],['project',projectVersions]])for(const k of ['artifact','version1','version2'])stored[scope+k]=h[k];
    const assets=await visualAssets(dir,side);
    Object.assign(stored,{attachment:assets.files.readable.attachment,attachmentRevision:assets.files.readable.revisionID,
      upload:assets.files.receiving.upload,display:assets.display.instance,
      layout:assets.spatial.layout.asset,layoutVersion:assets.spatial.layout.version,model:assets.spatial.model.asset,modelVersion:assets.spatial.model.version});
    write(evidence,stored);return stored;
  }finally{await admin.client.dispose();}
}

const platform='platform',tenantPage='/t/$tenantId',projectPage=tenantPage+'/p/$projectId';
const shared=[
  ['/access/accounts','/access/accounts'],['/access/roles','/access/roles'],['/access/permissions','/access/roles'],
  ['/access/audit','/audit/events'],['/notifications','/me/notifications'],['/tasks','/tasks'],
  ['/'+platform+'/device-categories','/platform/business-dictionaries/device-category'],[tenantPage+'/members','/members'],[projectPage+'/members','/members'],
  [tenantPage+'/devices','/devices'],[projectPage+'/devices','/devices'],[tenantPage+'/files','/files/attachments'],
  [tenantPage+'/spatial','/engineering/resources/tree'],[tenantPage+'/resources/facilities','/engineering/resources/tree'],
  [tenantPage+'/monitoring','/monitoring/jobs'],[projectPage+'/monitoring','/monitoring/jobs'],
  [tenantPage+'/monitoring/analysis','/analysis/artifacts'],[tenantPage+'/monitoring/derived','/monitoring/derived/channels'],
  [tenantPage+'/monitoring/imports/$batchId','/monitoring/imports'],['/history','/transitions']
];
const splitPath=(path,s)=>(path.replaceAll('$tenantId',s.tenant).replaceAll('$projectId',s.project).replaceAll('$batchId',s.batch));
const pageHeading=name=>page=>page.getByRole('heading',{name,exact:true}).first().waitFor({timeout:8000});
const button=name=>page=>page.getByRole('button',{name,exact:true}).first().waitFor({timeout:8000});

function observedEndpoint(row, suffix) {
  const paths = (row.baselineRequests ?? [])
    .filter(request => request.startsWith('GET ') && request.endsWith(' 200'))
    .map(request => new URL(request.split(' ')[1], 'http://localhost').pathname);
  return paths.find(path => path.endsWith(suffix)) ?? paths.find(path => path.includes(suffix + '/'));
}

export async function visualStates(dir,side,{resume=true,filter,outputDirectory=dir,requestOrder='natural'}={}){
  outputDirectory=visualOutputDirectory(outputDirectory);
  assert(['natural','serial'].includes(requestOrder),'Expected natural or serial visual request order');
  // Only the frozen browser's observed request paths are used to select a
  // loading/error injection point. Both pages, statuses and PNGs below always
  // come from the live pair for this selected release.
  const archives=join(outputDirectory,'visual/attempts/releases'),historical=existsSync(archives)?readdirSync(archives,{withFileTypes:true}).filter(v=>v.isDirectory()).map(v=>join(archives,v.name)):[];
  const commit=read(join(dir,'baseline.json')).commit,snapshot=read(join(dir,'snapshot/manifest.json')).pg.seed;
  const matrixPaths=[join(outputDirectory,'visual'),...historical]
    .flatMap(folder=>['self','b'].map(peer=>join(folder,'paired',peer,'matrix.json'))).filter(existsSync);
  const source=matrixPaths.map(read).find(v=>v.baseline===commit&&v.snapshot===snapshot&&shared.every(([template,suffix])=>v.rows.some(r=>r.template===template&&r.viewport.width===1440&&r.state==='default'&&observedEndpoint(r,suffix))));
  assert(source,'Run the frozen browser route inventory against this S0 before its shared-state variants');
  const samples=read(join(dir,'samples.json')),s=samples.structure;
  const inputs={a:await protectedInputs(dir,'a'),[side]:await protectedInputs(dir,side)};
  const rows=[];
  const add=(template,state,persona,makePath,verify)=>{
    const bounds={};for(const [side,ids] of Object.entries(inputs))bounds[side]={path:splitPath(makePath(ids),s),ids};
    rows.push({app:'console',template,path:bounds.a.path,persona,state,bounds,verify});
  };
  add('/notifications/$notificationId','source-detail','admin',v=>'/notifications/'+v.notification,button('打开业务页面'));
  add('/notifications/invitations/$invitationId','pending-invitation','member',v=>'/notifications/invitations/'+v.invitation,button('接受邀请并加入租户'));
  add(tenantPage+'/invitations/$invitationId','invitation-detail','admin',v=>tenantPage+'/invitations/'+v.invitation,page=>page.getByText('受限项目成员',{exact:false}).first().waitFor({timeout:7000}));
  for(const [scope,base] of [['tenant',tenantPage],['project',projectPage]]){
    add(base+'/monitoring/analysis/$artifactId','published-history','admin',v=>base+'/monitoring/analysis/'+v[scope+'artifact']+'?version='+v[scope+'version2'],pageHeading('既有历史成果'));
    add(base+'/monitoring/derived/$channelId','authorized-channel','admin',v=>base+'/monitoring/derived/'+v[scope+'Channel'],pageHeading(scope==='project'?'项目位移双倍':'租户位移双倍'));
  }
  add(projectPage+'/monitoring','project-limited-viewer','projectViewer',()=>projectPage+'/monitoring',pageHeading('监测浏览'));
  add(projectPage+'/files','image-revision','admin',v=>projectPage+'/files?attachment='+v.attachment+'&revision='+v.attachmentRevision,
    page=>page.getByRole('img',{name:/验收资料 · 桥梁断面.*安全预览/}).waitFor({timeout:15000}));
  add(projectPage+'/files','resumable-upload','admin',v=>projectPage+'/files?tab=uploads&upload='+v.upload,
    page=>page.getByText('walkthrough-resume.txt',{exact:false}).first().waitFor({timeout:8000}));
  add(projectPage+'/displays','published-instance','admin',v=>projectPage+'/displays?instance='+v.display,
    page=>page.getByRole('dialog').getByRole('heading',{name:'验收大屏 · 桥梁工程概览',exact:true}).waitFor({timeout:18000}));
  for(const [key,name] of [['layout','验收图纸 · 桥梁总体布置'],['model','验收模型 · 桥梁构件']]){
    add(projectPage+'/spatial',key+'-version','admin',v=>projectPage+'/spatial?asset='+v[key]+'&version='+v[key+'Version']+'&owner='+s.nodes.P1.id+'&owner_kind=point',
      page=>page.getByText(name,{exact:false}).first().waitFor({timeout:15000}));
  }
  const regular=source.rows.filter(v=>v.viewport.width===1440&&v.state==='default');
  for(const [template,suffix] of shared){
    const r=regular.find(v=>v.template===template);assert(r,'Unknown published template '+template);
    const endpoint=observedEndpoint(r,suffix);
    if(!endpoint)throw Error('No captured baseline GET for '+template+' and '+suffix);
    for(const state of ['pending','error'])rows.push({app:r.app,template,path:r.path,persona:r.persona,heading:null,endpoint,state});
    if(template.startsWith(tenantPage)&&!template.includes('$batchId')&&!template.endsWith('/members')){
      const empty=path=>path.replaceAll(s.tenant,samples.secondTenant).replaceAll(s.project,samples.secondProject);
      rows.push({app:r.app,template,path:empty(r.path),persona:'other',endpoint:empty(endpoint),requireEmpty:true,state:'empty-scope'});
    }
    if(template.startsWith('/access/')||template.startsWith(tenantPage)||template==='/platform/device-categories')
      rows.push({app:r.app,template,path:r.path,persona:'member',expectation:'scope-guard',state:'unauthorized-scope'});
  }
  rows.push({app:'console',template:'/invitations',path:'/invitations',persona:'admin',state:'creation-dialog',
    interact:page=>page.getByRole('button',{name:'创建邀请',exact:true}).click(),
    verify:page=>page.getByRole('dialog').getByLabel('用途备注',{exact:true}).waitFor({timeout:7000})});
  const audit=regular.find(r=>r.template==='/access/audit');
  rows.push({app:'console',template:audit.template,path:audit.path,persona:'admin',state:'second-page',interact:async page=>{
    const next=page.getByRole('button',{name:/下一页|下一批|加载更多/}).first();await next.waitFor({timeout:7000});assert(await next.isEnabled());await next.click();
  },verify:page=>page.getByRole('button',{name:/上一页|上一批/}).first().waitFor({timeout:7000})});
  for(const row of rows)if(!row.bounds){
    row.bounds=Object.fromEntries(Object.entries(inputs).map(([actor,ids])=>[actor,{path:row.path,ids}]));
  }
  const grantRevisions=Object.keys(inputs).map(actor=>read(join(dir,'private',actor,'visual-assets.json')).display.grant.revision);
  const extraSource=digest(JSON.stringify([digest(readFileSync(fileURLToPath(import.meta.url))),rows.map(row=>row.endpoint??null),grantRevisions]));
  let captureDirectory=outputDirectory,reference;
  if(requestOrder==='serial'){
    // A complete, independent observation in the ordinary browser schedule is
    // retained, including every error screenshot, before the paired replay.
    const source=join(outputDirectory,'visual/feature-states',side);
    const ordinary=read(join(source,'matrix.json'));
    assert.equal(ordinary.sourceDirectory,dir);
    assert.equal(ordinary.requestOrder,'natural');
    assert.equal(ordinary.snapshot,snapshot);
    assert.equal(ordinary.candidate,read(join(dir,'candidate.json')).inputs.sourceSHA256);
    assert.deepEqual(ordinary.roleSQL,visualRoleGrants(dir,side));
    for(const [field,hash] of Object.entries(visualRecordingDrivers()))assert.equal(ordinary[field],hash);
    assert(ordinary.complete && ordinary.rows.length===ordinary.expectedRows && ordinary.expectedRows===rows.length*viewports.length);
    const originals=new Map(ordinary.rows.map(row=>[row.key,row]));
    for(const [i,row] of rows.entries())for(const viewport of viewports){
      const observed=originals.get(row.app+'-'+viewport.width+'-'+String(i).padStart(3,'0'));
      assert(observed,'Missing natural observation for '+row.template+' '+row.state);
      assert.deepEqual([observed.template,observed.path,observed.state,observed.persona,observed.bounds],
        [row.template,row.path,row.state,row.persona,row.bounds]);
    }
    captureDirectory=join(outputDirectory,'visual/serialized-states');
    const failed=ordinary.rows.filter(row=>!row.passed);
    reference={
      identity:ordinary.identity, captured:ordinary.rows.length, failed:failed.map(row=>row.key),
      ...archiveNaturalVisualAttempt(source,join(captureDirectory,'natural',side),failed),
    };
  }
  const result=await visual(dir,side,{
    resume,extraRows:rows,extraSource:reference?{extraSource,naturalMatrixSHA256:reference.naturalMatrixSHA256}:extraSource,
    filter,requestOrder,outputDirectory:captureDirectory,
  });
  if(reference&&!filter){
    write(join(captureDirectory,side+'.json'),{
      sourceDirectory:dir,side,snapshot,candidate:result.candidate,roleSQL:result.roleSQL,reference,
      requestOrder,httpInflightPerBrowser:1,matrix:join(captureDirectory,'visual/ordered-read-states',side,'matrix.json'),
      expected:result.expectedRows,captured:result.rows.length,passed:result.passed,
    });
  }
  return result;
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const [input,side='b',requestOrder='natural',output=input]=process.argv.slice(2);
  const result=await visualStates(runDirectory(input),side,{filter:process.env.ARC_VISUAL_TEMPLATE,requestOrder,outputDirectory:runDirectory(output)});
  console.log('Protected and shared UI states',result.rows.filter(r=>r.passed).length+'/'+result.expectedRows);process.exitCode=result.passed?0:1;
}
