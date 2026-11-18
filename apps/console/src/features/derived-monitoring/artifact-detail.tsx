import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { AlarmClock, Download, History, List, RotateCw, Settings2 } from 'lucide-react';
import { ApiError, type AnalysisArtifactDetail, type AnalysisVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { ObjectHistory } from '../audit/object-history';
import { CommandError, Section, SelectField, TextField, useDerived } from './common';
import { InputRanges } from './run-panel';
import { RangeFields, recentNSRange } from './channel-controls';
import { EvidenceTree } from './channel-history';
import { AnalysisExecution } from './analysis-execution';
import { ArtifactComparison } from './artifact-comparison';
import { ArtifactExport } from './artifact-export';
import { ArtifactManagement } from './artifact-management';
import { ArtifactHeadline, ArtifactReadings, useArtifactInputs } from './artifact-readings';
import { reportTime } from './analysis-format';

type DetailProps = {id: string; version?: string; exportID?: string; onVersion: (id: string) => void; onDeleted: (name: string) => Promise<void>};
export function ArtifactDetail({id, version, exportID, onVersion, onDeleted}: DetailProps) {
  const m = useDerived(), cache = useQueryClient();
  const detail = useQuery({queryKey:[...m.prefix,'artifact-detail',id,version],
    queryFn:({signal}) => m.client.analysisArtifact(m.scope,id,version,signal), enabled:m.allows('analysis.results.read'), refetchInterval:30000, retry:false});
  if (!m.allows('analysis.results.read')) return <p role="status">当前身份没有分析成果读取权限。</p>;
  if (detail.isPending) return <LoadingState/>;
  if (detail.error || !detail.data) return <div className="space-y-4"><CommandError error={detail.error}/>
    {detail.error instanceof ApiError && [403,404].includes(detail.error.status ?? 0) && <ArtifactSelection id={id} version={version} exportID={exportID}/>}</div>;
  return <ArtifactContent key={detail.data.version.id} detail={detail.data} exportID={exportID} onVersion={onVersion}
    refresh={async () => { await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-artifacts']}); await detail.refetch(); }} onDeleted={onDeleted}/>;
}

function ArtifactSelection({id,version,exportID}: Pick<DetailProps,'id'|'version'|'exportID'>) {
  const m=useDerived(), [range,setRange]=useState(recentNSRange), [keys,setKeys]=useState('');
  const [selection,setSelection]=useState<{range:ReturnType<typeof recentNSRange>;outputs:string[];openExport?:string}>();
  const result=useQuery({queryKey:[...m.prefix,'artifact-selected-detail',id,version,selection],
    queryFn:({signal})=>m.client.analysisArtifactRows(m.scope,id,{version_id:version??'',output_range:selection!.range,outputs:selection!.outputs,limit:1},signal),
    enabled:!!selection,retry:false,refetchInterval:30000});
  return <div className="space-y-4"><Section title="按获准片段读取成果">
    <p className="text-sm text-muted-foreground">可按获准的时间范围和输出读取片段。窗口、差分前驱和固定基准仍需对应的输入权限。</p>
    <RangeFields value={range} onChange={setRange}/><TextField label="输出标识（逗号分隔，可留空）" value={keys} onChange={setKeys}/>
    <Button variant="outline" disabled={result.isFetching} onClick={()=>setSelection({range,openExport:selection?undefined:exportID,outputs:keys.split(/[,，]/).map(v=>v.trim()).filter(Boolean)})}>读取获准片段</Button>
    <CommandError error={result.error}/></Section>
    {result.data?.detail&&!result.error&&<ArtifactContent key={JSON.stringify(selection)} detail={result.data.detail} exportID={selection?.openExport}/>}
  </div>;
}

type Panel = 'rows' | 'versions' | 'source' | 'rerun' | 'export' | 'manage';
function ArtifactContent({detail, exportID, onVersion, refresh, onDeleted}: {
  detail:AnalysisArtifactDetail; exportID?:string; onVersion?:(id:string)=>void; refresh?:()=>Promise<unknown>; onDeleted?:DetailProps['onDeleted'];
}) {
  const m=useDerived(), inputs=useArtifactInputs(detail);
  const [panel,setPanel]=useState<Panel>(exportID?'export':'rows');
  const [range,setRange]=useState(detail.version.output_range);
  const [outputs,setOutputs]=useState(detail.outputs.map(v=>v.key));
  const [fixed,setFixed]=useState({range,outputs});
  const versions=usePagedQuery({queryKey:[...m.prefix,'artifact-versions',detail.artifact.id],
    queryFn:({cursor,signal})=>m.client.analysisArtifactVersions(m.scope,detail.artifact.id,{cursor},signal),
    enabled:!!onVersion && (panel==='versions' || detail.artifact.latest_version!=='1')});
  const rows=usePagedQuery({queryKey:[...m.prefix,'artifact-rows',detail.version.id,fixed],
    queryFn:({cursor,limit,signal})=>m.client.analysisArtifactRows(m.scope,detail.artifact.id,{version_id:detail.version.id,output_range:fixed.range,outputs:fixed.outputs,cursor,limit},signal),
    enabled:panel==='rows'||panel==='versions'||panel==='export',retry:false,refetchInterval:30000});
  const lineage=useQuery({queryKey:[...m.prefix,'artifact-lineage',detail.version.id,fixed],
    queryFn:({signal})=>m.client.analysisArtifactLineage(m.scope,detail.artifact.id,{version_id:detail.version.id,output_range:fixed.range,outputs:fixed.outputs},signal),
    enabled:panel==='source',retry:false,refetchInterval:30000});
  const canRun=m.editable&&m.allows('analysis.read')&&m.allows('analysis.run')&&detail.artifact.status==='active'&&!detail.version.partial;
  const full = !!onVersion && !detail.version.partial;
  const tabs = [
    {key:'rows',label:'逐条结果',icon:List}, {key:'source',label:'固定来源',icon:AlarmClock},
    {key:'versions',label:'版本与比较',icon:History},
    ...(m.allows('analysis.export')?[{key:'export',label:'导出',icon:Download}]:[]),
    ...(full&&m.editable&&m.allows('analysis.manage')?[{key:'manage',label:'成果管理',icon:Settings2}]:[]),
  ] as const;
  return <article aria-label="固定成果" className="min-w-0 rounded-xl border bg-background">
    <header className="space-y-4 p-4 md:p-6"><ArtifactHeadline detail={detail} inputs={inputs}/>
      {canRun&&<Button variant={panel==='rerun'?'secondary':'outline'} aria-pressed={panel==='rerun'} onClick={()=>setPanel('rerun')}><RotateCw/>换时段再算</Button>}
    </header>
    <nav aria-label="成果操作" className="flex flex-wrap gap-x-4 border-y px-4 md:px-6">{tabs.map(({key,label,icon:Icon})=><button type="button" key={key}
      aria-pressed={panel===key} onClick={()=>setPanel(key as Panel)}
      className="inline-flex min-h-11 items-center gap-1.5 border-b-2 border-transparent text-sm text-muted-foreground aria-pressed:border-teal-700 aria-pressed:font-semibold aria-pressed:text-teal-800 dark:aria-pressed:border-teal-400 dark:aria-pressed:text-teal-300"><Icon size={15}/>{label}</button>)}</nav>
    <div className="space-y-3 p-4 md:p-6">
      {panel==='rows'&&<><h3 className="font-semibold">固定成果明细</h3>
        <details className="text-sm"><summary className="min-h-10 cursor-pointer py-2 font-medium">筛选时段与输出</summary><div className="space-y-3 pb-3">
          <RangeFields value={range} onChange={setRange}/><fieldset><legend>显示输出</legend><div className="flex flex-wrap gap-3">{detail.outputs.map(v=><label key={v.key} className="flex min-h-10 items-center gap-2"><input type="checkbox" checked={outputs.includes(v.key)} onChange={e=>setOutputs(old=>e.target.checked?[...old,v.key]:old.filter(k=>k!==v.key))}/>{v.name||v.key}</label>)}</div></fieldset>
          <Button variant="outline" disabled={!outputs.length} onClick={()=>setFixed({range,outputs})}>查询所选范围与输出</Button></div></details>
        <CommandError error={rows.error}/>{rows.isPending&&<LoadingState/>}{rows.data&&!rows.error&&<ArtifactReadings items={rows.data.items} detail={detail} inputs={inputs} pagination={rows.pagination}/>}
      </>}
      {panel==='versions'&&<>{onVersion&&<><SelectField label="选择固定成果版本" value={detail.version.id} onChange={onVersion}
        options={[detail.version,...(versions.data?.items??[]).filter(v=>v.id!==detail.version.id)].map(v=>({value:v.id,label:'V'+v.version_no+' · '+new Date(v.created_at).toLocaleString()}))}/>
        <CommandError error={versions.error}/>{versions.data?.next_cursor || versions.pagination.page>0 ? <ListPagination label="版本" {...versions.pagination}/> : null}</>}
        <ArtifactComparison left={rows.data?.detail??detail}/></>}
      {panel==='source'&&<><h3 className="font-semibold">原固定来源</h3><CommandError error={lineage.error}/>{lineage.isPending&&<LoadingState/>}
        {lineage.data&&!lineage.error&&<div className="space-y-3 text-sm">{Object.values(inputs).map((source,i)=><p key={i}>{source.label} · {source.unit}</p>)}
          <InputRanges values={lineage.data.input_ranges}/><EvidenceTree values={lineage.data.upstream_inputs}/>{lineage.data.baselines.map(v=><p key={v.reference}>基准 {v.reference} · 快照 {v.snapshot_id}</p>)}
          <p className="break-all text-xs">原始快照：{lineage.data.raw_snapshot.id}</p><p className="break-all text-xs">固定配置 {lineage.data.config_version_id} · 来源位置 {detail.version.source_position}</p></div>}
        {detail.changes.map((v,i)=><p key={i} className="break-all text-sm">{({raw_append:'新增输入',raw_revoke:'输入撤销',derived_publish:'上游派生修订',channel_config:'配置修订'})[v.kind as 'raw_append']??v.kind} · {inputs[v.channel_id]?.label||v.channel_id} · {reportTime(v.affected_range.from_ns)} 至 {reportTime(v.affected_range.to_ns)}</p>)}
        {detail.changes_truncated && <p className="text-sm">这里只展示部分变化记录。</p>}<ObjectHistory objectType="analysis_artifact" objectId={detail.artifact.id}/></>}
      {panel==='rerun'&&canRun&&<ArtifactRerun detail={detail}/>}
      {panel==='export'&&<ArtifactExport detail={rows.data?.detail??detail} range={fixed.range} outputs={fixed.outputs} exportID={exportID}/>}
      {panel==='manage'&&refresh&&onDeleted&&<ArtifactManagement artifact={detail.artifact} refresh={refresh} onDeleted={onDeleted}/>}
    </div>
  </article>;
}
function ArtifactRerun({detail}:{detail:AnalysisArtifactDetail}) {
 const m=useDerived(),[selected,setSelected]=useState<AnalysisVersion>(detail.config);
 const versions=usePagedQuery({queryKey:[...m.prefix, 'rerun-config-versions', detail.config.config_id],queryFn:({ cursor, signal }) => m.client.analysisConfigVersions(m.scope,detail.config.config_id,{cursor},signal),enabled:m.allows('analysis.read')});
 return <Section title="选择本次重算的固定配置"><CommandError error={versions.error}/><SelectField label="分析配置版本" value={selected.id} onChange={id=>{const v=versions.data?.items.find(v=>v.id===id);if(v)setSelected(v);}} options={[{value:detail.config.id,label:`原配置 V${detail.config.version_no}`},...(versions.data?.items??[]).filter(v=>v.id!==detail.config.id).map(v=>({value:v.id,label:`V${v.version_no} · ${new Date(v.created_at).toLocaleString()}`}))]}/><ListPagination label="版本" {...versions.pagination} /><AnalysisExecution key={selected.id} version={selected} artifact={detail}/></Section>;
}
