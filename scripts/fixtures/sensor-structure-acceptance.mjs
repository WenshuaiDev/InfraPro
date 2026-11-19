import {randomUUID,createHash} from 'node:crypto';

// Section 9.5 is created with the public API, including publications, grants,
// source bindings and formal observation adoption. The caller owns its session.
export async function seedSensorStructure({call,client,api,headers,accountID,wait,categoryIDs}) {
 const meta=()=>({request_id:randomUUID(),reason:'SBM 方案9.5合成验收'}),wm=(revision='0')=>({...meta(),expected_revision:revision});
 const state={synthetic:true,fixture:'sensor-structure-proposal-9.5',nodes:{},channels:{},devices:[],categories:{},types:{}};
 const publish=async(kind,code,content)=>{const d=await call(`/platform/engineering/${kind}`,{...meta(),code,content:{metamodel_version:'1',...content}});return call(`/platform/engineering/${kind}/${d.definition_id}/publish`,{...meta(),revision:d.revision});};
 await call('/platform/engineering/unit-categories',{...meta(),key:'sbm_length',name:'SBM 长度',description:'合成位移量纲'});
 const unit=await publish('units','sbm_mm',{name:'毫米',unit:{symbol:'mm',category:'sbm_length'}}),metric=await publish('metrics','sbm_displacement',{name:'SBM 位移',metric:{shape:'decimal',category:'sbm_length',standard_unit:unit.version_id,allowed_units:[unit.version_id],options:[]}});
 state.unit=unit.version_id;state.metric=metric.version_id;
 for(const [key,name,holes]of [['A','SBM 路基',true],['B','SBM 隧道',true],['N','SBM 无孔类型',false]])state.types[key]=await publish('type-definitions','sbm_type_'+key.toLowerCase(),{metamodel_version:'2',name,fields:[],components:[{key:'span',name:'构件',fields:[]}],relations:[{parent:'root',child:'span',min_count:0,max_count:100,order:0}],structure:{monitoring_section:{supported:true,fields:[]},borehole:{supported:holes,fields:[]}}});
 const tenant=await call('/platform/tenants',{name:'设备分类与工程结构 · SBM 验收',code:'sbm-acceptance',administrator_account_id:accountID,client_request_id:randomUUID(),reason:'完整方案独立合成验收'});state.tenant=tenant.tenant_id;
 const base=`/tenants/${state.tenant}`,resources=base+'/engineering/resources/facilities';
 const rootPath=code=>resources+'/'+state.nodes[state.nodes[code].facility].id;
 const paths={component:'components',monitoring_section:'monitoring-sections',borehole:'boreholes',point:'points'};
 state.resourcePath=resources;
 for(const key of ['A','B','N'])state.nodes[key]={...await call(resources,{request_id:randomUUID(),code:key,name:state.types[key].definition_id===state.types.A.definition_id?'路基设施 A':key==='B'?'隧道设施 B':'无孔设施 N',type_version_id:state.types[key].version_id,values:{}}),kind:'facility',facility:key};
 for(const [code,parent,kind]of [['S1','A','monitoring_section'],['H1','S1','borehole'],['P1','H1','point'],['P2','H1','point'],['S1a','S1','monitoring_section'],['P3','S1a','point'],['C1','A','component'],['S2','C1','monitoring_section'],['H3','S2','borehole'],['P6','H3','point'],['H2','A','borehole'],['S3','H2','monitoring_section'],['P4','S3','point'],['P5','A','point'],['S4','B','monitoring_section'],['H4','S4','borehole'],['P7','H4','point']]) {
  const ancestor=state.nodes[parent],fid=ancestor.facility,root=await call(resources+'/'+state.nodes[fid].id);
  const value=await call(resources+'/'+root.id+'/'+paths[kind],{request_id:randomUUID(),code,name:code,parent_ref:{kind:ancestor.kind,id:ancestor.id},expected_structure_revision:root.structure_revision,...(kind==='component'?{category_key:'span'}:{}),...(kind==='borehole'?{borehole:{measured_depth_m:10}}:{})});
  state.nodes[code]={...value,kind,facility:fid};
  if(kind==='point')state.channels[code]=await call(resources+'/'+root.id+'/points/'+value.id+'/channels',{request_id:randomUUID(),code:'CH-'+code,name:code+' 位移通道',metric_version_id:metric.version_id,unit_version_id:unit.version_id,timestamp_meaning:'observation_time'});
 }
 const point=await call(rootPath('P1')+'/points/'+state.nodes.P1.id),facility=await call(rootPath('P1'));
 const pos={...meta(),expected_revision:point.revision,expected_structure_revision:facility.structure_revision,action:'position',position:{borehole_id:state.nodes.H1.id,borehole_revision:1,distance_m:0,vertical_depth_m:null,reference_start:'孔口',direction:'沿孔向下',unit:'m',vertical_datum:'',source:'合成实测记录',measured_at:null,consistency_reason:''}},posPath=rootPath('P1')+'/points/'+point.id;
 const posPreview=await call(posPath+'/structure-records-preview',pos);await call(posPath+'/structure-records',{...pos,preview_hash:posPreview.preview_hash});
 const dictPath='/platform/business-dictionaries/device-category',dictionary=await call(dictPath),names={sensor:'传感器',displacement:'位移传感器',temperature:'温度传感器',environment:'环境类',collector:'采集器'};
 for(const key of Object.keys(names))state.categories[key]=categoryIDs?.[key] || randomUUID();
 const content={dictionary_code:'device_category',sensor_root_id:state.categories.sensor,nodes:Object.entries(names).map(([code,name],order)=>({id:state.categories[code],code,name,description:'方案9.5合成分类',order,selectable:true,status:'active',...(code!=='sensor'&&code!=='collector'?{parent_id:state.categories.sensor}:{})}))};
 const saved=await call(dictPath+'/draft',{...wm(dictionary.draft_revision),content},'PUT'),publication=wm(saved.revision),preview=await call(dictPath+'/publish-preview',publication),release=await call(dictPath+'/publish',{...publication,preview_hash:preview.preview_hash});state.release=release.release_id;
 const template=await publish('monitoring-templates','sbm_public_template',{metamodel_version:'2',name:'SBM 公共孔内监测模板',template:{type_versions:[state.types.A.version_id,state.types.N.version_id],items:[{key:'movement',name:'孔内位移',enabled:true,suggested_category:{dictionary_release_id:state.release,category_id:state.categories.displacement},targets:[{type_version:state.types.A.version_id,component:'',applicability:'applicable',anchor_kind:'monitoring_section',point_scope:'descendant_points',requires_borehole_ancestor:true,quantity_basis:'per_point'},{type_version:state.types.N.version_id,component:'',applicability:'not_applicable',reason:'固定类型不支持孔位'}],bindings:[{metric_version:metric.version_id,unit_version:unit.version_id,resource_kind:'observation_channel',min_count:1}]}]}});
 const copy=await call(base+'/engineering/monitoring-templates/copy',{...meta(),code:'sbm_private_template',source_version:template.version_id});state.template=await call(base+'/engineering/monitoring-templates/'+copy.definition_id+'/publish',{...meta(),revision:copy.revision});state.publicTemplate=template;
 const catalog='/platform/device-definitions',maker=await call(catalog+'/manufacturers',{...meta(),code:'SBM',name:'SBM 合成厂商'}),versions=[];
 for(const [code,classification]of [['SBM-S',{mode:'classify',dictionary_release_id:state.release,category_id:state.categories.displacement}],['SBM-U',{mode:'unclassified'}]]) {
  const p=await call(catalog+'/products',{...meta(),manufacturer_id:maker.id,code,name:'SBM 双输出设备 '+code,model:code,output_slots:['a','b'].map((key,order)=>({key,name:key,mode:'fixed',order,metric_version_id:metric.version_id,unit_version_id:unit.version_id,timestamp_meaning:'observation_time'}))});
  versions.push(await call(catalog+'/products/'+p.id+'/publish',{...meta(),expected_draft_revision:'1',classification}));
 }
 const outputs=[];
 for(const [i,category]of ['','', 'temperature','environment','collector','unclassified'].entries()) {
  const d=await call(base+'/devices',{...meta(),code:'V'+(i+1),name:'设备 V'+(i+1),product_version_id:versions[i===5?1:0].version_id,parsing_adoption:{mode:'unconfigured'}});state.devices.push(d);outputs.push((await call(base+'/devices/'+d.id+'/outputs')).items.map(v=>v.current));
  if(category&&category!=='unclassified') {const {kind,id,expected_classification_revision,expected_entity_revision}=await call(base+'/device-classifications/device/'+d.id),input={...meta(),targets:[{kind,id,expected_classification_revision,expected_entity_revision}],selection:{mode:'classify',dictionary_release_id:state.release,category_id:state.categories[category]}},p=await call(base+'/device-classifications/preview',input);await call(base+'/device-classifications/apply',{...input,preview_id:p.preview_id,preview_hash:p.preview_hash});}
 }
 for(const [point,i,slot]of [['P1',0,0],['P2',0,1],['P3',1,0],['P7',1,1],['P6',2,0],['P4',4,0]]) {
  const d=await call(base+'/devices/'+state.devices[i].id),output=outputs[i][slot],path=rootPath(point)+'/points/'+state.nodes[point].id+'/channels/'+state.channels[point].id+'/source',input={...meta(),expected_source_revision:'0',expected_channel_revision:'1',output_id:output.id,expected_new_output_revision:output.revision,expected_new_device_revision:d.revision},p=await call(path+'/preview',{action:'bind',...input});await call(path+'/bind',{...input,preview_hash:p.preview_hash});
 }
 const member=(await call(base+'/context')).membership_id,project=await call(base+'/projects',{name:'SBM 完整验收项目',code:'SBM',purpose:'方案9.5实体／设备／70条观测',owner_tenant_membership_id:member,administrator_tenant_membership_id:member,client_request_id:randomUUID(),reason:'明确项目授权'});state.project=project.project_id;const pb=base+'/projects/'+state.project;
 for(const root of ['A','B']) {
  const path=pb+'/resource-access/facilities/'+state.nodes[root].id,current=await call(path),add=Object.values(state.nodes).filter(n=>n.facility===root).map(n=>({kind:n.kind,...(n.kind==='facility'?{}:{[n.kind+'_id']:n.id}),capabilities:['read']}));
  for(const [p,c]of Object.entries(state.channels))if(state.nodes[p].facility===root)add.push({kind:'channel',channel_id:c.id,capabilities:['read']});
  const input={...meta(),action:'create_link',expected_access_revision:current.policy.revision,add},preview=await call(path+'/preview',input);await call(path+'/changes',{...input,preview_hash:preview.preview_hash});
  for(const g of (await call(path)).grants.filter(g=>g.resource_kind==='channel'||g.kind==='channel')) {const a=await call(pb+'/monitoring-access');await call(pb+'/monitoring-access',{...wm(a.access_revision),channel_id:g.resource_id,resource_grant_id:g.id,range:{from:null,to:null},capabilities:['read','history']});}
 }
 const access=await call(pb+'/device-access'),input={...meta(),expected_device_access_revision:access.device_access_revision,expected_project_revision:access.project_revision,add:state.devices.flatMap((d,i)=>[{kind:'device',target_id:d.id,use_source:false},...outputs[i].map(o=>({kind:'output',target_id:o.id,use_source:false}))]),update:[],end:[]},dp=await call(pb+'/device-access/preview',input);await call(pb+'/device-access/changes',{...input,preview_hash:dp.preview_hash});
 const monitoring=base+'/monitoring';let csv='channel,observed_at,value,quality\n';for(const p of Object.keys(state.channels))for(let n=0;n<10;n++)csv+=`${p},2026-11-01T00:${String(n).padStart(2,'0')}:00Z,${n},good\n`;
 const sha256=createHash('sha256').update(csv).digest('hex');let batch=await call(monitoring+'/imports',{...wm(),name:'SBM-70.csv',size_bytes:String(Buffer.byteLength(csv))});
 const uploaded=await client.put(api+monitoring+'/imports/'+batch.id+'/upload/parts/1',{headers:{...headers,'Content-Type':'application/octet-stream','X-Expected-Revision':batch.revision,'X-Content-SHA256':sha256},data:Buffer.from(csv)});if(!uploaded.ok())throw Error('SBM CSV upload failed');
 batch=await call(monitoring+'/imports/'+batch.id+'/upload/complete',{...wm(batch.revision),sha256});batch=await call(monitoring+'/imports/'+batch.id+'/mappings',{...wm(batch.revision),mapping:{format:'csv',mode:'long',encoding:'utf-8',delimiter:',',header_row:1,channel_column:'channel',time_column:'observed_at',value_column:'value',quality_column:'quality',timezone:'UTC',empty_wide:'error',channels:Object.entries(state.channels).map(([alias,c])=>({alias,channel_id:c.id,unit:'mm'}))}});
 await call(monitoring+'/imports/'+batch.id+'/validate',wm(batch.revision));batch=await wait(monitoring+'/imports/'+batch.id,v=>v.state,'ready');await call(monitoring+'/imports/'+batch.id+'/publish',wm(batch.revision));await wait(monitoring+'/imports/'+batch.id,v=>v.state,'published');state.batch=batch.id;
 state.expected={facilities:2,sections:5,boreholes:4,points:7,channels:7,observations:70,authorized_devices:6,bound_devices:4,sensors:4};return state;
}
