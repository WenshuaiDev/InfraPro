import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { digest } from './architecture-lab.mjs';

const fixture = 'packages/go/platform/resources/derived_clock_horizon_test.go';
const caseName = 'TestDerivedPrimaryClockAdvancesPublishedPrefixBeforeUncomputedTail';

function replaceOnce(source, before, after) {
  assert.equal(source.split(before).length, 2, 'Clock setup fixture changed: ' + before.trim());
  return source.replace(before, after);
}

// This test enables two channels with a completed trial receipt before testing
// prefix publication. Keep its existing assertions and execute an accepted
// background job through the same service the original fixture already uses.
export function installClockTrialSetup(cwd, overlayDirectory, replacements, forceFirstHandoff = false) {
  const file = join(cwd, fixture);
  const original = readFileSync(file, 'utf8');
  const existing = replacements[file] ? readFileSync(replacements[file], 'utf8') : original;
  const helper = readFileSync(
    new URL('./fixtures/architecture-clock-trial.go.inc', import.meta.url), 'utf8',
  );
  const launch = '\t\ttrial, e := s.ChannelTrial(';
  const assertion = '\t\tif trial.Trial == nil {\n\t\t\tt.Fatal("expected completed bounded trial", trial.Job)';
  let patched = replaceOnce(existing, 'import (\n', 'import (\n\t"context"\n\t"reflect"\n');
  patched = replaceOnce(patched, launch, '\t\tstarted := time.Now()\n' + launch);
  patched = replaceOnce(patched, assertion,
    '\t\ttrial = completeArchitectureClockTrial(t, f, s, scope, channel, cfg, from, trialTo, trial, time.Since(started))\n' + assertion,
  );
  if (forceFirstHandoff) {
    const first = '\tenable(parent, parentConfig)';
    patched = replaceOnce(patched, first,
      '\tobserver := &architectureClockDeadline{deadlineTrialExecutor: &deadlineTrialExecutor{Service: s, first: true}, t: t}\n\t_, setupErr := f.service.WithTaskExecutors(map[string]md.TaskExecutor{"derive": observer})\n\tmust(t, setupErr)\n' + first,
    );
    patched += '\n' + readFileSync(
      new URL('./fixtures/architecture-clock-deadline.go.inc', import.meta.url), 'utf8',
    );
  }
  patched += '\n' + helper;
  const sourceSHA256 = digest(original);
  const overlaySHA256 = digest(patched);
  const target = join(overlayDirectory, 'clock-setup-' + overlaySHA256 + '.go');
  writeFileSync(target, patched, { mode: 0o600 });
  replacements[file] = target;
  return { fixture, case: caseName, sourceSHA256, overlaySHA256, setupHelperSHA256: digest(helper), forceFirstHandoff };
}
