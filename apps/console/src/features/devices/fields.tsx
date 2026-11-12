import { useId, type ReactNode } from 'react';
import { Label } from '@infrapro/ui/components/label';
import { Input } from '@infrapro/ui/components/input';
import { Textarea } from '@infrapro/ui/components/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import type { DeviceFieldIssue, DeviceOutput } from '@infrapro/api-client';
import { normalizeDeviceText } from '@infrapro/api-client';
import { z } from '../../config/validation';
// eslint-disable-next-line react-refresh/only-export-components
export const shortText=z.string().refine(v=>Array.from(normalizeDeviceText(v)).length<=200,'最多 200 字符。');
// eslint-disable-next-line react-refresh/only-export-components
export const requiredText=shortText.refine(v=>!!normalizeDeviceText(v),'请填写此项。');
// eslint-disable-next-line react-refresh/only-export-components
export const codeRule=z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/,'编号须为 1–64 位字母、数字、点、下划线或连字符。');
// eslint-disable-next-line react-refresh/only-export-components
export const noteRule=z.string().refine(v=>Array.from(normalizeDeviceText(v)).length<=2000,'最多 2,000 字符。');
export function DeviceField({name,label,value,onChange,issues=[],multiline=false,disabled=false,hint,children}:{name:string;label:string;value?:string;onChange?:(v:string)=>void;issues?:DeviceFieldIssue[];multiline?:boolean;disabled?:boolean;hint?:string;children?:ReactNode}){
 const id=`/${name}`;const messages=issues.filter(i=>i.path===id||i.path===`/content${id}`);const issueId=useId();return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label>{children??(multiline?<Textarea id={id} value={value??''} onChange={e=>onChange?.(e.target.value)} disabled={disabled} aria-invalid={!!messages.length} aria-describedby={messages.length?issueId:undefined}/>:<Input id={id} value={value??''} onChange={e=>onChange?.(e.target.value)} disabled={disabled} aria-invalid={!!messages.length} aria-describedby={messages.length?issueId:undefined}/>)}{hint&&<p className="text-xs text-muted-foreground">{hint}</p>}{!!messages.length&&<div id={issueId} className="text-sm text-destructive">{messages.map((m,i)=><p key={i}>{m.message}</p>)}</div>}</div>;
}
export function DeviceSelect({name,label,value,onChange,options,disabled=false,issues=[]}:{name:string;label:string;value:string;onChange:(v:string)=>void;options:{value:string;label:string}[];disabled?:boolean;issues?:DeviceFieldIssue[]}){return <DeviceField name={name} label={label} issues={issues}><Select value={value||undefined} onValueChange={onChange} disabled={disabled}><SelectTrigger id={`/${name}`} className="h-11 w-full min-w-0"><SelectValue placeholder="请选择"/></SelectTrigger><SelectContent>{options.map(o=><SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent></Select></DeviceField>;}
export function OutputMeaning({output}:{output:DeviceOutput}){
 const metric=output.definitions.find(v=>v.id===output.metric_version_id);const unit=output.definitions.find(v=>v.id===output.unit_version_id);const pairs=[['指标',metric?`${metric.content.name} v${metric.number}`:'固定指标版本'],['单位',unit?`${unit.content.name}（${unit.content.unit?.symbol??''}） v${unit.number}`:'无单位（枚举）'],['值形态',({integer:'整数',decimal:'小数',enum:'枚举'} as Record<string,string>)[output.value_shape]],['时间含义',output.timestamp_meaning==='observation_time'?'观测时刻':'统计区间结束时刻'],['统计含义',output.aggregation_description||'未填写'],['观测方向',output.observation_direction||'未填写'],['参考基准',output.reference_basis||'未填写']];return <dl className="grid min-w-0 gap-3 sm:grid-cols-2">{pairs.map(([name,value])=><div key={name} className="min-w-0"><dt className="text-xs text-muted-foreground">{name}</dt><dd className="whitespace-pre-wrap break-words text-sm">{value}</dd></div>)}</dl>;
}
