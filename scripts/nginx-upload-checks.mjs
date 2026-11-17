// Real Nginx boundary/streaming checks. The upstream records received bytes only.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { createHash } from 'node:crypto';
import http from 'node:http';
import https from 'node:https';
import net from 'node:net';

const out = resolve('.artifacts/nginx-uploads');
mkdirSync(out, { recursive: true });
rmSync(join(out, 'proxy-result.json'), { force: true });
const nginx = process.env.INFRAPRO_TEST_NGINX ?? 'nginx';
const results = [];
let receivedHeaders, firstByte;
const payload = Buffer.alloc(8 * 1024 ** 2 + 17, 107);
const digest = value => createHash('sha256').update(value).digest('hex');
const upstream = http.createServer(async (req, res) => {
  receivedHeaders = req.headers;
  if (req.headers['x-probe'] === 'disconnect') { req.socket.destroy(); return; }
  if (req.headers['x-probe'] === 'unavailable') { res.writeHead(503, { 'Content-Type': 'application/json' }); res.end('{"code":"domain_unavailable"}'); return; }
  if (req.headers['x-probe'] === 'download') { res.setHeader('Content-Type', 'application/octet-stream'); res.setHeader('Content-Disposition', 'attachment; filename="fixture.bin"'); res.end(payload); return; }
  if (req.headers['x-probe'] === 'events') { res.setHeader('Content-Type', 'text/event-stream'); res.write('data: first\n\n'); setTimeout(() => res.end('data: last\n\n'), 600); return; }
  let bytes = 0; const hash = createHash('sha256');
  for await (const data of req) { bytes += data.length; hash.update(data); firstByte?.(); }
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({ bytes, sha256: hash.digest('hex') }));
});
await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
async function freePort() { const server = net.createServer(); await new Promise(resolve => server.listen(0, '127.0.0.1', resolve)); const port = server.address().port; await new Promise(resolve => server.close(resolve)); return port; }
function call(origin, path, body, headers = {}, options = {}) {
  return new Promise((resolve, reject) => {
    const req = (origin.startsWith('https:') ? https : http).request(origin + path, { method: body ? 'PUT' : 'GET', ...options, headers: { Host: origin.includes('https:') ? new URL(origin).host : 'console.infra.lan', ...headers, ...(body ? { 'Content-Length': body.length } : {}) } }, res => {
      const chunks = []; res.on('data', b => chunks.push(b)); res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, body: Buffer.concat(chunks) })); res.on('error', reject);
    });
    req.on('error', reject); req.end(body);
  });
}
const tls = join(out, 'tls'); mkdirSync(tls, { recursive: true, mode: 0o700 });
execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', join(tls, 'server-key.pem'), '-out', join(tls, 'server-chain.pem'), '-days', '2', '-subj', '/CN=127.0.0.1', '-addext', 'subjectAltName=IP:127.0.0.1'], { stdio: 'pipe' });
try {
  for (const template of ['deploy/nginx/default.conf', 'deploy/display/nginx.conf', 'deploy/display/https.nginx.conf']) {
    const secure = template.includes('https.'), port = await freePort(), runtimePort = await freePort(), assetsPort = await freePort();
    const directory = join(out, template.replaceAll('/', '-')); mkdirSync(directory, { recursive: true });
    const target = `127.0.0.1:${upstream.address().port}`;
    let conf = readFileSync(template, 'utf8').replaceAll('api:8080', target).replaceAll('127.0.0.1:8080', target).replaceAll('127.0.0.1:18000', target).replaceAll('127.0.0.1:5180', target).replaceAll('127.0.0.1:5181', target);
    if (secure) conf = conf.replaceAll('10.23.52.152', '127.0.0.1').replaceAll('18443', String(port)).replaceAll('18444', String(runtimePort)).replaceAll('18445', String(assetsPort)).replaceAll('/etc/infrapro/tls', tls).replace(/listen (\d+) ssl;/g, 'listen 127.0.0.1:$1 ssl;');
    else conf = conf.replaceAll('listen 80;', `listen 127.0.0.1:${port};`);
    const path = join(directory, 'nginx.conf'); writeFileSync(path, `pid ${directory}/nginx.pid;\nerror_log ${directory}/nginx.log;\nevents {}\nhttp { ${conf}\n}\n`);
    execFileSync(nginx, ['-t', '-p', `${directory}/`, '-c', path], { stdio: 'inherit' });
    const child = spawn(nginx, ['-p', `${directory}/`, '-c', path, '-g', 'daemon off;'], { stdio: 'inherit' });
    const origin = `${secure ? 'https' : 'http'}://127.0.0.1:${port}`, options = secure ? { ca: readFileSync(join(tls, 'server-chain.pem')) } : {};
    try {
      let ready = false;
      for (let i = 0; i < 50; i++) { try { await call(origin, '/api/v1/auth/me', undefined, {}, options); ready = true; break; } catch { await new Promise(resolve => setTimeout(resolve, 100)); } }
      assert.ok(ready, 'proxy started');
      let count = 0;
      for (const scope of ['/api/v1/tenants/t', '/api/v1/tenants/t/projects/p']) {
        for (const [suffix, size] of [['files/uploads/u/parts/1', 8<<20], ['spatial/uploads/u/parts/1', 8<<20], ['monitoring/imports/i/upload/parts/1', 32<<20], ...(scope.includes('/projects/') ? [['displays/instances/i/uploads/u/parts/0', 4<<20]] : [])]) {
          const body = Buffer.alloc(size, 105), path = `${scope}/${suffix}`;
          const ok = await call(origin, path, body, { 'Content-Type': 'application/octet-stream', 'X-Forwarded-Proto': 'forged', Forwarded: 'forged' }, options);
          assert.equal(ok.status, 200, `${template} ${suffix}`); assert.deepEqual(JSON.parse(ok.body), { bytes: size, sha256: digest(body) });
          assert.equal(receivedHeaders['x-forwarded-proto'], secure ? 'https' : 'http'); assert.equal(receivedHeaders.forwarded, undefined);
          const reject = await call(origin, path, Buffer.alloc(size+1), {}, options);
          assert.equal(reject.status, 413); assert.equal(JSON.parse(reject.body).code, 'request_too_large'); count += 2;
        }
      }
      for (const [path, size] of [['/api/v1/auth/login',16<<10], ['/api/v1/tenants/t/files/uploads',64<<10], ['/api/v1/tenants/t/spatial/assets',256<<10], ['/api/v1/tenants/t/monitoring/imports',256<<10], ['/api/v1/tenants/t/engineering/type-definitions',272<<10], ['/api/v1/platform/device-classifications/apply',8<<20], ['/api/v1/platform/parsing/samples/s/content',6<<20]]) {
        assert.equal((await call(origin,path,Buffer.alloc(size),{},options)).status,200,path);
        assert.equal((await call(origin,path,Buffer.alloc(size+1),{},options)).status,413,path); count += 2;
      }
      for (const [path, size] of [['/api/v1/platform/tenants',32<<10], ['/api/v1/tenants/t/projects/p/resource-access/grants',256<<10]]) {
        assert.equal((await call(origin,path,Buffer.alloc(size),{},options)).status,200,path);
        assert.equal((await call(origin,path,Buffer.alloc(size+1),{},options)).status,413,path); count += 2;
      }
      const downloads = ['/api/v1/tenants/t/files/attachments/a/revisions/r/content','/api/v1/tenants/t/spatial/versions/v/source','/api/v1/tenants/t/monitoring/source-files/s/download','/api/v1/tenants/t/engineering/resources/facilities/f/attachments/a/attachment-content'];
      for (const path of downloads) { const r = await call(origin,path,undefined,{'X-Probe':'download'},options); assert.equal(r.status,200); assert.equal(digest(r.body),digest(payload)); count++; }
      const domain = await call(origin,'/api/v1/auth/me',undefined,{'X-Probe':'unavailable'},options); assert.equal(JSON.parse(domain.body).code,'domain_unavailable'); count++;
      const gateway = await call(origin,'/api/v1/auth/me',undefined,{'X-Probe':'disconnect'},options); assert.equal(gateway.status,503); assert.equal(JSON.parse(gateway.body).code,'unavailable'); count++;
      // Upload streaming: the upstream must receive the first byte while the
      // remaining request body has not even been sent by this client.
      await new Promise((resolve, reject) => {
        const req = (secure ? https : http).request(origin+'/api/v1/tenants/t/files/uploads/u/parts/1', { ...options, method:'PUT',headers:{Host:secure?new URL(origin).host:'console.infra.lan','Content-Length':2} }, res => { res.resume();res.on('end',resolve); });
        const timer=setTimeout(()=>{req.destroy();reject(Error('request was buffered'));},2000);
        firstByte=()=>{firstByte=undefined;clearTimeout(timer);req.end('b');};req.on('error',reject);req.write('a');
      }); count++;
      // SSE must deliver its first event before the upstream finishes.
      await new Promise((resolve,reject)=>{const start=Date.now();const req=(secure?https:http).get(origin+'/api/v1/tenants/t/projects/p/displays/events',{...options,headers:{Host:secure?new URL(origin).host:'console.infra.lan','X-Probe':'events'}},res=>{let first=true;res.on('data',()=>{if(first){first=false;try{assert.ok(Date.now()-start<500,'SSE was buffered');}catch(e){reject(e);}}});res.on('end',resolve);});req.on('error',reject);});count++;
      if (secure) {
        const cookie='infrapro_session=person; infrapro_display=runtime; infrapro_terminal=terminal';
        await call(`https://127.0.0.1:${runtimePort}`,'/tool/t/inspect',undefined,{Cookie:cookie},options);
        assert.ok(!receivedHeaders.cookie.includes('infrapro_session'));assert.ok(receivedHeaders.cookie.includes('infrapro_display'));
        await call(`https://127.0.0.1:${assetsPort}`,'/assets/test',undefined,{Cookie:cookie,Authorization:'Bearer synthetic'},options);
        assert.equal(receivedHeaders.cookie,undefined);assert.equal(receivedHeaders.authorization,undefined);count+=2;
      }
      if (template.includes('/display/')) {
        const toolOrigin=secure?`https://127.0.0.1:${runtimePort}`:origin;
        const headers=secure?{Cookie:'infrapro_session=person; infrapro_display=runtime'}:{Host:'display.infra.lan'};
        const ok=await call(toolOrigin,'/tool/t/part?part=0',Buffer.alloc(4<<20),headers,options);
        assert.equal(ok.status,200);assert.equal(JSON.parse(ok.body).bytes,4<<20);
        if(secure)assert.ok(!receivedHeaders.cookie.includes('infrapro_session'));
        const tooLarge=await call(toolOrigin,'/tool/t/part?part=0',Buffer.alloc((4<<20)+1),headers,options);
        assert.equal(tooLarge.status,413);assert.equal(JSON.parse(tooLarge.body).code,'request_too_large');count+=2;
      }
      results.push({template,checks:count,passed:true});
    } finally { child.kill('SIGQUIT'); await new Promise(resolve=>child.once('exit',resolve)); }
  }
  writeFileSync(join(out,'proxy-result.json'),JSON.stringify({passed:true,productionDeployment:false,results},null,2)+'\n');
  console.log(JSON.stringify(results));
} finally { upstream.closeAllConnections();await new Promise(resolve=>upstream.close(resolve)); }
