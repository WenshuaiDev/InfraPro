import { useState } from 'react';
import { flushSync } from 'react-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@infrapro/ui/components/button';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import type { AnalysisBody, AnalysisConfig, AnalysisValidation, AnalysisVersion } from '@infrapro/api-client';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { requestUUID } from '../tenancy/membership-utils';
import { BindingEditor } from './bindings-editor';
import { CommandError, Section, SelectField, TextField, UnsavedChanges, emptyRule, useDerived } from './common';
import { RuleEditor } from './rule-editor';
import { useIssueReveal } from './editor-navigation';
import { AnalysisExecution } from './analysis-execution';
import type { DerivedSearch } from './search';

type Area = 'inputs' | 'bindings' | 'nodes' | 'clock' | 'outputs';
const areas: {value:Area;label:string}[] = [{value:'inputs',label:'输入要求'},{value:'bindings',label:'测点与实际参数'},
  {value:'nodes',label:'计算方法'},{value:'clock',label:'时间与质量'},{value:'outputs',label:'输出与精度'}];
type Props = {config?:AnalysisConfig;initialBody?:AnalysisBody;initialName?:string;onSelect:(id:string)=>void|Promise<void>;prefill?:DerivedSearch};

export function AnalysisEditor({config,initialBody,initialName,onSelect,prefill}: Props) {
  const m=useDerived(),cache=useQueryClient(),meta=useRequestMeta();
  const [current,setCurrent]=useState(config),[code,setCode]=useState(()=>config?.code??(initialBody?'AN-'+requestUUID().slice(0,8).toUpperCase():''));
  const [name,setName]=useState(config?.name??initialName??''),[description,setDescription]=useState(config?.description??'');
  const [body,setBody]=useState<AnalysisBody>(()=>config?.draft?.body??initialBody??{rule:emptyRule(),input_bindings:prefill?.channel?{x:{channel_id:prefill.channel}}:{},parameters:{}});
  const [baseVersion,setBaseVersion]=useState(''),[reset,setReset]=useState(false),[reason,setReason]=useState('');
  const fields={code,name,description,body},[saved,setSaved]=useState(JSON.stringify(fields)),dirty=JSON.stringify(fields)!==saved;
  const [validation,setValidation]=useState<AnalysisValidation>(),[notice,setNotice]=useState(''),[templatePending,setTemplatePending]=useState(false);
  const [selected,setSelected]=useState<AnalysisVersion>(),[copyCode,setCopyCode]=useState(''),[copyName,setCopyName]=useState('');
  const [editing,setEditing]=useState(!config||config.latest_version==='0');
  const [advanced,setAdvanced]=useState(!config&&!initialBody),[area,setArea]=useState<Area>('inputs');
  const writable=m.editable&&m.allows('analysis.manage')&&current?.status!=='archived'&&!current?.inputs_restricted;
  const versions=usePagedQuery({queryKey:[...m.prefix,'analysis-versions',current?.id],queryFn:({cursor,signal})=>m.client.analysisConfigVersions(m.scope,current!.id,{cursor},signal),enabled:!!current&&!current.inputs_restricted});
  const selectedVersion=selected??versions.data?.items[0];
  const update=(v:AnalysisBody)=>{setBody(v);setValidation(undefined);};
  function editVersion(version:AnalysisVersion) {
    update(structuredClone(version.body));setBaseVersion(version.id);setReset(false);setEditing(true);setAdvanced(true);
  }
  useIssueReveal(path=>{
    setEditing(true);
    if (!path.startsWith('/rule') && !path.includes('bindings') && !path.startsWith('/parameters')) return;
    setAdvanced(true);
    setArea(path.includes('bindings')||path.startsWith('/parameters')?'bindings':path.startsWith('/rule/inputs')?'inputs':path.startsWith('/rule/outputs')?'outputs':/^\/rule\/(clock|policies)/.test(path)?'clock':'nodes');
  });
  async function save() {
    if(current?.draft&&!dirty) return current;
    const input={name,description,body};
    const payload=current?{...input,expected_revision:current.revision}:{...input,code};
    const result=current?await m.client.analysisSaveConfig(m.scope,current.id,{...input,expected_revision:current.revision,...meta('save-analysis',payload,reason)}):
      await m.client.analysisCreateConfig(m.scope,{...input,code,...meta('create-analysis',payload,reason)});
    flushSync(()=>{setCurrent(result.config);setSaved(JSON.stringify(fields));setValidation(undefined);setNotice('分析草稿已保存。');});
    cache.setQueryData([...m.prefix,'analysis-config',result.config.id],result.config);
    await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-configs']});
    return result.config;
  }
  const operation=useOperation({mutationFn:async(action:'save'|'validate'|'publish'|'prepare'|'copy'|'archive')=>{
    if(action==='copy'&&current&&selectedVersion) {
      const payload={code:copyCode,name:copyName,source_version_id:selectedVersion.id};
      const result=await m.client.analysisCopyConfig(m.scope,current.id,{...payload,...meta('copy-analysis',payload,reason)});
      await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-configs']}); await onSelect(result.config.id);return;
    }
    if(action==='archive'&&current) {
      const payload={expected_revision:current.revision};
      const result=await m.client.analysisArchiveConfig(m.scope,current.id,{...payload,...meta('archive-analysis',payload,reason)});
      setCurrent(result.config);setNotice('分析方案已归档。');await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-configs']});return;
    }
    const draft=action==='save'||action==='prepare'?await save():current;
    if(!draft?.draft) throw new Error('请先保存草稿。');
    if(action==='save'){if(!config)await onSelect(draft.id);return;}
    const checked=action==='publish'?validation:await m.client.analysisValidateConfig(m.scope,draft.id,{draft_revision:draft.draft.revision});
    setValidation(checked);
    if(!checked||checked.issues.length||action==='validate')return;
    const payload={expected_revision:draft.revision,draft_revision:draft.draft.revision,validation_hash:checked.validation_hash,base_version_id:baseVersion||undefined,reset_baselines:reset};
    const result=await m.client.analysisPublishConfig(m.scope,draft.id,{...payload,...meta('publish-analysis',payload,reason)});
    cache.setQueryData([...m.prefix,'analysis-config',draft.id],result.config);
    setCurrent(result.config);setSelected(result.version);setValidation(undefined);if(config)setEditing(false);setNotice('固定执行版本 V'+result.version?.version_no+' 已保存。');
    await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-configs']});
    await cache.invalidateQueries({queryKey:[...m.prefix,'analysis-versions',draft.id]});
    if(!config)await onSelect(draft.id);
  }});
  const busy=operation.isPending||templatePending;
  if(current?.inputs_restricted) return <Section title={current.name}><p role="status">当前身份不能读取此方案使用的全部输入。</p></Section>;
  return <div className="space-y-4"><UnsavedChanges dirty={dirty}/>
    {notice&&<p role="status" className="text-sm">{notice}</p>}
    {editing?<><Section title={current?current.name:'准备分析方案'}>
      <TextField label="分析名称" value={name} onChange={v=>{setName(v);setValidation(undefined);}} disabled={!writable||busy} path="/name"/>
      <div className="flex flex-wrap gap-2 text-sm">{body.rule.inputs.map(v=><span key={v.key} className="rounded border bg-muted/30 px-2 py-1">{v.name||v.key}</span>)}</div>
      <details open={!config&&!initialBody||undefined}><summary className="min-h-10 cursor-pointer py-2 text-sm">方案编码与说明</summary><div className="space-y-3">
        <TextField label="分析编码" value={code} onChange={setCode} disabled={!!current||!writable||busy} path="/code"/>
        <TextField label="分析目标与说明" value={description} onChange={v=>{setDescription(v);setValidation(undefined);}} disabled={!writable||busy} multiline path="/description"/></div></details>
      <Button type="button" variant="outline" aria-expanded={advanced} disabled={busy} onClick={()=>setAdvanced(!advanced)}>{advanced?'收起规则设置':'调整测点与规则'}</Button>
      {advanced&&<div className="space-y-3"><SelectField label="编辑内容" value={area} onChange={setArea} options={areas}/>
        {area==='bindings'?<fieldset disabled={!writable||busy}><BindingEditor rule={body.rule} value={body.input_bindings} parameters={body.parameters} baselines={body.baseline_bindings}
          onChange={input_bindings=>update({...body,input_bindings})} onParameters={parameters=>update({...body,parameters})} onBaselines={baseline_bindings=>update({...body,baseline_bindings})}/></fieldset>:
          <RuleEditor section={area} onPendingChange={setTemplatePending} analysis rule={body.rule} onChange={rule=>update({...body,rule,input_bindings:Object.fromEntries(rule.inputs.filter(v=>body.input_bindings[v.key]).map(v=>[v.key,body.input_bindings[v.key]!]))})} readOnly={!writable||operation.isPending}/>}
      </div>}
      {baseVersion&&<label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" checked={reset} onChange={e=>setReset(e.target.checked)}/>重新冻结基准</label>}
      <TextField label="操作原因" value={reason} onChange={setReason} disabled={busy}/>
      {validation&&!validation.issues.length&&<p role="status">校验通过，可生成固定执行版本</p>}
      <CommandError error={operation.error} issues={validation?.issues}/>
      {writable&&<div className="flex flex-wrap gap-2">
        <Button disabled={busy||!reason.trim()||!name.trim()||!code.trim()} onClick={()=>operation.mutate('prepare')}>保存方案，选择时段</Button>
        <Button variant="outline" disabled={busy} onClick={()=>operation.mutate('save')}>保存分析草稿</Button>
        {current?.draft&&<Button variant="ghost" disabled={busy||dirty} onClick={()=>operation.mutate('validate')}>校验已保存草稿</Button>}
        {validation&&!validation.issues.length&&<Button variant="outline" disabled={busy||dirty} onClick={()=>operation.mutate('publish')}>保存固定执行版本</Button>}
      </div>}
    </Section></>:null}
    {current&&<Section title="固定执行版本">
      <CommandError error={versions.error??(!editing?operation.error:null)}/>
      {selectedVersion&&<><SelectField label="选择执行方案版本" value={selectedVersion.id} onChange={id=>setSelected(versions.data?.items.find(v=>v.id===id))}
        options={(versions.data?.items??[]).map(v=>({value:v.id,label:'V'+v.version_no+' · '+v.name}))}/>
        {!editing&&current.status==='active'&&<AnalysisExecution key={selectedVersion.id} version={selectedVersion} prefill={prefill}/>}
        <details><summary className="min-h-11 cursor-pointer py-3 text-sm">查看固定规则与方案操作</summary><div className="space-y-3">
          {writable&&<Button variant="outline" disabled={dirty||busy} onClick={()=>editVersion(selectedVersion)}>基于此版本编辑</Button>}
          <RuleEditor analysis rule={selectedVersion.body.rule} onChange={()=>{}} readOnly/>
          {writable&&<><TextField label="新分析编码" value={copyCode} onChange={setCopyCode}/><TextField label="新分析名称" value={copyName} onChange={setCopyName}/>
            <Button variant="outline" disabled={busy||dirty||!copyCode.trim()||!copyName.trim()} onClick={()=>operation.mutate('copy')}>复制为独立草稿</Button></>}
        </div></details></>}
      {!!versions.data?.items.length&&<ListPagination label="版本" {...versions.pagination}/>}
      {!editing&&writable&&(current.draft||selectedVersion)&&<Button variant="outline" onClick={()=>current.draft?setEditing(true):selectedVersion&&editVersion(selectedVersion)}>{current.draft?'继续编辑草稿':'编辑方案'}</Button>}
      {writable&&<details><summary className="min-h-10 cursor-pointer py-2 text-sm">归档方案</summary><TextField label="归档方案原因" value={reason} onChange={setReason}/>
        <Button variant="outline" disabled={!reason.trim()||dirty||busy} onClick={()=>operation.mutate('archive')}>归档配置</Button></details>}
    </Section>}
  </div>;
}
