import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, symlinkSync } from 'node:fs';
import { join } from 'node:path';
import { digest } from './architecture-lab.mjs';

const routeTree = 'apps/console/src/routeTree.gen.ts';

// The original TanStack generator writes its tracked route tree during a
// typecheck and preview build. Other experiments read the frozen source live.
export function prepareFrozenBuild(dir, release, name, env) {
  assert.equal(digest(readFileSync(release.sourceArchive)), release.source_archive_sha256);
  const parent = join(dir, 'private', 'frozen-builds');
  mkdirSync(parent, { recursive: true, mode: 0o700 });
  const cwd = mkdtempSync(join(parent, name.replaceAll(':', '-') + '-'));
  const options = { cwd, env: { ...env, GIT_WORK_TREE: cwd }, stdio: 'pipe' };
  execFileSync('tar', ['-xf', release.sourceArchive], options);
  execFileSync('pnpm', ['install', '--offline', '--frozen-lockfile', '--reporter=silent'], options);
  const spatial = join(release.source, '.artifacts', 'spatial-venv');
  if (existsSync(join(spatial, 'bin', 'python'))) {
    mkdirSync(join(cwd, '.artifacts'), { recursive: true, mode: 0o700 });
    symlinkSync(spatial, join(cwd, '.artifacts', 'spatial-venv'), 'dir');
  }
  const differences = () =>
    Object.entries(release.sourceFiles).flatMap(([path, frozen]) => {
      const file = join(cwd, path),
        actual = existsSync(file) ? digest(readFileSync(file)) : null;
      return actual === frozen ? [] : [{ path, frozen, actual }];
    });
  assert.deepEqual(differences(), [], 'Frozen build inputs changed before the original command');
  return {
    cwd,
    inspect(result) {
      const changed = differences();
      const sameLines = (file) => readFileSync(file, 'utf8').split('\n').sort();
      const onlyGenerated = changed.every(
        ({ path, actual }) =>
          path === routeTree &&
          actual &&
          JSON.stringify(sameLines(join(cwd, path))) ===
            JSON.stringify(sameLines(join(release.source, path))),
      );
      result.frozenBuild = {
        directory: cwd,
        archiveSHA256: release.source_archive_sha256,
        sourceFiles: Object.keys(release.sourceFiles).length,
        generated: changed,
        onlyGenerated,
      };
      if (!onlyGenerated) {
        result.processStatus = result.status;
        result.status = 'failed';
        result.failure =
          'The original build changed an unexpected frozen input. Inspect frozenBuild.generated.';
      }
    },
  };
}
