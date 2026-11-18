import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Permissions, type Device, type DeviceLegacySection, type DeviceLegacySectionResolution, type DeviceTenantScope } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { DirectorySearch, DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from './context';
import { CommandFeedback, useDeviceCommand } from './command';
import { DeviceField, DeviceSelect } from './fields';

const labels = {pending:'待核对',historical:'仅作历史依据',mapped:'已映射正式断面'};
export function InstallationLegacySections({device}:{device:Device}) {
 const {scope,can,canHistory}=useDevices();
 return scope.kind==='tenant'&&canHistory&&can(Permissions.TenantEngineeringResourceRead)?<LegacyList scope={scope} device={device}/>:null;
}
function LegacyList({scope,device}:{scope:DeviceTenantScope;device:Device}) {
 const {client,prefix,available}=useDevices();const cache=useQueryClient();const [open,setOpen]=useState(false);const [pageSize,setPageSize]=useState(25);const [selected,setSelected]=useState<DeviceLegacySection>();const focus=useRef<HTMLElement|null>(null);
 const q=useCursorList({ pageSize,queryKey:[...prefix,'legacy-sections',device.id,pageSize],enabled:open,load:(cursor,signal)=>client.deviceLegacySections(scope,device.id,{cursor,limit:pageSize},signal)});
 return <section className="space-y-3 border-t pt-4"><Button variant="outline" aria-expanded={open} onClick={()=>setOpen(!open)}>旧断面资料核对</Button>{open&&<><FormNotice>每条资料分别保留原文、来源和修订。待核对资料不计作断面；映射引用由维护者明确选择。</FormNotice><DirectoryTable label="旧断面迁移证据" items={q.items} rowId={r=>r.id} loading={q.isPending} error={q.error?.message} retry={()=>void q.refetch()} empty="该设备没有旧断面文字需要核对。" columns={[
 {id:'original',label:'原文与来源',cell:r=><div className="space-y-1"><pre className="max-w-lg whitespace-pre-wrap break-all font-sans text-sm">{typeof r.original==='string'?r.original:JSON.stringify(r.original,null,2)}</pre><p className="text-xs text-muted-foreground">{r.source_table==='device_installations'?'安装记录':'安装变更历史'} · 修订 {r.source_revision} · 第 {r.entry_index+1} 条</p><p className="text-xs text-muted-foreground">{r.historical?'历史资料':'迁移时的当前资料'} · {new Date(r.captured_at).toLocaleString('zh-CN')}</p></div>},
 {id:'resolution',label:'核对结果',cell:r=><div className="space-y-1"><p>{labels[r.status]}</p>{r.target_name&&<p>正式断面：{r.target_name}</p>}{r.resolution_reason&&<p className="text-sm text-muted-foreground">{r.resolution_reason}</p>}</div>},
 {id:'actions',label:'操作',cell:r=>device.actions.install&&<Button variant="outline" disabled={!available} onClick={e=>{focus.current=e.currentTarget;setSelected(r);}}>核对此条</Button>}
 ]} pagination={<ListPagination label="资料" {...q.pagination} setPageSize={setPageSize} />}/></>}
 <ResponsivePanel open={!!selected} onOpenChange={v=>{if(!v)setSelected(undefined);}} title="核对旧断面资料" description="选择核对结果并填写原因，保存后保留历次记录。" returnFocus={focus}>{selected&&<ResolutionForm scope={scope} device={device} record={selected} done={()=>{setSelected(undefined);void cache.invalidateQueries({queryKey:prefix});}}/>}</ResponsivePanel></section>;
}
function ResolutionForm({scope,device,record,done}:{scope:DeviceTenantScope;device:Device;record:DeviceLegacySection;done:()=>void}) {
 const {client,prefix,available}=useDevices();const [status,setStatus]=useState<DeviceLegacySectionResolution['status']>(record.status);const [target,setTarget]=useState(record.target_id??'');const [reason,setReason]=useState('');const [search,setSearch]=useState('');
 const targets=useCursorList({ initialPageSize: 10,queryKey:[...prefix,'legacy-section-targets',device.id,record.facility_id,search],enabled:status==='mapped',load:(cursor, signal, limit)=>client.deviceInstallationTargets(scope,device.id,{kind:'monitoring_section',state:'history',facility_id:record.facility_id??undefined,q:search,cursor,limit},signal)});
 const command=useDeviceCommand<DeviceLegacySectionResolution>({available,preview:input=>client.previewDeviceLegacySection(scope,device.id,record.id,input),write:input=>client.resolveDeviceLegacySection(scope,device.id,record.id,input),receipt:id=>client.deviceReceipt(scope,id),done});
 return <form className="space-y-4" onSubmit={e=>{e.preventDefault();command.submit({request_id:'',status,reason,...(status==='mapped'?{target:{kind:'monitoring_section',id:target}}:{})});}}><fieldset disabled={command.pending||!!command.stage||command.uncertain||!available} className="space-y-4"><pre className="whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-sans text-sm">{typeof record.original==='string'?record.original:JSON.stringify(record.original,null,2)}</pre><DeviceSelect name="status" label="核对结果" value={status} onChange={v=>{setStatus(v as typeof status);command.clear();}} options={Object.entries(labels).map(([value,label])=>({value,label}))}/>{status==='mapped'&&<fieldset id="/target" tabIndex={-1} className="space-y-3 rounded-lg border p-3"><legend>选择正式监测断面</legend>{!record.facility_id&&<FormNotice>旧记录未注明设施。请确认真实设施归属后选择；无法确认时保留待核对。</FormNotice>}<DirectorySearch label="搜索正式断面" value={search} onChange={setSearch}/>{targets.error&&<FormNotice error>{targets.error.message}</FormNotice>}{targets.items.map(v=><Button key={v.id} type="button" className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" variant={v.id===target?'secondary':'outline'} aria-pressed={v.id===target} onClick={()=>setTarget(v.id)}>{v.name} · {v.code}{v.status==='retired'?' · 已退役':''}</Button>)}<ListPagination label="目标" {...targets.pagination} /></fieldset>}<DeviceField name="reason" label="核对依据 *" value={reason} onChange={setReason} multiline issues={command.issues}/><Button type="submit" disabled={!reason.trim()||status==='mapped'&&!target}>预览核对结果</Button></fieldset><CommandFeedback command={command}><p>核对结果：{labels[status]}。安装期间、安装目标和当前来源保持原事实。</p></CommandFeedback></form>;
}
