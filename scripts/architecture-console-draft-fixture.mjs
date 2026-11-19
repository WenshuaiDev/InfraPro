import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { existsSync } from 'node:fs';
import { join, relative } from 'node:path';
import { request } from '@playwright/test';
import { fileLease, read, write } from './architecture-lab.mjs';

export const draftWrites = [
  'tenant.member.suspend',
  'tenant.invitation.create',
  'tenant.invitation.regenerate',
];
export const draftReads = ['tenant.profile.read', 'tenant.member.read', 'tenant.invitation.read'];
const reason = '租户操作草稿与撤权功能验真';
const labels = {
  owner: '草稿租户管理员',
  operator: '草稿操作人',
  target: '草稿目标成员',
  pending: '草稿受邀人',
};
const tokenFrom = (link) => new URLSearchParams(new URL(link).hash.slice(1)).get('token');
const sameSet = (a, b) => JSON.stringify([...a].sort()) === JSON.stringify([...b].sort());

export function newDraftFixture(tag = randomBytes(5).toString('hex')) {
  const people = {};
  for (const [kind, label] of Object.entries(labels))
    people[kind] = {
      username: 'd_' + kind + '_' + tag,
      password: 'Qa! ' + randomBytes(18).toString('hex') + ' 松',
      display_name: label + ' ' + tag,
    };
  return { version: 1, tag, people, keys: {}, accounts: {}, invitations: {} };
}

export function openDraftHTTP({ api, origin }, trace = []) {
  for (const url of [api, origin])
    assert(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(url).hostname));
  const clients = [];
  async function connection() {
    const c = await request.newContext({ baseURL: api, maxRedirects: 0 });
    clients.push(c);
    const r = await c.get('/api/v1/auth/csrf');
    try {
      assert.equal(r.status(), 200);
      const csrf = (await r.json()).token;
      assert(csrf);
      return { c, csrf };
    } finally {
      await r.dispose();
    }
  }
  async function call(actor, method, path, data, expected = [200, 201, 204], commandID = randomUUID()) {
    const r = await actor.c.fetch('/api/v1' + path, {
      method,
      data,
      headers: { Origin: origin, 'X-CSRF-Token': actor.csrf, 'X-Command-ID': commandID },
      maxRetries: 0,
      timeout: 20000,
    });
    try {
      const status = r.status(),
        body = status === 204 ? {} : await r.json();
      const template = path.split('?')[0].replaceAll(/[0-9a-f]{8}-[0-9a-f-]{27}/gu, ':id');
      trace.push({ method, path: template, status, code: status >= 400 ? (body.code ?? null) : null });
      assert(
        expected.includes(status),
        method + ' ' + template + ' returned ' + status + ' ' + (body.code ?? ''),
      );
      return { body, status };
    } finally {
      await r.dispose();
    }
  }
  return { connection, call, close: () => Promise.all(clients.map((c) => c.dispose())) };
}

// Both the browser comparison and the retained human walkthrough use these
// ordinary onboarding and role-assignment operations. State is saved between
// requests so a process restart can find the original accounts and grants.
export async function prepareDraftFixture({ http, adminLogin, state, save = () => {} }) {
  const { connection, call } = http,
    people = state.people;
  const key = (label) => {
    if (!state.keys[label]) {
      state.keys[label] = randomUUID();
      save();
    }
    return state.keys[label];
  };
  const admin = await connection();
  await call(admin, 'POST', '/auth/login', adminLogin);
  const users = {};
  for (const [kind, person] of Object.entries(people)) {
    const actor = await connection(),
      login = { username: person.username, password: person.password };
    let auth;
    if (state.accounts[kind]) {
      const result = await call(
        actor,
        'POST',
        '/auth/login',
        login,
        state.accounts[kind].id ? [200] : [200, 401],
      );
      if (result.status === 200) auth = result.body;
    }
    if (!auth) {
      state.accounts[kind] = { pending: true };
      save();
      const invitation = (await call(admin, 'POST', '/invitations', { note: reason })).body;
      await call(
        actor,
        'POST',
        '/public/invitations/accept',
        { token: tokenFrom(invitation.url), ...person },
        [204],
      );
      auth = (await call(actor, 'POST', '/auth/login', login)).body;
    }
    assert.equal(auth.account.username, person.username);
    if (state.accounts[kind].id) assert.equal(auth.account.id, state.accounts[kind].id);
    state.accounts[kind] = { id: auth.account.id };
    save();
    users[kind] = { ...actor, id: auth.account.id };
  }
  const { owner, operator, target, pending } = users;
  const name = '草稿与撤权 ' + state.tag;
  if (!state.tenantID) {
    const requestID = key('tenant');
    const made = await call(
      admin,
      'POST',
      '/platform/tenants',
      {
        name,
        code: 'draft-' + state.tag,
        description: '隔离场景',
        administrator_account_id: owner.id,
        client_request_id: requestID,
        reason,
      },
      [200, 201],
      requestID,
    );
    state.tenantID = made.body.tenant_id;
    assert(state.tenantID);
    save();
  }
  const tenantID = state.tenantID,
    space = '/tenants/' + tenantID;
  const ctx = (await call(owner, 'GET', space + '/context')).body;
  assert.equal(ctx.tenant.id, tenantID);
  assert.equal(ctx.tenant.name, name);
  const authority = (await call(admin, 'GET', '/platform/tenants/' + tenantID)).body;
  assert.equal(authority.administrator.id, owner.id);

  async function visibleInvitation(kind) {
    const result = (
      await call(
        owner,
        'GET',
        space + '/member-invitations?q=' + encodeURIComponent(people[kind].username) + '&status=pending',
      )
    ).body;
    return result.items.find((item) => item.target.id === users[kind].id && item.status === 'pending');
  }
  async function invitation(kind, needLink, allowRotation = true) {
    let current = await visibleInvitation(kind);
    const prior = state.invitations[kind];
    // The server filters expired generations by its own clock. A manual
    // regeneration may leave a different pending invitation for this target.
    if (current && !needLink) {
      if (prior?.id !== current.id) {
        state.invitations[kind] = { id: current.id };
        save();
      }
      return { item: current };
    }
    if (current && prior?.id === current.id && prior.token)
      return { item: current, token: prior.token };
    let result;
    if (current) {
      const requestID = key('rotate-' + current.id + '-' + current.version);
      result = await call(
        owner,
        'POST',
        space + '/member-invitations/' + current.id + '/regenerate',
        { expected_version: current.version, client_request_id: requestID, reason },
        [200, 201],
        requestID,
      );
    } else {
      // If a previous generation expired or was deliberately consumed, start
      // a new command for this same synthetic recipient.
      const requestID = key('invite-' + kind + '-' + (prior?.id ?? 'initial'));
      result = await call(
        owner,
        'POST',
        space + '/member-invitations',
        { target_account_id: users[kind].id, client_request_id: requestID, reason },
        [200, 201],
        requestID,
      );
    }
    current = result.body.invitation;
    assert.equal(current.target.id, users[kind].id);
    const token = needLink && result.body.link ? tokenFrom(result.body.link) : undefined;
    state.invitations[kind] = { id: current.id, ...(token && { token }) };
    save();
    if (needLink && !token) {
      assert(allowRotation, 'Cannot recover the private signup link from the fixture invitation');
      return invitation(kind, needLink, false);
    }
    return { item: current, token };
  }
  async function member(kind) {
    const rows = (await call(owner, 'GET', space + '/members?q=' + encodeURIComponent(people[kind].username)))
      .body.items;
    let row = rows.find((r) => r.account.id === users[kind].id && r.membership_status !== 'ended');
    if (row?.membership_status === 'suspended') {
      await call(owner, 'POST', space + '/members/' + row.id + '/resume', {
        expected_version: row.version,
        reason,
      });
      row = (await call(owner, 'GET', space + '/members/' + row.id)).body;
    }
    if (row) {
      assert.equal(row.membership_status, 'active');
      return { membership_id: row.id, membership_version: row.version };
    }
    const { item, token } = await invitation(kind, true);
    const value = (
      await call(users[kind], 'POST', '/account/tenant-invitations/accept', {
        token,
        expected_version: item.version,
      })
    ).body;
    assert(value.membership_id);
    delete state.invitations[kind].token;
    save();
    return value;
  }
  const actorMembership = await member('operator'),
    targetMembership = await member('target');
  const pendingInvite = (await invitation('pending', false)).item;
  const ensureRole = async (title, permissions) => {
    const roleName = '草稿' + title + ' ' + state.tag;
    const rows = (await call(owner, 'GET', space + '/roles?q=' + encodeURIComponent(roleName))).body.items;
    let role = rows.find((r) => r.name === roleName);
    if (!role) {
      const requestID = key('role-' + title);
      role = (
        await call(
          owner,
          'POST',
          space + '/roles',
          {
            name: roleName,
            description: reason,
            permission_codes: permissions,
            reason,
            client_request_id: requestID,
          },
          [200, 201],
          requestID,
        )
      ).body.role;
    }
    assert(role.id && sameSet(role.permission_codes, permissions));
    return role;
  };
  const edit = await ensureRole('管理', [...draftReads, ...draftWrites]),
    readOnly = await ensureRole('只读', draftReads);
  const actorPath = space + '/members/' + actorMembership.membership_id;
  async function assign(roleID) {
    assert([edit.id, readOnly.id].includes(roleID));
    const actual = (await call(owner, 'GET', actorPath + '/authorization')).body;
    const roleIDs = actual.roles.map((r) => r.id),
      others = roleIDs.filter((id) => id !== edit.id && id !== readOnly.id);
    const next = [...others, roleID];
    if (!sameSet(roleIDs, next)) {
      const input = { role_ids: next, expected_membership_version: actual.membership_version };
      const url = actorPath + '/roles',
        preview = (await call(owner, 'POST', url + '/preview', input)).body;
      await call(owner, 'PUT', url, {
        ...input,
        expected_membership_version: preview.target_version,
        role_versions: preview.role_versions,
        preview_hash: preview.preview_hash,
        reason,
      });
    }
    return (await call(operator, 'GET', space + '/context')).body;
  }
  const allowed = await assign(edit.id);
  assert(draftWrites.every((p) => allowed.permissions.includes(p)));
  return {
    people,
    tenantID,
    name,
    space,
    owner,
    operator,
    target,
    pending,
    actorMembership,
    targetMembership,
    pendingInvite,
    edit,
    readOnly,
    allowed,
    assign,
  };
}

export function retainedDraftFiles(directory, environment, side) {
  assert(/^[a-f0-9]{12}$/.test(environment) && ['a', 'b'].includes(side));
  const folder = join(directory, 'private', 'walkthrough-drafts', side + '-' + environment);
  return {
    stateFile: join(folder, 'state.json'),
    peopleFile: join(folder, 'operator-accounts.json'),
    lockFile: join(folder, 'seed.flock'),
  };
}

export async function seedWalkthroughDrafts({ directory, environment, side, cfg, admin }) {
  const { stateFile, peopleFile, lockFile } = retainedDraftFiles(directory, environment, side);
  const http = openDraftHTTP(cfg);
  const lock = await fileLease(lockFile);
  try {
    const state = existsSync(stateFile) ? read(stateFile) : newDraftFixture();
    assert.equal(state.version, 1);
    const save = () => write(stateFile, state);
    save();
    const f = await prepareDraftFixture({ http, adminLogin: admin, state, save });
    write(peopleFile, { owner: f.people.owner, operator: f.people.operator });
    const prefix = '/t/' + f.tenantID;
    return {
      credentialFile: relative(directory, peopleFile),
      tenant: { id: f.tenantID, name: f.name },
      owner: { displayName: f.people.owner.display_name },
      operator: {
        displayName: f.people.operator.display_name,
        membershipID: f.actorMembership.membership_id,
      },
      target: { displayName: f.people.target.display_name, membershipID: f.targetMembership.membership_id },
      invitation: { displayName: f.people.pending.display_name, id: f.pendingInvite.id },
      editableRole: { id: f.edit.id, name: f.edit.name },
      readonlyRole: { id: f.readOnly.id, name: f.readOnly.name },
      pages: {
        members: prefix + '/members',
        invitations: prefix + '/invitations',
        operatorAuthorization: prefix + '/members/' + f.actorMembership.membership_id,
      },
      readback: {
        memberStatus: 'active',
        invitationStatus: f.pendingInvite.status,
        initiallyEditable: draftWrites.every((p) => f.allowed.permissions.includes(p)),
      },
    };
  } finally {
    try {
      await http.close();
    } finally {
      await lock();
    }
  }
}
