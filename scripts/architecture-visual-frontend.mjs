import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { digest, read, write } from './architecture-lab.mjs';
import { consoleDigest, sharedConsoleFor } from './architecture-shared-console.mjs';

export function visualOutputDirectory(directory, selection = sharedConsoleFor(directory)) {
  if (!selection) return directory;
  const hash = selection.console.sha256;
  assert.match(hash, /^[a-f0-9]{64}$/);
  for (let ancestor = resolve(directory); ancestor !== dirname(ancestor); ancestor = dirname(ancestor)) {
    if (basename(ancestor) === hash && basename(dirname(ancestor)) === 'profiles' &&
        basename(dirname(dirname(ancestor))) === 'visual') return directory;
  }
  return join(directory, 'visual', 'profiles', hash);
}

export function consoleFiles(files) {
  return Object.fromEntries(Object.entries(files)
    .filter(([name]) => name.startsWith('console/'))
    .map(([name, sha]) => [name.slice('console/'.length), sha]));
}

// Both processes in a frozen comparison run S0. Its console may come from an
// earlier sealed delivery of the same approved source changes and exact files.
// Retain that delivery's identity on every receipt, including after a reseal.
export function frozenConsoleIssuer(directory, selection) {
  if (!selection) return null;
  const descriptor = join(directory, 'shared-console.json');
  const build = join(directory, 'candidate.json');
  if (!existsSync(descriptor) || !existsSync(build)) return null;
  const issuer = read(descriptor), candidate = read(build);
  if (issuer.baselineReleaseSHA256 !== selection.baselineReleaseSHA256 ||
      issuer.backendReleaseSHA256 !== candidate.inputs.sourceSHA256 ||
      issuer.deliveryReleaseSHA256 !== candidate.inputs.sourceSHA256 ||
      issuer.console.sha256 !== selection.console.sha256 ||
      consoleDigest(issuer.console.files) !== issuer.console.sha256 ||
      !isDeepStrictEqual(issuer.approvedSources, selection.approvedSources) ||
      !isDeepStrictEqual(issuer.sourceDiff, selection.sourceDiff) ||
      !isDeepStrictEqual(issuer.console.files, selection.console.files) ||
      !isDeepStrictEqual(issuer.console.files, consoleFiles(candidate.staticFiles))) return null;
  return issuer;
}

function localBinding(origin) {
  const url = new URL(origin);
  assert(['127.0.0.1','localhost','[::1]'].includes(url.hostname) && !url.username && !url.password);
  return { host:url.hostname, port:Number(url.port || (url.protocol === 'https:' ? 443 : 80)), originSHA256:digest(url.origin) };
}

// Bound the work to two ordinary, unauthenticated static requests at a time.
// A web preview may return the application shell with HTTP 200 for an absent
// filename, so each file and the root document must match the expected bytes.
export async function readServedConsole(origin, expected) {
  const files = {}, failures = [];
  const index = expected['index.html'];
  assert(index && Object.keys(expected).length > 1, 'Sealed console file manifest required');
  const names = Object.keys(expected).sort();
  const iterator = names.values();
  async function readPath(name, urlPath, wanted) {
    assert(!name.split('/').some(part => part === '..' || part === '.'));
    try {
      const response = await fetch(new URL(urlPath, origin), { redirect: 'error', signal: AbortSignal.timeout(15000) });
      const actual = digest(Buffer.from(await response.arrayBuffer()));
      if (response.status !== 200 || actual !== wanted) failures.push({ name, status: response.status, expected: wanted, actual });
      return actual;
    } catch (error) {
      failures.push({ name, status: null, expected: wanted, error: error.name });
      return null;
    }
  }
  async function readFiles() {
    for (const name of iterator) {
      assert(name.length && !name.startsWith('/'));
      files[name] = await readPath(name, '/' + name.split('/').map(encodeURIComponent).join('/'), expected[name]);
    }
  }
  const root = await readPath('index.html', '/', index);
  if (root !== null) await Promise.all([readFiles(), readFiles()]);
  return { sha256: consoleDigest(files), root, count: Object.keys(files).length, files, failures };
}

export async function visualFrontend(dir, side, output) {
  assert(['b', 'self'].includes(side));
  const selected = sharedConsoleFor(output) ?? sharedConsoleFor(dir);
  const scoped = visualOutputDirectory(output, selected);
  const selection = side === 'self' && selected ? frozenConsoleIssuer(dir, selected) : selected;
  assert(!selected || selection, 'Frozen console source or files differ from the selected delivery');
  const frozen = read(join(dir, 'baseline.json'));
  const current = read(join(dir, 'candidate.json'));
  const lab = read(join(dir, 'private/lab.json'));
  if (selection) {
    assert.equal(selection.baselineReleaseSHA256, frozen.source_archive_sha256);
    assert.equal(selection.backendReleaseSHA256, current.inputs.sourceSHA256);
    assert.deepEqual(selection.console.files, consoleFiles(current.staticFiles));
  }
  const expected = selection?.console.files ?? consoleFiles(frozen.staticFiles);
  const peer = selection?.console.files ?? consoleFiles(side === 'b' ? current.staticFiles : frozen.staticFiles);
  const at = new Date().toISOString(), pair = {};
  assert.notEqual(localBinding(lab.a.origin).originSHA256,localBinding(lab[side].origin).originSHA256,
    'The reference and peer must run on independent HTTP origins');
  for (const [name, files] of [['a', expected], [side, peer]]) {
    pair[name] = { binding:localBinding(lab[name].origin), ...await readServedConsole(lab[name].origin, files) };
  }
  const report = {
    side, sourceDirectory: dir, at,
    baselineReleaseSHA256: frozen.source_archive_sha256,
    backendReleaseSHA256: current.inputs.sourceSHA256,
    deliveryReleaseSHA256: selection?.deliveryReleaseSHA256 ?? current.inputs.sourceSHA256,
    consoleSHA256: consoleDigest(expected),
    sources: selection?.sourceDiff ?? [],
    pair, passed: Object.values(pair).every(value => !value.failures.length),
  };
  const path = join(scoped, 'visual', 'served-console', side, at.replaceAll(':', '-') + '.json');
  write(path, report);
  assert(report.passed, 'Running console differs from its sealed release; inspect ' + path);
  const assets = Object.fromEntries(Object.entries(pair).map(([key, value]) => [key, { sha256: value.sha256, files: value.count }]));
  return {
    output: scoped,
    frontend: {
      profile: selection ? 'shared-console' : 'original-console',
      consoleSHA256: report.consoleSHA256,
      baselineReleaseSHA256: report.baselineReleaseSHA256,
      backendReleaseSHA256: report.backendReleaseSHA256,
      deliveryReleaseSHA256: report.deliveryReleaseSHA256,
      assets,
      identity: digest(JSON.stringify([report.consoleSHA256, report.baselineReleaseSHA256, report.backendReleaseSHA256, report.deliveryReleaseSHA256, assets])),
      evidence: resolve(path), evidenceSHA256: digest(readFileSync(path)),
    },
  };
}

export async function finishVisualFrontend(dir, side, output, prior) {
  const { frontend } = await visualFrontend(dir,side,output);
  assert.equal(frontend.identity, prior.identity, 'Published console changed during browser capture');
  return frontend;
}

export function verifiedVisualFrontend(selection, directory, report, side) {
  if (!selection) return true;
  if (side === 'self') {
    if (!report?.sourceDirectory) return false;
    selection = frozenConsoleIssuer(report.sourceDirectory, selection);
    if (!selection) return false;
  }
  const frontend = report?.frontend;
  if (!frontend || frontend.profile !== 'shared-console' || !frontend.evidence ||
      frontend.consoleSHA256 !== selection.console.sha256 ||
      frontend.baselineReleaseSHA256 !== selection.baselineReleaseSHA256 ||
      frontend.backendReleaseSHA256 !== selection.backendReleaseSHA256 ||
      frontend.deliveryReleaseSHA256 !== selection.deliveryReleaseSHA256) return false;
  const path = relative(resolve(visualOutputDirectory(directory, selection)), resolve(frontend.evidence));
  if (!path || path.startsWith('..') || !existsSync(frontend.evidence) ||
      digest(readFileSync(frontend.evidence)) !== frontend.evidenceSHA256) return false;
  const observed = read(frontend.evidence), names = ['a', side];
  const labPath = join(report.sourceDirectory,'private/lab.json');
  if (!existsSync(labPath)) return false;
  const lab = read(labPath);
  return observed.passed && observed.sourceDirectory === report.sourceDirectory &&
    observed.side === side && observed.backendReleaseSHA256 === selection.backendReleaseSHA256 &&
    observed.deliveryReleaseSHA256 === selection.deliveryReleaseSHA256 &&
    observed.baselineReleaseSHA256 === selection.baselineReleaseSHA256 &&
    observed.consoleSHA256 === selection.console.sha256 &&
    observed.pair.a?.binding?.originSHA256 !== observed.pair[side]?.binding?.originSHA256 &&
    isDeepStrictEqual(observed.sources, selection.sourceDiff) &&
    names.every(name => {
      const actual = observed.pair[name], saved = frontend.assets?.[name];
      return actual && saved && actual.failures.length === 0 &&
        isDeepStrictEqual(actual.binding,localBinding(lab[name].origin)) &&
        actual.root === selection.console.files['index.html'] &&
        actual.count === Object.keys(selection.console.files).length &&
        saved.files === actual.count && saved.sha256 === selection.console.sha256 &&
        actual.sha256 === saved.sha256 && consoleDigest(actual.files) === saved.sha256 &&
        isDeepStrictEqual(actual.files, selection.console.files);
    });
}
