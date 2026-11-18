export type CursorPage = { items: unknown[] | null; next_cursor?: string | null; total?: number | null };
export type Bookmark = { offset: number; cursor: string };
type Read<P> = (cursor: string, signal: AbortSignal) => Promise<P>;

// An index contains only offsets and opaque server cursors. Scanning a legacy
// endpoint for its exact total must not retain every record in the browser.
export async function indexCursorPages<P extends CursorPage>(first: P, read: Read<P>, signal: AbortSignal) {
  const bookmarks: Bookmark[] = [];
  const seen = new Set<string>();
  let total = first.items?.length ?? 0;
  let cursor = first.next_cursor;
  while (cursor) {
    signal.throwIfAborted();
    if (seen.has(cursor)) throw new Error('分页游标重复，请刷新列表。');
    seen.add(cursor);
    bookmarks.push({ offset: total, cursor });
    const chunk = await read(cursor, signal);
    total += chunk.items?.length ?? 0;
    cursor = chunk.next_cursor;
  }
  return { total, bookmarks };
}

export async function readCursorWindow<P extends CursorPage>(first: P, read: Read<P>, signal: AbortSignal, offset: number, size: number, bookmarks: Iterable<Bookmark>, remember?: (entry: Bookmark) => void) {
  let start = 0;
  let chunk = first;
  let bookmark: Bookmark | undefined;
  for (const entry of bookmarks) {
    if (entry.offset <= offset && (!bookmark || entry.offset >= bookmark.offset)) bookmark = entry;
  }
  if (bookmark) {
    start = bookmark.offset;
    signal.throwIfAborted();
    chunk = await read(bookmark.cursor, signal);
  }
  const seen = new Set(bookmark ? [bookmark.cursor] : []);
  let metadata = chunk;
  const items: unknown[] = [];
  while (true) {
    signal.throwIfAborted();
    const rows = chunk.items ?? [];
    const next = chunk.next_cursor;
    if (next && seen.has(next)) throw new Error('分页游标重复，请刷新列表。');
    if (next) remember?.({ offset: start + rows.length, cursor: next });
    const skip = Math.max(0, offset - start);
    if (skip < rows.length) {
      if (!items.length) metadata = chunk;
      const count = Math.min(size - items.length, rows.length - skip);
      items.push(...rows.slice(skip, skip + count));
      if (items.length === size) {
        return { data: { ...metadata, items } as P, hasNext: skip + count < rows.length || !!chunk.next_cursor };
      }
    }
    if (!next) return { data: { ...metadata, items } as P, hasNext: false };
    seen.add(next);
    start += rows.length;
    chunk = await read(next, signal);
  }
}
