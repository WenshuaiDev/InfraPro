import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { root, command, readJSON, privateJSON, freePort, compose, saveLab, startProcess, sleep, ownedContainers, checkContainerOwnership } from './diagnostics-lab.mjs';
const images = {
  postgres: 'postgres:17-alpine@sha256:3540c0362c809df084ffac0ae69fe794515f9de92692f02178bcdce0ef0302ac',
  redis: 'redis:8-alpine',
  keeper: 'clickhouse/clickhouse-keeper:26.8.36.3-alpine@sha256:82cb1687db7d461b180ac17b5a7a1db91d9fcbf8ba91aa32aea85998cd61383d',
  clickhouse: 'clickhouse/clickhouse-server:26.8.36.3-alpine@sha256:f5443f9ffa48f0154cdd50cab39fed732201762574a27f04f1f564bbba81832d',
  s3: 'chrislusf/seaweedfs:4.53@sha256:ccad23e18639c8b98cee17e7100b95830a06077b56d8e5b006d6dcf8ec5e94a0',
  web: 'nginx:1.28-alpine',
};
export async function prepareBusiness(lab) {
  lab.compose_project = `infrapro-${lab.run_id}`;
  const deploy = join(lab.directory, 'compose'); mkdirSync(deploy, { mode: 0o700 });
  for (const name of ['postgres', 'redis', 'clickhouse', 'clickhouse2', 's3', 'keeper', 'api', 'web', 'display', 'assets']) lab.ports[name] = await freePort();
  const passwords = { postgres: randomBytes(24).toString('hex'), redis: randomBytes(24).toString('hex'), s3: randomBytes(24).toString('hex'), clickhouse: randomBytes(24).toString('hex') };
  const labels = (replica) => ({ 'io.infrapro.lab.run_id': lab.run_id, 'io.infrapro.instance': lab.instance_id, 'io.infrapro.replica': replica });
  const port = (name, target) => [`127.0.0.1:${lab.ports[name]}:${target}`];
  const text = (name, value) => writeFileSync(join(deploy, name), value, { mode: 0o600 });
  text('keeper.xml', readFileSync(join(root, 'deploy/monitoring-contract/keeper.xml'), 'utf8').replace('<tcp_port>', '<four_letter_word_white_list>ruok</four_letter_word_white_list><tcp_port>'));
  let ch = readFileSync(join(root, 'deploy/monitoring-contract/clickhouse.xml'), 'utf8').replaceAll('contract-ch-test-only', passwords.s3).replaceAll('<access_key_id>contract-ch</access_key_id>', '<access_key_id>diagnostics-lab</access_key_id>');
  ch = ch.replace(/<named_collections>[\s\S]*?<\/named_collections>/, ''); text('clickhouse.xml', ch);
  privateJSON(join(deploy, 's3.json'), { identities: [{ name: 'diagnostics-lab', credentials: [{ accessKey: 'diagnostics-lab', secretKey: passwords.s3 }], actions: ['Admin', 'Read', 'Write', 'List', 'Tagging'] }, { name: 'denied', credentials: [{ accessKey: 'diagnostics-denied', secretKey: passwords.s3 }], actions: ['Read:other-bucket'] }] });
  text('redis.conf', `bind 0.0.0.0\nprotected-mode yes\nrequirepass ${passwords.redis}\nappendonly no\nsave ""\n`);
  const chService = (name, replica) => ({ image: images.clickhouse, labels: labels(replica), mem_limit: '2g', environment: { CLICKHOUSE_USER: 'default', CLICKHOUSE_PASSWORD: passwords.clickhouse, CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: '1', CONTRACT_REPLICA: name, CONTRACT_S3_ENDPOINT: `http://s3:8333/diagnostics-ch/${name}/` }, ports: port(name === 'ch1' ? 'clickhouse' : 'clickhouse2', 9000), volumes: [`${join(deploy, 'clickhouse.xml')}:/etc/clickhouse-server/config.d/diagnostics.xml:ro`], healthcheck: { test: ['CMD-SHELL', 'clickhouse-client --password "$$CLICKHOUSE_PASSWORD" --query "SELECT 1"'], interval: '2s', timeout: '3s', retries: 60 }, depends_on: { keeper: { condition: 'service_started' }, s3: { condition: 'service_started' } } });
  const config = { services: {
    postgres: { image: images.postgres, labels: labels('postgres-1'), environment: { POSTGRES_USER: 'diagnostics_lab', POSTGRES_PASSWORD: passwords.postgres, POSTGRES_DB: 'diagnostics_lab' }, ports: port('postgres', 5432), volumes: ['postgres-data:/var/lib/postgresql/data'], healthcheck: { test: ['CMD', 'pg_isready', '-U', 'diagnostics_lab', '-d', 'diagnostics_lab'], interval: '2s', timeout: '3s', retries: 30 }, mem_limit: '512m' },
    redis: { image: images.redis, labels: labels('redis-1'), command: ['redis-server', '/etc/redis/diagnostics.conf'], volumes: [`${join(deploy, 'redis.conf')}:/etc/redis/diagnostics.conf:ro`], ports: port('redis', 6379), mem_limit: '256m' },
    keeper: { image: images.keeper, labels: labels('keeper-1'), volumes: [`${join(deploy, 'keeper.xml')}:/etc/clickhouse-keeper/keeper_config.xml:ro`], ports: port('keeper', 9181), mem_limit: '512m' },
    s3: { image: images.s3, labels: labels('s3-1'), command: ['server', '-ip=s3', '-dir=/data', '-s3', '-s3.config=/etc/diagnostics/s3.json', '-volume.max=24', '-master.volumeSizeLimitMB=64', '-s3.readerCacheSizeMB=64', '-s3.concurrentUploadLimitMB=64', '-s3.concurrentFileUploadLimit=4'], environment: { GOMEMLIMIT: '512MiB', GOGC: '50' }, ports: port('s3', 8333), volumes: [`${join(deploy, 's3.json')}:/etc/diagnostics/s3.json:ro`], mem_limit: '768m' },
    ch1: chService('ch1', 'clickhouse-1'), ch2: chService('ch2', 'clickhouse-2'),
    web: { image: images.web, labels: labels('web-1'), ports: port('web', 80), volumes: [`${join(root, 'apps/console/dist')}:/usr/share/nginx/html:ro`], mem_limit: '128m' },
  }, volumes: { 'postgres-data': { labels: { 'io.infrapro.lab.run_id': lab.run_id } } } };
  lab.compose_file = join(deploy, 'compose.json'); privateJSON(lab.compose_file, config); saveLab(lab);
  execFileSync('pnpm', ['--filter', '@infrapro/console', 'build'], { cwd: root, stdio: 'inherit' });
  console.log('Starting only the isolated diagnostics Compose project.'); const composeLog = openLog(lab); try { compose(lab, ['up', '-d', '--wait', '--wait-timeout', '180'], { stdio: ['ignore', composeLog, composeLog] }); } finally { closeLog(composeLog); }
  const actual = [];
  for (const id of ownedContainers(lab)) { checkContainerOwnership(lab, id); const value = JSON.parse(command('docker', ['inspect', '--format', '{"id":{{json .Id}},"image":{{json .Image}},"configured_image":{{json .Config.Image}},"memory_limit":{{json .HostConfig.Memory}},"cpu_limit":{{json .HostConfig.NanoCpus}}}', id])); actual.push(value); }
  privateJSON(join(lab.directory, 'evidence/container-versions.json'), { type: 'real-container', architecture: process.arch, containers: actual });
  for (const name of ['monitoring', 'files', 'audit', 'spatial', 'checker']) mkdirSync(join(lab.directory, name), { mode: 0o700 });
  const keyring = join(lab.directory, 'mfa-keyring.json');
  privateJSON(keyring, { format_version: 1, active_key_id: 'lab', keys: [{id: 'lab', key_base64: randomBytes(32).toString('base64')}] });
  const business = {
    mfa_keyring_file: keyring,
    support_contact: "diagnostics-lab@example.test",
    database_url: `postgres://diagnostics_lab:${passwords.postgres}@127.0.0.1:${lab.ports.postgres}/diagnostics_lab`, redis_url: `redis://:${passwords.redis}@127.0.0.1:${lab.ports.redis}/0`, redis_key_prefix: `infrapro:${lab.run_id}:identity:v1:`, http_addr: `127.0.0.1:${lab.ports.api}`, public_console_url: `http://127.0.0.1:${lab.ports.web}`, trusted_origins: `http://127.0.0.1:${lab.ports.web}`, csrf_auth_key: randomBytes(32).toString('hex'),
    monitoring_clickhouse_replicas: `127.0.0.1:${lab.ports.clickhouse},127.0.0.1:${lab.ports.clickhouse2}`, monitoring_clickhouse_database: 'diagnostics_monitoring', monitoring_clickhouse_user: 'default', monitoring_clickhouse_password: passwords.clickhouse, monitoring_clickhouse_policy: 'monitoring_tiered', monitoring_object_endpoint: `http://127.0.0.1:${lab.ports.s3}`, monitoring_object_access_key: 'diagnostics-lab', monitoring_object_secret_key: passwords.s3, monitoring_source_bucket: 'diagnostics-source', monitoring_temp_directory: join(lab.directory, 'monitoring'),
    spatial_python: join(root, '.artifacts/spatial-venv/bin/python'), spatial_converter_script: join(root, 'tools/spatial-converter/convert.py'), spatial_temp_directory: join(lab.directory, 'spatial'),
    display: { runtime_origin: `http://127.0.0.1:${lab.ports.display}`, asset_origin: `http://127.0.0.1:${lab.ports.assets}`, runtime_addr: `127.0.0.1:${lab.ports.display}`, asset_addr: `127.0.0.1:${lab.ports.assets}`, shell_directory: join(root, 'apps/display-runtime/dist') },
    files: { object_endpoint: `http://127.0.0.1:${lab.ports.s3}`, access_key: 'diagnostics-lab', secret_key: passwords.s3, bucket: 'diagnostics-source', node_id: 'diagnostics-files', temp_directory: join(lab.directory, 'files'), checker_socket: join(lab.directory, 'files/check.sock'), workers: '1' },
    audit: { object_endpoint: `http://127.0.0.1:${lab.ports.s3}`, access_key: 'diagnostics-lab', secret_key: passwords.s3, bucket: 'diagnostics-source', node_id: 'diagnostics-audit', temp_directory: join(lab.directory, 'audit'), export_worker_concurrency: '1' },
  };
  lab.business_config = join(lab.directory, 'business.json'); privateJSON(lab.business_config, business);
  const maintenance = (args) => { const fd = openLog(lab); try { command(lab.cli, args, { env: { ...process.env, INFRAPRO_CONFIG: lab.business_config }, stdio: ['ignore', fd, fd] }); } finally { closeLog(fd); } };
  maintenance(['database', 'migrate']); maintenance(['access', 'sync']);
  const s3Credential=join(lab.directory,'lab-s3.json');privateJSON(s3Credential,{schema_version:1,kind:'s3',access_key_id:'diagnostics-lab',secret_access_key:passwords.s3});
  let bucketsReady=false;for(let i=0;i<30;i++){try{command(lab.helper,['buckets',business.monitoring_object_endpoint,s3Credential]);bucketsReady=true;break;}catch{await sleep(1000);}}if(!bucketsReady)throw Error('Owned S3 buckets did not initialize');
  let initialized = false; for (let attempt = 0; attempt < 30; attempt++) { try { maintenance(['monitoring', 'storage', 'init']); initialized = true; break; } catch { await sleep(1000); } } if (!initialized) throw Error('Owned monitoring storage did not initialize');
  const roles = readJSON(join(root, 'deploy/architecture/roles.json')).roles;
  // This fixture exercises the compatible business topology; the split executable
  // and its independent observer are validated by diagnostics-architecture-check.
  const catalog = readJSON(join(root, 'contracts/diagnostics/runtime-catalog.json')).entries.filter((c) =>
    c.entry_point?.startsWith('services/api/cmd/') && ['both', 'compat'].includes(roles[c.id]?.mode) && c.default_expectation === 'continuous');
  const binaries = join(lab.directory, 'bin'); mkdirSync(binaries, { mode: 0o700 });
  for (const c of catalog) { console.log(`Building owned runtime ${c.id}`); execFileSync('go', ['build', '-trimpath', '-ldflags', `-X github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport.BuildID=${lab.run_id}`, '-o', join(binaries, c.id), `./services/api/cmd/${c.id}`], { cwd: root, stdio: 'inherit' }); }
  const manifest = readJSON(lab.manifest_file); const bindingFile = join(lab.directory, 'native-a/bindings.json'); const b = readJSON(bindingFile);
  const enabled = (id, node, required) => { const c = manifest.components.find((v) => v.id === id); c.expectation = 'continuous'; c.expectation_reason = '隔离 lab 中的实际运行进程／服务'; c.min_ready_replicas = 1; const r = manifest.replicas.find((v) => v.id === id + '-1'); r.node_id = node; r.expectation = 'continuous'; r.required_probe_ids = required; return r; };
  const addProbe = (id, node, target_kind, target_id, adapter, facet, required, binding) => { manifest.probes.push({ id, node_id: node, target_kind, target_id, adapter, binding_id: id, facet, required, interval_ms: 5000, timeout_ms: 2000, stale_after_ms: 20000 }); binding.id = id; binding.adapter = adapter; return binding; };
  for (const c of catalog) {
    const replica = c.id + '-1', presence = c.id + '-presence', report = c.id + '-report'; enabled(c.id, 'native-a', [presence, report]);
    const reportFile = join(lab.directory, 'reports', c.id + '.json'), executable = join(binaries, c.id);
    const common = { report_file: reportFile, allowed_root: join(lab.directory, 'reports'), expected_executable: executable, expected_uid: process.getuid() };
    b.bindings.push(addProbe(presence, 'native-a', 'replica', replica, 'native_process', 'presence', true, { ...common }));
    b.bindings.push(addProbe(report, 'native-a', 'replica', replica, 'self_report', 'loop', true, { ...common, expected_replica_id: replica, pid_namespace: 'host' }));
    if (c.id === 'notification-worker') {
      const dependency = 'notification-postgres', probe = dependency + '-check';
      manifest.dependencies.push({ id: dependency, from_replica_id: replica, to_component_id: 'postgres', probe_id: probe, required: true, evidence_mode: 'self_report' });
      b.bindings.push(addProbe(probe, 'native-a', 'dependency', dependency, 'self_report', 'readiness', true, { ...common, expected_replica_id: replica, pid_namespace: 'host', dependency_key: 'postgres' }));
    }
    for (const [suffix, address] of c.id === 'api' ? [['ready', lab.ports.api]] : c.id === 'display-runtime' ? [['runtime', lab.ports.display], ['assets', lab.ports.assets]] : []) { const ep = `${c.id}-${suffix}`; manifest.endpoints.push({ id: ep, replica_id: replica, purpose: 'readiness', display_name: suffix === 'assets' ? '大屏资源服务' : 'HTTP 就绪端点' }); b.bindings.push(addProbe(ep + '-check', 'native-a', 'endpoint', ep, 'http_ready', 'readiness', true, { url: `http://127.0.0.1:${address}/readyz`, expected_status: 200 })); manifest.replicas.find((v) => v.id === replica).required_probe_ids.push(ep + '-check'); }
  }
  const db = readJSON(join(lab.directory, 'native-b/bindings.json'));
  const socket = JSON.parse(command('docker', ['context', 'inspect']))[0].Endpoints.docker.Host.replace(/^unix:\/\//, '');
  for (const id of ['postgres', 'redis', 'clickhouse', 'keeper', 's3', 'web']) {
    const presence = id + '-container', ready = id + '-protocol'; enabled(id, 'native-b', [presence, ready]);
    db.bindings.push(addProbe(presence, 'native-b', 'replica', id + '-1', 'docker_container', 'presence', true, { socket_path: socket, expected_replica_id: id + '-1', selector_labels: labels(id + '-1'), stats_enabled: true }));
    let target; const credential = join(lab.directory, 'native-b', id + '-credential.json');
    if (['postgres', 'redis', 'clickhouse'].includes(id)) privateJSON(credential, { schema_version: 1, kind: 'password', password: passwords[id] });
    if (id === 'postgres') target = { host: '127.0.0.1', port: lab.ports.postgres, database: 'diagnostics_lab', username: 'diagnostics_lab', tls_mode: 'disable', credential_file: credential };
    if (id === 'redis') target = { address: `127.0.0.1:${lab.ports.redis}`, database: 0, credential_file: credential };
    if (id === 'clickhouse') target = { address: `127.0.0.1:${lab.ports.clickhouse}`, database: 'default', username: 'default', credential_file: credential };
    if (id === 'keeper') target = { host: '127.0.0.1', port: lab.ports.keeper, enabled: true };
    if (id === 's3') { privateJSON(credential, { schema_version: 1, kind: 's3', access_key_id: 'diagnostics-lab', secret_access_key: passwords.s3 }); target = { endpoint: `http://127.0.0.1:${lab.ports.s3}`, bucket: 'diagnostics-source', region: 'us-east-1', credential_file: credential }; }
    if (id === 'web') target = { url: `http://127.0.0.1:${lab.ports.web}/`, expected_status: 200 };
    db.bindings.push(addProbe(ready, 'native-b', 'replica', id + '-1', id === 'web' ? 'http_ready' : id === 'keeper' ? 'keeper_live' : id, 'readiness', true, target));
  }
  // A second CH replica remains an explicit slot; it is never collapsed to one PID.
  const ch2 = structuredClone(manifest.replicas.find((r) => r.id === 'clickhouse-1')); ch2.id = 'clickhouse-2'; ch2.required_probe_ids = ['clickhouse2-container', 'clickhouse2-protocol']; manifest.replicas.push(ch2);
  db.bindings.push(addProbe('clickhouse2-container', 'native-b', 'replica', ch2.id, 'docker_container', 'presence', true, { socket_path: socket, expected_replica_id: ch2.id, selector_labels: labels(ch2.id), stats_enabled: true }));
  db.bindings.push(addProbe('clickhouse2-protocol', 'native-b', 'replica', ch2.id, 'clickhouse', 'readiness', true, { address: `127.0.0.1:${lab.ports.clickhouse2}`, database: 'default', username: 'default', credential_file: join(lab.directory, 'native-b/clickhouse-credential.json') }));
  manifest.revision = String(Number(manifest.revision) + 1); privateJSON(lab.manifest_file, manifest); privateJSON(bindingFile, b); privateJSON(join(lab.directory, 'native-b/bindings.json'), db);
  command(lab.cli, ['diagnostics', 'config', 'validate', '--server', lab.server_file]);
  for (const c of catalog) { const args = c.id === 'files-checker' ? ['--directory', join(lab.directory, 'files'), '--socket', join(lab.directory, 'files/check.sock')] : []; await startProcess(lab, c.id, join(binaries, c.id), args, { INFRAPRO_CONFIG: lab.business_config, INFRAPRO_DIAGNOSTICS_INSTANCE_ID: lab.instance_id, INFRAPRO_DIAGNOSTICS_NODE_ID: 'native-a', INFRAPRO_DIAGNOSTICS_REPLICA_ID: c.id + '-1', INFRAPRO_DIAGNOSTICS_REPORT_FILE: join(lab.directory, 'reports', c.id + '.json') }); }
  saveLab(lab);
}
// File descriptors keep operator output in the private lab, never in CLI errors.
import { openSync, closeSync } from 'node:fs';
function openLog(lab) { return openSync(join(lab.directory, 'logs/business-setup.log'), 'a', 0o600); }
function closeLog(fd) { closeSync(fd); }
