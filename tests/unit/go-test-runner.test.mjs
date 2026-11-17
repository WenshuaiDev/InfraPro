import { test } from 'node:test';
import assert from 'node:assert/strict';
import { planGoTests, checkGoResults } from '../../scripts/go-tests.mjs';

test('Go batches preserve all tests and fuzz seeds exactly once', () => {
  const names = ['TestFirst', 'TestSecond', 'ExampleBoundary', 'FuzzInput', 'TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke'];
  const jobs = planGoTests([{ cwd: '.', name: 'example/package', tests: names }], 2);
  assert.deepEqual(jobs.flatMap(job => job.tests).sort(), [...names].sort());
  assert.ok(jobs.every(job => job.tests.length <= 2));
  assert.equal(jobs.filter(job => !job.race).length, 1);
  assert.deepEqual(jobs.find(job => !job.race).tests, ['TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke']);
});

test('a successful process cannot hide a missing or failing Go test', () => {
  const job = { tests: ['TestExpected', 'FuzzSeed'] };
  assert.equal(checkGoResults(job, new Map([['TestExpected', 'pass']]), 0).passed, false);
  assert.equal(checkGoResults(job, new Map([['TestExpected', 'pass'], ['FuzzSeed', 'fail']]), 0).passed, false);
  assert.equal(checkGoResults(job, new Map([['TestExpected', 'pass'], ['FuzzSeed', 'skip']]), 0).passed, true);
  assert.equal(checkGoResults(job, new Map([['TestExpected', 'pass'], ['FuzzSeed', 'pass']]), 1).passed, false);
});
