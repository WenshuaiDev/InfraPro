import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';

export function useCursorList<T>({ queryKey, load, enabled = true }: {
  queryKey: readonly unknown[];
  load: (cursor: string, signal: AbortSignal) => Promise<{ items: T[]; next_cursor: string }>;
  enabled?: boolean;
}) {
  const key = JSON.stringify(queryKey);
  const current = useRef(key);
  useEffect(() => { current.current = key; return () => { current.current = ''; }; }, [key]);
  const [position, setPosition] = useState({ key, page: 0 });
  const query = useInfiniteQuery({ queryKey, initialPageParam: '', queryFn: ({ pageParam, signal }) => load(pageParam, signal), getNextPageParam: (page) => page.next_cursor || undefined, enabled });
  const pages = query.data?.pages ?? [];
  let page = position.key === key ? Math.min(position.page, Math.max(0, pages.length - 1)) : 0;
  while (page > 0 && !query.isFetching && !pages[page]?.items.length) page--;
  const go = (next: number) => setPosition({ key, page: next });
  const next = async () => {
    if (page + 1 < pages.length) { go(page + 1); return; }
    if (!query.hasNextPage || query.isFetching) return;
    const result = await query.fetchNextPage();
    if (current.current === key && !result.isError && result.data?.pages[page + 1]) go(page + 1);
  };
  return { ...query, items: pages[page]?.items ?? [], page, pageCount: pages.length, hasNext: page + 1 < pages.length || query.hasNextPage, go, next };
}

export const useRoleList = useCursorList;
