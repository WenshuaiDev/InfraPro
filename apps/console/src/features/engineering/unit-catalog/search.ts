export type UnitCatalogSearch = {
  category?: string;
  q?: string;
  topic?: string;
  status?: string;
  within?: string;
  withinStatus?: string;
};

export function unitCatalogSearch(search: Record<string, unknown>): UnitCatalogSearch {
  const text = (value: unknown) => typeof value === 'string' ? value.slice(0, 200) || undefined : undefined;
  const status = (value: unknown) => value === 'draft' || value === 'usable' || value === 'unavailable' ? value : undefined;
  return {
    category: text(search.category),
    q: text(search.q),
    topic: text(search.topic),
    status: status(search.status),
    within: text(search.within),
    withinStatus: status(search.withinStatus),
  };
}
