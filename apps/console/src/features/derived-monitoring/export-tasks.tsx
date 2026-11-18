import { SourceTaskPanel } from '../tasks/source-panel';
import type { AnalysisExportRecord, GenerationExportRecord } from '@infrapro/api-client';
type ManagedExport = AnalysisExportRecord | GenerationExportRecord;
import { useQuery } from '@tanstack/react-query';

import { stateNames } from '../monitoring/format';
import { AnalysisExportFiles } from './artifact-export';
import { CommandError, useDerived } from './common';

export function AnalysisExportTask({id,generation=false}:{id:string;generation?:boolean}) {
 const m=useDerived();
 const result=useQuery<ManagedExport>({queryKey:[...m.prefix,generation?'generation-export':'analysis-export',id],queryFn:async({signal})=>(generation?m.client.generationExport:m.client.analysisExport)(m.scope,id,signal),refetchInterval:q=>q.state.data&&['queued','running','retry_wait'].includes(q.state.data.job.state)?2000:30000,retry:false});
 const job=result.data?.job;
 return <div className="space-y-4 rounded-lg border p-4"><CommandError error={result.error}/>{job&&!result.error&&<><p role="status">{stateNames[job.state]} · {result.data!.format.toUpperCase()}</p><SourceTaskPanel source={job.task_ref ?? {source_module:'derived',source_kind:'export',source_root_id:id}}/>{job.state==='succeeded'&&<AnalysisExportFiles id={id} generation={generation}/>}</>}</div>;
}
