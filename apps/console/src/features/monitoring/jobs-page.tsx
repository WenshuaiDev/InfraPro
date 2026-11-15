import { ObjectHistory } from '../audit/object-history';
import { DomainFiles } from '../files/domain-files';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { MonitoringJob } from '@infrapro/api-client';
import { MonitoringPageHeader } from './page-header';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useMonitoring } from './context';
import { CursorButtons, MonitoringError } from './common';
import { count, qualityNames, rangeLabel } from './format';
import { TaskCenter } from '../tasks/center';
import { TaskDetailContent } from '../tasks/detail';
import { FixedQueryResult } from './task-query-result';
import { TaskError } from '../tasks/common';
const progressNames: Record<string, string> = { input_rows:'已读输入行', output_rows:'已完成输出行', completed_parts:'已完成分块', compared_rows:'已比较结果', differences:'差异数', bytes_read: '已读取字节', total_bytes: '文件总字节', parsed_rows: '已解析行', candidates: '候选观测', completed_chunks: '完成块数', total_chunks: '总块数', rows: '已写观测', completed_files: '完成分卷', files: '分卷数', channels: '通道数', phase: '阶段', adopted: '实际采用', sequence: '发布水位' };
const phases: Record<string, string> = { computing:'计算', publishing:'发布固定结果', comparing:'比较固定版本', waiting_upstream:'等待明确上游结果', reading: '读取与规范化', validating: '校验与核对', preparing: '准备生效', writing: '写入并核验', verifying: '逐副本校验', summary: '构建摘要', complete: '完成' };
export function MonitoringJobsPage({ jobID }: { jobID?: string }) {
 const m=useMonitoring(),[open,setOpen]=useState(!!jobID);
 return <section className="min-w-0 space-y-4"><MonitoringPageHeader title="监测任务" description="统一查看当前范围内的监测和计算任务。"/><TaskCenter embedded filter={{mode:'mine',tenant_id:m.scope.tenantId,project_id:m.scope.kind === 'project' ? m.scope.projectId : undefined,source:'monitoring,derived'}}/><ResponsivePanel open={open} onOpenChange={setOpen} title="任务状态与结果" description="旧任务链接与统一任务中心使用相同的状态和操作。" size="wide">{jobID&&<LegacyJobDetail id={jobID}/>}</ResponsivePanel></section>;
}
function LegacyJobDetail({id}:{id:string}) {
 const m=useMonitoring(),[execution,setExecution]=useState<string>();
 const resolve=useQuery({queryKey:[...m.prefix,'task-resolve',id],queryFn:({signal})=>m.client.taskResolve({source_module:'monitoring',source_kind:'job',source_root_id:id},signal),retry:false});
 const job=useQuery({queryKey:[...m.prefix,'job',id],queryFn:({signal})=>m.client.monitoringJob(m.scope,id,signal),retry:false});
 return <div className="space-y-5"><ObjectHistory objectType="monitoring_job" objectId={id}/><TaskError error={resolve.error} retry={()=>void resolve.refetch()}/>{job.data?.query && job.data.state==='succeeded' && <FixedQueryResult action={job.data.query.action} input={job.data.query.input}/>} {job.data?.kind==='export'&&job.data.result_state==='available'&&<ExportFiles id={job.data.target_id}/>} {resolve.data?.task_id&&<TaskDetailContent id={resolve.data.task_id} execution={execution} onExecution={setExecution}/>}</div>;
}
export function JobProgress({ job }: { job: MonitoringJob }) {
  const fields = Object.entries(job.progress).filter(([key, value]) => progressNames[key] && (typeof value === 'string' || typeof value === 'number'));
  return <div className="space-y-1 text-xs">{fields.length ? fields.map(([key, value]) => <p key={key}>{progressNames[key]}：{key === 'phase' ? phases[String(value)] ?? String(value) : /^\d+$/.test(String(value)) ? count(String(value)) : String(value)}</p>) : <p className="text-muted-foreground">{job.state === 'queued' ? '等待可用工作进程' : job.state === 'running' ? '处理中，等待首个完成进度' : '无额外进度数据'}</p>}{job.error_code && <p className="break-all">原因：{job.error_code}</p>}</div>;
}
function ExportFiles({ id }: { id: string }) {
  const { client, scope, prefix } = useMonitoring(); const [page, setPage] = useState(0);
  const result = useQuery({ queryKey: [...prefix, 'export-files', id], queryFn: ({ signal }) => client.monitoringExportFiles(scope, id, signal), retry: false, refetchInterval: 30_000 });
  const data = result.data;
  return <section className="space-y-4"><h3 className="font-semibold">完整导出结果</h3><MonitoringError error={result.error} retry={() => void result.refetch()} />{data && !result.error && <><div className="space-y-1 rounded-lg bg-muted/50 p-3 text-sm"><p>{data.kind === 'statistics' ? '统计表' : '原始明细'} · {data.format.toUpperCase()} · 共 {count(data.row_count)} 行 / {data.files.length} 卷</p><p className="break-words">{(data.query.view_range ?? data.query.range) ? rangeLabel((data.query.view_range ?? data.query.range)!) : '全部获准历史'}</p><p>质量 {(data.query.qualities ?? ['good', 'unmarked']).map(q => qualityNames[q]).join('、')}</p><p>过期时间 {new Date(data.expires_at).toLocaleString()}</p><p>固定水位 {data.snapshot.watermark}</p>{data.escaping && <p className="break-words">CSV 转义说明：{data.escaping}</p>}</div><Button variant="outline" asChild><a href={client.monitoringDownloadURL(scope, id, 'manifest')} target="_blank" rel="noreferrer">下载字段、筛选与校验清单</a></Button><DomainFiles files={data.files.slice(page*50,(page+1)*50).map(file=>({ref:{kind:'monitoring.export',scope,exportId:id,part:file.part},display_name:file.name,media_type:data.format,size_bytes:file.size_bytes,sha256:file.sha256,state:'available',expires_at:data.expires_at,allowed_actions:['download']}))} actions={file=>file.ref.kind==='monitoring.export'&&file.allowed_actions.includes('download')?<Button variant="outline" asChild><a href={client.monitoringDownloadURL(scope,id,file.ref.part)} download aria-label={file.display_name}>下载分卷</a></Button>:null}/>{data.files.length > 50 && <CursorButtons previous={page ? () => setPage(page - 1) : undefined} next={(page + 1) * 50 < data.files.length ? () => setPage(page + 1) : undefined} caption={`分卷第 ${page + 1} 页`} />}</>}{result.isPending && <p role="status">正在核验结果可下载资格…</p>}</section>;
}
