import { useState } from 'react';
import { ArrowLeft, Check } from 'lucide-react';
import { Permissions as P, type DisplayDetail, type DisplayReadViewDescription } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { EmptyState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useProject } from '../projects/context';
import { ConfigSnapshot } from './overview';
import { configChanges, displayTime } from './presentation';
import { ConfirmDisplayAction } from './confirm-action';

type Preparation = { read_views?:DisplayReadViewDescription[]; hash: string; binding_sets: Record<string, string[]>; grant_revision: string; revision: string; configID: string };
export function Publication({ data, review, editable, busy, command, onPublished }: { data: DisplayDetail; review: boolean; editable: boolean; busy: boolean; command: (path: string, body: object, method?: string, revision?: string) => Promise<unknown>; onPublished: () => void }) {
  const project = useProject(), { client } = useIdentity();
  const [rollback, setRollback] = useState(''), [preparation, setPreparation] = useState<Preparation | null>(null), [checking, setChecking] = useState(false), [error, setError] = useState('');
  const { instance } = data, canPublish = project.can(P.ProjectDisplayPublish), disabled = busy || checking || !editable || !canPublish;
  const current = data.releases?.find(r => r.id === instance.current_release_id), target = data.releases?.find(r => r.id === rollback);
  const configID = rollback ? target?.config_id : instance.draft_config_id, config = data.configs?.find(c => c.id === configID), live = data.configs?.find(c => c.id === current?.config_id);
  const changes = configChanges(live, config);
  const fresh = preparation?.revision === instance.revision && preparation.configID === configID;
  async function prepare() {
    if (!configID || disabled) return;
    setChecking(true); setError(''); setPreparation(null);
    try {
      const result = await client.displayPrepare({ tenantId: project.project.tenant_id, projectId: project.project.id }, instance.id, configID, instance.revision);
      setPreparation({ ...result, revision: instance.revision, configID });
    } catch (e) { setError(e instanceof Error ? e.message : '核验失败'); }
    finally { setChecking(false); }
  }
  if (review || rollback) return <div className="space-y-5">
    {rollback && <Button variant="ghost" size="sm" onClick={() => { setRollback(''); setPreparation(null); setError(''); }}><ArrowLeft aria-hidden="true" />返回发布记录</Button>}
    <div><h3 className="font-semibold">{rollback ? '回退版本' : '发布更改'}</h3><p className="mt-1 text-sm text-muted-foreground">{changes.length ? `本次更改：${changes.join('、')}` : current ? '配置与线上版本一致' : '首次发布'}</p></div>
    <div className="grid gap-4 sm:grid-cols-2"><section className="space-y-4 rounded-xl border p-5"><h4 className="text-sm font-semibold">线上版本</h4>{live ? <ConfigSnapshot config={live} data={data} /> : <p className="text-sm text-muted-foreground">暂无线上版本</p>}</section><section className="space-y-4 rounded-xl border bg-muted/25 p-5"><h4 className="text-sm font-semibold">{rollback ? '回退至此配置' : '待发布草稿'}</h4>{config ? <ConfigSnapshot config={config} data={data} /> : <p className="text-sm text-muted-foreground">{configID ? "配置详情不可用" : "暂无可发布草稿"}</p>}</section></div>
    <FormNotice error>{error}</FormNotice>
    {preparation && (fresh ? <div className="space-y-2 rounded-lg border bg-muted/40 p-4"><p className="flex items-center gap-2 text-sm font-medium"><Check className="size-4" aria-hidden="true" />核验通过</p><p className="text-xs text-muted-foreground">读取授权修订 {preparation.grant_revision} · {Object.values(preparation.binding_sets).reduce((sum, ids) => sum + ids.length, 0)} 个绑定对象</p>{preparation.read_views?.map(v=><div key={v.name} className="border-t pt-2 text-sm"><p>{v.name} · {v.member_count} 个当前匹配成员 · {v.state}</p><p className="text-xs text-muted-foreground">{v.field_groups.join('、')} · {v.include_future?'未来成员可进入；发布后仍逐次校验':'固定成员'}</p>{v.time_window&&<p className="text-xs">{v.time_window.from??'无额外下限'} 至 {v.time_window.to??'无额外上限'}</p>}{v.default_versions&&<div className="text-xs"><p>项目默认资料版本 · {v.default_versions.total} 份</p>{v.default_versions.items.map(item=><p key={item.asset_id}>{item.name} · {item.state==='set'?`固定版本 ${item.number}`:item.state==='unset'?'尚未设置默认版本':'默认版本不在获准范围'}</p>)}{v.default_versions.has_more&&<p>当前展示 20 份；完整目录可在真实预览中分页核对。</p>}</div>}</div>)}</div> : <FormNotice error>大屏配置或授权已更新，请重新核验。</FormNotice>)}
    {canPublish && <div className="flex justify-end gap-2"><Button variant={fresh ? 'outline' : 'default'} disabled={disabled || !configID} onClick={() => void prepare()}>{checking ? '正在核验…' : preparation ? '重新核验' : '核验版本'}</Button>{fresh && <Button disabled={disabled} onClick={() => { if (!preparation || !fresh) return; void command(rollback ? '/rollback' : '/publish', rollback ? { release_id: rollback, preparation: preparation.hash } : { config_id: preparation.configID, preparation: preparation.hash }, 'POST', preparation.revision).then(() => { setPreparation(null); setRollback(''); onPublished(); }).catch(e => { setPreparation(null); setError(e instanceof Error ? e.message : '发布失败'); }); }}>{rollback ? '确认回退' : '确认发布'}</Button>}</div>}
  </div>;
  return <div className="space-y-4">{!data.releases?.length && <EmptyState title="暂无发布记录" />}{data.releases?.map(release => {
    const pack = data.packages?.find(p => p.id === release.package_id);
    return <section className="space-y-4 rounded-xl border p-5" key={release.id}>
      <div className="flex flex-wrap items-center justify-between gap-2"><p className="break-words text-sm font-medium">{pack ? `${pack.manifest.name ?? pack.manifest.app_key} · ${pack.manifest.version}` : '程序详情不可用'}</p><Badge variant="outline">{release.withdrawn ? '已撤回' : instance.current_release_id === release.id ? '当前发布' : '历史发布'}</Badge></div>
      <p className="text-xs text-muted-foreground">{displayTime(release.created_at)}</p>
      {canPublish && <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={disabled || release.withdrawn || instance.current_release_id === release.id} onClick={() => { setRollback(release.id); setPreparation(null); setError(''); }}>回退至此版本</Button><ConfirmDisplayAction label="撤回发布" description="撤回后，此发布版本将无法继续运行，也不能用于回退。" disabled={busy || release.withdrawn} action={() => command('/withdraw', { release_id: release.id })} /></div>}
      <details><summary className="cursor-pointer text-xs text-muted-foreground">发布编号</summary><p className="mt-2 break-all font-mono text-xs">{release.id}</p></details>
    </section>;
  })}</div>;
}
