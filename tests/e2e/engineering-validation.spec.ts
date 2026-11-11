import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';
import type { EngineeringField } from '../../packages/ts/api-client/src/engineering';
import { validateFields } from '../../apps/console/src/features/engineering/validation';
test('shared engineering corpus matches the server field rules without coercion', () => {
 const cases = JSON.parse(readFileSync('tests/fixtures/engineering-values.json', 'utf8')) as { name: string; fields: EngineeringField[]; values: Record<string, unknown>; valid: boolean }[];
 for (const c of cases) { const before = JSON.stringify(c.values); expect(validateFields(c.fields, c.values, '/sample/values').length === 0, c.name).toBe(c.valid); expect(JSON.stringify(c.values)).toBe(before); }
});
