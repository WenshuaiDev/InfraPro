import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative } from 'node:path';
import { test } from 'node:test';
import { installDisplayBoundarySetup } from '../../scripts/architecture-display-boundary-setup.mjs';
import { digest, read, root, write } from '../../scripts/architecture-lab.mjs';
import { membershipResult, membershipRow, qualifyMembership } from '../../scripts/architecture-membership-result.mjs';

const testName='TestDisplayReadFiftyThousandMemberBoundary';
const command=['pnpm','test:display:load','--boundary'];
const nativeCommand=['go','test','-tags','monitoringintegration','-count=1','-v','-timeout=25m','./packages/go/platform/resources','-run','^'+testName+'$'];
const stamps={baseline:'a'.repeat(64),candidate:'b'.repeat(64)};

function activity(t) {
  const dir=mkdtempSync(join(tmpdir(),'architecture-member-gate-'));
  t.after(()=>rmSync(dir,{recursive:true,force:true}));
  const files={};
  for(const name of ['display_read_load_test.go','display_scope.go','display_reads.go']){
    const path='packages/go/platform/resources/'+name;files[path]=digest(readFileSync(join(root,path)));
  }
  write(join(dir,'baseline.json'),{source:root,source_archive_sha256:stamps.baseline,sourceFiles:files});
  write(join(dir,'candidate.evaluated.json'),{directory:join(dir,'release'),inputs:{sourceSHA256:stamps.candidate}});
  write(join(dir,'release/application-source.json'),{application_source_sha256:stamps.candidate,files});
  let tick=0;
  function execute(side,profile,state='passed'){
    const parent=join(dir,'regression',side);
    const folder=profile==='default'?parent:join(parent,'membership-planners',profile);
    const stamp=new Date(Date.UTC(2026,0,1,0,0,++tick*2)).toISOString();
    const finished=new Date(Date.parse(stamp)+1000).toISOString();
    const archive='evidence/test-display-load/'+stamp.replaceAll(':','-'), file=join(folder,'test-display-load.json');
    const trace=join(dir,'private/regression-storage',side+'-monitoring/membership-fixtures',stamp.replaceAll(':','-'));
    // Use the same original SQL and assertion extraction as the real runner.
    write(join(trace,'input.json'),{synthetic:true});
    const recipe=installDisplayBoundarySetup(root,trace,{},trace,profile);
    const values={enable_nestloop:recipe.planner.enableNestloop,jit:recipe.planner.jit,statement_timeout:'10s',lock_timeout:'5s'};
    const settings=[],plans=[],passed=state==='passed';
    for(const count of passed?[49999,50000]:[49999])for(const stage of ['before-analyze','after-analyze']){
      const path=join(trace,count+'-settings-'+stage+'.json');write(path,values);
      settings.push({points:count,stage,observed:values,path:relative(folder,path)});
      for(const [query,prefix,hash] of [['members','',recipe.previewSQLSHA256],['preview-directory','directory-',recipe.directorySQLSHA256]]){
        const plan=join(trace,count+'-'+prefix+stage+'.json');
        write(plan,[{Plan:{'Node Type':'Limit','Plan Rows':10}}]);
        plans.push({points:count,stage,query,sqlSHA256:hash,path:relative(folder,plan),sha256:digest(readFileSync(plan)),root:'Limit',estimatedRows:10});
      }
    }
    const bounds=[[50000,1,49999,100],[50001,1,50000,1]];
    let text='=== RUN   '+testName+'\n    display_read_load_test.go:140: constraint-checked membership fixture total=50000 roots=1 points=49999 committed_batches=100\n';
    if(passed)text+='    display_read_load_test.go:143: member boundary 50000 resolved in 1s\n'+
      '    display_read_load_test.go:148: constraint-checked membership fixture total=50001 roots=1 points=50000 committed_batches=1\n'+
      '    display_read_load_test.go:158: 50000 accepted with exact total; 50001 preview and runtime rejected\n--- PASS: '+testName+' (1s)\n';
    else text+='    display_read_load_test.go:143: ERROR: '+(state==='timeout'?'canceling statement due to statement timeout (SQLSTATE 57014)':'deadlock detected (SQLSTATE 40P01)')+'\n--- FAIL: '+testName+' (1s)\n';
    const nativeFile=archive+'/member-boundary-result.json';
    write(join(folder,nativeFile),{command:nativeCommand,completedAt:finished,passed});
    const log='test-display-load.log', nativeLog=archive+'/member-boundary.log';
    const services=['accounts-postgres','accounts-redis','postgres','ch1','ch2','keeper','s3','backup-s3','audit-volume'];
    const ready=services.map(name=>' Container arc-reg-test-'+side+'-monitoring-'+name+'-1 Healthy').join('\n')+'\n';
    writeText(join(folder,log),ready+text);writeText(join(folder,nativeLog),text);
    const artifacts={
      'member-boundary-result.json':{archived:nativeFile,sha256:digest(readFileSync(join(folder,nativeFile)))},
      'member-boundary.log':{archived:nativeLog,sha256:digest(text)},
    };
    const common={command,startedAt:stamp,finishedAt:finished,durationMs:1000,exitCode:passed?0:1};
    const artifact=archive+'/summary.json';
    write(join(folder,artifact),{...common,releaseSHA256:stamps[side],passed,mode:'member-boundary',files:artifacts,
      go:{tests:passed?[{name:testName}]:[]},
      memberBoundary:{acceptedMembers:50000,rejectedMembers:50001,fixture:{...recipe,settings,plans,memberships:passed?bounds:bounds.slice(0,1)}}});
    const source={sourceSHA256:stamps[side],expectedSHA256:stamps[side],matchesSelectedRelease:true};
    write(file,{...common,status:passed?'passed':'failed',log,provenance:{role:side,releaseSHA256:stamps[side],
      storageProject:'arc-reg-test-'+side+'-monitoring',monitoringStorage:{project:'arc-reg-test-'+side+'-monitoring'},membershipFixture:recipe},
      originalLoadEvidence:{passed,artifact},sourceAtStart:source,sourceAtCompletion:source});
    return {file,default:read(file),folder,artifact:join(folder,artifact),trace,native:join(folder,nativeFile)};
  }
  return {dir,execute};
}

function writeText(file,text){mkdirSync(dirname(file),{recursive:true});writeFileSync(file,text);}

test('both untouched default successes need no fallback',async t=>{
  const {dir,execute}=activity(t);
  execute('baseline','default');execute('candidate','default');
  const outcome=await qualifyMembership(dir,()=>assert.fail('matched was not required'));
  assert.equal(outcome.mode,'both-default');assert.equal(outcome.passed,true);
});

test('a selected frozen timeout runs fresh matched originals on both sides and retains the raw failure',async t=>{
  const {dir,execute}=activity(t);
  const raw=execute('baseline','default','timeout');execute('candidate','default');
  const invoked=[];
  const result=await qualifyMembership(dir,async(_dir,side,suites,options)=>{
    assert.deepEqual(suites,['test:display:load']);assert.deepEqual(options,{stopStorage:true,membershipPlan:'matched'});
    invoked.push(side);execute(side,'matched');
  });
  assert.deepEqual(invoked,['baseline','candidate']);assert(result.passed);assert.equal(result.defaults.baseline.sqlState,'57014');
  const final=membershipRow(dir,'baseline',read(raw.file));
  assert.equal(final.status,'passed');assert.equal(final.defaultOutcome.status,'failed');
  assert.deepEqual(read(raw.file),raw.default);
  assert.equal((await qualifyMembership(dir,()=>assert.fail('receipts are current'))).passed,true);
  execute('candidate','default');assert.equal(membershipResult(dir).passed,false,'An earlier paired run cannot qualify a fresh default benchmark');
});

test('a new candidate lacking native default sources, a frozen non-timeout, or a candidate timeout cannot promote matched results',async t=>{
  for(const failure of ['unrelated baseline','candidate timeout','source changed']){
    await t.test(failure,async t=>{
      const {dir,execute}=activity(t);
      execute('baseline','default',failure==='unrelated baseline'?'deadlock':'timeout');
      const row=execute('candidate','default',failure==='candidate timeout'?'timeout':'passed');
      execute('baseline','matched');execute('candidate','matched');
      if(failure==='source changed'){const prior=read(row.file);prior.sourceAtStart.sourceSHA256=stamps.baseline;write(row.file,prior);}
      const result=await qualifyMembership(dir,()=>assert.fail('Invalid defaults must not start a scoped retry'));
      assert.equal(result.passed,false);
      assert.equal(result.mayUseMatched,false);
    });
  }
});

test('the actual transaction, query shape, fixture counts and raw original body are required',async t=>{
  for(const corruption of ['wrong settings','different query','wrong count','changed case']){
    await t.test(corruption,t=>{
      const {dir,execute}=activity(t); execute('baseline','default','timeout');execute('candidate','default');execute('baseline','matched');
      const row=execute('candidate','matched');
      assert(membershipResult(dir).passed);
      if(corruption==='wrong settings'){
        const plan=join(row.trace,'50000-settings-after-analyze.json');const x=read(plan);x.statement_timeout='30s';write(plan,x);
      }else if(corruption==='different query'){
        const x=read(row.artifact);x.memberBoundary.fixture.plans[0].sqlSHA256='0'.repeat(64);write(row.artifact,x);
      }else if(corruption==='wrong count'){
        const x=read(row.artifact);x.memberBoundary.acceptedMembers=49999;write(row.artifact,x);
      }else{
        const x=read(row.file);x.provenance.membershipFixture.originalAssertionsSHA256='0'.repeat(64);write(row.file,x);
      }
      assert.equal(membershipResult(dir).passed,false);
    });
  }
});
