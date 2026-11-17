import type {
  SpatialAsset,
  SpatialLocation,
  SpatialObjectRef,
} from "@infrapro/api-client";

/** A facility owns its assets; shared reference layers are an explicit alternative. */
export function facilityAssets(
  assets: SpatialAsset[],
  facility?: string,
  shared = false,
) {
  return assets
    .filter((asset) =>
      shared
        ? asset.kind === "geolayer" && !asset.facility_id
        : !!facility && asset.facility_id === facility,
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
}

/** An ancestor's asset and a historical placement are not a placement in the active version. */
export function directLocations(
  locations: SpatialLocation[],
  owner?: SpatialObjectRef,
  version?: string,
) {
  return owner && owner.kind !== "facility" && version
    ? locations.filter(
        (location) =>
          location.version_id === version &&
          location.owner.kind === owner.kind &&
          location.owner.id === owner.id,
      )
    : [];
}

export async function allSpatialPages<T>(
  read: (cursor?: string) => Promise<{ items: T[]; next_cursor?: string }>,
) {
  const items: T[] = [],
    cursors = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await read(cursor);
    items.push(...page.items);
    cursor = page.next_cursor || undefined;
    if (cursor && cursors.has(cursor))
      throw new Error("空间目录分页重复，请重试。");
    if (cursor) cursors.add(cursor);
  } while (cursor);
  return { items };
}
