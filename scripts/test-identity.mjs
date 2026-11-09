import { execFileSync } from 'node:child_process';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
ensureTestDependencies();
execFileSync('node', ['scripts/go.mjs', 'test'], { stdio: 'inherit', env: { ...process.env, ...testEnv } });
