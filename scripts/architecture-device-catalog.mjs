import assert from 'node:assert/strict';

const items=v=>Array.isArray(v)?v:v.items;
const effect=(name,ok)=>{assert.ok(ok,name);return {[name]:true};};
export async function deviceCatalog({call,wm,tag,tenant,sample,command,stageLegacyInstallation},section) {
  const reason='隔离配对设备配置';
  const cat='/platform/business-dictionaries/device-category',projectCode='arc-'+tag;
  const ctx=await call('tenant_context','GET',tenant+'/context');
  const proj=await call('new_project','POST',tenant+'/projects',{name:'设备权限 '+tag,code:projectCode,purpose:reason,owner_tenant_membership_id:ctx.membership_id,administrator_tenant_membership_id:ctx.membership_id,client_request_id:command('new_project'),reason});
  const project=tenant+'/projects/'+proj.project_id;
  let typedRelease,category;
  if(section==='taxonomy') {
    const dict=await call('platform_dictionary','GET',cat);
    category=command('category');
    const content={...dict.draft,nodes:dict.draft.nodes.map(({id,parent_id,code,name,description,order,selectable,status})=>({id,...(parent_id?{parent_id}:{}),code,name,description,order,selectable,status}))};
    content.nodes.push({id:category,parent_id:content.sensor_root_id,code:'qa_'+tag,name:'合成分类 '+tag,description:reason,order:content.nodes.length,selectable:true,status:'active'});
    const saved=await call('dictionary_draft','PUT',cat+'/draft',{...wm(),expected_revision:dict.draft_revision,content});
    const pub={...wm(),expected_revision:saved.revision},preview=await call('dictionary_publish_preview','POST',cat+'/publish-preview',pub);
    const next=await call('dictionary_publish','POST',cat+'/publish',{...pub,preview_hash:preview.preview_hash},v=>effect('new_release',!!v.release_id&&v.release_id!==dict.current_release_id));typedRelease=next.release_id;
    for(const [key,root]of [['platform','/platform'],['tenant',tenant],['project',project]]){
      const p=root+'/business-dictionaries/device-category';
      await call(key+'_categories','GET',p+'/categories',null,v=>effect('new_category_visible',JSON.stringify(v).includes(category)));
      await call(key+'_releases','GET',p+'/releases',null,v=>effect('new_release_visible',JSON.stringify(v).includes(typedRelease)));
    }
  }
  const slots=[{key:'fixed',name:'固定位移',mode:'fixed',order:0,metric_version_id:sample.metric,unit_version_id:sample.unit,timestamp_meaning:'observation_time'},
    {key:'config',name:'可配置量',mode:'configurable',order:1,allowed_shapes:['decimal']}];
  const products={};
  for(const [scope,root]of [['platform','/platform'],['tenant',tenant]]){
    const path=root+'/device-definitions',label=scope+'_',makerPath=path+'/manufacturers',productPath=path+'/products';
    const input={...wm(),code:label+tag,name:'厂家 '+label+tag};
    let maker=await call(label+'manufacturer_create','POST',makerPath,input);
    const mp=makerPath+'/'+maker.id;
    async function change(name,route,previewPath,action,method,data,extra={}) {
      const p=await call(label+name+'_preview','POST',previewPath,{action,...data,...extra},v=>effect('allowed',v.can_submit));
      return call(label+name,method,route,{...data,preview_hash:p.preview_hash});
    }
    await call(label+'maker_receipt','GET',root+'/device-requests/'+input.request_id,null,v=>effect('receipt_matches',v.id===maker.id));
    if(section==='products') {
      maker=await change('manufacturer_update',mp,mp+'/mutation-preview','update','PATCH',{...wm(),expected_revision:maker.revision,description:reason});
      for(const status of ['archived','active'])maker=await change('manufacturer_'+status,mp+'/state',mp+'/mutation-preview','state','POST',{...wm(),expected_revision:maker.revision,target_status:status});
      await call(label+'manufacturer_list','GET',makerPath+'?q='+input.code,null,v=>effect('manufacturer_found',items(v).some(x=>x.id===maker.id)));
      await call(label+'manufacturer_detail','GET',mp,null,v=>effect('active_maker',v.status==='active'));
      const spare=await call(label+'spare_maker','POST',makerPath,{...wm(),code:'temp_'+tag,name:'无产品厂商'});
      const sparePath=makerPath+'/'+spare.id;
      await change('delete_maker',sparePath+'/delete',sparePath+'/mutation-preview','delete','POST',{...wm(),expected_revision:spare.revision});
    }
    const create={...wm(),manufacturer_id:maker.id,code:label+tag,name:'设备模型 '+label+tag,model:'Q2',output_slots:slots};
    const draft=await call(label+'product_create','POST',productPath,create),pp=productPath+'/'+draft.id;
    let draftRev=draft.revision;
    if(section==='products'){
      const initial=await call(label+'product_draft','GET',pp);
      const saved=await call(label+'save_product','PUT',pp+'/draft',{...wm(),expected_draft_revision:draftRev,content:{...initial.draft.content,description:reason}});draftRev=saved.revision;
      await call(label+'validate_product','POST',pp+'/validate',{},v=>effect('valid_slots',v.valid));
    }
    const first=await call(label+'publish_product','POST',pp+'/publish',{...wm(),expected_draft_revision:draftRev,classification:{mode:'unclassified'}},v=>effect('published',!!v.version_id));
    const vp=pp+'/versions/'+first.version_id;
    const version=await call(label+'product_version','GET',vp);
    products[scope]={id:draft.id,version:first.version_id,maker:maker.id,root:vp};
    if(section==='taxonomy') {
      await setCategory(root,'product_version',first.version_id,scope+'_product');continue;
    }
    if(section!=='products')continue;
    await call(label+'product_list','GET',productPath+'?q='+create.code,null,v=>effect('product_found',items(v).some(p=>p.id===draft.id)));
    const det=await call(label+'product_read','GET',pp);
    const next=await call(label+'new_product_draft','POST',pp+'/drafts',{...wm(),expected_revision:det.revision,version_id:first.version_id});
    await call(label+'discard_product_draft','POST',pp+'/draft/delete',{...wm(),expected_draft_revision:next.revision});
    let versionRev=version.revision;
    for(const enabled of [false,true]){
      const state=await change('version_'+enabled,vp+'/state',pp+'/mutation-preview','state','POST',{...wm(),expected_revision:versionRev,enabled},{version_id:first.version_id});
      const current=await call(label+'version_state_'+enabled,'GET',vp,null,v=>effect('effective_adoption_state',v.enabled===enabled));versionRev=current.revision;assert.ok(state.changed);
    }
    const duplicate=await call(label+'copy_product','POST',productPath+'/copy',{...wm(),source_version_id:first.version_id,manufacturer_id:maker.id,code:'copy_'+tag});
    const cp=productPath+'/'+duplicate.id;
    await change('delete_unused_product',cp+'/delete',cp+'/mutation-preview','delete','POST',{...wm(),expected_revision:duplicate.revision});
    await call(label+'product_changes','GET',path+'/changes?product_id='+draft.id,null,v=>effect('has_product_history',items(v).length>0));
    await call(label+'parsing_options','GET',vp+'/parsing-configs');
    await call(label+'parsing_default','GET',vp+'/parsing-default');
    const inherited=tenant+'/device-catalog/products';
    await call(label+'inherited_products','GET',inherited+'?q='+create.code,null,v=>effect('public_catalog_respects_owner',items(v).some(x=>x.id===draft.id)===(scope==='platform')));
    if(scope==='tenant') continue;
    await call(label+'inherited_versions','GET',inherited+'/'+draft.id+'/versions');
    await call(label+'inherited_version','GET',inherited+'/'+draft.id+'/versions/'+first.version_id);
    for(const id of ['parsing-configs','parsing-default'])await call(label+'inherited_'+id,'GET',inherited+'/'+draft.id+'/versions/'+first.version_id+'/'+id);
  }
  await call('catalog_manufacturers','GET',tenant+'/device-catalog/manufacturers?q='+tag);
  async function setCategory(root,kind,id,label) {
    const path=root+'/device-classifications',ref=path+'/'+kind+'/'+id;
    const current=await call(label+'_classification','GET',ref);
    const {expected_classification_revision,expected_entity_revision}=current;
    const input={...wm(),targets:[{kind,id,expected_classification_revision,expected_entity_revision}],selection:{mode:'classify',dictionary_release_id:typedRelease,category_id:category}};
    const preview=await call(label+'_classify_preview','POST',path+'/preview',input);
    await call(label+'_classify','POST',path+'/apply',{...input,preview_id:preview.preview_id,preview_hash:preview.preview_hash});
    await call(label+'_classification_effect','GET',ref,null,v=>effect('assigned_category',JSON.stringify(v).includes(category)));
    await call(label+'_class_history','GET',ref+'/history',null,v=>effect('category_history',items(v).length>0));
    await call(label+'_class_targets','GET',path+'?kind='+kind);
  }
  if(section==='products')return;
  const reg={...wm(),code:'D-'+tag,name:'设备 '+tag,product_version_id:products.tenant.version,parsing_adoption:{mode:'unconfigured'}};
  const devices=tenant+'/devices',a=await call('device_register','POST',devices,reg),dp=devices+'/'+a.id;
  if(section==='taxonomy'){await setCategory(tenant,'device',a.id,'device');return;}
  await deviceLinks({call,wm,tag,tenant,sample,command,dp,products,devices,project,reg,device:a,stageLegacyInstallation});
}

async function deviceLinks({call,wm,tag,tenant,sample,command,dp,products,devices,project,reg,device,stageLegacyInstallation}) {
  const probe=(name,path)=>call(name,'GET',path);
  const state=(name)=>probe(name,dp);
  async function change(label,action,method,path,input,extra={}) {
    const p=await call(label+'_preview','POST',dp+'/mutation-preview',{action,...input,...extra},v=>effect('allowed',v.can_submit));
    return call(label,method,path,{...input,preview_hash:p.preview_hash});
  }
  const details=await call('registered_product_version','GET',dp,null,v=>effect('pinned_product',v.product_version_id===products.tenant.version&&v.product.number===1));
  await call('registration_receipt','GET',tenant+'/device-requests/'+reg.request_id,null,v=>effect('single_device',v.id===device.id));
  await change('device_description','update','PATCH',dp,{...wm(),expected_revision:details.revision,description:'来源核对'});
  for(const path of ['/changes','/ingestion','/parsing','/parsing/configs','/parsing/history','/legacy-sections','/installation-targets?kind=facility'])
    await probe('device_'+path.slice(1),dp+path);
  const area=()=>({target:{kind:'location'},location_description:'本次安装 '+tag});
  let d=await state('before_install');
  const start='2025-04-01T00:00:00Z',moved='2025-04-02T00:00:00Z',end='2025-04-03T00:00:00Z';
  const install=await change('install','installation_create','POST',dp+'/installations',{...wm(),...area(),expected_installation_revision:d.installation_revision,installed_at:start});
  const history=()=>dp+'/installations';
  let rows=items(await probe('installed',history()));
  let installed=rows.find(i=>i.id===install.installation_id);assert.ok(installed);
  const evidence=stageLegacyInstallation(device.id,installed.id),legacyPath=dp+'/legacy-sections';
  const evidenceRow=async label=>items(await probe(label,legacyPath)).find(v=>v.id===evidence);
  const original=await evidenceRow('legacy_pending');
  assert.equal(original.status,'pending');
  const decision={...wm(),status:'historical'};
  const oldPreview=await call('historical_installation_evidence_preview','POST',legacyPath+'/'+evidence+'/preview',decision,v=>effect('historical_decision_permitted',v.can_submit));
  await call('record_installation_evidence_decision','POST',legacyPath+'/'+evidence+'/resolve',{...decision,preview_hash:oldPreview.preview_hash},v=>effect('affected_installation',v.changed&&v.installation_id===installed.id));
  const resolved=await evidenceRow('legacy_historical');
  assert.equal(resolved.status,'historical');assert.equal(resolved.original,original.original);
  assert.equal(resolved.source_revision,original.source_revision);
  const corrected=await change('correct_installation','installation_correct','PATCH',history()+'/'+installed.id,{...wm(),...area(),location_description:'补正位置',installed_at:start,uninstalled_at:null,
    expected_revision:installed.revision,expected_installation_revision:install.installation_revision},{installation_id:installed.id});
  rows=items(await probe('corrected',history()));installed=rows.find(i=>i.id===installed.id);
  const move=await change('move_installation','installation_relocate','POST',history()+'/'+installed.id+'/relocate',{...wm(),...area(),location_description:'迁入位置',moved_at:moved,
    expected_revision:installed.revision,expected_installation_revision:corrected.installation_revision},{installation_id:installed.id});
  rows=items(await probe('moved',history()));installed=rows.find(i=>!i.uninstalled_at);
  await change('end_installation','installation_end','POST',history()+'/'+installed.id+'/end',{...wm(),expected_revision:installed.revision,expected_installation_revision:move.installation_revision,uninstalled_at:end},{installation_id:installed.id});
  await call('retained_installations','GET',history(),null,v=>effect('two_closed_installations',items(v).length===2&&items(v).every(x=>x.uninstalled_at)));
  const slots=()=>dp+'/outputs',outputList=async name=>items(await probe(name,slots()));
  const s=await outputList('unconfigured_slots'),fixed=s.find(x=>x.current)?.current;assert.ok(fixed);
  const semantics={name:'位移 '+tag,metric_version_id:sample.metric,unit_version_id:sample.unit,timestamp_meaning:'observation_time'};
  d=await state('before_configure');
  const configured=await call('configure_output','POST',slots(),{...wm(),...semantics,slot_key:'config',expected_output_revision:d.output_revision});
  const outRoot=slots()+'/'+configured.output_id;
  let output=await probe('configured_output',outRoot);
  await call('edit_output','PATCH',outRoot,{...wm(),expected_revision:output.revision,name:'新的输出名'});
  output=await probe('edited_output',outRoot);d=await state('before_replace');
  const replacement=await change('replace_output','output_replace','POST',outRoot+'/replace',{...wm(),...semantics,expected_revision:output.revision,expected_output_revision:d.output_revision},{output_id:output.id});
  assert.notEqual(replacement.output_id,output.id);
  await probe('output_generations',dp+'/output-generations?slot_key=config');
  const newOut=slots()+'/'+replacement.output_id;
  output=await probe('replacement_generation',newOut);d=await state('before_end_generation');
  await change('end_output','output_end','POST',newOut+'/end',{...wm(),expected_revision:output.revision,expected_output_revision:d.output_revision},{output_id:output.id});
  const second=await call('register_second_device','POST',devices,{...reg,...wm(),code:'E-'+tag});
  const secondPath=devices+'/'+second.id,fixed2=items(await probe('second_outputs',secondPath+'/outputs')).find(x=>x.current).current;
  const resources=tenant+'/engineering/resources/facilities',facility=await call('source_facility','POST',resources,{request_id:command('facility'),code:'F-'+tag,name:'设备来源试验',type_version_id:sample.types.A.version_id,values:{}});
  const fp=resources+'/'+facility.id,point=await call('source_point','POST',fp+'/points',{request_id:command('point'),code:'P-'+tag,name:'来源点',parent_ref:{kind:'facility',id:facility.id},expected_structure_revision:facility.structure_revision});
  const root=fp+'/points/'+point.id+'/channels';
  const chan=async n=>call('source_channel_'+n,'POST',root,{request_id:command('channel_'+n),code:'C'+n,name:'位移 '+n,metric_version_id:sample.metric,unit_version_id:sample.unit,timestamp_meaning:'observation_time'});
  const chan1=await chan(1),chan2=await chan(2),src=root+'/'+chan1.id+'/source';
  const inverse=id=>tenant+'/device-outputs/'+id;
  for(const [name,path]of [['source_candidates',src+'/candidates'],['reverse_candidates',inverse(fixed.id)+'/channel-candidates']])await probe(name,path);
  async function sourceInput(label,path,action,newEnd) {
    const v=await probe(label+'_before',path),input={...wm(),expected_source_revision:v.source_revision,expected_channel_revision:v.channel_revision};
    const scoped=path.startsWith(project+'/');
    const outputURL=o=>scoped?project+'/device-outputs/'+o.id:devices+'/'+o.device_id+'/outputs/'+o.id;
    if(scoped)Object.assign(input,{expected_resource_access_revision:v.resource_access_revision,expected_link_revision:v.link_revision,expected_device_access_revision:v.device_access_revision});
    if(v.source.configured){
      const old=v.source.output;
      const current=await probe(label+'_old_output',outputURL(old));
      Object.assign(input,{expected_binding_id:v.source.id,expected_binding_revision:v.source.revision,expected_old_output_revision:current.revision,expected_old_device_revision:current.device_revision});
    }
    if(newEnd){
      const current=await probe(label+'_new_output',outputURL(newEnd));
      Object.assign(input,{output_id:current.id,expected_new_output_revision:current.revision,expected_new_device_revision:current.device_revision});
    }
    return input;
  }
  async function source(label,path,action,newEnd){
    const input=await sourceInput(label,path,action,newEnd);
    const preview=await call(label+'_preview','POST',path+'/preview',{action,...input},v=>effect('source_transition_allowed',v.can_submit));
    return call(label,'POST',path+'/'+action,{...input,preview_hash:preview.preview_hash});
  }
  await source('bind_source',src,'bind',fixed);
  const conflictSrc=root+'/'+chan2.id+'/source',bad=await sourceInput('occupied',conflictSrc,'bind',fixed);
  const conflict=await call('occupied_preview','POST',conflictSrc+'/preview',{action:'bind',...bad},()=>({}),[200,409]);
  assert.ok(conflict.can_submit===false||conflict.code);
  await call('channel_unchanged','GET',conflictSrc,null,v=>effect('conflicting_channel_unbound',!v.source.configured));
  await source('switch_source',src,'switch',fixed2);
  await call('switched_channel','GET',src,null,v=>effect('second_output_active',v.source.output.id===fixed2.id));
  await call('old_output_history','GET',inverse(fixed.id)+'/source-history',null,v=>effect('old_binding_ended',JSON.stringify(v).includes(chan1.id)));
  await call('new_output_history','GET',inverse(fixed2.id)+'/source-history',null,v=>effect('reverse_reference',JSON.stringify(v).includes(chan1.id)));
  await source('unbind_source',src,'unbind',null);
  await call('ended_sources','GET',src+'/history',null,v=>effect('two_bindings_preserved',items(v).length===2));
  const grants=project+'/device-access';
  async function grant(label,add,ends=[],update=[]){
    const current=await probe(label+'_current',grants);
    const input={...wm(),expected_device_access_revision:current.device_access_revision,expected_project_revision:current.project_revision,add,update,end:ends};
    const preview=await call(label+'_preview','POST',grants+'/preview',input,v=>effect('grant_change_allowed',v.can_submit));
    return call(label,'POST',grants+'/changes',{...input,preview_hash:preview.preview_hash});
  }
  await probe('grant_candidates',grants+'/candidates?kind=device');
  await grant('grant_metadata',[{kind:'device',target_id:device.id,use_source:false},{kind:'output',target_id:fixed.id,use_source:false}]);
  for(const route of ['/devices','/devices/'+device.id,'/devices/'+device.id+'/changes','/devices/'+device.id+'/installations','/devices/'+device.id+'/ingestion','/devices/'+device.id+'/parsing','/devices/'+device.id+'/parsing/history','/device-outputs','/device-outputs/'+fixed.id,'/device-outputs/'+fixed.id+'/changes','/device-outputs/'+fixed.id+'/source-history'])
    await probe('project_read_'+route.replace(/[a-f0-9-]{36}/g,'id'),project+route);
  await call('project_without_numeric_grants','GET',project+'/monitoring/points',null,v=>effect('no_measurements_granted',!JSON.stringify(v).includes(chan1.id)));
  const active=await probe('metadata_grants',grants);
  const mapped=items(active).find(x=>x.kind==='output');
  const candidates=project+'/device-outputs/'+fixed.id+'/channel-candidates';
  await call('metadata_only_reverse_candidates_hidden','GET',candidates,null,v=>effect('source_use_not_granted',!!v.code),[403,404]);
  await grant('grant_source_use',[],[],[{grant_id:mapped.id,expected_revision:mapped.revision,use_source:true}]);
  await probe('source_grant_authorizes_candidates',candidates);
  const facilityGrant=project+'/resource-access/facilities/'+facility.id;
  const resource=await probe('attach_project_facility',facilityGrant);
  const link={...wm(),action:'create_link',expected_access_revision:resource.policy.revision,add:[
    {kind:'facility',capabilities:['read']},{kind:'channel',channel_id:chan1.id,capabilities:['read','source_manage']}]};
  const preview=await call('project_source_resource_preview','POST',facilityGrant+'/preview',link);
  await call('project_source_resource_grant','POST',facilityGrant+'/changes',{...link,preview_hash:preview.preview_hash});
  const projectSource=project+src.slice(tenant.length),projectBind=await sourceInput('project_bind',projectSource,'bind',fixed);
  const bindPreview=await call('project_bind_preview','POST',projectSource+'/preview',{action:'bind',...projectBind},v=>effect('both_grants_allow_source',v.can_submit));
  const bound=await call('project_source_binding','POST',projectSource+'/bind',{...projectBind,preview_hash:bindPreview.preview_hash});
  await call('project_binding_receipt','GET',project+'/device-requests/'+projectBind.request_id,null,v=>effect('same_source_binding',v.binding_id===bound.binding_id&&!!v.binding_id));
  await grant('permit_project_replacement',[{kind:'device',target_id:second.id,use_source:false},{kind:'output',target_id:fixed2.id,use_source:true}]);
  await call('project_qualified_source_candidates','GET',projectSource+'/candidates',null,v=>effect('authorized_replacement_visible',JSON.stringify(v).includes(fixed2.id)));
  await source('switch_project_source',projectSource,'switch',fixed2);
  await call('project_replacement_source','GET',projectSource,null,v=>effect('qualified_replacement_applied',v.source.output.id===fixed2.id));
  await source('project_unbind',projectSource,'unbind',null);
  const permitted=await probe('after_source_grant',grants);
  await grant('withdraw_metadata',[],items(permitted).map(x=>({grant_id:x.id,expected_revision:x.revision})));
  await probe('project_grant_log',grants+'/changes');
  await call('revoked_project_device','GET',project+'/devices/'+device.id,null,v=>effect('metadata_hidden',!!v.code),[403,404]);
  d=await state('before_disable');const disabled=await change('disable_device','state','POST',dp+'/state',{...wm(),expected_revision:d.revision,status:'disabled'});
  await change('enable_device','state','POST',dp+'/state',{...wm(),expected_revision:disabled.revision,status:'enabled'});
  const spare=await call('unused_device','POST',devices,{...reg,...wm(),code:'R-'+tag}),sparePath=devices+'/'+spare.id;
  const deletion={...wm(),expected_revision:spare.revision},deletePreview=await call('delete_spare_preview','POST',sparePath+'/mutation-preview',{action:'delete',...deletion},v=>effect('no_history_references',v.can_submit));
  await call('delete_spare_device','POST',sparePath+'/delete',{...deletion,preview_hash:deletePreview.preview_hash},v=>effect('device_removed',v.changed));
}
