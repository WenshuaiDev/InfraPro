import { test } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { startServer } from '../../packages/ts/display-devtools/src/server.mjs';

test('display tool supports HTTPS endpoints and keeps pairing on loopback', async () => {
  for (const scheme of ['http', 'https']) {
    const server = await startServer({ port: 29920, upload: true, console: `${scheme}://console.example/console/`, runtime: `${scheme}://runtime.example:18444`, instance: 'example-instance' });
    try {
      const url = new URL(server.loginURL);
      assert.equal(url.origin, `${scheme}://console.example`);
      assert.equal(url.pathname, '/console/display-launch');
      assert.equal(url.searchParams.get('audience'), 'http://127.0.0.1:29920');
      assert.equal(url.searchParams.get('purpose'), 'upload');
    } finally { await server.close(); }
  }
});

test('display tool rejects invalid remote endpoints and releases its ports', async () => {
  for (const edit of [
    { console: 'https://user:secret@console.example/console' },
    { console: 'https://console.example/console?x=1' },
    { console: 'https://console.example/console#' },
    { console: 'ftp://console.example' },
    { runtime: 'http://runtime.example' },
    { runtime: 'https://user:secret@runtime.example' },
    { runtime: 'https://runtime.example/path' },
    { runtime: 'https://runtime.example?' },
    { runtime: 'https://runtime.example#x' },
    { runtime: undefined },
    { runtime: 'not a URL' },
  ]) {
    await assert.rejects(startServer({ port: 29920, upload: true, console: 'https://console.example/console', runtime: 'https://runtime.example', instance: 'example-instance', ...edit }));
  }
  const server = await startServer({ port: 29920 });
  await server.close();
});

test('display pairing does not follow redirects carrying exchange credentials', async () => {
  let escaped = false, visits = 0;
  const target = http.createServer((_req, res) => { escaped = true; res.end('{}'); });
  await new Promise(resolve => target.listen(0, '127.0.0.1', resolve));
  const source = http.createServer((_req, res) => { visits++; res.writeHead(307, { Location: `http://127.0.0.1:${target.address().port}/capture` }); res.end(); });
  await new Promise(resolve => source.listen(0, '127.0.0.1', resolve));
  const server = await startServer({ port: 29920, upload: true, console: 'http://console.example', runtime: `http://127.0.0.1:${source.address().port}`, instance: 'synthetic' });
  try {
    const state = new URL(server.loginURL).searchParams.get('state');
    const reply = await fetch(server.url + '/pair', { method: 'POST', headers: { Origin: 'http://127.0.0.1:29920', 'X-Display-Protocol': '1', 'Content-Type': 'application/json' }, body: JSON.stringify({ state, code: 'synthetic-exchange-code' }) });
    assert.equal(reply.status, 400);
    assert.equal((await reply.json()).code, 'request_failed');
    assert.equal(visits, 1);
    assert.equal(escaped, false);
  } finally {
    await server.close();
    for (const s of [source, target]) { s.closeAllConnections(); await new Promise(resolve => s.close(resolve)); }
  }
});
