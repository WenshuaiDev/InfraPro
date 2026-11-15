import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { isDeepStrictEqual } from 'node:util';
const { schemas, policies } = JSON.parse(execFileSync('go', ['run', './tools/infrapro/cmd/files-contract'], { maxBuffer: 16 * 1024 * 1024 }));
const ref = name => ({ $ref: `#/components/schemas/${name}` });
for (const schema of Object.values(schemas)) for (const [key, value] of Object.entries(schema.properties ?? {})) {
 if (/^(size_bytes|max_file_bytes|part_bytes|part_size|revision|revision_no|expected_revision|generation|control_revision|expected_generation|expected_control_revision|content_revision|authorization_revision)$/.test(key)) Object.assign(value, { type: 'string', pattern: '^[0-9]+$' });
 if (key === 'sha256') Object.assign(value, { pattern: '^[0-9a-f]{64}$' });
}
schemas.FileStartUploadInput.properties.intent.enum = ['add', 'replace'];
schemas.FileHost.properties.kind.enum = ['tenant.profile', 'project.profile'];
const outputs = {capabilities:'Capabilities','attachments.list':'AttachmentPage','attachments.detail':'AttachmentDetail','attachments.revisions':'RevisionPage','uploads.list':'UploadPage','uploads.detail':'UploadDetail','uploads.part':'PartReceipt','jobs.detail':'JobDetail'};
const inputs = {'uploads.create':'StartUploadInput','uploads.complete':'RevisionCommand','uploads.cancel':'RevisionCommand','uploads.reconfirm':'ReconfirmInput','attachments.update':'AttachmentCommand','attachments.trash':'RevisionCommand','attachments.restore':'RevisionCommand','attachments.revert':'AttachmentCommand'};
const url = new URL('../contracts/openapi/api.yaml',import.meta.url), original = JSON.parse(await readFile(url,'utf8')), contract = structuredClone(original);
Object.assign(contract.components.schemas,schemas);
for (const p of policies) {
 const action=p.Operation.split(':')[2], method=p.Method.toLowerCase(), path=p.Path.replace(/:([a-z_]+)/g,'{$1}'), binary=/^(content|preview)(\.head)?$/.test(action);
 const parameters=[...path.matchAll(/\{([^}]+)\}/g)].map(([,name])=>({name,in:'path',required:true,schema:name==='part'?{type:'integer',minimum:1}:{type:'string',format:'uuid'}}));
 const queries={'attachments.list':['host_kind','host_id','state','q','created_from','created_to','cursor','limit'],'attachments.revisions':['cursor','limit'],'uploads.list':['view','state','cursor','limit'],'jobs.detail':['generation']}[action]??[];
 for(const name of queries) parameters.push({name,in:'query',schema:name==='limit'?{type:'integer',minimum:1,maximum:100}:{type:'string'}});
 if(!['get','head'].includes(method)) for(const name of ['X-CSRF-Token','X-Files-Epoch']) parameters.push({name,in:'header',required:true,schema:{type:'string'}});
 if(binary) for(const name of ['Range','If-Range','If-Match','If-None-Match']) parameters.push({name,in:'header',schema:{type:'string'}});
 if(action==='uploads.part') parameters.push({name:'X-Content-SHA256',in:'header',required:true,schema:{type:'string',pattern:'^[0-9a-f]{64}$'}});
 const success={description:binary?'当前人员授权、完整性核验及逐块授权的文件正文。':'当前宿主权限裁剪后的文件元数据。',headers:{'Cache-Control':{schema:{type:'string',enum:['private, no-store']}}},content:{[binary?'application/octet-stream':'application/json']:{schema:binary?{type:'string',format:'binary'}:ref('File'+(outputs[action]??'CommandResult'))}}};
 const responses={ [action==='uploads.create'?201:['uploads.complete','uploads.reconfirm'].includes(action)?202:200]:success };
 if(binary) for(const status of [206,304,412,416]) responses[status]={description:'单 Range / 条件请求；身份、权限和完整性先行核验。'};
 for(const status of [400,401,403,404,409,413,415,422,429,503]) responses[status]={description:'权限、输入、修订、恢复代次、预算或依赖不满足。',content:{'application/json':{schema:ref('Error')}}};
 const operation={operationId:p.Operation,tags:['Files and attachments'],security:[{sessionCookie:[]}],parameters,responses};
 if(inputs[action]) operation.requestBody={required:true,content:{'application/json':{schema:ref('File'+inputs[action])}}};
 if(action==='uploads.part') operation.requestBody={required:true,content:{'application/octet-stream':{schema:{type:'string',format:'binary',maxLength:8388608}}}};
 (contract.paths[path]??={})[method]=operation;
}
if(process.argv.includes('--check')) {if(!isDeepStrictEqual(original,contract)) throw Error('Files OpenAPI is stale. Run pnpm files:contracts.');} else await writeFile(url,`${JSON.stringify(contract,null,2)}\n`);
