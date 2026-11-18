import { useMonitoringDetailPages } from '../monitoring/query-hooks';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useState } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import type { DerivedCreateChannelInput, DerivedInputSpec, DerivedMetricVersion, DerivedRule, MonitoringChannel, MonitoringQuery } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { detailRowKey, observationTime } from '../monitoring/detail-format';
import { qualityNames, rangeLabel } from '../monitoring/format';
import { useOperation } from '../account/use-operation';
import { CommandError, Section, SelectField, TextField, TimeField, isoNS, nsISO, useDerived } from './common';

type Bindings=DerivedCreateChannelInput['input_bindings'];
type Baselines=NonNullable<DerivedCreateChannelInput['baseline_bindings']>;
type Owner=DerivedCreateChannelInput['owner'];
export function DefinitionVersionPicker({value,onChange,initialDefinition}:{value:string;onChange:(v:DerivedMetricVersion)=>void;initialDefinition?:string}) {
 const m=useDerived(),[q,setQ]=useState(''),[choice,setDefinition]=useState(initialDefinition??'');
 const fixed=useQuery({queryKey:[...m.prefix,'derived-version',value],queryFn:({signal})=>m.client.derivedMetricVersion(m.scope,value,signal),enabled:!!value&&m.allows('metrics.read')});
 const definition=choice||fixed.data?.definition_id||'';
 const list=usePagedQuery({queryKey:[...m.prefix, 'definition-picker', q],queryFn:({ cursor, signal }) => m.client.derivedMetrics(m.scope,{q,state:'published',cursor},signal),enabled:m.allows('metrics.read')});
 const versions=usePagedQuery({queryKey:[...m.prefix, 'version-picker', definition],queryFn:({ cursor, signal }) => m.client.derivedMetricVersions(m.scope,definition,{cursor},signal),enabled:!!definition&&m.allows('metrics.read')});
 return <Section title="选择固定指标版本" path="/definition_version_id"><TextField label="搜索已发布指标" value={q} onChange={v=>{setQ(v);}}/><CommandError error={list.error??versions.error??fixed.error}/><SelectField label="指标定义" value={definition} onChange={v=>{setDefinition(v);}} options={(list.data?.items??[]).map(v=>({value:v.id,label:`${v.name} · ${v.code}`}))}/><ListPagination label="列表" {...list.pagination} />{definition&&<><div className="space-y-2">{versions.data?.items.map(v=><Button key={v.id} type="button" className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" variant={value===v.id?'secondary':'outline'} disabled={!v.usable} aria-pressed={value===v.id} onClick={()=>onChange(v)}><span className="min-w-0 break-all">V{v.version_no} · {v.name}{!v.usable?' · 不可用于新配置':''}</span></Button>)}</div><ListPagination label="版本" {...versions.pagination} /></>}{value&&<p className="break-all text-xs text-muted-foreground">已固定：{fixed.data?`V${fixed.data.version_no} · ${fixed.data.name}`:value}</p>}</Section>;
}
export function OwnerPicker({value,onChange}:{value:Owner;onChange:(v:Owner)=>void}) {
 const m=useDerived(),[q,setQ]=useState(''),[childQ,setChildQ]=useState('');
 const facilities=usePagedQuery({queryKey:[...m.prefix, 'derived-owner-facilities', q],queryFn:({ cursor, signal }) => m.client.resourceList(m.scope,{kind:'facility'},{q,cursor,status:'normal'},signal),enabled:m.canNavigate});
 const children=usePagedQuery({queryKey:[...m.prefix, 'derived-owner-children', value.kind, value.facility_id, childQ],queryFn:({ cursor, signal }) => m.client.resourceList(m.scope,{kind:value.kind,facility:value.facility_id},{q:childQ,cursor,status:'normal'},signal),enabled:m.canNavigate&&value.kind!=='facility'&&!!value.facility_id});
 return <Section title="所属工程对象" path="/owner"><SelectField path="/owner/kind" label="归属类型" value={value.kind} onChange={kind=>{onChange({...value,kind,id:kind==='facility'?value.facility_id:''});}} options={[{value:'point',label:'测点'},{value:'component',label:'构件'},{value:'facility',label:'设施'}]}/><TextField label="搜索设施" value={q} onChange={v=>{setQ(v);}}/><CommandError error={facilities.error??children.error}/><SelectField path="/owner/facility_id" label="所属设施" value={value.facility_id} onChange={facility_id=>{onChange({...value,facility_id,id:value.kind==='facility'?facility_id:''});}} options={(facilities.data?.items??[]).filter(v=>value.kind!=='facility'||v.visibility!=='context').map(v=>({value:v.id,label:`${v.name} · ${v.code}${v.visibility==='context'?'（仅上下文）':''}`}))}/><ListPagination label="设施" {...facilities.pagination} />{value.kind!=='facility'&&value.facility_id&&<><TextField label={`搜索${value.kind==='component'?'构件':'测点'}`} value={childQ} onChange={v=>{setChildQ(v);}}/><SelectField path="/owner/id" label={value.kind==='component'?'所属构件':'所属测点'} value={value.id} onChange={id=>onChange({...value,id})} options={(children.data?.items??[]).filter(v=>v.visibility!=='context').map(v=>({value:v.id,label:`${v.name} · ${v.code}`}))}/><ListPagination label="工程对象" {...children.pagination} /></>}<p className="text-sm text-muted-foreground">归属对象决定通道业务位置；输入可来自同租户中当前获准的其他对象。</p></Section>;
}
export function BindingEditor({rule,value,parameters,baselines={},onChange,onParameters,onBaselines,section}:{section?:'inputs'|'parameters'|'none';rule:DerivedRule;value:Bindings;parameters:Record<string,string>;baselines?:Baselines;onChange:(v:Bindings)=>void;onParameters:(v:Record<string,string>)=>void;onBaselines:(v:Baselines)=>void}) {
 const m=useDerived();
 const bound=useQueries({queries:rule.inputs.map(input=>({queryKey:[...m.prefix,'bound-channel',value[input.key]?.channel_id??''],queryFn:({signal}:{signal:AbortSignal})=>m.client.monitoringDirectory(m.scope,{channel_id:value[input.key]!.channel_id},signal),enabled:!!value[input.key]?.channel_id&&m.canRead&&m.canNavigate}))});
 const peers=rule.inputs.flatMap((input,index)=>{const c=bound[index]?.data?.items[0];return c?[{input,channel:c}]:[];});
 const references=rule.nodes.flatMap(n=>n.params.baseline?.reference?[{node:n.id,baseline:n.params.baseline}]:[]);
 return <div hidden={section==='none'}><Section title={section==='parameters'?'参数与固定基准':'绑定输入与实际参数'} path="/input_bindings"><div hidden={section==='parameters'} className="space-y-4">{rule.inputs.map(input=><Section key={input.key} path={`/input_bindings/${input.key}`} title={`${input.name||input.key} · ${input.key}`}><BoundChannel input={input} peers={peers.filter(p=>p.input.key!==input.key)} value={value[input.key]?.channel_id??''} onChange={channel_id=>onChange({...value,[input.key]:{channel_id}})}/></Section>)}</div><div hidden={section==='inputs'} className="space-y-4">{section==='parameters'&&!rule.parameters?.length&&!references.length&&<p className="text-sm text-muted-foreground">此规则没有需要单独绑定的参数或固定基准。</p>}{(rule.parameters??[]).map(p=><TextField key={p.key} label={`${p.key}${p.identity_affecting?'（改变业务身份）':''}`} path={`/parameters/${p.key}`} value={parameters[p.key]??p.default} onChange={v=>onParameters({...parameters,[p.key]:v})} hint={`${p.type}；${p.minimum??'不限最小值'} 至 ${p.maximum??'不限最大值'}`}/>)}{references.map(({node,baseline})=>{
 const ref=baseline.reference!,bound=baselines[ref]??{input:rule.inputs[0]?.key??''};return <Section key={`${node}:${ref}`} path={`/baseline_bindings/${ref}`} title={`固定基准 · ${ref}`}><SelectField label="基准来自哪个输入槽" value={bound.input} onChange={input=>onBaselines({...baselines,[ref]:{input}})} options={rule.inputs.map(v=>({value:v.key,label:v.name||v.key}))}/>{baseline.kind==='observation'?<ObservationPicker key={`${ref}:${bound.input}:${value[bound.input]?.channel_id}`} channel={value[bound.input]?.channel_id??''} value={bound} onChange={v=>onBaselines({...baselines,[ref]:v})}/>:<p className="text-sm text-muted-foreground">固定参考窗：{nsISO(baseline.from_ns??'')} 至 {nsISO(baseline.to_ns??'')}；{baseline.method}。首次完整试算时冻结所选输入的实际依据，后续新增样本不会改变此基准。</p>}</Section>;
 })}</div></Section></div>;
}
type Peer={input:DerivedInputSpec;channel:MonitoringChannel};
function semantic(c:MonitoringChannel,field:'direction'|'reference') {
 return (c.source_kind==='derived'?c.output_semantics?.[field]:field==='direction'?c.observation_direction:c.reference_basis)?.trim().replace(/\s+/g,' ')??'';
}
function incompatible(c:MonitoringChannel,input:DerivedInputSpec,peers:Peer[]) {
 if(c.value_kind==='enum')return '枚举值不能参与数值规则';
 if(!input.source_kinds.includes(c.source_kind))return '来源类型不符合输入要求';
 if(input.metric_version_id&&c.metric_version_id!==input.metric_version_id)return '公共指标版本不同';
 if(c.status!=='normal')return '已退役；新绑定不可选';
 const q=input.quantity_spec;
 if(c.source_kind==='direct') {
  if(c.unit_version_id!==q.base_unit_version_id||q.base_power!==1||q.time_power!==0)return '固定单位或单位幂次与输入要求不同';
  if((c.timestamp_meaning==='observation_time'?'instant':c.timestamp_meaning)!==input.time_semantics.kind)return '原始通道的时间含义与输入要求不同';
 } else {
  if(['base_unit_version_id','base_power','time_unit','time_power','quantity_mode','level_scale','dimensionless_kind'].some(k=>(c.quantity_spec?.[k]??'')!==(q[k as keyof typeof q]??'')))return '派生单位、幂次或量值含义与输入要求不同';
  const t=c.output_semantics?.time_semantics;
  if(t?.kind!==input.time_semantics.kind||(t?.aggregation??'')!==(input.time_semantics.aggregation??''))return '派生输出的时间或聚合含义不同';
 }
 for(const field of ['direction','reference'] as const)if(input[field]==='must_match') {
  const label=field==='direction'?'观测方向':'参考基准',v=semantic(c,field);
  if(!v)return `${label}未明确，不能满足必须一致的约束`;
  if(peers.some(p=>p.input[field]==='must_match'&&semantic(p.channel,field)!==v))return `${label}与已绑定输入不一致`;
 }
 return '';
}
function channelMeaning(c:MonitoringChannel) {
 const time=c.source_kind==='direct'?(c.timestamp_meaning==='observation_time'?'瞬时样本':'原始区间结束'):c.output_semantics?.time_semantics?.kind==='derived_window'?`派生窗口 ${c.output_semantics.time_semantics.aggregation??''}`:'派生瞬时结果';
 return `${c.source_kind==='derived'?'正式派生':'原始观测'} · ${c.metric_name} · ${c.unit||'无量纲'} · ${time} · ${semantic(c,'direction')||'方向未声明'} · ${semantic(c,'reference')||'基准未声明'}`;
}
export function BoundChannel({input,value,peers=[],sourceKinds,onChange}:{input?:DerivedInputSpec;value:string;peers?:Peer[];sourceKinds?:MonitoringChannel['source_kind'][];onChange:(v:string)=>void}) {
 const m=useDerived(),[q,setQ]=useState(''),[open,setOpen]=useState(!value);
 const selected=useQuery({queryKey:[...m.prefix,'bound-channel',value],queryFn:({signal})=>m.client.monitoringDirectory(m.scope,{channel_id:value},signal),enabled:!!value&&m.canRead&&m.canNavigate});
 const list=usePagedQuery({queryKey:[...m.prefix, 'binding-options', q],queryFn:({ cursor, signal }) => m.client.monitoringDirectory(m.scope,{search:q,cursor},signal),enabled:open&&m.canRead&&m.canNavigate});
 const channel=selected.data?.items[0];
 return <div className="space-y-3"><CommandError error={selected.error??list.error}/>{value&&<div className="rounded-lg bg-muted/40 p-3"><p className="break-all font-medium">{channel?`${channel.owner_name} · ${channel.name}`:selected.isFetching?'正在读取通道…':'原绑定通道当前不可读'}</p>{channel&&<p className="text-sm text-muted-foreground">{channelMeaning(channel)}</p>}<Button type="button" variant="ghost" onClick={()=>setOpen(v=>!v)}>{open?'收起选择':'更换通道'}</Button></div>}{(!m.canRead||!m.canNavigate)&&<p className="text-sm">选择输入需要当前空间的工程资源及监测数据读取权限。</p>}{open&&m.canRead&&m.canNavigate&&<><TextField label="搜索输入通道或所属对象" value={q} onChange={v=>{setQ(v);}}/><div className="max-h-80 space-y-2 overflow-auto">{list.data?.items.map(c=>{const reason=input?incompatible(c,input,peers):sourceKinds&&!sourceKinds.includes(c.source_kind)?'来源类型不符合要求':'';return <button key={c.id} type="button" disabled={!!reason} onClick={()=>{onChange(c.id);setOpen(false);}} className="min-h-14 w-full rounded-lg border p-3 text-left hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-60"><span className="block break-all text-sm font-medium">{c.owner_name} · {c.name} · {c.code}</span><span className="block break-all text-xs text-muted-foreground">{channelMeaning(c)}</span><span className="block text-xs">{reason||`可读：${c.read_ranges.map(rangeLabel).join('；')}`}</span></button>;})}{list.data?.items.length===0&&<p className="text-sm text-muted-foreground">没有可读的匹配通道。</p>}</div><ListPagination label="列表" {...list.pagination} /></>}</div>;
}
function ObservationPicker({channel,value,onChange}:{channel:string;value:Baselines[string];onChange:(v:Baselines[string])=>void}) {
 const m=useDerived(), [from,setFrom]=useState(''), [to,setTo]=useState('');
 const [input,setInput]=useState<MonitoringQuery>();
 const load=useOperation({mutationFn:async()=>{
   if (!channel) throw new Error('请先绑定该输入槽的通道。');
   const range = { from: nsISO(from), to: nsISO(to) };
   const selection = await m.client.monitoringSelect(m.scope, { channel_ids: [channel] });
   return { selection_id: selection.id, range };
 },onSuccess:setInput});
 const query = useMonitoringDetailPages(input, 100);
 const result = query.data;
 return <div className="space-y-3"><div className="grid gap-3 sm:grid-cols-2"><TimeField label="查找起点" value={from} onChange={setFrom}/><TimeField label="查找终点（不含）" value={to} onChange={setTo}/></div><Button type="button" variant="outline" disabled={load.isPending||!channel} onClick={()=>load.mutate(undefined)}>查找基准样本</Button><CommandError error={load.error ?? query.error}/>{value.at_ns&&<p role="status" className="text-sm">已选择 {nsISO(value.at_ns)} 的{value.result_id?'正式派生结果':'原始观测及其采用记录'}。</p>}{result&&<DirectoryTable label="基准候选样本" items={result.items} rowId={detailRowKey} columns={[{id:'time',label:'时刻',cell:r=>observationTime(r)},{id:'value',label:'精确值',cell:r=><span className="break-all font-mono text-xs">{r.value??'无结果'}</span>},{id:'quality',label:'质量',cell:r=>qualityNames[r.source_kind==='derived'?r.data_quality:r.quality]},{id:'select',label:'选择',cell:r=><Button type="button" size="sm" variant="outline" disabled={r.value===null||r.source_kind==='direct'&&r.revoked||r.source_kind==='derived'&&r.calculation_status!=='valid'} onClick={()=>onChange(r.source_kind==='direct'?{input:value.input,observation_id:r.observation_id,adoption_id:r.adoption_id,at_ns:isoNS(r.observed_at)}:{input:value.input,result_id:r.result_id,generation_id:r.generation_id,at_ns:isoNS(r.output_at)})}>固定此样本</Button>}]} pagination={<ListPagination label="基准候选样本" {...query.pagination} />}/>}</div>;
}
