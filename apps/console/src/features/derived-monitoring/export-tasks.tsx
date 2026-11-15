import { SourceTaskPanel } from '../tasks/source-panel';
import type { AnalysisExportRecord, GenerationExportRecord } from '@infrapro/api-client';
type ManagedExport = AnalysisExportRecord | GenerationExportRecord;
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { CursorButtons } from '../monitoring/common';
import { stateNames } from '../monitoring/format';
import { AnalysisExportFiles } from './artifact-export';
import { CommandError, Section, useDerived } from './common';
import { useCursor } from './use-cursor';

export function AnalysisExportTasks({id,generation=false}:{id?:string;generation?:boolean}) {
 const m=useDerived(),page=useCursor(),navigate=useNavigate();
 const result=useQuery<{items:ManagedExport[];next_cursor:string}>({queryKey:[...m.prefix,generation?'generation-export-tasks':'analysis-export-tasks',page.cursor],queryFn:async({signal})=>(generation?m.client.generationExports:m.client.analysisExports)(m.scope,{cursor:page.cursor},signal),enabled:generation?m.allows('channels.read')&&m.allows('results.history'):m.allows('analysis.export')&&m.allows('analysis.results.read'),refetchInterval:10000,retry:false});
 if(generation?(!m.allows('channels.read')||!m.allows('results.history')):(!m.allows('analysis.export')||!m.allows('analysis.results.read')))return null;
 return <Section title={generation?'派生历史输入依据导出':'历史分析导出任务'}><p className="text-sm text-muted-foreground">本人在当前空间仍有权访问的固定版本导出。可离开页面后返回下载、取消或重试。</p><DirectoryTable tableClassName="min-w-[48rem]" label={generation?'派生历史输入依据导出记录':'历史分析导出记录'} items={result.data?.items??[]} rowId={v=>v.id} loading={result.isPending} error={result.error?.message} columns={[{id:'format',label:'文件格式',cell:v=><Button variant="link" className="h-auto min-h-11 w-full min-w-0 justify-start whitespace-normal break-all px-0 text-left" onClick={()=>void navigate({to:m.runsPath,search:generation?{evidence_export_id:v.id}:{export_id:v.id}})}>{v.format.toUpperCase()} · {v.id.slice(0,8)}</Button>},{id:'content',label:'内容',cell:v=>v.content==='inputs'?'完整输入依据':'正式计算结果'},{id:'state',label:'状态',cell:v=>stateNames[v.job.state]},{id:'version',label:'固定成果版本',secondary:true,cell:v=><span className="break-all text-xs">{v.source_version_id}</span>},{id:'expires',label:'导出有效至',cell:v=>new Date(v.expires_at).toLocaleString()}]} pagination={<CursorButtons previous={page.previous} next={result.data?.next_cursor?()=>page.next(result.data!.next_cursor):undefined} busy={result.isFetching}/>}/>{id&&<AnalysisExportTask key={id} id={id} generation={generation}/>}</Section>;
}
export function AnalysisExportTask({id,generation=false}:{id:string;generation?:boolean}) {
 const m=useDerived();
 const result=useQuery<ManagedExport>({queryKey:[...m.prefix,generation?'generation-export':'analysis-export',id],queryFn:async({signal})=>(generation?m.client.generationExport:m.client.analysisExport)(m.scope,id,signal),refetchInterval:q=>q.state.data&&['queued','running','retry_wait'].includes(q.state.data.job.state)?2000:30000,retry:false});
 const job=result.data?.job;
 return <div className="space-y-4 rounded-lg border p-4"><CommandError error={result.error}/>{job&&!result.error&&<><p role="status">{stateNames[job.state]} · {result.data!.format.toUpperCase()}</p><SourceTaskPanel source={{source_module:'derived',source_kind:'export',source_root_id:id}}/>{job.state==='succeeded'&&<AnalysisExportFiles id={id} generation={generation}/>}</>}</div>;
}
