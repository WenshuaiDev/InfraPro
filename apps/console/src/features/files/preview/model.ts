export const documentFormats = ['pdf', 'docx', 'xlsx', 'csv', 'txt'];
export const maxPreviewBytes = 100 * 1024 ** 2;
export const limits = { rows: 20000, columns: 200, cells: 500000, sheets: 100, textLines: 100000, textCharacters: 5000000 };
export type Encoding = 'auto' | 'utf-8' | 'utf-16le' | 'utf-16be' | 'gb18030';
export type PreviewCell = { text: string; formula?: string };
export type PreviewSheet = { name: string; rows: PreviewCell[][]; columns: number; totalRows: number; totalColumns: number; rowCountIsLowerBound?: boolean; merges: { r: number; c: number; rows: number; columns: number }[]; hidden?: boolean };
export type ParsedPreview = { kind: 'text'; lines: string[]; encoding: string; truncated: boolean } | { kind: 'table'; sheets: PreviewSheet[]; encoding?: string; delimiter?: string; warnings: string[] };
export type ParseRequest = { bytes: ArrayBuffer; format: string; encoding: Encoding; delimiter: string };

export function decodeText(bytes: ArrayBuffer, encoding: Encoding): { text: string; encoding: string } {
  const head = new Uint8Array(bytes, 0, Math.min(3, bytes.byteLength));
  let detected = encoding === 'auto' ? head[0] === 0xff && head[1] === 0xfe ? 'utf-16le' : head[0] === 0xfe && head[1] === 0xff ? 'utf-16be' : 'utf-8' : encoding;
  if (encoding === 'auto' && detected === 'utf-8') {
    try { return { text: new TextDecoder('utf-8', { fatal: true }).decode(bytes), encoding: 'utf-8' }; }
    catch { detected = 'gb18030'; }
  }
  return { text: new TextDecoder(detected).decode(bytes), encoding: detected };
}

export function columnName(index: number): string {
  let result = '';
  for (let value = index + 1; value > 0; value = Math.floor((value - 1) / 26)) result = String.fromCharCode(65 + (value - 1) % 26) + result;
  return result;
}

export function mergedCells(sheet: PreviewSheet, page: number, pageSize: number) {
  const cells = new Map<string, { rows: number; columns: number; cell?: PreviewCell } | null>();
  const first = (page - 1) * pageSize, end = first + pageSize;
  let work = 0;
  for (const merge of sheet.merges) {
    const start = Math.max(first, merge.r), last = Math.min(end, merge.r + merge.rows);
    if (start >= last) continue;
    // Invalid overlapping merges cannot spend more work than the visible grid.
    work += (last - start) * merge.columns;
    if (work > pageSize * sheet.columns) { cells.clear(); break; }
    for (let r = start; r < last; r++) for (let c = merge.c; c < merge.c + merge.columns; c++) cells.set(`${r}:${c}`, r === start && c === merge.c ? { rows: last - start, columns: merge.columns, cell: sheet.rows[merge.r]?.[merge.c] } : null);
  }
  return cells;
}
