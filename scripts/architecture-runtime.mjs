// Shared by release packaging, deployment rendering and the two local launchers.
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const applicationInputPaths = [
  'services', 'tools/infrapro', 'packages', 'apps', 'go.work', 'go.work.sum',
  'package.json', 'pnpm-lock.yaml', 'pnpm-workspace.yaml',
  'scripts/diagnostics-build.mjs', 'scripts/prepare-document-preview.mjs',
];
export function applicationFiles(directory) {
  const files = execFileSync('git', [
    'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', ...applicationInputPaths,
  ], { cwd: directory, encoding: 'utf8', maxBuffer: 12 << 20 });
  return files.split('\0').filter(Boolean).sort();
}
export function checkoutApplicationDigest(directory) {
  return applicationSourceDigest(directory, applicationFiles(directory));
}

export function applicationSourceDigest(directory, names) {
  const hash = createHash('sha256');
  for (const name of names.filter(name =>
    applicationInputPaths.some(prefix => name === prefix || name.startsWith(prefix + '/'))).sort()) {
    const path = join(directory, name);
    const file = existsSync(path) ? createHash('sha256').update(readFileSync(path)).digest('hex') : 'deleted';
    hash.update(name).update('\0').update(file).update('\0');
  }
  return hash.digest('hex');
}

export const inventory = JSON.parse(readFileSync(join(root, 'deploy/architecture/roles.json')));
export const managed = Object.entries(inventory.roles).filter(([, spec]) =>
  spec.module === 'services/api' && !['child', 'manual'].includes(spec.mode)).map(([name]) => name);
export const httpOwners = ['identity-api', 'control-api', 'monitoring-query-api', 'transfer-api'];
export const splitWorkers = ['monitoring-publish-worker','monitoring-incremental-worker','monitoring-history-worker','monitoring-read-worker'];

export function deploymentDirectory(env) {
  return env.INFRAPRO_ROLE_CONFIG_ROOT || (env.INFRAPRO_ARCHITECTURE_FILE && dirname(resolve(env.INFRAPRO_ARCHITECTURE_FILE)));
}

export function serviceInstances(role,directory){
  if(directory&&existsSync(join(directory,'layout.json'))){
    const record=JSON.parse(readFileSync(join(directory,'layout.json'))).instances?.[role];
    if(record)return record;
  }
  return [{role,id:role+'-1',label:role,directory:'roles/'+role,pgConnections:inventory.roles[role]?.pgConnections??0}];
}

export function writeRuntimeJSON(path,record){
  const body=JSON.stringify(record)+'\n';
  try{if(readFileSync(path,'utf8')===body)return;}
  catch(e){if(e.code!=='ENOENT')throw e;}
  mkdirSync(dirname(path),{recursive:true,mode:0o700});
  const temp=path+'.'+randomUUID();
  try{writeFileSync(temp,body,{mode:0o600,flag:'wx'});renameSync(temp,path);}
  finally{rmSync(temp,{force:true});}
}

export function localReplicaProxy(directory,environment){
  if(!directory||!existsSync(join(directory,'replica-proxy.json')))return;
  const original=join(directory,'replica-proxy.json'),listeners=JSON.parse(readFileSync(original));
  const variables={api:'INFRAPRO_HTTP_ADDR','display-runtime':'INFRAPRO_DISPLAY_RUNTIME_ADDR','display-assets':'INFRAPRO_DISPLAY_ASSET_ADDR',parser:'INFRAPRO_PARSER_ADDR'};
  let adjusted=false;
  for(const entry of listeners){
    const addr=environment[variables[entry.role]];
    if(entry.protocol!=='external-http'||!addr)continue;
    const withHost=addr.startsWith(':')?'127.0.0.1'+addr:addr;
    const parsed=new URL('http://'+withHost);
    const host=parsed.hostname.replace(/^\[|\]$/g,''),port=Number(parsed.port||80);
    if(host!==entry.listen.host||port!==entry.listen.port){
      entry.listen={host,port};adjusted=true;
    }
  }
  if(!adjusted)return original;
  const local=join(directory,'replica-proxy.local.json');writeRuntimeJSON(local,listeners);
  return local;
}

function configurationGroups(role) {
  const info = inventory.roles[role];
  const groups = new Set(['LOG_', 'HTTP_ADDR', 'SHUTDOWN_TIMEOUT', 'DIAGNOSTICS_']);
  const add = (...items) => items.forEach(item => groups.add(item));
  if (info.pgConnections > 0 && role !== 'ingest-relay') add('DATABASE_URL');
  if (info.dependencies.includes('redis')) add('REDIS_');
  if (info.dependencies.includes('ch')) add('MONITORING_CLICKHOUSE_', 'MONITORING_PHYSICAL_', 'MONITORING_ONLINE_', 'MONITORING_HOT_');
  if (Object.keys(info.objects).some(key => key.startsWith('source:'))) add('MONITORING_SOURCE_', 'MONITORING_OBJECT_', 'MONITORING_TEMP_');
  if (Object.keys(info.objects).some(key => key.startsWith('files:')) || role.startsWith('files-')) add('FILES_');
  if (Object.keys(info.objects).some(key => key.startsWith('audit:'))) add('AUDIT_');
  if (Object.keys(info.objects).some(key => key.startsWith('parsing:'))) add('PARSING_OBJECT_','PARSING_BUCKET');
  if (info.dependencies.includes('parser')) add('PARSING_');
  if (['ingest-custody-worker','ingest-worker','compat-api','control-api','parsing-dispatch-worker','transfer-api'].includes(role)) add('INGESTION_FILE');
  if (role === 'identity-api' || role === 'compat-api') add('MFA_', 'SUPPORT_CONTACT','INVITATION_', 'RESET_', 'PASSWORD_');
  if ([...httpOwners, 'compat-api', 'api', 'display-runtime','display-assets'].includes(role)) add('PUBLIC_CONSOLE_URL','TRUSTED_','COOKIE_SECURE','SESSION_','DISPLAY_');
  if ([...httpOwners, 'compat-api','api'].includes(role)) add('CSRF_AUTH_KEY','CREDENTIAL_REQUEST_LIMIT','IP_REQUEST_LIMIT','UPLOAD_');
  if (role === 'control-api' || role === 'compat-api' || role === 'spatial-worker') add('SPATIAL_');
  if (role.includes('monitoring') || role === 'control-api' || role === 'compat-api') add('DERIVED_', 'MONITORING_TASK_', 'MONITORING_EXPORT_');
  if (role === 'task-index-worker') add('TASKS_');
  if (role === 'notification-worker') add('NOTIFICATION_');
  if (role === 'display-package-worker') add('DISPLAY_','AUDIT_RETENTION');
  return [...groups];
}

// A deployed role file takes precedence over the legacy, all-in-one environment.
// Local fallback projects only fields used by this role. Reporter settings stay per process.
export function roleEnvironment(role, base, directory,instance) {
  const globals = Object.fromEntries(Object.entries(base).filter(([key]) =>
    /^(?:PATH|HOME|TMPDIR|TMP|TEMP|TZ|LANG|LC_[A-Z_]+|SSL_CERT_(?:FILE|DIR)|GOMAXPROCS|GOMEMLIMIT)$/.test(key)));
  if (!directory || !inventory.roles[role]) return { ...base };
  instance??=serviceInstances(role,directory)[0];
  const mount = join(directory,instance.directory);
  const privateRuntime = join(mount,'runtime.yaml');
  const env = { ...globals };
  if (existsSync(privateRuntime)) {
    env.INFRAPRO_CONFIG = privateRuntime;
  } else {
    const groups = configurationGroups(role);
    for (const [key,value] of Object.entries(base)) {
      if (!key.startsWith('INFRAPRO_')) continue;
      const name = key.slice('INFRAPRO_'.length);
      if (groups.some(prefix => prefix.endsWith('_') ? name.startsWith(prefix) : name === prefix)) env[key] = value;
    }
  }
  const secretEnv = join(mount,'environment.json');
  if (existsSync(secretEnv)) Object.assign(env, JSON.parse(readFileSync(secretEnv)));
  const config = join(mount,'architecture.json');
  if (existsSync(config)) env.INFRAPRO_ARCHITECTURE_FILE = config;
  else if (base.INFRAPRO_ARCHITECTURE_FILE) env.INFRAPRO_ARCHITECTURE_FILE = base.INFRAPRO_ARCHITECTURE_FILE;
  const events = join(mount,'events.json');
  if (existsSync(events)) env.INFRAPRO_EVENTS_FILE = events;
  const ingestion = join(mount,'ingestion.json');
  if (existsSync(ingestion)) env.INFRAPRO_INGESTION_FILE = ingestion;
  else if (!existsSync(privateRuntime) && env.INFRAPRO_INGESTION_FILE && ['control-api','compat-api','transfer-api','parsing-dispatch-worker'].includes(role)) {
    const source=JSON.parse(readFileSync(env.INFRAPRO_INGESTION_FILE));
    const projected=join(mount,'ingestion.generated.json');
    writeRuntimeJSON(projected,ingestionProfile(role,source));
    env.INFRAPRO_INGESTION_FILE=projected;
  }
  env.INFRAPRO_REPLICA_ID=instance.id;
  if(instance.diagnostic_node)env.INFRAPRO_DIAGNOSTICS_NODE_ID=instance.diagnostic_node;
  if(env.INFRAPRO_DIAGNOSTICS_REPORT_FILE)env.INFRAPRO_DIAGNOSTICS_REPLICA_ID=instance.id;
  if(instance.memoryMiB)env.GOMEMLIMIT??=Math.floor(instance.memoryMiB*.8)+'MiB';
  if(instance.public_address){
    const addr={api:'INFRAPRO_HTTP_ADDR','display-runtime':'INFRAPRO_DISPLAY_RUNTIME_ADDR','display-assets':'INFRAPRO_DISPLAY_ASSET_ADDR'}[role];
    if(addr)env[addr]=instance.public_address;
  }
  return env;
}

export function ingestionProfile(role, source) {
  if(['ingest-custody-worker','ingest-worker'].includes(role))return source;
  const limits={enabled:source.enabled,storage_budget_bytes:source.storage_budget_bytes,queue_budget:source.queue_budget};
  if(['control-api','compat-api'].includes(role)){
    const relays=(source.relays??[]).map(relay=>Object.fromEntries(
      ['id','configuration_ref','url','consumer_id'].map(key=>[key,relay[key]])));
    return {...limits,relays};
  }
  if(['transfer-api','parsing-dispatch-worker'].includes(role))return {...limits,relays:[]};
}

export function enabledSplitRoles(env, directory) {
  const enabled = [...httpOwners];
  if (env.INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS || existsSync(join(directory,'roles/monitoring-publish-worker/runtime.yaml'))) enabled.push(...splitWorkers);
  if (env.INFRAPRO_INGESTION_FILE || existsSync(join(directory,'roles/ingest-custody-worker/runtime.yaml'))) enabled.push('ingest-custody-worker');
  if (enabled.includes('ingest-custody-worker') || env.INFRAPRO_PARSING_ENABLED === 'true') enabled.push('parsing-dispatch-worker');
  if (existsSync(join(directory,'roles/event-relay/events.json'))) enabled.push('event-relay');
  if (env.INFRAPRO_DISPLAY_RUNTIME_ORIGIN || existsSync(join(directory,'roles/display-runtime/runtime.yaml'))) enabled.push('display-runtime','display-assets');
  return enabled;
}
