import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import type { ResourceTreeFilter, ResourceTreeRef } from '@infrapro/api-client';
import { useResources } from './context';

export function useResourceTree(filter: ResourceTreeFilter, enabled = true) {
  const { client, scope, prefix, canRead } = useResources();
  const query = usePagedQuery({
    queryKey: [...prefix, 'tree', filter],
    queryFn: ({ cursor, limit, signal }) => client.resourceTree(scope, { ...filter, limit, cursor }, signal),
    initialPageSize: 25,
    enabled: enabled && canRead,
    retry: false,
    staleTime: 30_000,
  });
  return { ...query, items: query.data?.items ?? [], summary: query.data };
}
export function childFilter(node: ResourceTreeRef): ResourceTreeFilter {
  return { facility_id: node.facility_id, parent_kind: node.kind === 'channel' ? 'point' : node.kind, parent_id: node.id, status: 'all' };
}
export { navigationRef, navigationPath } from './resource-navigation';
