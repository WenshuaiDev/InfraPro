import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { isDeepStrictEqual } from 'node:util';
const {schemas,policies}=JSON.parse(execFileSync('go',['run','./tools/infrapro/cmd/spatial-contract'],{maxBuffer:16*1024*1024}));
const ref=name=>({$ref:`#/components/schemas/${name}`});
const sr=name=>ref(`Spatial${name}`);
for(const name of ['Asset','Version','Page','Element','Placement','ComponentLink','FeatureView','Object','Location','Basemap']) schemas[`Spatial${name}List`]={type:'object',required:['items','next_cursor'],properties:{items:{type:'array',items:sr(name)},next_cursor:{type:'string'}}};
schemas.SpatialHistoryList={type:'object',required:['items','next_cursor'],properties:{items:{type:'array',items:{type:'object',additionalProperties:true}},next_cursor:{type:'string'}}};
const channelSummary={type:'object',required:['channel','selection','latest'],properties:{channel:ref('MonitoringChannel'),selection:ref('MonitoringSelection'),latest:ref('MonitoringAnalyticResult')}};
const ownerSummary={type:'object',required:['owner','channels','next_cursor'],properties:{owner:sr('ObjectRef'),next_cursor:{type:'string'},channels:{type:'array',items:channelSummary}}};
schemas.SpatialSummaries={type:'object',required:['items','refreshed_at','snapshot_note'],properties:{refreshed_at:{type:'string',format:'date-time'},snapshot_note:{type:'string'},items:{type:'array',items:ownerSummary}}};
const outputs={'assets.list':'AssetList','assets.detail':'Asset','assets.versions':'VersionList','assets.history':'HistoryList','versions.draft':'Draft','versions.validate':'Draft','uploads.create':'Result','uploads.detail':'Upload','uploads.part':'Result','versions.process':'Result','jobs.detail':'Job','jobs.retry':'Result','versions.manifest':'Version','versions.pages':'PageList','versions.elements':'ElementList','versions.properties':'ElementList','versions.placements':'PlacementList','versions.component-links':'ComponentLinkList','versions.features':'FeatureViewList','objects':'ObjectList','objects.detail':'Object','basemaps':'BasemapList','summaries':'Summaries','locate':'LocationList','access.detail':'AccessDetail','access.preview':'Preview','views.detail':'View'};
const inputs={'assets.create':'AssetInput','assets.update':'AssetInput','versions.create':'VersionInput','versions.edit':'DraftInput','versions.delete':'RevisionInput','versions.validate':'RevisionInput','uploads.create':'UploadInput','uploads.complete':'RevisionInput','uploads.cancel':'RevisionInput','versions.process':'RevisionInput','jobs.cancel':'RevisionInput','jobs.retry':'RevisionInput','summaries':'ObjectQuery','locate':'ObjectQuery','related-artifacts':'RelatedInput','access.preview':'AccessInput','access.save':'AccessInput','views.save':'ViewInput'};
const pathURL=new URL('../contracts/openapi/api.yaml',import.meta.url);const contract=JSON.parse(await readFile(pathURL,'utf8'));
for(const key of Object.keys(contract.paths)){if(Object.values(contract.paths[key]).some(o=>o.operationId?.startsWith('Spatial:')))delete contract.paths[key];}
for(const key of Object.keys(contract.components.schemas)){if(key.startsWith('Spatial'))delete contract.components.schemas[key];}
Object.assign(contract.components.schemas,schemas);
for(const p of policies){
 const operation=p.operation??p.Operation,method=(p.method??p.Method).toLowerCase(),path=(p.path??p.Path).replace(/:([a-z_]+)/g,'{$1}');const action=operation.split(':')[2];
 const parameters=[...path.matchAll(/\{([^}]+)\}/g)].map(([,name])=>({name,in:'path',required:true,schema:{type:'string',...(!['part','file_id','tile'].includes(name)?{format:'uuid'}:{})}}));
 if(method==='get'&&!['versions.files','versions.source','basemaps.tile'].includes(action))for(const name of ['q','kind','owner_id','facility_id','state','page_id','element_key','cursor','limit','bbox'])parameters.push({name,in:'query',schema:name==='limit'?{type:'integer',minimum:1,maximum:100}:{type:'string'}});
 const output=action==='related-artifacts'?ref('AnalysisArtifactPage'):sr(action.endsWith('-preview')?'Preview':outputs[action]??'Result');
 const file=['versions.files','versions.source','basemaps.tile'].includes(action);const status=action==='versions.process'?'202':'200';
 const responses={[status]:{description:'已按当前身份与固定版本验证；修订号使用十进制字符串。',headers:{'X-Spatial-Authorization-Revision':{schema:{type:'string'}}},content:file?{'application/octet-stream':{schema:{type:'string',format:'binary'}}}:{'application/json':{schema:output}}}};
 for(const status of [400,401,403,404,409,413,422,503])responses[status]={description:'输入、授权、修订或文件状态不满足要求。写入结果未知时查验原请求，禁止自动重复提交。',content:{'application/json':{schema:sr('Error')}}};
 const op={operationId:operation,tags:['Spatial visualization'],summary:action,security:[{sessionCookie:[]}],'x-permissions':p.permissions??p.Permissions??p.codes??p.Codes,parameters,responses};
 if(method!=='get'){const input=inputs[action]??'ControlInput';op.requestBody={required:true,content:action==='uploads.part'?{'application/octet-stream':{schema:{type:'string',format:'binary'}}}:{'application/json':{schema:sr(input)}}};if(action==='uploads.part')parameters.push({name:'X-Content-SHA256',in:'header',required:true,schema:{type:'string',pattern:'^[a-f0-9]{64}$'}});}
 (contract.paths[path]??={})[method]=op;
}
const output=JSON.stringify(contract,null,2)+'\n';if(process.argv.includes('--check')){if(!isDeepStrictEqual(JSON.parse(await readFile(pathURL,'utf8')),contract))throw Error('Spatial OpenAPI is stale. Run pnpm spatial:contracts.');}else await writeFile(pathURL,output);
console.log('Spatial contracts synchronized.');
