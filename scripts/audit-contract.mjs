import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {isDeepStrictEqual} from 'node:util';
const {schemas,policies,catalog,producers,command_operations,all_policies}=JSON.parse(execFileSync('go',['run','./tools/infrapro/cmd/audit-contract'],{maxBuffer:16*1024*1024}));
const ref=name=>({$ref:`#/components/schemas/Audit${name}`});
schemas.AuditScope={oneOf:[{type:'object',required:['kind'],additionalProperties:false,properties:{kind:{type:'string',enum:['platform']}}},{type:'object',required:['kind','tenant_id'],additionalProperties:false,properties:{kind:{type:'string',enum:['tenant']},tenant_id:{type:'string',format:'uuid'}}},{type:'object',required:['kind','tenant_id','project_id'],additionalProperties:false,properties:{kind:{type:'string',enum:['project']},tenant_id:{type:'string',format:'uuid'},project_id:{type:'string',format:'uuid'}}}]};
for(const name of ['AuditFilter','AuditEvent']){
 schemas[name].properties.outcome.enum=['success','failure','denied','conflict','cancelled','rate_limited','unknown'];
 schemas[name].properties.category.enum=['security','management','business','task','file','maintenance'];
}
schemas.AuditExportFilter.properties.state={type:"string",enum:["queued","running","retry_wait","cancelling","succeeded","failed","cancelled"]};
schemas.AuditExportFilter.properties.limit={type:"integer",minimum:1,maximum:100};
schemas.AuditFilter.properties.limit={type:'integer',minimum:1,maximum:100};
schemas.AuditFilter.properties.q.maxLength=100;
schemas.AuditExportInput.properties.field_profile.enum=['standard','technical'];
schemas.AuditExportInput.properties.reason={type:'string',minLength:1,maxLength:500};
schemas.AuditExportFile.properties.kind.enum=['audit.export'];
schemas.AuditExport.properties.state.enum=['queued','running','retry_wait','cancelling','succeeded','failed','cancelled'];
schemas.AuditExport.properties.result_state.enum=['none','preparing','available','expired','invalidated','missing','restricted','unknown'];
for(const state of ['before_state','after_state'])schemas.AuditChange.properties[state].enum=['recorded','not_recorded','redacted','truncated'];
const url=new URL('../contracts/openapi/api.yaml',import.meta.url),original=JSON.parse(await readFile(url,'utf8')),contract=structuredClone(original);
Object.assign(contract.components.schemas,schemas);
const outputs={capabilities:'Capabilities',events:'EventPage',event:'EventDetail',trace:'TracePage',target:'Target','exports.create':'Export',exports:'ExportPage',export:'Export'};
for(const policy of policies){
 const action=policy.Operation.split(':').slice(2).join(':'),method=policy.Method.toLowerCase(),path=policy.Path.replace(/:([a-z_]+)/g,'{$1}');
 const parameters=[...path.matchAll(/\{([^}]+)\}/g)].map(([,name])=>({name,in:'path',required:true,schema:{type:'string',...(name==='event_id'||name==='generation'?{pattern:'^[1-9][0-9]*$'}:{format:'uuid'})}}));
 const filter=action==='events'?schemas.AuditFilter.properties:action==='trace'?schemas.AuditTraceFilter.properties:action==='exports'?{...schemas.AuditExportFilter.properties,mode:{type:'string',enum:['mine','managed']}}:action==='export'?{generation:{type:'string',pattern:'^[1-9][0-9]*$'}}:{};
 for(const[name,schema]of Object.entries(filter))parameters.push({name,in:'query',schema});
 const binary=action.startsWith('download');
 const responses={};
 for(const status of binary?[200,206,304]:[action==='exports.create'?202:200])responses[status]={description:binary?'当前所有人及同一授权向量的受控成果。':'当前账号与精确空间的授权投影。',headers:{'Cache-Control':{schema:{type:'string',enum:['private, no-store']}}},...(method==='head'||status===304?{}:{content:{[binary?'text/csv':'application/json']:{schema:binary?{type:'string',format:'binary'}:ref(outputs[action])}}})};
 for(const status of [400,401,403,404,409,410,416,428,429,503])responses[status]={description:'资格、命令收据、修订、查询预算或依赖不满足要求。',content:{'application/json':{schema:ref('Error')}}};
 const operation={operationId:policy.Operation,tags:['Audit and operation tracing'],security:[{sessionCookie:[]}],parameters,responses};
 if(action==='exports.create'){
  parameters.push({name:'X-Command-ID',in:'header',required:true,schema:{type:'string',format:'uuid'}},{name:'X-CSRF-Token',in:'header',required:true,schema:{type:'string'}});
  operation.requestBody={required:true,content:{'application/json':{schema:ref('ExportInput')}}};
 }
 if(binary)for(const name of ['Range','If-Range','If-Match','If-None-Match','If-Unmodified-Since','If-Modified-Since'])parameters.push({name,in:'header',schema:{type:'string'}});
 (contract.paths[path]??={})[method]=operation;
}
for(const p of all_policies.filter(p=>command_operations.includes(p.Operation))){
 const operation=contract.paths[p.Path.replace(/:([a-z_]+)/g,'{$1}')][p.Method.toLowerCase()];
 operation.parameters??=[];
 if(!operation.parameters.some(p=>p.in==='header'&&p.name==='X-Command-ID'))operation.parameters.push({name:'X-Command-ID',in:'header',required:true,schema:{type:'string',format:'uuid'}});
 operation.responses[428]={description:'此入口需要稳定的命令编号。',content:{'application/json':{schema:ref('Error')}}};
}
contract['x-audit-command-operations']=command_operations;
if(process.argv.includes('--check')){if(!isDeepStrictEqual(contract,original))throw Error('Audit OpenAPI is stale. Run pnpm audit:contracts.');}
else await writeFile(url,`${JSON.stringify(contract,null,2)}\n`);
await mkdir('.artifacts/audit',{recursive:true});
await writeFile('.artifacts/audit/catalog.json',`${JSON.stringify(catalog,null,2)}\n`);
await writeFile('.artifacts/audit/producers.json',`${JSON.stringify(producers,null,2)}\n`);
