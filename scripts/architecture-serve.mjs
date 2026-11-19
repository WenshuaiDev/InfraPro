// Starts only the private databases and listeners recorded by architecture-lab.
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { connect, createServer } from 'node:net';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';
import { availablePort, stop } from './fixtures/parsing-lab.mjs';
import { configEnvironment, walkthroughProbes } from './architecture-walkthrough-runtime.mjs';
import { walkthroughDiagnosticServer } from './architecture-walkthrough-evidence.mjs';
import { checkoutApplicationDigest, roleEnvironment } from './architecture-runtime.mjs';
import { provision } from './architecture-postgres.mjs';
import { archiveCandidateSource } from './architecture-source-archive.mjs';
import { assetDigests as fileDigests, sharedConsoleFor } from './architecture-shared-console.mjs';
import {
  digest,
  fileLease,
  launch,
  prepareLab,
  prepareWalkthroughSide,
  processEnv,
  read,
  root,
  runDirectory,
  sleep,
  write,
} from './architecture-lab.mjs';

function run(cmd, args, opts = {}) {
  return execFileSync(cmd, args, {
    cwd: root,
    stdio: 'pipe',
    maxBuffer: 12 << 20,
    ...opts,
  });
}
const auxiliary = ['notification-worker', 'audit-worker', 'task-index-worker'];
const baselineHTTP = ['api', 'display-runtime', 'monitoring-worker', ...auxiliary];
const candidateHTTP = [
  'identity-api',
  'control-api',
  'monitoring-query-api',
  'transfer-api',
  'api',
  'display-runtime',
  'display-assets',
  'monitoring-read-worker',
  'monitoring-publish-worker',
  'monitoring-incremental-worker',
  'monitoring-history-worker',
  ...auxiliary,
];
export async function availableRange(n, firstAvailable = availablePort) {
  for (let i = 0; i < 100; i++) {
    const port = await firstAvailable();
    if (port + n > 65534) continue;
    const listeners = [];
    try {
      for (let idx = 0; idx < n; idx++) {
        const listener = createServer();
        listeners.push(listener);
        await new Promise((ok, no) =>
          listener.once('error', no).listen(port + idx, '127.0.0.1', ok),
        );
      }
      return port;
    } catch {
      /* retry a different ephemeral range */
    } finally {
      await Promise.all(
        listeners.filter((s) => s.listening).map((s) => new Promise((r) => s.close(r))),
      );
    }
  }
  throw Error('No consecutive private ports available');
}
export async function dependencyGate(address) {
  const [host, port] = address.split(':');
  const connections = new Set();
  let enabled = true;
  const listener = createServer((socket) => {
    if (!enabled) {
      socket.destroy();
      return;
    }
    const upstream = connect(Number(port), host);
    connections.add(socket);
    connections.add(upstream);
    for (const stream of [socket, upstream]) {
      stream.on('error', () => {
        socket.destroy();
        upstream.destroy();
      });
      stream.on('close', () => {
        connections.delete(stream);
      });
    }
    socket.pipe(upstream);
    upstream.pipe(socket);
  });
  await new Promise((ok, no) => listener.once('error', no).listen(0, '127.0.0.1', ok));
  return {
    address: '127.0.0.1:' + listener.address().port,
    set(open) {
      enabled = open;
      if (!open) for (const socket of connections) socket.destroy();
    },
    close() {
      this.set(false);
      return new Promise((done) => listener.close(done));
    },
  };
}
function prepareDiagnosticsPrograms(lab, side, bin, source) {
  const helper = join(bin, 'diagnostics-labtool');
  if (!existsSync(helper))
    run('go', ['build', '-o', helper, './services/diagnostics/internal/labtool'], { cwd: source });
  if (!existsSync(join(bin, 'infrapro-diagnostics'))) {
    if (side !== 'b')
      cpSync(join(lab.assets, 'diagnostics'), join(source, 'apps/diagnostics/dist'), {
        recursive: true,
      });
    run(process.execPath, ['scripts/diagnostics-build.mjs', '--skip-ui'], {
      cwd: source,
    });
    cpSync(join(source, 'build/bin/infrapro-diagnostics'), join(bin, 'infrapro-diagnostics'));
  }
}
export function diagnostics(lab, side, bin, source, brokerProbe, walkthrough) {
  const split = side === 'b';
  const dir = join(lab.directory, 'private', side, 'diagnostics');
  const config = join(dir, 'server.json');
  mkdirSync(dirname(dir), { recursive: true, mode: 0o700 });
  const cli = join(bin, 'infrapro');
  const helper = join(bin, 'diagnostics-labtool');
  prepareDiagnosticsPrograms(lab, side, bin, source);
  if (!existsSync(config)) {
    const args = [
      'diagnostics',
      'config',
      'init',
      '--directory',
      dir,
      '--instance-id',
      'arc-' + lab.id,
      '--profile',
      split ? 'native' : 'mixed',
    ];
    if (split)
      args.push(
        '--topology',
        'split',
        '--bin-directory',
        bin,
        '--report-directory',
        join(lab.directory, 'private', side, 'reports'),
        '--runtime-uid',
        String(process.getuid()),
      );
    if (split && brokerProbe)
      args.push(
        '--broker-pid-file',
        brokerProbe.pidFile,
        '--broker-executable',
        brokerProbe.binary,
        '--broker-monitor-port',
        String(brokerProbe.port),
      );
    run(cli, args);
    const cfg = read(config);
    const host = '127.0.0.1:' + lab[side].ports.diagnostics;
    Object.assign(cfg, {
      listen: host,
      public_origin: 'http://' + host,
      allowed_hosts: [host],
    });
    write(config, cfg);
    run(helper, ['auth', config, join(dir, 'accounts.json')]);
  }
  const bindingsPath = join(dir, 'bindings.json'),
    manifestPath = join(dir, 'manifest.json');
  const bindings = read(bindingsPath),
    manifest = read(manifestPath);
  const bind = (key) => bindings.bindings.find((b) => b.id === key);
  bind('diagnostics-ready').url = 'http://127.0.0.1:' + lab[side].ports.diagnostics + '/readyz';
  // A sealed second release lives at its own path. Match the executable that
  // is actually selected, including when the saved login configuration is reused.
  for (const binding of bindings.bindings) {
    if (binding.expected_executable && binding.id !== 'nats-server-presence') {
      const executable = binding.id === 'diagnostics-process' && walkthrough?.diagnosticServer
        ? walkthrough.diagnosticServer.binary : join(bin, basename(binding.expected_executable));
      if (existsSync(executable)) binding.expected_executable = executable;
    }
  }
  if (brokerProbe) {
    const presence = bindings.bindings.findIndex((b) => b.id === 'nats-server-presence');
    if (presence < 0) throw Error('Split topology is missing its broker presence probe');
    bindings.bindings[presence] = {
      id: 'nats-server-presence',
      adapter: 'native_process',
      pid_file: brokerProbe.pidFile,
      pid_file_format: 'decimal',
      allowed_root: dirname(brokerProbe.pidFile),
      expected_executable: brokerProbe.binary,
      expected_uid: process.getuid(),
    };
    manifest.probes.find((p) => p.id === 'nats-server-presence').adapter = 'native_process';
    bind('nats-server-ready').port = brokerProbe.port;
  }
  if (walkthrough) {
    walkthroughProbes({ lab, side, bin, manifest, bindings, ...walkthrough });
    // A restarted network gate or parser can have a new port. Apply both
    // the declaration and local assignment as a new monotonic revision.
    manifest.revision = String(BigInt(manifest.revision) + 1n);
    const node = manifest.nodes.find((row) => row.id === 'local');
    node.assignment_revision = String(BigInt(node.assignment_revision) + 1n);
    bindings.assignment_revision = node.assignment_revision;
    const server = read(config);
    server.local_observer.assignment_revision = node.assignment_revision;
    write(config, server);
  }
  write(bindingsPath, bindings);
  write(manifestPath, manifest);
  if (walkthrough) run(cli, ['diagnostics', 'config', 'validate', '--server', config]);
  return config;
}
function candidateInputs() {
  return {
    commit: run('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(),
    sourceSHA256: checkoutApplicationDigest(root),
    go: run('go', ['version'], { encoding: 'utf8' }).trim(),
    pnpm: run('pnpm', ['--version'], { encoding: 'utf8' }).trim(),
    node: process.version,
  };
}
function goCommands() {
  return [
    ...readdirSync(join(root, 'services/api/cmd'), { withFileTypes: true })
      .filter((f) => f.isDirectory())
      .map((f) => './services/api/cmd/' + f.name),
    './services/ingest-relay/cmd/ingest-relay',
    './services/parser/cmd/parser',
    './services/parser/cmd/parser-runner',
    './services/parser/cmd/parser-package',
    './tools/infrapro/cmd/infrapro',
  ];
}
function executableNames(directories) {
  return [
    ...directories.map((d) => d.slice(d.lastIndexOf('/') + 1)),
    'infrapro-diagnostics',
    'diagnostics-labtool',
  ];
}
export function prepareCandidate(dir) {
  const release = join(dir, 'candidate-assets'),
    bin = join(dir, 'candidate-bin'),
    manifestFile = join(dir, 'candidate.json');
  const inputs = candidateInputs(),
    previous = existsSync(manifestFile) ? read(manifestFile) : null;
  const directories = goCommands(),
    names = executableNames(directories);
  const migrationSHA256 = digest(
    readFileSync(join(root, 'packages/go/platform/migrations/00059_architecture_events.sql')),
  );
  const binaries = () =>
    Object.fromEntries(
      names
        .filter((n) => existsSync(join(bin, n)))
        .map((n) => [n, digest(readFileSync(join(bin, n)))]),
    );
  if (
    previous?.schema?.version === 59 &&
    previous.schema.migrationSHA256 === migrationSHA256 &&
    JSON.stringify(previous.inputs) === JSON.stringify(inputs) &&
    names.every((n) => previous.binaries[n]) &&
    JSON.stringify(previous.binaries) === JSON.stringify(binaries()) &&
    JSON.stringify(previous.staticFiles) === JSON.stringify(fileDigests(release))
  ) {
    if (!existsSync(join(dir, 'application-source.json'))) archiveCandidateSource(dir, previous);
    return previous;
  }
  mkdirSync(bin, { recursive: true });
  run('pnpm', ['build:apps']);
  for (const app of ['console', 'display-runtime', 'diagnostics']) {
    const target = join(release, app);
    rmSync(target, { force: true, recursive: true });
    cpSync(join(root, 'apps', app, 'dist'), target, { recursive: true });
  }
  run('go', ['build', '-trimpath', '-o', bin + '/', ...directories]);
  run('go', [
    'build',
    '-trimpath',
    '-o',
    join(bin, 'diagnostics-labtool'),
    './services/diagnostics/internal/labtool',
  ]);
  run(process.execPath, ['scripts/diagnostics-build.mjs', '--skip-ui']);
  cpSync(join(root, 'build/bin/infrapro-diagnostics'), join(bin, 'infrapro-diagnostics'));
  const result = {
    inputs,
    release,
    bin,
    binaries: binaries(),
    staticFiles: fileDigests(release),
    builtAt: new Date().toISOString(),
    schema: { version: 59, migrationSHA256 },
  };
  const baseline = read(join(dir, 'baseline.json'));
  write(join(dir, 'static-files.json'), {
    baseline: baseline.staticFiles,
    candidate: result.staticFiles,
    different: [
      ...new Set([...Object.keys(result.staticFiles), ...Object.keys(baseline.staticFiles)]),
    ].filter((k) => result.staticFiles[k] !== baseline.staticFiles[k]),
  });
  if (candidateInputs().sourceSHA256 !== inputs.sourceSHA256)
    throw Error('Candidate inputs changed during the build; re-run preparation');
  write(manifestFile, result);
  archiveCandidateSource(dir, result);
  return result;
}
// A second running experiment may be measuring a sealed candidate. Start the
// chosen release without rebuilding or replacing that experiment's binaries.
export function selectedCandidate(directory) {
  const dir = runDirectory(directory),
    build = read(join(dir, 'candidate.json'));
  if (build.bin !== join(dir, 'candidate-bin') || build.release !== join(dir, 'candidate-assets')) {
    throw Error('Candidate manifest points outside its release directory');
  }
  const names = executableNames(goCommands());
  if (
    Object.keys(build.binaries).length !== names.length ||
    names.some((name) => !build.binaries[name])
  ) {
    throw Error('Candidate executable inventory is incomplete');
  }
  for (const [name, hash] of Object.entries(build.binaries)) {
    if (!/^[a-z][a-z0-9-]*$/.test(name) || digest(readFileSync(join(build.bin, name))) !== hash) {
      throw Error('Candidate executable differs from the recorded release: ' + name);
    }
  }
  const actual = fileDigests(build.release);
  if (
    Object.keys(actual).length !== Object.keys(build.staticFiles).length ||
    Object.entries(build.staticFiles).some(([name, hash]) => actual[name] !== hash)
  ) {
    throw Error('Candidate frontend files differ from the recorded release');
  }
  return build;
}
export async function prepareTopology(dir) {
  const target = join(dir, 'topology');
  if (!existsSync(join(target, 'layout.json'))) {
    const port = await availableRange(7);
    run(process.execPath, [
      'scripts/architecture-deploy.mjs',
      'prepare',
      '--mode',
      'local',
      '--directory',
      target,
      '--port-base',
      String(port),
    ]);
  }
  return target;
}
export async function startSide(dir, side, options = {}) {
  if (!options.walkthrough) return startEnvironment(dir, side, options);
  if (!['a', 'b'].includes(side)) throw Error('Unknown walkthrough side');
  const release = await fileLease(join(dir, 'private', 'walkthrough-' + side + '.lock'), {
    wait: false,
  });
  try {
    const stack = await startEnvironment(dir, side, options);
    return {
      ...stack,
      async close() {
        try {
          await stack.close();
        } finally {
          await release();
        }
      },
    };
  } catch (error) {
    await release();
    throw error;
  }
}
async function startEnvironment(
  dir,
  side,
  { withWorkers = true, walkthrough = false, releaseDirectory } = {},
) {
  if (!['a', 'self', 'b'].includes(side)) throw Error('Unsupported architecture environment');
  const parent = dir,
    split = side === 'b';
  const primary = await prepareLab(parent);
  const saved = join(parent, 'manual', 'walkthrough-candidate.json');
  const evaluated = join(parent, 'candidate.evaluated.json');
  const sealed =
    releaseDirectory ??
    process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE ??
    (existsSync(evaluated) ? read(evaluated).directory : undefined) ??
    (walkthrough && existsSync(saved) ? read(saved).directory : undefined) ??
    (walkthrough && existsSync(join(parent, 'candidate.json')) ? parent : undefined);
  const build = split ? (sealed ? selectedCandidate(sealed) : prepareCandidate(parent)) : null;
  const sharedConsole = sharedConsoleFor(parent, { verifyFiles: true });
  if (sharedConsole && split && sharedConsole.backendReleaseSHA256 !== build.inputs.sourceSHA256)
    throw Error('Shared console selected for a different backend release');
  const schema = build?.schema?.migrationSHA256 ?? build?.inputs.sourceSHA256;
  const lab = walkthrough ? await prepareWalkthroughSide(parent, side, schema) : primary;
  dir = lab.directory;
  const cfg = lab[side];
  const base = read(join(dir, 'baseline.json'));
  const source = split ? root : base.source;
  const bin = build?.bin || base.bin;
  let businessEnvironment;
  if (
    !existsSync(join(source, 'node_modules/.bin/vite')) &&
    !existsSync(join(source, 'apps/console/node_modules/vite'))
  ) {
    run('pnpm', ['install', '--offline', '--frozen-lockfile'], { cwd: source });
  }
  if (split) {
    cfg.release = build.release;
    const runtime = read(cfg.config);
    runtime.display.shell_directory = join(build.release, 'display-runtime');
    write(cfg.config, runtime);
    await prepareTopology(dir);
    const env = processEnv(lab, side);
    run(join(bin, 'infrapro'), ['database', 'migrate'], { env });
    const events = (args) =>
      JSON.parse(run(join(bin, 'event-relay'), args, { env, encoding: 'utf8' }));
    const initial = events(['status']);
    for (const row of initial.ownership || []) {
      if (row.mode !== 'split') {
        events([
          'switch',
          '--role',
          row.role,
          '--mode',
          'split',
          '--expected',
          String(row.generation),
          '--reason',
          '隔离对照环境',
        ]);
      }
    }
    write(join(dir, 'worker-ownership.json'), events(['status']));
    if (!walkthrough) {
      const directory = join(dir, 'topology');
      const roleSQL = provision({
        url: runtime.database_url,
        container: lab.container,
        directory,
        localLogins: true,
        roles: [...candidateHTTP, 'files-worker', 'display-package-worker', 'spatial-worker'],
      });
      write(join(dir, 'role-grants.json'), roleSQL);
      const projected = configEnvironment(runtime);
      businessEnvironment = (role) => roleEnvironment(role, projected, directory);
    }
  }
  let diagConfig, diagnosticServer;
  const chGate =
    split || walkthrough
      ? await dependencyGate(read(cfg.config).monitoring_clickhouse_replicas)
      : null;
  const children = {};
  let manual;
  const startRole = (role) => {
    if (children[role]?.exitCode === null && children[role]?.signalCode === null) return;
    const log = join(dir, 'logs', side + '-' + role + '.log');
    const business = !['diagnostics', 'web'].includes(role);
    const environment = manual?.environment ?? businessEnvironment;
    const env = business && environment ? environment(role) : processEnv(lab, side);
    if (business && chGate && env.INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS)
      env.INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS = chGate.address;
    if ((split || manual) && business) {
      const report = join(dir, 'private', side, 'reports', role, 'process.json');
      mkdirSync(dirname(report), { recursive: true, mode: 0o700 });
      Object.assign(env, {
        INFRAPRO_DIAGNOSTICS_REPORT_FILE: report,
        INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'arc-' + lab.id,
        INFRAPRO_DIAGNOSTICS_REPLICA_ID: role + '-1',
        INFRAPRO_DIAGNOSTICS_NODE_ID: 'local',
      });
    }
    if (role === 'web')
      children[role] = launch(
        process.execPath,
        [
          join(source, 'apps/console/node_modules/vite/bin/vite.js'),
          'preview',
          '--outDir',
          sharedConsole?.console.directory || join(cfg.release || base.assets, 'console'),
        ],
        {
          ...env,
          API_PROXY_TARGET: cfg.api,
          CONSOLE_PORT: String(cfg.ports.web),
        },
        log,
        join(source, 'apps/console'),
      );
    else if (role === 'diagnostics')
      children[role] = launch(
        diagnosticServer?.binary ?? join(bin, 'infrapro-diagnostics'),
        ['server', '--config', diagConfig],
        env,
        log,
      );
    else
      children[role] = launch(
        join(bin, role),
        role === 'files-checker'
          ? ['--directory', manual.checker, '--socket', join(manual.checker, 'checker.sock')]
          : [],
        env,
        log,
      );
  };
  async function close() {
    await Promise.all(
      Object.values(children)
        .filter((c) => c.exitCode === null && c.signalCode === null)
        .map(stop),
    );
    if (manual) await manual.close();
    if (chGate) await chGate.close();
  }
  try {
    if (walkthrough) {
      const { openWalkthrough } = await import('./architecture-walkthrough-runtime.mjs');
      manual = await openWalkthrough(lab, side, bin, source, join(dir, 'topology'));
      if (!existsSync(join(bin, 'infrapro-diagnostics'))) prepareDiagnosticsPrograms(lab, side, bin, source);
      diagnosticServer = walkthroughDiagnosticServer(parent, lab, side, build ?? base);
    }
    diagConfig = diagnostics(
      lab,
      side,
      bin,
      source,
      manual?.brokerProbe,
      manual && {
        manual,
        diagnosticServer,
        clickhouse: chGate.address,
        roles: [...baselineHTTP, ...manual.roles],
      },
    );
    const roles = [
      ...(split ? candidateHTTP : baselineHTTP).filter((r) => withWorkers || !r.endsWith('worker')),
      ...(manual?.roles ?? []),
      'web',
      'diagnostics',
    ];
    roles.forEach(startRole);
    const probes = [
      cfg.api + '/readyz',
      cfg.origin + '/api/v1/public/account-help',
      cfg.runtime + '/healthz',
      'http://127.0.0.1:' + cfg.ports.diagnostics + '/healthz',
    ];
    await Promise.all(
      probes.map(async (url) => {
        const until = Date.now() + 90000;
        let last;
        do {
          const exited = Object.entries(children).find(
            ([, p]) => p.exitCode !== null || p.signalCode !== null,
          );
          if (exited)
            throw Error(
              'Stopped ' + side + ' ' + exited[0] + ' during startup; inspect its private log',
            );
          try {
            const r = await fetch(url, { signal: AbortSignal.timeout(2500) });
            await r.body?.cancel();
            if (r.ok) return;
            last = 'HTTP ' + r.status;
          } catch (error) {
            last = error.cause?.code || error.name;
          }
          await sleep(250);
        } while (Date.now() < until);
        throw Error('Walkthrough readiness failed at ' + url + ' (' + last + ')');
      }),
    );
    for (const [role, child] of Object.entries(children)) {
      if (child.exitCode !== null || child.signalCode !== null)
        throw Error('Stopped ' + side + ' ' + role + '; inspect private log');
    }
    const stopRole = (role) => {
      const proc = children[role];
      return proc?.exitCode === null && proc.signalCode === null ? stop(proc) : undefined;
    };
    return {
      lab,
      sharedConsoleSHA256: sharedConsole?.console.sha256,
      side,
      cfg,
      bin,
      diagnosticServer,
      children,
      chGate,
      startRole,
      stopRole,
      close,
      manual,
      releaseSHA256: build?.inputs.sourceSHA256 ?? base.source_archive_sha256,
    };
  } catch (e) {
    await close();
    throw e;
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const {
    positionals: [directory, side = 'a'],
    values,
  } = parseArgs({
    options: {
      walkthrough: { type: 'boolean' },
      'release-dir': { type: 'string' },
    },
    allowPositionals: true,
  });
  const dir = runDirectory(directory),
    walkthrough = values.walkthrough === true;
  const env = await startSide(dir, side, {
    walkthrough,
    releaseDirectory: values['release-dir'],
  });
  let closeControl;
  const shutdown = Promise.withResolvers();
  process.once('SIGTERM', shutdown.resolve);
  process.once('SIGINT', shutdown.resolve);
  try {
    if (walkthrough) {
      const { walkthroughInputs } = await import('./architecture-walkthrough.mjs');
      await walkthroughInputs(dir, env);
    }
    if (walkthrough) {
      const { walkthroughControl } = await import('./architecture-walkthrough-control.mjs');
      closeControl = await walkthroughControl(env, () => shutdown.resolve());
    }
    write(join(dir, 'environment-' + side + '.json'), {
      side,
      sharedConsoleSHA256: env.sharedConsoleSHA256,
      console: env.cfg.origin,
      diagnostics: 'http://127.0.0.1:' + env.cfg.ports.diagnostics,
      runtime: env.cfg.runtime,
      walkthrough,
      ...(walkthrough && { launchID: env.manual.launchID, supervisorPID: process.pid }),
      releaseSHA256: env.releaseSHA256,
      startedAt: new Date().toISOString(),
    });
    console.log(side + ' ready at ' + env.cfg.origin);
    await shutdown.promise;
  } finally {
    process.off('SIGTERM', shutdown.resolve);
    process.off('SIGINT', shutdown.resolve);
    if (closeControl) await closeControl();
    await env.close();
  }
}
