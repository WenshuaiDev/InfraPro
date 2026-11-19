import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, readdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { digest, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { withHTTPPair, startPairedWorkers } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';
import { spatialAssetWriter } from './fixtures/spatial-acceptance.mjs';

const reason = '隔离验收空间版本与项目文件授权';
const wm = (expected_revision) => ({ request_id: randomUUID(), reason, expected_revision });
const sourceBytes = (file) => readFileSync(join(root, 'samples/spatial', file));
const restrictedRoles = [
  'identity-api',
  'control-api',
  'monitoring-query-api',
  'transfer-api',
  'spatial-worker',
];

function spatialSQL(dir, side, sql) {
  const lab = read(join(dir, 'private/lab.json')),
    cfg = lab[side].config,
    db = 'arc_http_' + lab.id + '_' + side;
  assert.match(lab.id, /^[a-f0-9]{12}$/);
  assert.equal(new URL(read(cfg).database_url).pathname, '/' + db);
  return execFileSync(
    'docker',
    [
      'exec',
      lab.container,
      'psql',
      '-XAtq',
      '-v',
      'ON_ERROR_STOP=1',
      '-U',
      'archtest',
      '-d',
      db,
      '-c',
      sql,
    ],
    { encoding: 'utf8', stdio: 'pipe' },
  ).trim();
}
function storedObjects(seed, dir, side) {
  const cfg = read(join(dir, 'private/lab.json'))[side].config;
  const env = {
    ...Object.fromEntries(
      Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_')),
    ),
    INFRAPRO_CONFIG: cfg,
  };
  return JSON.parse(
    execFileSync(join(seed, 'fixture'), ['list', dir], { env, stdio: 'pipe', encoding: 'utf8' }),
  ).filter((x) => x.key.startsWith('spatial/'));
}
function spatialFacts(seed, dir, side, interrupted) {
  const cfg = read(join(dir, 'private/lab.json'))[side].config;
  const query = (sql) => JSON.parse(spatialSQL(dir, side, sql));
  const info = query(`SELECT jsonb_build_object(
    'receipts',(SELECT jsonb_object_agg(operation,n) FROM (SELECT operation,count(*) n FROM spatial_receipts GROUP BY operation) r),
    'audit',(SELECT jsonb_object_agg(event_type||'/'||outcome,n) FROM (SELECT event_type,outcome,count(*) n FROM account_security_events WHERE source_module='spatial' AND phase<>'transfer' GROUP BY event_type,outcome) a),
    'terminalJobs',(SELECT jsonb_object_agg(state,n) FROM (SELECT state,count(*) n FROM spatial_processing_jobs GROUP BY state) j),
    'acceptedUploads',(SELECT count(*) FROM spatial_uploads WHERE state='sealed'),
    'cancelledUploads',(SELECT count(*) FROM spatial_uploads WHERE state='canceled'),
    'badDraftsRemoved',(SELECT count(*) FROM spatial_asset_versions WHERE deleted_at IS NOT NULL),
    'badDraftParts',(SELECT count(*) FROM spatial_upload_parts p JOIN spatial_uploads u ON u.id=p.upload_id JOIN spatial_asset_versions v ON v.id=u.version_id WHERE v.deleted_at IS NOT NULL),
    'invalidReferences',(SELECT count(*) FROM spatial_version_files f WHERE NOT EXISTS(SELECT FROM spatial_blobs b WHERE b.id=f.blob_id AND b.tenant_id=f.tenant_id)),
    'openTransfers',(SELECT count(*) FROM audit_transfer_receipts WHERE source_module='spatial' AND closed_at IS NULL)
  )`);
  assert.equal(info.openTransfers, 0);
  assert.equal(info.badDraftsRemoved, 1);
  assert.equal(info.badDraftParts, 0);
  assert.equal(info.invalidReferences, 0);
  assert.equal(info.cancelledUploads, 1);
  assert.equal(info.acceptedUploads, 5);
  assert.equal(info.terminalJobs.failed, 1);
  assert.equal(info.terminalJobs.succeeded, 4);
  assert.equal(info.receipts['uploads.complete'], 5);
  assert.equal(info.audit['spatial.uploads.complete/success'], 5);
  assert.match(interrupted.id, /^[A-Za-z0-9_-]{8,90}$/);
  const boundary = query(
    `SELECT jsonb_build_object('result',outcome,'sent',sent_bytes,'facts',(SELECT count(*) FROM account_security_events a WHERE a.source_module='spatial' AND a.source_key=t.id::text AND a.phase='transfer')) FROM audit_transfer_receipts t WHERE source_module='spatial' AND request_id='${interrupted.id}'`,
  );
  assert.equal(boundary.facts, 2);
  assert(boundary.sent >= interrupted.first && boundary.sent <= interrupted.total);
  if (boundary.result === 'complete') assert.equal(boundary.sent, interrupted.total);
  else assert(['interrupted', 'error'].includes(boundary.result));
  const objects = storedObjects(seed, dir, side);
  assert(objects.length > 0 && objects.every((v) => !v.key.includes('/temporary/')));
  const references = query(
    "SELECT jsonb_agg(jsonb_build_object('key',object_key,'size',size,'sha256',sha256)) FROM spatial_blobs",
  );
  const byKey = new Map(objects.map((v) => [v.key, v]));
  assert.equal(objects.length, references.length);
  for (const ref of references) assert.deepEqual(byKey.get(ref.key), ref);
  assert.equal(
    readdirSync(read(cfg).spatial_temp_directory).filter((v) => v.startsWith('spatial-read-'))
      .length,
    0,
  );
  return {
    effects: info,
    interrupted: boundary,
    objects: references.sort((a, b) => a.key.localeCompare(b.key)),
    immutableBlobs: objects.map(({ size, sha256 }) => size + '/' + sha256).sort(),
  };
}

class SpatialPair extends PairedHTTP {
  constructor(dir, out, snapshot) {
    super(dir, 'spatial', out);
    this.snapshotIDs = new Set(
      JSON.stringify(snapshot).match(/[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}/gi),
    );
  }
  bind(a, b) {
    if (this.snapshotIDs.has(a) || this.snapshotIDs.has(b)) return a === b;
    return super.bind(a, b);
  }
  verify(a, b, op, at) {
    if (!op.operationId.startsWith('Spatial:')) return super.verify(a, b, op, at);
    // Draft associations are publicly sorted by their independently allocated
    // primary keys. Check each original order and compare the same attachment
    // by object, page/element and geometry on both sides.
    function byGeometry(v) {
      const r = v.placement ?? v.component_link ?? v;
      return JSON.stringify([
        r.point_id ?? r.component_id,
        r.page_id,
        r.element_key,
        r.anchor ?? r.region,
      ]);
    }
    function normalize(v) {
      if (Array.isArray(v)) {
        const association =
          v.length > 0 &&
          v.every(
            (x) =>
              x &&
              typeof x === 'object' &&
              ('anchor' in x || 'region' in x) &&
              x.id &&
              (x.point_id || x.component_id),
          );
        const candidates =
          v.length > 0 &&
          v.every((x) => x && typeof x === 'object' && (x.placement || x.component_link));
        if (association || candidates) {
          if (association)
            assert(
              v.every((x, i) => i === 0 || v[i - 1].id < x.id),
              'spatial association UUID order changed',
            );
          assert.equal(new Set(v.map(byGeometry)).size, v.length, 'ambiguous fixture association');
          return [...v].sort((x, y) => byGeometry(x).localeCompare(byGeometry(y))).map(normalize);
        }
        return v.map(normalize);
      }
      if (v && typeof v === 'object' && !Buffer.isBuffer(v))
        return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, normalize(x)]));
      return v;
    }
    function orderedLocations(v) {
      if (!op.operationId.endsWith(':locate') || !Array.isArray(v?.items)) return v;
      // Versions are ordered by publication scope and time. The associations
      // inside each version are ordered by newly allocated placement UUIDs.
      const groups = [];
      for (const row of v.items) {
        const key = JSON.stringify([row.owner.kind, row.owner.id, row.version_id]);
        if (groups.at(-1)?.key !== key) groups.push({ key, items: [] });
        groups.at(-1).items.push(row);
      }
      const coordinate = (row) =>
        JSON.stringify([row.page_id, row.element_key, row.anchor, row.geometry]);
      return {
        ...v,
        items: groups.flatMap((g) =>
          g.items.sort((x, y) => coordinate(x).localeCompare(coordinate(y))),
        ),
      };
    }
    const checked = super.verify(
      normalize(orderedLocations(a)),
      normalize(orderedLocations(b)),
      op,
      at,
    );
    if (op.operationId.endsWith(':summaries') && checked.differences.includes('/refreshed_at')) {
      const now = Date.now();
      if (
        [a.refreshed_at, b.refreshed_at].every(
          (t) =>
            typeof t === 'string' &&
            /^\d{4}-\d\d-\d\dT/.test(t) &&
            Date.parse(t) >= at - 1000 &&
            Date.parse(t) <= now + 1000,
        )
      ) {
        checked.differences = checked.differences.filter((v) => v !== '/refreshed_at');
        checked.mapped.push('/refreshed_at');
      }
    }
    return checked;
  }
  save() {
    const evidence = {
      sourceRun: this.dir,
      scenario: this.name,
      inputs: 'paired public sessions and command IDs',
      records: this.records,
      failures: this.problems,
    };
    write(join(this.output, 'http/scenarios/spatial.json'), evidence);
  }
}

export function spatialParity(seed, output = seed) {
  const workers = [];
  return withHTTPPair(seed, 'spatial', (fixture) => exercise(seed, fixture, output, workers), {
    restrictedRoles,
    configure: ({ side, cfg, privateDir, baseline }) => {
      const folder = join(privateDir, 'spatial');
      mkdirSync(folder, { recursive: true, mode: 0o700 });
      cfg.spatial_python = join(root, '.artifacts/spatial-venv/bin/python');
      cfg.spatial_converter_script = join(
        side === 'a' ? baseline.source : root,
        'tools/spatial-converter/convert.py',
      );
      cfg.spatial_temp_directory = folder;
      assert.ok(
        existsSync(cfg.spatial_python) && existsSync(cfg.spatial_converter_script),
        'isolated spatial converter required',
      );
      return {
        start: (opts) =>
          startPairedWorkers({
            ...opts,
            side,
            privateDir,
            legacy: ['spatial-worker'],
            split: ['spatial-worker'],
            claimRoles: [],
            controls: workers,
          }),
      };
    },
  });
}

async function exercise(seed, dir, out, workers) {
  const sample = read(join(dir, 'samples.json'));
  const pair = new SpatialPair(dir, out, sample);
  const { structure, secondTenant } = sample;
  const { nodes, channels } = structure;
  const state = { assets: {} },
    admin = await pair.login(read(join(dir, 'private/accounts.json')).admin);
  const { call, client, headers, api } = admin,
    base = '/tenants/' + structure.tenant,
    spatial = base + '/spatial';
  const project = base + '/projects/' + structure.project,
    projectSpatial = project + '/spatial';
  const facility = nodes.A.id,
    point = nodes.P1.id,
    component = nodes.C1;
  const prior = {
    tenant: structure.tenant,
    facility,
    point,
    second_point: nodes.P2.id,
    channels: { A: channels.P1 },
  };
  const wait = (path, pick, want) => client.wait(api + path, headers, pick, want);
  const proof = [],
    stages = [],
    interrupted = [];
  const pipeline = async (name, arcs, fn) => {
    const first = pair.records.length;
    try {
      await fn();
    } catch (e) {
      pair.problems.push({ name, error: e.message.split('\n')[0].slice(0, 260) });
      // Every subsequent case starts from the same four published versions.
      // Its independent transfer and commit checks still run if a read fails.
      if (!stages.length) throw e;
    } finally {
      const records = pair.records.slice(first);
      for (const arc of arcs)
        stages.push({
          arc,
          name,
          passed:
            records.length > 0 &&
            !pair.problems.some((p) => p.name === name) &&
            records.every((v) => !v.differences.length),
          caseIds: records.map((r) => r.command),
          checks: proof.filter((p) => p.stage === name),
        });
      pair.save();
      write(join(out, 'http/workflows/spatial.json'), { sourceRun: dir, cases: stages });
    }
  };
  const bytes = async (path, { requestHeaders = {}, status = 200, expected, kind } = {}) => {
    const r = await client.get(api + path, { headers: { ...headers, ...requestHeaders } });
    try {
      assert.equal(r.status(), status);
      const b = await r.body(),
        h = r.headers();
      if (expected) assert.ok(b.equals(expected), 'read bytes disagree with immutable source');
      if (kind) assert.match(h['content-type'], kind);
      if ([200, 206].includes(status)) assert.match(h['cache-control'], /no-store/);
      return { body: b, headers: h };
    } finally {
      await r.dispose();
    }
  };
  const denied = async (path, status = 404) => {
    const r = await client.get(api + path, { headers });
    try {
      assert.equal(r.status(), status);
      assert.equal(r.headers()['content-range'], undefined);
    } finally {
      await r.dispose();
    }
  };
  let firstProcessing = true;
  const builder = spatialAssetWriter({
    state,
    prior,
    component,
    slope: nodes.B,
    spatial,
    call,
    wait,
    wm,
    client,
    api,
    headers,
    origin: pair.origin,
    save: () => {},
    mainChannels: { ['point:' + point]: channels.P1.id },
    processVersion: async (vid, v) => {
      const path = spatial + '/versions/' + vid + '/process';
      if (!firstProcessing) return call(path, wm(v.revision));
      firstProcessing = false;
      assert.equal(workers.length, 2);
      await Promise.all(workers.map((worker) => worker.stop()));
      try {
        const job = await call(path, wm(v.revision)),
          j = spatial + '/jobs/' + job.id;
        let curr = await call(j);
        assert.equal(curr.state, 'queued');
        await call(j + '/cancel', wm(curr.revision));
        curr = await call(j);
        assert.equal(curr.state, 'canceled');
        await call(j + '/retry', wm(curr.revision));
        curr = await call(j);
        assert.equal(curr.state, 'queued');
        return job;
      } finally {
        await Promise.all(workers.map((worker) => worker.start()));
      }
    },
    beforePublish: (vid, v) => call(spatial + '/versions/' + vid + '/validate', wm(v.revision)),
  });
  try {
    await pipeline('图纸、GLB、GIS 的真实转换、挂接和发布', ['ARC-012', 'ARC-013'], async () => {
      for (const [key, kind, files] of [
        ['layout', 'layout', ['bridge-layouts-v1.pdf', 'bridge-layouts-v2.pdf']],
        ['model', 'model', ['bridge-v1.glb']],
        ['region', 'geolayer', ['reference.geojson']],
      ]) {
        const fid = kind === 'geolayer' ? undefined : facility;
        await builder.createAsset(key, kind, fid, '架构配对 ' + kind);
        let priorVersion;
        for (const f of files)
          priorVersion = await builder.publish(key, f, fid, fid && point, priorVersion);
      }
    });
    const layout = state.assets.layout,
      model = state.assets.model,
      region = state.assets.region;
    const first = layout.versions[0],
      second = layout.versions[1],
      m = model.versions[0],
      g = region.versions[0];
    await pipeline(
      '按冻结项目授予固定版本并独立控制原件下载',
      ['ARC-012', 'ARC-013', 'ARC-030'],
      async () => {
        const acl = project + '/spatial-access',
          curr = await call(acl);
        const change = {
          ...wm(curr.revision),
          add: [first, m, g].map((v) => ({
            version_id: v.id,
            source_download: v !== first,
            is_default: true,
          })),
          end: [],
        };
        const preview = await call(acl + '/preview', change);
        await call(acl, { ...change, preview_hash: preview.preview_hash }, 'PUT');
        const actual = await call(acl);
        assert(
          actual.grants.some((entry) => entry.version_id === first.id && !entry.source_download),
        );
        await denied(projectSpatial + '/versions/' + first.id + '/source');
        await denied(projectSpatial + '/versions/' + second.id + '/manifest');
        const viewPath = projectSpatial + '/views/' + first.id,
          v = await call(viewPath);
        await call(
          viewPath,
          {
            ...wm(v.revision),
            version_id: first.id,
            settings: { ...v.settings, basemap: 'local' },
          },
          'PUT',
        );
        assert.equal((await call(viewPath)).settings.basemap, 'local');
      },
    );
    await pipeline(
      '两种作用域的清单、模型属性、实际原件和成果字节',
      ['ARC-012', 'ARC-013'],
      async () => {
        for (const [entry, scope] of [
          ['tenant', spatial],
          ['project', projectSpatial],
        ]) {
          assert((await call(scope + '/assets')).items.some((v) => v.id === layout.id));
          await call(scope + '/assets/' + layout.id);
          await call(scope + '/assets/' + layout.id + '/versions');
          await call(scope + '/basemaps');
          await bytes(scope + '/basemaps/local/tiles/0-0-0.png', { kind: /image\/png/ });
          await call(scope + '/objects?facility_id=' + facility);
          assert.equal((await call(scope + '/objects/' + point + '?kind=point')).id, point);
          const o = { kind: 'point', id: point };
          const loc = await call(scope + '/locate', { objects: [o] });
          assert(loc.items.length > 0);
          await call(scope + '/summaries', {
            objects: [o],
            main_channels: { ['point:' + point]: channels.P1.id },
          });
          await call(scope + '/related-artifacts', { owner: o, descendants: false, limit: 10 });
          const manifest = await call(scope + '/versions/' + first.id + '/manifest');
          const f = manifest.manifest.files[0],
            vpath = scope + '/versions/' + first.id;
          assert(f?.id);
          assert.equal(manifest.manifest.source_sha256, digest(sourceBytes(first.file)));
          assert((await call(vpath + '/pages')).items.length >= 2);
          assert((await call(vpath + '/placements')).items.some((v) => v.point_id === point));
          assert(
            (await call(vpath + '/component-links')).items.some(
              (v) => v.component_id === component.id,
            ),
          );
          const body = await bytes(vpath + '/files/' + f.id);
          assert.equal(digest(body.body), f.sha256);
          assert.equal(body.headers.etag, '"' + f.sha256 + '"');
          await bytes(vpath + '/files/' + f.id, {
            requestHeaders: { 'If-None-Match': body.headers.etag },
            expected: body.body,
          });
          await bytes(vpath + '/files/' + f.id, {
            requestHeaders: { Range: 'bytes=3-22' },
            status: 206,
            expected: body.body.subarray(3, 23),
          });
          await bytes(vpath + '/files/' + f.id, {
            requestHeaders: { Range: 'bytes=0-1,5-9' },
            status: 416,
          });
          const modelPath = scope + '/versions/' + m.id;
          assert((await call(modelPath + '/elements')).items.length);
          assert((await call(modelPath + '/properties')).items.length);
          assert((await call(scope + '/versions/' + g.id + '/features')).items.length);
          const original = sourceBytes(m.file),
            download = await bytes(modelPath + '/source', { expected: original });
          assert.match(download.headers['content-disposition'], /^attachment/);
          await bytes(modelPath + '/source', {
            requestHeaders: { Range: 'bytes=-25' },
            status: 206,
            expected: original.subarray(-25),
          });
          proof.push({
            stage: '两种作用域的清单、模型属性、实际原件和成果字节',
            entry,
            source_sha256: digest(original),
            artifact_sha256: f.sha256,
            pages: manifest.manifest.pages.length,
          });
        }
        await denied('/tenants/' + secondTenant + '/spatial/versions/' + first.id + '/manifest');
      },
    );
    await pipeline('空间原件中途断开再按偏移续读', ['ARC-013', 'ARC-031'], async () => {
      const bytesExpected = sourceBytes(first.file),
        path = '/api/v1' + spatial + '/versions/' + first.id + '/source';
      const offsets = await Promise.all(
        pair.sides.map(async (side, i) => {
          const { cookies } = await pair.contexts[i].storageState();
          const abort = new AbortController(),
            endpoint = side.api + (i ? pair.translate(path) : path);
          const response = await fetch(endpoint, {
            headers: { cookie: cookies.map((c) => c.name + '=' + c.value).join('; ') },
            signal: abort.signal,
          });
          const reader = response.body.getReader();
          try {
            assert.equal(response.status, 200);
            assert.equal(Number(response.headers.get('content-length')), bytesExpected.length);
            const part = await reader.read();
            assert(!part.done && part.value.length > 0 && part.value.length < bytesExpected.length);
            assert.deepEqual(Buffer.from(part.value), bytesExpected.subarray(0, part.value.length));
            interrupted[i] = {
              id: response.headers.get('x-request-id'),
              first: part.value.length,
              total: bytesExpected.length,
            };
            return part.value.length;
          } finally {
            abort.abort();
            await reader.cancel().catch(() => {});
          }
        }),
      );
      for (const offset of new Set(offsets)) {
        const r = await bytes(path.slice(7), {
          requestHeaders: { Range: 'bytes=' + offset + '-' },
          status: 206,
          expected: bytesExpected.subarray(offset),
        });
        assert.equal(
          r.headers['content-range'],
          'bytes ' + offset + '-' + (bytesExpected.length - 1) + '/' + bytesExpected.length,
        );
      }
      proof.push({
        stage: '空间原件中途断开再按偏移续读',
        firstBodyBytes: offsets,
        wholeBytes: bytesExpected.length,
        sourceSHA256: digest(bytesExpected),
      });
    });
    await pipeline(
      '原件上传取消、转换失败及草稿移除',
      ['ARC-012', 'ARC-013', 'ARC-031'],
      async () => {
        const asset = await call(spatial + '/assets/' + layout.id);
        const d = await call(spatial + '/assets/' + layout.id + '/versions', {
          ...wm(asset.revision),
          new_source: true,
          change_note: '错误输入处理',
        });
        const id = d.version_id || d.id,
          path = spatial + '/versions/' + id;
        const original = sourceBytes('invalid/broken.pdf'),
          hash = digest(original);
        const input = async () => ({
          ...wm((await call(path + '/manifest')).revision),
          version_id: id,
          filename: 'broken.pdf',
          size: original.length,
          sha256: hash,
        });
        const over = await client.post(api + spatial + '/uploads', {
          headers,
          data: { ...(await input()), size: Number.MAX_SAFE_INTEGER },
        });
        try {
          assert.equal(over.status(), 422);
          assert((await over.json()).issues.some((x) => x.path === '/file'));
        } finally {
          await over.dispose();
        }
        const upload = async () => call(spatial + '/uploads', await input());
        let u = await upload(),
          up = spatial + '/uploads/' + u.id;
        const part = async () => {
          const r = await client.put(api + up + '/parts/1', {
            headers: {
              ...headers,
              'Content-Type': 'application/octet-stream',
              'X-Content-SHA256': hash,
            },
            data: original,
          });
          try {
            assert.equal(r.status(), 200);
            return await r.json();
          } finally {
            await r.dispose();
          }
        };
        assert((await part()).changed);
        await call(up + '/cancel', wm((await call(up)).revision));
        u = await upload();
        up = spatial + '/uploads/' + u.id;
        // Pause at the deferred commit so a separate reader can inspect the
        // physical S3 object and the still-uncommitted PostgreSQL receipt.
        const sides = ['a', 'b'],
          ids = [u.id, pair.translate(u.id)],
          installed = [],
          pendingObjects = [],
          visibleParts = [],
          orphanTemporaryObjects = [];
        const countParts = (side, upload) =>
          Number(spatialSQL(dir, side, `SELECT count(*) FROM spatial_upload_parts WHERE upload_id='${upload}'`));
        const temporaryParts = (side, upload) =>
          storedObjects(seed, dir, side).filter((o) => o.key.includes('/temporary/' + upload + '/'));
        try {
          for (const [i, side] of sides.entries()) {
            assert.match(ids[i], /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/);
            spatialSQL(
              dir,
              side,
              `CREATE SEQUENCE arc_spatial_at_commit; CREATE SEQUENCE arc_spatial_release;
            CREATE FUNCTION arc_spatial_reject_part() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
            BEGIN
              PERFORM nextval('public.arc_spatial_at_commit');
              FOR i IN 1..200 LOOP
                EXIT WHEN (SELECT is_called FROM public.arc_spatial_release);
                PERFORM pg_sleep(0.05);
              END LOOP;
              RAISE EXCEPTION 'isolated spatial receipt failure';
            END $$;
            CREATE CONSTRAINT TRIGGER arc_spatial_reject_part AFTER INSERT ON spatial_upload_parts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.upload_id='${ids[i]}') EXECUTE FUNCTION arc_spatial_reject_part()`,
            );
            installed.push(side);
          }
          const failure = client.put(api + up + '/parts/1', {
            headers: {
              ...headers,
              'Content-Type': 'application/octet-stream',
              'X-Content-SHA256': hash,
            },
            data: original,
          });
          // The request is deliberately in flight until the observer opens
          // both gates. Always drain it, including when an assertion fails.
          void failure.catch(() => {});
          try {
            for (const [i, side] of sides.entries()) {
              const end = Date.now() + 7000;
              let atCommit;
              do {
                atCommit = spatialSQL(dir, side, 'SELECT is_called FROM arc_spatial_at_commit');
                if (atCommit === 't') break;
                await sleep(50);
              } while (Date.now() < end);
              assert.equal(atCommit, 't', 'upload must reach the deferred commit');
              const objects = temporaryParts(side, ids[i]);
              assert.equal(objects.length, 1);
              assert.equal(objects[0].sha256, hash);
              assert.equal(objects[0].size, original.length);
              const receipts = countParts(side, ids[i]);
              assert.equal(receipts, 0);
              pendingObjects.push({
                side,
                upload: ids[i],
                key: objects[0].key,
                sha256: objects[0].sha256,
                size: objects[0].size,
                receipts,
              });
            }
          } finally {
            for (const side of sides) spatialSQL(dir, side, "SELECT nextval('arc_spatial_release')");
            const refused = await failure;
            try {
              assert.equal(refused.status(), 503);
              assert.equal((await refused.json()).code, 'unavailable');
            } finally {
              await refused.dispose();
            }
          }
          for (const [i, side] of sides.entries()) {
            visibleParts.push(countParts(side, ids[i]));
            orphanTemporaryObjects.push(temporaryParts(side, ids[i]).length);
          }
          assert.deepEqual(visibleParts, [0, 0]);
          assert.deepEqual(orphanTemporaryObjects, [0, 0]);
        } finally {
          for (const side of installed)
            spatialSQL(
              dir,
              side,
              'DROP TRIGGER arc_spatial_reject_part ON spatial_upload_parts; DROP FUNCTION arc_spatial_reject_part(); DROP SEQUENCE arc_spatial_at_commit, arc_spatial_release',
            );
        }
        const originalPart = await part(),
          duplicatePart = await part();
        assert(originalPart.changed && duplicatePart.replayed);
        assert.equal(originalPart.revision, duplicatePart.revision);
        await call(up + '/complete', wm((await call(up)).revision));
        const job = await call(path + '/process', wm((await call(path + '/manifest')).revision));
        const f = await wait(spatial + '/jobs/' + job.id, (v) => v.state, 'failed');
        assert(f.error_code);
        proof.push({
          stage: '原件上传取消、转换失败及草稿移除',
          reason: f.error_code,
          failed_state: f.state,
          failedCommit: {
            afterObjectWrite: pendingObjects.length === sides.length,
            pendingObjects,
            visibleParts,
            orphanTemporaryObjects,
            retryAndReplay: originalPart.changed && duplicatePart.replayed,
          },
        });
        await call(path + '/draft', wm((await call(path + '/manifest')).revision), 'DELETE');
      },
    );
    await pipeline(
      '修改资料、切回历史版、撤回项目可见版并恢复资料',
      ['ARC-012', 'ARC-030'],
      async () => {
        const asset = spatial + '/assets/' + layout.id;
        let before = await call(asset);
        await call(
          asset,
          {
            ...wm(before.revision),
            kind: before.kind,
            code: before.code,
            facility_id: facility,
            name: '架构审定图纸',
            category: before.category,
            source_note: before.source_note,
          },
          'PATCH',
        );
        const control = async (path, action, revision, extra = {}) => {
          const input = { ...wm(revision), ...extra },
            preview = await call(path + '/' + action + '-preview', input);
          return call(path + '/' + action, { ...input, preview_hash: preview.preview_hash });
        };
        before = await call(asset);
        await control(asset, 'current', before.revision, { version_id: first.id });
        assert.equal((await call(asset)).current_version_id, first.id);
        const vpath = spatial + '/versions/' + first.id,
          v = await call(vpath + '/manifest');
        await control(vpath, 'withdraw', v.revision, { replacement_version_id: second.id });
        assert.equal((await call(asset)).current_version_id, second.id);
        await denied(projectSpatial + '/versions/' + first.id + '/manifest');
        await control(asset, 'retire', (await call(asset)).revision);
        assert.equal((await call(asset)).status, 'retired');
        await call(asset + '/restore', wm((await call(asset)).revision));
        assert.equal((await call(asset)).status, 'normal');
        await call(asset + '/history');
      },
    );
    await pipeline(
      '逐键核对空间收据、审计、对象与清理',
      ['ARC-012', 'ARC-013', 'ARC-031'],
      async () => {
        await call(spatial + '/assets/' + layout.id + '/history');
        const [a, b] = ['a', 'b'].map((side, i) => spatialFacts(seed, dir, side, interrupted[i]));
        assert.deepEqual(a.effects, b.effects);
        assert.deepEqual(a.immutableBlobs, b.immutableBlobs);
        proof.push({
          stage: '逐键核对空间收据、审计、对象与清理',
          effects: a.effects,
          sourceAndArtifactCount: a.immutableBlobs.length,
          sourceAndArtifactSetSHA256: digest(Buffer.from(a.immutableBlobs.join('\n'))),
          verifiedObjectReferences: { a: a.objects, b: b.objects },
          interrupted: [a.interrupted, b.interrupted],
        });
      },
    );
  } finally {
    await pair.close();
  }
  const records = pair.records,
    routes = readFileSync(join(root, 'services/api/internal/routes/ownership.tsv'), 'utf8')
      .trim()
      .split('\n')
      .map((l) => l.split('\t'))
      .filter((r) => r[3]?.startsWith('Spatial:'));
  const positive = new Set(
    records
      .filter((r) => r.status.every((s) => s >= 200 && s < 300) && !r.differences.length)
      .map((r) => r.operationId),
  );
  const missing = routes.filter((r) => !positive.has(r[3])).map((r) => r[3]);
  const { candidateSHA256, appliedRoleSQL } = read(join(dir, 'snapshot.json'));
  const report = {
    sourceRun: dir,
    baseline: read(join(dir, 'baseline.json')).commit,
    candidateSHA256,
    appliedRoleSQL,
    restrictedRoles,
    passed: !missing.length && stages.every((s) => s.passed),
    total: routes.length,
    successful: routes.length - missing.length,
    missing,
    proof,
  };
  write(join(out, 'http/spatial-parity.json'), report);
  write(join(dir, 'spatial-scenario.json'), read(join(out, 'http/scenarios/spatial.json')));
  write(join(dir, 'spatial-workflow.json'), read(join(out, 'http/workflows/spatial.json')));
  write(join(dir, 'spatial-parity.json'), report);
  assert(
    report.passed,
    JSON.stringify({ missing, failed: stages.filter((s) => !s.passed).map((s) => s.name) }),
  );
  return report;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const seed = runDirectory(process.argv[2]);
  try {
    const r = await spatialParity(seed, runDirectory(process.argv[3] || seed));
    console.log('Spatial matched method/templates', r.successful);
  } catch (e) {
    console.error(
      'Spatial parity failed',
      String(e.message)
        .split('\n')[0]
        .replace(/https?:\/\/\S+/g, '[loopback endpoint]')
        .slice(0, 400),
    );
    process.exitCode = 1;
  }
}
