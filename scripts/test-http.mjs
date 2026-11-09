import { execFileSync } from 'node:child_process';
import { networkInterfaces } from 'node:os';
const networks = networkInterfaces();
const address = [...(networks.en0 || []), ...(networks.eth0 || [])].find((entry) => entry.family === 'IPv4' && !entry.internal)?.address;
process.env.INFRAPRO_HTTP_TEST_HOST ||= address;
if (!process.env.INFRAPRO_HTTP_TEST_HOST) throw new Error('Set INFRAPRO_HTTP_TEST_HOST to this machine’s LAN IPv4 address.');
const args = ['compose', '-f', 'deploy/compose/http-test.yaml'];
// This stack is dedicated to acceptance, with no external database or volume bindings.
try {
  execFileSync('docker', [...args, 'up', '-d', '--build', '--wait'], { stdio: 'inherit' });
  execFileSync('python3', ['tests/http/cli.py', 'bootstrap'], { stdio: 'inherit' });
  execFileSync('pnpm', ['exec', 'playwright', 'test', '-c', 'playwright.http.config.ts'], { stdio: 'inherit' });
} finally {
  execFileSync('docker', [...args, 'down', '--volumes'], { stdio: 'inherit' });
}
