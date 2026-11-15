import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { isDeepStrictEqual } from 'node:util';
const { schemas, policies } = JSON.parse(execFileSync('go', ['run', './tools/infrapro/cmd/notification-contract'], { maxBuffer: 16 * 1024 * 1024 }));
const ref = name => ({ $ref: `#/components/schemas/${name}` });
const output = { MyNotifications: 'NotificationPage', MyNotificationSummary: 'NotificationSummary', MyNotification: 'NotificationItem', ChangeMyNotification: 'NotificationItem', MyNotificationTarget: 'NotificationTarget', MyNotificationPreferences: 'NotificationPreferences', SaveMyNotificationPreferences: 'NotificationPreferences', MyTenantInvitation: 'TenantInvitationInspection', AcceptMyTenantInvitation: 'TenantAcceptanceResult' };
const input = { ChangeMyNotification: 'NotificationChange', ReadMyNotifications: 'NotificationReadAllInput', SaveMyNotificationPreferences: 'NotificationPreferences' };
schemas.NotificationFilter.properties.status.enum = ['all', 'unread', 'archived'];
schemas.NotificationFilter.properties.category.enum = ['security', 'organization', 'tasks', 'updates'];
schemas.NotificationFilter.properties.scope.enum = ['account', 'platform', 'tenant', 'project'];
schemas.NotificationFilter.properties.limit = { type: 'integer', minimum: 1, maximum: 100 };
const url = new URL('../contracts/openapi/api.yaml', import.meta.url), contract = JSON.parse(await readFile(url, 'utf8'));
Object.assign(contract.components.schemas, schemas);
for (const p of policies) {
 const method = p.Method.toLowerCase(), path = p.Path.replace(/:([a-z_]+)/g, '{$1}');
 const parameters = [...path.matchAll(/\{([^}]+)\}/g)].map(([, name]) => ({ name, in: 'path', required: true, schema: { type: 'string', format: 'uuid' } }));
 if (p.Operation === 'MyNotifications') for (const [name, schema] of Object.entries(schemas.NotificationFilter.properties)) parameters.push({ name, in: 'query', schema });
 if (p.Operation === 'MyTenantInvitations') for (const name of ['cursor', 'limit']) parameters.push({ name, in: 'query', schema: schemas.NotificationFilter.properties[name] });
 let result = ref(output[p.Operation] ?? 'NotificationPage');
 if (p.Operation === 'MyTenantInvitations') result = { type: 'object', required: ['items'], properties: { items: { type: 'array', items: ref('TenantInvitationInspection') }, next_cursor: { type: 'string' } } };
 if (p.Operation === 'ReadMyNotifications') result = { type: 'object', required: ['changed'], properties: { changed: { type: 'integer' } } };
 const responses = { 200: { description: '本人通知；按当前业务资格裁剪。', headers: { 'Cache-Control': { schema: { type: 'string', enum: ['private, no-store'] } }, 'X-Authorization-Revision': { schema: { type: 'string' } } }, content: { 'application/json': { schema: result } } } };
 for (const status of [400, 401, 403, 404, 409, 422, 503]) responses[status] = { description: '身份、资格、修订或依赖不满足要求。', content: { 'application/json': { schema: ref('Error') } } };
 const op = { operationId: p.Operation, tags: ['Personal notifications'], security: [{ sessionCookie: [] }], parameters, responses };
 if (method !== 'get') { parameters.push({ name: 'X-CSRF-Token', in: 'header', required: true, schema: { type: 'string' } }); op.requestBody = { required: true, content: { 'application/json': { schema: input[p.Operation] ? ref(input[p.Operation]) : { type: 'object', additionalProperties: false, required: ['expected_version'], properties: { expected_version: { type: 'string' } } } } } }; }
 (contract.paths[path] ??= {})[method] = op;
}
if (process.argv.includes('--check')) { if (!isDeepStrictEqual(contract, JSON.parse(await readFile(url, 'utf8')))) throw Error('Notification OpenAPI is stale. Run pnpm notifications:contracts.'); } else await writeFile(url, `${JSON.stringify(contract, null, 2)}\n`);
