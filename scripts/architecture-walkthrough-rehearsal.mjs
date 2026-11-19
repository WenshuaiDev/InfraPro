import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { client, digest, fileLease, read, runDirectory, sleep, write } from './architecture-lab.mjs';
import { startSide } from './architecture-serve.mjs';
import { walkthroughInputs } from './architecture-walkthrough.mjs';
import { changeDependency, liveWalkthroughStatus, walkthroughControl } from './architecture-walkthrough-control.mjs';
import { archiveWalkthroughRecord } from './architecture-walkthrough-evidence.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';

async function observation(diag, probe) {
  const response = await diag.get('/api/diagnostics/v1/checks/' + probe);
  try {
    assert.equal(response.status(), 200, probe);
    return (await response.json()).data.current_observation;
  } finally {
    await response.dispose();
  }
}

async function messageProgress(diag, since) {
  const reports = [
    ['event-relay-report', ['event.outbox_pending', 'event.outbox_oldest_ms'], ['event.publish_acks']],
    ['task-index-worker-report', ['event.pg_reconciliations'], ['event.consumer_position']],
  ];
  const snapshots = await Promise.all(reports.map(async ([probe, required, optional]) => {
    const until = Date.now() + 65000;
    do {
      const value = await observation(diag, probe);
      const keys = [...required, ...optional];
      const metrics = Object.fromEntries(value.metrics
        .filter(m => m.availability === 'available' && !m.dimension_id && keys.includes(m.key))
        .map(m => [m.key, m.value]));
      if (value.freshness === 'fresh' && Date.parse(value.observed_at) >= Date.parse(since) &&
          required.every(key => /^[0-9]+$/.test(String(metrics[key])))) {
        return { probe, observedAt: value.observed_at, outcome: value.outcome, reason: value.reason_code, metrics };
      }
      // Two independent reports share this operator's normal read allowance.
      await sleep(2000);
    } while (Date.now() < until);
    throw Error('No current queue observation for ' + probe);
  }));
  return snapshots;
}

async function processIdentity(diag, role, since, previousBoot) {
  const until = Date.now() + 65000, probe = role + '-presence';
  do {
    const value = await observation(diag, probe);
    const identity = value.process;
    if (value.freshness === 'fresh' && value.outcome === 'ok' &&
        Date.parse(value.observed_at) >= Date.parse(since) &&
        identity?.pid > 0 && identity.os_start_time_ms && identity.boot_id &&
        identity.boot_id !== previousBoot) {
      return { probe, at: value.observed_at, pid: identity.pid, started: identity.os_start_time_ms, boot: identity.boot_id };
    }
    await sleep(1200);
  } while (Date.now() < until);
  throw Error('No current process identity for ' + probe);
}

async function readAcceptedMessage(business, fixture) {
  const { receipt, unit, result, rawSHA256 } = fixture.data;
  const path = '/tenants/' + fixture.scope.tenant + '/ingestion/messages/' + unit;
  const message = await business.call(path);
  assert.equal(message.unit.id, unit);
  assert.equal(message.receipt.id, receipt);
  assert.match(message.receipt.source_sequence, /^[1-9][0-9]*$/);
  assert.match(message.receipt.current_generation, /^[1-9][0-9]*$/);
  assert.equal(message.unit.state, 'parsed');
  const results = await business.call(path + '/results');
  assert.deepEqual(results.map(item => item.id), [result]);
  const content = await business.client.get(business.api + path + '/content', { headers: business.headers });
  try {
    assert.equal(content.status(), 200);
    const raw = await content.body();
    assert.equal(digest(raw), rawSHA256);
    return { receipt, unit, result, sourceSequence: message.receipt.source_sequence, generation: message.receipt.current_generation, rawBytes: raw.length, rawSHA256 };
  } finally {
    await content.dispose();
  }
}

export async function rehearseWalkthrough(dir, fixture) {
  assert(['a', 'b'].includes(fixture.side));
  const release = await fileLease(join(dir, 'private', 'walkthrough-rehearsal-' + fixture.side + '.lock'));
  try {
    await exerciseWalkthrough(dir, fixture);
  } finally {
    await release();
  }
}

async function exerciseWalkthrough(dir, fixture) {
  const descriptor = join(dir, 'manual', 'walkthrough-' + fixture.side + '.json');
  const descriptorBytes = readFileSync(descriptor);
  assert.deepEqual(JSON.parse(descriptorBytes), JSON.parse(JSON.stringify(fixture)));
  const descriptorSHA256 = digest(descriptorBytes);
  const sharedConsoleSHA256 = sharedConsoleFor(dir, { verifyFiles: true })?.console.sha256;
  assert.equal(fixture.sharedConsoleSHA256, sharedConsoleSHA256);
  assert.equal(fixture.execution.sharedConsoleSHA256, sharedConsoleSHA256);
  const selected = join(dir, 'candidate.evaluated.json');
  const expected = fixture.side === 'a' ? read(join(dir, 'baseline.json')).source_archive_sha256
    : read(existsSync(selected) ? selected : join(dir, 'candidate.json')).inputs.sourceSHA256;
  assert.equal(fixture.releaseSHA256, expected, 'Reopen the walkthrough using the selected release');
  const running = await liveWalkthroughStatus(fixture);
  const incorrect = expected[0] === '0' ? '1' : '0';
  await assert.rejects(liveWalkthroughStatus({ ...fixture, releaseSHA256: incorrect + expected.slice(1) }), /does not target this running walkthrough/);
  if (sharedConsoleSHA256) {
    const changed = (sharedConsoleSHA256[0] === '0' ? '1' : '0') + sharedConsoleSHA256.slice(1);
    await assert.rejects(liveWalkthroughStatus({ ...fixture, sharedConsoleSHA256: changed }), /does not target this running walkthrough/);
  }
  const diag = await request.newContext({ baseURL: fixture.diagnostics });
  const records = [],
    file = join(dir, 'manual', 'operator-' + fixture.side + '.json');
  if (existsSync(file)) archiveWalkthroughRecord(dir, 'operator-' + fixture.side, read(file));
  const capture = (status) =>
    write(file, {
      environment: fixture.environment,
      side: fixture.side,
      releaseSHA256: fixture.releaseSHA256,
      ...(sharedConsoleSHA256 && { sharedConsoleSHA256, mismatchedConsoleRejected: true }),
      descriptorSHA256,
      launchID: running.launchID,
      databaseSchema: fixture.execution.schema.version,
      diagnosticBootID: fixture.execution.serverBootID,
      mismatchedReleaseRejected: true,
      status,
      records,
    });
  const sample = async (probe, available) => {
    const until = Date.now() + 65000;
    let current;
    do {
      current = await observation(diag, probe);
      const { outcome, reason_code: reason, observed_at: at } = current;
      if (
        current.freshness === 'fresh' &&
        (available ? outcome === 'ok' : ['failed', 'timeout'].includes(outcome))
      )
        return { probe, outcome, reason, at };
      await sleep(1200);
    } while (Date.now() < until);
    throw Error(
      probe +
        ' did not reach its expected state (' +
        current.outcome +
        '/' +
        current.reason_code +
        ')',
    );
  };
  try {
    const account = read(join(dir, fixture.credentials.diagnostics));
    const csrf = await (await diag.get('/api/diagnostics/v1/session/bootstrap')).json();
    const login = await diag.post('/api/diagnostics/v1/session/login', {
      headers: {
        Origin: fixture.diagnostics,
        'X-CSRF-Token': csrf.data.csrf_token,
      },
      data: { username: account.username, password: account.password },
    });
    assert.equal(login.status(), 200);
    await login.dispose();
    const self = await diag.get('/api/diagnostics/v1/self');
    try {
      assert.equal(self.status(), 200);
      assert.equal((await self.json()).data.server_boot_id, fixture.execution.serverBootID);
    } finally {
      await self.dispose();
    }
    const business = await client(
      { origin: fixture.console, api: fixture.console },
      read(join(dir, fixture.credentials.people)).admin,
    );
    try {
      const accepted = await readAcceptedMessage(business, fixture);
      for (const [name, probe] of [
        ['parser', 'parser-health'],
        ['clickhouse', 'clickhouse-protocol'],
        ...(fixture.side === 'b' ? [['broker', 'nats-server-ready']] : []),
      ]) {
        const record = { name, startedAt: new Date().toISOString(), acceptedBefore: accepted };
        records.push(record);
        capture('running');
        record.before = await sample(probe, true);
        const role = name === 'broker' ? 'nats-server' : name === 'parser' ? 'parser' : null;
        if (role) {
          record.processBefore = await processIdentity(diag, role, record.before.at);
          const original = fixture.execution.processes[role];
          record.processBefore.matchesLaunch = record.processBefore.pid === original.pid &&
            record.processBefore.started === String(original.started);
        }
        if (name === 'broker') record.messageProgressBefore = await messageProgress(diag, record.before.at);
        await changeDependency(fixture, name, 'stop');
        try {
          record.stopped = await sample(probe, false);
          const facilities = await business.call(
            '/tenants/' + fixture.scope.tenant + '/engineering/resources/facilities',
          );
          assert(facilities.items.length > 0);
          record.facilities = facilities.items.length;
          record.acceptedDuringOutage = await readAcceptedMessage(business, fixture);
          assert.deepEqual(record.acceptedDuringOutage, accepted);
          if (name === 'broker') record.messageProgressStopped = await messageProgress(diag, record.stopped.at);
        } finally {
          await changeDependency(fixture, name, 'start');
        }
        record.recovered = await sample(probe, true);
        if (role) record.processAfter = await processIdentity(diag, role, record.recovered.at, record.processBefore.boot);
        record.acceptedAfterRecovery = await readAcceptedMessage(business, fixture);
        assert.deepEqual(record.acceptedAfterRecovery, accepted);
        if (name === 'broker') record.messageProgressRecovered = await messageProgress(diag, record.recovered.at);
        capture('running');
      }
    } finally {
      await business.client.dispose();
    }
    assert.deepEqual(await liveWalkthroughStatus(fixture), running);
    assert.equal(digest(readFileSync(descriptor)), descriptorSHA256);
    capture('passed');
  } catch (error) {
    capture('failed');
    throw error;
  } finally {
    await diag.dispose();
  }
}

export async function startWalkthroughRehearsal(dir, side, releaseDirectory) {
  assert(['a', 'b'].includes(side));
  const descriptor = join(dir, 'manual', 'walkthrough-' + side + '.json');
  const environment = join(dir, 'environment-' + side + '.json');
  if (existsSync(descriptor) && existsSync(environment)) {
    const fixture = read(descriptor), serving = read(environment);
    if (serving.walkthrough && fixture.side === side && fixture.execution?.launchID &&
        serving.launchID === fixture.execution.launchID) {
      let live;
      try {
        live = await liveWalkthroughStatus(fixture);
      } catch (error) {
        if (!['ENOENT', 'ECONNREFUSED'].includes(error.code)) throw error;
      }
      if (live) {
        if (side === 'b' && releaseDirectory)
          assert.equal(fixture.releaseSHA256, read(join(releaseDirectory, 'candidate.json')).inputs.sourceSHA256);
        return rehearseWalkthrough(dir, fixture);
      }
    }
  }
  const stack = await startSide(dir, side, { walkthrough: true, releaseDirectory });
  try {
    const fixture = await walkthroughInputs(dir, stack);
    const closeControl = await walkthroughControl(stack);
    try {
      await rehearseWalkthrough(dir, fixture);
    } finally {
      await closeControl();
    }
  } finally {
    await stack.close();
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [directory, side, mode, ...extra] = process.argv.slice(2);
  assert(['a', 'b'].includes(side) && [undefined, '--start'].includes(mode) && !extra.length);
  const dir = runDirectory(directory);
  if (mode === '--start') await startWalkthroughRehearsal(dir, side);
  else await rehearseWalkthrough(dir, read(join(dir, 'manual', 'walkthrough-' + side + '.json')));
}
