import type { ApiClient, EngineeringCategory } from '@infrapro/api-client';

export const unassigned = '__unassigned';
export const statusOptions = [
  { value: '', label: '全部状态' }, { value: 'usable', label: '可新使用' },
  { value: 'unavailable', label: '不可新使用' }, { value: 'draft', label: '存在草稿' },
];

export type CatalogUnit = {
  id: string;
  code: string;
  name: string;
  symbol: string;
  category: string;
  latest: number;
  has_draft: boolean;
  usable: boolean;
};
export type CategoryGroup = { category: EngineeringCategory; units: CatalogUnit[]; matches: CatalogUnit[] };
// Display order for the card index only. The platform category key still owns every unit.
const topics = [
  { name: '几何与运动', keys: ['length', 'area', 'volume', 'plane_angle', 'velocity', 'acceleration', 'reciprocal_length', 'sbm_length'] },
  { name: '力学与材料', keys: ['force', 'pressure_stress', 'strain', 'mass', 'mass_density', 'moment_of_force'] },
  { name: '电学与能量', keys: ['electric_current', 'electric_potential', 'electric_resistance', 'electrical_resistivity', 'current_density', 'power', 'energy'] },
  { name: '热学与流体', keys: ['temperature', 'temperature_gradient', 'volume_flow_rate', 'mass_concentration', 'ph_scale', 'turbidity'] },
  { name: '光学与声学', keys: ['illuminance', 'luminance', 'irradiance', 'radiant_exposure', 'sound_level'] },
  { name: '时间、计数与评价', keys: ['duration', 'frequency', 'count', 'count_rate', 'dimensionless_ratio', 'assessment_score', 'pendulum_scale'] },
];
const topicByKey = new Map(topics.flatMap(topic => topic.keys.map(key => [key, topic.name] as const)));
const cardOrder = new Map(topics.flatMap(topic => topic.keys.map((key, index) => [key, index] as const)));
export function cardSections(groups: CategoryGroup[], searching = false) {
  const buckets = new Map<string, CategoryGroup[]>();
  for (const group of groups) {
    const name = topicByKey.get(group.category.key) ?? '其他类别';
    if (!buckets.has(name)) buckets.set(name, []);
    buckets.get(name)!.push(group);
  }
  // Search results keep the exact-match ranking of filterGroups.
  const names = searching ? [...buckets.keys()] : [...topics.map(t => t.name), '其他类别'];
  return names.flatMap(name => {
    const items = buckets.get(name);
    if (!items) return [];
    if (!searching) items.sort((a, b) => (cardOrder.get(a.category.key) ?? 0) - (cardOrder.get(b.category.key) ?? 0));
    return [{ name, groups: items }];
  });
}
const byName = new Intl.Collator('zh-CN', { numeric: true });

export async function readUnitIndex(client: ApiClient, signal: AbortSignal): Promise<CatalogUnit[]> {
  const units = new Map<string, CatalogUnit>();
  let offset = 0;
  for (;;) {
    const page = await client.engineeringList({ kind: 'units' }, { limit: 100, offset }, signal);
    for (const item of page.items) {
      units.set(item.id, {
        id: item.id, code: item.code, name: item.name || '未命名草稿',
        symbol: item.unit?.symbol ?? '', category: item.unit?.category ?? '',
        latest: item.latest, has_draft: item.has_draft, usable: item.usable,
      });
    }
    offset += page.items.length;
    if (!page.items.length || offset >= page.total) return [...units.values()];
  }
}

export function groupUnits(categories: EngineeringCategory[], units: CatalogUnit[]) {
  const groups = new Map<string, CategoryGroup>();
  for (const category of categories) groups.set(category.key, { category, units: [], matches: [] });
  for (const unit of units) {
    const key = groups.has(unit.category) ? unit.category : unassigned;
    if (!groups.has(key)) groups.set(key, {
      category: { key, name: '待归类', description: '这些单位尚未归入现有类别。', used: false, revision: 0 },
      units: [], matches: [],
    });
    groups.get(key)!.units.push(unit);
  }
  for (const group of groups.values()) {
    group.units.sort((a, b) => byName.compare(a.name, b.name) || byName.compare(a.code, b.code));
    group.matches = group.units;
  }
  return [...groups.values()].sort((a, b) =>
    b.units.length - a.units.length || byName.compare(a.category.name, b.category.name));
}

export const unitStatus = (unit: CatalogUnit) => unit.usable ? '可新使用' : unit.latest ? '不可新使用' : '待发布';

function exactUnit(unit: CatalogUnit, term: string) {
  return [unit.name, unit.symbol, unit.code].some(value => value.toLowerCase() === term);
}
export function filterUnits(units: CatalogUnit[], term: string, status: string, categoryHit = false) {
  const matches = units.filter(unit =>
    (categoryHit || [unit.name, unit.symbol, unit.code].some(value => value.toLowerCase().includes(term))) &&
    (!status || (status === 'usable' && unit.usable) || (status === 'unavailable' && !unit.usable) ||
      (status === 'draft' && unit.has_draft)));
  if (term) matches.sort((a, b) => Number(exactUnit(b, term)) - Number(exactUnit(a, term)));
  return matches;
}

export function filterGroups(groups: CategoryGroup[], query: string, status: string) {
  const term = query.trim().toLowerCase();
  const includes = (value: string) => value.toLowerCase().includes(term);
  const matched = groups.flatMap(group => {
    const categoryHit = includes(group.category.name) || (group.category.key !== unassigned && includes(group.category.key));
    const matches = filterUnits(group.units, term, status, categoryHit);
    return matches.length || (categoryHit && !status) ? [{ ...group, matches }] : [];
  });
  if (term) {
    const rank = (g: CategoryGroup) => {
      if ([g.category.name, g.category.key].some(value => value.toLowerCase() === term)) return 2;
      return g.matches.some(u => exactUnit(u, term)) ? 1 : 0;
    };
    matched.sort((a, b) => rank(b) - rank(a));
  }
  return matched;
}
