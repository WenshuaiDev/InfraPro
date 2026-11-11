import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import type { EngineeringVersion, ResourceChange, ResourceKind } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { CodeBlock, CodeBlockContent, CodeBlockLine } from '@infrapro/ui/components/code-block';
import { DataTable } from '@infrapro/ui/patterns/data-table';
import { SelectControl } from '../engineering-fields/controls';
import { useResources } from './context';
const operations:Record<string,string>={create:'新增',update:'修改',retire:'退役',restore:'恢复',delete:'删除'};
const kinds:Record<string,string>={facility:'设施',component:'构件',point:'测点',channel:'通道'};
function FixedDefinitions({ change, versions }: { change: ResourceChange; versions: EngineeringVersion[] }) {
 const wanted = new Set<string>();
 const include = (id: string) => { if (wanted.has(id)) return; wanted.add(id); versions.find((v) => v.id === id)?.dependencies.forEach(include); };
 versions.filter((v) => v.kind === 'type-definitions' || change.definition_versions.includes(v.id)).forEach((v) => include(v.id));
 const adopted = versions.filter((v) => wanted.has(v.id));
 return <section aria-label="历史固定定义" className="my-3 space-y-3 rounded-md border p-3"><h3 className="font-medium">当时采用的定义</h3>{adopted.map((v) => <div key={v.id} className="space-y-1 text-sm"><p>{v.content.name} · V{v.number}{v.content.unit && ` · ${v.content.unit.symbol}`}</p>{v.content.metric?.options?.map((o) => <p key={o.key}>{o.key}：{o.name}</p>)}<details><summary className="cursor-pointer underline">定义完整内容</summary><CodeBlock><CodeBlockContent wrap className="max-h-80" aria-label={`${v.content.name}固定版本定义`}>{JSON.stringify(v.content, null, 2).split('\n').map((line, i) => <CodeBlockLine key={i}>{line}</CodeBlockLine>)}</CodeBlockContent></CodeBlock></details></div>)}</section>;
}
export function ResourceHistory({facility,resource}:{facility:string;resource?:{kind:ResourceKind;id:string}}){
 const {client,scope,prefix}=useResources();const [kind,setKind]=useState<ResourceKind|''>('');
 const q=useInfiniteQuery({queryKey:[...prefix,'changes',facility,resource,kind],initialPageParam:'',queryFn:({signal,pageParam})=>client.resourceChanges(scope,facility,{kind:resource?.kind??(kind||undefined),id:resource?.id,cursor:pageParam},signal),getNextPageParam:(page)=>page.next_cursor||undefined});
 const definitions = [...new Map(q.data?.pages.flatMap((p) => p.definitions.map((v) => [v.id, v] as const)) ?? []).values()];
 return <section className="space-y-4"><h2 className="text-lg font-semibold">变更记录</h2>{!resource&&<SelectControl label="对象种类" value={kind} onChange={(v)=>setKind(v as ResourceKind|'')} options={Object.entries(kinds).map(([value,label])=>({value,label}))}/>}<DataTable label="资源变更记录" data={q.data?.pages.flatMap((p)=>p.items)??[]} getRowId={(v)=>v.id} loading={q.isPending} error={q.error?.message} onRetry={()=>void q.refetch()} hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={()=>void q.fetchNextPage()} minWidth={580} columns={[{id:'object',header:'对象与操作',size:240,cell:({row})=><div className="space-y-1"><p className="break-words font-medium">{row.original.after.name} · {row.original.after.code}</p><p className="text-xs text-muted-foreground">{kinds[row.original.resource_kind]} · {operations[row.original.operation]}</p></div>},{id:'actor',header:'操作者与时间',size:200,cell:({row})=><div>{row.original.actor_name}<p className="text-xs text-muted-foreground">{new Date(row.original.changed_at).toLocaleString()}</p></div>},{id:'content',header:'变更内容',size:180,cell:({row})=><details><summary className="cursor-pointer underline">查看前后值</summary><p className="my-2 whitespace-pre-wrap break-words">{row.original.reason}</p><FixedDefinitions change={row.original} versions={definitions} /><CodeBlock><CodeBlockContent wrap className="max-h-96" aria-label="资源变更前后值">{JSON.stringify({before:row.original.before,after:row.original.after},null,2).split("\n").map((line,i)=><CodeBlockLine key={i}>{line}</CodeBlockLine>)}</CodeBlockContent></CodeBlock></details>}]} emptyLabel="暂无变更记录"/>{q.error&&<Button variant="outline" onClick={()=>void q.refetch()}>重新读取历史</Button>}</section>;
}
