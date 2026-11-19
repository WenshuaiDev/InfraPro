import assert from 'node:assert/strict';
import { execFile, execFileSync } from 'node:child_process';
import { readFileSync, realpathSync } from 'node:fs';
import { availableParallelism, cpus, totalmem } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { digest, read } from './architecture-lab.mjs';

const execute = promisify(execFile);

export function displayProcessLimits(stack, manifest) {
  const entries = Object.entries(stack.processes).map(([role, child]) => {
    const { pid } = child;
    const actualExe = realpathSync(
      process.platform === 'linux'
        ? '/proc/' + pid + '/exe'
        : execFileSync('ps', ['-ww', '-p', String(pid), '-o', 'comm='], {
            encoding: 'utf8',
          }).trim(),
    );
    assert.equal(actualExe, realpathSync(join(manifest.bin, role)));
    const binarySHA256 = digest(readFileSync(actualExe));
    assert.equal(binarySHA256, manifest.binaries[role]);
    // The operating system can return the complete initial environment. Only
    // these two non-sensitive settings leave this function.
    const env =
      process.platform === 'linux'
        ? readFileSync('/proc/' + pid + '/environ', 'utf8').split('\0')
        : execFileSync('ps', ['eww', '-p', String(pid), '-o', 'command='], {
            encoding: 'utf8',
          })
            .trim()
            .split(/\s+/);
    const value = (key) => env.find((v) => v.startsWith(key + '='))?.slice(key.length + 1);
    const [, ch, goThreads, heapMiB] = stack.budget.roles[role];
    assert.equal(value('GOMAXPROCS'), String(goThreads), role + ' OS scheduler setting');
    assert.equal(value('GOMEMLIMIT'), heapMiB + 'MiB', role + ' OS heap setting');
    const split = stack.side === 'b';
    const topology = split
      ? read(join(stack.workspace, 'topology', 'roles', role, 'architecture.json'))
      : null;
    const inflight = split ? (topology.max_inflight?.[role] ?? 32) : null;
    // The frozen application applies the per-endpoint min(6, HTTP quota).
    // This fixture intentionally defines exactly one query endpoint backed by two data replicas.
    const endpoints = stack.cfg.monitoring_clickhouse_replicas.split(',').length;
    assert.equal(endpoints, 1);
    if (ch)
      assert.equal(
        split ? Math.min(6, Math.floor(inflight / endpoints)) * endpoints : 6 * endpoints,
        ch,
      );
    return [
      role,
      {
        pid,
        binarySHA256,
        goThreads,
        heapMiB,
        clickhouseLimit: ch,
        httpInflight: inflight,
      },
    ];
  });
  return Object.fromEntries(entries);
}

export async function openDisplaySockets(stack) {
  const processes = Object.fromEntries(
    Object.entries(stack.processes).map(([name, p]) => [p.pid, name]),
  );
  const addr = stack.cfg.monitoring_clickhouse_replicas.split(',')[0];
  const chPort = new URL(addr.includes('://') ? addr : 'tcp://' + addr).port;
  const pgPort = new URL(stack.cfg.database_url).port;
  const roles = Object.fromEntries(
    Object.values(processes).map((r) => [r, { clickhouse: 0, postgres: 0 }]),
  );
  const { stdout } = await execute(
    'lsof',
    ['-nP', '-a', '-p', Object.keys(processes).join(','), '-iTCP', '-sTCP:ESTABLISHED', '-Fpn'],
    { maxBuffer: 4 << 20 },
  );
  let role;
  for (const line of stdout.split('\n')) {
    if (line.startsWith('p')) role = processes[line.slice(1)];
    if (!role || !line.startsWith('n') || !line.includes('->')) continue;
    const port = line.match(/:(\d+)$/)?.[1];
    if (port === chPort) roles[role].clickhouse++;
    if (port === pgPort) roles[role].postgres++;
  }
  return { at: new Date().toISOString(), roles };
}

export function displayHardware() {
  const docker = JSON.parse(
    execFileSync('docker', ['info', '--format', '{{json .}}'], {
      encoding: 'utf8',
    }),
  );
  const host = {
    platform: process.platform,
    model: cpus()[0].model,
    logicalCPUs: availableParallelism(),
    memoryBytes: totalmem(),
  };
  if (process.platform === 'darwin') {
    const sysctl = (key) =>
      Number(execFileSync('sysctl', ['-n', key], { encoding: 'utf8' }).trim());
    host.physicalCPUs = sysctl('hw.physicalcpu');
    const levels = sysctl('hw.nperflevels');
    host.cpuClasses = Array.from({ length: levels }, (_, i) => ({
      name: execFileSync('sysctl', ['-n', 'hw.perflevel' + i + '.name'], {
        encoding: 'utf8',
      }).trim(),
      physical: sysctl('hw.perflevel' + i + '.physicalcpu'),
    }));
  }
  return { host, docker: { cpus: docker.NCPU, memoryBytes: docker.MemTotal } };
}

export function publishedServiceLimits(preset) {
  const limits = {};
  for (const [side, roles] of [
    ['a', preset.schema58],
    ['b', preset.roles],
  ]) {
    const cpu = Object.values(roles).reduce((sum, slot) => sum + slot.cpu, 0);
    const memoryMiB = Object.values(roles).reduce((sum, slot) => sum + slot.memory_mib, 0);
    limits[side] = { cpu, memoryMiB };
  }
  assert.deepEqual(
    limits.a,
    limits.b,
    'The two published process profiles must have equal total service limits',
  );
  return limits.a;
}

// A completed scene must be identifiable from its archived OS observations.
// Configuration alone does not show which binary ran or which pool it used.
export function measuredDisplayBudget(row, profile, binaries) {
  const budget = row?.budget,
    planned = profile?.roles?.[row?.side],
    deployed = budget?.deployed;
  if (
    !budget ||
    !planned ||
    !binaries ||
    !deployed ||
    budget.profile !== profile.budgetProfile ||
    JSON.stringify(budget.roles) !== JSON.stringify(planned) ||
    JSON.stringify(budget.total) !== JSON.stringify(profile.total)
  )
    return false;
  const roles = Object.keys(planned),
    active = budget.processLimits ?? {};
  if (Object.keys(active).length !== roles.length || Object.keys(deployed).length !== roles.length)
    return false;
  const limits = Object.values(deployed);
  if (
    !profile.serviceLimits ||
    limits.reduce((sum, slot) => sum + slot.cpu, 0) !== profile.serviceLimits.cpu ||
    limits.reduce((sum, slot) => sum + slot.memoryMiB, 0) !== profile.serviceLimits.memoryMiB
  )
    return false;
  const pids = new Set();
  for (const role of roles) {
    const [pg, ch, threads, heap] = planned[role],
      proc = active[role],
      slot = deployed[role];
    if (
      !proc ||
      !slot ||
      !Number.isInteger(proc.pid) ||
      proc.pid < 1 ||
      pids.has(proc.pid) ||
      !/^[a-f0-9]{64}$/.test(proc.binarySHA256 ?? '') ||
      proc.binarySHA256 !== binaries[role] ||
      proc.goThreads !== threads ||
      slot.goThreads !== threads ||
      proc.heapMiB !== heap ||
      slot.goMemoryMiB !== heap ||
      proc.clickhouseLimit !== ch ||
      slot.cpu <= 0 ||
      slot.memoryMiB < heap ||
      (row.side === 'b' && pg && budget.roleLogins?.accounts?.[role]?.limit !== pg)
    )
      return false;
    pids.add(proc.pid);
  }
  const objects = budget.objectRequests;
  if (
    objects?.limit !== profile.objectRequests ||
    objects.peak < 1 ||
    objects.peak > objects.limit ||
    objects.completed < 1
  )
    return false;
  const samples = (row.resources ?? []).filter((s) => s.connections?.roles);
  if (!samples.length) return false;
  let usingPostgres = false,
    usingClickHouse = false;
  for (const { connections } of samples)
    for (const role of roles) {
      const [pg, ch] = planned[role],
        seen = connections.roles[role];
      if (
        !seen ||
        !Number.isInteger(seen.postgres) ||
        seen.postgres < 0 ||
        seen.postgres > pg ||
        !Number.isInteger(seen.clickhouse) ||
        seen.clickhouse < 0 ||
        seen.clickhouse > ch
      )
        return false;
      usingPostgres ||= seen.postgres > 0;
      usingClickHouse ||= seen.clickhouse > 0;
    }
  return usingPostgres && usingClickHouse;
}
