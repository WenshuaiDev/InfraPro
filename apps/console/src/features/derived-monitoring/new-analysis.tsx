import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeftRight, ChevronRight, SlidersHorizontal } from 'lucide-react';
import type { AnalysisBody, DerivedInputSpec, DerivedQuantity, DerivedRule, MonitoringChannel } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { BoundChannel } from './bindings-editor';
import { AnalysisEditor } from './analysis-editor';
import { CommandError, Section, SelectField, emptyRule, useDerived } from './common';
import type { DerivedSearch } from './search';
import { useOperation } from '../account/use-operation';

type Method = 'subtract' | 'difference' | 'mean';
const methods: Record<Method,{label: string; meaning: string}> = {
  subtract: {label:'两处测点相差多少', meaning:'第一处减去第二处，按相同时刻匹配。'},
  difference: {label:'与前一次观测相比变化多少', meaning:'当前读数减去前一次有效读数。'},
  mean: {label:'一段时间的平均值是多少', meaning:'对整个时段的合格样本求平均，结果标记在时段终点。'},
};

function inputFor(c: MonitoringChannel, key: string, compare: boolean): DerivedInputSpec {
  const derived = c.source_kind === 'derived';
  if (c.status !== 'normal' || c.value_kind === 'enum' || (derived ? !c.quantity_spec || !c.output_semantics?.time_semantics : !c.unit_version_id))
    throw new Error('请选择在用且具有固定单位和时间含义的数值通道。');
  const time = derived ? c.output_semantics!.time_semantics! : {kind:'instant' as const};
  if (!derived && c.timestamp_meaning !== 'observation_time') throw new Error('区间观测需要指定聚合含义，请使用自定义规则。');
  // Raw channels carry a fixed unit, but do not declare a ratio/affine level scale.
  const quantity: DerivedQuantity = derived ? c.quantity_spec as DerivedQuantity :
    {base_unit_version_id:c.unit_version_id!,base_power:1,time_power:0,quantity_mode:'level',level_scale:'unknown'};
  return {key,name:c.owner_name+' · '+c.name,shape:'numeric',source_kinds:[c.source_kind],
    quantity_spec:quantity,time_semantics:time,direction:compare?'must_match':'not_applicable',reference:compare?'must_match':'not_applicable'};
}

export function NewAnalysis({prefill,onSelect}: {prefill:DerivedSearch;onSelect:(id:string)=>Promise<void>}) {
  const m = useDerived(), [method,setMethod] = useState<Method>('subtract');
  const [first,setFirst] = useState(''), [second,setSecond] = useState('');
  const [seed,setSeed] = useState<{body?:AnalysisBody;name?:string}>();
  const a = first || prefill.channel || '';
  const channel = (id:string) => ({queryKey:[...m.prefix,'bound-channel',id],
    queryFn:({signal}:{signal:AbortSignal})=>m.client.monitoringDirectory(m.scope,{channel_id:id},signal),enabled:!!id&&m.canRead&&m.canNavigate});
  const left=useQuery(channel(a)), right=useQuery(channel(second));
  const build=useOperation({mutationFn:async()=>{
    const one=left.data?.items[0], two=right.data?.items[0], pair=method==='subtract';
    if (!one || pair&&!two) throw new Error('请选好本次分析使用的测点。');
    if (pair&&a===second) throw new Error('请为两处输入选择不同的通道。');
    const inputs=[inputFor(one,'x',pair),...(pair&&two?[inputFor(two,'y',true)]:[])];
    const keys=inputs.map(v=>'input:'+v.key);
    const node:DerivedRule['nodes'][number]=method==='mean'
      ? {id:'result',op:'window_mean',inputs:keys,params:{window:{kind:'whole_range'},min_samples:'1'}}
      : method==='subtract' ? {id:'result',op:'subtract',inputs:keys,params:{alignment:'exact'}}
      : {id:'result',op:'difference',inputs:keys,params:{difference_mode:'adjacent',invalid_policy:'break',max_gap_ns:'1800000000000'}};
    const name=pair?one.owner_name+'与'+two!.owner_name+'差值':one.owner_name+' · '+one.name+(method==='mean'?'时段均值':'相邻变化');
    const rule:DerivedRule={...emptyRule(),inputs,nodes:[node],clock:method==='mean'?{kind:'whole_range'}:{kind:'primary_input',input:'x'},
      outputs:[{key:'value',name:method==='subtract'?'两处读数差值':method==='mean'?'时段平均值':'相邻变化',semantic_key:method==='mean'?'whole_range_mean':method,
        node:'result',numeric_contract:{scale:6,rounding:'half_even'}}]};
    const body:AnalysisBody={rule,input_bindings:{x:{channel_id:a},...(pair?{y:{channel_id:second}}:{})},parameters:{}};
    return {body,name};
  },onSuccess:setSeed});
  if (seed) return <div className="space-y-3"><AnalysisEditor initialBody={seed.body} initialName={seed.name} prefill={prefill} onSelect={onSelect}/></div>;
  const chooseMethod=(value:Method)=>{setMethod(value);build.reset();};
  const meter = (c?:MonitoringChannel) => c && <p className="text-sm text-muted-foreground">{c.facility_name} · {c.owner_name} · {c.unit}</p>;
  return <Section title="新建历史分析"><SelectField label="想分析什么" value={method} onChange={chooseMethod}
    options={(Object.keys(methods) as Method[]).map(value=>({value,label:methods[value].label}))}/>
    <p className="text-sm text-muted-foreground">{methods[method].meaning}</p>
    <div className="space-y-3 rounded-lg bg-muted/25 p-3"><h3 className="font-semibold">{method==='subtract'?'第一处（被减数）':'选择测点'}</h3>
      <BoundChannel key={'a'+a} value={a} onChange={setFirst}/>{meter(left.data?.items[0])}</div>
    {method==='subtract'&&a&&<div className="space-y-3 rounded-lg bg-muted/25 p-3"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">第二处（减数）</h3>
      {second&&<Button size="sm" variant="ghost" onClick={()=>{setFirst(second);setSecond(a);}}><ArrowLeftRight/>交换顺序</Button>}</div>
      <BoundChannel key={'b'+second} value={second} onChange={setSecond}/>{meter(right.data?.items[0])}</div>}
    <CommandError error={build.error??left.error??right.error}/>
    <div className="flex flex-wrap gap-2"><Button disabled={build.isPending||!left.data?.items.length||method==='subtract'&&!right.data?.items.length} onClick={()=>build.mutate()}>使用这些测点<ChevronRight/></Button>
      <Button variant="outline" onClick={()=>setSeed({})}><SlidersHorizontal/>自定义规则</Button></div>
  </Section>;
}
