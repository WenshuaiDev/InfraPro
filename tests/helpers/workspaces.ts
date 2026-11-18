import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { expect, createAccount, csrf, password, consoleURL } from '../e2e/fixtures';

const api = new URL('/api/v1', consoleURL).href;
async function signIn(request: APIRequestContext, username: string) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  const current = await request.get(`${api}/auth/me`);
  if (current.status() === 200 && (await current.json()).account.username === username) return headers;
  expect((await request.post(`${api}/auth/logout`, { headers })).status()).toBe(204);
  expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username, password } })).status()).toBe(200);
  return headers;
}

export async function prepareWorkspaces(request: APIRequestContext, withInvitationRole = true) {
  const account = await createAccount(request, withInvitationRole);
  const ownerHeaders = await signIn(request, 'e2e_owner');
  const accounts = await (await request.get(`${api}/access/accounts?q=${account.username}`)).json();
  const tenantIds: string[] = [];
  for (const name of ['交通监测中心', '城市基础设施中心']) {
    const response = await request.post(`${api}/platform/tenants`, { headers: { ...ownerHeaders, 'X-Command-ID': randomUUID() }, data: { name, code: `ws-${randomBytes(5).toString('hex')}`, administrator_account_id: accounts.items[0].id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(201);
    tenantIds.push((await response.json()).tenant_id);
  }
  const headers = await signIn(request, account.username);
  const projectIds: string[] = [];
  for (const [index, name] of ['桥梁健康监测', '隧道运行监测'].entries()) {
    const tenantId = tenantIds[index]!;
    const ctx = await (await request.get(`${api}/tenants/${tenantId}/context`)).json();
    const response = await request.post(`${api}/tenants/${tenantId}/projects`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { name, code: `project-${index}`, purpose: '工作空间切换验收', owner_tenant_membership_id: ctx.membership_id, administrator_tenant_membership_id: ctx.membership_id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(200);
    projectIds.push((await response.json()).project_id);
  }
  return { account, tenantIds, projectIds };
}

export async function prepareProjectOnly(request: APIRequestContext, count = 1) {
  const f = await prepareWorkspaces(request);
  const member = await createAccount(request, false);
  await signIn(request, 'e2e_owner');
  const target = (await (await request.get(`${api}/access/accounts?q=${member.username}`)).json()).items[0].id as string;
  const memberships: string[] = [];
  const projectMemberships: string[] = [];
  for (let i = 0; i < count; i++) {
    const tenant = f.tenantIds[i]!; const project = f.projectIds[i]!;
    let headers = await signIn(request, f.account.username);
    const invited = await request.post(`${api}/tenants/${tenant}/member-invitations`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { target_account_id: target, client_request_id: randomUUID(), reason: '项目独立授权验收' } });
    expect(invited.status()).toBe(201); const invite = await invited.json();
    headers = await signIn(request, member.username);
    const accepted = await request.post(`${api}/account/tenant-invitations/accept`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { token: new URLSearchParams(new URL(invite.link).hash.slice(1)).get('token'), expected_version: invite.invitation.version } });
    expect(accepted.status()).toBe(200); const membership = (await accepted.json()).membership_id as string; memberships.push(membership);
    headers = await signIn(request, f.account.username);
    const current = await (await request.get(`${api}/tenants/${tenant}/members/${membership}/authorization`)).json();
    const body = { role_ids: [], expected_membership_version: current.membership_version };
    const preview = await request.post(`${api}/tenants/${tenant}/members/${membership}/roles/preview`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: body });
    expect(preview.status(), await preview.text()).toBe(200); const impact = await preview.json();
    const changed = await request.put(`${api}/tenants/${tenant}/members/${membership}/roles`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { ...body, role_versions: impact.role_versions, preview_hash: impact.preview_hash, reason: '仅保留项目范围' } });
    expect(changed.status(), await changed.text()).toBe(200);
    const context = await (await request.get(`${api}/tenants/${tenant}/projects/${project}/context`)).json();
    const added = await request.post(`${api}/tenants/${tenant}/projects/${project}/members`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { tenant_membership_id: membership, expected_project_version: context.project.version, client_request_id: randomUUID(), reason: '项目独立授权验收' } });
    expect(added.status(), await added.text()).toBe(200); projectMemberships.push((await added.json()).object_id as string);
  }
  const suspend = async (index = 0) => {
    const headers = await signIn(request, f.account.username);
    const base = `${api}/tenants/${f.tenantIds[index]}/projects/${f.projectIds[index]}/members/${projectMemberships[index]}/suspend`;
    const preview = await (await request.post(`${base}/preview`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: {} })).json();
    const result = await request.post(base, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, client_request_id: randomUUID(), reason: '撤销入口验收' } });
    expect(result.status(), await result.text()).toBe(200);
  };
  return { ...f, member, memberships, suspend };
}
