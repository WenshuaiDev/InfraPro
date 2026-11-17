// Isolated Nginx TLS integration. No production services or trust stores are modified.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, rmSync, openSync, closeSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { createHash } from 'node:crypto';
import http from 'node:http';
import net from 'node:net';
import assert from 'node:assert/strict';
import { ensureTestDependencies } from './test-environment.mjs';

const root = resolve('.'), out = join(root, '.artifacts/https-20261117');
mkdirSync(out, { recursive: true });
rmSync(join(out, 'result.json'), { force: true });
const run = (cmd, args, options = {}) => execFileSync(cmd, args, { stdio: 'inherit', ...options });
const nginx = process.env.INFRAPRO_TEST_NGINX ?? 'nginx';
run(nginx, ['-v']);
ensureTestDependencies();
run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', 's3', 'audit-volume']);
if (process.env.INFRAPRO_HTTPS_SKIP_BUILD !== '1') {
  run('pnpm', ['display:pack']);
  run('pnpm', ['--filter', '@infrapro/console', 'build', '--outDir', join(out, 'web/console'), '--emptyOutDir'], { env: { ...process.env, VITE_APP_BASE: '/console/' } });
}
const tls = join(out, 'tls');
mkdirSync(tls, { recursive: true, mode: 0o700 });
const openssl = (args, input) => execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] });
openssl(['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', join(tls, 'ca-key.pem'), '-out', join(tls, 'ca.pem'), '-days', '2', '-subj', '/CN=InfraPro isolated HTTPS test CA', '-addext', 'basicConstraints=critical,CA:TRUE', '-addext', 'keyUsage=critical,keyCertSign,cRLSign']);
openssl(['req', '-newkey', 'rsa:2048', '-nodes', '-keyout', join(tls, 'server-key.pem'), '-out', join(tls, 'server.csr'), '-subj', '/CN=127.0.0.1']);
writeFileSync(join(tls, 'ext.cnf'), 'subjectAltName=IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n');
openssl(['x509', '-req', '-in', join(tls, 'server.csr'), '-CA', join(tls, 'ca.pem'), '-CAkey', join(tls, 'ca-key.pem'), '-CAcreateserial', '-out', join(tls, 'server-chain.pem'), '-days', '2', '-extfile', join(tls, 'ext.cnf')]);
openssl(['verify', '-CAfile', join(tls, 'ca.pem'), '-verify_ip', '127.0.0.1', join(tls, 'server-chain.pem')]);
const pub = openssl(['x509', '-in', join(tls, 'server-chain.pem'), '-pubkey', '-noout']);
const spki = createHash('sha256').update(openssl(['pkey', '-pubin', '-outform', 'DER'], pub)).digest('base64');

// Record only booleans/header names, never credentials or asset permit URLs.
const observed = { api: [], runtime: [], assets: [] }, spies = [];
async function freePort() { const s=net.createServer(); await new Promise(r=>s.listen(0,'127.0.0.1',r)); const port=s.address().port; await new Promise(r=>s.close(r)); return port; }
const apiPort=await freePort(), runtimePort=await freePort(), assetsPort=await freePort(), apiSpy=await freePort(), runtimeSpy=await freePort(), assetsSpy=await freePort();
let processNginx;
try {
  for (const port of [28443, 28444, 28445]) {
    const probe = net.createServer();
    await new Promise((resolve, reject) => {
      probe.once('error', reject);
      probe.listen(port, '127.0.0.1', () => probe.close(resolve));
    });
  }
  for (const [name, port, upstream] of [['api', apiSpy, apiPort], ['runtime', runtimeSpy, runtimePort], ['assets', assetsSpy, assetsPort]]) {
    const server = http.createServer((req, res) => {
      const cookieNames = (req.headers.cookie ?? '').split(';').map(v => v.trim().split('=')[0]).filter(Boolean);
      observed[name].push({ cookieNames, authorization: Boolean(req.headers.authorization), forwarded: req.headers['x-forwarded-proto'], ip: req.headers['x-forwarded-for'] });
      const target = http.request({ hostname: '127.0.0.1', port: upstream, path: req.url, method: req.method, headers: req.headers }, response => {
        res.writeHead(response.statusCode, response.headers); response.pipe(res);
      });
      target.on('error', () => { if (!res.headersSent) res.writeHead(503); res.end(); });
      req.pipe(target); res.on('close', () => target.destroy());
    });
    await new Promise((resolve, reject) => { server.once('error', reject); server.listen(port, '127.0.0.1', resolve); });
    spies.push(server);
  }
  let conf = readFileSync('deploy/display/https.nginx.conf', 'utf8');
  conf = conf.replaceAll('10.23.52.152', '127.0.0.1').replaceAll('18443', '28443').replaceAll('18444', '28444').replaceAll('18445', '28445')
    .replace(/listen (2844[345]) ssl;/g, 'listen 127.0.0.1:$1 ssl;')
    .replaceAll('/etc/infrapro/tls', tls).replace('/opt/infrapro/web', join(out, 'web')).replaceAll('127.0.0.1:18000', `127.0.0.1:${apiSpy}`).replaceAll('127.0.0.1:5180', `127.0.0.1:${runtimeSpy}`).replaceAll('127.0.0.1:5181', `127.0.0.1:${assetsSpy}`);
  // The worker uses the same user; explicit mime table keeps this harness portable.
  writeFileSync(join(out, 'nginx.conf'), `worker_processes 1;\npid ${join(out, 'nginx.pid')};\nerror_log ${join(out, 'nginx.log')};\nevents { worker_connections 1024; }\nhttp { types { text/html html; text/css css; application/javascript js mjs; image/svg+xml svg; }\n${conf}\n}\n`);
  run(nginx, ['-t', '-p', out + '/', '-c', join(out, 'nginx.conf')]);
  const fd = openSync(join(out, 'nginx-process.log'), 'w');
  processNginx = spawn(nginx, ['-p', out + '/', '-c', join(out, 'nginx.conf'), '-g', 'daemon off;'], { stdio: ['ignore', fd, fd] }); closeSync(fd);
  const env = { ...process.env, NODE_EXTRA_CA_CERTS: join(tls, 'ca.pem'), INFRAPRO_DISPLAY_API_ADDR: `127.0.0.1:${apiPort}`, INFRAPRO_E2E_RUNTIME_ADDR: `127.0.0.1:${runtimePort}`, INFRAPRO_E2E_ASSET_ADDR: `127.0.0.1:${assetsPort}`, INFRAPRO_DISPLAY_HTTPS: '1', INFRAPRO_DISPLAY_SKIP_BUILD: '1', INFRAPRO_DISPLAY_OUT: join(out, 'browser'), INFRAPRO_HTTPS_SPKI: spki };
  const child = spawn(process.execPath, ['scripts/display-browser-check.mjs'], { env, stdio: 'inherit' });
  const code = await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', resolve); });
  assert.equal(code, 0, 'HTTPS browser and tool journeys');
  assert.ok(observed.api.length > 0 && observed.runtime.length > 0 && observed.assets.length > 0);
  assert.ok(observed.api.every(r => r.forwarded === 'https' && r.ip === '127.0.0.1'), 'proxy overwrites scheme/client IP');
  assert.ok(observed.runtime.every(r => r.cookieNames.every(n => ['infrapro_display', 'infrapro_terminal'].includes(n))), 'runtime receives only its own cookies');
  assert.ok(observed.assets.every(r => !r.cookieNames.length && !r.authorization), 'asset upstream receives no credentials');
  const result = { passed: true, synthetic: true, browser: JSON.parse(readFileSync(join(out, 'browser/result.json'))), identity: JSON.parse(readFileSync(join(out, 'browser/https-identity.json'))), proxyRequests: Object.fromEntries(Object.entries(observed).map(([k, v]) => [k, v.length])), tls: 'Node/Playwright API verifies the CA chain and IP SAN; Chromium pins only this generated leaf SPKI, without changing OS trust', productionDeployment: false };
  writeFileSync(join(out, 'result.json'), JSON.stringify(result, null, 2) + '\n');
  console.log('HTTPS Nginx, identity, display, tool and credential-boundary checks passed.');
} finally {
  processNginx?.kill('SIGQUIT');
  for (const s of spies) { s.closeAllConnections(); s.close(); }
}
