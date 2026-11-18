import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { Navigate } from '@tanstack/react-router';
import { DomainFiles } from '../files/domain-files';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { MonitoringJob } from '@infrapro/api-client';
import { MonitoringPageHeader } from './page-header';
import { Button } from '@infrapro/ui/components/button';
import { workspaceTaskFilter } from '../tasks/search';
import { useMonitoring } from './context';
import { MonitoringError } from './common';
import { count, qualityNames, rangeLabel } from './format';
import { LegacyTaskLocation } from '../tasks/legacy-location';

import { FixedQueryResult } from './task-query-result';
import { TaskError } from '../tasks/common';
const progressNames: Record<string, string> = { input_rows:'已读输入行', output_rows:'已完成输出行', completed_parts:'已完成分块', compared_rows:'已比较结果', differences:'差异数', bytes_read: '已读取字节', total_bytes: '文件总字节', parsed_rows: '已解析行', candidates: '候选观测', completed_chunks: '完成块数', total_chunks: '总块数', rows: '已写观测', completed_files: '完成分卷', files: '分卷数', channels: '通道数', phase: '阶段', adopted: '实际采用', sequence: '发布水位' };
const phases: Record<string, string> = { computing:'计算', publishing:'发布固定结果', comparing:'比较固定版本', waiting_upstream:'等待明确上游结果', reading: '读取与规范化', validating: '校验与核对', preparing: '准备生效', writing: '写入并核验', verifying: '逐副本校验', summary: '构建摘要', complete: '完成' };
export function MonitoringJobsPage({jobID,result,execution}:{jobID?:string;result?:string;execution?:string}) {
 const m=useMonitoring();
 if(!jobID)return <Navigate replace to="/tasks" search={{...workspaceTaskFilter(m.scope),source:'monitoring,derived'}} />;
 if(!result)return <LegacyTaskLocation kind="job" id={jobID} execution={execution} />;
 return <MonitoringJobResult id={jobID} kind={result} execution={execution} />;
}
function MonitoringJobResult({id,kind,execution}:{id:string;kind:string;execution?:string}) {
 const m=useMonitoring();
 const job=useQuery({queryKey:[...m.prefix,'job',id],queryFn:({signal})=>m.client.monitoringJob(m.scope,id,signal),retry:false});
 const current=job.error ? undefined : job.data;
 const same=!!current && (!execution || current.task_ref?.generation===execution);
 return <section className="space-y-4"><MonitoringPageHeader workspaceTasks title={kind==='files'?'监测导出文件':'监测查询成果'} description="查看本次执行的监测结果。" />
   <TaskError error={job.error} retry={()=>void job.refetch()}/>{job.isPending&&<p role="status">正在核实结果…</p>}
   {current && (same && current.state==='succeeded' && kind==='query' && current.query ? <FixedQueryResult action={current.query.action} input={current.query.input}/> :
     same && current.state==='succeeded' && current.kind==='export' && kind==='files' && current.result_state==='available' ? <ExportFiles id={current.target_id}/> : <p role="status">本次执行的结果当前不可访问。</p>)}</section>;
}
export function JobProgress({ job }: { job: MonitoringJob }) {
  const fields = Object.entries(job.progress).filter(([key, value]) => progressNames[key] && (typeof value === 'string' || typeof value === 'number'));
  return <div className="space-y-1 text-xs">{fields.length ? fields.map(([key, value]) => <p key={key}>{progressNames[key]}：{key === 'phase' ? phases[String(value)] ?? String(value) : /^\d+$/.test(String(value)) ? count(String(value)) : String(value)}</p>) : <p className="text-muted-foreground">{job.state === 'queued' ? '等待可用工作进程' : job.state === 'running' ? '处理中，等待首个完成进度' : '无额外进度数据'}</p>}{job.error_code && <p className="break-all">原因：{job.error_code}</p>}</div>;
}
function ExportFiles({ id }: { id: string }) {
  const { client, scope, prefix } = useMonitoring(); const [pageSize, setPageSize] = useState(50), [page, setPage] = useState(0);
  const result = useQuery({ queryKey: [...prefix, 'export-files', id], queryFn: ({ signal }) => client.monitoringExportFiles(scope, id, signal), retry: false, refetchInterval: 30_000 });
  const data = result.data;
  return <section className="space-y-4"><h3 className="font-semibold">完整导出结果</h3><MonitoringError error={result.error} retry={() => void result.refetch()} />{data && !result.error && <><div className="space-y-1 rounded-lg bg-muted/50 p-3 text-sm"><p>{data.kind === 'statistics' ? '统计表' : '原始明细'} · {data.format.toUpperCase()} · 共 {count(data.row_count)} 行 / {data.files.length} 卷</p><p className="break-words">{(data.query.view_range ?? data.query.range) ? rangeLabel((data.query.view_range ?? data.query.range)!) : '全部获准历史'}</p><p>质量 {(data.query.qualities ?? ['good', 'unmarked']).map(q => qualityNames[q]).join('、')}</p><p>过期时间 {new Date(data.expires_at).toLocaleString()}</p><p>固定水位 {data.snapshot.watermark}</p>{data.escaping && <p className="break-words">CSV 转义说明：{data.escaping}</p>}</div><Button variant="outline" asChild><a href={client.monitoringDownloadURL(scope, id, 'manifest')} target="_blank" rel="noreferrer">下载字段、筛选与校验清单</a></Button><DomainFiles files={data.files.slice(page * pageSize,(page + 1) * pageSize).map(file=>({ref:{kind:'monitoring.export',scope,exportId:id,part:file.part},display_name:file.name,media_type:data.format,size_bytes:file.size_bytes,sha256:file.sha256,state:'available',expires_at:data.expires_at,allowed_actions:['download']}))} actions={file=>file.ref.kind==='monitoring.export'&&file.allowed_actions.includes('download')?<Button variant="outline" asChild><a href={client.monitoringDownloadURL(scope,id,file.ref.part)} download aria-label={file.display_name}>下载分卷</a></Button>:null}/>{<ListPagination label="导出分卷" total={data.files.length} page={page} go={setPage} pageSize={pageSize} setPageSize={size => { setPageSize(size); setPage(0); }} />}</>}{result.isPending && <p role="status">正在核验结果可下载资格…</p>}</section>;
}
