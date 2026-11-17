import { useState, type ReactNode } from 'react';
import { ArrowUpRight, Copy, Monitor, ShieldCheck, Users, Link2, Package, Pencil, Eye } from 'lucide-react';
import { Permissions as P, type DisplayConfig, type DisplayDetail, type DisplayList } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { useProject } from '../projects/context';
import { bindingSummary, configChanges, displaySummary, displayTime, type DisplayView } from './presentation';

export function ConfigSnapshot({ config, data }: { config: DisplayConfig; data: DisplayDetail }) {
  const pack = data.packages?.find(p => p.id === config.package_id);
  return <dl className="space-y-3 text-sm">
    <div><dt className="text-xs text-muted-foreground">程序版本</dt><dd className="mt-1 break-words">{pack ? `${pack.manifest.name ?? pack.manifest.app_key} · ${pack.manifest.version}` : '程序详情不可用'}</dd></div>
    <div><dt className="text-xs text-muted-foreground">数据绑定</dt><dd className="mt-1">{bindingSummary(config.bindings)}</dd></div>
    {!!Object.keys(config.read_views??{}).length&&<div><dt className="text-xs text-muted-foreground">命名读取视图</dt><dd className="mt-1 space-y-2">{Object.entries(config.read_views??{}).map(([name,v])=><div key={name}><p>{name} · {v.scope_id} · {v.include_future?'包含未来符合条件成员':'固定成员'}</p><p className="text-xs text-muted-foreground">{v.field_groups.join('、')}</p>{v.time_window&&<p className="text-xs text-muted-foreground">{v.time_window.from??'无额外下限'} 至 {v.time_window.to??'无额外上限'}</p>}</div>)}</dd></div>}
    <div><dt className="text-xs text-muted-foreground">时间模式</dt><dd className="mt-1">{config.mode === 'follow' ? '跟随时间窗口' : '固定历史范围'}</dd></div>
    {Object.entries(config.parameters).map(([key, value]) => <div key={key}><dt className="text-xs text-muted-foreground">{pack?.manifest.config_schema.properties?.[key]?.title ?? key}</dt><dd className="mt-1 whitespace-pre-wrap break-words">{typeof value === 'boolean' ? value ? '是' : '否' : typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)}</dd></div>)}
    <div><dt className="text-xs text-muted-foreground">保存时间</dt><dd className="mt-1">{displayTime(config.created_at)}</dd></div>
  </dl>;
}

function Section({ title, extra, children }: { title: string; extra?: ReactNode; children: ReactNode }) {
  return <section className="min-w-0 rounded-xl border bg-card"><header className="flex flex-wrap items-center justify-between gap-2 px-5 pt-5"><h3 className="text-sm font-semibold">{title}</h3>{extra}</header><div className="space-y-4 p-5">{children}</div></section>;
}

export function DisplayOverview({ data, list, editable, busy, go }: { data: DisplayDetail; list: DisplayList; editable: boolean; busy: boolean; go: (view: DisplayView) => void }) {
  const project = useProject(), s = displaySummary(data, list.settings.enabled), { instance } = data;
  const [copy, setCopy] = useState('');
  const changes = configChanges(s.liveConfig, s.draft);
  const canConfigure = editable && project.can(P.ProjectDisplayConfigure), canPreview = editable && project.can(P.ProjectDisplayPreview);
  const canPublish = editable && project.can(P.ProjectDisplayPublish);
  const access = <Section title="访问" extra={<Link2 aria-hidden="true" className="size-4 text-muted-foreground" />}>
    {s.status !== '已发布' && <p className="text-sm text-muted-foreground">{s.status}</p>}
    <div><label htmlFor={`display-address-${instance.id}`} className="text-xs text-muted-foreground">访问地址</label><div className="mt-2 flex items-center gap-1 rounded-md border bg-muted/30 pl-3"><input id={`display-address-${instance.id}`} className="h-10 min-w-0 flex-1 bg-transparent text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring" value={data.access_url} readOnly onFocus={e => e.target.select()} /><Button variant="ghost" size="icon" className="size-10 shrink-0" aria-label="复制访问地址" onClick={() => { if (!navigator.clipboard?.writeText) { setCopy('请选中地址后手动复制'); return; } void navigator.clipboard.writeText(data.access_url).then(() => setCopy('访问地址已复制')).catch(() => setCopy('复制失败，请选中地址后复制')); }}><Copy aria-hidden="true" /></Button></div>{copy && <p className="mt-2 text-xs text-muted-foreground" role="status">{copy}</p>}</div>
    <div className="flex items-start gap-3 border-t pt-4"><Users aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><div className="min-w-0 flex-1 text-sm"><p>查看人员</p><p className="mt-1 text-xs leading-5 text-muted-foreground">{instance.viewers.length ? `指定 ${instance.viewers.length} 名项目成员` : '具备大屏查看权限的项目成员'}</p></div>{project.can(P.ProjectDisplayManage) && <Button size="sm" variant="ghost" className="h-8 px-2" onClick={() => go('设置')}>设置</Button>}</div>
    {s.management && <div className="flex items-start gap-3 border-t pt-4"><Monitor aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><div className="flex-1 text-sm"><p>固定终端</p><p className="mt-1 text-xs text-muted-foreground">{data.terminals?.filter(t => t.state !== 'revoked').length ?? 0} 台</p></div><Button size="sm" variant="ghost" className="h-8 px-2" onClick={() => go('运行与终端')}>{project.can(P.ProjectDisplayTerminals) ? '管理' : '查看'}</Button></div>}
  </Section>;
  if (!s.management) return <div className="max-w-xl">{access}</div>;
  return <div className="grid min-w-0 gap-4 min-[900px]:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
    <div className="min-w-0 space-y-4">
      <Section title="线上版本" extra={s.release && <Badge variant="outline">{s.release.withdrawn ? '已撤回' : '已发布'}</Badge>}>
        {s.release ? <>
          <div className="flex items-center gap-3"><span className="rounded-lg border bg-muted/50 p-3"><Monitor className="size-5 text-muted-foreground" aria-hidden="true" /></span><div className="min-w-0"><p className="break-words text-sm font-medium">{s.livePackage?.manifest.name ?? s.livePackage?.manifest.app_key ?? '程序详情不可用'}</p><p className="mt-1 text-xs text-muted-foreground">程序 {s.livePackage?.manifest.version ?? '—'}</p></div></div>
          <p className="text-sm">{s.liveConfig ? bindingSummary(s.liveConfig.bindings) : '配置详情不可用'}</p>
          <p className="text-xs text-muted-foreground">发布于 {displayTime(s.release.created_at)}</p>
          {s.liveConfig && <details className="border-t pt-3"><summary className="cursor-pointer text-sm font-medium">查看线上配置</summary><div className="mt-4"><ConfigSnapshot config={s.liveConfig} data={data} /></div></details>}
        </> : <div className="flex min-h-32 flex-col items-center justify-center gap-2 text-center"><Monitor className="size-7 text-muted-foreground/60" aria-hidden="true" /><p className="text-sm font-medium">{instance.current_release_id ? '线上版本详情不可用' : '暂无线上版本'}</p><p className="text-xs text-muted-foreground">{instance.current_release_id ? '请在发布记录中查看' : '访问入口尚未开放'}</p></div>}
      </Section>
      <Section title="工作草稿" extra={s.draft && <Badge variant="secondary">{s.liveConfig ? changes.length ? `${changes.length} 项更改` : '与线上一致' : '未发布'}</Badge>}>
        {s.draft ? <><div className="space-y-3 text-sm"><p className="flex items-start gap-2"><Package aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><span>{s.draftPackage?.manifest.name ?? s.draftPackage?.manifest.app_key ?? '程序详情不可用'} · {s.draftPackage?.manifest.version ?? '—'}</span></p><p>{bindingSummary(s.draft.bindings)}</p></div><p className="text-xs text-muted-foreground">{changes.length ? `${changes.join('、')}已更改` : `保存于 ${displayTime(s.draft.created_at)}`}</p></> : <p className="py-3 text-sm text-muted-foreground">{data.packages?.some(p => p.state === 'ready') ? '尚未选择程序与展示内容' : '尚未上传程序'}</p>}
        {s.grant !== '有效' && <button className="flex w-full items-center justify-between gap-2 rounded-lg bg-muted/60 p-3 text-left text-xs hover:bg-muted focus-visible:outline-ring" onClick={() => go('读取授权')}><span>数据授权{s.grant === '未授权' ? '未设置' : s.grant}</span><ArrowUpRight className="size-3.5 shrink-0" aria-hidden="true" /></button>}
        <div className="flex flex-wrap items-center gap-2 border-t pt-3">
          {canConfigure && (data.packages?.some(p => p.state === 'ready') ? <Button variant="ghost" size="sm" disabled={busy} onClick={() => go('配置')}><Pencil aria-hidden="true" />{s.draft ? '编辑配置' : '选择程序'}</Button> : project.can(P.ProjectDisplayUpload) && <Button variant="outline" size="sm" onClick={() => go('程序版本')}>上传程序</Button>)}
          {!canConfigure && s.draft && <Button variant="ghost" size="sm" onClick={() => go('配置')}>查看配置</Button>}
          <div className="ml-auto flex gap-2">{canPreview && s.draft && <Button asChild size="sm" variant="outline"><a href={`${list.runtime_origin}/display/${instance.id}?config=${s.draft.id}`} target="_blank" rel="noreferrer"><Eye aria-hidden="true" />预览</a></Button>}{canPublish && <Button size="sm" disabled={busy || !s.draft} onClick={() => go('发布')}>{s.release ? '发布更改' : '发布'}</Button>}</div>
        </div>
      </Section>
    </div>
    <div className="min-w-0 space-y-4">{access}<Section title="数据授权" extra={<Badge variant="outline">{s.grant}</Badge>}>
      <div className="flex items-start gap-2 text-sm"><ShieldCheck aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" /><p>{data.grant ? `${data.grant.capabilities.length} 项读取能力 · ${bindingSummary({ resources: { kind: 'resources', ids: data.grant.scope.resources }, channels: { kind: 'channels', ids: data.grant.scope.channels }, spatial: { kind: 'spatial', ids: data.grant.scope.spatial }, artifacts: { kind: 'artifacts', ids: data.grant.scope.artifacts } })}` : '尚无数据读取授权'}</p></div>
      {!!data.grant?.read_scopes?.length&&<p className="text-xs text-muted-foreground">{data.grant.read_scopes.length} 个命名读取范围 · {data.grant.read_scopes.filter(s=>s.include_future).length} 个允许未来成员</p>}
      {data.grant?.scope.dynamic && <p className="text-xs text-muted-foreground">含动态通道范围{data.grant.scope.dynamic.include_future ? '与未来新增通道' : ''}</p>}
      <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-3"><span className="text-xs text-muted-foreground">{data.grant ? `至 ${displayTime(data.grant.valid_until)}` : '—'}</span><Button size="sm" variant="ghost" onClick={() => go('读取授权')}>{project.can(P.ProjectDisplayGrant) ? '管理授权' : '查看授权'}</Button></div>
    </Section></div>
  </div>;
}
