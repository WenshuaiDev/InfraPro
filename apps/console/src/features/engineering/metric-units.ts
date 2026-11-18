import type { EngineeringMetric, EngineeringSummary, EngineeringVersion } from '@infrapro/api-client';

export type UnitChoice = NonNullable<EngineeringSummary['unit_versions']>[number];

export function fixedUnit(version: EngineeringVersion): UnitChoice | undefined {
  const unit = version.content.unit;
  if (version.kind === 'units' && unit) return { id: version.id, number: version.number, name: version.content.name, ...unit };
}

export const unitLabel = (unit: UnitChoice) => `${unit.name}（${unit.symbol}）· V${unit.number}`;

export function metricUnitOptions(metric: EngineeringMetric | undefined, references: EngineeringVersion[], current = '') {
  if (!metric || metric.shape === 'enum') return [];
  return (metric.allowed_units ?? []).flatMap(id => {
    const version = references.find(ref => ref.id === id && ref.kind === 'units');
    const unit = version && fixedUnit(version);
    if (!version || !unit || unit.category !== metric.category || (!version.usable && id !== current)) return [];
    return [{ value: id, label: unitLabel(unit) + (id === metric.standard_unit ? ' · 标准' : '') + (version.usable ? '' : ' · 已不可新采用') }];
  });
}

export function defaultMetricUnit(version?: EngineeringVersion, references: EngineeringVersion[] = []) {
  const rule = version?.content.metric;
  return rule && metricUnitOptions(rule, references).some(option => option.value === rule.standard_unit) ? rule.standard_unit : '';
}
