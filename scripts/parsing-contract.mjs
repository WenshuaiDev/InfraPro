import {readFile,writeFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {isDeepStrictEqual} from 'node:util';
const {schemas,policies}=JSON.parse(execFileSync('go',['run','./tools/infrapro/cmd/parsing-contract'],{maxBuffer:16*1024*1024}));
const ref=name=>({$ref:`#/components/schemas/Parsing${name}`});
for(const s of Object.values(schemas))for(const [k,v]of Object.entries(s.properties??{})){
 if(/^(revision|expected_revision|expected_default_revision|source_default_revision|generation|current_generation|control_revision|fence|number)$/.test(k)&&v.type==='string')v.pattern='^[0-9]+$';
 if(/digest$|^sha256$/.test(k)&&v.type==='string')v.pattern='^[0-9a-f]{64}$';
}
const inputs={'ingestion-catalog.sync':'Meta','ingress-protocols.state':'StateInput','identification-methods.state':'StateInput','ingress-compatibilities.state':'StateInput','catalog.sync':'Meta','methods.state':'StateInput','configs.create':'ConfigInput','configs.revision':'ConfigInput','configs.state':'StateInput','defaults.set':'DefaultInput','device-configs.create':'ConfigInput','device-configs.revision':'ConfigInput','device.preview':'Command','device.commands':'Command','batch.preview':'BatchCommand','batch.commands':'BatchCommand','samples.create':'SampleInput','samples.confirm':'Meta','runs.create':'RunInput','runs.compare':'CompareInput'};
const outputs={'ingress-protocols.list':'CapabilityPage','identification-methods.list':'CapabilityPage','ingress-compatibilities.list':'CapabilityPage','catalog.status':'CatalogStatus','methods.list':'MethodPage','configs.list':'ConfigRevisionPage','device-configs.list':'ConfigRevisionPage','public-configs.list':'ConfigRevisionPage','defaults.detail':'Default','public-defaults.detail':'Default','device.detail':'DeviceState','device.history':'AdoptionPage','device.preview':'Preview','samples.list':'SamplePage','samples.detail':'Sample','runs.list':'RunPage','runs.detail':'RunDetail','runs.execution':'Execution','runs.result':'ResultView','runs.compare':'CompareResult'};
const url=new URL('../contracts/openapi/api.yaml',import.meta.url),original=JSON.parse(await readFile(url,'utf8')),contract=structuredClone(original);
Object.assign(contract.components.schemas,schemas);
contract.components.schemas.DeviceCreate.properties.parsing_adoption={type:'object',properties:{mode:{type:'string',enum:['inherit_default','unconfigured']},expected_default_revision:{type:'string',pattern:'^[0-9]+$'}},required:['mode'],additionalProperties:false};
for(const p of policies){
 const [, ,kind,action]=p.Operation.split(':'),key=`${kind}.${action}`,method=p.Method.toLowerCase(),path=p.Path.replace(/:([a-z_]+)/g,'{$1}');
 const parameters=[...path.matchAll(/\{([^}]+)\}/g)].map(([,name])=>({name,in:'path',required:true,schema:{type:'string',...(['method_id','version','generation'].includes(name)?{}:{format:'uuid'})}}));
 if(action==='list'||key==='device.history')for(const name of ['cursor','limit','q','state'])parameters.push({name,in:'query',schema:name==='limit'?{type:'integer',minimum:1,maximum:100}:{type:'string'}});
 if(key==='runs.list')parameters.push({name:'device_id',in:'query',schema:{type:'string',format:'uuid'}});
 if(key==='runs.result')for(const name of ['offset','limit'])parameters.push({name,in:'query',schema:{type:'integer',minimum:0,maximum:name==='limit'?100:50000}});
 if(key==='samples.download')parameters.push({name:'representation',in:'query',schema:{type:'string',enum:['original']}});
 if(method!=='get')parameters.push({name:'X-CSRF-Token',in:'header',required:true,schema:{type:'string'}});
 if(key==='samples.content')parameters.push({name:'X-Command-ID',in:'header',required:true,schema:{type:'string',format:'uuid'}});
 const binary=['samples.download','runs.response'].includes(key),body=key==='batch.preview'?{type:'array',items:ref('Preview')}:key==='catalog.detail'?{type:'object',additionalProperties:true}:ref(outputs[key]??'Result');
 const response={description:'当前权限检查后的解析配置或证据；不产生正式观测。',headers:{'Cache-Control':{schema:{type:'string',enum:['private, no-store']}}},content:{[binary?'application/octet-stream':'application/json']:{schema:binary?{type:'string',format:'binary'}:body}}};
 const operation={operationId:p.Operation,tags:['Parsing'],security:[{sessionCookie:[]}],parameters,responses:{[key==='runs.create'||key==='runs.compare'?202:200]:response}};
 for(const code of [400,401,403,404,409,413,422,429,503])operation.responses[code]={description:'授权、固定版本、修订、预算或依赖检查失败。',content:{'application/json':{schema:{$ref:'#/components/schemas/Error'}}}};
 if(inputs[key])operation.requestBody={required:true,content:{'application/json':{schema:ref(inputs[key])}}};
 if(key==='samples.content')operation.requestBody={required:true,content:{'application/octet-stream':{schema:{type:'string',format:'binary',maxLength:6291456}}}};
 (contract.paths[path]??={})[method]=operation;
}
// The separate machine API has no tenant/session credentials and shares the same DTOs.
const machine={openapi:'3.1.0',info:{title:'InfraPro fixed parsing service',version:'1'},servers:[{url:'http://127.0.0.1:8091'}],components:{schemas:Object.fromEntries(Object.entries(schemas).filter(([k])=>k.startsWith('ParsingWire'))),securitySchemes:{machineToken:{type:'http',scheme:'bearer'}}},security:[{machineToken:[]}],paths:{}};
for(const [path,method,input,output]of [['/healthz','get',null,null],['/readyz','get',null,null],['/v1/catalog','get',null,'WireCatalog'],['/v1/parse','post','WireRequest','WireResponse']]){
 const op={responses:{200:{description:'Bounded machine response',content:{'application/json':{schema:output?ref(output):{type:'object',additionalProperties:true}}}},401:{description:'Machine authentication required'},413:{description:'Budget exceeded'},429:{description:'Four child processes are already running'},503:{description:'Artifact unavailable'},504:{description:'Child process exceeded five seconds and was reaped'}}};
 if(path==='/healthz')op.security=[];
 if(input)op.requestBody={required:true,content:{'application/json':{schema:ref(input)}}};
 machine.paths[path]={[method]:op};
}
for(const [target,before,after]of [[url,original,contract],[new URL('../contracts/openapi/parsing-service.yaml',import.meta.url),JSON.parse(await readFile(new URL('../contracts/openapi/parsing-service.yaml',import.meta.url),'utf8').catch(()=>'{}')),machine]]){
 if(process.argv.includes('--check')){if(!isDeepStrictEqual(before,after))throw Error('Parsing contracts are stale. Run pnpm parser:contracts.');}else await writeFile(target,JSON.stringify(after,null,2)+'\n');
}
