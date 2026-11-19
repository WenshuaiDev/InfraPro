import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';

const spec = JSON.parse(readFileSync('docs/development/system-architecture-refactor-routes.json'));
const api = JSON.parse(readFileSync('contracts/openapi/api.yaml'));
const rows = [...spec.operations, ...spec.registered_without_openapi];
const keys = new Set();
const table = rows.map(([method, path, frozenID, owner, operation, kind]) => {
  const operationID = api.paths[path]?.[method.toLowerCase()]?.operationId;
  if (!operationID || (frozenID && operationID !== frozenID)) throw Error('Contract drift at ' + method + ' ' + path);
  keys.add(method + ' ' + path);
  return [method, path.replace(/\{(\w+)\}/g, ':$1'), owner, operationID, operation, kind].join('\t');
}).sort().join('\n') + '\n';
for (const [path, item] of Object.entries(api.paths)) {
  for (const [method, op] of Object.entries(item)) {
    if (op.operationId && !keys.has(method.toUpperCase() + ' ' + path)) throw Error('Unowned operation ' + path);
  }
}
const target = 'services/api/internal/routes/ownership.tsv';
if (process.argv.includes('--check')) {
  if (readFileSync(target, 'utf8') !== table) throw Error('Stale ' + target);
  const actual = JSON.parse(execFileSync('go', ['run', './services/api/cmd/api', 'routes'], {encoding:'utf8', maxBuffer:8 << 20}));
  const wire = op => [op.method, op.path, op.owner, op.operation_id, op.access_operation, op.access_class].join('\t');
  for (const [owner, operations] of Object.entries(actual)) {
    const expected = table.trimEnd().split('\n').filter(line => owner === 'compat-api' || line.split('\t')[2] === owner);
    if (operations.map(wire).sort().join('\n') !== expected.join('\n')) throw Error('Gin registration drift for ' + owner);
  }
  if (Object.keys(actual).length !== Object.keys(spec.registered_owner_counts).length + 1) throw Error('Missing HTTP owner');

} else writeFileSync(target, table);
console.log(rows.length + ' operations assigned.');
