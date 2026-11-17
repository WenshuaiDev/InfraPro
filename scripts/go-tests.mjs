import { spawn, execFileSync } from 'node:child_process';
import { mkdirSync, createWriteStream, writeFileSync } from 'node:fs';
import { join, resolve, relative } from 'node:path';
import { createInterface } from 'node:readline';

// This acceptance case measures a production wall-clock budget. As before,
// run it without race instrumentation; every other test retains -race.
const timedBoundary = 'TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke';
export function planGoTests(packages, size = 8) {
  if (!Number.isInteger(size) || size < 1) throw Error('Go test batch size must be a positive integer');
  const jobs = [];
  for (const { cwd, name, tests } of packages) {
    if (new Set(tests).size !== tests.length) throw Error(`Duplicate discovered tests in ${name}`);
    const regular = tests.filter(test => test !== timedBoundary);
    for (let i = 0; i < regular.length; i += size) jobs.push({ cwd, name, tests: regular.slice(i, i + size), race: true });
    if (tests.includes(timedBoundary)) jobs.push({ cwd, name, tests: [timedBoundary], race: false });
  }
  return jobs;
}

export function checkGoResults(job, completed, code) {
  const missing = job.tests.filter(test => !completed.has(test));
  const unexpected = [...completed.keys()].filter(test => !job.tests.includes(test));
  const failed = [...completed].filter(([, action]) => action === 'fail').map(([test]) => test);
  return { passed: code === 0 && !missing.length && !unexpected.length && !failed.length, missing, unexpected, failed };
}

export async function runGoTests(modules, root) {
  const concurrency = Number(process.env.INFRAPRO_GO_TEST_JOBS ?? 2);
  if (!Number.isInteger(concurrency) || concurrency < 1 || concurrency > 4) throw Error('INFRAPRO_GO_TEST_JOBS must be 1–4');
  const output = resolve(process.env.INFRAPRO_GO_TEST_OUTPUT ?? join(root, '.artifacts/go-tests', new Date().toISOString().replaceAll(':', '-')));
  mkdirSync(output, { recursive: true, mode: 0o700 });
  const packages = [];
  for (const cwd of modules) {
    const events = execFileSync('go', ['test', '-list', '.', '-json', '-p', '2', './...'], { cwd, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
    const found = new Map();
    for (const line of events.split('\n').filter(Boolean)) {
      const event = JSON.parse(line);
      if (!event.Package) continue;
      if (!found.has(event.Package)) found.set(event.Package, []);
      const name = event.Output?.trim();
      if (/^(Test|Example|Fuzz)\S*$/.test(name ?? '')) found.get(event.Package).push(name);
    }
    for (const [name, tests] of found) packages.push({ cwd, name, tests });
  }
  const jobs = planGoTests(packages), results = [], active = new Set();
  const started = Date.now();
  let next = 0, interrupted = false;
  const terminate = () => {
    interrupted = true;
    for (const child of active) {
      try { process.kill(process.platform === 'win32' ? child.pid : -child.pid, 'SIGTERM'); }
      catch (error) { if (error.code !== 'ESRCH') throw error; }
    }
  };
  process.on('SIGINT', terminate); process.on('SIGTERM', terminate);
  writeFileSync(join(output, 'plan.json'), JSON.stringify({ packages, jobs, concurrency }, null, 2));
  console.log(`[go:test] ${jobs.length} batches, ${packages.reduce((n, p) => n + p.tests.length, 0)} tests; evidence ${relative(root, output)}`);
  async function run(job, index) {
    const log = createWriteStream(join(output, `${index + 1}.jsonl`), { mode: 0o600 });
    const completed = new Map(), skipReasons = new Map(), began = Date.now();
    const pattern = `^(${job.tests.join('|')})$`;
    const args = ['test', '-json', '-p', '1', ...(job.race ? ['-race'] : []), '-count=1', `-timeout=${job.race ? '20m' : '4m'}`, '-run', pattern, job.name];
    console.log(`[go:test] ${index + 1}/${jobs.length} ${job.name} (${job.tests.length} tests${job.race ? ', race' : ', timed boundary'})`);
    const child = spawn('go', args, { cwd: job.cwd, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'pipe'] });
    active.add(child);
    const lines = createInterface({ input: child.stdout });
    lines.on('line', line => {
      log.write(line + '\n');
      try {
        const event = JSON.parse(line);
        if (event.Test && !event.Test.includes('/') && ['pass', 'skip', 'fail'].includes(event.Action)) completed.set(event.Test, event.Action);
        if (event.Test && event.Action === 'output') skipReasons.set(event.Test, (skipReasons.get(event.Test) ?? '') + event.Output);
      } catch { /* Non-JSON compiler diagnostics remain in the log. */ }
    });
    child.stderr.on('data', data => log.write(JSON.stringify({ Action: 'diagnostic', Output: String(data) }) + '\n'));
    let code, error;
    try { code = await new Promise((yes, no) => { child.once('error', no); child.once('close', value => yes(value ?? 1)); }); }
    catch (failure) { code = 1; error = failure.message; }
    finally { active.delete(child); lines.close(); await new Promise(resolve => log.end(resolve)); }
    const result = { index: index + 1, ...job, code, ...(error ? { error } : {}), seconds: (Date.now() - began) / 1000, ...checkGoResults(job, completed, code), tests: [...completed].map(([name, action]) => ({ name, action, ...(action === 'skip' ? { reason: skipReasons.get(name) } : {}) })) };
    console.log(`[go:test] ${index + 1}/${jobs.length} ${result.passed ? 'passed' : 'FAILED'} in ${result.seconds.toFixed(1)}s`);
    results.push(result);
  }
  try {
    const workers = await Promise.allSettled(Array.from({ length: concurrency }, async () => {
      while (next < jobs.length && !interrupted) { const index = next++; await run(jobs[index], index); }
    }));
    const failed = workers.find(worker => worker.status === 'rejected');
    if (failed) throw failed.reason;
  } finally {
    process.off('SIGINT', terminate); process.off('SIGTERM', terminate);
    const passed = !interrupted && results.length === jobs.length && results.every(r => r.passed);
    writeFileSync(join(output, 'result.json'), JSON.stringify({ passed, interrupted, seconds: (Date.now() - started) / 1000, expectedBatches: jobs.length, results: results.sort((a, b) => a.index - b.index) }, null, 2));
  }
  if (interrupted || results.length !== jobs.length || results.some(r => !r.passed)) throw Error(`Go regression did not pass; inspect ${join(output, 'result.json')}`);
}
