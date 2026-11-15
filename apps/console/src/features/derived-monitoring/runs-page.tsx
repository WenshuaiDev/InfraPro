import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { MonitoringPageHeader } from '../monitoring/page-header';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { CursorButtons } from '../monitoring/common';
import { stateNames } from '../monitoring/format';
import { nsISO, useDerived } from './common';
import { AnalysisExportTasks } from './export-tasks';
import { RunPanel } from './run-panel';
import { useCursor } from './use-cursor';
export function DerivedRunsPage({id,exportID,evidenceExportID}:{id?:string;exportID?:string;evidenceExportID?:string}) {
 const m=useDerived(),page=useCursor(),navigate=useNavigate();
 const query=useQuery({queryKey:[...m.prefix,'derived-runs',page.cursor],queryFn:({signal})=>m.client.derivedRuns(m.scope,{cursor:page.cursor},signal),enabled:m.allows('runs.read'),refetchInterval:10000,retry:false});
 return <div className="min-w-0 space-y-4"><MonitoringPageHeader title="计算运行" description="试算、持续计算、历史回算和分析共用运行记录。任务离开页面后仍会继续。"/>{m.allows('runs.read')?<><DirectoryTable tableClassName="min-w-[40rem] lg:min-w-[56rem]" label="计算运行记录" items={query.data?.items??[]} rowId={v=>v.id} loading={query.isPending} error={query.error?.message} columns={[{id:'mode',className:'w-36',label:'运行类型',cell:v=><Button variant="link" className="h-auto min-h-11 w-full min-w-0 justify-start whitespace-normal break-all px-0 text-left" onClick={()=>void navigate({to:m.runsPath,search:{id:v.id}})}>{({trial:'试算',automatic:'自动计算',backfill:'历史回算',correction:'历史修正',analysis:'历史分析',comparison:'成果比较',historical_replay:'原依据回放'})[v.mode]}</Button>},{id:'state',className:'w-28',label:'状态',cell:v=>stateNames[v.job.state]??v.job.state},{id:'range',label:'输出范围',secondary:true,cell:v=><span className="text-xs">{nsISO(v.output_range.from_ns)} 至 {v.output_range.to_ns?nsISO(v.output_range.to_ns):'不限'}</span>},{id:'created',className:'w-48',label:'提交时间',cell:v=>new Date(v.created_at).toLocaleString()}]} pagination={<CursorButtons previous={page.previous} next={query.data?.next_cursor?()=>page.next(query.data!.next_cursor):undefined} busy={query.isFetching}/>}/>{id&&<RunPanel key={id} id={id}/>}</>:<p role="status">当前身份没有计算运行读取权限。</p>}<AnalysisExportTasks id={exportID}/><AnalysisExportTasks generation id={evidenceExportID}/></div>;
}
