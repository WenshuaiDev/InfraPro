import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import openapiTS, { astToString } from 'openapi-typescript';

const root = fileURLToPath(new URL('../', import.meta.url));
const check = process.argv.includes('--check');
const source = JSON.parse(execFileSync('go', ['run', './services/diagnostics/internal/contractgen'], { cwd: root, encoding: 'utf8', maxBuffer: 16 << 20 }));
const reasons = JSON.parse(readFileSync(resolve(root, 'contracts/diagnostics/reasons.json'), 'utf8'));
const emit = (path, text) => {
  const absolute = resolve(root, path);
  if (check) {
    if (!existsSync(absolute) || readFileSync(absolute, 'utf8') !== text) throw new Error(`Diagnostic contract drift: ${path}`);
  } else { mkdirSync(dirname(absolute), { recursive: true }); writeFileSync(absolute, text); }
};
const json = (value) => `${JSON.stringify(value, null, 2)}\n`;
const files = { Manifest: 'manifest', ServerConfig: 'server', ObserverConfig: 'observer', Bindings: 'bindings', Auth: 'auth', Keyring: 'keyring', ObserverKey: 'observer-key', Credential: 'credential', ProcessReport: 'process-report', Observation: 'observation', ObserverFrame: 'observer-frame' };
for (const [type, name] of Object.entries(files)) emit(`contracts/diagnostics/${name}.schema.json`, json({ $schema: 'https://json-schema.org/draft/2020-12/schema', $id: `urn:infrapro:diagnostics:v1:${name}`, ...source.schemas[type] }));
emit('contracts/diagnostics/runtime-catalog.json', json({ schema_version: 1, entries: source.catalog }));
const schemas = source.schemas;
schemas.Error = { type: 'object', additionalProperties: false, required: ['meta', 'error'], properties: { meta: { $ref: '#/components/schemas/Meta' }, error: { type: 'object', additionalProperties: false, required: ['code', 'message_key', 'request_id', 'retry_after_ms'], properties: { code: { type: 'string', enum: reasons.reasons.map((r) => r.code) }, message_key: { type: 'string' }, request_id: { type: 'string', format: 'uuid' }, retry_after_ms: { type: 'integer', minimum: 0 } } } } };
schemas.Self = { type: 'object', required: ['build_id', 'protocol_major', 'uptime_ms', 'config_state', 'auth_state', 'store_state', 'collectors', 'limits', 'retention', 'warnings', 'manifest_revision', 'server_boot_id', 'observers', 'diagnostic_gaps'], additionalProperties: false, properties: {
  build_id: { type: 'string' }, protocol_major: { type: 'integer', const: 1 }, uptime_ms: { type: 'string' }, config_state: { type: 'string' }, auth_state: { type: 'object', required: ['initialized', 'available', 'revision'], properties: { initialized: { type: 'boolean' }, available: { type: 'boolean' }, revision: { type: 'string' } } }, store_state: { type: 'object', required: ['status', 'budget'], properties: { status: { type: 'string' }, budget: { type: ['object', 'null'], additionalProperties: { type: 'number' } } } }, collectors: { type: 'object', additionalProperties: { type: ['number', 'boolean'] } }, limits: schemas.ServerConfig.properties.limits, retention: { $ref: '#/components/schemas/Retention' }, warnings: { type: 'array', items: { type: 'string' } }, manifest_revision: { type: 'string' }, server_boot_id: { type: 'string' }, observers: { type: 'array', items: { $ref: '#/components/schemas/NodeView' } }, diagnostic_gaps: { type: 'array', items: { $ref: '#/components/schemas/Check' } },
} };
const envelope = (type) => ({ type: 'object', additionalProperties: false, required: ['meta', 'data'], properties: { meta: { $ref: '#/components/schemas/Meta' }, data: { $ref: `#/components/schemas/${type}` } } });
const page = (type) => ({ type: 'object', additionalProperties: false, required: ['items', 'total', 'next_cursor', 'snapshot_id', 'expires_at'], properties: { items: { type: 'array', items: { $ref: `#/components/schemas/${type}` } }, total: { type: 'integer', minimum: 0 }, next_cursor: { type: 'string' }, snapshot_id: { type: 'string' }, expires_at: { type: 'string', format: 'date-time' } } });
for (const type of ['NodeView', 'ComponentView', 'Transition']) schemas[`${type}Page`] = page(type);
const paths = {};
const routes = [
 ['get', '/session/bootstrap', 'Bootstrap', null, false], ['post', '/session/login', 'Session', 'LoginRequest', false], ['get', '/session', 'Session'], ['post', '/session/activity', 'Activity', 'Empty'], ['post', '/session/logout', null, 'Empty'], ['get', '/overview', 'Overview'], ['get', '/nodes', 'NodeViewPage'], ['get', '/nodes/{id}', 'NodeDetail'], ['get', '/components', 'ComponentViewPage'], ['get', '/components/{id}', 'ComponentDetail'], ['get', '/replicas/{id}', 'ReplicaView'], ['get', '/checks/{id}', 'Check'], ['get', '/transitions', 'TransitionPage'], ['get', '/metrics', 'MetricSeries'], ['get', '/self', 'Self'],
];
schemas.Empty = { type: 'object', additionalProperties: false, maxProperties: 0 };
for (const [method, path, type, input, secured = true] of routes) {
 const response = type ? { description: 'Independent diagnostic view; target failure remains HTTP 200', content: { 'application/json': { schema: envelope(type) } } } : { description: 'Signed out; no response body' };
 const operation = { operationId: `diagnostics_${method}_${path.replace(/[^a-z]/g, '_')}`, security: secured ? [{ diagnosticSession: [] }] : [], responses: { [type ? '200' : '204']: response, default: { description: 'Safe error code', content: { 'application/json': { schema: { $ref: '#/components/schemas/Error' } } } } } };
 if (input) operation.requestBody = { required: true, content: { 'application/json': { schema: { $ref: `#/components/schemas/${input}` } } } };
 if (method === 'post') operation.parameters = [{ name: 'X-CSRF-Token', in: 'header', required: true, schema: { type: 'string' } }, { name: 'Origin', in: 'header', required: true, schema: { type: 'string', format: 'uri' } }];
 if (path.includes('{id}')) operation.parameters = [{ name: 'id', in: 'path', required: true, schema: { type: 'string', pattern: '^[a-z][a-z0-9_.-]{0,63}$' } }, { name: 'snapshot_id', in: 'query', schema: { type: 'string' } }];
 const params = path === '/nodes' ? ['state', 'kind', 'cursor', 'limit'] : path === '/components' ? ['state', 'kind', 'node_id', 'cursor', 'limit'] : path === '/transitions' ? ['target_kind', 'target_id', 'from', 'to', 'cursor', 'limit'] : path === '/metrics' ? ['target_kind', 'target_id', 'metric_key', 'dimension_id', 'from', 'to'] : [];
 if (params.length) operation.parameters = params.map((name) => ({ name, in: 'query', required: path === '/metrics' && name !== 'dimension_id', schema: name === 'limit' ? { type: 'integer', minimum: 1, maximum: 100, default: 50 } : { type: 'string', maxLength: name === 'cursor' ? 2048 : 128 } }));
 paths[`/api/diagnostics/v1${path}`] = { [method]: operation };
}
for (const [path, request, response, auth] of [['sessions', 'ObserverSessionRequest', 'ObserverSession', 'observerKey'], ['reports', 'ObserverFrame', 'ObserverAck', 'observerToken']]) paths[`/observer/v1/${path}`] = { post: { operationId: `observer_${path}`, security: [{ [auth]: [] }], requestBody: { required: true, content: { 'application/json': { schema: { $ref: `#/components/schemas/${request}` } } } }, responses: { 200: { description: 'Durably accepted', content: { 'application/json': { schema: { $ref: `#/components/schemas/${response}` } } } }, default: { description: 'Safe error', content: { 'application/json': { schema: { $ref: '#/components/schemas/Error' } } } } } } };
for (const path of ['healthz', 'readyz']) paths[`/${path}`] = { get: { operationId: path, security: [], responses: { 200: { description: 'Fixed public status only', content: { 'application/json': { schema: { type: 'object', properties: { status: { type: 'string', enum: ['ok', 'ready', 'not_ready'] } }, required: ['status'], additionalProperties: false } } } }, 503: { description: 'Independent diagnostic service not ready' } } } };
const document = { openapi: '3.1.0', info: { title: 'InfraPro Independent Runtime Diagnostics', version: '1.0.0' }, paths, components: { schemas, securitySchemes: { diagnosticSession: { type: 'apiKey', in: 'cookie', name: '__Host-infrapro_diag', description: 'Loopback HTTP uses infrapro_diag_local; independent from business identity' }, observerKey: { type: 'http', scheme: 'bearer' }, observerToken: { type: 'http', scheme: 'bearer' } } } };
emit('contracts/openapi/diagnostics.yaml', json(document));
const ast = await openapiTS(document);
emit('packages/ts/diagnostics-client/src/generated.ts', astToString(ast));
emit('packages/ts/diagnostics-client/src/reasons.ts', `// Generated by scripts/diagnostics-contract.mjs.\nexport const reasons = ${JSON.stringify(Object.fromEntries(reasons.reasons.map((r) => [r.code, { title: r.title, hint: r.hint }])), null, 2)} as const;\n`);
let go = '// Code generated by scripts/diagnostics-contract.mjs; DO NOT EDIT.\npackage settings\n\ntype Reason struct { Code string `json:"code"`; Title string `json:"title"`; Hint string `json:"hint"` }\nvar Reasons = map[string]Reason{\n';
for (const r of reasons.reasons) go += `${JSON.stringify(r.code)}: {Code:${JSON.stringify(r.code)},Title:${JSON.stringify(r.title)},Hint:${JSON.stringify(r.hint)}},\n`;
go += '}\nfunc KnownReason(code string) bool { _, ok := Reasons[code]; return ok }\n';
emit('services/diagnostics/settings/reasons_generated.go', execFileSync('gofmt', [], { input: go, encoding: 'utf8' }));
console.log(`Diagnostics contracts ${check ? 'checked' : 'generated'} (${source.catalog.length} catalog entries, ${reasons.reasons.length} reasons).`);
