import { DomainFiles } from '../files/domain-files';
import { useEffect,useRef } from 'react';
import { useQuery,useMutation } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import type { AuditScope } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { type FileDescriptor } from '@infrapro/ui/patterns/files';
import { TaskStatus } from '../tasks/common';
import { TaskActions } from '../tasks/detail';
import { date,nextActions } from './presentation';
export function ExportDetail({scope,id,generation,queryKey,close}:{scope:AuditScope;id:string;generation?:string;queryKey:readonly unknown[];close:()=>void}){
 const{client}=useIdentity(),controller=useRef<AbortController|null>(null),objectUrl=useRef('');
 useEffect(()=>()=>{controller.current?.abort();if(objectUrl.current)URL.revokeObjectURL(objectUrl.current);},[]);
 const q=useQuery({queryKey:[...queryKey,'export',id,generation],queryFn:({signal})=>client.auditExport(scope,id,generation,signal),retry:false,refetchInterval:5000,refetchIntervalInBackground:false});
 const e=q.error?undefined:q.data;
 const task=useQuery({queryKey:[...queryKey,'export-task',e?.task_id,e?.generation],queryFn:({signal})=>client.task(e!.task_id,e!.generation,signal),enabled:!!e?.task_id,retry:false,refetchInterval:5000,refetchIntervalInBackground:false});
 const download=useMutation({mutationFn:async()=>{if(!e?.file||!e.owned)throw Error('当前成果不可下载。');controller.current=new AbortController();const blob=await client.downloadAuditExport(scope,id,e.generation,controller.current.signal);if(controller.current.signal.aborted)return;if(objectUrl.current)URL.revokeObjectURL(objectUrl.current);objectUrl.current=URL.createObjectURL(blob);const a=document.createElement('a');a.href=objectUrl.current;a.download=e.file.name;a.click();URL.revokeObjectURL(objectUrl.current);objectUrl.current='';}});
 const file:FileDescriptor|undefined=e?.file?{ref:{kind:'audit.export',scope:scope.kind==='platform'?{kind:'platform'}:scope.kind==='tenant'?{kind:'tenant',tenantId:scope.tenant_id}:{kind:'project',tenantId:scope.tenant_id,projectId:scope.project_id},exportId:id,generation:e.generation},display_name:e.file.name,media_type:e.file.media_type,size_bytes:e.file.size_bytes,state:e.result_state==='none'?'unknown':e.result_state,expires_at:e.expires_at??undefined,allowed_actions:e.owned&&e.result_state==='available'?['download']:[]}:undefined;
 return <ResponsivePanel open onOpenChange={open=>{if(!open)close();}} title="审计导出详情" description="执行结果、文件有效期和当前下载资格分别核验。" size="wide"><div className="space-y-5">{q.error&&<ErrorState message={q.error.message} onRetry={()=>void q.refetch()}/ >}{q.isPending&&<p role="status">正在核实导出…</p>}{e&&<><TaskStatus task={e}/><p className="text-sm">第 {e.generation} 代 · {e.owned?'本人申请':'管理视图'}</p><dl className="grid gap-3 text-sm sm:grid-cols-2">{[['提交时间',date(e.created_at)],['快照时刻',date(e.snapshot_at)],['有效期至',date(e.expires_at)],['记录数',e.row_count??'尚未物化'],['字段档',e.field_profile==='technical'?'技术排障':'标准'],['原因',e.reason??'管理视图不展示筛选内容']].map(([k,v])=><div key={k} className="min-w-0"><dt className="text-muted-foreground">{k}</dt><dd className="break-words">{v}</dd></div>)}</dl>{e.state==='cancelling'&&<p role="status">取消请求已接受，工作进程正在停止。尚未确认实际取消。</p>}{e.error_code&&<div role="status" className="rounded-md border p-3 text-sm"><p>{nextActions[e.next_action]||'请打开任务详情核实原因和可用操作。'}</p><details><summary className="min-h-11 cursor-pointer">诊断原因</summary><p className="break-words">{e.error_code}</p></details></div>}<DomainFiles files={file?[file]:[]} actions={f=>f.allowed_actions.includes('download')?<Button className="min-h-11" disabled={download.isPending} onClick={()=>download.mutate()}>{download.isPending?'正在读取…':'下载 CSV'}</Button>:null}/>{download.error&&<ErrorState message={download.error.message}/ >}<Button className="min-h-11" variant="outline" asChild><Link to="/tasks/$taskId" params={{taskId:e.task_id}} search={{execution:e.generation}}>打开本代任务</Link></Button>{task.data&&!task.error&&<TaskActions task={task.data}/>}</>}</div></ResponsivePanel>;
}
