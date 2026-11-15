import {spawn,execFileSync} from 'node:child_process';
import {mkdirSync,appendFileSync,existsSync,rmSync,writeFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {createServer} from 'node:net';
import {ensureTestDependencies,testEnv} from '../test-environment.mjs';

export async function startFilesLab({build=true,spatial=false,consolePort=15574,apiPort=18574,directory=resolve('.artifacts/files/lab'),consoleDist}={}) {
 const root=resolve('.');mkdirSync(directory,{recursive:true,mode:0o700});
 const origin=`http://127.0.0.1:${consolePort}`,apiOrigin=`http://127.0.0.1:${apiPort}`;
 for(const port of [consolePort,apiPort])await new Promise((ok,no)=>{const s=createServer();s.once('error',()=>no(Error(`Port ${port} is occupied; existing processes were left untouched.`)));s.listen(port,'127.0.0.1',()=>s.close(ok));});
 ensureTestDependencies();execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
 if(build)execFileSync('pnpm',['--filter','@infrapro/console','build'],{stdio:'inherit'});
 const checkerBinary=join(directory,'files-checker');execFileSync('go',['build','-o',checkerBinary,'./services/api/cmd/files-checker'],{stdio:'inherit'});
 const indexBinary=join(directory,'task-index-worker'),indexPaused=join(directory,'index.paused');
 const binary=join(directory,'api-test'),workerBinary=join(directory,'notification-worker'),config=join(directory,'operator.json'),paused=join(directory,'worker.paused');
 execFileSync('go',['test','-c','-o',binary,'./services/api/internal/transport/http'],{stdio:'inherit'});execFileSync('go',['build','-o',workerBinary,'./services/api/cmd/notification-worker'],{stdio:'inherit'});
 execFileSync('go',['build','-o',indexBinary,'./services/api/cmd/task-index-worker'],{stdio:'inherit'});
 rmSync(join(directory,'files-acceptance.json'),{force:true});rmSync(indexPaused,{force:true});rmSync(config,{force:true});rmSync(paused,{force:true});writeFileSync(join(directory,'server.log'),'');const children=[];let worker,indexWorker,closed=false;
 function start(cmd,args,cwd,env={}){const child=spawn(cmd,args,{cwd,detached:true,env:{...process.env,...env},stdio:['ignore','pipe','pipe']});children.push(child);for(const s of [child.stdout,child.stderr])s.on('data',v=>appendFileSync(join(directory,'server.log'),v));return child;}
 async function stop(child){if(!child||child.exitCode!==null)return;const ended=new Promise(r=>child.once('exit',r));try{process.kill(-child.pid,'SIGTERM');}catch{return;}await Promise.race([ended,new Promise(r=>{const timer=setTimeout(r,15000);timer.unref();})]);}
 async function waitURL(url){for(let i=0;i<120;i++){try{if((await fetch(url,{signal:AbortSignal.timeout(1000)})).ok)return;}catch{/* startup */}await new Promise(r=>setTimeout(r,500));}throw Error('Files lab startup failed; inspect '+join(directory,'server.log'));}
 const session=()=>writeFileSync(join(directory,'session.json'),JSON.stringify({synthetic:true,origin,apiOrigin,workerPID:worker?.pid,indexPID:indexWorker?.pid,workerRunning:!!worker&&!closed,paused:existsSync(paused),state:closed?'stopped':'running',directory},null,2));
 const resume=async()=>{if(worker||closed)return;worker=start(workerBinary,[],root,{INFRAPRO_CONFIG:config});session();};
 const pause=async()=>{const prior=worker;worker=undefined;await stop(prior);session();};
 const close=async()=>{if(closed)return;closed=true;clearInterval(control);await pause();await stop(indexWorker);indexWorker=undefined;for(const c of [...children].reverse())await stop(c);session();};
 let changing=false;const control=setInterval(()=>{if(changing||closed)return;changing=true;(async()=>{if(existsSync(paused))await pause();else if(existsSync(config))await resume();if(existsSync(indexPaused)){await stop(indexWorker);indexWorker=undefined;}else if(!indexWorker&&existsSync(config)){indexWorker=start(indexBinary,[],root,{INFRAPRO_CONFIG:config});}session();})().catch(e=>console.error(e.message)).finally(()=>{changing=false;});},1000);
 try {
 start(binary,['-test.run','^TestE2EServer$','-test.v','-test.timeout','0'],join(root,'services/api/internal/transport/http'),{...testEnv,INFRAPRO_E2E_SERVER:'1',INFRAPRO_NOTIFICATIONS_E2E:'1',INFRAPRO_TASKS_E2E:'1',INFRAPRO_FILES_E2E:'1',...(spatial?{INFRAPRO_SPATIAL_E2E:'1',INFRAPRO_SPATIAL_PYTHON:join(root,'.artifacts/spatial-venv/bin/python'),INFRAPRO_SPATIAL_CONVERTER_SCRIPT:join(root,'tools/spatial-converter/convert.py')}:{}),INFRAPRO_FILES_CHECKER_EXECUTABLE:checkerBinary,INFRAPRO_HTTP_ADDR:`127.0.0.1:${apiPort}`,INFRAPRO_E2E_CONSOLE_URL:origin,INFRAPRO_E2E_ORIGINS:origin,INFRAPRO_E2E_OPS_CONFIG:config});
 start(process.execPath,[join(root,'apps/console/node_modules/vite/bin/vite.js'),'preview',...(consoleDist?['--outDir',resolve(consoleDist)]:[])],join(root,'apps/console'),{API_PROXY_TARGET:apiOrigin,CONSOLE_PORT:String(consolePort),VITE_API_BASE_URL:'/api/v1'});
 await waitURL(apiOrigin+'/healthz');await waitURL(origin);await resume();session();
 return {origin,directory,workerBinary,config,paused,indexPaused,close,pause:async()=>{writeFileSync(paused,'paused');await pause();},resume:async()=>{rmSync(paused,{force:true});await resume();}};
 }catch(e){await close();throw e;}
}
