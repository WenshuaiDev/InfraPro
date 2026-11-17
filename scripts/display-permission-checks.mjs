import { expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';

/** A real custom project role, not a mocked permission response. */
export async function checkDisplayPermissions({ page, call, tenant, project, instance, consoleOrigin, workspace }) {
  const browser = page.context().browser(), reader = await browser.newContext();
  const username = 'display_view_' + randomUUID().slice(0, 8), password = 'Display reader 合成 password 529';
  const api = consoleOrigin + '/api/v1', projectPath = `/tenants/${tenant}/projects/${project}`;
  const token = (await (await reader.request.get(api + '/auth/csrf')).json()).token;
  const headers = { Origin: consoleOrigin, 'X-CSRF-Token': token };
  const read = async (path, data, method = data === undefined ? 'GET' : 'POST') => {
    const r = await reader.request.fetch(api + path, { method, headers: { ...headers, 'X-Command-ID': randomUUID() }, data });
    expect(r.ok(), `${method} ${path}: ${r.status()}`).toBe(true);
    return r.status() === 204 ? {} : r.json();
  };
  const change = async (path, data, method = 'POST') => {
    const preview = await call(path + '/preview', data);
    return call(path, { ...data, expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, ...(path.includes('/members/') ? { role_versions: preview.role_versions } : {}), client_request_id: randomUUID(), reason: '大屏真实权限回归' }, method);
  };
  try {
    const invitation = await call('/invitations', { note: '大屏查看者合成测试' });
    await read('/public/invitations/accept', { token: new URLSearchParams(new URL(invitation.url).hash.slice(1)).get('token'), username, display_name: '大屏只读验收', password });
    await read('/auth/login', { username, password });
    const account = (await call('/access/accounts?q=' + username)).items[0];
    const invite = await call(`/tenants/${tenant}/member-invitations`, { target_account_id: account.id, client_request_id: randomUUID(), reason: '合成权限回归' });
    const joined = await read('/account/tenant-invitations/accept', { token: new URLSearchParams(new URL(invite.link).hash.slice(1)).get('token'), expected_version: invite.invitation.version });
    const ctx = await call(projectPath + '/context');
    const member = await call(projectPath + '/members', { client_request_id: randomUUID(), reason: '合成权限回归', expected_project_version: ctx.project.version, tenant_membership_id: joined.membership_id });
    const next = await call(projectPath + '/context');
    const role = await call(projectPath + '/roles', { name: '大屏只读测试', description: '隔离环境', permission_codes: ['project.display.view'], client_request_id: randomUUID(), reason: '合成权限回归', expected_project_version: next.project.version });
    await change(projectPath + `/members/${member.object_id}/roles`, { role_ids: [role.object_id] }, 'PUT');
    const readPath = projectPath + '/displays/instances/' + instance.id;
    const detail = await read(readPath); expect(detail.instance.id).toBe(instance.id); expect(detail.packages).toBeUndefined(); expect(detail.grant).toBeUndefined();
    const denied = await reader.request.post(api + projectPath + '/displays/instances', { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { request_id: randomUUID(), reason: '必须拒绝只读写入', expected_revision: '0', name: '不应创建' } }); expect([403, 404]).toContain(denied.status());
    const readPage = await reader.newPage(); await readPage.goto(consoleOrigin + workspace + '?instance=' + instance.id);
    await expect(readPage.getByRole('heading', { name: '访问', exact: true })).toBeVisible(); await expect(readPage.getByRole('button', { name: '新建大屏', exact: true })).toHaveCount(0); await expect(readPage.getByRole('button', { name: '程序版本', exact: true })).toHaveCount(0);
    // A restricted viewer list hides the instance even if project.display.view remains.
    const current = await call(readPath); await call(readPath, { request_id: randomUUID(), reason: '查看名单边界', expected_revision: current.instance.revision, name: current.instance.name, state: current.instance.state, viewers: [(await call('/access/accounts?q=e2e_owner')).items[0].id] }, 'PATCH');
    const hidden = await reader.request.get(api + readPath); expect([403, 404]).toContain(hidden.status());
    const restore = await call(readPath); await call(readPath, { request_id: randomUUID(), reason: '还原合成实例名单', expected_revision: restore.instance.revision, name: restore.instance.name, state: restore.instance.state, viewers: current.instance.viewers }, 'PATCH');
    await change(projectPath + `/roles/${role.object_id}/permissions`, { permission_codes: [] }, 'PUT'); await readPage.reload(); await expect(readPage.getByRole('heading', { name: '当前项目不可访问', exact: true })).toBeVisible(); await expect(readPage.getByRole('heading', { name: instance.name, exact: true })).toHaveCount(0);
    return ['real view-only role hides management DTO and controls', 'server rejects read-only mutations', 'viewer allowlist blocks unauthorized detail', 'role revocation removes display access'];
  } finally { await reader.close(); }
}
