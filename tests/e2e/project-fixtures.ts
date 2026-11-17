import { randomUUID, randomBytes } from 'node:crypto';
import type { APIRequestContext, Browser, BrowserContext } from '@playwright/test';
import { expect, consoleURL, createAccount, csrf, password } from './fixtures';
const origin = new URL(consoleURL).origin; const api = `${origin}/api/v1`;
async function signIn(request: APIRequestContext, username = 'e2e_owner') { const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) }; expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username, password } })).status()).toBe(200); return headers; }
export async function prepareProject(request: APIRequestContext, browser: Browser) {
  const admin = await createAccount(request, false); const bob = await createAccount(request, false); const carol = await createAccount(request, false); const h = await signIn(request);
  const id = async (u: string) => (await (await request.get(`${api}/access/accounts?q=${u}`)).json()).items[0].id as string;
  const adminId = await id(admin.username); const bobId = await id(bob.username); const carolId = await id(carol.username);
  const contexts: BrowserContext[] = []; const forUser = async (username: string) => { const c = await browser.newContext(); contexts.push(c); return { request: c.request, h: await signIn(c.request, username) }; };
  const ta = await forUser(admin.username); const pb = await forUser(bob.username); const pc = await forUser(carol.username);
  const tenant = async () => { const r = await request.post(`${api}/platform/tenants`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { name: '项目验收租户', code: `pj-${randomBytes(5).toString('hex')}`, administrator_account_id: adminId, client_request_id: randomUUID(), reason: '项目验收' } }); expect(r.status()).toBe(201); return (await r.json()).tenant_id as string; };
  const tid = await tenant(); const other = await tenant();
  const join = async (tid: string, accountId: string, user: typeof pb) => { const r = await ta.request.post(`${api}/tenants/${tid}/member-invitations`, { headers: { ...ta.h, 'X-Command-ID': randomUUID() }, data: { target_account_id: accountId, client_request_id: randomUUID(), reason: '项目成员准备' } }); expect(r.status()).toBe(201); const invitation = await r.json(); const token = new URLSearchParams(new URL(invitation.link).hash.slice(1)).get('token'); const accepted = await user.request.post(`${api}/account/tenant-invitations/accept`, { headers: { ...user.h, 'X-Command-ID': randomUUID() }, data: { token, expected_version: invitation.invitation.version } }); expect(accepted.status()).toBe(200); return (await accepted.json()).membership_id as string; };
  const bid = await join(tid, bobId, pb); const cid = await join(tid, carolId, pc); const otherBid = await join(other, bobId, pb);
  const create = async (code: string, tenantId = tid, administrator = bid, owner = cid) => { const r = await ta.request.post(`${api}/tenants/${tenantId}/projects`, { headers: { ...ta.h, 'X-Command-ID': randomUUID() }, data: { name: `项目 ${code}`, code, purpose: '业务资料内容仅项目可读', description: '边界验收', owner_tenant_membership_id: owner, administrator_tenant_membership_id: administrator, client_request_id: randomUUID(), reason: '项目验收创建' } }); expect(r.status(), await r.text()).toBe(200); return (await r.json()).project_id as string; };
  return { admin, bob, carol, tid, other, bid, cid, otherBid, ta, pb, pc, h, create, close: async () => { await Promise.all(contexts.map((c) => c.close().catch(() => {}))); } };
}
