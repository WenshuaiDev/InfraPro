import {spawn,execFileSync} from 'node:child_process';
import {mkdirSync,writeFileSync,appendFileSync,existsSync,rmSync} from 'node:fs';
import {randomBytes} from 'node:crypto';
import {join,resolve} from 'node:path';
import {createServer} from 'node:net';
import {ensureTestDependencies,testEnv} from '../test-environment.mjs';
export async function availablePort(){return new Promise((ok,no)=>{const s=createServer();s.on('error',no);s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>ok(p));});});}
export async function startParser(directory=resolve('.artifacts/parsing/service')){
 mkdirSync(directory,{recursive:true,mode:0o700});const token=join(directory,'machine-token'),pkg=join(directory,'release'),runner=join(directory,'parser-runner'),binary=join(directory,'parser');
 if(!existsSync(token))writeFileSync(token,randomBytes(48).toString('hex'),{mode:0o600});
 for(const [file,target]of [[runner,'parser-runner'],[binary,'parser']])execFileSync('go',['build','-trimpath','-o',file,`./services/parser/cmd/${target}`],{stdio:'inherit'});
 execFileSync('go',['run','./services/parser/cmd/parser-package',runner,'samples/parsing',pkg],{stdio:'inherit'});
 const port=await availablePort(),url=`http://127.0.0.1:${port}`;let child;
 async function start(){child=spawn(binary,[],{env:{...process.env,INFRAPRO_PARSER_PACKAGE:pkg,INFRAPRO_PARSER_TOKEN_FILE:token,INFRAPRO_PARSER_ADDR:`127.0.0.1:${port}`,INFRAPRO_PARSER_RELEASE:'parsing-acceptance',INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'parsing-lab',INFRAPRO_DIAGNOSTICS_NODE_ID:'local',INFRAPRO_DIAGNOSTICS_REPLICA_ID:'parser',INFRAPRO_DIAGNOSTICS_REPORT_FILE:join(directory,'process-report.json')},stdio:['ignore','pipe','pipe']});for(const s of [child.stdout,child.stderr])s.on('data',v=>appendFileSync(join(directory,'service.log'),v));await waitURL(url+'/healthz');}
 async function pause(){await stop(child);child=undefined;}
 await start();return {url,token,pkg,runner,binary,pause,resume:start,close:pause};
}
export async function stop(child){if(!child||child.exitCode!==null)return;const ended=new Promise(r=>child.once('exit',r));child.kill('SIGTERM');const timer=setTimeout(()=>child.kill('SIGKILL'),10000);try{await ended;}finally{clearTimeout(timer);}}
export async function waitURL(url){for(let i=0;i<100;i++){try{if((await fetch(url,{signal:AbortSignal.timeout(1000)})).ok)return;}catch{/* starting */}await new Promise(r=>setTimeout(r,300));}throw Error('Parsing lab startup timed out: '+url);}
export async function startParsingLab({build=true,environment={},consolePort=Number(process.env.INFRAPRO_PARSING_CONSOLE_PORT??15578),apiPort=Number(process.env.INFRAPRO_PARSING_API_PORT??18578),directory=resolve('.artifacts/parsing/browser-lab')}={}){
 for(const port of [consolePort,apiPort])await new Promise((ok,no)=>{const s=createServer();s.once('error',()=>no(Error('Parsing lab port is already occupied: '+port)));s.listen(port,'127.0.0.1',()=>s.close(ok));});
 mkdirSync(directory,{recursive:true,mode:0o700});ensureTestDependencies();execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
 if(build)execFileSync('pnpm',['--filter','@infrapro/console','build'],{stdio:'inherit'});
 const parser=await startParser(join(directory,'parser')),binary=join(directory,'api-test'),workerBinary=join(directory,'monitoring-worker'),config=join(directory,'operator.json');
 execFileSync('go',['test','-c','-o',binary,'./services/api/internal/transport/http'],{stdio:'inherit'});execFileSync('go',['build','-o',workerBinary,'./services/api/cmd/monitoring-worker'],{stdio:'inherit'});rmSync(config,{force:true});
 const origin=`http://127.0.0.1:${consolePort}`,apiOrigin=`http://127.0.0.1:${apiPort}`,children=[];
 const start=(cmd,args,cwd,env={})=>{const child=spawn(cmd,args,{cwd,env:{...process.env,...env},stdio:['ignore','pipe','pipe']});children.push(child);for(const s of [child.stdout,child.stderr])s.on('data',v=>appendFileSync(join(directory,'server.log'),v));return child;};
 const close=async()=>{for(const child of [...children].reverse())await stop(child);await parser.close();writeFileSync(join(directory,'session.json'),JSON.stringify({origin,apiOrigin,synthetic:true,state:'stopped'},null,2));};
 try{start(binary,['-test.run','^TestE2EServer$','-test.v','-test.timeout','0'],resolve('services/api/internal/transport/http'),{...testEnv,...environment,INFRAPRO_E2E_SERVER:'1',INFRAPRO_PARSING_E2E:'1',INFRAPRO_PARSING_URL:parser.url,INFRAPRO_PARSING_TOKEN_FILE:parser.token,INFRAPRO_E2E_OPS_CONFIG:config,INFRAPRO_HTTP_ADDR:`127.0.0.1:${apiPort}`,INFRAPRO_E2E_CONSOLE_URL:origin,INFRAPRO_E2E_ORIGINS:origin});
 start(process.execPath,[resolve('apps/console/node_modules/vite/bin/vite.js'),'preview'],resolve('apps/console'),{API_PROXY_TARGET:apiOrigin,CONSOLE_PORT:String(consolePort),VITE_API_BASE_URL:'/api/v1'});
 await waitURL(apiOrigin+'/healthz');await waitURL(origin);start(workerBinary,[],resolve('.'),{INFRAPRO_CONFIG:config,INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'parsing-lab',INFRAPRO_DIAGNOSTICS_NODE_ID:'local',INFRAPRO_DIAGNOSTICS_REPLICA_ID:'monitoring-worker',INFRAPRO_DIAGNOSTICS_REPORT_FILE:join(directory,'worker-report.json')});
 writeFileSync(join(directory,'session.json'),JSON.stringify({origin,apiOrigin,synthetic:true,state:'running',pids:children.map(v=>v.pid)},null,2));return {origin,apiOrigin,config,directory,parser,close};
 }catch(e){await close();throw e;}
}
