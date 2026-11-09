import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { testEnv } from './test-environment.mjs';
mkdirSync('build/e2e', { recursive: true });
execFileSync('go', ['test', '-c', '-o', 'build/e2e/api-test', './services/api/internal/transport/http'], { stdio: 'inherit' });
const child = spawn('./build/e2e/api-test', ['-test.run=^TestE2EServer$', '-test.timeout=0'], { stdio: 'inherit', env: { ...process.env, ...testEnv, INFRAPRO_E2E_SERVER: '1', INFRAPRO_E2E_FIXTURE_FILE: `${process.cwd()}/build/e2e/fixture.json` } });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('exit', (code) => process.exit(code ?? 1));
