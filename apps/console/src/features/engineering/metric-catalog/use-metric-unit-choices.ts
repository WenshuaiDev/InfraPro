import { useState } from 'react';
import type { EngineeringVersion } from '@infrapro/api-client';
import { fixedUnit, type UnitChoice } from '../metric-units';

export function useMetricUnitChoices(references: EngineeringVersion[] = []) {
  const [picked, setPicked] = useState<UnitChoice[]>([]);
  const known = new Map<string, UnitChoice>(picked.map(unit => [unit.id, unit]));
  for (const reference of references) {
    const unit = fixedUnit(reference);
    if (unit) known.set(unit.id, unit);
  }
  const remember = (unit: UnitChoice) => setPicked(current => [...current.filter(item => item.id !== unit.id), unit]);
  return { known, remember };
}
