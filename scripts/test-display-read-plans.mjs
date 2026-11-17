import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const out = resolve('.artifacts/display/monitoring-read/query-plans');
const tests = ['DirectorySnapshotAndIsolation', 'DevicesSourcesAndDistinctStatistics', 'Spatial101LocationsFixedVersionAndFiles', 'SpatialRepresentationsAndRelationships', 'WorkerFencingCancellationDeadlineAndRestore'];
const startedAt = new Date().toISOString();
mkdirSync(out, { recursive: true });
ensureTestDependencies();
execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait'], { stdio: 'inherit' });
execFileSync('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=5m', './packages/go/platform/resources', '-run', `^TestDisplayRead(${tests.join('|')})$`], {
  stdio: 'inherit',
  env: { ...process.env, ...testEnv, INFRAPRO_SPATIAL_CONTRACT: '1', INFRAPRO_DISPLAY_READ_PLAN_REPORT: out },
});
const reports = tests.map(name => JSON.parse(readFileSync(resolve(out, `TestDisplayRead${name}.json`), 'utf8')));
if (reports.some(report => !report.passed || report.distinct_explained_queries < 1)) throw Error('Missing successful query-plan evidence');
const combined = reports.flatMap(report => report.queries).map(query => query.sql).join('\n');
for (const relation of ['project_resource_visibility', 'project_device_grants', 'monitoring_source_bindings', 'project_spatial_grants', 'project_spatial_views', 'display_read_query_rows', 'display_read_scope_members', 'display_read_queries', 'display_read_jobs']) {
  if (!combined.includes(relation)) throw Error(`Query plan missing for ${relation}`);
}
writeFileSync(resolve(out, 'result.json'), JSON.stringify({ passed: true, startedAt, completedAt: new Date().toISOString(), tests: reports.map(({ test, sql_calls, distinct_explained_queries }) => ({ test, sql_calls, distinct_explained_queries })) }, null, 2));
console.log('Actual resource/device/source/spatial/query/worker SQL plans captured.');
