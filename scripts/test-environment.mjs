import { execFileSync } from 'node:child_process';
export const testEnv = {
  INFRAPRO_TEST_DATABASE_URL: 'postgres://infrapro_test:test-only-local-password@127.0.0.1:55432/infrapro_test',
  INFRAPRO_TEST_REDIS_URL: 'redis://:test-only-local-password@127.0.0.1:56379/0',
};
export function ensureTestDependencies() {
  execFileSync('docker', ['compose', '-f', 'deploy/compose/test.yaml', 'up', '-d', '--wait'], { stdio: 'inherit' });
}
