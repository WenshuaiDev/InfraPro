import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, unlinkSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { digest, read, write } from './architecture-lab.mjs';
import { prepareFrozenBuild } from './architecture-frozen-build.mjs';

export const importFocusSources = new Set([
  'apps/console/src/features/monitoring/batch-page.tsx',
  'apps/console/src/features/monitoring/imports-page.tsx',
  'apps/console/src/features/monitoring/import-return-focus.ts',
]);

// The original backend and its original console remain immutable. The
// selected frontend is served independently by both backend topologies.
export const approvedConsoleSources = new Set([
  'apps/console/src/features/projects/governance-page.tsx',
  'apps/console/src/features/tenancy/member-actions.tsx',
  'apps/console/src/features/tenancy/context.tsx',
  'apps/console/src/features/tenancy/directory-action.ts',
  'apps/console/src/features/tenancy/membership-shared.tsx',
  'apps/console/src/features/tenancy/membership-ui.tsx',
  'apps/console/src/features/tenancy/members-page.tsx',
  'apps/console/src/features/tenancy/invitation-actions.tsx',
  'apps/console/src/features/tenancy/invitations-page.tsx',
  'apps/console/src/hooks/use-cursor-list.ts',
  'packages/ts/ui/src/hooks/use-paged-query.tsx',
  ...importFocusSources,
]);

const firstPublication = JSON.parse(readFileSync(new URL('./fixtures/architecture-console-before-import-focus.json',import.meta.url),'utf8'));

export function assetDigests(directory) {
  const files = {};
  if (!existsSync(directory)) return files;
  for (const entry of readdirSync(directory, { recursive: true, withFileTypes: true })) {
    if (!entry.isFile()) continue;
    const path = join(entry.parentPath, entry.name);
    files[path.slice(directory.length + 1)] = digest(readFileSync(path));
  }
  return files;
}

export const consoleDigest = files =>
  digest(JSON.stringify(Object.keys(files).sort().map(path => [path, files[path]])));

export function importFocusDelta(frozen, changes) {
  const focusChanges = changes.filter(change => importFocusSources.has(change.path));
  if (!focusChanges.length) return null;
  assert.equal(frozen.source_archive_sha256, firstPublication.frozenReleaseSHA256);
  assert.deepEqual(changes.filter(change => !importFocusSources.has(change.path)),firstPublication.sourceChanges,
    'The previous approved console corrections differ from their frozen sources');
  return focusChanges;
}

function beforeImportFocus(directory, releaseDirectory, frozen, currentSource, changes, previous) {
  const focusChanges = importFocusDelta(frozen,changes);
  if (!focusChanges) return null;
  const preserved = join(directory,'console-corrections','before-import-focus');
  const candidates = [{directory:preserved}, previous?.console, previous?.beforeImportFocus?.console];
  // A fresh architecture run can use the same immutable release or reproduce
  // it from S0 plus the eleven approved source changes. Both must yield the
  // checksum of the actual first publication.
  if (frozen.source) {
    const descriptor=join(dirname(frozen.source),'history/shared-console',firstPublication.consoleSHA256+'.json');
    if (existsSync(descriptor)) candidates.push(read(descriptor).console);
  }
  if(!candidates.some(entry=>entry?.directory && existsSync(entry.directory) &&
      (!entry.sha256 || entry.sha256===firstPublication.consoleSHA256))){
    for(const entry of readdirSync(dirname(directory),{withFileTypes:true})){
      if(!entry.isDirectory())continue;
      const path=join(dirname(directory),entry.name,'candidate.json');
      if(!existsSync(path) || !existsSync(join(dirname(path),'application-source.json')))continue;
      const release=read(path);
      if(release.inputs?.sourceSHA256===firstPublication.issuerReleaseSHA256)
        candidates.push({directory:join(release.release,'console'),sha256:firstPublication.consoleSHA256});
    }
  }
  let files = null, origin = null, rebuild = null, materialization='retained-publication';
  for (const saved of candidates) {
    if (!saved?.directory || !existsSync(saved.directory) || (saved.sha256 && saved.sha256!==firstPublication.consoleSHA256)) continue;
    const measured = assetDigests(saved.directory);
    if (consoleDigest(measured)===firstPublication.consoleSHA256 && Object.keys(measured).length===firstPublication.files) {
      origin=saved.directory;files=measured;break;
    }
  }
  if (!files) {
    materialization='reconstructed-and-byte-verified';
    const build=prepareFrozenBuild(directory,frozen,'console-before-import-focus',process.env);
    rebuild=build.cwd;
    const archive=join(releaseDirectory,'application-source.tar.gz');
    assert.equal(currentSource.archive,'application-source.tar.gz');
    assert.equal(digest(readFileSync(archive)),currentSource.archive_sha256);
    const paths=firstPublication.sourceChanges.map(change=>change.path);
    execFileSync('tar',['-xf',archive,...paths],{cwd:build.cwd});
    for(const change of firstPublication.sourceChanges) assert.equal(digest(readFileSync(join(build.cwd,change.path))),change.after);
    execFileSync('pnpm',['--filter','@infrapro/console','build'],{cwd:build.cwd,stdio:'inherit'});
    origin=join(build.cwd,'apps/console/dist');files=assetDigests(origin);
    assert.equal(Object.keys(files).length,firstPublication.files);
    assert.equal(consoleDigest(files),firstPublication.consoleSHA256,'Rebuilt intermediate console differs from the genuine publication');
  }
  if(origin!==preserved) {
    mkdirSync(dirname(preserved),{recursive:true,mode:0o700});
    if(existsSync(preserved))renameSync(preserved,preserved+'-unmatched-'+Date.now());
    cpSync(origin,preserved,{recursive:true});
  }
  assert(isDeepStrictEqual(assetDigests(preserved),files));
  const description=join(directory,'console-corrections','before-import-focus-source.json');
  if(origin===preserved && existsSync(description)){
    const retained=read(description);
    if(retained.sha256===firstPublication.consoleSHA256)materialization=retained.materialization;
  }else write(description,{materialization,origin:rebuild?frozen.sourceArchive:origin,sha256:firstPublication.consoleSHA256,
    ...(rebuild&&{sourceArchiveSHA256:currentSource.archive_sha256,command:['pnpm','--filter','@infrapro/console','build']})});
  if(rebuild)rmSync(rebuild,{recursive:true});
  return {issuerReleaseSHA256:firstPublication.issuerReleaseSHA256,sourceChanges:firstPublication.sourceChanges,materialization,
    changesToCurrent:focusChanges,console:{directory:preserved,files,sha256:firstPublication.consoleSHA256}};
}

export function sharedConsoleFor(directory, { verifyFiles = false } = {}) {
  const file = process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE || join(directory, 'shared-console.json');
  if (!existsSync(file)) {
    if (process.env.INFRAPRO_ARCHITECTURE_SHARED_CONSOLE) throw Error('Shared console descriptor is missing');
    return null;
  }
  const selection = read(file);
  assert.equal(consoleDigest(selection.console.files), selection.console.sha256);
  if (selection.beforeImportFocus) {
    const previous=selection.beforeImportFocus;
    assert.equal(previous.issuerReleaseSHA256,firstPublication.issuerReleaseSHA256);
    assert.equal(previous.console.sha256,firstPublication.consoleSHA256);
    assert.equal(Object.keys(previous.console.files).length,firstPublication.files);
    assert.equal(consoleDigest(previous.console.files),previous.console.sha256);
    assert.deepEqual(previous.sourceChanges,firstPublication.sourceChanges);
    assert.deepEqual(previous.changesToCurrent,importFocusDelta({source_archive_sha256:selection.baselineReleaseSHA256},selection.sourceDiff));
    if(verifyFiles){
      const source=read(selection.sourceManifest);
      assert.equal(source.application_source_sha256,selection.deliveryReleaseSHA256);
      for(const change of selection.sourceDiff)assert.equal(change.after,source.files[change.path]??null);
      assert.deepEqual(assetDigests(previous.console.directory),previous.console.files);
    }
  } else {
    assert(!selection.sourceDiff?.some(change=>importFocusSources.has(change.path)),'Intermediate console evidence is missing');
  }
  if (verifyFiles)
    assert.deepEqual(assetDigests(selection.console.directory), selection.console.files,
      'Served shared console no longer matches its selected files');
  return selection;
}

export function selectSharedConsole(directory, releaseDirectory, backend) {
  const frozen = read(join(directory, 'baseline.json'));
  const source = read(join(releaseDirectory, 'application-source.json'));
  assert.equal(source.application_source_sha256, backend.inputs.sourceSHA256);
  const checkedPaths = [...new Set([...Object.keys(frozen.sourceFiles), ...Object.keys(source.files)])]
    .filter(path => path.startsWith('apps/console/') || path.startsWith('packages/ts/ui/'))
    .sort();
  const sourceDiff = checkedPaths.filter(path => frozen.sourceFiles[path] !== source.files[path])
    .map(path => ({ path, before: frozen.sourceFiles[path] ?? null, after: source.files[path] ?? null }));
  const file = join(directory, 'shared-console.json');
  if (!sourceDiff.length) {
    if (existsSync(file)) {
      const prior = read(file);
      write(join(directory, 'history', 'shared-console', prior.console.sha256 + '.json'), prior);
      unlinkSync(file);
    }
    return null;
  }
  for (const change of sourceDiff)
    assert(approvedConsoleSources.has(change.path), 'Unexpected shared console source change: ' + change.path);
  const consoleDirectory = join(backend.release, 'console');
  const files = Object.fromEntries(Object.entries(backend.staticFiles)
    .filter(([path]) => path.startsWith('console/')).map(([path, sha]) => [path.slice(8), sha]));
  assert(files['index.html'] && Object.keys(files).length > 1);
  assert.deepEqual(assetDigests(consoleDirectory), files, 'Shared console differs from the sealed release');
  const prior = existsSync(file) ? read(file) : null;
  const predecessor = beforeImportFocus(directory,releaseDirectory,frozen,source,sourceDiff,prior);
  if(predecessor)assert.notEqual(consoleDigest(files),predecessor.console.sha256,
    'The new release still serves the previous console');
  const descriptor = {
    decision: predecessor
      ? '2026-11-19 批准项目治理分页、成员和邀请草稿以及监测导入回焦修复，两种后端使用同一控制台'
      : '2026-11-19 批准修复项目治理分页和成员、邀请草稿，使用同一控制台重验两种后端',
    baselineReleaseSHA256: frozen.source_archive_sha256,
    backendReleaseSHA256: backend.inputs.sourceSHA256,
    deliveryReleaseSHA256: source.application_source_sha256,
    sourceManifest: join(releaseDirectory, 'application-source.json'),
    approvedSources: sourceDiff.map(item => item.path),
    sourceDiff,
    console: { directory: consoleDirectory, sha256: consoleDigest(files), files },
    ...(predecessor && {beforeImportFocus:predecessor}),
  };
  if (prior) {
    if (prior.console.sha256 !== descriptor.console.sha256)
      write(join(directory, 'history', 'shared-console', prior.console.sha256 + '.json'), prior);
  }
  write(file, descriptor);
  return descriptor;
}
