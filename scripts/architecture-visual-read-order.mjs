import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { chromium, expect } from '@playwright/test';
import { client, digest, queryPG, read, safeDiagnosticLine, write } from './architecture-lab.mjs';
import { visual } from './architecture-visual.mjs';
import { visualOutputDirectory } from './architecture-visual-frontend.mjs';

function observations(stats, channelName) {
  return (stats.channels ?? [])
    .map((channel) => {
      const datum = channel.latest_output ?? channel.last;
      return {
        channel: channelName(channel.channel_id),
        kind: channel.source_kind,
        count: channel.n,
        at: datum?.output_at ?? datum?.observed_at ?? null,
        value: datum?.value ?? null,
        quality: datum?.data_quality ?? datum?.quality ?? null,
      };
    })
    .sort((a, b) => a.channel.localeCompare(b.channel));
}

function orderedFacts(structure, request, status, body, ids) {
  const path = new URL(request.url()).pathname;
  if (
    request.method() !== 'POST' ||
    !/(?:\/monitoring\/queries\/latest|\/spatial\/summaries)$/.test(path)
  )
    return undefined;
  const operation = path.endsWith('/latest') ? 'latest' : 'summaries';
  const channels = new Map(
    Object.entries(structure.channels).map(([name, channel]) => [
      channel.id,
      name,
    ]),
  );
  channels.set(ids.tenantChannel, 'tenantChannel');
  channels.set(ids.projectChannel, 'projectChannel');
  const key = (id) => channels.get(id) ?? id;
  const result = {
    operation,
    status,
    requestSHA256: digest(request.postDataBuffer()),
    responseSHA256: digest(body),
  };
  if (status !== 200) return result;
  const envelope = JSON.parse(body),
    data = envelope.data ?? envelope;
  if (operation === 'latest')
    return {
      ...result,
      data: { state: data.state, observations: observations(data, key) },
    };
  return {
    ...result,
    data: data.items.map((item) => ({
      ownerKind: item.owner.kind,
      owner: item.owner.id,
      channels: item.channels
        .map((entry) => ({
          channel: key(entry.channel.id),
          state: entry.latest.state,
          observations: observations(entry.latest, key),
        }))
        .sort((a, b) => a.channel.localeCompare(b.channel)),
    })),
  };
}

function sameSuccessfulValues(pair, state, structure) {
  const expected = state === 'authorized-channel' ? 'latest' : 'summaries';
  function inspected(items) {
    if (
      !items.length ||
      items.some(
        (item) =>
          item.operation !== expected || item.status !== 200 || !item.data,
      )
    )
      return null;
    const values = items.map((item) => item.data);
    if (expected === 'latest') {
      if (
        !values.every((v) => v.state === 'ready' && v.observations.length > 0)
      )
        return null;
      const rows = values.flatMap((v) => v.observations);
      if (
        !rows.some(
          (r) =>
            r.channel === 'tenantChannel' &&
            Number(r.value) === 18 &&
            r.quality === 'good',
        )
      )
        return null;
    } else {
      const owners = values
        .flat()
        .filter(
          (v) => v.ownerKind === 'point' && v.owner === structure.nodes.P1.id,
        );
      if (
        !owners.length ||
        owners.some(
          (v) =>
            v.channels.length < 2 ||
            v.channels.some((c) => c.state !== 'ready'),
        )
      )
        return null;
      const rows = owners.flatMap((v) =>
        v.channels.flatMap((c) => c.observations),
      );
      if (
        !rows.some(
          (r) =>
            r.channel === 'P1' && Number(r.value) === 9 && r.quality === 'good',
        )
      )
        return null;
      if (
        !rows.some(
          (r) =>
            ['tenantChannel', 'projectChannel'].includes(r.channel) &&
            Number(r.value) === 18 &&
            r.quality === 'good',
        )
      )
        return null;
    }
    // Two independently built channels have different public IDs and selection
    // receipts. The known fixture maps those IDs above; values, timestamps and
    // QA flags still have to match exactly across both real responses.
    return values.map((v) => JSON.stringify(v)).sort();
  }
  const first = inspected(pair.baseline),
    second = inspected(pair.peer);
  return Boolean(first && second && isDeepStrictEqual(first, second));
}

// Observe the actual Chromium transport on the pages which issue several
// simultaneous authorized reads. This run installs no routing or interception.
async function nativeReadEvidence(dir, side, ordinary, states, destination) {
  const lab = read(join(dir, 'private/lab.json')), samples = read(join(dir, 'samples.json'));
  const account = read(join(dir, 'private/accounts.json')).admin;
  const evidence = join(destination, 'native-browser', new Date().toISOString().replaceAll(':','-'));
  mkdirSync(evidence, { recursive: true, mode: 0o700 });
  const report = { sourceDirectory: dir, candidate: ordinary.candidate, snapshot: ordinary.snapshot,
    frontend: ordinary.frontend, naturalIdentity: ordinary.identity, transport: 'Chromium', httpOverrides: [],
    expected: states.length, rows: [], passed: false };
  const file = join(evidence, 'result.json'), browser = await chromium.launch(), sessions = {};
  try {
    for (const actor of ['a', side]) {
      const session = await client(lab[actor], account);
      try { sessions[actor] = await session.client.storageState(); }
      finally { await session.client.dispose(); }
    }
    for (const row of states) {
      const group = { key: row.key, state: row.state, viewport: row.viewport, pair: {}, passed: false };
      report.rows.push(group);
      for (const actor of ['a', side]) {
        const bounds = row.bounds[actor], cfg = lab[actor];
        const ctx = await browser.newContext({ viewport: row.viewport, storageState: sessions[actor], hasTouch: row.viewport.width === 375,
          isMobile: row.viewport.width === 375, deviceScaleFactor: 1, locale: 'zh-CN', timezoneId: 'Asia/Shanghai', reducedMotion: 'reduce' });
        const page = await ctx.newPage(), requests = new Set(), results = [], pendingBodies = [];
        const probe = { requested: [], reads: [], errors: [], failed: [], path: bounds.path };
        group.pair[actor] = probe;
        let touched = Date.now();
        const api = request => new URL(request.url()).pathname.startsWith('/api/v1/') && !request.url().includes('/events/stream');
        const stop = request => { if (requests.delete(request)) touched = Date.now(); };
        const reads = request => request.method() === 'POST' &&
          /(?:\/monitoring\/queries\/latest|\/spatial\/summaries)$/.test(new URL(request.url()).pathname);
        page.on('request', request => {
          if (api(request)) { requests.add(request); touched = Date.now(); }
          if (reads(request)) probe.requested.push(digest(request.postDataBuffer()));
        });
        page.on('requestfinished', stop);
        page.on('requestfailed', request => {
          stop(request);
          if (api(request)) probe.failed.push({ method: request.method(), path: new URL(request.url()).pathname, failure: request.failure()?.errorText });
        });
        page.on('pageerror', error => { probe.errors.push(safeDiagnosticLine(error.message).slice(0,250)); });
        page.on('response', response => {
          const request = response.request();
          if (!reads(request)) return;
          const work = response.body().then(bytes => {
            results.push(orderedFacts(samples.structure, request, response.status(), bytes, bounds.ids));
          }).catch(error => { probe.errors.push(safeDiagnosticLine(error.message).slice(0,250)); });
          pendingBodies.push(work);
        });
        try {
          probe.deadlocksBefore = Number(queryPG(lab,actor,'SELECT deadlocks FROM pg_stat_database WHERE datname=current_database()'));
          await page.clock.setFixedTime(new Date(samples.clock || '2026-11-18T06:00:00Z'));
          await page.goto(cfg.origin + bounds.path);
          const heading = row.state === 'authorized-channel' ? '租户位移双倍' : '工程空间';
          await expect(page.getByRole('heading',{ name: heading, exact:true })).toBeVisible({timeout:12000});
          if (row.state !== 'authorized-channel') {
            const name = row.state === 'layout-version' ? '验收图纸 · 桥梁总体布置' : '验收模型 · 桥梁构件';
            await expect(page.getByText(name,{ exact:false }).first()).toBeVisible({timeout:12000});
          }
          await expect.poll(() => results.length > 0 && requests.size === 0 && Date.now()-touched >= 500, { timeout:15000 }).toBe(true);
          await Promise.all(pendingBodies);
          const content=await page.locator('body').ariaSnapshot();
          probe.accessibilitySHA256=digest(content);
          probe.screenshot=row.key+'-'+actor+'.png';
          probe.screenshotSHA256=digest(await page.screenshot({ path:join(evidence,probe.screenshot), animations:'disabled', caret:'hide' }));
        } catch (error) { probe.errors.push(safeDiagnosticLine(error.message).slice(0,300)); }
        finally {
          await Promise.all(pendingBodies);
          probe.reads = results;
          probe.everySelectionAnswered = probe.requested.length > 0 &&
            isDeepStrictEqual([...probe.requested].sort(),results.map(item=>item.requestSHA256).sort());
          probe.deadlocksAfter = Number(queryPG(lab,actor,'SELECT deadlocks FROM pg_stat_database WHERE datname=current_database()'));
          await ctx.close();
        }
      }
      group.passed = Object.values(group.pair).every(result=>!result.errors.length && result.screenshotSHA256 && result.everySelectionAnswered) &&
        sameSuccessfulValues({baseline:group.pair.a.reads,peer:group.pair[side].reads},row.state,samples.structure);
      write(file,report);
    }
    report.passed = report.rows.length === states.length && report.rows.every(row=>row.passed);
    write(file,report);
    return {path:file, sha256:digest(readFileSync(file)), captured:report.rows.length, passed:report.passed};
  } finally { await browser.close(); }
}

// A shared release/driver identity can produce several real browser outcomes.
// Select the immutable archive by the measured matrix and all its captured files.
export function archiveNaturalVisualAttempt(source, destination, rows) {
  const matrixPath = join(source, 'matrix.json');
  const matrix = readFileSync(matrixPath);
  const identity = JSON.parse(matrix).identity;
  const naturalMatrixSHA256 = digest(matrix);
  const files = new Map([['matrix.json', matrix]]);
  const manifests = [];
  for (const row of rows) {
    const screenshots = {}, accessibility = {};
    for (const view of ['old-live', 'live', 'old', 'paired']) {
      for (const [extension, entries] of [['png', screenshots], ['json', accessibility]]) {
        const input = join(source, row.key, view + '.' + extension);
        if (!existsSync(input)) continue;
        const bytes = readFileSync(input);
        files.set(row.key + '-' + view + '.' + extension, bytes);
        entries[view] = digest(bytes);
      }
    }
    manifests.push({ key: row.key, viewport: row.viewport, passed: row.passed, screenshots, accessibility });
  }
  const naturalArtifactsSHA256 = digest(JSON.stringify(manifests));
  for (const manifest of manifests)
    files.set(manifest.key + '.json', Buffer.from(JSON.stringify(manifest, null, 2) + '\n'));
  const parent = join(destination, identity, naturalMatrixSHA256);
  const directory = join(parent, naturalArtifactsSHA256);
  if (!existsSync(directory)) {
    mkdirSync(parent, { recursive: true, mode: 0o700 });
    const temporary = mkdtempSync(join(parent, '.capture-'));
    try {
      for (const [name, bytes] of files)
        writeFileSync(join(temporary, name), bytes, { mode: 0o600 });
      try {
        renameSync(temporary, directory);
      } catch (error) {
        if (!['EEXIST', 'ENOTEMPTY'].includes(error.code)) throw error;
      }
    } finally {
      rmSync(temporary, { recursive: true, force: true });
    }
  }
  for (const [name, bytes] of files)
    assert(bytes.equals(readFileSync(join(directory, name))), 'Natural evidence changed: ' + name);
  return { naturalMatrix: join(directory, 'matrix.json'), naturalMatrixSHA256, naturalArtifactsSHA256 };
}

// Preserve natural browser results and their original PostgreSQL errors.
// This separate comparison exercises a repeatable, one-at-a-time browser
// schedule on both real applications with the same multi-channel authority.
export async function orderedVisualReads(
  dir,
  side,
  output,
  { resume = true } = {},
) {
  assert(['self', 'b'].includes(side));
  output = visualOutputDirectory(output);
  const pgSchedule = 'native';
  const path = join(output, 'visual', 'feature-states', side, 'matrix.json');
  const ordinary = read(path),
    structure = read(join(dir, 'samples.json')).structure;
  assert.equal(ordinary.sourceDirectory, dir);
  assert(
    ordinary.complete,
    'Preserve the complete natural attempt before running the supplemental order',
  );
  const selected = ordinary.rows.filter(
    (row) =>
      row.viewport.width === 1440 &&
      ((row.state === 'authorized-channel' && !row.template.includes('/p/')) ||
        row.state === 'layout-version' ||
        row.state === 'model-version'),
  );
  assert.equal(
    selected.length,
    3,
    'Expected the three natural multi-channel pages in the source matrix',
  );
  const rows = selected.map(
    ({ app, template, path, persona, state, bounds, key }) => ({
      app,
      template,
      path,
      persona,
      state,
      bounds,
      originalKey: key,
      async verify(page) {
        const heading =
          state === 'authorized-channel' ? '租户位移双倍' : '工程空间';
        await page
          .getByRole('heading', { name: heading, exact: true })
          .first()
          .waitFor({ timeout: 12000 });
        if (state !== 'authorized-channel') {
          const title =
            state === 'layout-version'
              ? '验收图纸 · 桥梁总体布置'
              : '验收模型 · 桥梁构件';
          await page
            .getByText(title, { exact: false })
            .first()
            .waitFor({ timeout: 15000 });
        }
      },
      observeRead: (req, status, body, ids) =>
        orderedFacts(structure, req, status, body, ids),
      verifyReads: (pair) => sameSuccessfulValues(pair, state, structure),
    }),
  );
  const destination = join(output, 'visual', 'read-order', pgSchedule);
  const states = ordinary.rows.filter((row) =>
    selected.some((item) => item.template === row.template && item.state === row.state),
  );
  const archive = archiveNaturalVisualAttempt(
    join(output, 'visual', 'feature-states', side),
    join(destination, 'natural', side),
    states,
  );
  const native = await nativeReadEvidence(dir,side,ordinary,states,destination);
  const signature = {
    pgSchedule,
    driver: digest(readFileSync(new URL(import.meta.url))),
    naturalIdentity: ordinary.identity,
    naturalMatrixSHA256: archive.naturalMatrixSHA256,
    naturalArtifactsSHA256: archive.naturalArtifactsSHA256,
  };
  const result = await visual(dir, side, {
    extraRows: rows,
    extraSource: signature,
    requestOrder: 'serial',
    outputDirectory: destination,
    resume,
  });
  const run = {
    source: ordinary.sourceDirectory,
    snapshot: ordinary.snapshot,
    candidate: result.candidate,
    side,
    natural: path,
    native,
    ...archive,
    signature,
    pgSchedule,
    httpInflight: 1,
    orderedIdentity: result.identity,
    matrix: join(
      destination,
      'visual',
      'ordered-read-states',
      side,
      'matrix.json',
    ),
    required: 9,
    captured: result.rows.length,
    equal: result.rows.filter((row) => row.passed).length,
    passed: result.passed,
  };
  write(join(destination, side + '.json'), run);
  return run;
}
