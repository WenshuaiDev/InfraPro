import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { digest, read, write } from '../../scripts/architecture-lab.mjs';
import { importFocusDelta, importFocusSources, selectSharedConsole, sharedConsoleFor } from '../../scripts/architecture-shared-console.mjs';

test('the second common console preserves the complete published first source change and isolates import focus', () => {
  const first=JSON.parse(readFileSync(new URL('../../scripts/fixtures/architecture-console-before-import-focus.json',import.meta.url),'utf8'));
  const frozen={source_archive_sha256:first.frozenReleaseSHA256};
  const focus=[...importFocusSources].map(path=>({path,before:null,after:digest(path)}));
  const changes=[...first.sourceChanges, ...focus];
  assert.deepEqual(importFocusDelta(frozen,changes),focus);
  assert.equal(importFocusDelta(frozen,first.sourceChanges),null);
  assert.throws(()=>importFocusDelta(frozen,[...first.sourceChanges.slice(1),...focus]));
  assert.throws(()=>importFocusDelta(frozen,[{...first.sourceChanges[0],after:digest('other edit')},...changes.slice(1)]));
  assert.throws(()=>importFocusDelta(frozen,[...changes,{path:'apps/console/src/unapproved.tsx',before:null,after:digest('extra')}]));
  assert.throws(()=>importFocusDelta({source_archive_sha256:digest('another baseline')},changes));
});

test('both backends select verified identical console bytes and retain the old selection', t => {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-console-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const delivery = join(dir, 'delivery');
  const assets = join(delivery, 'web', 'console');
  mkdirSync(assets, { recursive: true });
  const page = 'apps/console/src/features/projects/governance-page.tsx';
  const frozen = { source_archive_sha256: digest('old release'), sourceFiles: { [page]: digest('original layout') } };
  write(join(dir, 'baseline.json'), frozen);
  const source = { application_source_sha256: digest('release'), files: { [page]: digest('repaired layout') } };
  const release = { inputs: { sourceSHA256: source.application_source_sha256 }, release: join(delivery, 'web'), staticFiles: {} };
  for (const [name, body] of Object.entries({ 'index.html': '<script src="/app.js"></script>', 'app.js': 'document.title="InfraPro"' })) {
    release.staticFiles['console/' + name] = digest(body);
    writeFileSync(join(assets, name), body);
  }
  write(join(delivery, 'application-source.json'), source);
  const selected = selectSharedConsole(dir, delivery, release);
  assert.deepEqual(selected.approvedSources, [page]);
  assert.deepEqual(sharedConsoleFor(dir, { verifyFiles: true }), selected);
  writeFileSync(join(assets, 'app.js'), 'altered');
  assert.throws(() => sharedConsoleFor(dir, { verifyFiles: true }), /no longer matches/);
  source.files['apps/console/src/other.ts'] = digest('unrelated change');
  write(join(delivery, 'application-source.json'), source);
  assert.throws(() => selectSharedConsole(dir, delivery, release), /Unexpected shared console/);
  write(join(delivery, 'application-source.json'), { ...source, files: frozen.sourceFiles });
  assert.equal(selectSharedConsole(dir, delivery, release), null);
  assert(!existsSync(join(dir, 'shared-console.json')));
  assert.deepEqual(read(join(dir, 'history/shared-console', selected.console.sha256 + '.json')), selected);
});
