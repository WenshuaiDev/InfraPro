import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { digest, read, root, runDirectory, write } from './architecture-lab.mjs';
import { currentPublicEvidence, observedPublicFixture } from './architecture-parity-evidence.mjs';

const key = (method, template) => method + ' ' + template;
const positive = (status) => status >= 200 && status < 300;
const negative = (status) => status >= 400 && status < 500;
const emptyChallenge = digest(Buffer.from('{"status":"none","policy_required":false}'));
function jsonFiles(dir) {
  return existsSync(dir)
    ? readdirSync(dir)
        .filter((f) => f.endsWith('.json'))
        .sort()
    : [];
}

// A named domain test contributes only when the original test's assertions
// passed on both releases and actually observed this response on each side.
function domainObservations(dir, side, release) {
  const folder = join(dir, 'http/domain', side),
    log = join(folder, 'http-tests.log');
  const observations = new Map();
  if (!existsSync(log)) return observations;
  const passed = new Set(
    [...readFileSync(log, 'utf8').matchAll(/^\s*--- PASS: (\S+) \(/gm)].map((m) => m[1]),
  );
  const summary = existsSync(join(folder, 'http-tests.json'))
    ? read(join(folder, 'http-tests.json'))
    : {};
  if (summary.status !== 'passed' || summary.provenance?.releaseSHA256 !== release)
    return observations;
  const records = join(folder, summary.recordDirectory || 'records');
  if (!existsSync(records)) return observations;
  for (const file of readdirSync(records)
    .filter((name) => name.endsWith('.jsonl'))
    .sort()) {
    const rows = readFileSync(join(records, file), 'utf8').trimEnd().split('\n').filter(Boolean);
    for (const [i, line] of rows.entries()) {
      const row = JSON.parse(line);
      if (!passed.has(row.Test) || (!positive(row.Status) && !negative(row.Status))) continue;
      const identity = JSON.stringify([
        row.Test,
        row.Method,
        row.Template,
        row.Status,
        row.Session,
      ]);
      if (!observations.has(identity))
        observations.set(identity, {
          row,
          source: 'http/domain/' + side + '/' + (summary.recordDirectory || 'records') + '/' + file,
          line: i + 1,
        });
    }
  }
  return observations;
}
function browserObservations(dir, side, release) {
  const pointer = join(dir, 'regression', side, 'browser-records.json'),
    seen = new Map();
  if (!existsSync(pointer)) return seen;
  const recorded = read(pointer);
  if (recorded.releaseSHA256 !== release) return seen;
  const folder = recorded.directory;
  assert.equal(dirname(folder), join(dir, 'http/domain', side));
  const cases = join(folder, 'browser-cases.jsonl');
  if (!existsSync(cases)) return seen;
  const readLines = (file) =>
    readFileSync(file, 'utf8')
      .split('\n')
      .filter(Boolean)
      .map((v) => JSON.parse(v));
  const passed = readLines(cases)
    .filter((c) => c.status === 'passed' && c.expected === 'passed')
    .sort((x, y) => x.start - y.start);
  for (const file of readdirSync(folder).filter(
    (f) => f.startsWith('requests-') && f.endsWith('.jsonl'),
  )) {
    let i = 0;
    for (const [line, r] of readLines(join(folder, file)).entries()) {
      if (r.Test !== 'TestE2EServer' || !Number.isFinite(r.Started)) continue;
      while (i < passed.length && passed[i].end < r.Started) i++;
      // Concurrent responses from the browser can complete out of order at a
      // test boundary. Only accept requests wholly inside one passing test.
      const c = passed[i];
      if (!c || r.Started < c.start || r.Finished > c.end) continue;
      const test = c.file + ' ' + c.title.join(' / '),
        k = JSON.stringify([test, r.Method, r.Template, r.Status, r.Session]);
      if (!seen.has(k))
        seen.set(k, {
          row: { ...r, Test: test },
          source: relative(dir, join(folder, file)),
          line: line + 1,
        });
    }
  }
  return seen;
}

export function writeHTTPCoverage(dir, recorded) {
  const selected = join(dir, 'candidate.evaluated.json'),
    built = join(dir, 'candidate.json');
  const release = existsSync(selected)
    ? read(selected).inputs.sourceSHA256
    : existsSync(built)
      ? read(built).inputs.sourceSHA256
      : null;
  const baseline = join(dir, 'baseline.json'),
    frozen = existsSync(baseline) ? read(baseline).source_archive_sha256 : null;
  recorded ??= currentPublicEvidence(dir, release);
  const catalog = readFileSync(join(root, 'services/api/internal/routes/ownership.tsv'), 'utf8')
    .trimEnd()
    .split('\n');
  const operations = catalog.map((line) => {
    const [method, template, owner, operationId, permission, access] = line.split('\t');
    return {
      method,
      template,
      owner,
      operationId,
      permission,
      access,
      positive: false,
      negative: false,
      evidence: [],
    };
  });
  const byKey = new Map(operations.map((op) => [key(op.method, op.template), op]));
  assert.equal(byKey.size, 1071);
  function add(op, status, entry, hidden = false) {
    if (positive(status) && !hidden) op.positive = true;
    if (negative(status) || hidden) op.negative = true;
    op.evidence.push({ ...entry, status });
  }
  const publicRecords = join(dir, 'http/operations.json');
  const publicReport = existsSync(publicRecords) ? read(publicRecords) : null;
  if (observedPublicFixture(dir, 'http/operations.json', publicReport, release, recorded))
    for (const row of publicReport.operations) {
      const op = byKey.get(key(row.method, row.template));
      assert.equal(op.owner, row.owner);
      for (const [index, c] of row.cases.entries())
        if (c.equal && c.status[0] === c.status[1]) {
          // Challenge polling permits anonymous access. Frozen clients distinguish
          // a missing credential by the exact empty view, including its policy bit.
          // Both raw HTTP bodies must be that view before counting this denial.
          const hidden =
            op.operationId === 'mfaChallenge' &&
            c.scenario === 'missing-session' &&
            c.principal === 'anonymous' &&
            c.status[0] === 200 &&
            c.body_sha256?.length === 2 &&
            c.body_sha256.every((v) => v === emptyChallenge);
          add(
            op,
            c.status[0],
            {
              source: 'http/operations.json',
              scenario: c.scenario,
              principal: c.principal,
              index,
              ...(hidden ? { denial: 'no_challenge' } : {}),
            },
            hidden,
          );
        }
    }
  for (const file of jsonFiles(join(dir, 'http/scenarios'))) {
    const source = 'http/scenarios/' + file;
    const report = read(join(dir, source));
    if (!observedPublicFixture(dir, source, report, release, recorded)) continue;
    for (const [index, c] of (report.records || []).entries()) {
      const op = byKey.get(key(c.method, c.template));
      if (!op) continue; // Private RPCs and the separate display runtime have their own ledger.
      if (c.owner) assert.equal(op.owner, c.owner);
      if (c.operationId) assert.equal(op.operationId, c.operationId);
      if (
        c.differences?.length !== 0 ||
        c.equal === false ||
        !Array.isArray(c.status) ||
        c.status[0] !== c.status[1]
      )
        continue;
      add(op, c.status[0], { source, index, scenario: c.scenario || c.label || file });
    }
  }
  for (const collect of [domainObservations, browserObservations]) {
    const a = collect(dir, 'baseline', frozen),
      b = collect(dir, 'candidate', release);
    for (const [k, first] of a) {
      const second = b.get(k);
      if (!second) continue;
      const r = first.row,
        op = byKey.get(key(r.Method, r.Template));
      assert.ok(op);
      add(op, r.Status, {
        scenario: r.Test,
        authenticated: r.Session,
        sources: [first, second].map((v) => ({ file: v.source, line: v.line })),
        assertions:
          collect === domainObservations
            ? 'original domain test passed on A and B'
            : 'original browser test passed on A and B',
      });
    }
  }
  const result = {
    candidateSHA256: release,
    baselineSHA256: frozen,
    operations,
    total: operations.length,
    positive: operations.filter((o) => o.positive).length,
    missingPositive: operations.filter((o) => !o.positive).map((o) => key(o.method, o.template)),
    rejected: operations.filter((o) => o.negative).length,
  };
  write(join(dir, 'http/coverage.json'), result);
  return result;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const summary = writeHTTPCoverage(runDirectory(process.argv[2]));
  console.log(`${summary.positive}/${summary.total} paired successful public operations`);
}
