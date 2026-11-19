import assert from 'node:assert/strict';
import { copyFileSync, existsSync, mkdirSync } from 'node:fs';
import { basename,join,resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fileLease,read,runDirectory,safeDiagnosticLine,sleep,write } from './architecture-lab.mjs';
import { displayStack, retainDisplayStorage } from './architecture-display-lab.mjs';
import { historyInputs,historyJobs } from './architecture-display-history.mjs';
import { openScreens,personnel,sdk,readyStatistics,checkCurve,displayFailure } from './architecture-display-load.mjs';
import { prepareLargeSource,readLargeSource } from './architecture-display-stream.mjs';

// Check actual SDK grants, signatures, full result counts and all six kinds
// of historical job before timing the cold and warm concurrent cohorts.
export async function displayPreflight(dir,side,planner='matched',releaseDirectory=dir){
  const unlock=await fileLease(join(dir,'load/display/campaign.lock'),{wait:false});
  try{return await runPreflight(dir,side,planner,releaseDirectory);}finally{await unlock();}
}
async function runPreflight(dir,side,planner,releaseDirectory){
  assert(['a','b'].includes(side));assert(['default','matched'].includes(planner));
  const snapshot=read(join(dir,'private/display-snapshot.json')).path;
  const version=side==='b'?read(join(releaseDirectory,'candidate.json')).inputs.sourceSHA256:read(join(dir,'baseline.json')).commit;
  const result={side,planner,version,started:new Date().toISOString(),stages:[],passed:false};
  const output=join(dir,'load/display','preflight-'+side+'-'+planner+'-'+version.slice(0,12)+'.json');
  if (existsSync(output)) {
    const previous=read(output),archive=join(dir,'load/display/attempts','preflight-'+side+'-'+previous.started.replaceAll(':','-'));
    mkdirSync(archive,{recursive:true,mode:0o700});copyFileSync(output,join(archive,'result.json'));
  }
  const save=()=>write(output,result);
  const step=async(name,work)=>{
    const start=Date.now(),stage={name,startedAt:new Date(start).toISOString(),state:'running'};
    result.stages.push(stage);save();
    try{const evidence=await work();stage.state='passed';stage.evidence=evidence;}
    catch(e){stage.state='failed';stage.error=displayFailure(e);throw e;}
    finally{stage.ms=Date.now()-start;save();}
  };
  let stack,closeScreens,user,seed;
  try{
    await step('restore fixed benchmark database, objects and physical columns',async()=>{
      stack=await displayStack(dir,snapshot,side,0,{planner,releaseDirectory});
      return {workspace:basename(stack.workspace),settings:stack.planner,objectLimit:stack.budget.objectRequests.limit,roleLogins:stack.budget.roleLogins};
    });
    await step('issue a fresh personnel grant and SDK session',async()=>{
      user=await personnel(stack);closeScreens=await openScreens(stack,1);return{sessions:stack.s.sessions.length};
    });
    await step('read a scoped directory on the actual SDK',async()=>{
      const durations=[];
      for(let n=0;n<3;n++){
        const start=performance.now();
        const value=await sdk(stack,stack.s.sessions[0],'resources.query',{target:{view:'r'},limit:50},'preflight-directory-'+n,AbortSignal.timeout(120000));
        assert.equal(value.data.total,'2600');assert.equal(value.data.items.length,50);durations.push(performance.now()-start);
      }
      return {total:2600,pageSize:50,durationsMs:durations};
    });
    await step('read a verified cold SDK result and the complete personnel curve',async()=>{
      const prefix='/tenants/'+stack.s.tenant+'/monitoring';
      const selection=await user.call(prefix+'/selections',{channel_ids:[stack.s.channel]});
      const result=await user.call(prefix+'/queries/trends',{selection_id:selection.id,all_history:true,qualities:['good'],timezone:'UTC',limit:200});
      const curve=checkCurve(result,stack.s);
      return {curve,statistic:await readyStatistics(stack,stack.s.sessions[0],'preflight-cold',true)};
    });
    await step('receive two native upload parts and stream every authorized byte',async()=>{
      const source=await prepareLargeSource(user,'/tenants/'+stack.s.tenant+'/monitoring');
      return readLargeSource(user,source);
    });
    await step('publish the fixed formula, trial and immutable generations',async()=>{
      seed=await historyInputs(stack,user);return{outputRows:10,roles:stack.budget.roles};
    });
    await step('run every history queue category to a successful terminal state',async()=>{
      const desired=await historyJobs(seed,user);const ids=desired.map(v=>"'"+v.job_id+"'").join(',');
      const deadline=Date.now()+240000;
      while(Date.now()<deadline){
        const jobs=JSON.parse(stack.sql("SELECT json_agg(json_build_object('kind',kind,'state',state,'error',error_code)) FROM monitoring_jobs WHERE id IN ("+ids+")"));
        assert.equal(jobs.length,desired.length);
        const failed=jobs.find(v=>['failed','cancelled','needs_action'].includes(v.state));
        assert.ok(!failed,'History job '+failed?.kind+' '+failed?.error);
        if(jobs.every(v=>v.state==='succeeded'))return jobs;
        await sleep(350);
      }
      throw Error('Historical jobs did not drain within the preflight');
    });
    result.passed=true;
  }catch(e){result.failure=displayFailure(e);}
  finally{
    for (const [stage, close] of [
      ['personnel session', () => user?.close()],
      ['screen sessions', closeScreens],
      ['role processes', () => stack?.close()],
    ]) {
      try { await close?.(); }
      catch (error) {
        result.passed = false;
        (result.cleanupErrors ??= []).push({ stage, message: safeDiagnosticLine(error.message).slice(0,320) });
      }
    }
    if (stack) {
      try {
        const retained = retainDisplayStorage(stack);
        result.retained = { database: retained.database, tables: retained.tables.length };
      } catch (error) {
        result.passed = false;
        result.retentionError = safeDiagnosticLine(error.message).slice(0,320);
      }
    }
    result.finished=new Date().toISOString();save();
  }
  return result;
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const value=await displayPreflight(runDirectory(process.argv[2]),process.argv[3],process.argv[4]||'matched',runDirectory(process.argv[5]||process.argv[2]));
  console.log(value.side,value.passed?'passed':value.failure);
  process.exitCode=value.passed?0:1;
}
