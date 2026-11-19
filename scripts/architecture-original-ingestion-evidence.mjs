import { createHash } from 'node:crypto';
import { existsSync, lstatSync, readFileSync, realpathSync, statSync } from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';

const test = 'TestIngestionSustainedLoad';
const duration = 180;
const frames = 18000;
const receipts = 13501;

class EvidenceError extends Error {}

function requireEvidence(condition, message) {
  if (!condition) throw new EvidenceError(message);
}

function fileEvidence(base, path, evidence) {
  requireEvidence(lstatSync(path).isFile(), 'An original session evidence entry is not a regular file');
  const bytes = readFileSync(path);
  evidence.push({
    path: relative(base, path),
    bytes: bytes.length,
    sha256: createHash('sha256').update(bytes).digest('hex'),
  });
  return bytes.toString('utf8');
}

// Source references point at the unique ordinary test session, so a recheck
// of a saved record is independent of the original runner's mutable latest link.
export function archiveOriginalIngestionLoad(directory, cwd, name, result) {
  if (name !== 'test:ingestion:load') return;
  const saved = result.ingestionLoadEvidence;
  const savedSession = saved?.session;
  const evidence = { complete: false, test, files: [] };
  result.ingestionLoadEvidence = evidence;
  try {
    const runner = readFileSync(join(directory, result.log));
    evidence.runnerLogSHA256 = createHash('sha256').update(runner).digest('hex');
    const base = resolve(cwd, '.artifacts/ingestion');
    const session = savedSession
      ? resolve(cwd, savedSession)
      : resolve(JSON.parse(readFileSync(join(base, 'latest-tests.json'), 'utf8')).session);
    requireEvidence(
      dirname(session) === base && /^test-[A-Za-z0-9]{6}$/.test(basename(session)) && existsSync(session) &&
        realpathSync(session) === join(realpathSync(base), basename(session)),
      'The original load session does not belong to this checkout',
    );
    evidence.session = relative(cwd, session);
    const original = JSON.parse(fileEvidence(cwd, join(session, 'result.json'), evidence.files));
    const log = fileEvidence(cwd, join(session, 'tests.log'), evidence.files);
    const raw = fileEvidence(cwd, join(session, 'load.jsonl'), evidence.files);
    const archivedTranscript = saved?.complete || saved?.verifiedTranscript;
    if (archivedTranscript)
      requireEvidence(
        saved.runnerLogSHA256 === evidence.runnerLogSHA256 &&
          saved.files.every((f) => evidence.files.some((g) => g.path === f.path && g.sha256 === f.sha256)),
        'The archived source transcript changed after the original command',
      );
    const startedAt = Date.parse(result.startedAt), finishedAt = Date.parse(result.finishedAt);
    const sessionEnd = statSync(join(session, 'result.json')).mtimeMs;
    requireEvidence(
      original.synthetic === true && resolve(original.session) === session &&
        original.exit === result.exitCode && (archivedTranscript || (sessionEnd >= startedAt - 2 && sessionEnd <= finishedAt + 2)),
      'The original session does not match this command or its execution window',
    );
    const rows = raw.trim().split('\n').map((line) => JSON.parse(line));
    const required = ['started', 'inner_channel_stopped', 'inner_channel_restored', 'sending_complete', 'latencies', 'reconciled'];
    requireEvidence(
      rows.filter((r) => required.includes(r.event)).map((r) => r.event).join('\n') === required.join('\n'),
      'The load transcript is missing its ordered start, stop, restore, send or reconciliation evidence',
    );
    const [start, stop, restore, sent, latency, end] = required.map((event) => rows.find((r) => r.event === event));
    requireEvidence(
      start.synthetic === true && start.real_parser === true && /^identity_test_[a-z0-9]+$/.test(start.schema) &&
        Date.parse(start.started_at) >= startedAt - 2 && Date.parse(start.started_at) <= finishedAt + 2 &&
        start.duration_seconds === duration && start.target_measurement_frames_per_second === 100 &&
        start.tenants === 2 && start.projects === 2 && start.transport === 'mTLS DDP + HTTPS batch',
      'The original fixture, duration, transport, tenants or start time differs',
    );
    requireEvidence(
      Number.isFinite(stop.seconds) && stop.seconds >= 120 && Number.isFinite(restore.seconds) &&
        restore.seconds >= 165 && restore.seconds > stop.seconds && sent.seconds >= restore.seconds,
      'The original worker did not run both scheduled stop and restore branches',
    );
    requireEvidence(
      sent.measurement_frames === frames && sent.receipts === receipts && Number.isFinite(sent.seconds) && sent.seconds > 0 &&
        Number.isFinite(sent.frames_per_second) && Math.abs(sent.frames_per_second - frames / sent.seconds) < 1e-6,
      'The original input frame and receipt totals or rate are inconsistent',
    );
    const values = latency.values;
    requireEvidence(
      ['takeover_p95_seconds', 'end_to_end_p95_seconds', 'total_seconds'].every((key) => Number.isFinite(values?.[key]) && values[key] >= 0) &&
        values.total_seconds >= sent.seconds && Date.parse(start.started_at) + values.total_seconds * 1000 <= finishedAt + 2000,
      'The original takeover, parsing or drain measurements are incomplete',
    );
    const outcome = original.exit === 0 ? 'PASS' : 'FAIL';
    const verdict = new RegExp(`^--- ${outcome}: TestIngestionSustainedLoad \\(`, 'm');
    requireEvidence(verdict.test(log) && verdict.test(runner.toString('utf8')),
      'The original Go verdict is absent from one of the command transcripts');
    const inputWithinOriginalTiming = sent.seconds <= duration * 1.005;
    const reconciled = end.results === frames && end.expected_results === frames &&
      end.receipts === receipts && end.expected_receipts === receipts &&
      ['pending_ack', 'failed', 'duplicate_default_plans', 'cross_tenant_units'].every((key) => end[key] === 0);
    Object.assign(evidence, {
      verifiedTranscript: true,
      originalGoOutcome: outcome,
      inputWithinOriginalTiming,
      reconciled,
      schema: start.schema,
      startedAt: start.started_at,
      inputSeconds: start.duration_seconds,
      inputFrames: sent.measurement_frames,
      confirmedReceipts: end.receipts,
      completedResults: end.results,
      inputFPS: sent.frames_per_second,
      sendingSeconds: sent.seconds,
      stopScheduledSeconds: 120,
      restoreScheduledSeconds: 165,
      stoppedAtSeconds: stop.seconds,
      restoredAtSeconds: restore.seconds,
      observedStoppedSeconds: restore.seconds - stop.seconds,
      takeoverP95Seconds: values.takeover_p95_seconds,
      endToEndP95Seconds: values.end_to_end_p95_seconds,
      totalSeconds: values.total_seconds,
      pendingAcks: end.pending_ack,
      failed: end.failed,
      duplicatePlans: end.duplicate_default_plans,
      crossTenantUnits: end.cross_tenant_units,
    });
    requireEvidence(reconciled, 'The original 18,000 results and 13,501 receipts did not reconcile');
    requireEvidence(inputWithinOriginalTiming, 'The original input exceeded its 180.9-second limit');
    requireEvidence(original.exit === 0, 'The original Go assertion did not pass');
    evidence.complete = true;
  } catch (error) {
    evidence.reason = error instanceof EvidenceError ? error.message : 'An original load evidence file could not be read or parsed';
    if (result.status === 'passed') {
      result.status = 'failed';
      result.failure = evidence.reason;
    }
  }
}
