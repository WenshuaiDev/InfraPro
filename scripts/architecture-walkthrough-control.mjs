import assert from 'node:assert/strict';
import { chmodSync } from 'node:fs';
import { createConnection, createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, runDirectory } from './architecture-lab.mjs';

// This fixture socket has filesystem permissions and no TCP listener. Its
// dependency commands and optional shutdown affect only this stack.
const socketPath = (id, side) => join('/tmp', 'infrapro-arc-' + id + '-' + side, 'operator.sock');
export async function walkthroughControl(stack, onPark) {
  const { lab, side, manual, chGate } = stack,
    path = socketPath(lab.id, side);
  const cfg = lab[side];
  const identity = {
    environment: lab.id,
    side,
    releaseSHA256: stack.releaseSHA256,
    ...(stack.sharedConsoleSHA256 && { sharedConsoleSHA256: stack.sharedConsoleSHA256 }),
    launchID: manual.launchID,
    database: cfg.database,
    console: cfg.origin,
    runtime: cfg.runtime,
    diagnostics: 'http://127.0.0.1:' + cfg.ports.diagnostics,
  };
  let previous = Promise.resolve();
  const server = createServer({ allowHalfOpen: true }, (socket) => {
    socket.setTimeout(15000, () => socket.destroy());
    socket.setEncoding('utf8');
    socket.on('error', () => socket.destroy());
    let body = '';
    socket.on('data', (chunk) => {
      body += chunk;
      if (body.length > 200) socket.destroy();
    });
    socket.once('end', () => {
      const match = /^(status|park|(parser|broker|clickhouse) (stop|start)) ([a-f0-9-]{36}) ([a-f0-9]{64})(?: ([a-f0-9]{64}))?\n$/.exec(body);
      if (!match || match[4] !== identity.launchID || match[5] !== identity.releaseSHA256 || match[6] !== identity.sharedConsoleSHA256) {
        socket.end('Operation does not target this running walkthrough\n');
        return;
      }
      const [, command, name, action] = match;
      previous = previous
        .then(async () => {
          for (const key of ['api', 'diagnostics']) {
            assert.equal(stack.children[key].exitCode, null);
            assert.equal(stack.children[key].signalCode, null);
            process.kill(stack.children[key].pid, 0);
          }
          if (command === 'park' && !onPark) {
            socket.end('This walkthrough is managed by its current command\n');
            return;
          }
          if (name) {
            if (name === 'clickhouse') chGate.set(action === 'start');
            else await manual.dependency(name, action === 'start');
          }
          socket.end(JSON.stringify({ ...identity, command }) + '\n', command === 'park' ? onPark : undefined);
        })
        .catch(() => socket.end('Operation failed; inspect the private walkthrough logs\n'));
    });
  });
  await new Promise((ok, no) => server.once('error', no).listen(path, ok));
  chmodSync(path, 0o600);
  return async () => {
    await new Promise((done) => server.close(done));
    await previous;
  };
}
async function send(fixture, command) {
  const { side, releaseSHA256, sharedConsoleSHA256 } = fixture, launchID = fixture.execution?.launchID;
  assert(['a', 'b'].includes(side));
  assert.match(fixture.environment, /^[a-f0-9]{12}$/);
  assert.match(launchID ?? '', /^[a-f0-9-]{36}$/, 'Open this walkthrough before changing a dependency');
  assert.match(releaseSHA256, /^[a-f0-9]{64}$/);
  if (sharedConsoleSHA256 !== undefined) assert.match(sharedConsoleSHA256, /^[a-f0-9]{64}$/);
  const client = createConnection(socketPath(fixture.environment, side));
  client.setTimeout(20000, () => client.destroy(Error('Local operator timed out')));
  client.setEncoding('utf8');
  client.once('connect', () => client.end([command, launchID, releaseSHA256, ...(sharedConsoleSHA256 ? [sharedConsoleSHA256] : [])].join(' ') + '\n'));
  let reply = '';
  for await (const chunk of client) reply += chunk;
  if (!reply.startsWith('{')) throw Error(reply.trim() || 'Local operator disconnected');
  const receipt = JSON.parse(reply);
  assert.deepEqual(receipt, {
    environment: fixture.environment,
    side,
    releaseSHA256,
    ...(sharedConsoleSHA256 && { sharedConsoleSHA256 }),
    launchID,
    database: fixture.execution.schema.database,
    console: fixture.console,
    runtime: fixture.runtime,
    diagnostics: fixture.diagnostics,
    command,
  });
  return receipt;
}
export function liveWalkthroughStatus(fixture) {
  return send(fixture, 'status');
}
export async function changeDependency(fixture, name, action) {
  assert(['parser', 'clickhouse', ...(fixture.side === 'b' ? ['broker'] : [])].includes(name));
  assert(['stop', 'start'].includes(action));
  await send(fixture, name + ' ' + action);
  return 'OK ' + name + ' ' + action + '\n';
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [directory, side, name, action] = process.argv.slice(2);
  assert(['a', 'b'].includes(side));
  const fixture = read(join(runDirectory(directory), 'manual', 'walkthrough-' + side + '.json'));
  if (['status', 'park'].includes(name) && !action) process.stdout.write(JSON.stringify(await send(fixture, name)) + '\n');
  else process.stdout.write(await changeDependency(fixture, name, action));
}
