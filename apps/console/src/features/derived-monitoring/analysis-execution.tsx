import type { DerivedSearch } from './search';
import { useState } from 'react';
import type { AnalysisArtifactDetail, AnalysisRunInput, AnalysisVersion, DerivedRun } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { qualityNames } from '../monitoring/format';
import { CommandError, Section, TextField, useDerived, isoNS } from './common';
import { RangeFields, recentNSRange } from './channel-controls';
import { RunPanel } from './run-panel';
import { AnalysisPeriod } from './analysis-period';

export function AnalysisExecution({version,artifact,prefill}:{version:AnalysisVersion;artifact?:AnalysisArtifactDetail;prefill?:DerivedSearch}) {
 const m=useDerived(),meta=useRequestMeta(),[range,setRange]=useState(()=>artifact?.version.output_range??(prefill?.from&&prefill.to&&Number.isFinite(Date.parse(prefill.from))&&Number.isFinite(Date.parse(prefill.to))?{from_ns:isoNS(prefill.from),to_ns:isoNS(prefill.to)}:recentNSRange())),[source,setSource]=useState(artifact?.run.source_range??range),[outputs,setOutputs]=useState<string[]>(artifact?.run.outputs??version.body.rule.outputs.map(v=>v.key));
 const [quality,setQuality]=useState<NonNullable<AnalysisRunInput['quality']>>((artifact?.run.quality??version.body.rule.policies.quality??['good','unmarked']) as NonNullable<AnalysisRunInput['quality']>),[name,setName]=useState(artifact?.artifact.name??version.name),[reason,setReason]=useState(''),[run,setRun]=useState<DerivedRun>();
 const whole=version.body.rule.clock.kind==='whole_range';
 const [settings,setSettings]=useState(false);
 const operation=useOperation({mutationFn:(trial:boolean)=>{const body={config_version_id:version.id,config_hash:version.config_hash,output_range:range,source_range:whole?{...source,to_ns:range.to_ns}:undefined,outputs,quality,...(!trial?artifact?{artifact_id:artifact.artifact.id}:{artifact_name:name}:{})};const input={...body,...meta(trial?'analysis-trial':'analysis-run',body,reason)};return trial?m.client.analysisTrial(m.scope,input):m.client.analysisStartRun(m.scope,input);},onSuccess:setRun});
 const pickRange=(next:typeof range)=>{
  if(whole&&source.from_ns===range.from_ns)setSource(next);
  setRange(next);
 };
 return <Section title={artifact?'重新运行，生成新成果版本':`运行固定配置 V${version.version_no}`}>
   <AnalysisPeriod value={range} original={artifact?.version.output_range} onChange={pickRange}/>
   <details className="text-sm" open={settings} onToggle={e=>setSettings(e.currentTarget.open)}><summary className="min-h-11 cursor-pointer py-3 font-medium">本次输出、质量与来源设置</summary><div className="space-y-3">
     {whole&&settings&&<><RangeFields label="完整分析源范围" value={{...source,to_ns:range.to_ns}} onChange={v=>{setSource(v);setRange(r=>({...r,to_ns:v.to_ns}));}}/>
       <p className="text-muted-foreground">结果标记在完整源范围的终点。</p></>}
     <fieldset><legend className="font-medium">本次指定输出</legend><div className="flex flex-wrap gap-4">{version.body.rule.outputs.map(v=><label key={v.key} className="flex min-h-10 items-center gap-2"><input type="checkbox" checked={outputs.includes(v.key)} onChange={e=>setOutputs(old=>e.target.checked?[...old,v.key]:old.filter(k=>k!==v.key))}/>{v.name||v.key}</label>)}</div></fieldset>
     <fieldset><legend className="font-medium">参与计算的质量</legend><div className="flex flex-wrap gap-4">{(['good','unmarked','suspect','bad'] as const).map(q=><label key={q} className="flex min-h-10 items-center gap-2"><input type="checkbox" checked={quality.includes(q)} onChange={e=>setQuality(old=>e.target.checked?[...old,q]:old.filter(v=>v!==q))}/>{qualityNames[q]}</label>)}</div></fieldset>
   </div></details>
   {!artifact&&<TextField label="成果名称" value={name} onChange={setName}/>}<TextField label="运行原因" value={reason} onChange={setReason}/>
   <CommandError error={operation.error}/>
   {m.editable&&m.allows('analysis.run')&&<div className="flex flex-wrap gap-2"><Button variant="outline" disabled={operation.isPending||!outputs.length||!quality.length} onClick={()=>operation.mutate(true)}>完整试算并解释</Button><Button disabled={operation.isPending||!outputs.length||!quality.length} onClick={()=>operation.mutate(false)}>{artifact?'生成新的完整成果版本':'运行并生成完整成果'}</Button></div>}
   {run&&<RunPanel key={run.id} id={run.id} initial={run}/>}
 </Section>;
}
