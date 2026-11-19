import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { pairedRoleConfigs } from './architecture-http-sandbox.mjs';
import { read, write } from './architecture-lab.mjs';
import { inventory } from './architecture-runtime.mjs';

// Authenticate independently as every worker and HTTP owner before measuring.
// Only this fresh clone's logins may be narrowed to the fixed cohort budget.
export function displayRoleConfigs(stack, config, roles, budgets) {
  const { workspace, dbName, s, sql, lab } = stack;
  assert.match(dbName, /^arc_load_[a-f0-9]{12}_test$/);
  assert.match(s.schema, /^identity_test_[a-z0-9]+$/);
  sql('ALTER DATABASE ' + dbName + ' SET search_path TO ' + s.schema + ',public');
  const owners = roles.filter(role => role !== 'api');
  const { roleEnv, appliedRoleSQL } = pairedRoleConfigs(workspace, config, lab.container, owners, stack.roleSQLDirectory);
  const accounts = {};
  for (const role of owners) {
    const target = new URL(read(roleEnv(role).INFRAPRO_CONFIG).database_url);
    const login = decodeURIComponent(target.username), limit = budgets[role][0];
    const group = inventory.roles[role].sqlRole;
    assert.equal(target.pathname, '/' + dbName);
    assert.equal(target.searchParams.get('search_path'), s.schema);
    assert.match(login, /^arc_[a-z_]+_[a-f0-9]{12}$/);
    assert.match(group, /^infrapro_[a-z_]+$/);
    assert(limit > 0 && limit <= inventory.roles[role].pgConnections);
    sql('ALTER ROLE ' + login + ' CONNECTION LIMIT ' + limit);
    const args = ['exec', '-i', '-e', 'PGPASSWORD', lab.container, 'psql', '-XqAt',
      '--host', '127.0.0.1', '--username', login, '--dbname', dbName, '-v', 'ON_ERROR_STOP=1'];
    const input = `SELECT json_build_object('schema', current_schema(), 'database', current_database(),
      'nonprivileged', NOT (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication),
      'member', pg_has_role(current_user, '${group}', 'USAGE'),
      'limit', rolconnlimit) FROM pg_roles WHERE rolname=current_user`;
    const env = { ...process.env, PGPASSWORD: decodeURIComponent(target.password) };
    const facts = JSON.parse(execFileSync('docker', args, { encoding: 'utf8', env, input }));
    assert.deepEqual(facts, {schema:s.schema, database:dbName, nonprivileged:true, member:true, limit});
    accounts[role] = {group, limit, authenticated: true, nonprivileged: true};
  }
  const total = Object.values(accounts).reduce((sum, item) => sum + item.limit, 0);
  const gatewayDatabase = Boolean(read(roleEnv('api').INFRAPRO_CONFIG).database_url);
  assert.equal(gatewayDatabase, false);
  const receipt = {accounts, total, gatewayDatabase, appliedRoleSQL};
  write(join(workspace, 'role-budget.json'), receipt);
  return {roleEnv, receipt};
}
