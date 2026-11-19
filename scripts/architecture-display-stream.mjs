import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';

// The formal 100,000 observations stay fixed. This separate source spans two
// public upload parts and remains a received file throughout the read benchmark.
export async function prepareLargeSource(user, prefix) {
  const partSize = 32 * 1024 * 1024;
  const header = Buffer.from('observed_at,value,quality\n');
  const row = Buffer.from('2026-11-01T00:00:00Z,42,good\n');
  const size = header.length + Math.ceil((partSize + 1024 - header.length) / row.length) * row.length;
  const source = Buffer.allocUnsafe(size);
  header.copy(source);
  source.fill(row, header.length);
  const parts = [source.subarray(0, partSize), source.subarray(partSize)];
  const whole = createHash('sha256');
  const meta = (revision) => ({ request_id: randomUUID(), expected_revision: revision, reason: '大屏混合负载原件' });
  const batch = await user.call(prefix + '/imports', { ...meta('0'), name: 'load-stream.csv', size_bytes: String(size) });
  const path = prefix + '/imports/' + batch.id;
  for (const [index, part] of parts.entries()) {
    const hash = createHash('sha256').update(part).digest('hex');
    whole.update(part);
    const response = await user.fetch(path + '/upload/parts/' + (index + 1), {
      method: 'PUT', body: part,
      headers: { 'Content-Type': 'application/octet-stream', 'X-Expected-Revision': batch.revision, 'X-Content-SHA256': hash },
    });
    try { assert.equal(response.status, 200, 'The public import endpoint must accept each complete upload part'); }
    finally { await response.body?.cancel(); }
  }
  const sha256 = whole.digest('hex');
  const received = await user.call(path + '/upload/complete', { ...meta(batch.revision), sha256 });
  assert.equal(received.state, 'received');
  assert.match(received.source_file_id, /^[a-f0-9-]{36}$/);
  return { path: prefix + '/source-files/' + received.source_file_id + '/download', size, sha256 };
}

export async function readLargeSource(user, source) {
  const started = performance.now();
  const response = await user.fetch(source.path, { headers: { 'Accept-Encoding': 'identity' } });
  let bytes = 0, chunks = 0, firstByteMs;
  try {
    assert.equal(response.status, 200);
    assert.equal(Number(response.headers.get('content-length')), source.size);
    assert.equal(response.headers.get('x-content-sha256'), source.sha256);
    assert.equal(response.headers.get('cache-control'), 'private, no-store');
    const whole = createHash('sha256');
    for await (const chunk of response.body) {
      firstByteMs ??= performance.now() - started;
      bytes += chunk.length;
      chunks++;
      whole.update(chunk);
    }
    assert.equal(bytes, source.size);
    assert.equal(whole.digest('hex'), source.sha256);
    assert(chunks > 1, 'The protected file must be received incrementally');
    return { bytes, chunks, firstByteMs, sha256: source.sha256 };
  } finally { await response.body?.cancel().catch(() => {}); }
}
