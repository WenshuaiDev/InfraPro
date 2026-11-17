import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { ClassificationScope, DeviceClassificationSelection } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { SelectControl, TextControl } from '../engineering-fields/controls';

export function CategoryPicker({scope,value,onChange,kind='product_version',suggestion=false,filterOnly=false}:{scope:ClassificationScope;value:DeviceClassificationSelection;onChange:(v:DeviceClassificationSelection)=>void;kind?:'device'|'product_version';suggestion?:boolean;filterOnly?:boolean}) {
 const {client,account}=useIdentity();const [search,setSearch]=useState('');
 const query=useInfiniteQuery({queryKey:['classification-categories',account?.id,scope,search],initialPageParam:{cursor:'',release_id:''},queryFn:({pageParam,signal})=>client.deviceCategories(scope,{...pageParam,q:search,limit:100},signal),getNextPageParam:p=>p.next_cursor?{cursor:p.next_cursor,release_id:p.dictionary_release_id}:undefined});
 const nodes=query.data?.pages.flatMap(p=>p.items)??[];const selected=nodes.find(n=>n.id===value.category_id);const release=query.data?.pages[0]?.dictionary_release_id;
 return <div className="space-y-3"><SelectControl label={filterOnly?'分类筛选':suggestion?'建议分类':'分类方式 *'} value={value.mode} options={[{value:'classify',label:kind==='device'?'覆盖产品默认分类':'指定设备分类'},{value:kind==='device'?'inherit':'unclassified',label:filterOnly?'不限分类':suggestion?'不提供建议':kind==='device'?'继承当前产品默认分类':'明确保留为未分类'}]} onChange={mode=>onChange({mode:mode==='classify'?'classify':kind==='device'?'inherit':'unclassified'})}/>
 {value.mode==='classify'&&<><TextControl label="查找分类名称或编号" value={search} onChange={setSearch}/>{query.error&&<FormNotice error>{query.error.message}</FormNotice>}{query.isPending?<p role="status">正在读取分类…</p>:<div className="max-h-64 space-y-1 overflow-auto rounded-lg border p-2" aria-label="设备分类选择">{nodes.map(n=><Button key={n.id} type="button" variant={n.id===value.category_id?'secondary':'ghost'} className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" disabled={!filterOnly&&(!n.selectable||n.status!=='active')} onClick={()=>onChange({mode:'classify',category_id:n.id,dictionary_release_id:release})}><span>{n.name_path?.join(' / ')||n.name} <span className="text-xs text-muted-foreground">· {n.code}{!n.selectable?' · 仅作分组':''}{n.status!=='active'?' · 停用':''}</span></span></Button>)}{!nodes.length&&<p className="p-2 text-sm text-muted-foreground">没有匹配的分类。</p>}{query.hasNextPage&&<Button type="button" variant="outline" className="w-full" disabled={query.isFetchingNextPage} onClick={()=>void query.fetchNextPage()}>加载更多分类</Button>}</div>}{value.category_id&&<p className="text-sm">已选：{selected?.name_path?.join(' / ')||'已保留所选分类'}。提交时将重新检查分类状态和字典版本。</p>}</>}
 </div>;
}
