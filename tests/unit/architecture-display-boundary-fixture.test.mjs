import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { installDisplayBoundarySetup, membershipPlanner } from '../../scripts/architecture-display-boundary-setup.mjs';
import { digest, root } from '../../scripts/architecture-lab.mjs';

test('prepares all constrained original memberships and preserves the exact preview, runtime and rejection assertions', t => {
  const temp = mkdtempSync(join(tmpdir(),'infrapro-member-fixture-'));
  t.after(()=>rmSync(temp,{recursive:true,force:true}));
  const source=join(root,'packages/go/platform/resources/display_read_load_test.go');
  const code=readFileSync(source,'utf8'), start=code.indexOf('func TestDisplayReadFiftyThousandMemberBoundary(');
  const original=code.slice(start);
  const mapped=join(temp,'existing.go');
  writeFileSync(mapped,code.replace('type displaySQLCounter struct', '// retained endpoint overlay\ntype displaySQLCounter struct'));
  const replacements={[source]:mapped};
  const evidence=installDisplayBoundarySetup(root,temp,replacements,join(temp,'plans'));
  const adjusted=readFileSync(replacements[source],'utf8');
  assert.equal(adjusted.slice(adjusted.indexOf('func TestDisplayReadFiftyThousandMemberBoundary(')),original);
  assert.equal(evidence.originalAssertionsSHA256,digest(original));
  assert.deepEqual(Object.keys(replacements),[source]);
  assert.match(adjusted,/retained endpoint overlay/);
  const body=adjusted.slice(adjusted.indexOf('func displayBulkPoints'),adjusted.indexOf('func TestDisplayReadFiftyThousandMemberBoundary'));
  assert.match(body,/first \+= 500/);
  assert.match(body,/ANALYZE engineering_facilities/);
  assert.match(body,/total != end\+1 \|\| roots != 1/);
  assert.match(body,/EXPLAIN \(FORMAT JSON\) WITH visible AS MATERIALIZED/);
  assert.match(body,/EXPLAIN \(FORMAT JSON\) WITH RECURSIVE nodes AS MATERIALIZED/);
  assert.match(body,/sample := dr\.Bytes\(\[\]dr.Member\{\{Kind:"facility", ID:facility\}\}\)/);
  assert.match(body,/f\.env\.Service\.Access\(\)\.Transact/);
  assert.match(body,/statement != "10s" \|\| lock != "5s"/);
  assert.doesNotMatch(body,/disable trigger|session_replication_role|SET\s+(?:LOCAL\s+|SESSION\s+)?(?:lock|statement)_timeout/i);
  assert.match(evidence.directorySQLSHA256,/^[0-9a-f]{64}$/);
  assert.deepEqual(evidence.planner,membershipPlanner());
});

test('records a distinct matched PostgreSQL replay without changing the ordinary planner or either deadline', () => {
  assert.equal(membershipPlanner('default').pgOptions,'');
  assert.deepEqual(membershipPlanner('matched'), { profile:'matched',pgOptions:'-c enable_nestloop=off -c jit=off',enableNestloop:'off',jit:'off' });
  assert.throws(()=>membershipPlanner('suppress-timeout'));
});
