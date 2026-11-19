import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { existsSync } from 'node:fs';
import { join, relative } from 'node:path';
import { digest, read, write } from './architecture-lab.mjs';

export function openDisplayCampaign(directory, signature, scope, provenance, resume) {
  const idFile = join(directory, 'campaign-pointers', digest(JSON.stringify([signature, scope])) + '.json');
  const metadata = resume && existsSync(idFile) ? read(idFile) : {
    campaign: randomUUID(), signature, scope, provenance, created: new Date().toISOString(),
  };
  assert.match(metadata.campaign, /^[a-f0-9-]{36}$/);
  assert.equal(metadata.signature, signature);
  assert.deepEqual(metadata.scope, scope);
  assert.deepEqual(metadata.provenance, provenance);
  const folder = join(directory, 'campaigns', metadata.campaign);
  write(idFile, metadata);
  if (!existsSync(join(folder,'manifest.json'))) write(join(folder, 'manifest.json'), metadata);
  const paths = (trial) => {
    const key = [trial.side, trial.round, trial.scene].join('-');
    assert.match(key, /^[ab]-[1-3]-(idle|history|mixed)$/);
    return { key, started: join(folder, key + '.started.json'), result: join(folder, key + '.json') };
  };
  const store = (trial, row) => {
    const p = paths(trial);
    const result = { ...row, ...trial, campaign: metadata.campaign, signature, provenance };
    // A preparation failure has no timed sample. Keep its complete record,
    // then permit preparation to resume. Every timed outcome is immutable.
    const file = existsSync(p.started) ? p.result : join(folder, 'preparation', p.key + '-' + randomUUID() + '.json');
    result.receipt = relative(folder, file);
    assert(!existsSync(file), 'The measured result of a display scenario is already sealed');
    write(file, result);
    return result;
  };
  function previous(trial) {
    const p = paths(trial);
    if (existsSync(p.result)) return read(p.result);
    if (!existsSync(p.started)) return undefined;
    const stamp = read(p.started);
    assert.equal(stamp.campaign,metadata.campaign);
    assert.match(stamp.capture,/^[a-f0-9-]{36}\.json$/);
    const capture = join(folder, 'captures', stamp.capture);
    if (existsSync(capture)) return store(trial, { ...stamp.cohort, ...read(capture) });
    return store(trial, {
      ...stamp.cohort,
      at: stamp.at, measurementStart: stamp.at, finishedAt: new Date().toISOString(),
      passed: false, metrics: {}, samples: {}, execution: [], resources: [], history: [], historyOverlap: [],
      failures: [['timed-interruption', 'The process stopped after measured requests began and before a complete result was recorded']],
    });
  }
  function prepare(trial) {
    const p = paths(trial);
    assert(!existsSync(p.started) && !existsSync(p.result));
    const name = randomUUID() + '.json';
    return {
      file: join(folder, 'captures', name),
      begin(at,cohort={}) {
        assert(!existsSync(p.started));
        write(p.started, { campaign: metadata.campaign, ...trial, at, capture: name,cohort });
      },
    };
  }
  return { ...metadata, folder, previous, prepare, store };
}
