import test from 'node:test';
import assert from 'node:assert/strict';
import { issueFieldPath, derivedWorkspaceTab } from '../../apps/console/src/features/derived-monitoring/workspace-model.ts';
import { derivedSearch } from '../../apps/console/src/features/derived-monitoring/search.ts';

test('compiler and request diagnostics target rule steps without confusing binding parameters', () => {
  for (const prefix of ['', '/body', '/body/rule', '/rule']) assert.equal(issueFieldPath(`${prefix}/inputs/0/key`), '/rule/inputs/0/key');
  assert.equal(issueFieldPath('/nodes/2/params/window'), '/rule/nodes/2/params/window');
  assert.equal(issueFieldPath('/parameters/1/quantity_spec'), '/rule/parameters/1/quantity_spec');
  assert.equal(issueFieldPath('/body/parameters/window'), '/parameters/window');
  assert.equal(issueFieldPath('/body/input_bindings/x'), '/input_bindings/x');
  assert.equal(issueFieldPath('/clock/input'), '/rule/clock/input');
  assert.equal(issueFieldPath('/policies/quality'), '/rule/policies/quality');
  assert.equal(issueFieldPath('/outputs/0/numeric_contract/scale'), '/rule/outputs/0/numeric_contract/scale');
});

test('a metrics-only identity lands on definitions and an explicit view never changes identity', () => {
  assert.equal(derivedWorkspaceTab(undefined, false), 'definitions');
  assert.equal(derivedWorkspaceTab(undefined, true), 'channels');
  assert.equal(derivedWorkspaceTab('channels', false), 'channels');
  assert.equal(derivedWorkspaceTab('definitions', true), 'definitions');
});

test('unified links retain fixed version and engineering context, rejecting unsupported tab values', () => {
  const search = derivedSearch({tab:'definitions',id:'new',view:'edit',version:'v1',definition:'d1',owner:'p1',owner_kind:'point',facility:'f1',extra:'ignored'});
  assert.equal(search.tab, 'definitions');
  assert.equal(search.version, 'v1');
  assert.equal(search.owner, 'p1');
  assert.equal(search.facility, 'f1');
  assert.equal(search.extra, undefined);
  assert.equal(derivedSearch({tab:'unknown',id:1}).tab, undefined);
  assert.equal(derivedSearch({id:1}).id, undefined);
});
