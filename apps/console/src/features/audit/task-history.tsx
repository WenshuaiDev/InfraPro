import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { AuditScope,TaskView } from '@infrapro/api-client';
import { ObjectHistory } from './object-history';
export function TaskHistory({task}:{task:TaskView}) {
 const v=task.scope;
 const scope:AuditScope=v.kind==='platform'?{kind:'platform'}:v.project_id?{kind:'project',tenant_id:v.tenant_id,project_id:v.project_id}:{kind:'tenant',tenant_id:v.tenant_id};
 return <ObjectHistory scope={scope} filter={{task_module:task.source_ref.source_module,task_kind:task.source_ref.source_kind,task_id:task.source_ref.source_root_id,generation:task.generation}}/>;
}
export function NotificationHistory({taskId,generation}:{taskId:string;generation?:string}) {
 const {client,account,authorization_revision,authorizationEpoch}=useIdentity();
 const q=useQuery({queryKey:['notification-audit-task',account?.id,authorization_revision,authorizationEpoch,taskId,generation],queryFn:({signal})=>client.task(taskId,generation,signal),retry:false,staleTime:0});
 return q.data&&!q.error?<TaskHistory task={q.data}/>:null;
}
