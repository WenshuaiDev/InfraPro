import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { read, write } from './architecture-lab.mjs';

export const skipReason = '按用户要求跳过';
export const scopePath = 'scope/functional-acceptance.json';
export const performanceArcs = ['ARC-037', 'ARC-038'];
export const performanceCommands = new Set([
  'test:ingestion:load', 'test:derived:load', 'test:derived:pipeline', 'test:derived:soak',
]);
export const performanceStages = {
  'test:monitoring:contract': ['TestInitialCapacity'],
  'test:monitoring': ['TestInitialCapacity'],
  'test:display:load': ['TestDisplayReadLoadTenInstances'],
};
const historicalLoadSteps = new Set([
  'immutable-display-load-evidence', 'ingestion-test-databases', 'ingestion-test-login-cache',
  'ingestion-nine-rounds', 'display-eighteen-rounds',
]);

export function registerFunctionalScope(dir) {
  const path = join(dir, scopePath);
  if (existsSync(path)) return read(path);
  const record = {
    scope: 'functional', recordedAt: new Date().toISOString(), reason: skipReason,
    required: '全部功能自验通过，提供可独立操作的两套人工验收环境',
    skippedArcs: performanceArcs, skippedCommands: [...performanceCommands],
    skippedStages: performanceStages, historicalEvidence: ['ingestion/campaigns/', 'load/display/campaigns/', 'regression/'],
  };
  write(path, record);
  return record;
}

export function skippedOriginal(dir, side, name) {
  const path = 'regression/' + side + '/' + name.replaceAll(':', '-') + '.json';
  const previous = existsSync(join(dir, path)) ? read(join(dir, path)) : undefined;
  return {
    command: ['pnpm', name], status: 'skipped', skipReason, scope: scopePath,
    ...(previous && { priorStatus: previous.status, priorRelease: previous.provenance?.releaseSHA256, previousRecord: path }),
  };
}

export function satisfied(row) {
  if (row.status === 'passed') return true;
  if (row.status !== 'skipped' || row.skipReason !== skipReason) return false;
  if (row.id) return performanceArcs.includes(row.id);
  if (row.suite === 'load' && historicalLoadSteps.has(row.label)) return true;
  const command = Array.isArray(row.command) ? row.command[1] : row.command;
  return performanceCommands.has(command);
}
