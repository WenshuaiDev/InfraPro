import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  fileLease,
  read,
  root,
  runDirectory,
  safeDiagnosticLine,
  write,
} from './architecture-lab.mjs';
import {
  recordCommand,
  regressionRelease,
  verifyRegressionSource,
} from './architecture-regression.mjs';
import { regressionInfra, stopRegressionStorage } from './architecture-regression-infra.mjs';
import { testEnv } from './test-environment.mjs';
import { httpStorage } from './architecture-http-storage.mjs';

const frozenOwners = `package http_test
import("testing";"log/slog";"net/http";"github.com/WenshuaiDev/InfraPro/packages/go/platform/config";"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity";"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring")
func architectureOwners(t *testing.T,f originalRouter,l *slog.Logger,s *identity.Service,c config.Config,stores ...*monitoring.Storage) http.Handler{return f(l,s,c,stores...)}\n`;
// The test overlay adds recording to the unchanged baseline test fixtures.
// It changes only *_test.go. It never modifies a frozen handler or attestation.
export async function domainTestEnvironment(
  dir,
  side,
  suite = 'http',
  selectedStorage,
  workingSource,
) {
  assert.ok(['baseline', 'candidate'].includes(side));
  const repo =
    workingSource ?? (side === 'baseline' ? read(join(dir, 'baseline.json')).source : root);
  const storage =
    selectedStorage || (await httpStorage(dir, read(join(dir, 'private/lab.json')).id));
  const out = join(dir, 'http', 'domain', side);
  mkdirSync(out, { recursive: true, mode: 0o700 });
  const record = mkdtempSync(join(out, suite + '-records-'));
  const folder = join(repo, 'services/api/internal/transport/http'),
    replacements = {};
  for (const name of readdirSync(folder).filter((f) => f.endsWith('_test.go'))) {
    const file = join(folder, name),
      src = readFileSync(file, 'utf8');
    const changed = src
      .replaceAll(
        'httptransport.NewRouter(logger,',
        'architectureRouter(t, httptransport.NewRouter, logger,',
      )
      .replaceAll('127.0.0.1:58333', new URL(storage.s3).host)
      .replaceAll('127.0.0.1:59000', storage.ch)
      .replaceAll('127.0.0.1:59001', storage.ch2);
    if (changed === src) continue;
    const target = join(out, name);
    writeFileSync(target, changed);
    replacements[file] = target;
  }
  for (const [name, text] of [
    [
      'architecture_recorder_test.go',
      readFileSync(join(root, 'scripts/fixtures/architecture-http-recorder.go.txt'), 'utf8'),
    ],
    [
      'architecture_owners_test.go',
      side === 'baseline'
        ? frozenOwners
        : readFileSync(join(root, 'scripts/fixtures/architecture-test-owners.go.txt'), 'utf8'),
    ],
  ]) {
    const target = join(out, name);
    writeFileSync(target, text);
    replacements[join(folder, name)] = target;
  }
  const overlay = join(out, 'overlay.json');
  write(overlay, { Replace: replacements });
  return {
    cwd: repo,
    env: {
      ...process.env,
      ...testEnv,
      GOFLAGS: '-overlay=' + overlay,
      INFRAPRO_ARCH_TEST_ROUTES: join(root, 'services/api/internal/routes/ownership.tsv'),
      INFRAPRO_ARCH_TEST_RECORD: record,
      INFRAPRO_ARCH_TEST_TOPOLOGY: join(dir, 'topology'),
    },
  };
}
export async function domainHTTP(dir, side, { stopStorage = false } = {}) {
  assert.ok(['baseline', 'candidate'].includes(side));
  const release = regressionRelease(dir, side);
  const cwd = side === 'baseline' ? release.source : root;
  const unlock = await fileLease(join(dir, 'regression', 'locks', 'contract-' + side + '.flock'));
  try {
    verifyRegressionSource(release);
    const scoped = await regressionInfra(dir, side, cwd, {}, async (storage) => {
      const recording = await domainTestEnvironment(dir, side, 'http', storage);
      return Object.fromEntries(
        Object.entries(recording.env).filter(
          ([name]) => name === 'GOFLAGS' || name.startsWith('INFRAPRO_ARCH_TEST_'),
        ),
      );
    });
    const isolated = read(scoped.ARC_REGRESSION_INFRA);
    const env = {
      ...Object.fromEntries(
        Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_')),
      ),
      ...scoped,
      INFRAPRO_FILES_CHECKER_EXECUTABLE: join(release.bin, 'files-checker'),
    };
    // The original Node entry points receive remapped URLs through the module
    // hook. This entry point invokes go test directly and needs the same ports.
    for (const [name, value] of Object.entries(testEnv)) {
      const endpoint = new URL(value);
      assert.equal(endpoint.hostname, '127.0.0.1');
      assert.ok(isolated.mapping[endpoint.port]);
      endpoint.port = isolated.mapping[endpoint.port];
      env[name] = endpoint.href;
    }
    execFileSync('docker', ['compose', '-f', isolated.accountCompose, 'up', '-d', '--wait'], {
      cwd,
      env,
      stdio: ['ignore', 'ignore', 'pipe'],
    });
    const folder = join(dir, 'http', 'domain', side);
    const result = await recordCommand(
      folder,
      'http-tests',
      'go',
      ['test', '-v', '-count=1', '-timeout', '30m', './services/api/internal/transport/http'],
      {
        cwd,
        env,
        provenance: {
          role: side,
          commit: release.inputs?.commit ?? release.commit,
          releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
          node: process.version,
          storageProject: isolated.project,
          accountProject: isolated.accounts,
        },
      },
    );
    result.recordDirectory = basename(env.INFRAPRO_ARCH_TEST_RECORD);
    verifyRegressionSource(release, result);
    write(join(folder, 'http-tests.json'), result);
    return result;
  } finally {
    try {
      if (stopStorage) stopRegressionStorage(dir, side);
    } finally {
      await unlock();
    }
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const result = await domainHTTP(runDirectory(process.argv[2]), process.argv[3]);
    console.log(result.status);
    process.exitCode = result.status === 'passed' ? 0 : result.exitCode || 1;
  } catch (error) {
    console.error(safeDiagnosticLine(error.message ?? error.name).slice(0, 450));
    process.exitCode = 1;
  }
}
