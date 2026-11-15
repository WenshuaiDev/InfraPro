import { useMutation } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { FileState } from '@infrapro/ui/patterns/files';
export function FixedFileTaskLink({job,generation}:{job:string;generation:string}) {const {client}=useIdentity(),navigate=useNavigate(),m=useMutation({mutationFn:()=>client.taskResolve({source_module:'files',source_kind:'job',source_root_id:job}),onSuccess:r=>{if(r.task_id)void navigate({to:'/tasks/$taskId',params:{taskId:r.task_id},search:{execution:generation}});}});return <span><Button variant="outline" disabled={m.isPending} onClick={()=>m.mutate()}>查看第 {generation} 代任务</Button>{m.error&&<FileState error>{m.error.message}</FileState>}</span>;}
