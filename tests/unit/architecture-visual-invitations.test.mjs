import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { digest } from '../../scripts/architecture-lab.mjs';
import { verifiedVisualInvitations } from '../../scripts/architecture-visual-invitations.mjs';

test('the invitation gate verifies both recipients, the issuer, and every unrelated-account read', (t) => {
  const folder = mkdtempSync(join(tmpdir(), 'architecture-inbox-'));
  t.after(() => rmSync(folder, { recursive: true, force: true }));
  const file = join(folder, 'capture.json');
  const expected = {
    baseline: digest('frozen'),
    candidate: digest('candidate'),
    consoleSHA256: digest('console'),
    sourceDirectory: folder,
    snapshot: digest('database'),
    roleSQL: (value) => value?.control === digest('control'),
  };
  function actors(side) {
    return [
      ['pending-recipient', 'pending'],
      ['accepted-recipient', 'accepted'],
      ['accepted-inviter', 'accepted'],
    ].map(([name, state]) => {
      const recipient = !name.endsWith('inviter');
      return {
        name,
        state,
        passed: true,
        inputSHA256: digest(side + name),
        available: recipient,
        summary: {
          unread: 3,
          categoryTotal: 3,
          categories: { organization: 2, security: 0, tasks: 1, updates: 0 },
          watermark: '8',
        },
        receipt: { category: 'organization', outcome: state, read: false, archived: false, redacted: false },
        ...(recipient && { canAccept: state === 'pending', projectedTenantSHA256: digest('tenant') }),
        target: recipient
          ? { status: 'available', kind: 'tenant_invitation', reason: null }
          : { status: 'unavailable', kind: null, reason: 'invitation_accepted' },
        calls: [
          ...['own summary', 'own receipt', 'own target', ...(recipient ? ['personal invitation'] : [])].map(
            (label) => ({ label, status: 200 }),
          ),
          { label: 'unrelated account', status: 404 },
          { label: 'unrelated account/target', status: 403 },
        ],
      };
    });
  }
  const original = {
    passed: true,
    side: 'b',
    candidate: expected.candidate,
    baselineSourceSHA256: expected.baseline,
    consoleSHA256: expected.consoleSHA256,
    sourceDirectory: folder,
    snapshot: expected.snapshot,
    roleSQL: { control: digest('control') },
    actors: { a: actors('a'), b: actors('b') },
  };
  function admits(edit) {
    const record = structuredClone(original);
    edit?.(record);
    writeFileSync(file, JSON.stringify(record));
    const pointer = { passed: true, path: file, sha256: digest(readFileSync(file)) };
    return verifiedVisualInvitations(pointer, folder, expected);
  }
  assert.equal(admits(), true);
  for (const invalidate of [
    (record) => {
      record.actors.b[0].calls[0].status = 503;
    },
    (record) => {
      record.actors.a[2].calls.at(-1).status = 200;
    },
    (record) => {
      record.actors.b[2].calls.pop();
    },
    (record) => {
      record.actors.b[1].projectedTenantSHA256 = digest('unrelated tenant');
    },
    (record) => {
      record.actors.b[1].canAccept = true;
    },
    (record) => {
      record.actors.b[2].target.kind = 'tenant_invitation';
    },
    (record) => {
      record.actors.a[0].summary.categoryTotal = 1;
    },
    (record) => {
      record.actors.a[0].summary.categories = { organization: 1, security: 0, tasks: 2, updates: 0 };
    },
    (record) => {
      record.actors.a[0].receipt.read = true;
    },
    (record) => {
      record.actors.b[2].calls.at(-1).status = 404;
    },
    (record) => {
      record.actors.b[0].receipt.redacted = true;
    },
    (record) => {
      record.actors.a[0].inputSHA256 = record.actors.a[1].inputSHA256;
    },
    (record) => {
      record.candidate = digest('previous candidate');
    },
    (record) => {
      record.roleSQL.control = digest('different deployment');
    },
  ])
    assert.equal(admits(invalidate), false);
  assert.equal(
    verifiedVisualInvitations({ passed: true, path: file, sha256: digest('wrong bytes') }, folder, expected),
    false,
  );
});
