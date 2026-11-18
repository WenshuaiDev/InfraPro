import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { AnalysisArtifactDetail, AnalysisCompareInput, DerivedRun } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { CommandError, Section, SelectField, TextField, useDerived } from './common';
import { RangeFields } from './channel-controls';
import { RunPanel } from './run-panel';

export function ArtifactComparison({left}:{left:AnalysisArtifactDetail}) {
 const m=useDerived(),meta=useRequestMeta(),[q,setQ]=useState(''),[artifact,setArtifact]=useState(left.artifact.id),[version,setVersion]=useState(left.version.partial?left.version.id:''),[range,setRange]=useState(left.version.output_range),[restrict,setRestrict]=useState(!!left.version.partial),[mapping,setMapping]=useState<NonNullable<AnalysisCompareInput['output_mapping']>>([]),[reason,setReason]=useState(''),[run,setRun]=useState<DerivedRun>();
 const artifacts=usePagedQuery({queryKey:[...m.prefix, 'comparison-artifacts', q],queryFn:({ cursor, signal }) => m.client.analysisArtifacts(m.scope,{q,cursor},signal)});
 const versions=usePagedQuery({queryKey:[...m.prefix, 'comparison-versions', artifact],queryFn:({ cursor, signal }) => m.client.analysisArtifactVersions(m.scope,artifact,{cursor},signal),enabled:!!artifact&&!(left.version.partial&&artifact===left.artifact.id)});
 const right=useQuery({queryKey:[...m.prefix,'comparison-right',artifact,version],queryFn:({signal})=>m.client.analysisArtifact(m.scope,artifact,version,signal),enabled:!!artifact&&!!version&&!(artifact===left.artifact.id&&version===left.version.id)});
 const rightDetail=artifact===left.artifact.id&&version===left.version.id?left:right.data;
 const knownArtifacts=[{value:left.artifact.id,label:left.artifact.name},...(artifacts.data?.items??[]).filter(v=>v.artifact.id!==left.artifact.id).map(v=>({value:v.artifact.id,label:v.artifact.name}))];
 const knownVersions=[...(artifact===left.artifact.id?[{value:left.version.id,label:`V${left.version.version_no} · 当前获准版本`}]:[]),...(versions.data?.items??[]).filter(v=>artifact!==left.artifact.id||v.id!==left.version.id).map(v=>({value:v.id,label:`V${v.version_no} · ${new Date(v.created_at).toLocaleString()}`}))];
 const pairs=left.outputs.map(v=>({left:v.key,right:mapping.find(p=>p.left===v.key)?.right??(rightDetail?.outputs.some(r=>r.key===v.key)?v.key:'')}));
 const compare=useOperation({mutationFn:()=>{const body={left_version_id:left.version.id,right_version_id:version,output_mapping:pairs.filter(v=>v.right),output_range:restrict?range:undefined};return m.client.analysisCompare(m.scope,{...body,...meta('compare',body,reason)});},onSuccess:setRun});
 return <Section title={`比较固定成果：左侧 V${left.version.version_no}`}><TextField label="搜索右侧成果" value={q} onChange={v=>{setQ(v);}}/><CommandError error={artifacts.error??versions.error??right.error??compare.error}/><SelectField label="右侧成果" value={artifact} onChange={v=>{setArtifact(v);setVersion('');setRun(undefined);setMapping([]);}} options={knownArtifacts}/><ListPagination label="成果" {...artifacts.pagination} /><SelectField label="右侧固定成果版本" value={version} onChange={v=>{setVersion(v);setRun(undefined);setMapping([]);}} options={knownVersions}/><ListPagination label="版本" {...versions.pagination} />{rightDetail&&<div className="space-y-3"><p className="text-sm">明确选择输出对应关系。比较保留精确十进制、状态、质量、样本数量与区间差异。</p>{pairs.map(p=><SelectField key={p.left} label={`左侧 ${left.outputs.find(v=>v.key===p.left)?.name||p.left} 对应右侧`} value={p.right} onChange={right=>setMapping(old=>[...old.filter(v=>v.left!==p.left),{left:p.left,right}])} options={[{value:'',label:'不比较此输出'},...rightDetail!.outputs.map(v=>({value:v.key,label:v.name||v.key}))]}/>)}</div>}<label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={restrict} disabled={!!left.version.partial} onChange={e=>setRestrict(e.target.checked)}/>只比较明确选择的共同时间范围</label>{restrict&&<RangeFields value={range} onChange={setRange}/>}<TextField label="比较原因" value={reason} onChange={setReason}/><Button disabled={!version||!pairs.some(v=>v.right)||compare.isPending||!m.available} onClick={()=>compare.mutate()}>{restrict?'提交所选片段比较':'提交完整版本比较'}</Button>{run&&<RunPanel key={run.id} id={run.id} initial={run}/>}</Section>;
}
