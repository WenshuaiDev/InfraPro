import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { isDeepStrictEqual } from 'node:util';
const { schemas, policies } = JSON.parse(execFileSync('go', ['run', './tools/infrapro/cmd/task-contract'], { maxBuffer: 16 * 1024 * 1024 }));
const ref = name => ({ $ref: `#/components/schemas/${name}` });
const states = ['queued', 'running', 'waiting', 'retry_wait', 'cancelling', 'needs_action', 'succeeded', 'failed', 'cancelled', 'unknown'];
for (const name of ['TaskView', 'TaskDetail']) {
 schemas[name].properties.state.enum = states;
 schemas[name].properties.result_state.enum = ['none', 'preparing', 'available', 'invalidated', 'expired', 'missing', 'restricted', 'unknown'];
}
schemas.TaskFilter.properties.scope_kind.enum=['platform','tenant','project'];
schemas.TaskFilter.properties.mode.enum = ['mine', 'managed'];
schemas.TaskFilter.properties.group.enum = ['active', 'attention', 'ended'];
schemas.TaskFilter.properties.sort.enum = ['newest', 'oldest'];
schemas.TaskFilter.properties.type.description = '一个或多个任务类型，以逗号分隔；服务端校验、去重并按当前权限筛选。';
schemas.TaskFilter.properties.q.maxLength = 100;
schemas.TaskFilter.properties.limit = { type: 'integer', minimum: 1, maximum: 100 };
schemas.TaskCommand.properties.action.enum = ['cancel', 'retry'];
for (const schema of Object.values(schemas)) for (const [key, value] of Object.entries(schema.properties ?? {})) {
 if (['generation', 'current_generation', 'expected_generation', 'expected_control_revision', 'control_revision', 'source_fence'].includes(key)) Object.assign(value, { type: 'string', pattern: '^[0-9]+$' });
}
const outputs = { TasksList: 'TaskPage', TasksSummary: 'TaskSummary', TasksResolve: 'TaskRef', TasksDetail: 'TaskDetail', TasksExecutions: 'TaskExecutionPage', TasksEvents: 'TaskEventPage', TasksCommand: 'TaskCommandResult', TasksTarget: 'TaskTarget' };
const url = new URL('../contracts/openapi/api.yaml', import.meta.url), contract = JSON.parse(await readFile(url, 'utf8'));
Object.assign(contract.components.schemas, schemas);
for (const name of ['MonitoringJob','MonitoringImportBatch','MonitoringExportRecord','MonitoringAnalyticResult']) if (contract.components.schemas[name]?.properties) contract.components.schemas[name].properties.task_ref = ref('TaskRef');
contract.components.schemas.MonitoringJob.properties.query = {type:'object',required:['action','input'],additionalProperties:false,properties:{action:{type:'string',enum:['overview','latest','statistics','trends']},input:ref('MonitoringQuery')}};
for (const policy of policies) {
 const method = policy.Method.toLowerCase(), path = policy.Path.replace(/:([a-z_]+)/g, '{$1}');
 const parameters = [...path.matchAll(/\{([^}]+)\}/g)].map(([, name]) => ({ name, in: 'path', required: true, schema: { type: 'string', format: 'uuid' } }));
 if (['TasksList', 'TasksSummary', 'TasksCatalog', 'TasksOperations'].includes(policy.Operation)) for (const [name, schema] of Object.entries(schemas.TaskFilter.properties)) parameters.push({ name, in: 'query', schema });
 const extra = { TasksResolve: ['module', 'kind', 'source_id'], TasksDetail: ['generation'], TasksExecutions: ['cursor'], TasksEvents: ['generation', 'cursor'], TasksTarget: ['generation', 'target_kind'] }[policy.Operation] ?? [];
 for (const name of extra) parameters.push({ name, in: 'query', required: policy.Operation === 'TasksResolve' || policy.Operation === 'TasksEvents' && name === 'generation', schema: { type: 'string' } });
 let result = ref(outputs[policy.Operation] ?? 'TaskPage');
 if (['TasksCatalog', 'TasksOperations'].includes(policy.Operation)) result = { type: 'object', required: ['items'], properties: { items: { type: 'array', items: ref(policy.Operation === 'TasksCatalog' ? 'TaskDefinition' : 'TaskOperation') } } };
 const responses = { 200: { description: '按当前业务资格裁剪的任务元数据。', headers: { 'Cache-Control': { schema: { type: 'string', enum: ['private, no-store'] } } }, content: { 'application/json': { schema: result } } } };
 if (policy.Operation === 'TasksCommand') responses[202] = { ...responses[200], description: '取消请求已提交，执行尚未停止。' };
 for (const status of [400, 401, 403, 404, 409, 422, 503]) responses[status] = { description: '身份、资格、修订、候选预算或来源依赖不满足要求。', content: { 'application/json': { schema: ref('Error') } } };
 const operation = { operationId: policy.Operation, tags: ['Task center'], security: [{ sessionCookie: [] }], parameters, responses };
 if (method === 'post') { parameters.push({ name: 'X-CSRF-Token', in: 'header', required: true, schema: { type: 'string' } }); operation.requestBody = { required: true, content: { 'application/json': { schema: ref('TaskCommand') } } }; }
 (contract.paths[path] ??= {})[method] = operation;
}
if (process.argv.includes('--check')) { if (!isDeepStrictEqual(contract, JSON.parse(await readFile(url, 'utf8')))) throw Error('Task OpenAPI is stale. Run pnpm tasks:contracts.'); } else await writeFile(url, `${JSON.stringify(contract, null, 2)}\n`);
