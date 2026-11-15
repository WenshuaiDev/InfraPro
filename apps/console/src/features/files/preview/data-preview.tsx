import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@infrapro/ui/components/button';
import { Highlight, PageControls, PreviewFrame, SearchControl, ZoomControls } from './controls';
import { columnName, mergedCells, type Encoding, type ParsedPreview, type PreviewSheet } from './model';

const selectClass = 'h-9 max-w-full rounded-md border bg-background px-2 text-xs focus-visible:outline-2';

export default function DataPreview({ blob, format, title }: { blob: Blob; format: string; title: string }) {
  const [encoding, setEncoding] = useState<Encoding>('auto'), [delimiter, setDelimiter] = useState(''), [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<{ data?: ParsedPreview; error?: string; key: string }>();
  const key = `${encoding}:${delimiter}:${attempt}`;
  useEffect(() => {
    const worker = new Worker(new URL('./parse.worker.ts', import.meta.url), { type: 'module' });
    let stopped = false;
    const timeout = setTimeout(() => { worker.terminate(); if (!stopped) setResult({ key, error: '解析超过 30 秒，请下载原件查看。' }); }, 30000);
    worker.onmessage = (event: MessageEvent<{ result?: ParsedPreview; error?: string }>) => { clearTimeout(timeout); worker.terminate(); if (!stopped) setResult({ key, data: event.data.result, error: event.data.error }); };
    worker.onerror = () => { clearTimeout(timeout); worker.terminate(); if (!stopped) setResult({ key, error: '阅读器未能解析文件，请重试或下载原件。' }); };
    void blob.arrayBuffer().then(bytes => { if (!stopped) worker.postMessage({ bytes, format, encoding, delimiter }, [bytes]); }).catch(() => { if (!stopped) setResult({ key, error: '文件内容读取失败。' }); });
    return () => { stopped = true; clearTimeout(timeout); worker.terminate(); };
  }, [blob, format, encoding, delimiter, key]);
  const data = result?.key === key ? result.data : undefined;
  const options = <>{format !== 'xlsx' && <select className={selectClass} aria-label="文本编码" value={encoding} onChange={event => setEncoding(event.target.value as Encoding)}>{[['auto', '自动识别编码'], ['utf-8', 'UTF-8'], ['gb18030', 'GB18030 / GBK'], ['utf-16le', 'UTF-16 LE'], ['utf-16be', 'UTF-16 BE']].map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>}{format === 'csv' && <select className={selectClass} aria-label="CSV 分隔符" value={delimiter} onChange={event => setDelimiter(event.target.value)}>{[['', '自动分隔符'], [',', '逗号'], [';', '分号'], ['\t', '制表符'], ['|', '竖线']].map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>}</>;
  if (!data) return <PreviewFrame title={title} tools={options}><div className="p-6 text-sm" role={result?.key === key && result.error ? 'alert' : 'status'}>{result?.key === key && result.error ? <>{result.error}<Button variant="outline" className="ml-2" onClick={() => setAttempt(value => value + 1)}>重试预览</Button></> : '正在解析文件…'}</div></PreviewFrame>;
  return data.kind === 'text' ? <TextReader key={key} title={title} data={data} options={options} /> : <TableReader key={key} title={title} data={data} options={options} />;
}

function TextReader({ title, data, options }: { title: string; data: Extract<ParsedPreview, { kind: 'text' }>; options: React.ReactNode }) {
  const [page, setPage] = useState(1), [query, setQuery] = useState(''), [zoom, setZoom] = useState(1), [wrap, setWrap] = useState(true), [match, setMatch] = useState(0);
  const search = useDeferredValue(query), viewport = useRef<HTMLDivElement>(null), size = 300;
  const matches = useMemo(() => search ? data.lines.flatMap((line, index) => line.toLocaleLowerCase().includes(search.toLocaleLowerCase()) ? [index] : []) : [], [data.lines, search]);
  const count = Math.max(1, Math.ceil(data.lines.length / size));
  const jump = (value: number) => { if (!matches.length) return; const next = (value + matches.length) % matches.length, line = matches[next]; if (line === undefined) return; setMatch(next); setPage(Math.floor(line / size) + 1); requestAnimationFrame(() => viewport.current?.querySelector(`[data-line="${line}"]`)?.scrollIntoView({ block: 'center' })); };
  return <PreviewFrame title={title} tools={<><PageControls page={page} count={count} onPage={value => { setPage(value); viewport.current?.scrollTo(0, 0); }} /><ZoomControls zoom={zoom} onZoom={setZoom} /><SearchControl value={query} onChange={value => { setQuery(value); setMatch(-1); }} onNext={() => jump(match + 1)} onPrevious={() => jump(match - 1)} count={`${matches.length ? Math.max(0, match + 1) : 0} / ${matches.length} 行`} /><Button size="sm" variant={wrap ? 'secondary' : 'ghost'} aria-pressed={wrap} onClick={() => setWrap(!wrap)}>自动换行</Button>{options}</>} status={`${data.encoding.toUpperCase()} · ${data.lines.length} 行${data.truncated ? ' · 已达到预览上限，完整内容请下载原件。' : ''}`}>
    <div ref={viewport} className="absolute inset-0 overflow-auto p-3" tabIndex={0} aria-label="文本内容"><div className="font-mono leading-6" style={{ fontSize: `${14 * zoom}px` }}>{data.lines.slice((page - 1) * size, page * size).map((line, index) => <div key={(page - 1) * size + index} data-line={(page - 1) * size + index} className="flex min-h-6"><span aria-hidden="true" className="mr-4 w-12 shrink-0 select-none text-right text-muted-foreground">{(page - 1) * size + index + 1}</span><div className={wrap ? 'min-w-0 flex-1 whitespace-pre-wrap break-all' : 'whitespace-pre'}><Highlight text={line || ' '} query={search} /></div></div>)}</div></div>
  </PreviewFrame>;
}

function TableReader({ title, data, options }: { title: string; data: Extract<ParsedPreview, { kind: 'table' }>; options: React.ReactNode }) {
  const [sheetIndex, setSheetIndex] = useState(0);
  const sheet = data.sheets[sheetIndex];
  if (!sheet) return <p role="alert">工作表不存在。</p>;
  return <SheetReader key={sheetIndex} title={title} sheet={sheet} warnings={data.warnings} encoding={data.encoding} options={<>{data.sheets.length > 1 && <select className={selectClass} aria-label="工作表" value={sheetIndex} onChange={event => setSheetIndex(Number(event.target.value))}>{data.sheets.map((sheet, index) => <option key={sheet.name} value={index}>{sheet.name}{sheet.hidden ? '（隐藏表）' : ''}</option>)}</select>}{options}</>} />;
}

function SheetReader({ title, sheet, warnings, encoding, options }: { title: string; sheet: PreviewSheet; warnings: string[]; encoding?: string; options: React.ReactNode }) {
  const [page, setPage] = useState(1), [query, setQuery] = useState(''), [zoom, setZoom] = useState(1), [selection, setSelection] = useState<{ row: number; column: number }>();
  const [sort, setSort] = useState<{ column: number; descending: boolean }>();
  const search = useDeferredValue(query), size = 50, scroll = useRef<HTMLDivElement>(null);
  const rows = useMemo(() => {
    const values = sheet.rows.map((cells, row) => ({ cells, row })).filter(item => !search || item.cells.some(cell => cell.text.toLocaleLowerCase().includes(search.toLocaleLowerCase()) || cell.formula?.toLocaleLowerCase().includes(search.toLocaleLowerCase())));
    if (sort) values.sort((a, b) => (a.cells[sort.column]?.text ?? '').localeCompare(b.cells[sort.column]?.text ?? '', 'zh-CN', { numeric: true }) * (sort.descending ? -1 : 1));
    return values;
  }, [sheet, search, sort]);
  const count = Math.max(1, Math.ceil(rows.length / size)), activePage = Math.min(page, count), visible = rows.slice((activePage - 1) * size, activePage * size);
  const merged = useMemo(() => mergedCells(search || sort ? { ...sheet, merges: [] } : sheet, activePage, size), [sheet, search, sort, activePage]);
  const selected = selection && (merged.get(`${selection.row}:${selection.column}`)?.cell ?? sheet.rows[selection.row]?.[selection.column]);
  const selectionVisible = selection && visible.some(item => item.row === selection.row) && merged.get(`${selection.row}:${selection.column}`) !== null;
  const moveCell = (row: number, column: number, key: string) => {
    const index = rows.findIndex(item => item.row === row), delta = key === 'ArrowDown' ? 1 : key === 'ArrowUp' ? -1 : 0;
    const nextIndex = Math.max(0, Math.min(rows.length - 1, index + delta)), nextRow = rows[nextIndex]?.row;
    const nextColumn = Math.max(0, Math.min(sheet.columns - 1, column + (key === 'ArrowRight' ? 1 : key === 'ArrowLeft' ? -1 : 0)));
    if (nextRow === undefined) return;
    setSelection({ row: nextRow, column: nextColumn }); setPage(Math.floor(nextIndex / size) + 1);
    requestAnimationFrame(() => scroll.current?.querySelector<HTMLButtonElement>(`[data-cell="${nextRow}:${nextColumn}"]`)?.focus());
  };
  return <PreviewFrame title={title} tools={<>{options}<PageControls page={activePage} count={count} onPage={value => { setPage(value); scroll.current?.scrollTo(0, 0); }} /><SearchControl value={query} onChange={value => { setQuery(value); setPage(1); }} count={`${rows.length} 行`} /><ZoomControls zoom={zoom} onZoom={setZoom} />{sort && <Button size="sm" variant="ghost" onClick={() => setSort(undefined)}>原始顺序</Button>}</>} status={<><span>{sheet.name} · {sheet.rowCountIsLowerBound ? '至少 ' : ''}{sheet.totalRows} 行 × {sheet.totalColumns} 列{encoding ? ` · ${encoding.toUpperCase()}` : ' · 公式显示文件中已保存的结果，不重新计算。'}</span>{warnings.length > 0 && <details className="mt-1"><summary className="cursor-pointer">部分内容超过预览范围，查看 {warnings.length} 项说明</summary><div className="max-h-20 overflow-auto">{warnings.map(warning => <p key={warning}>{warning}</p>)}</div></details>}</>}>
    <div className="flex h-full min-w-0 flex-col">
      <div className="max-h-24 shrink-0 overflow-auto border-b bg-muted/20 px-3 py-2 text-xs" aria-label="单元格详情">{selection && selected ? <><strong>{columnName(selection.column)}{selection.row + 1}</strong><span className="ml-3 whitespace-pre-wrap break-all">{selected.text || '（空）'}</span>{selected.formula && <p className="mt-1 break-all font-mono">公式：={selected.formula}</p>}</> : '选择单元格查看完整内容；点击列标可排序。'}</div>
      <div ref={scroll} className="min-h-0 flex-1 overflow-auto" tabIndex={0} aria-label="表格内容">
        <table className="border-separate border-spacing-0 text-left" aria-label={`${sheet.name} 数据`} style={{ fontSize: `${13 * zoom}px` }}><thead className="sticky top-0 z-10 bg-muted"><tr><th className="border-b border-r px-3 py-2" scope="col">行</th>{Array.from({ length: sheet.columns }, (_, column) => <th key={column} scope="col" className="border-b border-r" aria-sort={sort?.column === column ? sort.descending ? 'descending' : 'ascending' : 'none'}><button className="min-h-10 w-full px-3 text-center focus-visible:outline-2" aria-label={`按 ${columnName(column)} 列排序`} onClick={() => { setSort({ column, descending: sort?.column === column && !sort.descending }); setPage(1); }}>{columnName(column)}{sort?.column === column ? sort.descending ? ' ↓' : ' ↑' : ''}</button></th>)}</tr></thead><tbody>{visible.map(({ cells, row }) => <tr key={row}><th scope="row" className="sticky left-0 border-b border-r bg-muted px-3 py-2 text-xs tabular-nums">{row + 1}</th>{Array.from({ length: sheet.columns }, (_, column) => {
          const span = merged.get(`${row}:${column}`); if (span === null) return null;
          const selected = selection?.row === row && selection.column === column;
          return <td key={column} rowSpan={span?.rows} colSpan={span?.columns} className={`max-w-96 min-w-28 border-b border-r p-0 align-top ${selected ? 'bg-accent ring-2 ring-inset ring-primary' : 'bg-background'}`}><button data-cell={`${row}:${column}`} tabIndex={selected || !selectionVisible && row === visible[0]?.row && column === 0 ? 0 : -1} className="min-h-10 w-full px-3 py-2 text-left align-top focus-visible:outline-2" aria-label={`单元格 ${columnName(column)}${row + 1}`} onClick={() => setSelection({ row, column })} onKeyDown={event => { if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(event.key)) { event.preventDefault(); moveCell(row, column, event.key); } }}><span className="line-clamp-3 whitespace-pre-wrap break-words"><Highlight text={(span?.cell ?? cells[column])?.text || '\u00a0'} query={search} /></span></button></td>;
        })}</tr>)}</tbody></table>{!rows.length && <p className="p-5 text-sm text-muted-foreground">{query ? '没有匹配的单元格。' : '此工作表为空。'}</p>}
      </div>
    </div>
  </PreviewFrame>;
}
