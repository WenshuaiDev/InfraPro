import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { client, digest, queryPG, read, safeDiagnosticLine, sleep, write } from './architecture-lab.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { visualOutputDirectory } from './architecture-visual-frontend.mjs';
import { visualRoleGrants } from './architecture-visual.mjs';

const uuid = (value) => {
  assert.match(value, /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/);
  return value;
};

const categories = ['organization', 'security', 'tasks', 'updates'];

function sameInvitation(left, right) {
  return (
    !!left?.passed &&
    !!right?.passed &&
    isDeepStrictEqual(left.summary.categories, right.summary.categories) &&
    left.summary.unread === right.summary.unread &&
    isDeepStrictEqual(left.receipt, right.receipt) &&
    isDeepStrictEqual(left.calls, right.calls) &&
    isDeepStrictEqual(left.target, right.target) &&
    left.projectedTenantSHA256 === right.projectedTenantSHA256 &&
    left.canAccept === right.canAccept
  );
}

// Read only identifiers from the synthetic databases. Every visibility,
// summary and target assertion goes through the published HTTP entry.
function invitationReceipts(lab, side, pending, tenant, viewer, member, inviter) {
  assert.match(viewer, /^[a-zA-Z0-9_]+$/);
  const query = `WITH targets AS (
    SELECT i.id,i.target_account_id,i.inviter_account_id,i.status FROM tenant_invitations i
    JOIN accounts a ON a.id=i.target_account_id WHERE i.tenant_id='${uuid(tenant)}' AND
      (i.id='${uuid(pending)}' AND i.status='pending' AND i.target_account_id='${uuid(member)}' OR
       i.status='accepted' AND a.username='${viewer}' AND i.inviter_account_id='${uuid(inviter)}')
  ) SELECT coalesce(json_agg(json_build_object('invitation',t.id,'receipt',n.id,'state',t.status,
    'role',CASE WHEN n.account_id=t.target_account_id THEN 'target' ELSE 'inviter' END)), '[]')
    FROM targets t JOIN notification_events e ON e.target_kind='tenant_invitation'
      AND e.target_id=t.id::text AND e.outcome=t.status
    JOIN account_notifications n ON n.event_id=e.id AND n.account_id IN(t.target_account_id,t.inviter_account_id)`;
  return JSON.parse(queryPG(lab, side, query));
}

function visualInvitationInput(directory, side) {
  const lab = read(join(directory, 'private/lab.json')),
    subjects = read(join(directory, 'private/accounts.json'));
  const viewer = read(join(directory, 'private/manual-project-account.json'));
  const session = read(join(directory, 'manual', side + '.json')),
    actors = read(join(directory, 'samples.json')).actors;
  const pending = uuid(session.pendingInvitation),
    tenant = uuid(session.scope.tenant);
  return { lab, subjects, viewer, pending, tenant, member: actors.member, inviter: actors.admin };
}

async function inspect(directory, side, state, supplied) {
  const { lab, subjects, viewer, pending, tenant, member, inviter } =
    supplied ?? visualInvitationInput(directory, side);
  const deadline = Date.now() + 30000;
  let rows;
  do {
    rows = invitationReceipts(lab, side, pending, tenant, viewer.username, member, inviter);
    if (rows.length === 3) break;
    await sleep(250);
  } while (Date.now() < deadline);
  assert.equal(rows.length, 3, 'Missing original pending recipient and accepted recipient/inviter receipts');
  const required = [
    ['pending-recipient', subjects.member, 'pending', 'target'],
    ['accepted-recipient', viewer, 'accepted', 'target'],
    ['accepted-inviter', subjects.admin, 'accepted', 'inviter'],
  ];
  const endpoint = { ...lab[side], api: lab[side].origin }; // Use the same origin as the browser.
  const outsider = await client(endpoint, subjects.other);
  try {
    for (const [name, person, expected, origin] of required) {
      const input = rows.find((row) => row.state === expected && row.role === origin);
      assert(input);
      const invite = uuid(input.invitation),
        receipt = uuid(input.receipt),
        api = await client(endpoint, person);
      const output = { name, inputSHA256: digest(JSON.stringify([invite, receipt])), calls: [] };
      state.push(output);
      async function get(who, path, label, expected) {
        const response = await who.client.get(who.api + path, { headers: who.headers });
        try {
          const status = response.status(),
            body = await response.json();
          output.calls.push({ label, status, code: body.code || null });
          assert(expected.includes(status), label + ' returned ' + status);
          return body;
        } finally {
          await response.dispose();
        }
      }
      try {
        const summary = await get(api, '/me/notifications/summary', 'own summary', [200]);
        assert.deepEqual(Object.keys(summary.categories).sort(), categories);
        assert(Object.values(summary.categories).every((count) => Number.isSafeInteger(count) && count >= 0));
        const categoryTotal = Object.values(summary.categories).reduce((n, count) => n + count, 0);
        assert.equal(summary.unread, categoryTotal);
        for (const key of ['revision', 'authorization_revision', 'watermark'])
          assert.match(summary[key], /^\d+$/);
        output.summary = {
          unread: summary.unread,
          categories: summary.categories,
          categoryTotal,
          watermark: summary.watermark,
        };
        const item = await get(api, '/me/notifications/' + receipt, 'own receipt', [200]);
        assert.equal(item.id, receipt);
        assert.equal(item.type, 'invitation.changed');
        assert.equal(item.category, 'organization');
        assert.equal(item.outcome, expected);
        assert.equal(item.current_state, expected);
        assert.equal(item.redacted, false);
        assert(Object.hasOwn(item, 'read_at') && Object.hasOwn(item, 'archived_at'));
        output.receipt = {
          category: item.category,
          outcome: item.outcome,
          read: item.read_at !== null,
          archived: item.archived_at !== null,
          redacted: item.redacted,
        };
        const target = await get(api, '/me/notifications/' + receipt + '/target', 'own target', [200]);
        if (origin === 'target') {
          const invitation = await get(api, '/me/tenant-invitations/' + invite, 'personal invitation', [200]);
          assert.equal(invitation.invitation_id, invite);
          assert.equal(invitation.status, expected);
          assert.equal(invitation.can_accept, expected === 'pending');
          assert.equal(item.object_name, invitation.preview.tenant.name);
          assert.equal(item.target_available, true);
          assert.equal(target.status, 'available');
          assert.equal(target.id, invite);
          assert.equal(target.kind, 'tenant_invitation');
          output.projectedTenantSHA256 = digest(invitation.preview.tenant.name);
          output.canAccept = invitation.can_accept;
        } else {
          assert.equal(item.target_available, false);
          assert.equal(target.status, 'unavailable');
          assert.equal(target.reason, 'invitation_accepted');
        }
        for (const suffix of ['', '/target'])
          await get(
            outsider,
            '/me/notifications/' + receipt + suffix,
            'unrelated account' + suffix,
            [403, 404],
          );
        output.state = item.current_state;
        output.available = item.target_available;
        output.target = { status: target.status, kind: target.kind ?? null, reason: target.reason ?? null };
        output.passed = true;
      } finally {
        await api.client.dispose();
      }
    }
  } finally {
    await outsider.client.dispose();
  }
}

export async function visualInvitationReceipts(directory, side, output, inputs) {
  assert(['b', 'self'].includes(side));
  const shared = sharedConsoleFor(output),
    profile = visualOutputDirectory(output, shared),
    started = new Date().toISOString();
  const baseline = read(join(directory, 'baseline.json'));
  const report = {
    started,
    side,
    sourceDirectory: directory,
    baseline: baseline.commit,
    baselineSourceSHA256: baseline.source_archive_sha256,
    consoleSHA256: shared?.console.sha256 ?? null,
    candidate: read(join(directory, 'candidate.json')).inputs.sourceSHA256,
    roleSQL: visualRoleGrants(directory, side),
    snapshot: read(join(directory, 'snapshot/manifest.json')).pg.seed,
    actors: {},
    passed: false,
  };
  for (const name of ['a', side]) {
    const state = [];
    report.actors[name] = state;
    try {
      await inspect(directory, name, state, inputs?.[name]);
    } catch (error) {
      (report.errors ??= []).push({ side: name, message: safeDiagnosticLine(error.message).slice(0, 300) });
    }
  }
  report.passed =
    !report.errors &&
    Object.values(report.actors).every((rows) => rows.length === 3 && rows.every((row) => row.passed));
  if (report.passed) {
    report.comparison = report.actors.a.map((left) => ({
      name: left.name,
      equal: sameInvitation(
        left,
        report.actors[side].find((row) => row.name === left.name),
      ),
    }));
    report.passed = report.comparison.every((row) => row.equal);
  }
  report.finished = new Date().toISOString();
  const folder = join(profile, 'visual', 'invitation-notifications', side);
  const path = join(folder, started.replaceAll(':', '-') + '.json');
  write(path, report);
  write(join(folder, 'latest.json'), { path, sha256: digest(readFileSync(path)), passed: report.passed });
  return report;
}

// The action only needs the applications' genuine public inbox. Bind its
// signed receipt to the actual snapshot, role grants and browser fixture.
export function verifiedVisualInvitations(
  pointer,
  folder,
  { baseline, candidate, consoleSHA256, sourceDirectory, snapshot, roleSQL },
) {
  if (
    !pointer?.passed ||
    typeof pointer.path !== 'string' ||
    dirname(resolve(pointer.path)) !== resolve(folder)
  )
    return false;
  let report;
  try {
    if (!existsSync(pointer.path)) return false;
    const bytes = readFileSync(pointer.path);
    if (digest(bytes) !== pointer.sha256) return false;
    report = JSON.parse(bytes);
  } catch {
    return false;
  }
  if (
    !report?.passed ||
    report.side !== 'b' ||
    report.baselineSourceSHA256 !== baseline ||
    report.candidate !== candidate ||
    report.consoleSHA256 !== consoleSHA256 ||
    report.sourceDirectory !== sourceDirectory ||
    report.snapshot !== snapshot ||
    !roleSQL(report.roleSQL)
  )
    return false;
  const hash = (value) => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
  for (const side of ['a', 'b']) {
    const rows = report.actors?.[side];
    if (!Array.isArray(rows) || rows.length !== 3 || new Set(rows.map((row) => row?.inputSHA256)).size !== 3)
      return false;
    for (const [name, state, recipient] of [
      ['pending-recipient', 'pending', true],
      ['accepted-recipient', 'accepted', true],
      ['accepted-inviter', 'accepted', false],
    ]) {
      const row = rows.find((value) => value?.name === name);
      if (
        !row?.passed ||
        !hash(row.inputSHA256) ||
        row.state !== state ||
        row.available !== recipient ||
        !Number.isSafeInteger(row.summary?.unread) ||
        row.summary.unread < 0 ||
        row.summary.unread !== row.summary.categoryTotal ||
        !isDeepStrictEqual(Object.keys(row.summary.categories ?? {}).sort(), categories) ||
        !categories.every(
          (key) => Number.isSafeInteger(row.summary.categories[key]) && row.summary.categories[key] >= 0,
        ) ||
        row.summary.unread !== Object.values(row.summary.categories).reduce((n, count) => n + count, 0) ||
        row.receipt?.category !== 'organization' ||
        row.receipt.outcome !== state ||
        row.receipt.redacted !== false ||
        typeof row.receipt.read !== 'boolean' ||
        typeof row.receipt.archived !== 'boolean' ||
        !/^\d+$/.test(row.summary.watermark) ||
        !Array.isArray(row.calls)
      )
        return false;
      const expected = [
        'own summary',
        'own receipt',
        'own target',
        ...(recipient ? ['personal invitation'] : []),
        'unrelated account',
        'unrelated account/target',
      ];
      if (
        expected.length !== row.calls.length ||
        !expected.every((label, index) => {
          const actual = row.calls[index];
          return (
            actual?.label === label &&
            (label.startsWith('unrelated') ? [403, 404].includes(actual.status) : actual.status === 200)
          );
        })
      )
        return false;
      if (
        recipient
          ? !(
              hash(row.projectedTenantSHA256) &&
              row.canAccept === (state === 'pending') &&
              row.target?.status === 'available' &&
              row.target.kind === 'tenant_invitation'
            )
          : !(
              row.target?.status === 'unavailable' &&
              row.target.reason === 'invitation_accepted' &&
              row.target.kind === null
            )
      )
        return false;
    }
  }
  return report.actors.a.every((left) =>
    sameInvitation(
      left,
      report.actors.b.find((right) => right.name === left.name),
    ),
  );
}
