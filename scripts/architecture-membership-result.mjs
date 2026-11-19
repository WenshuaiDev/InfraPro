import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { digest, fileLease, read, root, write } from './architecture-lab.mjs';
import { membershipPlanner } from './architecture-display-boundary-setup.mjs';

const suite = 'test:display:load';
const command = ['pnpm',suite,'--boundary'];
const test = 'TestDisplayReadFiftyThousandMemberBoundary';
const goCommand = ['go','test','-tags','monitoringintegration','-count=1','-v','-timeout=25m',
  './packages/go/platform/resources','-run','^'+test+'$'];
const counts = [[50000,1,49999,100],[50001,1,50000,1]];
const stages = ['before-analyze','after-analyze'];
const sources = ['display_read_load_test.go','display_scope.go','display_reads.go'];
const policyPath = 'regression/membership-planners/qualification.json';

function locations(dir,side,profile) {
  const parent=join(dir,'regression',side);
  const base=profile==='default'?parent:join(parent,'membership-planners',profile);
  return {base,receipt:join(base,'test-display-load.json')};
}

function trustedInputs(dir,side) {
  const baseline=read(join(dir,'baseline.json'));
  const release=side==='candidate'?read(join(dir,'candidate.evaluated.json')):null;
  const selected=release?.inputs.sourceSHA256??baseline.source_archive_sha256;
  const manifest=release?read(join(release.directory,'application-source.json')):null;
  if(release)assert.equal(manifest.application_source_sha256,selected);
  const files=manifest?.files??baseline.sourceFiles;
  const cwd=release?root:baseline.source;
  const [go,scope,directory]=sources.map(name=>{
    const path='packages/go/platform/resources/'+name;
    const bytes=readFileSync(join(cwd,path));
    assert.equal(digest(bytes),files[path],'Membership source no longer matches its selected release');
    return bytes.toString('utf8');
  });
  const start=go.indexOf('func '+test+'('), helper=go.indexOf('func displayBulkPoints(');
  assert(start>helper && helper>=0);
  const source=go.slice(start).split('\nfunc ',1)[0];
  const insert=/f\.env\.Pool\.Exec\(ctx, (`WITH inserted AS [^`]+`), f\.tenant, facility, f\.actor\.ID, offset\+1, offset\+count, p\.id\)/.exec(go.slice(helper,start))?.[1];
  const preview=/tx\.Query\(ctx, `(WITH visible AS MATERIALIZED [^`]+LIMIT 50001)`/.exec(scope)?.[1];
  const metadata=/query := strings\.ReplaceAll\(`(WITH RECURSIVE nodes AS MATERIALIZED[^`]+)`, "engineering_resource_nodes", nodes\)/.exec(directory)?.[1];
  assert(insert && preview && metadata);
  assert(source.includes('preview.MemberCount != "50000"') && source.includes('complete.Totals.PointCount != "49999"') &&
    source.match(/problem\.Code != "budget_exceeded"/g)?.length===2 && source.includes('h.s.Query(ctx, h.session'));
  return {release:selected,originalAssertionsSHA256:digest(go.slice(start)),goCaseSHA256:digest(source),
    insertSHA256:digest(insert),previewSQLSHA256:digest(preview),directorySQLSHA256:digest(metadata)};
}

function inspect(dir,side,profile) {
  const {base,receipt}=locations(dir,side,profile);
  const file=relative(dir,receipt);
  if(!existsSync(receipt))return {side,profile,file,state:'missing'};
  try {
    const expected=trustedInputs(dir,side), row=read(receipt);
    assert.deepEqual(row.command,command);
    assert.equal(row.provenance.role,side);assert.equal(row.provenance.releaseSHA256,expected.release);
    for(const stamp of [row.sourceAtStart,row.sourceAtCompletion])
      assert(stamp?.matchesSelectedRelease && stamp.sourceSHA256===expected.release && stamp.expectedSHA256===expected.release);
    const p=row.provenance.membershipFixture;
    assert(p && p.originalAssertionsSHA256===expected.originalAssertionsSHA256 && p.insertSHA256===expected.insertSHA256 &&
      p.previewSQLSHA256===expected.previewSQLSHA256 && p.committedBatchSize===500 && p.constraints==='original schema');
    const plan=membershipPlanner(profile);
    // The original default replay predates the additional per-transaction
    // planner probes. It can establish its own failure only, never a pass.
    const oldDefault=side==='baseline' && profile==='default' && row.status==='failed' && !p.planner && !p.directorySQLSHA256;
    if(!oldDefault){assert.deepEqual(p.planner,plan);assert.equal(p.directorySQLSHA256,expected.directorySQLSHA256);}
    const duration=Date.parse(row.finishedAt)-Date.parse(row.startedAt);
    assert(Number.isFinite(duration)&&duration>0&&Math.abs(row.durationMs-duration)<=2);
    const physical=row.provenance.monitoringStorage;
    assert(physical?.project===row.provenance.storageProject && physical.project.endsWith('-'+side+'-monitoring'));
    const artifact=resolve(base,row.originalLoadEvidence.artifact);
    assert(relative(base,artifact).startsWith('evidence/test-display-load/'));
    const summary=read(artifact);
    assert.deepEqual(summary.command,command);assert.equal(summary.releaseSHA256,expected.release);
    for(const key of ['startedAt','finishedAt','durationMs','exitCode'])assert.equal(summary[key],row[key]);
    assert.equal(summary.mode,'member-boundary');
    const archived=summary.files['member-boundary-result.json'];
    const nativeFile=resolve(base,archived.archived);
    assert.equal(dirname(nativeFile),dirname(artifact));assert.equal(digest(readFileSync(nativeFile)),archived.sha256);
    const native=read(nativeFile);
    assert.deepEqual(native.command,goCommand);
    assert(Date.parse(native.completedAt)>=Date.parse(row.startedAt) && Date.parse(native.completedAt)<=Date.parse(row.finishedAt));
    const logFile=join(base,row.log);assert.equal(logFile,join(base,'test-display-load.log'));
    const transcript=readFileSync(logFile,'utf8');
    const components=['accounts-postgres','accounts-redis','postgres','ch1','ch2','keeper','s3','backup-s3','audit-volume'];
    const healthy=new Set([...transcript.matchAll(/^\s*Container (\S+) Healthy\s*$/gm)].map(match=>match[1]));
    assert(components.every(name=>healthy.has(physical.project+'-'+name+'-1')),'The original nine service readiness checks are incomplete');
    assert.deepEqual([...transcript.matchAll(/^=== RUN\s+(\S+)/gm)].map(x=>x[1]),[test]);
    const fixtures=[...transcript.matchAll(/constraint-checked membership fixture total=(\d+) roots=(\d+) points=(\d+) committed_batches=(\d+)/g)].map(x=>x.slice(1).map(Number));
    const observed={side,profile,file,receiptSHA256:digest(readFileSync(receipt)),source:expected.release,
      log:relative(dir,logFile),logSHA256:digest(transcript),summary:relative(dir,artifact),summarySHA256:digest(readFileSync(artifact)),
      original:relative(dir,nativeFile),originalSHA256:archived.sha256,startedAt:row.startedAt,finishedAt:row.finishedAt,
      durationMs:row.durationMs,exitCode:row.exitCode,input:expected,counts:fixtures,storage:physical.project};
    const matchedSettings={enable_nestloop:plan.enableNestloop,jit:plan.jit,statement_timeout:'10s',lock_timeout:'5s'};
    if(side==='baseline'&&profile==='default'&&row.status==='failed'&&row.exitCode===1&&!native.passed&&!summary.passed) {
      const fail = transcript.split('=== RUN   '+test)[1]?.split('--- FAIL: '+test)[0];
      assert(fail && transcript.includes('--- FAIL: '+test+' (') && fixtures.length===1 && isDeepStrictEqual(fixtures[0],counts[0]) &&
        !fail.includes('member boundary 50000 resolved') &&
        /display_read_load_test\.go:\d+: ERROR: canceling statement due to statement timeout \(SQLSTATE 57014\)/.test(fail));
      assert(!/--- (PASS|SKIP): |panic:|deadlock detected|lock timeout|database system is shutting down/i.test(fail));
      const partial=[];
      if(!oldDefault)for(const stage of stages){
        const path=join(p.traces,'49999-settings-'+stage+'.json');
        assert.deepEqual(read(path),matchedSettings);partial.push({file:relative(dir,path),sha256:digest(readFileSync(path))});
      }
      return {...observed,state:'original-default-timeout',sqlState:'57014',failedAtMembers:50000,
        recordedSettings:partial,oldDefault,observation:'Frozen preview did not return before its original PostgreSQL statement deadline.'};
    }
    assert(row.status==='passed'&&row.exitCode===0&&native.passed&&summary.passed===true&&row.originalLoadEvidence.passed);
    assert.equal(summary.go?.tests?.length,1);assert.equal(summary.go.tests[0].name,test);
    assert(transcript.includes('--- PASS: '+test+' (') && !transcript.includes('--- FAIL:') && !transcript.includes('--- SKIP:'));
    assert(transcript.includes('50000 accepted with exact total; 50001 preview and runtime rejected'));
    assert.deepEqual(fixtures,counts);
    const evidence=summary.memberBoundary;assert.equal(evidence.acceptedMembers,50000);assert.equal(evidence.rejectedMembers,50001);
    const fixture=evidence.fixture;assert.deepEqual(fixture.memberships,counts);
    for(const key of ['originalAssertionsSHA256','insertSHA256','previewSQLSHA256','directorySQLSHA256','traces','planner'])assert.deepEqual(fixture[key],p[key]);
    assert.equal(fixture.plans.length,8);assert.equal(fixture.settings.length,4);
    const traceRoot=join(dir,'private/regression-storage',side+'-monitoring','membership-fixtures');
    assert.equal(dirname(resolve(p.traces)),resolve(traceRoot));
    const records=[];
    for(const n of [49999,50000])for(const stage of stages){
      const setting=fixture.settings.find(v=>v.points===n&&v.stage===stage);
      const path=join(p.traces,n+'-settings-'+stage+'.json');
      assert.deepEqual(setting?.observed,matchedSettings);assert.equal(resolve(base,setting.path),resolve(path));
      assert.deepEqual(read(path),matchedSettings);
      records.push({file:relative(dir,path),sha256:digest(readFileSync(path)),kind:'settings',points:n,stage});
      for(const [query,prefix,sql] of [['members','',expected.previewSQLSHA256],['preview-directory','directory-',expected.directorySQLSHA256]]){
        const plan=fixture.plans.find(v=>v.points===n&&v.stage===stage&&v.query===query);
        const file=join(p.traces,n+'-'+prefix+stage+'.json');const bytes=readFileSync(file);
        assert(plan && plan.sqlSHA256===sql && resolve(base,plan.path)===resolve(file) && plan.sha256===digest(bytes));
        const physical=JSON.parse(bytes)[0]?.Plan;
        assert(physical?.['Node Type']===plan.root && physical['Plan Rows']===plan.estimatedRows);
        records.push({file:relative(dir,file),sha256:plan.sha256,kind:query,points:n,stage});
      }
    }
    const bulkLog = summary.files['member-boundary.log'];
    const goLog = resolve(base,bulkLog.archived);
    assert.equal(dirname(goLog),dirname(artifact));assert.equal(digest(readFileSync(goLog)),bulkLog.sha256);
    assert(readFileSync(goLog,'utf8').includes('50000 accepted with exact total; 50001 preview and runtime rejected'));
    return {...observed,state:'passed',settings:matchedSettings,physical:records,nativeGoLog:relative(dir,goLog),
      nativeGoLogSHA256:bulkLog.sha256,acceptedMembers:50000,runtimePointCount:49999,rejectedMembers:50001,
      rejection:'budget_exceeded',preview50000:transcript.match(/member boundary 50000 resolved in (\S+)/)?.[1]};
  } catch(error) {
    return {side,profile,file,state:'ineligible',reason:error.message.split('\n')[0].slice(0,240)};
  }
}

function sameAssertions(a,b) {
  return a.input?.goCaseSHA256===b.input?.goCaseSHA256 && a.input?.insertSHA256===b.input?.insertSHA256 &&
    a.input?.previewSQLSHA256===b.input?.previewSQLSHA256 && a.storage!==b.storage;
}

export function membershipResult(dir) {
  dir=resolve(dir);
  const baseline=inspect(dir,'baseline','default'),candidate=inspect(dir,'candidate','default');
  const common={command,defaults:{baseline,candidate},passed:false,mayUseMatched:false,evidencePath:policyPath};
  if(candidate.state!=='passed')return {...common,mode:'candidate-default-required'};
  if(baseline.state==='passed')return {...common,mode:'both-default',passed:sameAssertions(baseline,candidate)};
  if(baseline.state!=='original-default-timeout'||!sameAssertions(baseline,candidate))return {...common,mode:'baseline-default-required'};
  const a=inspect(dir,'baseline','matched'),b=inspect(dir,'candidate','matched');
  const notBefore=Math.max(Date.parse(baseline.finishedAt),Date.parse(candidate.finishedAt));
  const pair=a.state==='passed'&&b.state==='passed'&&sameAssertions(a,b)&&sameAssertions(a,candidate)&&
    a.source===baseline.source&&b.source===candidate.source&&a.input.directorySQLSHA256===baseline.input.directorySQLSHA256&&
    b.input.directorySQLSHA256===candidate.input.directorySQLSHA256&&isDeepStrictEqual(a.settings,b.settings)&&
    Date.parse(a.startedAt)>=notBefore&&Date.parse(b.startedAt)>=Date.parse(a.finishedAt);
  return {...common,mode:'both-matched-after-frozen-timeout',mayUseMatched:true,matched:{baseline:a,candidate:b},passed:pair};
}

export function membershipRow(dir,side,row) {
  const result=membershipResult(dir);
  if (side==='candidate' || result.mode==='both-default') {
    const record=result.defaults[side];
    return row.status==='passed'&&(record.state!=='passed'||(side==='baseline' && result.mode==='both-default' && !result.passed))
      ? {...row,status:'pending',priorStatus:row.status,reason:'Current source, original boundary assertions, input or database planning receipts are incomplete.'}
      : row;
  }
  if(!result.passed)return row.status==='passed'&&result.defaults[side].state!=='passed'
    ? {...row,status:'pending',priorStatus:row.status,reason:'Original member boundary evidence is incomplete.'}:row;
  const recorded=join(dir,policyPath);
  if (!existsSync(recorded) || !isDeepStrictEqual(read(recorded),result)) return row;
  const actual=result.matched.baseline;
  const matched=read(join(dir,actual.file));
  return {...matched,evidencePath:actual.file,qualificationEvidence:policyPath,
    defaultOutcome:{status:row.status,exitCode:row.exitCode,sqlState:result.defaults.baseline.sqlState,
      evidencePath:result.defaults.baseline.file},qualification:'Both backends passed with the same planner after a frozen default timeout.'};
}

// A fresh decision is made after both normal commands. A resume can reuse
// the pair only when both original inputs and the most recent defaults match.
export async function qualifyMembership(dir,run) {
  const release=await fileLease(join(dir,'private','membership-qualification.flock'));
  try {
    let result=membershipResult(dir);
    if(result.mayUseMatched&&!result.passed){
      for(const side of ['baseline','candidate'])
        await run(dir,side,[suite],{stopStorage:true,membershipPlan:'matched'});
      result=membershipResult(dir);
    }
    const destination=join(dir,policyPath);
    if(existsSync(destination)){
      const previous=read(destination);
      if(!isDeepStrictEqual(previous,result))
        write(join(dirname(destination),'attempts',digest(JSON.stringify(previous))+'.json'),previous);
    }
    write(destination,result);
    return result;
  } finally {await release();}
}
