import { test } from 'node:test';
import assert from 'node:assert/strict';
import { displaySummary, grantStatus, configChanges, bindingSummary, matchesDisplay } from '../../apps/console/src/features/displays/presentation.ts';

const now = Date.parse('2026-11-16T00:00:00Z');
const instance = { id: 'screen', name: '青澜 Monitor', state: 'enabled', revision: '9', current_release_id: 'live', draft_config_id: 'draft', viewers: [] };
const config = { id: 'config', package_id: 'package', parameters: { title: '桥梁', limit: 5 }, bindings: { site: { kind: 'resources', ids: ['site'] } }, mode: 'follow' };
const detail = { instance, access_url: 'http://localhost:25180/display/screen', packages: [{ id: 'package', state: 'ready', manifest: { version: '1.0.0' } }], configs: [config, { ...config, id: 'draft' }], releases: [{ id: 'live', package_id: 'package', config_id: 'config', withdrawn: false }], grant: { valid_until: '2027-01-01', revoked: false } };

test('draft and live are resolved by immutable IDs, not array order or the newest package', () => {
  const s = displaySummary({ ...detail, packages: [{ id: 'new', state: 'ready', manifest: { version: '2.0.0' } }, ...detail.packages], configs: [{ ...config, id: 'other' }, ...detail.configs] }, true, now);
  assert.equal(s.livePackage.id, 'package'); assert.equal(s.draft.id, 'draft'); assert.equal(s.canOpen, true);
});
test('view-only DTO does not imply missing grant or unpublished state', () => {
  const s = displaySummary({ instance, access_url: detail.access_url }, true, now);
  assert.equal(s.management, false); assert.equal(s.grant, undefined); assert.equal(s.status, '已发布'); assert.equal(s.canOpen, true);
});
for (const [label, patch, enabled, expected] of [
  ['disabled project', {}, false, '项目已停用'],
  ['disabled instance', { instance: { ...instance, state: 'disabled' } }, true, '已停用'],
  ['archived instance', { instance: { ...instance, state: 'archived' } }, true, '已归档'],
  ['unpublished draft', { instance: { ...instance, current_release_id: '' } }, true, '未发布'],
  ['withdrawn release', { releases: [{ ...detail.releases[0], withdrawn: true }] }, true, '已撤回'],
  ['missing package', { packages: [{ ...detail.packages[0], state: 'missing' }] }, true, '程序不可用'],
  ['missing grant', { grant: undefined }, true, '授权未设置'],
  ['expired grant', { grant: { valid_until: '2026-11-16T00:00:00Z', revoked: false } }, true, '授权已过期'],
  ['revoked grant', { grant: { ...detail.grant, revoked: true } }, true, '授权已撤销'],
]) test(`${label} blocks launch without hiding management data`, () => {
  const s = displaySummary({ ...detail, ...patch }, enabled, now);
  assert.equal(s.status, expected); assert.equal(s.canOpen, false); assert.equal(s.management, true);
});
test('invalid expiry fails closed', () => assert.equal(grantStatus({ valid_until: 'invalid', revoked: false }, now), '已过期'));
test('change summary ignores new revision and object key ordering but catches each changed domain', () => {
  assert.deepEqual(configChanges(config, { ...config, id: 'new', parameters: { limit: 5, title: '桥梁' } }), []);
  assert.deepEqual(configChanges(config, { ...config, package_id: 'new', parameters: {}, bindings: {}, mode: 'fixed' }), ['程序版本', '展示参数', '数据绑定', '时间模式']);
  assert.deepEqual(configChanges(undefined, config), []);
});
test('binding summary deduplicates references and keeps dynamic scopes explicit', () => {
  assert.equal(bindingSummary({ a: { kind: 'resources', ids: ['same'] }, b: { kind: 'resources', ids: ['same', 'other'] }, c: { kind: 'channels', ids: [], dynamic: { facility_ids: ['site'] } } }), '2 个资源 · 动态通道范围');
});
test('status filters distinguish stopped screens and preserve trimmed case-insensitive search', () => {
  assert.equal(matchesDisplay(instance, ' monitor ', 'published'), true);
  assert.equal(matchesDisplay({ ...instance, state: 'disabled' }, '', 'published'), false);
  assert.equal(matchesDisplay({ ...instance, current_release_id: '' }, '', 'unpublished'), true);
  assert.equal(matchesDisplay({ ...instance, state: 'archived' }, '', 'archived'), true);
});
