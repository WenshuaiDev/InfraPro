import test from 'node:test';
import assert from 'node:assert/strict';
import { registerHooks, createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
const root = new URL('../../', import.meta.url).href;
registerHooks({ resolve(s, c, next) { if (s.startsWith('.') && !/\.(ts|js|mjs)$/.test(s) && c.parentURL?.startsWith(root) && c.parentURL.endsWith('.ts')) return next(new URL(s + '.ts', c.parentURL).href, c); return next(s, c); }, load(url, c, next) { if (url.startsWith(root) && url.endsWith('.ts')) return { format: 'module', shortCircuit: true, source: ts.transpileModule(readFileSync(new URL(url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText }; return next(url, c); } });
const require = createRequire(new URL('../../apps/console/package.json', import.meta.url));
const XLSX = require('xlsx');
const { parseDocument } = await import('../../apps/console/src/features/files/preview/parse.ts');
const { decodeText, mergedCells } = await import('../../apps/console/src/features/files/preview/model.ts');
const { fileMethods } = await import('../../packages/ts/api-client/src/files.ts');
const bytes = text => new TextEncoder().encode(text).buffer;

test('CSV preserves quoted newlines, escaped quotes, leading zeros and formula-like plain text', () => {
  const parsed = parseDocument({ bytes: bytes('\ufeff编号,备注,值\r\n001,"中文,内容",=1+1\r\n002,"两行\n双""引号",0\r\n'), format: 'csv', encoding: 'auto', delimiter: '' });
  assert.equal(parsed.sheets[0].rows.length, 3);
  assert.equal(parsed.sheets[0].rows[1][0].text, '001');
  assert.equal(parsed.sheets[0].rows[1][2].text, '=1+1');
  assert.equal(parsed.sheets[0].rows[2][1].text, '两行\n双"引号');
  assert.throws(() => parseDocument({ bytes: bytes('a,b\n1,"broken'), format: 'csv', encoding: 'auto', delimiter: ',' }), /引号/);
});
test('encoding detection supports UTF-16 BOM, GB18030 fallback and explicit override', () => {
  const utf16 = Buffer.concat([Buffer.from([0xff, 0xfe]), Buffer.from('中文\r\n文本', 'utf16le')]);
  assert.equal(decodeText(Uint8Array.from(utf16).buffer, 'auto').text, '中文\r\n文本');
  assert.equal(decodeText(Uint8Array.from([0xd6, 0xd0, 0xce, 0xc4]).buffer, 'auto').text, '中文');
  const parsed = parseDocument({ bytes: Uint8Array.from(utf16).buffer, format: 'txt', encoding: 'auto', delimiter: '' });
  assert.deepEqual(parsed.lines, ['中文', '文本']);
});
test('workbooks retain sheet identity, formatted values, saved formulas and merged regions', () => {
  const book = XLSX.utils.book_new(), sheet = XLSX.utils.aoa_to_sheet([['标题', '值'], ['P-01', 42]]);
  sheet.B2.f = 'SUM(B3:B4)';
  XLSX.utils.book_append_sheet(book, sheet, '观测');
  const second = XLSX.utils.aoa_to_sheet([['汇总', '', ''], ['<script>literal</script>']]);
  second['!merges'] = [{ s: { r: 0, c: 0 }, e: { r: 0, c: 2 } }];
  XLSX.utils.book_append_sheet(book, second, '汇总');
  const parsed = parseDocument({ bytes: XLSX.write(book, { type: 'array', bookType: 'xlsx' }), format: 'xlsx', encoding: 'auto', delimiter: '' });
  assert.deepEqual(parsed.sheets.map(s => s.name), ['观测', '汇总']);
  assert.deepEqual(parsed.sheets[0].rows[1][1], { text: '42', formula: 'SUM(B3:B4)' });
  assert.deepEqual(parsed.sheets[1].merges[0], { r: 0, c: 0, rows: 1, columns: 3 });
  assert.equal(parsed.sheets[1].rows[1][0].text, '<script>literal</script>');
});
test('wide CSV and long text report explicit bounded preview instead of silent truncation', () => {
  const table = parseDocument({ bytes: bytes(Array.from({ length: 250 }, (_, i) => String(i)).join(',')), format: 'csv', encoding: 'auto', delimiter: ',' });
  assert.equal(table.sheets[0].columns, 200); assert.equal(table.warnings.length, 1);
  const text = parseDocument({ bytes: bytes('line\n'.repeat(100005)), format: 'txt', encoding: 'auto', delimiter: '' });
  assert.equal(text.lines.length, 100000); assert.equal(text.truncated, true);
  const longCSV = parseDocument({ bytes: bytes('a,b\n'.repeat(25000)), format: 'csv', encoding: 'auto', delimiter: ',' });
  assert.equal(longCSV.sheets[0].rowCountIsLowerBound, true); assert.equal(longCSV.sheets[0].rows.length, 20000);
});
test('cross-page merged cells retain their source and overlapping merges have bounded work', () => {
  const sheet = { rows: Array.from({ length: 100 }, () => [{ text: '' }]), columns: 1, merges: [{ r: 44, c: 0, rows: 11, columns: 1 }] };
  sheet.rows[44][0].text = '跨页合并内容';
  const second = mergedCells(sheet, 2, 50);
  assert.equal(second.get('50:0').cell.text, '跨页合并内容'); assert.equal(second.get('50:0').rows, 5); assert.equal(second.get('51:0'), null);
  sheet.merges = Array.from({ length: 10000 }, () => ({ r: 0, c: 0, rows: 100, columns: 1 }));
  assert.equal(mergedCells(sheet, 1, 50).size, 0);
});
test('document reads use the authorized immutable content endpoint and preserve abort signal', async () => {
  const calls = [], api = fileMethods(async (...args) => { calls.push(args); return new Blob(['value']); }, '/api/v1');
  const signal = new AbortController().signal;
  await api.fileContent({ tenant: 't', project: 'p' }, 'a', 'r', signal);
  assert.equal(calls[0][0], '/tenants/t/projects/p/files/attachments/a/revisions/r/content');
  assert.equal(calls[0][4], signal); assert.equal(calls[0][5].responseType, 'blob'); assert.equal(calls[0][5].timeoutMs, 120000);
});
