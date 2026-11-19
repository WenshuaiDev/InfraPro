import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { beforeEach, test } from 'node:test';
import { digest, read, write } from '../../scripts/architecture-lab.mjs';
import { assetDigests, consoleDigest } from '../../scripts/architecture-shared-console.mjs';
import { compareStaticBundles, servedDiagnosticsStyles } from '../../scripts/architecture-static.mjs';
import {
  finishVisualFrontend, readServedConsole, verifiedVisualFrontend, visualFrontend, visualOutputDirectory,
} from '../../scripts/architecture-visual-frontend.mjs';

beforeEach(t => {
  const prior = process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
  delete process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
  t.after(() => {
    if (prior === undefined) delete process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
    else process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE=prior;
  });
});

test('qualifies the same served console on both independent backends and preserves original evidence', async t => {
  const directory = mkdtempSync(join(tmpdir(), 'infrapro-console-evidence-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const files = { 'index.html': '<title>Shared</title>', 'assets/tool.js': 'window.arc=true;' };
  const manifest = Object.fromEntries(Object.entries(files).map(([name, bytes]) => [name, digest(bytes)]));
  const origins = {};
  for (const side of ['a', 'b', 'self']) {
    const server = createServer((req, res) => {
      res.end(files[req.url.slice(1)] || files['index.html']);
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    t.after(() => new Promise(resolve => server.close(resolve)));
    origins[side] = { origin: 'http://127.0.0.1:' + server.address().port };
  }
  write(join(directory, 'private/lab.json'), origins);
  const selection = {
    baselineReleaseSHA256: digest('S0'), backendReleaseSHA256: digest('B9'), deliveryReleaseSHA256: digest('B9'),
    console: { files: manifest, sha256: consoleDigest(manifest) }, sourceDiff: [],
  };
  const fixed = Object.fromEntries(Object.entries(manifest).map(([path, sha]) => ['console/' + path,sha]));
  write(join(directory, 'baseline.json'), { source_archive_sha256: selection.baselineReleaseSHA256, staticFiles: fixed });
  write(join(directory, 'candidate.json'), { inputs: { sourceSHA256: selection.backendReleaseSHA256 }, staticFiles: fixed });
  const original = join(directory, 'visual/paired/b/matrix.json');
  write(original, { original: true });
  write(join(directory, 'shared-console.json'), selection);
  const { output, frontend } = await visualFrontend(directory, 'b', directory);
  assert.equal(output, join(directory, 'visual/profiles', selection.console.sha256));
  assert.equal(visualOutputDirectory(join(output, 'visual/serialized'), selection), join(output, 'visual/serialized'));
  const report = { sourceDirectory: directory, frontend };
  assert.equal(verifiedVisualFrontend(selection, directory, report, 'b'), true);
  const end = await finishVisualFrontend(directory, 'b', output, frontend);
  assert.equal(end.identity, frontend.identity);
  assert.notEqual(end.evidence, frontend.evidence);
  files['assets/tool.js'] = 'changed after initial observations';
  await assert.rejects(finishVisualFrontend(directory, 'b', output, frontend), /Running console differs/);
  files['assets/tool.js'] = 'window.arc=true;';
  assert.equal(verifiedVisualFrontend(selection, directory, report, 'self'), false);
  assert.equal(verifiedVisualFrontend({ ...selection, deliveryReleaseSHA256: digest('different release') }, directory, report, 'b'), false);
  const { frontend: frozen } = await visualFrontend(directory, 'self', directory);
  const selfReport = { sourceDirectory: directory, frontend: frozen };
  const next = { ...selection, backendReleaseSHA256: digest('B10'), deliveryReleaseSHA256: digest('B10') };
  assert.equal(verifiedVisualFrontend(next, directory, selfReport, 'self'), true);
  assert.equal(verifiedVisualFrontend(next, directory, report, 'b'), false);
  assert.equal(verifiedVisualFrontend({ ...next, sourceDiff: [{ path: 'outside-the-approved-set' }] }, directory, selfReport, 'self'), false);
  const otherConsole = { ...manifest, 'assets/tool.js': digest('different served JavaScript') };
  assert.equal(verifiedVisualFrontend({ ...next, console: { files: otherConsole, sha256: consoleDigest(otherConsole) } }, directory, selfReport, 'self'), false);
  const replacement = join(directory, 'next.json');
  write(replacement, next);
  process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE = replacement;
  const { frontend: reused } = await visualFrontend(directory, 'self', directory);
  assert.equal(reused.identity, frozen.identity);
  assert.equal(reused.deliveryReleaseSHA256, selection.deliveryReleaseSHA256);
  await assert.rejects(visualFrontend(directory, 'b', directory));
  delete process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE;
  assert.deepEqual(read(original), { original: true });
  const observed = read(frontend.evidence);
  observed.pair.b.files['assets/tool.js'] = digest('fallback');
  writeFileSync(frontend.evidence, JSON.stringify(observed));
  assert.equal(verifiedVisualFrontend(selection, directory, report, 'b'), false);
  const fallback = await readServedConsole(origins.a.origin, { ...manifest, 'assets/missing.js': digest('expected javascript') });
  assert.equal(fallback.failures.length, 1);
  assert.equal(fallback.failures[0].status, 200);
  assert.equal(fallback.failures[0].actual, manifest['index.html']);
});

test('checks the independently served diagnostics HTML and CSS against their sealed bytes', async t => {
  const directory = mkdtempSync(join(tmpdir(), 'infrapro-diagnostics-styles-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const lab = {}, sheets = { a: 'main{display:block}', b: 'main{display:block}.paused{animation-play-state:paused}' };
  for (const side of ['a', 'b']) {
    const file = '/assets/index-' + side + '.css';
    const html = '<link rel="stylesheet" href="' + file + '">';
    const server = createServer((request, response) => response.end(request.url === file ? sheets[side] : html));
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    t.after(() => new Promise(resolve => server.close(resolve)));
    lab[side] = { ports: { diagnostics: server.address().port } };
    write(join(directory, side === 'a' ? 'baseline.json' : 'candidate.json'), {
      staticFiles: { 'diagnostics/index.html': digest(html), ['diagnostics' + file]: digest(sheets[side]) },
    });
  }
  write(join(directory, 'private/lab.json'), lab);
  const actual = await servedDiagnosticsStyles(directory);
  assert.equal(actual.b.sha256, digest(sheets.b));
  sheets.b = sheets.a;
  await assert.rejects(servedDiagnosticsStyles(directory), { code: 'ERR_ASSERTION' });
});

test('classifies rebuilt console chunks only with the sealed approved source diff', t => {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-shared-build-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  for (const variant of ['old', 'new']) {
    const files = {
      'console/index.html': '<script src="/assets/' + variant + '.js"></script>',
      ['console/assets/' + variant + '.js']: 'document.title=' + JSON.stringify(variant),
      'console/vendor/fixed': 'fixed vendor bytes',
      'diagnostics/index.html': '<script src="/assets/index-abc.js"></script>',
      'diagnostics/assets/index-abc.js': 'document.title="diagnostics"',
      'diagnostics/.vite/manifest.json': JSON.stringify({ 'index.html': { file: 'assets/index-abc.js' } }),
    };
    for (const [name, bytes] of Object.entries(files)) {
      const path = join(dir, variant, name);
      mkdirSync(dirname(path), { recursive: true });
      writeFileSync(path, bytes);
    }
  }
  const path = 'apps/console/src/features/projects/governance-page.tsx';
  const frozen = {
    assets: join(dir, 'old'), commit: 'frozen', source_archive_sha256: digest('baseline'),
    sourceFiles: { [path]: digest('old source') }, staticFiles: assetDigests(join(dir, 'old')),
  };
  const release = { release: join(dir, 'new'), inputs: { sourceSHA256: digest('new source root') }, staticFiles: assetDigests(join(dir, 'new')) };
  const files = assetDigests(join(dir, 'new/console')), change = { path, before: frozen.sourceFiles[path], after: digest('new source') };
  const selection = {
    baselineReleaseSHA256: frozen.source_archive_sha256, backendReleaseSHA256: release.inputs.sourceSHA256,
    deliveryReleaseSHA256: release.inputs.sourceSHA256, approvedSources: [path], sourceDiff: [change],
    console: { directory: join(dir, 'new/console'), files, sha256: consoleDigest(files) },
    sourceManifest: join(dir, 'application-source.json'),
  };
  write(selection.sourceManifest, { application_source_sha256: release.inputs.sourceSHA256, files: { [path]: change.after } });
  write(join(dir, 'baseline.json'), frozen);
  write(join(dir, 'shared-console.json'), selection);
  const result = compareStaticBundles(dir, release);
  assert.equal(result.passed, true);
  assert.equal(result.files.length, 3);
  assert(result.files.every(file => file.classification === 'approved-common-console-build'));
  const other = 'apps/console/src/features/projects/unapproved.tsx';
  write(selection.sourceManifest, { application_source_sha256: release.inputs.sourceSHA256, files: { [path]: change.after, [other]: digest('changed') } });
  assert.throws(() => compareStaticBundles(dir, release));
});
