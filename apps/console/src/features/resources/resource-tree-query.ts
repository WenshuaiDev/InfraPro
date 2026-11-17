import { useInfiniteQuery } from '@tanstack/react-query';
import type { ResourceTreeFilter, ResourceTreeRef } from '@infrapro/api-client';
import { useResources } from './context';

export function useResourceTree(filter: ResourceTreeFilter, enabled = true) {
  const { client, scope, prefix, canRead } = useResources();
  const query = useInfiniteQuery({
    queryKey: [...prefix, 'tree', filter],
    queryFn: ({ pageParam, signal }) => client.resourceTree(scope, { ...filter, limit: 25, cursor: pageParam }, signal),
    initialPageParam: '',
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: enabled && canRead,
    retry: false,
    staleTime: 30_000,
  });
  return { ...query, items: query.data?.pages.flatMap((p) => p.items) ?? [], summary: query.data?.pages[0] };
}
export function childFilter(node: ResourceTreeRef): ResourceTreeFilter {
  return { facility_id: node.facility_id, parent_kind: node.kind === 'channel' ? 'point' : node.kind, parent_id: node.id, status: 'all' };
}
export { navigationRef, navigationPath } from './resource-navigation';
