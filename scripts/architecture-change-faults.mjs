// Exercise a real paused consumer and stopped publisher on a private S0 clone.
// The public requests go through both the frozen and split HTTP entry points.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { client as login, digest, launch, read, root, runDirectory, sleep, write } from './architecture-lab.mjs';
import { withHTTPPair, startPairedWorkers } from './architecture-http-sandbox.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';
import { stop, waitURL } from './fixtures/parsing-lab.mjs';
import {
  changeDisplayReader,
  configureChangeDisplay,
  startChangeDisplay,
} from './fixtures/architecture-change-display.mjs';

const roles = [
  'monitoring-publish-worker',
  'monitoring-incremental-worker',
  'monitoring-history-worker',
  'monitoring-read-worker',
];
const httpRoles = ['identity-api', 'control-api', 'transfer-api', 'monitoring-query-api'];
const groups = ['derived', 'display-cache', 'query-cache', 'summary'];
const meta = (expected_revision = '0') => ({
  request_id: randomUUID(),
  expected_revision,
  reason: '消息照常接收、历史更正和消费位置恢复',
});
const command = (bin, args, extra = {}) =>
  execFileSync(bin, args, {
    cwd: root,
    stdio: 'pipe',
    encoding: 'utf8',
    ...extra,
  });
async function until(label, probe, timeout = 60000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    const value = await probe();
    if (value) return value;
    await sleep(160);
  }
  throw Error(label + ' did not reach its required state');
}

async function setupBroker(args, side, priv, context) {
  if (side === 'a')
    return startPairedWorkers({
      ...args,
      side,
      privateDir: priv,
      legacy: ['monitoring-worker'],
      split: [],
      claimRoles: [],
    });
  const binary = join(args.bin, 'event-relay'),
    namespace = 'arc_recovery',
    certs = join(priv, 'broker');
  const info = JSON.parse(command(binary, ['status'], { env: args.env }));
  const epoch = info.protocol.restore_epoch;
  command(process.execPath, ['scripts/architecture-event-lab.mjs', certs, namespace, epoch]);
  const broker = read(join(certs, 'lab.json')),
    server = join(
      root,
      'build/events',
      process.platform + '-' + (process.arch === 'x64' ? 'amd64' : process.arch),
      'nats-server',
    );
  assert(existsSync(server), 'offline broker binary is required');
  const health = 'http://127.0.0.1:' + broker.monitorPort + '/healthz?js-enabled-only=true';
  let nats;
  const startBroker = async () => {
    nats = launch(server, ['-c', broker.config], args.env, join(priv, 'broker.log'));
    args.children.push(nats);
    await waitURL(health);
  };
  await startBroker();
  const envFor = (role) => ({
    ...args.roleEnv(role),
    INFRAPRO_EVENTS_FILE: join(certs, role, 'events.json'),
  });
  const admin = (...cmd) =>
    command(binary, cmd, {
      env: {
        ...args.env,
        INFRAPRO_EVENTS_FILE: join(certs, 'event-admin/events.json'),
      },
    });
  admin('provision');
  admin('enable', '--reason', 'isolated publication reordering');
  for (const role of roles)
    admin(
      'switch',
      '--role',
      role,
      '--expected',
      '1',
      '--mode',
      'split',
      '--reason',
      'isolated consumer recovery',
    );
  const report = join(priv, 'publisher-report.json');
  context.startRelay = async () => {
    const env = {
      ...args.env,
      ...envFor('event-relay'),
      INFRAPRO_DIAGNOSTICS_REPORT_FILE: report,
      INFRAPRO_DIAGNOSTICS_INSTANCE_ID: 'arc',
      INFRAPRO_DIAGNOSTICS_NODE_ID: 'local',
      INFRAPRO_DIAGNOSTICS_REPLICA_ID: 'event-relay',
    };
    const child = launch(binary, ['run'], env, join(priv, 'relay.log'));
    args.children.push(child);
    await until(
      'publisher ready',
      () => child.exitCode === null && existsSync(report) && read(report).phase === 'ready',
    );
    context.relay = child;
  };
  await context.startRelay();
  context.withDiskAndBrokerFault = async (probe) => {
    const adminFile = join(certs, 'event-admin/events.json');
    const relayFile = join(certs, 'event-relay/events.json');
    const adminSettings = read(adminFile), relaySettings = read(relayFile);
    await stop(context.relay);
    try {
      // Use the real retained formal changes and the smallest supported file
      // stream. The full-limit tests separately fill all three streams.
      write(adminFile, { ...adminSettings, monitoring_bytes: 65536 });
      admin('provision');
      write(relayFile, { ...relaySettings, disk_soft_percent: 1, disk_hard_percent: 2 });
      await context.startRelay();
      return await probe(async () => {
        await stop(nats);
        assert(nats.exitCode !== null || nats.signalCode);
        await assert.rejects(fetch(health, { signal: AbortSignal.timeout(1500) }));
        return { portRefused: true, stoppedPid: nats.pid };
      });
    } finally {
      if (nats.exitCode !== null || nats.signalCode) await startBroker();
      await stop(context.relay);
      write(adminFile, adminSettings);
      write(relayFile, relaySettings);
      admin('provision');
      await context.startRelay();
    }
  };
  for (const role of roles) {
    const own = ['monitoring-read-worker', 'monitoring-incremental-worker'].includes(role)
      ? envFor(role)
      : {
          ...args.roleEnv(role),
          INFRAPRO_EVENTS_FILE: join(certs, 'pg/events.json'),
        };
    await startPairedWorkers({
      ...args,
      side,
      privateDir: priv,
      legacy: [],
      split: [role],
      claimRoles: [],
      roleEnv: () => own,
    });
  }
}
export async function changeFaults(seed, out = seed) {
  const context = { runtime: {}, displays: [] };
  return withHTTPPair(seed, 'change-recovery', (dir) => exercise(dir, out, context), {
    restrictedRoles: [
      ...httpRoles,
      ...roles,
      'event-relay',
      'display-runtime',
      'display-assets',
      'display-package-worker',
    ],
    configure: async ({ side, cfg, privateDir }) => {
      context.runtime[side] = await configureChangeDisplay(cfg);
      return {
        start: async (args) => {
          await setupBroker(args, side, privateDir, context);
          await startChangeDisplay(side, privateDir, context.runtime[side], args);
        },
      };
    },
  });
}
async function exercise(dir, out, context) {
  const lab = read(join(dir, 'private/lab.json')),
    { candidateSHA256, appliedRoleSQL } = read(join(dir, 'snapshot.json')),
    sample = read(join(dir, 'samples.json')).structure,
    pair = new PairedHTTP(dir, 'change-recovery', out);
  const tenant = sample.tenant,
    channel = sample.channels.P1.id;
  assert.match(lab.id, /^[a-f0-9]{12}$/);
  assert.match(tenant, /^[0-9a-f-]{36}$/);
  const db = 'arc_http_' + lab.id + '_b';
  const sideSQL = (side, q) =>
    command('docker', [
      'exec',
      lab.container,
      'psql',
      '-XqAt',
      '-U',
      'archtest',
      '-d',
      'arc_http_' + lab.id + '_' + side,
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      q,
    ]).trim();
  const sql = (q) => sideSQL('b', q);
  assert.equal(new URL(read(lab.b.config).database_url).pathname, '/' + db);
  const { call, client, headers, api } = await pair.login(
    read(join(dir, 'private/accounts.json')).admin,
  );
  const base = '/tenants/' + tenant + '/monitoring',
    time = '2026-11-07T00:00:00Z',
    later = '2026-11-07T00:02:00Z';
  const checkpoints = () =>
    JSON.parse(
      sql(
        `SELECT coalesce(jsonb_object_agg(consumer,position),'{}'::jsonb) FROM event_consumer_positions WHERE scope='${tenant}' AND restore_epoch=(SELECT restore_epoch FROM event_protocol WHERE id=1)`,
      ),
    );
  const position = () =>
    Number(sql(`SELECT commit_sequence FROM monitoring_tenant_states WHERE tenant_id='${tenant}'`));
  const start = position(),
    waitState = (p, state) => client.wait(api + p, headers, (b) => b.state, state);
  const displayRange = { from: time, to: '2026-11-07T00:05:00Z' };
  const displayFailures = [];
  async function displayCheck(stage, probe) {
    try {
      return await probe();
    } catch (e) {
      displayFailures.push({
        stage,
        error: e.message
          .split('\n')[0]
          .replace(/https?:\/\/\S+/g, '[loopback endpoint]')
          .slice(0, 240),
      });
      write(join(dir,'display-failures.json'),displayFailures);
      return null;
    }
  }
  const visibleOnDisplays = async (expected) => {
    assert.equal(context.displays.length, 2, 'both authorized runtime sessions must exist');
    const pages = await Promise.all(context.displays.map((d) => d.read(displayRange)));
    for (const page of pages) {
      assert.deepEqual(
        page.rows.map((r) => r.value),
        expected,
      );
      assert.match(page.query, /^[0-9a-f-]{36}$/);
    }
    assert.deepEqual(pages[0].rows, pages[1].rows);
    assert.notEqual(pages[0].query, pages[1].query);
    return { a: pages[0].query, b: pages[1].query, rows: pages[0].rows };
  };
  const prepare = async (at, value) => {
    const bytes = Buffer.from('observed_at,reading,quality\n' + at + ',' + value + ',good\n'),
      sha = digest(bytes);
    let b = await call(base + '/imports', {
      ...meta(),
      name: 'measure.csv',
      size_bytes: String(bytes.length),
    });
    const path = base + '/imports/' + b.id;
    const part = await client.put(api + path + '/upload/parts/1', {
      headers: {
        ...headers,
        'Content-Type': 'application/octet-stream',
        'X-Expected-Revision': b.revision,
        'X-Content-SHA256': sha,
      },
      data: bytes,
    });
    try {
      assert.equal(part.status(), 200);
    } finally {
      await part.dispose();
    }
    b = await call(path + '/upload/complete', {
      ...meta(b.revision),
      sha256: sha,
    });
    b = await call(path + '/mappings', {
      ...meta(b.revision),
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
          {
            alias: 'c',
            value_column: 'reading',
            channel_id: channel,
            unit: 'mm',
          },
        ],
      },
    });
    await call(path + '/validate', meta(b.revision));
    return { path, b: await waitState(path, 'ready') };
  };
  const publish = async (at, value) => {
    const { path, b } = await prepare(at, value);
    await call(path + '/publish', meta(b.revision));
    return waitState(path, 'published');
  };
  const record = (arc, name, facts) => ({
    arc,
    name,
    passed: true,
    checks: facts,
  });
  const cases = [];
  let arc = 'ARC-022';
  function save() {
    const rows = { sourceRun: dir, candidateSHA256, appliedRoleSQL, cases };
    write(join(out, 'faults/change-recovery.json'), rows);
    write(join(dir, 'change-recovery.json'), rows);
    pair.save();
  }
  try {
    const account = read(join(dir, 'private/accounts.json')).admin;
    const commandIDs = [];
    for (const side of ['a', 'b']) {
      const reader = await displayCheck(side + ' publication and launch', () =>
        changeDisplayReader(lab[side], context.runtime[side], sample, account, commandIDs),
      );
      if (reader) context.displays.push(reader);
    }
    await stop(context.relay);
    assert(context.relay.exitCode !== null || context.relay.signalCode);
    // Only the summary group's claim of a new pending row fails. Other
    // subscriptions and the producer statement never touch this trigger.
    sql(
      "CREATE FUNCTION arc_pause_summary() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated summary pause'; END $$; CREATE TRIGGER arc_pause_summary BEFORE UPDATE OF state ON event_work FOR EACH ROW WHEN (NEW.consumer='summary' AND NEW.state='running' AND OLD.state<>'running') EXECUTE FUNCTION arc_pause_summary()",
    );
    let tail, whilePaused, publicationOrder, initiallyVisible;
    try {
      const first = await publish(time, 40);
      initiallyVisible = await displayCheck('before revocation', () => visibleOnDisplays([40]));
      await publish(later, 13);
      const revoke = base + '/batches/' + first.id,
        preview = await call(revoke + '/revoke-preview', {
          expected_revision: first.revision,
        });
      await call(revoke + '/revoke', {
        ...meta(first.revision),
        preview_hash: preview.preview_hash,
      });
      await publish(time, 42);
      tail = position();
      assert(tail >= start + 4);
      const events = () =>
        JSON.parse(
          sql(
            `SELECT coalesce(jsonb_agg(jsonb_build_object('position',o.generation_or_position,'state',state,'broker',broker_sequence,'change',c.change_type) ORDER BY o.generation_or_position),'[]'::jsonb) FROM event_outbox o JOIN monitoring_changes c ON c.id=o.aggregate_id WHERE o.type='monitoring.changed' AND o.scope='${tenant}' AND o.generation_or_position>${start}`,
          ),
        );
      const queued = events();
      assert.deepEqual(
        queued.map((v) => v.change),
        ['raw_publish', 'raw_publish', 'raw_revoke', 'raw_publish'],
      );
      assert(queued.every((v) => v.state === 'pending' && v.broker === null));
      whilePaused = await until('unpaused groups advance without relay', () => {
        const cp = checkpoints();
        return (
          (cp.summary ?? start) < tail &&
          groups.filter((n) => n !== 'summary').every((n) => cp[n] === tail) &&
          cp
        );
      });
      cases.push(
        record('ARC-027', '停消息转发器仍提交正式事实，另外三个订阅从 PG 对账', {
          start,
          tail,
          pending: queued.length,
          positions: whilePaused,
        }),
      );
      save();
      // Make the first publication's advisory unavailable until after all of
      // its successors have actually been acknowledged by the private broker.
      const delayed = queued[0].position,
        where = `type='monitoring.changed' AND scope='${tenant}' AND generation_or_position=${delayed}`;
      sql(
        `UPDATE event_outbox SET available_at=clock_timestamp()+interval '5 minutes' WHERE ${where} AND state='pending'`,
      );
      await context.startRelay();
      await until('later hints arrive before first', () => {
        const list = events();
        return list[0].broker === null && list.slice(1).every((x) => x.state === 'sent');
      });
      sql(
        `UPDATE event_outbox SET available_at=clock_timestamp() WHERE ${where} AND state='pending'`,
      );
      publicationOrder = await until('missing publication recovered', () => {
        const list = events();
        return list.every((x) => x.state === 'sent') && list;
      });
      assert(
        publicationOrder[0].broker > Math.max(...publicationOrder.slice(1).map((x) => x.broker)),
      );
    } finally {
      sql('DROP TRIGGER arc_pause_summary ON event_work; DROP FUNCTION arc_pause_summary()');
    }
    const reached = await until('independent durable positions', () => {
      const cp = checkpoints();
      return groups.every((k) => cp[k] === tail) && cp;
    });
    const floors = JSON.parse(
      sql(
        `SELECT jsonb_object_agg(consumer,revision) FROM monitoring_cache_floors WHERE tenant_id='${tenant}' AND domain_key IN ('${tenant}','${channel}')`,
      ),
    );
    const revoked = Number(
      sql(
        `SELECT revoke_revision FROM monitoring_channel_versions WHERE tenant_id='${tenant}' AND channel_id='${channel}'`,
      ),
    );
    const consumed = Number(
      sql(
        `SELECT durable_cursor FROM monitoring_change_consumers WHERE tenant_id='${tenant}' AND consumer='derived-invalidation/1'`,
      ),
    );
    assert.equal(floors['display-cache'], tail);
    assert.equal(floors['query-cache'], revoked);
    assert(revoked > 0);
    assert.equal(consumed, tail);
    const selected = await call(base + '/selections', {
        channel_ids: [channel],
      }),
      query = {
        selection_id: selected.id,
        range: { from: time, to: '2026-11-07T00:05:00Z' },
        limit: 20,
        include_total: true,
      };
    const details = await call(base + '/queries/details', query);
    assert.equal(details.total, 2);
    assert.deepEqual(
      details.items.map((i) => Number(i.value)),
      [13, 42],
    );
    const overview = await call(base + '/queries/overview', query);
    assert.equal(overview.state, 'ready');
    assert.equal(overview.overview.active_count, '2');
    assert.equal(overview.overview.ever_published_count, '3');
    assert.equal(overview.overview.sample_count, '2');
    const finallyVisible = await displayCheck('after recovery', () => visibleOnDisplays([13, 42]));
    assert(
      pair.records.every((r) => !r.differences.length),
      'public data or error mismatch across the frozen pair',
    );
    const executions = JSON.parse(
      sql(
        `SELECT jsonb_object_agg(consumer,n) FROM (SELECT w.consumer,count(*) n FROM event_work w JOIN event_outbox o USING(restore_epoch,event_id) WHERE o.type='monitoring.changed' AND o.scope='${tenant}' AND o.generation_or_position>${start} AND w.state='done' GROUP BY w.consumer) x`,
      ),
    );
    assert(groups.every((g) => executions[g] === tail - start));
    cases.push(
      record('ARC-022', '打乱两个发布、更正和撤销的到达顺序，暂停后四组均补齐', {
        start,
        tail,
        whilePaused,
        reached,
        delivered: publicationOrder,
        executions,
        derivedInputPosition: consumed,
        cacheFloors: floors,
        visibleValues: details.items.map((x) => Number(x.value)),
        overview: overview.overview,
        display: { before: initiallyVisible, after: finallyVisible, failures: displayFailures },
      }),
    );
    cases.at(-1).passed = displayFailures.length === 0;
    save();
    arc = 'ARC-031';
    const last = await prepare('2026-11-07T00:04:00Z', 7);
    const sides = ['a', 'b'],
      ids = [last.b.id, pair.translate(last.b.id)],
      installed = [],
      physical = {};
    try {
      for (const [i, side] of sides.entries()) {
        assert.match(ids[i], /^[0-9a-f-]{36}$/);
        sideSQL(
          side,
          `CREATE SEQUENCE arc_commit_fault_hits; CREATE FUNCTION arc_fail_ch_manifest() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$ BEGIN PERFORM nextval('public.arc_commit_fault_hits'); RAISE EXCEPTION 'private manifest fault'; END $$;
          CREATE CONSTRAINT TRIGGER arc_fail_ch_manifest AFTER INSERT ON monitoring_publications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.batch_id='${ids[i]}') EXECUTE FUNCTION arc_fail_ch_manifest()`,
        );
        installed.push(side);
      }
      await call(last.path + '/publish', meta(last.b.revision));
      for (const [i, side] of sides.entries()) {
        await until(
          'physical write precedes failed manifest ' + side,
          () => sideSQL(side, 'SELECT is_called FROM arc_commit_fault_hits') === 't',
        );
        const id = ids[i],
          chunk = sideSQL(side, `SELECT id FROM monitoring_chunks WHERE batch_id='${id}'`);
        assert.match(chunk, /^[0-9a-f-]{36}$/);
        assert.match(lab[side].ch, /^arc_http_[a-f0-9]+_[ab]$/);
        const ch = Number(
          command('docker', [
            'exec',
            lab.chContainer,
            'clickhouse-client',
            '--database',
            lab[side].ch,
            '--query',
            `SELECT count() FROM observations WHERE chunk_id='${chunk}'`,
          ]),
        );
        const chosen = Number(
          sideSQL(side, `SELECT count(*) FROM monitoring_publications WHERE batch_id='${id}'`),
        );
        assert(ch >= 1);
        assert.equal(chosen, 0);
        physical[side] = {
          storedRows: ch,
          selectedPublications: chosen,
          chunk,
        };
      }
      const before = await call(base + '/queries/details', query);
      assert.deepEqual(
        before.items.map((v) => Number(v.value)),
        [13, 42],
      );
    } finally {
      for (const side of installed)
        sideSQL(
          side,
          'DROP TRIGGER arc_fail_ch_manifest ON monitoring_publications; DROP FUNCTION arc_fail_ch_manifest(); DROP SEQUENCE arc_commit_fault_hits',
        );
    }
    await waitState(last.path, 'published');
    const repaired = await call(base + '/queries/details', query);
    assert.deepEqual(
      repaired.items.map((v) => Number(v.value)),
      [7, 13, 42],
    );
    for (const [i, side] of sides.entries()) {
      const count = Number(
        sideSQL(
          side,
          `SELECT count(*) FROM monitoring_publications p JOIN monitoring_publication_chunks c ON c.publication_id=p.id WHERE p.batch_id='${ids[i]}'`,
        ),
      );
      assert.equal(count, 1);
      physical[side].selectedAfterRepair = count;
    }
    cases.push(
      record('ARC-031', 'ClickHouse 实写后拒绝 PG 清单提交，故障消除后原任务只选中一次', {
        batch: ids,
        beforeRetry: [13, 42],
        afterRetry: [7, 13, 42],
        physical,
      }),
    );
    save();
    arc = 'ARC-024';
    const disk = () => JSON.parse(sql(`
      SELECT json_build_object('level',storage_level,'used',broker_bytes,'capacity',broker_limit_bytes)
      FROM event_pressure WHERE family='monitoring'`));
    const coldTenantRead = async () => {
      const account = read(join(dir, 'private/accounts.json')).admin;
      const fresh = await login(lab.b, account);
      try {
        const tenantPath = '/tenants/' + tenant;
        const membership = await fresh.call(tenantPath + '/context');
        const facilities = await fresh.call(tenantPath + '/engineering/resources/facilities');
        const id = sample.nodes.A.id;
        assert(membership.membership_id);
        assert(facilities.items.some((f) => f.id === id));
        const facility = await fresh.call(tenantPath + '/engineering/resources/facilities/' + id);
        assert.equal(facility.id, id);
        return { coldLogin: true, membership: membership.membership_id,
          facility: facility.id, code: facility.code, rows: facilities.items.length };
      } finally {
        await fresh.client.dispose();
      }
    };
    const isolation = await context.withDiskAndBrokerFault(async (stopBroker) => {
      const watermark = await until('observed physical hard watermark', () => {
        const state = disk();
        return state.level === 'hard' && state.capacity === 65536 && state.used * 100 >= state.capacity * 2 && state;
      });
      const published = await coldTenantRead();
      const stopped = await stopBroker();
      const offline = await coldTenantRead();
      assert.deepEqual(offline, published);
      return { hardPercent: 2, watermark, whileHard: published, stoppedBroker: stopped, whileOffline: offline };
    });
    isolation.recovered = await until('original stream capacity restored', () => {
      const state = disk();
      return state.level === 'normal' && state.capacity > isolation.watermark.capacity && state;
    });
    cases.push(record('ARC-024', '消息硬水位及代理停机期间仍可新登录、读取租户与工程台账', isolation));
    save();
    return { passed: cases.every((c) => c.passed), cases };
  } catch (e) {
    cases.push({
      arc,
      name: 'isolated ordered change workflows',
      passed: false,
      error: e.message.split('\n')[0].slice(0, 220),
    });
    save();
    throw e;
  } finally {
    await Promise.all(context.displays.map((d) => d.close()));
    await pair.close();
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const r = await changeFaults(
      runDirectory(process.argv[2]),
      process.argv[3] ? runDirectory(process.argv[3]) : undefined,
    );
    console.log('Independent role and change recovery', r.passed);
    if (!r.passed) process.exitCode = 1;
  } catch (e) {
    console.error(
      'Isolated change recovery failed',
      e.message
        .split('\n')[0]
        .replace(/https?:\/\/\S+/g, '[loopback endpoint]')
        .slice(0, 300),
    );
    process.exitCode = 1;
  }
}
