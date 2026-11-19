import { existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { read, runDirectory, safeDiagnosticLine, write } from './architecture-lab.mjs';
import { currentRoleSQL } from './architecture-parity-evidence.mjs';
import { visualRecordingDrivers } from './architecture-visual-recording.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { verifiedVisualFrontend, visualOutputDirectory } from './architecture-visual-frontend.mjs';

function httpFailures(requests = []) {
  const failures = [];
  for (const request of requests) {
    const match = /^(\w+) (\/\S+) (\d{3})$/.exec(request);
    if (!match || Number(match[3]) < 400) continue;
    const path = match[2]
      .split('?')[0]
      .replaceAll(/[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}/gi, '{id}');
    failures.push({ method: match[1], path, status: Number(match[3]) });
  }
  return failures;
}

function frozenDisplayReadback(dir, matrix) {
  const attempts = (matrix.rows ?? []).filter((row) => row.state === 'published-instance');
  if (!attempts.length) return;
  const source = matrix.sourceDirectory;
  const descriptor = read(join(dir, 'visual/frozen-fixture-self.json'));
  if (descriptor.directory !== source) return;
  const grants = Object.fromEntries(
    ['a', 'self'].map((side) => {
      const file = join(source, 'private', side, 'visual-assets.json');
      return [side, existsSync(file) ? read(file).display?.grant : null];
    }),
  );
  const rows = attempts
    .filter((row) => row.passed)
    .map((row) => {
      const folder = join('visual', 'feature-states', 'self', row.key);
      const sides = [
        ['a', 'old-live', row.baselineRequests],
        ['self', 'live', row.liveRequests],
      ];
      const checks = sides.map(([side, name, requests]) => {
        const path = join(folder, name + '.json');
        const content = existsSync(join(dir, path)) ? read(join(dir, path)).aria : '';
        const grant = grants[side];
        return {
          side,
          path,
          detail200: requests.some((req) => /^GET .*\/displays\/instances\/[^/ ]+ 200$/.test(req)),
          rendered: content.includes('验收大屏 · 桥梁工程概览') && /1 个资源/.test(content),
          sameScope:
            grant?.resources === 1 &&
            ['channels', 'spatial', 'artifacts'].every((k) => grant[k] === 0),
        };
      });
      return {
        viewport: row.viewport,
        checks,
        passed: checks.every((c) => c.detail200 && c.rendered && c.sameScope),
      };
    });
  write(join(dir, 'visual/frozen-display-detail-recheck.json'), {
    at: new Date().toISOString(),
    sourceDirectory: source,
    snapshot: matrix.snapshot,
    grants,
    rows,
    passed: rows.length === 3 && rows.every((r) => r.passed),
  });
}

export function visualTriage(run) {
  const selection = sharedConsoleFor(run);
  const dir = visualOutputDirectory(run, selection);
  const stats = [];
  const saved = (name) => (existsSync(join(dir, name)) ? read(join(dir, name)) : null);
  const release = ['candidate.evaluated.json', 'candidate.json'].map(name => join(run, name)).find(existsSync);
  const selected = release && read(release).inputs?.sourceSHA256;
  const grants = currentRoleSQL();
  for (const family of ['paired', 'feature-states', 'published-screens']) {
    for (const side of ['self', 'b']) {
      const relative = join('visual', family, side);
      const path = join(dir, relative, 'matrix.json');
      if (!existsSync(path)) {
        stats.push({ family, side, status: 'pending' });
        continue;
      }
      const matrix = read(path),
        rows = matrix.rows ?? [];
      if (
        family === 'feature-states' &&
        side === 'self' &&
        saved('visual/frozen-fixture-self.json')
      )
        frozenDisplayReadback(dir, matrix);
      const fixtures = family !== 'paired';
      const source = saved(
        'visual/' +
          (side === 'self'
            ? fixtures
              ? 'frozen-fixture-self'
              : 'frozen-self'
            : fixtures
              ? 'fixture-environment'
              : 'environment') +
          '.json',
      );
      const recording = visualRecordingDrivers();
      const currentRecording = family === 'published-screens' || matrix.requestOrder === 'natural' &&
        Object.entries(recording).every(([name, hash]) => matrix[name] === hash);
      const sameInput = Boolean(
        source &&
        verifiedVisualFrontend(selection,run,matrix,side) &&
        currentRecording &&
        matrix.sourceDirectory === source.directory &&
        (side === 'self'
          ? matrix.baseline === source.inputs?.baseline &&
            matrix.snapshot === source.inputs?.snapshot
          : matrix.candidate === selected &&
            source.releaseSHA256 === selected &&
            isDeepStrictEqual(matrix.roleSQL, grants)),
      );
      const failures = rows
        .filter((row) => row.passed !== true)
        .map((row) => {
          const facts = {
            key: row.key,
            app: row.app,
            template: row.template,
            persona: row.persona,
            state: row.state,
            viewport: row.viewport,
            artifact: join(relative, row.key ?? ''),
          };
          if (family === 'published-screens') {
            facts.subject = row.subject;
            facts.artifact = relative;
            facts.screen = {
              changedPixels: row.changedPixels,
              liveChangedPixels: row.liveChangedPixels,
              sameAccessibility: row.sameAccessibility,
              sameLayout: row.sameLayout,
              sameData: row.sameData,
              sameURL: row.sameURL,
            };
          }
          if (row.error) facts.error = safeDiagnosticLine(row.error).slice(0, 500);
          if (row.fixed) facts.fixed = row.fixed;
          if (row.live) facts.live = row.live;
          if (row.liveAuthorized !== undefined) facts.liveAuthorized = row.liveAuthorized;
          if (row.requestsMatch !== undefined) facts.requestsMatch = row.requestsMatch;
          if (row.browserErrors?.length) facts.browserErrors = row.browserErrors;
          if (row.unusedInputs) facts.unusedInputs = row.unusedInputs;
          facts.baselineHTTP = httpFailures(row.baselineRequests);
          facts.peerHTTP = httpFailures(row.liveRequests);
          facts.transport = (row.transportErrors ?? []).map(({ method, error }) => ({
            method,
            error,
          }));
          return facts;
        });
      stats.push({
        family,
        side,
        identity: matrix.identity,
        baseline: matrix.baseline,
        candidate: matrix.candidate,
        actualGrants: matrix.roleSQL,
        snapshot: matrix.snapshot,
        sourceDirectory: matrix.sourceDirectory,
        expected: matrix.expectedRows,
        recorded: rows.length,
        passed: rows.filter((row) => row.passed === true).length,
        currentRecording,
        currentInput: sameInput,
        complete: rows.length === matrix.expectedRows,
        status: !sameInput
          ? 'previous_inputs'
          : matrix.passed === true
            ? 'passed'
            : rows.length === matrix.expectedRows
              ? 'failed'
              : 'in_progress',
        failures,
      });
    }
  }
  const supplemental = ['self', 'b'].map((side) => {
    const result = join('visual', 'read-order', 'native', side + '.json');
    const attempt = saved(result);
    if (!attempt) return { side, result, status: 'pending' };
    const matrix = read(attempt.matrix);
    const rows = matrix.rows.map((row) => ({
      state: row.state, viewport: row.viewport, passed: row.passed,
      original: row.originalKey, recorded: row.recording?.requests ?? null,
      liveAuthorized: row.liveAuthorized ?? null, identicalValues: row.readsMatch ?? false,
      sameInputPixels: row.fixed?.changedPixels ?? null,
      originalHTTP: httpFailures(row.baselineRequests), peerHTTP: httpFailures(row.liveRequests),
    }));
    const native = attempt.native && existsSync(attempt.native.path) ? read(attempt.native.path) : undefined;
    return {
      side, result, natural: attempt.naturalMatrix, naturalSHA256: attempt.naturalMatrixSHA256,
      ...(native && { nativeBrowser: { evidence: attempt.native.path, observed: native.rows.length, passed: native.passed,
        failed: native.rows.filter(row=>!row.passed).map(row=>({ key:row.key, statuses:Object.fromEntries(
          Object.entries(row.pair).map(([actor,facts])=>[actor,facts.reads.map(r=>r.status)])) })) } }),
      httpInflight: attempt.httpInflight, pgSchedule: attempt.pgSchedule,
      candidate: attempt.candidate, source: attempt.source, rows,
      status: attempt.passed ? 'passed' : rows.length === attempt.required ? 'failed' : 'pending',
    };
  });
  const report = { at: new Date().toISOString(), consoleSHA256: selection?.console.sha256, matrices: stats, supplemental };
  write(join(dir, 'visual/comparison-progress.json'), report);
  return report;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url))
  visualTriage(runDirectory(process.argv[2]));
