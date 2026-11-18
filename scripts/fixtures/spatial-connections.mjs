import { randomUUID } from 'node:crypto';
// Adds the direct-source timeline and a component-owned calculated channel
// through the same commands used by the application.
export async function seedSpatialConnections({call,wait,base,prior,state,component,save}) {
 const meta=()=>({request_id:randomUUID(),reason:'合成空间与设备联动验收'}),wm=r=>({...meta(),expected_revision:r});
 const path=`${base}/engineering/resources/facilities/${prior.facility}/points/${prior.point}/channels/${prior.channels.A.id}`,channel=await call(path);
 if(!state.devices?.length){
 const maker=await call(base+'/device-definitions/manufacturers',{...meta(),code:'SPATIAL-MAKER',name:'空间示例厂家 · 自制资料'});
 const product=await call(base+'/device-definitions/products',{...meta(),manufacturer_id:maker.id,code:'SPATIAL-SENSOR',name:'合成位移采集仪',model:'SYNTHETIC-01',output_slots:[{key:'displacement',name:'位移输出',mode:'fixed',metric_version_id:channel.metric_version_id,unit_version_id:channel.unit_version_id,timestamp_meaning:'observation_time'}]});
 const published=await call(base+'/device-definitions/products/'+product.id+'/publish',{...meta(),expected_draft_revision:product.revision,classification:{mode:'unclassified'}});
 state.devices=[];
 for(const [code,name]of [['OLD','历史示例采集仪'],['NEW','当前示例采集仪']]){
  const created=await call(base+'/devices',{...meta(),code:'SPATIAL-'+code,name,product_version_id:published.version_id,serial_number:'PRIVATE-SYNTHETIC-'+code,description:'租户设备详情，不向仅有输出授权的项目展示'});
  await call(base+'/devices/'+created.id+'/installations',{...meta(),expected_installation_revision:'0',target:{kind:'point',id:prior.point},location_description:'合成安装位置；与通道来源独立'});
  const device=await call(base+'/devices/'+created.id),outputs=await call(base+'/devices/'+created.id+'/outputs');state.devices.push({id:created.id,output:outputs.items[0].current.id,revision:device.revision,output_revision:outputs.items[0].current.revision});save();
 }
 }
 for(const device of state.devices){const slots=await call(base+'/devices/'+device.id+'/outputs'),output=slots.items[0].current;device.output=output.id;device.output_revision=output.revision;device.revision=output.device_revision;}save();
 const sourcePath=path+'/source';
 for(let i=0;i<state.devices.length;i++){
  const current=await call(sourcePath),device=state.devices[i],old=state.devices[0];
  const input={...meta(),action:i?'switch':'bind',expected_source_revision:current.source_revision,expected_channel_revision:current.channel_revision,output_id:device.output,expected_new_output_revision:device.output_revision,expected_new_device_revision:device.revision,...(i?{expected_binding_id:current.source.id,expected_binding_revision:current.source.revision,expected_old_output_revision:old.output_revision,expected_old_device_revision:old.revision}:{})};
  const preview=await call(sourcePath+'/preview',input),{action,...command}=input;await call(sourcePath+'/'+action,{...command,preview_hash:preview.preview_hash});
 }
 state.expected.source_history=(await call(sourcePath+'/history')).items;save();
 const derived=base+'/monitoring/derived',c=(await call(derived+'/channels',{...wm('0'),code:'COMPONENT-DIFF',name:'主梁直属 A−B',owner:{kind:'component',id:component,facility_id:prior.facility},definition_version_id:prior.channels.A_MINUS_B.version,input_bindings:{A:{channel_id:prior.channels.A.id},B:{channel_id:prior.channels.B.id}},parameters:{}})).channel;
 const cp=derived+'/channels/'+c.resource.id,config=(await call(cp+'/configs')).items[0],trial=await call(cp+'/trials',{...wm(c.config_revision),config_id:config.id,config_hash:config.config_hash,output_range:prior.range});const complete=await wait(derived+'/runs/'+trial.id,v=>v.job.state,'succeeded');
 const preview=await call(cp+'/control-preview',{expected_revision:c.control_revision,action:'enable',auto_start_ns:prior.range.from_ns,stop_ns:prior.range.to_ns,trial_receipt_id:complete.trial.trial_receipt_id});await call(cp+'/control',{...wm(c.control_revision),preview_id:preview.id,preview_hash:preview.hash});
 const end=Date.now()+120000;let generation;while(Date.now()<end){generation=(await call(cp+`/generations?from_ns=${prior.range.from_ns}&to_ns=${prior.range.to_ns}`)).items[0];if(generation)break;await new Promise(r=>setTimeout(r,300));}if(!generation)throw Error('Component calculation did not publish');
 const rows=await call(derived+'/generations/'+generation.id+'/query',{});if(!rows.items.at(-1)?.value?.startsWith('9.'))throw Error('Component calculation differs from A−B');
 prior.channels.COMPONENT_DIFF={id:c.resource.id,version:prior.channels.A_MINUS_B.version,generation:generation.id};state.channels=prior.channels;save();
}
export async function grantSpatialDeviceOutputs(call,base,project,state){
 const path=base+'/projects/'+project+'/device-access',current=await call(path),input={request_id:randomUUID(),reason:'仅授予示例输出与最小设备上下文',expected_device_access_revision:current.device_access_revision,expected_project_revision:current.project_revision,add:state.devices.map(d=>({kind:'output',target_id:d.output,use_source:false})),update:[],end:[]},preview=await call(path+'/preview',input);await call(path+'/changes',{...input,preview_hash:preview.preview_hash});
}
