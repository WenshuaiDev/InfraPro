import { spawn, execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, realpathSync, openSync, closeSync, chmodSync, renameSync } from 'node:fs';
import { resolve, join, dirname } from 'node:path';

export const root = resolve('.');
export const artifacts = join(root, '.artifacts/diagnostics');
export function readJSON(path) { return JSON.parse(readFileSync(path, 'utf8')); }
export function privateJSON(path, value) { mkdirSync(dirname(path), { recursive: true, mode: 0o700 }); const temp = `${path}.new-${randomBytes(4).toString('hex')}`; writeFileSync(temp, JSON.stringify(value, null, 2) + '\n', { mode: 0o600, flag: 'wx' }); renameSync(temp, path); }
export const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
export async function freePort() { return new Promise((ok, no) => { const server = createServer(); server.once('error', no); server.listen(0, '127.0.0.1', () => { const port = server.address().port; server.close(() => ok(port)); }); }); }
export function cleanEnv(extra = {}) { return Object.fromEntries(Object.entries({ ...process.env, ...extra }).filter(([key]) => !key.startsWith('INFRAPRO_') || Object.hasOwn(extra, key))); }
export function command(cmd, args, options = {}) { try { return execFileSync(cmd, args, { cwd: root, encoding: 'utf8', stdio: 'pipe', maxBuffer: 8 << 20, ...options }); } catch { throw Error(`Diagnostics lab command failed (${cmd.split('/').at(-1)} ${args[0] ?? ''}); inspect the private lab logs.`); } }
export function saveLab(lab) { privateJSON(join(lab.directory, 'lab.json'), lab); if (!lab.ephemeral) privateJSON(join(artifacts, 'active.json'), { run_id: lab.run_id, directory: lab.directory }); }
export function loadLab() { const active = readJSON(join(artifacts, 'active.json')); const lab = readJSON(join(active.directory, 'lab.json')); if (lab.run_id !== active.run_id || !/^diag-[a-f0-9]{16}$/.test(lab.run_id) || ![realpathSync(tmpdir()),realpathSync('/tmp')].some((dir)=>lab.directory.startsWith(dir + '/infrapro-diag-'))) throw Error('Invalid lab ownership record'); return lab; }
export async function startProcess(lab, name, executable, args, env = {}) {
  if (lab.processes[name]?.running && processAlive(lab, name)) throw Error(`Owned process already running: ${name}`);
  const log = join(lab.directory, 'logs', `${name}.log`); const fd = openSync(log, 'a', 0o600); chmodSync(log, 0o600);
  const child = spawn(executable, args, { cwd: root, detached: true, stdio: ['ignore', fd, fd], env: cleanEnv(env) }); closeSync(fd); child.unref();
  await new Promise((ok, no) => { child.once('spawn', ok); child.once('error', no); });
  await sleep(100);
  let identity;
  try { identity = JSON.parse(command(lab.helper, ['identity', String(child.pid)])); } catch { throw Error(`Owned process exited during startup: ${name}; inspect ${log}`); }
  const identityFile = join(lab.directory, 'processes', `${name}.json`); privateJSON(identityFile, identity);
  lab.processes[name] = { identity, identity_file: identityFile, executable, args, env, log, running: true };
  saveLab(lab); return child;
}
export function processAlive(lab, name) {
  const p = lab.processes[name]; if (!p?.running) return false;
  try { return JSON.stringify(JSON.parse(command(lab.helper, ['identity', String(p.identity.pid)]))) === JSON.stringify(p.identity); } catch { return false; }
}
export async function stopProcess(lab, name, signal = 'TERM') {
  const p = lab.processes[name]; if (!p) return;
  if (processAlive(lab, name)) {
    command(lab.helper, ['signal', p.identity_file, signal]);
    if (signal === 'STOP' || signal === 'CONT') return;
    for (let i = 0; i < 100 && processAlive(lab, name); i++) await sleep(100);
    if (processAlive(lab, name)) command(lab.helper, ['signal', p.identity_file, 'KILL']);
  }
  p.running = false; saveLab(lab);
}
export async function resumeProcess(lab, name) { const p = lab.processes[name]; if (!p || processAlive(lab, name)) return; return startProcess(lab, name, p.executable, p.args, p.env); }
export async function waitHTTP(origin, path = '/healthz', expected = 200) { for (let i = 0; i < 90; i++) { try { if ((await fetch(origin + path, { signal: AbortSignal.timeout(1000) })).status === expected) return; } catch { /* owned startup */ } await sleep(300); } throw Error('Diagnostics lab listener did not become ready'); }
export function compose(lab, args, options = {}) {
  if (!lab.compose_file || !lab.compose_project?.startsWith('infrapro-diag-')) throw Error('Lab has no owned compose project');
  return command('docker', ['compose', '-p', lab.compose_project, '-f', lab.compose_file, ...args], options);
}
export function ownedContainers(lab) {
  if (!lab.compose_project) return [];
  return command('docker', ['ps', '-a', '-q', '--filter', `label=io.infrapro.lab.run_id=${lab.run_id}`, '--filter', `label=com.docker.compose.project=${lab.compose_project}`]).trim().split('\n').filter(Boolean);
}
export function checkContainerOwnership(lab, id) {
  const value = JSON.parse(command('docker', ['inspect', '--format', '{{json .Config.Labels}}', id]));
  if (value['io.infrapro.lab.run_id'] !== lab.run_id || value['com.docker.compose.project'] !== lab.compose_project) throw Error('Container ownership changed');
}
export async function stopLab(lab) {
  for (const name of Object.keys(lab.processes).reverse()) await stopProcess(lab, name);
  if (lab.compose_project) { for (const id of ownedContainers(lab)) checkContainerOwnership(lab, id); compose(lab, ['down', '--volumes', '--remove-orphans']); }
  lab.state = 'stopped'; saveLab(lab);
}

export async function startDiagnosticsLab({ build = true, business = false, ephemeral = false } = {}) {
  mkdirSync(artifacts, { recursive: true, mode: 0o700 });
  if (build) { execFileSync('node', ['scripts/diagnostics-build.mjs'], { cwd: root, stdio: 'inherit' }); execFileSync('go', ['build', '-o', 'build/bin/infrapro', './tools/infrapro/cmd/infrapro'], { cwd: root, stdio: 'inherit' }); execFileSync('go', ['build', '-o', 'build/bin/diagnostics-labtool', './services/diagnostics/internal/labtool'], { cwd: root, stdio: 'inherit' }); }
  const directory = mkdtempSync(join(realpathSync('/tmp'), 'infrapro-diag-')); chmodSync(directory, 0o700);
  const run = `diag-${randomBytes(8).toString('hex')}`;
  const lab = { schema_version: 1, run_id: run, directory, instance_id: run, business, ephemeral, state: 'starting', created_at: new Date().toISOString(), helper: join(root, 'build/bin/diagnostics-labtool'), binary: join(root, 'build/bin/infrapro-diagnostics'), cli: join(root, 'build/bin/infrapro'), processes: {}, faults: [], ports: {}, evidence: {} };
  for (const name of ['logs', 'processes', 'reports', 'evidence']) mkdirSync(join(directory, name), { mode: 0o700 });
  lab.ports.diagnostics = await freePort(); lab.origin = `http://127.0.0.1:${lab.ports.diagnostics}`;
  const configDir = join(directory, 'diagnostics');
  command(lab.cli, ['diagnostics', 'config', 'init', '--directory', configDir, '--instance-id', run, '--profile', 'mixed']);
  lab.server_file = join(configDir, 'server.json');
  const cfg = readJSON(lab.server_file); cfg.listen = `127.0.0.1:${lab.ports.diagnostics}`; cfg.public_origin = lab.origin; cfg.allowed_hosts = [`127.0.0.1:${lab.ports.diagnostics}`]; privateJSON(lab.server_file, cfg);
  const bindings = readJSON(join(configDir, 'bindings.json')); bindings.bindings.find((b) => b.id === 'diagnostics-ready').url = lab.origin + '/readyz'; privateJSON(join(configDir, 'bindings.json'), bindings);
  lab.accounts_file = join(directory, 'accounts.json'); command(lab.helper, ['auth', lab.server_file, lab.accounts_file]);
  lab.manifest_file = cfg.manifest_file;
  // The three agents have independent keys, process identities, state and files.
  const manifest = readJSON(cfg.manifest_file);
  manifest.display_name = '隔离本机验收 · 独立运行诊断';
  for (const id of ['native-a', 'native-b', 'native-c']) {
    const observerDir = join(directory, id); mkdirSync(observerDir, { mode: 0o700 }); mkdirSync(join(observerDir, 'state'), { mode: 0o700 });
    manifest.nodes.push({ id, display_name: `逻辑测试节点 ${id}`, kind: 'logical_test', resource_scope: 'none', assignment_revision: '1', enabled: true });
    const probe = `${id}-diagnostics`; manifest.probes.push({ id: probe, node_id: id, target_kind: 'node', target_id: id, adapter: 'http_live', binding_id: probe, facet: 'readiness', required: false, interval_ms: 5000, timeout_ms: 1500, stale_after_ms: 20000 });
    const binding = { schema_version: 1, instance_id: run, node_id: id, assignment_revision: '1', bindings: [{ id: probe, adapter: 'http_live', url: lab.origin + '/healthz', expected_status: 200, expected_json: { status: 'ok' } }] };
    privateJSON(join(observerDir, 'bindings.json'), binding);
    privateJSON(join(observerDir, 'observer.json'), { schema_version: 1, instance_id: run, node_id: id, assignment_revision: '1', server_url: lab.origin, manifest_file: cfg.manifest_file, bindings_file: join(observerDir, 'bindings.json'), credential_file: join(observerDir, 'key.json'), state_dir: join(observerDir, 'state') });
  }
  manifest.revision = '2'; privateJSON(cfg.manifest_file, manifest);
  for (const id of ['native-a', 'native-b', 'native-c']) command(lab.cli, ['diagnostics', 'observer-key', 'issue', '--server', lab.server_file, '--node-id', id, '--output-file', join(directory, id, 'key.json')]);
  saveLab(lab);
  try {
    if (business) { const { prepareBusiness } = await import('./diagnostics-business.mjs'); await prepareBusiness(lab); }
    await startProcess(lab, 'server', lab.binary, ['server', '--config', lab.server_file]); await waitHTTP(lab.origin);
    for (const id of ['native-a', 'native-b', 'native-c']) await startProcess(lab, id, lab.binary, ['observer', '--config', join(directory, id, 'observer.json')]);
    await sleep(1500); lab.state = 'running'; saveLab(lab); return lab;
  } catch (error) { await stopLab(lab); throw error; }
}
