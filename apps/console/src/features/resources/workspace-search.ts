import type { ResourceTreeFilter, ResourceTreeRef, ResourceTarget } from '@infrapro/api-client';

export type FacilityWorkspaceSearch = {
  q?: string;
  type?: string;
  status?: 'normal' | 'retired';
  deleted?: 'only';
  component?: string;
  monitoring_section?: string;
  borehole?: string;
  point?: string;
  channel?: string;
  directory?: true;
  view?: 'details' | 'children' | 'source' | 'bindings' | 'usage' | 'changes';
};
const id = (value: unknown) => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value) ? value : undefined;
export function facilityWorkspaceSearch(raw: Record<string, unknown>): FacilityWorkspaceSearch {
  const component = id(raw.component), point = id(raw.point), channel = id(raw.channel);
  let view: FacilityWorkspaceSearch['view'];
  if (typeof raw.view === 'string' && ['details', 'children', 'source', 'bindings', 'usage', 'changes'].includes(raw.view)) view = raw.view as FacilityWorkspaceSearch['view'];
  // Existing object links remain usable; tabs no longer discard the current object.
  else if (raw.tab === 'changes' || raw.tab === 'usage') view = raw.tab;
  else if (raw.tab === 'components' && !component || raw.tab === 'points' && !point || raw.tab === 'channels' && !channel) view = 'children';
  return { q: typeof raw.q === 'string' ? raw.q.slice(0, 200) || undefined : undefined, type: id(raw.type), status: raw.status === 'normal' || raw.status === 'retired' ? raw.status : undefined, deleted: raw.deleted === 'only' ? 'only' : undefined, component, monitoring_section:id(raw.monitoring_section),borehole:id(raw.borehole), point, channel, directory: raw.directory === true || raw.directory === 'true' ? true : undefined, view };
}
export function treeFilter(search: FacilityWorkspaceSearch): ResourceTreeFilter {
  return { q: search.q?.trim() || undefined, type_definition_id: search.type, status: search.deleted ? 'all' : search.status ?? 'all', deleted: search.deleted };
}
export function resourceSelection(node: Pick<ResourceTreeRef, 'id' | 'kind' | 'parent_id' | 'parent_kind' | 'deleted_at'>): Pick<FacilityWorkspaceSearch, 'component' | 'monitoring_section' | 'borehole' | 'point' | 'channel' | 'view'> {
  return {monitoring_section:node.kind==='monitoring_section'?node.id:undefined,borehole:node.kind==='borehole'?node.id:undefined, component: node.kind === 'component' ? node.id : undefined, point: node.kind === 'point' ? node.id : node.kind === 'channel' && node.parent_kind === 'point' ? node.parent_id ?? undefined : undefined, channel: node.kind === 'channel' ? node.id : undefined, view: node.deleted_at ? 'changes' : undefined };
}
export function selectedTarget(facility: string, search: FacilityWorkspaceSearch): ResourceTarget {
  return search.channel ? { kind: 'channel', facility, point: search.point, id: search.channel } : search.point ? { kind: 'point', facility, id: search.point } : search.component ? { kind: 'component', facility, id: search.component } : search.monitoring_section ? {kind:'monitoring_section',facility,id:search.monitoring_section} : search.borehole ? {kind:'borehole',facility,id:search.borehole} : { kind: 'facility', facility, id: facility };
}
