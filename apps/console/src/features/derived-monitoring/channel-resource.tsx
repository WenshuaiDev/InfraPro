import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { DerivedChannel, ResourceTarget } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { TenantResources, ProjectResources } from '../resources/context';
import { LifecycleForm } from '../resources/lifecycle';
import { ResourceEditor } from '../resources/editor';
import { CommandError, Section, useDerived } from './common';

export function ChannelResource({channel,onChanged}:{channel:DerivedChannel;onChanged:()=>void}) {
 const m=useDerived(),[action,setAction]=useState<'edit'|'retire'|'restore'|'delete'>();
 const target:ResourceTarget={kind:'channel',facility:channel.resource.facility_id,id:channel.resource.id};
 const detail=useQuery({queryKey:[...m.prefix,'derived-resource',channel.resource.id,channel.resource.revision],queryFn:({signal})=>m.client.resourceDetail(m.scope,target,signal),enabled:m.canNavigate});
 const done=()=>{setAction(undefined);void detail.refetch();onChanged();};
 const labels={edit:'编辑通道资料',retire:'退役通道',restore:'恢复通道',delete:'删除未被引用通道'};
 if(!m.canNavigate)return null;
 const Boundary=m.scope.kind==='tenant'?TenantResources:ProjectResources;
 return <Section title="工程资源资料与生命周期"><CommandError error={detail.error}/><p className="text-sm text-muted-foreground">退役保留历史并固定停止边界。恢复后保持暂停，继续自动计算需另外试算并确认恢复。</p><div className="flex flex-wrap gap-2">{detail.data&&m.editable&&(['edit','retire','restore','delete'] as const).filter(v=>detail.data!.actions[v]).map(v=><Button key={v} variant="outline" onClick={()=>setAction(v)}>{labels[v]}</Button>)}</div><ResponsivePanel open={!!action} onOpenChange={open=>{if(!open)setAction(undefined);}} title={action?labels[action]:'工程资源操作'} description="沿用当前空间的工程资源授权、影响预览和审计记录。">{action&&detail.data&&<Boundary>{action==='edit'?<ResourceEditor target={target} initial={detail.data} done={done}/>:<LifecycleForm target={target} resource={detail.data} action={action} done={done}/>}</Boundary>}</ResponsivePanel></Section>;
}
