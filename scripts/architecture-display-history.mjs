import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { analysisBody } from './architecture-analysis-parity.mjs';
import { sleep } from './architecture-lab.mjs';

const from=BigInt(Date.parse('2026-11-01T00:00:00Z'))*1000000n;
const range={from_ns:from.toString(),to_ns:(from+600000000000n).toString()};
const command=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'隔离混合负载历史任务'});
async function until(read,pick,accept,label) {
  const deadline=Date.now()+180000;
  while(Date.now()<deadline){
    const value=await read(),state=pick(value);
    if(accept(state))return value;
    if(['failed','cancelled','canceled','needs_action'].includes(state))throw Error(label+' '+state+' '+(value.job?.error_code||''));
    await sleep(180);
  }
  throw Error(label+' did not finish');
}

// Make signed, queryable historical inputs before the timed concurrent scene.
// The new derived channel is deliberately outside the display project's grant.
export async function historyInputs(stack,user){
  if(stack.historyInputs)return stack.historyInputs;
  const raw=JSON.parse(stack.sql("SELECT jsonb_build_object('metric',metric_version_id,'unit',unit_version_id,'facility',facility_id,'point',point_id) FROM engineering_channels WHERE id='"+stack.s.channel+"'"));
  const body=analysisBody({...raw,channels:{P1:{id:stack.s.channel}}});
  const d='/tenants/'+stack.s.tenant+'/monitoring/derived',analysis=d+'/analysis',code='ARC_HISTORY';
  const cfg=(await user.call(analysis+'/configs',{...command(),code,name:'历史负载计算',body})).config;
  const path=analysis+'/configs/'+cfg.id;
  const check=await user.call(path+'/validate',{draft_revision:cfg.draft.revision});assert.equal(check.issues.length,0);
  const version=(await user.call(path+'/versions',{...command(cfg.revision),draft_revision:cfg.draft.revision,validation_hash:check.validation_hash})).version;
  const runInput={config_version_id:version.id,config_hash:version.config_hash,output_range:range,outputs:['double']};
  const run=await user.call(analysis+'/runs',{...command(),...runInput,artifact_name:'冻结历史成果'});
  const fixed=await until(()=>user.call(d+'/runs/'+run.id),v=>v.job.state,v=>v==='succeeded','analysis input');
  const result=await user.call(analysis+'/artifacts/'+fixed.analysis.artifact_id+'?version_id='+fixed.analysis.artifact_version_id);
  assert.equal(result.version.output_rows,'10');
  const metric=(await user.call(d+'/metric-definitions',{...command(),code,name:'历史负载指标',description:'固定输入与精确结果',rule:body.rule})).definition;
  const metrics=d+'/metric-definitions/'+metric.id;
  const validation=await user.call(metrics+'/validate',{draft_revision:metric.draft.revision});
  const immutable=(await user.call(metrics+'/publish',{...command(metric.revision),draft_revision:metric.draft.revision,validation_hash:validation.validation_hash})).version;
  const channel=(await user.call(d+'/channels',{...command(),code,name:'历史负载派生量',owner:{kind:'point',id:raw.point,facility_id:raw.facility},definition_version_id:immutable.id,input_bindings:body.input_bindings,parameters:{}})).channel;
  const target=d+'/channels/'+channel.resource.id,compiled=(await user.call(target+'/configs')).items[0];
  const trial=await user.call(target+'/trials',{...command(channel.config_revision),config_id:compiled.id,config_hash:compiled.config_hash,output_range:range});
  const checked=await until(()=>user.call(d+'/runs/'+trial.id),v=>v.job.state,v=>v==='succeeded','derived trial');
  const preview=await user.call(target+'/control-preview',{expected_revision:channel.control_revision,action:'enable',auto_start_ns:range.from_ns,stop_ns:range.to_ns,trial_receipt_id:checked.trial.trial_receipt_id});
  const enabled=await user.call(target+'/control',{...command(channel.control_revision),preview_id:preview.id,preview_hash:preview.hash});
  assert.equal(enabled.channel.resource.id,channel.resource.id);
  const visible=await until(()=>user.call(target+'/generations?from_ns='+range.from_ns+'&to_ns='+range.to_ns),v=>v.items.length,v=>v>0,'immutable generation');
  assert.equal(stack.sql("SELECT count(DISTINCT (kind,id)) FROM project_resource_visibility('"+stack.s.tenant+"','"+stack.s.project+"',NULL) WHERE full_access"),'2600');
  stack.historyInputs={d,analysis,runInput,target,generation:visible.items[0].id,version:fixed.analysis.artifact_version_id,artifact:fixed.analysis.artifact_id};
  return stack.historyInputs;
}
export async function historyJobs(input,user){
  const {d,analysis,runInput,target,generation,version,artifact}=input;
  const current=await user.call(target);
  const preview=await user.call(target+'/backfill-preview',{expected_revision:current.config_revision||current.channel.config_revision,output_range:range});
  const backfilled=await user.call(target+'/backfill',{...command(current.config_revision||current.channel.config_revision),preview_id:preview.id,preview_hash:preview.hash});
  const created=await Promise.all([
    user.call(analysis+'/runs',{...command(),...runInput,artifact_name:'本轮现场历史计算'}),
    user.call(d+'/generations/'+generation+'/replay',{...command(),output_range:range}),
    user.call(analysis+'/comparisons',{...command(),left_version_id:version,right_version_id:version}),
    user.call(analysis+'/artifacts/'+artifact+'/exports',{...command(),version_id:version,format:'csv'}),
    user.call(d+'/generations/'+generation+'/exports',{...command(),format:'csv'}),
  ]);
  const jobs=backfilled.runs.concat(created).map(r=>({job_id:r.job.id,kind:r.job.kind}));
  assert.deepEqual([...new Set(jobs.map(v=>v.kind))].sort(),['analysis','analysis_compare','analysis_export','derive','derived_evidence_export','derived_replay']);
  return jobs;
}
