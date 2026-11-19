import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { client } from './architecture-lab.mjs';

const urlNamespace = Buffer.from('6ba7b8119dad11d180b400c04fd430c8', 'hex');
const reason = '项目治理分页功能验证';

// RFC 4122 name-based UUID. The fixed request IDs recover the original result
// even if a previous run stopped after the public command committed.
function commandID(scope, operation) {
  const name = 'urn:infrapro:walkthrough:project-pagination:v1:' + scope + '/' + operation;
  const bytes = createHash('sha1').update(urlNamespace).update(name).digest().subarray(0, 16);
  bytes[6] = (bytes[6] & 0x0f) | 0x50;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  return bytes.toString('hex').replace(/^(\w{8})(\w{4})(\w{4})(\w{4})(\w{12})$/, '$1-$2-$3-$4-$5');
}

export async function seedPaginationProjects(call, username, scope) {
  assert(typeof scope === 'string' && scope.length > 0);
  const candidates = await call('/platform/tenant-admin-candidates?q=' + encodeURIComponent(username));
  const admin = candidates.items.find(person => person.username === username);
  assert(admin?.id);
  const tenantRequest = commandID(scope, 'tenant');
  const tenantCode = 'arc-paging-' + tenantRequest.slice(0, 8);
  const tenantName = '项目治理分页 ' + tenantCode;
  const createTenant = () => call('/platform/tenants', {
    code: tenantCode, name: tenantName, description: reason, administrator_account_id: admin.id,
    reason, client_request_id: tenantRequest,
  });
  const createdTenant = await createTenant();
  const tenant = createdTenant.tenant_id;
  assert(tenant);
  const path = '/tenants/' + tenant;
  const context = await call(path + '/context');
  assert.equal(context.tenant.code, tenantCode);
  assert(context.membership_id);
  const createProject = (code, name) => call(path + '/projects', {
    code, name, purpose: reason, reason, client_request_id: commandID(scope, code),
    owner_tenant_membership_id: context.membership_id,
    administrator_tenant_membership_id: context.membership_id,
  });
  const projects = [];
  let replayedProjects = 0;
  for (let i = 1; i <= 11; i++) {
    const code = 'page-' + String(i).padStart(2, '0');
    const name = '分页项目 ' + code;
    const created = await createProject(code, name);
    assert(created.project_id);
    projects.push({ id: created.project_id, code, name });
    if (created.replayed) replayedProjects++;
  }
  assert.equal(new Set(projects.map(project => project.id)).size, 11);
  const last = projects.at(-1);
  const tenantReplay = await createTenant();
  assert.equal(tenantReplay.tenant_id, tenant);
  assert.equal(tenantReplay.replayed, true);
  const projectReplay = await createProject(last.code, last.name);
  assert.equal(projectReplay.project_id, last.id);
  assert.equal(projectReplay.replayed, true);
  return {
    tenant, tenantCode, tenantName, openPath: '/t/' + tenant + '/projects', projects,
    replayed: { tenant: createdTenant.replayed, projects: replayedProjects, verifiedTenant: true, verifiedProject: last.id },
  };
}

export async function seedWalkthroughPagination({ lab, account, scope }) {
  const session = await client(lab, account);
  try {
    const sample = await seedPaginationProjects(session.call, account.username, scope);
    const path = '/tenants/' + sample.tenant + '/project-governance?status=all&limit=10';
    const first = await session.call(path);
    assert.equal(first.overview.total, 11);
    assert.equal(first.items.length, 10);
    assert(first.next_cursor);
    const last = await session.call(path + '&cursor=' + encodeURIComponent(first.next_cursor));
    assert.equal(last.items.length, 1);
    assert(!last.next_cursor);
    const entries = response => response.items.map(({ project }) => ({ id: project.id, code: project.code, name: project.name }));
    const firstPage = entries(first), secondPage = entries(last);
    const byCode = (a, b) => a.code.localeCompare(b.code);
    assert.deepEqual([...firstPage, ...secondPage].sort(byCode), [...sample.projects].sort(byCode));
    assert.deepEqual(entries(await session.call(path)), firstPage);
    return { ...sample, pageSize: 10, total: first.overview.total, firstPage, secondPage, returnToFirstPage: true };
  } finally {
    await session.client.dispose();
  }
}
