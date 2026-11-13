import { useRef, useState } from 'react';
import { ArrowLeft, Package, Radio } from 'lucide-react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { Device, DeviceInstallation, DeviceOutput, DeviceProductSlot, DeviceRevisionInput, DeviceState, EngineeringVersion } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@infrapro/ui/components/card';
import { DirectoryActions, DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { LoadingState, ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from './context';
import { DeviceEditor } from './device-editor';
import { DeviceField, OutputMeaning } from './fields';
import { InstallationEditor } from './installations';
import { OutputEditor } from './output-editor';
import { OutputSources } from './source-panel';
import { DeviceHistory } from './history';
import { useDeviceCommand, CommandFeedback } from './command';
export type DeviceDetailSearch={tab?:'details'|'outputs'|'installations'|'changes';output?:string};
export function DevicePage({ id, search, onSearch }: { id: string; search: DeviceDetailSearch; onSearch: (value: DeviceDetailSearch) => void }) {
  const { scope, client, prefix, paths } = useDevices();
  const query = useQuery({ queryKey: [...prefix, 'detail', id], queryFn: ({ signal }) => client.device(scope, id, signal) });
  return <section className="min-w-0 space-y-5">
    <Link to={paths.list} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="size-4" aria-hidden="true" />返回设备列表</Link>
    {query.isPending ? <LoadingState /> : query.error || !query.data ? <ErrorState message={query.error?.message ?? '无法读取设备'} onRetry={() => void query.refetch()} /> : <DeviceContent device={query.data} search={search} onSearch={onSearch} />}
  </section>;
}
export function DeviceContent({ device, search, onSearch, embedded = false, onDeleted }: { device: Device; search: DeviceDetailSearch; onSearch: (value: DeviceDetailSearch) => void; embedded?: boolean; onDeleted?: () => void }) {
  const { scope, prefix, available, paths } = useDevices();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const [panel, setPanel] = useState<'edit' | 'state' | 'delete' | null>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const tab = search.tab === 'changes' && !device.actions.history ? 'details' : search.tab ?? 'details';
  const done = () => { setPanel(null); void cache.invalidateQueries({ queryKey: prefix }); };
  const actions = [
    ...(device.actions.state ? [{ label: device.status === 'enabled' ? '停用设备' : '启用设备', disabled: !available, opensPanel: true, run: (trigger: HTMLButtonElement | null) => { returnFocus.current = trigger; setPanel('state'); } }] : []),
    ...(device.actions.delete ? [{ label: '删除设备', disabled: !available, opensPanel: true, run: (trigger: HTMLButtonElement | null) => { returnFocus.current = trigger; setPanel('delete'); } }] : []),
  ];
  const headerActions = <div className="flex items-center gap-2">
    {device.actions.edit && <Button id="edit-device" variant="outline" disabled={!available} onClick={(event) => { returnFocus.current = event.currentTarget; setPanel('edit'); }}>编辑档案</Button>}
    {(device.actions.state || device.actions.delete) && <DirectoryActions id="device-actions" name="设备" actions={actions} />}
  </div>;
  return <>
    {embedded ? <div className="flex flex-wrap items-start justify-between gap-3 border-b pb-5"><div className="min-w-0 flex-1 space-y-2"><h2 className="break-words text-xl font-semibold tracking-tight">{device.name}</h2><div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground"><span className="break-all font-mono text-xs">{device.code}</span><Badge variant={device.status === 'enabled' ? 'secondary' : 'outline'}>{device.status === 'enabled' ? '启用' : '停用'}</Badge></div><p className="break-words text-sm text-muted-foreground">{device.product.name} · v{device.product.number}</p></div>{headerActions}</div> : <PageHeader title={device.name} description={device.code} actions={headerActions} />}
    <div className={embedded ? "min-w-0" : "grid min-w-0 items-start gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]"}>
      {!embedded && <aside className="min-w-0 space-y-4" aria-label="设备摘要">
        <Card className="gap-4 shadow-none"><CardHeader><div className="mb-2 flex items-center justify-between gap-2"><Package className="size-5 text-muted-foreground" aria-hidden="true" /><Badge variant={device.status === 'enabled' ? 'secondary' : 'outline'}>{device.status === 'enabled' ? '启用' : '停用'}</Badge></div><CardDescription>采用产品</CardDescription><CardTitle className="break-words text-base leading-relaxed">{device.product.name}</CardTitle></CardHeader>
          <CardContent className="space-y-4"><dl className="space-y-3 text-sm">{[['产品版本', `v${device.product.number}`], ['型号', device.product.model], ['厂家', device.product.manufacturer_name]].map(([label, value]) => <div key={label} className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-words">{value}</dd></div>)}</dl><Badge variant="outline">{device.product.source === 'public' ? '平台公共' : '本租户私有'}</Badge><p className="border-t pt-4 text-xs leading-relaxed text-muted-foreground">采用的产品版本固定保留，产品发布新版本不会自动更新此设备。</p></CardContent>
        </Card>
      </aside>}
      <Tabs value={tab} className="min-w-0 gap-5" onValueChange={(value) => onSearch({ tab: value as DeviceDetailSearch['tab'] })}>
        <TabsList className="h-auto max-w-full flex-wrap" aria-label="设备详情"><TabsTrigger value="details">设备档案</TabsTrigger><TabsTrigger value="outputs">{scope.kind === 'project' ? '获准输出' : '输出槽'}</TabsTrigger><TabsTrigger value="installations">安装履历</TabsTrigger>{device.actions.history && <TabsTrigger value="changes">变更记录</TabsTrigger>}</TabsList>
        <TabsContent value="details" className="min-w-0 space-y-5">
          <Card className="shadow-none"><CardHeader><CardTitle><h2>设备档案</h2></CardTitle><CardDescription>设备自身的身份与业务资料。</CardDescription></CardHeader><CardContent><dl className="grid gap-5 sm:grid-cols-2">{[['设备编号', device.code], ['序列号', device.serial_number || '未填写'], ['登记时间', new Date(device.created_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })], ['业务说明', device.description || '未填写'], ...(embedded ? [['采用产品', `${device.product.name} · v${device.product.number}`], ['产品型号', device.product.model], ['设备厂家', device.product.manufacturer_name], ['产品来源', device.product.source === 'public' ? '平台公共' : '本租户私有']] : [])].map(([label, value]) => <div key={label} className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 whitespace-pre-wrap break-words text-sm">{value}</dd></div>)}</dl></CardContent></Card>
          <Card className="gap-4 shadow-none"><CardHeader><CardTitle><h2>当前安装</h2></CardTitle><CardDescription>安装记录与设备输出、来源配置分别维护。</CardDescription></CardHeader><CardContent>{device.current_installation ? <InstallationFacts value={device.current_installation} /> : <p className="text-sm text-muted-foreground">无当前安装记录</p>}</CardContent></Card>
        </TabsContent>
        <TabsContent value="outputs" className="min-w-0"><Outputs device={device} selected={search.output} onSelect={(output) => onSearch({ tab: 'outputs', output })} /></TabsContent>
        <TabsContent value="installations" className="min-w-0"><Installations device={device} /></TabsContent>
        {device.actions.history && <TabsContent value="changes" className="min-w-0"><DeviceHistory device={device.id} /></TabsContent>}
      </Tabs>
    </div>
    <ResponsivePanel open={!!panel} onOpenChange={(open) => { if (!open) setPanel(null); }} title={panel === 'edit' ? '编辑设备档案' : panel === 'state' ? '变更设备状态' : '删除设备'} description="核对当前记录后提交。" returnFocus={returnFocus} fallbackFocusSelector={panel === 'edit' ? '#edit-device' : '#device-actions'}>
      {panel === 'edit' ? <DeviceEditor initial={device} done={done} /> : panel && <DeviceLifecycle key={panel} device={device} mode={panel} done={() => { done(); if (panel === 'delete') { if (onDeleted) onDeleted(); else void navigate({ to: paths.list }); } }} />}
    </ResponsivePanel>
  </>;
}
function DeviceLifecycle({device,mode,done}:{device:Device;mode:'state'|'delete';done:()=>void}){const {scope,client,available}=useDevices();const [reason,setReason]=useState('');const status=device.status==='enabled'?'disabled':'enabled';const cmd=useDeviceCommand<DeviceState|DeviceRevisionInput>({available,write:input=>{if(scope.kind!=='tenant')throw new Error('请从租户入口操作。');return mode==='state'?client.stateDevice(scope,device.id,input as DeviceState):client.deleteDevice(scope,device.id,input);},preview:input=>{if(scope.kind!=='tenant')throw new Error('租户范围无效');return client.previewDevice(scope,device.id,mode==='state'?{...input,action:'state',status}:{...input,action:'delete'});},receipt:id=>client.deviceReceipt(scope,id),done});return <form className="space-y-4" onSubmit={e=>{e.preventDefault();cmd.submit({request_id:'',expected_revision:device.revision,reason,...(mode==='state'?{status}:{})});}}><fieldset className="space-y-4" disabled={cmd.pending||!!cmd.stage||cmd.uncertain||!available}><FormNotice>{mode==='delete'?'存在安装、输出使用或来源历史等引用时，删除会被阻止。已使用过的编号永久保留。':'变更设备状态保留安装和已有来源。停用后可以解除原来源，不能建立新的来源关系。'}</FormNotice><DeviceField name="reason" label="操作原因 *" value={reason} onChange={setReason} multiline issues={cmd.issues}/><Button type="submit">预览变更</Button></fieldset><CommandFeedback command={cmd}><p>{mode==='delete'?'删除':status==='disabled'?'停用':'启用'}设备：{device.name}。</p></CommandFeedback></form>;}
function Outputs({device,selected,onSelect}:{device:Device;selected?:string;onSelect:(v?:string)=>void}){const {scope,client,prefix,paths,available}=useDevices();const cache=useQueryClient();const [edit,setEdit]=useState<{slot:DeviceProductSlot;output?:DeviceOutput;mode:'configure'|'edit'|'replace'|'end';definitions?:EngineeringVersion[]}>();const [history,setHistory]=useState<string>();const outputFocus=useRef<HTMLElement|null>(null);const [pageSize,setPageSize]=useState(25);const slots=useCursorList({queryKey:[...prefix,'slots',device.id,pageSize],enabled:scope.kind==='tenant',load:(cursor,signal)=>{if(scope.kind!=='tenant')throw new Error('租户范围无效');return client.deviceSlots(scope,device.id,{cursor,limit:pageSize},signal);}});const outputs=useCursorList({queryKey:[...prefix,'device-outputs',device.id,pageSize],enabled:scope.kind==='project',load:(cursor,signal)=>{if(scope.kind!=='project')throw new Error('项目范围无效');return client.projectDeviceOutputs(scope,{device_id:device.id,cursor,limit:pageSize},signal);}});const detail=useQuery({queryKey:[...prefix,'output',device.id,selected],enabled:!!selected,queryFn:({signal})=>client.deviceOutput(scope,selected!,device.id,signal)});
const openOutputAction=(mode:'edit'|'replace'|'end',trigger:HTMLButtonElement|null)=>{outputFocus.current=trigger;const output=detail.data;if(output)setEdit({slot:output.template,output,mode});};
return <div className="min-w-0 space-y-5">
  {!selected && (scope.kind === 'tenant' ? <DirectoryTable label="产品输出槽" items={slots.items} rowId={value => value.template.key} loading={slots.isPending} error={slots.error?.message} retry={() => void slots.refetch()} empty="此产品版本没有输出槽。"
    columns={[
      { id: 'name', label: '输出槽', cell: value => <div className="space-y-1"><p className="font-medium">{value.template.name}</p><p className="break-all font-mono text-xs text-muted-foreground">{value.template.key}</p><Badge variant="outline">{value.template.mode === 'fixed' ? '固定' : '可配置'}</Badge></div> },
      { id: 'current', label: '当前输出', cell: value => value.current ? <Button variant="link" className="h-auto min-h-11 max-w-full justify-start px-0 text-left whitespace-normal" onClick={() => onSelect(value.current!.id)}>{value.current.name} · 第 {value.current.generation} 代</Button> : <Badge variant="outline">未配置</Badge> },
      { id: 'actions', label: '操作', className: 'w-14', cell: value => <DirectoryActions name={value.template.name} actions={[
        ...(!value.current && device.actions.configure ? [{ label: '配置输出', disabled: !available, opensPanel: true, run: (trigger: HTMLButtonElement | null) => { outputFocus.current = trigger; setEdit({ slot: value.template, mode: 'configure', definitions: value.definitions }); } }] : []),
        { label: '代次历史', opensPanel: true, run: (trigger: HTMLButtonElement | null) => { outputFocus.current = trigger; setHistory(value.template.key); } },
      ]} /> },
    ]} pagination={<ListPagination pageSizes={[25, 50, 100]} pageSize={pageSize} setPageSize={setPageSize} label="输出槽" count={slots.items.length} page={slots.page} pageCount={slots.pageCount} hasNext={!!slots.hasNext} fetching={slots.isFetching} go={slots.go} next={() => void slots.next()} />} />
    : <DirectoryTable label="该设备的获准输出" items={outputs.items} rowId={value => value.id} loading={outputs.isPending} error={outputs.error?.message} retry={() => void outputs.refetch()} empty="该设备尚无明确授予本项目的输出。"
      columns={[{ id: 'name', label: '输出', cell: value => <Link to={paths.output(value.id)} className="font-medium underline-offset-4 hover:underline">{value.name}</Link> }, { id: 'generation', label: '代次', cell: value => `第 ${value.generation} 代${value.ended_at ? '（已结束）' : ''}` }]}
      pagination={<ListPagination pageSizes={[25, 50, 100]} pageSize={pageSize} setPageSize={setPageSize} label="输出" count={outputs.items.length} page={outputs.page} pageCount={outputs.pageCount} hasNext={!!outputs.hasNext} fetching={outputs.isFetching} go={outputs.go} next={() => void outputs.next()} />} />)}
  {selected && <>
    <Button variant="ghost" size="sm" onClick={() => onSelect()}><ArrowLeft aria-hidden="true" />收起输出</Button>
    {detail.isPending ? <LoadingState /> : detail.error || !detail.data ? <ErrorState message={detail.error?.message ?? '无法读取输出'} onRetry={() => void detail.refetch()} /> : <>
      <div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-lg font-semibold">{detail.data.name} · 第 {detail.data.generation} 代{detail.data.ended_at ? '（已结束）' : ''}</h2><p className="mt-1 text-sm text-muted-foreground">输出槽：{detail.data.template.name} · {detail.data.template.key}</p></div>
        {device.actions.configure && !detail.data.ended_at && <DirectoryActions name="输出" actions={[
          { label: '编辑名称与说明', disabled: !available, opensPanel: true, run: trigger => openOutputAction('edit', trigger) },
          { label: '替换观测含义', disabled: !available, opensPanel: true, run: trigger => openOutputAction('replace', trigger) },
          { label: '结束输出', disabled: !available, opensPanel: true, run: trigger => openOutputAction('end', trigger) },
        ]} />}
      </div>
      <Tabs key={selected} defaultValue="meaning" className="min-w-0 gap-4"><TabsList aria-label="当前输出"><TabsTrigger value="meaning">观测含义</TabsTrigger><TabsTrigger value="sources">来源关联</TabsTrigger></TabsList>
        <TabsContent value="meaning"><Card className="shadow-none"><CardContent><OutputMeaning output={detail.data} />{detail.data.description && <p className="mt-5 border-t pt-4 text-sm whitespace-pre-wrap break-words">{detail.data.description}</p>}</CardContent></Card></TabsContent>
        <TabsContent value="sources"><OutputSources output={detail.data} /></TabsContent>
      </Tabs>
    </>}
  </>}
  <ResponsivePanel open={!!edit} onOpenChange={open => { if (!open) setEdit(undefined); }} returnFocus={outputFocus} title="维护设备输出" description="核对输出槽和当前代次。">{edit && <OutputEditor device={device} {...edit} done={result => { setEdit(undefined); void cache.invalidateQueries({ queryKey: prefix }); if (result.output_id) onSelect(result.output_id); }} />}</ResponsivePanel>
  <ResponsivePanel open={!!history} onOpenChange={open => { if (!open) setHistory(undefined); }} returnFocus={outputFocus} title="输出代次历史" description="每一代使用独立输出身份，授权和来源均按具体代次保留。">{history && <Generations device={device.id} slot={history} onSelect={id => { onSelect(id); setHistory(undefined); }} />}</ResponsivePanel>
</div>;}

function Generations({device,slot,onSelect}:{device:string;slot:string;onSelect:(id:string)=>void}){const {scope,client,prefix}=useDevices();const [pageSize,setPageSize]=useState(25);const q=useCursorList({queryKey:[...prefix,'generations',device,slot,pageSize],load:(cursor,signal)=>{if(scope.kind!=='tenant')throw new Error('请在租户入口查看全部代次。');return client.deviceGenerations(scope,device,{slot_key:slot,cursor,limit:pageSize},signal);}});return <div className="space-y-3">{q.error&&<FormNotice error>{q.error.message}</FormNotice>}{q.items.map(v=><Button key={v.id} variant="outline" className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" onClick={()=>onSelect(v.id)}>{v.name} · 第 {v.generation} 代 · {v.ended_at?'已结束':'当前'}</Button>)}<ListPagination pageSizes={[25,50,100]} pageSize={pageSize} setPageSize={setPageSize} label="代次" count={q.items.length} page={q.page} pageCount={q.pageCount} hasNext={!!q.hasNext} fetching={q.isFetching} go={q.go} next={()=>void q.next()}/></div>;}
function InstallationFacts({value:v}:{value:DeviceInstallation}){return <div className="space-y-2 text-sm"><p>{v.visibility==='restricted'?'安装目标详情受限':v.target_kind==='location'?v.location_description:`${v.target_name??'历史安装目标'}${v.target_status==='retired'?'（已退役）':''}`}</p><p>实际期间：{new Date(v.installed_at).toLocaleString()} — {v.uninstalled_at?new Date(v.uninstalled_at).toLocaleString():'当前'}</p><p className="text-muted-foreground">登记时间：{new Date(v.recorded_at).toLocaleString()}</p>{v.visibility!=='restricted'&&v.position&&<details><summary className="cursor-pointer underline">位置描述</summary><pre className="whitespace-pre-wrap break-all text-xs">{JSON.stringify(v.position,null,2)}</pre></details>}</div>;}
function Installations({device}:{device:Device}){const {scope,client,prefix,available}=useDevices();const cache=useQueryClient();const [mode,setMode]=useState<'create'|'end'|'relocate'|'correct'>();const [initial,setInitial]=useState<DeviceInstallation>();const [pageSize,setPageSize]=useState(25);const q=useCursorList({queryKey:[...prefix,'installations',device.id,pageSize],load:(cursor,signal)=>client.deviceInstallations(scope,device.id,{cursor,limit:pageSize},signal)});return <div className="space-y-4">{device.actions.install&&<Button id="create-installation" disabled={!available} onClick={()=>{setInitial(undefined);setMode('create');}}>记录安装或补录历史</Button>}<DirectoryTable label="安装履历" items={q.items} rowId={v=>v.id} loading={q.isPending} error={q.error?.message} retry={()=>void q.refetch()} empty="尚未登记安装记录。设备可以保持无安装记录。" columns={[{id:'facts',label:'安装事实',cell:v=><InstallationFacts value={v}/>},{id:'actions',label:'操作',cell:v=>device.actions.install&&<div className="flex flex-wrap gap-2">{(v.uninstalled_at?['correct'] as const:['end','relocate','correct'] as const).map(mode=><Button key={mode} size="sm" variant="outline" disabled={!available} onClick={()=>{setInitial(v);setMode(mode);}}>{({end:'拆除',relocate:'迁移',correct:'纠错'})[mode]}</Button>)}</div>}]} pagination={<ListPagination pageSizes={[25,50,100]} pageSize={pageSize} setPageSize={setPageSize} label="安装" count={q.items.length} page={q.page} pageCount={q.pageCount} hasNext={!!q.hasNext} fetching={q.isFetching} go={q.go} next={()=>void q.next()}/>}/><ResponsivePanel open={!!mode} onOpenChange={open=>{if(!open)setMode(undefined);}} title="维护安装记录" description="实际时间与登记时间分别保留，纠错后仍可追溯原记录。" fallbackFocusSelector="#create-installation">{mode&&<InstallationEditor key={`${mode}:${initial?.id??''}`} mode={mode} initial={initial} device={device} done={()=>{setMode(undefined);void cache.invalidateQueries({queryKey:prefix});}}/>}</ResponsivePanel></div>;}
export function ProjectOutputPage({ id }: { id: string }) {
  const { scope, client, prefix, paths, canHistory, canSource, canSourceHistory } = useDevices();
  const query = useQuery({ queryKey: [...prefix, 'output', id], queryFn: ({ signal }) => client.deviceOutput(scope, id, undefined, signal) });
  const output = query.data;
  return <section className="min-w-0 space-y-5">
    <Link to={paths.list} search={{ tab: 'outputs' }} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="size-4" aria-hidden="true" />返回获准设备与输出</Link>
    {query.isPending ? <LoadingState /> : query.error || !output ? <ErrorState message={query.error?.message ?? '无法读取输出'} onRetry={() => void query.refetch()} /> : <>
      <PageHeader title={output.name} description={`${output.device.name} · ${output.device.code}`} />
      <div className="grid min-w-0 items-start gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <Card className="gap-4 shadow-none" aria-label="获准输出摘要"><CardHeader><Radio className="mb-2 size-5 text-muted-foreground" aria-hidden="true" /><CardDescription>具体输出</CardDescription><CardTitle className="text-base">第 {output.generation} 代</CardTitle></CardHeader><CardContent className="space-y-4"><Badge variant={output.ended_at || output.device_status === 'disabled' ? 'outline' : 'secondary'}>{output.ended_at ? '已结束，仅供历史查看' : output.device_status === 'disabled' ? '设备停用' : '当前代次'}</Badge><dl className="space-y-3 text-sm"><div><dt className="text-xs text-muted-foreground">所属产品</dt><dd className="mt-1 break-words">{output.device.product_name}</dd></div><div><dt className="text-xs text-muted-foreground">型号</dt><dd className="mt-1 break-words">{output.device.product_model}</dd></div></dl><p className="border-t pt-4 text-xs leading-relaxed text-muted-foreground">此页展示当前输出的授权内容。完整设备档案与其他输出分别授权。</p></CardContent></Card>
        <Tabs defaultValue="meaning" className="min-w-0 gap-5" key={id}>
          <TabsList className="h-auto max-w-full flex-wrap" aria-label="输出详情"><TabsTrigger value="meaning">观测含义</TabsTrigger>{(canSource || canHistory && canSourceHistory) && <TabsTrigger value="sources">来源关联</TabsTrigger>}{canHistory && <TabsTrigger value="changes">变更记录</TabsTrigger>}</TabsList>
          <TabsContent value="meaning"><Card className="shadow-none"><CardHeader><CardTitle><h2>观测含义</h2></CardTitle><CardDescription>本代输出采用的指标、单位与时间解释。</CardDescription></CardHeader><CardContent className="space-y-5"><OutputMeaning output={output} />{output.description && <div className="border-t pt-4"><p className="text-xs text-muted-foreground">输出说明</p><p className="mt-1 whitespace-pre-wrap break-words text-sm">{output.description}</p></div>}</CardContent></Card></TabsContent>
          {(canSource || canHistory && canSourceHistory) && <TabsContent value="sources"><OutputSources output={output} /></TabsContent>}
          {canHistory && <TabsContent value="changes"><DeviceHistory output={id} /></TabsContent>}
        </Tabs>
      </div>
    </>}
  </section>;
}
