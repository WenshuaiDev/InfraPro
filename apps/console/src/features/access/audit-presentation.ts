import type { AccessAuditEvent } from '@infrapro/api-client';
import { tenantActions } from '../tenancy/labels';

export const auditActions: Record<string, string> = {
  ...tenantActions,
  login: '登录账号', logout: '退出登录', logout_all: '退出全部会话', session_revoked: '撤销登录会话',
  reauthentication: '确认本人密码', password_changed: '修改密码', password_reset: '重置密码',
  bootstrap: '初始化管理员', reset_link_created: '生成密码重置链接', backup_access_revoked: '撤销恢复前的访问凭据',
  account_created: '开通账号', invitation_created: '创建开户邀请', invitation_revoked: '撤销开户邀请', invitation_accepted: '接受开户邀请',
  role_created: '创建角色', RoleUpdate: '编辑角色资料', RoleSetPermissions: '配置角色权限', RoleSetStatus: '调整角色状态', RoleDelete: '删除角色',
  account_roles_changed: '配置账号角色', account_authorization_changed: '账号有效授权变化',
  AccountDisable: '停用账号', AccountEnable: '恢复账号', catalog_synced: '同步权限目录', initial_role_bound: '开户绑定角色',
  admin_recovered: '恢复管理员角色', account_disabled: '维护停用账号', account_enabled: '维护恢复账号', account_emergency_disabled: '应急停用账号',
};
export const auditCategories = [{ value: '', label: '全部记录' }, { value: 'management', label: '管理变更' }, { value: 'security', label: '登录与安全' }, { value: 'system', label: '系统维护' }];
export const auditOutcomes = [{ value: '', label: '全部结果' }, { value: 'success', label: '成功' }, { value: 'failure', label: '失败' }, { value: 'rate_limited', label: '已限流' }];
const objectLabels: Record<string, string> = { engineering_definition: '工程定义', account: '账号', role: '平台角色', tenant_role: '租户角色', tenant: '租户', tenant_member: '租户成员', tenant_invitation: '成员邀请', invitation: '开户邀请', session: '账号会话', password_reset: '账号密码', catalog: '权限目录' };
const securityEvents = new Set(['login', 'logout', 'logout_all', 'session_revoked', 'reauthentication', 'password_changed', 'password_reset']);
const systemEvents = new Set(['bootstrap', 'reset_link_created', 'backup_access_revoked', 'catalog_synced', 'admin_recovered', 'account_disabled', 'account_enabled', 'account_emergency_disabled', 'tenant.authorization.preset_synced']);
export const eventCategory = (event: string) => systemEvents.has(event) ? 'system' : securityEvents.has(event) ? 'security' : 'management';
export const objectLabel = (event: AccessAuditEvent) => objectLabels[event.object_type] ?? '其他对象';
export const outcomeLabel = (event: AccessAuditEvent) => auditOutcomes.find((item) => item.value && item.value === event.display?.outcome)?.label ?? '未记录';
export const scopeLabel = (event: AccessAuditEvent) => event.display?.tenant_name || (event.tenant_id ? '租户名称未记录' : '平台');

export function record(value: unknown): Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function text(value: unknown) { return typeof value === 'string' ? value : ''; }
function snapshot(value: unknown) { const item = record(value); return record(item.tenant ?? item.role ?? value); }
function person(value: unknown) { const item = record(value); return text(item.display_name) || text(item.username); }

export function actorLabel(event: AccessAuditEvent) {
  if (event.actor_type === 'maintenance') return '主机维护人员';
  if (event.actor_type === 'release') return '系统发布任务';
  if (!event.actor_id) return event.event_type === 'login' ? '未认证的访问者' : '操作者未记录';
  return event.display?.actor_name || person(event.details.actor) || '账号名称未记录';
}
export function targetLabel(event: AccessAuditEvent) {
  if (event.object_type === 'catalog') return '平台权限与预置角色';
  if (event.object_type === 'invitation') return '账号开户邀请';
  if (event.object_type === 'tenant_invitation') return '租户成员邀请';
  return event.display?.target_name || '对象名称未记录';
}
export function actionLabel(event: AccessAuditEvent) {
  const after = snapshot(event.details.after);
  if (event.event_type === 'engineering.state') return after.enabled === true ? '恢复工程版本' : '停用工程版本';
  if (['RoleSetStatus', 'tenant.role.status_changed'].includes(event.event_type) && ['active', 'disabled'].includes(text(after.status))) return after.status === 'active' ? '启用角色' : '停用角色';
  return auditActions[event.event_type] ?? '其他操作';
}
export function auditTime(value: string) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return { day: '时间未记录', time: '' };
  const pad = (n: number) => String(n).padStart(2, '0');
  return { day: `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`, time: `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}` };
}

export type AuditChange = { label: string; before: string[]; after: string[]; added?: string[]; removed?: string[] };
type NamedValue = { id: string; name: string };
function setChange(label: string, before: NamedValue[], after: NamedValue[]): AuditChange | undefined {
  const old = new Set(before.map((value) => value.id)), next = new Set(after.map((value) => value.id));
  const added = after.filter((value) => !old.has(value.id)).map((value) => value.name);
  const removed = before.filter((value) => !next.has(value.id)).map((value) => value.name);
  if (!added.length && !removed.length) return undefined;
  return { label, before: before.map((value) => value.name), after: after.map((value) => value.name), added, removed };
}
function permissions(event: AccessAuditEvent, value: unknown): NamedValue[] {
  const historical = new Map<string, string>();
  if (Array.isArray(event.details.permission_definitions)) for (const entry of event.details.permission_definitions) {
    const item = record(entry); if (text(item.code) && text(item.name)) historical.set(text(item.code), text(item.name));
  }
  return Array.isArray(value) ? value.filter((code): code is string => typeof code === 'string').map((code) => ({ id: code, name: historical.get(code) || event.display?.permission_names[code] || `未收录权限（${code}）` })) : [];
}
function roles(event: AccessAuditEvent, value: unknown): NamedValue[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry) => !record(entry).revoked_at).map((entry) => {
    const item = record(entry), id = text(item.role_id) || text(item.id) || text(entry);
    return { id, name: text(item.name) || event.display?.role_names[id] || '角色名称未记录' };
  });
}
const fieldLabels: Record<string, string> = { name: '名称', description: '说明', code: '租户编号', status: '状态', display_name: '显示名称', username: '用户名', membership_status: '成员状态', account_status: '账号状态', tenant_status: '租户状态', available: '管理员可用性', administrator_available: '管理员可用性', tenant_notice: '租户可见说明', note: '邀请备注', expires_at: '有效期至' };
const valueLabels: Record<string, string> = { active: '启用', disabled: '停用', suspended: '暂停', ended: '已移除', pending: '待接受', accepted: '已接受', revoked: '已撤销', expired: '已过期', invalidated: '已失效' };
function fieldValue(key: string, value: unknown): string[] {
  if (value === undefined || value === null || value === '') return [];
  if (typeof value === 'boolean') return [value ? '可用' : '不可用'];
  if (typeof value === 'string') {
    if (key.endsWith('_status') && value === 'active') return ['正常'];
    if (key === 'expires_at') { const date = auditTime(value); return [`${date.day} ${date.time}`]; }
    return [key === 'status' || key.endsWith('_status') ? valueLabels[value] ?? value : value];
  }
  return [];
}

export function auditChanges(event: AccessAuditEvent): AuditChange[] {
  if (event.display?.outcome && event.display.outcome !== 'success') return [];
  if (['RoleDelete', 'tenant.role.deleted'].includes(event.event_type)) return [{ label: '角色', before: [targetLabel(event)], after: ['已删除'] }];
  if (event.event_type === 'catalog_synced') return [];
  const before = snapshot(event.details.before), after = snapshot(event.details.after);
  const changes: AuditChange[] = [];
  for (const [key, label] of Object.entries(fieldLabels)) {
    if (!(key in before) && !(key in after)) continue;
    const old = fieldValue(key, before[key]), next = fieldValue(key, after[key]);
    if (JSON.stringify(old) !== JSON.stringify(next)) changes.push({ label, before: old, after: next });
  }
  const oldAdmin = person(before.administrator), nextAdmin = person(after.administrator);
  if (oldAdmin !== nextAdmin && (oldAdmin || nextAdmin)) changes.push({ label: '租户管理员', before: oldAdmin ? [oldAdmin] : [], after: nextAdmin ? [nextAdmin] : [] });
  if (typeof event.details.before === 'string' && typeof after.enabled === 'boolean') changes.push({ label: '账号状态', before: [valueLabels[event.details.before] ?? event.details.before], after: [after.enabled ? '启用' : '停用'] });
  const permissionChange = setChange('权限', permissions(event, before.permission_codes ?? before.permissions), permissions(event, after.permission_codes ?? after.permissions));
  if (permissionChange) changes.push(permissionChange);
  const roleChange = setChange('绑定角色', roles(event, Array.isArray(event.details.before) ? event.details.before : before.roles), roles(event, Array.isArray(event.details.after) ? event.details.after : after.roles));
  if (roleChange) changes.push(roleChange);
  return changes;
}

export function auditSummary(event: AccessAuditEvent, changes = auditChanges(event)): string {
  if (event.display?.outcome === 'rate_limited') return '尝试过于频繁，系统已限制本次请求。';
  if (event.display?.outcome === 'failure') return event.event_type === 'login' ? '登录验证未通过，未建立登录会话。' : '验证未通过，本次操作未完成。';
  const outcomeKnown = event.display?.outcome === 'success';
  if (securityEvents.has(event.event_type) && !outcomeKnown) return '历史记录未提供操作结果，请查看技术信息。';
  if (['role_created', 'tenant.role.created', 'tenant.role.copied'].includes(event.event_type)) {
    const codes = snapshot(event.details.after).permission_codes;
    return Array.isArray(codes) ? (codes.length ? `新建角色，配置了 ${codes.length} 项权限。` : '新建角色，尚未配置权限。') : '新建了一个角色。';
  }
  if (['RoleDelete', 'tenant.role.deleted'].includes(event.event_type)) return '该角色已删除，不能再分配。';
  const narratives: Record<string, string> = {
    login: '验证通过，建立了登录会话。', logout: '结束了当前登录会话。', logout_all: '已结束该账号的全部登录会话。', session_revoked: '指定的登录会话已失效。',
    reauthentication: '已确认本人密码，可继续需要复验的操作。', password_changed: '密码已更新，原登录会话已失效。', password_reset: '已完成密码重置。',
    catalog_synced: '同步权限定义和预置角色，供系统校验授权。', bootstrap: '完成系统首次初始化并创建管理员账号。',
    invitation_created: '生成了一个账号开户邀请。', invitation_revoked: '该开户邀请已不能继续使用。', invitation_accepted: '受邀人已通过邀请开通账号。', account_created: '新账号已开通。',
    reset_link_created: '维护人员生成了密码重置凭据。', backup_access_revoked: '恢复前的会话及相关凭据已撤销。',
    account_authorization_changed: '绑定角色的配置发生变化，需要重新计算账号权限。',
  };
  if (narratives[event.event_type]) return narratives[event.event_type]!;
  const set = changes.find((change) => change.label === (event.event_type.includes('roles_changed') || event.event_type === 'initial_role_bound' ? '绑定角色' : '权限'));
  if (set) {
    const unit = set.label === '权限' ? '项权限' : '个角色';
    return [set.added?.length ? `新增 ${set.added.length} ${unit}` : '', set.removed?.length ? `移除 ${set.removed.length} ${unit}` : ''].filter(Boolean).join('，');
  }
  const status = changes.find((change) => ['状态', '账号状态', '成员状态', '租户状态'].includes(change.label));
  if (status?.before.length && status.after.length) return `${status.before.join('、')} → ${status.after.join('、')}`;
  const admin = changes.find((change) => change.label === '租户管理员');
  if (admin?.after.length) return `管理员${admin.before.length ? '交接为' : '指定为'} ${admin.after.join('、')}`;
  if (changes.length) return `变更了${changes.slice(0, 3).map((change) => change.label).join('、')}${changes.length > 3 ? `等 ${changes.length} 项内容` : ''}`;
  if (event.event_type === 'tenant.authorization.preset_synced') return '平台发布同步了该成员的预置角色与授权。';
  return '已记录本次操作；可在详情中查看原因与上下文。';
}

export function auditExplanation(event: AccessAuditEvent) {
  if (securityEvents.has(event.event_type)) return '这是一条账号安全事件，没有业务字段的前后差异。';
  if (event.event_type === 'catalog_synced') {
    const after = record(event.details.after);
    const counts = [['permissions', '项权限'], ['roles', '个预置角色'], ['tenants', '个租户']].flatMap(([key, label]) => { const value = after[key!]; return typeof value === 'number' ? [`${value} ${label}`] : Array.isArray(value) ? [`${value.length} ${label}`] : []; });
    return counts.length ? `本次发布目录包含或涉及 ${counts.join('、')}。目录版本可在技术信息中核对。` : '同步权限定义与预置角色。这是系统发布记录，目录版本可在技术信息中核对。';
  }
  if (event.details.before == null && event.details.after == null) return '这条历史记录未保存字段快照，无法还原具体的前后变化。';
  return '没有可展示的业务字段差异；完整记录保留在技术信息中。';
}
