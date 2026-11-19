// New synthetic definitions and devices are created independently in the paired
// databases. Only canonical routes, contract shapes and named effects leave
// the local lab. No credentials or business response bodies enter evidence.
import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { queryPG, read, runDirectory, write } from './architecture-lab.mjs';
import { operation } from './architecture-workflow-http.mjs';
import { deviceCatalog } from './architecture-device-catalog.mjs';

const reason='架构双端目录生命周期核验';
const truth=(name,valid)=>{assert.ok(valid,name);return {[name]:true};};
const items=v=>Array.isArray(v)?v:v.items;
const keys=v=>v&&typeof v==='object'&&!Array.isArray(v)?Object.keys(v).sort():Array.isArray(v)?['array']:[typeof v];

async function definitions(c) {
  const {call,wm,tag,tenant,sample}=c;
  const base='/platform/engineering',tenantEng=tenant+'/engineering',def={};
  const category='qa_'+tag;
  const updated=await call('unit_category_create','POST',base+'/unit-categories',{...wm(),key:category,name:'量纲 '+tag,description:reason});
  await call('unit_category_rename','POST',base+'/unit-categories',{...wm(),key:category,name:'自验量纲 '+tag,description:reason,revision:updated.revision});
  for(const root of [base,tenantEng])await call('unit_category_visible_'+root.split('/')[1],'GET',root+'/unit-categories',null,v=>truth('published_category_in_scope',items(v).some(c=>c.key===category)));
  const free=await call('unused_category_create','POST',base+'/unit-categories',{...wm(),key:'unused_'+tag,name:'未引用量纲 '+tag,description:reason});
  await call('unused_category_delete','POST',base+'/unit-categories/unused_'+tag+'/delete',{...wm(),revision:free.revision});
  const content={
    units:{name:'配对单位 '+tag,unit:{symbol:'mm',category}},
    metrics:{name:'配对指标 '+tag,metric:{shape:'decimal',category:'sbm_length',standard_unit:sample.unit,allowed_units:[sample.unit],options:[]}},
    'type-definitions':{name:'配对设施 '+tag,fields:[{key:'title',name:'标识',shape:'text',required:false,order:0,group:'',description:'',multiline:false}],components:[{key:'span',name:'构件',fields:[]}],relations:[{parent:'root',child:'span',min_count:0,max_count:12,order:0}]},
    'monitoring-templates':{name:'配对模板 '+tag,template:{type_versions:[sample.types.A.version_id],items:[{key:'movement',name:'位移',enabled:true,targets:[{type_version:sample.types.A.version_id,component:'span'}],bindings:[{metric_version:sample.metric,unit_version:sample.unit,resource_kind:'observation_channel',min_count:1}]}]}}
  };
  for(const [kind,body] of Object.entries(content)){
    const root=base+'/'+kind,tagged=kind.replaceAll('-','_');
    const code='q_'+tagged+'_'+tag;
    async function cycle(collection,slot,body,code,copyFrom) {
      const prefix=slot+'_'+tagged+'_',cmd=wm();
      const created=await call(prefix+'create','POST',collection,{...cmd,code,content:{metamodel_version:'1',...body}});
      const path=collection+'/'+created.definition_id;
      await call(prefix+'receipt','GET',collection+'/requests/'+cmd.request_id,null,v=>truth('receipt_matches_definition',v.definition_id===created.definition_id));
      const detail=await call(prefix+'draft','GET',path,null,v=>truth('draft_code',v.code===code&&!!v.draft));
      const updatedBody={...detail.draft.content,description:'配对修订 '+tag};
      const saved=await call(prefix+'save','PUT',path+'/draft',{...wm(),revision:created.revision,content:updatedBody});
      const preview=await call(prefix+'preview','POST',path+'/preview',{revision:saved.revision},v=>truth('usable_sample_only',v.valid&&v.sample_only));
      assert.equal(preview.content.description,updatedBody.description);
      const first=await call(prefix+'publish','POST',path+'/publish',{...wm(),revision:saved.revision},v=>truth('immutable_version_created',Boolean(v.version_id)));
      await call(prefix+'list','GET',collection+'?q='+code,null,v=>truth('own_definition_listed',items(v).some(x=>x.id===created.definition_id)));
      await call(prefix+'impact','GET',path+'/versions/'+first.version_id+'/impact');
      const next=await call(prefix+'new_draft','POST',path+'/draft',{...wm(),revision:first.revision,version_id:first.version_id});
      const revised=await call(prefix+'save_next','PUT',path+'/draft',{...wm(),revision:next.revision,content:{...updatedBody,release_notes:'第二次发布'}});
      const second=await call(prefix+'publish_next','POST',path+'/publish',{...wm(),revision:revised.revision},v=>truth('distinct_immutable_version',v.version_id!==first.version_id));
      await call(prefix+'diff','GET',path+'/diff?from='+first.version_id+'&to='+second.version_id,null,v=>truth('compared_revisions',items(v).length>0));
      const empty=await call(prefix+'discardable_draft','POST',path+'/draft',{...wm(),revision:second.revision,version_id:second.version_id});
      await call(prefix+'discard','POST',path+'/draft/delete',{...wm(),revision:empty.revision});
      const retired=await call(prefix+'retire','POST',path+'/versions/'+first.version_id+'/state',{...wm(),revision:1,enabled:false});
      await call(prefix+'retired_detail','GET',path,null,v=>truth('retired_version_still_visible',v.versions.some(x=>x.id===first.version_id&&!x.enabled)));
      await call(prefix+'restore','POST',path+'/versions/'+first.version_id+'/state',{...wm(),revision:retired.revision,enabled:true});
      const copy=await call(prefix+'copy','POST',collection+'/copy',{...wm(),source_version:copyFrom??second.version_id,code:'c_'+code});
      await call(prefix+'copy_origin','GET',collection+'/'+copy.definition_id,null,v=>truth('copy_is_new_draft',!!v.draft&&v.id!==created.definition_id));
      return {id:created.definition_id,first:first.version_id,second:second.version_id,code};
    }
    const published=await cycle(root,'platform',body,code);def[kind]=published;
    const cat=tenantEng+'/catalog/'+kind,pk=published.id;
    await call(tagged+'_catalog_list','GET',cat+'?q='+code,null,v=>truth('public_definition_inherited',items(v).some(x=>x.id===pk)));
    await call(tagged+'_catalog_detail','GET',cat+'/'+pk);
    if(kind!=='monitoring-templates')await call(tagged+'_catalog_version','GET',cat+'/versions/'+published.second,null,v=>truth('adoptable_version_matches',v.version.id===published.second));
    await call(tagged+'_catalog_preview','POST',cat+'/'+pk+'/preview',{version_id:published.second},v=>truth('published_preview',v.valid));
    await call(tagged+'_catalog_diff','GET',cat+'/'+pk+'/diff?from='+published.first+'&to='+published.second);
    if(kind==='monitoring-templates')await cycle(tenantEng+'/'+kind,'tenant',body,'local_'+tag,published.second);
  }
  return def;
}

export async function catalogParity(directory,output=directory,section='definitions') {
  const lab=read(join(directory,'private/lab.json')),accounts=read(join(directory,'private/accounts.json')),sample=read(join(directory,'samples.json')).structure;
  assert.equal(resolve(lab.directory),resolve(directory));const tag=randomBytes(4).toString('hex'),ids=new Map(),ledgers={a:[],b:[]};
  const command=key=>{if(!ids.has(key))ids.set(key,randomUUID());return ids.get(key);};
  const name='catalog-'+section;
  async function run(side) {
    const target=lab[side];assert.equal(new URL(target.api).hostname,'127.0.0.1');
    const http=await request.newContext({baseURL:target.api,timeout:20000}),records=ledgers[side];
    const csrf=await http.get('/api/v1/auth/csrf'),headers={Origin:target.origin,'X-CSRF-Token':(await csrf.json()).token};await csrf.dispose();
    let step='bootstrap',wmSeq=0;
    const wm=()=>({request_id:command('wm'+wmSeq++),reason});
    async function call(label,method,path,body=null,effect=()=>({}),statuses=[200,201,204]) {
      step=label;
      const uri='/api/v1'+path,op=operation(method,uri.split('?')[0]);assert.ok(op,'canonical route missing');
      const res=await http.fetch(uri,{method,headers:{...headers,'X-Command-ID':command('call_'+label)},...(body!==null?{data:body}:{})});
      try {
        const v=res.status()===204?{}:await res.json();
        const rec={label,method,template:op.template,owner:op.owner,operationId:op.operationId,status:res.status(),code:res.status()>=400?v.code:null,shape:keys(v),effects:{}};
        records.push(rec);if(!statuses.includes(rec.status))throw Error('HTTP '+rec.status+' '+(v.code??'')+' '+Object.keys(v.fields??{}).join(',')+' '+(v.issues??[]).slice(0,3).map(i=>i.path).join(','));
        rec.effects=effect(v);return v;
      }finally{await res.dispose();}
    }
    try {
      const self=await call('operator_login','POST','/auth/login',accounts.admin);
      const tenantData=await call('work_area','POST','/platform/tenants',{name:'配对目录 '+tag,code:'catalog-'+tag,administrator_account_id:self.account.id,client_request_id:command('tenant'),reason});
      // Migration 49 is the only producer of pre-structure section evidence.
      // Stage its immutable output against this run's own installation.
      function stageLegacyInstallation(device,installation) {
        const ids=[tenantData.tenant_id,device,installation];
        for(const id of ids)assert.match(id,/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/);
        const [tid,did,iid]=ids;
        const result=queryPG(lab,side,`INSERT INTO engineering_legacy_section_evidence
          (tenant_id,facility_id,resource_kind,resource_id,device_id,source_table,source_id,source_revision,source_path,entry_index,original,historical)
          SELECT i.tenant_id,i.facility_id,'installation',i.id,i.device_id,'device_installations',i.id,i.revision::text,
            'position.sections',0,to_jsonb('合成旧断面'::text),i.uninstalled_at IS NOT NULL
          FROM device_installations i JOIN devices d ON d.id=i.device_id JOIN tenants t ON t.id=i.tenant_id
          WHERE t.id='${tid}' AND t.code='catalog-${tag}' AND d.id='${did}' AND d.code='D-${tag}' AND i.id='${iid}' RETURNING id::text`);
        assert.match(result,/^[0-9a-f-]{36}$/);return result;
      }
      const c={call,wm,tag,tenant:'/tenants/'+tenantData.tenant_id,sample,command,stageLegacyInstallation};
      if(section==='definitions')await definitions(c);
      else if(['devices','taxonomy','products'].includes(section))await deviceCatalog(c,section);
      else throw Error('Unrecognized section');
    } catch(e){const message=String(e.message).split('\n')[0];throw Error(side+' '+step+' '+(/^[A-Za-z0-9._ /:,-]{1,190}$/.test(message)?message:'test condition failed'),{cause:e});} finally {await http.dispose();}
  }
  const results=await Promise.allSettled(['a','b'].map(run)),failures=results.filter(r=>r.status==='rejected').map(r=>r.reason.message);
  const records=Array.from({length:Math.max(ledgers.a.length,ledgers.b.length)},(_,i)=>{
    const a=ledgers.a[i],b=ledgers.b[i],row=a??b;
    return {...row,scenario:row.label,command:command('call_'+row.label),status:[a?.status,b?.status],errors:[a?.code,b?.code],
      differences:JSON.stringify(a)===JSON.stringify(b)?[]:['business_assertion_or_shape']};});
  const passed=!failures.length&&records.every(r=>!r.differences.length);
  const destination=passed?output:join(directory,'attempts',name+'-'+tag);
  write(join(destination,'http','scenarios',name+'.json'),{scenario:name,records,failures});
  write(join(destination,'http','workflows',name+'.json'),{cases:[{arc:['definitions','taxonomy'].includes(section)?'ARC-007':'ARC-008',name,passed,checks:records.map(r=>r.label)}]});
  if(!passed)throw Error(failures.join(' / ')||'Paired business effects diverged');
  return {section,checks:records.length};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url))console.log(await catalogParity(runDirectory(process.argv[2]),resolve(process.argv[3]??process.argv[2]),process.argv[4]??'definitions'));
