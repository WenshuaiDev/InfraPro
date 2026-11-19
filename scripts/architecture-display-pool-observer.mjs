import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { basename, join, resolve } from 'node:path';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { read, sleep, write } from './architecture-lab.mjs';

const queryFamily = `CASE
  WHEN query ILIKE '%pg_advisory%' THEN 'advisory'
  WHEN query ~* 'account_sessions|authorization_state|tenant_memberships|permission_catalog' THEN 'authority'
  WHEN query ~* 'display_read_|display_runtime|display_sessions' THEN 'display query or session'
  WHEN query ~* 'monitoring_selection|monitoring_query|monitoring_summary' THEN 'monitoring query or summary'
  WHEN query ~* 'monitoring_jobs|derived_runs|derived_generations' THEN 'background job'
  WHEN query ~* 'monitoring_tenant_states|monitoring_publications' THEN 'monitoring watermark'
  WHEN query ~* 'engineering_|project_resource_visibility' THEN 'resource scope'
  WHEN query ~* '^(begin|commit|rollback|set )' THEN 'transaction'
  ELSE 'other' END`;
const query = `SELECT coalesce(jsonb_object_agg(role, measures),'{}') FROM (
  SELECT replace(substring(usename FROM '^arc_(.+)_[0-9a-f]{12}$'),'_','-') role,
    jsonb_build_object('connected',count(*),'active',count(*) FILTER (WHERE state='active'),
      'activeTransaction',count(*) FILTER (WHERE xact_start IS NOT NULL),
      'waitingForLock',count(*) FILTER (WHERE wait_event_type='Lock'),
      'blockedByBackend',count(*) FILTER (WHERE cardinality(pg_blocking_pids(pid))>0),
      'blockedStatements',coalesce(jsonb_agg(jsonb_build_object('family',${queryFamily},'wait',wait_event,
        'fingerprint',md5(query))) FILTER (WHERE wait_event_type='Lock'),'[]'),
      'longestActiveMs',coalesce(max(extract(epoch FROM clock_timestamp()-query_start)*1000) FILTER (WHERE state='active'),0)) measures
    FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid()
      AND usename ~ '^arc_.+_[0-9a-f]{12}$' GROUP BY usename
) samples`;
const execute = promisify(execFile);

export function observeDisplayPools(workspace, file) {
  const id = basename(workspace);
  assert.match(id, /^[a-f0-9]{12}$/);
  const cohort = read(join(workspace, 'cohort.json'));
  assert.equal(cohort.side, 'b');
  assert.match(cohort.database, /^arc-display-postgres-[a-f0-9]{12}$/);
  const database = 'arc_load_' + id + '_test';
  const evidence = {
    workspace: id,
    periodMs: 750,
    startedAt: new Date().toISOString(),
    samples: [],
    errors: [],
  };
  let running = true;
  const polling = (async () => {
    while (running) {
      try {
        const { stdout } = await execute(
          'docker',
          ['exec', cohort.database, 'psql', '-XqAt', '-U', 'archtest', '-d', database, '-c', query],
          { timeout: 5000 },
        );
        evidence.samples.push({
          at: new Date().toISOString(),
          roles: JSON.parse(stdout),
        });
      } catch (error) {
        evidence.errors.push({
          at: new Date().toISOString(),
          code: error.code || error.name,
        });
      }
      await sleep(evidence.periodMs);
    }
  })();
  return async () => {
    running = false;
    await polling;
    evidence.finishedAt = new Date().toISOString();
    write(file, evidence);
    return {
      file: basename(file),
      observations: evidence.samples.length,
      errors: evidence.errors.length,
    };
  };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [workspace, file, seconds] = process.argv.slice(2);
  const duration = Number(seconds);
  assert(Number.isInteger(duration) && duration > 0 && duration <= 300);
  const finish = observeDisplayPools(resolve(workspace), resolve(file));
  await sleep(duration * 1000);
  console.log(JSON.stringify(await finish()));
}
