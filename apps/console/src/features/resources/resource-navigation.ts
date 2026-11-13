import type { Resource, ResourceContext, ResourceDetail, ResourceTreeRef } from '@infrapro/api-client';

export function navigationRef(resource: Resource | ResourceContext, facility?: string): ResourceTreeRef {
  const ownerKind = 'owner_kind' in resource ? resource.owner_kind : undefined;
  const facilityID = resource.kind === 'facility' ? resource.id : resource.facility_id ?? facility!;
  const parentID = resource.kind === 'facility' ? null : resource.kind === 'component' ? resource.parent_component_id ?? facilityID : resource.kind === 'point' ? resource.component_id ?? facilityID : ownerKind === 'facility' ? facilityID : ownerKind === 'component' ? resource.component_id! : resource.point_id!;
  const parentKind = resource.kind === 'facility' ? null : resource.kind === 'channel' ? ownerKind ?? 'point' : parentID === facilityID ? 'facility' : 'component';
  return { id: resource.id, kind: resource.kind, facility_id: facilityID, parent_kind: parentKind, parent_id: parentID, name: resource.name, code: resource.code, status: resource.status, deleted_at: 'deleted_at' in resource ? resource.deleted_at ?? null : null };
}
export function navigationPath(resource: ResourceDetail) {
  const facility = resource.facility_id ?? resource.id;
  return [...resource.ancestors, resource].map((r) => navigationRef(r, facility));
}
