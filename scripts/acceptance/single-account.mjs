import { request } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { chmodSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { directory, environment, origin, readPrivate, savePrivate } from './environment.mjs';

// Consolidate the user-facing walkthrough through ordinary role/transfer APIs.
// Other accounts remain sample members and historical actors, not login steps.
export async function configureSingleAccount() {
  environment();
  const accounts = readPrivate('accounts.local.json');
  const fixture = readPrivate('manifest.json');
  const target = accounts.admin;
  const sessions = new Map();
  const changes = [];
  const reason = '按用户要求使用一个账号查看全部验收功能';
  async function call(session, method, path, data) {
    const response = await session.ctx.fetch(`${origin}/api/v1${path}`, { method, headers: session.headers, ...(data === undefined ? {} : { data }) });
    const body = response.status() === 204 ? null : await response.json();
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${body?.code ?? ''} ${body?.message ?? ''}`);
    return body;
  }
  async function signIn(account) {
    if (sessions.has(account.id)) return sessions.get(account.id);
    const ctx = await request.newContext();
    const response = await ctx.get(`${origin}/api/v1/auth/csrf`);
    if (!response.ok()) { await ctx.dispose(); throw new Error('Acceptance CSRF unavailable'); }
    const session = { ctx, headers: { Origin: origin, 'X-CSRF-Token': (await response.json()).token } };
    sessions.set(account.id, session);
    await call(session, 'POST', '/auth/login', { username: account.username, password: account.password });
    return session;
  }
  try {
    const platform = await signIn(accounts.platform);
    const user = await call(platform, 'GET', `/access/accounts/${target.id}`);
    const role = (await call(platform, 'GET', '/access/roles')).items.find((r) => r.builtin_key === 'platform_admin');
    if (!role) throw new Error('Platform administrator role is missing.');
    if (!user.roles.some((r) => r.id === role.id)) {
      await call(platform, 'PUT', `/access/accounts/${target.id}/roles`, { expected_version: user.management_version, role_ids: [...user.roles.map((r) => r.id), role.id], reason });
      changes.push('Granted platform administrator to the walkthrough account');
    }
    const single = await signIn(target);
    const memberships = {};
    for (const [key, tenant] of Object.entries(fixture.tenants)) {
      const path = `/platform/tenants/${tenant.id}`;
      let current = await call(platform, 'GET', path);
      if (current.administrator.id !== target.id) {
        const preview = await call(platform, 'POST', `${path}/administrator-transfer/preview`, {});
        if (preview.projects.length) throw new Error(`Tenant ${key} transfer has existing project obligations; inspect them before changing it.`);
        await call(platform, 'POST', `${path}/administrator-transfer`, { new_administrator_account_id: target.id, expected_version: preview.expected_version, reason });
        current = await call(platform, 'GET', path);
        changes.push(`Transferred tenant ${key} to the walkthrough account`);
      }
      memberships[key] = current.administrator_membership_id;
      tenant.admin = 'admin';
    }
    for (const [key, project] of Object.entries(fixture.projects)) {
      const tenantId = fixture.tenants.A.id;
      const current = await call(single, 'GET', `/tenants/${tenantId}/project-governance/${project.id}`);
      if (current.administrator.account_id === target.id) continue;
      const credentials = Object.values(accounts).find((a) => a.id === current.administrator.account_id);
      if (!credentials) throw new Error(`Unknown current project administrator for ${key}; existing data left intact.`);
      const manager = await signIn(credentials);
      const path = `/tenants/${tenantId}/projects/${project.id}/administrator-transfer`;
      const input = { tenant_membership_id: memberships.A };
      const preview = await call(manager, 'POST', `${path}/preview`, input);
      if (!preview.can_submit) throw new Error(`Project ${key}: ${preview.blocked_reason}`);
      await call(manager, 'POST', path, { ...input, client_request_id: randomUUID(), expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, reason });
      changes.push(`Transferred project ${key} to the walkthrough account`);
    }
    const profile = await call(single, 'GET', '/account/profile');
    if (profile.account.display_name !== '验收管理员') await call(single, 'PATCH', '/account/profile', { display_name: '验收管理员' });
    target.display_name = '验收管理员';
    target.purpose = '唯一验收登录账号：平台、全部验收租户与项目管理';
    fixture.single_account = { username: target.username, account_id: target.id, tenant_memberships: memberships };
    fixture.urls.privateTemplateB = `${origin}/t/${fixture.tenants.B.id}/engineering/templates?definition=${fixture.definitions.privateB.definition_id}`;
    fixture.urls.practice = `${origin}/t/${fixture.tenants.A.id}/p/${fixture.projects.practice.id}/profile`;
    savePrivate('accounts.local.json', accounts);
    savePrivate('manifest.json', fixture);
    savePrivate('single-account.local.json', { ...target, origin, tenant_memberships: memberships });
    savePrivate('single-account-configuration.json', { updated_at: new Date().toISOString(), username: target.username, changes, tenant_count: Object.keys(fixture.tenants).length, project_count: Object.keys(fixture.projects).length });
    const guide = [
      '# 一个账号查看全部功能', '',
      `入口：[打开验收系统](${origin}/login)`, '',
      `- 用户名：\`${target.username}\``, `- 密码：\`${target.password}\``, '',
      '登录一次后，点击左上角“切换工作空间”，在平台、租户和项目之间切换即可。', '',
      '## 建议查看顺序', '',
      `1. [平台管理](${fixture.urls.platform})：租户、账号、角色、工程定义、公共模板和审计。`,
      `2. [华东交通租户](${fixture.urls.tenantA})：成员、租户角色、项目治理、私有模板和操作记录。`,
      `3. [青澜大桥及测点、通道](${fixture.urls.bridge})：设施属性、构件树、位置、监测通道和变更历史。`,
      `4. [青澜大桥项目](${fixture.urls.project})：项目资料、成员、项目角色和审计。`,
      `5. [西部公路租户](${fixture.urls.tenantB})：另一个租户的资源和私有模板。`, '',
      '停用练习租户可在平台查看并恢复；已归档项目可在项目内查看并恢复。成员列表中的其他账号作为示例数据展示。', '',
      '详细说明：[单账号验收手册](../../docs/operations/manual-acceptance.md)。', '',
      '入口未启动时，在仓库根目录执行：', '', '```bash',
      'docker compose --env-file .env -f deploy/compose/local.yaml up -d --wait',
      'node scripts/acceptance/start.mjs', '```', '',
    ].join('\n');
    const path = resolve(directory, '账号与验收入口.local.md');
    writeFileSync(path, guide, { mode: 0o600 });
    chmodSync(path, 0o600);
    console.log(`Single acceptance login ready: ${target.username}; 4 tenants and 3 projects. Password retained in the local guide.`);
  } finally {
    for (const session of sessions.values()) {
      try { await call(session, 'POST', '/auth/logout'); } finally { await session.ctx.dispose(); }
    }
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await configureSingleAccount();
