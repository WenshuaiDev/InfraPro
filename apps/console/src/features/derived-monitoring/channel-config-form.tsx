import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@infrapro/ui/components/alert-dialog';
import { EditorSteps, useIssueReveal } from './editor-navigation';
import { RuleSummary } from './rule-summary';
import { MetricEditor } from './definitions-page';
import { flushSync } from 'react-dom';
import type { DerivedChannel, DerivedChannelConfig, DerivedCreateChannelInput, DerivedSaveConfigInput, DerivedMetricVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { qualityNames } from '../monitoring/format';
import { BindingEditor, DefinitionVersionPicker, OwnerPicker } from './bindings-editor';
import { CommandError, Section, SelectField, TextField, UnsavedChanges, emptyRule, useDerived } from './common';
import { RuleEditor } from './rule-editor';

type Owner=DerivedCreateChannelInput['owner'];
type Props={channel?:DerivedChannel;base?:DerivedChannelConfig;initialOwner?:Owner;initialDefinition?:string;initialVersion?:string;onCreated:(v:DerivedChannel)=>void;onSaved?:(v:DerivedChannel,cfg:DerivedChannelConfig)=>void};
export function ChannelConfigForm(props:Props) {
 const m=useDerived();
 const version=useQuery({queryKey:[...m.prefix,'derived-version',props.initialVersion],queryFn:({signal})=>m.client.derivedMetricVersion(m.scope,props.initialVersion!,signal),enabled:!!props.initialVersion&&!props.base});
 if(props.initialVersion&&!props.base){if(version.isPending)return <LoadingState/>;if(version.error)return <CommandError error={version.error}/>;if(!version.data?.usable)return <p role="status">所选固定版本当前不可用于新配置，请从指标定义选择可用版本。</p>;}
 return <ConfigForm key={[props.base?.id,version.data?.id,props.initialOwner?.kind,props.initialOwner?.id].join(':')} {...props} selectedVersion={version.data}/>;
}
function ConfigForm({channel,base,initialOwner,initialDefinition,selectedVersion,onCreated,onSaved}:Props&{selectedVersion?:DerivedMetricVersion}) {
 const m=useDerived(),meta=useRequestMeta();
 const [rule,setRule]=useState(base?.rule??selectedVersion?.rule??emptyRule()),[version,setVersion]=useState(base?.definition_version_id??selectedVersion?.id??'');
 const [code,setCode]=useState(''),[name,setName]=useState(''),[description,setDescription]=useState(''),[owner,setOwner]=useState<Owner>(initialOwner??{kind:'point',facility_id:'',id:''});
 const [bindings,setBindings]=useState<DerivedCreateChannelInput['input_bindings']>(base?.input_bindings??{}),[parameters,setParameters]=useState<Record<string,string>>(base?.parameters??Object.fromEntries((selectedVersion?.rule.parameters??[]).map(p=>[p.key,p.default]))),[baselines,setBaselines]=useState<NonNullable<DerivedCreateChannelInput['baseline_bindings']>>(base?.baseline_bindings??{});
 const [quality,setQuality]=useState<NonNullable<DerivedCreateChannelInput['quality']>>(base?.rule?.policies.quality??selectedVersion?.rule.policies.quality??['good','unmarked']),[scale,setScale]=useState(String(base?.rule?.outputs[0]?.numeric_contract.scale??selectedVersion?.rule.outputs[0]?.numeric_contract.scale??18));
 const [change,setChange]=useState<'ordinary'|'correction'>('ordinary'),[semantic,setSemantic]=useState(false),[reset,setReset]=useState(false),[reason,setReason]=useState('');
 const [stage,setStage]=useState<'rule'|'inputs'|'parameters'>('rule'),[newMetric,setNewMetric]=useState(false),[metricDirty,setMetricDirty]=useState(false),[confirmReturn,setConfirmReturn]=useState(false);
 const selectVersion=(v:DerivedMetricVersion)=>{if(v.id===version)return;setVersion(v.id);setRule(v.rule);setBindings(old=>Object.fromEntries(v.rule.inputs.filter(i=>old[i.key]).map(i=>[i.key,old[i.key]!])));setParameters(old=>Object.fromEntries((v.rule.parameters??[]).map(p=>[p.key,old[p.key]??p.default])));setBaselines({});setQuality(v.rule.policies.quality??['good','unmarked']);setScale(String(v.rule.outputs[0]?.numeric_contract.scale??18));};
 useIssueReveal(path=>{if(!newMetric)setStage(path.startsWith('/input_bindings')?'inputs':path.startsWith('/parameters')||path.startsWith('/baseline_bindings')||path.startsWith('/quality')||path.startsWith('/numeric_contract')||path==='/reason'?'parameters':'rule');});
 const payload={version,code,name,description,owner,bindings,parameters,baselines,quality,scale,change,semantic,reset};
 const [saved,setSaved]=useState(JSON.stringify(payload));
 const operation=useOperation({mutationFn:async()=>{
  const input={definition_version_id:version,input_bindings:bindings,parameters,baseline_bindings:baselines,quality,numeric_contract:{scale:Number(scale),rounding:'half_even' as const}};
  if(channel&&base){const body={...input,base_config_id:base.id,change_kind:change,semantic_intent_confirmed:semantic,reset_baselines:reset,expected_revision:channel.config_revision};const request:DerivedSaveConfigInput={...body,...meta('save-config',body,reason)};const result=await m.client.derivedSaveConfig(m.scope,channel.resource.id,request);flushSync(()=>setSaved(JSON.stringify(payload)));onSaved?.(result.channel,result.config);}
  else{const body={...input,code,name,description,owner};const result=await m.client.derivedCreateChannel(m.scope,{...body,...meta('create-channel',body,reason)});flushSync(()=>setSaved(JSON.stringify(payload)));onCreated(result.channel);}
 }});
 return <div className="min-w-0 space-y-5"><UnsavedChanges dirty={JSON.stringify(payload)!==saved||newMetric&&metricDirty}/>
 {newMetric?<><div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-muted/30 p-4"><p className="text-sm">为当前通道新建规则。所属对象、名称与输入配置已暂存，发布后可直接返回。</p><Button variant="outline" onClick={()=>metricDirty?setConfirmReturn(true):setNewMetric(false)}>返回通道配置</Button></div><MetricEditor editingMode navigationGuard={false} onDirtyChange={setMetricDirty} onUseVersion={v=>{selectVersion(v);setNewMetric(false);}}/>
 <AlertDialog open={confirmReturn} onOpenChange={setConfirmReturn}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>放弃未保存的规则修改？</AlertDialogTitle><AlertDialogDescription>已保存的指标草稿仍可在指标定义中继续编辑，当前通道配置会保留。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>继续编辑</AlertDialogCancel><AlertDialogAction onClick={()=>{setNewMetric(false);setConfirmReturn(false);}}>放弃修改并返回通道</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></>:<>
 <EditorSteps label="通道配置步骤" value={stage} onChange={setStage} items={[{value:'rule',label:'对象与规则'},{value:'inputs',label:'绑定输入'},{value:'parameters',label:'参数与范围'}]}/>
 <CommandError error={operation.error}/>
 <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_17rem]"><div className="min-w-0 space-y-5">
 <div hidden={stage!=='rule'} className="space-y-5">{!channel&&<><Section title="通道业务含义"><div className="grid gap-4 sm:grid-cols-2"><TextField label="通道编码" path="/code" value={code} onChange={setCode}/><TextField label="通道名称" path="/name" value={name} onChange={setName}/></div><TextField label="用途说明" path="/description" value={description} onChange={setDescription} multiline/></Section><OwnerPicker value={owner} onChange={setOwner}/></>}
 <DefinitionVersionPicker value={version} initialDefinition={initialDefinition??selectedVersion?.definition_id} onChange={selectVersion}/>
 {m.scope.kind==='tenant'&&m.allows('metrics.manage')&&m.editable&&<div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-dashed p-4"><p className="text-sm text-muted-foreground">没有合适的规则？先创建并发布指标定义。</p><Button variant="outline" onClick={()=>setNewMetric(true)}>新建规则并返回</Button></div>}
 {version&&<details><summary className="min-h-11 cursor-pointer py-3 font-medium">查看所选版本的固定规则</summary><RuleEditor rule={rule} onChange={()=>{}} readOnly/></details>}</div>
 {!version&&stage!=='rule'&&<Section title="请先选择指标规则"><p className="text-sm text-muted-foreground">固定版本决定输入要求和可配置参数。</p><Button variant="outline" onClick={()=>setStage('rule')}>选择固定版本</Button></Section>}
 {version&&<><BindingEditor section={stage==='rule'?'none':stage} rule={rule} value={bindings} parameters={parameters} baselines={baselines} onChange={setBindings} onParameters={setParameters} onBaselines={setBaselines}/>
 <div hidden={stage!=='parameters'} className="space-y-5"><Section title="本次配置的质量与精度"><fieldset id="/quality"><legend className="text-sm font-medium">参与计算的输入质量</legend><div className="flex flex-wrap gap-4">{(['good','unmarked','suspect','bad'] as const).map(q=><label key={q} className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={quality.includes(q)} onChange={e=>setQuality(old=>e.target.checked?[...old,q]:old.filter(v=>v!==q))}/>{qualityNames[q]}</label>)}</div></fieldset><TextField path="/numeric_contract/scale" label="结果保留小数位（0–60）" value={scale} onChange={setScale}/></Section>
 {channel&&<Section title="配置变更意图"><SelectField label="变更方式" value={change} onChange={v=>{setChange(v);setSemantic(false);setReset(false);}} options={[{value:'ordinary',label:'普通修改，从明确时刻起生效'},{value:'correction',label:'保持原业务含义，修正明确历史范围'}]}/><p className="text-sm text-muted-foreground">改变窗口或公式若改变了指标业务含义，应新建通道；历史修正会保留原配置和原结果。</p>{change==='correction'&&<><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={semantic} onChange={e=>setSemantic(e.target.checked)}/>确认修正后仍表达原有业务含义</label><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={reset} onChange={e=>setReset(e.target.checked)}/>重新冻结基准（不勾选则继承原固定依据）</label></>}</Section>}
 <Section title={channel?'保存候选配置':'准备试算'}><TextField path="/reason" label="操作原因" value={reason} onChange={setReason}/><p className="text-sm text-muted-foreground">保存固定配置后，在下一步选择明确的试算范围。核对完整结果，再预览自动起点与启用影响。</p></Section></div></>}
 </div>{version?<RuleSummary rule={rule}/>:<aside className="rounded-xl border bg-muted/20 p-5 text-sm text-muted-foreground">每个通道固定使用一个已发布指标版本。先明确所属工程对象，再选择规则和实际输入。</aside>}</div>
 <div className="sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-background/95 p-4 shadow-sm backdrop-blur"><p className="text-xs text-muted-foreground">{channel?'保存后生成候选配置，不会立即替换当前规则。':'创建后处于待启用状态，不会立即自动计算。'}</p><div className="flex flex-wrap gap-2">{stage!=='rule'&&<Button variant="ghost" onClick={()=>setStage(stage==='parameters'?'inputs':'rule')}>上一步</Button>}{stage!=='parameters'?<Button disabled={!version} onClick={()=>setStage(stage==='rule'?'inputs':'parameters')}>下一步</Button>:<Button disabled={!version||operation.isPending||change==='correction'&&!semantic} onClick={()=>operation.mutate()}>{operation.isPending?'正在保存…':channel?'保存候选配置，随后试算':'创建通道并进入试算'}</Button>}</div></div>
 </>}
 </div>;
}
