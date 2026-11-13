import type { ResourceTreeFilter, ResourceTreeNode, ResourceTreePage } from '@infrapro/api-client';
import type { useResources } from './context';
import { navigationRef } from './resource-navigation';

// Project navigation must use project-scoped metadata APIs, never the tenant tree.
export async function projectTreePage(context: Pick<ReturnType<typeof useResources>, 'client' | 'scope'>, filter: ResourceTreeFilter, signal: AbortSignal): Promise<ResourceTreePage> {
  const { client, scope } = context;
  const cursors: Record<string, string | null> = filter.cursor ? JSON.parse(filter.cursor) : {};
  const kinds = !filter.parent_kind ? ['facility'] as const : filter.parent_kind === 'point' ? ['channel'] as const : ['component', 'point', 'channel'] as const;
  const pages = await Promise.all(kinds.map(async kind => {
    if (cursors[kind] === null) return { kind, items: [], next_cursor: '' };
    const page = await client.resourceList(scope, { kind, facility: filter.facility_id, point: kind === 'channel' && filter.parent_kind === 'point' ? filter.parent_id : undefined }, {
      q: filter.q, status: 'all', limit: 25, cursor: cursors[kind] ?? undefined,
      ...(kind === 'component' ? { parent_id: filter.parent_kind === 'facility' ? 'root' : filter.parent_id } : {}),
      ...(kind === 'channel' ? { owner_kind: filter.parent_kind, owner_id: filter.parent_id } : {}),
      ...(kind === 'point' ? { owner: filter.parent_kind === 'facility' ? 'root' : filter.parent_id } : {}),
    }, signal);
    return { kind, ...page };
  }));
  const items: ResourceTreeNode[] = pages.flatMap(page => page.items.map(item => {
    const resource = item;
    return { ...navigationRef(resource, filter.facility_id), created_at: 'created_at' in resource ? resource.created_at : '', has_children: resource.kind !== 'channel', ancestors: [], type_definition_id: '', type_name: '', type_number: 0, metric_name: '', unit_symbol: '' };
  }));
  const next = Object.fromEntries(pages.map(page => [page.kind, page.next_cursor || null]));
  return { items, next_cursor: Object.values(next).some(Boolean) ? JSON.stringify(next) : '', total: items.length, counts: { facility: 0, component: 0, point: 0, channel: 0 }, types: [] };
}
