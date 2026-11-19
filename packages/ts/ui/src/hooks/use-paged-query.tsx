import { useState } from 'react';
import { hashKey, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { indexCursorPages, readCursorWindow, type Bookmark, type CursorPage } from '../lib/cursor-pagination';
import type { ListPage } from '../patterns/list-controls';

type Options<P extends CursorPage> = Pick<UseQueryOptions<NoInfer<P>>, 'queryKey' | 'enabled' | 'retry' | 'staleTime' | 'gcTime' | 'refetchInterval' | 'refetchOnWindowFocus' | 'refetchOnReconnect' | 'refetchIntervalInBackground'> & {
  queryFn: (params: { cursor: string; signal: AbortSignal; limit: number; snapshot?: string }) => Promise<P>;
  getSnapshot?: (first: NoInfer<P>) => string | undefined;
  resumeSnapshot?: (previous: NoInfer<P>) => string | undefined;
  paused?: boolean;
  initialPageSize?: number;
  pageSize?: number;
  setPageSize?: (size: number) => void;
};

export function usePagedQuery<P extends CursorPage>({ queryKey, queryFn, getSnapshot, resumeSnapshot, paused = false, initialPageSize = 20, pageSize: controlledSize, setPageSize: changeSize, ...options }: Options<P>) {
  const [size, setSize] = useState(initialPageSize);
  const pageSize = controlledSize ?? size;
  const key = [...queryKey, 'pagination', pageSize];
  const identity = hashKey([...key, options.enabled !== false]);
  const initial = { identity, page: 0, requested: 0 };
  const [position, setPosition] = useState(initial);
  if (position.identity !== identity) setPosition(initial);
  const { page, requested } = position.identity === identity ? position : initial;
  const cache = useQueryClient();
  const first = useQuery({ ...options, enabled: paused ? false : options.enabled, queryKey: key, queryFn: ({ signal }) => {
    const previous = cache.getQueryData<P>(key);
    return queryFn({ cursor: '', signal, limit: pageSize, snapshot: previous && resumeSnapshot?.(previous) });
  } });
  // A temporary pause hides all rows and suspends reads while keeping the cursor position.
  const source = paused || options.enabled === false ? undefined : first.data;
  const rows = source?.items ?? [];
  const snapshot = source && getSnapshot?.(source);
  const read = (cursor: string, signal: AbortSignal) => queryFn({ cursor, signal, limit: pageSize, snapshot });
  const snapshotKey = [...key, snapshot ?? null];
  const index = useQuery({
    queryKey: [...snapshotKey, 'index'],
    enabled: !!source?.next_cursor && source.total == null && !first.isError,
    queryFn: async ({ signal }) => ({ ...await indexCursorPages(source!, read, signal), root: source!.next_cursor }),
    retry: false, staleTime: 60_000, gcTime: 60_000,
    refetchInterval: options.refetchInterval ? 60_000 : false,
  });
  // Reuse the cursors discovered by normal navigation on counted endpoints.
  // A changed root cursor or snapshot starts a fresh traversal.
  const root = source?.next_cursor;
  const cursorKey = hashKey([identity, snapshot, root]);
  const [cursorCache, setCursorCache] = useState(() => ({ key: cursorKey, bookmarks: new Map<number, Bookmark>() }));
  const bookmarks = cursorCache.key === cursorKey ? cursorCache.bookmarks : new Map<number, Bookmark>();
  if (cursorCache.key !== cursorKey) setCursorCache({ key: cursorKey, bookmarks });
  const total = source ? source.total ?? (!source.next_cursor ? rows.length : index.data?.total) : undefined;
  const last = total === undefined ? undefined : Math.max(0, Math.ceil(total / pageSize) - 1);
  const target = Math.min(requested, last ?? requested);
  const needsWindow = target > 0 || (!!source?.next_cursor && rows.length < pageSize);
  const windowKey = (n: number) => [...snapshotKey, 'window', root ?? null, n];
  const window = useQuery({
    queryKey: windowKey(target),
    enabled: !!source && needsWindow && !first.isError,
    queryFn: ({ signal }) => readCursorWindow(source!, read, signal, target * pageSize, pageSize,
      index.data && index.data.root === root ? index.data.bookmarks : bookmarks.values(),
      entry => bookmarks.set(entry.offset, entry)),
    retry: false, staleTime: typeof options.staleTime === 'number' ? options.staleTime : 60_000, gcTime: 60_000,
  });
  const firstWindow: { data: P; hasNext: boolean } | undefined = source && { data: { ...source, items: rows.slice(0, pageSize) }, hasNext: rows.length > pageSize || !!source.next_cursor };
  const ready = needsWindow ? window.data : firstWindow;
  const displayed = ready ? target : Math.min(page, last ?? page);
  const fallback = displayed === 0 ? firstWindow : cache.getQueryData<NonNullable<typeof ready>>(windowKey(displayed));
  const view = paused || first.isError ? undefined : ready ?? fallback;
  if (ready && (page !== target || requested !== target)) setPosition({ identity, page: target, requested: target });
  const go = (n: number) => {
    const requested = Math.max(0, Math.min(n, last ?? n));
    if (requested === target && window.isError) void window.refetch();
    setPosition({ identity, page: displayed, requested });
  };
  const setPageSize = (n: number) => { setPosition(initial); (changeSize ?? setSize)(n); };
  const pagination: ListPage = {
    page: displayed, pageSize, total, go, setPageSize,
    fetching: first.isFetching || window.isFetching,
    hasNext: last === undefined ? !!view?.hasNext : displayed < last,
    countError: index.isError, retryCount: () => void index.refetch(),
  };
  const refetch = () => {
    if (source) {
      void cache.invalidateQueries({ queryKey: [...snapshotKey, 'index'] });
      if (!window.isError) void cache.invalidateQueries({ queryKey: [...snapshotKey, 'window'] });
    }
    if (window.isError) {
      if (target === displayed) void window.refetch();
      else setPosition({ identity, page: displayed, requested: displayed });
    }
    return first.refetch();
  };
  const error = first.error ?? window.error;
  return {
    ...first, data: view?.data, error, isError: !!error, isSuccess: first.isSuccess && !error,
    isFetching: !!pagination.fetching, isPending: !view && (first.isPending || window.isFetching), refetch, pagination,
  };
}
