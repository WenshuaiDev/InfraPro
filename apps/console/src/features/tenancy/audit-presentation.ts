import type { TenantAuditEvent } from '@infrapro/api-client';
import { actionLabel, auditChanges, auditExplanation, record } from '../access/audit-presentation';
import { tenantActions } from './labels';

// Business-only events belong to the tenant filter, not the platform audit menu.
export const tenantAuditActions: Record<string, string> = {
  ...tenantActions,
  'tenant.project.governance': '项目治理',
  'resource.create': '新增工程资源', 'resource.update': '修改工程资源', 'resource.retire': '退役工程资源', 'resource.restore': '恢复工程资源', 'resource.delete': '删除工程资源',
  'resource_access.policy.updated': '调整项目资源策略', 'resource_access.link.created': '关联项目设施', 'resource_access.link.revoked': '撤销项目设施关联', 'resource_access.grants.changed': '调整项目资源授权',
};

export function tenantAuditAction(event: TenantAuditEvent) {
  const label = actionLabel(event);
  return label === '其他操作' ? tenantAuditActions[event.event_type] ?? label : label;
}

const text = (value: unknown) => typeof value === 'string' ? value : '';
const formatter = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
});

export function tenantAuditTime(value: string | Date) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return { day: '时间未记录', time: '' };
  const parts = Object.fromEntries(formatter.formatToParts(date).map((part) => [part.type, part.value]));
  return { day: `${parts.year}-${parts.month}-${parts.day}`, time: `${parts.hour}:${parts.minute}:${parts.second}` };
}

export const tenantAuditISO = (value: string) => value ? new Date(`${value}+08:00`).toISOString() : '';

export function tenantAuditRange(range: string) {
  if (!range) return { range: '', from: '', to: '' };
  const now = new Date();
  const start = range === 'today' ? new Date(`${tenantAuditTime(now).day}T00:00:00+08:00`) : new Date(now.getTime() - (range === 'week' ? 7 : 30) * 86_400_000);
  const time = tenantAuditTime(start);
  return { range, from: `${time.day}T${time.time.slice(0, 5)}`, to: '' };
}

export function tenantAuditActor(event: TenantAuditEvent) {
  if (event.actor_type === 'maintenance') return { name: '主机维护人员', caption: '维护命令' };
  if (event.actor_type === 'release') return { name: '系统发布任务', caption: '自动执行' };
  if (event.actor_type === 'platform' && !event.actor_id) return { name: '平台管理员', caption: '平台操作' };
  const actor = record(event.details.actor);
  const name = text(actor.display_name) || text(actor.username);
  return { name: name || (event.actor_id ? '账号名称未记录' : '操作者未记录'), caption: text(actor.username) ? `@${text(actor.username)}` : name ? '历史名称' : '详见技术信息' };
}

export function tenantAuditScope(event: TenantAuditEvent) {
  if (event.actor_scope === 'tenant') return '租户操作';
  if (event.actor_scope === 'platform') return '平台操作';
  if (['maintenance', 'release'].includes(event.actor_type)) return '系统操作';
  return '来源未记录';
}

function summaryOnly(event: TenantAuditEvent) {
  return event.details.before == null && event.details.after != null && !/\.(create|created|copy|copied|joined)$/.test(event.event_type);
}

export const tenantAuditChanges = (event: TenantAuditEvent) => summaryOnly(event) ? [] : auditChanges(event);
export const tenantAuditExplanation = (event: TenantAuditEvent) => summaryOnly(event) ? '这条记录仅保留操作摘要，没有可还原的前后差异；可在技术信息中查看原始内容。' : auditExplanation(event);

export function tenantAuditSummary(event: TenantAuditEvent) {
  const changes = tenantAuditChanges(event);
  const admin = changes.find((change) => change.label === '租户管理员');
  if (admin?.after.length) return `管理员${admin.before.length ? '交接为' : '指定为'} ${admin.after.join('、')}`;
  const status = changes.find((change) => ['状态', '成员状态', '租户状态'].includes(change.label));
  if (status?.before.length && status.after.length) return `${status.before.join('、')} → ${status.after.join('、')}`;
  const set = changes.find((change) => change.added);
  if (set) return [set.added?.length ? `新增 ${set.added.length} 项${set.label}` : '', set.removed?.length ? `移除 ${set.removed.length} 项${set.label}` : ''].filter(Boolean).join('，');
  const narratives: Record<string, string> = {
    'resource.create': '创建了一项工程资源。', 'resource.update': '更新了工程资源资料。', 'resource.retire': '工程资源已退役，历史记录继续保留。', 'resource.restore': '恢复了工程资源。', 'resource.delete': '删除了工程资源，历史记录继续保留。',
    'resource_access.policy.updated': '更新了项目的资源访问策略。', 'resource_access.link.created': '建立了项目与设施的关联。', 'resource_access.link.revoked': '撤销了项目与设施的关联。', 'resource_access.grants.changed': '更新了项目的资源授权范围。',
    'tenant.project.governance': '记录项目创建、管理员治理或管理异常。',
  };
  if (narratives[event.event_type]) {
    const after = record(event.details.after), before = record(event.details.before);
    const name = text(after.name) || text(before.name);
    return name && event.event_type.startsWith('resource.') ? `${name} · ${narratives[event.event_type]}` : narratives[event.event_type];
  }
  if (changes.length) return `涉及${changes.slice(0, 3).map((change) => change.label).join('、')}${changes.length > 3 ? `等 ${changes.length} 项内容` : ''}`;
  return '查看详情了解操作原因与历史内容。';
}
