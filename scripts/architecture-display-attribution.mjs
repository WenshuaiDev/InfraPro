import assert from 'node:assert/strict';
import { copyFileSync, existsSync, mkdirSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, runDirectory, write } from './architecture-lab.mjs';

const percentile = (values, p) =>
  values.length ? [...values].sort((a, b) => a - b)[Math.ceil(p * values.length) - 1] : null;
const round = (v) => Math.round(v * 10) / 10;
function waves(row) {
  const runs = [];
  for (const sample of row.samples.hot || []) {
    const at = Date.parse(sample.at);
    let group = runs.find((v) => Math.abs(v.start - at) < 350);
    if (!group) {
      group = { start: at, values: [] };
      runs.push(group);
    }
    group.values.push(sample.ms);
  }
  return runs
    .sort((a, b) => a.start - b.start)
    .map((group) => {
      const end = group.start + Math.max(...group.values),
        near = row.resources.filter(
          (r) =>
            r.roles && Date.parse(r.at) >= group.start - 2500 && Date.parse(r.at) <= end + 2500,
        );
      const earlier = row.historyOverlap
        .filter((sample) => Date.parse(sample.at) <= group.start + 100)
        .at(-1);
      return {
        start: new Date(group.start).toISOString(),
        samples: group.values.length,
        sortedMS: group.values.map(round).sort((a, b) => a - b),
        overlap: earlier ?? null,
        resourceObservations: near.map((r) => ({
          at: r.at,
          pgConnections: r.pgConnections,
          pgCPUPercent: r.databases?.[0]
            ? Number(r.databases[0].split('|')[1].replace('%', ''))
            : null,
          blocked: r.pg?.blocked ?? null,
          host: r.machine ?? null,
        })),
      };
    });
}
function phase(row) {
  const jobs = row.execution.filter((job) => job.state === 'done');
  const count = row.samples.cold?.length || 0;
  // New captures limit the actual job query to the two database timestamps
  // bracketing the cold wave. Older captures also include earlier scenes.
  const cold = jobs.slice(-count);
  const mean = (key) =>
    cold.length ? round(cold.reduce((sum, r) => sum + r[key], 0) / cold.length) : null;
  const streams = (row.samples.stream || [])
    .filter((sample) => sample.ok)
    .map((sample) => ({
      started: sample.at,
      bytes: sample.bytes,
      chunks: sample.chunks,
      totalMS: round(sample.ms),
      firstByteMS: round(sample.firstByteMs),
      remainingTransferMS: round(sample.ms - sample.firstByteMs),
    }));
  const roleLimits = {};
  for (const [role, [postgres, clickhouse, goThreads, heapMiB]] of Object.entries(
    row.budget?.roles || {},
  )) {
    const observations = row.resources.map((record) => record.roles?.[role]).filter(Boolean);
    const observed = row.budget?.processLimits?.[role];
    const manifest =
      !observed &&
      row.workspace &&
      join(row.workspace, 'topology', 'roles', role, 'architecture.json');
    const split = manifest && existsSync(manifest) ? read(manifest) : null;
    roleLimits[role] = {
      postgres,
      clickhouse,
      goThreads,
      heapMiB,
      httpInflight: observed?.httpInflight ?? split?.max_inflight?.[role] ?? null,
      ...(observed ? { pid: observed.pid, binarySHA256: observed.binarySHA256 } : {}),
      observedCPUPercentP95: percentile(
        observations.map((x) => x.cpuPercent),
        0.95,
      ),
      observedPeakRSSKiB: observations.length
        ? Math.max(...observations.map((x) => x.rssKiB))
        : null,
    };
  }
  return {
    samples: Object.fromEntries(
      Object.entries(row.samples).map(([name, s]) => [
        name,
        { count: s.length, failures: s.filter((value) => !value.ok).length },
      ]),
    ),
    roleLimits,
    objectRequests: row.budget?.objectRequests,
    streams,
    whereResourcesRan: row.infrastructure,
    hotWaves: waves(row),
    displayJobs: {
      total: row.execution.length,
      states: [...new Set(row.execution.map((v) => v.state))],
      maxAttempts: jobs.length ? Math.max(...jobs.map((v) => v.attempts)) : null,
      coldLast: count,
      meanExecutionMS: mean('execution_ms'),
      meanQueueMS: mean('queue_ms'),
      queueMaxMS: cold.length ? round(Math.max(...cold.map((v) => v.queue_ms))) : null,
    },
    metrics: row.metrics,
    budget: row.budget,
    planner: row.planner,
    submittedHistoryJobs: row.submittedHistoryJobs,
    historyFinal: row.history.at(-1) ?? null,
    coldStatements: row.coldStatements ?? null,
    failures: row.failures,
    hostPressureRecorded: row.resources.some((x) => x.machine),
    pgWaitsRecorded: row.resources.some((x) => x.pg),
  };
}
export function displayAttribution(dir) {
  const folder = join(dir, 'load/display'),
    result = read(join(folder, 'comparison.json')),
    rows = [];
  const archive = join(folder, 'campaigns', result.campaign || result.signature);
  mkdirSync(archive, { recursive: true, mode: 0o700 });
  if (result.campaign)
    for (const trial of result.rounds) {
      assert.match(
        trial.receipt,
        /^(preparation\/)?[ab]-[1-3]-(idle|history|mixed)(-[a-f0-9-]{36})?\.json$/,
      );
      const row = read(join(archive, trial.receipt));
      assert.equal(row.campaign, result.campaign);
      assert.equal(row.signature, result.signature);
      rows.push(row);
    }
  else
    for (const name of readdirSync(folder).filter((n) =>
      /^[ab]-[1-3]-(idle|history|mixed)\.json$/.test(n),
    )) {
      const row = read(join(folder, name));
      if (row.signature !== result.signature) continue;
      rows.push(row);
      if (!existsSync(join(archive, name))) copyFileSync(join(folder, name), join(archive, name));
    }
  assert.equal(rows.length, result.rounds.length);
  const output = {
    signature: result.signature,
    complete: rows.length === 18,
    passed: result.passed,
    rounds: {},
    paired: [],
  };
  for (const row of rows) output.rounds[row.side + '-' + row.round + '-' + row.scene] = phase(row);
  for (const a of rows.filter((v) => v.side === 'a')) {
    const b = rows.find((v) => v.side === 'b' && v.round === a.round && v.scene === a.scene);
    if (!b) continue;
    const ratios = {};
    for (const [name, value] of Object.entries(a.metrics))
      if (b.metrics[name]?.count && value.count)
        ratios[name] = {
          a: value.p95Ms,
          b: b.metrics[name].p95Ms,
          ratio: b.metrics[name].p95Ms / value.p95Ms,
        };
    output.paired.push({ scene: a.scene, round: a.round, ratios });
  }
  const group = rows
    .filter((v) => v.scene === 'history' && v.side === 'a' && v.metrics.hot?.count)
    .map((v) => v.metrics.hot.p95Ms);
  output.observedFrozenHistoryP95MS = {
    rounds: group,
    median: group.length === 3 ? percentile(group, 0.5) : null,
  };
  write(join(archive, 'comparison.json'), result);
  write(join(archive, 'attribution.json'), output);
  return output;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url))
  displayAttribution(runDirectory(process.argv[2]));
