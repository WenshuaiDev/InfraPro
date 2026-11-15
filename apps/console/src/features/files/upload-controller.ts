import type { FileHashes, HashMessage } from './hash.worker';
export type UploadPhase = 'hash'|'transfer'|'server_check'|'needs_action'|'bound'|'paused';
export type UploadProgress = { phase:UploadPhase; bytes:number; total:number };
export type UploadSession = { id:string; revision:string; state:string; parts:{part:number;sha256:string;size:number}[] };
export interface UploadAdapter<S extends UploadSession,R> {
 partBytes:number;
 validate(file:File):void;
 open(file:File,hashes:FileHashes,signal:AbortSignal):Promise<S>;
 put(session:S,part:FileHashes['parts'][number],body:Blob,signal:AbortSignal):Promise<unknown>;
 complete(session:S,hashes:FileHashes,signal:AbortSignal):Promise<R>;
}
export function hashFile(file:Blob,partBytes:number,signal:AbortSignal,progress:(bytes:number)=>void=()=>{}):Promise<FileHashes> {
 return new Promise((resolve,reject)=>{
  signal.throwIfAborted();
  const worker=new Worker(new URL('./hash.worker.ts',import.meta.url),{type:'module'});
  const finish=()=>{signal.removeEventListener('abort',abort);worker.terminate();};
  const abort=()=>{finish();reject(new DOMException('传输已暂停，已接收内容保留。','AbortError'));};
  signal.addEventListener('abort',abort,{once:true});
  worker.onmessage=(event:MessageEvent<HashMessage>)=>{const m=event.data;if(m.kind==='progress')progress(m.bytes);if(m.kind==='done'){finish();resolve(m.result);}if(m.kind==='error'){finish();reject(Error(m.message));}};
  worker.onerror=()=>{finish();reject(Error('无法计算文件摘要，请重新选择文件。'));};
  worker.postMessage({file,partBytes});
 });
}
/** Bounded sequential transfer, matching receipts before sending missing parts. */
export async function transferFile<S extends UploadSession,R>(adapter:UploadAdapter<S,R>,file:File,signal:AbortSignal,progress:(v:UploadProgress)=>void):Promise<R> {
 adapter.validate(file);signal.throwIfAborted();
 const report=(phase:UploadPhase,bytes:number)=>{signal.throwIfAborted();progress({phase,bytes,total:file.size});};
 report('hash',0);const hashes=await hashFile(file,adapter.partBytes,signal,n=>report('hash',n));
 const session=await adapter.open(file,hashes,signal);signal.throwIfAborted();
 let bytes=0;
 const seen=new Set<number>();
 for(const receipt of session.parts){const p=hashes.parts[receipt.part-1];if(seen.has(receipt.part)||!p||p.sha256!==receipt.sha256||p.size!==receipt.size)throw Error('所选文件与已接收的内容不一致，请选择原文件。');seen.add(receipt.part);bytes+=p.size;}
 report('transfer',bytes);
 for(const part of hashes.parts){signal.throwIfAborted();if(seen.has(part.part))continue;const start=(part.part-1)*adapter.partBytes;await adapter.put(session,part,file.slice(start,start+part.size),signal);bytes+=part.size;report('transfer',bytes);}
 report('server_check',bytes);return adapter.complete(session,hashes,signal);
}
