import { useState, type RefObject } from 'react';
import type { TenantAuditEvent } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Switch } from '@infrapro/ui/components/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { AuditChangeCard } from '../access/audit-panel';
import { tenantAuditAction as actionLabel } from './audit-presentation';
import { AuditRawContent } from '../access/audit-raw-content';
import { tenantAuditActor, tenantAuditChanges, tenantAuditExplanation, tenantAuditScope, tenantAuditSummary, tenantAuditTime } from './audit-presentation';

export function TenantAuditPanel({ event, tenantName, close, returnFocus, initialTab = 'details' }: {
  event: TenantAuditEvent; tenantName: string; close: () => void; returnFocus: RefObject<HTMLElement | null>; initialTab?: string;
}) {
  const [full, setFull] = useState(false), [raw, setRaw] = useState(false);
  const changes = tenantAuditChanges(event), time = tenantAuditTime(event.created_at), actor = tenantAuditActor(event);
  const info = [['发生时间', `${time.day} ${time.time}（上海）`], ['操作者', actor.name], ['所属租户', tenantName]];
  const technical = [['记录编号', event.id], ['事件编码', event.event_type], ['操作者类型', event.actor_type], ['操作者 ID', event.actor_id], ['操作来源', event.actor_scope], ['租户 ID', event.tenant_id], ['请求 ID', event.request_id]];
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title={actionLabel(event)} description={`${time.day} ${time.time}（上海） · ${tenantName}`} returnFocus={returnFocus} fallbackFocusSelector="[data-tenant-audit-search]">
    <Tabs defaultValue={initialTab} className="gap-5">
      <TabsList className="w-full"><TabsTrigger value="details" className="flex-1 max-sm:min-h-11">操作详情</TabsTrigger><TabsTrigger value="technical" className="flex-1 max-sm:min-h-11">技术信息</TabsTrigger></TabsList>
      <TabsContent value="details" className="space-y-6">
        <div className="space-y-3 rounded-xl border bg-muted/20 p-4"><Badge variant="outline">{tenantAuditScope(event)}</Badge><p className="text-sm leading-6">{tenantAuditSummary(event)}</p></div>
        <dl className="space-y-4 text-sm">{info.map(([label, value]) => <div key={label} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-3"><dt className="text-muted-foreground">{label}</dt><dd className="break-words">{value}{label === '操作者' && <span className="mt-1 block text-xs text-muted-foreground">{actor.caption}</span>}</dd></div>)}</dl>
        <section className="space-y-2"><h3 className="text-sm font-semibold">操作原因</h3><p className="whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">{event.reason || '这条记录未填写原因。'}</p></section>
        <section className="space-y-3"><h3 className="text-sm font-semibold">变更内容</h3>
          {changes.some((change) => change.added) && <label className="flex min-h-11 items-center gap-3 text-sm"><Switch checked={full} onCheckedChange={setFull} />显示完整前后内容</label>}
          {changes.length ? changes.map((change) => <AuditChangeCard key={change.label} change={change} full={full} />) : <p className="rounded-lg border border-dashed p-4 text-sm leading-6 text-muted-foreground">{tenantAuditExplanation(event)}</p>}
        </section>
      </TabsContent>
      <TabsContent value="technical" className="space-y-5">
        <p className="text-sm leading-6 text-muted-foreground">用于核对历史快照与定位日志。请求 ID 可与服务端日志对应。</p>
        <dl className="space-y-4 text-sm">{technical.map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-muted-foreground">{label}</dt><dd className="break-all font-mono text-xs leading-5">{value || '未记录'}</dd></div>)}</dl>
        <label className="flex min-h-11 items-center gap-3 text-sm"><Switch checked={raw} onCheckedChange={setRaw} />显示原始内容</label>
        {raw && <AuditRawContent value={JSON.stringify(event.details, null, 2)} />}
      </TabsContent>
    </Tabs>
  </ResponsivePanel>;
}
