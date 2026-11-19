import { processEnvironment } from './architecture-process-resources.mjs';
import { deploymentDirectory, roleEnvironment, enabledSplitRoles, serviceInstances,localReplicaProxy,writeRuntimeJSON } from './architecture-runtime.mjs';
import { boundedRelayConfig } from './architecture-machines.mjs';
import { createMFAKeyring } from './mfa-keyring.mjs';
import { execFileSync, spawn } from 'node:child_process';
import { randomBytes, createHash, createHmac } from 'node:crypto';
import { mkdirSync, existsSync, readFileSync, writeFileSync, openSync, closeSync, chmodSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const directory = resolve(process.env.INFRAPRO_LOCAL_DIRECTORY || join(homedir(), '.local/share/infrapro/runtime'));
export const origin = 'http://localhost:25173';
const binary = name => join(root, 'build/bin', name);
export function privateJSON(name, value) {
  const path = join(directory, name); mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  writeFileSync(path, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 }); chmodSync(path, 0o600);
}
export const readJSON = name => JSON.parse(readFileSync(join(directory, name), 'utf8'));
export function environment() {
  const base = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_') && !key.startsWith('VITE_')));
  return { ...base, ...readJSON('environment.json'), CONSOLE_PORT: '25173', API_PROXY_TARGET: 'http://127.0.0.1:28080', VITE_API_BASE_URL: '/api/v1', VITE_APP_BASE: '/' };
}
const run = (cmd, args, env = process.env) => execFileSync(cmd, args, { cwd: root, env, stdio: 'inherit' });
const compose = args => run('docker', ['compose', '-p', 'infrapro', '-f', join(directory, 'compose.json'), ...args]);
const pause = ms => new Promise(r => setTimeout(r, ms));

async function bucket(name, endpoint, secret) {
  const host = new URL(endpoint).host, stamp = new Date().toISOString().replace(/[-:]|\.\d{3}/g, ''), date = stamp.slice(0, 8);
  const hash = value => createHash('sha256').update(value).digest('hex');
  const hmac = (key, value) => createHmac('sha256', key).update(value).digest();
  const headers = `host:${host}\nx-amz-content-sha256:${hash('')}\nx-amz-date:${stamp}\n`;
  const names = 'host;x-amz-content-sha256;x-amz-date', scope = `${date}/us-east-1/s3/aws4_request`;
  const canonical = `PUT\n/${name}\n\n${headers}\n${names}\n${hash('')}`;
  const key = hmac(hmac(hmac(hmac('AWS4' + secret, date), 'us-east-1'), 's3'), 'aws4_request');
  const signature = createHmac('sha256', key).update(`AWS4-HMAC-SHA256\n${stamp}\n${scope}\n${hash(canonical)}`).digest('hex');
  const response = await fetch(`${endpoint}/${name}`, { method: 'PUT', headers: { 'x-amz-date': stamp, 'x-amz-content-sha256': hash(''), Authorization: `AWS4-HMAC-SHA256 Credential=infrapro/${scope}, SignedHeaders=${names}, Signature=${signature}` } });
  if (![200, 409].includes(response.status)) throw Error(`Cannot initialize object bucket ${name}: ${response.status}`);
}

async function init() {
  mkdirSync(directory, { recursive: true, mode: 0o700 }); chmodSync(directory, 0o700);
  if (!existsSync(join(directory, 'environment.json'))) {
    const mfaKeyring = join(directory, 'private/mfa-keyring.json');
    if (!existsSync(mfaKeyring)) createMFAKeyring(mfaKeyring);
    const passwords = Object.fromEntries(['postgres', 'redis', 's3'].map(k => [k, randomBytes(24).toString('hex')]));
    privateJSON('service-secrets.json', passwords);
    const env = {
      INFRAPRO_DATABASE_URL: `postgres://infrapro:${passwords.postgres}@127.0.0.1:5432/infrapro`,
      INFRAPRO_REDIS_URL: `redis://:${passwords.redis}@127.0.0.1:6379/0`, INFRAPRO_REDIS_KEY_PREFIX: 'infrapro:local:identity:v1:',
      INFRAPRO_MFA_KEYRING_FILE: mfaKeyring, INFRAPRO_MFA_ISSUER: 'InfraPro',
      INFRAPRO_CSRF_AUTH_KEY: randomBytes(32).toString('hex'), INFRAPRO_HTTP_ADDR: '127.0.0.1:28080',
      INFRAPRO_PUBLIC_CONSOLE_URL: origin + '/', INFRAPRO_TRUSTED_ORIGINS: origin, INFRAPRO_COOKIE_SECURE: 'false',
      INFRAPRO_SUPPORT_CONTACT: '江南交通基础设施监测中心信息运维室',
      INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS: '127.0.0.1:61000,127.0.0.1:61001',
      INFRAPRO_MONITORING_CLICKHOUSE_DATABASE: 'infrapro_monitoring', INFRAPRO_MONITORING_CLICKHOUSE_USER: 'default',
      INFRAPRO_MONITORING_CLICKHOUSE_POLICY: 'monitoring_tiered', INFRAPRO_MONITORING_CLICKHOUSE_TLS: 'false',
      INFRAPRO_MONITORING_OBJECT_ENDPOINT: 'http://127.0.0.1:61333', INFRAPRO_MONITORING_OBJECT_ACCESS_KEY: 'infrapro', INFRAPRO_MONITORING_OBJECT_SECRET_KEY: passwords.s3,
      INFRAPRO_MONITORING_SOURCE_BUCKET: 'infrapro-source', INFRAPRO_MONITORING_TEMP_DIRECTORY: join(directory, 'temporary/monitoring'),
      INFRAPRO_MONITORING_BACKUP_ENDPOINT: 'http://127.0.0.1:61334', INFRAPRO_MONITORING_BACKUP_ACCESS_KEY: 'infrapro', INFRAPRO_MONITORING_BACKUP_SECRET_KEY: passwords.s3,
      INFRAPRO_MONITORING_BACKUP_BUCKET: 'infrapro-backups', INFRAPRO_MONITORING_BACKUP_NAMED_COLLECTION: 'monitoring_backups',
      INFRAPRO_SPATIAL_PYTHON: join(directory, 'python/bin/python'), INFRAPRO_SPATIAL_CONVERTER_SCRIPT: join(root, 'tools/spatial-converter/convert.py'), INFRAPRO_SPATIAL_TEMP_DIRECTORY: join(directory, 'temporary/spatial'),
      INFRAPRO_DISPLAY_RUNTIME_ORIGIN: 'http://localhost:25180', INFRAPRO_DISPLAY_ASSET_ORIGIN: 'http://localhost:25181',
      INFRAPRO_DISPLAY_RUNTIME_ADDR: '127.0.0.1:25180', INFRAPRO_DISPLAY_ASSET_ADDR: '127.0.0.1:25181', INFRAPRO_DISPLAY_SHELL_DIRECTORY: join(root, 'apps/display-runtime/dist'),
      INFRAPRO_FILES_OBJECT_ENDPOINT: 'http://127.0.0.1:61333', INFRAPRO_FILES_BUCKET: 'infrapro-files', INFRAPRO_FILES_ACCESS_KEY: 'infrapro', INFRAPRO_FILES_SECRET_KEY: passwords.s3,
      INFRAPRO_FILES_NODE_ID: 'files-local', INFRAPRO_FILES_TEMP_DIRECTORY: join(directory, 'temporary/files'), INFRAPRO_FILES_CHECKER_SOCKET: join(directory, 'temporary/files/checker.sock'),
      INFRAPRO_AUDIT_OBJECT_ENDPOINT: 'http://127.0.0.1:61333', INFRAPRO_AUDIT_BUCKET: 'infrapro-audit', INFRAPRO_AUDIT_ACCESS_KEY: 'infrapro', INFRAPRO_AUDIT_SECRET_KEY: passwords.s3,
      INFRAPRO_AUDIT_NODE_ID: 'audit-local', INFRAPRO_AUDIT_TEMP_DIRECTORY: join(directory, 'temporary/audit'),
    };
    privateJSON('environment.json', env);
    for (const name of ['logs', 'reports', 'engineering', 'temporary/monitoring', 'temporary/spatial', 'temporary/files', 'temporary/audit']) mkdirSync(join(directory, name), { recursive: true, mode: 0o700 });
    privateJSON('s3.json', { identities: [{ name: 'infrapro', credentials: [{ accessKey: 'infrapro', secretKey: passwords.s3 }], actions: ['Admin', 'Read', 'Write', 'List'] }] });
    const clickhouse = readFileSync(join(root, 'deploy/monitoring-contract/clickhouse.xml'), 'utf8')
      .replaceAll('contract-backups', 'infrapro-backups').replaceAll('contract-backup-test-only', passwords.s3).replaceAll('contract-backup', 'infrapro')
      .replaceAll('contract-ch-test-only', passwords.s3).replaceAll('contract-ch', 'infrapro');
    writeFileSync(join(directory, 'clickhouse.xml'), clickhouse, { mode: 0o600 });
    const images = {
      postgres: 'postgres:17-alpine@sha256:3540c0362c809df084ffac0ae69fe794515f9de92692f02178bcdce0ef0302ac', redis: 'redis:7.4-alpine',
      clickhouse: 'clickhouse/clickhouse-server:26.8.36.3-alpine@sha256:f5443f9ffa48f0154cdd50cab39fed732201762574a27f04f1f564bbba81832d',
      keeper: 'clickhouse/clickhouse-keeper:26.8.36.3-alpine@sha256:82cb1687db7d461b180ac17b5a7a1db91d9fcbf8ba91aa32aea85998cd61383d',
      s3: 'chrislusf/seaweedfs:4.53@sha256:ccad23e18639c8b98cee17e7100b95830a06077b56d8e5b006d6dcf8ec5e94a0',
    };
    // A registry prefix is an explicit local installation choice; digests stay fixed.
    const registry = process.env.INFRAPRO_IMAGE_REGISTRY;
    if (registry) for (const [key, value] of Object.entries(images)) images[key] = registry.replace(/\/$/, '') + '/' + (value.includes('/') ? value : 'library/' + value);
    const health = test => ({ test, interval: '2s', timeout: '3s', retries: 60 });
    const services = {
      postgres: { image: images.postgres, environment: { POSTGRES_USER: 'infrapro', POSTGRES_DB: 'infrapro', POSTGRES_PASSWORD: passwords.postgres }, ports: ['127.0.0.1:5432:5432'], volumes: ['postgres:/var/lib/postgresql/data'], healthcheck: health(['CMD', 'pg_isready', '-U', 'infrapro', '-d', 'infrapro']) },
      redis: { image: images.redis, environment: { REDIS_PASSWORD: passwords.redis }, command: ['sh', '-c', 'exec redis-server --requirepass "$$REDIS_PASSWORD" --appendonly yes --maxmemory 256mb --maxmemory-policy noeviction'], ports: ['127.0.0.1:6379:6379'], volumes: ['redis:/data'], healthcheck: health(['CMD-SHELL', 'REDISCLI_AUTH="$$REDIS_PASSWORD" redis-cli ping']) },
      keeper: { image: images.keeper, volumes: [join(root, 'deploy/monitoring-contract/keeper.xml') + ':/etc/clickhouse-keeper/keeper_config.xml:ro', 'keeper:/var/lib/clickhouse-keeper'], ports: ['127.0.0.1:61181:9181'], mem_limit: '384m' },
    };
    for (const [name, port] of [['s3', 61333], ['backup-s3', 61334]]) services[name] = { image: images.s3, command: ['server', '-ip=' + name, '-dir=/data', '-s3', '-s3.config=/etc/infrapro/s3.json', '-volume.max=48', '-master.volumeSizeLimitMB=128'], environment: { GOMEMLIMIT: '384MiB' }, ports: [`127.0.0.1:${port}:8333`], volumes: [join(directory, 's3.json') + ':/etc/infrapro/s3.json:ro', name + ':/data'], mem_limit: '512m' };
    for (const [name, port] of [['ch1', 61000], ['ch2', 61001]]) services[name] = { image: images.clickhouse, environment: { CLICKHOUSE_SKIP_USER_SETUP: '1', CONTRACT_REPLICA: name, CONTRACT_S3_ENDPOINT: `http://s3:8333/infrapro-ch/${name}/` }, ports: [`127.0.0.1:${port}:9000`], volumes: [join(directory, 'clickhouse.xml') + ':/etc/clickhouse-server/config.d/infrapro.xml:ro', name + ':/var/lib/clickhouse'], depends_on: ['keeper', 's3'], mem_limit: '2g', healthcheck: health(['CMD', 'clickhouse-client', '--query', 'SELECT 1']) };
    for (const [name, service] of Object.entries(services)) {
      service.restart = 'unless-stopped';
      service.hostname = name;
      const replica = name === 'ch1' ? 'clickhouse-1' : name === 'ch2' ? 'clickhouse-2' : name + '-1';
      service.labels = { 'io.infrapro.instance': 'infrapro-local', 'io.infrapro.replica': replica };
    }
    privateJSON('compose.json', { services, volumes: Object.fromEntries(['postgres', 'redis', 'keeper', 's3', 'backup-s3', 'ch1', 'ch2'].map(name => [name, {}])) });
  }
  compose(['up', '-d', '--wait', '--wait-timeout', '180']);
  const secrets = readJSON('service-secrets.json');
  for (const name of ['infrapro-source', 'infrapro-ch', 'infrapro-files', 'infrapro-audit']) await bucket(name, 'http://127.0.0.1:61333', secrets.s3);
  await bucket('infrapro-backups', 'http://127.0.0.1:61334', secrets.s3);
  for (const args of [['database', 'migrate'], ['access', 'sync'], ['access', 'check'], ['monitoring', 'storage', 'init']]) run(binary('infrapro'), args, environment());
  console.log('Local infrastructure initialized. Bootstrap an account with the regular infrapro CLI.');
}
function alive(p) {
  try { return execFileSync('ps', ['-p', String(p.pid), '-o', 'lstart='], { encoding: 'utf8' }).trim() === p.started; } catch { return false; }
}
async function start() {
  compose(['up', '-d', '--wait']);
  const state = existsSync(join(directory, 'processes.json')) ? readJSON('processes.json') : {};
  const env = environment();
  const architecture = deploymentDirectory(process.env) ?? deploymentDirectory(env);
  // A successful TCP/SELECT health check can precede Keeper session recovery.
  // Wait for the actual storage contract before starting its consumers.
  let storageReady = false;
  for (let attempt = 0; attempt < (architecture ? 1 : 120); attempt++) {
    try {
      execFileSync(binary('infrapro'), ['monitoring', 'storage', 'check'], { cwd: root, env, stdio: 'ignore', timeout: 10000 });
      storageReady = true; break;
    } catch { await pause(1000); }
  }
  if (!storageReady && !architecture) throw Error('Monitoring storage did not become ready; run pnpm local:cli monitoring storage check.');
  // Independent parser and relay processes receive only their own credentials.
  const independentEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_') && !key.startsWith('VITE_')));
  const commands = [];
  const parserSettings=env.INFRAPRO_PARSING_ENABLED==='true'&&existsSync(join(directory,'parsing/service.json'))?readJSON('parsing/service.json'):{};
  const proxy=localReplicaProxy(architecture,{...env,...parserSettings});
  if(proxy)commands.push(['replica-proxy','python3',[join(root,'deploy/architecture/replica-proxy.py'),proxy],independentEnv]);
  if (env.INFRAPRO_PARSING_ENABLED === 'true' && existsSync(join(directory, 'parsing/service.json'))) {
    for(const i of serviceInstances('parser',architecture))commands.push([i.label,binary('parser'),[],processEnvironment(i,{...independentEnv,...parserSettings,
      ...(i.public_address&&{INFRAPRO_PARSER_ADDR:i.public_address}),...(i.maxInflight&&{INFRAPRO_PARSER_CONCURRENCY:String(i.maxInflight)})}),i.id,i.diagnostic_node]);
  }
  if (env.INFRAPRO_INGESTION_FILE && existsSync(join(directory, 'ingestion/relay.json'))) {
    for(const i of serviceInstances('ingest-relay',architecture)){
      let file=architecture&&join(architecture,i.directory,'relay.json');
      if(!file||!existsSync(file)){
        file=join(directory,'ingestion/relay.json');
        if(architecture&&i.maxInflight){
          const bounded=boundedRelayConfig(readJSON('ingestion/relay.json'),i);
          file=join(architecture,i.directory,'relay.local.json');
          writeRuntimeJSON(file,bounded);
        }
      }
      commands.push([i.label,binary('ingest-relay'),['serve',file],processEnvironment(i,independentEnv),i.id,i.diagnostic_node]);
    }
  }
  const roles = ['files-checker', 'api', 'spatial-worker', 'display-package-worker', 'notification-worker', 'task-index-worker', 'files-worker', 'audit-worker'];
  if (architecture) roles.push(...enabledSplitRoles(env,architecture));
  else roles.push('monitoring-worker','display-runtime',...(env.INFRAPRO_INGESTION_FILE ? ['ingest-worker'] : []));
  for (const name of roles) {
    const args = name==='files-checker' ? ['--directory',env.INFRAPRO_FILES_TEMP_DIRECTORY,'--socket',env.INFRAPRO_FILES_CHECKER_SOCKET] : [];
    for(const i of serviceInstances(name,architecture))commands.push([i.label,binary(name),args,processEnvironment(i,name==='files-checker'?independentEnv:roleEnvironment(name,env,architecture,i)),i.id,i.diagnostic_node]);
  }
  commands.push(['web', process.execPath, [join(root, 'apps/console/node_modules/vite/bin/vite.js'), 'preview', '--host', '127.0.0.1']], ['diagnostics', binary('infrapro-diagnostics'), ['server', '--config', join(directory, 'diagnostics/server.json')]]);
  if (existsSync(join(directory, 'diagnostics/observer.json'))) commands.push(['diagnostics-observer', binary('infrapro-diagnostics'), ['observer', '--config', join(directory, 'diagnostics/observer.json')]]);
  for (const [name, cmd, args, serviceEnv, replicaID, nodeID] of commands) {
    if (name === 'diagnostics' && !existsSync(join(directory, 'diagnostics/server.json'))) continue;
    if (state[name] && alive(state[name])) continue;
    const fd = openSync(join(directory, 'logs', name + '.log'), 'a', 0o600);
    const extra = { INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'infrapro-local', INFRAPRO_DIAGNOSTICS_NODE_ID: nodeID??serviceEnv?.INFRAPRO_DIAGNOSTICS_NODE_ID??'local', INFRAPRO_DIAGNOSTICS_REPLICA_ID: replicaID??name+'-1', INFRAPRO_DIAGNOSTICS_REPORT_FILE: join(directory, 'reports', name + '.json') };
    const child = spawn(cmd, args, { cwd: name === 'web' ? join(root, 'apps/console') : root, env: { ...(serviceEnv ?? env), ...extra }, detached: true, stdio: ['ignore', fd, fd] }); closeSync(fd);
    await new Promise((ok, no) => { child.once('spawn', ok); child.once('error', no); }); child.unref(); await pause(200);
    state[name] = { pid: child.pid, started: execFileSync('ps', ['-p', String(child.pid), '-o', 'lstart='], { encoding: 'utf8' }).trim() }; privateJSON('processes.json', state);
    if (name === 'web') writeFileSync(join(directory, 'reports/web.pid'), String(child.pid) + '\n', { mode: 0o600 });
  }
  const endpoints = [origin + '/api/v1/public/account-help', 'http://127.0.0.1:28080/readyz'].map(url => ({ url }));
  if (serviceInstances('parser',architecture).some(i=>state[i.label])) {
    const parser = readJSON('parsing/service.json');
    endpoints.push({ url: `http://${parser.INFRAPRO_PARSER_ADDR}/readyz`, headers: { Authorization: 'Bearer ' + readFileSync(parser.INFRAPRO_PARSER_TOKEN_FILE, 'utf8').trim() } });
  }
  if (state.diagnostics) endpoints.push({ url: 'http://127.0.0.1:8096/healthz' });
  for (let i = 0; i < 60; i++) {
    const stopped = Object.entries(state).filter(([, process]) => !alive(process)).map(([name]) => name);
    if (stopped.length) throw Error('Runtime process stopped: ' + stopped.join(', ') + '. Inspect private runtime logs.');
    try {
      const ready = await Promise.all(endpoints.map(async ({ url, headers }) => (await fetch(url, { headers, signal: AbortSignal.timeout(1000) })).ok));
      if (ready.every(Boolean)) { console.log('Console: ' + origin); return; }
    } catch { /* bounded readiness wait */ }
    await pause(500);
  }
  throw Error('Console did not become ready; inspect private runtime logs.');
}
async function stop() {
  const state = existsSync(join(directory, 'processes.json')) ? readJSON('processes.json') : {};
  for (const p of Object.values(state).reverse()) if (alive(p)) process.kill(-p.pid, 'SIGTERM');
  for (let waits=0; waits<240 && Object.values(state).some(alive); waits++) await pause(500);
  for (const p of Object.values(state)) if (alive(p)) throw Error('A runtime process has not stopped; inspect before restarting.');
  privateJSON('processes.json', {});
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const command = process.argv[2];
  if (command === 'init') await init();
  else if (command === 'start') await start();
  else if (command === 'stop') await stop();
  else if (command === 'restart') { await stop(); await start(); }
  else if (command === 'cli') run(binary('infrapro'), process.argv.slice(3), environment());
  else if (command === 'status') { compose(['ps']); for (const [name, p] of Object.entries(readJSON('processes.json'))) console.log(name + ': ' + (alive(p) ? 'running' : 'stopped')); }
  else throw Error('Use init | start | stop | restart | status | cli <arguments>');
}
