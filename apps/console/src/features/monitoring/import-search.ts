import type { MonitoringImportBatch, MonitoringImportFilter } from '@infrapro/api-client';

export const importStates: MonitoringImportBatch['state'][] = ['uploading', 'received', 'validating', 'needs_resolution', 'ready', 'preparing', 'published', 'completed_noop', 'failed', 'cancelled', 'revoked'];
export const importTabs = ['overview', 'issues', 'mapping', 'history'] as const;
export type ImportTab = typeof importTabs[number];
export type ImportSearch = MonitoringImportFilter & { tab?: ImportTab };

export function importSearch(value: Record<string, unknown>): ImportSearch {
  const multiple = (key: string, allowed: string[]) => typeof value[key] === 'string' ? [...new Set(value[key].split(',').filter(v => allowed.includes(v)))].sort().join(',') || undefined : undefined;
  const limit = Number(value.limit);
  return {
    q: typeof value.q === 'string' ? [...value.q.replace(/[\r\n\0]/g, '').trim()].slice(0, 200).join('') || undefined : undefined,
    states: multiple('states', importStates), formats: multiple('formats', ['csv', 'xlsx']),
    sort: value.sort === 'oldest' ? 'oldest' : undefined,
    limit: ([10, 20, 25, 50, 100] as const).find(value => value === limit) ?? 10,
    cursor: typeof value.cursor === 'string' ? value.cursor.slice(0, 2048) || undefined : undefined,
    tab: importTabs.includes(value.tab as ImportTab) ? value.tab as ImportTab : undefined,
  };
}
export function importFilter({ q, states, formats, sort, limit, cursor }: ImportSearch): MonitoringImportFilter { return { q, states, formats, sort, limit, cursor }; }

export function batchCapabilities(batch: MonitoringImportBatch, canManage: boolean, canRevoke: boolean, canSource: boolean, available: boolean) {
  const writable = available && !batch.scope_clipped;
  const editable = ['received', 'needs_resolution', 'ready', 'failed'].includes(batch.state);
  return {
    upload: writable && canManage && batch.state === 'uploading',
    mapping: writable && canManage && editable && !batch.readopt_batch_id,
    validate: writable && canManage && editable && batch.mapping_revision !== '0',
    publish: writable && canManage && batch.state === 'ready',
    cancel: writable && canManage && !['published', 'revoked', 'completed_noop', 'cancelled'].includes(batch.state),
    revoke: writable && canRevoke && batch.state === 'published',
    readopt: writable && canManage && batch.state === 'revoked',
    resolve: writable && canManage && batch.state === 'needs_resolution' && batch.counts?.errors === '0' && !!batch.counts && batch.counts.duplicates !== '0' && !!batch.validation_hash,
    source: available && !batch.scope_clipped && canSource && !!batch.source_file_id,
  };
}

export function importDate(value: string, full = false) { return new Date(value).toLocaleString('zh-CN', { ...(full ? { year: 'numeric' as const } : {}), month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Shanghai', hour12: false }); }
export const importFileType = (batch: MonitoringImportBatch) => batch.scope_clipped ? undefined : batch.name.toLowerCase().endsWith('.xlsx') ? 'XLSX' : 'CSV';
export function importFileSize(value?: string) { return value ? Number(value) < 1024 ? `${value} B` : Number(value) < 1048576 ? `${(Number(value) / 1024).toFixed(1)} KB` : `${(Number(value) / 1048576).toFixed(1)} MB` : '—'; }
