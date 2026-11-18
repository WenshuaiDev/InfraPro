import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { ClassificationScope, DeviceClassificationTargetDetail } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { CheckControl, TextControl } from '../engineering-fields/controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ClassificationAssignment, ClassificationValue } from './assignment';
export function ClassificationBulk({scope,kind,disabled=false}:{scope:ClassificationScope;kind:'device'|'product_version';disabled?:boolean}){
 const [open,setOpen]=useState(false);const focus=useRef<HTMLElement|null>(null);
 return <><Button variant="outline" disabled={disabled} onClick={e=>{focus.current=e.currentTarget;setOpen(true);}}>批量调整分类</Button><ResponsivePanel open={open} onOpenChange={setOpen} title={kind==='device'?'批量调整设备分类':'批量调整产品默认分类'} description="逐项选择固定目标，预览后统一提交。" returnFocus={focus}>{open&&<BulkForm scope={scope} kind={kind} done={()=>setOpen(false)}/>}</ResponsivePanel></>;
}
function BulkForm({scope,kind,done}:{scope:ClassificationScope;kind:'device'|'product_version';done:()=>void}){
 const {client,account}=useIdentity();const cache=useQueryClient();const [search,setSearch]=useState('');const [selected,setSelected]=useState<Record<string,DeviceClassificationTargetDetail>>({});const [ready,setReady]=useState(false);
 const list=usePagedQuery({ initialPageSize: 100, queryKey:['classification-targets',account?.id,scope,kind,search], queryFn: ({cursor,signal, limit})=>client.deviceClassificationTargets(scope,{kind,q:search,cursor,limit},signal) });
 const items=list.data?.items??[];const count=Object.keys(selected).length;
 return ready?<ClassificationAssignment scope={scope} targets={Object.values(selected)} done={()=>{void cache.invalidateQueries({queryKey:[scope.kind==='platform'?'platform':'tenant',account?.id]});void cache.invalidateQueries({queryKey:['classification-target']});void cache.invalidateQueries({queryKey:['classification-targets']});done();}}/>:<div className="space-y-4"><TextControl label="搜索名称或编号" value={search} onChange={setSearch}/><p className="text-sm">已选 {count} 个对象（最多 5000）。跨页选择保留；搜索结果的后续变化不会自动加入此次调整。</p><FormNotice error>{list.error?.message}</FormNotice><div className="max-h-[50dvh] divide-y overflow-auto rounded-lg border px-3">{items.map(v=><div key={v.id} className="space-y-2 py-3"><CheckControl label={v.name} value={!!selected[v.id]} onChange={checked=>{if(checked&&count>=5000)return;setSelected(old=>{const next={...old};if(checked)next[v.id]=v;else delete next[v.id];return next;});}}/><ClassificationValue value={v.effective}/></div>)}{list.isPending&&<p role="status" className="py-4">正在读取候选对象…</p>}{!list.isPending&&!items.length&&<p className="py-4 text-sm text-muted-foreground">没有当前可选的对象。</p>}</div>{list.data &&<ListPagination label="对象" {...list.pagination} />}<div className="flex gap-2"><Button disabled={!count} onClick={()=>setReady(true)}>为已选对象设置分类</Button><Button variant="outline" disabled={!count} onClick={()=>setSelected({})}>清空选择</Button></div></div>;
}
