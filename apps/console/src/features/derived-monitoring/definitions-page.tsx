import { useState } from 'react';
import { flushSync } from 'react-dom';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { DerivedMetricDefinition, DerivedMetricVersion, DerivedRuleValidation } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { CursorButtons } from '../monitoring/common';
import { CommandError, DerivedTabs, Section, TextField, UnsavedChanges, emptyRule, useDerived } from './common';
import { RuleEditor } from './rule-editor';
import { useCursor } from './use-cursor';

export function DerivedDefinitionsPage({id}:{id?:string}) {
 const m=useDerived(),navigate=useNavigate(),[q,setQ]=useState(''),page=useCursor();
 const list=useQuery({queryKey:[...m.prefix,'derived-definitions',q,page.cursor],queryFn:({signal})=>m.client.derivedMetrics(m.scope,{q,cursor:page.cursor},signal),enabled:m.allows('metrics.read')});
 const detail=useQuery({queryKey:[...m.prefix,'derived-definition',id],queryFn:({signal})=>m.client.derivedMetric(m.scope,id!,signal),enabled:!!id&&id!=='new'&&m.allows('metrics.read')});
 const choose=(value:string)=>void navigate({to:m.metricsPath,search:{id:value}});
 return <div className="space-y-6"><PageHeader title="派生指标定义" description="声明指标含义、输入条件与计算规则；发布固定版本后供派生通道复用。" actions={m.scope.kind==='tenant'&&m.allows('metrics.manage')&&m.editable?<Button onClick={()=>choose('new')}>新建指标定义</Button>:undefined}/><DerivedTabs current="metrics"/>{!m.allows('metrics.read')?<p role="status">当前身份没有指标定义读取权限。</p>:<div className="grid items-start gap-6 xl:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]"><Section title="指标目录"><TextField label="搜索指标名称或编码" value={q} onChange={v=>{setQ(v);page.reset();}}/><CommandError error={list.error}/>{list.isPending?<LoadingState/>:<ul className="space-y-1">{list.data?.items.map(v=><li key={v.id}><button type="button" aria-current={id===v.id?'true':undefined} className={`min-h-14 w-full rounded-lg px-3 py-2 text-left hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring ${id===v.id?'bg-muted':''}`} onClick={()=>choose(v.id)}><span className="block break-all font-medium">{v.name}</span><span className="block text-xs text-muted-foreground">{v.code} · {v.latest_version==='0'?'未发布':`已发布 V${v.latest_version}`}</span></button></li>)}{list.data?.items.length===0&&<li className="py-4 text-sm text-muted-foreground">暂无匹配的指标定义。</li>}</ul>}<CursorButtons previous={page.previous} next={list.data?.next_cursor?()=>page.next(list.data!.next_cursor):undefined} busy={list.isFetching}/></Section><div className="min-w-0 space-y-4"><CommandError error={detail.error}/>{id==='new'?<MetricEditor key="new" onSelect={choose}/>:detail.data?<MetricEditor key={detail.data.id} definition={detail.data} onSelect={choose}/>:detail.isFetching?<LoadingState/>:<Section title="选择指标定义"><p className="text-sm text-muted-foreground">从左侧选择定义查看草稿和发布历史，或创建新定义。</p></Section>}</div></div>}</div>;
}
function MetricEditor({definition,onSelect}:{definition?:DerivedMetricDefinition;onSelect:(id:string)=>void}) {
 const m=useDerived(),cache=useQueryClient(),meta=useRequestMeta();
 const initial={code:definition?.code??'',name:definition?.name??'',description:definition?.description??'',rule:definition?.draft?.rule??emptyRule(),base_version_id:definition?.draft?.base_version_id};
 const [editing,setEditing]=useState(!definition||!!definition.draft);
 const [fields,setFields]=useState(initial),[saved,setSaved]=useState(JSON.stringify(initial)),[current,setCurrent]=useState(definition),[reason,setReason]=useState(''),[validation,setValidation]=useState<DerivedRuleValidation>(),[notice,setNotice]=useState('');
 const [copyCode,setCopyCode]=useState(''),[copyName,setCopyName]=useState(''),[copyVersion,setCopyVersion]=useState('');
 const pages=useCursor(),[view,setView]=useState<DerivedMetricVersion>();
 const versions=useQuery({queryKey:[...m.prefix,'derived-versions',current?.id,pages.cursor],queryFn:({signal})=>m.client.derivedMetricVersions(m.scope,current!.id,{cursor:pages.cursor},signal),enabled:!!current});
 const writable=m.scope.kind==='tenant'&&m.allows('metrics.manage')&&m.editable;
 const dirty=JSON.stringify(fields)!==saved;
 const update=(patch:Partial<typeof fields>)=>{setFields(v=>({...v,...patch}));setValidation(undefined);setNotice('');};
 const operation=useOperation({mutationFn:async(action:'save'|'validate'|'publish'|'delete'|'copy')=>{
  if(m.scope.kind!=='tenant')throw new Error('指标定义由租户工作空间维护。');
  if(action==='validate'){if(!current?.draft)throw new Error('请先保存草稿。');const v=await m.client.derivedValidateMetric(m.scope,current.id,{draft_revision:current.draft.revision});setValidation(v);return;}
  if(action==='save'){
   const input={name:fields.name,description:fields.description,rule:fields.rule,base_version_id:fields.base_version_id};
   const body=current?{...input,expected_revision:current.revision}:{...input,code:fields.code};
   const result=current?await m.client.derivedSaveMetric(m.scope,current.id,{...input,expected_revision:current.revision,...meta('save',body,reason)}):await m.client.derivedCreateMetric(m.scope,{code:fields.code,...input,...meta('create',body,reason)});
   setCurrent(result.definition);flushSync(()=>setSaved(JSON.stringify(fields)));setValidation(undefined);setNotice('草稿已保存，尚未发布。');if(!current)onSelect(result.definition.id);
  } else if(action==='publish'){
   if(!current?.draft||!validation?.valid||dirty)throw new Error('请保存并校验当前草稿。');
   const body={expected_revision:current.revision,draft_revision:current.draft.revision,validation_hash:validation.validation_hash};
   const result=await m.client.derivedPublishMetric(m.scope,current.id,{...body,...meta('publish',body,reason)});setCurrent(result.definition);setEditing(false);setValidation(undefined);setNotice(`已发布固定版本 V${result.version?.version_no}。`);
  } else if(action==='delete'){
   if(!current?.draft)throw new Error('没有可删除草稿。');const body={expected_revision:current.revision};await m.client.derivedDeleteMetricDraft(m.scope,current.id,{...body,...meta('delete',body,reason)});flushSync(()=>setSaved(JSON.stringify(fields)));onSelect('');
  } else {
   if(!current||!copyVersion)throw new Error('请选择复制来源版本。');const body={code:copyCode,name:copyName,source_version_id:copyVersion};const result=await m.client.derivedCopyMetric(m.scope,current.id,{...body,...meta('copy',body,reason)});onSelect(result.definition.id);
  }
  await cache.invalidateQueries({queryKey:m.prefix});
 }});
 const state=useOperation({mutationFn:async(version:DerivedMetricVersion)=>{
  if(m.scope.kind!=='tenant')throw new Error('需要租户入口');const body={enabled:!version.enabled,expected_revision:version.state_revision};await m.client.derivedSetVersionState(m.scope,version.id,{...body,...meta(`state:${version.id}`,body,reason)});await versions.refetch();setNotice(version.enabled?'版本已停用，既有配置和历史证据仍保留。':'版本已启用。');
 }});
 const [templatePending,setTemplatePending]=useState(false);
 const busy=operation.isPending||state.isPending||templatePending;
 return <div className="space-y-5"><UnsavedChanges dirty={dirty}/><Section title={current?`${current.name} · ${current.code}`:'新建指标定义'}><div className="grid gap-4 sm:grid-cols-2"><TextField label="指标编码" value={fields.code} onChange={code=>update({code})} disabled={!!current||!writable} path="/code"/><TextField label="指标名称" value={fields.name} onChange={name=>update({name})} disabled={!writable||!editing} path="/name"/></div><TextField label="业务说明" value={fields.description} onChange={description=>update({description})} multiline disabled={!writable||!editing} path="/description"/>{current&&!current.draft&&<p role="status" className="text-sm">当前没有草稿。可从下方发布历史选择“基于此版本编辑”。</p>}</Section>{editing&&<RuleEditor onPendingChange={setTemplatePending} rule={fields.rule} onChange={rule=>update({rule})} readOnly={!writable}/>}<Section title="保存、校验与发布"><TextField label="操作原因" value={reason} onChange={setReason} hint="写入审计记录，发布、停用与删除时请说明业务原因。"/><CommandError error={operation.error??state.error} issues={validation?.issues}/>{notice&&<p role="status" className="rounded-md bg-muted p-3">{notice}</p>}{validation&&<div role="status" className="space-y-2 rounded-md border p-3"><p className="font-medium">{validation.valid?'校验通过，可发布当前已保存草稿':'校验未通过，请修正规则'}</p><p className="text-sm">{validation.input_count} 个输入 · {validation.node_count} 个步骤</p>{validation.issues.map((v,i)=><p key={i} className="text-sm">{v.path}：{v.message}</p>)}{validation.normalized_rule.outputs.map(v=><p key={v.key} className="text-sm">{v.name||v.key}：{v.quantity_spec?.quantity_mode} · 小数 {v.numeric_contract.scale} 位</p>)}</div>}<div className="flex flex-wrap gap-2">{writable&&editing&&<><Button disabled={busy} onClick={()=>operation.mutate('save')}>{current?'保存草稿':'创建草稿'}</Button><Button variant="outline" disabled={busy||dirty||!current?.draft} onClick={()=>operation.mutate('validate')}>校验已保存草稿</Button></>}{m.allows('metrics.publish')&&m.editable&&<Button disabled={busy||dirty||!validation?.valid} onClick={()=>operation.mutate('publish')}>发布固定版本</Button>}{writable&&current?.draft&&<Button variant="ghost" disabled={busy} onClick={()=>operation.mutate('delete')}>删除此草稿</Button>}</div></Section>{current&&<Section title="发布历史"><DirectoryTable label="指标发布版本" items={versions.data?.items??[]} rowId={v=>v.id} loading={versions.isPending} error={versions.error?.message} columns={[{id:'version',label:'版本',cell:v=><Button variant="link" onClick={()=>setView(v)}>V{v.version_no} · {v.name}</Button>},{id:'state',label:'可用情况',cell:v=>v.usable?'可用于新配置':v.enabled?'引用的公共版本不可用':'已停用'},{id:'time',label:'发布时间',secondary:true,cell:v=>new Date(v.published_at).toLocaleString()},{id:'actions',label:'操作',cell:v=><div className="flex flex-wrap gap-2">{writable&&<Button size="sm" variant="outline" disabled={dirty||busy} onClick={()=>{setEditing(true);update({rule:structuredClone(v.rule),base_version_id:v.id});setView(undefined);}}>基于此版本编辑</Button>}{m.allows('metrics.publish')&&m.editable&&<Button size="sm" variant="ghost" disabled={busy} onClick={()=>state.mutate(v)}>{v.enabled?'停用':'启用'}</Button>}</div>}]} pagination={<CursorButtons previous={pages.previous} next={versions.data?.next_cursor?()=>pages.next(versions.data!.next_cursor):undefined} busy={versions.isFetching}/>}/>{view&&<details open><summary className="cursor-pointer py-3 font-medium">固定版本 V{view.version_no} 的规则</summary><RuleEditor rule={view.rule} onChange={()=>{}} readOnly/></details>}{writable&&<details><summary className="cursor-pointer py-3 font-medium">复制为新指标定义</summary><div className="space-y-3"><label className="block text-sm">来源版本<select className="mt-2 min-h-11 w-full rounded-md border bg-background px-3" value={copyVersion} onChange={e=>setCopyVersion(e.target.value)}><option value="">请选择来源版本</option>{versions.data?.items.map(v=><option key={v.id} value={v.id}>V{v.version_no} · {v.name}</option>)}</select></label><TextField label="新指标编码" value={copyCode} onChange={setCopyCode}/><TextField label="新指标名称" value={copyName} onChange={setCopyName}/><Button variant="outline" disabled={busy||!copyVersion} onClick={()=>operation.mutate('copy')}>复制为独立草稿</Button></div></details>}</Section>}</div>;
}
