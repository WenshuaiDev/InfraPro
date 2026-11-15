import Papa from 'papaparse';
import * as XLSX from 'xlsx';
import { decodeText, limits, type ParseRequest, type ParsedPreview, type PreviewCell, type PreviewSheet } from './model';

export function parseDocument({ bytes, format, encoding, delimiter }: ParseRequest): ParsedPreview {
  if (format === 'txt') {
    const decoded = decodeText(bytes, encoding), text = decoded.text.slice(0, limits.textCharacters), all = text.replace(/\r\n?/g, '\n').split('\n');
    return { kind: 'text', lines: all.slice(0, limits.textLines), encoding: decoded.encoding, truncated: text.length < decoded.text.length || all.length > limits.textLines };
  }
  if (format === 'csv') {
    const decoded = decodeText(bytes, encoding), parsed = Papa.parse<string[]>(decoded.text, { delimiter, skipEmptyLines: false, preview: limits.rows + 1, dynamicTyping: false });
    const warnings: string[] = [];
    if (parsed.errors.some(error => error.type === 'Quotes')) throw Error('CSV 引号不完整，无法可靠解析。请下载原件检查。');
    let rows = parsed.data;
    if (rows.at(-1)?.length === 1 && rows.at(-1)?.[0] === '' && /[\r\n]$/.test(decoded.text)) rows = rows.slice(0, -1);
    const totalColumns = rows.reduce((max, row) => Math.max(max, row.length), 0), totalRows = rows.length;
    let budget = limits.cells;
    const output: PreviewCell[][] = [];
    for (const row of rows.slice(0, limits.rows)) {
      if (budget < Math.min(totalColumns, limits.columns)) break;
      output.push(row.slice(0, limits.columns).map(text => ({ text }))); budget -= Math.min(totalColumns, limits.columns);
    }
    if (parsed.meta.truncated || output.length < totalRows || totalColumns > limits.columns) warnings.push(`当前显示前 ${output.length} 行、最多 ${limits.columns} 列；完整内容请下载原件。`);
    return { kind: 'table', encoding: decoded.encoding, delimiter: parsed.meta.delimiter, warnings, sheets: [{ name: 'CSV 数据', rows: output, columns: Math.min(totalColumns, limits.columns), totalRows, totalColumns, rowCountIsLowerBound: parsed.meta.truncated, merges: [] }] };
  }
  if (format !== 'xlsx') throw Error('未支持的阅读格式。');
  const book = XLSX.read(bytes, { type: 'array', sheetRows: limits.rows + 1, cellFormula: true, cellHTML: false, cellStyles: false, cellText: true, bookVBA: false, bookDeps: false, WTF: false });
  const sheets: PreviewSheet[] = [], warnings: string[] = [];
  let budget = limits.cells;
  for (const name of book.SheetNames.slice(0, limits.sheets)) {
    const sheet = book.Sheets[name];
    if (!sheet) throw Error('工作表内容缺失，请下载原件检查。');
    const range = XLSX.utils.decode_range(sheet['!fullref'] || sheet['!ref'] || 'A1:A1');
    const totalRows = sheet['!ref'] ? range.e.r + 1 : 0, totalColumns = sheet['!ref'] ? range.e.c + 1 : 0;
    const columns = Math.min(totalColumns, limits.columns), count = columns ? Math.min(totalRows, limits.rows, Math.floor(budget / columns)) : 0;
    const rows: PreviewCell[][] = [];
    for (let r = 0; r < count; r++) {
      const row: PreviewCell[] = [];
      for (let c = 0; c < columns; c++) {
        const cell = sheet[XLSX.utils.encode_cell({ r, c })] as XLSX.CellObject | undefined;
        row.push({ text: cell ? cell.v === undefined && cell.f ? '（未保存计算结果）' : XLSX.utils.format_cell(cell) : '', ...(cell?.f ? { formula: cell.f } : {}) });
      }
      rows.push(row);
    }
    budget -= count * columns;
    sheets.push({ name, rows, columns, totalRows, totalColumns, hidden: !!book.Workbook?.Sheets?.find(sheet => sheet.name === name)?.Hidden,
      merges: (sheet['!merges'] ?? []).filter(merge => merge.s.r < count && merge.s.c < columns).slice(0, limits.cells).map(merge => ({ r: merge.s.r, c: merge.s.c, rows: Math.min(merge.e.r + 1, count) - merge.s.r, columns: Math.min(merge.e.c + 1, columns) - merge.s.c })) });
    if (count < totalRows || columns < totalColumns) warnings.push(`“${name}”显示 ${count}/${totalRows} 行、${columns}/${totalColumns} 列；完整内容请下载原件。`);
  }
  if (book.SheetNames.length > limits.sheets) warnings.push(`仅载入前 ${limits.sheets} 个工作表。`);
  if (!sheets.length) throw Error('工作簿中没有可读取的工作表。');
  return { kind: 'table', sheets, warnings };
}
