// Read-only regression probe against an explicitly named, populated isolated fixture.
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
const schema = process.argv[2];
if (!/^identity_test_[a-z0-9]+$/.test(schema ?? '')) throw Error('Pass an owned identity_test_ schema; production schemas are not accepted');
function sql(query) {
  return execFileSync('docker', ['exec', '-i', 'infrapro-ingestion-lab-business-1', 'psql', '-U', 'ingestion_test', '-d', 'ingestion_test', '-At', '-v', 'ON_ERROR_STOP=1'], { input: `SET search_path TO ${schema};${query}`, encoding: 'utf8' });
}
const queries = {};
for (const stage of ['identify', 'parse']) {
  const source = readFileSync(`packages/go/platform/ingestion/${stage}.go`, 'utf8');
  queries[stage] = source.match(/rows, e := tx.Query\(ctx, `(WITH busy [\s\S]*?)`\)/)?.[1];
  if (!queries[stage]) throw Error(`${stage} call-site candidate query not found`);
}
const [tenant, grant] = sql("SELECT tenant_id::text,snapshot->'execution_grant'->>'id' FROM ingestion_parse_plans LIMIT 1").trim().split('\n').at(-1).split('|');
const uuid = /^[0-9a-f-]{36}$/;
if (!uuid.test(tenant ?? '') || !uuid.test(grant ?? '')) throw Error('A populated production-plan fixture is required');
queries.grant = readFileSync('packages/go/platform/ingestion/planning.go', 'utf8').match(/`(SELECT count\(\*\) FROM ingestion_parse_plans WHERE tenant_id=\$1 AND snapshot[^`]+)`/)?.[1]?.replace('$1', `'${tenant}'`).replace('$2', `'${grant}'`);
if (!queries.grant) throw Error('Grant accounting call-site query not found');
const results = {};
for (const [stage, query] of Object.entries(queries)) {
  const plans = [];
  for (let n = 0; n < 5; n++) {
    const output = sql(`EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) ${query}`);
    plans.push(JSON.parse(output.slice(output.indexOf('[')))[0]);
  }
  const times = plans.map(p => p['Execution Time']).sort((a, b) => a - b);
  results[stage] = { passed: times[2] < 25, execution_ms: times, median_ms: times[2], budget_ms: 25, plans };
}
const result = { passed: Object.values(results).every(r => r.passed), results };
mkdirSync('.artifacts/ingestion', { recursive: true });
writeFileSync('.artifacts/ingestion/queue-profile.json', JSON.stringify(result, null, 2));
console.log(JSON.stringify({ passed: result.passed, stages: Object.fromEntries(Object.entries(results).map(([k, v]) => [k, { passed: v.passed, median_ms: v.median_ms, budget_ms: v.budget_ms }])) }));
if (!result.passed) process.exitCode = 1;
