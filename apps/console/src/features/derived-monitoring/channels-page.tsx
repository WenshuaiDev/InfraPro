import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ObjectHistory } from '../audit/object-history';
import { useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { DerivedChannel, DerivedChannelConfig, DerivedCreateChannelInput } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/select';
import { X } from 'lucide-react';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { CommandError, Section, SelectField, nsISO, useDerived } from './common';
import { DirectorySearch } from './directory-search';
import { ChannelConfigForm } from './channel-config-form';
import { ChannelTrialControl } from './channel-controls';
import { ChannelResource } from './channel-resource';
import { ChannelStatus } from './channel-status';
import { ChannelHistory } from './channel-history';
import { RuleSummary } from './rule-summary';
import { RuleEditor } from './rule-editor';
import type { DerivedSearch } from './search';
const controls={draft:'待启用',enabled:'自动计算中',paused:'已暂停',retired:'已退役'};
export function DerivedChannelsPage({id,owner,owner_kind,facility,definition,version,view}:DerivedSearch) {
 const m=useDerived(),navigate=useNavigate(),[q,setQ]=useState(''),[state,setState]=useState<''|'draft'|'enabled'|'paused'|'retired'>('');
 const list=usePagedQuery({queryKey:[...m.prefix, 'derived-channels', q, state, facility],queryFn:({ cursor, signal }) => m.client.derivedChannels(m.scope,{q,control_state:state||undefined,cursor,facility_id:facility},signal),enabled:m.allows('channels.read')});
 const detail=useQuery({queryKey:[...m.prefix,'derived-channel',id],queryFn:({signal})=>m.client.derivedChannel(m.scope,id!,signal),enabled:!!id&&id!=='new'&&m.allows('channels.read')});
 const choose=(value:string,run=false)=>void navigate({to:m.derivedPath,search:{tab:'channels',id:value||undefined,view:run?'run':undefined,facility}});
 const initialOwner:DerivedCreateChannelInput['owner']|undefined=owner&&facility&&['facility','component','point'].includes(owner_kind??'')?{kind:owner_kind as 'facility'|'component'|'point',id:owner,facility_id:facility}:undefined;
 if(!m.allows('channels.read'))return <p role="status">当前身份没有派生通道读取权限。</p>;
 return <div className="@container/monitoring min-w-0 space-y-4">{id==='new'?<><div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="text-lg font-semibold">新建监测通道</h2><p className="text-sm text-muted-foreground">选择规则、绑定输入，试算核对后启用持续计算。</p></div><Button variant="outline" onClick={()=>choose('')}>返回通道目录</Button></div>{m.allows('channels.manage')&&m.editable?<ChannelConfigForm initialOwner={initialOwner} initialDefinition={definition} initialVersion={version} onCreated={v=>choose(v.resource.id,true)}/>:<p role="status">当前空间仅可查看监测通道。</p>}</>:<>
 <div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="text-lg font-semibold">监测通道</h2><p className="text-sm text-muted-foreground">查看实际工程对象的派生结果和计算状态。{facility?'当前已限定所属设施。':''}</p></div>{m.allows('channels.manage')&&m.editable&&<Button onClick={()=>choose('new')}>新建监测通道</Button>}</div>
 <div className="flex min-w-0 flex-wrap items-center gap-2"><DirectorySearch label="搜索通道名称或编码" value={q} onChange={v=>{setQ(v);}}/><Select value={state||'all'} onValueChange={v=>{setState(v==='all'?'':v as Exclude<typeof state,''>);}}><SelectTrigger aria-label="控制状态" className="h-9 w-auto min-w-28 max-sm:data-[size=default]:h-11"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem>{Object.entries(controls).map(([value,label])=><SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select>{(q||state)&&<Button variant="ghost" className="h-9 px-2 text-muted-foreground max-sm:h-11" onClick={()=>{setQ('');setState('');}}>重置<X aria-hidden="true"/></Button>}</div>
 <DirectoryTable label="派生监测通道目录" items={list.data?.items??[]} rowId={v=>v.resource.id} loading={list.isPending} error={list.error?.message} retry={()=>void list.refetch()} empty="暂无匹配的监测通道，可选择已发布规则创建。" columns={[
 {id:'name',className:'w-[47%] md:w-[27%]',label:'通道 / 所属对象',cell:v=><div className="max-w-sm"><Button id={`channel-${v.resource.id}`} className="h-auto min-h-11 max-w-full justify-start whitespace-normal break-words px-0 text-left" variant="link" onClick={()=>choose(v.resource.id)}>{v.resource.name}</Button><p className="break-words text-xs text-muted-foreground">{v.resource.code} · {v.facility_name} / {v.owner_name}</p><p className="text-xs text-muted-foreground">{({facility:'设施',component:'构件',monitoring_section:'监测断面',borehole:'孔位',point:'测点'})[v.resource.owner_kind??'point']} · {v.owner_code}</p><p className="mt-1 text-xs text-muted-foreground lg:hidden">{controls[v.control_state]}</p><p className="text-xs text-muted-foreground xl:hidden">{v.health==='healthy'?'计算正常':v.health==='waiting'?'等待输入 / 输出时刻':`受阻：${v.blocked_reason}`}</p></div>},
 {id:'metric',className:'hidden md:table-cell md:w-[21%]',label:'指标规则',cell:v=><div className="max-w-56 break-words text-sm"><p>{v.metric_name}</p><p className="mt-1 text-xs text-muted-foreground">{v.current_config_id?'固定配置已生效':'当前时刻无生效配置'}</p></div>},
 {id:'latest',className:'w-[33%] md:w-[20%]',label:'最新输出',cell:v=><ChannelStatus channel={v} compact/>},
 {id:'state',className:'hidden lg:table-cell lg:w-[11%]',label:'持续计算',cell:v=><span className={`inline-flex rounded-md px-2 py-1 text-xs ${v.control_state==='enabled'?'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300':'bg-muted text-muted-foreground'}`}>{controls[v.control_state]}</span>},
 {id:'health',className:'hidden xl:table-cell xl:w-[12%]',label:'更新状态',cell:v=><span className={`text-xs ${v.health==='blocked'?'text-destructive':'text-muted-foreground'}`}>{v.health==='healthy'?'正常':v.health==='waiting'?'等待输入 / 输出时刻':`受阻：${v.blocked_reason}`}</span>},
 {id:'action',className:'w-[20%] md:w-[9%]',label:'操作',cell:v=><Button variant="outline" size="sm" className="px-2" onClick={()=>choose(v.resource.id)}><span className="md:hidden">详情</span><span className="hidden md:inline">查看详情</span></Button>}
 ]} pagination={<ListPagination label="列表" {...list.pagination} />}/>
 <p className="text-xs text-muted-foreground">时间按本地时区显示。完整覆盖区间、上一有效值与历史依据可在详情中查看。</p>
 <ResponsivePanel open={!!id} onOpenChange={open=>{if(!open)choose('');}} title={detail.data?.resource.name??'监测通道详情'} description={detail.data?`${detail.data.resource.code} · ${detail.data.facility_name} / ${detail.data.owner_name}`:'正在读取通道'} size="workspace" fallbackFocusSelector={`#channel-${id}`}><CommandError error={detail.error}/>{detail.isPending?<LoadingState/>:detail.data&&<ChannelDetail key={detail.data.resource.id} channel={detail.data} initialView={view}/>}</ResponsivePanel>
 </>}</div>;
}
function ChannelDetail({channel,initialView}:{channel:DerivedChannel;initialView?:string}) {
 const m=useDerived(),navigate=useNavigate(),cache=useQueryClient(),[selected,setSelected]=useState<DerivedChannelConfig>(),[editing,setEditing]=useState(false),[copy,setCopy]=useState(false),[view,setView]=useState(initialView==='run'?'run':'overview');
 const configs=usePagedQuery({queryKey:[...m.prefix, 'channel-configs', channel.resource.id],queryFn:({ cursor, signal }) => m.client.derivedChannelConfigs(m.scope,channel.resource.id,{cursor},signal)});
 const cfg=selected??configs.data?.items.find(v=>v.id===channel.draft_config_id)??configs.data?.items[0];
 const assignments=usePagedQuery({queryKey:[...m.prefix, 'channel-assignments', channel.resource.id, channel.config_revision],queryFn:({ cursor, signal }) => m.client.derivedAssignments(m.scope,channel.resource.id,channel.config_revision,cursor,signal)});
 const refresh=(v:DerivedChannel)=>{cache.setQueryData([...m.prefix,'derived-channel',channel.resource.id],v);void cache.invalidateQueries({queryKey:[...m.prefix,'derived-channels']});void cache.invalidateQueries({queryKey:[...m.prefix,'channel-configs',channel.resource.id]});void cache.invalidateQueries({queryKey:[...m.prefix,'channel-assignments',channel.resource.id]});};
 const r=channel.resource,owner:DerivedCreateChannelInput['owner']={kind:r.owner_kind??'point',id:r.owner_kind==='facility'?r.facility_id??'':r.owner_kind==='component'?r.component_id??'':r.point_id??'',facility_id:r.facility_id??''};
 return <div className="min-w-0 space-y-5"><div className="flex flex-wrap items-center justify-between gap-3"><p className="text-sm">{controls[channel.control_state]} · {channel.health==='healthy'?'计算正常':channel.health==='waiting'?'等待数据或下一输出时刻':channel.blocked_reason}</p><ObjectHistory objectType="derived_channel" objectId={r.id}/></div>
 <nav aria-label="监测通道详情视图" className="flex flex-wrap gap-1 border-b">{[{key:'overview',label:'监测概览'},{key:'config',label:'规则与输入'},...(m.allows('results.history')?[{key:'history',label:'历史与依据'}]:[]),{key:'run',label:'运行管理'}].map(item=><Button key={item.key} variant={view===item.key?'secondary':'ghost'} aria-current={view===item.key?'page':undefined} onClick={()=>setView(item.key)}>{item.label}</Button>)}</nav>
 <div hidden={view!=='overview'} className="space-y-5"><ChannelStatus channel={channel}/><Section title="通道信息"><p className="text-sm">{r.description||'尚未填写用途说明。'}</p><dl className="grid gap-4 text-sm sm:grid-cols-2"><div><dt className="text-muted-foreground">指标规则</dt><dd>{channel.metric_name}</dd></div><div><dt className="text-muted-foreground">所属对象</dt><dd>{channel.facility_name} / {channel.owner_name}</dd></div><div><dt className="text-muted-foreground">自动起点</dt><dd className="break-all">{channel.auto_start_ns?nsISO(channel.auto_start_ns):'未确定'}</dd></div><div><dt className="text-muted-foreground">停止边界</dt><dd className="break-all">{channel.stop_ns?nsISO(channel.stop_ns):'持续计算，不设终点'}</dd></div></dl>{m.canRead&&<Button asChild variant="outline"><Link to={m.paths.root} search={{channel:r.id}}>查看当前趋势、明细与统计</Link></Button>}</Section><ChannelResource channel={channel} onChanged={()=>{void cache.invalidateQueries({queryKey:m.prefix});}}/></div>
 <div hidden={view!=='config'&&view!=='run'} className="space-y-5"><CommandError error={configs.error}/>{configs.isPending&&<LoadingState/>}<Section title="固定配置"><SelectField label="选择配置记录" disabled={editing||copy} value={cfg?.id??''} onChange={id=>setSelected(configs.data?.items.find(v=>v.id===id))} options={(configs.data?.items??[]).map(v=>({value:v.id,label:`${new Date(v.created_at).toLocaleString('zh-CN',{hour12:false})} · ${v.change_kind==='initial'?'初始配置':v.change_kind==='correction'?'历史修正':'普通变更'}${v.id===channel.draft_config_id?' · 当前候选':''}${v.id===channel.current_config_id?' · 当前生效':''}`}))}/><ListPagination label="配置" {...configs.pagination} fetching={editing || copy || configs.isFetching} />{cfg&&m.allows('channels.manage')&&m.editable&&!cfg.inputs_restricted&&<div className="flex flex-wrap gap-2"><Button variant="outline" disabled={editing||copy} onClick={()=>{setEditing(true);setView('config');}}>基于此配置编辑</Button><Button variant="outline" disabled={editing||copy} onClick={()=>{setCopy(true);setView('config');}}>复制为新通道</Button></div>}</Section>
 {cfg?.inputs_restricted?<p role="status">当前身份不能读取完整输入配置。输入名称、样本和计算结果已隐藏。</p>:cfg&&<>
 <div hidden={view!=='config'} className="space-y-5">{(editing||copy)?<ChannelConfigForm key={`${cfg.id}:${copy}`} channel={copy?undefined:channel} base={cfg} initialOwner={owner} onCreated={v=>{void navigate({to:m.derivedPath,search:{tab:'channels',id:v.resource.id,view:'run'}});}} onSaved={(v,next)=>{refresh(v);setSelected(next);setEditing(false);setView('run');}}/>:cfg.rule&&<><RuleSummary rule={cfg.rule} compact/><ConfigInputs config={cfg}/><details><summary className="min-h-11 cursor-pointer py-3 font-medium">查看完整固定规则</summary><RuleEditor rule={cfg.rule} onChange={()=>{}} readOnly/></details></>}</div>
 <div hidden={view!=='run'}>{editing||copy?<p className="text-sm">请先完成“规则与输入”中的配置编辑，再对保存后的固定配置试算。</p>:<ChannelTrialControl key={cfg.id} channel={channel} config={cfg} onChanged={refresh}/>}</div>
 </>}</div>
 {view==='history'&&<div className="space-y-5"><details><summary className="min-h-11 cursor-pointer py-3 font-medium">配置生效区间</summary><CommandError error={assignments.error}/><ul className="space-y-2 text-xs">{assignments.data?.items.map(v=><li key={v.id} className="break-all">{v.config_id}：{nsISO(v.effective_range.from_ns)} 至 {v.effective_range.to_ns?nsISO(v.effective_range.to_ns):'持续生效'}</li>)}</ul><ListPagination label="绑定记录" {...assignments.pagination} /></details><ChannelHistory channel={channel}/></div>}
 </div>;
}
function ConfigInputs({config}:{config:DerivedChannelConfig}) {
 return <Section title="输入与参数"><div className="divide-y rounded-lg border">{config.rule?.inputs.map(input=><div className="grid gap-2 p-3 sm:grid-cols-[12rem_1fr]" key={input.key}><p className="text-sm font-medium">{input.name||input.key} <span className="text-xs text-muted-foreground">{input.key}</span></p><BoundInputName id={config.input_bindings?.[input.key]?.channel_id}/></div>)}</div>{Object.entries(config.parameters??{}).map(([key,value])=><p key={key} className="break-all text-sm">{key} = <span className="font-mono">{value}</span></p>)}{!!Object.keys(config.baseline_bindings??{}).length&&<details><summary className="min-h-11 cursor-pointer py-3 text-sm">固定基准绑定</summary><pre className="overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(config.baseline_bindings,null,2)}</pre></details>}<p className="break-all text-xs text-muted-foreground">固定版本 {config.definition_version_id} · 配置 {config.id}</p></Section>;
}
function BoundInputName({id}:{id?:string}) {
 const m=useDerived();
 const query=useQuery({queryKey:[...m.prefix,'bound-channel',id??''],queryFn:({signal})=>m.client.monitoringDirectory(m.scope,{channel_id:id},signal),enabled:!!id&&m.canRead&&m.canNavigate});
 const c=query.data?.items[0];
 return <div className="text-sm">{c?<><p>{c.owner_name} / {c.name}</p><p className="text-xs text-muted-foreground">{c.code} · {c.unit||'无量纲'} · {c.source_kind==='derived'?'正式派生':'原始观测'}</p></>:<p>{query.isFetching?'读取输入…':'当前不能读取此输入的详细信息。'}</p>}</div>;
}
