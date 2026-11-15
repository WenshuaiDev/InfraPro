import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from 'react';
import { useBlocker } from '@tanstack/react-router';
import { ApiError, type DerivedRule, type DerivedIssue, type PermissionCode } from '@infrapro/api-client';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Textarea } from '@infrapro/ui/components/textarea';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@infrapro/ui/components/alert-dialog';
import { useMonitoring } from '../monitoring/context';

// All requests and caches inherit the explicit workspace; project pages never
// substitute a tenant identity when a project operation is denied.
// eslint-disable-next-line react-refresh/only-export-components
export function useDerived() {
 const m=useMonitoring();
 const allows=(suffix:string)=>m.can(`${m.scope.kind}.derived.${suffix}` as PermissionCode);
 return {...m,allows,editable:m.available&&!m.archived,derivedPath:`${m.paths.root}/derived`,metricsPath:`${m.paths.root}/derived-definitions`,analysisPath:`${m.paths.root}/analysis`,runsPath:`${m.paths.root}/derived-runs`};
}
export function TextField({label,value,onChange,path,hint,multiline=false,disabled=false,type='text'}:{label:string;value:string;onChange:(v:string)=>void;path?:string;hint?:string;multiline?:boolean;disabled?:boolean;type?:string}){
 const generated=useId(),id=path??generated;
 return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label>{multiline?<Textarea id={id} value={value} disabled={disabled} aria-describedby={hint?`${id}-hint`:undefined} onChange={e=>onChange(e.target.value)}/>:<Input id={id} className="min-h-11" type={type} value={value} disabled={disabled} aria-describedby={hint?`${id}-hint`:undefined} onChange={e=>onChange(e.target.value)}/>}{hint&&<p id={`${id}-hint`} className="text-sm text-muted-foreground">{hint}</p>}</div>;
}
export function SelectField<T extends string>({label,value,onChange,options,path,disabled=false}:{label:string;value:T;onChange:(v:T)=>void;options:readonly {value:T;label:string}[];path?:string;disabled?:boolean}){
 const generated=useId(),id=path??generated;
 return <div className="min-w-0 space-y-2"><Label htmlFor={id}>{label}</Label><select id={id} className="min-h-11 w-full rounded-md border bg-background px-3 py-2 text-sm" value={value} disabled={disabled} onChange={e=>onChange(e.target.value as T)}>{!options.some(v=>v.value===value)&&<option value={value}>{value?'当前选择':'请选择'}</option>}{options.map(v=><option key={v.value} value={v.value}>{v.label}</option>)}</select></div>;
}
export function Section({title,path,actions,children}:{title:string;path?:string;actions?:ReactNode;children:ReactNode}){
 return <section id={path} tabIndex={-1} className="min-w-0 scroll-mt-20 space-y-4 rounded-xl border p-4 md:p-5"><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="min-w-0 break-all font-semibold">{title}</h2>{actions}</div>{children}</section>;
}
export function CommandError({error,issues:provided}:{error:Error|null;issues?:DerivedIssue[]}){
 const ref=useRef<HTMLDivElement>(null);
 useEffect(()=>{if(error||provided?.length)ref.current?.focus();},[error,provided]);
 if(!error&&!provided?.length)return null;
 const issues=provided?.length?provided:error instanceof ApiError?error.issues:[];
 const navigate=(path:string)=>{path=path.replace(/^\/body(?=\/)/,'');let target:HTMLElement|null=null;while(path&&!target){target=document.getElementById(path);path=path.slice(0,path.lastIndexOf('/'));}target?.scrollIntoView({block:'center'});target?.focus();};
 return <div ref={ref} role="alert" tabIndex={-1} className="space-y-3 rounded-lg border border-destructive/40 bg-destructive/5 p-4"><h2 className="font-semibold">{error?.message??'规则校验未通过，请修正以下项目'}</h2>{issues.length>0&&<ul className="space-y-2 text-sm">{issues.map((v,i)=><li key={i}><button type="button" className="text-left underline underline-offset-4" onClick={()=>navigate(v.path)}>{v.message} · {v.path}</button></li>)}</ul>}</div>;
}
export function UnsavedChanges({dirty}:{dirty:boolean}){
 // A successful create saves and navigates in the same event. The router may
 // still hold its previous callback until its passive effect runs; read the
 // committed value so that a saved draft never raises a false discard prompt.
 const committedDirty=useRef(dirty);
 useLayoutEffect(()=>{committedDirty.current=dirty;},[dirty]);
 const blocker=useBlocker({shouldBlockFn:()=>committedDirty.current,withResolver:true,enableBeforeUnload:dirty});
 return <AlertDialog open={blocker.status==='blocked'}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>离开尚未保存的编辑？</AlertDialogTitle><AlertDialogDescription>本次未保存的修改会丢失。已保存的草稿仍可在原工作空间继续编辑。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel onClick={()=>blocker.reset?.()}>继续编辑</AlertDialogCancel><AlertDialogAction onClick={()=>blocker.proceed?.()}>放弃修改并离开</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function nsISO(ns:string):string{
 if(ns.startsWith('invalid:'))return ns.slice(8);
 try {const n=BigInt(ns);const second=n>=0n?n/1000000000n:(n-999999999n)/1000000000n;const fraction=n-second*1000000000n;return new Date(Number(second*1000n)).toISOString().slice(0,19)+'.'+fraction.toString().padStart(9,'0')+'Z';}catch{return ns;}
}
// eslint-disable-next-line react-refresh/only-export-components
export function isoNS(value:string):string{
 const m=/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value);
 if(!m)return `invalid:${value}`;
 const ms=Date.parse(m[1]!+m[3]!);return Number.isFinite(ms)?(BigInt(ms)*1000000n+BigInt((m[2]??'').padEnd(9,'0'))).toString():`invalid:${value}`;
}
export function TimeField({label,value,onChange,path}:{label:string;value:string;onChange:(v:string)=>void;path?:string}){
 return <TextField label={label} path={path} value={nsISO(value)} onChange={v=>onChange(isoNS(v))} hint="完整时刻（含时区），例如 2024-01-01T00:00:00+08:00；最多保留9位小数。"/>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function emptyRule():DerivedRule{
 return {schema_version:'derived-rule/1',inputs:[{key:'x',name:'输入一',shape:'numeric',source_kinds:['direct','derived'],quantity_spec:{base_power:0,time_power:0,quantity_mode:'ratio',level_scale:'ratio',dimensionless_kind:'ratio'},time_semantics:{kind:'instant'},direction:'not_applicable',reference:'not_applicable'}],clock:{kind:'primary_input',input:'x'},nodes:[{id:'result',op:'abs',inputs:['input:x'],params:{}}],outputs:[{key:'value',name:'绝对值',node:'result',semantic_key:'absolute',numeric_contract:{scale:18,rounding:'half_even'}}],parameters:[],policies:{quality:['good','unmarked'],config_boundary:'break',allowed_lateness_ns:'0'}};
}
