import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { request } from '@playwright/test';
import { client, digest, launch, root, sleep, write } from '../architecture-lab.mjs';
import { availablePort, waitURL } from './parsing-lab.mjs';
import { pairedRoleManifest } from '../architecture-http-sandbox.mjs';

export async function configureChangeDisplay(cfg) {
  const runtime = '127.0.0.1:' + (await availablePort());
  const assets = '127.0.0.1:' + (await availablePort());
  Object.assign(cfg.display, {
    runtime_addr: runtime,
    runtime_origin: 'http://' + runtime,
    asset_addr: assets,
    asset_origin: 'http://' + assets,
    lease: '120s',
  });
  return cfg.display.runtime_origin;
}

export async function startChangeDisplay(side, priv, runtime, args) {
  const roles =
    side === 'a'
      ? ['display-runtime', 'display-package-worker']
      : ['display-runtime', 'display-assets', 'display-package-worker'];
  for (const role of roles) {
    const environment = { ...args.env, ...args.roleEnv(role) };
    if (side === 'b') {
      const manifest = join(priv, role + '.json');
      write(manifest, pairedRoleManifest(args.topology, role, args.endpoints));
      environment.INFRAPRO_ARCHITECTURE_FILE = manifest;
    }
    args.children.push(launch(join(args.bin, role), [], environment, join(priv, role + '.log')));
  }
  await waitURL(runtime + '/readyz');
}

// Open a person-bound named view through the published package. Its project
// already has the channel resource and data grants in the frozen S0 backup.
export async function changeDisplayReader(side, runtime, sample, account, commandIDs) {
  const user = await client(side, account);
  const http = await request.newContext({ timeout: 15000, maxRedirects: 0 });
  const close = async () => {
    await Promise.all([user.client.dispose(), http.dispose()]);
  };
  let sequence = 0;
  const meta = (revision = '0') => ({
    request_id: (commandIDs[sequence++] ??= randomUUID()),
    expected_revision: revision,
    reason: '正式发布、更正和撤销后的大屏命名监测查询',
  });
  const runtimeCall = async (path, data) => {
    const reply = await http.post(runtime + path, {
      headers: { Origin: runtime, 'X-Display-Protocol': '1' },
      data,
    });
    try {
      const result = await reply.json();
      if (!reply.ok())
        throw Error(
          'SDK ' +
            path.replace(/[a-f\d-]{36}/g, ':id') +
            ' ' +
            reply.status() +
            ' ' +
            (result.code || ''),
        );
      return result;
    } finally {
      await reply.dispose();
    }
  };
  try {
    const displays = '/tenants/' + sample.tenant + '/projects/' + sample.project + '/displays';
    const settings = (await user.call(displays)).settings;
    await user.call(displays + '/settings', { ...meta(settings.revision), enabled: true }, 'PUT');
    const screen = await user.call(displays + '/instances', {
      ...meta(),
      name: '连续位置验收大屏',
    });
    const path = displays + '/instances/' + screen.id;
    const command = async (suffix, input, method = 'POST') =>
      user.call(
        path + suffix,
        {
          ...meta((await user.call(path)).instance.revision),
          ...input,
        },
        method,
      );
    const bytes = readFileSync(join(root, '.artifacts/display/monitoring-read-example.zip')),
      sha = digest(bytes);
    const upload = await command('/uploads', {
      size: bytes.length,
      sha256: sha,
    });
    const partPath = path + '/uploads/' + upload.upload_id;
    const part = await user.client.put(user.api + partPath + '/parts/0', {
      data: bytes,
      headers: {
        ...user.headers,
        'Content-Type': 'application/octet-stream',
        'X-Content-SHA256': sha,
      },
    });
    try {
      assert(part.ok(), 'display package upload part ' + part.status());
    } finally {
      await part.dispose();
    }
    await command('/uploads/' + upload.upload_id + '/complete', {});
    let pkg;
    const expires = Date.now() + 45000;
    while (!pkg && Date.now() < expires) {
      pkg = (await user.call(path)).packages.find((p) => p.state === 'ready');
      if (!pkg) await sleep(150);
    }
    assert.equal(pkg?.manifest.manifest_version, 2);
    const scope = {
      domain: 'channels',
      selector: { kind: 'ids', ids: [sample.channels.P1.id] },
      field_groups: ['resource.metadata', 'monitoring.values', 'monitoring.quality', 'history'],
    };
    await command(
      '/grant',
      {
        grant: {
          scope_format: 2,
          read_scopes: [{ ...scope, id: 'raw' }],
          capabilities: ['views.read', 'queries.read', 'channels.read', 'monitoring.details'],
          valid_until: new Date(Date.now() + 3600000).toISOString(),
        },
      },
      'PUT',
    );
    const config = await command('/configs', {
      package_id: pkg.id,
      config_format: 2,
      parameters: { title: '连续位置验收大屏' },
      read_views: { project_raw: { ...scope, scope_id: 'raw' } },
      bindings: {},
      mode: 'follow',
    });
    const prepared = await command('/prepare', { config_id: config.config_id });
    await command('/publish', {
      config_id: config.config_id,
      preparation: prepared.hash,
    });
    const verifier = randomBytes(32).toString('hex');
    const owner = await command('/launch', {
      challenge: digest(Buffer.from(verifier)),
      audience: runtime,
      config_id: config.config_id,
    });
    const { session } = await runtimeCall('/runtime/exchange', {
      code: owner.code,
      verifier,
      audience: runtime,
    });
    assert.equal(session.preview, false);
    const sessionPath = '/runtime/' + session.id;
    await runtimeCall(sessionPath + '/control', { action: 'ready' });
    return {
      close,
      async renew() {
        const renewed = await runtimeCall(sessionPath + '/renew', {});
        assert.equal(renewed.id, session.id);
        return { session: renewed.id, lease_until: renewed.lease_until };
      },
      async read(range) {
        const call = (method, params) => runtimeCall(sessionPath + '/query', { method, params });
        let result = await call('monitoring.details', {
          target: { view: 'project_raw' },
          field_groups: ['monitoring.values', 'monitoring.quality'],
          range,
          limit: 20,
        });
        const id = result.query.id,
          end = Date.now() + 90000;
        while (result.state === 'pending' && Date.now() < end) {
          await sleep(Math.min(Math.max(result.retry_after_ms || 100, 100), 500));
          result = await call('queries.get', { query_id: id });
          assert.equal(result.query.id, id);
        }
        assert.equal(result.state, 'ready', 'monitoring.details final state ' + result.state);
        for (const field of ['monitoring.values', 'monitoring.quality'])
          assert(result.included_field_groups.includes(field), 'missing requested SDK group ' + field);
        const rows = result.data.items.map((row) => {
          assert.equal(row.channel_id, sample.channels.P1.id, 'SDK returned another channel');
          assert.equal(row.quality, 'good', 'SDK row quality ' + row.quality);
          assert(Number.isFinite(Number(row.value)), 'SDK reading must be numeric');
          return {
            at: row.observed_at,
            value: Number(row.value),
            quality: row.quality,
          };
        });
        return { query: id, rows };
      },
    };
  } catch (e) {
    await close();
    throw e;
  }
}
