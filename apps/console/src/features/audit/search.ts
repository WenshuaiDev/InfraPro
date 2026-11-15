import type { AuditFilter, AuditScope } from '@infrapro/api-client';
export type AuditSearch = AuditFilter & { event?: string; export_id?: string; execution?: string; tab?: 'exports' };
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
export function auditSearch(raw: Record<string, unknown>): AuditSearch {
 const out: Record<string, string | number> = {};
 for (const key of ['q','event_type','object_type','task_module','task_kind','generation','from','to','category','outcome','request_id']) if (typeof raw[key] === 'string' && raw[key]) out[key] = raw[key].slice(0,200);
 for (const key of ['reference_id','export_id','actor_id','object_id','operation_id','task_id']) if (typeof raw[key] === 'string' && uuid.test(raw[key])) out[key]=raw[key];
 if (typeof raw.cursor === 'string') out.cursor=raw.cursor.slice(0,4096);
 if (typeof raw.execution === 'string' && /^[1-9][0-9]{0,18}$/.test(raw.execution) && BigInt(raw.execution)<=9223372036854775807n) out.execution=raw.execution;
 if ([10,25,50,100].includes(Number(raw.limit))) out.limit=Number(raw.limit);
 if (typeof raw.event === 'string' && /^[1-9][0-9]{0,18}$/.test(raw.event)) out.event=raw.event;
 if (raw.tab === 'exports') out.tab='exports';
 return out as AuditSearch;
}
export function auditPath(scope: AuditScope) { return scope.kind === 'platform' ? '/access/audit' : `/t/${scope.tenant_id}${scope.kind === 'project' ? `/p/${scope.project_id}` : ''}/audit`; }
export function auditFilter(search:AuditSearch):AuditFilter { const filter={...search};delete filter.event;delete filter.export_id;delete filter.execution;delete filter.tab;return filter;}
