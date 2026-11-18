import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ObjectHistory } from '../audit/object-history';
import { useState } from 'react';
import { ArrowLeft, Plus } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Permissions as P, type DeviceGrantChangeSet, type DeviceGrantCandidate, type DeviceGrant } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Badge } from '@infrapro/ui/components/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@infrapro/ui/components/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { LoadingState, ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from '../devices/context';
import { useDeviceCommand, CommandFeedback } from '../devices/command';
import { DeviceField, DeviceSelect } from '../devices/fields';
export function DeviceAccessPage({project}:{project:string}){const d=useDevices();const allowed=[P.TenantProjectRead,P.TenantDeviceRead,P.TenantDeviceAuthorizationRead].every(d.can);if(!allowed)return <ErrorState message="需要租户项目治理查看、设备查看和设备授权查看权限。"/>;return <AccessBody key={`${JSON.stringify(d.prefix)}:${project}`} project={project}/>;}
function AccessBody({project}:{project:string}){const d=useDevices();const {client,scope}=d;const cache=useQueryClient();const prefix=[...d.prefix,'access',project];const [tab,setTab]=useState('current');const [q,setQ]=useState('');const [search,setSearch]=useState('');const [kind,setKind]=useState('device');const [size,setSize]=useState(25);const [reason,setReason]=useState('');const [added,setAdded]=useState<Record<string,{candidate:DeviceGrantCandidate;use:boolean}>>({});const [changes,setChanges]=useState<Record<string,{grant:DeviceGrant;use?:boolean;end?:boolean}>>({});const state=useQuery({queryKey:[...prefix,'state'],queryFn:({signal})=>client.deviceGrants(scope.tenantId,project,{limit:1},signal)});const revision=state.data?.device_access_revision;const list=useCursorList({ pageSize: size,queryKey:[...prefix,'grants',revision,q,size],load:(cursor,signal)=>client.deviceGrants(scope.tenantId,project,{q,cursor,limit:size},signal)});const canManage=d.can(P.TenantDeviceAuthorizationManage);const editable=d.available&&canManage;const archived=state.data?.project_status==='archived';const candidates=useCursorList({ pageSize: size,queryKey:[...prefix,'candidates',revision,kind,search,size],enabled:editable&&!archived,load:(cursor,signal)=>client.deviceGrantCandidates(scope.tenantId,project,{kind,q:search,cursor,limit:size},signal)});const cmd=useDeviceCommand<DeviceGrantChangeSet>({available:editable,write:input=>client.changeDeviceGrants(scope.tenantId,project,input),preview:input=>client.previewDeviceGrants(scope.tenantId,project,input),receipt:id=>client.deviceReceipt(scope,id),done:()=>{setTab('current');setAdded({});setChanges({});setReason('');void cache.invalidateQueries({queryKey:d.prefix});void cache.invalidateQueries({queryKey:['project-context']});}});const count=Object.keys(added).length+Object.keys(changes).length;const locked=cmd.pending||!!cmd.stage||cmd.uncertain||!editable;const submit=()=>{if(!state.data)return;cmd.submit({request_id:'',reason,expected_device_access_revision:state.data.device_access_revision,expected_project_revision:state.data.project_revision,add:Object.values(added).map(v=>({kind:v.candidate.kind,target_id:v.candidate.id,use_source:v.candidate.kind==='output'&&v.use})),update:Object.values(changes).filter(v=>!v.end).map(v=>({grant_id:v.grant.id,expected_revision:v.grant.revision,use_source:!!v.use})),end:Object.values(changes).filter(v=>v.end).map(v=>({grant_id:v.grant.id,expected_revision:v.grant.revision}))});};const edit=(grant:DeviceGrant,use:boolean)=>{setChanges(v=>{const next={...v};if(use===grant.can_use_source)delete next[grant.id];else next[grant.id]={grant,use};return next;});cmd.clear();};return <section className="min-w-0 space-y-6">
  <Link to="/t/$tenantId/projects/$projectId" params={{ tenantId: scope.tenantId, projectId: project }} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="size-4" aria-hidden="true" />返回项目治理</Link>
  <ObjectHistory objectType="device_access" objectId={project}/><PageHeader title={`${state.data?.project_name ?? '项目'} · 设备授权`} description="分别授予设备档案与具体输出；新输出和新代次需要独立授权。" actions={canManage && !archived && <Button disabled={!editable} onClick={() => setTab('add')}><Plus aria-hidden="true" />新增授权</Button>} />
  {state.isPending ? <LoadingState /> : state.error ? <ErrorState message={state.error.message} onRetry={() => void state.refetch()} /> : <>
    {archived && <FormNotice>项目已归档，当前仅可缩减或结束授权。已有来源关系保留。</FormNotice>}
    <Tabs value={tab} onValueChange={setTab} className="min-w-0 gap-5">
      <TabsList className="h-auto max-w-full flex-wrap" aria-label="设备授权"><TabsTrigger value="current">当前授权</TabsTrigger>{canManage && !archived && <TabsTrigger value="add">新增对象{Object.keys(added).length > 0 && <Badge variant="secondary">{Object.keys(added).length}</Badge>}</TabsTrigger>}<TabsTrigger value="history">变更记录</TabsTrigger></TabsList>
      <TabsContent value="current" className="min-w-0 space-y-4">
        <DirectoryTable label="当前设备与输出授权" items={list.items} rowId={value => value.id} loading={list.isPending} error={list.error?.message} retry={() => void list.refetch()} empty="此项目尚无设备或输出授权。项目管理员角色不会自动获得对象范围。"
          toolbar={<DirectorySearch label="搜索当前设备或输出授权" value={q} onChange={setQ} />}
          columns={[
            { id: 'target', label: '授权对象', cell: value => <div className="space-y-1"><p className="font-medium">{value.name}</p><p className="text-xs text-muted-foreground">{value.device_name} · {value.device_code}</p><Badge variant="outline">{value.kind === 'device' ? '设备完整档案' : '具体输出'}</Badge></div> },
            { id: 'use', label: '授权能力', cell: value => <div className="space-y-1"><p className="text-sm">读取</p>{value.kind === 'output' && (canManage ? <label className="flex min-h-11 items-center gap-2 text-sm"><Checkbox checked={changes[value.id]?.use ?? value.can_use_source} disabled={locked || !!changes[value.id]?.end || archived && !value.can_use_source} onCheckedChange={checked => edit(value, checked === true)} />可作为来源使用</label> : <p className="text-xs text-muted-foreground">{value.can_use_source ? '可作为来源使用' : '不可作为来源使用'}</p>)}</div> },
            ...(canManage ? [{ id: 'end', label: '结束授权', className: 'w-24 sm:w-36', cell: (value: DeviceGrant) => <label className="flex min-h-11 items-center gap-2 text-sm"><Checkbox checked={!!changes[value.id]?.end} disabled={locked} onCheckedChange={checked => { setChanges(previous => { const next = { ...previous }; if (checked) next[value.id] = { grant: value, end: true }; else delete next[value.id]; return next; }); cmd.clear(); }} />结束此授权</label> }] : []),
          ]} pagination={<ListPagination label="授权" {...list.pagination} pageSizes={[25, 50, 100]} setPageSize={setSize} />} />
      </TabsContent>
      {canManage && !archived && <TabsContent value="add" className="min-w-0 space-y-4">
        <div className="space-y-1"><h2 className="font-semibold">选择具体授权对象</h2><p className="text-sm text-muted-foreground">设备档案不包含全部输出；单个输出授权不包含完整设备档案、安装位置或兄弟输出。</p></div>
        <div className="max-w-sm"><DeviceSelect name="grant-kind" label="授权对象种类" value={kind} onChange={setKind} options={[{ value: 'device', label: '设备完整档案' }, { value: 'output', label: '具体输出' }]} /></div>
        <DirectoryTable label="可授权对象" items={candidates.items} rowId={value => value.id} loading={candidates.isPending} error={candidates.error?.message} retry={() => void candidates.refetch()} empty="没有符合条件的未授权对象。" toolbar={<DirectorySearch label="搜索候选设备或输出" value={search} onChange={setSearch} />}
          columns={[
            { id: 'select', label: '选择', className: 'w-14', cell: value => <label className="flex min-h-11 items-center"><Checkbox checked={!!added[value.id]} disabled={locked || !added[value.id] && count >= 1000} onCheckedChange={checked => { setAdded(previous => { const next = { ...previous }; if (checked) next[value.id] = { candidate: value, use: false }; else delete next[value.id]; return next; }); cmd.clear(); }} /><span className="sr-only">选择 {value.name}</span></label> },
            { id: 'name', label: '对象', cell: value => <div className="space-y-1"><p className="font-medium">{value.name}</p><p className="text-xs text-muted-foreground">{value.device_name} · {value.device_code}</p></div> },
            { id: 'use', label: '来源使用', className: 'w-24 sm:w-44', cell: value => value.kind === 'output' ? <label className="flex min-h-11 items-center gap-2"><Checkbox checked={added[value.id]?.use ?? false} disabled={locked || !added[value.id]} onCheckedChange={checked => { setAdded(previous => ({ ...previous, [value.id]: { candidate: value, use: checked === true } })); cmd.clear(); }} />允许使用</label> : <span className="text-xs text-muted-foreground">不适用于设备档案</span> },
          ]} pagination={<ListPagination label="候选" {...candidates.pagination} pageSizes={[25, 50, 100]} setPageSize={setSize} />} />
      </TabsContent>}
      <TabsContent value="history"><GrantHistory project={project} /></TabsContent>
    </Tabs>
    {canManage && count > 0 && <Card className="shadow-none"><CardHeader><CardTitle><h2>待提交变更 <Badge variant="secondary">{count}</Badge></h2></CardTitle><CardDescription>核对已选对象并预览影响后提交，最多 1,000 项。</CardDescription></CardHeader><CardContent>
      <form className="space-y-4" onSubmit={event => { event.preventDefault(); submit(); }}><fieldset className="space-y-4" disabled={locked}>
        <ul className="max-h-64 space-y-2 overflow-auto rounded-lg bg-muted/40 p-4 text-sm">{Object.values(added).map(value => <li key={value.candidate.id} className="break-words">新增：{value.candidate.device_name} · {value.candidate.name} · 读取{value.use ? '、来源使用' : ''}</li>)}{Object.values(changes).map(value => <li key={value.grant.id} className="break-words">{value.end ? '结束' : '调整'}：{value.grant.name}{!value.end ? ` · ${value.use ? '读取与来源使用' : '仅读取'}` : ''}</li>)}</ul>
        <DeviceField name="reason" label="授权变更原因 *" value={reason} onChange={setReason} multiline issues={cmd.issues} />
        <Button type="submit" disabled={!count || count > 1000 || !reason.trim()}>预览授权变更</Button>
      </fieldset><CommandFeedback command={cmd}><p>项目：{state.data?.project_name}。新增 {Object.keys(added).length} 项，调整或结束 {Object.keys(changes).length} 项。</p><p>结束授权立即收回项目读取或使用资格，已有来源关系保留供有权人员处理。</p></CommandFeedback></form>
    </CardContent></Card>}
  </>}
</section>;}

function GrantHistory({project}:{project:string}){const {scope,client,prefix}=useDevices();const q=usePagedQuery({ initialPageSize: 25, queryKey:[...prefix,'access-history',project], queryFn: ({signal,cursor, limit})=>client.deviceGrantChanges(scope.tenantId,project,{cursor,limit},signal) });return <Card className="shadow-none"><CardHeader><CardTitle><h2>设备授权变更记录</h2></CardTitle><CardDescription>按提交时间查看明确授权对象与变更原因。</CardDescription></CardHeader><CardContent className="space-y-4">
  {q.isPending ? <LoadingState /> : q.error ? <ErrorState message={q.error.message} onRetry={() => void q.refetch()} /> : !q.data?.items.length && <p className="py-6 text-sm text-muted-foreground">暂无设备授权变更记录。</p>}
  <ol className="space-y-4">{q.data?.items.map(value => <li key={value.id} className="space-y-2 border-b pb-4"><div className="flex flex-wrap justify-between gap-2"><p className="text-sm font-medium">{value.actor_name}</p><p className="text-xs text-muted-foreground">{new Date(value.changed_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}</p></div><p className="whitespace-pre-wrap break-words text-sm">{value.reason}</p><details><summary className="cursor-pointer text-xs text-muted-foreground">查看明确授权对象与前后值</summary><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify({ before: value.before, after: value.after }, null, 2)}</pre></details></li>)}</ol>
  {q.data && <ListPagination label="记录" {...q.pagination} />}
</CardContent></Card>;}
