import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { digest, read, root, write } from './architecture-lab.mjs';
import { approvedConsoleSources, assetDigests, consoleDigest, sharedConsoleFor } from './architecture-shared-console.mjs';
import { consoleFiles, visualOutputDirectory } from './architecture-visual-frontend.mjs';

function authorizedConsoleChanges(dir, base, release) {
  const selection = sharedConsoleFor(dir);
  if (!selection) return null;
  assert.equal(selection.baselineReleaseSHA256, base.source_archive_sha256);
  assert.equal(selection.backendReleaseSHA256, release.inputs.sourceSHA256);
  assert.equal(selection.deliveryReleaseSHA256, release.inputs.sourceSHA256);
  const source = read(selection.sourceManifest);
  assert.equal(source.application_source_sha256, selection.deliveryReleaseSHA256);
  const names = [...new Set([...Object.keys(base.sourceFiles), ...Object.keys(source.files)])]
    .filter(name => name.startsWith('apps/console/') || name.startsWith('packages/ts/ui/')).sort();
  const differences = names.filter(name => base.sourceFiles[name] !== source.files[name])
    .map(path => ({ path, before: base.sourceFiles[path] ?? null, after: source.files[path] ?? null }));
  assert(differences.length > 0);
  assert.deepEqual(differences, selection.sourceDiff);
  assert.deepEqual(differences.map(change => change.path), selection.approvedSources);
  assert(differences.every(change => approvedConsoleSources.has(change.path)));
  assert.deepEqual(assetDigests(join(base.assets, 'console')), consoleFiles(base.staticFiles));
  assert.deepEqual(assetDigests(join(release.release, 'console')), consoleFiles(release.staticFiles));
  assert.deepEqual(assetDigests(selection.console.directory), consoleFiles(release.staticFiles));
  assert.deepEqual(selection.console.files, consoleFiles(release.staticFiles));
  assert.equal(consoleDigest(selection.console.files), selection.console.sha256);
  return selection;
}

// The independent diagnostics server embeds its own UI. Verify the actual
// HTTP bytes as well as the release directories before reviewing its layout.
export async function servedDiagnosticsStyles(dir) {
  const lab = read(join(dir, 'private/lab.json'));
  const releases = {
    a: read(join(dir, 'baseline.json')),
    b: read(join(dir, 'candidate.json')),
  };
  const hosts = {};
  for (const [side, release] of Object.entries(releases)) {
    const origin = new URL('http://127.0.0.1:' + lab[side].ports.diagnostics);
    const options = { redirect: 'error', signal: AbortSignal.timeout(10000) };
    const index = await fetch(new URL('/login', origin), options);
    assert.equal(index.status, 200);
    const html = Buffer.from(await index.arrayBuffer());
    const indexSHA256 = digest(html);
    assert.equal(indexSHA256, release.staticFiles['diagnostics/index.html']);
    const styles = [...html.toString().matchAll(/<link\b[^>]*\bhref="(\/assets\/[\w-]+\.css)"/g)];
    assert.equal(styles.length, 1);
    const file = styles[0][1].slice(1);
    const response = await fetch(new URL('/' + file, origin), options);
    assert.equal(response.status, 200);
    const css = Buffer.from(await response.arrayBuffer());
    const sha256 = digest(css);
    assert.equal(sha256, release.staticFiles['diagnostics/' + file]);
    hosts[side] = { indexSHA256, file, sha256, bytes: css.length };
  }
  return hosts;
}

// Compare the Vite entry references separately from the HTML, manifest and CSS.
export function compareStaticBundles(dir, release) {
  const base = read(join(dir, 'baseline.json'));
  const selection = authorizedConsoleChanges(dir, base, release);
  const output = visualOutputDirectory(dir, selection);
  const apps = ['console', 'display-runtime', 'diagnostics'];
  const before = base.staticFiles;
  const after = release.staticFiles;
  const paths = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
  const changes = paths.filter((path) => before[path] !== after[path]);
  let previousConsole;
  if(selection?.beforeImportFocus){
    const sealed=selection.beforeImportFocus.console,selected=selection.console;
    const names=[...new Set([...Object.keys(sealed.files),...Object.keys(selected.files)])].sort();
    const changed=names.filter(name=>sealed.files[name]!==selected.files[name]);
    assert(changed.every(name=>name==='index.html' || /^assets\/[^/]+\.(?:js|css)$/.test(name)),
      'Unexpected frontend asset difference between the first and second common console');
    previousConsole={unchanged:names.length-changed.length,baselineFiles:Object.keys(sealed.files).length,
      candidateFiles:Object.keys(selected.files).length,changed:changed.map(path=>({path,
        before:sealed.files[path]??null,after:selected.files[path]??null}))};
  }
  const entries = [base.assets, release.release].map((assets) => {
    const path = join(assets, 'diagnostics');
    const manifest = readFileSync(join(path, '.vite', 'manifest.json'), 'utf8');
    const html = readFileSync(join(path, 'index.html'), 'utf8');
    const entry = JSON.parse(manifest)['index.html'];
    const file = entry.file;
    const [css] = entry.css || [];
    assert.match(file, /^assets\/index-[a-z\d_-]+\.js$/i);
    assert.ok(html.includes('/' + file));
    if (css) {
      assert.equal(entry.css.length,1);
      assert.match(css,/^assets\/index-[a-z\d_-]+\.css$/i);
      assert(html.includes('/'+css));
    }
    return { file, html, manifest, css, cssContent:css ? readFileSync(join(path,css),'utf8') : '' };
  });
  const [old, next] = entries;
  const sharedPaging = selection?.approvedSources.includes('packages/ts/ui/src/hooks/use-paged-query.tsx');
  const utility = '.paused{animation-play-state:paused}';
  const sameStyle = old.cssContent === next.cssContent;
  const addedUtility = sharedPaging && old.css && next.css && !old.cssContent.includes(utility) &&
    next.cssContent.includes(utility) && next.cssContent.replace(utility,'')===old.cssContent;
  const pausedSources = [];
  if (addedUtility) {
    const sources = read(selection.sourceManifest).files;
    for (const [path, hash] of Object.entries(sources)) {
      if (!/^(apps\/diagnostics|packages\/ts\/(ui|layouts))\/src\/.*\.(?:tsx?|jsx?|css)$/.test(path)) continue;
      const bytes = readFileSync(join(root,path));
      assert.equal(digest(bytes),hash,'Unexpected diagnostic source drift: '+path);
      const matches = bytes.toString('utf8').split('\n').flatMap((line,index)=>/\bpaused\b/.test(line)?[index+1]:[]);
      if(matches.length)pausedSources.push({path,sha256:hash,lines:matches});
    }
  }
  const permittedSource = !addedUtility || (pausedSources.length === 1 &&
    pausedSources[0].path === 'packages/ts/ui/src/hooks/use-paged-query.tsx');
  const fonts = paths.filter(name=>name.startsWith('diagnostics/') && /\.woff2?$/.test(name));
  const replaceReferences = text => {
    const script = text.replaceAll(old.file,next.file);
    return old.css && next.css ? script.replaceAll(old.css,next.css) : script;
  };
  const sameMarkup = replaceReferences(old.html) === next.html;
  const sameManifest = replaceReferences(old.manifest) === next.manifest;
  const reason = new Map([
    ['diagnostics/index.html', sameMarkup && 'entry-file-reference'],
    ['diagnostics/.vite/manifest.json', sameManifest && 'entry-file-reference'],
    ...entries.map(({ file }) => ['diagnostics/' + file, sharedPaging ? 'diagnostics-and-approved-shared-pagination' : 'diagnostic-metrics-and-event-reasons']),
    ...(addedUtility ? entries.map(({css})=>['diagnostics/'+css,'generated-utility-from-approved-shared-pagination']) : []),
  ]);
  const files = changes.map((path) => ({
    path,
    baselineSHA256: before[path] ?? null,
    candidateSHA256: after[path] ?? null,
    classification:
      selection && (path === 'console/index.html' || /^console\/assets\/[^/]+\.(js|css)$/.test(path))
        ? 'approved-common-console-build'
        : reason.get(path) || 'unexplained',
  }));
  const counts = Object.fromEntries(
    apps.map((app) => {
      const names = paths.filter((f) => f.startsWith(app + '/'));
      return [
        app,
        {
          baseline: names.filter((f) => before[f]).length,
          candidate: names.filter((f) => after[f]).length,
          identical: names.filter((f) => before[f] === after[f]).length,
        },
      ];
    }),
  );
  const result = {
    baseline: base.commit,
    candidate: release.inputs.sourceSHA256,
    counts,
    ...(selection && { frontend: {
      originalConsoleSHA256: consoleDigest(consoleFiles(before)),
      consoleSHA256: selection.console.sha256,
      backendReleaseSHA256: selection.backendReleaseSHA256,
      deliveryReleaseSHA256: selection.deliveryReleaseSHA256,
      baselineReleaseSHA256: selection.baselineReleaseSHA256,
      approvedSources: selection.approvedSources,
      sourceDiff: selection.sourceDiff,
      ...(selection.beforeImportFocus && {previousConsoleSHA256:selection.beforeImportFocus.console.sha256,
        previousIssuerSHA256:selection.beforeImportFocus.issuerReleaseSHA256,
        previousFiles:Object.keys(selection.beforeImportFocus.console.files).length,
        importFocusSourceDiff:selection.beforeImportFocus.changesToCurrent,previousConsole}),
    } }),
    sameMarkup,
    sameManifest,
    diagnosticsStyle: {
      sameBytes:sameStyle, permittedAddition:addedUtility ? utility : null,
      original:{file:old.css ?? null,bytes:Buffer.byteLength(old.cssContent),sha256:digest(old.cssContent)},
      candidate:{file:next.css ?? null,bytes:Buffer.byteLength(next.cssContent),sha256:digest(next.cssContent)},
      candidateWithoutAdditionSHA256:digest(addedUtility ? next.cssContent.replace(utility,'') : next.cssContent),
      fonts:{count:fonts.length,allIdentical:fonts.every(name=>before[name]===after[name])},
      pausedSources,permittedSource,
    },
    files,
    sources: ['apps/diagnostics/src/format.ts', 'packages/ts/diagnostics-client/src/reasons.ts'],
    passed: sameMarkup && sameManifest && permittedSource && (sameStyle || Boolean(addedUtility)) &&
      files.every((v) => v.classification !== 'unexplained'),
  };
  const path = join(output, 'visual', 'static-assets.json');
  if (existsSync(path)) {
    const previous = read(path);
    if (previous.candidate !== result.candidate && /^[a-f0-9]{64}$/.test(previous.candidate))
      write(join(output, 'visual', 'attempts', 'static-assets-' + previous.candidate + '.json'), previous);
  }
  write(path, result);
  return result;
}
