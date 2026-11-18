import { defaultMetricUnit, metricUnitOptions } from '../engineering/metric-units';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useState } from 'react';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { QuantityLabel } from './rule-summary';
import { useIssueReveal } from './editor-navigation';
import { useQuery } from '@tanstack/react-query';
import type { DerivedQuantity, DerivedRule, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { VersionPicker } from '../engineering/reference-picker';
import { CommandError, Section, SelectField, TextField, useDerived } from './common';

type Input=DerivedRule['inputs'][number];
type Adopt = (id: string, version?: EngineeringVersion, dependencies?: EngineeringVersion[]) => void;
type PickerProps = { kind: 'units' | 'metrics'; value: string; onChange: Adopt; label: string };
function useFixedVersion(kind: PickerProps['kind'], id: string) {
  const m = useDerived();
  return useQuery({ queryKey: [...m.prefix, 'fixed-public-version', kind, id], enabled: !!id,
    queryFn: ({ signal }) => m.client.resourceCatalogVersion(m.scope, kind, id, signal) });
}
function PublicVersionPicker({ kind, value, onChange, label }: PickerProps) {
  const m = useDerived();
  const fixed = useFixedVersion(kind, value);
  if (m.scope.kind === 'tenant') return <VersionPicker key={fixed.data?.version.definition_id ?? 'select'}
    known={fixed.data ? [fixed.data.version, ...fixed.data.dependencies] : []} kind={kind} value={value} onChange={onChange} label={label}
    acceptVersion={kind === 'metrics' ? version => !!version.content.metric && version.content.metric.shape !== 'enum' : undefined} />;
  return <ProjectVersionPicker current={fixed.data?.version} kind={kind} value={value} onChange={onChange} label={label} />;
}
function ProjectVersionPicker({ kind, value, onChange, label, current }: PickerProps & { current?: EngineeringVersion }) {
  const m = useDerived(), [q, setQ] = useState(''), [pageSize, setPageSize] = useState(20), [offset, setOffset] = useState(0);
  const [candidate, setCandidate] = useState(value);
  const choice = useFixedVersion(kind, kind === 'metrics' ? candidate : '');
  const query = useQuery({ queryKey: [...m.prefix, 'derived-catalog', kind, q, offset, pageSize], queryFn: ({signal}) => {
    if (m.scope.kind !== 'project') throw new Error('需要项目空间');
    return m.client.projectResourceCatalog(m.scope, kind, { q, offset, limit: pageSize }, signal);
  }});
  const options = (query.data?.items ?? []).map(v => ({ value: v.id, label: v.name + ' · ' + v.code + ' V' + v.number }));
  for (const v of [current, choice.data?.version]) if (v && !options.some(option => option.value === v.id)) options.unshift({value: v.id, label: v.content.name + ' · V' + v.number});
  const isEnum = choice.data?.version.content.metric?.shape === 'enum';
  return <div className="space-y-3 rounded-lg bg-muted/30 p-3"><TextField label={label + '搜索'} value={q} onChange={v => { setQ(v); setOffset(0); }} />
    <CommandError error={query.error ?? choice.error} /><SelectField label={label} value={kind === 'metrics' ? candidate : value} onChange={id => {
      if (kind === 'metrics') setCandidate(id);
      if (!id || kind === 'units') onChange(id);
    }} options={[{value: '', label: '不限定公共版本'}, ...options]} />
    {kind === 'metrics' && candidate && <><Button variant="outline" type="button" disabled={!choice.data?.version.usable || isEnum || candidate === value}
      onClick={() => { if (choice.data) onChange(candidate, choice.data.version, choice.data.dependencies); }}>采用所选指标</Button>
      {isEnum && <p role="status">派生输入需要数值指标。</p>}</>}
    <ListPagination label="工程目录" total={query.data?.total} page={Math.floor(offset / pageSize)} go={n => setOffset(n * pageSize)} pageSize={pageSize} setPageSize={size => { setPageSize(size); setOffset(0); }} /></div>;
}
function MetricQuantityUnit({ metricId, value, change }: { metricId: string; value: string; change: (id: string) => void }) {
  const fixed = useFixedVersion('metrics', metricId);
  const options = metricUnitOptions(fixed.data?.version.content.metric, fixed.data?.dependencies ?? [], value);
  return <><SelectField label="固定单位版本" value={value} onChange={change} disabled={!fixed.data}
    options={[{value: '', label: '选择指标允许的单位'}, ...options]} /><CommandError error={fixed.error} /></>;
}
function catalogQuantity(previous: DerivedQuantity, id: string): DerivedQuantity {
  return { base_unit_version_id: id || undefined, base_power: 1, time_power: 0,
    quantity_mode: previous.quantity_mode === 'difference' ? 'difference' : 'level', level_scale: previous.base_power ? previous.level_scale : 'unknown' };
}

export function QuantityEditor({ value, onChange, path, metricId }: { value: DerivedQuantity; onChange: (v: DerivedQuantity) => void; path: string; metricId?: string }) {
  const dimensionless = !metricId && value.base_power === 0;
  const modes = dimensionless ? [{value:'ratio',label:'比值'}, {value:'count',label:'计数'}] as const
    : [{value:'level',label:'量值'}, {value:'difference',label:'差值'}] as const;
  return <div id={path} tabIndex={-1} className="space-y-3">
    <div className="grid gap-3 sm:grid-cols-2">
      <SelectField label="物理量" disabled={!!metricId} value={dimensionless ? 'dimensionless' : 'unit'}
        onChange={kind => onChange(kind === 'dimensionless' ? {base_power:0,time_power:0,quantity_mode:'ratio',level_scale:'ratio',dimensionless_kind:'ratio'} : catalogQuantity(value, ''))}
        options={[{value:'unit',label:'带计量单位'},{value:'dimensionless',label:'无量纲'}]} />
      <SelectField label="数值含义" value={value.quantity_mode} onChange={mode => onChange({...value, quantity_mode:mode, ...(dimensionless ? {dimensionless_kind:mode === 'count' ? 'count' as const : 'ratio' as const} : {})})} options={modes} />
    </div>
    {!dimensionless && <>
      {metricId ? <MetricQuantityUnit metricId={metricId} value={value.base_unit_version_id ?? ''} change={id => onChange(catalogQuantity(value,id))} /> :
        <PublicVersionPicker label="固定单位版本" kind="units" value={value.base_unit_version_id ?? ''} onChange={id => onChange({...value,base_unit_version_id:id || undefined})} />}
      <div className="grid gap-3 sm:grid-cols-2">
        <SelectField label="量值尺度" value={value.level_scale} onChange={level_scale => onChange({...value, level_scale})}
          options={[{value:'unknown',label:'未知（限制乘除等运算）'},{value:'ratio',label:'比例尺度（有绝对零点）'},{value:'affine',label:'仿射尺度（例如摄氏温度）'}]} />
        {!metricId && <TextField label="单位幂次" value={String(value.base_power)} onChange={v => onChange({...value, base_power:Number(v)})} />}
      </div></>}
    {metricId ? (value.base_power !== 1 || value.time_power !== 0) && <Button type="button" variant="outline" onClick={() => onChange(catalogQuantity(value,value.base_unit_version_id ?? ''))}>恢复为指标单位</Button> :
      <div className="grid gap-3 sm:grid-cols-2"><SelectField label="时间量纲" value={value.time_power === 0 ? 'none' : value.time_unit ?? 'second'}
        onChange={v => onChange({...value,time_power:v === 'none' ? 0 : -1,time_unit:v === 'none' ? undefined : v as DerivedQuantity['time_unit']})}
        options={[{value:'none',label:'无时间分母'},{value:'second',label:'每秒'},{value:'minute',label:'每分钟'},{value:'hour',label:'每小时'},{value:'day',label:'每固定24小时'}]} />
        {value.time_power !== 0 && <TextField label="时间幂次（负数为分母）" value={String(value.time_power)} onChange={v => onChange({...value,time_power:Number(v)})} />}</div>}
  </div>;
}
export function RuleInputs({rule,onChange,compact=false,readOnly=false}:{rule:DerivedRule;onChange:(v:DerivedRule)=>void;compact?:boolean;readOnly?:boolean}){
 const [selected,setSelected]=useState<number>();
 useIssueReveal(path=>{const match=/^\/rule\/inputs\/(\d+)/.exec(path);if(match&&compact)setSelected(Number(match[1]));});
 const update=(i:number,v:Input)=>onChange({...rule,inputs:rule.inputs.map((old,j)=>i===j?v:old)});
 const editor=(input:Input,i:number)=><fieldset disabled={readOnly} className="min-w-0 space-y-4"><div className="grid gap-3 sm:grid-cols-2"><TextField path={`/rule/inputs/${i}/key`} label="槽位键（公式引用）" value={input.key} onChange={key=>update(i,{...input,key})}/><TextField label="输入名称" value={input.name??''} onChange={name=>update(i,{...input,name})}/></div><PublicVersionPicker kind="metrics" label="限定公共指标（可选）" value={input.metric_version_id??''} onChange={(id,version,deps) => update(i,{...input,metric_version_id:id || undefined,quantity_spec:id ? catalogQuantity(input.quantity_spec,defaultMetricUnit(version,deps)) : input.quantity_spec})}/><QuantityEditor metricId={input.metric_version_id} path={`/rule/inputs/${i}/quantity_spec`} value={input.quantity_spec} onChange={quantity_spec=>update(i,{...input,quantity_spec})}/><div className="grid gap-3 sm:grid-cols-2"><SelectField label="允许来源" value={input.source_kinds.length===2?'both':(input.source_kinds[0]??'direct')} onChange={v=>update(i,{...input,source_kinds:v==='both'?['direct','derived']:[v as 'direct'|'derived']})} options={[{value:'both',label:'原始观测或正式派生'},{value:'direct',label:'仅原始观测'},{value:'derived',label:'仅正式派生'}]}/><SelectField label="输入时间含义" value={input.time_semantics.kind} onChange={v=>update(i,{...input,time_semantics:{kind:v,...(v==='instant'?{}:{aggregation:'mean'})}})} options={[{value:'instant',label:'瞬时样本'},{value:'interval_end',label:'原始区间结果（结束时刻）'},{value:'derived_window',label:'派生窗口结果'}]}/>{input.time_semantics.kind!=='instant'&&<SelectField label="区间聚合含义" value={input.time_semantics.aggregation??'mean'} options={(['mean','sum','min','max','range','count','variance','stddev','median','quantile'] as const).map(value=>({value,label:value}))} onChange={aggregation=>update(i,{...input,time_semantics:{...input.time_semantics,aggregation}})}/>}<SelectField label="观测方向约束" value={input.direction} onChange={direction=>update(i,{...input,direction})} options={[{value:'must_match',label:'必须一致'},{value:'not_applicable',label:'不适用'}]}/><SelectField label="观测基准约束" value={input.reference} onChange={reference=>update(i,{...input,reference})} options={[{value:'must_match',label:'必须一致'},{value:'not_applicable',label:'不适用'}]}/></div></fieldset>;
 const remove=(i:number)=>{onChange({...rule,inputs:rule.inputs.filter((_,j)=>j!==i)});setSelected(undefined);};
 return <Section path="/rule/inputs" title="输入要求" actions={<Button type="button" variant="outline" disabled={readOnly||rule.inputs.length>=16} onClick={()=>{const i=rule.inputs.length;onChange({...rule,inputs:[...rule.inputs,{...structuredClone(rule.inputs[0]!),key:`x${i+1}`,name:`输入${i+1}`}]});setSelected(i);}}>添加输入</Button>}>
 <p className="text-sm text-muted-foreground">声明需要什么数据；创建通道时再绑定实际来源。单位、尺度、时间与方向均以这里的声明为准。</p>
 {compact?<div className="divide-y rounded-lg border">{rule.inputs.map((input,i)=><div key={i} id={`/rule/inputs/${i}`} tabIndex={-1} className="flex flex-wrap items-center gap-3 p-4"><span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-medium">{i+1}</span><div className="min-w-0 flex-1"><p className="break-words font-medium">{input.name||input.key} <span className="ml-2 text-xs font-normal text-muted-foreground">{input.key}</span></p><p className="text-xs text-muted-foreground"><QuantityLabel quantity={input.quantity_spec}/> · {input.time_semantics.kind==='instant'?'瞬时样本':input.time_semantics.kind==='interval_end'?'区间结束':'派生窗口'} · {input.source_kinds.length===2?'原始或派生':input.source_kinds[0]==='direct'?'原始观测':'正式派生'}</p></div><Button type="button" variant="outline" onClick={()=>setSelected(i)}>{readOnly?'查看要求':'设置要求'}</Button><Button type="button" variant="ghost" disabled={readOnly||rule.inputs.length===1} aria-label={`移除输入 ${i+1}`} onClick={()=>remove(i)}>移除</Button></div>)}</div>:rule.inputs.map((input,i)=><Section key={i} path={`/rule/inputs/${i}`} title={`输入 ${i+1} · ${input.name||input.key}`} actions={<Button type="button" variant="ghost" disabled={readOnly||rule.inputs.length===1} onClick={()=>remove(i)}>移除输入 {i+1}</Button>}>{editor(input,i)}</Section>)}
 {compact&&<ResponsivePanel open={selected!==undefined&&!!rule.inputs[selected]} onOpenChange={open=>{if(!open)setSelected(undefined);}} title={`输入 ${(selected??0)+1} · ${rule.inputs[selected??0]?.name??'输入要求'}`} description="配置当前输入的完整数据契约。修改暂存于表单，完成后请保存草稿。" size="wide" footer={<Button onClick={()=>setSelected(undefined)}>完成设置</Button>}>{selected!==undefined&&rule.inputs[selected]&&editor(rule.inputs[selected],selected)}</ResponsivePanel>}
 </Section>;
}
