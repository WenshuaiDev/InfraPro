// Seed and re-open only the named synthetic manual-acceptance resources.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import {
  client,
  digest,
  launch,
  read,
  root,
  runDirectory,
  sleep,
  write,
} from './architecture-lab.mjs';
import { stop } from './fixtures/parsing-lab.mjs';
import { roleEnvironment } from './architecture-runtime.mjs';
import { configEnvironment } from './architecture-walkthrough-runtime.mjs';
import { spatialAssetWriter } from './fixtures/spatial-acceptance.mjs';

const reason = '架构改造人工验收样本';

// The published grant editor initializes all four object lists. Preserve the
// existing scope when reopening a fixture created by an earlier seed runner.
export async function walkthroughDisplayGrant(call, path, facility) {
  const before = await call(path);
  const grant = before.grant;
  assert.deepEqual(grant.capabilities, ['resources.read']);
  assert.deepEqual(grant.scope.resources, [facility]);
  assert.equal(grant.revoked, false);
  const scope = { ...grant.scope };
  let changed = false;
  for (const key of ['channels', 'spatial', 'artifacts']) {
    if (scope[key] == null) { scope[key] = []; changed = true; }
    assert.deepEqual(scope[key], []);
  }
  if (changed) {
    await call(path + '/grant', {
      request_id: randomUUID(), expected_revision: before.instance.revision, reason,
      grant: {
        scope_format: grant.scope_format, read_scopes: grant.read_scopes,
        capabilities: grant.capabilities, scope, fields: grant.fields, valid_until: grant.valid_until,
      },
    }, 'PUT');
  }
  const after = changed ? await call(path) : before;
  assert.deepEqual(after.grant.scope, scope);
  assert.deepEqual(after.grant.capabilities, grant.capabilities);
  assert.deepEqual(after.grant.fields, grant.fields);
  assert.equal(after.grant.valid_until, grant.valid_until);
  assert.equal(after.instance.current_release_id, before.instance.current_release_id);
  return {
    revision: after.grant.revision, resources: 1, channels: 0, spatial: 0, artifacts: 0,
    ...(changed ? {
      previousRevision: grant.revision,
      previousListSizes: Object.fromEntries(['resources','channels','spatial','artifacts'].map(name => [name, grant.scope[name]?.length ?? null])),
    } : {}),
  };
}

async function until(label, readState, pick, expected) {
  for (let i = 0; i < 350; i++) {
    const x = await readState(),
      state = pick(x);
    if (state === expected) return x;
    if (['failed', 'canceled', 'cancelled', 'needs_action'].includes(state))
      throw Error(label + ' ' + state + ' ' + (x.error_code ?? ''));
    await sleep(230);
  }
  throw Error(label + ' timed out');
}
function key(namespace, label) {
  const id = createHash('sha256')
    .update(namespace + ' / ' + label)
    .digest('hex')
    .slice(0, 32);
  return (
    id.slice(0, 8) +
    '-' +
    id.slice(8, 12) +
    '-4' +
    id.slice(13, 16) +
    '-a' +
    id.slice(17, 20) +
    '-' +
    id.slice(20)
  );
}
function domainWorkers(directory, side, lab, priv) {
  const base = read(join(directory, 'baseline.json')),
    release = process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE || directory,
    bin = side === 'b' ? read(join(runDirectory(release), 'candidate.json')).bin : base.bin,
    cfg = read(lab.config);
  const scratch = mkdtempSync(join(tmpdir(), 'arc-accept-')),
    spatial = join(priv, 'spatial');
  mkdirSync(spatial, { recursive: true, mode: 0o700 });
  cfg.files = {
    ...cfg.files,
    temp_directory: scratch,
    checker_socket: join(scratch, 'checker.sock'),
  };
  cfg.spatial_python = join(root, '.artifacts', 'spatial-venv', 'bin', 'python');
  cfg.spatial_converter_script = join(
    side === 'b' ? root : base.source,
    'tools/spatial-converter/convert.py',
  );
  cfg.spatial_temp_directory = spatial;
  assert(
    existsSync(cfg.spatial_python),
    'prepare the local spatial converter before seeding assets',
  );
  const file = join(priv, 'workers.json');
  write(file, cfg);
  const env = {
    ...Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_'))),
    INFRAPRO_CONFIG: file,
    GOMAXPROCS: '2',
  };
  const roles = [
    'files-checker',
    'files-worker',
    'display-package-worker',
    'spatial-worker',
  ];
  for (const role of roles) assert(existsSync(join(bin, role)), role + ' binary missing');
  const children = [];
  const topology = join(directory, 'topology');
  const projected = side === 'b' ? configEnvironment(cfg) : undefined;
  const environments = {};
  for (const role of roles) {
    if (side !== 'b') { environments[role] = env; continue; }
    const roleEnv = roleEnvironment(role, {...projected, GOMAXPROCS: '2'}, topology);
    if (role === 'files-checker') {
      assert.equal(roleEnv.INFRAPRO_DATABASE_URL, undefined);
    } else {
      const issued = read(join(topology, 'roles', role, 'environment.json')).INFRAPRO_DATABASE_URL;
      assert.equal(roleEnv.INFRAPRO_DATABASE_URL, issued);
      assert.notEqual(new URL(issued).username, new URL(cfg.database_url).username);
    }
    environments[role] = roleEnv;
  }
  for (const role of roles) {
    const extra =
      role === 'files-checker'
        ? ['--directory', scratch, '--socket', cfg.files.checker_socket]
        : [];
    children.push(launch(join(bin, role), extra, environments[role], join(priv, role + '.log')));
  }
  return children;
}
export async function seedWalkthroughFilesDisplay({
  directory,
  side,
  lab,
  structure,
  account,
  workers,
  verifyTerminal = false,
}) {
  assert(['a', 'self', 'b'].includes(side));
  const tenant = structure.tenant,
    project = structure.project,
    fac = structure.nodes.A.id,
    point = structure.nodes.P1.id;
  for (const k of ['origin', 'api', 'runtime', 'assets'])
    assert.equal(new URL(lab[k]).hostname, '127.0.0.1', 'local fixture required');
  const db = new URL(read(lab.config).database_url);
  assert.equal(db.hostname, '127.0.0.1');
  assert.match(db.pathname, /^\/arc_/);
  const priv = join(directory, 'private', side, 'walkthrough-assets');
  mkdirSync(priv, { recursive: true, mode: 0o700 });
  const manifest = join(priv, 'manifest.json'),
    state = existsSync(manifest)
      ? read(manifest)
      : { tenant, project, assets: {}, display: {}, files: {}, spatial: {} };
  assert.equal(state.tenant, tenant);
  assert.equal(state.project, project);
  const save = () => write(manifest, state),
    id = (label) => key(tenant + '/' + project, label);
  const meta = (rev = '0') => ({
    request_id: randomUUID(),
    expected_revision: rev,
    reason,
  });
  const t = '/tenants/' + tenant,
    p = t + '/projects/' + project,
    ui = '/t/' + tenant + '/p/' + project,
    fp = p + '/files',
    sp = t + '/spatial',
    dp = p + '/displays';
  const person = await client(lab, account),
    { call, api, headers } = person;
  let own = [];
  const get = async (path, data, extra = {}, method = 'POST') => {
    const res = await person.client.fetch(api + path, {
      method,
      headers: { ...headers, ...extra, 'X-Command-ID': randomUUID() },
      data,
    });
    try {
      if (!res.ok()) {
        const e = await res.json().catch(() => ({}));
        throw Error(
          method +
            ' ' +
            path.replace(/[0-9a-f-]{36}/g, ':id') +
            ' ' +
            res.status() +
            ' ' +
            (e.code || ''),
        );
      }
      return await res.json();
    } finally {
      await res.dispose();
    }
  };
  const bytes = async (path, sha) => {
    const r = await person.client.get(api + path, { headers });
    try {
      assert.equal(r.status(), 200);
      assert.equal(digest(await r.body()), sha);
    } finally {
      await r.dispose();
    }
  };
  try {
    if (workers !== 'external' && workers !== false) own = domainWorkers(directory, side, lab, priv);
    const cap = await call(fp + '/capabilities'),
      fh = { 'X-Files-Epoch': cap.restore_epoch };
    const makeFile = async (tag, data, filename, label, type, hold = false) => {
      const storage = (state.files[tag] ??= {});
      const create = {
        request_id: id(tag + ' open ' + (storage.cycle || 0)),
        reason,
        host: cap.host,
        intent: 'add',
        filename,
        display_name: label,
        description: reason,
        size_bytes: String(data.length),
        sha256: digest(data),
        declared_media_type: type,
      };
      let current =
        storage.upload && (await call(fp + '/uploads/' + storage.upload).catch(() => null));
      if (
        hold &&
        current &&
        (current.state !== 'receiving' || Date.parse(current.expires_at) <= Date.now())
      ) {
        storage.cycle = (storage.cycle || 0) + 1;
        delete storage.upload;
        create.request_id = id(tag + ' open ' + storage.cycle);
        save();
      }
      if (!storage.upload) {
        const uploaded = await get(fp + '/uploads', create, fh);
        storage.upload = uploaded.upload_id;
        storage.revision = uploaded.revision;
        save();
      }
      const path = fp + '/uploads/' + storage.upload;
      current = await call(path);
      const part = async (n, b) =>
        get(
          path + '/parts/' + n,
          b,
          {
            ...fh,
            'Content-Type': 'application/octet-stream',
            'X-Content-SHA256': digest(b),
          },
          'PUT',
        );
      if (current.state === 'receiving') {
        const partSize = 8 * 1024 * 1024;
        await part(1, data.subarray(0, partSize));
        if (hold) {
          current = await call(path);
          assert.equal(current.parts.length, 1);
          assert.equal(current.parts[0].part, 1);
          assert.equal(current.parts[0].sha256, digest(data.subarray(0, partSize)));
          storage.nextPart = 2;
          storage.sourceSHA256 = digest(data);
          storage.secondPartSHA256 = digest(data.subarray(partSize));
          storage.size = data.length;
          storage.resumeFile = join(priv, filename);
          writeFileSync(storage.resumeFile, data, { mode: 0o600 });
          storage.openPath = ui + '/files?tab=uploads&upload=' + storage.upload;
          storage.expiresAt = current.idle_expires_at ?? current.expires_at;
          save();
          return;
        }
        const accepted = await get(
          path + '/complete',
          {
            request_id: id(tag + ' complete ' + (storage.cycle || 0)),
            expected_revision: storage.revision,
            reason,
          },
          fh,
        );
        storage.job = accepted.job_id;
        save();
      }
      assert(storage.job);
      const job = await until(
        'file check',
        () => call(fp + '/jobs/' + storage.job),
        (x) => x.state,
        'succeeded',
      );
      storage.attachment = job.result_attachment_id;
      storage.revisionID = job.result_revision_id;
      storage.sourceSHA256 = digest(data);
      storage.openPath =
        ui + '/files?attachment=' + storage.attachment + '&revision=' + storage.revisionID;
      await bytes(
        fp + '/attachments/' + storage.attachment + '/revisions/' + storage.revisionID + '/content',
        storage.sourceSHA256,
      );
      save();
    };
    await makeFile(
      'readable',
      readFileSync(join(root, 'samples/spatial/bridge-section.png')),
      'bridge-section.png',
      '验收资料 · 桥梁断面',
      'image/png',
    );
    const pending = Buffer.alloc(8 * 1024 * 1024 + 359, 0x61);
    Buffer.from('InfraPro architecture synthetic manual acceptance\n').copy(pending);
    await makeFile(
      'receiving',
      pending,
      'walkthrough-resume.txt',
      '验收资料 · 待续传说明',
      'text/plain',
      true,
    );
    const prior = {
      tenant,
      facility: fac,
      point,
      second_point: structure.nodes.P2.id,
      channels: { A: structure.channels.P1 },
    };
    const build = spatialAssetWriter({
      state,
      prior,
      component: structure.nodes.C1,
      slope: structure.nodes.B,
      spatial: sp,
      call,
      wait: (path, pick, want) => until(path, () => call(path), pick, want),
      wm: meta,
      client: person.client,
      api,
      headers,
      origin: lab.origin,
      save,
      mainChannels: { ['point:' + point]: structure.channels.P1.id },
      beforePublish: (vid, v) => call(sp + '/versions/' + vid + '/validate', meta(v.revision)),
    });
    for (const [tag, name, kind, file] of [
      ['arc-accept-layout', '验收图纸 · 桥梁总体布置', 'layout', 'bridge-layouts-v1.pdf'],
      ['arc-accept-model', '验收模型 · 桥梁构件', 'model', 'bridge-v1.glb'],
    ]) {
      if (!state.assets[tag]) {
        const previous = (await call(sp + '/assets')).items.find((r) => r.code === tag);
        if (previous) {
          state.assets[tag] = { id: previous.id, versions: [], name };
          save();
        } else await build.createAsset(tag, kind, fac, name);
      }
      const asset = await call(sp + '/assets/' + state.assets[tag].id),
        version = asset.current_version_id || (await build.publish(tag, file, fac, point));
      const m = await call(sp + '/versions/' + version + '/manifest');
      const hash = digest(readFileSync(join(root, 'samples/spatial', file)));
      assert.equal(m.manifest.source_sha256, hash);
      if (kind === 'layout') assert(m.manifest.pages.length >= 2);
      else {
        const elements = await call(sp + '/versions/' + version + '/elements');
        assert(elements.items.some((element) => element.fragments.length > 0));
      }
      const f = m.manifest.files[0];
      await bytes(sp + '/versions/' + version + '/files/' + f.id, f.sha256);
      state.spatial[kind] = {
        asset: asset.id,
        version,
        sourceSHA256: hash,
        openPath:
          ui +
          '/spatial?asset=' +
          asset.id +
          '&version=' +
          version +
          '&owner=' +
          point +
          '&owner_kind=point',
      };
      save();
    }
    const a = p + '/spatial-access',
      access = await call(a),
      add = Object.values(state.spatial)
        .filter((v) => !access.grants.some((g) => g.version_id === v.version && !g.ended_at))
        .map((v) => ({
          version_id: v.version,
          source_download: true,
          is_default: true,
        }));
    if (add.length) {
      const q = { ...meta(access.revision), add, end: [] },
        preview = await call(a + '/preview', q);
      await call(a, { ...q, preview_hash: preview.preview_hash }, 'PUT');
    }
    for (const v of Object.values(state.spatial))
      assert.equal(
        (await call(p + '/spatial/versions/' + v.version + '/manifest')).manifest.source_sha256,
        v.sourceSHA256,
      );
    const listing = await call(dp),
      displayName = '验收大屏 · 桥梁工程概览';
    if (!listing.settings.enabled)
      await call(dp + '/settings', { ...meta(listing.settings.revision), enabled: true }, 'PUT');
    const d = state.display;
    if (!d.instance) {
      const known = listing.items.find((x) => x.name === displayName);
      d.instance =
        known?.id ?? (await call(dp + '/instances', { ...meta(), name: displayName })).id;
      save();
    }
    const base = dp + '/instances/' + d.instance,
      detail = () => call(base);
    const command = async (suffix, body = {}, method = 'POST') =>
      call(base + suffix, { ...meta((await detail()).instance.revision), ...body }, method);
    const bundle = join(root, '.artifacts/display/example.zip');
    if (!existsSync(bundle))
      execFileSync(process.execPath, ['scripts/display-external-check.mjs'], {
        cwd: root,
        stdio: 'pipe',
      });
    const zip = readFileSync(bundle),
      hash = digest(zip);
    if (!d.upload) {
      d.upload = (await command('/uploads', { size: zip.length, sha256: hash })).upload_id;
      save();
    }
    const edition = '/uploads/' + d.upload;
    if ((await call(base + edition)).state === 'receiving') {
      await get(
        base + edition + '/parts/0',
        zip,
        {
          'Content-Type': 'application/octet-stream',
          'X-Content-SHA256': hash,
        },
        'PUT',
      );
      const job = await command(edition + '/complete');
      d.job = job.job_id;
      save();
    }
    await until(
      'display package',
      () => call(base + edition),
      (v) => v.jobs[0]?.state,
      'ready',
    );
    const pack =
      (await detail()).packages.find(
        (x) => x.archive_sha256 === hash || (x.state === 'ready' && x.id === d.package),
      ) ?? (await detail()).packages.find((x) => x.state === 'ready');
    assert(pack?.manifest?.manifest_version === 1);
    d.package = pack.id;
    d.archiveSHA256 = hash;
    const expiry = new Date((Math.floor(Date.now() / 86400000) + 31) * 86400000).toISOString();
    if (!d.release) {
      await command(
        '/grant',
        {
          grant: {
            capabilities: ['resources.read'],
            valid_until: expiry,
            scope: { resources: [fac], channels: [], spatial: [], artifacts: [] },
          },
        },
        'PUT',
      );
      if (!d.config) {
        d.config = (
          await command('/configs', {
            package_id: d.package,
            config_format: 1,
            parameters: { title: '桥梁工程概览' },
            bindings: { site: { kind: 'resources', ids: [fac] } },
            read_views: {},
            mode: 'follow',
          })
        ).config_id;
        save();
      }
      const prep = await command('/prepare', { config_id: d.config });
      await command('/publish', {
        config_id: d.config,
        preparation: prep.hash,
      });
      d.release = (await detail()).instance.current_release_id;
      save();
    }
    assert.equal((await detail()).instance.current_release_id, d.release);
    d.grant = await walkthroughDisplayGrant(call, base, fac);
    const sessionFile = join(priv, 'terminal-session.json');
    if (verifyTerminal || !d.terminalSessionFile || !existsSync(sessionFile)) {
      const session = await request.newContext(
          existsSync(sessionFile) ? { storageState: sessionFile } : {},
        ),
        rh = { Origin: lab.runtime, 'X-Display-Protocol': '1' };
      try {
        if (!existsSync(sessionFile)) {
          const term = d.terminal
            ? await command('/terminals/' + d.terminal, { state: 'reset' }, 'PATCH')
            : await command('/terminals', {
                name: '验收固定终端 · 值班室',
                valid_until: expiry,
              });
          d.terminal = term.terminal_id;
          save();
          const response = await session.post(lab.runtime + '/runtime/terminal/pair', {
            headers: rh,
            data: { code: term.pair_code },
          });
          assert.equal(response.status(), 200, 'fixed terminal pairing');
          await response.dispose();
          write(sessionFile, await session.storageState());
        }
        const start = await session.post(lab.runtime + '/runtime/terminal/start', {
          headers: rh,
          data: {},
        });
        assert.equal(start.status(), 200, 'fixed terminal startup');
        // The successful start rotates the device credential.
        write(sessionFile, await session.storageState());
        assert.equal((await start.json()).session.release_id, d.release);
        await start.dispose();
        d.terminalSessionFile = sessionFile;
      } finally {
        await session.dispose();
      }
    }
    d.openPath = ui + '/displays?instance=' + d.instance;
    d.runtimePath = '/display/' + d.instance;
    save();
    return {
      tenant,
      project,
      display: d,
      files: state.files,
      spatial: state.spatial,
      manifest,
    };
  } finally {
    try { await person.client.dispose(); }
    finally { await Promise.all(own.map(stop)); }
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const directory = runDirectory(process.argv[2]),
      side = process.argv[3];
    assert(['a', 'b'].includes(side));
    const lab = read(join(directory, 'private/lab.json'))[side],
      structure = read(join(directory, 'samples.json')).structure,
      account = read(join(directory, 'private/accounts.json')).admin;
    const r = await seedWalkthroughFilesDisplay({
      directory,
      side,
      lab,
      structure,
      account,
      workers: process.argv.includes('--external-workers') ? 'external' : undefined,
    });
    console.log('Prepared walkthrough assets', r.manifest);
  } catch (e) {
    console.error(
      'Walkthrough assets failed',
      e.message
        .split('\n')[0]
        .replace(/https?:\/\/\S+/g, '[loopback endpoint]')
        .slice(0, 320),
    );
    process.exitCode = 1;
  }
}
