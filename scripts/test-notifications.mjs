import { execFileSync } from 'node:child_process';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
ensureTestDependencies();
execFileSync('go', ['test', '-race', '-count=1', '-timeout=10m', './packages/go/platform/notifications', './services/api/internal/transport/http/notifications'], { stdio: 'inherit', env: { ...process.env, ...testEnv } });

execFileSync('go', ['test', '-race', '-count=1', '-timeout=10m', './services/api/internal/transport/http', '-run', '^TestNotification'], { stdio: 'inherit', env: { ...process.env, ...testEnv } });
