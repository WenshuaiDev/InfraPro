import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { AuditScope } from '@infrapro/api-client';
import { History, ScrollText } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ChangeList } from './change-list';
import { Outcome } from './event-table';
import { OperationTrace, AuditLinks } from './operation-trace';
import { TechnicalDetail } from './technical-detail';
import { date, nameSources, phases } from './presentation';

export function EventDetail({ scope, id, queryKey, close, history }: {
  scope: AuditScope; id: string; queryKey: readonly unknown[]; close: () => void; history: (objectType: string, objectId: string) => void;
}) {
  const { client } = useIdentity();
  const q = useQuery({ queryKey: [...queryKey, 'event', id], queryFn: ({ signal }) => client.auditEvent(scope, id, signal), retry: false, refetchOnWindowFocus: 'always' });
  const e = q.error || q.data && q.data.authorization_revision !== queryKey.at(-3) ? undefined : q.data;
  return <ResponsivePanel open onOpenChange={open => { if (!open) close(); }} title="操作记录详情" description="核实历史事实、实际变化与后续处理。" fallbackFocusSelector={`[data-audit-record="${id}"]`} size="wide"
    footer={<><Button variant="outline" onClick={close}>关闭</Button>{e?.object.id && /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(e.object.id) && e.object.type && <Button onClick={() => history(e.object.type, e.object.id)}><History aria-hidden="true" />查看对象历史</Button>}</>}>
    <div className="space-y-6">{q.error && <ErrorState message={q.error.message} onRetry={() => void q.refetch()} />}{q.isPending && <p role="status">正在核实记录…</p>}{q.data && !e && !q.error && <p role="status">授权已变化，正在重新核验；旧记录已隐藏。</p>}
      {e && <><div className="flex items-start gap-3"><span className="flex size-11 shrink-0 items-center justify-center rounded-lg border bg-muted/30 text-muted-foreground"><ScrollText aria-hidden="true" className="size-5" /></span><div className="min-w-0 space-y-2"><h2 className="break-words text-lg font-semibold">{e.action_label}</h2><div className="flex flex-wrap items-center gap-2"><Outcome value={e.outcome} /><span className="text-xs text-muted-foreground">{phases[e.phase] ?? e.phase}</span></div></div></div>
        <dl className="grid gap-x-6 gap-y-4 text-sm sm:grid-cols-2">{[['对象', e.object.label || '名称未记录或受限'], ['操作者', e.actor.name || '未记录'], ['发生时间', date(e.occurred_at)], ['记录时间', date(e.recorded_at)], ['名称来源', nameSources[e.object.label_source] ?? e.object.label_source], ['所属空间', scope.kind === 'platform' ? '当前平台' : scope.kind === 'tenant' ? '当前租户' : '当前项目'], ['原发起人', e.initiator.name || '未记录'], ['本次请求人', e.requested_by.name || '未记录']].map(([label, value]) => <div key={label} className="min-w-0"><dt className="mb-1.5 text-xs text-muted-foreground">{label}</dt><dd className="break-words">{value}</dd></div>)}</dl>
        <div className="rounded-lg bg-muted/40 p-3"><p className="mb-1 text-xs text-muted-foreground">操作原因</p><p className="whitespace-pre-wrap break-words text-sm leading-6">{e.reason || '未记录操作原因'}</p></div>
        {e.evidence_quality !== 'native' && <p className="rounded-md border p-3 text-xs leading-5 text-muted-foreground">历史兼容记录：缺失字段不会用当前状态补造。{e.missing_fields.length > 0 && `未记录：${e.missing_fields.join('、')}`}</p>}
        <Tabs defaultValue="overview" className="gap-5"><TabsList aria-label="记录详情视图" className="h-auto w-full justify-start gap-4 rounded-none border-b bg-transparent p-0"><TabsTrigger value="overview" className={tabClass}>记录详情</TabsTrigger>{e.actions.trace && <TabsTrigger value="trace" className={tabClass}>操作追踪</TabsTrigger>}{e.technical && <TabsTrigger value="technical" className={tabClass}>技术信息</TabsTrigger>}</TabsList>
          <TabsContent value="overview" className="space-y-6"><ChangeList changes={e.changes} /><section className="space-y-3"><h3 className="text-sm font-semibold">关联来源</h3><AuditLinks scope={scope} event={id} links={e.links} /></section></TabsContent>
          {e.actions.trace && <TabsContent value="trace"><OperationTrace scope={scope} event={id} queryKey={queryKey} /></TabsContent>}
          {e.technical && <TabsContent value="technical"><TechnicalDetail value={e.technical} expanded /></TabsContent>}
        </Tabs>
      </>}
    </div>
  </ResponsivePanel>;
}
const tabClass = 'h-10 flex-none rounded-none border-x-0 border-t-0 border-b-2 border-transparent px-0 text-xs text-muted-foreground data-[state=active]:border-foreground data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:border-foreground dark:data-[state=active]:bg-transparent';
