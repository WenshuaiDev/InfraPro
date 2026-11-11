import { request } from '@playwright/test';
import { randomBytes, randomUUID } from 'node:crypto';
import { existsSync, writeFileSync, chmodSync } from 'node:fs';
import { resolve } from 'node:path';
import { setTimeout as pause } from 'node:timers/promises';
import { directory, environment, origin, readPrivate, savePrivate } from './environment.mjs';

environment(); // Validate the dedicated environment before making any request.
const api = `${origin}/api/v1`;
const accounts = readPrivate('accounts.local.json');
const state = existsSync(resolve(directory, 'seed-state.local.json')) ? readPrivate('seed-state.local.json') : { actions: {}, data: {}, started: new Date().toISOString() };
if (state.complete) {
  console.log('Acceptance data already prepared. Existing manual changes and credentials are retained.');
  process.exit(0);
}
const sessions = {};
const contexts = [];
const persist = () => savePrivate('seed-state.local.json', state);
async function once(key, action) {
  if (Object.hasOwn(state.actions, key)) return state.actions[key];
  const result = await action();
  state.actions[key] = result ?? null;
  persist();
  return result;
}
async function call(session, method, path, data, expected = [200, 201, 204], attempt = 0) {
  const response = await session.ctx.fetch(api + path, { method: method.toUpperCase(), headers: session.headers, ...(data === undefined ? {} : { data }) });
  let value;
  try { value = await response.json(); } catch { value = null; }
  if (response.status() === 429 && attempt < 2) {
    console.log('Waiting for the existing invitation rate-limit window.');
    await pause(31_000);
    return call(session, method, path, data, expected, attempt + 1);
  }
  if (response.status() === 403 && value?.code === 'reauthentication_required' && session.credentials && attempt < 2) {
    await call(session, 'post', '/auth/reauthenticate', { password: session.credentials.password });
    return call(session, method, path, data, expected, attempt + 1);
  }
  if (!expected.includes(response.status())) throw new Error(`${method} ${path}: ${response.status()} ${JSON.stringify({ code: value?.code, message: value?.message, issues: value?.issues })}`);
  return value;
}
async function open(credentials) {
  const ctx = await request.newContext();
  contexts.push(ctx);
  const response = await ctx.get(`${api}/auth/csrf`);
  if (response.status() !== 200) throw new Error('Acceptance CSRF endpoint is unavailable.');
  const session = { ctx, credentials, headers: { Origin: origin, 'X-CSRF-Token': (await response.json()).token } };
  if (credentials) await call(session, 'post', '/auth/login', { username: credentials.username, password: credentials.password });
  return session;
}
const wm = () => ({ request_id: randomUUID(), reason: '全功能人工验收数据准备（虚构资料）' });
const tm = () => ({ client_request_id: randomUUID(), reason: '全功能人工验收数据准备（虚构资料）' });
const credential = (url) => new URLSearchParams(new URL(url).hash.slice(1)).get('token');
const data = state.data;
try {
  sessions.platform = await open(accounts.platform);
  const platform = sessions.platform;
  const labels = {
    audit: '平台审计员（只读）', editor: '公共工程编制员（不能发布）',
    admin: '华东交通验收租户管理员', other: '西部交通验收租户管理员',
    engineer: '工程资源维护员', reader: '资源只读员（无公共目录权）',
    member: '普通租户成员', project: '项目管理员（无租户资源权）',
    project_editor: '项目资料协作员', invitee: '待加入租户的受邀账号',
    suspended: '已暂停成员', removed: '已移除成员', disabled: '已停用账号',
    spare: '管理员交接候选人', personal: '个人资料与密码练习账号',
    noauth: '已清空租户角色成员', template: '租户模板编制员（不能发布）',
  };
  for (const [key, label] of Object.entries(labels)) {
    if (!accounts[key]) {
      accounts[key] = { username: `accept_${key}`, display_name: `验收·${label}`, purpose: label, password: randomBytes(15).toString('base64url') + '!Aa7' };
      savePrivate('accounts.local.json', accounts);
    }
    const a = accounts[key];
    await once(`account:${key}`, async () => {
      const invitation = await call(platform, 'post', '/invitations', { note: `人工验收：${label}` });
      const anonymous = await open();
      await call(anonymous, 'post', '/public/invitations/accept', { token: credential(invitation.url), username: a.username, display_name: a.display_name, password: a.password });
      return { created: true };
    });
    const found = (await call(platform, 'get', `/access/accounts?q=${a.username}`)).items.find((item) => item.username === a.username);
    if (!found) throw new Error(`Missing prepared account ${key}`);
    a.id = found.id;
    if (found.status !== 'disabled') sessions[key] = await open(a);
  }
  accounts.platform.id = (await call(platform, 'get', '/access/accounts?q=accept_platform')).items.find((item) => item.username === 'accept_platform').id;
  savePrivate('accounts.local.json', accounts);
  console.log('Accounts prepared.');

  async function platformRole(key, name, codes, accountKey) {
    const role = await once(`platform-role:${key}`, () => call(platform, 'post', '/access/roles', { name, description: '人工验收专用自定义角色', reason: '人工验收准备' }));
    await once(`platform-role-permissions:${key}`, async () => {
      const current = await call(platform, 'get', `/access/roles/${role.id}`);
      return call(platform, 'put', `/access/roles/${role.id}/permissions`, { expected_version: current.version, permission_codes: codes, reason: '人工验收准备' });
    });
    if (accountKey) await once(`platform-bind:${accountKey}`, async () => {
      const current = await call(platform, 'get', `/access/accounts/${accounts[accountKey].id}`);
      return call(platform, 'put', `/access/accounts/${current.id}/roles`, { expected_version: current.management_version, role_ids: [...current.roles.map((r) => r.id), role.id], reason: '人工验收职责分工' });
    });
    return role;
  }
  data.platformRoles = {};
  data.platformRoles.audit = await platformRole('audit', '验收·平台审计只读', ['platform.audit.read', 'platform.account.read', 'platform.role.read', 'platform.tenant.read'], 'audit');
  data.platformRoles.editor = await platformRole('editor', '验收·公共工程编制', ['engineering.type.read', 'engineering.type.edit', 'engineering.metric.read', 'engineering.metric.edit', 'engineering.template.read', 'engineering.template.edit'], 'editor');
  data.platformRoles.disabled = await platformRole('disabled', '验收·已停用练习角色', ['platform.invitation.read']);
  await once('platform-role-disable', async () => {
    const r = await call(platform, 'get', `/access/roles/${data.platformRoles.disabled.id}`);
    return call(platform, 'put', `/access/roles/${r.id}/status`, { expected_version: r.version, status: 'disabled', reason: '保留停用角色样例' });
  });
  data.platformRoles.archive = await platformRole('archived', '验收·已归档练习角色', []);
  await once('platform-role-archive', async () => {
    const r = await call(platform, 'get', `/access/roles/${data.platformRoles.archive.id}`);
    return call(platform, 'post', `/access/roles/${r.id}/archive`, { expected_version: r.version, reason: '保留归档角色审计样例' });
  });

  data.tenants = {};
  for (const [key, name, admin] of [
    ['A', '验收·华东交通监测中心', 'admin'], ['B', '验收·西部公路监测中心', 'other'],
    ['C', '验收·停用恢复练习单位', 'admin'], ['D', '验收·管理员交接练习单位', 'admin'],
  ]) {
    const created = await once(`tenant:${key}`, () => call(platform, 'post', '/platform/tenants', { ...tm(), name, code: `acceptance-${key.toLowerCase()}`, administrator_account_id: accounts[admin].id }));
    data.tenants[key] = { id: created.tenant_id, name, admin };
    await once(`tenant-description:${key}`, async () => {
      const path = `/platform/tenants/${created.tenant_id}`;
      const current = await call(platform, 'get', path);
      return call(platform, 'patch', path, { expected_version: current.version, description: `全功能人工验收虚构单位 ${key}。${key === 'A' ? '主要验收成员、角色、项目、监测模板和工程资源。' : key === 'B' ? '用于验证跨租户数据隔离与多租户切换。' : key === 'C' ? '用于停用与恢复练习，避免影响主租户。' : '用于管理员交接与编号保留练习。'}`, reason: '补齐人工验收租户档案' });
    });
  }
  const A = data.tenants.A.id;
  const B = data.tenants.B.id;
  const tenantPath = (tid) => `/tenants/${tid}`;
  const admin = sessions.admin;
  data.memberships = {};
  async function join(tid, accountKey, manager = admin, suffix = '') {
    return once(`join:${tid}:${accountKey}${suffix}`, async () => {
      const invitation = await call(manager, 'post', `${tenantPath(tid)}/member-invitations`, { ...tm(), target_account_id: accounts[accountKey].id });
      const accepted = await call(sessions[accountKey], 'post', '/account/tenant-invitations/accept', { token: credential(invitation.link), expected_version: invitation.invitation.version });
      return accepted.membership_id;
    });
  }
  for (const key of ['engineer', 'reader', 'member', 'project', 'project_editor', 'suspended', 'removed', 'disabled', 'spare', 'noauth', 'template']) data.memberships[key] = await join(A, key);
  data.memberships.admin = (await call(admin, 'get', `${tenantPath(A)}/context`)).membership_id;
  data.memberships.engineerB = await join(B, 'engineer', sessions.other);
  data.memberships.spareD = await join(data.tenants.D.id, 'spare');
  data.tenantRoles = {};
  async function tenantRole(key, name, permissions) {
    const result = await once(`tenant-role:${key}`, () => call(admin, 'post', `${tenantPath(A)}/roles`, { ...tm(), name, description: '人工验收自定义职责', permission_codes: permissions }));
    data.tenantRoles[key] = result.role;
    return result.role;
  }
  await tenantRole('engineer', '验收·工程资源维护', ['tenant.engineering.resource.read', 'tenant.engineering.resource.manage', 'tenant.engineering.catalog.read']);
  await tenantRole('reader', '验收·工程资源只读', ['tenant.engineering.resource.read']);
  await tenantRole('profile', '验收·租户资料维护', ['tenant.profile.read', 'tenant.profile.update']);
  await tenantRole('template', '验收·私有模板编制', ['tenant.engineering.template.read', 'tenant.engineering.template.edit', 'tenant.engineering.catalog.read']);
  await tenantRole('unused', '验收·可删除空角色', []);
  async function tenantBind(key, roleKeys) {
    await once(`tenant-bind:${key}`, async () => {
      const path = `${tenantPath(A)}/members/${data.memberships[key]}`;
      const current = await call(admin, 'get', `${path}/authorization`);
      const roles = roleKeys.length ? [...current.roles.map((r) => r.id), ...roleKeys.map((r) => data.tenantRoles[r].id)] : [];
      const input = { expected_membership_version: current.membership_version, role_ids: roles };
      const preview = await call(admin, 'post', `${path}/roles/preview`, input);
      return call(admin, 'put', `${path}/roles`, { ...input, preview_hash: preview.preview_hash, role_versions: preview.role_versions, reason: '人工验收权限分工' });
    });
  }
  await tenantBind('engineer', ['engineer', 'profile']);
  await tenantBind('reader', ['reader']);
  await tenantBind('template', ['template']);
  await tenantBind('noauth', []);
  for (const [key, action] of [['suspended', 'suspend'], ['removed', 'remove']]) {
    await once(`membership-state:${key}`, async () => {
      const path = `${tenantPath(A)}/members/${data.memberships[key]}`;
      const member = await call(admin, 'get', path);
      return call(admin, 'post', `${path}/${action}`, { expected_version: member.version, reason: '保留人工验收成员生命周期样例' });
    });
  }
  console.log('Tenant profiles, memberships and authorization prepared.');

  data.projects = {};
  const projectAdmin = sessions.project;
  for (const [key, name] of [['bridge', '验收·青澜大桥健康监测项目'], ['archived', '验收·北山隧道一期（已归档）'], ['practice', '验收·项目交接与成员练习']]) {
    const result = await once(`project:${key}`, () => call(admin, 'post', `${tenantPath(A)}/projects`, { ...tm(), name, code: `accept-${key}`, purpose: '验收项目治理、成员权限与历史；不含真实观测数据。', description: '虚构的交通基础设施监测项目，验收专用。', planned_start: '2026-11-01', planned_end: '2027-10-31', owner_tenant_membership_id: data.memberships.spare, administrator_tenant_membership_id: data.memberships.project }));
    data.projects[key] = { id: result.project_id, name };
  }
  const projectPath = (key) => `${tenantPath(A)}/projects/${data.projects[key].id}`;
  async function projectChange(key, path, input = {}, method = 'post') {
    return once(key, async () => {
      const preview = await call(projectAdmin, 'post', `${path}/preview`, input);
      if (!preview.can_submit) throw new Error(`Project preparation blocked: ${preview.blocked_reason}`);
      return call(projectAdmin, method, path, { ...input, ...tm(), expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, ...(input.role_ids ? { role_versions: preview.role_versions } : {}) });
    });
  }
  data.projectMembers = {};
  for (const key of ['project_editor', 'reader', 'member', 'spare']) {
    const member = await once(`project-member:${key}`, async () => {
      const profile = await call(projectAdmin, 'get', `${projectPath('bridge')}/profile`);
      return call(projectAdmin, 'post', `${projectPath('bridge')}/members`, { ...tm(), expected_project_version: profile.version, tenant_membership_id: data.memberships[key] });
    });
    data.projectMembers[key] = member.object_id;
  }
  data.projectRoles = {};
  for (const [key, name, permissions] of [['editor', '验收·项目资料维护', ['project.profile.read', 'project.profile.update', 'project.member.read', 'project.audit.read']], ['reader', '验收·项目资料只读', ['project.profile.read']]]) {
    const role = await once(`project-role:${key}`, async () => {
      const project = await call(projectAdmin, 'get', `${projectPath('bridge')}/profile`);
      return call(projectAdmin, 'post', `${projectPath('bridge')}/roles`, { ...tm(), expected_project_version: project.version, name, description: '人工验收项目职责', permission_codes: permissions });
    });
    data.projectRoles[key] = role.object_id;
  }
  for (const [key, roles] of [['project_editor', ['editor', 'reader']], ['reader', ['reader']]]) {
    const path = `${projectPath('bridge')}/members/${data.projectMembers[key]}`;
    const member = await call(projectAdmin, 'get', path);
    const project = await call(projectAdmin, 'get', `${projectPath('bridge')}/profile`);
    await projectChange(`project-assign:${key}`, `${path}/roles`, { expected_version: member.version, expected_project_version: project.version, role_ids: roles.map((r) => data.projectRoles[r]) }, 'put');
  }
  await projectChange('project-archive', `${projectPath('archived')}/archive`);
  console.log('Active, archived and practice projects prepared.');

  data.definitions = {};
  const eng = '/platform/engineering';
  const content = (name, extra = {}) => ({ metamodel_version: '1', name, description: '虚构工程验收资料，可用于人工操作。', release_notes: '人工验收初始版本', ...extra });
  async function publish(key, kind, code, c, session = platform, prefix = eng) {
    const draft = await once(`definition-create:${key}`, () => call(session, 'post', `${prefix}/${kind}`, { ...wm(), code, content: c }));
    const version = await once(`definition-publish:${key}`, () => call(session, 'post', `${prefix}/${kind}/${draft.definition_id}/publish`, { ...wm(), revision: draft.revision }));
    data.definitions[key] = { ...version, kind, name: c.name, code };
    return version;
  }
  for (const [key, name] of [['length', '长度'], ['temperature', '温度']]) await once(`category:${key}`, () => call(platform, 'post', `${eng}/unit-categories`, { ...wm(), key: `accept_${key}`, name: `验收·${name}`, description: `${name}量纲` }));
  const mm = await publish('mm', 'units', 'accept_mm', content('毫米', { unit: { symbol: 'mm', category: 'accept_length' } }));
  const meter = await publish('meter', 'units', 'accept_meter', content('米', { unit: { symbol: 'm', category: 'accept_length' } }));
  const celsius = await publish('celsius', 'units', 'accept_celsius', content('摄氏度', { unit: { symbol: '°C', category: 'accept_temperature' } }));
  const displacement = await publish('displacement', 'metrics', 'accept_displacement', content('验收·结构位移', { metric: { shape: 'decimal', category: 'accept_length', standard_unit: mm.version_id, allowed_units: [mm.version_id, meter.version_id], options: [] } }));
  const temperature = await publish('temperature', 'metrics', 'accept_temperature', content('验收·结构温度', { metric: { shape: 'decimal', category: 'accept_temperature', standard_unit: celsius.version_id, allowed_units: [celsius.version_id], options: [], minimum: -50, maximum: 100 } }));
  const condition = await publish('condition', 'metrics', 'accept_condition', content('验收·巡查状态', { metric: { shape: 'enum', category: '', standard_unit: '', allowed_units: [], options: [{ key: 'normal', name: '正常' }, { key: 'attention', name: '需关注' }, { key: 'maintenance', name: '待检修' }] } }));
  const field = (key, name, shape, extra = {}) => ({ key, name, shape, required: false, description: '', group: '基础信息', order: 0, multiline: false, ...extra });
  const bridgeContent = content('验收·梁式桥梁', {
    fields: [field('length', '设计长度', 'decimal', { required: true, default: 0, minimum: 0, unit_version: meter.version_id }), field('lanes', '车道数', 'integer', { default: 4, minimum: 1, maximum: 12 }), field('opened', '通车日期', 'date'), field('inspected_at', '最近检查时间', 'datetime'), field('in_service', '在役运行', 'boolean', { required: true, default: false }), field('material', '结构材料', 'enum', { options: [{ key: 'concrete', name: '混凝土' }, { key: 'steel', name: '钢结构' }] }), field('methods', '巡检方式', 'multi_enum', { options: [{ key: 'manual', name: '人工巡检' }, { key: 'sensor', name: '传感监测' }] }), field('notes', '备注', 'text', { multiline: true })],
    components: [{ key: 'span', name: '跨', description: '参考最少两跨，但实际实例允许只建一跨；最多两跨。', fields: [field('length', '跨长', 'decimal', { default: 0, unit_version: meter.version_id })] }, { key: 'beam', name: '梁', description: '只能挂在跨下', fields: [] }, { key: 'pier', name: '桥墩', description: '设施整体下的桥墩', fields: [] }],
    relations: [{ parent: 'root', child: 'span', min_count: 2, max_count: 2, order: 0 }, { parent: 'span', child: 'beam', min_count: 0, max_count: 3, order: 0 }, { parent: 'root', child: 'pier', min_count: 0, max_count: 3, order: 1 }],
  });
  const bridge = await publish('bridge', 'type-definitions', 'accept_bridge', bridgeContent);
  const slope = await publish('slope', 'type-definitions', 'accept_slope', content('验收·公路边坡', { fields: [field('height', '边坡高度', 'decimal', { required: true, minimum: 0, unit_version: meter.version_id }), field('material', '岩土类别', 'text', { required: true })], components: [{ key: 'zone', name: '监测分区', description: '', fields: [] }], relations: [{ parent: 'root', child: 'zone', min_count: 0, order: 0 }] }));
  const tunnel = await publish('tunnel', 'type-definitions', 'accept_tunnel', content('验收·公路隧道', { fields: [field('length', '隧道长度', 'decimal', { required: true, unit_version: meter.version_id })], components: [{ key: 'section', name: '衬砌断面', description: '', fields: [] }], relations: [{ parent: 'root', child: 'section', min_count: 0, order: 0 }] }));
  const road = await publish('road', 'type-definitions', 'accept_road', content('验收·路基路面', { fields: [field('grade', '道路等级', 'text', { required: true }), field('length', '路段长度', 'decimal', { unit_version: meter.version_id })], components: [{ key: 'layer', name: '结构层', description: '', fields: [] }], relations: [{ parent: 'root', child: 'layer', min_count: 0, order: 0 }] }));
  const v2draft = await once('bridge-v2-draft', () => call(platform, 'post', `${eng}/type-definitions/${bridge.definition_id}/draft`, { ...wm(), revision: bridge.revision, version_id: bridge.version_id }));
  const v2saved = await once('bridge-v2-save', () => call(platform, 'put', `${eng}/type-definitions/${bridge.definition_id}/draft`, { ...wm(), revision: v2draft.revision, content: { ...bridgeContent, release_notes: 'V2 增加选填的养护等级；已有设施继续固定引用 V1。', fields: [...bridgeContent.fields, field('maintenance_grade', '养护等级', 'text')] } }));
  data.definitions.bridgeV2 = await once('bridge-v2-publish', () => call(platform, 'post', `${eng}/type-definitions/${bridge.definition_id}/publish`, { ...wm(), revision: v2saved.revision }));
  data.definitions.draft = await once('definition-draft-only', () => call(sessions.editor, 'post', `${eng}/type-definitions`, { ...wm(), code: 'accept_culvert_draft', content: content('验收·涵洞（待发布草稿）', { fields: [field('length', '涵洞长度', 'decimal')], components: [], relations: [] }) }));
  const template = await publish('template', 'monitoring-templates', 'accept_bridge_template', content('验收·桥梁位移与巡查模板', { template: { purpose: '为梁式桥梁提供位移与人工巡查通道要求，不生成或采集观测数据。', type_versions: [bridge.version_id], groups: [{ key: 'structure', name: '结构监测', description: '', order: 0 }], items: [
    { key: 'displacement', name: '跨中位移', description: '每跨可布置两个位移通道', enabled: true, group: 'structure', order: 0, targets: [{ type_version: bridge.version_id, component: 'span' }], bindings: [{ metric_version: displacement.version_id, unit_version: mm.version_id, resource_kind: 'observation_channel', min_count: 1, max_count: 2 }] },
    { key: 'condition', name: '桥梁整体巡查', description: '无单位枚举观测', enabled: true, group: 'structure', order: 1, targets: [{ type_version: bridge.version_id, component: 'root' }], bindings: [{ metric_version: condition.version_id, unit_version: '', resource_kind: 'observation_channel', min_count: 1 }] },
  ] } }));
  for (const [key, tid, manager, name] of [['privateA', A, admin, '验收·华东桥梁私有监测模板'], ['privateB', B, sessions.other, '验收·西部私有巡查模板']]) {
    const base = `${tenantPath(tid)}/engineering/monitoring-templates`;
    const copy = await once(`copy:${key}`, () => call(manager, 'post', `${base}/copy`, { ...wm(), code: `accept_private_${key.toLowerCase()}`, source_version: template.version_id }));
    const detail = await call(manager, 'get', `${base}/${copy.definition_id}`);
    const saved = await once(`copy-save:${key}`, () => call(manager, 'put', `${base}/${copy.definition_id}/draft`, { ...wm(), revision: copy.revision, content: { ...detail.draft.content, name, release_notes: '复制公共 V1 后独立修改并发布。' } }));
    data.definitions[key] = await once(`copy-publish:${key}`, () => call(manager, 'post', `${base}/${copy.definition_id}/publish`, { ...wm(), revision: saved.revision }));
  }
  data.definitions.privateDraft = await once('private-template-draft', () => call(sessions.template, 'post', `${tenantPath(A)}/engineering/monitoring-templates/copy`, { ...wm(), code: 'accept_private_draft', source_version: template.version_id }));
  const disabledType = await publish('disabledType', 'type-definitions', 'accept_disabled_type', content('验收·停用类型（恢复练习）', { fields: [], components: [], relations: [] }));
  await once('definition-disable', async () => {
    const definition = await call(platform, 'get', `${eng}/type-definitions/${disabledType.definition_id}`);
    const version = definition.versions.find((item) => item.id === disabledType.version_id);
    return call(platform, 'post', `${eng}/type-definitions/${disabledType.definition_id}/versions/${disabledType.version_id}/state`, { ...wm(), revision: version.state_revision, enabled: false });
  });
  console.log('Versioned definitions, public and tenant templates prepared.');

  data.resources = {};
  const resourceBase = (tid) => `${tenantPath(tid)}/engineering/resources/facilities`;
  async function facility(key, code, name, type, values, extra = {}, tid = A, manager = sessions.engineer) {
    const item = await once(`facility:${key}`, () => call(manager, 'post', resourceBase(tid), { request_id: randomUUID(), code, name, type_version_id: type.version_id, values, description: '人工验收虚构设施，可放心进行操作练习。', ...extra }));
    data.resources[key] = { ...item, tenant_id: tid };
    return item;
  }
  const main = await facility('bridge', 'QL-BRIDGE-001', '验收·青澜大桥', bridge, { length: 120, lanes: 4, opened: '2020-06-18', inspected_at: '2026-11-10T09:30:00+08:00', in_service: true, material: 'concrete', methods: ['manual', 'sensor'], notes: '虚构示例；V1 固定引用，两跨三梁示例。' }, { position: { geographic: { crs: 'EPSG:4326', longitude: 120.15, latitude: 30.28 }, elevation: { value_m: 35.2, datum: '验收假定高程基准' }, linear_references: [{ route_code: 'G000-TEST', start_station: 'K12+300', end_station: 'K12+420' }], source_reference: '人工构造的验收坐标，非现场测量' } });
  await facility('empty', 'EMPTY-BRIDGE-001', '验收·空设施（可直接新增构件）', bridge, { length: 0, in_service: false });
  const single = await facility('single', 'ONE-SPAN-001', '验收·单跨参考最少数量', bridge, { length: 30, in_service: false });
  const slopeF = await facility('slope', 'SLOPE-001', '验收·南岭高边坡', slope, { height: 42.5, material: '砂岩与粉质土' });
  const tunnelF = await facility('tunnel', 'TUNNEL-001', '验收·北山隧道', tunnel, { length: 860 });
  const roadF = await facility('road', 'ROAD-001', '验收·沿江试验路段', road, { grade: '一级公路', length: 1500 });
  const retired = await facility('retired', 'RETIRED-BRIDGE-001', '验收·已退役便桥', bridge, { length: 24, in_service: false });
  const deleted = await facility('deleted', 'DELETED-001', '验收·已删除误建记录', road, { grade: '临时道路' });
  await facility('other', 'WEST-SLOPE-001', '验收·西部租户隔离边坡', slope, { height: 65, material: '花岗岩' }, {}, B, sessions.other);
  async function component(key, f, code, name, category, parent) {
    const result = await once(`component:${key}`, async () => {
      const current = await call(sessions.engineer, 'get', `${resourceBase(A)}/${f.id}`);
      return call(sessions.engineer, 'post', `${resourceBase(A)}/${f.id}/components`, { request_id: randomUUID(), expected_structure_revision: current.structure_revision, code, name, category_key: category, ...(parent ? { parent_component_id: parent.id } : {}) });
    });
    data.resources[key] = result;
    return result;
  }
  const span1 = await component('span1', main, 'SPAN-01', '第一跨', 'span');
  const span2 = await component('span2', main, 'SPAN-02', '第二跨', 'span');
  const beam = await component('beam1', main, 'BEAM-01', '第一跨左侧主梁', 'beam', span1);
  await component('beam2', main, 'BEAM-02', '第一跨右侧主梁', 'beam', span1);
  await component('beam3', main, 'BEAM-03', '第二跨主梁', 'beam', span2);
  await component('pier', main, 'PIER-01', '中央桥墩', 'pier');
  await component('singleSpan', single, 'SPAN-01', '仅有一跨也可正常使用', 'span');
  const zone = await component('zone', slopeF, 'ZONE-01', '坡顶监测分区', 'zone');
  const section = await component('section', tunnelF, 'SECTION-01', '进口衬砌断面', 'section');
  const layer = await component('layer', roadF, 'LAYER-01', '沥青面层', 'layer');
  async function point(key, f, code, name, comp, position) {
    const result = await once(`point:${key}`, () => call(sessions.engineer, 'post', `${resourceBase(A)}/${f.id}/points`, { request_id: randomUUID(), code, name, purpose: '人工验收测点', ...(comp ? { component_id: comp.id } : {}), ...(position ? { position } : {}) }));
    data.resources[key] = result;
    return result;
  }
  const p1 = await point('point', main, 'P-DISP-01', '跨中位移测点', beam, { local: { x: 0, y: 0, z: 0, unit: 'm', axes_description: 'X 顺桥向、Y 横桥向、Z 向上', reference_description: '第一跨跨中梁底原点' }, linear_references: [{ route_code: 'G000-TEST', start_station: 'K12+330' }], sections: [{ kind: 'route_cross_section', code: 'DM-01', reference_description: '虚构路线横断面' }, { kind: 'facility_monitoring_section', code: 'JK-01', reference_description: '桥梁跨中监测断面' }] });
  const p0 = await point('zeroPoint', main, 'P-ZERO-01', '真实零坐标验证点', null, { geographic: { crs: 'EPSG:4326', longitude: 0, latitude: 0 }, elevation: { value_m: 0, datum: '验收零高程基准' } });
  await point('emptyPoint', main, 'P-EMPTY-01', '无通道且位置留空测点', span2);
  const oldPoint = await point('retiredPoint', main, 'P-OLD-01', '已自行退役测点', span1);
  const ps = await point('slopePoint', slopeF, 'P-SLOPE-01', '坡顶位移测点', zone);
  const pt = await point('tunnelPoint', tunnelF, 'P-TUNNEL-01', '衬砌温度测点', section);
  const pr = await point('roadPoint', roadF, 'P-ROAD-01', '路面人工巡查点', layer);
  const prt = await point('ancestorRetiredPoint', retired, 'P-RETIRED-01', '祖先退役后只读测点');
  async function channel(key, f, p, code, name, metric, unit, extra = {}) {
    const result = await once(`channel:${key}`, () => call(sessions.engineer, 'post', `${resourceBase(A)}/${f.id}/points/${p.id}/channels`, { request_id: randomUUID(), code, name, metric_version_id: metric.version_id, ...(unit ? { unit_version_id: unit.version_id } : {}), timestamp_meaning: 'observation_time', ...extra }));
    data.resources[key] = result;
    return result;
  }
  await channel('channel', main, p1, 'CH-X', '横向位移', displacement, mm, { observation_direction: '横桥向，向右为正', reference_basis: '安装时刻为零基准', sampling: { mode: 'periodic', interval_seconds: 60, description: '计划每分钟一次，不下发采集命令' } });
  await channel('channelY', main, p1, 'CH-Y', '纵向位移（同指标第二通道）', displacement, mm, { observation_direction: '顺桥向，朝大桩号为正' });
  await channel('channelStatus', main, p1, 'CH-STATE', '人工巡查状态', condition, null, { sampling: { mode: 'manual', description: '人工巡检登记' } });
  await channel('channelZero', main, p0, 'CH-ZERO', '零点位移', displacement, meter);
  await channel('channelSlope', slopeF, ps, 'CH-SLOPE', '坡顶水平位移', displacement, mm);
  await channel('channelTunnel', tunnelF, pt, 'CH-TEMP', '断面温度', temperature, celsius, { timestamp_meaning: 'interval_end', aggregation_description: '前十分钟观测平均值，时间表示统计区间结束', sampling: { mode: 'periodic', interval_seconds: 600, description: '计划十分钟统计一次' } });
  await channel('channelRoad', roadF, pr, 'CH-CHECK', '路面巡查状态', condition, null);
  await channel('channelRetired', retired, prt, 'CH-OLD', '退役设施保留通道', displacement, mm);
  const mainPath = `${resourceBase(A)}/${main.id}`;
  await once('point-history', () => call(sessions.engineer, 'patch', `${mainPath}/points/${p1.id}`, { request_id: randomUUID(), expected_revision: p1.revision, name: '第一跨跨中位移测点', purpose: '位移与状态联合验收，查看历史可见原名称' }));
  for (const [key, path, action] of [
    ['retired-point', `${mainPath}/points/${oldPoint.id}`, 'retire'],
    ['retired-facility', `${resourceBase(A)}/${retired.id}`, 'retire'],
    ['deleted-facility', `${resourceBase(A)}/${deleted.id}`, 'delete'],
  ]) await once(key, async () => {
    const current = await call(sessions.engineer, 'get', path);
    return call(sessions.engineer, 'post', `${path}/${action}`, { request_id: randomUUID(), expected_revision: current.revision, reason: '保留退役、删除与历史追溯验收样例' });
  });
  console.log('Facilities, components, points, channels and history prepared.');

  data.invitations = {};
  data.invitations.member = await once('pending-member-invite', () => call(admin, 'post', `${tenantPath(A)}/member-invitations`, { ...tm(), target_account_id: accounts.invitee.id }));
  const revoked = await once('revoked-member-invite-create', () => call(admin, 'post', `${tenantPath(A)}/member-invitations`, { ...tm(), target_account_id: accounts.personal.id }));
  await once('revoked-member-invite', () => call(admin, 'post', `${tenantPath(A)}/member-invitations/${revoked.invitation.id}/revoke`, { expected_version: revoked.invitation.version, reason: '邀请撤销验收样例' }));
  data.invitations.revoked = revoked;
  data.invitations.signup = await once('pending-signup', () => call(platform, 'post', '/invitations', { note: '保留给用户手动完成的开户邀请；建议用户名 accept_newuser' }));
  await once('disable-account', async () => {
    const current = await call(platform, 'get', `/access/accounts/${accounts.disabled.id}`);
    return call(platform, 'post', `/access/accounts/${current.id}/disable`, { expected_version: current.management_version, reason: '保留停用账号，供平台管理员恢复验收' });
  });
  await once('disable-tenant', async () => {
    const path = `/platform/tenants/${data.tenants.C.id}/disable`;
    const preview = await call(platform, 'post', `${path}/preview`, {});
    return call(platform, 'post', path, { expected_version: preview.expected_version, preview_hash: preview.preview_hash, reason: '人工验收保留停用状态', tenant_notice: '验收演练：该单位已暂停服务，请由平台管理员恢复。' });
  });
  data.urls = {
    login: `${origin}/login`, platform: `${origin}/platform/tenants`,
    tenantA: `${origin}/t/${A}/profile`, tenantB: `${origin}/t/${B}/profile`,
    tenantC: `${origin}/platform/tenants/${data.tenants.C.id}`, tenantD: `${origin}/platform/tenants/${data.tenants.D.id}`,
    resources: `${origin}/t/${A}/resources/facilities`,
    bridge: `${origin}/t/${A}/resources/facilities/${main.id}?component=${beam.id}&point=${p1.id}`,
    otherResources: `${origin}/t/${B}/resources/facilities`,
    project: `${origin}/t/${A}/p/${data.projects.bridge.id}/profile`,
    archive: `${origin}/t/${A}/p/${data.projects.archived.id}/profile`,
    definitions: `${origin}/platform/engineering/type-definitions`,
    publicTemplate: `${origin}/platform/engineering/monitoring-templates?definition=${template.definition_id}`,
    privateTemplate: `${origin}/t/${A}/engineering/templates?definition=${data.definitions.privateA.definition_id}`,
  };
  // Manifest is intentionally separate from credential-bearing invitation responses.
  const { invitations, ...manifest } = data;
  savePrivate('manifest.json', { origin, created: new Date().toISOString(), ...manifest });
  savePrivate('invitations.local.json', invitations);
  const linkNames = { login: '登录入口', platform: '平台租户管理', tenantA: 'A：华东交通租户资料', tenantB: 'B：西部公路租户资料', tenantC: 'C：停用恢复练习', tenantD: 'D：管理员交接练习', resources: 'A：设施列表', bridge: '青澜大桥、主梁及跨中测点', otherResources: 'B：隔离设施列表', project: '青澜大桥健康监测项目', archive: '已归档的北山隧道一期', definitions: '公共设施类型定义', publicTemplate: '公共监测模板', privateTemplate: 'A：私有监测模板' };
  const lines = ['# 全功能人工验收账号与入口', '', `入口：${origin}/login`, '', '这是仅供本机使用的虚构验收环境。密码只在此文件及 accounts.local.json 中保存，未写入 Git。', '', '| 用户名 | 密码 | 用途 |', '| --- | --- | --- |', ...Object.values(accounts).map((a) => `| ${a.username} | \`${a.password}\` | ${a.purpose} |`), '', '建议先用 accept_admin 查看租户业务，用 accept_platform 查看平台功能，用 accept_project 查看项目；先用 accept_personal 练习改密码，避免影响其他验收步骤。', '', '## 数据入口', '', ...Object.entries(data.urls).map(([key, url]) => `- [${key}](${url})`), '', '## 一次性邀请（通常 24 小时有效）', '', `- [成员邀请：先登录 accept_invitee，再确认加入](${invitations.member.link})`, `- [开户邀请：退出当前账号或新开隐私窗口，再创建 accept_newuser](${invitations.signup.url})`, '', '邀请过期时：成员邀请由 accept_admin 在租户 A 的“成员邀请”重新生成；开户邀请由 accept_platform 在“开户邀请”重新生成。链接不要发布到聊天、Git 或共享文档。', '', '启动：在仓库根目录运行 node scripts/acceptance/start.mjs。重启只启动服务，不重新生成账号、不重置数据。', ''];
  const localGuide = resolve(directory, '账号与验收入口.local.md');
  writeFileSync(localGuide, lines.join('\n').replace(/^- \[([^\]]+)\]/gm, (whole, key) => linkNames[key] ? `- [${linkNames[key]}]` : whole), { mode: 0o600 });
  chmodSync(localGuide, 0o600);
  state.complete = true;
  state.completed = new Date().toISOString();
  persist();
  // Clear setup sessions, retaining no seed process credentials in browser state files.
  for (const [key, session] of Object.entries(sessions)) if (key !== 'disabled') await call(session, 'post', '/auth/logout');
  console.log(`Prepared ${Object.keys(accounts).length} accounts, ${Object.keys(data.tenants).length} tenants and ${Object.keys(data.projects).length} projects. Credentials are in the private local guide.`);
} finally {
  await Promise.all(contexts.map((ctx) => ctx.dispose()));
}
