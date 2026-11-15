import {connect} from '@infrapro/display-sdk';
const output=document.querySelector<HTMLElement>('#output')!,button=document.querySelector<HTMLButtonElement>('#refresh')!;
const abort=new AbortController();
const client=await connect({signal:abort.signal});
const context=await client.context.get(),config=await client.config.get();
document.querySelector<HTMLElement>('h1')!.textContent=String(config.title??'SDK 接入示例');
document.querySelector<HTMLElement>('#mode')!.textContent=context.simulated?'模拟数据':context.preview?'真实草稿预览':'正式数据';
async function refresh(){button.disabled=true;try{const result=await client.resources.list({target:{binding:'site'}},{signal:abort.signal});output.textContent=JSON.stringify(result,null,2);}catch(e){output.textContent=e instanceof Error?e.message:'读取失败';}finally{button.disabled=false;}}
button.onclick=()=>void refresh();client.runtime.on(event=>{document.querySelector<HTMLElement>('#connection')!.textContent=event.type;});
const site=await client.bindings.resolve('site');
const open=document.querySelector<HTMLButtonElement>('#open')!;
if(site.ids[0]&&!context.simulated){open.disabled=false;open.onclick=()=>void client.navigation.open('resource',site.ids[0]!).catch(e=>{output.textContent=e instanceof Error?e.message:'跳转失败';});}
const checks:string[]=[];
try {
  await WebAssembly.instantiateStreaming(fetch(new URL('./probe.wasm',import.meta.url)));
  checks.push('WASM 可用');
  const workerURL=client.spatial.createObjectURL(new Blob(['postMessage("worker-ready")'],{type:'text/javascript'}));
  const worker=new Worker(workerURL);
  await new Promise<void>((resolve,reject)=>{const timeout=setTimeout(()=>reject(Error('Worker timeout')),3000);worker.onmessage=()=>{clearTimeout(timeout);resolve()};worker.onerror=()=>{clearTimeout(timeout);reject(Error('Worker unavailable'))}}).finally(()=>{worker.terminate();client.spatial.revokeObjectURL(workerURL)});
  checks.push('Worker 可用');
  const gl=document.createElement('canvas').getContext('webgl');
  if(gl){checks.push('WebGL 可用');gl.getExtension('WEBGL_lose_context')?.loseContext();}else checks.push('WebGL 不可用');
}catch(e){checks.push(e instanceof Error?e.message:'能力检测失败');}
document.querySelector<HTMLElement>('#compatibility')!.textContent=checks.join(' · ');
await refresh();await client.runtime.ready();
window.addEventListener('pagehide',()=>{abort.abort();client.dispose();},{once:true});
