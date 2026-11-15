import type { AuditFilter } from '@infrapro/api-client';

export const filterLabels: Partial<Record<keyof AuditFilter, string>> = {
  q: '搜索', outcome: '操作结果', category: '事件分类', from: '开始时间', to: '结束时间',
  event_type: '事件类型', object_type: '对象类型', object_id: '对象 ID', actor_id: '操作者 ID',
  reference_id: '关联对象 ID', operation_id: '操作链 ID', request_id: '排障请求 ID',
  task_module: '任务来源模块', task_kind: '任务来源类型', task_id: '来源任务 ID', generation: '任务代次',
};
export const categories = { security: '安全事件', management: '管理操作', business: '业务操作', task: '任务处理', file: '文件操作', maintenance: '维护操作' };
const day = 86400000;
export function recentFilter(value: AuditFilter, days: number, now = Date.now()): AuditFilter {
  return { ...value, from: new Date(now - days * day).toISOString(), to: new Date(now).toISOString(), cursor: undefined };
}
export function rangePreset(value: AuditFilter): string {
  if (!value.from && !value.to) return '7';
  if (!value.from || !value.to) return 'custom';
  const span = Date.parse(value.to) - Date.parse(value.from);
  return Math.abs(Date.now() - Date.parse(value.to)) < 600000 ? String([1, 7, 30].find(days => Math.abs(span - days * day) < 1000) ?? 'custom') : 'custom';
}
/** datetime-local is explicitly Shanghai time, independent of the browser's time zone. */
export function shanghaiInput(value?: string): string {
  if (!value || !Number.isFinite(Date.parse(value))) return '';
  return new Date(Date.parse(value) + 8 * 3600000).toISOString().slice(0, 19);
}
export function shanghaiISO(value: string): string | undefined {
  return value ? new Date(`${value}+08:00`).toISOString() : undefined;
}
export function validateFilter(value: AuditFilter, now = Date.now()): string | undefined {
  const from = value.from ? Date.parse(value.from) : now - 7 * day;
  const to = value.to ? Date.parse(value.to) : now;
  if (!Number.isFinite(from) || !Number.isFinite(to)) return '请输入有效的开始和结束时间。';
  if (from >= to) return '结束时间必须晚于开始时间。';
  if (to - from > 366 * day) return '单次查询时间跨度不能超过 366 天。';
  for (const key of ['actor_id', 'object_id', 'reference_id', 'operation_id', 'task_id'] as const) {
    if (value[key] && !/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(value[key])) return `${filterLabels[key]} 必须是完整的 UUID。`;
  }
  if (value.task_id && (!value.task_module || !value.task_kind || !value.generation)) return '筛选来源任务时，请同时填写任务来源模块、类型和代次。';
  if (value.generation && (!/^[1-9][0-9]{0,18}$/.test(value.generation) || BigInt(value.generation) > 9223372036854775807n)) return '任务代次必须是有效的正整数。';
}
