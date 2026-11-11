// Uses only the dedicated manual fixture, and rolls back all synthetic rows.
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { environment, readPrivate, root } from '../project-resource-acceptance/environment.mjs';

const env = environment();
const fixture = readPrivate('fixture.local.json').data;
const actor = readPrivate('accounts.local.json').tenant.id;
const { tenant, A: project, F109: facility } = fixture;
for (const value of [tenant, project, facility, actor]) if (!/^[0-9a-f-]{36}$/.test(value)) throw new Error('Invalid fixture identity');
const database = new URL(env.INFRAPRO_DATABASE_URL).pathname.slice(1);
if (database !== 'infrapro_project_resource_acceptance_20261111') throw new Error('Dedicated acceptance database required');
const sql = `BEGIN;
SET LOCAL statement_timeout='30s';
INSERT INTO engineering_points(tenant_id,facility_id,code,name,created_by,updated_by)
 SELECT '${tenant}','${facility}','QUERY-PLAN-'||n,'查询计划临时测点 '||n,'${actor}','${actor}' FROM generate_series(1,2000) n;
INSERT INTO project_resource_grants(tenant_id,project_id,facility_id,link_id,resource_kind,point_id,source,granted_by)
 SELECT '${tenant}','${project}','${facility}',l.id,'point',p.id,'tenant_grant','${actor}'
 FROM engineering_points p JOIN project_facility_links l ON l.facility_id=p.facility_id
 WHERE p.code LIKE 'QUERY-PLAN-%' AND p.facility_id='${facility}' AND l.project_id='${project}' AND l.status='active'
 ORDER BY p.code LIMIT 1000;
ANALYZE engineering_points; ANALYZE project_resource_grants;
EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON)
 WITH visible AS MATERIALIZED (SELECT * FROM project_resource_visibility('${tenant}'::uuid,'${project}'::uuid,'${facility}'::uuid))
 SELECT to_jsonb(r),false FROM engineering_points r
 WHERE r.tenant_id::text='${tenant}' AND r.facility_id::text='${facility}' AND r.status='normal' AND r.deleted_at IS NULL
 AND EXISTS(SELECT 1 FROM visible v WHERE v.kind='point' AND v.id=r.id)
 ORDER BY r.created_at,r.id LIMIT 26;
ROLLBACK;`;
const output = execFileSync('docker', ['compose', '-f', 'deploy/compose/test.yaml', 'exec', '-T', 'postgres', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'infrapro_test', '-d', database], { cwd: root, input: sql, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
const plan = JSON.parse(output);
if (plan[0].Plan['Actual Rows'] !== 26) throw new Error('Expected a bounded 25+1 row page');
const directory = resolve(root, '.artifacts/project-resource-query-plan');
mkdirSync(directory, { recursive: true });
writeFileSync(resolve(directory, 'report.json'), JSON.stringify({ synthetic_points: 2000, explicitly_granted_points: 1000, rolled_back: true, plan }, null, 2) + '\n');
console.log(JSON.stringify({ synthetic_points: 2000, explicitly_granted_points: 1000, result_rows: plan[0].Plan['Actual Rows'], execution_ms: plan[0]['Execution Time'], rolled_back: true }));
