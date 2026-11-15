import { useQuery } from '@tanstack/react-query';
import { Permissions as P, type MonitoringImportBatch as Batch, type AuditScope } from '@infrapro/api-client';
import { useCursorList } from '../../hooks/use-cursor-list';
import { ObjectHistory } from '../audit/object-history';
import { outcomes, phases } from '../audit/presentation';
import { TaskLink } from '../tasks/common';
import { useMonitoring } from './context';
import { CursorButtons, MonitoringError } from './common';
import { importDate } from './import-search';
import { ImportSectionTitle } from './import-overview';

export function ImportHistory({ batch }: { batch: Batch }) {
  const m = useMonitoring();
  const scope: AuditScope = m.scope.kind === 'tenant' ? { kind: 'tenant', tenant_id: m.scope.tenantId } : { kind: 'project', tenant_id: m.scope.tenantId, project_id: m.scope.projectId };
  const allowed = !batch.scope_clipped && m.available && m.can(m.scope.kind === 'tenant' ? P.TenantAuditRead : P.ProjectAuditRead);
  const caps = useQuery({ queryKey: [...m.prefix, 'import-audit-capabilities', m.authorizationEpoch], queryFn: ({ signal }) => m.client.auditCapabilities(scope, signal), enabled: allowed, retry: false, refetchInterval: 30000, refetchOnWindowFocus: 'always' });
  const ready = allowed && !caps.error && !!caps.data?.read;
  const records = useCursorList({ queryKey: [...m.prefix, 'import-history', batch.id, batch.revision, m.authorizationEpoch, caps.data?.authorization_revision, caps.data?.epoch, caps.data?.projection_version], load: (cursor, signal) => m.client.auditEvents(scope, { object_type: 'monitoring_batch', object_id: batch.id, limit: 10, cursor }, signal), enabled: ready });
  if (!allowed) return <p className="text-sm text-muted-foreground">当前范围没有完整批次处理记录的查看权限。</p>;
  return <section className="space-y-5"><ImportSectionTitle action={<ObjectHistory objectType="monitoring_batch" objectId={batch.id} label="查看完整审计" />}>批次处理记录</ImportSectionTitle><MonitoringError error={caps.error} retry={() => void caps.refetch()} />{!caps.error && !caps.isPending && !caps.data?.read && <p className="text-sm text-muted-foreground">当前授权不允许读取处理记录。</p>}{caps.isPending || ready && records.isPending ? <p role="status" className="text-sm">正在读取处理记录…</p> : null}{ready && <><MonitoringError error={records.error} retry={() => void records.refetch()} />{!records.error && <><ol className="ml-2 space-y-6 border-l pl-6">{records.items.map(event => <li key={event.id} className="relative"><span className="absolute -left-[29px] top-1.5 size-2.5 rounded-full border-2 border-background bg-primary" /><div className="flex flex-wrap items-start justify-between gap-2"><h4 className="text-sm font-medium">{event.action_label}</h4><span className="text-xs text-muted-foreground">{outcomes[event.outcome] ?? event.outcome}</span></div><p className="mt-1 text-xs text-muted-foreground">{importDate(event.occurred_at, true)} · {event.actor.name || '操作者未记录'}</p><p className="mt-2 break-words text-xs leading-6 text-muted-foreground">{phases[event.phase] ?? event.phase}{event.reason ? ` · ${event.reason}` : ''}</p></li>)}</ol>{!records.isPending && !records.items.length && <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">当前授权范围内没有可见的处理记录。</p>}<CursorButtons previous={records.page ? () => records.go(records.page - 1) : undefined} next={records.hasNext ? () => void records.next() : undefined} busy={records.isFetching} caption={`记录第 ${records.page + 1} 页`} /></>}</>}{batch.job_id && <TaskLink source={{ source_module: 'monitoring', source_kind: 'job', source_root_id: batch.job_id }} />}</section>;
}
