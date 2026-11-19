import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { compareQuotaReport, resourceLimits, writeResourceLimits } from './architecture-capacities.mjs';
import { prepare } from './architecture-deploy.mjs';
import { processEnvironment } from './architecture-process-resources.mjs';
import { root } from './architecture-runtime.mjs';

function site(t, options={}) {
  const temp=mkdtempSync(join(tmpdir(),'infrapro-resource-table-'));
  t.after(()=>rmSync(temp,{recursive:true,force:true}));
  const path=join(temp,'installation');
  const file=(name,settings)=>{const dest=join(temp,name);writeFileSync(dest,JSON.stringify(settings));return dest;};
  const settings={};
  for(const [key,value] of Object.entries(options)) settings[key]=file(key+'.json',value);
  prepare({directory:path,mode:'native',...settings});
  return {path,file};
}
const find=(report,role)=>report.slots.filter(row=>row.role===role);
const single=(report,role)=>find(report,role)[0];

test('single production pool, same-schema rollback and original storage processes',t=>{
  const {path}=site(t);
  const report=JSON.parse(readFileSync(join(path,'resource-limits.json')));
  assert.equal(report.totals.split_application.ch_connections,27);
  assert.equal(report.totals.same_schema_compatibility_additions.ch_connections,10);
  assert.equal(report.totals.independent_edge.pg_connections,8);
  assert.equal(single(report,'api').http_inflight,92);
  assert.equal(single(report,'api').pg_connections,0);
  assert.equal(single(report,'display-runtime').clickhouse.max_open,0);
  assert.equal(single(report,'monitoring-read-worker').http_inflight,null);
  assert.equal(single(report,'monitoring-read-worker').work_limits.display_read_jobs_shared,1);
  assert.equal(single(report,'parser').work_limits.parser_executions,4);
  assert.equal(report.original_schema58_storage_processes.find(r=>r.role==='display-runtime').pg_connections,8);
  assert(report.original_schema58_storage_processes.every(r=>r.clickhouse.max_open===6 && r.architecture_http_gate===null));
  assert.equal(report.manual_storage_command.clickhouse.max_open,6);
});

test('per-slot division, mounted overrides, same host serving two pools, and independent S0 settings',t=>{
  const {path,file}=site(t,{replicas:{api:['gateway-east','gateway-west'],'transfer-api':['transfer-east','transfer-west']},
    'max-inflight':{'transfer-api':5}});
  const settings=file('resolved.json',{defaults:{monitoring_clickhouse_replicas:'ch:9000',
    monitoring_physical_shards:JSON.stringify([{name:'archive',database:'archive',replicas:['ch:9000']}]),
    monitoring_clickhouse_password:'private-placeholder'},schema58:{'spatial-worker':{monitoring_clickhouse_replicas:'',monitoring_physical_shards:''},
    'display-runtime':{display:{pool_size:24}}}});
  const result=resourceLimits(path,{'runtime-config':settings});
  assert.deepEqual(find(result,'transfer-api').map(r=>[r.slot_max_inflight,r.http_inflight,r.clickhouse.max_open_per_pool,r.clickhouse.max_open]),[[3,3,1,2],[2,2,1,2]]);
  assert.deepEqual(find(result,'api').map(r=>r.http_inflight),[43,42]);
  assert.equal(single(result,'control-api').clickhouse.max_open,12);
  assert.equal(result.original_schema58_storage_processes.find(r=>r.role==='api').clickhouse.max_open,12);
  assert.equal(result.original_schema58_storage_processes.find(r=>r.role==='spatial-worker').clickhouse.max_open,0);
  assert.equal(result.original_schema58_storage_processes.find(r=>r.role==='display-runtime').pg_connections,24);
  assert(!JSON.stringify(result).includes('private-placeholder'));
  assert(!JSON.stringify(result).includes('ch:9000'));
  const arch=join(path,'roles/transfer-api/architecture.json');
  const changed=JSON.parse(readFileSync(arch));changed.max_inflight['transfer-api']=9;writeFileSync(arch,JSON.stringify(changed));
  assert.equal(single(resourceLimits(path,{'runtime-config':settings}),'transfer-api').clickhouse.max_open,4);
});

test('one released knob affects both the HTTP gate and driver pools',t=>{
  const {path,file}=site(t,{'max-inflight':{'transfer-api':2,'monitoring-incremental-worker':1}});
  const declared=resourceLimits(path);
  const transfer=single(declared,'transfer-api'),incremental=single(declared,'monitoring-incremental-worker');
  assert.deepEqual([transfer.http_inflight,transfer.clickhouse.max_open,incremental.http_inflight,incremental.clickhouse.max_open],[2,2,1,1]);
  assert.throws(()=>resourceLimits(path,{'runtime-config':file('two.json',{monitoring_clickhouse_replicas:'c1:9010,c2:9010'})}),/below physical replica count/);
  const disabled=resourceLimits(path,{'runtime-config':file('disabled.json',{monitoring_clickhouse_replicas:''})});
  assert.equal(disabled.totals.split_application.ch_connections,0);
  assert.equal(single(disabled,'transfer-api').http_inflight,2);
});

test('verify process identity, PG, CH, HTTP and attached object limits',t=>{
  const {path,file}=site(t);
  const configured=single(resourceLimits(path),'monitoring-incremental-worker');
  const report={phase:'ready',replica_id:configured.id,process:{pid:1001,boot_id:'b3b5e19f-ccb3-49fd-8446-a0db4e2963fb'},
    metrics:[{key:'quota.pg_connections',value:10},{key:'quota.ch_connections',value:3},
      {key:'quota.http_inflight',value:3},{key:'quota.s3_inflight',value:3}]};
  const map=file('index.json',{[configured.id]:file('ready.json',report)});
  const output=writeResourceLimits(path,{'report-map':map});
  assert.equal(output.checked_reports,1);
  assert.equal(single(output,configured.role).process_report.observed['quota.ch_connections'],3);
  assert.throws(()=>compareQuotaReport(configured,{...report,replica_id:'unrelated-1'}),/ready process/);
  assert.throws(()=>compareQuotaReport(configured,{...report,metrics:report.metrics.slice(2)}),/missing quota/);
  report.metrics[1].value=4;
  assert.throws(()=>compareQuotaReport(configured,report),/mismatched quota.ch_connections/);
});

test('reject unknown role, impossible slot count, duplicate physical replica and PG/login mismatch',t=>{
  const {path,file}=site(t);
  assert.throws(()=>resourceLimits(path,{'runtime-config':file('dupes.json',{monitoring_clickhouse_replicas:'ch:9000,CH:9000'})}),/duplicate/i);
  assert.throws(()=>site(t,{'max-inflight':{'typo-api':2}}),/Invalid in-flight role/);
  assert.throws(()=>site(t,{replicas:{'transfer-api':['t-east','t-west']},'max-inflight':{'transfer-api':1}}),/exceeds quota/);
  const arch=join(path,'roles/control-api/architecture.json');
  const changed=JSON.parse(readFileSync(arch));changed.pg_connections['control-api']=4;writeFileSync(arch,JSON.stringify(changed));
  assert.throws(()=>resourceLimits(path),/provisioned login budget/);
});

test('equal CPU/Go memory site profile and S0 drop-ins across deployment modes',t=>{
  const base=mkdtempSync(join(tmpdir(),'infrapro-process-profile-'));
  t.after(()=>rmSync(base,{recursive:true,force:true}));
  const profiles=join(root,'deploy/architecture/display-processes.example.json');
  const inflight=join(root,'deploy/architecture/display-inflight.example.json');
  for(const mode of ['native','compose','mixed']){
    const dir=join(base,mode);
    prepare({mode,directory:dir,'process-budgets':profiles,'max-inflight':inflight,'container-roles':'monitoring-query-api'});
    const limits=resourceLimits(dir),summary=limits.selected_process_profiles;
    for(const totals of [summary.split,summary.schema58])
      assert.deepEqual([totals.cpu_cores,totals.memory_mib,totals.go_threads,totals.go_memory_mib],[13,2560,13,2048]);
    assert.equal(single(limits,'identity-api').process_resources.go_threads,2);
    assert.equal(single(limits,'monitoring-query-api').process_resources.go_memory_mib,384);
    if(mode!=='compose'){
      const service=readFileSync(join(dir,'units/infrapro-identity-api.service'),'utf8');
      for(const assignment of ['GOMAXPROCS=2','GOMEMLIMIT=256MiB','CPUQuota=200.00%','MemoryMax=320M']) assert(service.includes(assignment));
    }
    if(mode!=='native'){
      const service=JSON.parse(readFileSync(join(dir,'compose.json'))).services['monitoring-query-api'];
      assert.deepEqual([service.cpus,service.mem_limit,service.environment.GOMAXPROCS,service.environment.GOMEMLIMIT],[2,'480m','2','384MiB']);
    }
    const old=readFileSync(join(dir,'schema58/systemd/infrapro-api.service.d/30-resource-limits.conf'),'utf8');
    assert(old.includes('CPUQuota=500.00%') && old.includes('Environment=GOMAXPROCS=5'));
    const oldCompose=JSON.parse(readFileSync(join(dir,'schema58/compose-limits.json'))).services;
    assert.equal(oldCompose['monitoring-worker'].environment.GOMEMLIMIT,'768MiB');
  }
});

test('same role divides CPU, Go scheduler threads and memory across independent processes',t=>{
  const {path}=site(t,{replicas:{'identity-api':['identity-east','identity-west']},
    'process-budgets':{'identity-api':{cpu:2.0001,memory_mib:513,go_threads:3,go_memory_mib:321}}});
  const instances=JSON.parse(readFileSync(join(path,'layout.json'))).instances['identity-api'];
  assert.deepEqual(instances.map(i=>[i.cpu,i.memoryMiB,i.goThreads,i.goMemoryMiB]),[[1.0001,257,2,161],[1,256,1,160]]);
  assert.deepEqual(processEnvironment(instances[0],{GOMAXPROCS:'8',GOMEMLIMIT:'1GiB'}),{GOMAXPROCS:'2',GOMEMLIMIT:'161MiB'});
  assert.deepEqual(processEnvironment({}, {GOMAXPROCS:'8'}),{GOMAXPROCS:'8'});
  assert.throws(()=>site(t,{replicas:{'identity-api':['id-east','id-west']},'process-budgets':{'identity-api':{go_threads:1}}}),/below slot count/);
  assert.throws(()=>site(t,{'process-budgets':{'identity-api':{memory_mib:128,go_memory_mib:256}}}),/exceeds/);
});
