import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { expect, createAccount, csrf, password, consoleURL } from '../e2e/fixtures';

const api = new URL('/api/v1', consoleURL).href;
async function signIn(request: APIRequestContext, username: string) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200);
  return headers;
}

export async function prepareWorkspaces(request: APIRequestContext) {
  const account = await createAccount(request);
  const ownerHeaders = await signIn(request, 'e2e_owner');
  const accounts = await (await request.get(`${api}/access/accounts?q=${account.username}`)).json();
  const tenantIds: string[] = [];
  for (const name of ['交通监测中心', '城市基础设施中心']) {
    const response = await request.post(`${api}/platform/tenants`, { headers: ownerHeaders, data: { name, code: `ws-${randomBytes(5).toString('hex')}`, administrator_account_id: accounts.items[0].id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(201);
    tenantIds.push((await response.json()).tenant_id);
  }
  const headers = await signIn(request, account.username);
  const projectIds: string[] = [];
  for (const [index, name] of ['桥梁健康监测', '隧道运行监测'].entries()) {
    const tenantId = tenantIds[index]!;
    const ctx = await (await request.get(`${api}/tenants/${tenantId}/context`)).json();
    const response = await request.post(`${api}/tenants/${tenantId}/projects`, { headers, data: { name, code: `project-${index}`, purpose: '工作空间切换验收', owner_tenant_membership_id: ctx.membership_id, administrator_tenant_membership_id: ctx.membership_id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(200);
    projectIds.push((await response.json()).project_id);
  }
  return { account, tenantIds, projectIds };
}
