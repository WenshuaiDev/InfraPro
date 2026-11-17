import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {registerHooks} from 'node:module';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
const root=new URL('../../',import.meta.url).href;
registerHooks({resolve(s,c,next){if(s.startsWith('.')&&!/\.(ts|js|mjs)$/.test(s)&&c.parentURL?.startsWith(root)&&c.parentURL.endsWith('.ts'))return next(new URL(s+'.ts',c.parentURL).href,c);return next(s,c)},load(url,c,next){if(url.startsWith(root)&&url.endsWith('.ts'))return {format:'module',shortCircuit:true,source:ts.transpileModule(readFileSync(new URL(url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText};return next(url,c)}});
const {SHA256}=await import('../../apps/console/src/features/files/sha256.ts');
const {transferFile,hashFile}=await import('../../apps/console/src/features/files/upload-controller.ts');
const {ordinaryAdapter}=await import('../../apps/console/src/features/files/adapters/ordinary.ts');
const {monitoringAdapter}=await import('../../apps/console/src/features/files/adapters/monitoring.ts');
const {spatialAdapter}=await import('../../apps/console/src/features/files/adapters/spatial.ts');
const {displayAdapter}=await import('../../apps/console/src/features/files/adapters/display.ts');
const {fileMethods}=await import('../../packages/ts/api-client/src/files.ts');
const {filesSearch}=await import('../../apps/console/src/features/files/state.ts');
const hash=data=>createHash('sha256').update(data).digest('hex');
const signal=()=>new AbortController().signal;
class HashWorker {static stopped=0;terminated=false;postMessage({file,partBytes}){queueMicrotask(async()=>{const all=Buffer.from(await file.arrayBuffer()),parts=[];for(let start=0;start<all.length;start+=partBytes){const b=all.subarray(start,start+partBytes);parts.push({part:parts.length+1,size:b.length,sha256:hash(b)});}if(!this.terminated)this.onmessage({data:{kind:'done',result:{sha256:hash(all),parts}}});});}terminate(){this.terminated=true;HashWorker.stopped++;}}

test('incremental SHA-256 matches independent implementation across block boundaries',()=>{for(const size of [0,1,55,56,63,64,65,65537,8388609]){const data=Buffer.alloc(size,0x65),h=new SHA256();for(let i=0;i<size;i+=413)h.update(data.subarray(i,i+413));assert.equal(h.digest(),hash(data));assert.throws(()=>h.update(new Uint8Array([1])));}});
test('controller resumes only verified receipts, stops unsent bytes on abort and rejects ambiguous receipts',async()=>{globalThis.Worker=HashWorker;const file=new File(['abcdefg'],'same.txt');const parts=[{part:1,size:3,sha256:hash('abc')}],sent=[];const adapter={partBytes:3,validate(){},open:async()=>({id:'u',revision:'1',state:'receiving',parts}),put:async(_s,p)=>sent.push(p.part),complete:async()=>({state:'queued'})};assert.deepEqual(await transferFile(adapter,file,signal(),()=>{}),{state:'queued'});assert.deepEqual(sent,[2,3]);parts[0].sha256=hash('wrong');await assert.rejects(transferFile(adapter,file,signal(),()=>{}),/不一致/);parts[0].sha256=hash('abc');parts.push(parts[0]);await assert.rejects(transferFile(adapter,file,signal(),()=>{}),/不一致/);parts.length=0;sent.length=0;const abort=new AbortController();adapter.put=async(_s,p)=>{sent.push(p.part);abort.abort();};await assert.rejects(transferFile(adapter,file,abort.signal,()=>{}),{name:'AbortError'});assert.deepEqual(sent,[1]);});
test('hash workers terminate on completion and identity boundary abort',async()=>{globalThis.Worker=HashWorker;const before=HashWorker.stopped;await hashFile(new Blob(['abc']),8,signal());const abort=new AbortController(),pending=hashFile(new Blob(['abc']),8,abort.signal);abort.abort();await assert.rejects(pending,{name:'AbortError'});assert.equal(HashWorker.stopped-before,2);});
test('ordinary create and complete response loss preserves command identity, jpg and frozen part size',async()=>{const data=new File(['abc'],'photo.jpg'),hashes={sha256:hash('abc'),parts:[{part:1,size:3,sha256:hash('abc')}]},caps={host:{kind:'tenant.profile',id:'t'},can_manage:true,accept_uploads:true,max_file_bytes:'104857600',part_bytes:'8388608',formats:['jpeg','txt'],restore_epoch:'e'};let current,loseCreate=true,loseComplete=true;const starts=[],completes=[];const client={fileStartUpload:async(_s,v)=>{starts.push(structuredClone(v));current={id:'u',state:'receiving',revision:'1',filename:data.name,size_bytes:'3',sha256:hashes.sha256,restore_epoch:'e',part_size:'8388608',parts:hashes.parts.map(p=>({...p,size_bytes:String(p.size)}))};if(loseCreate){loseCreate=false;throw Error('lost create')}return{upload_id:'u'}},fileUpload:async()=>current,fileCompleteUpload:async(_s,_id,v)=>{completes.push(structuredClone(v));current.state='submitted';if(loseComplete){loseComplete=false;throw Error('lost complete')}return{job_id:'j'}}};const a=ordinaryAdapter(client,{tenant:'t'},caps,{reason:'reason',displayName:'photo',description:''},()=>{});a.validate(data);await assert.rejects(a.open(data,hashes,signal()),/lost create/);const s=await a.open(data,hashes,signal());assert.deepEqual(starts[0],starts[1]);await assert.rejects(a.complete(s,hashes,signal()),/lost complete/);const retry=await a.open(data,hashes,signal());assert.deepEqual(await a.complete(retry,hashes,signal()),{job_id:'j'});assert.deepEqual(completes[0],completes[1]);await assert.rejects(a.open(new File(['bad'],'photo.jpg'),{...hashes,sha256:hash('bad')},signal()),/原文件/);assert.equal(ordinaryAdapter(client,{}, {...caps,accept_uploads:false},{upload:{...current,part_size:'4'},reason:'resume'},()=>{}).partBytes,4);});
test('professional adapters use deployment limits while retaining part numbering',async()=>{
 const limits={monitoring_csv_max_bytes:String(64*1024**3),monitoring_xlsx_max_bytes:String(1024**3),spatial_max_file_bytes:String(1024**3),display_max_package_bytes:String(256*1024**2)};
 let reads=0;const client={uploadLimits:async()=>{reads++;return limits;}};
 const monitoring=monitoringAdapter(client,{},'reason',undefined,()=>{});assert.equal(monitoring.partBytes,32*1024**2);
 await monitoring.validate({name:'source.csv',size:64*1024**3});await monitoring.validate({name:'source.xlsx',size:1024**3});
 await assert.rejects(monitoring.validate({name:'source.xlsx',size:1024**3+1}));
 const spatial=spatialAdapter(client,{},{},'',()=>{});assert.equal(spatial.partBytes,8*1024**2);await spatial.validate({size:1024**3});await assert.rejects(spatial.validate({size:1024**3+1}));
 const sent=[],commands=[];const display=displayAdapter({...client,displayCommand:async(...args)=>{commands.push(args);return{upload_id:'u',revision:'2'}},displayPart:async(...args)=>sent.push(args)},{tenantId:'t',projectId:'p'},'i','1');
 assert.equal(display.partBytes,4*1024**2);await display.validate({name:'file.zip',size:256*1024**2});await assert.rejects(display.validate({name:'file.zip',size:256*1024**2+1}));
 const session=await display.open(new File(['x'],'file.zip'),{sha256:hash('x')},signal());await display.put(session,{part:1,size:1,sha256:hash('x')},new Blob(['x']),signal());assert.equal(sent[0][3],0);
 await display.complete(session,{},signal());await display.complete(session,{},signal());assert.deepEqual(commands[1][2],commands[2][2]);assert.equal(reads,7);
 limits.display_max_package_bytes='1';await display.validate({name:'file.zip',size:2});
});

test('API uses scoped native content paths, binary epochs, and decimal generations without coercion',async()=>{const calls=[],methods=fileMethods(async(...args)=>{calls.push(args);return{}},'/api/v1');const s={tenant:'tenant',project:'project'};await methods.fileUploadPart(s,'upload',1,'hash',new Blob(['x']),'epoch',signal());assert.equal(calls[0][0],'/tenants/tenant/projects/project/files/uploads/upload/parts/1');assert.equal(calls[0][5].headers['X-Files-Epoch'],'epoch');assert.ok(calls[0][5].binary instanceof Blob);assert.equal(methods.fileDownloadURL(s,'attachment','revision'),'/api/v1/tenants/tenant/projects/project/files/attachments/attachment/revisions/revision/content');assert.equal(filesSearch({generation:'9223372036854775806',url:'https://invalid',attachment:'not-id'}).generation,'9223372036854775806');assert.equal(filesSearch({attachment:'not-id'}).attachment,undefined);});

test('controller awaits server size validation before hashing or sending',async()=>{
 let opened=false;globalThis.Worker=HashWorker;const before=HashWorker.stopped;
 const adapter={partBytes:8,validate:async()=>{await Promise.resolve();throw Error('configured limit');},open:async()=>{opened=true;}};
 await assert.rejects(transferFile(adapter,new File(['ab'],'x.txt'),signal(),()=>{}),/configured limit/);assert.equal(opened,false);assert.equal(HashWorker.stopped,before);
});
