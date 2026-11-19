import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';

type Page<T> = { items: T[]; next_cursor?: string | null; total?: number | null; counts?: Record<string, number> };

export function useCursorList<T>({ queryKey, load, enabled = true, paused, staleTime, refetchInterval, pageSize, initialPageSize }: {
  queryKey: readonly unknown[];
  load: (cursor: string, signal: AbortSignal, limit: number) => Promise<Page<T>>;
  enabled?: boolean;
  paused?: boolean;
  staleTime?: number;
  refetchInterval?: number;
  pageSize?: number;
  initialPageSize?: number;
}) {
  const query = usePagedQuery({
    queryKey, enabled, paused, staleTime, pageSize, initialPageSize, refetchInterval,
    queryFn: ({ cursor, signal, limit }) => load(cursor, signal, limit),
  });
  return { ...query, ...query.pagination, items: query.data?.items ?? [], hasNext: !!query.pagination.hasNext };
}
