import { spawn, execFileSync } from 'node:child_process';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
ensureTestDependencies();
execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', 's3'], { stdio: 'inherit' });
const child = spawn('go', ['test', './packages/go/platform/identity', './packages/go/platform/internal/contentcheck', './services/api/internal/transport/http', '-run', 'Avatar|RoutePolicyCoverage|HTTPAccountBoundary', '-count=1'], { stdio: 'inherit', env: { ...process.env, ...testEnv } });
child.on('exit', code => { process.exitCode = code ?? 1; });
