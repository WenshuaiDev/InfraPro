import { existsSync, readFileSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, root, runDirectory, safeDiagnosticLine, write } from './architecture-lab.mjs';
import { readChecks } from './architecture-regression.mjs';

function redact(text) {
  return safeDiagnosticLine(text)
    .replaceAll(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi, '[synthetic ID]')
    .slice(0, 320);
}
function goBatches(log, cwd) {
  const match = log.match(/\[go:test\] [^\n]*evidence (\.artifacts\/go-tests\/[\w.-]+)/);
  if (!match) return [];
  const folder = join(cwd, match[1]),
    result = join(folder, 'result.json');
  if (!existsSync(result)) return [];
  return read(result)
    .results.filter((r) => !r.passed)
    .map((batch) => {
      const file = join(folder, batch.index + '.jsonl'),
        lines = readFileSync(file, 'utf8')
          .trim()
          .split('\n')
          .map((line) => JSON.parse(line));
      const cases = batch.failed.map((name) => {
        const output = lines
          .filter((v) => v.Test === name && v.Action === 'output')
          .map((v) => v.Output.trim());
        const causes = output.filter((v) =>
          /\.go:\d+:|SQLSTATE |connection refused|failed to connect|did not |changed legacy|timed out|object_write_failed/i.test(
            v,
          ),
        );
        return { name, reason: [...new Set(causes.map(redact))].slice(0, 3) };
      });
      return {
        index: batch.index,
        package: batch.name,
        source: relative(root, file),
        exit: batch.code,
        cases,
        missing: batch.missing,
      };
    });
}
function sourceEvidence(name, text) {
  const lines = text.split('\n');
  if (name === 'test:e2e:preview')
    return lines
      .filter((s) => /^\s*(?:\d+ (?:failed|skipped|passed)|[0-9]+\) \[.*?\] › )/.test(s))
      .map(redact);
  const candidates = lines.filter((s) =>
    /^--- FAIL: |\.go:\d+: |(?:ts|tsx)\(\d+,\d+\): error |strict mode violation|Docker daemon|docker API|Cannot connect|failed to solve|^Error: |before upgrade|SQLSTATE\s\w{5}/.test(
      s,
    ),
  );
  return [...new Set(candidates.map(redact))].slice(0, 18);
}
export function regressionTriage(dir) {
  const report = { at: new Date().toISOString(), sides: {} };
  for (const side of ['baseline', 'candidate']) {
    report.sides[side] = readChecks(dir, side).map((v) => {
      const name = v.command[1],
        record = {
          command: name,
          status: v.status,
          durationMs: v.durationMs ?? null,
          exit: v.exitCode ?? null,
        };
      if (v.status === 'passed' || !v.log) return record;
      const file = join(dir, 'regression', side, v.log);
      if (existsSync(file)) {
        const text = readFileSync(file, 'utf8');
        Object.assign(record, { log: relative(dir, file), observed: sourceEvidence(name, text) });
        if (name === 'test:identity') record.goBatches = goBatches(text, v.cwd);
      }
      return record;
    });
  }
  write(join(dir, 'regression/triage.json'), report);
  return report;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url))
  regressionTriage(runDirectory(process.argv[2]));
