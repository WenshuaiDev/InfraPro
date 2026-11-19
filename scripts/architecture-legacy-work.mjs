// Freeze in-progress work created by the original public API, clone just that
// database, and start the split owners on the same loopback URLs and objects.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { analysisBody } from './architecture-analysis-parity.mjs';
import {
  pairedRoleConfigs,
  pairedRoleManifest,
  startPairedWorkers,
  withHTTPPair,
} from './architecture-http-sandbox.mjs';
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
import { availablePort, startParser, stop, waitURL } from './fixtures/parsing-lab.mjs';

const method = 'example.hongdian.ddp.tcp.monitoring-json',
  version = '1.0.0';
const meta = (expected_revision = '0') => ({
  request_id: randomUUID(),
  expected_revision,
  reason: '旧版本提交后保持相同业务身份',
});
const exec = (bin, args, opts = {}) =>
  execFileSync(bin, args, { cwd: root, stdio: 'pipe', encoding: 'utf8', ...opts });
const clean = Object.fromEntries(
  Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_')),
);
const sourceBytes = readFileSync(join(root, 'samples/parsing/measurement.bin'));
async function wait(label, work) {
  for (let i = 0; i < 450; i++) {
    const x = await work();
    if (x) return x;
    await sleep(200);
  }
  throw Error(label + ' timeout');
}
export async function legacyWork(seed, out = seed) {
  const own = {};
  try {
    return await withHTTPPair(seed, 'old-pending', (dir) => runCold(dir, out, own), {
      configure: async ({ side, cfg, privateDir, baseline }) => {
        if (side === 'b')
          return {
            start: async (args) => {
              own.options = args;
            },
          };
        own.parser = await startParser(join(privateDir, 'parser'), {
          bin: baseline.bin,
          source: baseline.source,
        });
        cfg.parsing = {
          enabled: 'true',
          url: own.parser.url,
          token_file: own.parser.token,
          object_endpoint: cfg.monitoring_object_endpoint,
          object_access_key: cfg.monitoring_object_access_key,
          object_secret_key: cfg.monitoring_object_secret_key,
          bucket: cfg.monitoring_source_bucket,
        };
        own.tmp = mkdtempSync('/tmp/infrapro-old-work-');
        cfg.files = {
          ...cfg.files,
          temp_directory: own.tmp,
          checker_socket: join(own.tmp, 'checker.sock'),
        };
        own.runtime = 'http://127.0.0.1:' + (await availablePort());
        own.assets = 'http://127.0.0.1:' + (await availablePort());
        cfg.display = {
          ...cfg.display,
          runtime_addr: new URL(own.runtime).host,
          asset_addr: new URL(own.assets).host,
          runtime_origin: own.runtime,
          asset_origin: own.assets,
        };
        return {
          start: async (args) => {
            const opts = {
              ...args,
              side,
              privateDir,
              legacy: ['monitoring-worker'],
              split: [],
              claimRoles: [],
            };
            [own.oldMonitoring] = await startPairedWorkers(opts);
          },
        };
      },
    });
  } finally {
    if (own.parser) await own.parser.close();
    if (own.tmp) rmSync(own.tmp, { recursive: true, force: true });
  }
}
async function runCold(dir, out, own) {
  const lab = read(join(dir, 'private/lab.json')),
    s = read(join(dir, 'samples.json')).structure,
    creds = read(join(dir, 'private/accounts.json'));
  const baseline = read(join(dir, 'baseline.json')),
    candidateSHA256 = read(join(dir, 'snapshot.json')).candidateSHA256,
    db = 'arc_http_' + lab.id + '_a',
    next = db + '_upgrade';
  assert.match(db, /^arc_http_[a-f0-9]{12}_a$/);
  assert.equal(new URL(read(lab.a.config).database_url).pathname, '/' + db);
  const sql = (database, q) =>
    exec('docker', [
      'exec',
      lab.container,
      'psql',
      '-XqtA',
      '-U',
      'archtest',
      '-d',
      database,
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      q,
    ]).trim();
  const schema = (database) =>
    Number(sql(database, 'SELECT max(version_id) FROM goose_db_version WHERE is_applied'));
  const captureRuntimeSessions = () => {
    const path = join(dir, 'private/upgrade-runtime-sessions.json');
    try {
      const sessions = sql(
        'postgres',
        `SELECT COALESCE(json_agg(json_build_object('pid',pid,'login',usename) ORDER BY pid),'[]')
         FROM pg_stat_activity WHERE datname='${next}' AND starts_with(usename,'arc_')`,
      );
      write(path, { observedAt: new Date().toISOString(), sessions: JSON.parse(sessions) });
    } catch {
      write(path, { observedAt: new Date().toISOString(), unavailable: true });
    }
  };
  assert.equal(schema(db), 58);
  const old = await client(lab.a, creds.admin),
    terminal = await request.newContext();
  const { call, api, headers } = old;
  const tenant = '/tenants/' + s.tenant,
    project = tenant + '/projects/' + s.project,
    analysis = tenant + '/monitoring/derived/analysis';
  const opened = Date.now(),
    cases = [],
    oldKeys = {};
  let appliedRoleSQL, restrictedRoles;
  function record(name, facts) {
    cases.push({ arc: 'ARC-029', name, passed: true, checks: facts });
  }
  function save() {
    const evidence = {
      sourceRun: dir,
      frozen: baseline.commit,
      candidateSHA256,
      appliedRoleSQL,
      restrictedRoles,
      schemaBefore: 58,
      oldKeys,
      cases,
    };
    write(join(out, 'faults/legacy-work.json'), evidence);
    write(join(dir, 'legacy-work.json'), evidence);
  }
  const post = async (path, data, extra = {}, method = 'POST') => {
    const r = await old.client.fetch(api + path, {
      method,
      headers: { ...headers, ...extra, 'X-Command-ID': randomUUID() },
      data,
    });
    try {
      if (!r.ok()) {
        const v = await r.json().catch(() => ({}));
        throw Error(
          method +
            ' ' +
            path.replace(/[a-f0-9-]{36}/g, ':id') +
            ' ' +
            r.status() +
            ' ' +
            (v.code || ''),
        );
      }
      return await r.json();
    } finally {
      await r.dispose();
    }
  };
  const waitState = (path, fn, state) =>
    wait(path, async () => {
      const r = await call(path),
        actual = fn(r);
      if (actual === state) return r;
      assert(
        !['failed', 'cancelled', 'canceled'].includes(actual),
        'source task failed ' + actual + ' ' + r?.error_code,
      );
    });
  const getBytes = async (path, want) => {
    const r = await old.client.get(api + path, { headers });
    try {
      assert.equal(r.status(), 200);
      assert.deepEqual(await r.body(), want);
    } finally {
      await r.dispose();
    }
  };
  try {
    // A published historical result is already selected before the upgrade.
    const cfg = (
      await call(analysis + '/configs', {
        ...meta(),
        code: 'PREUPGRADE',
        name: '升级前位移',
        body: analysisBody(s),
      })
    ).config;
    const configs = analysis + '/configs/' + cfg.id,
      validation = await call(configs + '/validate', { draft_revision: cfg.draft.revision });
    const cv = (
      await call(configs + '/versions', {
        ...meta(cfg.revision),
        draft_revision: cfg.draft.revision,
        validation_hash: validation.validation_hash,
      })
    ).version;
    const output_range = { from_ns: '1793491200000000000', to_ns: '1793491800000000000' };
    const run = await call(analysis + '/runs', {
      ...meta(),
      config_version_id: cv.id,
      config_hash: cv.config_hash,
      output_range,
    });
    const completed = await waitState(
      tenant + '/monitoring/derived/runs/' + run.id,
      (x) => x.job.state,
      'succeeded',
    );
    const versionID = completed.analysis.artifact_version_id,
      artifact = analysis + '/artifacts/' + completed.analysis.artifact_id;
    const rows = async () =>
      (await call(artifact + '/query', { version_id: versionID, limit: 20 })).items.map((x) =>
        Number(x.value),
      );
    const frozenValues = await rows();
    assert.deepEqual(frozenValues, [0, 2, 4, 6, 8, 10, 12, 14, 16, 18]);
    oldKeys.analysis = {
      run: run.id,
      version: versionID,
      artifact: completed.analysis.artifact_id,
    };
    await stop(own.oldMonitoring);
    const fp = tenant + '/files',
      cap = await call(fp + '/capabilities'),
      fheaders = { 'X-Files-Epoch': cap.restore_epoch };
    const file = Buffer.from('persistent old-generation attachment\n');
    const input = {
      request_id: randomUUID(),
      reason: '升级可恢复附件',
      host: cap.host,
      intent: 'add',
      filename: 'old.txt',
      display_name: '升级前附件',
      description: '合法合成输入',
      size_bytes: String(file.length),
      sha256: digest(file),
      declared_media_type: 'text/plain',
    };
    const upload = await post(fp + '/uploads', input, fheaders),
      filePath = fp + '/uploads/' + upload.upload_id;
    await post(
      filePath + '/parts/1',
      file,
      { ...fheaders, 'Content-Type': 'application/octet-stream', 'X-Content-SHA256': digest(file) },
      'PUT',
    );
    const filed = await post(filePath + '/complete', meta(upload.revision), fheaders);
    const fileJob = fp + '/jobs/' + filed.job_id;
    assert.equal((await call(fileJob)).state, 'queued');
    oldKeys.file = {
      upload: upload.upload_id,
      job: filed.job_id,
      epoch: cap.restore_epoch,
      sha256: digest(file),
    };
    const ap = '/platform/parsing';
    await call(ap + '/catalog-syncs', { request_id: randomUUID(), reason: '升级前固定方法' });
    const entries = await call(ap + '/methods?limit=100'),
      existing = entries.items.find(
        (v) => v.entry.method_id === method && v.entry.method_version === version,
      );
    assert(existing);
    if (existing.state !== 'enabled')
      await call(ap + '/methods/' + method + '/versions/' + version + '/state', {
        ...meta(existing.revision),
        state: 'enabled',
      });
    const sample = await call(ap + '/samples', {
      request_id: randomUUID(),
      reason: '旧版试解析',
      received_at: '2026-11-17T00:00:00Z',
      representation: 'binary',
      source: 'synthetic',
    });
    const samplePath = ap + '/samples/' + sample.id;
    await post(
      samplePath + '/content',
      sourceBytes,
      { 'Content-Type': 'application/octet-stream' },
      'PUT',
    );
    await call(samplePath + '/confirm', { request_id: randomUUID(), reason: '旧版已封存样本' });
    const trial = await call(ap + '/runs', {
      request_id: randomUUID(),
      reason: '试解析升级续接',
      kind: 'method_test',
      sample_id: sample.id,
      transport: 'tcp',
      method_id: method,
      method_version: version,
    });
    const trialPath = ap + '/runs/' + trial.id,
      beforeTrial = await call(trialPath);
    assert.equal(beforeTrial.run.state, 'queued');
    oldKeys.parser = {
      run: trial.id,
      sample: sample.id,
      generation: beforeTrial.run.current_generation,
      sha256: digest(sourceBytes),
    };
    // Both the API-created queue record and its immutable input already exist.
    // Simulate an old process dying immediately after taking its lease.
    assert.match(trial.id, /^[0-9a-f-]{36}$/);
    sql(
      db,
      `UPDATE parsing_run_executions SET state='running',fence=fence+1,lease_until=clock_timestamp()-interval '1 second',started_at=clock_timestamp()-interval '2 minutes' WHERE run_id='${trial.id}' AND state='queued'`,
    );
    assert.equal((await call(trialPath)).run.state, 'running');
    const mp = tenant + '/monitoring/imports',
      csv = Buffer.from('observed_at,reading,quality\n2026-11-09T15:00:00Z,18,good\n');
    let imp = await call(mp, { ...meta(), name: 'old.csv', size_bytes: String(csv.length) });
    const importPath = mp + '/' + imp.id;
    await post(
      importPath + '/upload/parts/1',
      csv,
      {
        'Content-Type': 'application/octet-stream',
        'X-Content-SHA256': digest(csv),
        'X-Expected-Revision': imp.revision,
      },
      'PUT',
    );
    imp = await call(importPath + '/upload/complete', {
      ...meta(imp.revision),
      sha256: digest(csv),
    });
    imp = await call(importPath + '/mappings', {
      ...meta(imp.revision),
      mapping: {
        format: 'csv',
        mode: 'wide',
        encoding: 'utf-8',
        delimiter: ',',
        header_row: 1,
        time_column: 'observed_at',
        quality_column: 'quality',
        timezone: 'UTC',
        empty_wide: 'error',
        channels: [
          { alias: 'x', value_column: 'reading', channel_id: s.channels.P1.id, unit: 'mm' },
        ],
      },
    });
    const queued = await call(importPath + '/validate', meta(imp.revision));
    oldKeys.import = {
      batch: imp.id,
      job: queued.job_id,
      source: imp.source_file_id,
      sha256: digest(csv),
    };
    const area = project + '/displays',
      policy = (await call(area)).settings;
    if (!policy.enabled)
      await call(area + '/settings', { ...meta(policy.revision || '0'), enabled: true }, 'PUT');
    const display = await call(area + '/instances', { ...meta(), name: '旧终端升级' });
    const dp = area + '/instances/' + display.id,
      detail = () => call(dp);
    const dc = async (path, extra = {}, method = 'POST') =>
      call(dp + path, { ...meta((await detail()).instance.revision), ...extra }, method);
    const exp = new Date(Date.now() + 86400000).toISOString(),
      term = await dc('/terminals', { name: '旧终端', valid_until: exp });
    const bundle = join(root, '.artifacts/display/example.zip');
    if (!existsSync(bundle)) exec(process.execPath, ['scripts/display-external-check.mjs']);
    const zip = readFileSync(bundle),
      zipHash = digest(zip),
      up = await dc('/uploads', { size: zip.length, sha256: zipHash });
    const edition = '/uploads/' + up.upload_id;
    await post(
      dp + edition + '/parts/0',
      zip,
      { 'Content-Type': 'application/octet-stream', 'X-Content-SHA256': zipHash },
      'PUT',
    );
    const packageJob = await dc(edition + '/complete');
    const before = await call(dp + edition);
    assert(before.jobs.some((j) => j.id === packageJob.job_id && j.state === 'queued'));
    oldKeys.display = {
      instance: display.id,
      terminal: term.terminal_id,
      upload: up.upload_id,
      job: packageJob.job_id,
      archive: zipHash,
    };
    save();
    // All handles are children of this sandbox. A remains as an exact,
    // read-only comparison database after creating its separate upgrade clone.
    const args = own.options;
    await Promise.all(args.children.map(stop));
    exec('docker', ['exec', lab.container, 'createdb', '-U', 'archtest', '-T', db, next]);
    const current = read(lab.a.config),
      database = new URL(current.database_url);
    database.pathname = '/' + next;
    current.database_url = database.href;
    const priv = join(dir, 'private', 'upgrade');
    mkdirSync(priv, { recursive: true, mode: 0o700 });
    const config = join(priv, 'service.json');
    write(config, current);
    const env = { ...clean, INFRAPRO_CONFIG: config },
      bin = args.bin;
    exec(join(bin, 'infrapro'), ['database', 'migrate'], { env });
    const newVersion = schema(next);
    assert.equal(newVersion, read(join(bin, '..', 'candidate.json')).schema.version);
    assert.equal(schema(db), 58);
    const httpRoles = [
      'identity-api',
      'control-api',
      'monitoring-query-api',
      'transfer-api',
      'display-runtime',
      'display-assets',
    ];
    const workerNames = [
      'monitoring-publish-worker',
      'monitoring-history-worker',
      'monitoring-incremental-worker',
      'monitoring-read-worker',
      'parsing-dispatch-worker',
      'files-worker',
      'display-package-worker',
    ];
    restrictedRoles = [...httpRoles, ...workerNames];
    const credentials = pairedRoleConfigs(priv, current, lab.container, restrictedRoles);
    const { roleEnv } = credentials;
    appliedRoleSQL = credentials.appliedRoleSQL;
    for (const role of [...httpRoles, 'api']) {
      const manifest = join(priv, role + '.json');
      write(manifest, pairedRoleManifest(args.topology, role, args.endpoints));
      args.children.push(
        launch(
          join(bin, role),
          [],
          { ...env, ...roleEnv(role), INFRAPRO_ARCHITECTURE_FILE: manifest },
          join(priv, role + '.log'),
        ),
      );
    }
    await waitURL(lab.a.api + '/readyz');
    assert((await call(tenant + '/context')).membership_id);
    assert.deepEqual(await rows(), frozenValues);
    const original = sql(
      db,
      `SELECT current_generation::text FROM parsing_runs WHERE id='${trial.id}'`,
    );
    assert.equal(original, String(oldKeys.parser.generation));
    assert.equal((await call(trialPath)).run.state, 'running');
    await getBytes(samplePath + '/content', sourceBytes);
    record('保留原会话、原 URL、旧任务根、旧发布版本与历史分析逐行结果', {
      schemaBefore: 58,
      schemaAfter: newVersion,
      artifactVersion: versionID,
      values: frozenValues,
      sampleSHA256: digest(sourceBytes),
      frozenParsingGeneration: original,
    });
    const runtime = { ...args, bin, env, side: 'b', privateDir: priv, roleEnv };
    args.children.push(
      launch(
        join(bin, 'files-checker'),
        ['--directory', own.tmp, '--socket', join(own.tmp, 'checker.sock')],
        clean,
        join(priv, 'files-checker.log'),
      ),
    );
    await startPairedWorkers({
      ...runtime,
      legacy: [],
      split: workerNames,
      claimRoles: workerNames.slice(0, 5),
    });
    const after = await waitState(trialPath, (r) => r.run.state, 'succeeded');
    assert.equal(after.run.current_generation, oldKeys.parser.generation);
    const response = await call(
      trialPath + '/executions/' + after.run.current_generation + '/result',
    );
    assert.equal(response.response.input_sha256, digest(sourceBytes));
    assert.equal(response.formally_adopted, false);
    record('过期试解析租约在原根和代次内恢复', {
      run: trial.id,
      generation: after.run.current_generation,
      method: response.response.method,
      formallyAdopted: response.formally_adopted,
    });
    const finished = await waitState(fileJob, (r) => r.state, 'succeeded');
    assert.equal(finished.generation, filed.job_generation);
    await getBytes(
      fp +
        '/attachments/' +
        finished.result_attachment_id +
        '/revisions/' +
        finished.result_revision_id +
        '/content',
      file,
    );
    assert.equal((await post(fp + '/uploads', input, fheaders)).upload_id, upload.upload_id);
    record('待检查附件续接、旧请求键重放和读回摘要', {
      job: filed.job_id,
      generation: finished.generation,
      sha256: digest(file),
      sameUpload: true,
    });
    const ready = await waitState(importPath, (r) => r.state, 'ready');
    assert.equal(ready.id, imp.id);
    await call(importPath + '/publish', meta(ready.revision));
    const formal = await waitState(importPath, (r) => r.state, 'published');
    assert.equal(formal.counts.proposed, '1');
    await getBytes(tenant + '/monitoring/source-files/' + formal.source_file_id + '/download', csv);
    record('验证中的导入按原批次完成发布', {
      batch: formal.id,
      job: queued.job_id,
      sourceFile: formal.source_file_id,
      sha256: digest(csv),
      proposed: formal.counts.proposed,
    });
    const p = await wait('old display package job', async () => {
      const upload = await call(dp + edition);
      const job = upload.jobs.find((j) => j.id === packageJob.job_id);
      assert(job);
      return job.state === 'ready' && (await detail()).packages.find((x) => x.state === 'ready');
    });
    assert.equal(p.manifest.manifest_version, 1);
    await dc(
      '/grant',
      {
        grant: {
          capabilities: ['resources.read'],
          valid_until: exp,
          scope: { resources: [s.nodes.A.id] },
        },
      },
      'PUT',
    );
    const configVersion = await dc('/configs', {
      package_id: p.id,
      config_format: 1,
      parameters: { title: '升级前后' },
      bindings: { site: { kind: 'resources', ids: [s.nodes.A.id] } },
      read_views: {},
      mode: 'follow',
    });
    const prep = await dc('/prepare', { config_id: configVersion.config_id });
    await dc('/publish', { config_id: configVersion.config_id, preparation: prep.hash });
    const release = (await detail()).instance.current_release_id,
      rh = { Origin: own.runtime, 'X-Display-Protocol': '1' };
    const operation = async (path, data) => {
      const res = await terminal.post(own.runtime + path, { headers: rh, data });
      try {
        assert.equal(res.status(), 200, 'old terminal operation ' + path);
        return await res.json();
      } finally {
        await res.dispose();
      }
    };
    await operation('/runtime/terminal/pair', { code: term.pair_code });
    const runSession = await operation('/runtime/terminal/start', {});
    assert.equal(runSession.session.release_id, release);
    record('旧包作业与旧终端配对身份继续使用原发布地址', {
      packageJob: packageJob.job_id,
      package: p.id,
      archiveSHA256: zipHash,
      terminal: term.terminal_id,
      release,
    });
    assert.deepEqual(await rows(), frozenValues);
    const completedCount = Number(
      sql(next, `SELECT count(*) FROM analysis_artifact_versions WHERE id='${versionID}'`),
    );
    assert.equal(completedCount, 1);
    save();
    captureRuntimeSessions();
    return { passed: true, elapsed: Date.now() - opened, cases };
  } catch (e) {
    if (appliedRoleSQL) captureRuntimeSessions();
    cases.push({
      arc: 'ARC-029',
      name: '旧发行物冷升级',
      passed: false,
      error: e.message.split('\n')[0].slice(0, 190),
    });
    save();
    throw e;
  } finally {
    await Promise.all([old.client.dispose(), terminal.dispose()]);
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const r = await legacyWork(
      runDirectory(process.argv[2]),
      process.argv[3] ? runDirectory(process.argv[3]) : undefined,
    );
    console.log('Legacy in-progress state re-opened', r.passed);
  } catch (e) {
    console.error(
      'Legacy cold start failed',
      e.message
        .split('\n')[0]
        .replace(/https?:\/\/\S+/g, '[loopback endpoint]')
        .slice(0, 300),
    );
    process.exitCode = 1;
  }
}
