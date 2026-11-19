import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { digest, fileLease, read, runDirectory, write } from './architecture-lab.mjs';
import {
  displayRoleBudget,
  displayStack,
  retainDisplayStorage,
} from './architecture-display-lab.mjs';
import { displayScene } from './architecture-display-load.mjs';
import { displayHostResources } from './architecture-display-retention.mjs';
import { displayProcessLimits, openDisplaySockets } from './architecture-display-processes.mjs';
import { observeDisplayPools } from './architecture-display-pool-observer.mjs';

export async function calibrateDisplayBudget(dir, profile, scene) {
  const unlock = await fileLease(join(dir, 'load/display/campaign.lock'), {
    wait: false,
  });
  try {
    return await measureBudget(dir, profile, scene);
  } finally {
    await unlock();
  }
}

async function measureBudget(dir, profile, scene) {
  assert(['two-transfers', 'two-transfers-two-login-threads'].includes(profile));
  assert(['idle', 'history', 'mixed'].includes(scene));
  const folder = join(dir, 'load/display');
  const selected = read(join(dir, 'candidate.evaluated.json')),
    reference = read(join(folder, 'comparison.json'));
  const snapshot = read(join(dir, 'private/display-snapshot.json')).path;
  assert(
    reference.rounds.length === 18 &&
      reference.provenance.candidate === selected.inputs.sourceSHA256,
  );
  const roleSQLDirectory = join(folder, 'campaigns', reference.campaign, 'role-sql');
  const id =
    new Date().toISOString().replaceAll(':', '-') +
    '-' +
    profile +
    '-' +
    scene +
    '-' +
    randomUUID().slice(0, 8);
  const out = join(folder, 'calibrations', id);
  mkdirSync(out, { recursive: true, mode: 0o700 });
  const result = {
    id,
    profile,
    scene,
    startedAt: new Date().toISOString(),
    passed: false,
    formal: false,
    sourceCampaign: reference.campaign,
    sourceSHA256: selected.inputs.sourceSHA256,
    roleSQL: reference.provenance.roleSQL,
    publishedSource: reference.provenance.snapshot,
    roles: displayRoleBudget('b', profile),
    sockets: [],
    errors: [],
  };
  let stack, sampling, stopObserving;
  try {
    assert.equal(digest(readFileSync(join(snapshot, 'manifest.json'))), result.publishedSource);
    stack = await displayStack(dir, snapshot, 'b', 0, {
      releaseDirectory: resolve(selected.directory),
      roleSQLDirectory,
      budgetProfile: profile,
    });
    result.workspace = stack.workspace;
    result.operatingProcesses = displayProcessLimits(
      stack,
      read(join(selected.directory, 'candidate.json')),
    );
    result.infrastructure = displayHostResources(dir);
    assert.deepEqual(
      result.infrastructure,
      read(join(folder, 'campaigns', reference.campaign, 'infrastructure.json')).resources,
    );
    result.pgRoleLogins = stack.budget.roleLogins;
    assert.deepEqual(result.pgRoleLogins.appliedRoleSQL, result.roleSQL);
    const append = async () => {
      try {
        result.sockets.push(await openDisplaySockets(stack));
      } catch (error) {
        result.errors.push('socket observation: ' + (error.code || error.name));
      }
    };
    await append();
    stopObserving = observeDisplayPools(stack.workspace, join(out, 'postgres-active.json'));
    const row = await displayScene(stack, scene, join(out, 'scene.json'), () => {
      sampling = append();
    });
    await sampling;
    await append();
    for (const observation of result.sockets)
      for (const [role, sample] of Object.entries(observation.roles)) {
        assert(sample.clickhouse <= result.roles[role][1], role + ' ClickHouse connections');
        assert(sample.postgres <= result.roles[role][0], role + ' PostgreSQL connections');
      }
    result.observedConnections =
      result.sockets.some((obs) => Object.values(obs.roles).some((s) => s.postgres > 0)) &&
      result.sockets.some((obs) => Object.values(obs.roles).some((s) => s.clickhouse > 0));
    result.absolute = row.passed;
    result.relative = Object.fromEntries(
      reference.relative
        .filter((v) => v.scene === scene)
        .map((v) => [
          v.metric,
          {
            baselineMedianP95Ms: v.baselineMedianP95Ms,
            observedP95Ms: row.metrics[v.metric]?.p95Ms ?? null,
            ratio: row.metrics[v.metric]?.p95Ms / v.baselineMedianP95Ms,
            within110: row.metrics[v.metric]?.p95Ms <= v.baselineMedianP95Ms * 1.1,
          },
        ]),
    );
    result.transfers = row.samples.stream?.map((s) => ({
      at: s.at,
      bytes: s.bytes,
      sha256: s.sha256,
      firstByteMs: s.firstByteMs,
      durationMs: s.ms,
    }));
    result.login = row.samples.login?.map((s) => ({
      at: s.at,
      ms: s.ms,
      authenticated: s.ok && s.status === 'authenticated',
    }));
    result.objectBudget = stack.budget.objectRequests;
    result.completedHistory =
      row.history?.at(-1)?.states.filter((v) => v.state === 'succeeded').length ?? 0;
    result.passed =
      row.passed &&
      result.observedConnections &&
      !result.errors.length &&
      Object.values(result.relative).every((v) => v.within110);
  } catch (error) {
    result.errors.push(
      (error.code || error.name) +
        ': ' +
        String(error.message || '')
          .split('\n')[0]
          .replace(/(?:https?|postgres(?:ql)?):\/\/\S+/g, '[endpoint]')
          .slice(0, 300),
    );
  } finally {
    if (stopObserving) {
      try {
        result.postgresActivity = await stopObserving();
      } catch (error) {
        result.passed = false;
        result.errors.push('PG observation: ' + (error.code || error.name));
      }
    }
    if (stack) {
      try {
        await stack.close();
        retainDisplayStorage(stack);
      } catch (error) {
        result.passed = false;
        result.errors.push('cleanup: ' + (error.code || error.name));
      }
    }
    result.finishedAt = new Date().toISOString();
    write(join(out, 'result.json'), result);
  }
  console.log(
    JSON.stringify({
      id,
      passed: result.passed,
      absolute: result.absolute,
      relative: result.relative,
      errors: result.errors,
    }),
  );
  return result;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = await calibrateDisplayBudget(
    runDirectory(process.argv[2]),
    process.argv[3],
    process.argv[4] || 'idle',
  );
  process.exitCode = result.passed ? 0 : 1;
}
