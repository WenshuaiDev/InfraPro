import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { useEffect, useState } from 'react';
import { flushSync } from 'react-dom';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { DerivedMetricDefinition, DerivedMetricVersion, DerivedRuleValidation } from '@infrapro/api-client';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { RuleSummary, QuantityLabel } from './rule-summary';
import { EditorSteps, useIssueReveal } from './editor-navigation';
import type { DerivedSearch } from './search';
import { Button } from '@infrapro/ui/components/button';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { CommandError, Section, SelectField, TextField, UnsavedChanges, emptyRule, useDerived } from './common';
import { RuleEditor } from './rule-editor';
import { DirectorySearch } from './directory-search';

const stages = [{value:'basic',label:'基本信息'},{value:'inputs',label:'输入要求'},{value:'nodes',label:'计算规则'},{value:'clock',label:'时间与质量'},{value:'outputs',label:'输出与发布'}] as const;
type Stage = typeof stages[number]['value'];
export function DerivedDefinitionsPage({id,view,version}:DerivedSearch) {
 const m=useDerived(),navigate=useNavigate(),[q,setQ]=useState('');
 const list=usePagedQuery({queryKey:[...m.prefix, 'derived-definitions', q],queryFn:({ cursor, signal }) => m.client.derivedMetrics(m.scope,{q,cursor},signal),enabled:m.allows('metrics.read')});
 const detail=useQuery({queryKey:[...m.prefix,'derived-definition',id],queryFn:({signal})=>m.client.derivedMetric(m.scope,id!,signal),enabled:!!id&&id!=='new'&&m.allows('metrics.read')});
 const base=useQuery({queryKey:[...m.prefix,'derived-version',version],queryFn:({signal})=>m.client.derivedMetricVersion(m.scope,version!,signal),enabled:!!version&&view==='edit'&&m.allows('metrics.read')});
 const choose=(value:string,edit=false,baseVersion?:string)=>void navigate({to:m.derivedPath,search:{tab:'definitions',id:value||undefined,view:value&&edit?'edit':undefined,version:baseVersion}});
 const useVersion=(v:DerivedMetricVersion)=>void navigate({to:m.derivedPath,search:{tab:'channels',id:'new',definition:v.definition_id,version:v.id}});
 const writable=m.scope.kind==='tenant'&&m.allows('metrics.manage')&&m.editable;
 const full=id==='new'||view==='edit';
 if(!m.allows('metrics.read'))return <p role="status">当前身份没有指标定义读取权限。</p>;
 const editor=(editing:boolean)=>id==='new'?writable?<MetricEditor key="new" editingMode onSelect={value=>choose(value,true)} onUseVersion={useVersion}/>:<p role="status">当前空间仅可查看指标定义。</p>:detail.data?(version&&view==='edit'&&base.isPending?<LoadingState/>:version&&base.data?.definition_id!==id?<CommandError error={base.error??new Error('所选版本不属于此指标，或当前不可读取。')}/>:<MetricEditor key={`${detail.data.id}:${editing}:${version??''}`} definition={detail.data} initialVersion={editing?base.data:undefined} editingMode={editing&&writable} onSelect={value=>choose(value,true)} onEditVersion={v=>choose(id!,true,v?.id)} onUseVersion={useVersion}/>):detail.isFetching?<LoadingState/>:<CommandError error={detail.error}/>;
 return <div className="min-w-0 space-y-4">{full?<><div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="text-lg font-semibold">{id==='new'?'新建指标定义':'编辑指标定义'}</h2><p className="text-sm text-muted-foreground">按步骤声明规则，保存并校验后发布固定版本。</p></div><Button variant="outline" onClick={()=>choose(id==='new'?'':id??'')}>返回{ id==='new'?'指标目录':'定义详情'}</Button></div>{editor(true)}</>:<>
 <div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="text-lg font-semibold">指标定义</h2><p className="text-sm text-muted-foreground">维护一次规则，在多个工程对象上复用。{m.scope.kind==='project'?'指标规则由租户维护。':''}</p></div>{writable&&<Button onClick={()=>choose('new',true)}>新建指标定义</Button>}</div>
 <DirectoryTable label="指标定义目录" toolbar={<DirectorySearch label="搜索指标名称或编码" value={q} onChange={v=>{setQ(v);}}/>} items={list.data?.items??[]} rowId={v=>v.id} loading={list.isPending} error={list.error?.message} retry={()=>void list.refetch()} empty="暂无匹配的指标定义，可新建规则后发布使用。" columns={[
 {id:'name',className:'w-[60%] md:w-[30%]',label:'指标名称',cell:v=><div><Button id={`definition-${v.id}`} variant="link" className="h-auto min-h-11 max-w-full justify-start whitespace-normal break-all px-0 text-left" onClick={()=>choose(v.id)}>{v.name}</Button><p className="text-xs text-muted-foreground">{v.code}</p></div>},
 {id:'rule',className:'hidden md:table-cell md:w-[30%]',label:'计算规则与输出',cell:v=><DefinitionSummary definition={v}/>},
 {id:'state',className:'w-[40%] md:w-[15%]',label:'版本状态',cell:v=><div className="space-y-1 text-sm"><p>{v.latest_version==='0'?'尚未发布':`已发布 V${v.latest_version}`}</p>{v.draft&&<span className="inline-flex rounded-md bg-muted px-2 py-1 text-xs">有草稿</span>}</div>},
 {id:'updated',label:'最近更新',secondary:true,cell:v=><span className="text-xs text-muted-foreground">{new Date(v.updated_at).toLocaleString('zh-CN',{hour12:false})}</span>},
 {id:'actions',className:'hidden md:table-cell md:w-[12%]',label:'操作',cell:v=><Button variant="outline" size="sm" onClick={()=>choose(v.id)}>查看定义</Button>}
 ]} pagination={<ListPagination label="列表" {...list.pagination} />}/>
 <ResponsivePanel open={!!id} onOpenChange={open=>{if(!open)choose('');}} title={detail.data?.name??'指标定义'} description={detail.data?`${detail.data.code} · 固定版本供通道复用`:'正在读取指标定义'} size="wide" fallbackFocusSelector={`#definition-${id}`}>{editor(false)}</ResponsivePanel>
 </>}</div>;
}
function DefinitionSummary({definition}:{definition:DerivedMetricDefinition}) {
 const m=useDerived();
 const versions=useQuery({queryKey:[...m.prefix,'definition-summary',definition.id,definition.latest_version],queryFn:({signal})=>m.client.derivedMetricVersions(m.scope,definition.id,{limit:1},signal),enabled:definition.latest_version!=='0'});
 const rule=versions.data?.items[0]?.rule??definition.draft?.rule;
 return rule?<RuleSummary rule={rule} compact/>:<p className="text-xs text-muted-foreground">{versions.isFetching?'读取固定规则…':versions.error?'规则暂不可读，请打开详情重试':'尚未配置规则'}</p>;
}

export function MetricEditor({definition,initialVersion,editingMode=false,onSelect,onEditVersion,onUseVersion,onDirtyChange,navigationGuard=true}:{definition?:DerivedMetricDefinition;initialVersion?:DerivedMetricVersion;editingMode?:boolean;onSelect?:(id:string)=>void;onEditVersion?:(version?:DerivedMetricVersion)=>void;onUseVersion?:(version:DerivedMetricVersion)=>void;onDirtyChange?:(dirty:boolean)=>void;navigationGuard?:boolean}) {
 const m=useDerived(),cache=useQueryClient(),meta=useRequestMeta();
 const initial={code:definition?.code??'',name:definition?.name??'',description:definition?.description??'',rule:initialVersion?.rule??definition?.draft?.rule??emptyRule(),base_version_id:initialVersion?.id??definition?.draft?.base_version_id};
 const [editing,setEditing]=useState(editingMode),[stage,setStage]=useState<Stage>('basic'),[published,setPublished]=useState<DerivedMetricVersion>();
 useIssueReveal(path=>setStage(path.startsWith('/rule/inputs')?'inputs':path.startsWith('/rule/nodes')||path.startsWith('/rule/parameters')?'nodes':path.startsWith('/rule/clock')||path.startsWith('/rule/policies')?'clock':path.startsWith('/rule/outputs')||path==='/reason'?'outputs':'basic'));
 const [fields,setFields]=useState(initial),[saved,setSaved]=useState(JSON.stringify(initial)),[current,setCurrent]=useState(definition),[reason,setReason]=useState(''),[validation,setValidation]=useState<DerivedRuleValidation>(),[notice,setNotice]=useState('');
 const [copyCode,setCopyCode]=useState(''),[copyName,setCopyName]=useState(''),[copyVersion,setCopyVersion]=useState('');
 const [view,setView]=useState<DerivedMetricVersion>();
 const versions=usePagedQuery({queryKey:[...m.prefix, 'derived-versions', current?.id],queryFn:({ cursor, signal }) => m.client.derivedMetricVersions(m.scope,current!.id,{cursor},signal),enabled:!!current});
 const writable=m.scope.kind==='tenant'&&m.allows('metrics.manage')&&m.editable;
 const dirty=editing&&JSON.stringify(fields)!==saved;
 useEffect(()=>{onDirtyChange?.(dirty);return ()=>onDirtyChange?.(false);},[dirty,onDirtyChange]);
 const update=(patch:Partial<typeof fields>)=>{setFields(v=>({...v,...patch}));setValidation(undefined);setNotice('');};
 const operation=useOperation({mutationFn:async(action:'save'|'validate'|'publish'|'delete'|'copy')=>{
  if(m.scope.kind!=='tenant')throw new Error('指标定义由租户工作空间维护。');
  if(action==='validate'){if(!current?.draft)throw new Error('请先保存草稿。');const v=await m.client.derivedValidateMetric(m.scope,current.id,{draft_revision:current.draft.revision});setValidation(v);return;}
  if(action==='save'){
   const input={name:fields.name,description:fields.description,rule:fields.rule,base_version_id:fields.base_version_id};
   const body=current?{...input,expected_revision:current.revision}:{...input,code:fields.code};
   const result=current?await m.client.derivedSaveMetric(m.scope,current.id,{...input,expected_revision:current.revision,...meta('save',body,reason)}):await m.client.derivedCreateMetric(m.scope,{code:fields.code,...input,...meta('create',body,reason)});
   setCurrent(result.definition);flushSync(()=>setSaved(JSON.stringify(fields)));setValidation(undefined);setNotice('草稿已保存，尚未发布。');if(!current)onSelect?.(result.definition.id);
  } else if(action==='publish'){
   if(!current?.draft||!validation?.valid||dirty)throw new Error('请保存并校验当前草稿。');
   const body={expected_revision:current.revision,draft_revision:current.draft.revision,validation_hash:validation.validation_hash};
   const result=await m.client.derivedPublishMetric(m.scope,current.id,{...body,...meta('publish',body,reason)});setCurrent(result.definition);setEditing(false);setPublished(result.version);setValidation(undefined);setNotice(`已发布固定版本 V${result.version?.version_no}。`);
  } else if(action==='delete'){
   if(!current?.draft)throw new Error('没有可删除草稿。');const body={expected_revision:current.revision};await m.client.derivedDeleteMetricDraft(m.scope,current.id,{...body,...meta('delete',body,reason)});flushSync(()=>{setSaved(JSON.stringify(fields));if(!onSelect){setCurrent(undefined);setEditing(true);setStage('basic');}});onSelect?.('');
  } else {
   if(!current||!copyVersion)throw new Error('请选择复制来源版本。');const body={code:copyCode,name:copyName,source_version_id:copyVersion};const result=await m.client.derivedCopyMetric(m.scope,current.id,{...body,...meta('copy',body,reason)});
   const next={code:result.definition.code,name:result.definition.name,description:result.definition.description,rule:result.definition.draft?.rule??emptyRule(),base_version_id:result.definition.draft?.base_version_id};
   flushSync(()=>{setCurrent(result.definition);setFields(next);setSaved(JSON.stringify(next));setEditing(true);setStage('basic');});onSelect?.(result.definition.id);
  }
  await cache.invalidateQueries({queryKey:m.prefix});
 }});
 const state=useOperation({mutationFn:async(version:DerivedMetricVersion)=>{
  if(m.scope.kind!=='tenant')throw new Error('需要租户入口');const body={enabled:!version.enabled,expected_revision:version.state_revision};await m.client.derivedSetVersionState(m.scope,version.id,{...body,...meta(`state:${version.id}`,body,reason)});await versions.refetch();setNotice(version.enabled?'版本已停用，既有配置和历史证据仍保留。':'版本已启用。');
 }});
 const [templatePending,setTemplatePending]=useState(false);
 const busy=operation.isPending||state.isPending||templatePending;
 const editVersion=(version?:DerivedMetricVersion)=>{if(onEditVersion){onEditVersion(version);return;}if(version)update({rule:structuredClone(version.rule),base_version_id:version.id});setEditing(true);setStage('basic');};
 return <div className="min-w-0 space-y-5"><UnsavedChanges dirty={navigationGuard&&dirty}/>
 <CommandError error={operation.error??state.error} issues={validation?.issues}/>
 {notice&&<div role="status" className="space-y-3 rounded-lg border bg-muted/30 p-4"><p>{notice}</p>{published&&onUseVersion&&m.editable&&m.allows('channels.manage')&&<Button onClick={()=>onUseVersion(published)}>使用 V{published.version_no} 创建通道</Button>}</div>}
 {editing?<><EditorSteps label="指标定义编辑步骤" value={stage} onChange={setStage} items={stages}/>
 <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_17rem]"><div className="min-w-0 space-y-5">
 <div hidden={stage!=='basic'}><Section title="基本信息"><div className="grid gap-4 sm:grid-cols-2"><TextField label="指标编码" value={fields.code} onChange={code=>update({code})} disabled={!!current||!writable} path="/code"/><TextField label="指标名称" value={fields.name} onChange={name=>update({name})} disabled={!writable} path="/name"/></div><TextField label="业务说明" value={fields.description} onChange={description=>update({description})} multiline disabled={!writable} path="/description"/><p className="text-sm text-muted-foreground">使用可复用的业务名称。实际设施、测点和数据来源在创建监测通道时指定。</p>{fields.base_version_id&&<p className="text-sm">正在基于固定版本创建新草稿，原发布规则与已有通道继续保留。</p>}</Section></div>
 <RuleEditor onPendingChange={setTemplatePending} rule={fields.rule} onChange={rule=>update({rule})} readOnly={!writable} section={stage==='basic'?'none':stage}/>
 <div hidden={stage!=='outputs'}><Section title="发布核对"><p className="text-sm text-muted-foreground">发布后生成不可修改的固定版本。已有通道继续使用原版本，更新需单独保存、试算并应用配置。</p><TextField path="/reason" label="操作原因" value={reason} onChange={setReason} hint="写入审计记录，请说明发布或删除草稿的业务原因。"/>{validation&&<div role="status" className="space-y-2 rounded-md border p-3"><p className="font-medium">{validation.valid?'校验通过，可发布当前已保存草稿':'校验未通过，请修正规则'}</p><p className="text-sm">{validation.input_count} 个输入 · {validation.node_count} 个步骤</p>{validation.normalized_rule.outputs.map(v=><p key={v.key} className="text-sm">{v.name||v.key}：<QuantityLabel quantity={v.quantity_spec}/> · 小数 {v.numeric_contract.scale} 位</p>)}</div>}{writable&&current?.draft&&<Button variant="ghost" disabled={busy} onClick={()=>operation.mutate('delete')}>删除此草稿</Button>}</Section></div>
 </div><RuleSummary rule={fields.rule}/></div>
 <div className="sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-background/95 p-4 shadow-sm backdrop-blur"><p className="text-xs text-muted-foreground" role="status">{busy?'正在处理…':dirty?'有未保存的修改':validation?.valid?'已保存并通过校验':current?'草稿已保存，发布前请校验':'新草稿尚未保存'}</p><div className="flex flex-wrap gap-2">{stage!=='basic'&&<Button variant="ghost" onClick={()=>setStage(stages[stages.findIndex(s=>s.value===stage)-1]!.value)}>上一步</Button>}{stage!=='outputs'&&<Button variant="outline" onClick={()=>setStage(stages[stages.findIndex(s=>s.value===stage)+1]!.value)}>下一步</Button>}{writable&&<><Button variant="outline" disabled={busy} onClick={()=>operation.mutate('save')}>{current?'保存草稿':'创建草稿'}</Button><Button variant="outline" disabled={busy||dirty||!current?.draft} onClick={()=>{setStage('outputs');operation.mutate('validate');}}>校验已保存草稿</Button></>}{m.allows('metrics.publish')&&m.editable&&<Button disabled={busy||dirty||!validation?.valid} onClick={()=>operation.mutate('publish')}>发布固定版本</Button>}</div></div>
 </>:<>{current&&<div className="space-y-3"><p className="text-sm text-muted-foreground">{current.description||'尚未填写业务说明。'}</p>{(view?.rule??published?.rule??versions.data?.items[0]?.rule??current.draft?.rule)&&<RuleSummary rule={(view?.rule??published?.rule??versions.data?.items[0]?.rule??current.draft?.rule)!} compact/>}{writable&&current.draft&&<Button onClick={()=>editVersion()}>继续编辑草稿</Button>}<TextField path="/reason" label="操作原因" value={reason} onChange={setReason} hint="版本启停与复制操作将记录审计原因。"/></div>}</>}
 {current&&!editing&&<Section title="发布历史"><CommandError error={versions.error}/><DirectoryTable tableClassName="max-sm:[&_thead]:hidden max-sm:[&_tr]:block max-sm:[&_td]:block max-sm:[&_td]:w-full" label="指标发布版本" items={published&&versions.pagination.page===0&&!versions.data?.items.some(v=>v.id===published.id)?[published,...versions.data?.items??[]]:versions.data?.items??[]} rowId={v=>v.id} loading={versions.isPending} error={versions.error?.message} retry={()=>void versions.refetch()} columns={[
 {id:'version',label:'版本',cell:v=><Button variant="link" className="h-auto min-h-11 max-w-full whitespace-normal break-all px-0 text-left" onClick={()=>setView(v)}>V{v.version_no} · {v.name}</Button>},
 {id:'state',label:'可用情况',cell:v=>v.usable?'可用于新配置':v.enabled?'引用的公共版本不可用':'已停用'},
 {id:'time',label:'发布时间',secondary:true,cell:v=>new Date(v.published_at).toLocaleString('zh-CN',{hour12:false})},
 {id:'actions',label:'操作',cell:v=><div className="flex flex-wrap gap-2">{onUseVersion&&m.allows('channels.manage')&&m.editable&&<Button size="sm" disabled={!v.usable} onClick={()=>onUseVersion(v)}>创建通道</Button>}{writable&&<Button size="sm" variant="outline" disabled={dirty||busy} onClick={()=>editVersion(v)}>基于此版本编辑</Button>}{m.scope.kind==='tenant'&&m.allows('metrics.publish')&&m.editable&&<Button size="sm" variant="ghost" disabled={busy} onClick={()=>state.mutate(v)}>{v.enabled?'停用':'启用'}</Button>}</div>}
 ]} pagination={<ListPagination label="版本" {...versions.pagination} />}/>
 {view&&<details open><summary className="cursor-pointer py-3 font-medium">固定版本 V{view.version_no} 的规则</summary><RuleSummary rule={view.rule} compact/><RuleEditor rule={view.rule} onChange={()=>{}} readOnly/></details>}
 {writable&&<details><summary className="cursor-pointer py-3 font-medium">复制为新指标定义</summary><div className="space-y-3"><SelectField label="来源版本" value={copyVersion} onChange={setCopyVersion} options={[{value:'',label:'请选择来源版本'},...(versions.data?.items??[]).map(v=>({value:v.id,label:`V${v.version_no} · ${v.name}`}))]}/><TextField label="新指标编码" value={copyCode} onChange={setCopyCode}/><TextField label="新指标名称" value={copyName} onChange={setCopyName}/><Button variant="outline" disabled={busy||!copyVersion} onClick={()=>operation.mutate('copy')}>复制为独立草稿</Button></div></details>}
 </Section>}
 </div>;
}
