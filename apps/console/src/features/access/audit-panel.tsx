import { useState, type RefObject } from 'react';
import type { AccessAuditEvent } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Switch } from '@infrapro/ui/components/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { actionLabel, actorLabel, auditChanges, auditExplanation, auditSummary, auditTime, objectLabel, outcomeLabel, scopeLabel, targetLabel, type AuditChange } from './audit-presentation';
import { AuditRawContent } from './audit-raw-content';

export function AuditOutcome({ event }: { event: AccessAuditEvent }) {
  return <Badge variant={event.display?.outcome === 'failure' ? 'destructive' : event.display?.outcome === 'success' ? 'secondary' : 'outline'} className="whitespace-nowrap font-normal">{outcomeLabel(event)}</Badge>;
}

function Values({ values, empty = '未设置' }: { values: string[]; empty?: string }) {
  return values.length ? <ul className="space-y-1.5">{values.map((value, index) => <li key={`${index}:${value}`} className="break-words whitespace-pre-wrap">{value}</li>)}</ul> : <span className="text-muted-foreground">{empty}</span>;
}
function Change({ change, full }: { change: AuditChange; full: boolean }) {
  return <section aria-label={`${change.label}变化`} className="overflow-hidden rounded-lg border">
    <h4 className="border-b bg-muted/30 px-4 py-2.5 text-sm font-medium">{change.label}</h4>
    {change.added && !full ? <div className="space-y-4 p-4 text-sm">
      {!!change.added.length && <div><p className="mb-2 font-medium">新增 {change.added.length} 项</p><Values values={change.added} /></div>}
      {!!change.removed?.length && <div><p className="mb-2 font-medium">移除 {change.removed.length} 项</p><Values values={change.removed} /></div>}
    </div> : <div className="grid divide-y text-sm sm:grid-cols-2 sm:divide-x sm:divide-y-0">
      <div className="min-w-0 p-4"><p className="mb-2 text-xs text-muted-foreground">操作前</p><Values values={change.before} empty={change.added ? '无' : '未设置'} /></div>
      <div className="min-w-0 p-4"><p className="mb-2 text-xs text-muted-foreground">操作后</p><Values values={change.after} empty={change.added ? '无' : '未设置'} /></div>
    </div>}
  </section>;
}

export function AuditPanel({ event, close, returnFocus, initialTab = 'details' }: { event: AccessAuditEvent; close: () => void; returnFocus: RefObject<HTMLElement | null>; initialTab?: string }) {
  const [full, setFull] = useState(false), [raw, setRaw] = useState(false);
  const changes = auditChanges(event), time = auditTime(event.created_at);
  const display = event.display;
  const info = [
    ['发生时间', `${time.day} ${time.time}`],
    ['操作者', actorLabel(event) + (display?.actor_username && display.actor_username !== display.actor_name ? `（${display.actor_username}）` : '')],
    ['影响对象', `${objectLabel(event)} · ${targetLabel(event)}`],
    ['所属范围', scopeLabel(event)],
  ];
  const technical = [
    ['记录编号', event.id], ['事件编码', event.event_type], ['操作者 ID', event.actor_id],
    ['关联账号 ID', event.account_id], ['对象类型', event.object_type], ['对象 ID', event.object_id], ['租户 ID', event.tenant_id],
    ['请求 ID', event.request_id], ['来源 IP', display?.source_ip], ['客户端', display?.user_agent],
  ];
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title={actionLabel(event)} description={`${time.day} ${time.time} · ${scopeLabel(event)}`} returnFocus={returnFocus} fallbackFocusSelector="[data-audit-search]">
    <Tabs defaultValue={initialTab} className="gap-5">
      <TabsList className="w-full"><TabsTrigger value="details" className="flex-1">操作详情</TabsTrigger><TabsTrigger value="technical" className="flex-1">技术信息</TabsTrigger></TabsList>
      <TabsContent value="details" className="space-y-6">
        <div className="space-y-3 rounded-xl border bg-muted/20 p-4"><AuditOutcome event={event} /><p className="text-sm leading-6">{auditSummary(event, changes)}</p></div>
        <dl className="space-y-4 text-sm">{info.map(([label, value]) => <div key={label} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-3"><dt className="text-muted-foreground">{label}</dt><dd className="break-words">{value}</dd></div>)}</dl>
        {(display?.actor_name_source === 'current' || display?.target_name_source === 'current') && <p className="rounded-lg bg-muted/40 p-3 text-xs leading-5 text-muted-foreground">这条记录没有保存{[display.actor_name_source === 'current' ? '操作者' : '', display.target_name_source === 'current' ? '对象' : ''].filter(Boolean).join('及')}的历史名称，以上使用当前名称辅助识别；原始 ID 保留在技术信息中。</p>}
        <section className="space-y-2"><h3 className="text-sm font-semibold">操作原因</h3><p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{event.reason || (event.display?.category === 'security' ? '此类账号安全操作不要求填写原因。' : '这条记录未填写原因。')}</p></section>
        <section className="space-y-3"><h3 className="text-sm font-semibold">{event.display?.category === 'security' ? '事件说明' : '变更内容'}</h3>
          {changes.some((change) => change.added) && <label className="flex min-h-11 items-center gap-3 text-sm"><Switch checked={full} onCheckedChange={setFull} />显示完整前后内容</label>}
          {changes.length ? changes.map((change) => <Change key={change.label} change={change} full={full} />) : <p className="rounded-lg border border-dashed p-4 text-sm leading-6 text-muted-foreground">{auditExplanation(event)}</p>}
          {changes.some((change) => change.label === '权限') && !Array.isArray(event.details.permission_definitions) && <p className="text-xs leading-5 text-muted-foreground">权限名称由当前权限目录解释；具体权限编码保留在技术信息中。</p>}
        </section>
      </TabsContent>
      <TabsContent value="technical" className="space-y-5">
        <p className="text-sm leading-6 text-muted-foreground">用于定位日志、核对原始对象和排查问题。请求 ID 可与服务端日志对应。</p>
        <dl className="space-y-4 text-sm">{technical.map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-muted-foreground">{label}</dt><dd className="break-all font-mono text-xs leading-5">{value || '未记录'}</dd></div>)}</dl>
        <label className="flex min-h-11 items-center gap-3 text-sm"><Switch checked={raw} onCheckedChange={setRaw} />显示原始内容</label>
        {raw && <AuditRawContent value={JSON.stringify(event.details, null, 2)} />}
      </TabsContent>
    </Tabs>
  </ResponsivePanel>;
}
