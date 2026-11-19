// Summarize only complete, independently warmed trials from one pinned campaign.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { evidenceDatabase, grantFiles, workerPG } from './fixtures/architecture-ingestion-database.mjs';

const read = (file) => JSON.parse(readFileSync(file, 'utf8'));
const stamp = (value) => Date.parse(value);
const sum = (values) => values.reduce((a, b) => a + b, 0);
const sha = (value) => createHash('sha256').update(JSON.stringify(value)).digest('hex');
const modes = ['a', 'pg', 'mq'];

export function readIngestionRelease(file, roles) {
  const bytes = readFileSync(file);
  const manifest = JSON.parse(bytes);
  assert.match(manifest.inputs?.sourceSHA256 || '', /^[a-f0-9]{64}$/, 'Missing candidate source SHA-256');
  const workers = {};
  for (const role of roles) {
    const expected = manifest.binaries?.[role];
    assert.match(expected || '', /^[a-f0-9]{64}$/, 'Missing candidate binary SHA-256 for ' + role);
    const physical = createHash('sha256').update(readFileSync(join(manifest.bin, role))).digest('hex');
    assert.equal(physical, expected, 'Selected release file differs from its manifest for ' + role);
    workers[role] = expected;
  }
  return {
    manifest: file,
    manifest_sha256: createHash('sha256').update(bytes).digest('hex'),
    source_sha256: manifest.inputs.sourceSHA256,
    workers,
  };
}

export function verifyIngestionRelease(binding, actual) {
  for (const [role, expected] of Object.entries(binding.workers))
    assert.equal(actual[role], expected, 'Measured binary differs from the selected release for ' + role);
}

export function verifyCompletedIngestionRound(directory, round, binaries, manifest) {
  const result = read(join(directory, 'result.json'));
  assert.equal(result.passed, true, 'Incomplete round ' + round.name);
  assert.equal(result.name, round.name, 'Saved result belongs to another round');
  assert.equal(result.mode, round.mode, 'Saved result uses another transport');
  assert.deepEqual(read(join(directory, 'binaries.json')), binaries, 'Saved round uses different executables');
  if (manifest)
    assert.equal(read(join(directory, 'release-binding.json')).manifest_sha256, manifest, 'Saved release differs');
}

export function verifyIngestionPause(directory, rounds, stopped, verifyCompleted) {
  let next;
  for (const round of rounds.filter((r) => !r.reused)) {
    const folder = join(directory, round.name);
    if (existsSync(join(folder, 'result.json'))) {
      assert.ok(!next, 'Completed round follows a gap in the registered sequence');
      verifyCompleted(folder, round);
    } else {
      assert.ok(!existsSync(folder), 'An unfinished round already contains raw data: ' + round.name);
      next ??= round;
    }
  }
  assert.ok(next && stopped.before === next.name, 'Pause checkpoint differs from the first unfinished round');
}

export function partitionEvidence(folder, outage, driver) {
  if (outage.duration_clock === 'monotonic') {
    assert.ok(outage.duration_seconds >= 45 && outage.duration_seconds < 46, 'Monotonic partition duration');
    assert.ok(Number.isFinite(outage.utc_seconds), 'Missing independent UTC clock difference');
    return { clock: 'monotonic', elapsed_seconds: outage.duration_seconds, utc_seconds: outage.utc_seconds };
  }
  assert.equal(outage.duration_clock, undefined, 'Unknown partition clock');
  const evidence = join(folder, 'outage-clock.json');
  if (!existsSync(evidence)) {
    assert.ok(outage.duration_seconds >= 45 && outage.duration_seconds < 46, 'Archived UTC partition duration');
    return { clock: 'utc', utc_seconds: outage.duration_seconds };
  }
  // Older drivers called UTC() before subtracting timestamps, discarding Go's
  // monotonic reading. Preserve that observation and verify the independently
  // inspected executable bound to this exact round.
  const proof = read(evidence);
  const digest = (name) => createHash('sha256').update(readFileSync(join(folder, name))).digest('hex');
  assert.equal(proof.driver_sha256, driver);
  assert.equal(proof.outage_sha256, digest('outage.json'));
  assert.equal(proof.disassembly_sha256, digest('outage-clock.asm'));
  assert.equal(proof.goarch, 'arm64');
  const code = readFileSync(join(folder, 'outage-clock.asm'), 'utf8');
  let offset = 0;
  const instructions = [
    'ORR $1, ZR, R1',
    '(*handoffLink).offline(SB)',
    'CALL time.Now(SB)',
    'MOVD $33280, R0',
    'MOVK $(31285<<16), R0',
    'MOVK $(10<<32), R0',
    'CALL time.Sleep(SB)',
    'MOVD ZR, R1',
    '(*handoffLink).offline(SB)',
    'CALL time.Now(SB)',
  ];
  for (const operation of instructions) {
    const position = code.indexOf(operation, offset);
    assert.ok(position >= 0, 'Frozen partition driver is missing an ordered instruction: ' + operation);
    offset = position + operation.length;
  }
  assert.equal(proof.down, outage.down);
  assert.equal(proof.restored, outage.restored);
  assert.equal(proof.utc_seconds, outage.duration_seconds);
  assert.equal(proof.sleep_nanoseconds, 45_000_000_000);
  assert.equal(proof.closed_before_sleep_and_reopened_after_return, true);
  assert.ok(outage.duration_seconds > 0 && outage.duration_seconds < 46, 'Archived partition UTC bounds');
  return {
    clock: 'utc_with_verified_monotonic_sleep',
    utc_seconds: outage.duration_seconds,
    monotonic_minimum_seconds: proof.sleep_nanoseconds / 1e9,
    evidence: 'outage-clock.json',
  };
}

export function campaignRounds(dir, campaign) {
  const file = join(dir, 'campaigns', campaign, 'reused-baseline.json');
  const saved = existsSync(file) ? read(file) : null;
  const baselines = saved?.round_names ?? (saved ? [saved.round] : []);
  return [1, 2, 3].flatMap((ordinal) =>
    modes.map((mode) => {
      const original = mode === 'a' && baselines[ordinal - 1];
      return { mode, ordinal, name: original || mode + '-' + campaign + ordinal, reused: !!original };
    }),
  );
}

function percentile(values, fraction) {
  assert.ok(values.length && values.every(Number.isFinite), 'Incomplete percentile population');
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.ceil(fraction * sorted.length) - 1];
}
function distribution(values) {
  return {
    count: values.length,
    p50: percentile(values, 0.5),
    p95: percentile(values, 0.95),
    p99: percentile(values, 0.99),
    max: Math.max(...values),
  };
}
function bytes(display) {
  const match = /^([\d.]+)\s*([KMGT]i?B|B)$/i.exec(display.trim());
  assert.ok(match, 'Unrecognized Docker byte quantity ' + display);
  const powers = { b: 0, kb: 1, kib: 1, mb: 2, mib: 2, gb: 3, gib: 3, tb: 4, tib: 4 };
  const unit = match[2].toLowerCase();
  return Number(match[1]) * (unit.includes('i') ? 1024 : 1000) ** powers[unit];
}
function resources(file, begin, end) {
  const all = readFileSync(file, 'utf8')
    .trim()
    .split('\n')
    .map((line) => JSON.parse(line));
  const valid = all.filter(
    (s) =>
      Array.isArray(s.containers) &&
      s.containers.length === 3 &&
      Object.keys(s.processes || {}).length &&
      !s.processes.error &&
      Number.isFinite(s.postgres?.connections),
  );
  const before = valid.findLast((s) => stamp(s.at) <= begin);
  const after = valid.find((s) => stamp(s.at) >= end);
  const samples = [before, ...valid.filter((s) => stamp(s.at) > begin && stamp(s.at) < end), after].filter(
    Boolean,
  );
  assert.ok(
    samples.length >= 2 &&
      Math.abs(stamp(samples[0].at) - begin) < 12000 &&
      Math.abs(stamp(samples.at(-1).at) - end) < 12000,
    'Measurements do not cover the observation window',
  );
  const oldest = samples.map((s) => s.domain?.OldestSeconds || 0);
  const roleCounters = (s) => {
    const values = Object.values(s.processes);
    return {
      cpu: sum(values.map((p) => (p.cpu_ns + p.completed_child_cpu_ns) / 1e9)),
      rss: sum(values.map((p) => p.rss_kib * 1024)),
      read: sum(values.map((p) => p.disk_read_bytes || 0)),
      written: sum(values.map((p) => p.disk_written_bytes || 0)),
    };
  };
  const container = (s) => {
    const io = s.containers.map((c) => c.BlockIO.split('/').map(bytes));
    return {
      cpu: sum(s.containers.map((c) => Number.parseFloat(c.CPUPerc) / 100)),
      rss: sum(s.containers.map((c) => bytes(c.MemUsage.split('/')[0]))),
      read: sum(io.map((n) => n[0])),
      written: sum(io.map((n) => n[1])),
    };
  };
  let serviceCpu = 0,
    containerCpu = 0,
    readBytes = 0,
    writtenBytes = 0,
    sampledSeconds = 0,
    maxGapSeconds = 0;
  for (let i = 1; i < samples.length; i++) {
    const previous = samples[i - 1],
      next = samples[i],
      a = stamp(previous.at),
      b = stamp(next.at);
    const covered = Math.max(0, Math.min(b, end) - Math.max(a, begin)) / 1000;
    if (!covered) continue;
    const seconds = (b - a) / 1000,
      weight = covered / seconds;
    maxGapSeconds = Math.max(maxGapSeconds, seconds);
    sampledSeconds += covered;
    const p = roleCounters(previous),
      q = roleCounters(next),
      c = container(previous),
      d = container(next);
    serviceCpu += Math.max(0, q.cpu - p.cpu) * weight;
    containerCpu += ((c.cpu + d.cpu) / 2) * covered;
    readBytes += Math.max(0, q.read - p.read) * weight + Math.max(0, d.read - c.read) * weight;
    writtenBytes += Math.max(0, q.written - p.written) * weight + Math.max(0, d.written - c.written) * weight;
  }
  const connections = samples.map((s) => s.postgres.connections),
    waiters = samples.map((s) => s.postgres.locks_waiting);
  const loginPeaks = {};
  for (const sample of samples)
    for (const [name, count] of Object.entries(sample.postgres.by_login || {}))
      loginPeaks[name] = Math.max(loginPeaks[name] || 0, count);
  const cpu = serviceCpu + containerCpu;
  return {
    valid_samples: samples.length,
    ...(Object.keys(loginPeaks).length ? { peak_connections_by_login: loginPeaks } : {}),
    sampled_seconds: sampledSeconds,
    largest_sample_gap_seconds: maxGapSeconds,
    role_cpu_seconds: serviceCpu,
    dependency_container_cpu_seconds: containerCpu,
    total_cpu_seconds: cpu,
    peak_total_rss_and_container_bytes: Math.max(
      ...samples.map((s) => roleCounters(s).rss + container(s).rss),
    ),
    estimated_disk_read_bytes: Math.round(readBytes),
    estimated_disk_written_bytes: Math.round(writtenBytes),
    peak_worker_pg_connections: Math.max(...connections),
    peak_lock_waiters: Math.max(...waiters),
    samples_with_lock_waiters: waiters.filter(Boolean).length,
    peak_sampled_lock_wait_ms: Math.max(...samples.map((s) => s.postgres.lock_wait_ms)),
    oldest_sampled_domain_backlog_seconds: Math.max(...oldest),
  };
}
function evidenceRows(folder, statement) {
  const { schema } = read(join(folder, 'fixture.json'));
  const { started } = read(join(folder, 'start.json'));
  assert.match(schema, /^identity_test_[a-z0-9]+$/);
  assert.match(started, /^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/);
  const sql =
    'SET search_path TO ' + schema + ';' + statement.replaceAll('$START', "'" + started + "'::timestamptz");
  const raw = execFileSync(
    'docker',
    [
      'exec',
      '-i',
      'infrapro-ingestion-lab-business-1',
      'psql',
      '-qAt',
      '-U',
      'ingestion_test',
      '-d',
      evidenceDatabase(folder),
      '-v',
      'ON_ERROR_STOP=1',
    ],
    { input: sql, encoding: 'utf8', maxBuffer: 32 << 20 },
  );
  return raw
    .trim()
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
}

function deliverySamples(folder, mode, expected) {
  const file = join(folder, 'event-deliveries.json');
  let rows;
  if (existsSync(file)) rows = read(file);
  else {
    rows = evidenceRows(
      folder,
      `SELECT json_build_object('event',o.event_id,'aggregate',o.aggregate_id,'stage',w.consumer,
      'generation',o.generation_or_position,'ready',o.occurred_at,'first_handoff',c.accepted_at,'milliseconds',extract(epoch FROM c.accepted_at-o.occurred_at)*1000,
      'via',c.via,'duplicates',c.duplicates,'work_attempts',w.attempts,'outcome',w.state,'published',o.published_at,'broker_sequence',o.broker_sequence)
      FROM event_outbox o JOIN event_work w USING(restore_epoch,event_id) JOIN event_consumptions c USING(consumer,restore_epoch,event_id)
      WHERE o.occurred_at>=$START AND w.consumer IN ('identify','plan','parse') ORDER BY w.consumer,o.event_id`,
    );
    writeFileSync(file, JSON.stringify(rows) + '\n', { mode: 0o600 });
  }
  assert.equal(rows.length, expected, 'Missing or extra durable event handoff');
  assert.ok(
    rows.every(
      (v) => v.outcome === 'done' && v.milliseconds >= 0 && (mode !== 'mq' || v.broker_sequence != null),
    ),
  );
  return distribution(rows.map((v) => v.milliseconds));
}

// These intervals are collected after the load clock stops. Both frozen A
// and B already retain their first actual identity and measurement attempts.
export function stageSamples(folder) {
  const file = join(folder, 'stage-attempts.json');
  if (existsSync(file)) return read(file);
  const sql = `WITH receipts AS MATERIALIZED (SELECT id,stored_at FROM ingestion_receipts WHERE received_at>=$START),
  attempts AS MATERIALIZED (SELECT stage,target_id,min(started_at) AS started FROM ingestion_attempts GROUP BY stage,target_id),
  selected AS MATERIALIZED (SELECT u.id AS unit,u.created_at AS ready,p.id AS plan,p.created_at AS planned FROM receipts r JOIN ingestion_units u ON u.receipt_id=r.id JOIN ingestion_parse_plans p ON p.unit_id=u.id JOIN ingestion_results z ON z.plan_id=p.id),
  stages AS(
    SELECT 'identify' AS stage,r.id AS id,r.stored_at AS ready,a.started AS claimed FROM receipts r JOIN ingestion_identity_decisions d ON d.receipt_id=r.id JOIN attempts a ON a.target_id=d.id AND a.stage='identify'
    UNION ALL SELECT 'parse',s.plan,s.planned,a.started FROM selected s JOIN attempts a ON a.stage='parse' AND a.target_id=s.plan
    UNION ALL SELECT 'plan_effect',s.unit,s.ready,s.planned FROM selected s
  ) SELECT json_build_object('stage',stage,'id',id,'ready',ready,'claimed',claimed,'milliseconds',extract(epoch FROM claimed-ready)*1000) FROM stages ORDER BY stage,id`;
  const rows = evidenceRows(folder, sql);
  const counts = Object.fromEntries(
    ['identify', 'parse', 'plan_effect'].map((key) => [key, rows.filter((v) => v.stage === key).length]),
  );
  assert.deepEqual(
    counts,
    { identify: 4504, parse: 6004, plan_effect: 6004 },
    'Incomplete durable stage population',
  );
  assert.ok(rows.every((v) => Number.isFinite(v.milliseconds) && v.milliseconds >= 0));
  writeFileSync(file, JSON.stringify(rows) + '\n', { mode: 0o600 });
  return rows;
}
export function trial(dir, name, mode) {
  const folder = join(dir, name),
    get = (key) => read(join(folder, key + '.json'));
  const verified = get('verified'),
    start = get('start'),
    end = get('drained'),
    outage = get('outage'),
    bins = get('binaries'),
    done = get('result');
  const inputs = get('inputs'),
    cloud = get('cloud'),
    receipts = get('receipts'),
    frames = get('frames');
  assert.equal(done.passed, true, name);
  assert.equal(done.name, name);
  assert.equal(done.mode, mode);
  assert.equal(get('warm').readable_results, 3);
  assert.equal(sum(done.configuration.pgRoleLimits), 12);
  assert.equal(inputs.length, 4504);
  assert.equal(cloud.length, 4504);
  assert.equal(receipts.length, 4504);
  assert.equal(frames.length, 6004);
  for (const key of ['Result', 'Plan', 'Unit'])
    assert.equal(new Set(frames.map((r) => r[key])).size, frames.length);
  const byLocal = new Map(receipts.map((r) => [r.LocalID, r]));
  const byCloud = new Map(cloud.map((r) => [r.CloudID, r]));
  const projected = new Map();
  const receiptID = (c) => (c.ProducerKey ? 'https/' + c.ProducerKey : 'ddp/' + c.Sequence);
  for (const r of receipts) {
    const c = byCloud.get(r.CloudID);
    assert.ok(
      c &&
        c.LocalID === r.LocalID &&
        c.SHA === r.ObjectSHA &&
        c.EnvelopeSHA === r.EnvelopeSHA &&
        c.Epoch === r.Epoch &&
        c.Source === r.Source &&
        c.Sequence === r.Sequence &&
        r.Ack === 'acknowledged',
    );
    const k = receiptID(c);
    assert.ok(!projected.has(k));
    projected.set(k, [k, r.State, r.Error, c.SHA, []]);
  }
  for (const f of frames) {
    const r = byLocal.get(f.Receipt),
      c = byCloud.get(r.CloudID);
    assert.equal(r.Tenant, f.Tenant);
    assert.equal(r.Source, f.Source);
    assert.equal(f.Status, 'parsed');
    assert.equal(f.Value, '12.34');
    assert.equal(f.ValueUnit, 'mm');
    assert.equal(f.OutputKey, 'displacement');
    assert.equal(f.Quality, 'not_evaluated');
    projected
      .get(receiptID(c))[4]
      .push([
        f.UnitKey,
        f.Method,
        f.Version,
        f.Status,
        f.Value,
        f.ValueUnit,
        f.OutputKey,
        f.Mapping,
        f.Quality,
      ]);
  }
  for (const input of inputs) {
    const k = input.Source + '/' + (input.Key || input.Sequence),
      v = projected.get(k);
    assert.ok(
      v && v[3] === input.SHA && v[4].length === input.Frames,
      'Input is missing from selected results: ' + k,
    );
    v[4].sort((a, b) => a[0].localeCompare(b[0]));
  }
  assert.equal(verified.foreign_read_denied, 2);
  assert.equal(verified.bad_packet, 'invalid_json');
  assert.equal(verified.foreign_terminal_reason, 'terminal_unregistered');
  assert.equal(verified.foreign_terminal_state, 'awaiting_identity');
  assert.equal(verified.formal_publications + verified.cross_tenant_effects, 0);
  assert.equal(verified.authorized_results, 6004);
  assert.ok(outage.offered_fps >= 99.5 && outage.offered_fps <= 100.5);
  const partition = partitionEvidence(folder, outage, bins.driver);
  let event = null;
  if (mode !== 'a') {
    const a = get('event-start'),
      b = get('event-end'),
      n = b.Done - a.Done,
      scans = b.Scans - a.Scans;
    assert.ok(scans > 0 && n >= frames.length && b.PG + b.Broker >= n);
    const pg = b.PG - a.PG,
      broker = b.Broker - a.Broker,
      duplicate = b.Duplicate - a.Duplicate;
    if (mode === 'mq') {
      assert.ok(broker > 0, 'No delivery through the actual broker');
      assert.equal(b.Unsent, 0);
      assert.deepEqual(get('broker-tail').unacknowledged_or_waiting, { identify: 0, plan: 0, parse: 0 });
    }
    if (mode === 'pg') {
      assert.equal(broker, 0);
      assert.ok(pg > 0);
    }
    event = {
      candidate_scans: scans,
      durable_handoff_latency_ms: deliverySamples(folder, mode, n),
      completed_stage_jobs: n,
      scans_per_1000_jobs: (scans * 1000) / n,
      pg_accepted: pg,
      broker_accepted: broker,
      duplicate_deliveries: duplicate,
      duplicate_rate: duplicate / (pg + broker + duplicate),
      pending_publication: b.Unsent,
      counter_start_emitted: a.ReportAt,
      counter_end_emitted: b.ReportAt,
    };
  }
  const classification = Array.from(projected.values()).sort((a, b) => a[0].localeCompare(b[0]));
  const stages = stageSamples(folder);
  const interval = (key) =>
    distribution(stages.filter((row) => row.stage === key).map((row) => row.milliseconds));
  return {
    name,
    mode,
    passed: true,
    performance_eligible: !existsSync(join(folder, 'interference.json')),
    raw_directory: name,
    fixture: {
      ...start,
      offered_fps: outage.offered_fps,
      core_acknowledged_fps: outage.effective_fps,
      partition_seconds: outage.duration_seconds,
      partition,
    },
    counts: {
      received: receipts.length,
      confirmed: cloud.length,
      correct_authorized_results: frames.length,
      bad: 1,
      foreign: 1,
      lost: 0,
      extra_formal_observations: 0,
      cross_tenant_effects: 0,
    },
    classification_sha256: sha(classification),
    time_to_owner_readable_ms: distribution(frames.map((f) => f.E2EMs)),
    ready_to_first_effective_claim_ms: distribution(
      stages.filter((row) => row.stage !== 'plan_effect').map((row) => row.milliseconds),
    ),
    identify_ready_to_first_effective_claim_ms: interval('identify'),
    unit_ready_to_first_plan_effect_ms: interval('plan_effect'),
    plan_ready_to_first_effective_claim_ms: distribution(frames.map((f) => f.ClaimMs)),
    receipt_to_cloud_confirmation_ms: distribution(cloud.map((c) => stamp(c.Confirmed) - stamp(c.Received))),
    completion_fps: verified.completion_rate,
    catchup_seconds: verified.recovery_seconds,
    total_seconds: verified.total_seconds,
    events: event,
    resources: resources(join(folder, 'resources.jsonl'), stamp(start.started), stamp(end.at)),
    binaries: bins,
    ...(existsSync(join(folder, 'release-binding.json'))
      ? { candidate_manifest_sha256: get('release-binding').manifest_sha256 }
      : {}),
    pg_worker_budgets: done.configuration.pgRoleLimits,
    replica_slots: existsSync(join(folder, 'replica-pools.json')) ? get('replica-pools') : null,
    ...(existsSync(join(folder, 'database.json')) ? { database: get('database').database } : {}),
    ...(existsSync(join(folder, 'worker-database-roles.json')) ? {
      database_principals: get('worker-database-roles'),
      final_replica_slots: get('replica-pools-at-end'),
    } : {}),
    storage: read(
      existsSync(join(folder, 'storage.json')) ? join(folder, 'storage.json') : join(dir, 'storage.json'),
    ).container,
  };
}
export function verifyWorkerPrincipals(round, grants) {
  const { database_principals: principals } = round;
  assert.equal(principals.database, round.database);
  assert.equal(principals.issued.length, 3);
  assert.equal(new Set(principals.issued.map((x) => x.login)).size, 3);
  assert.deepEqual(Object.keys(principals.grant_sql_sha256).sort(), [...grantFiles].sort());
  if (grants) assert.deepEqual(principals.grant_sql_sha256, grants, 'Installed role grants differ');
  const peaks = round.resources.peak_connections_by_login;
  assert.deepEqual(Object.keys(peaks).sort(), principals.issued.map((r) => r.login).sort());
  for (const { login, budget } of principals.issued) assert.ok(peaks[login] >= 1 && peaks[login] <= budget);
  for (const proof of [round.replica_slots, round.final_replica_slots]) {
    assert.equal(proof.database, round.database);
    assert.equal(proof.namespace, principals.schema);
    assert.equal(proof.roles.length, principals.issued.length);
    for (const issued of principals.issued) {
      const row = proof.roles.find((r) => r.role === issued.role);
      assert.equal(row.login, issued.login);
      assert.equal(row.expected_group, issued.group);
      assert.deepEqual(row.member_of, [issued.group]);
      assert.equal(row.other_round_databases, 0);
      assert.ok(row.restricted && row.non_owner && row.single_group);
      for (const flag of ['superuser', 'create_database', 'create_role', 'replication', 'bypass_rls'])
        assert.equal(row[flag], false);
      assert.equal(row.connection_limit, workerPG[issued.role]);
      assert.equal(row.connection_limit, issued.budget);
      assert.equal(row.budget, issued.budget);
      assert.ok(row.sessions >= 1 && row.sessions <= row.budget && row.slot_sessions === row.sessions);
    }
  }
  return true;
}

export function report(dir, campaign) {
  const rounds = campaignRounds(dir, campaign).map(({ ordinal, mode, name }) => ({
    ...trial(dir, name, mode),
    ordinal,
  }));
  const registrationPath = join(dir, 'campaigns', campaign, 'registration.json');
  const registration = existsSync(registrationPath) ? read(registrationPath) : null;
  if (registration?.fresh_rounds)
    assert.deepEqual(rounds.map((r) => r.name), registration.rounds, 'Round sequence changed after registration');
  if (registration?.baseline_binaries) {
    const a = registration.baseline_binaries;
    for (const { mode, binaries } of rounds) {
      assert.equal(binaries.driver, registration.builds[mode === 'a' ? 'fixture-a' : 'fixture-b']);
      assert.equal(binaries.frozenParser, a.parser);
      assert.equal(binaries.releaseRunner, a['parser-runner']);
      assert.equal(binaries.externalRelay, a['ingest-relay']);
      if (mode === 'a') {
        assert.equal(binaries.source, registration.baseline_commit);
        assert.deepEqual(binaries.workers, { 'ingest-worker': a['ingest-worker'] });
      } else {
        for (const role of Object.keys(workerPG)) assert.equal(binaries.workers[role], registration.builds[role]);
      }
    }
  }
  const bindingPath = join(dir, 'campaigns', campaign, 'release-binding.json');
  const candidateRelease = existsSync(bindingPath) ? read(bindingPath) : null;
  if (candidateRelease)
    for (const row of rounds.filter((r) => r.mode !== 'a')) {
      verifyIngestionRelease(candidateRelease, row.binaries.workers);
      if (row.candidate_manifest_sha256)
        assert.equal(row.candidate_manifest_sha256, candidateRelease.manifest_sha256);
      row.candidate_manifest_sha256 = candidateRelease.manifest_sha256;
    }
  const medians = {};
  for (const mode of modes) {
    const values = rounds.filter((r) => r.mode === mode);
    assert.equal(
      new Set(values.map((v) => sha(v.binaries))).size,
      1,
      'Mixed executable versions for ' + mode,
    );
    const mid = (getter) => percentile(values.map(getter), 0.5);
    medians[mode] = {
      end_to_end_p95_ms: mid((r) => r.time_to_owner_readable_ms.p95),
      catchup_seconds: mid((r) => r.catchup_seconds),
      ready_to_first_claim_p95_ms: mid((r) => r.ready_to_first_effective_claim_ms.p95),
      parse_only_ready_to_claim_p95_ms: mid((r) => r.plan_ready_to_first_effective_claim_ms.p95),
      scans_per_1000_jobs: mode === 'a' ? null : mid((r) => r.events.scans_per_1000_jobs),
      completion_fps: mid((r) => r.completion_fps),
    };
  }
  const a = medians.a,
    pg = medians.pg,
    mq = medians.mq;
  const net = {
    claim_p95_ratio: mq.ready_to_first_claim_p95_ms / pg.ready_to_first_claim_p95_ms,
    scan_ratio: mq.scans_per_1000_jobs / pg.scans_per_1000_jobs,
  };
  const gates = {
    independent_clean_rounds: rounds.every((r) => r.performance_eligible),
    ...(registration?.single_role_database_logins
      ? {
          isolated_worker_database_roles: rounds.filter((r) => r.mode !== 'a').every((r) => verifyWorkerPrincipals(r, registration.grant_sql_sha256)),
          observed_pg_connection_limits: rounds.every((r) =>
            r.resources.peak_worker_pg_connections >= 1 && r.resources.peak_worker_pg_connections <= sum(r.pg_worker_budgets)),
        }
      : {}),
    ...(registration?.fresh_rounds
      ? { independent_round_databases: rounds.every((r) => r.database) && new Set(rounds.map((r) => r.database)).size === 9 }
      : {}),
    bounded_replica_slots: rounds
      .filter((r) => r.mode !== 'a')
      .every(
        (r) =>
          r.replica_slots?.roles?.length === 3 &&
          r.replica_slots.roles.every((x) => x.sessions >= 1 && x.sessions <= x.budget) &&
          sum(r.replica_slots.roles.map((x) => x.budget)) === 12,
      ),
    same_storage_budget:
      new Set(
        rounds.map((r) => {
          const s = read(
            existsSync(join(dir, r.name, 'storage.json'))
              ? join(dir, r.name, 'storage.json')
              : join(dir, 'storage.json'),
          );
          return [s.image, s.cpus, s.memory, s.goMemLimit, s.maxVolumes, s.volumeMiB].join('/');
        }),
      ).size === 1,
    business_keys_and_values: new Set(rounds.map((r) => r.classification_sha256)).size === 1,
    same_parser_runner_relay: ['frozenParser', 'releaseRunner', 'externalRelay'].every(
      (k) => new Set(rounds.map((r) => r.binaries[k])).size === 1,
    ),
    same_candidate_build:
      sha(rounds.find((r) => r.mode === 'pg').binaries.workers) ===
      sha(rounds.find((r) => r.mode === 'mq').binaries.workers),
    pg_end_to_end_and_catchup:
      pg.end_to_end_p95_ms <= 1.1 * a.end_to_end_p95_ms && pg.catchup_seconds <= 1.1 * a.catchup_seconds,
    mq_end_to_end_and_catchup:
      mq.end_to_end_p95_ms <= 1.1 * a.end_to_end_p95_ms && mq.catchup_seconds <= 1.1 * a.catchup_seconds,
    mq_net_benefit:
      (net.claim_p95_ratio <= 0.8 || net.scan_ratio <= 0.8) &&
      Math.max(net.claim_p95_ratio, net.scan_ratio) <= 1.1,
  };
  const result = {
    campaign,
    passed: Object.values(gates).every(Boolean),
    comparison: 'ARC-037',
    ...(candidateRelease ? { candidate_release: candidateRelease } : {}),
    rounds,
    medians,
    gates,
    net,
    measurement: {
      percentiles: 'nearest rank across all 6004 valid frames. Median of three per-mode round statistics.',
      end_to_end:
        'Cloud receive clock to first independent observation of a committed selection, polled every 250 ms. Every selected immutable object is subsequently fetched and validated through the owner-authorized Results method.',
      claim:
        'Receipt storage or parse plan creation to first committed real identity or parse attempt for the same immutable work. Includes every 4504 receipt and 6004 selected parse plan. Planning commits its effect in its claim transaction and is reported separately.',
      scans:
        'Unkeyed legacy-domain and event-work candidate database calls, including empty calls, per 1000 completed identification, planning and parse stage jobs. Counter snapshots preserve reporter timestamps.',
      resource_scope:
        'All owned parser/runner, public relay, application workers and broker processes, business and spool PG containers, and dedicated S3. CPU and Docker block I/O use the sampled window with endpoint interpolation. Docker block counters have CLI precision.',
      user_queries: 'Interactive query and historical-load gates are tracked independently under ARC-038.',
    },
  };
  const encoded = JSON.stringify(result, null, 2) + '\n';
  writeFileSync(join(dir, 'campaigns', campaign, 'comparison.json'), encoded, { mode: 0o600 });
  writeFileSync(join(dir, 'comparison.json'), encoded, { mode: 0o600 });
  console.log(JSON.stringify({ campaign, passed: result.passed, medians, gates, net }, null, 2));
  return result;
}
