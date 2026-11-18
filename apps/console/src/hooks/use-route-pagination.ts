import { useQuery } from '@tanstack/react-query';
import { indexCursorPages, type Bookmark, type CursorPage } from '@infrapro/ui/lib/cursor-pagination';
import type { ListPage } from '@infrapro/ui/patterns/list-controls';

// These screens keep opaque cursors in the URL so reload, back and detail links
// continue to open the same slice of the authorized result.
export function useRoutePagination<P extends CursorPage>({ key, data, cursor, trail, load, loadIndex, move, pageSize, setPageSize, fetching }: {
  key: readonly unknown[]; data?: P; cursor?: string; trail: (string | null)[];
  load: (cursor: string, signal: AbortSignal) => Promise<P>;
  loadIndex?: (signal: AbortSignal) => Promise<{ total: number; bookmarks: Bookmark[] }>;
  move: (cursor: string | undefined, trail: (string | null)[]) => void;
  pageSize: number; setPageSize: (size: number) => void; fetching: boolean;
}): ListPage {
  const index = useQuery({
    queryKey: [...key, 'page-index', pageSize],
    enabled: !!data && !!(cursor || data.next_cursor),
    refetchInterval: 60_000,
    queryFn: async ({ signal }) => loadIndex ? loadIndex(signal) : indexCursorPages(cursor ? await load('', signal) : data!, load, signal),
    staleTime: 60_000, gcTime: 60_000, retry: false,
  });
  const bookmarks = index.data?.bookmarks ?? [];
  const known = !cursor ? 0 : trail.length || bookmarks.findIndex(b => b.cursor === cursor) + 1;
  const tail = useQuery({
    queryKey: [...key, 'linked-page', cursor, pageSize],
    enabled: !!cursor && !!data && !!index.data && !known,
    queryFn: ({ signal }) => indexCursorPages(data!, load, signal),
    staleTime: 60_000, gcTime: 60_000, retry: false,
  });
  const total = data?.total ?? (!cursor && data && !data.next_cursor ? data.items?.length ?? 0 : index.data?.total);
  const page = known || (tail.data && total !== undefined ? Math.max(0, Math.floor((total - tail.data.total) / pageSize)) : 0);
  const positions = [null, ...bookmarks.map(b => b.cursor)];
  const go = (n: number) => {
    if (n === page) return;
    if (!n) return move(undefined, []);
    if (n < trail.length) return move(trail[n] ?? undefined, trail.slice(0, n));
    if (n === page + 1 && data?.next_cursor) return move(data.next_cursor, [...(trail.length === page ? trail : positions.slice(0, page)), cursor ?? null]);
    if (positions[n]) move(positions[n]!, positions.slice(0, n));
  };
  return { total, page, pageSize, setPageSize, fetching, hasNext: !!data?.next_cursor, go, countError: index.isError, retryCount: () => void index.refetch() };
}
