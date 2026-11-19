import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { client, digest } from './architecture-lab.mjs';

export const importFocusReason = '原批次行回焦功能验真';

export async function seedImportFocus(call, { tenant, scope, keepLastUploading = false }) {
  const prefix = '回焦批次-' + digest(scope).slice(0, 10) + '-';
  const path = '/tenants/' + tenant + '/monitoring/imports';
  const list = (query) => call(path + '?' + new URLSearchParams({ q: prefix, ...query }));
  const existing = new Map((await list({ limit: '100' })).items.map(row => [row.name, row]));
  const rows = [];
  for (let index = 0; index < 23; index++) {
    const name = prefix + String(index).padStart(2, '0') + (index % 3 === 0 ? '.xlsx' : '.csv');
    const row = existing.get(name) || await call(path, {
      request_id: randomUUID(), expected_revision: '0', reason: importFocusReason, name, size_bytes: '4',
    });
    if (row.state === 'uploading' && (index !== 22 || !keepLastUploading)) {
      const terminal = await call(path + '/' + row.id + '/cancel', {
        request_id: randomUUID(), expected_revision: row.revision, reason: '保留可回访的合成历史批次',
      });
      assert.equal(terminal.state, 'cancelled');
    }
    rows.push({ id: row.id, name });
  }
  const first = await list({ limit: '10' });
  assert.equal(first.items.length, 10);
  assert(first.next_cursor);
  const second = await list({ limit: '10', cursor: first.next_cursor });
  assert.equal(second.items.length, 10);
  const third = await list({ limit: '10', cursor: second.next_cursor });
  assert.equal(third.items.length, 3);
  const all = [...first.items, ...second.items, ...third.items];
  assert.deepEqual(new Set(all.map(row => row.id)), new Set(rows.map(row => row.id)));
  const row = second.items[0];
  const base = '/t/' + tenant + '/monitoring/imports';
  return {
    tenant, prefix, count: all.length, pageSize: 10, pageNumber: 2, rows,
    target: { id: row.id, name: row.name, domID: 'import-' + row.id },
    listPath: base + '?' + new URLSearchParams({ q: prefix, limit: '10' }),
    secondPagePath: base + '?' + new URLSearchParams({ q: prefix, limit: '10', cursor: first.next_cursor }),
    detailPath: base + '/' + row.id + '?' + new URLSearchParams({ q: prefix, limit: '10', cursor: first.next_cursor, tab: 'history' }),
    instructions: '以本侧管理员进入批次列表，点击下一页批次，打开指定批次，选择处理记录并刷新，按 Esc。焦点应回到第二页同一批次。列表仍在读取时，可点击搜索框并输入，原行出现后应继续停在搜索框。',
  };
}

export async function seedWalkthroughImportFocus({ lab, account, tenant }) {
  const session = await client(lab, account);
  try {
    return await seedImportFocus(session.call, { tenant, scope: 'walkthrough-' + tenant });
  } finally {
    await session.client.dispose();
  }
}
