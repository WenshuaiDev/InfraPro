import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync } from 'node:fs';
import { resolve } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
import { startParser } from './fixtures/parsing-lab.mjs';
ensureTestDependencies();
// HTTP attachment regression uses the real isolated object store and checker.
execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', 's3', 'audit-volume'], { stdio: 'inherit' });
mkdirSync('build/test-dependencies', { recursive: true });
const checker = resolve('build/test-dependencies/files-checker');
execFileSync('go', ['build', '-o', checker, './services/api/cmd/files-checker'], { stdio: 'inherit' });
// The all-module suite includes real parser integration tests. Keep the event
// loop running while Go tests execute so the fixture's output is drained.
mkdirSync('.artifacts/identity', { recursive: true });
const parser = await startParser(mkdtempSync(resolve('.artifacts/identity/parser-')));
try {
  const code = await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ['scripts/go.mjs', 'test'], {
      stdio: 'inherit',
      env: {
        ...process.env, ...testEnv,
        INFRAPRO_FILES_CHECKER_EXECUTABLE: checker,
        INFRAPRO_PARSING_URL: parser.url,
        INFRAPRO_PARSING_TOKEN_FILE: parser.token,
        INFRAPRO_PARSER_TEST_RUNNER: parser.runner,
        INFRAPRO_PARSER_TEST_PACKAGE: parser.pkg,
      },
    });
    const interrupt = () => child.kill('SIGINT');
    const terminate = () => child.kill('SIGTERM');
    process.on('SIGINT', interrupt);
    process.on('SIGTERM', terminate);
    child.once('close', () => {
      process.off('SIGINT', interrupt);
      process.off('SIGTERM', terminate);
    });
    child.once('error', reject);
    child.once('exit', code => resolve(code ?? 1));
  });
  process.exitCode = code;
} finally {
  await parser.close();
}
