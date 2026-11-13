import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import type { ResourceDetail } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { CursorButtons } from '../monitoring/common';
import { useCursor } from '../derived-monitoring/use-cursor';
import { derivedSearch } from '../derived-monitoring/search';
import { useResources } from './context';

export function OwnerDerivedChannels({owner}:{owner:ResourceDetail}) {
 const r=useResources(),page=useCursor(),kind=owner.kind==='channel'?'point':owner.kind;
 const facility=owner.kind==='facility'?owner.id:owner.facility_id;
 const base=r.scope.kind==='tenant'?`/t/${r.scope.tenantId}`:`/t/${r.scope.tenantId}/p/${r.scope.projectId}`;
 const channelsPath:string=`${base}/monitoring/derived`;
 const channelPath=(id:string):string=>`${channelsPath}/${id}`;
 const query=useQuery({queryKey:[...r.prefix,'owner-derived',owner.id,page.cursor],queryFn:({signal})=>r.client.derivedChannels(r.scope,{facility_id:facility,owner_kind:kind,owner_id:owner.id,cursor:page.cursor},signal),enabled:r.canDerivedRead&&owner.kind!=='channel'});
 if(owner.kind==='channel'||!r.canDerivedRead)return null;
 return <section className="min-w-0 space-y-4 border-t pt-4"><div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-semibold">此对象直属的派生通道</h3>{r.available&&r.canDerivedManage&&owner.actions.create_derived_channel&&<Button asChild variant="outline"><Link to={channelsPath} search={derivedSearch({id:'new',owner_kind:kind,owner:owner.id,facility})}>新建直属派生通道</Link></Button>}</div><DirectoryTable label="直属派生通道" items={query.data?.items??[]} rowId={v=>v.resource.id} loading={query.isPending} error={query.error?.message} columns={[{id:'name',label:'通道',cell:v=><Link className="break-words font-medium underline" to={channelPath(v.resource.id)}>{v.resource.name} · {v.resource.code}</Link>},{id:'state',label:'控制状态',cell:v=>({draft:'待启用',enabled:'自动计算中',paused:'暂停',retired:'退役'})[v.control_state]},{id:'health',label:'更新状态',cell:v=>v.health==='healthy'?'正常':v.health==='waiting'?'等待输入或输出时刻':v.blocked_reason}]} pagination={<CursorButtons previous={page.previous} next={query.data?.next_cursor?()=>page.next(query.data!.next_cursor):undefined} busy={query.isFetching}/>}/></section>;
}
