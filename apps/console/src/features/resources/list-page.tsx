import { useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useInfiniteQuery } from '@tanstack/react-query';
import type { ResourceFilter } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { SelectControl, TextControl } from '../engineering-fields/controls';
import { useResources } from './context';
import { ResourceEditor } from './editor';
export function FacilitiesPage(){const scope=useResources();if(!scope.canRead)return <ErrorState title="没有工程资源查看权限" message="请联系租户管理员配置相应角色。"/>;return <FacilitiesBody/>;}
function FacilitiesBody(){
 const {client,tenantId,prefix,canManage,canAdopt}=useResources();const navigate=useNavigate();const [filter,setFilter]=useState<ResourceFilter>({status:'normal'});const [creating,setCreating]=useState(false);const [selectedType,setSelectedType]=useState<{value:string;label:string}>();
 const q=useInfiniteQuery({queryKey:[...prefix,'facilities',filter],initialPageParam:'',queryFn:({signal,pageParam})=>client.resourceList(tenantId,{kind:'facility'},{...filter,cursor:pageParam},signal),getNextPageParam:(page)=>page.next_cursor||undefined});
 const types=Array.from(new Map((q.data?.pages.flatMap((p)=>p.items)??[]).filter((r)=>r.type_definition_id).map((r)=>[r.type_definition_id!,{value:r.type_definition_id!,label:r.type_name!}])).values());if(selectedType&&!types.some((t)=>t.value===selectedType.value))types.unshift(selectedType);
 return <section className="min-w-0 space-y-5"><PageHeader title="设施管理" description="租户统一管理设施、实际构件、测点和业务监测通道。"/><div className="grid items-end gap-3 sm:grid-cols-[1fr_12rem_auto]"><TextControl label="搜索设施名称或编号" value={filter.q??''} onChange={(q)=>setFilter({...filter,q})}/><SelectControl label="设施状态" value={filter.deleted?'deleted':filter.status??'normal'} onChange={(value)=>setFilter({...filter,deleted:value==='deleted'?'only':undefined,status:value==='deleted'?'all':value as ResourceFilter['status']})} options={[{value:'normal',label:'正常'},{value:'retired',label:'已退役'},{value:'all',label:'全部未删除'},{value:'deleted',label:'已删除记录'}]}/>{canManage&&canAdopt&&<Button id="create-facility" onClick={()=>setCreating(true)}>新增设施</Button>}</div>
 <SelectControl label="设施类型（已读取列表）" value={filter.type_definition_id??''} onChange={(value)=>{setSelectedType(types.find((t)=>t.value===value));setFilter({...filter,type_definition_id:value||undefined});}} options={types}/>
 <DataTable label="设施列表" data={q.data?.pages.flatMap((p)=>p.items)??[]} getRowId={(v)=>v.id} loading={q.isPending} error={q.error?.message} onRetry={()=>void q.refetch()} emptyLabel="暂无设施" minWidth={480} hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={()=>void q.fetchNextPage()} columns={[{id:'identity',header:'设施名称与编号',size:260,cell:({row})=><div><p className="break-words font-medium">{row.original.name}</p><p className="break-all text-xs text-muted-foreground">{row.original.code}</p></div>},{id:'type',header:'采用类型',size:170,cell:({row})=>row.original.type_name?`${row.original.type_name} · V${row.original.type_number}`:'—'},{id:'status',header:'实际状态',size:100,cell:({row})=>row.original.deleted_at?'已删除':row.original.status==='normal'?'正常':'已退役'},{id:'open',header:'操作',size:120,cell:({row})=><Button asChild variant="outline" size="sm"><Link to="/t/$tenantId/resources/facilities/$facilityId" params={{tenantId,facilityId:row.original.id}} search={row.original.deleted_at?{tab:'changes'}:{}}>{row.original.deleted_at?'查看历史':'打开设施'}</Link></Button>}]}/>
 <ResponsivePanel open={creating} onOpenChange={setCreating} title="新增设施" description="选择固定类型版本，填写当前设施资料后直接保存。" fallbackFocusSelector="#create-facility">{creating&&<ResourceEditor target={{kind:'facility'}} done={(v)=>{setCreating(false);void navigate({to:'/t/$tenantId/resources/facilities/$facilityId',params:{tenantId,facilityId:v.id},search:{}});}}/>}</ResponsivePanel></section>;
}
