import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { request } from '@playwright/test';
import { digest, read, root, write } from './architecture-lab.mjs';

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const embeddedID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;
const exportIdentity =
  /(?<![\da-f])(?:[a-f\d]{64}|[a-f\d]{8}(?:-[a-f\d]{4}){3}-[a-f\d]{12})(?![\da-f])/gi;
const time = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/;
function detailAnchor(value, body, direction) {
  const [payload, signature, ...rest] = value.split('.');
  if (rest.length || !signature || Buffer.from(signature, 'base64url').length !== 32) return false;
  let c;
  try {
    c = JSON.parse(Buffer.from(payload, 'base64url'));
  } catch {
    return false;
  }
  const row = direction === 'forward' ? body.items.at(-1) : body.items[0];
  if (
    !row ||
    !body.snapshot?.id ||
    c.Direction !== direction ||
    c.Snapshot !== body.snapshot.id ||
    c.Channel !== row.channel_id ||
    c.Observation !== row.observation_id ||
    c.Adoption !== row.adoption_id ||
    !/^\d+$/.test(c.At)
  )
    return false;
  return BigInt(c.At) / 1000000n === BigInt(Date.parse(row.observed_at)) &&
    /^[a-f0-9]{64}$/.test(c.View)
    ? c
    : false;
}
const clocks = new Set([
  'created_at',
  'updated_at',
  'requested_at',
  'activated_at',
  'ended_at',
  'started_at',
  'finished_at',
  'completed_at',
  'committed_at',
  'issued_at',
  'expires_at',
  'published_at',
  'recorded_at',
  'read_at',
  'as_of',
  'as_of_time',
  'generated_at',
  'observed_at',
  'last_activity_at',
  'reauthenticated_at',
  'verified_at',
  'deadline',
  'next_attempt_at',
  'computed_at',
  'uploaded_at',
  'adopted_at',
  'materialized_at',
  'validated_at',
  'snapshot_at',
  'effective_from',
  'granted_at',
]);
const derivedHash = new Set([
  'validation_hash',
  'config_hash',
  'control_hash',
  'plan_hash',
  'output_contract_hash',
  'trial_hash',
  'mapping_hash',
  'preview_hash',
  'basis_hash',
  'scope_hash',
  'request_fingerprint',
  'filter_hash',
  'input_hash',
  'definition_hash',
  'authorization_revision',
  'query_hash',
  'change_hash',
]);
// The public reader orders raw rows by channel, instant, observation and
// adoption. Normalized windows order by channel and their nullable bounds.
function compareRawKey(a, b, fields, physical) {
  for (const field of fields) {
    let l = a[field],
      r = b[field];
    if (l === r) continue;
    if (l == null) return field === 'to_ns' ? 1 : -1;
    if (r == null) return field === 'to_ns' ? -1 : 1;
    if (field.endsWith('_ns')) {
      l = BigInt(l);
      r = BigInt(r);
    } else if (physical && field.endsWith('_id')) {
      // ClickHouse's UUID sorting compares the last eight bytes first. The
      // Go-normalized range list uses canonical UUID strings instead.
      const sortKey = (id) =>
        id.slice(19).replaceAll('-', '') + id.slice(0, 18).replaceAll('-', '');
      l = sortKey(l);
      r = sortKey(r);
    }
    return l < r ? -1 : 1;
  }
  return 0;
}
const routes = readFileSync(join(root, 'services/api/internal/routes/ownership.tsv'), 'utf8')
  .trimEnd()
  .split('\n')
  .map((line) => {
    const [method, template, owner, operationId] = line.split('\t');
    return { method, template, owner, operationId, parts: template.split('/') };
  });
export function operation(method, path) {
  const parts = path.split('/');
  let chosen,
    rank = -1;
  for (const r of routes) {
    if (r.method !== method || r.parts.length !== parts.length) continue;
    let score = 0,
      ok = true;
    for (let i = 0; i < parts.length; i++) {
      score *= 2;
      if (r.parts[i] === parts[i]) score++;
      else if (!r.parts[i].startsWith(':') || !parts[i]) {
        ok = false;
        break;
      }
    }
    if (ok && score > rank) {
      chosen = r;
      rank = score;
    }
  }
  return chosen;
}
// New public resource identities are paired at their issuing command and must
// then remain bijective across all subsequent calls. Snapshot identities,
// observation times, row order, numeric values and binary digests stay exact.
export class PairedHTTP {
  constructor(dir, name, output = dir) {
    const lab = read(join(dir, 'private/lab.json'));
    this.sides = [lab.a, lab.b];
    for (const side of this.sides) {
      for (const url of [side.origin, side.api]) assert.equal(new URL(url).hostname, '127.0.0.1');
    }
    this.origin = lab.a.origin;
    this.dir = dir;
    this.output = output;
    this.name = name;
    this.start = Date.now();
    const snapshot = join(dir, 'snapshot.json');
    this.fixture = existsSync(snapshot) ? read(snapshot) : undefined;
    this.aToB = new Map();
    this.bToA = new Map();
    this.created = new Map();
    this.clocks = new Map();
    this.records = [];
    this.contexts = [];
    this.problems = [];
  }
  bind(a, b) {
    if (a === b) return true;
    if (this.aToB.has(a)) return this.aToB.get(a) === b;
    if (this.bToA.has(b)) return false;
    this.aToB.set(a, b);
    this.bToA.set(b, a);
    return true;
  }
  translate(value) {
    if (typeof value === 'string') {
      if (this.aToB.has(value)) return this.aToB.get(value);
      return value
        .replaceAll(this.sides[0].origin, this.sides[1].origin)
        .replace(embeddedID, (id) => this.aToB.get(id) || id);
    }
    if (Array.isArray(value)) return value.map((v) => this.translate(v));
    if (Buffer.isBuffer(value) || value === null || typeof value !== 'object') return value;
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [this.translate(k), this.translate(v)]),
    );
  }
  verify(a, b, op, at) {
    const changes = [],
      mapped = [],
      mappingReasons = {};
    const recordMapping = (field, reason) => {
      mapped.push(field);
      mappingReasons[field] = reason;
    };
    if (op.operationId.startsWith('Derived:') && op.operationId.endsWith(':channels.create')) {
      for (const [l, r] of [
        [a?.channel?.resource?.id, b?.channel?.resource?.id],
        [a?.channel?.draft_config_id, b?.channel?.draft_config_id],
      ]) {
        if (uuid.test(l) && uuid.test(r))
          this.created.set(l, {
            peer: r,
            from: at - 1000,
            to: Date.now() + 1000,
          });
      }
    }
    const sortedIDs = (list) =>
      list.every((row, i) => uuid.test(row.id) && (!i || list[i - 1].id < row.id));
    const generatedNS = (value) =>
      /^\d{19}$/.test(value) &&
      Number(BigInt(value) / 1000000n) >= this.start - 1000 &&
      Number(BigInt(value) / 1000000n) <= Date.now() + 1000;
    const check = (x, y, loc, key, object, peer) => {
      if (Object.is(x, y)) return;
      if (typeof x === 'string' && typeof y === 'string') {
        if (this.translate(x) === y) {
          recordMapping(loc, 'Previously paired value, embedded identity or environment origin');
          return;
        }
        if (
          op.operationId.startsWith('Monitoring:') &&
          op.operationId.endsWith(':details') &&
          ['next_cursor', 'prev_cursor'].includes(key)
        ) {
          const direction = key === 'next_cursor' ? 'forward' : 'backward',
            l = detailAnchor(x, a, direction),
            r = detailAnchor(y, b, direction);
          if (
            l &&
            r &&
            l.View === r.View &&
            l.At === r.At &&
            this.translate(l.Channel) === r.Channel &&
            this.translate(l.Observation) === r.Observation &&
            this.translate(l.Adoption) === r.Adoption &&
            this.translate(l.Snapshot) === r.Snapshot &&
            this.bind(x, y)
          ) {
            recordMapping(
              loc,
              'Signed details cursor with matching direction, snapshot and row anchor',
            );
            return;
          }
        }
        if ((clocks.has(key) || key === 'revoked_at') && time.test(x) && time.test(y)) {
          const left = Date.parse(x),
            right = Date.parse(y),
            clockKey = JSON.stringify([object, peer, key, x]);
          const command = this.created.get(object);
          const issued =
            command?.peer === peer &&
            command !== undefined &&
            ['created_at', 'updated_at'].includes(key) &&
            Math.min(left, right) >= command.from &&
            Math.max(left, right) <= command.to;
          if (
            (object && this.clocks.get(clockKey) === y) ||
            issued ||
            (Math.min(left, right) >= this.start - 1000 &&
              Math.abs(left - right) <= Math.max(2000, Date.now() - at + 1000))
          ) {
            if (object && peer) this.clocks.set(clockKey, y);
            recordMapping(
              loc,
              'Issued timestamp in the command window or a previously matched timestamp of this object',
            );
            return;
          }
        }
        if (
          uuid.test(x) &&
          uuid.test(y) &&
          (key === 'id' ||
            key.endsWith('_id') ||
            ['generation', 'release', 'instance', 'config', 'subject', 'actor', 'target'].includes(
              key,
            )) &&
          this.bind(x, y)
        ) {
          recordMapping(loc, 'Allocated UUID with one-to-one correspondence');
          return;
        }
        if (
          derivedHash.has(key) &&
          /^[a-f0-9]{64}$/.test(x) &&
          /^[a-f0-9]{64}$/.test(y) &&
          this.bind(x, y)
        ) {
          recordMapping(
            loc,
            'Allowlisted fingerprint with one-to-one correspondence across the fixture',
          );
          return;
        }
        const derived = op.operationId.startsWith('Derived:');
        if (
          ((derived &&
            [
              'compiled_plan_hash',
              'content_hash',
              'evidence_hash',
              'lineage_id',
              'lineage_root',
              'lineage_ref',
              'lineage_hash',
              'manifest_hash',
              'manifest_root',
              'hash',
            ].includes(key)) ||
            (key === 'hash' && op.operationId.endsWith(':selections.create')) ||
            (op.operationId.startsWith('Device:') &&
              (key === 'semantic_hash' || (key === 'digest' && loc.includes('/definitions/')))) ||
            (key === 'payload_hash' && op.operationId.endsWith(':imports.issues'))) &&
          /^[a-f0-9]{64}$/.test(x) &&
          /^[a-f0-9]{64}$/.test(y) &&
          this.bind(x, y)
        ) {
          recordMapping(loc, 'Operation-specific generated digest with one-to-one correspondence');
          return;
        }
        if (
          derived &&
          key === 'effective_at_ns' &&
          generatedNS(x) &&
          generatedNS(y) &&
          Math.abs(Number((BigInt(x) - BigInt(y)) / 1000000n)) <= Date.now() - at + 1500 &&
          this.bind(x, y)
        ) {
          recordMapping(loc, 'Effective nanosecond timestamp issued during this trial');
          return;
        }
        if (
          key === 'token' &&
          op.operationId === 'getCSRF' &&
          x.length >= 32 &&
          x.length === y.length &&
          this.bind(x, y)
        ) {
          recordMapping(loc, 'Equally sized CSRF credentials issued by the two origins');
          return;
        }
        if (
          key === 'url' &&
          x.startsWith(this.sides[0].origin) &&
          y.startsWith(this.sides[1].origin)
        ) {
          const left = new URL(x),
            right = new URL(y);
          const leftHash = new URLSearchParams(left.hash.slice(1)),
            rightHash = new URLSearchParams(right.hash.slice(1));
          if (
            left.pathname === right.pathname &&
            left.search === right.search &&
            leftHash.size === 1 &&
            rightHash.size === 1 &&
            leftHash.has('token') &&
            rightHash.has('token')
          ) {
            const l = leftHash.get('token'),
              r = rightHash.get('token');
            if (l.length === r.length && l.length >= 20 && this.bind(l, r) && this.bind(x, y)) {
              recordMapping(
                loc,
                'Invitation URL with matching path and query and independently issued one-time credentials',
              );
              return;
            }
          }
        }
      }
      const structuralGrants =
        key === 'grants' &&
        op.method === 'GET' &&
        op.template.endsWith('/resource-access/facilities/:facility_id');
      const dataGrants =
        key === 'items' &&
        op.operationId.endsWith(':access.list') &&
        op.operationId.startsWith('Monitoring:');
      const definitions = key === 'definitions' && op.operationId.startsWith('Device:');
      const assignments =
        key === 'items' &&
        op.operationId.startsWith('Derived:') &&
        op.operationId.endsWith(':channels.assignments');
      const derived = op.operationId.startsWith('Derived:');
      const ranges = derived && ['input_ranges', 'raw_input_ranges'].includes(key);
      const raw = derived && key === 'input_examples';
      if ((ranges || raw) && Array.isArray(x) && Array.isArray(y)) {
        const fields = ranges
          ? ['channel_id', 'from_ns', 'to_ns']
          : ['channel_id', 'observed_at_ns', 'observation_id', 'adoption_id'];
        const ordered = (rows) =>
          rows.every(
            (row, i) =>
              uuid.test(row.channel_id) && (!i || compareRawKey(rows[i - 1], row, fields, raw) < 0),
          );
        if (x.length !== y.length || !ordered(x) || !ordered(y)) {
          changes.push(loc + '/order');
          return;
        }
        // These fixed inputs have at most one observation per channel/instant.
        // Repeated keys require a previously proven observation identity.
        const identify = (row, side) =>
          JSON.stringify(
            (ranges ? fields : fields.slice(0, 2)).map((k) =>
              side === 'a' ? this.translate(row[k]) : row[k],
            ),
          );
        const peers = new Map(y.map((row) => [identify(row, 'b'), row]));
        if (peers.size !== y.length) {
          changes.push(loc + '/ambiguous_identity');
          return;
        }
        for (const [i, row] of x.entries()) {
          const id = identify(row, 'a'),
            other = peers.get(id);
          if (!other) {
            changes.push(loc + '/' + i);
            continue;
          }
          check(row, other, loc + '/' + i, '');
          peers.delete(id);
        }
        if (peers.size) changes.push(loc + '/unpaired');
        return;
      }
      if (
        (structuralGrants || dataGrants || definitions || assignments) &&
        Array.isArray(x) &&
        Array.isArray(y)
      ) {
        // The public cursor pages by grant UUID. Newly allocated keys can have
        // different lexical order on each side. Verify both original orders,
        // then compare each grant against its already paired resource identity.
        if (!sortedIDs(x) || !sortedIDs(y) || x.length !== y.length) {
          changes.push(loc + '/order');
          return;
        }
        const identity = (row) =>
          assignments
            ? JSON.stringify([
                row.config_id,
                row.effective_range.from_ns,
                row.effective_range.to_ns,
                row.created_revision,
              ])
            : structuralGrants
              ? row.kind + '/' + row.resource_id
              : row.id;
        const peers = new Map(y.map((row) => [identity(row), row]));
        if (peers.size !== y.length) {
          changes.push(loc + '/ambiguous_identity');
          return;
        }
        for (const [i, row] of x.entries()) {
          const k = this.translate(identity(row)),
            peer = peers.get(k);
          if (!peer) {
            changes.push(loc + '/' + i);
            continue;
          }
          check(row, peer, loc + '/' + i, '');
          peers.delete(k);
        }
        if (peers.size) changes.push(loc + '/unpaired');
        else
          recordMapping(
            loc + '/uuid_order',
            'Both sides satisfy the public UUID order and contain matching rows by business identity',
          );
        return;
      }
      if (
        x &&
        y &&
        typeof x === 'object' &&
        typeof y === 'object' &&
        Array.isArray(x) === Array.isArray(y)
      ) {
        const keys = Object.keys(x).sort(
          (l, r) => Number(l.endsWith('_cursor')) - Number(r.endsWith('_cursor')),
        );
        const right = new Set(Object.keys(y));
        for (const k of keys) {
          const translated = this.translate(k);
          if (!right.delete(translated)) {
            changes.push(loc + '/' + k);
            continue;
          }
          check(x[k], y[translated], loc + '/' + k, k, x.id, y.id);
        }
        for (const k of right) changes.push(loc + '/' + k);
        return;
      }
      changes.push(loc);
    };
    check(a, b, '', '');
    return { differences: changes, mapped, mappingReasons };
  }
  async login(account) {
    const c = await this.client(),
      api = this.origin + '/api/v1',
      csrf = await (await c.get(api + '/auth/csrf')).json();
    const headers = { Origin: this.origin, 'X-CSRF-Token': csrf.token };
    const call = async (path, data, method = data === undefined ? 'GET' : 'POST') => {
      const r = await c.fetch(api + path, { method, headers, data });
      if (!r.ok()) {
        const problem = await r.json().catch(() => ({}));
        throw Error(
          `${method} ${operation(method, new URL(api + path).pathname)?.template} ${r.status()} ${problem.code || ''} ${problem.issues?.map((i) => i.path + ' ' + i.code).join(',') || ''}`,
        );
      }
      return r.status() === 204 ? {} : r.json();
    };
    const proof = await call('/auth/login', account);
    assert.equal(proof.status, 'authenticated');
    return { call, client: c, headers, api };
  }
  async client() {
    const clients = await Promise.all(
      this.sides.map(() => request.newContext({ timeout: 35000, maxRedirects: 0 })),
    );
    this.contexts.push(...clients);
    const call = async (method, url, options = {}) => {
      const u = new URL(url, this.origin);
      assert.equal(u.origin, this.origin);
      const op = operation(method, u.pathname);
      assert.ok(op, method + ' ' + u.pathname);
      const paths = [u.pathname + u.search, this.translate(u.pathname + u.search)],
        command = options.headers?.['X-Command-ID'] || randomUUID();
      const at = Date.now(),
        observations = [[], []],
        until = options.pairedWait;
      const playwright = { ...options };
      delete playwright.pairedWait;
      delete playwright.exportParts;
      const fetchSide = async (i) => {
        try {
          return await clients[i].fetch(this.sides[i].api + paths[i], {
            ...playwright,
            method,
            headers: {
              ...(i ? this.translate(options.headers || {}) : options.headers),
              'X-Command-ID': command,
            },
            data: i ? this.translate(options.data) : options.data,
          });
        } catch (error) {
          // Playwright includes request headers in its exceptions. Only export the
          // error class to the verification log.
          // eslint-disable-next-line preserve-caught-error -- Playwright's original cause includes private headers.
          throw Error(`Transport ${error.name} on ${i ? 'B' : 'A'} at ${method} ${op.template}`);
        }
      };
      const get = async (i) => {
        let response, bytes, value;
        for (;;) {
          response = await fetchSide(i);
          bytes = await response.body();
          value =
            response.headers()['content-type']?.includes('json') && bytes.length
              ? JSON.parse(bytes)
              : bytes;
          if (!until || !response.ok()) break;
          const observed = until.pick(value);
          observations[i].push({ state: observed, ms: Date.now() - at });
          if (
            observed === until.want ||
            ['failed', 'cancelled', 'canceled', 'needs_action'].includes(observed) ||
            Date.now() - at >= 120000
          )
            break;
          await response.dispose();
          await new Promise((r) => setTimeout(r, 200));
        }
        return { response, bytes, value };
      };
      const result = await Promise.all([get(0), get(1)]),
        replies = result.map((v) => v.response),
        raw = result.map((v) => v.bytes),
        contents = result.map((v) => v.value);
      const exports =
        op.operationId.startsWith('Derived:') &&
        op.operationId.endsWith('exports.files') &&
        contents.every((v) => v?.format === 'csv' && Array.isArray(v.files));
      if (exports && contents[0].files.length === contents[1].files.length) {
        // Establish source/row identities from public metadata and earlier reads.
        this.verify(contents[0].source, contents[1].source, op, at);
        for (const [i, part] of contents[0].files.entries()) {
          const peer = contents[1].files[i];
          assert.equal(part.part, peer.part);
          const file = await call('GET', url + '/' + part.part, {
            headers: options.headers,
            exportParts: [part, peer],
          });
          await file.dispose();
        }
      }
      let compared;
      if (Buffer.isBuffer(contents[0]) && options.exportParts) {
        const ok = raw.every(
          (bytes, i) =>
            digest(bytes) === options.exportParts[i].sha256 &&
            bytes.length === Number(options.exportParts[i].size_bytes) &&
            replies[i].ok(),
        );
        const decoded = raw.map((bytes) =>
          new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes),
        );
        const expected = decoded[0].replace(exportIdentity, (id) => this.aToB.get(id) || id);
        if (ok && expected === decoded[1] && this.bind(...options.exportParts.map((p) => p.sha256)))
          compared = {
            differences: [],
            mapped: ['/csv/issued_identities'],
            mappingReasons: {
              '/csv/issued_identities':
                'Only identities previously paired in public responses are substituted',
            },
            verified: [
              'both file SHA-256 and lengths',
              'every UTF-8 CSV byte after substituting previously verified identities',
            ],
          };
        else
          compared = {
            differences: [ok ? '/csv/content' : '/manifest/digest_or_length'],
            mapped: [],
          };
      } else
        compared = Buffer.isBuffer(contents[0])
          ? {
              differences: contents[0].equals(contents[1]) ? [] : ['/bytes'],
              mapped: [],
            }
          : this.verify(contents[0], contents[1], op, at);
      if (
        contents[0]?.code &&
        contents[0]?.request_id === replies[0].headers()['x-request-id'] &&
        contents[1]?.request_id === replies[1].headers()['x-request-id']
      ) {
        if (compared.differences.includes('/request_id')) {
          compared.mapped.push('/request_id');
          (compared.mappingReasons ??= {})['/request_id'] =
            'Each error body carries the request ID emitted by its own HTTP response';
        }
        compared.differences = compared.differences.filter((p) => p !== '/request_id');
      }
      const status = replies.map((r) => r.status());
      if (status[0] !== status[1]) compared.differences.unshift('status');
      const entry = {
        method,
        template: op.template,
        operationId: op.operationId,
        owner: op.owner,
        command,
        status,
        errors: contents.map((v) => v?.code || null),
        issues: contents.map((v) =>
          Array.isArray(v?.issues) ? v.issues.slice(0, 4).map((i) => i.path) : [],
        ),
        sha256: raw.map(digest),
        ...compared,
        ...(until && { expectedState: until.want, observations }),
        elapsedMs: Date.now() - at,
      };
      this.records.push(entry);
      this.save();
      await replies[1].dispose();
      if (until && observations.some((list) => list.at(-1)?.state !== until.want))
        throw Error(`Terminal state mismatch at ${method} ${op.template}, expected ${until.want}`);
      if (status[0] !== status[1])
        throw Error(`HTTP parity at ${method} ${op.template} (${status.join('/')})`);
      if (
        status[0] === 200 &&
        op.operationId.startsWith('Monitoring:') &&
        op.operationId.endsWith(':details') &&
        !options.data?.cursor
      ) {
        for (const [direction, key] of [
          ['forward', 'next_cursor'],
          ['backward', 'prev_cursor'],
        ]) {
          const token = contents[0][key];
          if (!token) continue;
          const page = await call(method, url, {
            ...options,
            data: {
              ...options.data,
              snapshot_id: contents[0].snapshot.id,
              cursor: token,
              direction,
            },
          });
          try {
            assert.equal(page.status(), 200, 'signed details cursor rejected');
            const more = await page.json(),
              prior = new Set(contents[0].items.map((v) => v.observation_id + '/' + v.adoption_id));
            assert.ok(
              more.items.every((v) => !prior.has(v.observation_id + '/' + v.adoption_id)),
              'overlapping details pages',
            );
          } finally {
            await page.dispose();
          }
        }
      }
      return replies[0];
    };
    return {
      wait: async (url, headers, pick, want) =>
        (await call('GET', url, { headers, pairedWait: { pick, want } })).json(),
      fetch: (url, opt = {}) => call(opt.method || 'GET', url, opt),
      get: (u, o) => call('GET', u, o),
      put: (u, o) => call('PUT', u, o),
      post: (u, o) => call('POST', u, o),
      delete: (u, o) => call('DELETE', u, o),
      head: (u, o) => call('HEAD', u, o),
      dispose: () => Promise.all(clients.map((c) => c.dispose())),
    };
  }
  save() {
    write(join(this.output, 'http', 'scenarios', this.name + '.json'), {
      scenario: this.name,
      inputs: 'paired public sessions and command IDs',
      fixture: this.fixture,
      startedAt: new Date(this.start).toISOString(),
      observedAt: new Date().toISOString(),
      records: this.records,
      failures: this.problems,
    });
  }
  async close() {
    await Promise.all(this.contexts.map((c) => c.dispose()));
    this.save();
  }
}
