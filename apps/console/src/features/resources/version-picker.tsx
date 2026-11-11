import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { EngineeringVersion, ResourceScope } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { VersionPicker } from '../engineering/reference-picker';
import { SelectControl, TextControl } from '../engineering-fields/controls';
import { useResources } from './context';
type Props = {kind:'type-definitions'|'metrics';label:string;value:string;known:EngineeringVersion[];onChange:(id:string,v?:EngineeringVersion,deps?:EngineeringVersion[])=>void};
export function ResourceVersionPicker(props:Props){const {scope}=useResources();return scope.kind==='tenant'?<VersionPicker {...props}/>:<ProjectPicker {...props} scope={scope}/>;}
function ProjectPicker({scope,kind,label,value,known,onChange}:Props&{scope:Extract<ResourceScope,{kind:'project'}>}){
 const {client,prefix,canAdopt}=useResources();const [q,setQ]=useState('');const [offset,setOffset]=useState(0);const [choice,setChoice]=useState(value);
 const list=useQuery({queryKey:[...prefix,'catalog',kind,q,offset],queryFn:({signal})=>client.projectResourceCatalog(scope,kind,{q,offset,limit:25},signal),enabled:canAdopt});
 const detail=useQuery({queryKey:[...prefix,'catalog-version',kind,choice],queryFn:({signal})=>client.projectResourceCatalogVersion(scope,kind,choice,signal),enabled:!!choice&&canAdopt});
 const current=known.find(v=>v.id===value);const options=(list.data?.items??[]).map(v=>({value:v.id,label:`${v.name} · ${v.code} · V${v.number}`}));
 if(current&&!options.some(o=>o.value===current.id))options.unshift({value:current.id,label:`${current.content.name} · V${current.number}`});
 return <section className="space-y-3 rounded-lg bg-muted/30 p-3"><TextControl label={`${label}搜索`} value={q} onChange={v=>{setQ(v);setOffset(0);}}/><SelectControl label={`${label}版本`} value={choice} onChange={setChoice} options={options}/><Button type="button" variant="outline" disabled={!detail.data||detail.isFetching||!!detail.error} onClick={()=>{if(detail.data)onChange(choice,detail.data.version,detail.data.dependencies);}}>采用所选版本</Button>{value&&<p className="text-sm">当前采用：{current?.content.name} · V{current?.number}</p>}<FormNotice error>{list.error?.message??detail.error?.message}</FormNotice>{list.isFetching&&<p role="status">正在读取可采用版本…</p>}<div className="flex flex-wrap gap-2"><Button type="button" variant="ghost" disabled={offset===0} onClick={()=>setOffset(offset-25)}>上一页</Button><Button type="button" variant="ghost" disabled={offset+25>=(list.data?.total??0)} onClick={()=>setOffset(offset+25)}>下一页</Button></div></section>;
}
