// Observe competing local acceptance jobs. Persist only the owning command
// class and PID, never command lines, URLs, environment or access material.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const [output, campaign, mode = 'check'] = process.argv.slice(2);
assert.ok(output && /^[a-z][a-z0-9]{0,20}$/.test(campaign) && ['check', 'watch', 'summary'].includes(mode));
const directory = resolve(output, 'ingestion', 'campaigns', campaign);
const evidence = join(directory, 'competing-jobs.jsonl');
mkdirSync(directory, { recursive: true, mode: 0o700 });
function jobs() {
  const competing = [],
    resident = [];
  const running = execFileSync('ps', ['-axo', 'pid=,ppid=,comm=,args='], {
    encoding: 'utf8',
    maxBuffer: 8 << 20,
  });
  const rows = running.split('\n').flatMap((row) => {
    const match = /^\s*(\d+)\s+(\d+)\s+(\S+)\s+(.+)$/.exec(row);
    return match
      ? [{ pid: Number(match[1]), parent: Number(match[2]), program: basename(match[3]), args: match[4] }]
      : [];
  });
  const owns = new Set(
    rows
      .filter(
        (r) =>
          r.program === 'node' &&
          /scripts\/architecture-ingestion-load\.mjs(?:\s|$)/.test(r.args) &&
          r.args.split(/\s+/).includes(campaign),
      )
      .map((r) => r.pid),
  );
  for (let size = -1; size !== owns.size; ) {
    size = owns.size;
    for (const r of rows) if (owns.has(r.parent)) owns.add(r.pid);
  }
  for (const { pid, program, args } of rows) {
    if (owns.has(pid)) continue;
    let job;
    if (program === 'node') {
      const command =
        /(?:^|\s)(?:\S*\/)?scripts\/(architecture(?:-[\w-]+)?|test-[\w-]+|[\w-]+-(?:regression|browser-check|test-environment)|e2e|go|diagnostics-build)\.mjs(?:\s|$)/.exec(
          args,
        );
      // A linter may inspect an acceptance script without executing it.
      const lint = /(?:^|\s)(?:\S*\/)?eslint(?:\.js)?(?=\s|$)/.exec(args);
      if (lint && (!command || lint.index < command.index)) {
        job = 'ESLint';
      } else if (command) {
        if (command[1] === 'architecture-serve') {
          resident.push({ pid, job: command[1] });
          continue;
        }
        job = command[1];
      } else if (/\s--test\s/.test(args) || /\/playwright(?:-core)?\/cli\.js\s+test\b/.test(args)) {
        job = 'Node or browser test';
      } else if (/\/(?:typescript\/bin\/tsc|vite\/bin\/vite\.js\s+build)\b/.test(args)) {
        job = 'frontend build';
      }
    } else if (program === 'go' && /\s(test|build|vet)\s/.test(args))
      job = 'go ' + /\s(test|build|vet)\s/.exec(args)[1];
    else if (program.endsWith('.test')) job = 'Go integration test';
    else if (
      /^python[\d.]*$/.test(program) &&
      /(?:^|\s)(?:\S*\/)?(?:scripts\/architecture-|deploy\/production\/(?:architecture-restore|production-backup))/.test(
        args,
      )
    )
      job = 'architecture validation';
    if (job) competing.push({ pid, job });
  }
  return {
    competing: competing.sort((a, b) => a.pid - b.pid),
    resident: resident.sort((a, b) => a.pid - b.pid),
  };
}
const snapshot = () => {
  const at = new Date().toISOString();
  const record = { at, ...jobs() };
  appendFileSync(evidence, JSON.stringify(record) + '\n', { mode: 0o600 });
  return record;
};
if (mode === 'summary') {
  const records = readFileSync(evidence, 'utf8')
    .trim()
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
  const phases = readFileSync(join(directory, 'timeline.jsonl'), 'utf8')
    .trim()
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
  const rounds = [];
  for (const start of phases.filter((p) => p.phase === 'measuring')) {
    const end = phases.find((p) => p.round === start.round && p.phase === 'verified');
    if (!end) continue;
    const within = records.filter(
      (r) => Date.parse(r.at) >= Date.parse(start.at) && Date.parse(r.at) <= Date.parse(end.at),
    );
    rounds.push({
      round: start.round,
      samples: within.length,
      first: within[0]?.at ?? null,
      last: within.at(-1)?.at ?? null,
      competing: within.filter((r) => r.competing.length).map((r) => ({ at: r.at, jobs: r.competing })),
    });
  }
  const out = { sampling_seconds: 10, rounds };
  writeFileSync(join(directory, 'competing-jobs-summary.json'), JSON.stringify(out, null, 2) + '\n', {
    mode: 0o600,
  });
  console.log(JSON.stringify(out));
} else if (mode === 'check') {
  const row = snapshot();
  console.log(JSON.stringify(row));
  if (row.competing.length) process.exitCode = 1;
} else {
  const report = join(resolve(output), 'ingestion', 'comparison.json');
  for (;;) {
    snapshot();
    if (
      existsSync(join(directory, 'stopped.json')) ||
      (existsSync(report) && JSON.parse(readFileSync(report, 'utf8')).campaign === campaign)
    )
      break;
    await delay(10000);
  }
}
