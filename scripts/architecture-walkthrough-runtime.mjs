import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID, X509Certificate } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import https from 'node:https';
import { join } from 'node:path';
import { availablePort, startParser, stop, waitURL } from './fixtures/parsing-lab.mjs';
import { activeDatabasePort, digest, launch, queryPG, read, root, sleep, write } from './architecture-lab.mjs';
import { provision } from './architecture-postgres.mjs';
import { roleEnvironment } from './architecture-runtime.mjs';

const publicWorkers = ['files-checker', 'files-worker', 'spatial-worker', 'display-package-worker'];
const executable = (path, args, env) =>
  execFileSync(path, args, { cwd: root, env, stdio: 'pipe', encoding: 'utf8' });
const mkdir = (path) => mkdirSync(path, { mode: 0o700, recursive: true });
function reportEnvironment(lab, side, role) {
  const directory = join(lab.directory, 'private', side, 'reports', role);
  mkdir(directory);
  return {
    INFRAPRO_DIAGNOSTICS_REPORT_FILE: join(directory, 'process.json'),
    INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'arc-' + lab.id,
    INFRAPRO_DIAGNOSTICS_REPLICA_ID: role + '-1',
    INFRAPRO_DIAGNOSTICS_NODE_ID: 'local',
  };
}

// Docker may reassign a published port to the same local PostgreSQL container.
// Reopen a walkthrough's existing issued logins only after proving that the
// labelled database still holds the original slot and login assignments.
export function refreshWalkthroughRolePorts(lab, directory, databaseURL) {
  const layout = read(join(directory, 'layout.json'));
  assert.equal(layout.mode, 'local');
  const target = new URL(databaseURL);
  assert.equal(target.hostname, '127.0.0.1');
  assert.equal(target.pathname, '/arc_walk_' + lab.id + '_b');
  assert.equal(target.port, String(activeDatabasePort(lab)));
  const changed = [];
  for (const [role, slots] of Object.entries(layout.instances)) {
    for (const slot of slots) {
      assert.match(role, /^[a-z][a-z0-9-]*$/);
      assert.match(slot.id, /^[a-z][a-z0-9-]*$/);
      assert(['roles/' + role, 'replicas/' + role + '/' + slot.id].includes(slot.directory));
      const file = join(directory, slot.directory, 'environment.json');
      if (!existsSync(file)) continue;
      const config = read(file), uri = new URL(config.INFRAPRO_DATABASE_URL);
      assert.equal(uri.protocol, target.protocol);
      assert.equal(uri.hostname, target.hostname);
      assert.equal(uri.pathname, target.pathname);
      assert.equal(uri.search, target.search);
      if (uri.port === target.port) continue;
      assert.match(uri.username, /^arc_[a-z_]+_[a-f0-9]{8,12}$/);
      changed.push({ role, slot: slot.id, file, config, uri });
    }
  }
  if (!changed.length) return [];
  const users = changed.map(({ uri }) => "'" + uri.username + "'").join(',');
  const db = JSON.parse(queryPG(lab, 'b', `SELECT json_build_object('database',current_database(),'schema',current_schema(),
    'slots',(SELECT json_object_agg(rolname,shobj_description(oid,'pg_authid')) FROM pg_roles
    WHERE rolcanlogin AND NOT rolsuper AND NOT rolcreaterole AND NOT rolbypassrls AND rolname IN (${users})))`));
  assert.equal('/' + db.database, target.pathname);
  const prefix = 'infrapro-slot-v1:' + digest(db.database + '/' + db.schema).slice(0, 24) + ':';
  for (const { role, slot, uri } of changed) assert.equal(db.slots?.[uri.username], prefix + role + ':' + slot);
  for (const { file, config, uri } of changed) {
    uri.port = target.port;
    write(file, { ...config, INFRAPRO_DATABASE_URL: uri.href });
  }
  return changed.map(({ slot }) => slot);
}

// Each walkthrough reopens the restored S0 relay DB and its own durable
// endpoint and principals. Both are also available after restarting Docker.
async function openRelay(lab, side, folder, bin, children) {
  mkdir(folder);
  const ca = join(folder, 'ca.pem'),
    caKey = join(folder, 'ca.key'),
    principals = join(folder, 'principals.json');
  const relayFile = join(folder, 'relay.json'),
    workerFile = join(folder, 'worker.json');
  const ssl = (args) => executable('openssl', args);
  if (!existsSync(ca))
    ssl([
      'req',
      '-new',
      '-x509',
      '-newkey',
      'ec',
      '-pkeyopt',
      'ec_paramgen_curve:P-256',
      '-nodes',
      '-days',
      '365',
      '-subj',
      '/CN=InfraPro local walkthrough relay',
      '-keyout',
      caKey,
      '-out',
      ca,
    ]);
  const identities = existsSync(principals) ? read(principals) : {};
  function issue(name, role) {
    assert.match(name, /^[a-z][a-z0-9-]*$/);
    const cert = join(folder, name + '.pem'),
      key = join(folder, name + '.key');
    if (!existsSync(cert)) {
      const csr = join(folder, name + '.csr'),
        ext = join(folder, name + '.ext');
      ssl([
        'req',
        '-new',
        '-newkey',
        'ec',
        '-pkeyopt',
        'ec_paramgen_curve:P-256',
        '-nodes',
        '-subj',
        '/CN=' + name,
        '-keyout',
        key,
        '-out',
        csr,
      ]);
      writeFileSync(
        ext,
        'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=' +
          (role === 'server' ? 'serverAuth\nsubjectAltName=IP:127.0.0.1' : 'clientAuth') +
          '\n',
        { mode: 0o600 },
      );
      ssl([
        'x509',
        '-req',
        '-in',
        csr,
        '-CA',
        ca,
        '-CAkey',
        caKey,
        '-CAcreateserial',
        '-out',
        cert,
        '-days',
        '90',
        '-extfile',
        ext,
      ]);
      rmSync(csr);
      rmSync(ext);
    }
    if (role !== 'server') {
      const fingerprint = new X509Certificate(readFileSync(cert)).fingerprint256
        .replaceAll(':', '')
        .toLowerCase();
      identities[fingerprint] = {
        id: 'lab-' + name,
        role,
        ...(role === 'consumer' && { consumer_id: 'lab' }),
      };
      write(principals, identities);
    }
    return { certificate_file: cert, key_file: key, ca_file: ca };
  }
  const server = issue('server', 'server'),
    consumer = issue('consumer', 'consumer'),
    control = issue('control', 'control');
  const operator = read(join(lab.directory, 'private', 'relay-seed.json'));
  const database = new URL(read(lab[side].config).database_url);
  assert.equal(database.hostname, '127.0.0.1');
  assert.equal(database.pathname, '/' + lab[side].database);
  database.port = String(activeDatabasePort(lab));
  database.pathname = '/' + lab[side].relayDatabase;
  // The custody client gets this private address from the file written below
  // on every launch. The registered device listener, relay ID and spool stay
  // bound to their durable configuration when another lab reuses a former port.
  const port = await availablePort();
  const address = '127.0.0.1:' + port;
  write(relayFile, {
    ...operator,
    budget_bytes: String(2 ** 32),
    database_url: database.href,
    control_address: address,
    certificate_file: server.certificate_file,
    key_file: server.key_file,
    client_ca_file: ca,
    principals_file: principals,
  });
  const connection = {
    id: operator.relay_id,
    configuration_ref: 'synthetic-lab',
    url: 'https://' + address,
    consumer_id: 'lab',
    consumer,
    control,
  };
  write(workerFile, {
    enabled: true,
    storage_budget_bytes: String(2 ** 32),
    queue_budget: 500000,
    relays: [connection],
  });
  const binary = join(bin, 'ingest-relay');
  executable(binary, ['migrate', relayFile]);
  const child = launch(
    binary,
    ['serve', relayFile],
    { ...process.env, ...reportEnvironment(lab, side, 'ingest-relay') },
    join(folder, 'relay.log'),
  );
  children.push(child);
  const trust = {
    ca: readFileSync(ca),
    cert: readFileSync(consumer.certificate_file),
    key: readFileSync(consumer.key_file),
    timeout: 1000,
  };
  for (let i = 0; ; i++) {
    const ready = await new Promise((resolve) => {
      const probe = https.get(connection.url + '/readyz', trust, (res) => {
        res.resume();
        resolve(res.statusCode === 200);
      });
      probe.on('timeout', () => probe.destroy());
      probe.on('error', () => resolve(false));
    });
    if (ready) break;
    if (i === 100 || child.exitCode !== null || child.signalCode)
      throw Error('Walkthrough relay is unavailable; inspect its private log');
    await sleep(150);
  }
  const endpoint = join(folder, 'synthetic-listener.json');
  if (!existsSync(endpoint)) write(endpoint, { port: await availablePort() });
  return {
    id: connection.id,
    pid: child.pid,
    workerConfig: workerFile,
    ddpPort: read(endpoint).port,
    producerFor: (name) => ({
      principal: 'lab-producer-' + name,
      ...issue('producer-' + name, 'producer'),
    }),
  };
}

export function configEnvironment(cfg) {
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_')),
  );
  for (const [group, settings] of Object.entries(cfg)) {
    if (typeof settings === 'string') env['INFRAPRO_' + group.toUpperCase()] = settings;
    else if (settings && typeof settings === 'object')
      for (const [k, v] of Object.entries(settings))
        if (typeof v === 'string')
          env['INFRAPRO_' + group.toUpperCase() + '_' + k.toUpperCase()] = v;
  }
  return env;
}

// Bind the human walkthrough dashboard to the actual listeners and processes
// it can stop. Its ClickHouse probe takes the same gated path as the readers.
export function walkthroughProbes({
  lab,
  side,
  bin,
  manifest,
  bindings,
  manual,
  clickhouse,
  roles,
}) {
  const config = read(manual.operatorConfig);
  const attach = (role, facet, binding) => {
    const component = manifest.components.find((item) => item.id === role);
    const replica = manifest.replicas.find((item) => item.component_id === role);
    assert(component && replica && replica.node_id === 'local');
    if (role === 'clickhouse') {
      component.expectation = replica.expectation = 'external';
      component.min_ready_replicas = 0;
      component.expectation_reason = '通过本环境的业务网络连接执行 ClickHouse 协议查询';
    } else {
      component.expectation = replica.expectation = 'continuous';
      component.min_ready_replicas = 1;
      delete component.expectation_reason;
    }
    if (!replica.required_probe_ids.includes(binding.id))
      replica.required_probe_ids.push(binding.id);
    const probe = {
      id: binding.id,
      node_id: replica.node_id,
      target_kind: 'replica',
      target_id: replica.id,
      adapter: binding.adapter,
      binding_id: binding.id,
      facet,
      required: true,
      interval_ms: 5000,
      timeout_ms: 2000,
      stale_after_ms: 20000,
    };
    for (const [list, value] of [
      [manifest.probes, probe],
      [bindings.bindings, binding],
    ]) {
      const index = list.findIndex((item) => item.id === binding.id);
      if (index < 0) list.push(value);
      else list[index] = value;
    }
  };
  if (side === 'a') {
    for (const role of new Set([...roles, 'parser', 'ingest-relay'])) {
      const folder = join(lab.directory, 'private', side, 'reports', role);
      const target = {
        report_file: join(folder, 'process.json'),
        allowed_root: folder,
        expected_executable: join(bin, role),
        expected_uid: process.getuid(),
      };
      attach(role, 'presence', {
        id: role + '-presence',
        adapter: 'native_process',
        ...target,
      });
      attach(role, 'loop', {
        id: role + '-report',
        adapter: 'self_report',
        ...target,
        expected_replica_id: role + '-1',
        pid_namespace: 'host',
      });
    }
  }
  attach('parser', 'readiness', {
    id: 'parser-health',
    adapter: 'http_ready',
    url: config.parsing.url + '/healthz',
    expected_status: 200,
  });
  const credentials = join(manual.folder, 'clickhouse-probe.json');
  write(credentials, {
    schema_version: 1,
    kind: 'password',
    password: config.monitoring_clickhouse_password ?? '',
  });
  attach('clickhouse', 'readiness', {
    id: 'clickhouse-protocol',
    adapter: 'clickhouse',
    address: clickhouse,
    database: config.monitoring_clickhouse_database,
    username: config.monitoring_clickhouse_user,
    credential_file: credentials,
  });
}

export async function openWalkthrough(lab, side, bin, source, topology) {
  assert(['a', 'b'].includes(side));
  const split = side === 'b',
    folder = join(lab.directory, 'private', side, 'walkthrough');
  mkdir(folder);
  const children = [];
  let parser, brokerChild, startBroker, brokerProbe, appliedRoleSQL;
  async function close() {
    await Promise.all([...children.map(stop), parser?.close()]);
  }
  try {
    // Run the same immutable example method on both sides, as in the paired
    // ingestion workload. The method package is pinned to the frozen parser.
    const base = read(join(lab.directory, 'baseline.json'));
    parser = await startParser(join(folder, 'parser'), {
      bin: base.bin,
      source: base.source,
      serviceBinary: join(bin, 'parser'),
      reportEnvironment: reportEnvironment(lab, side, 'parser'),
    });
    const relay = await openRelay(lab, side, join(folder, 'relay'), bin, children);
    const cfg = read(lab[side].config);
    const spatialPython = join(root, '.artifacts/spatial-venv/bin/python');
    if (!existsSync(spatialPython)) executable('pnpm', ['spatial:setup']);
    cfg.spatial_python = spatialPython;
    cfg.spatial_converter_script = join(source, 'tools/spatial-converter/convert.py');
    cfg.spatial_temp_directory = join(folder, 'spatial');
    mkdir(cfg.spatial_temp_directory);
    // AF_UNIX paths have a short kernel limit on macOS.
    const checker = join('/tmp', 'infrapro-arc-' + lab.id + '-' + side);
    mkdir(checker);
    cfg.files = {
      ...cfg.files,
      temp_directory: checker,
      checker_socket: join(checker, 'checker.sock'),
    };
    cfg.parsing = {
      enabled: 'true',
      url: parser.url,
      token_file: parser.token,
      object_endpoint: cfg.monitoring_object_endpoint,
      object_access_key: cfg.monitoring_object_access_key,
      object_secret_key: cfg.monitoring_object_secret_key,
      bucket: cfg.monitoring_source_bucket,
    };
    cfg.ingestion_file = relay.workerConfig;
    const configPath = join(folder, 'operator.json');
    write(configPath, cfg);
    const env = configEnvironment(cfg),
      events = join(folder, 'events');
    if (split) {
      refreshWalkthroughRolePorts(lab, topology, cfg.database_url);
      appliedRoleSQL = provision({
        url: cfg.database_url,
        container: lab.container,
        directory: topology,
        localLogins: true,
      });
      const admin = (args, extra = {}) =>
        executable(join(bin, 'event-relay'), args, {
          ...env,
          INFRAPRO_CONFIG: configPath,
          ...extra,
        });
      const state = JSON.parse(admin(['status'])).protocol;
      const descriptor = join(events, 'lab.json');
      if (!existsSync(descriptor))
        executable(
          process.execPath,
          [
            'scripts/architecture-event-lab.mjs',
            events,
            'walkthrough_' + lab.id,
            state.restore_epoch,
          ],
          { ...process.env, INFRAPRO_EVENT_LAB_CERT_DAYS: '90' },
        );
      const cluster = read(descriptor);
      assert.equal(
        cluster.epoch,
        state.restore_epoch,
        'Walkthrough broker belongs to a previous restore epoch',
      );
      const broker = join(
        root,
        'build',
        'events',
        process.platform + '-' + (process.arch === 'x64' ? 'amd64' : process.arch),
        'nats-server',
      );
      if (!existsSync(broker))
        executable(process.execPath, ['scripts/architecture-broker-build.mjs']);
      brokerProbe = {
        binary: broker,
        pidFile: join(folder, 'broker.pid'),
        port: cluster.monitorPort,
      };
      startBroker = async () => {
        if (!brokerChild || brokerChild.exitCode !== null || brokerChild.signalCode) {
          brokerChild = launch(
            broker,
            ['-c', cluster.config, '-P', brokerProbe.pidFile],
            process.env,
            join(folder, 'broker.log'),
          );
          children.push(brokerChild);
        }
        await waitURL('http://127.0.0.1:' + cluster.monitorPort + '/healthz?js-enabled-only=true');
      };
      await startBroker();
      const eventAdmin = {
        INFRAPRO_EVENTS_FILE: join(events, 'event-admin', 'events.json'),
      };
      admin(['provision'], eventAdmin);
      if (!state.enabled) admin(['enable', '--reason', '隔离人工走查'], eventAdmin);
    }
    const roles = [
      ...publicWorkers,
      ...(split
        ? ['event-relay', 'ingest-custody-worker', 'parsing-dispatch-worker']
        : ['ingest-worker']),
    ];
    function environment(role) {
      if (!split) return { ...env, INFRAPRO_CONFIG: configPath };
      const projected = roleEnvironment(role, env, topology),
        file = join(events, role, 'events.json');
      if (existsSync(file)) projected.INFRAPRO_EVENTS_FILE = file;
      return projected;
    }
    async function dependency(name, available) {
      if (name === 'parser') {
        if (available && !parser.pid) await parser.resume();
        if (!available) await parser.pause();
      } else if (name === 'broker' && startBroker) {
        if (available) await startBroker();
        else await stop(brokerChild);
      } else throw Error('Dependency is unavailable on this topology');
    }
    return {
      launchID: randomUUID(),
      appliedRoleSQL,
      get parserPID() { return parser.pid; },
      roles,
      relay,
      folder,
      environment,
      checker,
      close,
      dependency,
      brokerProbe,
      operatorConfig: configPath,
    };
  } catch (error) {
    await close();
    throw error;
  }
}
