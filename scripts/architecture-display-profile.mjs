import assert from 'node:assert/strict';
import {join,resolve} from 'node:path';
import {existsSync,mkdirSync,renameSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {read,runDirectory,write} from './architecture-lab.mjs';
import {displayStack,resumeDisplayStack} from './architecture-display-lab.mjs';
import {openScreens,readyStatistics,sdk} from './architecture-display-load.mjs';

const phaseNames=['read_request_authorization','read_upstream','read_delivery_authorization','queries'];
const percentile=(values,p)=>[...values].sort((a,b)=>a-b)[Math.ceil(p*values.length)-1];
async function counters(stack){
  const response=await fetch(stack.runtime+'/internal/metrics');assert(response.ok);
  const data=await response.json();return data.measurements;
}
function delta(before,after){
  const result={};
  for(const name of phaseNames){
    const a=before[name],b=after[name];if(!b)continue;
    const count=b.count-(a?.count||0),failures=b.failures-(a?.failures||0),ms=b.total_ms-(a?.total_ms||0);
    result[name]={count,failures,totalMS:ms,meanMS:count?ms/count:null,processMaxMS:b.max_ms};
  }
  return result;
}
export async function displayProfile(dir,side,releaseDirectory){
  const source=read(join(dir,'private/display-snapshot.json')).path;
  const evidence={side,release:side==='a'?read(join(dir,'baseline.json')).commit:read(join(releaseDirectory,'candidate.json')).inputs.sourceSHA256,started:new Date().toISOString(),groups:[],passed:false};
  const file=join(dir,'load/display','profile-'+side+'-'+evidence.release.slice(0,12)+'.json');
  if(existsSync(file)){
    const previous=read(file);
    const archive=join(dir,'load/display/attempts');
    mkdirSync(archive,{recursive:true,mode:0o700});
    renameSync(file,join(archive,'profile-'+side+'-'+previous.started.replaceAll(':','-')+'.json'));
  }
  let stack,close,initial;
  try{
    const retained=process.env.ARC_DISPLAY_PROFILE_WORKSPACE;
    stack=retained?await resumeDisplayStack(dir,source,resolve(retained),side,{releaseDirectory}):await displayStack(dir,source,side,0,{releaseDirectory});
    initial=await counters(stack);
    close=await openScreens(stack);const sessions=stack.s.sessions;
    assert.equal(sessions.length,10);
    // This is a hot-read diagnostic. The timed scenario always includes a
    // separate ten-client cold-cache wave and requires all ten to succeed.
    evidence.warm=[];
    for(const [i,session] of sessions.entries()){
      const started=performance.now();
      await readyStatistics(stack,session,'profile-warm-'+i,true);
      evidence.warm.push({screen:i,ms:performance.now()-started});write(file,evidence);
    }
    for(const stage of ['hot','directory']){
      for(let pass=0;pass<3;pass++){
        const before=await counters(stack),time=performance.now();
        const values=await Promise.all(sessions.map(async(session,i)=>{
          const started=performance.now(),generation='qualified-'+stage+'-'+pass+'-'+i;
          if(stage==='hot')await readyStatistics(stack,session,generation,false);
          else{
            const page=await sdk(stack,session,'resources.query',{target:{view:'r'},limit:50},generation,AbortSignal.timeout(120000));
            assert.equal(page.data.total,'2600');assert.equal(page.data.items.length,50);
          }
          return performance.now()-started;
        }));
        evidence.groups.push({stage,pass,concurrency:10,elapsedMS:performance.now()-time,meanMS:values.reduce((a,b)=>a+b,0)/values.length,
          p95MS:percentile(values,.95),values,phases:delta(before,await counters(stack))});
        write(file,evidence);
      }
    }
    evidence.passed=evidence.groups.length===6&&evidence.groups.every(g=>g.values.length===10&&g.p95MS<=2000);
  }catch(e){evidence.error=e.message.replaceAll(/https?:\/\/[^\s]+/g,'[isolated URL]').slice(0,300);}
  finally{
    if(initial){try{evidence.measurements=delta(initial,await counters(stack));}catch{evidence.metricsUnavailable=true;}}
    await close?.();await stack?.close();evidence.finished=new Date().toISOString();write(file,evidence);}
  return evidence;
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const dir=runDirectory(process.argv[2]);const v=await displayProfile(dir,process.argv[3]||'b',runDirectory(process.argv[4]||process.argv[2]));
  console.log(v.side,v.passed,v.groups.map(g=>[g.stage,g.pass,Math.round(g.p95MS)]),v.error||'');process.exitCode=v.passed?0:1;
}
