import type { MonitoringScope, TaskFilter } from '@infrapro/api-client';
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
export type TaskSearch = TaskFilter & { automatic?: string; operations?: 'open'; run?: string; execution?: string; back?: string; action?: 'cancel' | 'retry' };
export function workspaceTaskFilter(scope: MonitoringScope): TaskFilter {
  return { mode: 'mine', scope_kind: scope.kind, tenant_id: scope.tenantId, project_id: scope.kind === 'project' ? scope.projectId : undefined };
}
export function taskSearch(search: Record<string, unknown>): TaskSearch {
  const text = (key: string, max = 100) => typeof search[key] === 'string' ? search[key].slice(0, max) : undefined;
  const tenant = text('tenant_id'), project = text('project_id'), run = text('run');
  const execution = String(search.execution ?? '');
  const limit = Number(search.limit);
  return {
    scope_kind: ['platform', 'tenant', 'project'].includes(String(search.scope_kind)) ? search.scope_kind as TaskFilter['scope_kind'] : undefined,
    limit: [10, 25, 50, 100].includes(limit) ? limit : undefined,
    source: text('source'), mode: search.mode === 'managed' ? 'managed' : 'mine',
    tenant_id: tenant && uuid.test(tenant) ? tenant : undefined,
    project_id: tenant && uuid.test(tenant) && project && uuid.test(project) ? project : undefined,
    type: text('type', 3200), group: search.group === 'active' || search.group === 'attention' || search.group === 'ended' ? search.group : undefined,
    sort: search.sort === 'oldest' ? 'oldest' : undefined,
    q: text('q'), from: text('from'), to: text('to'), cursor: text('cursor', 4096),
    execution: execution && /^[1-9][0-9]*$/.test(execution) ? execution : undefined,
    action: search.action === 'cancel' || search.action === 'retry' ? search.action : undefined,
    automatic: typeof search.automatic === 'string' && search.automatic.split('/').length <= 2 && search.automatic.split('/').every(id => uuid.test(id)) ? search.automatic : undefined,
    operations: search.operations === 'open' ? 'open' : undefined,
    run: run && uuid.test(run) ? run : undefined,
    back: text('back', 2048),
  };
}
