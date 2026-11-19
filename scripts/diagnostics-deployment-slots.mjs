import assert from 'node:assert/strict';
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { prepare } from './architecture-deploy.mjs';

const json = path => JSON.parse(readFileSync(path, 'utf8'));

// Compare the actual unit/Compose/local layout produced by the release
// generator with the independent manifest accepted by the diagnostics CLI.
export function checkDeploymentSlots(cli, directory) {
  const declarations = join(directory, 'slot-declarations.json');
  const slotsByRole = {
    api: ['gateway-a', 'gateway-b'],
    'identity-api': ['identity-a', 'identity-b'],
    parser: ['parser-a', 'parser-b', 'parser-c'],
  };
  const records = [];
  for (const mode of ['local', 'native', 'compose', 'mixed']) {
    const remote = mode === 'native' || mode === 'mixed';
    const slots = { ...slotsByRole };
    if (remote) {
      slots['identity-api'] = [{ id: 'identity-a', host: 'app-a' }, { id: 'identity-b', host: '10.24.8.2', diagnostic_node: 'app-b' }];
      slots['audit-worker'] = [{ id: 'audit-worker-1', host: 'app-b' }];
    }
    writeFileSync(declarations, JSON.stringify(slots), { mode: 0o600 });
    const deploy = join(directory, 'release-' + mode);
    prepare({ mode, directory: deploy, replicas: declarations });
    const layout = json(join(deploy, 'layout.json'));
    const profile = mode === 'compose' ? 'docker' : mode === 'local' ? 'native' : mode;
    const diagnose = join(directory, 'diagnostics-' + mode);
    const flags = ['diagnostics', 'config', 'init', '--topology', 'split', '--profile', profile,
      '--directory', diagnose, '--instance-id', 'slot-check', '--replicas', join(deploy, 'layout.json'),
      '--compat-api', '--compat-workers'];
    const reports = join(deploy, 'reports');
    if (mode === 'local') flags.push('--report-directory', reports, '--local-reports');
    if (mode === 'compose' || mode === 'mixed') flags.push('--container-report-directory', reports);
    execFileSync(cli, flags, { stdio: 'pipe' });
    execFileSync(cli, ['diagnostics', 'config', 'validate', '--server', join(diagnose, 'server.json')], { stdio: 'pipe' });
    const manifest = json(join(diagnose, 'manifest.json'));
    const bindings = readdirSync(diagnose).filter(f => f.endsWith('bindings.json')).flatMap(f => json(join(diagnose, f)).bindings);
    const compose = mode === 'compose' || mode === 'mixed' ? json(join(deploy, 'compose.json')).services : {};
    let checked = 0;
    for (const [role, instances] of Object.entries(layout.instances)) {
      for (const slot of instances) {
        const replica = manifest.replicas.find(r => r.id === slot.id && r.component_id === role);
        assert.equal(replica?.expectation, 'continuous', mode+'/'+role+'/'+slot.id);
        if (slot.diagnostic_node) assert.equal(replica.node_id, slot.diagnostic_node);
        if (slot.host) {
          assert(manifest.probes.filter(p => p.target_id === slot.id).every(p => p.node_id === slot.diagnostic_node));
          const nodeBindings = json(join(diagnose, 'observer-' + slot.diagnostic_node + '-bindings.json')).bindings;
          assert(replica.required_probe_ids.every(id => nodeBindings.some(b => b.id === id)));
        }
        if (role === 'replica-proxy') {
          const presence = bindings.find(b => b.id === 'replica-proxy-presence');
          if (mode === 'local') assert.equal(presence.pid_file, join(reports, 'replica-proxy.json'));
          else assert.equal(presence.unit_name, 'infrapro-replica-proxy.service');
          for (const socket of layout.tcp_proxies) {
            const check = bindings.find(b => b.id === 'proxy-'+socket.role+'-'+socket.protocol);
            assert.equal(check?.port, socket.listen.port);
            assert.equal(check?.host, socket.listen.host);
          }
          checked++;
          continue;
        }
        const report = bindings.find(b => b.adapter === 'self_report' && b.expected_replica_id === slot.id && !b.dependency_key);
        assert(report);
        const container = compose[slot.label];
        const expectedPath = container ? join(deploy, container.volumes.find(v => v.target === '/run/infrapro-report').source, 'process.json')
          : mode === 'local' ? join(reports, slot.label + '.json') : '/run/infrapro-' + slot.label + '/process.json';
        assert.equal(report.report_file, expectedPath);
        assert.equal(report.pid_namespace, container ? 'container' : 'host');
        if (container) {
          assert.equal(report.expected_executable, container.entrypoint[0]);
          assert.equal(container.environment.INFRAPRO_DIAGNOSTICS_NODE_ID, '${INFRAPRO_NODE:-' + slot.diagnostic_node + '}');
          const presence = bindings.find(b => b.adapter === 'docker_container' && b.expected_replica_id === slot.id);
          assert.equal(presence?.selector_labels['io.infrapro.replica'], container.labels['io.infrapro.replica']);
        } else if (mode !== 'local') {
          const file = join(deploy, role === 'ingest-relay' ? 'edge/units' : 'units', 'infrapro-'+slot.label+'.service');
          const unit = readFileSync(file, 'utf8');
          assert(unit.includes(report.expected_executable));
          assert(unit.includes('INFRAPRO_DIAGNOSTICS_REPORT_FILE='+expectedPath));
          assert(unit.includes('INFRAPRO_DIAGNOSTICS_NODE_ID='+slot.diagnostic_node));
        }
        checked++;
      }
    }
    records.push({ mode, slots: checked, remote_nodes: remote ? ['app-a', 'app-b'] : [], accepted: true });
  }
  return records;
}

export async function checkRunningReplicaProxy(cli, directory) {
  const holder = createServer();
  holder.listen(0, '127.0.0.1');
  await once(holder, 'listening');
  const port = holder.address().port;
  await new Promise(resolve => holder.close(resolve));
  const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', stdio: 'pipe', timeout: 30000 }).trim();
  const owner = randomUUID(), label = 'io.infrapro.proxy-check';
  const owned = [], backends = [];
  const cfg = join(directory, 'proxy-observer');
  const reports = join(directory, 'proxy-pids'), pidFile = join(reports, 'replica-proxy.json');
  const listeners = join(directory, 'isolated-listeners.json');
  const responder = join(directory, 'proxy-test-backend.sh');
  writeFileSync(responder, [
    '#!/bin/sh', 'cr=$(printf "\\r")', 'while IFS= read -r header; do',
    '  [ "$header" = "$cr" ] && break', 'done',
    'printf "HTTP/1.1 200 OK\\r\\nContent-Length: 1\\r\\nConnection: close\\r\\n\\r\\n%s" "$BACKEND_SLOT"', '',
  ].join('\n'), { mode: 0o555 });
  let proxy, exited;
  const changeBackend = (action, id) => {
    assert.equal(docker('inspect', '--format', '{{index .Config.Labels "' + label + '"}}', id), owner);
    docker(action, ...(action === 'stop' ? ['--timeout', '2'] : action === 'rm' ? ['-f'] : []), id);
  };
  const check = () => {
    const result = spawnSync(cli, ['diagnostics', 'config', 'validate', '--server', join(cfg, 'server.json'), '--probe'],
      { encoding: 'utf8', timeout: 45000 });
    assert([0, 3].includes(result.status), 'Independent proxy probe command failed: ' + result.status);
    return Object.fromEntries(JSON.parse(result.stdout)
      .filter(item => ['replica-proxy-presence', 'proxy-api-external-http'].includes(item.probe_id))
      .map(item => [item.probe_id, { outcome: item.outcome, reason_code: item.reason_code }]));
  };
  const request = async targetPort => {
    const r = await fetch('http://127.0.0.1:' + targetPort + '/', { signal: AbortSignal.timeout(4000), headers: { connection: 'close' } });
    assert.equal(r.status, 200);
    return (await r.text()).trim();
  };
  try {
    // Two separately addressed network namespaces represent the remote
    // backend hosts. Only their loopback-published test ports leave Docker.
    const namespaces = new Set();
    for (const slot of ['a', 'b']) {
      const id = docker('run', '-d', '--pull', 'never', '--label', label + '=' + owner,
        '--publish', '127.0.0.1::18080', '--read-only', '--user', '65534:65534',
        '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--pids-limit', '16', '--memory', '16m',
        '--mount', 'type=bind,src=' + responder + ',dst=/probe/answer,readonly',
        '--env', 'BACKEND_SLOT=' + slot, 'alpine:3.22', 'nc', '-lk', '-p', '18080', '-e', '/probe/answer');
      assert.match(id, /^[a-f0-9]{64}$/);
      owned.push(id);
      namespaces.add(docker('inspect', '--format', '{{.NetworkSettings.SandboxID}}', id));
      const addr = docker('port', id, '18080/tcp').match(/^127\.0\.0\.1:(\d+)$/);
      assert(addr);
      const backend = { host: '127.0.0.1', port: Number(addr[1]) };
      assert.equal(await request(backend.port), slot);
      backends.push(backend);
    }
    assert.equal(namespaces.size, 2);
    mkdirSync(reports, { mode: 0o700 });
    writeFileSync(listeners, JSON.stringify([{ role: 'api', protocol: 'external-http', max_connections: 4,
      listen: { host: '127.0.0.1', port }, backends }]), { mode: 0o600 });
    proxy = spawn('python3', ['deploy/architecture/replica-proxy.py', listeners], {
      env: { ...process.env, INFRAPRO_DIAGNOSTICS_REPORT_FILE: pidFile }, stdio: 'ignore' });
    exited = once(proxy, 'exit');
    for (let i = 0; i < 150 && !existsSync(pidFile); i++) await delay(30);
    assert.equal(Number(readFileSync(pidFile, 'utf8')), proxy.pid);
    execFileSync(cli, ['diagnostics', 'config', 'init', '--profile', 'native', '--topology', 'split',
      '--instance-id', 'proxy-check', '--directory', cfg, '--local-reports', '--report-directory', reports,
      '--runtime-uid', String(process.getuid()), '--proxy-listeners', listeners], { stdio: 'pipe' });
    assert.deepEqual([await request(port), await request(port)].sort(), ['a', 'b']);
    const live = check();
    assert.equal(live['replica-proxy-presence']?.outcome, 'ok');
    assert.equal(live['proxy-api-external-http']?.outcome, 'ok');
    changeBackend('stop', owned[0]);
    assert.deepEqual([await request(port), await request(port)], ['b', 'b']);
    proxy.kill('SIGTERM');
    await exited;
    const stopped = check();
    assert.equal(stopped['replica-proxy-presence']?.reason_code, 'process_locator_missing');
    assert.equal(stopped['proxy-api-external-http']?.reason_code, 'connection_refused');
    return { live, stopped, backend_network_namespaces: namespaces.size, forwarded_to_two_slots: true, surviving_backend_routed: true };
  } finally {
    if (proxy && proxy.exitCode === null && proxy.signalCode === null) { proxy.kill('SIGKILL'); await exited; }
    for (const id of owned) changeBackend('rm', id);
  }
}
