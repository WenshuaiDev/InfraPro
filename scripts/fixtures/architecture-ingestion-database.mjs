import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pairedRoleConfigs } from '../architecture-http-sandbox.mjs';
import { inventory } from '../architecture-runtime.mjs';
import { ingestionTestEnv } from './ingestion-environment.mjs';

const admin = new URL(ingestionTestEnv.INFRAPRO_TEST_DATABASE_URL);
const container = ingestionTestEnv.INFRAPRO_INGESTION_TEST_BUSINESS_CONTAINER;
const read = (path) => JSON.parse(readFileSync(path, 'utf8'));
const write = (path, value) =>
  writeFileSync(path, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
export const grantFiles = [
  'deploy/architecture/restore-base-grants.sql',
  'deploy/display/package-worker-role.sql',
  'deploy/architecture/postgres-roles.sql',
  'deploy/events/postgres-grants.sql',
];
export const workerPG = { 'ingest-custody-worker': 3, 'parsing-dispatch-worker': 8, 'event-relay': 1 };

function sql(database, statement) {
  assert.ok(database === admin.pathname.slice(1) || /^arc_ing_[0-9a-f]{20}_test$/.test(database));
  const args = ['exec', '-i', container, 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1', '-U', admin.username, '-d', database];
  return execFileSync('docker', args, {
    input: statement,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'pipe'],
  }).trim();
}

export function createBusinessDatabase(directory) {
  const name = 'arc_ing_' + createHash('sha256').update(directory).digest('hex').slice(0, 20) + '_test';
  sql(admin.pathname.slice(1), `CREATE DATABASE ${name} TEMPLATE template0;
    REVOKE CONNECT ON DATABASE ${name} FROM PUBLIC;`);
  const url = new URL(admin);
  url.pathname = '/' + name;
  write(join(directory, 'database.json'), { database: name, container, created: new Date().toISOString() });
  return url.toString();
}

export function prepareWorkerRoles(directory, fixture) {
  const privateDir = join(directory, 'private');
  const cfg = read(join(privateDir, 'service.json'));
  const database = read(join(directory, 'database.json')).database;
  assert.equal(new URL(cfg.database_url).pathname.slice(1), database);
  assert.match(fixture.schema, /^identity_test_[a-z0-9]+$/);
  // The original Go fixture migrates a fresh schema. Only this round's private
  // database inherits that namespace when the release's psql installer connects.
  sql(database, `ALTER DATABASE ${database} SET search_path TO ${fixture.schema};`);
  const roles = Object.keys(workerPG);
  const { roleEnv, appliedRoleSQL } = pairedRoleConfigs(privateDir, cfg, container, roles);
  assert.deepEqual(Object.keys(appliedRoleSQL).sort(), [...grantFiles].sort(), 'Unexpected release DCL files');
  const issued = roles.map((role) => {
    const login = new URL(read(roleEnv(role).INFRAPRO_CONFIG).database_url).username;
    assert.match(login, /^arc_[a-z_]+_[a-f0-9]{12}$/);
    const group = inventory.roles[role].sqlRole;
    assert.match(group, /^infrapro_[a-z_]+$/);
    const budget = workerPG[role];
    assert.ok(budget <= inventory.roles[role].pgConnections);
    return { role, login, group, budget };
  });
  const limits = issued.flatMap(({ login, budget }) => [
    `ALTER ROLE ${login} CONNECTION LIMIT ${budget};`,
    `GRANT CONNECT ON DATABASE ${database} TO ${login};`,
  ]);
  sql(database, ['BEGIN;', ...limits, 'COMMIT;'].join('\n'));
  write(join(directory, 'worker-database-roles.json'), {
    database,
    schema: fixture.schema,
    issued,
    grant_sql_sha256: appliedRoleSQL,
  });
  return roleEnv;
}

export function checkReplicaPools(directory, fixture, filename = 'replica-pools.json') {
  const { database, schema, issued } = read(join(directory, 'worker-database-roles.json'));
  assert.match(schema, /^identity_test_[a-z0-9]+$/);
  assert.equal(schema, fixture.schema);
  assert.equal(fixture.tag, 'arc_' + schema);
  const records = issued.map(({ role, login, group, budget }) => {
    assert.match(login, /^arc_[a-z_]+_[a-f0-9]{12}$/);
    assert.equal(group, inventory.roles[role].sqlRole);
    assert.equal(budget, workerPG[role]);
    return `('${role}','${login}','${group}',${budget})`;
  });
  const rows = JSON.parse(sql(database, `SET search_path TO ${schema};
    WITH expected(role,login,expected_group,budget) AS (VALUES ${records.join(',')}),
    locks AS (SELECT *,hashtextextended('infrapro/slot/'||current_schema()||'/'||role||'/'||role||'-1',0) key FROM expected)
    SELECT jsonb_agg(to_jsonb(found) ORDER BY role) FROM (
      SELECT role,budget,login,expected_group,r.rolconnlimit connection_limit,
        r.rolsuper superuser,r.rolcreatedb create_database,r.rolcreaterole create_role,
        r.rolreplication replication,r.rolbypassrls bypass_rls,
        r.rolcanlogin AND r.rolinherit AND NOT(r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls) restricted,
        r.oid<>(SELECT datdba FROM pg_database WHERE datname=current_database()) AND NOT has_database_privilege(r.oid,current_database(),'CREATE')
          AND r.oid<>(SELECT nspowner FROM pg_namespace WHERE nspname=current_schema()) AND NOT has_schema_privilege(r.oid,current_schema(),'CREATE') non_owner,
        (SELECT count(*) FROM pg_database d WHERE d.datname~'^arc_ing_[0-9a-f]{20}_test$' AND d.datname<>current_database()
          AND has_database_privilege(r.oid,d.oid,'CONNECT')) other_round_databases,
        (SELECT jsonb_agg(g.rolname ORDER BY g.rolname) FROM pg_auth_members m JOIN pg_roles g ON g.oid=m.roleid WHERE m.member=r.oid) member_of,
        (SELECT count(*)=1 AND bool_and(m.roleid=expected_group::regrole AND NOT m.admin_option) FROM pg_auth_members m WHERE m.member=r.oid) single_group,
        (SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='${fixture.tag}' AND usesysid=r.oid) sessions,
        (SELECT count(*) FROM pg_locks l JOIN pg_stat_activity a ON a.pid=l.pid
          WHERE l.locktype='advisory' AND l.granted AND l.mode='ShareLock' AND l.objsubid=1 AND a.datname=current_database()
            AND l.classid=(key>>32 & 4294967295)::oid AND l.objid=(key & 4294967295)::oid
            AND a.application_name='${fixture.tag}' AND a.usesysid=r.oid) slot_sessions
      FROM locks JOIN pg_roles r ON r.rolname=login) found;`));
  assert.equal(rows.length, Object.keys(workerPG).length);
  assert.equal(new Set(rows.map(({ login }) => login)).size, rows.length);
  for (const row of rows) {
    assert.deepEqual(row.member_of, [row.expected_group]);
    assert.equal(row.other_round_databases, 0, 'Login can connect to another ingestion round');
    assert.ok(
      row.restricted && row.non_owner && row.single_group,
      'Unexpected database privileges for ' + row.role,
    );
    assert.equal(row.connection_limit, row.budget, 'SQL login budget for ' + row.role);
    assert.ok(
      row.sessions >= 1 && row.sessions <= row.budget && row.slot_sessions === row.sessions,
      'Unverified scoped slot for ' + row.role,
    );
  }
  write(join(directory, filename), { at: new Date().toISOString(), namespace: schema, database, roles: rows });
}

export function evidenceDatabase(directory) {
  const file = join(directory, 'database.json');
  if (!existsSync(file)) return admin.pathname.slice(1);
  const { database, container: owned } = read(file);
  assert.equal(owned, container);
  assert.match(database, /^arc_ing_[0-9a-f]{20}_test$/);
  return database;
}
